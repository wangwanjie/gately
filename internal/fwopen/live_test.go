package fwopen

import (
	"context"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestLive 在真实防火墙上验证放行、补回与删除，仅在设置 GATELY_FWOPEN_LIVE 时运行。
// 需要 root，应在独立网络命名空间里执行（ip netns exec），并预先在命名空间内配置默认丢弃的防火墙；
// GATELY_FWOPEN_PEER 为命名空间内的地址，测试经 nsenter 从宿主机网络命名空间发起连接检查连通性。
func TestLive(t *testing.T) {
	if os.Getenv("GATELY_FWOPEN_LIVE") == "" {
		t.Skip("未设置 GATELY_FWOPEN_LIVE")
	}
	peer := os.Getenv("GATELY_FWOPEN_PEER")
	const port = 34999
	ln, err := net.Listen("tcp4", ":34999")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	reachable := func() bool {
		cmd := exec.Command("nsenter", "-t", "1", "-n", "timeout", "3", "bash", "-c", "echo > /dev/tcp/"+peer+"/34999")
		return cmd.Run() == nil
	}
	if reachable() {
		t.Fatal("放行前端口就能访问，防火墙没有配置成默认丢弃")
	}
	status, ok := Set("live:t", []Port{{"tcp", port}}, nil)
	t.Logf("Set: ok=%v %s", ok, status)
	if !ok || !reachable() {
		t.Fatal("放行后仍无法访问")
	}

	// 模拟防火墙重新加载：删掉 Gately 的规则，定时对账应当补回
	ctx := context.Background()
	targets, have, err := std.be.targets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tg := range targets {
		for _, e := range have[tg.String()] {
			if err := std.be.del(ctx, tg, e); err != nil {
				t.Fatal(err)
			}
		}
	}
	if reachable() {
		t.Fatal("删除规则后仍能访问")
	}
	std.reconcile(ctx, true)
	t.Logf("补回后: %s", Status("live:t"))
	if !reachable() {
		t.Fatal("对账后没有补回")
	}

	Clear("live:t")
	time.Sleep(200 * time.Millisecond)
	if reachable() {
		t.Fatal("取消放行后仍能访问")
	}
	_, have, _ = std.be.targets(ctx)
	for k, list := range have {
		if len(list) > 0 {
			t.Fatalf("%s 仍有 Gately 规则: %+v", k, list)
		}
	}
}
