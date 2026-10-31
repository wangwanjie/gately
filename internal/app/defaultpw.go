package app

import (
	"net/http"
	"strings"

	"github.com/wangwanjie/gately/internal/httpx"
)

// securityConfigPrefixes 是安全相关配置的路径前缀：账号与两步验证、通行密钥、附加账号、API 令牌、
// 访问控制等系统设置、Web 终端的安全设置。
var securityConfigPrefixes = []string{
	"/api/account",
	"/api/tokens",
	"/api/system/settings",
	"/api/system/token",
	"/api/webterminal/security",
	// 从备份恢复会整体覆盖账号、令牌与系统设置
	"/api/system/restore",
}

// securityHistoryNames 是回滚历史版本时属于安全配置的配置名：系统设置与 Web 终端（含安全设置）。
var securityHistoryNames = map[string]bool{settingsName: true, "webterminal": true}

// isSecurityConfigWrite 判断请求是否修改安全相关配置。修改管理员密码本身与界面语言除外，否则无法摆脱默认密码。
func isSecurityConfigWrite(method, path string) bool {
	if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
		return false
	}
	if method == http.MethodPut && (path == "/api/account" || path == "/api/account/ui-language") {
		return false
	}
	// /api/history/<配置名>/<版本>/restore 回滚安全配置等同于直接修改
	if rest, ok := strings.CutPrefix(path, "/api/history/"); ok && strings.HasSuffix(rest, "/restore") {
		name, _, _ := strings.Cut(rest, "/")
		if securityHistoryNames[name] {
			return true
		}
	}
	for _, p := range securityConfigPrefixes {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

// defaultPasswordGuard 在管理员仍使用默认密码时拒绝修改安全相关配置，要求先改掉默认密码。
func (a *App) defaultPasswordGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isSecurityConfigWrite(r.Method, r.URL.Path) && a.auth.UsingDefaultPassword() {
			httpx.Error(w, http.StatusForbidden, "当前仍在使用默认密码，请先修改管理员密码，再修改安全相关配置")
			return
		}
		next.ServeHTTP(w, r)
	})
}
