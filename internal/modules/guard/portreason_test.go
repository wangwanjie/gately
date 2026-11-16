package guard

import (
	"net/netip"
	"testing"

	"github.com/wangwanjie/gately/internal/banlist"
)

func TestPortBanReason(t *testing.T) {
	cases := []struct {
		source string
		ports  []int
		want   string
	}{
		{banlist.SourceScan, nil, "端口扫描"},
		{banlist.SourceScan, []int{22, 3306}, "端口扫描（端口 22, 3306）"},
		{banlist.SourceScan, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, "端口扫描（端口 1, 2, 3, 4, 5, 6, 7, 8 等 10 个）"},
		{banlist.SourceTrap, nil, "连接诱饵端口"},
		{banlist.SourceTrap, []int{23, 3389}, "连接诱饵端口 23, 3389"},
	}
	for _, c := range cases {
		if got := portBanReason(c.source, c.ports); got != c.want {
			t.Errorf("portBanReason(%s, %v) = %q，期望 %q", c.source, c.ports, got, c.want)
		}
	}
}

func TestPortHitsAggregation(t *testing.T) {
	h := newPortHits()
	h.add(netip.MustParseAddr("1.2.3.4"), 80)
	h.add(netip.MustParseAddr("1.2.3.4"), 22)
	h.add(netip.MustParseAddr("1.2.3.4"), 22)
	// 同一 /64 的两个 IPv6 地址合并到封禁表的键
	h.add(netip.MustParseAddr("2001:db8:1:2::1"), 443)
	h.add(netip.MustParseAddr("2001:db8:1:2::9"), 25)
	if got := h["1.2.3.4"]; len(got) != 2 || got[0] != 22 || got[1] != 80 {
		t.Errorf("IPv4 端口 %v", got)
	}
	key := banlist.KeyFor(netip.MustParseAddr("2001:db8:1:2::1"))
	if got := h[key]; len(got) != 2 || got[0] != 25 || got[1] != 443 {
		t.Errorf("IPv6 端口 %v（键 %s）", got, key)
	}
}
