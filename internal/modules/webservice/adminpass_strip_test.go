package webservice

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/httpx"
)

// 后台登录免认证的凭据等同于管理员身份，只有本实例能校验：开启转发登录 Cookie 的站点
// 与转发认证服务都不应收到它们，其他 Cookie 照常转发。
func TestAdminBridgeCookieNeverForwarded(t *testing.T) {
	got := make(chan string, 4)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("Cookie")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()
	l := Listener{Sites: []Site{
		{Enable: true, Key: "pass", Domains: []string{"pass.test"}, Targets: []string{backend.URL}, PassAuthCookies: true},
		{Enable: true, Key: "fa", Domains: []string{"fa.test"}, Type: TypeText, Text: "ok", ForwardAuth: ForwardAuth{URL: backend.URL}},
	}}
	addr, _ := startRunner(t, l, nil, nil)
	ck := adminBridgeCookie + "=secret1; other=1"
	for _, host := range []string{"pass.test", "fa.test"} {
		if resp, _ := get(t, noRedirect(), addr, host, "/", hdr("Cookie", ck)); resp.StatusCode != http.StatusOK {
			t.Fatalf("%s 应放行: %d", host, resp.StatusCode)
		}
		v := <-got
		if strings.Contains(v, "secret") || !strings.Contains(v, "other=1") {
			t.Fatalf("%s 转发的 Cookie 不对: %q", host, v)
		}
	}
}

// 双层部署：外层 Gately 开启转发登录 Cookie 反代到负责认证的内层 Gately，内层签发的后台登录免认证凭据
// 外层校验不过，应原样转发，内层的后台登录免认证才能生效；外层自己签发的凭据照常剥离。
// 未开启转发登录 Cookie 的站点与转发认证服务一律不转发。
func TestAdminBridgeCookieForeignForwarded(t *testing.T) {
	httpx.SetAdminBridge(func(*http.Request) (string, time.Time) { return "", time.Time{} }, func(v string) string {
		if v == "own-bridge" {
			return "admin"
		}
		return ""
	})
	t.Cleanup(func() { httpx.SetAdminBridge(nil, nil) })
	got := make(chan string, 4)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("Cookie")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()
	l := Listener{Sites: []Site{
		{Enable: true, Key: "pass", Domains: []string{"pass.test"}, Targets: []string{backend.URL}, PassAuthCookies: true},
		{Enable: true, Key: "plain", Domains: []string{"plain.test"}, Targets: []string{backend.URL}},
		{Enable: true, Key: "fa", Domains: []string{"fa.test"}, Type: TypeText, Text: "ok", ForwardAuth: ForwardAuth{URL: backend.URL}},
	}}
	addr, _ := startRunner(t, l, nil, nil)
	for _, tc := range []struct {
		host, cookie, want string
	}{
		{"pass.test", adminBridgeCookie + "=inner-bridge; other=1", adminBridgeCookie + "=inner-bridge; other=1"},
		{"pass.test", adminBridgeCookie + "=own-bridge; other=1", "other=1"},
		{"pass.test", adminBridgeCookie + "=own-bridge; " + adminBridgeCookie + "=inner-bridge", adminBridgeCookie + "=inner-bridge"},
		{"plain.test", adminBridgeCookie + "=inner-bridge; other=1", "other=1"},
		{"fa.test", adminBridgeCookie + "=inner-bridge; other=1", "other=1"},
	} {
		if resp, _ := get(t, noRedirect(), addr, tc.host, "/", hdr("Cookie", tc.cookie)); resp.StatusCode != http.StatusOK {
			t.Fatalf("%s 应放行: %d", tc.host, resp.StatusCode)
		}
		if v := <-got; v != tc.want {
			t.Errorf("%s 带 %q 时后端收到 %q，应为 %q", tc.host, tc.cookie, v, tc.want)
		}
	}
}
