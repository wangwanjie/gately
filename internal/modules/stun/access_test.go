package stun

import (
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/iplist"
)

// startAccessModule 以模块方式启动一条穿透规则，便于验证修改名单时不重启穿透。
func startAccessModule(t *testing.T, rule Rule, settings Settings) *Module {
	t.Helper()
	if err := rule.Validate(); err != nil {
		t.Fatal(err)
	}
	buf, _ := testLogger()
	m := New()
	m.logs = buf
	m.cfg = fileConfig{Settings: settings, Rules: []Rule{rule}}
	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.mu.Lock()
	m.startLocked(rule)
	m.mu.Unlock()
	t.Cleanup(func() {
		m.mu.Lock()
		m.stopLocked(rule.Key)
		m.mu.Unlock()
		cancel()
	})
	r := m.runners[rule.Key]
	waitFor(t, "穿透成功", func() bool { return r.snapshot().Status == StatusOK })
	return m
}

// setAccess 修改配置中的名单并按保存规则时的方式应用。
func setAccess(m *Module, key string, a iplist.Access) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.cfg.Rules {
		if m.cfg.Rules[i].Key == key {
			m.cfg.Rules[i].Access = a
		}
	}
	m.applyLocked(key)
}

func tcpPings(t *testing.T, addr string) bool {
	t.Helper()
	c, err := net.Dial("tcp4", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	_, _ = c.Write([]byte("ping"))
	got := make([]byte, 4)
	_, err = io.ReadFull(c, got)
	return err == nil && string(got) == "ping"
}

func udpPings(t *testing.T, addr string) bool {
	t.Helper()
	c, err := net.Dial("udp4", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, _ = c.Write([]byte("ping"))
	buf := make([]byte, 16)
	_ = c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	n, err := c.Read(buf)
	return err == nil && string(buf[:n]) == "ping"
}

func TestTCPAccessHotUpdate(t *testing.T) {
	e := newTestEnv(t)
	rule := Rule{
		Key: "k", Name: "ssh", Enable: true, Protocol: ProtoTCP4, Bind: Bind{Mode: BindIP, IP: "127.0.0.1"}, UseGlobalServers: true,
		TargetAddresses: []string{"127.0.0.1"}, TargetPort: e.tcpEcho.Addr().(*net.TCPAddr).Port,
		HeartbeatInterval: 500, StunTimeout: 2000, RetryInterval: 500, AutoRetry: true,
		// 有效性检测连接自身公网地址，握手在 accept 之前完成，白名单拒绝来源不影响检测
		Access: iplist.Access{IPMode: iplist.ModeWhitelist, IPList: []string{"10.0.0.0/8"}},
	}
	m := startAccessModule(t, rule, e.settings())
	r := m.runners["k"]
	pub := r.snapshot().PublicAddr
	if tcpPings(t, pub) {
		t.Fatal("不在白名单中的来源应被拒绝")
	}
	if s := r.snapshot().Stats; s.Denied < 1 || s.TotalConns != 0 {
		t.Fatalf("拒绝次数应计入 Denied 且不计入连接数: %+v", s)
	}

	// 只改名单：原地替换，不重新打洞
	setAccess(m, "k", iplist.Access{IPMode: iplist.ModeWhitelist, IPList: []string{"127.0.0.1"}})
	if m.runners["k"] != r || r.snapshot().PublicAddr != pub {
		t.Fatal("只修改名单时不应重启穿透")
	}
	if !tcpPings(t, pub) {
		t.Fatal("加入白名单后应放行")
	}

	// 封禁优先于白名单
	if _, _, err := banlist.Default.Ban("127.0.0.1", banlist.Options{Duration: time.Hour, Force: true, Source: banlist.SourceManual}); err != nil {
		t.Fatal(err)
	}
	unban := func() { banlist.Default.Unban("127.0.0.1") }
	t.Cleanup(unban)
	if tcpPings(t, pub) {
		t.Fatal("被封禁的地址即使在白名单中也应被拒绝")
	}
	unban()

	// 其它字段变化仍然重启
	m.mu.Lock()
	m.cfg.Rules[0].MaxConnections = 10
	m.applyLocked("k")
	m.mu.Unlock()
	if m.runners["k"] == r {
		t.Fatal("修改其它字段时应重启规则")
	}
}

func TestUDPAccessSubscription(t *testing.T) {
	set, _, _ := iplist.Parse(strings.NewReader("127.0.0.0/8\n"))
	iplist.Default.Store("t-stun", set)
	t.Cleanup(func() { iplist.Default.Store("t-stun", nil) })

	e := newTestEnv(t)
	rule := Rule{
		Key: "u", Name: "game", Enable: true, Protocol: ProtoUDP4, Bind: Bind{Mode: BindIP, IP: "127.0.0.1"}, UseGlobalServers: true,
		TargetAddresses: []string{"127.0.0.1"}, TargetPort: e.udpEcho.LocalAddr().(*net.UDPAddr).Port,
		HeartbeatInterval: 500, StunTimeout: 1000, RetryInterval: 500, AutoRetry: true, UDPSessionTimeout: 2,
		// STUN 响应与回环检测包在名单之前处理，黑名单命中本机也能完成穿透
		Access: iplist.Access{IPMode: iplist.ModeBlacklist, IPSets: []string{"t-stun"}},
	}
	m := startAccessModule(t, rule, e.settings())
	r := m.runners["u"]
	pub := r.snapshot().PublicAddr
	if udpPings(t, pub) {
		t.Fatal("命中订阅黑名单的来源应被丢弃")
	}
	if r.snapshot().Stats.Denied < 1 {
		t.Fatal("丢弃的数据包应计入 Denied")
	}
	// 订阅列表刷新后立即生效
	set2, _, _ := iplist.Parse(strings.NewReader("192.0.2.0/24\n"))
	iplist.Default.Store("t-stun", set2)
	if !udpPings(t, pub) {
		t.Fatal("列表刷新后已移出的来源应放行")
	}
}

func TestOnlyAccessChanged(t *testing.T) {
	a := Rule{Name: "x", TargetPort: 1}
	b := a
	b.Access = iplist.Access{IPMode: iplist.ModeBlacklist, IPList: []string{"10.0.0.1"}}
	if !onlyAccessChanged(a, b) {
		t.Error("只有名单不同时应返回 true")
	}
	b.TargetPort = 2
	if onlyAccessChanged(a, b) {
		t.Error("其它字段不同时应返回 false")
	}
}
