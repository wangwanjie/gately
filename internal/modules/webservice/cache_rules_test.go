package webservice

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestCachePathMatch 验证缓存规则路径：无 * 时按路径段边界前缀匹配，有 * 时整条路径通配，* 可跨 /。
func TestCachePathMatch(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"/api/products", "/api/products", true},
		{"/api/products", "/api/products/1", true},
		{"/api/products/", "/api/products", true},
		{"/api/products", "/api/productsx", false},
		{"/", "/anything", true},
		{"/api/products/*", "/api/products/1", true},
		{"/api/products/*", "/api/products/1/reviews", true},
		{"/api/products/*", "/api/products", false},
		{"/static/*.js", "/static/a/b.js", true},
		{"/static/*.js", "/static/a.css", false},
		{"/*/avatar", "/u1/avatar", true},
		{"/*/avatar", "/u1/avatar/big", false},
		{"/a*b*c", "/aXXbYYc", true},
		{"/a*b*c", "/aXXbYY", false},
		{"*", "/x", true},
	}
	for _, c := range cases {
		if got := cachePathMatch(c.pattern, c.path); got != c.want {
			t.Errorf("cachePathMatch(%q, %q) = %v，期望 %v", c.pattern, c.path, got, c.want)
		}
	}
	// 构造的长路径与多个 * 不会让匹配退化成指数级耗时
	start := time.Now()
	long := "/" + strings.Repeat("a", 4000)
	if cachePathMatch("/*a*a*a*a*a*a*a*a*b", long) {
		t.Fatal("不应匹配")
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("通配匹配耗时过长: %v", d)
	}
}

// TestCacheRulesValidate 验证规则校验与规范化。
func TestCacheRulesValidate(t *testing.T) {
	base := func(rules ...CacheRule) *Site {
		return &Site{Type: TypeProxy, Cache: CacheConfig{Enable: true, Rules: rules}}
	}
	bad := []*Site{
		base(CacheRule{Path: "api"}),
		base(CacheRule{Path: "/a b"}),
		base(CacheRule{Path: "/a?x=1"}),
		base(CacheRule{Path: "/a//b"}),
		base(CacheRule{Path: "/a/../b"}),
		base(CacheRule{Path: "/a/."}),
		base(CacheRule{Path: "/a"}, CacheRule{Path: " /a "}),
		base(CacheRule{Path: "/a", TTL: -1}),
		base(CacheRule{Path: "/a", TTL: maxCacheTTL + 1}),
		// 站点默认与规则都没有缓存时间时，忽略后端指令等于不缓存，提示填写
		base(CacheRule{Path: "/a", Override: true}),
	}
	for i, s := range bad {
		if err := s.validateProxyExtras(); err == nil {
			t.Errorf("第 %d 组非法规则应被拒绝: %+v", i, s.Cache.Rules)
		}
	}
	many := make([]CacheRule, maxCacheRules+1)
	for i := range many {
		many[i] = CacheRule{Path: "/p" + strings.Repeat("x", i)}
	}
	if err := base(many...).validateProxyExtras(); err == nil {
		t.Error("规则数量超过上限应被拒绝")
	}

	s := base(CacheRule{Path: " /api/products/* ", TTL: 3600}, CacheRule{Path: "/api/user", NoCache: true, TTL: 99, Override: true})
	if err := s.validateProxyExtras(); err != nil {
		t.Fatal(err)
	}
	if s.Cache.Rules[0].Path != "/api/products/*" {
		t.Fatalf("路径应去掉首尾空白: %q", s.Cache.Rules[0].Path)
	}
	if r := s.Cache.Rules[1]; r.TTL != 0 || r.Override {
		t.Fatalf("不缓存的规则应清掉缓存时间与忽略后端指令: %+v", r)
	}
	// 站点有默认缓存时间时，规则可以只勾选忽略后端指令
	s = base(CacheRule{Path: "/a", Override: true})
	s.Cache.TTL = 60
	if err := s.validateProxyExtras(); err != nil {
		t.Fatal(err)
	}
	// 未启用缓存时不校验规则（界面上不显示）
	s = &Site{Type: TypeProxy, Cache: CacheConfig{Rules: []CacheRule{{Path: "bad"}}}}
	if err := s.validateProxyExtras(); err != nil {
		t.Fatalf("未启用缓存时不应校验规则: %v", err)
	}
}

// cacheRulesBackend 按路径返回不同的缓存头，并记录每个路径被访问的次数。
func cacheRulesBackend(t *testing.T) (*httptest.Server, func(string) int) {
	var mu sync.Mutex
	hits := map[string]int{}
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		switch {
		case strings.HasPrefix(r.URL.Path, "/nostore/"):
			w.Header().Set("Cache-Control", "no-store")
		case strings.HasPrefix(r.URL.Path, "/short/"):
			w.Header().Set("Cache-Control", "max-age=1")
		case strings.HasPrefix(r.URL.Path, "/cookie/"):
			http.SetCookie(w, &http.Cookie{Name: "s", Value: "1"})
		}
		_, _ = w.Write([]byte("body:" + r.URL.Path))
	}))
	t.Cleanup(b.Close)
	return b, func(p string) int {
		mu.Lock()
		defer mu.Unlock()
		return hits[p]
	}
}

// TestCacheRulesServe 验证按路径的缓存规则：命中规则时使用规则的缓存时间，更具体的规则优先，
// 不缓存的路径不能用非规范写法绕过，忽略后端指令时仍不缓存带 Set-Cookie 的响应，未命中规则时使用站点默认。
func TestCacheRulesServe(t *testing.T) {
	backend, hits := cacheRulesBackend(t)
	site := Site{Enable: true, Domains: []string{"r.test"}, Targets: []string{backend.URL}, Cache: CacheConfig{Enable: true, TTL: 0, Rules: []CacheRule{
		{Path: "/api", NoCache: true},
		{Path: "/api/products", TTL: 3600},
		{Path: "/api/products/*/reviews", NoCache: true},
		{Path: "/nostore/*", TTL: 120, Override: true},
		{Path: "/cookie", TTL: 120, Override: true},
		{Path: "/short", TTL: 600, Override: true},
	}}}
	if err := site.validateProxyExtras(); err != nil {
		t.Fatal(err)
	}
	addr, r := startRunner(t, Listener{Sites: []Site{site}}, nil, nil)
	c := http.DefaultClient
	x := func(path string) string {
		t.Helper()
		resp, _ := get(t, c, addr, "r.test", path, nil)
		return resp.Header.Get("X-Cache")
	}
	twice := func(path string) (string, string) {
		t.Helper()
		return x(path), x(path)
	}

	// 站点默认 TTL 为 0（只遵循后端指令），后端没有缓存头的 /api/products 靠规则的 3600 秒缓存
	if a, b := twice("/api/products?page=1"); a != "MISS" || b != "HIT" {
		t.Fatalf("/api/products 应按规则缓存: %s %s", a, b)
	}
	if hits("/api/products") != 1 {
		t.Fatalf("后端应只被访问一次: %d", hits("/api/products"))
	}
	if a, b := twice("/api/products/7"); a != "MISS" || b != "HIT" {
		t.Fatalf("/api/products/7 按前缀命中规则应缓存: %s %s", a, b)
	}
	// 更具体的 /api/products/*/reviews 优先于 /api/products；/api/user/profile 只命中 /api，不缓存
	for _, p := range []string{"/api/products/7/reviews", "/api/user/profile"} {
		if a, b := twice(p); a != "" || b != "" {
			t.Fatalf("%s 不应走缓存: %q %q", p, a, b)
		}
	}
	if hits("/api/user/profile") != 2 {
		t.Fatalf("不缓存的路径每次都应访问后端: %d", hits("/api/user/profile"))
	}
	// 用连续斜杠或 .. 绕过不缓存规则：请求路径先规范化再匹配，仍按 /api 不缓存
	for _, p := range []string{"//api/user/profile", "/api//user/profile", "/static/../api/user/profile"} {
		before := hits(p) + hits("/api/user/profile")
		if a, b := twice(p); a != "" || b != "" {
			t.Fatalf("%s 应按规范化路径命中不缓存规则: %q %q", p, a, b)
		}
		// 两次请求都应真正到达后端（排除被跳转或拒绝而碰巧没有 X-Cache 的情况）
		if hits(p)+hits("/api/user/profile")-before < 2 {
			t.Fatalf("%s 两次请求都应到达后端", p)
		}
	}
	// 忽略后端指令：后端返回 no-store 也按规则缓存；max-age=1 被规则的 600 秒取代
	if a, b := twice("/nostore/x"); a != "MISS" || b != "HIT" {
		t.Fatalf("忽略后端指令时 no-store 响应应被缓存: %s %s", a, b)
	}
	if a, b := twice("/short/x"); a != "MISS" || b != "HIT" {
		t.Fatalf("忽略后端指令时应按规则缓存: %s %s", a, b)
	}
	// 带 Set-Cookie 的响应无论如何都不缓存
	if _, b := twice("/cookie/x"); b == "HIT" {
		t.Fatal("带 Set-Cookie 的响应即使忽略后端指令也不应缓存")
	}
	// 未命中任何规则时使用站点默认（TTL 0 且后端无缓存头 → 不缓存）
	if _, b := twice("/other"); b == "HIT" {
		t.Fatal("未命中规则且站点默认 TTL 为 0 时不应缓存")
	}
	// 带 Cookie 的请求仍然不走缓存
	if resp, _ := get(t, c, addr, "r.test", "/api/products?page=1", map[string]string{"Cookie": "a=b"}); resp.Header.Get("X-Cache") == "HIT" {
		t.Fatal("带 Cookie 的请求不应命中缓存")
	}

	// 条目的过期时间按规则：/short/x 为 600 秒而非后端的 1 秒
	cache := r.current().sites[0].cache
	cache.mu.Lock()
	for _, el := range cache.items {
		e := el.Value.(*cacheEntry)
		if e.path == "/short/x" {
			if d := e.expires.Sub(e.stored); d != 600*time.Second {
				cache.mu.Unlock()
				t.Fatalf("规则缓存时间应为 600 秒: %v", d)
			}
		}
	}
	cache.mu.Unlock()
}

// TestCacheRuleDefaultTTL 验证规则未填缓存时间时沿用站点默认，以及“只缓存指定路径”的写法（/ 设为不缓存）。
func TestCacheRuleDefaultTTL(t *testing.T) {
	backend, _ := cacheRulesBackend(t)
	site := Site{Enable: true, Domains: []string{"d.test"}, Targets: []string{backend.URL}, Cache: CacheConfig{Enable: true, TTL: 30, Rules: []CacheRule{
		{Path: "/", NoCache: true},
		{Path: "/assets"},
	}}}
	if err := site.validateProxyExtras(); err != nil {
		t.Fatal(err)
	}
	addr, r := startRunner(t, Listener{Sites: []Site{site}}, nil, nil)
	x := func(path string) string {
		resp, _ := get(t, http.DefaultClient, addr, "d.test", path, nil)
		return resp.Header.Get("X-Cache")
	}
	if x("/assets/a.css") != "MISS" || x("/assets/a.css") != "HIT" {
		t.Fatal("/assets 应沿用站点默认 30 秒缓存")
	}
	if x("/index.html") != "" || x("/index.html") != "" {
		t.Fatal("/ 设为不缓存后，其他路径都不应缓存")
	}
	cache := r.current().sites[0].cache
	cache.mu.Lock()
	defer cache.mu.Unlock()
	for _, el := range cache.items {
		e := el.Value.(*cacheEntry)
		if d := e.expires.Sub(e.stored); d != 30*time.Second {
			t.Fatalf("应沿用站点默认 30 秒: %v", d)
		}
	}
}

// TestRespCachePurge 验证按路径清空与统计。
func TestRespCachePurge(t *testing.T) {
	c := newRespCache(CacheConfig{MaxMB: 1})
	exp := time.Now().Add(time.Hour)
	for _, p := range []string{"/api/products", "/api/products/1", "/api/productsx", "/static/a.js", "/static/b.css"} {
		c.put(&cacheEntry{key: p, path: p, body: []byte("12345"), expires: exp})
	}
	if n, size := c.stats(); n != 5 || size != 25 {
		t.Fatalf("统计不符: %d %d", n, size)
	}
	if n := c.purge("/api/products"); n != 2 {
		t.Fatalf("按前缀应清空 2 条: %d", n)
	}
	if c.get("/api/productsx") == nil {
		t.Fatal("/api/productsx 不在 /api/products 之下，不应被清空")
	}
	if n := c.purge("/static/*.js"); n != 1 || c.get("/static/b.css") == nil {
		t.Fatalf("按通配应只清空 .js: %d", n)
	}
	if n := c.purge(""); n != 2 {
		t.Fatalf("清空全部应删除剩余 2 条: %d", n)
	}
	if n, size := c.stats(); n != 0 || size != 0 {
		t.Fatalf("清空后统计应归零: %d %d", n, size)
	}
}

// TestCacheAPI 验证缓存状态与清空接口：站点隔离、参数校验、未启用缓存的站点。
func TestCacheAPI(t *testing.T) {
	backend, _ := cacheRulesBackend(t)
	m, addr := newConnsModule(t, []Site{
		{Enable: true, Key: "a", Domains: []string{"a.test"}, Targets: []string{backend.URL}, Cache: CacheConfig{Enable: true, TTL: 60, MaxMB: 8}},
		{Enable: true, Key: "b", Domains: []string{"b.test"}, Targets: []string{backend.URL}, Cache: CacheConfig{Enable: true, TTL: 60, MaxMB: 8}},
		{Enable: true, Key: "n", Domains: []string{"n.test"}, Targets: []string{backend.URL}},
	}, Site{Enable: true, Targets: []string{backend.URL}})
	for _, h := range []string{"a.test", "b.test"} {
		for _, p := range []string{"/api/products/1", "/api/products/2", "/static/x.js"} {
			get(t, http.DefaultClient, addr, h, p, nil)
		}
	}
	code, out := callJSON(t, m.handleCacheStats, "GET", "/cache?listener=l1&site=a", nil, "")
	if code != 200 || out["enabled"] != true || out["entries"].(float64) != 3 || out["maxBytes"].(float64) != 8<<20 {
		t.Fatalf("站点 a 缓存状态不符: %d %v", code, out)
	}
	if code, out = callJSON(t, m.handleCacheStats, "GET", "/cache?listener=l1&site=n", nil, ""); code != 200 || out["enabled"] != false {
		t.Fatalf("未启用缓存的站点应返回 enabled=false: %d %v", code, out)
	}
	if code, _ = callJSON(t, m.handleCacheStats, "GET", "/cache?listener=l1&site=nope", nil, ""); code != 404 {
		t.Fatalf("不存在的站点应返回 404: %d", code)
	}

	code, out = callJSON(t, m.handleCachePurge, "POST", "/cache/purge", cachePurgeRequest{Listener: "l1", Site: "a", Path: "/api/products/*"}, "")
	if code != 200 || out["removed"].(float64) != 2 || out["entries"].(float64) != 1 {
		t.Fatalf("按路径清空站点 a 不符: %d %v", code, out)
	}
	// 清空站点 a 不影响站点 b
	if _, out = callJSON(t, m.handleCacheStats, "GET", "/cache?listener=l1&site=b", nil, ""); out["entries"].(float64) != 3 {
		t.Fatalf("站点 b 的缓存不应受影响: %v", out)
	}
	if resp, _ := get(t, http.DefaultClient, addr, "a.test", "/api/products/1", nil); resp.Header.Get("X-Cache") != "MISS" {
		t.Fatal("清空后再次请求应回源")
	}
	for _, bad := range []cachePurgeRequest{
		{Listener: "l1", Site: "a", Path: "api"},
		{Listener: "l1", Site: "a", Path: "/a/../b"},
	} {
		if code, _ = callJSON(t, m.handleCachePurge, "POST", "/cache/purge", bad, ""); code != 400 {
			t.Fatalf("非法路径应返回 400: %+v %d", bad, code)
		}
	}
	if code, _ = callJSON(t, m.handleCachePurge, "POST", "/cache/purge", cachePurgeRequest{Listener: "l1", Site: "n"}, ""); code != 400 {
		t.Fatalf("未启用缓存的站点清空应返回 400: %d", code)
	}
	if code, _ = callJSON(t, m.handleCachePurge, "POST", "/cache/purge", cachePurgeRequest{Listener: "l1", Site: "nope"}, ""); code != 404 {
		t.Fatalf("不存在的站点应返回 404: %d", code)
	}
	code, out = callJSON(t, m.handleCachePurge, "POST", "/cache/purge", cachePurgeRequest{Listener: "l1", Site: "b"}, "")
	if code != 200 || out["removed"].(float64) != 3 || out["entries"].(float64) != 0 || out["bytes"].(float64) != 0 {
		t.Fatalf("清空站点 b 全部缓存不符: %d %v", code, out)
	}
}
