package guard

import (
	"net/netip"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/trustsrc"
)

// 管理后台登录成功的地址 24 小时内免于自动封禁（IPv6 按所在 /64），并登记为可信来源供防火墙放行；
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

	if _, _, err := banlist.Default.Ban(banlist.KeyFor(admin), m.cur().banOptions(0, "端口扫描", banlist.SourceScan)); err != banlist.ErrIgnored {
		t.Fatalf("登录成功的地址不应被自动封禁: %v", err)
	}
	// 同一 /64 的其他地址也免封（封禁按 /64 进行）
	if _, _, err := banlist.Default.Ban(banlist.KeyFor(netip.MustParseAddr("2001:db8:7:8::99")), m.cur().banOptions(0, "x", banlist.SourceTrap)); err != banlist.ErrIgnored {
		t.Fatalf("登录地址所在的 /64 不应被自动封禁: %v", err)
	}
	other := netip.MustParseAddr("203.0.113.10")
	if _, added, err := banlist.Default.Ban(banlist.KeyFor(other), m.cur().banOptions(0, "x", banlist.SourceScan)); err != nil || !added {
		t.Fatalf("其他地址照常封禁: %v %v", added, err)
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

	// 24 小时后恢复自动封禁
	clock = clock.Add(24*time.Hour + time.Second)
	if _, added, err := banlist.Default.Ban(banlist.KeyFor(admin), m.cur().banOptions(0, "端口扫描", banlist.SourceScan)); err != nil || !added {
		t.Fatalf("免封到期后应照常封禁: %v %v", added, err)
	}
	_ = m.Stop()
	for _, s := range trustsrc.All() {
		if s.Addr == admin || s.Addr == admin6 {
			t.Fatal("模块停止后应撤销可信来源登记")
		}
	}
}
