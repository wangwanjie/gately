package portforward

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/l4proxy"
)

// tcpSilent 是只接受连接、从不发送数据的 TCP 目标（客户端先发言的协议，如 HTTP、TLS）。
func tcpSilent(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = io.Copy(io.Discard, c)
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

// tcpBanner 是连接后先发送问候语的 TCP 目标（服务端先发言的协议，如 SSH、FTP）。
func tcpBanner(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = c.Write([]byte("SSH-2.0-test\r\n"))
				_, _ = io.Copy(io.Discard, c)
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

// 连接后双方都不发数据的空连接按首个数据超时断开并释放名额；服务端先发言的连接不受影响。
func TestTCPFirstDataTimeout(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target int
		closed bool
	}{
		{"双方都不发数据", tcpSilent(t), true},
		{"服务端先发言", tcpBanner(t), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listenPort := freePort(t, "tcp")
			rule := Rule{
				Protocols: []string{"tcp4"}, ListenAddress: "127.0.0.1", ListenPorts: strconv.Itoa(listenPort),
				TargetAddresses: []string{"127.0.0.1"}, TargetPorts: strconv.Itoa(tc.target),
			}
			r := testRunner(rule)
			r.current().settings.FirstDataTimeout = 1
			if err := r.start(context.Background()); err != nil {
				t.Fatal(err)
			}
			defer r.stop()
			c, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort)))
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			start := time.Now()
			_ = c.SetReadDeadline(time.Now().Add(2500 * time.Millisecond))
			buf := make([]byte, 64)
			n, err := c.Read(buf)
			if tc.closed {
				if err == nil {
					t.Fatalf("空连接不应收到数据: %q", buf[:n])
				}
				var ne net.Error
				if d := time.Since(start); (errors.As(err, &ne) && ne.Timeout()) || d < 800*time.Millisecond {
					t.Fatalf("空连接应在约 1 秒后被断开: %v（%v）", err, d)
				}
				deadline := time.Now().Add(2 * time.Second)
				for r.stats.ActiveConns.Load() != 0 && time.Now().Before(deadline) {
					time.Sleep(20 * time.Millisecond)
				}
				if r.stats.ActiveConns.Load() != 0 {
					t.Fatal("断开后应释放连接名额")
				}
				return
			}
			if err != nil || string(buf[:n]) != "SSH-2.0-test\r\n" {
				t.Fatalf("应收到服务端的问候语: %q %v", buf[:n], err)
			}
			// 之后双方长时间不发数据也不断开
			time.Sleep(1500 * time.Millisecond)
			if _, err := c.Write([]byte("SSH-2.0-client\r\n")); err != nil {
				t.Fatal(err)
			}
			if r.stats.ActiveConns.Load() != 1 {
				t.Fatal("已有数据往来的连接不应因首个数据超时断开")
			}
		})
	}
}

// 单来源上限：未设置时取最大连接数的一半，规则设置优先于全局默认值；首个数据超时 0 表示默认 60 秒。
func TestEffectiveConnLimits(t *testing.T) {
	s := defaultSettings()
	if s.FirstDataTimeout != 60 {
		t.Fatalf("首个数据超时默认应为 60 秒: %d", s.FirstDataTimeout)
	}
	cases := []struct {
		rule Rule
		def  int
		want l4proxy.Limits
	}{
		{Rule{}, 0, l4proxy.Limits{Max: 256, PerSource: 128}},
		{Rule{MaxConnections: 10}, 0, l4proxy.Limits{Max: 10, PerSource: 5}},
		{Rule{MaxConnections: 1}, 0, l4proxy.Limits{Max: 1, PerSource: 1}},
		{Rule{}, 32, l4proxy.Limits{Max: 256, PerSource: 32}},
		{Rule{MaxConnsPerSource: 8}, 32, l4proxy.Limits{Max: 256, PerSource: 8}},
		{Rule{MaxConnections: 4, MaxConnsPerSource: 100}, 0, l4proxy.Limits{Max: 4, PerSource: 4}},
	}
	for _, c := range cases {
		st := s
		st.DefaultMaxConnsPerSource = c.def
		if got, _ := effectiveLimits(c.rule, st); !got.Equal(c.want) {
			t.Fatalf("%+v / 默认 %d: %+v，应为 %+v", c.rule, c.def, got, c.want)
		}
	}
	z := Settings{FirstDataTimeout: 0, DefaultMaxConnsPerSource: -3}
	z.normalize()
	if z.FirstDataTimeout != 60 || z.DefaultMaxConnsPerSource != 0 {
		t.Fatalf("规范化结果不对: %+v", z)
	}
	big := Settings{FirstDataTimeout: 99999}
	big.normalize()
	if big.FirstDataTimeout != 3600 {
		t.Fatalf("首个数据超时上限应为 3600 秒: %d", big.FirstDataTimeout)
	}
	bad := Rule{Protocols: []string{"tcp"}, ListenPorts: "1", TargetPorts: "1", TargetAddresses: []string{"127.0.0.1"}, MaxConnsPerSource: -1}
	if err := bad.Validate(); err == nil {
		t.Fatal("单来源上限为负数应校验失败")
	}
}

// 不受单来源限制的来源：保存时去空白去重并校验格式，生效到端口上限，列表变化时重新监听。
func TestPerSourceExemptSettings(t *testing.T) {
	s := defaultSettings()
	s.PerSourceExempt = []string{" 198.51.100.7 ", "", "2001:db8:9::/48", "198.51.100.7"}
	s.normalize()
	if !slices.Equal(s.PerSourceExempt, []string{"198.51.100.7", "2001:db8:9::/48"}) {
		t.Fatalf("规范化结果不对: %q", s.PerSourceExempt)
	}
	if err := s.validate(); err != nil {
		t.Fatal(err)
	}
	got, _ := effectiveLimits(Rule{}, s)
	want := l4proxy.Limits{Max: 256, PerSource: 128, Exempt: []netip.Prefix{netip.MustParsePrefix("198.51.100.7/32"), netip.MustParsePrefix("2001:db8:9::/48")}}
	if !got.Equal(want) {
		t.Fatalf("豁免列表应生效到端口上限: %+v", got)
	}
	r := &runner{rule: Rule{}, settings: defaultSettings()}
	if r.canUpdate(Rule{}, s) {
		t.Fatal("豁免列表变化时应重新监听")
	}
	bad := defaultSettings()
	bad.PerSourceExempt = []string{"relay.example.com"}
	if err := bad.validate(); err == nil {
		t.Fatal("非 IP/CIDR 应校验失败")
	}
	// 配置文件中手工写错时整体忽略，不影响规则启动
	ign, _ := effectiveLimits(Rule{}, bad)
	if len(ign.Exempt) != 0 {
		t.Fatalf("格式错误的豁免列表应忽略: %+v", ign.Exempt)
	}
}
