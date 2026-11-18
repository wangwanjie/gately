package webservice

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/httpx"
)

// adminCookieHeader 是带有后台登录态的浏览器访问同主机名站点时携带的 Cookie。
const adminCookieHeader = auth.CookieName + "=admin-token; gately_entry=entry-mark; gately_login_nonce=n1; other=1"

// cookieEcho 启动一个记录收到的 Cookie 的后端，返回它与收到的 Cookie 通道。
func cookieEcho(t *testing.T, network, addr string) (*httptest.Server, chan string) {
	t.Helper()
	got := make(chan string, 8)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("Cookie")
		// 模拟管理后台：带有效会话 Cookie 时为已登录
		if c, err := r.Cookie(auth.CookieName); err == nil && c.Value == "admin-token" {
			_, _ = w.Write([]byte("logged-in"))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	ln, err := net.Listen(network, addr)
	if err != nil {
		t.Skipf("无法监听 %s %s: %v", network, addr, err)
	}
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return srv, got
}

// withAdminListen 登记管理后台的监听地址，测试结束时取消。
func withAdminListen(t *testing.T, addrs ...string) {
	t.Helper()
	httpx.SetAdminListen(addrs)
	t.Cleanup(func() { httpx.SetAdminListen(nil) })
}

// TestAdminCookieOnlyToAdminUpstream 是安全审查的回归用例：后台登录 Cookie 只交给本实例的管理后台，
// 同机其他端口的应用（审查 PoC 的场景）收不到；站点反代到本实例后台时照常转发，后台经站点访问不会掉登录。
func TestAdminCookieOnlyToAdminUpstream(t *testing.T) {
	admin, adminGot := cookieEcho(t, "tcp4", "127.0.0.1:0")
	other, otherGot := cookieEcho(t, "tcp4", "127.0.0.1:0")
	_, adminPort, _ := net.SplitHostPort(admin.Listener.Addr().String())
	// 后台监听全部地址（默认 :16680 的形式）
	withAdminListen(t, "[::]:"+adminPort)
	l := Listener{Sites: []Site{
		{Enable: true, Key: "admin", Domains: []string{"admin.test"}, Targets: []string{admin.URL}},
		{Enable: true, Key: "byname", Domains: []string{"byname.test"}, Targets: []string{"http://localhost:" + adminPort}},
		{Enable: true, Key: "other", Domains: []string{"other.test"}, Targets: []string{other.URL}},
		// 转发 Gately 登录 Cookie 只针对站点认证 Cookie，后台 Cookie 同样不能交给其他应用
		{Enable: true, Key: "pass", Domains: []string{"pass.test"}, Targets: []string{other.URL}, PassAuthCookies: true},
	}}
	addr, _ := startRunner(t, l, nil, nil)
	for _, tc := range []struct {
		host  string
		got   chan string
		keep  bool
		login bool
	}{
		{"admin.test", adminGot, true, true},
		{"byname.test", adminGot, true, true},
		{"other.test", otherGot, false, false},
		{"pass.test", otherGot, false, false},
	} {
		resp, body := get(t, noRedirect(), addr, tc.host, "/api/session", hdr("Cookie", adminCookieHeader))
		v := <-tc.got
		for _, name := range []string{auth.CookieName, "gately_entry", "gately_login_nonce"} {
			if strings.Contains(v, name+"=") != tc.keep {
				t.Errorf("%s: Cookie %s 转发情况应为 %v: %q", tc.host, name, tc.keep, v)
			}
		}
		if !strings.Contains(v, "other=1") {
			t.Errorf("%s: 普通 Cookie 应保留: %q", tc.host, v)
		}
		if (resp.StatusCode == http.StatusOK && body == "logged-in") != tc.login {
			t.Errorf("%s: 登录状态应为 %v: %d %q", tc.host, tc.login, resp.StatusCode, body)
		}
	}
	// 没有登记后台地址（例如只经飞牛网关套接字提供后台）时一律剥离
	httpx.SetAdminListen(nil)
	get(t, noRedirect(), addr, "admin.test", "/", hdr("Cookie", adminCookieHeader))
	if v := <-adminGot; strings.Contains(v, auth.CookieName+"=") {
		t.Fatalf("未登记后台地址时不应转发后台 Cookie: %q", v)
	}
}

// 后台只监听具体地址时按该地址比对；IPv6 回环同样适用。
func TestAdminCookieSpecificAddress(t *testing.T) {
	admin6, admin6Got := cookieEcho(t, "tcp6", "[::1]:0")
	other6, other6Got := cookieEcho(t, "tcp6", "[::1]:0")
	withAdminListen(t, admin6.Listener.Addr().String())
	l := Listener{Sites: []Site{
		{Enable: true, Key: "a", Domains: []string{"a.test"}, Targets: []string{admin6.URL}},
		{Enable: true, Key: "o", Domains: []string{"o.test"}, Targets: []string{other6.URL}},
	}}
	addr, _ := startRunner(t, l, nil, nil)
	get(t, noRedirect(), addr, "a.test", "/", hdr("Cookie", adminCookieHeader))
	if v := <-admin6Got; !strings.Contains(v, auth.CookieName+"=admin-token") {
		t.Fatalf("IPv6 回环上的后台应收到后台 Cookie: %q", v)
	}
	get(t, noRedirect(), addr, "o.test", "/", hdr("Cookie", adminCookieHeader))
	if v := <-other6Got; strings.Contains(v, auth.CookieName+"=") {
		t.Fatalf("IPv6 回环其他端口不应收到后台 Cookie: %q", v)
	}
}

// 后端判定：主机名解析后逐个比对，解析失败、局域网地址、部分地址不是后台时都不算后台。
func TestUpstreamIsAdmin(t *testing.T) {
	withAdminListen(t, "[::]:16680", "127.0.0.1:9443")
	old := lookupUpstreamIPs
	defer func() {
		lookupUpstreamIPs = old
		resetUpstreamIPCache()
	}()
	lookupUpstreamIPs = func(_ context.Context, host string) ([]netip.Addr, error) {
		switch host {
		case "self.lan":
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		case "mixed.lan":
			return []netip.Addr{netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("192.0.2.10")}, nil
		case "nas.lan":
			return []netip.Addr{netip.MustParseAddr("192.0.2.10")}, nil
		}
		return nil, errors.New("no such host")
	}
	resetUpstreamIPCache()
	for raw, want := range map[string]bool{
		"http://127.0.0.1:16680":   true,
		"http://[::1]:16680":       true,
		"http://self.lan:16680":    true,
		"https://127.0.0.1:9443":   true,
		"http://127.0.0.1:16681":   false, // 同机其他端口
		"http://127.0.0.1":         false, // 默认端口 80
		"http://192.0.2.10:16680":  false, // 局域网其他主机
		"http://nas.lan:16680":     false,
		"http://mixed.lan:16680":   false, // 解析出的地址不全是后台
		"http://missing.lan:16680": false, // 解析失败
		"http://[::1]:9443":        false, // 9443 只监听 127.0.0.1
		"https://self.lan":         false,
		"http://0.0.0.0:16680":     false,
	} {
		u, _ := url.Parse(raw)
		if got := upstreamIsAdmin(context.Background(), u); got != want {
			t.Errorf("%s 应为 %v", raw, want)
		}
	}
}

// 转发认证服务无论地址如何都收不到后台登录 Cookie。
func TestForwardAuthStripsAdminCookie(t *testing.T) {
	fa, got := cookieEcho(t, "tcp4", "127.0.0.1:0")
	_, port, _ := net.SplitHostPort(fa.Listener.Addr().String())
	// 即使认证服务恰好位于登记的后台地址上也不转发
	withAdminListen(t, "[::]:"+port)
	l := Listener{Sites: []Site{
		{Enable: true, Key: "fa", Domains: []string{"fa.test"}, Type: TypeText, Text: "ok", ForwardAuth: ForwardAuth{URL: fa.URL}},
	}}
	addr, _ := startRunner(t, l, nil, nil)
	get(t, noRedirect(), addr, "fa.test", "/", hdr("Cookie", adminCookieHeader))
	v := <-got
	for _, name := range []string{auth.CookieName, "gately_entry", "gately_login_nonce"} {
		if strings.Contains(v, name+"=") {
			t.Errorf("转发认证服务不应收到 %s: %q", name, v)
		}
	}
	if !strings.Contains(v, "other=1") {
		t.Errorf("普通 Cookie 应保留: %q", v)
	}
}

// 后端是另一台 Gately（如在线演示实例，或外层 Gately 经 frp 回到内网 Gately 的双层部署）时，浏览器为该域名带的
// 同名后台 Cookie（会话、安全入口标记、第三方登录随机数）是后端签发的，应原样转发：否则后端登录后的每个请求都是 401、
// 开了安全入口的内层后台一直被拒、第三方登录回调提示浏览器不匹配。本实例签发的仍然剥离。
func TestAdminCookieForeignPassed(t *testing.T) {
	other, otherGot := cookieEcho(t, "tcp4", "127.0.0.1:0")
	withAdminListen(t, "127.0.0.1:1")
	own := map[string]string{auth.CookieName: "admin-token", "gately_entry": "own-entry", "gately_login_nonce": "own-nonce"}
	httpx.SetOwnAdminCookieCheck(func(name, v string) bool { return own[name] == v })
	t.Cleanup(func() { httpx.SetOwnAdminCookieCheck(nil) })
	l := Listener{Sites: []Site{
		{Enable: true, Key: "other", Domains: []string{"other.test"}, Targets: []string{other.URL}},
	}}
	addr, _ := startRunner(t, l, nil, nil)

	get(t, noRedirect(), addr, "other.test", "/", hdr("Cookie", auth.CookieName+"=backend-token; gately_entry=e; gately_login_nonce=n; other=1"))
	v := <-otherGot
	for _, want := range []string{auth.CookieName + "=backend-token", "gately_entry=e", "gately_login_nonce=n", "other=1"} {
		if !strings.Contains(v, want) {
			t.Fatalf("其他实例签发的后台 Cookie 与普通 Cookie 应转发给后端，缺 %s: %q", want, v)
		}
	}

	get(t, noRedirect(), addr, "other.test", "/", hdr("Cookie", auth.CookieName+"=admin-token; gately_entry=own-entry; gately_login_nonce=own-nonce; other=1"))
	if v := <-otherGot; v != "other=1" {
		t.Fatalf("本实例签发的后台 Cookie 不应转发给其他后端: %q", v)
	}

	// 同名 Cookie 混有本实例签发的时只剥离本实例的那个
	get(t, noRedirect(), addr, "other.test", "/", hdr("Cookie", auth.CookieName+"=admin-token; "+auth.CookieName+"=backend-token; gately_entry=own-entry; gately_entry=e"))
	if v := <-otherGot; strings.Contains(v, "admin-token") || strings.Contains(v, "own-entry") || !strings.Contains(v, "backend-token") || !strings.Contains(v, "gately_entry=e") {
		t.Fatalf("只应剥离本实例签发的后台 Cookie: %q", v)
	}
}
