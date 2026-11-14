package webservice

import (
	"net/http"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/httpx"
)

// 后台登录免认证（Site.AdminPass）：管理员登录 Gately 后台后，同一单点登录域下开启该选项的站点不再要求基本认证或网页认证。
//
// 管理后台的登录 Cookie 只发给后台所在的域名，其他站点收不到。管理员经本机某个站点访问后台时（带后台登录 Cookie 的请求），
// 站点向单点登录域写一个桥接凭据（见 auth.IssueBridge）：凭据绑定那一个后台会话，登出、退出所有设备、修改密码、
// 会话过期后立即失效，只读账号不会产生凭据。凭据只在站点认证处使用，不转发给后端应用，不能调用管理接口。

// adminBridgeCookie 是桥接凭据的 Cookie 名称。
const adminBridgeCookie = "gately_admin"

// adminVisitorPrefix 是后台登录放行的访客身份前缀：避免与同名的站点用户、共享用户混淆，按用户分流与身份头据此区分。
const adminVisitorPrefix = "gately:"

// maxBridgeCookies 限制校验同名 Cookie 的个数：兄弟子域名上的应用可以往父域写同名的无效 Cookie 排在前面，
// 所以逐个校验、任一有效即放行；个数设上限，防止请求头里塞满同名 Cookie 放大计算。
const maxBridgeCookies = 4

// issueAdminBridge 在请求带有管理员后台登录态时，向单点登录域写桥接凭据。
// 没有任何站点开启后台登录免认证、域名不在单点登录域下、请求没有后台 Cookie、已带有效的桥接凭据时都不做任何事，
// 常规请求只多一次原子读取与 Cookie 查找。
func (a *authenticator) issueAdminBridge(w http.ResponseWriter, r *http.Request) {
	if !a.adminPass.Load() {
		return
	}
	if _, err := r.Cookie(auth.CookieName); err != nil {
		return
	}
	host := hostOnly(r.Host)
	domain := a.shared.get().ssoFor(host)
	if domain == "" {
		return
	}
	if a.bridgeUser(r) != "" {
		return
	}
	value, exp := httpx.IssueAdminBridge(r)
	if value == "" {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: adminBridgeCookie, Value: value, Domain: domain, Path: "/", Expires: exp,
		HttpOnly: true, Secure: requestIsHTTPS(r), SameSite: http.SameSiteLaxMode,
	})
}

// bridgeUser 返回请求所带桥接凭据对应的管理员用户名，没有有效凭据时返回空串。
func (a *authenticator) bridgeUser(r *http.Request) string {
	n := 0
	for _, c := range r.Cookies() {
		if c.Name != adminBridgeCookie {
			continue
		}
		if n++; n > maxBridgeCookies {
			break
		}
		if user := httpx.VerifyAdminBridge(c.Value); user != "" {
			return user
		}
	}
	return ""
}

// adminPassVisitor 判断请求是否来自已登录后台的管理员，且站点开启了后台登录免认证；是则返回对应的访客身份。
// 凭据来自两条路径：单点登录域下的站点写在父域上；其他域名的站点经跳转握手写在自己的域名上（见 adminhandshake.go，
// 需配置后台入口）。两者都没有启用时，域外站点不认凭据，避免把凭据手工带到域外的站点上也放行。
func (a *authenticator) adminPassVisitor(r *http.Request, s Site) (visitor, bool) {
	if !s.AdminPass || (a.shared.get().ssoFor(hostOnly(r.Host)) == "" && a.hs.entry.Load() == nil) {
		return visitor{}, false
	}
	user := a.bridgeUser(r)
	if user == "" {
		return visitor{}, false
	}
	return visitor{names: []string{adminVisitorPrefix + user}}, true
}

// adminPassDomains 返回开启后台登录免认证的站点的域名（只统计启用的监听器与站点）。
func adminPassDomains(listeners []Listener) []string {
	var out []string
	for _, l := range listeners {
		if !l.Enable {
			continue
		}
		for _, s := range l.Sites {
			if s.Enable && s.AdminPass {
				out = append(out, s.Domains...)
			}
		}
	}
	return out
}

// anyAdminPass 判断配置中是否有站点开启了后台登录免认证（只统计启用的监听器与站点）。
func anyAdminPass(listeners []Listener) bool {
	for _, l := range listeners {
		if !l.Enable {
			continue
		}
		for _, s := range l.Sites {
			if s.Enable && s.AdminPass {
				return true
			}
		}
	}
	return false
}
