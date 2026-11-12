package app

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"testing"

	"github.com/wangwanjie/gately/internal/auth"
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
