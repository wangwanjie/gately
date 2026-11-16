package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 管理界面的页面带有内容安全策略：脚本只能来自自身，不能被其他站点嵌入；接口响应带有更严格的策略。
func TestContentSecurityPolicyHeaders(t *testing.T) {
	a, direct, gw := newGatewayApp(t)
	h := a.Handler()
	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec
	}
	for _, p := range []string{"/", "/ddns", "/some/deep/path"} {
		rec := get(p)
		csp := rec.Header().Get("Content-Security-Policy")
		if rec.Code != 200 || csp != uiCSP {
			t.Fatalf("%s 应返回页面并带上管理界面的策略: %d %q", p, rec.Code, csp)
		}
	}
	for _, want := range []string{"default-src 'self'", "script-src 'self';", "object-src 'none'", "base-uri 'self'", "form-action 'self'", "frame-ancestors 'none'"} {
		if !strings.Contains(uiCSP, want) {
			t.Errorf("管理界面的策略应包含 %q", want)
		}
	}
	// 脚本来源不能放宽到内联、eval 或任意主机
	script := uiCSP[strings.Index(uiCSP, "script-src"):]
	script = script[:strings.Index(script, ";")]
	for _, bad := range []string{"unsafe-inline", "unsafe-eval", "*", "http:", "https:", "data:", "blob:"} {
		if strings.Contains(script, bad) {
			t.Errorf("script-src 不应包含 %q: %s", bad, script)
		}
	}
	// 接口：未登录的 401、登录接口、不存在的接口都带接口策略
	for _, p := range []string{"/api/session", "/api/status", "/api/login/providers", "/api/nope"} {
		if csp := get(p).Header().Get("Content-Security-Policy"); csp != apiCSP {
			t.Errorf("%s 应带接口策略: %q", p, csp)
		}
	}
	// 静态资源不需要策略（策略作用于页面）
	if rec := get("/assets/a.js"); rec.Code != 200 || rec.Header().Get("Content-Security-Policy") != "" {
		t.Errorf("静态资源不带策略: %d %q", rec.Code, rec.Header().Get("Content-Security-Policy"))
	}
	// 经统一网关访问的页面同样带策略
	resp := gatewayRequest(t, http.DefaultClient, "GET", gw.URL+"/app/gately/", "", "")
	if resp.Header.Get("Content-Security-Policy") != uiCSP {
		t.Errorf("经网关的页面应带策略: %q", resp.Header.Get("Content-Security-Policy"))
	}
	_ = direct

	// 被访问控制拦下的请求返回“未授权访问响应”，自定义页面保留它自己的沙箱策略，接口路径也不例外
	a.settingsMu.Lock()
	a.settings.SafePath = "/entry"
	a.settings.DenyMode = denyModeCustom
	a.settings.DenyPage = "<h1>closed</h1>"
	a.settingsMu.Unlock()
	for _, p := range []string{"/", "/api/session"} {
		rec := get(p)
		if csp := rec.Header().Get("Content-Security-Policy"); csp != denyPageCSP {
			t.Errorf("被拦下的 %s 应保留自定义页面的策略: %d %q", p, rec.Code, csp)
		}
	}
}
