package openports

import (
	"net"
	"slices"
	"testing"
	"time"
)

// drain 清空通知通道，避免其他测试遗留的信号影响判断。
func drain() {
	for {
		select {
		case <-wake:
		default:
			return
		}
	}
}

func waitWake(t *testing.T) bool {
	t.Helper()
	select {
	case <-Notify():
		return true
	case <-time.After(200 * time.Millisecond):
		return false
	}
}

func rangesOf(owner string) []Range {
	mu.Lock()
	defer mu.Unlock()
	return slices.Clone(entries[owner].Ranges)
}

func TestSetNotifiesOnlyOnChange(t *testing.T) {
	drain()
	t.Cleanup(func() { Clear("t1") })
	Set("t1", Entry{Ranges: []Range{{Lo: 100, Hi: 110}}})
	if !waitWake(t) {
		t.Fatal("新增登记应发出通知")
	}
	// 内容相同的重复登记不通知，周期性重复登记不会引发无谓的防火墙刷新
	Set("t1", Entry{Ranges: []Range{{Lo: 100, Hi: 110}}})
	if waitWake(t) {
		t.Fatal("内容不变不应通知")
	}
	Set("t1", Entry{Ranges: []Range{{Lo: 100, Hi: 111}}})
	if !waitWake(t) {
		t.Fatal("内容变化应通知")
	}
	Clear("t1")
	if !waitWake(t) {
		t.Fatal("撤销登记应通知")
	}
	// 撤销不存在的登记不通知
	Clear("t1")
	if waitWake(t) {
		t.Fatal("撤销不存在的登记不应通知")
	}
}

// 连续多次变化合并为一个信号，且发送不阻塞调用方
func TestNotifyCoalesces(t *testing.T) {
	drain()
	t.Cleanup(func() { Clear("t2") })
	done := make(chan struct{})
	go func() {
		for i := 1; i <= 50; i++ {
			Set("t2", Entry{Ranges: []Range{{Lo: i, Hi: i}}})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("登记不应因通知通道已满而阻塞")
	}
	if !waitWake(t) {
		t.Fatal("应收到通知")
	}
	if waitWake(t) {
		t.Fatal("多次变化应合并为一个信号")
	}
}

func TestSetPortsCompacts(t *testing.T) {
	t.Cleanup(func() { Clear("t3") })
	SetPorts("t3", []int{6031, 6029, 6030, 6029, 0, 70000, -1, 8080})
	want := []Range{{Lo: 6029, Hi: 6031}, {Lo: 8080, Hi: 8080}}
	if got := rangesOf("t3"); !slices.Equal(got, want) {
		t.Fatalf("得到 %v，期望 %v", got, want)
	}
	SetPorts("t3", nil)
	if got := rangesOf("t3"); len(got) != 0 {
		t.Fatalf("空列表应撤销登记，得到 %v", got)
	}
}

func heldPorts() []Range {
	return rangesOf(holdOwner)
}

func TestHold(t *testing.T) {
	if len(heldPorts()) != 0 {
		t.Fatalf("测试开始时不应有占用: %v", heldPorts())
	}
	a := &net.TCPAddr{IP: net.IPv4zero, Port: 6029}
	// 同一端口的 IPv4 与 IPv6 两个监听：引用计数，两个都关闭才撤销
	r1 := Hold(a)
	r2 := Hold(&net.TCPAddr{IP: net.IPv6unspecified, Port: 6029})
	r3 := Hold(&net.TCPAddr{IP: net.IPv4zero, Port: 6030})
	if got, want := heldPorts(), []Range{{Lo: 6029, Hi: 6030}}; !slices.Equal(got, want) {
		t.Fatalf("得到 %v，期望 %v", got, want)
	}
	r1()
	r1() // 重复撤销只生效一次，不能把另一个监听的计数也减掉
	if got, want := heldPorts(), []Range{{Lo: 6029, Hi: 6030}}; !slices.Equal(got, want) {
		t.Fatalf("还有一个监听占用 6029，得到 %v", got)
	}
	r2()
	if got, want := heldPorts(), []Range{{Lo: 6030, Hi: 6030}}; !slices.Equal(got, want) {
		t.Fatalf("得到 %v，期望 %v", got, want)
	}
	r3()
	if len(heldPorts()) != 0 {
		t.Fatalf("全部撤销后应为空: %v", heldPorts())
	}
}

// 回环地址、非 TCP 地址与端口 0 不登记：外部访问不到的端口不应让扫描检测放过对同号端口的探测
func TestHoldIgnoresUnreachable(t *testing.T) {
	drain()
	for _, a := range []net.Addr{
		&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 16680},
		&net.TCPAddr{IP: net.IPv6loopback, Port: 16680},
		&net.TCPAddr{IP: net.IPv4zero, Port: 0},
		&net.UDPAddr{IP: net.IPv4zero, Port: 53},
		nil,
	} {
		release := Hold(a)
		release()
	}
	if len(heldPorts()) != 0 || waitWake(t) {
		t.Fatalf("不应登记任何端口: %v", heldPorts())
	}
}

// 并发占用与撤销不丢计数
func TestHoldConcurrent(t *testing.T) {
	done := make(chan struct{})
	const n = 64
	for i := 0; i < n; i++ {
		go func(i int) {
			r := Hold(&net.TCPAddr{IP: net.IPv4zero, Port: 20000 + i%8})
			time.Sleep(time.Millisecond)
			r()
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < n; i++ {
		<-done
	}
	if len(heldPorts()) != 0 {
		t.Fatalf("并发占用与撤销后应为空: %v", heldPorts())
	}
}

func TestHoldHostPort(t *testing.T) {
	cases := []struct {
		addr string
		held bool
	}{
		{"0.0.0.0:1055", true},
		{":1056", true},
		{"[::]:1057", true},
		{"192.168.1.5:1058", true},
		{"127.0.0.1:1059", false},
		{"[::1]:1060", false},
		{"localhost:1061", false},
		{"example.com:1062", false},
		{"no-port", false},
		{"0.0.0.0:abc", false},
		{"0.0.0.0:0", false},
	}
	for _, c := range cases {
		release := HoldHostPort(c.addr)
		got := len(heldPorts()) > 0
		release()
		if got != c.held {
			t.Errorf("%q: 登记=%v，期望 %v", c.addr, got, c.held)
		}
		if n := len(heldPorts()); n != 0 {
			t.Errorf("%q: 撤销后仍有登记 %v", c.addr, heldPorts())
		}
	}
}
