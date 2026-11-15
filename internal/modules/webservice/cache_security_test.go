package webservice

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCacheOverrideRespectsPrivate 验证强制缓存仍尊重后端的 Cache-Control: private（含带字段的写法），
// no-store、no-cache 与 max-age 照常被覆盖；普通模式下 private 同样不缓存。
func TestCacheOverrideRespectsPrivate(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/force/private", "/plain/private":
			w.Header().Set("Cache-Control", "private, max-age=600")
		case "/force/private-field":
			w.Header().Set("Cache-Control", `private="Set-Cookie"`)
		case "/force/nocache":
			w.Header().Set("Cache-Control", "no-cache, max-age=0")
		case "/force/nostore":
			w.Header().Set("Cache-Control", "no-store")
		}
		_, _ = w.Write([]byte("body:" + r.URL.Path))
	}))
	t.Cleanup(backend.Close)
	site := Site{Enable: true, Domains: []string{"p.test"}, Targets: []string{backend.URL}, Cache: CacheConfig{Enable: true, TTL: 30, Rules: []CacheRule{
		{Path: "/force", TTL: 600, Override: true},
	}}}
	if err := site.validateProxyExtras(); err != nil {
		t.Fatal(err)
	}
	addr, _ := startRunner(t, Listener{Sites: []Site{site}}, nil, nil)
	twice := func(path string) string {
		get(t, http.DefaultClient, addr, "p.test", path, nil)
		resp, _ := get(t, http.DefaultClient, addr, "p.test", path, nil)
		return resp.Header.Get("X-Cache")
	}
	for _, p := range []string{"/force/private", "/force/private-field", "/plain/private"} {
		if got := twice(p); got == "HIT" {
			t.Errorf("%s 后端声明 private，强制缓存与普通模式都不应缓存", p)
		}
	}
	for _, p := range []string{"/force/nocache", "/force/nostore"} {
		if got := twice(p); got != "HIT" {
			t.Errorf("%s 强制缓存应覆盖 no-store / no-cache: %q", p, got)
		}
	}
}

// TestCacheCredentialRequests 验证带常见凭据请求头或凭据查询参数的请求在任何模式下都不走缓存。
func TestCacheCredentialRequests(t *testing.T) {
	backend, _ := cacheRulesBackend(t)
	site := Site{Enable: true, Domains: []string{"k.test"}, Targets: []string{backend.URL}, Cache: CacheConfig{Enable: true, TTL: 30, Rules: []CacheRule{
		{Path: "/force", TTL: 600, Override: true},
	}}}
	if err := site.validateProxyExtras(); err != nil {
		t.Fatal(err)
	}
	addr, _ := startRunner(t, Listener{Sites: []Site{site}}, nil, nil)
	x := func(path string, hdr map[string]string) string {
		resp, _ := get(t, http.DefaultClient, addr, "k.test", path, hdr)
		return resp.Header.Get("X-Cache")
	}
	for _, base := range []string{"/plain/a", "/force/a"} {
		// 先放进缓存，确认带凭据的请求不会读到它
		x(base, nil)
		if x(base, nil) != "HIT" {
			t.Fatalf("%s 无凭据请求应命中缓存", base)
		}
		for _, h := range []string{"Proxy-Authorization", "X-Api-Key", "X-Auth-Token", "X-Access-Token", "X-Emby-Token", "X-Emby-Authorization",
			"X-MediaBrowser-Token", "X-Plex-Token", "X-Jellyfin-Token", "X-CSRF-Token", "Authorization", "Cookie"} {
			if got := x(base, map[string]string{h: "secret"}); got != "" {
				t.Errorf("%s 带 %s 的请求不应走缓存: %q", base, h, got)
			}
		}
		for _, q := range []string{"api_key=1", "API_KEY=1", "X-Plex-Token=1", "x-plex-token=1", "token=1", "Token=1", "access_token=1", "a=1&access_token=2", "a=1;token=2", "%74oken=1"} {
			if got := x(base+"?"+q, nil); got != "" {
				t.Errorf("%s?%s 带凭据参数的请求不应走缓存: %q", base, q, got)
			}
		}
	}
	// 只是名字相近的参数照常缓存
	x("/plain/b?tokens=1", nil)
	if x("/plain/b?tokens=1", nil) != "HIT" {
		t.Fatal("tokens 参数不是凭据，应照常缓存")
	}
}

// TestCacheAmbiguousPath 验证原始路径含 ;、%2F、%5C 或 \ 的请求不走缓存，避免规范化前后与后端理解不一致。
func TestCacheAmbiguousPath(t *testing.T) {
	for _, target := range []string{"/a;b", "/a/x;jsessionid=1", "/a%2Fb", "/a%2fb", "/a%5Cb", "/a%5cb", `/a\b`, "/a%252fb"} {
		r := httptest.NewRequest(http.MethodGet, "http://h.test"+target, nil)
		r.RequestURI = target
		if requestCacheable(r) {
			t.Errorf("%s 不应走缓存", target)
		}
	}
	for _, target := range []string{"/a/b", "/a/b?x=%2F;y", "/%E4%B8%AD.js"} {
		r := httptest.NewRequest(http.MethodGet, "http://h.test"+target, nil)
		r.RequestURI = target
		if !requestCacheable(r) {
			t.Errorf("%s 应可以走缓存", target)
		}
	}

	backend, hits := cacheRulesBackend(t)
	site := Site{Enable: true, Domains: []string{"a.test"}, Targets: []string{backend.URL}, Cache: CacheConfig{Enable: true, TTL: 30}}
	addr, _ := startRunner(t, Listener{Sites: []Site{site}}, nil, nil)
	for _, p := range []string{"/static/a;b.js", "/static/a%2Fb.js"} {
		get(t, http.DefaultClient, addr, "a.test", p, nil)
		resp, _ := get(t, http.DefaultClient, addr, "a.test", p, nil)
		if got := resp.Header.Get("X-Cache"); got != "" {
			t.Errorf("%s 不应走缓存: %q", p, got)
		}
	}
	if hits("/static/a;b.js") != 2 {
		t.Fatalf("含 ; 的请求每次都应到达后端: %d", hits("/static/a;b.js"))
	}
}

// TestCacheRulePathRejectsPercent 验证缓存规则路径不能含 %（规则按解码后的路径匹配）。
func TestCacheRulePathRejectsPercent(t *testing.T) {
	for _, p := range []string{"/a%2Fb", "/%E4%B8%AD", "/a%"} {
		if err := validateCachePath(p); err == nil {
			t.Errorf("%s 应被拒绝", p)
		}
	}
	if err := validateCachePath("/中文/*.js"); err != nil {
		t.Errorf("解码后的路径应允许: %v", err)
	}
}

// TestCacheCORSNotPoisoned 验证 Gately 处理跨域时，缓存不保存按某个 Origin 写入的跨域头：
// 命中时使用本次请求的 Origin，无 Origin 的请求不带跨域头。
func TestCacheCORSNotPoisoned(t *testing.T) {
	backend, hits := cacheRulesBackend(t)
	site := Site{Enable: true, Domains: []string{"c.test"}, Targets: []string{backend.URL}, CORS: true, Cache: CacheConfig{Enable: true, TTL: 30}}
	addr, _ := startRunner(t, Listener{Sites: []Site{site}}, nil, nil)
	req := func(origin string) *http.Response {
		var hdr map[string]string
		if origin != "" {
			hdr = map[string]string{"Origin": origin}
		}
		resp, _ := get(t, http.DefaultClient, addr, "c.test", "/static/app.js", hdr)
		return resp
	}
	if r := req("https://a.example"); r.Header.Get("X-Cache") != "MISS" || r.Header.Get("Access-Control-Allow-Origin") != "https://a.example" {
		t.Fatalf("首次请求应 MISS 并按来源 A 返回跨域头: %v", r.Header)
	}
	r := req("https://b.example")
	if r.Header.Get("X-Cache") != "HIT" {
		t.Fatalf("第二次请求应命中缓存: %v", r.Header)
	}
	if got := r.Header.Values("Access-Control-Allow-Origin"); len(got) != 1 || got[0] != "https://b.example" {
		t.Fatalf("命中时跨域头应按本次来源 B: %v", got)
	}
	if got := r.Header.Values("Vary"); len(got) != 1 || got[0] != "Origin" {
		t.Fatalf("命中时 Vary 不应重复: %v", got)
	}
	r = req("")
	if r.Header.Get("X-Cache") != "HIT" || r.Header.Get("Access-Control-Allow-Origin") != "" || r.Header.Get("Access-Control-Allow-Credentials") != "" {
		t.Fatalf("无 Origin 的请求命中时不应带缓存里的跨域头: %v", r.Header)
	}
	if hits("/static/app.js") != 1 {
		t.Fatalf("后端应只被访问一次: %d", hits("/static/app.js"))
	}
}

// TestCacheBackendVaryAndCORS 验证后端 Vary 除 Accept-Encoding（Gately 处理跨域时再加 Origin）外还有其他头时不缓存，
// 强制缓存也一样；后端按来源返回的 Access-Control-Allow-Origin 不缓存，返回 * 的照常缓存并在命中时保留。
func TestCacheBackendVaryAndCORS(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/vary-ua"):
			w.Header().Set("Vary", "Accept-Encoding, User-Agent")
		case strings.HasSuffix(p, "/vary-ae"):
			w.Header().Set("Vary", "Accept-Encoding")
		case strings.HasSuffix(p, "/vary-origin"):
			w.Header().Set("Vary", "Origin")
		case strings.HasSuffix(p, "/acao-echo"):
			w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
		case strings.HasSuffix(p, "/acao-star"):
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		_, _ = w.Write([]byte("body"))
	}))
	t.Cleanup(backend.Close)
	site := Site{Enable: true, Domains: []string{"v.test"}, Targets: []string{backend.URL}, Cache: CacheConfig{Enable: true, TTL: 30, Rules: []CacheRule{
		{Path: "/force", TTL: 600, Override: true},
	}}}
	if err := site.validateProxyExtras(); err != nil {
		t.Fatal(err)
	}
	addr, _ := startRunner(t, Listener{Sites: []Site{site}}, nil, nil)
	twice := func(path string, hdr map[string]string) *http.Response {
		get(t, http.DefaultClient, addr, "v.test", path, hdr)
		resp, _ := get(t, http.DefaultClient, addr, "v.test", path, hdr)
		return resp
	}
	origin := map[string]string{"Origin": "https://a.example"}
	for _, base := range []string{"/plain", "/force"} {
		if r := twice(base+"/vary-ua", nil); r.Header.Get("X-Cache") == "HIT" {
			t.Errorf("%s/vary-ua 后端按 User-Agent 区分内容，不应缓存", base)
		}
		// 站点未开启跨域，Origin 由后端处理，同样不缓存
		if r := twice(base+"/vary-origin", origin); r.Header.Get("X-Cache") == "HIT" {
			t.Errorf("%s/vary-origin 后端按 Origin 区分内容，不应缓存", base)
		}
		if r := twice(base+"/acao-echo", origin); r.Header.Get("X-Cache") == "HIT" {
			t.Errorf("%s/acao-echo 后端按来源返回跨域头，不应缓存", base)
		}
		if r := twice(base+"/vary-ae", nil); r.Header.Get("X-Cache") != "HIT" {
			t.Errorf("%s/vary-ae 只按 Accept-Encoding 区分时应缓存", base)
		}
		r := twice(base+"/acao-star", origin)
		if r.Header.Get("X-Cache") != "HIT" || r.Header.Get("Access-Control-Allow-Origin") != "*" {
			t.Errorf("%s/acao-star 应缓存并保留后端的 *: %v", base, r.Header)
		}
	}
}

// TestCacheIgnoresGatelyCookies 验证只带 Gately 自身认证 Cookie 的请求照常走缓存（单点登录域用主域名时所有请求都带这些 Cookie），
// 带其他 Cookie、或站点把登录 Cookie 转发给后端时仍不缓存。
func TestCacheIgnoresGatelyCookies(t *testing.T) {
	backend, _ := cacheRulesBackend(t)
	run := func(domain string, pass bool) func(path, cookie string) string {
		site := Site{Enable: true, Domains: []string{domain}, Targets: []string{backend.URL}, PassAuthCookies: pass, Cache: CacheConfig{Enable: true, TTL: 30}}
		if err := site.validateProxyExtras(); err != nil {
			t.Fatal(err)
		}
		addr, _ := startRunner(t, Listener{Sites: []Site{site}}, nil, nil)
		return func(path, cookie string) string {
			var hdr map[string]string
			if cookie != "" {
				hdr = map[string]string{"Cookie": cookie}
			}
			resp, _ := get(t, http.DefaultClient, addr, domain, path, hdr)
			return resp.Header.Get("X-Cache")
		}
	}
	x := run("c1.test", false)
	for _, c := range []string{"gately_sso=abc", "gately_admin=1; gately_oauth_nonce=n"} {
		x("/plain/"+c[:8], c)
		if got := x("/plain/"+c[:8], c); got != "HIT" {
			t.Errorf("只带 Gately 自身 Cookie %q 应命中缓存: %q", c, got)
		}
	}
	// 不是本实例签发的 gately_site_ Cookie 会原样转发给后端（可能是后端 Gately 的登录会话），不能忽略
	for _, c := range []string{"gately_site_0123456789ab=v", "gately_site_0123456789ab=v; gately_sso=abc"} {
		x("/plain/foreign", c)
		if got := x("/plain/foreign", c); got != "" {
			t.Errorf("带非本实例站点会话 %q 的请求不应走缓存: %q", c, got)
		}
	}
	// 夹带后端自己的 Cookie 时仍按凭据处理
	if got := x("/plain/mixed", "gately_sso=abc; sessionid=1"); got != "" {
		t.Errorf("带后端 Cookie 的请求不应走缓存: %q", got)
	}
	// 转发登录 Cookie 的站点：站点会话与单点登录 Cookie 会到后端，不能共享缓存；后台登录免认证凭据始终不转发
	y := run("c2.test", true)
	if got := y("/plain/sso", "gately_sso=abc"); got != "" {
		t.Errorf("转发登录 Cookie 的站点不应缓存带该 Cookie 的请求: %q", got)
	}
	y("/plain/admin", "gately_admin=1")
	if got := y("/plain/admin", "gately_admin=1"); got != "HIT" {
		t.Errorf("后台登录免认证 Cookie 不转发给后端，应命中缓存: %q", got)
	}
}

// TestCacheWithRealWebAuth 用真实的网页认证登录验证：已登录访客的请求（带站点会话 Cookie）能命中缓存，
// 未登录的请求读不到缓存内容，转发登录 Cookie 的站点不缓存。
func TestCacheWithRealWebAuth(t *testing.T) {
	backend, _ := cacheRulesBackend(t)
	mk := func(key, domain string, pass bool) Site {
		s := Site{Enable: true, Key: key, Domains: []string{domain}, Targets: []string{backend.URL}, PassAuthCookies: pass, Cache: CacheConfig{Enable: true, TTL: 30}}
		// 转发登录 Cookie 的站点由后端 Gately 负责认证，本站点不能再开认证
		if !pass {
			s.WebAuth, s.Users = true, []string{"alice:pw"}
		}
		if err := s.validateProxyExtras(); err != nil {
			t.Fatal(err)
		}
		return s
	}
	// c 为不认证的缓存站点，代理到的后端可能自己是一台 Gately
	open := Site{Enable: true, Key: "c", Domains: []string{"wc.test"}, Targets: []string{backend.URL}, Cache: CacheConfig{Enable: true, TTL: 30}}
	if err := open.validateProxyExtras(); err != nil {
		t.Fatal(err)
	}
	addr, _ := startRunner(t, Listener{Sites: []Site{mk("a", "wa.test", false), mk("b", "wb.test", true), open}}, nil, nil)

	cookie := loginCookie(t, addr, "wa.test", "alice", "pw")
	with := map[string]string{"Cookie": cookie}
	get(t, noRedirect(), addr, "wa.test", "/plain/x", with)
	if resp, _ := get(t, noRedirect(), addr, "wa.test", "/plain/x", with); resp.Header.Get("X-Cache") != "HIT" {
		t.Fatalf("已登录访客应命中缓存: %q", resp.Header.Get("X-Cache"))
	}
	// 缓存已有内容时，未登录的请求仍要先过认证，拿不到缓存内容
	resp, body := get(t, noRedirect(), addr, "wa.test", "/plain/x", nil)
	if resp.Header.Get("X-Cache") == "HIT" || strings.Contains(body, "body:/plain/x") {
		t.Fatalf("未登录请求不应读到缓存: %d %q", resp.StatusCode, body)
	}
	// 同一实例其他站点签发的会话 Cookie（同主机其他端口时浏览器也会带上）对后端没有意义，照常缓存
	with2 := map[string]string{"Cookie": cookie}
	get(t, noRedirect(), addr, "wc.test", "/plain/own", with2)
	if resp, _ := get(t, noRedirect(), addr, "wc.test", "/plain/own", with2); resp.Header.Get("X-Cache") != "HIT" {
		t.Fatalf("本实例签发的站点会话不影响缓存: %q", resp.Header.Get("X-Cache"))
	}
	// 后端 Gately 的会话 Cookie（本实例验证不了）原样到达后端：带它的响应不能缓存，否则未登录访客会拿到登录后的内容
	inner := map[string]string{"Cookie": "gately_site_0123456789ab=backend-session"}
	get(t, noRedirect(), addr, "wc.test", "/plain/inner", inner)
	if resp, _ := get(t, noRedirect(), addr, "wc.test", "/plain/inner", inner); resp.Header.Get("X-Cache") == "HIT" {
		t.Fatal("带后端会话的请求不应走缓存")
	}
	if resp, _ := get(t, noRedirect(), addr, "wc.test", "/plain/inner", nil); resp.Header.Get("X-Cache") == "HIT" {
		t.Fatal("未登录访客不应读到带后端会话时的响应")
	}
	// 转发登录 Cookie 的站点：后端 Gately 的会话 Cookie 到达后端，带它的请求不缓存
	cb := map[string]string{"Cookie": "gately_site_0123456789ab=v"}
	get(t, noRedirect(), addr, "wb.test", "/plain/x", cb)
	if resp, _ := get(t, noRedirect(), addr, "wb.test", "/plain/x", cb); resp.Header.Get("X-Cache") == "HIT" {
		t.Fatal("转发登录 Cookie 的站点不应缓存带会话的请求")
	}
}
