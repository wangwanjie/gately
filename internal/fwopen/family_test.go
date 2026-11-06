package fwopen

import (
	"reflect"
	"strings"
	"testing"
)

// 端口按地址族写入对应的链：只放行 IPv6 的端口不写 ip / iptables，只放行 IPv4 的不写 ip6 / ip6tables，inet 都写。
func TestReconcileFamilies(t *testing.T) {
	inet := target{Kind: "nft", Family: "inet", Table: "fw4", Chain: "input", Busy: true}
	ip := target{Kind: "nft", Family: "ip", Table: "a", Chain: "input", Policy: "drop"}
	ip6 := target{Kind: "nft", Family: "ip6", Table: "b", Chain: "input", Policy: "drop"}
	ipt := target{Kind: "iptables", Family: "ip", Table: "filter", Chain: "INPUT", Policy: "drop"}
	ip6t := target{Kind: "ip6tables", Family: "ip6", Table: "filter", Chain: "INPUT", Policy: "drop"}
	f := &fakeBackend{chains: []target{inet, ip, ip6, ipt, ip6t}, rules: map[string][]existing{}}
	m := newTestManager(f)

	v6 := Port{Proto: "tcp", Port: 50663, Family: "6"}
	v4 := Port{Proto: "udp", Port: 3478, Family: "4"}
	both := Port{Proto: "tcp", Port: 8000, End: 8010}
	status, ok := m.set("web:k", []Port{v6, v4, both}, nil)
	if !ok {
		t.Fatalf("status = %q", status)
	}
	count := func(tg target) int { return len(f.rules[tg.String()]) }
	if count(inet) != 3 || count(ip) != 2 || count(ip6) != 2 || count(ipt) != 2 || count(ip6t) != 2 {
		t.Fatalf("rules = %+v", f.rules)
	}
	for _, c := range comments(f, ip.String()) {
		if strings.Contains(c, "tcp6-") {
			t.Fatalf("IPv6 端口写入了 IPv4 链: %s", c)
		}
	}
	for _, c := range comments(f, ip6t.String()) {
		if strings.Contains(c, "udp4-") {
			t.Fatalf("IPv4 端口写入了 IPv6 链: %s", c)
		}
	}
	for _, want := range []string{"TCP 8000-8010", "TCP 50663 (IPv6)", "UDP 3478 (IPv4)", "ip6tables INPUT"} {
		if !strings.Contains(status, want) {
			t.Fatalf("status %q 缺少 %q", status, want)
		}
	}

	// 只剩 IPv6 端口时，IPv4 链上的规则全部删除
	m.set("web:k", []Port{v6}, nil)
	if count(ip) != 0 || count(ipt) != 0 || count(ip6) != 1 || count(inet) != 1 {
		t.Fatalf("rules = %+v", f.rules)
	}
	// 只有 IPv4 链可写时，IPv6 端口视为无需放行
	f2 := &fakeBackend{chains: []target{ip}, rules: map[string][]existing{}}
	m2 := newTestManager(f2)
	if s, ok := m2.set("web:k", []Port{v6}, nil); !ok || !strings.Contains(s, "无需放行") || len(f2.rules[ip.String()]) != 0 {
		t.Fatalf("status = %q rules = %+v", s, f2.rules)
	}
}

// 旧版注释（…:协议:端口）在对账时作为遗留规则删除并以新格式补回，不会残留两份。
func TestReconcileMigratesLegacyComments(t *testing.T) {
	fw4 := target{Kind: "nft", Family: "inet", Table: "fw4", Chain: "input", Busy: true}
	legacy := ownPrefix() + "stun:k:tcp:34811"
	f := &fakeBackend{chains: []target{fw4}, rules: map[string][]existing{fw4.String(): {{Comment: legacy, Ref: "old"}}}}
	m := newTestManager(f)
	m.set("stun:k", []Port{{Proto: "tcp", Port: 34811, Family: "4"}}, nil)
	got := comments(f, fw4.String())
	if len(got) != 1 || got[0] != ownPrefix()+"stun:k:tcp4-34811" {
		t.Fatalf("rules = %v", got)
	}
}

// 空端口列表等同取消放行；不安全的 owner 改用摘要，Set / Status / Clear 前后一致。
func TestSetEmptyAndUnsafeOwner(t *testing.T) {
	fw4 := target{Kind: "nft", Family: "inet", Table: "fw4", Chain: "input", Busy: true}
	f := &fakeBackend{chains: []target{fw4}, rules: map[string][]existing{}}
	m := newTestManager(f)
	owner := safeOwner(`web:a" accept; flush ruleset; "`)
	if !strings.HasPrefix(owner, "h-") {
		t.Fatalf("owner = %q", owner)
	}
	m.set(owner, []Port{{Proto: "tcp", Port: 1}}, nil)
	if c := comments(f, fw4.String()); len(c) != 1 || strings.ContainsAny(c[0], `"; `) {
		t.Fatalf("comments = %v", c)
	}
	m.set(owner, nil, nil)
	if len(f.rules[fw4.String()]) != 0 || m.status(owner) != "" {
		t.Fatalf("rules = %+v", f.rules)
	}
	if safeOwner("web:abc") != "web:abc" {
		t.Fatal("safe owner should be kept")
	}
}

// 地址无效的端口直接丢弃，绝不退化成不限地址的放行；地址决定地址族。
func TestNormalizeAddr(t *testing.T) {
	got := normalize([]Port{
		{Proto: "tcp", Port: 443, Addr: "not-an-ip"},
		{Proto: "tcp", Port: 443, Addr: "192.168.1.1", Family: "6"},
		{Proto: "tcp", Port: 443, Addr: "2001:db8::0001"},
		{Proto: "tcp", Port: 80, End: 70},
	})
	want := []Port{
		{Proto: "tcp", Port: 80},
		{Proto: "tcp", Port: 443, Family: "4", Addr: "192.168.1.1"},
		{Proto: "tcp", Port: 443, Family: "6", Addr: "2001:db8::1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	for _, p := range got {
		if strings.Contains(p.key(), ":") {
			t.Fatalf("key 含冒号: %s", p.key())
		}
	}
}

func TestNFTMatchAndIPTArgs(t *testing.T) {
	inet := target{Family: "inet"}
	ip6 := target{Family: "ip6"}
	cases := []struct {
		t    target
		p    Port
		want string
	}{
		{inet, Port{Proto: "tcp", Port: 443}, "tcp dport 443"},
		{inet, Port{Proto: "udp", Port: 3478, Family: "4"}, "meta nfproto ipv4 udp dport 3478"},
		{inet, Port{Proto: "tcp", Port: 50663, Family: "6"}, "meta nfproto ipv6 tcp dport 50663"},
		{ip6, Port{Proto: "tcp", Port: 50663, Family: "6"}, "tcp dport 50663"},
		{inet, Port{Proto: "tcp", Port: 8000, End: 8010, Family: "6", Addr: "2001:db8::1"}, "ip6 daddr 2001:db8::1 tcp dport 8000-8010"},
		{inet, Port{Proto: "tcp", Port: 80, Family: "4", Addr: "192.168.1.1"}, "ip daddr 192.168.1.1 tcp dport 80"},
	}
	for _, c := range cases {
		if got := nftMatch(c.t, c.p); got != c.want {
			t.Errorf("nftMatch(%+v) = %q, want %q", c.p, got, c.want)
		}
	}
	got := strings.Join(iptArgs(Port{Proto: "tcp", Port: 8000, End: 8010, Addr: "10.0.0.1"}), " ")
	if got != "-p tcp --dport 8000:8010 -d 10.0.0.1" {
		t.Fatalf("iptArgs = %q", got)
	}
}

func TestForListen(t *testing.T) {
	cases := []struct {
		network, host string
		ports         []int
		want          []Port
	}{
		// 双栈监听：两个地址族都放行，连续端口合并
		{"tcp", "", []int{443, 8001, 8000, 8002, 443}, []Port{{Proto: "tcp", Port: 443}, {Proto: "tcp", Port: 8000, End: 8002}}},
		{"tcp", "0.0.0.0", []int{80}, []Port{{Proto: "tcp", Port: 80}}},
		{"tcp6", "::", []int{50663}, []Port{{Proto: "tcp", Port: 50663, Family: "6"}}},
		{"udp4", "", []int{3478}, []Port{{Proto: "udp", Port: 3478, Family: "4"}}},
		{"tcp", "[2001:db8::1]", []int{443}, []Port{{Proto: "tcp", Port: 443, Addr: "2001:db8::1"}}},
		{"tcp", "fe80::1%eth0", []int{443}, []Port{{Proto: "tcp", Port: 443, Addr: "fe80::1"}}},
		// 只监听本机回环、无法识别的地址或协议：不放行
		{"tcp", "127.0.0.1", []int{80}, nil},
		{"tcp6", "::1", []int{80}, nil},
		{"tcp", "example.com", []int{80}, nil},
		{"unix", "", []int{80}, nil},
		{"tcp", "", nil, nil},
	}
	for _, c := range cases {
		if got := ForListen(c.network, c.host, c.ports); !reflect.DeepEqual(got, c.want) {
			t.Errorf("ForListen(%s, %s, %v) = %+v, want %+v", c.network, c.host, c.ports, got, c.want)
		}
	}
	// 合并后的端口经过 normalize 不变（地址族由地址决定）
	ports := normalize(ForListen("tcp", "192.168.1.1", []int{1, 2, 3}))
	if len(ports) != 1 || ports[0].Family != "4" || ports[0].String() != "TCP 192.168.1.1:1-3" {
		t.Fatalf("ports = %+v", ports)
	}
}

func TestIsLoopback(t *testing.T) {
	for h, want := range map[string]bool{"127.0.0.1": true, "[::1]": true, "127.1.2.3": true, "": false, "0.0.0.0": false, "192.168.1.1": false, "::": false, "x": false} {
		if IsLoopback(h) != want {
			t.Errorf("IsLoopback(%q) != %v", h, want)
		}
	}
}
