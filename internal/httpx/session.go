package httpx

import (
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
