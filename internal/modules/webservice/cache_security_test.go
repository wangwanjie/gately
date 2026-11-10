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
