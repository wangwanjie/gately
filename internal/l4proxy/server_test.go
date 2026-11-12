package l4proxy

import (
	"context"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// 轮询顺序应覆盖全部目标，且每次起点递增。
func TestPickerOrderRotates(t *testing.T) {
	p := NewPicker([]string{"a", "b", "c"})
	if got := strings.Join(p.Order(), ","); got != "a,b,c" {
		t.Fatalf("第一次顺序 = %s", got)
	}
	if got := strings.Join(p.Order(), ","); got != "b,c,a" {
		t.Fatalf("第二次顺序 = %s", got)
	}
	if len(NewPicker(nil).Order()) != 0 {
		t.Fatal("空列表应返回空顺序")
	}
}

func TestCleanTargets(t *testing.T) {
	got := CleanTargets([]string{" [::1] ", "", "  ", "10.0.0.1"})
	if len(got) != 2 || got[0] != "::1" || got[1] != "10.0.0.1" {
		t.Fatalf("CleanTargets = %v", got)
	}
}

func TestValidateProxyProtocol(t *testing.T) {
	for _, ok := range []string{"", "v1", "v2"} {
		if err := ValidateProxyProtocol(ok); err != nil {
			t.Fatalf("%q 应通过: %v", ok, err)
		}
	}
	if err := ValidateProxyProtocol("v3"); err == nil {
		t.Fatal("v3 应被拒绝")
	}
}

// 第一个目标不可达时应回退到下一个目标。
func TestPickerDialFallback(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	// 192.0.2.1 为文档保留地址，用极短超时让它快速失败
	p := NewPicker([]string{"192.0.2.1", "127.0.0.1"})
	conn, err := p.Dial("tcp", port, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("应回退到可达目标: %v", err)
	}
	conn.Close()
	if _, err := NewPicker(nil).Dial("tcp", port, time.Second); err == nil {
		t.Fatal("没有目标时应报错")
	}
}

func TestDialContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := NewPicker([]string{"127.0.0.1", "127.0.0.2"})
	if _, err := p.DialContext(ctx, "tcp", 1, time.Second); err == nil {
		t.Fatal("上下文已取消应报错")
	}
}

// WritePrefix 应把 PROXY 头与预读数据合并写给目标。
func TestWritePrefix(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, _ := net.Dial("tcp", ln.Addr().String())
		if c != nil {
			defer c.Close()
			io.Copy(io.Discard, c)
		}
	}()
	client, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	target, peer := net.Pipe()
	defer target.Close()
	defer peer.Close()
	got := make(chan string, 1)
	go func() {
		buf := make([]byte, 512)
		peer.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _ := peer.Read(buf)
		got <- string(buf[:n])
	}()
	if err := WritePrefix(target, client, "v1", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	s := <-got
	if !strings.HasPrefix(s, "PROXY TCP4 ") || !strings.HasSuffix(s, "hello") {
		t.Fatalf("写入内容 = %q", s)
	}

	// 无版本无数据时不应写入
	if err := WritePrefix(target, client, "", nil); err != nil {
		t.Fatal(err)
	}
}

// ServeTCP：处理连接、超限拒绝、Stop 后协程退出。
func TestServeTCPLimitAndStop(t *testing.T) {
	var s Server
	ctx := s.Context(context.Background())
	var handled, warned atomic.Int32
	release := make(chan struct{})
	// 先占用一个随机端口再释放，取得可用地址
	probe, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := probe.Addr().String()
	probe.Close()
	err := s.ServeTCP(ctx, "tcp", addr, 1, func(string, ...any) { warned.Add(1) }, func(c net.Conn) {
		handled.Add(1)
		<-release
		c.Close()
	})
	if err != nil {
		t.Fatal(err)
	}
	c1, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c1.Close()
	deadline := time.Now().Add(2 * time.Second)
	for handled.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	c2, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	c2.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := c2.Read(make([]byte, 1)); err == nil {
		t.Fatal("超限连接应被关闭")
	}
	if warned.Load() == 0 {
		t.Fatal("超限应记录警告")
	}
	close(release)
	s.Stop()
	if _, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
		t.Fatal("Stop 后监听应关闭")
	}
}
