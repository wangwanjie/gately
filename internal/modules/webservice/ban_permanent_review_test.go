package webservice

import (
	"net/netip"
	"testing"
)

func mustAddr(t *testing.T, s string) netip.Addr {
	t.Helper()
	a, err := netip.ParseAddr(s)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// 永久封禁同样不能封禁请求者自己的地址（IPv6 按 /64 比较）；已有永久封禁不会被之后的限时封禁缩短；
// 解除后恢复访问。
func TestAddBanPermanentGuards(t *testing.T) {
	m, _ := newConnsModule(t, []Site{}, Site{})
	t.Cleanup(func() {
		for _, k := range []string{"203.0.113.7", "2001:db8:1:2::/64"} {
			m.auth.unban(k)
		}
	})
	if code, _ := callJSON(t, m.handleAddBan, "POST", "/bans", banRequest{Key: "203.0.113.7", Seconds: -1}, "203.0.113.7:4000"); code != 400 {
		t.Fatalf("永久封禁自己的地址应拒绝: %d", code)
	}
	if code, _ := callJSON(t, m.handleAddBan, "POST", "/bans", banRequest{Key: "2001:db8:1:2::99", Seconds: -1}, "[2001:db8:1:2::1]:4000"); code != 400 {
		t.Fatalf("永久封禁自己所在的 /64 应拒绝: %d", code)
	}
	if m.auth.banned(mustAddr(t, "203.0.113.7")) || m.auth.banned(mustAddr(t, "2001:db8:1:2::99")) {
		t.Fatal("被拒绝的封禁不应生效")
	}
	if code, out := callJSON(t, m.handleAddBan, "POST", "/bans", banRequest{Key: "203.0.113.7", Seconds: -1}, "192.0.2.1:4000"); code != 200 || out["key"] != "203.0.113.7" {
		t.Fatalf("永久封禁失败: %d %v", code, out)
	}
	// 之后再限时封禁同一地址，保持永久
	if code, _ := callJSON(t, m.handleAddBan, "POST", "/bans", banRequest{Key: "203.0.113.7", Seconds: 3600}, "192.0.2.1:4000"); code != 200 {
		t.Fatalf("限时封禁失败: %d", code)
	}
	var info *BanInfo
	for _, b := range m.auth.listBans() {
		if b.Key == "203.0.113.7" {
			info = &b
		}
	}
	if info == nil || !info.Permanent || !info.Until.IsZero() || !info.Manual {
		t.Fatalf("应保持手动永久封禁: %+v", info)
	}
	// IPv6 永久封禁按 /64 生效
	if code, out := callJSON(t, m.handleAddBan, "POST", "/bans", banRequest{Key: "2001:db8:1:2::99", Seconds: -1}, "192.0.2.1:4000"); code != 200 || out["key"] != "2001:db8:1:2::/64" {
		t.Fatalf("IPv6 永久封禁失败: %d %v", code, out)
	}
	if !m.auth.banned(mustAddr(t, "2001:db8:1:2::5")) {
		t.Fatal("IPv6 永久封禁应覆盖整个 /64")
	}
	m.auth.unban("203.0.113.7")
	if m.auth.banned(mustAddr(t, "203.0.113.7")) {
		t.Fatal("解除后应恢复访问")
	}
}

// 超大的封禁秒数在换算成 time.Duration 时会溢出成负数，不能因此绕过 30 天上限变成永久封禁。
func TestBanSecondsOverflowRejected(t *testing.T) {
	m, _ := newConnsModule(t, []Site{}, Site{})
	t.Cleanup(func() { m.auth.unban("203.0.113.8") })
	for _, s := range []int{10000000000, 30*86400 + 1} {
		if code, _ := callJSON(t, m.handleAddBan, "POST", "/bans", banRequest{Key: "203.0.113.8", Seconds: s}, "192.0.2.1:4000"); code != 400 {
			t.Fatalf("seconds=%d 应拒绝: %d", s, code)
		}
		if code, _ := callJSON(t, m.handleCloseConn, "POST", "/connections/close", closeConnRequest{Listener: "l1", Site: "a", ID: "1", BanSeconds: s}, "192.0.2.1:4000"); code != 400 {
			t.Fatalf("banSeconds=%d 应拒绝: %d", s, code)
		}
	}
	if m.auth.banned(mustAddr(t, "203.0.113.8")) {
		t.Fatal("被拒绝的封禁不应生效")
	}
	if code, _ := callJSON(t, m.handleAddBan, "POST", "/bans", banRequest{Key: "203.0.113.8", Seconds: 30 * 86400}, "192.0.2.1:4000"); code != 200 {
		t.Fatalf("30 天整应允许: %d", code)
	}
}
