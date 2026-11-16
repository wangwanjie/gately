package webservice

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/thirdauth"
)

// oauthOriginEnv 启动一个只含给定监听的运行器，第三方登录指向假的身份提供方，返回监听地址与端口。
func oauthOriginEnv(t *testing.T, mk func(port int) Listener, setup func(a *authenticator, port int)) (addr string, port int) {
	t.Helper()
	idp := fakeIdP(t, map[string]map[string]any{"alice": {"login": "alice"}})
	old := thirdauth.Endpoints
	thirdauth.Endpoints.GitHub.Authorize, thirdauth.Endpoints.GitHub.Token, thirdauth.Endpoints.GitHub.User = idp.URL+"/authorize", idp.URL+"/token", idp.URL+"/user"
	t.Cleanup(func() { thirdauth.Endpoints = old })
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port = ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	l := mk(port)
	if err := l.Validate(); err != nil {
		t.Fatal(err)
	}
	auth := newAuthenticator("secret")
	auth.setOAuth(oauthEnv{providers: map[string]OAuthProvider{"gh": {Key: "gh", Name: "GitHub", Type: thirdauth.TypeGitHub, ClientID: "id", ClientSecret: "sec"}},
		callbackBase: fmt.Sprintf("http://auth.test:%d", port)})
	if setup != nil {
		setup(auth, port)
	}
	r, err := newListenerRunner(l, logbuf.New(50, log.New(io.Discard, "", 0)).Logger("t"), auth, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.start(nil, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.stop)
	return fmt.Sprintf("127.0.0.1:%d", port), port
}

// oauthDo 以指定 Host 发送浏览器式的 GET 请求（不跟随跳转）。
func oauthDo(t *testing.T, c *http.Client, addr, host, uri string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("GET", "http://"+addr+uri, nil)
	req.Host = host
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "text/html")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp
}

// TestOAuthStartForgedHostTicketLeak 是安全审查的回归用例：攻击者用伪造的 Host 发起第三方登录，
// 受害者完成授权后，回跳地址不能指向伪造的域名或同主机名的其他端口，攻击者也不能取得站点会话。
func TestOAuthStartForgedHostTicketLeak(t *testing.T) {
	for _, tc := range []struct {
		name, evilHost string
		mk             func(port int) Listener
	}{
		{"default-site", "evil.test", func(port int) Listener {
			return Listener{ListenIP: "127.0.0.1", Network: "tcp4", Port: port,
				Default: Site{Enable: true, Type: TypeText, Text: "secret area", WebAuth: true, OAuthUsers: []string{"gh:alice"}}}
		}},
		{"wildcard-site", "evil.test", func(port int) Listener {
			return Listener{ListenIP: "127.0.0.1", Network: "tcp4", Port: port, Sites: []Site{
				{Enable: true, Key: "p", Domains: []string{"*"}, Type: TypeText, Text: "secret area", WebAuth: true, OAuthUsers: []string{"gh:alice"}}},
				Default: Site{Enable: true, Type: TypeText, Text: "dflt"}}
		}},
		{"exact-site-with-port-suffix", "app.test:1", func(port int) Listener {
			return Listener{ListenIP: "127.0.0.1", Network: "tcp4", Port: port, Sites: []Site{
				{Enable: true, Key: "p", Domains: []string{"app.test"}, Type: TypeText, Text: "secret area", WebAuth: true, OAuthUsers: []string{"gh:alice"}}},
				Default: Site{Enable: true, Type: TypeText, Text: "dflt"}}
		}},
		{"pattern-site-with-port-suffix", "a.app.test:1", func(port int) Listener {
			return Listener{ListenIP: "127.0.0.1", Network: "tcp4", Port: port, Sites: []Site{
				{Enable: true, Key: "p", Domains: []string{"*.app.test"}, Type: TypeText, Text: "secret area", WebAuth: true, OAuthUsers: []string{"gh:alice"}}},
				Default: Site{Enable: true, Type: TypeText, Text: "dflt"}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			addr, port := oauthOriginEnv(t, tc.mk, nil)
			attacker, victim := noRedirect(), noRedirect()
			attacker.Jar, _ = cookiejar.New(nil)
			victim.Jar, _ = cookiejar.New(nil)
			// 1. 攻击者用伪造的 Host 发起登录：应被拒绝，不签发 state
			st := oauthDo(t, attacker, addr, tc.evilHost, oauthStart+"?p=gh&next=/")
			loc, _ := url.Parse(st.Header.Get("Location"))
			state := ""
			if loc != nil {
				state = loc.Query().Get("state")
			}
			if st.StatusCode != http.StatusBadRequest {
				t.Errorf("伪造 Host 发起第三方登录应被拒绝: %d %s", st.StatusCode, st.Header.Get("Location"))
			}
			if state == "" {
				return
			}
			// 2. 受害者完成授权后回调认证域名：回跳地址不能指向攻击者给出的地址
			cb := oauthDo(t, victim, addr, fmt.Sprintf("auth.test:%d", port), oauthCallback+"?code=alice&state="+url.QueryEscape(state))
			if bu, _ := url.Parse(cb.Header.Get("Location")); bu != nil && bu.Host == tc.evilHost {
				t.Errorf("回跳地址把票据送到了伪造的地址: %s", cb.Header.Get("Location"))
			}
		})
	}
}

// TestOAuthStartAcceptedOrigins 覆盖应当允许发起第三方登录的访问地址，确保存量的合法用法不受影响。
func TestOAuthStartAcceptedOrigins(t *testing.T) {
	site := func(domains ...string) Site {
		return Site{Enable: true, Key: "p", Domains: domains, Type: TypeText, Text: "secret area", WebAuth: true, OAuthUsers: []string{"gh:alice"}}
	}
	dflt := Site{Enable: true, Type: TypeText, Text: "secret area", WebAuth: true, OAuthUsers: []string{"gh:alice"}}
	type hostCase struct {
		host   string
		ok     bool
		origin string // ok 时期望 state 中的来源，空表示 http://host
	}
	for _, tc := range []struct {
		name  string
		mk    func(port int) Listener
		setup func(a *authenticator, port int)
		hosts func(port int) []hostCase
	}{
		{"exact-site", func(port int) Listener {
			return Listener{ListenIP: "127.0.0.1", Network: "tcp4", Port: port, Sites: []Site{site("app.test")}}
		}, nil, func(port int) []hostCase {
			return []hostCase{
				{host: "app.test", ok: true},
				{host: "APP.test.", ok: true, origin: "http://app.test"},
				{host: fmt.Sprintf("app.test:%d", port), ok: true},
				{host: "app.test:1"},
				{host: "app.test:"},
				{host: "app.test:0"},
				{host: "app.test:99999"},
			}
		}},
		{"access-and-http-port", func(port int) Listener {
			return Listener{ListenIP: "127.0.0.1", Network: "tcp4", Port: port, AccessPort: 8443, AccessScheme: "https", Sites: []Site{site("app.test")}}
		}, nil, func(port int) []hostCase {
			return []hostCase{
				{host: "app.test:8443", ok: true},
				{host: "app.test:8444"},
			}
		}},
		{"pattern-site", func(port int) Listener {
			return Listener{ListenIP: "127.0.0.1", Network: "tcp4", Port: port, Sites: []Site{site("*.app.test")}}
		}, nil, func(port int) []hostCase {
			return []hostCase{
				{host: "a.app.test", ok: true},
				{host: fmt.Sprintf("b.app.test:%d", port), ok: true},
				{host: "b.app.test:1"},
			}
		}},
		{"default-site", func(port int) Listener {
			return Listener{ListenIP: "127.0.0.1", Network: "tcp4", Port: port, Sites: []Site{{Enable: false, Key: "off", Domains: []string{"off.test"}, Type: TypeText}}, Default: dflt}
		}, func(a *authenticator, port int) {
			// 模块会把全部监听中填写的站点域名（含停用站点）登记给认证器
			a.setSiteHosts([]string{"off.test", "*.wild.test", "*", ""})
			a.setCertLookup(func(name string) bool { return name == "cert.test" })
		}, func(port int) []hostCase {
			return []hostCase{
				// 停用站点或其他监听的站点域名、通配域名
				{host: "off.test", ok: true},
				{host: "x.wild.test", ok: true},
				// 认证域名本身
				{host: fmt.Sprintf("auth.test:%d", port), ok: true},
				// 证书模块中有证书的域名
				{host: "cert.test", ok: true},
				// 连接到达的本机地址
				{host: fmt.Sprintf("127.0.0.1:%d", port), ok: true},
				{host: "127.0.0.1", ok: true},
				// 其余域名、其他 IP、本机地址的其他端口一律拒绝
				{host: "evil.test"},
				{host: "wild.test"},
				{host: "6.6.6.6"},
				{host: "127.0.0.1:1"},
				{host: "off.test:1"},
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			addr, port := oauthOriginEnv(t, tc.mk, tc.setup)
			auth := newAuthenticator("secret")
			for _, hc := range tc.hosts(port) {
				c := noRedirect()
				resp := oauthDo(t, c, addr, hc.host, oauthStart+"?p=gh&next=/")
				loc, _ := url.Parse(resp.Header.Get("Location"))
				if !hc.ok {
					if resp.StatusCode != http.StatusBadRequest {
						t.Errorf("Host %q 不应能发起第三方登录: %d %s", hc.host, resp.StatusCode, resp.Header.Get("Location"))
					}
					continue
				}
				if resp.StatusCode != http.StatusFound || loc == nil || loc.Query().Get("state") == "" {
					t.Errorf("Host %q 应能发起第三方登录: %d %s", hc.host, resp.StatusCode, resp.Header.Get("Location"))
					continue
				}
				var st oauthState
				// 状态由运行器中的认证器签名，密钥相同即可解出
				if err := auth.verifyBlob("state", loc.Query().Get("state"), &st); err != nil {
					t.Fatalf("state 无法解析: %v", err)
				}
				want := hc.origin
				if want == "" {
					want = "http://" + strings.ToLower(hc.host)
				}
				if st.Origin != want {
					t.Errorf("Host %q 的来源应为 %s，实际 %s", hc.host, want, st.Origin)
				}
			}
		})
	}
}

// TestOAuthCallbackRejectsBadOrigin 确认回调只把浏览器带回格式正确的站点来源：签名有效但来源异常（旧版本签发、
// 带路径或账号）的 state 不再跳转。
func TestOAuthCallbackRejectsBadOrigin(t *testing.T) {
	addr, port := oauthOriginEnv(t, func(port int) Listener {
		return Listener{ListenIP: "127.0.0.1", Network: "tcp4", Port: port, Default: Site{Enable: true, Type: TypeText, Text: "auth host"}}
	}, nil)
	a := newAuthenticator("secret")
	for _, st := range []oauthState{
		{Provider: "gh", Origin: "http://evil.test", Nonce: "n"},                            // 旧版本签发、未经校验的来源
		{Provider: "gh", Origin: "http://evil.test/x?", Nonce: "n", V: oauthStateVersion},   // 带路径
		{Provider: "gh", Origin: "http://u@app.test", Nonce: "n", V: oauthStateVersion},     // 带账号
		{Provider: "gh", Origin: "javascript:alert(1)//", Nonce: "n", V: oauthStateVersion}, // 非 http(s)
		{Provider: "gh", Origin: "http://app.test:0", Nonce: "n", V: oauthStateVersion},     // 端口无效
		{Provider: "gh", Origin: "http://app.test:99999", Nonce: "n", V: oauthStateVersion}, // 端口无效
	} {
		raw := a.signBlob("state", st, time.Minute)
		resp := oauthDo(t, noRedirect(), addr, fmt.Sprintf("auth.test:%d", port), oauthCallback+"?code=alice&state="+url.QueryEscape(raw))
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("来源 %q（版本 %d）的 state 应被拒绝: %d %s", st.Origin, st.V, resp.StatusCode, resp.Header.Get("Location"))
		}
	}
	// 格式正确的来源照常跳回
	raw := a.signBlob("state", oauthState{Provider: "gh", Origin: "https://app.test:8443", Nonce: "n", V: oauthStateVersion}, time.Minute)
	resp := oauthDo(t, noRedirect(), addr, fmt.Sprintf("auth.test:%d", port), oauthCallback+"?code=alice&state="+url.QueryEscape(raw))
	if resp.StatusCode != http.StatusFound || !strings.HasPrefix(resp.Header.Get("Location"), "https://app.test:8443"+oauthFinish+"?ticket=") {
		t.Fatalf("合法来源应跳回站点: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
}
