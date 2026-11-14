package webservice

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Passkey 只在站点的精确域名上提供：通配域名站点与默认站点会接住任意子域名，
// 单点登录父域作为 RP ID 时，攻击者控制的子域名页面可以借它们完成登录，所以返回 404。
func TestPasskeyExactDomainOnly(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer backend.Close()
	site := func(key, d string) Site {
		return Site{Enable: true, Key: key, Domains: []string{d}, Targets: []string{backend.URL}, WebAuth: true, SharedUsers: true}
	}
	l := Listener{
		Sites:   []Site{site("exact", "app.sso.test"), site("wild", "*.w.sso.test")},
		Default: Site{Enable: true, Targets: []string{backend.URL}, WebAuth: true, SharedUsers: true},
	}
	addr, r := startRunner(t, l, nil, nil)
	users, _ := normalizeAuthUsers([]AuthUser{pkAlice}, nil)
	r.auth.shared.set(users, "sso.test")
	r.auth.passkeyOn.Store(true)
	try := func(host string) int {
		req, _ := http.NewRequest("POST", "http://"+addr+authPath+passkeyPath+"/login/begin", strings.NewReader("{}"))
		req.Host = host
		req.Header.Set("Origin", "http://"+host)
		req.Header.Set("Content-Type", "application/json")
		resp, err := noRedirect().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if c := try("app.sso.test"); c != http.StatusOK {
		t.Fatalf("精确域名应可用: %d", c)
	}
	for _, h := range []string{"evil.w.sso.test", "evil.sso.test"} {
		if c := try(h); c != http.StatusNotFound {
			t.Fatalf("%s 应返回 404: %d", h, c)
		}
	}
}
