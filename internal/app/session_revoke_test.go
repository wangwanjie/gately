package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/backup"
)

// cookieToken 取出客户端当前保存的会话令牌。
func cookieToken(t *testing.T, c *http.Client, base string) string {
	t.Helper()
	u, _ := url.Parse(base)
	for _, ck := range c.Jar.Cookies(u) {
		if ck.Name == auth.CookieName {
			return ck.Value
		}
	}
	t.Fatal("没有会话 Cookie")
	return ""
}

// 登出后旧令牌立即被拒，退出所有设备使全部会话失效，API 令牌不受影响。
func TestLogoutRevokesTokenAndRevokeAll(t *testing.T) {
	_, srv, c := loginApp(t)
	old := cookieToken(t, c, srv.URL)
	if code, _ := tokenRequest(t, srv.URL, "GET", "/api/status", old, nil); code != 200 {
		t.Fatalf("登录后令牌应可用，实际 %d", code)
	}
	_, apiSecret := createToken(t, srv.URL, c, map[string]any{"name": "读", "scopes": []string{"*"}})

	if code, _ := doJSON(t, c, "POST", srv.URL+"/api/logout", nil); code != 200 {
		t.Fatalf("登出失败 %d", code)
	}
	if code, _ := tokenRequest(t, srv.URL, "GET", "/api/status", old, nil); code != http.StatusUnauthorized {
		t.Fatalf("登出后旧令牌应被拒绝，实际 %d", code)
	}

	// 两个设备登录，其中一个退出所有设备
	login := func() *http.Client {
		jar, _ := cookiejar.New(nil)
		cl := &http.Client{Jar: jar}
		if code, body := doJSON(t, cl, "POST", srv.URL+"/api/login", map[string]string{"username": "admin", "password": "test-password"}); code != 200 {
			t.Fatalf("登录失败 %d %s", code, body)
		}
		return cl
	}
	c1, c2 := login(), login()
	t2 := cookieToken(t, c2, srv.URL)
	if code, _ := doJSON(t, c1, "POST", srv.URL+"/api/account/sessions/revoke", nil); code != 200 {
		t.Fatalf("退出所有设备失败 %d", code)
	}
	if code, _ := tokenRequest(t, srv.URL, "GET", "/api/status", t2, nil); code != http.StatusUnauthorized {
		t.Fatalf("退出所有设备后其他会话应失效，实际 %d", code)
	}
	if code, _ := doJSON(t, c1, "GET", srv.URL+"/api/status", nil); code != http.StatusUnauthorized {
		t.Fatalf("当前会话也应失效，实际 %d", code)
	}
	if code, _ := tokenRequest(t, srv.URL, "GET", "/api/status", apiSecret, nil); code != 200 {
		t.Fatalf("API 令牌不应受影响，实际 %d", code)
	}
	// API 令牌不能调用该接口
	if code, _ := tokenRequest(t, srv.URL, "POST", "/api/account/sessions/revoke", apiSecret, nil); code != 403 {
		t.Fatalf("API 令牌不应能退出所有设备，实际 %d", code)
	}
}

// 恢复账号文件时换新的会话密钥：恢复旧备份不会让此后吊销的会话（纪元与吊销列表回退）重新生效。
func TestRestoreRotatesSessionSecret(t *testing.T) {
	a, srv, c := loginApp(t)
	resp, err := c.Get(srv.URL + "/api/system/backup")
	if err != nil {
		t.Fatal(err)
	}
	bk, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	parsed, err := backup.Parse(bk, "")
	if err != nil {
		t.Fatal(err)
	}
	var old auth.Account
	if err := json.Unmarshal(parsed["account.json"], &old); err != nil || old.SessionSecret == "" {
		t.Fatalf("备份中的账号文件异常: %v", err)
	}
	if code, _ := doJSON(t, c, "POST", srv.URL+"/api/account/sessions/revoke", nil); code != 200 {
		t.Fatalf("退出所有设备失败 %d", code)
	}
	jar, _ := cookiejar.New(nil)
	c2 := &http.Client{Jar: jar}
	if code, _ := doJSON(t, c2, "POST", srv.URL+"/api/login", map[string]string{"username": "admin", "password": "test-password"}); code != 200 {
		t.Fatal("重新登录失败")
	}
	if code, body := restoreWithFields(t, c2, srv.URL+"/api/system/restore", bk, nil); code != 200 {
		t.Fatalf("恢复失败 %d %s", code, body)
	}
	raw, _ := os.ReadFile(filepath.Join(a.opts.ConfigDir, "account.json"))
	var now auth.Account
	if err := json.Unmarshal(raw, &now); err != nil {
		t.Fatal(err)
	}
	if now.SessionSecret == "" || now.SessionSecret == old.SessionSecret {
		t.Fatal("恢复后应换新的会话密钥")
	}
	if now.SessionEpoch != 0 || len(now.Revoked) != 0 || now.Username != old.Username || now.PasswordHash != old.PasswordHash {
		t.Fatalf("其余字段应按备份恢复，纪元与吊销列表清空: %+v", now)
	}
}
