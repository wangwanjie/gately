package webservice

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// 公网 Gately 只做反向代理、内网 Gately 负责单点登录：开启转发登录 Cookie 后，经公网入口登录与访问都能通过内网认证；
// 未开启时公网入口照旧去掉 gately_sso，内网收不到会话，只能一直跳回登录页。
func TestPassAuthCookiesToBackendGately(t *testing.T) {
	var gotCookie string
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		_, _ = w.Write([]byte("app"))
	}))
	defer app.Close()
	users, err := normalizeAuthUsers([]AuthUser{{Name: "alice", Password: "pw"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// 内网 Gately：开网页认证与共享用户，单点登录域为 sso.test
	lanAddr, lan := startRunner(t, Listener{Sites: []Site{
		{Enable: true, Key: "lan", Domains: []string{"*.sso.test"}, Targets: []string{app.URL}, WebAuth: true, SharedUsers: true},
	}}, nil, nil)
	lan.auth.shared.set(users, []string{"sso.test"})
	// 公网 Gately：两个入口都反代到内网 Gately，只有 pass.sso.test 转发登录 Cookie
	vpsAddr, _ := startRunner(t, Listener{Sites: []Site{
		{Enable: true, Key: "pass", Domains: []string{"pass.sso.test"}, Targets: []string{"http://" + lanAddr}, PassAuthCookies: true},
		{Enable: true, Key: "strip", Domains: []string{"strip.sso.test"}, Targets: []string{"http://" + lanAddr}},
	}}, nil, nil)
	noFollow := noRedirect()

	// 经公网入口提交登录表单，由内网 Gately 校验并写出单点登录 Cookie
	form := url.Values{"username": {"alice"}, "password": {"pw"}, "next": {"/"}}
	req, _ := http.NewRequest("POST", "http://"+vpsAddr+authPath+"/login", strings.NewReader(form.Encode()))
	req.Host = "pass.sso.test"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := noFollow.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	var sso *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == ssoCookie {
			sso = c
		}
	}
	if resp.StatusCode != http.StatusFound || sso == nil {
		t.Fatalf("经公网入口登录应由内网 Gately 写出单点登录 Cookie: %d %+v", resp.StatusCode, resp.Cookies())
	}

	visit := func(host string) (*http.Response, string) {
		return get(t, noFollow, vpsAddr, host, "/", map[string]string{
			"User-Agent": "Mozilla/5.0", "Accept": "text/html", "Cookie": ssoCookie + "=" + sso.Value + "; other=1",
		})
	}
	// 开启转发：内网 Gately 收到会话后放行，并按它自己的规则去掉 gately_sso 再交给应用
	if resp, body := visit("pass.sso.test"); resp.StatusCode != http.StatusOK || body != "app" {
		t.Fatalf("开启转发后应通过内网认证: %d %q", resp.StatusCode, body)
	}
	if strings.Contains(gotCookie, ssoCookie) || !strings.Contains(gotCookie, "other=1") {
		t.Fatalf("应用只应收到普通 Cookie: %q", gotCookie)
	}
	// 未开启：公网入口去掉 gately_sso，内网 Gately 要求登录
	if resp, _ := visit("strip.sso.test"); resp.StatusCode != http.StatusFound || !strings.HasPrefix(resp.Header.Get("Location"), authPath+"/login") {
		t.Fatalf("未开启转发时应跳转登录: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
}

// 开启转发后本站点的会话 Cookie 与第三方登录随机数也原样交给后端；未开启时全部去掉，其他 Cookie 不受影响。
func TestPassAuthCookiesForwardsAll(t *testing.T) {
	var got string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Cookie")
	}))
	defer backend.Close()
	pass := Site{Enable: true, Key: "p", Domains: []string{"p.test"}, Targets: []string{backend.URL}, PassAuthCookies: true}
	strip := Site{Enable: true, Key: "s", Domains: []string{"s.test"}, Targets: []string{backend.URL}}
	addr, _ := startRunner(t, Listener{Key: "l", Sites: []Site{pass, strip}}, nil, nil)
	// 站点会话 Cookie 名按 监听器key/站点key 计算，与运行时一致
	pass.scope, strip.scope = "l/p", "l/s"
	for _, tc := range []struct {
		site Site
		want bool
	}{{pass, true}, {strip, false}} {
		host := tc.site.Domains[0]
		cookie := cookieName(tc.site) + "=s; " + oauthNonceCookie + "=n; " + ssoCookie + "=x; keep=1"
		get(t, http.DefaultClient, addr, host, "/", map[string]string{"Cookie": cookie})
		for _, name := range []string{cookieName(tc.site), oauthNonceCookie, ssoCookie} {
			if strings.Contains(got, name+"=") != tc.want {
				t.Fatalf("%s: Cookie %s 转发情况应为 %v: %q", host, name, tc.want, got)
			}
		}
		if !strings.Contains(got, "keep=1") {
			t.Fatalf("%s: 普通 Cookie 应保留: %q", host, got)
		}
	}
}

// 本站点启用任何认证时不能同时转发登录 Cookie。
func TestPassAuthCookiesRejectsLocalAuth(t *testing.T) {
	for _, s := range []Site{
		{Enable: true, Domains: []string{"a.test"}, Targets: []string{"http://127.0.0.1:1"}, PassAuthCookies: true, WebAuth: true, Users: []string{"u:p"}},
		{Enable: true, Domains: []string{"a.test"}, Targets: []string{"http://127.0.0.1:1"}, PassAuthCookies: true, BasicAuth: true, Users: []string{"u:p"}},
		{Enable: true, Domains: []string{"a.test"}, Targets: []string{"http://127.0.0.1:1"}, PassAuthCookies: true, ForwardAuth: ForwardAuth{URL: "http://127.0.0.1:2/verify"}},
	} {
		if err := (&Listener{Port: 80, Sites: []Site{s}}).Validate(); err == nil || !strings.Contains(err.Error(), "登录 Cookie") {
			t.Fatalf("启用本站点认证时应拒绝转发登录 Cookie: %v", err)
		}
	}
	ok := Site{Enable: true, Domains: []string{"a.test"}, Targets: []string{"http://127.0.0.1:1"}, PassAuthCookies: true}
	if err := (&Listener{Port: 80, Sites: []Site{ok}}).Validate(); err != nil {
		t.Fatalf("本站点不认证时应允许转发登录 Cookie: %v", err)
	}
}
