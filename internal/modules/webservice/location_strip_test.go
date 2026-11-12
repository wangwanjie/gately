package webservice

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// pathEcho 回显后端实际收到的请求路径与查询串，用于检查去前缀的结果。
func pathEcho(t *testing.T, name string) *httptest.Server {
	t.Helper()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(name + "|" + r.URL.RequestURI()))
	}))
	t.Cleanup(b.Close)
	return b
}

// TestStripLocationPrefix 覆盖 stripLocationPrefix 的路径边界：前缀本身、尾部斜杠、查询串、编码字符、点段与不匹配的路径。
func TestStripLocationPrefix(t *testing.T) {
	cases := []struct{ prefix, in, want string }{
		{"/api/zyqd/", "/api/zyqd/abc", "/abc"},
		{"/api/zyqd", "/api/zyqd/abc", "/abc"},
		{"/api/zyqd/", "/api/zyqd", "/"},
		{"/api/zyqd/", "/api/zyqd/", "/"},
		{"/api/zyqd/", "/api/zyqd/a/b/", "/a/b/"},
		{"/api/zyqd/", "/api/zyqdx/abc", "/api/zyqdx/abc"},
		{"/api/zyqd/", "/api//zyqd/a//b", "/a/b"},
		{"/api/zyqd/", "/api/zyqd/a/../b", "/b"},
		// 点段越过前缀的写法在清理后不再属于该前缀，原样不动（实际不会被分流到这里）
		{"/api/zyqd/", "/api/x/../zyqd/abc", "/abc"},
		{"/", "/abc", "/abc"},
		{"", "/abc", "/abc"},
	}
	for _, c := range cases {
		u := &url.URL{Path: c.in}
		stripLocationPrefix(u, c.prefix)
		if u.Path != c.want {
			t.Errorf("prefix=%q in=%q 得到 %q，期望 %q", c.prefix, c.in, u.Path, c.want)
		}
	}
	// 解码后的路径含 %2F 与空格时重新编码，查询串不受影响
	u := &url.URL{Path: "/p/a b/c%d", RawQuery: "x=1&y=%2F"}
	stripLocationPrefix(u, "/p/")
	if u.Path != "/a b/c%d" || u.RawQuery != "x=1&y=%2F" || u.RawPath != "" {
		t.Errorf("编码与查询串处理错误: %+v", u)
	}
}

// TestLocationStripPrefixProxy 覆盖端到端：去前缀、后端地址自带路径、查询串保留、未开启时原样转发、
// 最长前缀优先、站点后端不受影响、按用户分流沿用站点后端的路径同样去前缀。
func TestLocationStripPrefixProxy(t *testing.T) {
	main, zyqd, sub, plain, admin := pathEcho(t, "main"), pathEcho(t, "zyqd"), pathEcho(t, "sub"), pathEcho(t, "plain"), pathEcho(t, "admin")
	l := Listener{Sites: []Site{{
		Enable: true, Domains: []string{"app.test"}, Targets: []string{main.URL},
		BasicAuth: true, Users: []string{"admin:pw", "bob:pw"},
		Locations: []Location{
			{Prefix: "/api/zyqd/", Targets: []string{zyqd.URL + "/"}, StripPrefix: true},
			// 后端地址自带路径时接在去掉前缀后的路径前面
			{Prefix: "/v1/", Targets: []string{sub.URL + "/inner/"}, StripPrefix: true},
			// 未开启时保持原样转发
			{Prefix: "/keep/", Targets: []string{plain.URL}},
			// 只去前缀、沿用站点后端
			{Prefix: "/site/", StripPrefix: true},
			// 更长的前缀优先
			{Prefix: "/api/zyqd/raw/", Targets: []string{plain.URL}},
		},
		UserRoutes: []UserRoute{{Users: []string{"admin"}, Targets: []string{admin.URL}}},
	}}}
	addr, _ := startRunner(t, l, nil, nil)
	cases := []struct{ user, path, want string }{
		{"bob", "/api/zyqd/abc", "zyqd|/abc"},
		{"bob", "/api/zyqd/abc?x=1&y=%2F", "zyqd|/abc?x=1&y=%2F"},
		{"bob", "/api/zyqd/", "zyqd|/"},
		{"bob", "/api/zyqd", "zyqd|/"},
		{"bob", "/api/zyqd/a/b/", "zyqd|/a/b/"},
		{"bob", "/api/zyqdx/abc", "main|/api/zyqdx/abc"},
		{"bob", "/v1/users?id=2", "sub|/inner/users?id=2"},
		{"bob", "/keep/abc", "plain|/keep/abc"},
		{"bob", "/api/zyqd/raw/abc", "plain|/api/zyqd/raw/abc"},
		{"bob", "/site/abc", "main|/abc"},
		{"bob", "/other", "main|/other"},
		// 点段清理后落在前缀之外，仍按站点后端处理（与未开启去前缀时的行为一致）；清理后落回前缀内的写法同样去前缀
		{"bob", "/api/zyqd/../other", "main|/api/zyqd/../other"},
		{"bob", "/x/../api/zyqd/abc", "zyqd|/abc"},
		// 按用户分流沿用站点后端的路径规则：用户后端同样收到去掉前缀的路径
		{"admin", "/site/abc", "admin|/abc"},
		{"admin", "/other", "admin|/other"},
		{"admin", "/api/zyqd/abc", "zyqd|/abc"},
	}
	for _, c := range cases {
		if _, body := get(t, http.DefaultClient, addr, "app.test", c.path, basicHeader(c.user, "pw")); body != c.want {
			t.Errorf("%s 访问 %s 得到 %q，期望 %q", c.user, c.path, body, c.want)
		}
	}
}

// TestLocationStripPrefixValidate 覆盖保存校验：只开启去前缀的规则合法，什么都没填的规则仍被拒绝。
func TestLocationStripPrefixValidate(t *testing.T) {
	ok := Listener{Port: 80, Sites: []Site{{Domains: []string{"a.com"}, Targets: []string{"http://x"}, Locations: []Location{{Prefix: "/api/", StripPrefix: true}}}}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("只开启去前缀应通过: %v", err)
	}
	bad := Listener{Port: 80, Sites: []Site{{Domains: []string{"a.com"}, Targets: []string{"http://x"}, Locations: []Location{{Prefix: "/api/"}}}}}
	if err := bad.Validate(); err == nil {
		t.Fatal("没有后端、请求头也未开启去前缀的规则应被拒绝")
	}
}
