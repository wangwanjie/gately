package guard

import (
	"encoding/json"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
)

func TestKernelBanReason(t *testing.T) {
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
		if got := kernelBanReason(c.source, c.ports); got != c.want {
			t.Errorf("kernelBanReason(%s, %v) = %q，期望 %q", c.source, c.ports, got, c.want)
		}
	}
}

func TestElemAddrPort(t *testing.T) {
	for _, raw := range []string{
		`{"elem":{"val":{"concat":["1.2.3.4",22]},"timeout":300,"expires":290}}`,
		`{"concat":["1.2.3.4",22]}`,
	} {
		a, port, ok := elemAddrPort(json.RawMessage(raw))
		if !ok || a != netip.MustParseAddr("1.2.3.4") || port != 22 {
			t.Errorf("%s 解析为 %v %d %v", raw, a, port, ok)
		}
	}
	for _, raw := range []string{`"1.2.3.4"`, `{"concat":["1.2.3.4"]}`, `{"concat":["x",22]}`} {
		if _, _, ok := elemAddrPort(json.RawMessage(raw)); ok {
			t.Errorf("%s 不应解析成功", raw)
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

func TestParsePortSave(t *testing.T) {
	h := newPortHits()
	parsePortSave("create gately_port4 hash:ip,port family inet timeout 300\nadd gately_port4 1.2.3.4,tcp:22 timeout 290\nadd gately_port4 1.2.3.4,tcp:3306 timeout 100\nadd gately_port6 2001:db8::5,tcp:25 timeout 9\n", h)
	if got := h["1.2.3.4"]; len(got) != 2 || got[0] != 22 || got[1] != 3306 {
		t.Errorf("端口 %v", got)
	}
	if len(h) != 2 {
		t.Errorf("应有 2 个来源: %v", h)
	}
}

func TestBuildScriptPortLog(t *testing.T) {
	s := Settings{Firewall: true}
	s.Trap.Enable, s.Trap.Ports = true, "23"
	s.Scan.Enable, s.Scan.Threshold = true, 12
	now := time.Now()
	on, _, _ := buildScriptWith(true, s, nil, nil, []portRange{{443, 443}}, []portRange{{23, 23}}, now)
	for _, want := range []string{"set port4", "set port6", "tcp dport @trap add @port4 { ip saddr . tcp dport }", "tcp dport != @open add @port6 { ip6 saddr . tcp dport }"} {
		if !strings.Contains(on, want) {
			t.Errorf("脚本缺少 %q", want)
		}
	}
	off, _, _ := buildScriptWith(false, s, nil, nil, []portRange{{443, 443}}, []portRange{{23, 23}}, now)
	if strings.Contains(off, "port4") || strings.Contains(off, "port6") {
		t.Errorf("关闭端口记录后不应出现 port 集合")
	}
}

func TestIptRulesPortLog(t *testing.T) {
	s := Settings{Firewall: true}
	s.Trap.Enable, s.Trap.Ports = true, "23"
	s.Scan.Enable, s.Scan.Threshold = true, 12
	rules := iptRules(s, false, nil, nil, []portRange{{443, 443}}, []portRange{{23, 23}})
	want := "-j SET --add-set gately_port4 src,dst --exist --timeout 300"
	for _, chain := range []string{iptTrap, iptScan} {
		found := false
		for _, r := range rules[chain] {
			if strings.HasSuffix(r, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s 链缺少命中端口记录规则: %v", chain, rules[chain])
		}
	}
}
