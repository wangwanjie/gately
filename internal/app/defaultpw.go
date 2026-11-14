package app

import (
	"net/http"
	"strings"

	"github.com/wangwanjie/gately/internal/auth"
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

// passwordFixExempt 是密码存在问题时仍然放行的写操作：修改密码本身、密码强度预检、界面语言、首次引导、退出所有设备。
func passwordFixExempt(method, path string) bool {
	switch method + " " + path {
	case "PUT /api/account", "PUT /api/account/ui-language", "POST /api/account/password-check",
		"POST /api/system/onboarding/dismiss", "POST /api/account/sessions/revoke":
		return true
	}
	return false
}

// defaultPasswordGuard 在管理员密码仍是默认密码或强度不足时限制写操作，迫使先修改密码：
//   - 登录会话：除修改密码等少数操作外，拒绝一切写操作；
//   - API 令牌：保持自动化可用，只拒绝安全相关配置的修改。
//
// 读操作不受影响，页面仍可正常浏览；修改密码后旧会话全部失效，重新登录即恢复。
func (a *App) defaultPasswordGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		issue := a.auth.PasswordIssue()
		if issue == "" || passwordFixExempt(r.Method, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		viaToken := strings.HasPrefix(auth.UserFromContext(r.Context()), "api-token:")
		switch {
		case !viaToken:
			if issue == "default" {
				httpx.Error(w, http.StatusForbidden, "当前仍在使用默认密码，请先修改管理员密码")
			} else {
				httpx.Error(w, http.StatusForbidden, "当前管理员密码强度不足，请先修改管理员密码")
			}
			return
		case isSecurityConfigWrite(r.Method, r.URL.Path):
			if issue == "default" {
				httpx.Error(w, http.StatusForbidden, "当前仍在使用默认密码，请先修改管理员密码，再修改安全相关配置")
			} else {
				httpx.Error(w, http.StatusForbidden, "当前管理员密码强度不足，请先修改管理员密码，再修改安全相关配置")
			}
			return
		}
		next.ServeHTTP(w, r)
	})
}
