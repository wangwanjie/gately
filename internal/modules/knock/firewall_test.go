package knock

import (
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/spa"
)

var testPlan = fwPlan{
	Ports:  []spa.Access{{Proto: "tcp", Port: 22}, {Proto: "udp", Port: 51820}},
	Exempt: []netip.Prefix{netip.MustParsePrefix("192.168.0.0/16"), netip.MustParsePrefix("fc00::/7")},
}

// nftables 脚本：先建后删保证可重复执行，TCP 只拦截 SYN，白名单分地址族，放行带剩余超时。
func TestNFTScript(t *testing.T) {
	now := time.Unix(1000, 0)
	plan := testPlan
	plan.Grants = []fwGrant{
		{Addr: netip.MustParseAddr("203.0.113.5"), Access: spa.Access{Proto: "tcp", Port: 22}, Until: now.Add(2500 * time.Millisecond)},
		{Addr: netip.MustParseAddr("2001:db8::5"), Access: spa.Access{Proto: "udp", Port: 51820}, Until: now.Add(time.Minute)},
		{Addr: netip.MustParseAddr("203.0.113.6"), Access: spa.Access{Proto: "tcp", Port: 22}, Until: now.Add(-time.Second)},
	}
	s := nftScript(plan, now)
	for _, want := range []string{
		"table inet gately_knock {}\ndelete table inet gately_knock\n",
		"elements = { tcp . 22, udp . 51820 }",
		"type filter hook prerouting priority -160; policy accept;",
		"meta l4proto . th dport @ports jump gate",
		"fib daddr type != local return",
		"ct state established,related return",
		"tcp flags & (syn | ack) != syn return",
		"elements = { 192.168.0.0/16 }",
		"elements = { fc00::/7 }",
		"counter drop comment \"dropped\"",
		"add element inet gately_knock allow4 { 203.0.113.5 . tcp . 22 timeout 3s }",
		"add element inet gately_knock allow6 { 2001:db8::5 . udp . 51820 timeout 60s }",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("脚本缺少 %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "203.0.113.6") {
		t.Fatal("已到期的放行不应写入")
	}
	// 没有白名单时集合不带 elements（空 elements 是语法错误）
	s = nftScript(fwPlan{Ports: testPlan.Ports}, now)
	if strings.Contains(s, "elements = {  }") || strings.Count(s, "auto-merge") != 2 {
		t.Fatalf("空白名单集合不对:\n%s", s)
	}
}

func TestParseNFTCounts(t *testing.T) {
	out := `		ip saddr . meta l4proto . th dport @allow4 counter packets 4 bytes 240 return comment "allowed"
		ip6 saddr . meta l4proto . th dport @allow6 counter packets 1 bytes 80 return comment "allowed"
		counter packets 17 bytes 1020 drop comment "dropped"`
	c := parseNFTCounts(out)
	if c.allowed != 5 || c.dropped != 17 {
		t.Fatalf("计数不对: %+v", c)
	}
}

// iptables 规则：按端口分集合、multiport 分块、只写本地址族的白名单。
func TestIPTRules(t *testing.T) {
	plan := testPlan
	for i := 1000; i < 1020; i++ {
		plan.Ports = append(plan.Ports, spa.Access{Proto: "tcp", Port: i})
	}
	sortAccess(plan.Ports)
	v4 := iptRules(plan, false)
	for _, want := range []string{
		":GATELY_KNOCK - [0:0]", ":GATELY_KNOCK_GATE - [0:0]",
		"-A GATELY_KNOCK_GATE -m addrtype ! --dst-type LOCAL -j RETURN",
		"-A GATELY_KNOCK_GATE -p tcp ! --syn -j RETURN",
		"-A GATELY_KNOCK_GATE -s 192.168.0.0/16 -j RETURN",
		"-A GATELY_KNOCK_GATE -p tcp --dport 22 -m set --match-set gkn4_tcp_22 src -j RETURN",
		"-A GATELY_KNOCK_GATE -p udp --dport 51820 -j DROP",
		"COMMIT",
	} {
		if !strings.Contains(v4, want) {
			t.Fatalf("规则缺少 %q:\n%s", want, v4)
		}
	}
	if strings.Contains(v4, "fc00::/7") {
		t.Fatal("IPv4 规则不应包含 IPv6 白名单")
	}
	if n := strings.Count(v4, "-p tcp -m multiport"); n != 2 {
		t.Fatalf("21 个 TCP 端口应分两条 multiport: %d", n)
	}
	v6 := iptRules(plan, true)
	if !strings.Contains(v6, "fc00::/7") || !strings.Contains(v6, "gkn6_tcp_22") || strings.Contains(v6, "192.168.0.0/16") {
		t.Fatalf("IPv6 规则不对:\n%s", v6)
	}
}

// 跳转位置：只能排在 Gately 自己的跳转之后，且只有一条。
func TestJumpOK(t *testing.T) {
	cases := []struct {
		out string
		ok  bool
	}{
		{"-P PREROUTING ACCEPT\n-A PREROUTING -j GATELY_KNOCK\n-A PREROUTING -j DOCKER\n", true},
		{"-P PREROUTING ACCEPT\n-A PREROUTING -j GATELY_PRE\n-A PREROUTING -j GATELY_KNOCK\n", true},
		{"-P PREROUTING ACCEPT\n-A PREROUTING -j DOCKER\n-A PREROUTING -j GATELY_KNOCK\n", false},
		{"-P PREROUTING ACCEPT\n-A PREROUTING -j GATELY_KNOCK\n-A PREROUTING -j GATELY_KNOCK\n", false},
		{"-P PREROUTING ACCEPT\n", false},
	}
	for i, c := range cases {
		if jumpOK(c.out) != c.ok {
			t.Fatalf("#%d 应为 %v", i, c.ok)
		}
	}
}

func TestParseIPTCounts(t *testing.T) {
	out := `Chain GATELY_KNOCK_GATE (1 references)
    pkts      bytes target     prot opt in     out     source               destination
       0        0 RETURN     all  --  *      *       0.0.0.0/0            0.0.0.0/0            ADDRTYPE match dst-type !LOCAL
       3      180 RETURN     tcp  --  *      *       0.0.0.0/0            0.0.0.0/0            tcp dpt:22 match-set gkn4_tcp_22 src
       9      540 DROP       tcp  --  *      *       0.0.0.0/0            0.0.0.0/0            tcp dpt:22`
	c := parseIPTCounts(out)
	if c.allowed != 3 || c.dropped != 9 {
		t.Fatalf("计数不对: %+v", c)
	}
}

// 插入位置排在入侵防护等 Gately 其他跳转之后。
func TestLeadingGatelyJumps(t *testing.T) {
	if n := leadingGatelyJumps("-P PREROUTING ACCEPT\n-A PREROUTING -j GATELY_PRE\n-A PREROUTING -j DOCKER\n"); n != 1 {
		t.Fatalf("应为 1: %d", n)
	}
	if n := leadingGatelyJumps("-P PREROUTING ACCEPT\n-A PREROUTING -j DOCKER\n-A PREROUTING -j GATELY_PRE\n"); n != 0 {
		t.Fatalf("应为 0: %d", n)
	}
	if n := leadingGatelyJumps("-P PREROUTING ACCEPT\n"); n != 0 {
		t.Fatalf("应为 0: %d", n)
	}
}
