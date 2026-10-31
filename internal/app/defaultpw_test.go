package app

import (
	"net/http"
	"testing"
)

// 仍使用默认密码时不能修改安全配置，也不能把密码改成默认密码；改掉默认密码后恢复正常。
func TestDefaultPasswordLocksSecurityConfig(t *testing.T) {
	srv, c := newTestServer(t)
	if code, b := doJSON(t, c, "POST", srv.URL+"/api/login", map[string]string{"username": "admin", "password": "gately.cn"}); code != 200 {
		t.Fatalf("默认密码登录失败 %d %s", code, b)
	}
	if code, _ := doJSON(t, c, "POST", srv.URL+"/api/tokens", map[string]any{"name": "x", "scopes": []string{"status:read"}}); code != http.StatusForbidden {
		t.Fatalf("默认密码下创建令牌应被拒绝，实际 %d", code)
	}
	if code, _ := doJSON(t, c, "PUT", srv.URL+"/api/system/settings", map[string]any{}); code != http.StatusForbidden {
		t.Fatalf("默认密码下修改系统设置应被拒绝，实际 %d", code)
	}
	// 读取与改密本身不受限
	if code, _ := doJSON(t, c, "GET", srv.URL+"/api/tokens", nil); code != 200 {
		t.Fatalf("读取令牌列表不应受限: %d", code)
	}
	if code, _ := doJSON(t, c, "PUT", srv.URL+"/api/account", map[string]string{"oldPassword": "gately.cn", "username": "admin", "newPassword": "gately.cn"}); code != http.StatusBadRequest {
		t.Fatalf("不能把密码设为默认密码，实际 %d", code)
	}
	if code, b := doJSON(t, c, "PUT", srv.URL+"/api/account", map[string]string{"oldPassword": "gately.cn", "username": "admin", "newPassword": "a-new-password"}); code != 200 {
		t.Fatalf("修改密码失败 %d %s", code, b)
	}
	if code, b := doJSON(t, c, "POST", srv.URL+"/api/login", map[string]string{"username": "admin", "password": "a-new-password"}); code != 200 {
		t.Fatalf("新密码登录失败 %d %s", code, b)
	}
	if code, b := doJSON(t, c, "POST", srv.URL+"/api/tokens", map[string]any{"name": "x", "scopes": []string{"status:read"}}); code != 200 {
		t.Fatalf("改密后应能创建令牌 %d %s", code, b)
	}
	// 已改过密码后不能再改回默认密码
	if code, _ := doJSON(t, c, "PUT", srv.URL+"/api/account", map[string]string{"oldPassword": "a-new-password", "username": "admin", "newPassword": "gately.cn"}); code != http.StatusBadRequest {
		t.Fatalf("改密后不能改回默认密码，实际 %d", code)
	}
}
