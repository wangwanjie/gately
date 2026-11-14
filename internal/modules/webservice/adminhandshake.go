package webservice

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wangwanjie/gately/internal/httpx"
)

// 后台登录免认证的跨父域握手（Settings.AdminPassEntry）。
//
// 单点登录域只能覆盖一个父域：Cookie 不能跨注册域共享。站点与后台不在同一父域时（例如后台在 vanjay.cn，
// 站点在 thevast.uk），浏览器访问站点时改由跳转握手取得凭据，流程与第三方登录相同：
//
//  1. 站点（开启后台登录免认证、域名为精确域名）发现浏览器没有凭据，签发带签名的 state（站点来源、原路径、随机数的哈希），
//     把随机数写进站点自己的 Cookie，跳转到后台入口域名上的 admin-ticket。
//  2. 入口域名（只响应 AdminPassEntry 中的域名）校验 state 与来源域名，若请求带有管理员后台登录态，
//     在内存中登记一张一次性票据（128 位随机编号，只含编号，凭据本体不出现在网址里），跳回站点的 admin-login。
//     SameSite=Strict 的后台 Cookie 不会随跨站跳转发送，所以第一跳先返回一个同站的页面再自跳转一次。
//  3. 站点校验票据：编号存在且未过期、来源与本站点一致、随机数与发起握手的同一浏览器一致，通过后才写下
//     只属于本站点域名的凭据 Cookie，并跳回原路径。
//
// 安全约束：票据一次性（取用即删）、60 秒有效、绑定来源与浏览器；网址里没有任何可重放的凭据；
// 跳转目标只能是配置中开启该选项的站点的精确域名，通配域名与默认站点不参与，避免攻击者借未受控的子域名收到票据；
// 没有后台登录态时不登记票据；握手只对浏览器的页面导航发起，查询串不写入访问日志（认证保留路径的统一处理）。

const (
	// adminTicketPath 在后台入口域名上签发票据，adminLoginPath 在站点上兑换票据
	adminTicketPath = authPath + "/admin-ticket"
	adminLoginPath  = authPath + "/admin-login"
	// adminHSCookie 把握手绑定到发起它的浏览器；同时作为失败后一段时间内不再重复握手的标记
	adminHSCookie = "gately_admin_hs"
	// adminStateTTL 为 state 有效期，adminTicketTTL 为票据有效期，adminHSMaxAge 为握手标记 Cookie 的存活时间
	adminStateTTL  = 5 * time.Minute
	adminTicketTTL = 60 * time.Second
	adminHSMaxAge  = 120
	// maxAdminTickets 限制同时存在的票据数：签发需要管理员登录态，达到上限只会出现在异常情况
	maxAdminTickets = 256
)

// errInvalidAdminEntry 为后台入口地址格式错误。
var errInvalidAdminEntry = errors.New("后台入口地址需为 http(s)://域名[:端口] 的形式，不带路径")

// adminEntry 是后台入口：scheme://host[:port]。
type adminEntry struct {
	origin string
	host   string
}

// adminTicket 是入口域名登记的一次性票据。
type adminTicket struct {
	aud       string
	nonceHash string
	bridge    string
	bridgeExp time.Time
	next      string
	exp       time.Time
}

// adminState 是站点发起握手时签名的状态。
type adminState struct {
	Origin    string `json:"o"`
	Next      string `json:"n"`
	NonceHash string `json:"h"`
}

// adminHandshake 保存握手所需的运行时数据。
type adminHandshake struct {
	entry atomic.Pointer[adminEntry]
	// hosts 为开启后台登录免认证的站点的精确域名（小写），入口据此校验跳转目标
	hosts   atomic.Pointer[[]string]
	mu      sync.Mutex
	tickets map[string]adminTicket
}

// parseAdminEntry 规范化后台入口地址：只接受 http(s)://域名[:端口]，不带路径、查询、账号；空串表示不启用。
func parseAdminEntry(raw string) (adminEntry, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return adminEntry{}, false, nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return adminEntry{}, false, errInvalidAdminEntry
	}
	if p := u.Port(); p != "" && !validPort(p) {
		return adminEntry{}, false, errInvalidAdminEntry
	}
	host := strings.ToLower(u.Hostname())
	origin := u.Scheme + "://" + strings.ToLower(u.Host)
	return adminEntry{origin: origin, host: host}, true, nil
}

// validPort 判断端口字符串是否为 1 到 65535 的纯数字。
func validPort(p string) bool {
	n, err := strconv.Atoi(p)
	return err == nil && n >= 1 && n <= 65535 && strconv.Itoa(n) == p
}

// setAdminEntry 设置后台入口，空串或无效值关闭握手。
func (a *authenticator) setAdminEntry(raw string) {
	e, ok, err := parseAdminEntry(raw)
	if err != nil || !ok {
		a.hs.entry.Store(nil)
		return
	}
	a.hs.entry.Store(&e)
}

// setAdminHosts 设置开启后台登录免认证的站点域名；通配域名与 "*" 不参与握手。
func (a *authenticator) setAdminHosts(domains []string) {
	var hosts []string
	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" || strings.Contains(d, "*") || strings.ContainsAny(d, "{}") {
			continue
		}
		hosts = append(hosts, d)
	}
	a.hs.hosts.Store(&hosts)
}

// adminHostAllowed 判断域名是否为开启后台登录免认证的站点的精确域名。
func (a *authenticator) adminHostAllowed(host string) bool {
	hs := a.hs.hosts.Load()
	if hs == nil {
		return false
	}
	for _, h := range *hs {
		if h == host {
			return true
		}
	}
	return false
}

// hsNoStore 给握手相关的响应加上禁止缓存与不发送来源页的头：网址里带有一次性票据。
func hsNoStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

// siteOrigin 返回浏览器访问本请求时使用的站点来源 scheme://host[:端口]；域名必须是站点的精确域名。
func siteOrigin(r *http.Request, s Site) (string, bool) {
	host := hostOnly(r.Host)
	exact := false
	for _, d := range s.Domains {
		if strings.EqualFold(d, host) {
			exact = true
			break
		}
	}
	if !exact {
		return "", false
	}
	scheme := "http"
	if requestIsHTTPS(r) {
		scheme = "https"
	}
	origin := scheme + "://" + host
	if i := strings.LastIndexByte(r.Host, ':'); i > 0 && !strings.HasSuffix(r.Host, "]") {
		p := r.Host[i+1:]
		if !validPort(p) {
			return "", false
		}
		origin += ":" + p
	}
	return origin, true
}

func nonceHash(nonce string) string {
	sum := sha256.Sum256([]byte(nonce))
	return hex.EncodeToString(sum[:])
}

func randomTicketID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// startAdminHandshake 在站点上发起握手：浏览器的页面导航既没有桥接凭据也没有认证时，跳转到后台入口。
// 发起了跳转返回 true。没有配置入口、站点不是精确域名、非页面导航、近期已经握手过（Cookie 存在）时不处理。
func (a *authenticator) startAdminHandshake(w http.ResponseWriter, r *http.Request, s Site) bool {
	entry := a.hs.entry.Load()
	if entry == nil || !s.adminHS {
		return false
	}
	if r.Method != http.MethodGet || !isBrowser(r) {
		return false
	}
	// 只对页面导航发起：脚本请求、子资源、框架内的页面不跳转
	if m := r.Header.Get("Sec-Fetch-Mode"); m != "" && m != "navigate" {
		return false
	}
	if d := r.Header.Get("Sec-Fetch-Dest"); d != "" && d != "document" {
		return false
	}
	if strings.HasPrefix(r.URL.Path, authPath) {
		return false
	}
	// 后台入口本身不握手
	if hostOnly(r.Host) == entry.host {
		return false
	}
	if _, err := r.Cookie(adminHSCookie); err == nil {
		return false
	}
	origin, ok := siteOrigin(r, s)
	if !ok {
		return false
	}
	nonce := randomTicketID()
	state := a.signBlob("admin-state", adminState{Origin: origin, Next: safeNext(r.URL.RequestURI()), NonceHash: nonceHash(nonce)}, adminStateTTL)
	http.SetCookie(w, &http.Cookie{
		Name: adminHSCookie, Value: nonce, Path: "/", MaxAge: adminHSMaxAge,
		HttpOnly: true, Secure: requestIsHTTPS(r), SameSite: http.SameSiteLaxMode,
	})
	hsNoStore(w)
	http.Redirect(w, r, entry.origin+adminTicketPath+"?state="+url.QueryEscape(state), http.StatusFound)
	return true
}

// serveAdminTicket 在后台入口域名上处理 admin-ticket。返回 false 表示这不是入口上的握手请求，由调用方继续处理。
func (a *authenticator) serveAdminTicket(w http.ResponseWriter, r *http.Request) bool {
	entry := a.hs.entry.Load()
	if entry == nil || r.URL.Path != adminTicketPath || hostOnly(r.Host) != entry.host {
		return false
	}
	hsNoStore(w)
	var st adminState
	if err := a.verifyBlob("admin-state", r.URL.Query().Get("state"), &st); err != nil {
		http.Error(w, "验证已超时，请重新登录", http.StatusBadRequest)
		return true
	}
	// 来源必须是开启后台登录免认证的站点的精确域名（签名只能证明它由本实例签发，配置可能在此期间变化）
	ou, err := url.Parse(st.Origin)
	if err != nil || (ou.Scheme != "http" && ou.Scheme != "https") || ou.Hostname() == "" || ou.User != nil ||
		(ou.Port() != "" && !validPort(ou.Port())) || !a.adminHostAllowed(strings.ToLower(ou.Hostname())) || strings.ToLower(ou.Hostname()) == entry.host {
		http.Error(w, "原站点地址无效", http.StatusBadRequest)
		return true
	}
	back := ou.Scheme + "://" + strings.ToLower(ou.Host) + adminLoginPath
	next := safeNext(st.Next)
	bridge, bridgeExp := httpx.IssueAdminBridge(r)
	if bridge == "" {
		if r.URL.Query().Get("s") != "1" {
			// 跨站跳转不会带上 SameSite=Strict 的后台 Cookie：返回一个页面，由它发起同站的第二次请求再判断
			again := entry.origin + adminTicketPath + "?state=" + url.QueryEscape(r.URL.Query().Get("state")) + "&s=1"
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<!doctype html><meta charset=utf-8><meta name=robots content=noindex><meta http-equiv=refresh content=\"0;url=" +
				htmlAttr(again) + "\"><script>location.replace(" + jsString(again) + ")</script><a href=\"" + htmlAttr(again) + "\">&rarr;</a>"))
			return true
		}
		// 确实没有登录后台：回到站点，由站点按原有方式认证
		http.Redirect(w, r, back+"?none=1&next="+url.QueryEscape(next), http.StatusFound)
		return true
	}
	id := randomTicketID()
	if !a.storeAdminTicket(id, adminTicket{aud: st.Origin, nonceHash: st.NonceHash, bridge: bridge, bridgeExp: bridgeExp, next: next, exp: time.Now().Add(adminTicketTTL)}) {
		http.Error(w, "验证已超时，请重新登录", http.StatusServiceUnavailable)
		return true
	}
	http.Redirect(w, r, back+"?ticket="+id, http.StatusFound)
	return true
}

// storeAdminTicket 登记票据；先清理过期票据，仍达到上限时拒绝。
func (a *authenticator) storeAdminTicket(id string, t adminTicket) bool {
	a.hs.mu.Lock()
	defer a.hs.mu.Unlock()
	if a.hs.tickets == nil {
		a.hs.tickets = map[string]adminTicket{}
	}
	now := time.Now()
	if len(a.hs.tickets) >= maxAdminTickets {
		for k, v := range a.hs.tickets {
			if now.After(v.exp) {
				delete(a.hs.tickets, k)
			}
		}
		if len(a.hs.tickets) >= maxAdminTickets {
			return false
		}
	}
	a.hs.tickets[id] = t
	return true
}

// takeAdminTicket 取出并删除票据（一次性）；不存在或已过期返回 false。
func (a *authenticator) takeAdminTicket(id string) (adminTicket, bool) {
	a.hs.mu.Lock()
	defer a.hs.mu.Unlock()
	t, ok := a.hs.tickets[id]
	if !ok {
		return adminTicket{}, false
	}
	delete(a.hs.tickets, id)
	if time.Now().After(t.exp) {
		return adminTicket{}, false
	}
	return t, true
}

// serveAdminLogin 在站点上处理 admin-login：兑换票据后写下本站点域名的凭据 Cookie，或处理“没有登录后台”的返回。
// 返回 false 表示站点没有开启握手，由调用方继续处理。
func (a *authenticator) serveAdminLogin(w http.ResponseWriter, r *http.Request, s Site) bool {
	if r.URL.Path != adminLoginPath || !s.adminHS || a.hs.entry.Load() == nil {
		return false
	}
	hsNoStore(w)
	q := r.URL.Query()
	if q.Get("none") == "1" {
		// 入口确认没有登录后台：回到原路径，握手标记 Cookie 让站点在短时间内不再重复握手
		http.Redirect(w, r, safeNext(q.Get("next")), http.StatusFound)
		return true
	}
	t, ok := a.takeAdminTicket(q.Get("ticket"))
	if !ok {
		http.Error(w, "登录票据已使用", http.StatusBadRequest)
		return true
	}
	origin, ok := siteOrigin(r, s)
	if !ok || origin != t.aud {
		http.Error(w, "登录票据与站点不匹配", http.StatusBadRequest)
		return true
	}
	// 票据必须由发起握手的同一浏览器使用，防止拿到网址的人在别处兑换
	c, err := r.Cookie(adminHSCookie)
	if err != nil || subtle.ConstantTimeCompare([]byte(nonceHash(c.Value)), []byte(t.nonceHash)) != 1 {
		http.Error(w, "登录状态与当前浏览器不匹配，请重新登录", http.StatusBadRequest)
		return true
	}
	http.SetCookie(w, &http.Cookie{
		Name: adminBridgeCookie, Value: t.bridge, Path: "/", Expires: t.bridgeExp,
		HttpOnly: true, Secure: requestIsHTTPS(r), SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{Name: adminHSCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, safeNext(t.next), http.StatusFound)
	return true
}

// htmlAttr 转义 HTML 属性值。
func htmlAttr(s string) string {
	return strings.NewReplacer("&", "&amp;", "\"", "&quot;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// jsString 输出可嵌入 <script> 的 JavaScript 字符串字面量，转义引号、反斜杠、尖括号与换行类字符。
func jsString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, c := range s {
		switch {
		case c == '"' || c == '\\':
			b.WriteByte('\\')
			b.WriteRune(c)
		case c == '<' || c == '>' || c == '&' || c == '\u2028' || c == '\u2029' || c < 0x20:
			b.WriteString("\\u")
			b.WriteString(strings.ToUpper(strconv.FormatInt(int64(c)+0x10000, 16)[1:]))
		default:
			b.WriteRune(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}
