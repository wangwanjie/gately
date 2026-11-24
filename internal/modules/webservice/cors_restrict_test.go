package webservice

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// 跨域开放给任意来源会让任意网页借访客的登录态或网络位置读取站点内容：除基本认证、网页认证外，
// 转发认证、把认证交给后端 Gately、IP 白名单（站点或路径规则）与敲门准入的站点同样只允许站点自身域名作为来源。
func TestCORSRestrictedForProtectedSites(t *testing.T) {
	base := Site{CORS: true, Domains: []string{"app.test", "*.app.test"}}
	for name, mod := range map[string]func(s *Site){
		"转发认证":        func(s *Site) { s.ForwardAuth = ForwardAuth{URL: "http://127.0.0.1:1/verify"} },
		"转发登录 Cookie": func(s *Site) { s.PassAuthCookies = true },
		"信任后端认证":      func(s *Site) { s.TrustBackendAuth = true },
		"IP 白名单":      func(s *Site) { s.IPMode = IPModeWhitelist; s.IPList = []string{"10.0.0.0/8"} },
		"路径白名单": func(s *Site) {
			s.Rules = []PathRule{{Prefix: "/admin", IPMode: IPModeWhitelist, IPList: []string{"10.0.0.0/8"}}}
		},
		"敲门准入": func(s *Site) { s.Knock = "k1" },
	} {
		s := base
		mod(&s)
		h := &siteHandler{site: s}
		if h.corsAllowed("https://evil.com") {
			t.Errorf("%s：不应向任意来源开放跨域", name)
		}
		if !h.corsAllowed("https://api.app.test") || !h.corsAllowed("http://app.test:8080") {
			t.Errorf("%s：站点自身域名应允许", name)
		}
	}
	// 黑名单与没有任何访问限制的站点保持原行为
	for name, s := range map[string]Site{
		"无限制": base,
		"黑名单": {CORS: true, Domains: []string{"app.test"}, IPMode: IPModeBlacklist, IPList: []string{"1.2.3.4"}},
	} {
		h := &siteHandler{site: s}
		if !h.corsAllowed("https://evil.com") {
			t.Errorf("%s：应保持向任意来源开放", name)
		}
	}
}

// 末尾整段通配（audiobook.*）与 * 能匹配任何人都能注册的域名，不算站点自身的来源；最后一段固定的通配照常算。
func TestOwnedDomainForOrigins(t *testing.T) {
	for rule, want := range map[string]bool{
		"app.test": true, "*.app.test": true, "audiobook.*.app.test": true, "a.api-*.app.test": true,
		"*": false, "audiobook.*": false, "a.app.te*": false, "": false,
	} {
		if got := ownedDomain(rule); got != want {
			t.Errorf("ownedDomain(%q) = %v; want %v", rule, got, want)
		}
	}
	h := &siteHandler{site: Site{CORS: true, Domains: []string{"audiobook.*", "app.test"}, IPMode: IPModeWhitelist, IPList: []string{"10.0.0.0/8"}}}
	for origin, want := range map[string]bool{
		"https://audiobook.attacker.com": false, "https://audiobook.com": false, "https://app.test": true,
	} {
		if got := h.corsAllowed(origin); got != want {
			t.Errorf("corsAllowed(%q) = %v; want %v", origin, got, want)
		}
		if got := h.ownOrigin(origin); got != want {
			t.Errorf("ownOrigin(%q) = %v; want %v", origin, got, want)
		}
	}
}

// 经监听实际请求：转发认证站点对外来来源不写跨域头，对站点自身域名照常写。
func TestCORSForwardAuthSiteHeaders(t *testing.T) {
	fa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer fa.Close()
	srv := fa.URL
	l := Listener{Sites: []Site{{Enable: true, Key: "fa", Domains: []string{"fa.test"}, Type: TypeText, Text: "ok", CORS: true, ForwardAuth: ForwardAuth{URL: srv}}}}
	addr, _ := startRunner(t, l, nil, nil)
	resp, _ := get(t, noRedirect(), addr, "fa.test", "/", hdr("Origin", "https://evil.com"))
	if v := resp.Header.Get("Access-Control-Allow-Origin"); v != "" {
		t.Fatalf("外来来源不应得到跨域许可: %q", v)
	}
	resp, _ = get(t, noRedirect(), addr, "fa.test", "/", hdr("Origin", "https://fa.test"))
	if v := resp.Header.Get("Access-Control-Allow-Origin"); v != "https://fa.test" {
		t.Fatalf("站点自身域名应得到跨域许可: %q", v)
	}
}
