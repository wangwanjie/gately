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

// 优先拒绝名单（#95）：先于白名单、黑名单判断，与模式无关，订阅列表实时生效。
func TestFilterDenyFirst(t *testing.T) {
	set, _, _ := Parse(strings.NewReader("198.51.100.0/24\n"))
	Default.Store("t-deny", set)
	t.Cleanup(func() { Default.Store("t-deny", nil) })

	addr := func(s string) net.Addr { return &net.TCPAddr{IP: net.ParseIP(s), Port: 1} }
	// 白名单与拒绝名单同时命中：拒绝优先；拒绝名单未命中的白名单成员放行
	wl := (&Access{IPMode: ModeWhitelist, IPList: []string{"203.0.113.0/24", "198.51.100.0/24"}, IPDenyList: []string{"203.0.113.9"}, IPDenySets: []string{"t-deny"}}).Compile()
	// 模式关闭时拒绝名单同样生效，且会生成过滤器
	off := (&Access{IPDenyList: []string{"10.0.0.0/8"}}).Compile()
	// 黑名单模式叠加拒绝名单
	bl := (&Access{IPMode: ModeBlacklist, IPList: []string{"192.0.2.1"}, IPDenyList: []string{"10.0.0.0/8"}}).Compile()
	cases := []struct {
		name string
		f    *Filter
		ip   string
		want bool
	}{
		{"白名单成员", wl, "203.0.113.5", true},
		{"白名单成员同时在拒绝名单", wl, "203.0.113.9", false},
		{"拒绝订阅列表命中", wl, "198.51.100.7", false},
		{"关闭模式命中拒绝名单", off, "10.1.2.3", false},
		{"关闭模式未命中", off, "8.8.8.8", true},
		{"黑名单命中拒绝名单", bl, "10.9.9.9", false},
		{"黑名单命中黑名单", bl, "192.0.2.1", false},
		{"黑名单均未命中", bl, "8.8.8.8", true},
	}
	for _, c := range cases {
		if got := c.f.Allows(addr(c.ip)); got != c.want {
			t.Errorf("%s: %s 期望 %v", c.name, c.ip, c.want)
		}
	}
	// 订阅列表清空后立即放行
	Default.Store("t-deny", nil)
	if !wl.Allows(addr("198.51.100.7")) {
		t.Error("订阅列表清空后应按未命中处理")
	}
	// 无法解析来源的地址不会命中拒绝名单
	if !off.Allows(nil) {
		t.Error("关闭模式下未知来源应放行")
	}

	bad := Access{IPDenyList: []string{"nope"}}
	if err := bad.Normalize(); err == nil {
		t.Error("无效的拒绝名单条目应报错")
	}
	ok := Access{IPDenyList: []string{" 1.2.3.4 ", ""}, IPDenySets: []string{"x", " x ", ""}}
	if err := ok.Normalize(); err != nil || len(ok.IPDenyList) != 1 || len(ok.IPDenySets) != 1 {
		t.Errorf("规范化: %v %+v", err, ok)
	}
}

// 优先拒绝名单引用的订阅列表：模式关闭时也要登记引用方并校验存在。
func TestDenySetsReferencedWithoutMode(t *testing.T) {
	Default.SetCatalog([]Info{{Key: "t-exists", Name: "存在"}})
	t.Cleanup(func() { Default.SetCatalog(nil) })
	a := Access{IPDenySets: []string{"t-exists"}}
	users := map[string][]string{}
	a.AddUsers(users, "r1")
	if len(users["t-exists"]) != 1 {
		t.Fatalf("模式关闭也应登记拒绝名单引用: %v", users)
	}
	if err := a.CheckSets(); err != nil {
		t.Fatal(err)
	}
	missing := Access{IPDenySets: []string{"t-gone"}}
	if err := missing.CheckSets(); err == nil {
		t.Fatal("引用不存在的订阅列表应报错")
	}
}
