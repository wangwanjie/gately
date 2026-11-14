package webservice

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/auth"
)

// 跨父域握手测试使用一个简化的浏览器：按主机名保存 Cookie（区分仅主机与带 Domain），
// 跟随跳转并记录每一跳，跨注册域的跳转不发送 SameSite=Strict 的 Cookie（与浏览器对跨站导航的处理一致），
// 页面里的 meta refresh 视为页面自己发起的同站导航。

type hsCookie struct {
	c        *http.Cookie
	domain   string
	hostOnly bool
}

type hsHop struct {
	URL    string
	Status int
	Loc    string
}

type hsBrowser struct {
	t    *testing.T
	addr string
	jar  []hsCookie
	// ua 与 accept 模拟浏览器的页面导航
	hdr map[string]string
}

func newHSBrowser(t *testing.T, addr string) *hsBrowser {
	return &hsBrowser{t: t, addr: addr, hdr: map[string]string{"User-Agent": "Mozilla/5.0", "Accept": "text/html", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "document"}}
}

// site 返回主机名的“站点”（取最后两段，足够区分测试里的 a.test、b.test、c.test）。
func hsSite(host string) string {
	parts := strings.Split(host, ".")
	if len(parts) <= 2 {
		return host
	}
	return strings.Join(parts[len(parts)-2:], ".")
}

func (b *hsBrowser) set(host string, c *http.Cookie) {
	dom, only := host, true
	if c.Domain != "" {
		dom, only = strings.TrimPrefix(strings.ToLower(c.Domain), "."), false
	}
	for i, e := range b.jar {
		if e.c.Name == c.Name && e.domain == dom && e.hostOnly == only {
			b.jar = append(b.jar[:i], b.jar[i+1:]...)
			break
		}
	}
	if c.MaxAge < 0 || (c.Value == "" && c.MaxAge == 0 && !c.Expires.IsZero() && c.Expires.Before(time.Now())) {
		return
	}
	b.jar = append(b.jar, hsCookie{c: c, domain: dom, hostOnly: only})
}

func (b *hsBrowser) cookies(host string, crossSite bool) []string {
	var out []string
	for _, e := range b.jar {
		match := host == e.domain || (!e.hostOnly && strings.HasSuffix(host, "."+e.domain))
		if !match {
			continue
		}
		if crossSite && e.c.SameSite == http.SameSiteStrictMode {
			continue
		}
		out = append(out, e.c.Name+"="+e.c.Value)
	}
	return out
}

func (b *hsBrowser) cookie(host, name string) string {
	for _, e := range b.jar {
		if e.c.Name == name && (host == e.domain || (!e.hostOnly && strings.HasSuffix(host, "."+e.domain))) {
			return e.c.Value
		}
	}
	return ""
}

// metaRefresh 取页面里 meta refresh 的目标地址。
var metaRefresh = regexp.MustCompile(`http-equiv=refresh content="0;url=([^"]+)"`)

// nav 导航到地址并跟随跳转，返回最后的响应、正文与每一跳的记录。typed 表示由用户直接输入（首跳视为同站）。
func (b *hsBrowser) nav(raw string, extra map[string]string) (*http.Response, string, []hsHop) {
	b.t.Helper()
	var hops []hsHop
	cur := raw
	prevSite := ""
	for i := 0; i < 12; i++ {
		u, err := url.Parse(cur)
		if err != nil {
			b.t.Fatal(err)
		}
		host := hostOnlyTest(u.Host)
		cross := prevSite != "" && hsSite(host) != prevSite
		req, _ := http.NewRequest("GET", "http://"+b.addr+u.RequestURI(), nil)
		req.Host = u.Host
		for k, v := range b.hdr {
			req.Header.Set(k, v)
		}
		for k, v := range extra {
			req.Header.Set(k, v)
		}
		if cs := b.cookies(host, cross); len(cs) > 0 {
			req.Header.Set("Cookie", strings.Join(cs, "; "))
		}
		resp, err := noRedirect().Do(req)
		if err != nil {
			b.t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		for _, c := range resp.Cookies() {
			b.set(host, c)
		}
		loc := resp.Header.Get("Location")
		hops = append(hops, hsHop{URL: cur, Status: resp.StatusCode, Loc: loc})
		prevSite = hsSite(host)
		switch {
		case resp.StatusCode >= 300 && resp.StatusCode < 400 && loc != "":
			lu, err := u.Parse(loc)
			if err != nil {
				b.t.Fatal(err)
			}
			cur = lu.String()
		case resp.StatusCode == 200 && metaRefresh.Match(body):
			// 页面自己发起的导航：同站，且不再视为跨站跳转链
			m := metaRefresh.FindSubmatch(body)
			cur = strings.ReplaceAll(string(m[1]), "&amp;", "&")
			prevSite = ""
		default:
			return resp, string(body), hops
		}
	}
	b.t.Fatalf("跳转次数过多: %v", hops)
	return nil, "", nil
}

func hostOnlyTest(h string) string {
	if i := strings.LastIndexByte(h, ':'); i > 0 {
		return h[:i]
	}
	return h
}

// hsEnv 是握手测试环境：admin.a.test 为后台入口（不认证的站点），b.test 下是开启后台登录免认证的站点。
type hsEnv struct {
	env  *adminEnv
	addr string
	port string
	r    *listenerRunner
	b    *hsBrowser
}

func startHandshake(t *testing.T) *hsEnv {
	t.Helper()
	env := newAdminEnv(t)
	l := Listener{
		Sites: []Site{
			{Enable: true, Key: "adm", Domains: []string{"admin.a.test"}, Type: TypeText, Text: "admin-ui"},
			{Enable: true, Key: "basic", Domains: []string{"basic.b.test"}, Type: TypeText, Text: "basic-ok", BasicAuth: true, Users: []string{"bob:pw"}, AdminPass: true},
			{Enable: true, Key: "basic2", Domains: []string{"basic2.b.test"}, Type: TypeText, Text: "basic2-ok", BasicAuth: true, Users: []string{"bob:pw"}, AdminPass: true},
			{Enable: true, Key: "web", Domains: []string{"web.b.test"}, Type: TypeText, Text: "web-ok", WebAuth: true, Users: []string{"alice:pw"}, AdminPass: true},
			{Enable: true, Key: "plain", Domains: []string{"plain.b.test"}, Type: TypeText, Text: "plain-ok", BasicAuth: true, Users: []string{"bob:pw"}},
			{Enable: true, Key: "wild", Domains: []string{"*.c.test"}, Type: TypeText, Text: "wild-ok", BasicAuth: true, Users: []string{"bob:pw"}, AdminPass: true},
		},
		Default: Site{Enable: true, Type: TypeText, Text: "default-ok", BasicAuth: true, Users: []string{"bob:pw"}, AdminPass: true},
	}
	addr, r := startRunner(t, l, nil, nil)
	port := addr[strings.LastIndexByte(addr, ':')+1:]
	r.auth.setAdminEntry("http://admin.a.test:" + port)
	r.auth.setAdminHosts(adminPassDomains([]Listener{{Enable: true, Sites: l.Sites}}))
	r.auth.adminPass.Store(true)
	return &hsEnv{env: env, addr: addr, port: port, r: r, b: newHSBrowser(t, addr)}
}

// loginAdmin 让浏览器登录后台（后台 Cookie 为 SameSite=Strict，仅主机，只属于入口域名）。
func (e *hsEnv) loginAdmin(t *testing.T) string {
	t.Helper()
	tok, _, err := e.env.m.Login("1.1.1.1", "admin", e.env.pass)
	if err != nil {
		t.Fatal(err)
	}
	e.b.set("admin.a.test", &http.Cookie{Name: auth.CookieName, Value: tok, SameSite: http.SameSiteStrictMode})
	return tok
}

func (e *hsEnv) u(host, path string) string {
	return fmt.Sprintf("http://%s:%s%s", host, e.port, path)
}

// 完整流程：已登录后台的浏览器访问另一个父域的站点，经入口握手后直接放行，原路径与查询串保留；
// 握手后同站点的访问不再跳转；未开启选项的站点仍要求认证。
func TestHandshakeFlow(t *testing.T) {
	e := startHandshake(t)
	e.loginAdmin(t)

	resp, body, hops := e.b.nav(e.u("basic.b.test", "/x?y=1"), nil)
	if resp.StatusCode != http.StatusOK || body != "basic-ok" {
		t.Fatalf("握手后应放行: %d %q hops=%v", resp.StatusCode, body, hops)
	}
	last := hops[len(hops)-1]
	if last.URL != e.u("basic.b.test", "/x?y=1") {
		t.Fatalf("应回到原路径与查询串: %v", hops)
	}
	// 经过：站点 302 → 入口（跨站，无后台 Cookie，返回同站自跳转页面）→ 入口（带后台 Cookie）302 → 站点兑换 302 → 原路径
	if len(hops) != 5 {
		t.Fatalf("跳转链不符合预期: %v", hops)
	}
	if e.b.cookie("basic.b.test", adminBridgeCookie) == "" {
		t.Fatal("站点域名下应得到凭据 Cookie")
	}
	if e.b.cookie("basic.b.test", adminHSCookie) != "" {
		t.Fatal("握手完成后应清除握手标记")
	}
	// 凭据只属于该站点的域名（仅主机 Cookie），不会出现在其他域名上
	for _, h := range []string{"basic2.b.test", "web.b.test", "admin.a.test"} {
		if e.b.cookie(h, adminBridgeCookie) != "" {
			t.Fatalf("凭据不应出现在 %s 上", h)
		}
	}
	// 再次访问不再跳转
	resp, body, hops = e.b.nav(e.u("basic.b.test", "/again"), nil)
	if resp.StatusCode != http.StatusOK || body != "basic-ok" || len(hops) != 1 {
		t.Fatalf("握手后应直接放行: %d %q %v", resp.StatusCode, body, hops)
	}
	// 另一个站点（同父域）需要各自握手一次：凭据是仅主机的
	resp, body, hops = e.b.nav(e.u("basic2.b.test", "/"), nil)
	if resp.StatusCode != http.StatusOK || body != "basic2-ok" || len(hops) < 3 {
		t.Fatalf("另一个站点应各自握手: %d %q %v", resp.StatusCode, body, hops)
	}
	// 未开启选项的站点
	if resp, _, hops := e.b.nav(e.u("plain.b.test", "/"), nil); resp.StatusCode != http.StatusUnauthorized || len(hops) != 1 {
		t.Fatalf("未开启选项的站点应直接要求认证: %d %v", resp.StatusCode, hops)
	}
}

// 网页认证站点同样握手；握手失败时仍然显示原来的登录页。
func TestHandshakeWebAuth(t *testing.T) {
	e := startHandshake(t)
	e.loginAdmin(t)
	resp, body, _ := e.b.nav(e.u("web.b.test", "/p"), nil)
	if resp.StatusCode != http.StatusOK || body != "web-ok" {
		t.Fatalf("网页认证站点应握手放行: %d %q", resp.StatusCode, body)
	}
	e2 := startHandshake(t)
	resp, _, hops := e2.b.nav(e2.u("web.b.test", "/p"), nil)
	if resp.StatusCode != http.StatusOK || !strings.HasSuffix(hops[len(hops)-1].URL, authPath+"/login?next=%2Fp") {
		t.Fatalf("未登录后台应回到登录页: %d %v", resp.StatusCode, hops)
	}
}

// 没有登录后台：握手后回到站点原有的认证；握手标记使短时间内不再重复握手，也没有登记任何票据。
func TestHandshakeNotLoggedIn(t *testing.T) {
	e := startHandshake(t)
	resp, _, hops := e.b.nav(e.u("basic.b.test", "/"), nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("没有登录后台应要求认证: %d %v", resp.StatusCode, hops)
	}
	if e.b.cookie("basic.b.test", adminBridgeCookie) != "" {
		t.Fatal("不应得到凭据")
	}
	e.r.auth.hs.mu.Lock()
	n := len(e.r.auth.hs.tickets)
	e.r.auth.hs.mu.Unlock()
	if n != 0 {
		t.Fatalf("没有后台登录态不应登记票据: %d", n)
	}
	resp, _, hops = e.b.nav(e.u("basic.b.test", "/"), nil)
	if resp.StatusCode != http.StatusUnauthorized || len(hops) != 1 {
		t.Fatalf("短时间内不应重复握手: %d %v", resp.StatusCode, hops)
	}
	// 站点自己的账号照常可用
	if resp, body, _ := e.b.nav(e.u("basic.b.test", "/"), map[string]string{"Authorization": "Basic Ym9iOnB3"}); resp.StatusCode != http.StatusOK || body != "basic-ok" {
		t.Fatalf("站点账号应可用: %d %q", resp.StatusCode, body)
	}
}

// 后台登出后，已握手得到的凭据立即失效。
func TestHandshakeRevoked(t *testing.T) {
	e := startHandshake(t)
	tok := e.loginAdmin(t)
	if resp, _, _ := e.b.nav(e.u("basic.b.test", "/"), nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("握手应成功: %d", resp.StatusCode)
	}
	if err := e.env.m.Revoke(tok); err != nil {
		t.Fatal(err)
	}
	// 后台 Cookie 也已失效：站点重新握手，入口确认未登录，回到认证
	if resp, _, _ := e.b.nav(e.u("basic.b.test", "/"), nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("登出后应要求认证: %d", resp.StatusCode)
	}
}

// 不满足条件的请求不发起握手：非页面导航、非 GET、非浏览器、框架内、未配置入口、默认站点、通配域名站点、入口自身。
func TestHandshakeNotStarted(t *testing.T) {
	e := startHandshake(t)
	e.loginAdmin(t)
	get1 := func(host, path string, h map[string]string, method string) *http.Response {
		req, _ := http.NewRequest(method, "http://"+e.addr+path, nil)
		req.Host = host + ":" + e.port
		for k, v := range h {
			req.Header.Set(k, v)
		}
		resp, err := noRedirect().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}
	nav := map[string]string{"User-Agent": "Mozilla/5.0", "Accept": "text/html"}
	with := func(kv ...string) map[string]string {
		h := map[string]string{}
		for k, v := range nav {
			h[k] = v
		}
		for i := 0; i+1 < len(kv); i += 2 {
			h[kv[i]] = kv[i+1]
		}
		return h
	}
	cases := map[string]*http.Response{
		"fetch/XHR":  get1("basic.b.test", "/", with("Sec-Fetch-Mode", "cors"), "GET"),
		"iframe":     get1("basic.b.test", "/", with("Sec-Fetch-Dest", "iframe"), "GET"),
		"POST":       get1("basic.b.test", "/", nav, "POST"),
		"curl":       get1("basic.b.test", "/", map[string]string{"User-Agent": "curl/8"}, "GET"),
		"通配域名站点":     get1("x.c.test", "/", nav, "GET"),
		"默认站点（未知域名）": get1("unknown.example", "/", nav, "GET"),
	}
	for name, resp := range cases {
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s 不应跳转: %d %s", name, resp.StatusCode, resp.Header.Get("Location"))
		}
	}
	// 入口自身带着没有后台登录态的请求不握手（入口站点无认证，这里仅确认不跳转到自己）
	if resp := get1("admin.a.test", "/", nav, "GET"); resp.StatusCode != http.StatusOK {
		t.Errorf("入口站点不应跳转: %d", resp.StatusCode)
	}
	// 未配置入口
	e.r.auth.setAdminEntry("")
	if resp := get1("basic.b.test", "/", nav, "GET"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("未配置入口不应跳转: %d", resp.StatusCode)
	}
}

// signState 构造带有效签名的 state，用于检验入口对来源的二次校验（签名只能证明由本实例签发，配置与来源仍要核对）。
func (e *hsEnv) signState(origin, next, nonce string) string {
	return e.r.auth.signBlob("admin-state", adminState{Origin: origin, Next: next, NonceHash: nonceHash(nonce)}, adminStateTTL)
}

func (e *hsEnv) ticketReq(host, state string, hdr map[string]string) *http.Response {
	req, _ := http.NewRequest("GET", "http://"+e.addr+adminTicketPath+"?state="+url.QueryEscape(state)+"&s=1", nil)
	req.Host = host + ":" + e.port
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := noRedirect().Do(req)
	if err != nil {
		e.b.t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

// 入口对 state 的校验：伪造签名、过期、来源不在开启选项的站点中（含通配域名站点、未知域名、入口自身、外部域名）、
// 来源格式错误一律拒绝；原路径必须是站内相对路径。
func TestHandshakeEntryValidatesState(t *testing.T) {
	e := startHandshake(t)
	tok := e.loginAdmin(t)
	admin := map[string]string{"Cookie": auth.CookieName + "=" + tok}
	good := "http://basic.b.test:" + e.port

	if resp := e.ticketReq("admin.a.test", e.signState(good, "/", "n"), admin); resp.StatusCode != http.StatusFound {
		t.Fatalf("有效的 state 应签发票据: %d", resp.StatusCode)
	}
	bad := map[string]string{
		"伪造签名":      "garbage.value",
		"外部域名":      e.signState("http://evil.example", "/", "n"),
		"外部域名带端口":   e.signState("http://evil.example:"+e.port, "/", "n"),
		"通配域名站点":    e.signState("http://x.c.test:"+e.port, "/", "n"),
		"未开启选项的站点":  e.signState("http://plain.b.test:"+e.port, "/", "n"),
		"入口自身":      e.signState("http://admin.a.test:"+e.port, "/", "n"),
		"账号注入":      e.signState("http://basic.b.test@evil.example", "/", "n"),
		"无效端口":      e.signState("http://basic.b.test:99999", "/", "n"),
		"非 http 协议": e.signState("ftp://basic.b.test", "/", "n"),
		"空来源":       e.signState("", "/", "n"),
	}
	for name, st := range bad {
		if resp := e.ticketReq("admin.a.test", st, admin); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s 应被拒绝: %d %s", name, resp.StatusCode, resp.Header.Get("Location"))
		}
	}
	// 原路径不是站内相对路径时回退为 /
	resp := e.ticketReq("admin.a.test", e.signState(good, "//evil.example/x", "n"), admin)
	id := strings.TrimPrefix(resp.Header.Get("Location")[strings.Index(resp.Header.Get("Location"), "ticket="):], "ticket=")
	e.r.auth.hs.mu.Lock()
	tk := e.r.auth.hs.tickets[id]
	e.r.auth.hs.mu.Unlock()
	if tk.next != "/" {
		t.Fatalf("危险的原路径应回退为 /: %q", tk.next)
	}
	// 入口只响应入口域名：其他域名上同一路径不处理（落到站点自己的认证）
	if resp := e.ticketReq("basic.b.test", e.signState(good, "/", "n"), admin); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("非入口域名不应处理: %d", resp.StatusCode)
	}
	// 过期的 state
	old := e.r.auth.signBlob("admin-state", adminState{Origin: good, Next: "/", NonceHash: "x"}, -time.Minute)
	if resp := e.ticketReq("admin.a.test", old, admin); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("过期的 state 应拒绝: %d", resp.StatusCode)
	}
	// 其他用途的签名内容不能当 state 使用
	other := e.r.auth.signBlob("state", adminState{Origin: good, Next: "/", NonceHash: "x"}, time.Minute)
	if resp := e.ticketReq("admin.a.test", other, admin); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("其他用途的签名应拒绝: %d", resp.StatusCode)
	}
}

// 没有后台登录态时入口不签发票据：伪造的后台 Cookie、只读账号的 Cookie 同样不行。
func TestHandshakeEntryRequiresAdmin(t *testing.T) {
	e := startHandshake(t)
	good := "http://basic.b.test:" + e.port
	st := e.signState(good, "/", "n")
	for name, h := range map[string]map[string]string{
		"无 Cookie":  nil,
		"伪造 Cookie": {"Cookie": auth.CookieName + "=forged.value"},
	} {
		resp := e.ticketReq("admin.a.test", st, h)
		if resp.StatusCode != http.StatusFound || !strings.Contains(resp.Header.Get("Location"), "none=1") {
			t.Errorf("%s 应返回未登录: %d %s", name, resp.StatusCode, resp.Header.Get("Location"))
		}
	}
	if err := e.env.m.AddUser("viewer", "viewer-pass-1"); err != nil {
		t.Fatal(err)
	}
	vt, _, _ := e.env.m.Login("1.1.1.9", "viewer", "viewer-pass-1")
	resp := e.ticketReq("admin.a.test", st, map[string]string{"Cookie": auth.CookieName + "=" + vt})
	if !strings.Contains(resp.Header.Get("Location"), "none=1") {
		t.Errorf("只读账号不应得到票据: %s", resp.Header.Get("Location"))
	}
	e.r.auth.hs.mu.Lock()
	n := len(e.r.auth.hs.tickets)
	e.r.auth.hs.mu.Unlock()
	if n != 0 {
		t.Fatalf("不应登记票据: %d", n)
	}
}

// issueTicket 经入口取得一张有效票据，返回票据编号与发起握手的随机数。
func (e *hsEnv) issueTicket(t *testing.T, host string) (id, nonce string) {
	t.Helper()
	tok := e.loginAdmin(t)
	nonce = "nonce-" + host
	resp := e.ticketReq("admin.a.test", e.signState("http://"+host+":"+e.port, "/home", nonce), map[string]string{"Cookie": auth.CookieName + "=" + tok})
	loc := resp.Header.Get("Location")
	i := strings.Index(loc, "ticket=")
	if resp.StatusCode != http.StatusFound || i < 0 {
		t.Fatalf("应签发票据: %d %s", resp.StatusCode, loc)
	}
	return loc[i+len("ticket="):], nonce
}

func (e *hsEnv) redeem(host, ticket string, cookies string) (*http.Response, string) {
	req, _ := http.NewRequest("GET", "http://"+e.addr+adminLoginPath+"?ticket="+ticket, nil)
	req.Host = host + ":" + e.port
	if cookies != "" {
		req.Header.Set("Cookie", cookies)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := noRedirect().Do(req)
	if err != nil {
		e.b.t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(b)
}

// 票据兑换：一次性、绑定来源站点与发起握手的浏览器、有过期时间；失败的尝试同样消耗票据。
func TestHandshakeTicketRedemption(t *testing.T) {
	e := startHandshake(t)
	hs := func(n string) string { return adminHSCookie + "=" + n }

	// 正常兑换，且第二次使用被拒绝
	id, nonce := e.issueTicket(t, "basic.b.test")
	resp, _ := e.redeem("basic.b.test", id, hs(nonce))
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/home" {
		t.Fatalf("应兑换成功并回到原路径: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	var bridge *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == adminBridgeCookie {
			bridge = c
		}
	}
	if bridge == nil || bridge.Domain != "" || !bridge.HttpOnly || bridge.SameSite != http.SameSiteLaxMode || bridge.Expires.IsZero() {
		t.Fatalf("凭据 Cookie 应为仅主机、HttpOnly、Lax、带过期时间: %+v", bridge)
	}
	if resp, _ := e.redeem("basic.b.test", id, hs(nonce)); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("票据重复使用应拒绝: %d", resp.StatusCode)
	}

	// 随机数不符（拿到网址但不是发起握手的浏览器）或没有 Cookie：拒绝，且票据已作废
	id, nonce = e.issueTicket(t, "basic.b.test")
	for name, ck := range map[string]string{"无 Cookie": "", "随机数不符": hs("other")} {
		id2, _ := e.issueTicket(t, "basic.b.test")
		if resp, _ := e.redeem("basic.b.test", id2, ck); resp.StatusCode != http.StatusBadRequest || strings.Contains(resp.Header.Get("Set-Cookie"), adminBridgeCookie) {
			t.Errorf("%s 应拒绝: %d", name, resp.StatusCode)
		}
		if resp, _ := e.redeem("basic.b.test", id2, hs("nonce-basic.b.test")); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s 失败后票据应作废: %d", name, resp.StatusCode)
		}
	}

	// 来源不符：票据发给 basic.b.test，在另一个开启选项的站点上兑换
	if resp, _ := e.redeem("basic2.b.test", id, hs(nonce)); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("来源不符应拒绝: %d", resp.StatusCode)
	}
	// 在没有开启选项的站点、通配域名站点、未知域名（默认站点）上兑换：不处理，也不会得到凭据
	id, nonce = e.issueTicket(t, "basic.b.test")
	for _, h := range []string{"plain.b.test", "x.c.test", "unknown.example"} {
		resp, _ := e.redeem(h, id, hs(nonce))
		if strings.Contains(strings.Join(resp.Header.Values("Set-Cookie"), ";"), adminBridgeCookie) {
			t.Errorf("%s 上不应得到凭据", h)
		}
	}

	// 过期的票据
	id, nonce = e.issueTicket(t, "basic.b.test")
	e.r.auth.hs.mu.Lock()
	tk := e.r.auth.hs.tickets[id]
	tk.exp = time.Now().Add(-time.Second)
	e.r.auth.hs.tickets[id] = tk
	e.r.auth.hs.mu.Unlock()
	if resp, _ := e.redeem("basic.b.test", id, hs(nonce)); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("过期的票据应拒绝: %d", resp.StatusCode)
	}
	// 编号猜测
	for _, g := range []string{"", "0", strings.Repeat("0", 32), "../x"} {
		if resp, _ := e.redeem("basic.b.test", g, hs("x")); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("无效编号 %q 应拒绝: %d", g, resp.StatusCode)
		}
	}
}

// 网址里没有可重放的凭据：握手各跳的跳转地址与页面都不包含凭据本体；握手响应禁止缓存且不发送来源页。
func TestHandshakeNoSecretsInURLs(t *testing.T) {
	e := startHandshake(t)
	tok := e.loginAdmin(t)
	_, _, hops := e.b.nav(e.u("basic.b.test", "/p"), nil)
	bridge := e.b.cookie("basic.b.test", adminBridgeCookie)
	if bridge == "" {
		t.Fatal("握手应成功")
	}
	for _, h := range hops {
		if strings.Contains(h.Loc, bridge) || strings.Contains(h.URL, bridge) || strings.Contains(h.Loc, tok) || strings.Contains(h.URL, tok) {
			t.Fatalf("网址里不应出现凭据: %v", h)
		}
	}
	st := e.signState("http://basic.b.test:"+e.port, "/", "n")
	for _, h := range []map[string]string{nil, {"Cookie": auth.CookieName + "=" + tok}} {
		resp := e.ticketReq("admin.a.test", st, h)
		if resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("Referrer-Policy") != "no-referrer" {
			t.Fatalf("握手响应应禁止缓存且不发送来源页: %v", resp.Header)
		}
	}
	// 访问日志不记录认证保留路径的查询串（票据编号、state）
	if !strings.HasPrefix(adminTicketPath, authPath) || !strings.HasPrefix(adminLoginPath, authPath) {
		t.Fatal("握手路径应位于认证保留路径之下")
	}
}

// 第一跳是跨站跳转，SameSite=Strict 的后台 Cookie 不会发送：入口必须返回同站自跳转页面，而不是直接判定未登录。
func TestHandshakeStrictCookieBounce(t *testing.T) {
	e := startHandshake(t)
	st := e.signState("http://basic.b.test:"+e.port, "/", "n")
	req, _ := http.NewRequest("GET", "http://"+e.addr+adminTicketPath+"?state="+url.QueryEscape(st), nil)
	req.Host = "admin.a.test:" + e.port
	resp, err := noRedirect().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	m := metaRefresh.FindSubmatch(b)
	if resp.StatusCode != http.StatusOK || m == nil || !strings.Contains(string(m[1]), "&amp;s=1") || !strings.Contains(string(b), "location.replace(") {
		t.Fatalf("首跳应返回自跳转页面: %d %s", resp.StatusCode, b)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("页面类型不对: %s", resp.Header.Get("Content-Type"))
	}
}

// 票据数量受限，过期票据在登记时被清理。
func TestHandshakeTicketCap(t *testing.T) {
	a := newAuthenticator("s")
	exp := time.Now().Add(time.Minute)
	for i := 0; i < maxAdminTickets; i++ {
		if !a.storeAdminTicket(fmt.Sprintf("%032d", i), adminTicket{exp: exp}) {
			t.Fatalf("第 %d 张应能登记", i)
		}
	}
	if a.storeAdminTicket("extra", adminTicket{exp: exp}) {
		t.Fatal("达到上限后应拒绝")
	}
	a.hs.mu.Lock()
	for k, v := range a.hs.tickets {
		v.exp = time.Now().Add(-time.Second)
		a.hs.tickets[k] = v
		break
	}
	a.hs.mu.Unlock()
	if !a.storeAdminTicket("after-expire", adminTicket{exp: exp}) {
		t.Fatal("清理过期票据后应能登记")
	}
}

func TestParseAdminEntry(t *testing.T) {
	ok := map[string]string{
		"https://Admin.Example.com":        "https://admin.example.com",
		" https://admin.example.com:4433/": "https://admin.example.com:4433",
		"http://10.0.0.1:16680":            "http://10.0.0.1:16680",
	}
	for in, want := range ok {
		e, on, err := parseAdminEntry(in)
		if err != nil || !on || e.origin != want {
			t.Errorf("%q 应解析为 %q: %+v %v %v", in, want, e, on, err)
		}
	}
	if _, on, err := parseAdminEntry("  "); on || err != nil {
		t.Errorf("空值应表示关闭: %v %v", on, err)
	}
	for _, in := range []string{"admin.example.com", "ftp://x.com", "https://", "https://u:p@x.com", "https://x.com/path", "https://x.com?q=1", "https://x.com#f", "https://x.com:0", "https://x.com:99999", "javascript:alert(1)"} {
		if _, _, err := parseAdminEntry(in); err == nil {
			t.Errorf("%q 应被拒绝", in)
		}
	}
}

// 自跳转页面里的地址被正确转义，不会注入标记或脚本。
func TestHandshakeEscaping(t *testing.T) {
	evil := `a"><script>alert(1)</script>'\` + " "
	if h := htmlAttr(evil); strings.ContainsAny(h, `"<>`) {
		t.Errorf("HTML 属性未转义: %s", h)
	}
	j := jsString(evil)
	if strings.ContainsAny(j[1:len(j)-1], "<> ") || strings.Count(j, `"`) != 3 {
		t.Errorf("JS 字符串未转义: %s", j)
	}
}

func TestAdminPassDomains(t *testing.T) {
	ls := []Listener{
		{Enable: true, Sites: []Site{{Enable: true, AdminPass: true, Domains: []string{"a.test", "b.test"}}, {Enable: true, Domains: []string{"no.test"}}, {AdminPass: true, Domains: []string{"off.test"}}}},
		{Sites: []Site{{Enable: true, AdminPass: true, Domains: []string{"offl.test"}}}},
	}
	got := adminPassDomains(ls)
	if len(got) != 2 || got[0] != "a.test" || got[1] != "b.test" {
		t.Fatalf("域名列表不对: %v", got)
	}
	a := newAuthenticator("s")
	a.setAdminHosts([]string{"A.test", "*.wild.test", "*", "x{1}.test", "ok.test", ""})
	if !a.adminHostAllowed("a.test") || !a.adminHostAllowed("ok.test") || a.adminHostAllowed("x.wild.test") || a.adminHostAllowed("*") || a.adminHostAllowed("") {
		t.Fatal("通配、模式、空域名不应参与握手")
	}
}

func BenchmarkHandshakeIdle(b *testing.B) {
	a := newAuthenticator("s")
	a.setAdminEntry("https://admin.example.com")
	site := Site{AdminPass: true, adminHS: true, BasicAuth: true, Domains: []string{"x.test"}}
	r, _ := http.NewRequest("POST", "http://x.test/", nil)
	r.Header.Set("User-Agent", "Mozilla/5.0")
	r.Header.Set("Accept", "text/html")
	w := &discardWriter{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.startAdminHandshake(w, r, site)
	}
}

type discardWriter struct{ h http.Header }

func (d *discardWriter) Header() http.Header {
	if d.h == nil {
		d.h = http.Header{}
	}
	return d.h
}
func (d *discardWriter) Write(p []byte) (int, error) { return len(p), nil }
func (d *discardWriter) WriteHeader(int)             {}
