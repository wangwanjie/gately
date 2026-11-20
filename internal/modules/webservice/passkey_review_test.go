package webservice

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/webauthn"
	"github.com/wangwanjie/gately/internal/webauthn/webauthntest"
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
	r.auth.shared.set(users, []string{"sso.test"})
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

// 签名计数没能保存（写盘失败、Passkey 在此期间被删除）时拒绝登录：新计数没有保存，克隆的凭据可以重放。
func TestPasskeyLoginRejectedWhenNotPersisted(t *testing.T) {
	e := newPKEnv(t, []AuthUser{pkAlice}, "")
	key := webauthntest.New()
	tok := e.token("a.test", "alice", "alice-password", "")
	if resp, out := e.register("a.test", tok, "k", key); resp.StatusCode != 200 {
		t.Fatalf("注册失败: %d %v", resp.StatusCode, out)
	}
	e.a.savePasskeys = func(string, func([]AuthUser, int) ([]webauthn.Credential, error)) error {
		return errors.New("磁盘已满")
	}
	if resp, _ := e.login("a.test", key); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("保存失败时应拒绝登录: %d", resp.StatusCode)
	}
}
