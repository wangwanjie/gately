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
	"sync"
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
		// 网关转发时 Host 不带端口，而浏览器的 Origin 带端口：只有确认 Origin 就是管理界面所在的页面来源时才以它为准
		// （跨源校验与 Passkey 才能通过）。只比主机名是不够的：同一台 NAS 上其他端口的页面主机名相同，
		// 浏览器会带上会话 Cookie（Cookie 不区分端口），见 gatewayOriginHost
		if host, present, trusted := a.gatewayOriginHost(r, r2.URL.Path); trusted {
			r2.Host = host
		} else if present && originGuarded(r) {
			httpx.Error(w, http.StatusForbidden, "拒绝跨源请求")
			return
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

// ---- 页面来源确认 ----
//
// 经网关访问时 Gately 不知道浏览器地址栏里的端口，无法像直接访问那样拿 Origin 与 Host 逐字比较。
// 这里改为确认“这个 Origin 的页面就是管理界面自己”，依据有两种：
//   - 浏览器写入的 Sec-Fetch-Site: same-origin。这个请求头页面脚本无法伪造，同主机其他端口的页面发来的请求
//     是 same-site，其他站点是 cross-site；
//   - 之前确认过的来源。不带 Sec-Fetch-Site 的请求（部分浏览器的 WebSocket 握手、较旧的浏览器）按此判断。
//     来源在两种情况下记为已确认：带着管理员会话且浏览器标明 same-origin 的请求，以及管理员用密码或 Passkey 登录成功的请求。
//
// 已确认的来源保存在配置目录，重启后仍然有效（已打开的终端页面重连时不必先做一次别的操作）。

// gatewayOriginsName 是保存已确认来源的配置名；maxGatewayOrigins 为保留的数量上限，超出时丢弃最早的。
const (
	gatewayOriginsName = "gateway_origins"
	maxGatewayOrigins  = 16
)

// gatewayOrigins 是经网关访问时已确认的页面来源（scheme://host[:port]，小写）。
type gatewayOrigins struct {
	mu     sync.Mutex
	loaded bool
	list   []string
}

type gatewayOriginsFile struct {
	Origins []string `json:"origins"`
}

// originKey 把 Origin 请求头规范成 scheme://host[:port]（小写）；不是 http/https 来源时返回空串。
func originKey(origin string) (key string, u *url.URL) {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", nil
	}
	return strings.ToLower(u.Scheme + "://" + u.Host), u
}

func (a *App) loadGatewayOriginsLocked() {
	if a.gwOrigins.loaded {
		return
	}
	a.gwOrigins.loaded = true
	var f gatewayOriginsFile
	if _, err := a.store.Load(gatewayOriginsName, &f); err != nil {
		a.sysLogs.Add(logbuf.LevelWarn, "gateway", "读取已确认的网关页面来源失败: "+err.Error())
		return
	}
	for _, o := range f.Origins {
		if key, _ := originKey(o); key != "" && len(a.gwOrigins.list) < maxGatewayOrigins {
			a.gwOrigins.list = append(a.gwOrigins.list, key)
		}
	}
}

// gatewayOriginKnown 判断来源是否已确认。
func (a *App) gatewayOriginKnown(key string) bool {
	a.gwOrigins.mu.Lock()
	defer a.gwOrigins.mu.Unlock()
	a.loadGatewayOriginsLocked()
	for _, o := range a.gwOrigins.list {
		if o == key {
			return true
		}
	}
	return false
}

// learnGatewayOrigin 把来源记为已确认并保存。
func (a *App) learnGatewayOrigin(key string) {
	if key == "" {
		return
	}
	a.gwOrigins.mu.Lock()
	defer a.gwOrigins.mu.Unlock()
	a.loadGatewayOriginsLocked()
	for _, o := range a.gwOrigins.list {
		if o == key {
			return
		}
	}
	a.gwOrigins.list = append(a.gwOrigins.list, key)
	if n := len(a.gwOrigins.list); n > maxGatewayOrigins {
		a.gwOrigins.list = a.gwOrigins.list[n-maxGatewayOrigins:]
	}
	if err := a.store.Save(gatewayOriginsName, gatewayOriginsFile{Origins: a.gwOrigins.list}); err != nil {
		a.sysLogs.Add(logbuf.LevelWarn, "gateway", "保存已确认的网关页面来源失败: "+err.Error())
	}
}

// learnGatewayLoginOrigin 在管理员用密码或 Passkey 登录成功后调用：请求经网关而来时把它的 Origin 记为已确认。
// 能完成登录说明页面拿到了管理员密码或通过了 Passkey 的来源校验，不会是别的端口上的页面。
// 只读账号登录、凭飞牛登录态登录（POST /api/login/fnos，不需要任何凭据）都不能据此确认来源。
func (a *App) learnGatewayLoginOrigin(r *http.Request) {
	if _, viaGateway := gatewayPrefix(r); !viaGateway {
		return
	}
	if key, u := originKey(r.Header.Get("Origin")); key != "" && strings.EqualFold(u.Host, r.Host) {
		a.learnGatewayOrigin(key)
	}
}

// originGuarded 判断请求是否需要确认页面来源：修改状态的请求与 WebSocket 握手。
// 其余 GET 请求即使来自别的页面，浏览器也不会把响应交给它（没有跨源许可）。
func originGuarded(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
	}
	return true
}

// gatewayOriginHost 检查网关转发来的请求的 Origin：present 表示请求带有 Origin，trusted 表示已确认它就是管理界面
// 所在的页面来源，此时 host 为该来源的主机与端口，调用方用它替换不带端口的 Host。path 为去掉网关前缀后的路径。
func (a *App) gatewayOriginHost(r *http.Request, path string) (host string, present, trusted bool) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return "", false, false
	}
	key, u := originKey(origin)
	if key == "" || !strings.EqualFold(hostOnly(u.Host), hostOnly(r.Host)) {
		return "", true, false
	}
	switch strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))) {
	case "same-origin":
		// 带着管理员会话时把来源记下来，之后不带 Sec-Fetch-Site 的请求（如 WebSocket 握手）按它判断。
		// 只认管理员会话：只读账号的持有者可以不经浏览器自己构造请求头，不能让他替别的端口“确认”来源
		if id, ok := a.auth.Identify(auth.TokenFromRequest(r)); ok && id.Role == auth.RoleAdmin {
			a.learnGatewayOrigin(key)
		}
		return u.Host, true, true
	case "same-site", "cross-site":
		// 浏览器已标明请求来自别的来源，包括同主机的其他端口
		return "", true, false
	}
	if a.gatewayOriginKnown(key) {
		return u.Host, true, true
	}
	// 没有 Sec-Fetch-Site 的浏览器首次登录：登录与退出接口本身不依赖已有会话，放行后由登录成功确认来源
	if path == "/api/logout" || path == "/api/login" || strings.HasPrefix(path, "/api/login/") {
		return u.Host, true, true
	}
	return "", true, false
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

// ---- 登录限流的来源 ----

// gatewayLimitPrefix 是经统一网关登录时限流键的前缀，后接飞牛用户名。
const gatewayLimitPrefix = "fnOS:"

// loginSource 返回登录限流与 Passkey 挑战配额使用的来源键，以及客户端地址 ip 是否可信。
//
// 直接访问时来源就是客户端地址。经统一网关访问时客户端地址不可信：浏览器直连网关时它是空的，所有人会落进同一个限流桶，
// 几次错误密码就能把管理员也挡在外面；前面有反向代理时它取自 X-Real-IP，而这个请求头在直连时可以由客户端随意填写，
// 轮换它就能绕过按地址的锁定。网关转发的请求都已通过飞牛的登录校验，并带有飞牛用户名（X-Trim-Username，见 gatewaySSOUser），
// 因此改按飞牛用户限流：一个飞牛用户只能耗尽自己的额度。
// 地址不可信时，登录失败事件也不带地址（见 authFailedFields），入侵防护不会据此封禁一个可以伪造的地址。
func (a *App) loginSource(r *http.Request, ip string) (key string, ipTrusted bool) {
	if _, viaGateway := gatewayPrefix(r); !viaGateway {
		return ip, true
	}
	if user := clipActor(strings.TrimSpace(r.Header.Get("X-Trim-Username"))); user != "" {
		return gatewayLimitPrefix + user, false
	}
	return ip, false
}

// loginSourceKnown 判断登录来源是否不计入“陌生来源”的全局失败预算：曾成功登录过的地址，以及按飞牛用户限流的来源。
// 全局预算防的是轮换大量地址的爆破；飞牛用户不能凭空增加，各自的失败次数上限已经足够。
func (a *App) loginSourceKnown(src string) bool {
	return strings.HasPrefix(src, gatewayLimitPrefix) || a.loginIPs.IsKnown(src)
}

// authFailedFields 返回登录失败事件的字段：客户端地址可信时带上 ip，供入侵防护按地址计数与封禁。
func authFailedFields(ip string, ipTrusted bool) map[string]string {
	if !ipTrusted {
		return map[string]string{}
	}
	return map[string]string{"ip": ip}
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
		Secure: cookieSecure(r),
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
