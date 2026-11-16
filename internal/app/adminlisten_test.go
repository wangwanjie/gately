package app

import (
	"context"
	"io"
	"log"
	"net/netip"
	"strconv"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/httpx"
)

// 管理后台监听成功后登记实际地址（-listen 覆盖配置文件时以覆盖的地址为准），Web 服务据此判断站点后端是否为本实例后台；
// 退出时取消登记。
func TestRunRegistersAdminListen(t *testing.T) {
	port := freeTCPPort(t)
	a, err := New(Options{ConfigDir: t.TempDir(), Listen: "127.0.0.1:" + strconv.Itoa(port)})
	if err != nil {
		t.Fatal(err)
	}
	a.logger = log.New(io.Discard, "", 0)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()
	loopback := netip.MustParseAddr("127.0.0.1")
	deadline := time.Now().Add(5 * time.Second)
	for !httpx.IsAdminAddr(loopback, uint16(port)) {
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("后台监听成功后应登记实际监听地址")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// 配置文件中的默认地址被 -listen 覆盖，不应登记；只监听 127.0.0.1 时 IPv6 回环不算
	if httpx.IsAdminAddr(loopback, 16680) || httpx.IsAdminAddr(netip.MustParseAddr("::1"), uint16(port)) {
		t.Fatal("只应登记实际监听的地址")
	}
	cancel()
	<-done
	if httpx.IsAdminAddr(loopback, uint16(port)) {
		t.Fatal("退出后应取消登记")
	}
}
