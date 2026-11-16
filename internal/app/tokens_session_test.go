package app

import (
	"strings"
	"testing"
)

// 托管程序的路径与备份加密口令只允许登录会话修改或查看，拥有全部权限的令牌也不行。
func TestTokenSessionOnlySettings(t *testing.T) {
	_, srv, c := fullApp(t, t.TempDir(), nil)
	_, tok := createToken(t, srv.URL, c, map[string]any{"name": "all", "scopes": []string{"*"}})

	for _, mod := range []string{"easytier", "tailscale", "netbird"} {
		path := "/api/" + mod + "/settings"
		body := map[string]any{"corePath": "/bin/sh", "tailscaledPath": "/bin/sh", "tailscalePath": "/bin/sh", "netbirdPath": "/bin/sh"}
		if code, out := tokenRequest(t, srv.URL, "PUT", path, tok, body); code != 403 {
			t.Errorf("令牌修改 %s 的程序路径应 403: %d %s", mod, code, out)
		}
		if code, out := tokenRequest(t, srv.URL, "GET", path, tok, nil); code != 200 || strings.Contains(out, "/bin/sh") {
			t.Errorf("令牌应仍能读取 %s 的设置，且路径未被改动: %d %s", mod, code, out)
		}
		if code, out := doJSON(t, c, "PUT", srv.URL+path, map[string]any{}); code != 200 {
			t.Errorf("登录会话应能保存 %s 的设置: %d %s", mod, code, out)
		}
	}

	// 备份加密口令：会话可以设置和查看
	set := map[string]any{"enable": false, "schedule": "0 3 * * *", "keep": 7, "password": "backup-pass-1", "localEnable": true}
	if code, out := doJSON(t, c, "PUT", srv.URL+"/api/backup/settings", set); code != 200 {
		t.Fatalf("会话设置备份口令失败: %d %s", code, out)
	}
	if code, out := doJSON(t, c, "GET", srv.URL+"/api/backup/secrets", nil); code != 200 || !strings.Contains(string(out), "backup-pass-1") {
		t.Fatalf("会话应能查看备份口令: %d %s", code, out)
	}
	// 令牌：不能查看，不能修改或清空；口令保持占位符时仍可修改其余设置
	if code, out := tokenRequest(t, srv.URL, "GET", "/api/backup/secrets", tok, nil); code != 403 || strings.Contains(out, "backup-pass-1") {
		t.Fatalf("令牌查看备份口令应 403: %d %s", code, out)
	}
	for _, pw := range []string{"attacker-pass", ""} {
		set["password"] = pw
		if code, out := tokenRequest(t, srv.URL, "PUT", "/api/backup/settings", tok, set); code != 403 {
			t.Fatalf("令牌把备份口令改为 %q 应 403: %d %s", pw, code, out)
		}
	}
	set["password"], set["keep"] = "********", 9
	if code, out := tokenRequest(t, srv.URL, "PUT", "/api/backup/settings", tok, set); code != 200 || !strings.Contains(out, `"keep":9`) {
		t.Fatalf("口令不变时令牌应能修改其余设置: %d %s", code, out)
	}
	if code, out := doJSON(t, c, "GET", srv.URL+"/api/backup/secrets", nil); code != 200 || !strings.Contains(string(out), "backup-pass-1") {
		t.Fatalf("令牌的请求不应改动备份口令: %d %s", code, out)
	}
}
