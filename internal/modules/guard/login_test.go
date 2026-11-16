package guard

import (
	"net/netip"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/trustsrc"
)

// 管理后台登录成功的地址 24 小时内免于诱饵端口与扫描检测的封禁（IPv6 按所在 /64），并登记为可信来源让防火墙不重定向它们；
// 手动封禁不受影响；过期后恢复正常封禁；模块停止时撤销登记。
func TestLoginExemptsFromAutoBans(t *testing.T) {
	clock := time.Unix(1_800_000_000, 0)
	oldNow := loginNow
	loginNow = func() time.Time { return clock }
	t.Cleanup(func() { loginNow = oldNow })
	m := newTestModule(t)

	admin := netip.MustParseAddr("203.0.113.9")
	admin6 := netip.MustParseAddr("2001:db8:7:8::5")
	m.onEvent(events.Event{Type: events.LoginSuccessful, Fields: map[string]string{"ip": admin.String(), "user": "admin"}})
	m.onEvent(events.Event{Type: events.LoginSuccessful, Fields: map[string]string{"ip": "[" + admin6.String() + "]:51234"}})

	// 诱饵端口与扫描检测的封禁跳过登录地址（IPv6 按所在 /64，同一 /64 的其他地址也跳过）
	m.importPortBans(banlist.SourceScan, []netip.Addr{admin}, nil)
	m.importPortBans(banlist.SourceTrap, []netip.Addr{netip.MustParseAddr("2001:db8:7:8::99")}, nil)
	if banlist.Default.Banned(admin) || banlist.Default.Banned(netip.MustParseAddr("2001:db8:7:8::99")) {
		t.Fatal("登录成功的地址不应被扫描检测、诱饵端口封禁")
	}
	// 其他来源的自动封禁照常生效：日志规则、Web 探测、TLS、敲门、文件服务登录失败等
	if _, added, err := banlist.Default.Ban(banlist.KeyFor(admin), m.cur().banOptions(0, "日志规则", banlist.SourceJailPrefix+"sshd")); err != nil || !added {
		t.Fatalf("日志规则的封禁应照常生效: %v %v", added, err)
	}
	if !banlist.Default.Banned(admin) {
		t.Fatal("日志规则的封禁应写入封禁表")
	}
	banlist.Default.Unban(banlist.KeyFor(admin))
	other := netip.MustParseAddr("203.0.113.10")
	m.importPortBans(banlist.SourceScan, []netip.Addr{other}, nil)
	if !banlist.Default.Banned(other) {
		t.Fatal("其他地址的扫描封禁照常生效")
	}
	trusted := map[netip.Addr]time.Time{}
	for _, s := range trustsrc.All() {
		trusted[s.Addr] = s.Until
	}
	if !trusted[admin].Equal(clock.Add(24*time.Hour)) || !trusted[admin6].Equal(clock.Add(24*time.Hour)) {
		t.Fatalf("登录地址应登记为可信来源 24 小时: %v", trusted)
	}
	// 手动封禁照常生效
	if _, err := ManualBan(admin.String(), 3600, "手动", "198.51.100.1", m.cur()); err != nil || !banlist.Default.Banned(admin) {
		t.Fatalf("手动封禁不受免封影响: %v", err)
	}
	banlist.Default.Unban(banlist.KeyFor(admin))

	// 24 小时后恢复扫描检测的封禁
	clock = clock.Add(24*time.Hour + time.Second)
	m.importPortBans(banlist.SourceScan, []netip.Addr{admin}, nil)
	if !banlist.Default.Banned(admin) {
		t.Fatal("免封到期后应照常封禁")
	}
	_ = m.Stop()
	for _, s := range trustsrc.All() {
		if s.Addr == admin || s.Addr == admin6 {
			t.Fatal("模块停止后应撤销可信来源登记")
		}
	}
}
