package fwopen

import (
	"context"
	"net"
	"os"
	"os/exec"
	"strconv"
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
	status, ok := Set("live:t", []Port{{Proto: "tcp", Port: port}}, nil)
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

// TestLiveFamilies 在真实防火墙上验证地址族、端口区间与目标地址限定，仅在设置 GATELY_FWOPEN_LIVE 时运行。
// GATELY_FWOPEN_PEER / GATELY_FWOPEN_PEER6 为命名空间内的 IPv4 / IPv6 地址，GATELY_FWOPEN_PEER_ALT 为同一命名空间的另一个 IPv4 地址。
func TestLiveFamilies(t *testing.T) {
	if os.Getenv("GATELY_FWOPEN_LIVE") == "" {
		t.Skip("未设置 GATELY_FWOPEN_LIVE")
	}
	p4, p6, alt := os.Getenv("GATELY_FWOPEN_PEER"), os.Getenv("GATELY_FWOPEN_PEER6"), os.Getenv("GATELY_FWOPEN_PEER_ALT")
	for port := 34990; port <= 34995; port++ {
		ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
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
	}
	reach := func(host string, port int) bool {
		cmd := exec.Command("nsenter", "-t", "1", "-n", "timeout", "3", "bash", "-c", "echo > /dev/tcp/"+devTCP(host, port))
		return cmd.Run() == nil
	}
	expect := func(what string, host string, port int, want bool) {
		t.Helper()
		if got := reach(host, port); got != want {
			t.Errorf("%s %s:%d 可达=%v，期望 %v", what, host, port, got, want)
		}
	}
	for _, p := range []int{34990, 34991, 34994} {
		expect("放行前", p4, p, false)
		expect("放行前", p6, p, false)
	}
	status, ok := Set("live:fam", []Port{
		// 只放行 IPv6
		{Proto: "tcp", Port: 34990, Family: "6"},
		// IPv4 端口区间 34991-34993
		{Proto: "tcp", Port: 34991, End: 34993, Family: "4"},
		// 只放行发往 p4 的连接
		{Proto: "tcp", Port: 34994, Addr: p4},
	}, nil)
	t.Logf("Set: ok=%v %s", ok, status)
	if !ok {
		t.Fatal(status)
	}
	expect("IPv6 端口经 IPv6", p6, 34990, true)
	expect("IPv6 端口经 IPv4", p4, 34990, false)
	for p := 34991; p <= 34993; p++ {
		expect("区间内端口", p4, p, true)
		expect("IPv4 区间经 IPv6", p6, p, false)
	}
	expect("区间外端口", p4, 34995, false)
	expect("限定地址", p4, 34994, true)
	if alt != "" {
		expect("其他地址", alt, 34994, false)
	}
	expect("限定 IPv4 地址经 IPv6", p6, 34994, false)

	Clear("live:fam")
	time.Sleep(200 * time.Millisecond)
	expect("撤销后", p6, 34990, false)
	expect("撤销后", p4, 34992, false)
	_, have, _ := std.be.targets(context.Background())
	for k, list := range have {
		if len(list) > 0 {
			t.Fatalf("%s 仍有 Gately 规则: %+v", k, list)
		}
	}
}

// devTCP 生成 bash /dev/tcp 路径：IPv6 地址不加方括号。
func devTCP(host string, port int) string {
	return host + "/" + strconv.Itoa(port)
}
