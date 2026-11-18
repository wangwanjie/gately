package webservice

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
