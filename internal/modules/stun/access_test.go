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

func tcpRule(e *testEnv, key string, a iplist.Access) Rule {
	return Rule{
		Key: key, Name: key, Enable: true, Protocol: ProtoTCP4, Bind: Bind{Mode: BindIP, IP: "127.0.0.1"}, UseGlobalServers: true,
		TargetAddresses: []string{"127.0.0.1"}, TargetPort: e.tcpEcho.Addr().(*net.TCPAddr).Port,
		HeartbeatInterval: 500, StunTimeout: 2000, RetryInterval: 500, AutoRetry: true, Access: a,
	}
}

func udpRule(e *testEnv, key string, a iplist.Access) Rule {
	return Rule{
		Key: key, Name: key, Enable: true, Protocol: ProtoUDP4, Bind: Bind{Mode: BindIP, IP: "127.0.0.1"}, UseGlobalServers: true,
		TargetAddresses: []string{"127.0.0.1"}, TargetPort: e.udpEcho.LocalAddr().(*net.UDPAddr).Port,
		HeartbeatInterval: 500, StunTimeout: 1000, RetryInterval: 500, AutoRetry: true, UDPSessionTimeout: 5, Access: a,
	}
}

// TCP 黑名单命中拒绝，计数逐次累加
func TestTCPAccessBlacklist(t *testing.T) {
	e := newTestEnv(t)
	m := startAccessModule(t, tcpRule(e, "b", iplist.Access{IPMode: iplist.ModeBlacklist, IPList: []string{"127.0.0.0/8"}}), e.settings())
	r := m.runners["b"]
	pub := r.snapshot().PublicAddr
	for i := 1; i <= 2; i++ {
		if tcpPings(t, pub) {
			t.Fatal("命中黑名单的来源应被拒绝")
		}
		// 拒绝计数在 accept 协程中累加，等它落地
		waitFor(t, "拒绝计数", func() bool { return r.snapshot().Stats.Denied == int64(i) })
	}
	if s := r.snapshot().Stats; s.TotalConns != 0 {
		t.Fatalf("被拒连接不应计入连接数: %+v", s)
	}
	// 关闭名单后放行
	setAccess(m, "b", iplist.Access{})
	if !tcpPings(t, pub) {
		t.Fatal("关闭名单后应放行")
	}
}

// 订阅列表未载入或已删除时按空列表处理：白名单拒绝，黑名单放行
func TestAccessMissingSubscription(t *testing.T) {
	set, _, _ := iplist.Parse(strings.NewReader("127.0.0.0/8\n"))
	iplist.Default.Store("t-st-gone", set)
	iplist.Default.Store("t-st-gone", nil)
	for _, key := range []string{"t-st-never", "t-st-gone"} {
		e := newTestEnv(t)
		m := startAccessModule(t, tcpRule(e, "w", iplist.Access{IPMode: iplist.ModeWhitelist, IPSets: []string{key}}), e.settings())
		if tcpPings(t, m.runners["w"].snapshot().PublicAddr) {
			t.Fatalf("%s: 白名单引用的列表不可用时应拒绝", key)
		}
		setAccess(m, "w", iplist.Access{IPMode: iplist.ModeBlacklist, IPSets: []string{key}})
		if !tcpPings(t, m.runners["w"].snapshot().PublicAddr) {
			t.Fatalf("%s: 黑名单引用的列表不可用时应放行", key)
		}
	}
}

// UDP：被拒来源不建立会话；名单收紧后已有会话的上行被丢弃，放开后继续使用原会话
func TestUDPAccessSessions(t *testing.T) {
	e := newTestEnv(t)
	m := startAccessModule(t, udpRule(e, "u", iplist.Access{IPMode: iplist.ModeWhitelist, IPList: []string{"10.0.0.0/8"}}), e.settings())
	r := m.runners["u"]
	pub := r.snapshot().PublicAddr
	conn, err := net.Dial("udp4", pub)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	echo := func() bool {
		_, _ = conn.Write([]byte("ping"))
		_ = conn.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
		buf := make([]byte, 16)
		n, err := conn.Read(buf)
		return err == nil && string(buf[:n]) == "ping"
	}
	if echo() {
		t.Fatal("被拒来源不应收到回显")
	}
	if s := r.snapshot().Stats; s.TotalConns != 0 || s.Denied != 1 {
		t.Fatalf("被拒来源不应建立会话: %+v", s)
	}
	setAccess(m, "u", iplist.Access{IPMode: iplist.ModeWhitelist, IPList: []string{"127.0.0.1"}})
	if m.runners["u"] != r {
		t.Fatal("只修改名单时不应重启穿透")
	}
	if !echo() {
		t.Fatal("放行后应建立会话")
	}
	setAccess(m, "u", iplist.Access{IPMode: iplist.ModeBlacklist, IPList: []string{"127.0.0.1"}})
	if echo() {
		t.Fatal("名单收紧后已有会话的上行应被丢弃")
	}
	if s := r.snapshot().Stats; s.ActiveConns != 1 || s.Denied != 2 {
		t.Fatalf("会话应保留到空闲超时并计入拒绝: %+v", s)
	}
	setAccess(m, "u", iplist.Access{})
	if !echo() {
		t.Fatal("关闭名单后应继续使用原会话")
	}
	if s := r.snapshot().Stats; s.TotalConns != 1 {
		t.Fatalf("应复用原会话而不是新建: %+v", s)
	}
}

func TestAccessValidate(t *testing.T) {
	base := func(a iplist.Access) Rule {
		return Rule{Name: "x", Protocol: ProtoTCP4, UseGlobalServers: true, TargetAddresses: []string{"127.0.0.1"}, TargetPort: 22, Access: a}
	}
	for name, a := range map[string]iplist.Access{
		"模式非法":  {IPMode: "on", IPList: []string{"10.0.0.1"}},
		"网段非法":  {IPMode: iplist.ModeBlacklist, IPList: []string{"300.0.0.0/8"}},
		"白名单为空": {IPMode: iplist.ModeWhitelist, IPList: []string{""}},
	} {
		r := base(a)
		if err := r.Validate(); err == nil {
			t.Errorf("%s 应校验失败", name)
		}
	}
	ok := base(iplist.Access{IPMode: iplist.ModeWhitelist, IPSets: []string{"t-st-missing"}})
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := ok.Access.CheckSets(); err == nil {
		t.Error("引用不存在的订阅列表应报错")
	}
}
