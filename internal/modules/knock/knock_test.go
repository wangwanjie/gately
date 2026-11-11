package knock

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/fwopen"
	"github.com/wangwanjie/gately/internal/knockgate"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/spa"
)

func TestMain(m *testing.M) {
	// 测试中不修改系统防火墙
	fwopen.ForTesting()
	os.Exit(m.Run())
}

// fakeBackend 记录写入的规则与放行，代替 nft / iptables。
type fakeBackend struct {
	mu       sync.Mutex
	applies  []fwPlan
	added    []fwGrant
	deleted  []fwGrant
	present  bool
	applyErr error
	removed  int
	counts   fwCounts
}

func (b *fakeBackend) kind() string    { return "fake" }
func (b *fakeBackend) version() string { return "1" }
func (b *fakeBackend) apply(_ context.Context, plan fwPlan, _ time.Time) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.applyErr != nil {
		return b.applyErr
	}
	b.applies = append(b.applies, plan)
	b.present = true
	return nil
}
func (b *fakeBackend) add(_ context.Context, g []fwGrant, _ time.Time) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.added = append(b.added, g...)
	return nil
}
func (b *fakeBackend) del(_ context.Context, g []fwGrant) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.deleted = append(b.deleted, g...)
	return nil
}
func (b *fakeBackend) check(context.Context) (bool, fwCounts, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.present, b.counts, nil
}
func (b *fakeBackend) remove(context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.present = false
	b.removed++
	return nil
}
func (b *fakeBackend) text(context.Context) (string, error) { return "fake", nil }

func (b *fakeBackend) lastPlan() fwPlan {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.applies) == 0 {
		return fwPlan{}
	}
	return b.applies[len(b.applies)-1]
}

func (b *fakeBackend) addedList() []fwGrant {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]fwGrant{}, b.added...)
}

// testClock 是可并发读写的测试时钟。
type testClock struct{ ns atomic.Int64 }

func (c *testClock) set(t time.Time) { c.ns.Store(t.UnixNano()) }
func (c *testClock) now() time.Time  { return time.Unix(0, c.ns.Load()) }

type testEnv struct {
	m     *Module
	be    *fakeBackend
	logs  *logbuf.Buffer
	clock *testClock
	mu    sync.Mutex
	evts  []events.Event
}

// newEnv 创建并启动模块：UDP 端口为 0（不监听），防火墙使用假后端，时钟固定。
func newEnv(t *testing.T, rules ...Rule) *testEnv {
	t.Helper()
	knockgate.Default.Clear()
	t.Cleanup(knockgate.Default.Clear)
	old := localPrefixes
	localPrefixes = func() []netip.Prefix { return nil }
	t.Cleanup(func() { localPrefixes = old })
	store, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := defaultSettings()
	s.Port = 0
	s.AutoFirewall = false
	for i := range rules {
		if err := rules[i].Validate(); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Save(ID, fileConfig{Settings: s, Rules: rules}); err != nil {
		t.Fatal(err)
	}
	env := &testEnv{m: New(), be: &fakeBackend{}, logs: logbuf.New(500, nil)}
	env.clock = &testClock{}
	env.clock.set(time.Unix(1_800_000_000, 0))
	env.m.now = env.clock.now
	knockgate.Default.SetClock(env.m.now)
	t.Cleanup(func() { knockgate.Default.SetClock(nil) })
	if err := env.m.Init(module.Env{Store: store, Logs: env.logs}); err != nil {
		t.Fatal(err)
	}
	env.m.fw.detect = func(context.Context) (backend, string, string) { return env.be, "", "" }
	env.m.fw.now = env.m.now
	unsub := events.Subscribe(func(e events.Event) {
		if strings.HasPrefix(e.Type, "knock.") {
			env.mu.Lock()
			env.evts = append(env.evts, e)
			env.mu.Unlock()
		}
	})
	t.Cleanup(unsub)
	if err := env.m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = env.m.Stop() })
	return env
}

func (e *testEnv) eventsOf(typ string) []events.Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []events.Event
	for _, x := range e.evts {
		if x.Type == typ {
			out = append(out, x)
		}
	}
	return out
}

func (e *testEnv) hasLog(sub string) bool {
	for _, l := range e.logs.Since(0) {
		if strings.Contains(l.Message, sub) {
			return true
		}
	}
	return false
}

func sshRule(name string) Rule {
	return Rule{Key: "r-" + name, Name: name, Enable: true, Ports: []string{"tcp/22", "udp/51820"}, Firewall: true, Timeout: 30, MaxTimeout: 120}
}

func keyOf(t *testing.T, r Rule) spa.Key {
	t.Helper()
	k, err := r.spaKey()
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// packet 用规则的密钥生成报文，时间戳为模块时钟。
func (e *testEnv) packet(t *testing.T, r Rule, msg spa.Message) []byte {
	t.Helper()
	if msg.Timestamp == 0 {
		msg.Timestamp = e.clock.now().Unix()
	}
	if msg.Type == 0 && msg.Command == "" {
		msg.Type = spa.AccessMsg
	}
	if msg.Access == nil && msg.Type != spa.CommandMsg {
		msg.Access = []spa.Access{{Proto: "tcp", Port: 22}}
	}
	if msg.User == "" {
		msg.User = "alice"
	}
	p, err := spa.Encode(msg, keyOf(t, r), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (e *testEnv) rule(key string) Rule {
	e.m.mu.Lock()
	defer e.m.mu.Unlock()
	for _, r := range e.m.cfg.Rules {
		if r.Key == key {
			return r
		}
	}
	panic("no rule " + key)
}

func waitFor(t *testing.T, what string, f func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if f() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("超时：%s", what)
}

var src = netip.MustParseAddr("203.0.113.9")

// 敲门成功：登记授权、按请求的端口写防火墙放行、统计与事件；-s 报文放行来源地址。
func TestKnockGrants(t *testing.T) {
	env := newEnv(t, sshRule("ssh"))
	r := env.rule("r-ssh")
	if p := env.be.lastPlan(); len(p.Ports) != 2 || p.Ports[0] != (spa.Access{Proto: "tcp", Port: 22}) {
		t.Fatalf("拦截端口不对: %+v", p.Ports)
	}
	pkt := env.packet(t, r, spa.Message{Access: []spa.Access{{Proto: "tcp", Port: 22}}})
	if err := env.m.handle(pkt, src, "UDP"); err != nil {
		t.Fatal(err)
	}
	if !knockgate.Default.Allowed(src, "r-ssh") {
		t.Fatal("应登记授权")
	}
	waitFor(t, "写入防火墙放行", func() bool { return len(env.be.addedList()) == 1 })
	g := env.be.addedList()[0]
	if g.Addr != src || g.Access != (spa.Access{Proto: "tcp", Port: 22}) || !g.Until.Equal(env.clock.now().Add(30*time.Second)) {
		t.Fatalf("放行不对: %+v", g)
	}
	if len(env.eventsOf(events.KnockOpened)) == 0 {
		waitFor(t, "成功事件", func() bool { return len(env.eventsOf(events.KnockOpened)) == 1 })
	}
	env.m.stateMu.Lock()
	st := *env.m.stats["r-ssh"]
	c := env.m.counters
	env.m.stateMu.Unlock()
	if st.Knocks != 1 || st.LastAddr != src.String() || c.Accepted != 1 {
		t.Fatalf("统计不对: %+v %+v", st, c)
	}
	// 同一报文重放被拒绝
	if err := env.m.handle(pkt, src, "UDP"); err == nil || !strings.Contains(err.Error(), "重放") {
		t.Fatalf("重放应被拒绝: %v", err)
	}
	// 授权到期
	env.clock.set(env.clock.now().Add(31 * time.Second))
	if knockgate.Default.Allowed(src, "r-ssh") {
		t.Fatal("授权应已到期")
	}
}

// 报文写明了其他地址（fwknop -a）时放行该地址；规则要求写明地址时拒绝 -s 报文。
func TestKnockAddress(t *testing.T) {
	r1 := sshRule("a")
	r2 := sshRule("b")
	r2.RequireAddr = true
	env := newEnv(t, r1, r2)
	other := netip.MustParseAddr("198.51.100.4")
	if err := env.m.handle(env.packet(t, env.rule("r-a"), spa.Message{Addr: other}), src, "UDP"); err != nil {
		t.Fatal(err)
	}
	if !knockgate.Default.Allowed(other, "r-a") || knockgate.Default.Allowed(src, "r-a") {
		t.Fatal("应放行报文中的地址而不是来源地址")
	}
	err := env.m.handle(env.packet(t, env.rule("r-b"), spa.Message{}), src, "UDP")
	if err == nil || !strings.Contains(err.Error(), "-s") {
		t.Fatalf("要求写明地址时应拒绝 -s 报文: %v", err)
	}
	// 回环、组播地址不能被放行
	err = env.m.handle(env.packet(t, env.rule("r-a"), spa.Message{Addr: netip.MustParseAddr("127.0.0.1")}), src, "UDP")
	if err == nil {
		t.Fatal("不能放行回环地址")
	}
}

// 各种拒绝：时间戳过期、端口不在规则内、用户名不符、命令与 NAT 消息、错误密钥。
func TestKnockRejects(t *testing.T) {
	r := sshRule("ssh")
	r.User = "alice"
	env := newEnv(t, r)
	r = env.rule("r-ssh")
	cases := []struct {
		name string
		msg  spa.Message
		want string
	}{
		{"过期", spa.Message{Timestamp: env.clock.now().Unix() - 121}, "有效期"},
		{"未来", spa.Message{Timestamp: env.clock.now().Unix() + 300}, "有效期"},
		{"端口", spa.Message{Access: []spa.Access{{Proto: "tcp", Port: 22}, {Proto: "tcp", Port: 3389}}}, "tcp/3389"},
		{"用户名", spa.Message{User: "mallory"}, "用户名"},
		{"命令", spa.Message{Type: spa.CommandMsg, Command: "reboot"}, "命令"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := env.m.handle(env.packet(t, r, c.msg), src, "UDP")
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("应因 %s 拒绝: %v", c.want, err)
			}
		})
	}
	if knockgate.Default.Allowed(src, "r-ssh") {
		t.Fatal("被拒绝的敲门不应产生授权")
	}
	// 其他密钥：没有规则能通过 HMAC
	other := sshRule("other")
	if err := other.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := env.m.handle(env.packet(t, other, spa.Message{}), src, "UDP"); err == nil {
		t.Fatal("其他密钥应被拒绝")
	}
	// HMAC 密钥相同、加密密钥不同：提示加密密钥不一致
	mixed := r
	mixed.EncKey = genKey(32)
	if err := env.m.handle(env.packet(t, mixed, spa.Message{}), src, "UDP"); err == nil || !strings.Contains(err.Error(), "解密") {
		t.Fatalf("加密密钥不一致应解密失败: %v", err)
	}
	// 非 SPA 报文直接忽略，不计入拒绝、不发事件
	before := env.m.counters.Rejected
	if err := env.m.handle([]byte("GET / HTTP/1.1\r\n\r\n"), src, "UDP"); !errors.Is(err, errIgnored) {
		t.Fatalf("非 SPA 报文应被忽略: %v", err)
	}
	if env.m.counters.Rejected != before {
		t.Fatal("非 SPA 报文不应计入拒绝")
	}
}

// 失败事件按来源限速：同一来源 failInterval 内只发一次，供入侵防护计数。
func TestRejectEventsRateLimited(t *testing.T) {
	env := newEnv(t, sshRule("ssh"))
	other := sshRule("x")
	_ = other.Validate()
	for range 5 {
		_ = env.m.handle(env.packet(t, other, spa.Message{}), src, "UDP")
	}
	waitFor(t, "失败事件", func() bool { return len(env.eventsOf(events.KnockFailed)) >= 1 })
	time.Sleep(50 * time.Millisecond)
	if n := len(env.eventsOf(events.KnockFailed)); n != 1 {
		t.Fatalf("限速期内应只有 1 个失败事件，实际 %d", n)
	}
	if e := env.eventsOf(events.KnockFailed)[0]; e.Fields["ip"] != src.String() {
		t.Fatalf("事件应带来源地址: %+v", e)
	}
	env.clock.set(env.clock.now().Add(failInterval))
	_ = env.m.handle(env.packet(t, other, spa.Message{}), src, "UDP")
	waitFor(t, "第二个失败事件", func() bool { return len(env.eventsOf(events.KnockFailed)) == 2 })
	if env.m.counters.Rejected != 6 {
		t.Fatalf("拒绝计数应为 6，实际 %d", env.m.counters.Rejected)
	}
}

// 已被封禁的来源与超出速率的报文直接丢弃。
func TestBannedAndRateLimit(t *testing.T) {
	env := newEnv(t, sshRule("ssh"))
	r := env.rule("r-ssh")
	bad := netip.MustParseAddr("192.0.2.66")
	if _, _, err := banlist.Default.Ban(banlist.KeyFor(bad), banlist.Options{Duration: time.Hour, Force: true}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { banlist.Default.Unban(banlist.KeyFor(bad)) })
	if err := env.m.handle(env.packet(t, r, spa.Message{}), bad, "UDP"); !errors.Is(err, errBanned) {
		t.Fatalf("封禁来源应被丢弃: %v", err)
	}
	env.m.stateMu.Lock()
	env.m.bucket = tokenBucket{rate: 1, burst: 2}
	env.m.stateMu.Unlock()
	pkt := env.packet(t, r, spa.Message{})
	_ = env.m.handle(pkt, src, "UDP")
	_ = env.m.handle(pkt, src, "UDP")
	if err := env.m.handle(pkt, src, "UDP"); !errors.Is(err, errLimited) {
		t.Fatalf("超出速率应被丢弃: %v", err)
	}
}

// 客户端请求时长（-f）：更短时采用，更长时不超过 MaxTimeout，MaxTimeout 为 0 时忽略。
func TestEffectiveTimeout(t *testing.T) {
	r := Rule{Timeout: 30, MaxTimeout: 120}
	for _, c := range []struct{ client, want int }{{0, 30}, {10, 10}, {2, 5}, {60, 60}, {600, 120}} {
		if got := effectiveTimeout(r, c.client); got != time.Duration(c.want)*time.Second {
			t.Fatalf("client=%d: %v，期望 %ds", c.client, got, c.want)
		}
	}
	r.MaxTimeout = 0
	if got := effectiveTimeout(r, 600); got != 30*time.Second {
		t.Fatalf("MaxTimeout 为 0 时应忽略客户端请求: %v", got)
	}
}

// 真实 fwknop 客户端生成的报文（testdata 中的向量）可以通过模块认证。
func TestFwknopVector(t *testing.T) {
	raw, err := os.ReadFile("../../spa/testdata/fwknop-2.6.11.json")
	if err != nil {
		t.Fatal(err)
	}
	var vf struct {
		EncKey, HMACKey string
		Vectors         []struct {
			Name, Packet, HMAC string
			Timestamp          int64
		}
	}
	if err := json.Unmarshal(raw, &vf); err != nil {
		t.Fatal(err)
	}
	r := sshRule("fwknop")
	r.EncKey, r.HMACKey = vf.EncKey, vf.HMACKey
	r.Ports = []string{"tcp/22", "udp/51820", "tcp/2222"}
	env := newEnv(t, r)
	ok := 0
	for _, v := range vf.Vectors {
		if v.HMAC != spa.HMACSHA256 || v.Name == "cmd" || v.Name == "nat" {
			continue
		}
		env.clock.set(time.Unix(v.Timestamp+5, 0))
		if err := env.m.handle([]byte(v.Packet), src, "UDP"); err != nil {
			t.Fatalf("%s: %v", v.Name, err)
		}
		ok++
	}
	if ok < 6 {
		t.Fatalf("通过的向量太少: %d", ok)
	}
	if !knockgate.Default.Allowed(netip.MustParseAddr("203.0.113.7"), "r-fwknop") {
		t.Fatal("-a 203.0.113.7 的向量应放行该地址")
	}
}

// 修改或停用规则后撤销它的授权，并重建防火墙规则。
func TestRuleChangeRevokes(t *testing.T) {
	env := newEnv(t, sshRule("ssh"))
	if err := env.m.handle(env.packet(t, env.rule("r-ssh"), spa.Message{}), src, "UDP"); err != nil {
		t.Fatal(err)
	}
	env.m.mu.Lock()
	env.m.cfg.Rules[0].Enable = false
	env.m.ruleChangedLocked("r-ssh")
	env.m.mu.Unlock()
	if knockgate.Default.Allowed(src, "r-ssh") {
		t.Fatal("停用规则后授权应被撤销")
	}
	if env.m.fw.snapshot().Needed {
		t.Fatal("没有启用的规则时不应再拦截")
	}
	if env.be.removed == 0 {
		t.Fatal("应删除防火墙规则")
	}
}

// 防火墙规则被外部删除时轮询重建，且带上仍有效的放行；停止模块时删除规则。
func TestFirewallRecreate(t *testing.T) {
	env := newEnv(t, sshRule("ssh"))
	if err := env.m.handle(env.packet(t, env.rule("r-ssh"), spa.Message{}), src, "UDP"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "写入放行", func() bool { return len(env.be.addedList()) == 1 })
	env.be.mu.Lock()
	env.be.present = false
	n := len(env.be.applies)
	env.be.mu.Unlock()
	env.m.fw.poll(context.Background(), 1)
	plan := env.be.lastPlan()
	if len(env.be.applies) != n+1 || len(plan.Grants) != 1 || plan.Grants[0].Addr != src {
		t.Fatalf("应重建规则并带上放行: %+v", plan)
	}
	_ = env.m.Stop()
	if env.be.present {
		t.Fatal("停止模块后应删除规则")
	}
	if knockgate.Default.Allowed(src, "r-ssh") {
		t.Fatal("停止模块后授权应被清除")
	}
}

// 写入失败时记录原因，定期重试成功后生效。
func TestFirewallRetry(t *testing.T) {
	env := newEnv(t)
	env.be.applyErr = errors.New("Error: Could not process rule: Operation not permitted")
	env.m.mu.Lock()
	r := sshRule("ssh")
	_ = r.Validate()
	env.m.cfg.Rules = append(env.m.cfg.Rules, r)
	env.m.ruleChangedLocked(r.Key)
	env.m.mu.Unlock()
	st := env.m.fw.snapshot()
	if st.Active || st.Reason != ReasonPermission {
		t.Fatalf("应报告权限不足: %+v", st)
	}
	env.be.mu.Lock()
	env.be.applyErr = nil
	env.be.mu.Unlock()
	env.m.fw.poll(context.Background(), retryEvery)
	if !env.m.fw.snapshot().Active {
		t.Fatal("重试后应生效")
	}
}

// 规则与设置的校验。
func TestValidate(t *testing.T) {
	r := Rule{Name: " ssh ", Ports: []string{"22, udp/51820", "tcp/22"}}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if r.Name != "ssh" || strings.Join(r.Ports, ",") != "tcp/22,udp/51820" || r.Timeout != defaultTimeout || r.HMACType != spa.HMACSHA256 {
		t.Fatalf("规范化不对: %+v", r)
	}
	enc, _ := base64.StdEncoding.DecodeString(r.EncKey)
	mac, _ := base64.StdEncoding.DecodeString(r.HMACKey)
	if len(enc) != 32 || len(mac) != 64 {
		t.Fatalf("应生成 32/64 字节密钥: %d %d", len(enc), len(mac))
	}
	bad := []Rule{
		{Name: "", Ports: []string{"tcp/22"}},
		{Name: "x"},
		{Name: "x", Ports: []string{"icmp/1"}},
		{Name: "x", Ports: []string{"tcp/22"}, Timeout: 1},
		{Name: "x", Ports: []string{"tcp/22"}, Timeout: 60, MaxTimeout: 30},
		{Name: "x", Ports: []string{"tcp/22"}, EncKey: "c2hvcnQ="},
		{Name: "x", Ports: []string{"tcp/22"}, HMACType: "md5"},
		{Name: "x", Ports: []string{"tcp/22"}, User: "a\x01"},
	}
	for i, b := range bad {
		if err := b.Validate(); err == nil {
			t.Fatalf("#%d 应校验失败: %+v", i, b)
		}
	}
	s := Settings{Port: 62201, AllowList: []string{" 1.2.3.4 ", "", "10.0.0.0/8"}}
	if err := s.Validate(); err != nil || len(s.AllowList) != 2 || s.MaxAge != defaultMaxAge {
		t.Fatalf("设置规范化不对: %v %+v", err, s)
	}
	for _, s := range []Settings{{Port: 70000}, {AllowList: []string{"0.0.0.0/0"}}, {AllowList: []string{"x"}}, {MaxAge: 5}} {
		if err := s.Validate(); err == nil {
			t.Fatalf("应校验失败: %+v", s)
		}
	}
}

// 免敲门网段：开启内网时包含私有网段与本机 IPv6 /64，白名单追加在后。
func TestExemptPrefixes(t *testing.T) {
	local := []netip.Prefix{netip.MustParsePrefix("2001:db8:1:2::/64")}
	s := Settings{AllowLAN: true, AllowList: []string{"198.51.100.0/24"}}
	got := s.exemptPrefixes(local)
	want := map[string]bool{"192.168.0.0/16": true, "fc00::/7": true, "2001:db8:1:2::/64": true, "198.51.100.0/24": true}
	for _, p := range got {
		delete(want, p.String())
	}
	if len(want) != 0 {
		t.Fatalf("缺少网段: %v", want)
	}
	s.AllowLAN = false
	if got := s.exemptPrefixes(local); len(got) != 1 {
		t.Fatalf("关闭内网免敲门后只应有白名单: %v", got)
	}
}

// 经 UDP 端口收到的报文完成放行（真实监听）。
func TestUDPListener(t *testing.T) {
	env := newEnv(t, sshRule("ssh"))
	l, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.LocalAddr().(*net.UDPAddr).Port
	_ = l.Close()
	env.m.mu.Lock()
	env.m.cfg.Settings.Port = port
	env.m.applyLocked()
	listening := env.m.conn != nil
	env.m.mu.Unlock()
	if !listening {
		t.Fatalf("应监听 UDP %d: %s", port, env.m.listenErr)
	}
	// 时钟用真实时间，报文时间戳才在有效期内
	env.clock.set(time.Now())
	r := env.rule("r-ssh")
	pkt := env.packet(t, r, spa.Message{Addr: netip.MustParseAddr("198.51.100.20")})
	c, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Write(pkt); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "UDP 敲门放行", func() bool { return knockgate.Default.Allowed(netip.MustParseAddr("198.51.100.20"), "r-ssh") })
	// 停用全部规则后不再监听
	env.m.mu.Lock()
	env.m.cfg.Rules[0].Enable = false
	env.m.ruleChangedLocked("r-ssh")
	listening = env.m.conn != nil
	env.m.mu.Unlock()
	if listening {
		t.Fatal("没有启用的规则时不应监听")
	}
}

// 经协议分流转交（knockgate.Submit）的报文同样处理。
func TestSubmit(t *testing.T) {
	env := newEnv(t, sshRule("ssh"))
	pkt := env.packet(t, env.rule("r-ssh"), spa.Message{})
	if err := knockgate.Default.Submit(pkt, src, "协议分流"); err != nil {
		t.Fatal(err)
	}
	if !knockgate.Default.Allowed(src, "r-ssh") {
		t.Fatal("应放行")
	}
	if !env.hasLog("经 协议分流") {
		t.Fatal("日志应注明报文通道")
	}
	_ = env.m.Stop()
	if err := knockgate.Default.Submit(pkt, src, "x"); !errors.Is(err, knockgate.ErrUnavailable) {
		t.Fatalf("模块停止后应返回 ErrUnavailable: %v", err)
	}
}

// 撤销授权：从登记表与防火墙删除；同一地址端口仍被其他规则放行时保留防火墙元素。
func TestRevoke(t *testing.T) {
	env := newEnv(t, sshRule("a"), sshRule("b"))
	for _, k := range []string{"r-a", "r-b"} {
		if err := env.m.handle(env.packet(t, env.rule(k), spa.Message{}), src, "UDP"); err != nil {
			t.Fatal(err)
		}
	}
	if n := env.m.revoke(src, "r-a"); n != 1 {
		t.Fatalf("应撤销 1 个授权: %d", n)
	}
	time.Sleep(50 * time.Millisecond)
	env.be.mu.Lock()
	deleted := len(env.be.deleted)
	env.be.mu.Unlock()
	if deleted != 0 {
		t.Fatal("规则 b 仍放行同一端口，不应从防火墙删除")
	}
	env.m.revoke(src, "")
	waitFor(t, "从防火墙删除", func() bool {
		env.be.mu.Lock()
		defer env.be.mu.Unlock()
		return len(env.be.deleted) == 1
	})
	if knockgate.Default.Allowed(src, "r-b") {
		t.Fatal("应撤销全部授权")
	}
}
