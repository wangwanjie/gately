package iplist

import (
	"net"
	"strings"
	"testing"
)

func tcpAddr(s string) net.Addr {
	a, _ := net.ResolveTCPAddr("tcp", s)
	return a
}

func TestAccessNormalize(t *testing.T) {
	a := Access{IPMode: ModeWhitelist, IPList: []string{" 10.0.0.0/8 ", "", "192.0.2.1"}, IPSets: []string{"a", " a ", ""}}
	if err := a.Normalize(); err != nil {
		t.Fatal(err)
	}
	if len(a.IPList) != 2 || a.IPList[0] != "10.0.0.0/8" || len(a.IPSets) != 1 {
		t.Fatalf("规范化结果错误: %+v", a)
	}
	for _, bad := range []Access{
		{IPMode: "allow"},
		{IPMode: ModeWhitelist},
		{IPMode: ModeBlacklist, IPList: []string{"10.0.0.0/33"}},
		{IPMode: ModeBlacklist, IPList: []string{"not-an-ip"}},
	} {
		if err := bad.Normalize(); err == nil {
			t.Errorf("应拒绝 %+v", bad)
		}
	}
	// 关闭时允许保留名单，便于临时停用后再开启
	off := Access{IPList: []string{"10.0.0.1"}}
	if err := off.Normalize(); err != nil || off.Compile() != nil {
		t.Errorf("关闭模式应通过校验且不生成过滤器: %v", err)
	}
}

func TestFilterAllows(t *testing.T) {
	set, _, _ := Parse(strings.NewReader("198.51.100.0/24\n"))
	Default.Store("t-rule", set)
	t.Cleanup(func() { Default.Store("t-rule", nil) })

	wl := (&Access{IPMode: ModeWhitelist, IPList: []string{"203.0.113.5"}, IPSets: []string{"t-rule"}}).Compile()
	bl := (&Access{IPMode: ModeBlacklist, IPSets: []string{"t-rule", "t-missing"}}).Compile()
	var off *Filter
	cases := []struct {
		f     *Filter
		addr  net.Addr
		allow bool
	}{
		{off, tcpAddr("192.0.2.1:1"), true},
		{wl, tcpAddr("203.0.113.5:1"), true},
		{wl, tcpAddr("198.51.100.9:1"), true},
		{wl, &net.UDPAddr{IP: net.ParseIP("::ffff:198.51.100.9"), Port: 1}, true},
		{wl, tcpAddr("192.0.2.1:1"), false},
		{wl, nil, false},
		{bl, tcpAddr("198.51.100.9:1"), false},
		{bl, &net.UDPAddr{IP: net.ParseIP("192.0.2.1"), Port: 1}, true},
		{bl, nil, true},
	}
	for i, c := range cases {
		if got := c.f.Allows(c.addr); got != c.allow {
			t.Errorf("#%d %v: 放行=%v，期望 %v", i, c.addr, got, c.allow)
		}
	}

	// 订阅列表刷新后立即生效，无需重新编译
	set2, _, _ := Parse(strings.NewReader("192.0.2.0/24\n"))
	Default.Store("t-rule", set2)
	if !wl.Allows(tcpAddr("192.0.2.1:1")) || wl.Allows(tcpAddr("198.51.100.9:1")) {
		t.Error("白名单应按刷新后的列表判断")
	}
	if bl.Allows(tcpAddr("192.0.2.1:1")) || !bl.Allows(tcpAddr("198.51.100.9:1")) {
		t.Error("黑名单应按刷新后的列表判断")
	}
}

func TestAccessCheckSetsAndUsers(t *testing.T) {
	Default.SetCatalog([]Info{{Key: "cn", Name: "中国"}})
	t.Cleanup(func() {
		Default.SetCatalog(nil)
		Default.SetUsers("t-mod", nil)
	})
	if err := (&Access{IPMode: ModeWhitelist, IPSets: []string{"cn"}}).CheckSets(); err != nil {
		t.Error(err)
	}
	if err := (&Access{IPMode: ModeBlacklist, IPSets: []string{"gone"}}).CheckSets(); err == nil {
		t.Error("引用不存在的列表应报错")
	}
	// 关闭时不检查，也不登记引用
	if err := (&Access{IPSets: []string{"gone"}}).CheckSets(); err != nil {
		t.Error(err)
	}
	type rule struct {
		name string
		acc  Access
	}
	rules := []rule{
		{"a", Access{IPMode: ModeWhitelist, IPSets: []string{"cn"}}},
		{"b", Access{IPSets: []string{"cn"}}},
		{"c", Access{IPMode: ModeBlacklist, IPSets: []string{"cn"}}},
	}
	PublishUsers("t-mod", rules, func(r *rule) (*Access, string) { return &r.acc, "t-mod / " + r.name })
	if got := strings.Join(Default.Users("cn"), ","); got != "t-mod / a,t-mod / c" {
		t.Errorf("引用方 = %q", got)
	}
}

// 名单中以 IPv4 映射形式填写的地址与网段按 IPv4 处理
func TestParsePrefixesUnmap(t *testing.T) {
	ps, err := ParsePrefixes([]string{"::ffff:192.0.2.1", "::ffff:198.51.100.0/120", "2001:db8::/32"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"192.0.2.1/32", "198.51.100.0/24", "2001:db8::/32"}
	for i, p := range ps {
		if p.String() != want[i] {
			t.Errorf("#%d = %s，期望 %s", i, p, want[i])
		}
	}
}
