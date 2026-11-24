package webservice

import (
	"net/http"
	"net/url"
	"strings"
)

// fixRedirectScheme 把后端跳转里指向访客同一地址的 http:// 改成 https://。
// 访客以 HTTPS 访问而后端只收到 HTTP 请求时，不读取 X-Forwarded-Proto 的后端（如未开启转发头处理的 Spring Boot/Tomcat）
// 会按自己收到的协议把相对跳转拼成 http://<访问域名>/... 的绝对地址，浏览器随之降级到 HTTP（OAuth 登录回调后常见）。
// 只改写主机与端口都与访客请求一致的地址，指向其他站点或其他端口的跳转原样保留。
func fixRedirectScheme(h http.Header, in *http.Request) {
	loc := h.Get("Location")
	if len(loc) < 8 || !strings.EqualFold(loc[:7], "http://") || in == nil || !requestIsHTTPS(in) {
		return
	}
	u, err := url.Parse(loc)
	if err != nil || u.Host == "" || u.User != nil {
		return
	}
	if !sameVisitorHost(u, in.Host) {
		return
	}
	u.Scheme = "https"
	if u.Port() == "80" {
		// 后端按 HTTP 默认端口拼出的 :80 换成 HTTPS 后应去掉
		u.Host = strings.TrimSuffix(u.Host, ":80")
	}
	h.Set("Location", u.String())
}

// sameVisitorHost 判断跳转地址 u（http）是否指向访客请求的 Host（https）：主机名相同，
// 端口相同，或跳转地址不带端口/带 80 而访客使用 HTTPS 默认端口。
func sameVisitorHost(u *url.URL, reqHost string) bool {
	if !strings.EqualFold(strings.TrimSuffix(u.Hostname(), "."), hostOnly(reqHost)) {
		return false
	}
	reqPort := ""
	if i := strings.LastIndexByte(reqHost, ':'); i > 0 && !strings.HasSuffix(reqHost, "]") {
		reqPort = reqHost[i+1:]
	}
	if reqPort == "443" {
		reqPort = ""
	}
	locPort := u.Port()
	if locPort == "80" {
		locPort = ""
	}
	return locPort == reqPort
}
