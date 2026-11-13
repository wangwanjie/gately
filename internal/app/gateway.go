package app

// 统一网关接入：在 Unix Socket 上额外提供带路径前缀的管理界面，与 TCP 监听并存。
// 飞牛 fnOS 的统一网关把 https://<NAS 域名>/app/gately/... 转发到应用目录下的 Socket，
// 浏览器因此可以复用 NAS 自身的域名与端口访问管理界面。
// 仅在启动参数指定 Socket 时启用（fpk 包的 cmd/main 传入），其他部署方式的行为不变。

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wangwanjie/gately/internal/audit"
	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// gatewayKey 是请求上下文中标记“经统一网关访问”的键，值为路径前缀（可为空）。
type gatewayKey struct{}

// gatewayPrefix 返回请求经统一网关访问时的路径前缀；ok 为 false 表示直接访问 TCP 监听。
func gatewayPrefix(r *http.Request) (prefix string, ok bool) {
	prefix, ok = r.Context().Value(gatewayKey{}).(string)
	return prefix, ok
}

// normalizeGatewayPrefix 校验并规范路径前缀：必须以 / 开头，去掉末尾的 /；空字符串表示不加前缀。
func normalizeGatewayPrefix(p string) (string, error) {
	p = strings.TrimRight(strings.TrimSpace(p), "/")
	if p == "" {
		return "", nil
	}
	if !strings.HasPrefix(p, "/") || strings.ContainsAny(p, "?#\"'<> ") {
		return "", fmt.Errorf("统一网关路径前缀无效: %q", p)
	}
	return p, nil
}

// listenUnix 监听 Unix Socket。上次异常退出遗留的 Socket 文件先删除；其他类型的同名文件不覆盖。
func listenUnix(path string) (net.Listener, error) {
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&fs.ModeSocket == 0 {
			return nil, fmt.Errorf("%s 已存在且不是 Socket 文件", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	// 只允许属主与同组访问（fnOS 的网关进程以 root 运行）
	if err := os.Chmod(path, 0o660); err != nil {
		_ = ln.Close()
		return nil, err
	}
	return ln, nil
}

// gatewayHandler 把统一网关转发来的请求还原成直接访问时的形态，再交给 next：
// 去掉路径前缀、按转发头还原客户端地址、修正跨源校验用的 Host，并在响应中给重定向地址与 Cookie 路径补上前缀。
func (a *App) gatewayHandler(prefix string, next http.Handler) http.Handler {
	var logged atomic.Bool
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 首个请求记录网关传来的关键请求头，便于排查地址与路径问题（不含 Cookie 等敏感信息）
		if logged.CompareAndSwap(false, true) {
			a.sysLogs.Add(logbuf.LevelInfo, "gateway", fmt.Sprintf("统一网关首个请求：%s %s，Host=%s，Origin=%q，X-Real-IP=%q，X-Forwarded-For=%q，X-Forwarded-Proto=%q",
				r.Method, r.URL.Path, r.Host, r.Header.Get("Origin"), r.Header.Get("X-Real-IP"), r.Header.Get("X-Forwarded-For"), r.Header.Get("X-Forwarded-Proto")))
		}
		if prefix != "" && r.URL.Path == prefix {
			// 补上末尾斜杠，前端的相对资源路径才能正确解析
			target := prefix + "/"
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusMovedPermanently)
			return
		}

		r2 := new(http.Request)
		*r2 = *r
		u := *r.URL
		r2.URL = &u
		r2.Header = r.Header.Clone()
		// 网关可能保留前缀，也可能已经去掉；两种情况都能处理
		if prefix != "" {
			if rest, ok := strings.CutPrefix(u.Path, prefix+"/"); ok {
				u.Path = "/" + rest
				if u.RawPath != "" {
					u.RawPath = "/" + strings.TrimPrefix(u.RawPath, prefix+"/")
				}
				r2.RequestURI = u.RequestURI()
			}
		}
		// Socket 只有网关进程能连接，对端地址没有意义：改用网关转发头中的客户端地址，并删除转发头，
		// 避免后续按可信代理规则再次解析
		r2.RemoteAddr = gatewayClientAddr(r)
		r2.Header.Del("X-Real-IP")
		r2.Header.Del("X-Forwarded-For")
		// 网关转发时 Host 不带端口，而浏览器的 Origin 带端口；主机名一致时以 Origin 为准，跨源校验与 Passkey 才能通过
		if origin := r.Header.Get("Origin"); origin != "" {
			if o, err := url.Parse(origin); err == nil && o.Host != "" && strings.EqualFold(hostOnly(o.Host), hostOnly(r.Host)) {
				r2.Host = o.Host
			}
		}
		r2 = r2.WithContext(context.WithValue(r.Context(), gatewayKey{}, prefix))
		// 审计日志标注经由飞牛网关，提示客户端地址未经核实
		r2 = httpx.WithVia(r2, "fnOS")
		out := w
		if prefix != "" {
			out = &gatewayWriter{ResponseWriter: w, prefix: prefix}
		}
		// 打开管理界面首页（点击飞牛桌面图标）时，按飞牛登录态自动登录
		if r2.Method == http.MethodGet && r2.URL.Path == "/" {
			a.gatewayAutoLogin(out, r2)
		}
		next.ServeHTTP(out, r2)
	})
}

// gatewayClientAddr 依次从 X-Real-IP、X-Forwarded-For 最右侧取得客户端地址，返回 host:port 形式；取不到时为空。
// 实测 fnOS 网关不会改写 X-Real-IP：前面有反向代理时是代理写入的真实地址，浏览器直连时为空，也可能被客户端伪造；
// X-Forwarded-For 固定为 127.0.0.1（网关自身）。这里的地址只用于日志、审计与登录限流（审计中标注“经代理 fnOS”），
// 不用于访问控制；回环与未指定地址一律忽略。
func gatewayClientAddr(r *http.Request) string {
	candidates := []string{strings.TrimSpace(r.Header.Get("X-Real-IP"))}
	if xff := r.Header.Values("X-Forwarded-For"); len(xff) > 0 {
		parts := strings.Split(xff[len(xff)-1], ",")
		candidates = append(candidates, strings.TrimSpace(parts[len(parts)-1]))
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		ip, err := netip.ParseAddr(strings.Trim(c, "[]"))
		if err != nil {
			continue
		}
		ip = ip.Unmap()
		if ip.IsLoopback() || ip.IsUnspecified() {
			continue
		}
		return net.JoinHostPort(ip.String(), "0")
	}
	return ""
}

// gatewayWriter 在响应头发出前，给站内重定向地址与 Cookie 路径补上网关前缀。
type gatewayWriter struct {
	http.ResponseWriter
	prefix string
	fixed  bool
}

func (g *gatewayWriter) fixHeader() {
	if g.fixed {
		return
	}
	g.fixed = true
	h := g.Header()
	// 只处理以 / 开头的站内绝对路径；完整 URL、协议相对地址与相对路径保持不变
	if loc := h.Get("Location"); strings.HasPrefix(loc, "/") && !strings.HasPrefix(loc, "//") {
		h.Set("Location", g.prefix+loc)
	}
	if cookies := h.Values("Set-Cookie"); len(cookies) > 0 {
		out := make([]string, len(cookies))
		for i, c := range cookies {
			out[i] = prefixCookiePath(c, g.prefix)
		}
		h["Set-Cookie"] = out
	}
}

func (g *gatewayWriter) WriteHeader(code int) {
	g.fixHeader()
	g.ResponseWriter.WriteHeader(code)
}

func (g *gatewayWriter) Write(b []byte) (int, error) {
	g.fixHeader()
	return g.ResponseWriter.Write(b)
}

// Flush 支持流式输出（日志、终端等）。
func (g *gatewayWriter) Flush() {
	g.fixHeader()
	_ = http.NewResponseController(g.ResponseWriter).Flush()
}

// Hijack 让 WebSocket 升级能接管底层连接。外层中间件中有的只对内层做 http.Hijacker 类型断言，
// 不会沿 Unwrap 查找，这里显式实现，避免写出 101 后接管失败（issue #31）。
func (g *gatewayWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(g.ResponseWriter).Hijack()
}

// Unwrap 供 http.ResponseController 与 WebSocket 库取得底层连接（Hijack）。
func (g *gatewayWriter) Unwrap() http.ResponseWriter {
	return g.ResponseWriter
}

// prefixCookiePath 给 Set-Cookie 中的 Path 属性补上前缀：Path=/ 变为 Path=<前缀>，Path=/x 变为 Path=<前缀>/x。
func prefixCookiePath(cookie, prefix string) string {
	attrs := strings.Split(cookie, ";")
	for i, attr := range attrs {
		// 第一段是 Cookie 自身的名称与值，不是属性
		if i == 0 {
			continue
		}
		name, value, found := strings.Cut(strings.TrimSpace(attr), "=")
		if !found || !strings.EqualFold(name, "Path") || !strings.HasPrefix(value, "/") {
			continue
		}
		if value == "/" {
			value = prefix
		} else {
			value = prefix + value
		}
		attrs[i] = " Path=" + value
	}
	return strings.Join(attrs, ";")
}

// absolutizeAssets 把 index.html 中 Vite 生成的相对资源路径（="./...）改写为以前缀开头的绝对路径。
// 前缀为空时得到 ="/..."，与直接访问时以往的页面完全一致。
func absolutizeAssets(index []byte, prefix string) []byte {
	return []byte(strings.ReplaceAll(string(index), `="./`, `="`+prefix+`/`))
}

// injectBase 在 index.html 的 <head> 后插入网关前缀，前端据此给接口请求补前缀。
func injectBase(index []byte, prefix string) []byte {
	meta := `<head>` + "\n    " + `<meta name="gately-base" content="` + html.EscapeString(prefix) + `" />`
	return []byte(strings.Replace(string(index), "<head>", meta, 1))
}

// ---- 飞牛登录态自动登录 ----

// gatewaySSOUser 返回可凭飞牛登录态登录的飞牛用户名：请求须经统一网关、已开启该功能，且网关标明是飞牛管理员。
// X-Trim-* 请求头由 fnOS 网关在校验登录态后写入，只信任经网关 Socket 进入的请求，TCP 监听上的同名请求头一律忽略。
func (a *App) gatewaySSOUser(r *http.Request) (string, bool) {
	if _, viaGateway := gatewayPrefix(r); !viaGateway || !a.settingsSnapshot().GatewaySSO {
		return "", false
	}
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Trim-Isadmin")), "true") {
		return "", false
	}
	user := strings.TrimSpace(r.Header.Get("X-Trim-Username"))
	return user, user != ""
}

// gatewayAutoLogin 在没有有效会话时，为飞牛管理员签发 Gately 管理员会话。已有会话（含以只读账号登录）时保持不变。
func (a *App) gatewayAutoLogin(w http.ResponseWriter, r *http.Request) {
	user, ok := a.gatewaySSOUser(r)
	if !ok {
		return
	}
	if _, valid := a.auth.Identify(auth.TokenFromRequest(r)); valid {
		return
	}
	a.issueGatewaySession(w, r, user)
}

// handleGatewayLogin 供登录页“使用飞牛账号登录”按钮调用（退出登录或会话过期后）。
func (a *App) handleGatewayLogin(w http.ResponseWriter, r *http.Request) {
	user, ok := a.gatewaySSOUser(r)
	if !ok {
		httpx.Error(w, http.StatusForbidden, "未开启飞牛登录态自动登录，或当前飞牛账号不是管理员")
		return
	}
	exp := a.issueGatewaySession(w, r, user)
	httpx.OK(w, map[string]any{"username": a.auth.Username(), "expires": exp})
}

// issueGatewaySession 签发管理员会话 Cookie，并记录日志、审计与登录通知。
func (a *App) issueGatewaySession(w http.ResponseWriter, r *http.Request, user string) time.Time {
	token, exp := a.auth.Issue()
	http.SetCookie(w, &http.Cookie{
		Name: auth.CookieName, Value: token, Path: "/", Expires: exp, HttpOnly: true, SameSite: http.SameSiteStrictMode,
		// 网关以 HTTPS 对外提供时，Cookie 只经 HTTPS 发送
		Secure: r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
	})
	ip := httpx.ClientIP(r)
	// 整页跳转带的是浏览器自身的语言，不代表界面语言，不在这里记录；进入管理界面后由前端上报
	a.sysLogs.Add(logbuf.LevelInfo, "auth", fmt.Sprintf("飞牛用户 %s 从 %s 经统一网关自动登录", user, ip))
	a.audit.Record(audit.Entry{Actor: "fnOS:" + user, IP: ip, Via: audit.Via(r), Category: audit.CategoryLogin, Method: r.Method, Path: r.URL.Path, Status: http.StatusOK, Detail: "飞牛登录态自动登录"})
	events.Publish(events.Event{Type: events.LoginSuccessful, Source: "管理后台", Level: events.LevelInfo,
		Title: "管理后台登录成功", Message: fmt.Sprintf("飞牛用户 %s 从 %s 经统一网关自动登录", user, ip), Fields: map[string]string{"ip": ip, "user": "fnOS:" + user}})
	a.noteLoginIP(ip, "fnOS:"+user, "统一网关")
	return exp
}
