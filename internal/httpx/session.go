package httpx

import (
	"context"
	"net/http"
	"sync/atomic"
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
