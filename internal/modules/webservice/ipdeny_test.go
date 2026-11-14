package webservice

import (
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/iplist"
)

// 优先拒绝名单（#95）：先黑后白，命中的来源无论访问控制模式、路径规则如何一律拒绝。
func TestIPDenyOverridesAllow(t *testing.T) {
	b := okBackend(t)
	const key = "test-i95"
	set, _, err := iplist.Parse(strings.NewReader("127.0.0.0/8\n"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { iplist.Default.Store(key, nil) })

	// 测试客户端来自 127.0.0.1
	allow := []string{"127.0.0.0/8"}
	l := Listener{Sites: []Site{
		// 白名单与拒绝名单同时命中：拒绝优先
		{Enable: true, Domains: []string{"both.test"}, Targets: []string{b.URL}, IPMode: IPModeWhitelist, IPList: allow, IPDenyList: []string{"127.0.0.1"}},
		// 只在白名单内：放行
		{Enable: true, Domains: []string{"white.test"}, Targets: []string{b.URL}, IPMode: IPModeWhitelist, IPList: allow, IPDenyList: []string{"10.0.0.0/8"}},
		// 访问控制关闭时拒绝名单照样生效
		{Enable: true, Domains: []string{"off.test"}, Targets: []string{b.URL}, IPDenyList: []string{"127.0.0.0/8"}},
		// 路径规则的白名单不能覆盖站点的拒绝名单，且拒绝名单对其它路径同样有效
		{Enable: true, Domains: []string{"rule.test"}, Targets: []string{b.URL}, IPDenyList: []string{"127.0.0.1"},
			Rules: []PathRule{{Prefix: "/open", IPMode: IPModeWhitelist, IPList: allow}}},
		// 引用订阅列表作为拒绝名单
		{Enable: true, Domains: []string{"set.test"}, Targets: []string{b.URL}, IPMode: IPModeWhitelist, IPList: allow, IPDenySets: []string{key}},
		// 黑名单模式下与拒绝名单叠加
		{Enable: true, Domains: []string{"black.test"}, Targets: []string{b.URL}, IPMode: IPModeBlacklist, IPList: []string{"10.0.0.0/8"}, IPDenyList: []string{"127.0.0.1"}},
	}}
	addr, _ := startRunner(t, l, nil, nil)
	c := noRedirect()
	status := func(host, path string) int {
		t.Helper()
		resp, _ := get(t, c, addr, host, path, nil)
		return resp.StatusCode
	}

	cases := []struct {
		host, path string
		want       int
	}{
		{"both.test", "/", 403},
		{"white.test", "/", 200},
		{"off.test", "/", 403},
		{"rule.test", "/", 403},
		{"rule.test", "/open/x", 403},
		// 订阅列表尚未载入时按空列表处理，不拒绝任何人
		{"set.test", "/", 200},
		{"black.test", "/", 403},
	}
	for _, tc := range cases {
		if got := status(tc.host, tc.path); got != tc.want {
			t.Errorf("%s%s 应 %d，实际 %d", tc.host, tc.path, tc.want, got)
		}
	}

	// 订阅列表刷新后立即生效，不需要重启监听器
	iplist.Default.Store(key, set)
	if got := status("set.test", "/"); got != 403 {
		t.Errorf("订阅列表载入后应拒绝，实际 %d", got)
	}
	iplist.Default.Store(key, nil)
	if got := status("set.test", "/"); got != 200 {
		t.Errorf("订阅列表清空后应放行，实际 %d", got)
	}
}

func TestIPDenyValidate(t *testing.T) {
	s := Site{Domains: []string{"a.com"}, Targets: []string{"http://127.0.0.1:1"}, IPDenyList: []string{" 1.2.3.4 ", "", "10.0.0.0/8"}, IPDenySets: []string{"x", "x"}}
	if err := s.validate(false); err != nil {
		t.Fatal(err)
	}
	if len(s.IPDenyList) != 2 || len(s.IPDenySets) != 1 {
		t.Fatalf("应去掉空项与重复引用: %v %v", s.IPDenyList, s.IPDenySets)
	}
	bad := Site{Domains: []string{"a.com"}, Targets: []string{"http://127.0.0.1:1"}, IPDenyList: []string{"not-an-ip"}}
	if err := bad.validate(false); err == nil {
		t.Fatal("无效的拒绝名单条目应校验失败")
	}
}

// 引用的订阅列表被拒绝名单使用时，删除保护与存在性检查同样生效。
func TestIPDenySetReferences(t *testing.T) {
	m := &Module{}
	m.cfg.Listeners = []Listener{{Name: "L", Sites: []Site{{Domains: []string{"a.com"}, IPDenySets: []string{"k1"}}}}}
	if users := m.ipListUsersLocked("k1"); len(users) != 1 {
		t.Fatalf("拒绝名单的引用应计入使用方: %v", users)
	}
	if err := m.checkIPSetsLocked(&m.cfg.Listeners[0]); err == nil {
		t.Fatal("引用不存在的订阅列表应报错")
	}
}
