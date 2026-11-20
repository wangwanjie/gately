package webservice

import (
	"bufio"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	proxyproto "github.com/pires/go-proxyproto"
)

// sendViaProxy 模拟公网 Gately 经 frp 隧道明文转发：先发 PROXY v2 头声明真实访客 src，再发 HTTP 请求。
// useTLS 为 true 时在 PROXY 头之后做 TLS 握手（用于开启 HTTPS 的混合端口）。
func sendViaProxy(t *testing.T, addr, src string, useTLS bool, req *http.Request) *http.Response {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	h := proxyproto.HeaderProxyFromAddrs(2, &net.TCPAddr{IP: net.ParseIP(src), Port: 4321}, c.RemoteAddr())
	if _, err := h.WriteTo(c); err != nil {
		t.Fatal(err)
	}
	var rw net.Conn = c
	if useTLS {
		tc := tls.Client(c, &tls.Config{InsecureSkipVerify: true, ServerName: "secure.test"})
		if err := tc.Handshake(); err != nil {
			t.Fatal(err)
		}
		rw = tc
	}
	req.Header.Set("Connection", "close")
	if err := req.Write(rw); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(rw), req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

// loginReq 构造网页认证的登录请求，xfp 非空时带上 X-Forwarded-Proto。
func loginReq(host, xfp string) *http.Request {
	form := url.Values{"username": {"alice"}, "password": {"pw"}, "next": {"/"}}
	req, _ := http.NewRequest("POST", "http://"+host+authPath+"/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if xfp != "" {
		req.Header.Set("X-Forwarded-Proto", xfp)
	}
	return req
}

func sessionCookie(resp *http.Response) *http.Cookie {
	for _, c := range resp.Cookies() {
		if strings.HasPrefix(c.Name, "gately_site_") {
			return c
		}
	}
	return nil
}

// 经可信 PROXY 来源进入的明文连接，即使还原出的访客是公网地址，也采信 X-Forwarded-Proto: https，会话 Cookie 带 Secure。
func TestTrustedProxyForwardedProto(t *testing.T) {
	site := Site{Enable: true, Key: "s", Domains: []string{"app.test"}, Type: TypeText, Text: "ok", WebAuth: true, Users: []string{"alice:pw"}}
	addr, _ := startRunner(t, Listener{AcceptProxyProtocol: true, Sites: []Site{site}}, nil, []string{"127.0.0.1"})
	for _, tc := range []struct {
		name, xfp string
		secure    bool
	}{
		{"带 https 标记", "https", true},
		{"未带标记", "", false},
		{"标记为 http", "http", false},
	} {
		resp := sendViaProxy(t, addr, "203.0.113.5", false, loginReq("app.test", tc.xfp))
		c := sessionCookie(resp)
		if resp.StatusCode != http.StatusFound || c == nil {
			t.Fatalf("%s: 登录应成功: %d %v", tc.name, resp.StatusCode, resp.Cookies())
		}
		if c.Secure != tc.secure {
			t.Fatalf("%s: 会话 Cookie 的 Secure 应为 %v", tc.name, tc.secure)
		}
	}

	// 开启 HTTPS 并同端口提供明文 HTTP 时，经协议嗅探包装的连接同样能识别出可信 PROXY 来源
	mixed := plainHTTPListener(PlainHTTPServe)
	mixed.AcceptProxyProtocol = true
	mixed.Sites[0] = Site{Enable: true, Key: "m", Domains: []string{"secure.test"}, Type: TypeText, Text: "ok", WebAuth: true, Users: []string{"alice:pw"}}
	maddr, _ := startRunner(t, mixed, fakeCerts{selfSigned(t, "secure.test")}, []string{"127.0.0.1"})
	for _, useTLS := range []bool{false, true} {
		resp := sendViaProxy(t, maddr, "203.0.113.5", useTLS, loginReq("secure.test", "https"))
		if c := sessionCookie(resp); c == nil || !c.Secure {
			t.Fatalf("混合端口 tls=%v: 会话 Cookie 应带 Secure: %d %v", useTLS, resp.StatusCode, resp.Cookies())
		}
	}
}

// 不可信来源发来的 PROXY 头被忽略，公网访客自带的 X-Forwarded-Proto 也不被采信。
func TestRequestIsHTTPSUntrusted(t *testing.T) {
	r := httptest.NewRequest("GET", "http://app.test/", nil)
	r.RemoteAddr = "203.0.113.5:1234"
	r.Header.Set("X-Forwarded-Proto", "https")
	if requestIsHTTPS(r) {
		t.Fatal("公网来源自带的 X-Forwarded-Proto 不应被采信")
	}
	r.RemoteAddr = "192.168.1.2:1234"
	if !requestIsHTTPS(r) {
		t.Fatal("内网反向代理的 X-Forwarded-Proto 应被采信")
	}
}

// 签发者为 https 的内置 OIDC：经可信 PROXY 来源明文转发且带 https 标记时照常提供协议路径，未带标记时仍跳转到签发者地址。
func TestOIDCViaTrustedProxy(t *testing.T) {
	addr, r := startRunner(t, Listener{AcceptProxyProtocol: true}, nil, []string{"127.0.0.1"})
	users, err := normalizeAuthUsers([]AuthUser{{Name: "alice", Password: "alice-pw"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.auth.shared.set(users, nil)
	set := OIDCSettings{Enable: true, Issuer: "https://auth.test", Clients: []OIDCClient{{ID: "gitea", Secret: oidcTestSecret, RedirectURIs: []string{oidcTestRedirect}}}}
	if err := normalizeOIDC(&set, OIDCSettings{}); err != nil {
		t.Fatal(err)
	}
	if err := r.auth.oidc.set(set); err != nil {
		t.Fatal(err)
	}
	discovery := func(xfp string) *http.Response {
		req, _ := http.NewRequest("GET", "http://auth.test"+oidcDiscoveryPath, nil)
		if xfp != "" {
			req.Header.Set("X-Forwarded-Proto", xfp)
		}
		return sendViaProxy(t, addr, "203.0.113.5", false, req)
	}
	if resp := discovery("https"); resp.StatusCode != http.StatusOK {
		t.Fatalf("经可信 PROXY 来源以 https 转发时应提供发现文档: %d", resp.StatusCode)
	}
	if resp := discovery(""); resp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("未带 https 标记时应跳转到签发者地址: %d", resp.StatusCode)
	}
	// 登录页提交密码同样放行，并写出带 Secure 的会话 Cookie
	form := url.Values{"username": {"alice"}, "password": {"alice-pw"}, "next": {"/"}}
	req, _ := http.NewRequest("POST", "http://auth.test"+oidcLoginPath, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Forwarded-Proto", "https")
	resp := sendViaProxy(t, addr, "203.0.113.5", false, req)
	if resp.StatusCode != http.StatusFound || len(resp.Cookies()) == 0 || !resp.Cookies()[0].Secure {
		t.Fatalf("经可信 PROXY 来源登录应成功并写出 Secure Cookie: %d %v", resp.StatusCode, resp.Cookies())
	}
}

// 明文请求跳转 HTTPS 的端口：经可信 PROXY 来源以 https 转来的请求直接处理，未带标记的仍跳转，避免循环跳转。
func TestTrustedProxyNoHTTPSRedirectLoop(t *testing.T) {
	l := plainHTTPListener(PlainHTTPRedirect)
	l.AcceptProxyProtocol = true
	addr, _ := startRunner(t, l, fakeCerts{selfSigned(t, "secure.test")}, []string{"127.0.0.1"})
	get := func(xfp string) *http.Response {
		req, _ := http.NewRequest("GET", "http://secure.test/", nil)
		if xfp != "" {
			req.Header.Set("X-Forwarded-Proto", xfp)
		}
		return sendViaProxy(t, addr, "203.0.113.5", false, req)
	}
	if resp := get("https"); resp.StatusCode != http.StatusOK {
		t.Fatalf("可信来源以 https 转发时不应跳转: %d", resp.StatusCode)
	}
	if resp := get(""); resp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("未带 https 标记的明文请求应跳转: %d", resp.StatusCode)
	}
}

// 开启「添加 X-Forwarded-*」的反代站点：访客经可信 PROXY 来源以 HTTPS 访问时，向后端写 X-Forwarded-Proto: https；
// 未带标记时按本连接写 http。
func TestTrustedProxyForwardsProtoToBackend(t *testing.T) {
	got := make(chan string, 4)
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("X-Forwarded-Proto")
	}))
	t.Cleanup(b.Close)
	site := Site{Enable: true, Key: "p", Domains: []string{"app.test"}, Type: TypeProxy, Targets: []string{b.URL}, ForwardedHeaders: true}
	addr, _ := startRunner(t, Listener{AcceptProxyProtocol: true, Sites: []Site{site}}, nil, []string{"127.0.0.1"})
	for _, tc := range []struct{ xfp, want string }{{"https", "https"}, {"", "http"}} {
		req, _ := http.NewRequest("GET", "http://app.test/", nil)
		if tc.xfp != "" {
			req.Header.Set("X-Forwarded-Proto", tc.xfp)
		}
		if resp := sendViaProxy(t, addr, "203.0.113.5", false, req); resp.StatusCode != http.StatusOK {
			t.Fatalf("反代应成功: %d", resp.StatusCode)
		}
		if v := <-got; v != tc.want {
			t.Fatalf("带 %q 时后端收到的 X-Forwarded-Proto 应为 %q，实际 %q", tc.xfp, tc.want, v)
		}
	}
	// 本机的反向代理不经 PROXY 头直连、带 https 标记转来时，同样向后端写 https
	addr2, _ := startRunner(t, Listener{Sites: []Site{site}}, nil, nil)
	req, _ := http.NewRequest("GET", "http://"+addr2+"/", nil)
	req.Host = "app.test"
	req.Header.Set("X-Forwarded-Proto", "https")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if v := <-got; v != "https" {
		t.Fatalf("本机反向代理转来的 https 标记应传给后端，实际 %q", v)
	}
}
