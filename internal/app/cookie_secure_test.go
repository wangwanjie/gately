package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 登录 Cookie 的 Secure 属性：请求经 HTTPS 的可信代理而来时加上，访客自己带的 X-Forwarded-Proto 不算。
func TestLoginCookieSecureBehindProxy(t *testing.T) {
	a, _, _ := loginApp(t)
	h := a.Handler()
	login := func(remote, proto string) *http.Cookie {
		req := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"username":"admin","password":"test-password"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = remote
		if proto != "" {
			req.Header.Set("X-Forwarded-Proto", proto)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		for _, ck := range rec.Result().Cookies() {
			if ck.Name == "gately_token" {
				return ck
			}
		}
		t.Fatalf("登录失败: %d %s", rec.Code, rec.Body.String())
		return nil
	}
	// 本机反向代理（回环地址默认是可信代理）终止了 HTTPS
	if ck := login("127.0.0.1:40000", "https"); !ck.Secure || !ck.HttpOnly {
		t.Fatalf("经 HTTPS 的可信代理登录，Cookie 应带 Secure 与 HttpOnly: %+v", ck)
	}
	// 代理对外是 HTTP
	if ck := login("127.0.0.1:40000", "http"); ck.Secure {
		t.Fatal("代理对外是 HTTP 时不应带 Secure，否则浏览器不会保存 Cookie")
	}
	if ck := login("127.0.0.1:40000", ""); ck.Secure {
		t.Fatal("直接经 HTTP 访问时不应带 Secure")
	}
	// 不是可信代理的来源自己带上请求头
	if ck := login("203.0.113.5:40000", "https"); ck.Secure {
		t.Fatal("非可信代理带来的 X-Forwarded-Proto 不应采信")
	}
}

// 经统一网关用密码登录：网关以 HTTPS 对外提供时 Cookie 带 Secure。
func TestGatewayPasswordLoginCookieSecure(t *testing.T) {
	_, h := newOriginApp(t, t.TempDir(), true)
	for proto, want := range map[string]bool{"https": true, "http": false, "": false} {
		req := httptest.NewRequest("POST", "http://nas.example.com/app/gately/api/login", strings.NewReader(`{"username":"admin","password":"test-password"}`))
		req.Host = "nas.example.com"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", gwLegit)
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		if proto != "" {
			req.Header.Set("X-Forwarded-Proto", proto)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var got *http.Cookie
		for _, ck := range rec.Result().Cookies() {
			if ck.Name == "gately_token" {
				got = ck
			}
		}
		if got == nil || got.Secure != want {
			t.Fatalf("X-Forwarded-Proto=%q 时 Secure 应为 %v: %+v（%d）", proto, want, got, rec.Code)
		}
	}
}
