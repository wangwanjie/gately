package l4proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
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
	err := s.ServeTCP(ctx, "tcp", addr, Limits{Max: 1}, func(string, ...any) { warned.Add(1) }, func(c net.Conn) {
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

// ServeTCP 的单来源上限：同一 IPv4 地址、同一 IPv6 /64 的并发连接超过上限即被拒绝，释放后可以再连；
// 内网来源不受单来源上限约束；超限警告按时间合并。
func TestServeTCPPerSourceLimit(t *testing.T) {
	queue := make(chan netip.Addr, 32)
	old := remoteAddrOf
	remoteAddrOf = func(net.Conn) (netip.Addr, bool) { return <-queue, true }
	t.Cleanup(func() { remoteAddrOf = old })

	var s Server
	ctx := s.Context(context.Background())
	var warned atomic.Int32
	var warnMsg atomic.Value
	handled := make(chan string, 32)
	probe, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := probe.Addr().String()
	probe.Close()
	if err := s.ServeTCP(ctx, "tcp", addr, Limits{Max: 20, PerSource: 2, Exempt: mustExempt(t, "198.51.100.0/24", "2001:db8:9::/48")}, func(format string, args ...any) {
		warned.Add(1)
		warnMsg.Store(fmt.Sprintf(format, args...))
	}, func(c net.Conn) {
		buf := make([]byte, 16)
		n, _ := c.Read(buf)
		handled <- string(buf[:n])
		// 保持连接直到客户端关闭或服务停止
		_, _ = c.Read(buf)
		c.Close()
	}); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	var clients []net.Conn
	defer func() {
		for _, c := range clients {
			c.Close()
		}
	}()
	// connect 以 from 为来源建立一个连接，返回是否被接受
	connect := func(from, tag string) bool {
		t.Helper()
		queue <- netip.MustParseAddr(from)
		c, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		clients = append(clients, c)
		_, _ = c.Write([]byte(tag))
		select {
		case got := <-handled:
			if got != tag {
				t.Fatalf("处理的连接应为 %s，实际 %s", tag, got)
			}
			return true
		case <-time.After(time.Second):
		}
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		if _, err := c.Read(make([]byte, 1)); err == nil {
			t.Fatalf("%s 既没有被处理也没有被关闭", tag)
		}
		return false
	}
	steps := []struct {
		from, tag string
		want      bool
	}{
		{"203.0.113.7", "a1", true},
		{"203.0.113.7", "a2", true},
		{"203.0.113.7", "a3", false},
		{"203.0.113.8", "b1", true},
		{"2001:db8:1:2::1", "v1", true},
		{"2001:db8:1:2::ffff", "v2", true},
		{"2001:db8:1:2:1::1", "v3", false},
		{"2001:db8:1:3::1", "w1", true},
		{"192.168.1.20", "p1", true},
		{"192.168.1.20", "p2", true},
		{"192.168.1.20", "p3", true},
		{"::ffff:203.0.113.7", "a4", false},
		// 豁免列表中的来源（公网中转）不受单来源上限约束，IPv4 映射地址同样识别
		{"198.51.100.9", "x1", true},
		{"198.51.100.9", "x2", true},
		{"198.51.100.9", "x3", true},
		{"::ffff:198.51.100.9", "x4", true},
		{"2001:db8:9:1::1", "y1", true},
		{"2001:db8:9:1::1", "y2", true},
		{"2001:db8:9:1::1", "y3", true},
		// 豁免列表之外的相邻地址照常受限
		{"198.51.101.9", "z1", true},
		{"198.51.101.9", "z2", true},
		{"198.51.101.9", "z3", false},
	}
	for _, st := range steps {
		if got := connect(st.from, st.tag); got != st.want {
			t.Fatalf("%s（%s）接受 = %v，应为 %v", st.tag, st.from, got, st.want)
		}
	}
	if n := warned.Load(); n != 1 {
		t.Fatalf("10 秒内的单来源超限应只记录 1 条警告，实际 %d", n)
	}
	// 警告写明被拒绝的来源、上限与处理办法
	if msg, _ := warnMsg.Load().(string); !strings.Contains(msg, "来源 203.0.113.7/32") || !strings.Contains(msg, "单来源上限 2") || !strings.Contains(msg, "不受单来源限制的来源") {
		t.Fatalf("单来源超限警告内容不对: %q", msg)
	}
	// 释放 203.0.113.7 的一个连接后可以再连
	clients[0].Close()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if connect("203.0.113.7", "a5") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("释放名额后应能再次连接")
		}
	}
}

func TestPerSourceLimit(t *testing.T) {
	for _, c := range []struct{ rule, def, total, want int }{
		{0, 0, 256, 128}, {0, 0, 1, 1}, {0, 0, 3, 1}, {0, 64, 256, 64}, {8, 64, 256, 8}, {500, 0, 256, 256}, {0, 1000, 10, 10},
	} {
		if got := PerSourceLimit(c.rule, c.def, c.total); got != c.want {
			t.Fatalf("PerSourceLimit(%d, %d, %d) = %d，应为 %d", c.rule, c.def, c.total, got, c.want)
		}
	}
}

// mustExempt 解析豁免列表，失败时终止测试。
func mustExempt(t *testing.T, list ...string) []netip.Prefix {
	t.Helper()
	p, err := ParseExempt(list)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// Limits.Equal 比较豁免列表的内容，热更新据此判断是否要重新监听。
func TestLimitsEqual(t *testing.T) {
	a := Limits{Max: 10, PerSource: 5, Exempt: mustExempt(t, "198.51.100.1")}
	if !a.Equal(Limits{Max: 10, PerSource: 5, Exempt: mustExempt(t, "198.51.100.1/32")}) {
		t.Fatal("内容相同的豁免列表应相等")
	}
	if a.Equal(Limits{Max: 10, PerSource: 5}) || a.Equal(Limits{Max: 10, PerSource: 5, Exempt: mustExempt(t, "198.51.100.2")}) {
		t.Fatal("豁免列表不同应不相等")
	}
	if _, err := ParseExempt([]string{"not-an-ip"}); err == nil {
		t.Fatal("非法地址应报错")
	}
}
