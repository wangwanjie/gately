package portforward

import (
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/l4proxy"
	"github.com/wangwanjie/gately/internal/logbuf"

	proxyproto "github.com/pires/go-proxyproto"
)

// freePort 获取一个当前空闲的端口。
func freePort(t *testing.T, network string) int {
	t.Helper()
	if network == "udp" {
		pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer pc.Close()
		return pc.LocalAddr().(*net.UDPAddr).Port
	}
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func tcpEcho(t *testing.T) (int, func()) {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, func() { ln.Close() }
}

func udpEcho(t *testing.T) (int, func()) {
	t.Helper()
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = pc.WriteTo(buf[:n], addr)
		}
	}()
	return pc.LocalAddr().(*net.UDPAddr).Port, func() { pc.Close() }
}

func testRunner(rule Rule) *runner {
	s := defaultSettings()
	s.DialTimeout = 1
	return newRunner(rule, s, logbuf.New(100, log.New(io.Discard, "", 0)).Logger("test"))
}

func TestTCPForward(t *testing.T) {
	echoPort, closeEcho := tcpEcho(t)
	defer closeEcho()
	listenPort := freePort(t, "tcp")

	rule := Rule{
		Protocols:       []string{"tcp4"},
		ListenAddress:   "127.0.0.1",
		ListenPorts:     strconv.Itoa(listenPort),
		TargetAddresses: []string{"127.0.0.1"},
		TargetPorts:     strconv.Itoa(echoPort),
	}
	if err := rule.Validate(); err != nil {
		t.Fatal(err)
	}
	r := testRunner(rule)
	if err := r.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer r.stop()

	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort)))
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("hello gately")
	if _, err := conn.Write(msg); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len(msg))
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf, msg) {
		t.Fatalf("回显不一致: %q", buf)
	}
	conn.Close()

	// 等待统计落地
	deadline := time.Now().Add(2 * time.Second)
	for r.stats.ActiveConns.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	s := r.snapshot()
	if s.TotalConns != 1 || s.BytesIn != int64(len(msg)) || s.BytesOut != int64(len(msg)) || s.ActiveConns != 0 {
		t.Fatalf("统计错误: %+v", s)
	}
}

func TestTCPFailoverToSecondTarget(t *testing.T) {
	echoPort, closeEcho := tcpEcho(t)
	defer closeEcho()
	listenPort := freePort(t, "tcp")

	// 第一个目标为不可路由的 TEST-NET 地址 192.0.2.1，拨号超时后应回退到第二个目标
	rule := Rule{
		Protocols:       []string{"tcp4"},
		ListenAddress:   "127.0.0.1",
		ListenPorts:     strconv.Itoa(listenPort),
		TargetAddresses: []string{"127.0.0.1"},
		TargetPorts:     strconv.Itoa(echoPort),
	}
	r := testRunner(rule)
	// 直接替换目标列表，让轮询从不可达地址开始
	r.current().picker = l4proxy.NewPicker([]string{"192.0.2.1", "127.0.0.1"})
	r.current().settings.DialTimeout = 1
	if err := r.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer r.stop()

	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort)))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = conn.Write([]byte("x"))
	buf := make([]byte, 1)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("故障转移失败: %v", err)
	}
}

func TestTCPMaxConnections(t *testing.T) {
	echoPort, closeEcho := tcpEcho(t)
	defer closeEcho()
	listenPort := freePort(t, "tcp")
	rule := Rule{
		Protocols:       []string{"tcp4"},
		ListenAddress:   "127.0.0.1",
		ListenPorts:     strconv.Itoa(listenPort),
		TargetAddresses: []string{"127.0.0.1"},
		TargetPorts:     strconv.Itoa(echoPort),
		MaxConnections:  1,
	}
	r := testRunner(rule)
	if err := r.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer r.stop()
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort))

	first, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	_, _ = first.Write([]byte("a"))
	buf := make([]byte, 1)
	_ = first.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.ReadFull(first, buf); err != nil {
		t.Fatal(err)
	}

	second, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	_ = second.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := second.Read(buf); err == nil {
		t.Fatal("超过连接上限的连接应被关闭")
	}
}

func TestUDPForward(t *testing.T) {
	echoPort, closeEcho := udpEcho(t)
	defer closeEcho()
	listenPort := freePort(t, "udp")
	rule := Rule{
		Protocols:         []string{"udp4"},
		ListenAddress:     "127.0.0.1",
		ListenPorts:       strconv.Itoa(listenPort),
		TargetAddresses:   []string{"127.0.0.1"},
		TargetPorts:       strconv.Itoa(echoPort),
		UDPSessionTimeout: 1,
	}
	r := testRunner(rule)
	if err := r.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer r.stop()

	conn, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort)))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for i := 0; i < 3; i++ {
		msg := []byte("ping-" + strconv.Itoa(i))
		if _, err := conn.Write(msg); err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, 64)
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		n, err := conn.Read(buf)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(buf[:n], msg) {
			t.Fatalf("回显不一致: %q", buf[:n])
		}
	}
	if got := r.stats.ActiveConns.Load(); got != 1 {
		t.Fatalf("同一客户端应复用一个会话，实际 %d", got)
	}
	// 空闲超时后会话应被回收
	deadline := time.Now().Add(4 * time.Second)
	for r.stats.ActiveConns.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if got := r.stats.ActiveConns.Load(); got != 0 {
		t.Fatalf("UDP 会话未按超时回收: %d", got)
	}
}

func TestPartialListenFailure(t *testing.T) {
	// 先占用一个端口，规则中包含该端口时其余端口仍应启动
	busy, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	busyPort := busy.Addr().(*net.TCPAddr).Port
	freeP := freePort(t, "tcp")
	rule := Rule{
		Protocols:       []string{"tcp4"},
		ListenAddress:   "127.0.0.1",
		ListenPorts:     strconv.Itoa(busyPort) + "," + strconv.Itoa(freeP),
		TargetAddresses: []string{"127.0.0.1"},
		TargetPorts:     "1",
	}
	r := testRunner(rule)
	if err := r.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer r.stop()
	if len(r.listenErrors) != 1 {
		t.Fatalf("应记录 1 个监听失败，实际 %v", r.listenErrors)
	}
}

// 开启 PROXY 协议后，下游能拿到真实客户端地址，且数据照常转发
func TestTCPSendProxyProtocol(t *testing.T) {
	for _, v := range []string{"v1", "v2"} {
		t.Run(v, func(t *testing.T) {
			raw, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			ln := &proxyproto.Listener{Listener: raw}
			defer ln.Close()
			seen := make(chan string, 1)
			go func() {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				defer c.Close()
				// 读取数据时才会解析 PROXY 头
				buf := make([]byte, 5)
				_, _ = io.ReadFull(c, buf)
				seen <- c.RemoteAddr().String()
				_, _ = c.Write(buf)
			}()
			listenPort := freePort(t, "tcp")
			rule := Rule{Protocols: []string{"tcp4"}, ListenAddress: "127.0.0.1", ListenPorts: strconv.Itoa(listenPort),
				TargetAddresses: []string{"127.0.0.1"}, TargetPorts: strconv.Itoa(raw.Addr().(*net.TCPAddr).Port), ProxyProtocol: v}
			if err := rule.Validate(); err != nil {
				t.Fatal(err)
			}
			r := testRunner(rule)
			if err := r.start(context.Background()); err != nil {
				t.Fatal(err)
			}
			defer r.stop()
			conn, err := net.Dial("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort)))
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_, _ = conn.Write([]byte("hello"))
			buf := make([]byte, 5)
			_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			if _, err := io.ReadFull(conn, buf); err != nil || string(buf) != "hello" {
				t.Fatalf("数据转发失败: %q %v", buf, err)
			}
			if got := <-seen; got != conn.LocalAddr().String() {
				t.Fatalf("下游看到的客户端地址 = %s，期望 %s", got, conn.LocalAddr())
			}
		})
	}
	bad := Rule{Protocols: []string{"tcp4"}, ListenPorts: "1", TargetAddresses: []string{"x"}, TargetPorts: "1", ProxyProtocol: "v3"}
	if bad.Validate() == nil {
		t.Fatal("应拒绝未知 PROXY 协议版本")
	}
}
