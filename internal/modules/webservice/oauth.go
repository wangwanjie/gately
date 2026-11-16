package webservice

import (
	"context"
	"crypto/hmac"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/thirdauth"
)

// OAuthProvider 是一个自建的第三方登录配置（使用用户自己申请的 OAuth 应用，不经过任何中转服务器）。
type OAuthProvider = thirdauth.Provider

// 第三方登录相关的保留路径
const (
	oauthStart    = authPath + "/oauth/start"
	oauthCallback = authPath + "/oauth/callback"
	oauthFinish   = authPath + "/oauth/finish"
	// oauthNonceCookie 把登录流程绑定到发起登录的浏览器
	oauthNonceCookie = "gately_oauth_nonce"
	stateTTL         = 10 * time.Minute
	ticketTTL        = 2 * time.Minute
)

// signBlob 生成带过期时间的签名数据，用于 state 与回跳票据。
func (a *authenticator) signBlob(kind string, v any, ttl time.Duration) string {
	return thirdauth.Sign(a.secret, kind, v, ttl)
}

func (a *authenticator) verifyBlob(kind, token string, out any) error {
	return thirdauth.Verify(a.secret, kind, token, out)
}

// verifyBlobExp 与 verifyBlob 相同，另外返回过期时间（Unix 秒）。
func (a *authenticator) verifyBlobExp(kind, token string, out any) (int64, error) {
	return thirdauth.VerifyExp(a.secret, kind, token, out)
}

// oauthState 在跳转身份提供方时携带，回调时用于定位原站点。
type oauthState struct {
	Provider string `json:"p"`
	Origin   string `json:"o"` // 原站点的 scheme://host
	Scope    string `json:"s"`
	Next     string `json:"n"`
	Nonce    string `json:"r"`
	// V 为 state 格式版本：带版本的 state 由校验过访问地址的发起流程签发，回调只接受这类 state
	V int `json:"v,omitempty"`
}

// oauthStateVersion 为当前签发的 state 版本。
const oauthStateVersion = 1

// oauthTicket 由认证域名签发，原站点凭它建立会话。
type oauthTicket struct {
	Identity string `json:"i"`
	// Aliases 为同一账号的其他可匹配标识（如 GitHub 数字 ID）
	Aliases []string `json:"a,omitempty"`
	Host    string   `json:"h"`
	Scope   string   `json:"s"`
	Next    string   `json:"n"`
	Nonce   string   `json:"r"`
}

// oauthEnv 是处理第三方登录需要的全局配置。
type oauthEnv struct {
	providers map[string]OAuthProvider
	// callbackBase 为认证域名的外部地址，例如 https://auth.example.com:4434
	callbackBase string
}

func (e oauthEnv) callbackURL() string {
	return strings.TrimRight(e.callbackBase, "/") + oauthCallback
}

// oauthOrigins 是站点上发起第三方登录时校验访问地址所需的监听信息，构建站点表时设置，不持久化。
type oauthOrigins struct {
	// ports 为请求 Host 中允许出现的端口：监听端口、额外 HTTP 端口与访问端口（经端口映射、四层转发访问时的对外端口）
	ports []string
	// log 为所属监听的运行日志，拒绝发起时在这里说明原因
	log *logbuf.Logger
}

// newOAuthOrigins 按监听配置生成访问地址校验信息。
func newOAuthOrigins(cfg Listener, log *logbuf.Logger) *oauthOrigins {
	o := &oauthOrigins{log: log}
	for _, p := range []int{cfg.Port, cfg.HTTPPort, cfg.AccessPort} {
		if p > 0 {
			o.ports = append(o.ports, strconv.Itoa(p))
		}
	}
	return o
}

// oauthHosts 保存默认站点可接受的访问域名来源，由模块在配置变化时更新。
type oauthHosts struct {
	// sites 为全部监听中填写的站点域名（精确或通配，含停用的站点，不含 *）
	sites atomic.Pointer[[]string]
	// hasCert 判断证书模块中是否有覆盖该域名的证书，为 nil 时不使用
	hasCert atomic.Pointer[func(string) bool]
}

// setSiteHosts 登记全部监听中填写的站点域名，默认站点据此判断访问域名是否由管理员掌握。
func (a *authenticator) setSiteHosts(domains []string) {
	var out []string
	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" || d == "*" {
			continue
		}
		out = append(out, d)
	}
	a.oauthHosts.sites.Store(&out)
}

// setCertLookup 设置判断证书模块中是否有覆盖某域名的证书的函数，nil 表示不使用。
func (a *authenticator) setCertLookup(f func(name string) bool) {
	if f == nil {
		a.oauthHosts.hasCert.Store(nil)
		return
	}
	a.oauthHosts.hasCert.Store(&f)
}

// splitHostPort 把 Host 请求头拆成规范化的主机名与端口（没有端口时为空）；格式错误返回 ok=false。
func splitHostPort(h string) (host, port string, ok bool) {
	host = hostOnly(h)
	if host == "" {
		return "", "", false
	}
	i := strings.LastIndexByte(h, ':')
	if i < 0 || strings.HasSuffix(h, "]") {
		// 没有端口；IPv6 地址必须带方括号
		if strings.Contains(host, ":") && !strings.HasPrefix(h, "[") {
			return "", "", false
		}
		return host, "", true
	}
	hp := h[:i]
	// 不带方括号的 IPv6 地址无法区分端口，按格式错误处理
	if strings.Contains(hp, ":") && !(strings.HasPrefix(hp, "[") && strings.HasSuffix(hp, "]")) {
		return "", "", false
	}
	port = h[i+1:]
	if !validPort(port) {
		return "", "", false
	}
	return host, port, true
}

// oauthOrigin 返回发起第三方登录时浏览器所在的站点来源 scheme://host[:端口]，回调会带着登录票据跳回这里。
// Host 请求头由访客决定，必须确认它指向本实例，否则票据会被送到攻击者的域名或同主机名的其他端口：
//   - 端口：Host 带端口时只接受请求实际到达的端口、监听端口、额外 HTTP 端口与访问端口；
//   - 域名：普通站点必须匹配站点填写的域名（精确或通配，不含 *）；默认站点与填写 * 的站点接收任意域名，
//     只接受管理员掌握的域名：任一监听中填写的站点域名、第三方登录的认证域名、证书模块中有证书的域名，
//     以及连接到达的本机地址。
//
// 不接受时返回的 reason 说明原因，写入运行日志。
func (a *authenticator) oauthOrigin(r *http.Request, s Site, env oauthEnv) (origin, reason string) {
	host, port, ok := splitHostPort(r.Host)
	if !ok {
		return "", "访问地址格式无效"
	}
	if port != "" && port != localPort(r) && (s.oauth == nil || !slices.Contains(s.oauth.ports, port)) {
		return "", fmt.Sprintf("端口 %s 不是本监听的端口或访问端口（经端口映射或转发访问时，请在监听的访问地址中填写对外端口）", port)
	}
	if !a.oauthHostAllowed(r, s, host, env) {
		return "", "默认站点只接受站点中填写的域名、认证域名、有证书的域名或本机地址，请为该域名添加站点"
	}
	scheme := "http"
	if requestIsHTTPS(r) {
		scheme = "https"
	}
	h := host
	if strings.Contains(h, ":") {
		h = "[" + h + "]"
	}
	origin = scheme + "://" + h
	if port != "" {
		origin += ":" + port
	}
	return origin, ""
}

// oauthHostAllowed 判断访问域名能否作为第三方登录的回跳地址，见 oauthOrigin。
func (a *authenticator) oauthHostAllowed(r *http.Request, s Site, host string, env oauthEnv) bool {
	for _, d := range s.Domains {
		if d != "*" && matchDomain(strings.ToLower(d), host) > 0 {
			return true
		}
	}
	if sites := a.oauthHosts.sites.Load(); sites != nil {
		for _, d := range *sites {
			if matchDomain(d, host) > 0 {
				return true
			}
		}
	}
	if u, err := url.Parse(env.callbackBase); err == nil && env.callbackBase != "" && hostOnly(u.Host) == host {
		return true
	}
	if f := a.oauthHosts.hasCert.Load(); f != nil && (*f)(host) {
		return true
	}
	// 连接到达的本机地址（按 IP 访问）：访客无法让它指向别处
	if ip, err := netip.ParseAddr(host); err == nil {
		if la, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
			if ap, err := netip.ParseAddrPort(la.String()); err == nil && ap.Addr().Unmap().WithZone("") == ip.Unmap().WithZone("") {
				return true
			}
		}
	}
	return false
}

// validOAuthOrigin 校验 state 中的站点来源：只能是 http(s)://主机[:端口]，不带账号、路径、查询。
func validOAuthOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil ||
		u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	if p := u.Port(); p != "" && !validPort(p) {
		return false
	}
	// 端口分隔符后为空（host:）时 Port() 为空，单独拒绝
	return !strings.HasSuffix(u.Host, ":")
}

func randomNonce() string {
	return thirdauth.RandomNonce()
}

// identityAllowed 判断身份是否在站点允许列表中，格式见 thirdauth.ParseEntry。
func identityAllowed(s Site, id thirdauth.Identity) bool {
	return thirdauth.Allowed(s.OAuthUsers, id)
}

// handleOAuthStart 在站点域名上发起第三方登录。
func (a *authenticator) handleOAuthStart(w http.ResponseWriter, r *http.Request, s Site, env oauthEnv) {
	pk := r.URL.Query().Get("p")
	p, ok := env.providers[pk]
	if !ok || env.callbackBase == "" {
		http.Error(w, "第三方登录未配置", http.StatusBadRequest)
		return
	}
	origin, reason := a.oauthOrigin(r, s, env)
	if origin == "" {
		if s.oauth != nil && s.oauth.log != nil {
			s.oauth.log.Warnf("拒绝从 %s 发起第三方登录（来源 %s）：%s", r.Host, clientAddr(r).Addr(), reason)
		}
		http.Error(w, "不能从这个访问地址发起第三方登录，请通过站点的域名访问", http.StatusBadRequest)
		return
	}
	nonce := randomNonce()
	state := a.signBlob("state", oauthState{Provider: pk, Origin: origin, Scope: s.scope, Next: safeNext(r.URL.Query().Get("next")), Nonce: nonce, V: oauthStateVersion}, stateTTL)
	target, err := p.AuthorizeURL(r.Context(), env.callbackURL(), state)
	if err != nil {
		http.Error(w, "无法连接身份提供方: "+err.Error(), http.StatusBadGateway)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: oauthNonceCookie, Value: nonce, Path: authPath, MaxAge: int(stateTTL / time.Second),
		HttpOnly: true, Secure: requestIsHTTPS(r), SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, target, http.StatusFound)
}

// handleOAuthCallback 在认证域名上处理身份提供方的回调，然后带票据跳回原站点。
func (a *authenticator) handleOAuthCallback(w http.ResponseWriter, r *http.Request, env oauthEnv) {
	var st oauthState
	rawState := r.URL.Query().Get("state")
	if err := a.verifyBlob("state", rawState, &st); err != nil {
		http.Error(w, "登录状态无效: "+err.Error(), http.StatusBadRequest)
		return
	}
	// 只接受校验过访问地址的发起流程签发的 state，来源格式不对时不跳转（票据只能回到本实例的站点）
	if st.V < oauthStateVersion || !validOAuthOrigin(st.Origin) {
		http.Error(w, "登录状态无效，请重新登录", http.StatusBadRequest)
		return
	}
	if !a.consumeOnce("state|"+rawState, stateTTL) {
		http.Error(w, "登录状态已使用，请重新登录", http.StatusBadRequest)
		return
	}
	p, ok := env.providers[st.Provider]
	if !ok {
		http.Error(w, "第三方登录配置已变化，请重试", http.StatusBadRequest)
		return
	}
	if e := r.URL.Query().Get("error"); e != "" {
		http.Error(w, "身份提供方拒绝登录: "+e, http.StatusForbidden)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), thirdauth.HTTPTimeout)
	defer cancel()
	id, err := p.Exchange(ctx, r.URL.Query().Get("code"), env.callbackURL())
	if err != nil {
		http.Error(w, "登录失败: "+err.Error(), http.StatusBadGateway)
		return
	}
	u, err := url.Parse(st.Origin)
	if err != nil {
		http.Error(w, "原站点地址无效", http.StatusBadRequest)
		return
	}
	ticket := a.signBlob("ticket", oauthTicket{Identity: st.Provider + ":" + id.Primary, Aliases: id.Aliases, Host: hostOnly(u.Host), Scope: st.Scope, Next: st.Next, Nonce: st.Nonce}, ticketTTL)
	http.Redirect(w, r, st.Origin+oauthFinish+"?ticket="+url.QueryEscape(ticket), http.StatusFound)
}

// handleOAuthFinish 在原站点上校验票据与允许列表，建立会话。
func (a *authenticator) handleOAuthFinish(w http.ResponseWriter, r *http.Request, s Site) {
	var t oauthTicket
	raw := r.URL.Query().Get("ticket")
	if err := a.verifyBlob("ticket", raw, &t); err != nil {
		http.Error(w, "登录票据无效: "+err.Error(), http.StatusBadRequest)
		return
	}
	if t.Host != hostOnly(r.Host) || t.Scope != s.scope {
		http.Error(w, "登录票据与站点不匹配", http.StatusBadRequest)
		return
	}
	// 票据必须由发起登录的同一浏览器使用，防止把他人的登录结果注入受害者浏览器
	c, err := r.Cookie(oauthNonceCookie)
	if err != nil || !hmac.Equal([]byte(c.Value), []byte(t.Nonce)) {
		http.Error(w, "登录状态与当前浏览器不匹配，请重新登录", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: oauthNonceCookie, Value: "", Path: authPath, MaxAge: -1})
	if !a.consumeOnce("ticket|"+raw, ticketTTL) {
		http.Error(w, "登录票据已使用", http.StatusBadRequest)
		return
	}
	provider, primary, _ := strings.Cut(t.Identity, ":")
	if !identityAllowed(s, thirdauth.Identity{Provider: provider, Primary: primary, Aliases: t.Aliases}) {
		http.Error(w, fmt.Sprintf("账号 %s 没有访问此站点的权限（如需授权，把它加入站点的第三方身份列表）", t.Identity), http.StatusForbidden)
		return
	}
	a.issueSession(w, r, s, t.Identity, t.Aliases...)
	http.Redirect(w, r, safeNext(t.Next), http.StatusFound)
}
