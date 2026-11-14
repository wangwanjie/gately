package webservice

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/webauthn"
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

// 管理员修改共享用户的密码或 TOTP 密钥时同时删除其 Passkey（泄露处置），其他修改保留。
func TestPasskeyDroppedOnAdminCredentialChange(t *testing.T) {
	old, err := normalizeAuthUsers([]AuthUser{{Name: "alice", Password: "alice-password"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	old[0].Passkeys = []webauthn.Credential{{ID: "cid", RPID: "a.test"}}
	id := old[0].ID
	keep, _ := normalizeAuthUsers([]AuthUser{{ID: id, Name: "alice", Remark: "x"}}, old)
	if len(keep[0].Passkeys) != 1 {
		t.Fatal("未改密码与 TOTP 时应保留 Passkey")
	}
	pw, _ := normalizeAuthUsers([]AuthUser{{ID: id, Name: "alice", Password: "new-password-1"}}, old)
	if len(pw[0].Passkeys) != 0 {
		t.Fatal("修改密码后应删除 Passkey")
	}
	totp, _ := normalizeAuthUsers([]AuthUser{{ID: id, Name: "alice", TOTPSecret: auth.NewTOTPSecret()}}, old)
	if len(totp[0].Passkeys) != 0 {
		t.Fatal("设置 TOTP 后应删除 Passkey")
	}
}
