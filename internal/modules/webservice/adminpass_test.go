package webservice

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/httpx"
)

// adminEnv 是后台登录免认证测试的环境：一个真实的管理员账号管理器，已注册为桥接凭据的签发与校验来源。
type adminEnv struct {
	m    *auth.Manager
	pass string
}

func newAdminEnv(t *testing.T) *adminEnv {
	t.Helper()
	m := auth.NewManager(auth.Account{}, func(auth.Account) error { return nil })
	pass, err := m.EnsureInitialized()
	if err != nil {
		t.Fatal(err)
	}
	httpx.SetSessionCheck(func(r *http.Request) string {
		c, err := r.Cookie(auth.CookieName)
		if err != nil {
			return ""
		}
		id, ok := m.Identify(c.Value)
		if !ok || id.Role != auth.RoleAdmin {
			return ""
		}
		return id.User
	})
	httpx.SetAdminBridge(func(r *http.Request) (string, time.Time) {
		c, err := r.Cookie(auth.CookieName)
		if err != nil {
			return "", time.Time{}
		}
		v, exp, ok := m.IssueBridge(c.Value)
		if !ok {
			return "", time.Time{}
		}
		return v, exp
	}, func(v string) string {
		u, _ := m.VerifyBridge(v)
		return u
	})
	t.Cleanup(func() {
		httpx.SetSessionCheck(nil)
		httpx.SetAdminBridge(nil, nil)
	})
	return &adminEnv{m: m, pass: pass}
}

// login 以管理员登录，返回后台登录 Cookie 头。
func (e *adminEnv) login(t *testing.T, ip string) string {
	t.Helper()
	tok, _, err := e.m.Login(ip, "admin", e.pass)
	if err != nil {
		t.Fatal(err)
	}
	return auth.CookieName + "=" + tok
}

// bridgeCookie 从响应中取桥接凭据 Cookie。
func bridgeCookie(resp *http.Response) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == adminBridgeCookie {
			return c
		}
	}
	return nil
}

var browserHdr = map[string]string{"User-Agent": "Mozilla/5.0", "Accept": "text/html"}

func hdr(kv ...string) map[string]string {
	h := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		h[kv[i]] = kv[i+1]
	}
	return h
}

// adminPassListener 构造测试用监听器：admin.example.test 模拟后台所在的站点（不认证），
// 其余站点覆盖基本认证、网页认证、未开启选项、域外域名。
func adminPassListener(backendURL string) Listener {
	return Listener{Sites: []Site{
		{Enable: true, Key: "adm", Domains: []string{"admin.example.test"}, Type: TypeText, Text: "admin-ui"},
		{Enable: true, Key: "basic", Domains: []string{"basic.example.test"}, Type: TypeText, Text: "basic-ok", BasicAuth: true, Users: []string{"bob:pw"}, AdminPass: true},
		{Enable: true, Key: "web", Domains: []string{"web.example.test"}, Type: TypeText, Text: "web-ok", WebAuth: true, Users: []string{"alice:pw"}, AdminPass: true},
		{Enable: true, Key: "plain", Domains: []string{"plain.example.test"}, Type: TypeText, Text: "plain-ok", BasicAuth: true, Users: []string{"bob:pw"}},
		{Enable: true, Key: "outside", Domains: []string{"outside.test"}, Type: TypeText, Text: "outside-ok", BasicAuth: true, Users: []string{"bob:pw"}, AdminPass: true},
		{Enable: true, Key: "proxy", Domains: []string{"proxy.example.test"}, Targets: []string{backendURL}, BasicAuth: true, Users: []string{"bob:pw"}, AdminPass: true},
	}}
}

// startAdminPass 启动测试监听器并按单点登录域配置认证器。
func startAdminPass(t *testing.T, ssoDomain string, backendURL string) (string, *listenerRunner) {
	t.Helper()
	addr, r := startRunner(t, adminPassListener(backendURL), nil, nil)
	r.auth.shared.set(nil, ssoDomain)
	r.auth.adminPass.Store(true)
	return addr, r
}

// 完整流程：管理员经后台所在站点访问后台，写出桥接凭据；带凭据访问开启选项的站点免认证，
// 未带凭据仍要求认证；基本认证与网页认证站点都生效。
func TestAdminPassFlow(t *testing.T) {
	env := newAdminEnv(t)
	addr, _ := startAdminPass(t, "example.test", "http://127.0.0.1:1")
	admin := env.login(t, "1.1.1.1")

	resp, _ := get(t, noRedirect(), addr, "admin.example.test", "/", hdr("Cookie", admin))
	c := bridgeCookie(resp)
	if c == nil {
		t.Fatal("管理员访问后台所在站点应写出桥接凭据")
	}
	if c.Domain != "example.test" || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.Expires.IsZero() {
		t.Fatalf("桥接凭据 Cookie 属性不对: %+v", c)
	}
	bridge := adminBridgeCookie + "=" + c.Value

	// 基本认证站点：无凭据 401，带桥接凭据放行
	if resp, _ := get(t, noRedirect(), addr, "basic.example.test", "/", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("未登录后台应要求认证: %d", resp.StatusCode)
	}
	if resp, body := get(t, noRedirect(), addr, "basic.example.test", "/", hdr("Cookie", bridge)); resp.StatusCode != http.StatusOK || body != "basic-ok" {
		t.Fatalf("已登录后台应免基本认证: %d %q", resp.StatusCode, body)
	}
	// 网页认证站点：浏览器未登录跳转登录页，带桥接凭据直接放行；非浏览器客户端同样放行
	if resp, _ := get(t, noRedirect(), addr, "web.example.test", "/x", browserHdr); resp.StatusCode != http.StatusFound {
		t.Fatalf("未登录后台应跳转登录页: %d", resp.StatusCode)
	}
	h := hdr("Cookie", bridge)
	for k, v := range browserHdr {
		h[k] = v
	}
	if resp, body := get(t, noRedirect(), addr, "web.example.test", "/x", h); resp.StatusCode != http.StatusOK || body != "web-ok" {
		t.Fatalf("已登录后台应免网页认证: %d %q", resp.StatusCode, body)
	}
	if resp, body := get(t, noRedirect(), addr, "web.example.test", "/x", hdr("Cookie", bridge)); resp.StatusCode != http.StatusOK || body != "web-ok" {
		t.Fatalf("非浏览器客户端也应放行: %d %q", resp.StatusCode, body)
	}
	// 网页认证站点的登录页仍可打开（管理员也可以改用站点账号登录）
	if resp, _ := get(t, noRedirect(), addr, "web.example.test", authPath+"/login", h); resp.StatusCode != http.StatusOK {
		t.Fatalf("登录页应仍可访问: %d", resp.StatusCode)
	}
	// 站点自己的账号照常可用
	if resp, body := get(t, noRedirect(), addr, "basic.example.test", "/", basicHeader("bob", "pw")); resp.StatusCode != http.StatusOK || body != "basic-ok" {
		t.Fatalf("站点账号应仍可登录: %d %q", resp.StatusCode, body)
	}
}

// 未开启选项的站点、单点登录域之外的站点不认桥接凭据。
func TestAdminPassScope(t *testing.T) {
	env := newAdminEnv(t)
	addr, _ := startAdminPass(t, "example.test", "http://127.0.0.1:1")
	resp, _ := get(t, noRedirect(), addr, "admin.example.test", "/", hdr("Cookie", env.login(t, "1.1.1.1")))
	bridge := adminBridgeCookie + "=" + bridgeCookie(resp).Value

	for _, host := range []string{"plain.example.test", "outside.test"} {
		if resp, _ := get(t, noRedirect(), addr, host, "/", hdr("Cookie", bridge)); resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s 不应认桥接凭据: %d", host, resp.StatusCode)
		}
	}
	// 域外的站点收到后台 Cookie 也不签发凭据
	resp, _ = get(t, noRedirect(), addr, "outside.test", "/", hdr("Cookie", env.login(t, "1.1.1.2")))
	if bridgeCookie(resp) != nil {
		t.Fatal("单点登录域之外的站点不应签发桥接凭据")
	}
}

// 没有任何站点开启选项、没有配置单点登录域、没有后台登录态时都不签发凭据。
func TestAdminPassNoIssue(t *testing.T) {
	env := newAdminEnv(t)
	admin := hdr("Cookie", env.login(t, "1.1.1.1"))
	addr, r := startAdminPass(t, "example.test", "http://127.0.0.1:1")

	if resp, _ := get(t, noRedirect(), addr, "admin.example.test", "/", nil); bridgeCookie(resp) != nil {
		t.Fatal("没有后台登录态不应签发凭据")
	}
	if resp, _ := get(t, noRedirect(), addr, "admin.example.test", "/", hdr("Cookie", auth.CookieName+"=forged.value")); bridgeCookie(resp) != nil {
		t.Fatal("伪造的后台 Cookie 不应签发凭据")
	}
	r.auth.adminPass.Store(false)
	if resp, _ := get(t, noRedirect(), addr, "admin.example.test", "/", admin); bridgeCookie(resp) != nil {
		t.Fatal("没有站点开启选项时不应签发凭据")
	}
	r.auth.adminPass.Store(true)
	r.auth.shared.set(nil, "")
	if resp, _ := get(t, noRedirect(), addr, "admin.example.test", "/", admin); bridgeCookie(resp) != nil {
		t.Fatal("没有配置单点登录域时不应签发凭据")
	}
	r.auth.shared.set(nil, "example.test")
	if resp, _ := get(t, noRedirect(), addr, "admin.example.test", "/", admin); bridgeCookie(resp) == nil {
		t.Fatal("条件齐备时应签发凭据")
	}
}

// 已带有效桥接凭据的请求不重复签发。
func TestAdminPassNoReissue(t *testing.T) {
	env := newAdminEnv(t)
	addr, _ := startAdminPass(t, "example.test", "http://127.0.0.1:1")
	admin := env.login(t, "1.1.1.1")
	resp, _ := get(t, noRedirect(), addr, "admin.example.test", "/", hdr("Cookie", admin))
	bridge := adminBridgeCookie + "=" + bridgeCookie(resp).Value
	resp, _ = get(t, noRedirect(), addr, "admin.example.test", "/", hdr("Cookie", admin+"; "+bridge))
	if bridgeCookie(resp) != nil {
		t.Fatal("已有有效凭据时不应重复签发")
	}
}

// 登出、退出所有设备、只读账号：凭据立即失效或根本不签发。
func TestAdminPassRevocation(t *testing.T) {
	env := newAdminEnv(t)
	addr, _ := startAdminPass(t, "example.test", "http://127.0.0.1:1")

	tok, _, _ := env.m.Login("1.1.1.1", "admin", env.pass)
	resp, _ := get(t, noRedirect(), addr, "admin.example.test", "/", hdr("Cookie", auth.CookieName+"="+tok))
	bridge := adminBridgeCookie + "=" + bridgeCookie(resp).Value
	if resp, _ := get(t, noRedirect(), addr, "basic.example.test", "/", hdr("Cookie", bridge)); resp.StatusCode != http.StatusOK {
		t.Fatalf("登出前应放行: %d", resp.StatusCode)
	}
	if err := env.m.Revoke(tok); err != nil {
		t.Fatal(err)
	}
	if resp, _ := get(t, noRedirect(), addr, "basic.example.test", "/", hdr("Cookie", bridge)); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("后台登出后应立即要求认证: %d", resp.StatusCode)
	}

	tok2, _, _ := env.m.Login("1.1.1.2", "admin", env.pass)
	resp, _ = get(t, noRedirect(), addr, "admin.example.test", "/", hdr("Cookie", auth.CookieName+"="+tok2))
	bridge2 := adminBridgeCookie + "=" + bridgeCookie(resp).Value
	if err := env.m.RevokeAll(); err != nil {
		t.Fatal(err)
	}
	if resp, _ := get(t, noRedirect(), addr, "basic.example.test", "/", hdr("Cookie", bridge2)); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("退出所有设备后应要求认证: %d", resp.StatusCode)
	}

	if err := env.m.AddUser("viewer", "tide-Lamp-47-oak"); err != nil {
		t.Fatal(err)
	}
	vt, _, err := env.m.Login("1.1.1.3", "viewer", "tide-Lamp-47-oak")
	if err != nil {
		t.Fatal(err)
	}
	resp, _ = get(t, noRedirect(), addr, "admin.example.test", "/", hdr("Cookie", auth.CookieName+"="+vt))
	if bridgeCookie(resp) != nil {
		t.Fatal("只读账号不应获得桥接凭据")
	}
}

// 伪造与错用：后台会话令牌当桥接凭据、乱写的值、另一个实例的凭据、多个同名 Cookie 中夹带无效值。
func TestAdminPassForgery(t *testing.T) {
	env := newAdminEnv(t)
	addr, _ := startAdminPass(t, "example.test", "http://127.0.0.1:1")
	tok, _, _ := env.m.Login("1.1.1.1", "admin", env.pass)
	resp, _ := get(t, noRedirect(), addr, "admin.example.test", "/", hdr("Cookie", auth.CookieName+"="+tok))
	good := bridgeCookie(resp).Value

	for name, v := range map[string]string{"会话令牌": tok, "乱写": "abc.def", "空": "", "截断": good[:len(good)-4]} {
		if resp, _ := get(t, noRedirect(), addr, "basic.example.test", "/", hdr("Cookie", adminBridgeCookie+"="+v)); resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s 不应放行: %d", name, resp.StatusCode)
		}
	}
	// 兄弟子域名上的应用往父域写了同名的无效 Cookie 排在前面：逐个校验，有效的仍然放行
	mixed := adminBridgeCookie + "=junk; " + adminBridgeCookie + "=" + good
	if resp, _ := get(t, noRedirect(), addr, "basic.example.test", "/", hdr("Cookie", mixed)); resp.StatusCode != http.StatusOK {
		t.Fatalf("夹带无效同名 Cookie 时有效凭据仍应放行: %d", resp.StatusCode)
	}
	// 同名 Cookie 数量受限：有效凭据排在上限之后不再校验
	many := strings.Repeat(adminBridgeCookie+"=junk; ", maxBridgeCookies) + adminBridgeCookie + "=" + good
	if resp, _ := get(t, noRedirect(), addr, "basic.example.test", "/", hdr("Cookie", many)); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("同名 Cookie 超过上限时不应继续校验: %d", resp.StatusCode)
	}
	// 另一个实例（不同密钥）签发的凭据
	other := newAdminEnvPair(t)
	if resp, _ := get(t, noRedirect(), addr, "basic.example.test", "/", hdr("Cookie", adminBridgeCookie+"="+other)); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("其他实例的凭据不应放行: %d", resp.StatusCode)
	}
}

// newAdminEnvPair 另建一个管理器并签发凭据，不改变已注册的桥接钩子。
func newAdminEnvPair(t *testing.T) string {
	t.Helper()
	m := auth.NewManager(auth.Account{}, func(auth.Account) error { return nil })
	pass, _ := m.EnsureInitialized()
	tok, _, err := m.Login("9.9.9.9", "admin", pass)
	if err != nil {
		t.Fatal(err)
	}
	v, _, _ := m.IssueBridge(tok)
	return v
}

// 桥接凭据与后台会话令牌都不转发给后端：反向代理站点只把其他 Cookie 交给后端。
func TestAdminPassCookieNotForwarded(t *testing.T) {
	env := newAdminEnv(t)
	var mu sync.Mutex
	var gotCookie string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotCookie = r.Header.Get("Cookie")
		mu.Unlock()
		_, _ = w.Write([]byte("backend"))
	}))
	defer backend.Close()
	addr, _ := startAdminPass(t, "example.test", backend.URL)
	resp, _ := get(t, noRedirect(), addr, "admin.example.test", "/", hdr("Cookie", env.login(t, "1.1.1.1")))
	bridge := adminBridgeCookie + "=" + bridgeCookie(resp).Value
	resp, body := get(t, noRedirect(), addr, "proxy.example.test", "/", hdr("Cookie", "theme=dark; "+bridge))
	if resp.StatusCode != http.StatusOK || body != "backend" {
		t.Fatalf("应放行并转发: %d %q", resp.StatusCode, body)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Contains(gotCookie, adminBridgeCookie) || gotCookie != "theme=dark" {
		t.Fatalf("桥接凭据不应转发给后端: %q", gotCookie)
	}
}

// 免认证放行的访客身份带 gately: 前缀，不会与同名的站点用户混淆。
func TestAdminPassVisitorIdentity(t *testing.T) {
	env := newAdminEnv(t)
	addr, r := startAdminPass(t, "example.test", "http://127.0.0.1:1")
	resp, _ := get(t, noRedirect(), addr, "admin.example.test", "/", hdr("Cookie", env.login(t, "1.1.1.1")))
	req, _ := http.NewRequest("GET", "http://basic.example.test/", nil)
	req.AddCookie(&http.Cookie{Name: adminBridgeCookie, Value: bridgeCookie(resp).Value})
	site := Site{AdminPass: true, BasicAuth: true}
	v, ok := r.auth.adminPassVisitor(req, site)
	if !ok || len(v.names) != 1 || v.names[0] != "gately:admin" {
		t.Fatalf("身份应为 gately:admin: %+v %v", v, ok)
	}
	if _, ok := r.auth.adminPassVisitor(req, Site{BasicAuth: true}); ok {
		t.Fatal("未开启选项的站点不应放行")
	}
}

// 保存站点时：没有启用基本认证或网页认证则自动关闭后台登录免认证；启用认证时保留。
func TestAdminPassNormalized(t *testing.T) {
	l := Listener{Key: "k", Name: "n", Port: 18080, Sites: []Site{
		{Enable: true, Domains: []string{"a.test"}, Type: TypeText, Text: "x", AdminPass: true},
		{Enable: true, Domains: []string{"b.test"}, Type: TypeText, Text: "x", AdminPass: true, BasicAuth: true, Users: []string{"u:p"}},
		{Enable: true, Domains: []string{"c.test"}, Type: TypeText, Text: "x", AdminPass: true, WebAuth: true, Users: []string{"u:p"}},
	}}
	if err := l.Validate(); err != nil {
		t.Fatal(err)
	}
	if l.Sites[0].AdminPass || !l.Sites[1].AdminPass || !l.Sites[2].AdminPass {
		t.Fatalf("规范化结果不对: %v %v %v", l.Sites[0].AdminPass, l.Sites[1].AdminPass, l.Sites[2].AdminPass)
	}
}

func TestAnyAdminPass(t *testing.T) {
	on := Site{Enable: true, AdminPass: true}
	cases := []struct {
		name string
		ls   []Listener
		want bool
	}{
		{"没有站点", nil, false},
		{"开启", []Listener{{Enable: true, Sites: []Site{on}}}, true},
		{"站点已停用", []Listener{{Enable: true, Sites: []Site{{AdminPass: true}}}}, false},
		{"监听器已停用", []Listener{{Sites: []Site{on}}}, false},
		{"另一个监听器开启", []Listener{{Enable: true, Sites: []Site{{Enable: true}}}, {Enable: true, Sites: []Site{on}}}, true},
	}
	for _, c := range cases {
		if got := anyAdminPass(c.ls); got != c.want {
			t.Errorf("%s: 期望 %v 实际 %v", c.name, c.want, got)
		}
	}
}

// 请求没有后台 Cookie 与桥接凭据时的开销：用于确认常规请求几乎无额外成本。
func BenchmarkAdminPassIdleRequest(b *testing.B) {
	a := newAuthenticator("s")
	a.adminPass.Store(true)
	a.shared.set(nil, "example.test")
	r := httptest.NewRequest("GET", "http://basic.example.test/", nil)
	w := httptest.NewRecorder()
	site := Site{AdminPass: true, BasicAuth: true}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.issueAdminBridge(w, r)
		_, _ = a.adminPassVisitor(r, site)
	}
}
