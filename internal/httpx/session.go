package httpx

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"
)

// 管理后台常经本机的 Web 服务站点反向代理访问，站点访问日志会记下管理员自己的请求，
// 打开日志页后每 2 秒一次的轮询更是“看日志产生日志”。管理后台注册登录态校验函数，
// Web 服务据此给带有效后台登录态的请求打上标记，日志查看器可以隐藏这些请求。

// sessionUser 为当前生效的校验函数，未注册时为 nil。
var sessionUser atomic.Pointer[func(*http.Request) string]

// SetSessionCheck 注册后台登录态校验函数：请求带有本实例有效的后台登录态时返回用户名，否则返回空串。
// 传入 nil 取消注册。
func SetSessionCheck(f func(*http.Request) string) {
	if f == nil {
		sessionUser.Store(nil)
		return
	}
	sessionUser.Store(&f)
}

// SessionUser 返回请求所带后台登录态的用户名；没有登录态、登录态无效或未注册校验函数时返回空串。
// 登录态由服务端密钥签名，访客无法伪造，可放心用作日志标记。
func SessionUser(r *http.Request) string {
	f := sessionUser.Load()
	if f == nil {
		return ""
	}
	return (*f)(r)
}

// wafFlagKey 为请求上下文中“已被 WAF 拦截”标记的键。
type wafFlagKey struct{}

// WithWAFFlag 在请求上下文中放入 WAF 拦截标记，WAF 拦截时由 MarkWAFBlocked 置位，
// Web 服务据此在访问日志与统计中记录 deny=waf，不再按状态码推断（雷池可返回 403 以外的状态码，后端自己的 403 也不算）。
func WithWAFFlag(r *http.Request) (*http.Request, *atomic.Bool) {
	flag := &atomic.Bool{}
	return r.WithContext(context.WithValue(r.Context(), wafFlagKey{}, flag)), flag
}

// MarkWAFBlocked 标记请求已被 WAF 拦截；请求上下文中没有标记时忽略。
func MarkWAFBlocked(r *http.Request) {
	if flag, ok := r.Context().Value(wafFlagKey{}).(*atomic.Bool); ok {
		flag.Store(true)
	}
}

// 后台登录桥接：管理员登录后台后，Web 服务经由这里向单点登录域签发“后台已登录”凭据，
// 其他站点开启“后台登录免认证”时凭它放行。凭据由管理后台签发与校验（见 auth 包），Web 服务只负责携带。

// bridgeHooks 为桥接凭据的签发与校验函数。
type bridgeHooks struct {
	issue  func(r *http.Request) (string, time.Time)
	verify func(value string) string
}

var bridge atomic.Pointer[bridgeHooks]

// SetAdminBridge 注册桥接凭据的签发与校验函数：issue 根据请求所带的后台登录态返回凭据与过期时间（没有有效的管理员登录态时返回空串），
// verify 校验凭据并返回管理员用户名（无效时返回空串）。传入 nil 取消注册。
func SetAdminBridge(issue func(r *http.Request) (string, time.Time), verify func(value string) string) {
	if issue == nil || verify == nil {
		bridge.Store(nil)
		return
	}
	bridge.Store(&bridgeHooks{issue: issue, verify: verify})
}

// IssueAdminBridge 为带有效管理员后台登录态的请求签发桥接凭据；没有登录态或未注册时返回空串。
func IssueAdminBridge(r *http.Request) (string, time.Time) {
	h := bridge.Load()
	if h == nil {
		return "", time.Time{}
	}
	return h.issue(r)
}

// VerifyAdminBridge 校验桥接凭据，有效时返回管理员用户名，否则返回空串。
func VerifyAdminBridge(value string) string {
	h := bridge.Load()
	if h == nil {
		return ""
	}
	return h.verify(value)
}

// 本机 Web 服务站点反代到另一台 Gately（例如在线演示实例，或外层 Gately 经 frp 回到内网 Gately 的双层部署）时，
// 访客浏览器带的同名后台 Cookie（会话、安全入口标记、第三方登录随机数）可能是后端签发的，对本实例没有意义；
// 只有本实例签发的才是需要防泄露的凭据。管理后台注册归属判定函数，Web 服务据此只剥离本实例的后台 Cookie。

// ownAdminCookie 为后台 Cookie 归属判定函数，未注册时为 nil。
var ownAdminCookie atomic.Pointer[func(name, value string) bool]

// SetOwnAdminCookieCheck 注册后台 Cookie 归属判定函数：名为 name、值为 value 的 Cookie 是本实例签发且仍有效时返回 true。
// 传入 nil 取消注册。
func SetOwnAdminCookieCheck(f func(name, value string) bool) {
	if f == nil {
		ownAdminCookie.Store(nil)
		return
	}
	ownAdminCookie.Store(&f)
}

// OwnAdminCookie 判断后台 Cookie 是否为本实例签发。
// 未注册判定函数时一律视为本实例的，按凭据处理（调用方据此剥离，宁可多剥）。
func OwnAdminCookie(name, value string) bool {
	f := ownAdminCookie.Load()
	if f == nil {
		return true
	}
	return (*f)(name, value)
}

// OwnAdminBridge 判断后台登录桥接凭据是否为本实例签发且有效。未注册桥接函数时视为本实例的（宁可多剥）。
// 外层 Gately 反代到内层 Gately 时，内层签发的同名凭据本实例校验不过，应原样转发，内层的后台登录免认证才能生效。
func OwnAdminBridge(value string) bool {
	h := bridge.Load()
	if h == nil {
		return true
	}
	return h.verify(value) != ""
}
