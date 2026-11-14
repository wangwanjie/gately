package guard

import (
	"context"
	"encoding/json"
	"errors"
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
	rules := iptRules(s, false, nil, nil, []portRange{{443, 443}}, []portRange{{23, 23}}, true)
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

// 诱饵端口与扫描检测都关闭时不声明端口集合。
func TestBuildScriptNoPortSetsWhenOff(t *testing.T) {
	s := Settings{Firewall: true}
	script, _, _ := buildScriptWith(true, s, nil, nil, nil, nil, time.Now())
	if strings.Contains(script, "port4") || strings.Contains(script, "port6") {
		t.Fatal("诱饵与扫描都关闭时不应声明端口集合")
	}
}

// 内核缺少 hash:ip,port 时（精简固件），iptables 后端放弃记录端口，其余集合与规则照常写入；
// 诱饵与扫描都关闭时不创建端口集合；下一次重建会重新尝试。
func TestIptRebuildPortSetFallback(t *testing.T) {
	fake := &fakeIPT{}
	fail := true
	b := newFakeIPTBackend(fake)
	b.run = func(ctx context.Context, stdin, bin string, args ...string) (string, error) {
		if fail && bin == "ipset" && strings.Contains(stdin, "hash:ip,port") {
			return "", errors.New("ipset v7.21: Kernel error received: set type not supported")
		}
		return fake.run(ctx, stdin, bin, args...)
	}
	s := defaultSettings()
	s.Trap.Enable, s.Trap.Ports = true, "23"
	s.Scan.Enable, s.Scan.Threshold = true, 12
	plan := iptPlan{open: []portRange{{443, 443}}, trap: []portRange{{23, 23}}}
	if _, _, err := b.rebuild(context.Background(), s, plan, nil, time.Now()); err != nil {
		t.Fatalf("缺少端口集合类型时重建应成功: %v", err)
	}
	if b.portLogErr == nil {
		t.Fatal("应记录端口集合创建失败的原因")
	}
	if len(fake.find("--add-set gately_port")) != 0 {
		t.Fatal("端口集合不存在时规则不应引用它")
	}
	if len(fake.find("create gately_ban4")) == 0 || len(fake.find("--add-set gately_trap4")) == 0 {
		t.Fatal("其余集合与规则应照常写入")
	}
	// 下一次重建重新尝试，内核支持后恢复记录端口
	fail = false
	fake.reset()
	if _, _, err := b.rebuild(context.Background(), s, plan, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	if b.portLogErr != nil || len(fake.find("--add-set gately_port4")) == 0 {
		t.Fatal("重新尝试成功后应记录端口")
	}
	// 诱饵与扫描都关闭：不创建端口集合
	s.Trap.Enable, s.Scan.Enable = false, false
	fake.reset()
	if _, _, err := b.rebuild(context.Background(), s, iptPlan{}, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(fake.find("hash:ip,port")) != 0 {
		t.Fatal("诱饵与扫描都关闭时不应创建端口集合")
	}
}

// nft 不支持 (地址 . 端口) 集合时放弃记录端口、其余规则照常写入，只提示一次；
// 之后每次重写仍先按记录端口尝试，偶发错误不会永久关闭端口记录。
func TestNFTPortLogFallbackRetries(t *testing.T) {
	cleanDefault(t)
	unsupported := true
	fake := &fakeNFT{fail: func(stdin string, args []string) error {
		if unsupported && strings.Contains(stdin, "set port4") {
			return errorString("Error: Could not process rule: Invalid argument")
		}
		return nil
	}}
	fw, buf := inactiveFirewall(t, fake)
	ctx := context.Background()
	s := defaultSettings()
	s.Firewall = true
	s.Trap.Enable, s.Trap.Ports = true, "23"
	fw.apply(ctx, s)
	if !fw.snapshot().Active {
		t.Fatalf("不支持端口集合时其余规则应照常写入: %+v", fw.snapshot())
	}
	fw.apply(ctx, s)
	if w := logsAt(buf, "warn"); len(w) != 1 || !strings.Contains(w[0], "命中端口") {
		t.Fatalf("应只提示一次: %v", w)
	}
	unsupported = false
	fake.mu.Lock()
	fake.calls = nil
	fake.mu.Unlock()
	fw.apply(ctx, s)
	found := false
	for _, c := range fake.calls {
		if strings.Contains(c, "set port4") {
			found = true
		}
	}
	if !found || !fw.snapshot().Active {
		t.Fatal("恢复后重写应重新记录端口")
	}
}
