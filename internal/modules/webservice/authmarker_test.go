package webservice

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/webprobe"
)

const testMarkerKey = "marker-key-0123456789"

// 标记签名、过期、超长有效期、篡改与密钥不符都要拒绝。
func TestAuthMarkerSignVerify(t *testing.T) {
	key := []byte(testMarkerKey)
	now := time.Now()
	m := signAuthMarker(key, credSession("abc"), now)
	hash, exp, ok := verifyAuthMarker(key, m, now)
	if !ok || hash != credHash(credSession("abc")) || exp != now.Add(authMarkerTTL).Unix() {
		t.Fatalf("有效标记应通过: %v %s %d", ok, hash, exp)
	}
	if _, _, ok := verifyAuthMarker([]byte("other-key-0123456789"), m, now); ok {
		t.Fatal("密钥不同的标记不应通过")
	}
	if _, _, ok := verifyAuthMarker(key, m, now.Add(authMarkerTTL+time.Second)); ok {
		t.Fatal("过期的标记不应通过")
	}
	if _, _, ok := verifyAuthMarker(key, signAuthMarker(key, "x", now.Add(time.Hour)), now); ok {
		t.Fatal("过期时间过远的标记不应通过")
	}
	parts := strings.Split(m, ".")
	parts[2] = credHash(credSession("other"))
	if _, _, ok := verifyAuthMarker(key, strings.Join(parts, "."), now); ok {
		t.Fatal("换了凭据哈希的标记不应通过")
	}
	for _, bad := range []string{"", "v1", "v2.1.2.3", "v1.x." + parts[2] + "." + parts[3], m + ".x"} {
		if _, _, ok := verifyAuthMarker(key, bad, now); ok {
			t.Fatalf("格式错误的标记不应通过: %q", bad)
		}
	}
}

// markerPair 启动双层 Gately：内层负责网页认证并信任已登录用户，外层反代内层、挂 WAF 并信任后端认证标记。
// app 为内层的后端应用。
func markerPair(t *testing.T, app http.Handler, innerKey, outerKey string) (outerAddr string) {
	t.Helper()
	backend := httptest.NewServer(app)
	t.Cleanup(backend.Close)
	innerAddr, inner := startRunner(t, Listener{Key: "in", Sites: []Site{
		{Enable: true, Key: "app", Domains: []string{"*"}, Targets: []string{backend.URL}, WebAuth: true, Users: []string{"alice:pw"}, TrustAuthUsers: true},
	}}, nil, nil)
	inner.auth.marker.set(innerKey)
	outerAddr, outer := startRunnerWAF(t, Listener{Key: "out", Sites: []Site{
		{Enable: true, Key: "edge", Domains: []string{"app.test"}, Targets: []string{"http://" + innerAddr}, PassAuthCookies: true, TrustBackendAuth: true, WAF: "w"},
	}}, blockingWAF{})
	outer.auth.marker.set(outerKey)
	return outerAddr
}

// 双层 Gately：经外层登录内层后，第一个请求照常经过外层 WAF 并拿到内层的标记，之后同一会话的请求跳过外层 WAF；
// 标记头不返回给访客；未登录、伪造会话、两端密钥不一致时外层照常拦截。
func TestAuthMarkerTwoTier(t *testing.T) {
	app := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("app"))
	})
	addr := markerPair(t, app, testMarkerKey, testMarkerKey)
	cookie := loginCookie(t, addr, "app.test", "alice", "pw")
	with := map[string]string{"Cookie": cookie}
	if resp, _ := get(t, noRedirect(), addr, "app.test", "/attack", with); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("还没拿到标记时应照常经过 WAF: %d", resp.StatusCode)
	}
	resp, body := get(t, noRedirect(), addr, "app.test", "/", with)
	if resp.StatusCode != http.StatusOK || body != "app" {
		t.Fatalf("登录后应能访问: %d %q", resp.StatusCode, body)
	}
	if v := resp.Header.Get(authMarkerHeader); v != "" {
		t.Fatalf("标记头不应返回给访客: %q", v)
	}
	if resp, body := get(t, noRedirect(), addr, "app.test", "/attack", with); resp.StatusCode != http.StatusOK || body != "app" {
		t.Fatalf("得到标记后应跳过外层 WAF: %d %q", resp.StatusCode, body)
	}
	if resp, _ := get(t, noRedirect(), addr, "app.test", "/attack", nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("未登录的请求应被外层 WAF 拦截: %d", resp.StatusCode)
	}
	forged := cookie[:len(cookie)-2] + "xx"
	if resp, _ := get(t, noRedirect(), addr, "app.test", "/attack", map[string]string{"Cookie": forged}); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("伪造的会话应被外层 WAF 拦截: %d", resp.StatusCode)
	}

	// 两端密钥不一致：外层不认内层的标记
	addr2 := markerPair(t, app, testMarkerKey, "another-key-0123456789")
	cookie2 := loginCookie(t, addr2, "app.test", "alice", "pw")
	get(t, noRedirect(), addr2, "app.test", "/", map[string]string{"Cookie": cookie2})
	if resp, _ := get(t, noRedirect(), addr2, "app.test", "/attack", map[string]string{"Cookie": cookie2}); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("密钥不一致时应照常经过 WAF: %d", resp.StatusCode)
	}
}

// 后端应用自己写出的标记头：内层先去掉再按自己的判断签发，访客与外层都拿不到应用伪造的标记；
// 后端（不是 Gately 或被攻破）返回的有效签名标记若绑定的不是本次请求的凭据，外层同样不信任。
func TestAuthMarkerForgedByBackend(t *testing.T) {
	key := []byte(testMarkerKey)
	app := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(authMarkerHeader, signAuthMarker(key, credSession("someone-else"), time.Now()))
		_, _ = w.Write([]byte("app"))
	})
	backend := httptest.NewServer(app)
	t.Cleanup(backend.Close)
	// 内层：应用写出的标记被去掉；匿名访问本站点公开路径时不签发
	innerAddr, inner := startRunner(t, Listener{Key: "in", Sites: []Site{
		{Enable: true, Key: "app", Domains: []string{"*"}, Targets: []string{backend.URL}, BasicAuth: true, Users: []string{"alice:pw"}, TrustAuthUsers: true, AuthOptional: true},
	}}, nil, nil)
	inner.auth.marker.set(testMarkerKey)
	if resp, _ := get(t, noRedirect(), innerAddr, "x.test", "/", nil); resp.Header.Get(authMarkerHeader) != "" {
		t.Fatalf("内层应去掉应用写出的标记: %q", resp.Header.Get(authMarkerHeader))
	}

	// 外层直接反代这个应用：标记签名有效，但绑定的是别人的凭据
	outerAddr, outer := startRunnerWAF(t, Listener{Key: "out", Sites: []Site{
		{Enable: true, Key: "edge", Domains: []string{"app.test"}, Targets: []string{backend.URL}, PassAuthCookies: true, TrustBackendAuth: true, WAF: "w"},
	}}, blockingWAF{})
	outer.auth.marker.set(testMarkerKey)
	var mu sync.Mutex
	var hits []string
	webprobe.SetHandler(func(h webprobe.Hit) {
		mu.Lock()
		hits = append(hits, h.Listener+h.Path)
		mu.Unlock()
	})
	t.Cleanup(func() { webprobe.SetHandler(nil) })
	mine := map[string]string{"Cookie": ssoCookie + "=mine"}
	if resp, _ := get(t, noRedirect(), outerAddr, "app.test", "/mine", mine); resp.Header.Get(authMarkerHeader) != "" {
		t.Fatal("外层应去掉标记头")
	}
	// 绑定别人凭据的标记不算本次请求已认证，照常计入路径探测
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		got := strings.Join(hits, ",")
		mu.Unlock()
		if strings.Contains(got, "out/mine") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("绑定别人凭据的请求应照常上报: %s", got)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if resp, _ := get(t, noRedirect(), outerAddr, "app.test", "/attack", mine); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("绑定别人凭据的标记不应让本次请求的凭据跳过 WAF: %d", resp.StatusCode)
	}
	// 拿着别人的凭据才行：说明放行确实绑定在凭据上
	other := map[string]string{"Cookie": ssoCookie + "=someone-else"}
	get(t, noRedirect(), outerAddr, "app.test", "/", other)
	if resp, _ := get(t, noRedirect(), outerAddr, "app.test", "/attack", other); resp.StatusCode != http.StatusOK {
		t.Fatalf("与标记绑定的凭据应跳过 WAF: %d", resp.StatusCode)
	}
}

// 外层：带后端认证标记的响应不计入路径探测（第一个请求即生效），未登录访客照常上报。
func TestAuthMarkerSkipsWebProbe(t *testing.T) {
	var mu sync.Mutex
	var hits []string
	webprobe.SetHandler(func(h webprobe.Hit) {
		mu.Lock()
		hits = append(hits, h.Listener+h.Path)
		mu.Unlock()
	})
	t.Cleanup(func() { webprobe.SetHandler(nil) })
	app := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	addr := markerPair(t, app, testMarkerKey, testMarkerKey)
	cookie := loginCookie(t, addr, "app.test", "alice", "pw")
	get(t, noRedirect(), addr, "app.test", "/.env", map[string]string{"Cookie": cookie})
	get(t, noRedirect(), addr, "app.test", "/anon", nil)
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		got := strings.Join(hits, ",")
		mu.Unlock()
		if strings.Contains(got, "out/anon") || time.Now().After(deadline) {
			if strings.Contains(got, "/.env") || !strings.Contains(got, "out/anon") {
				t.Fatalf("上报不符: %s", got)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// 信任后端认证标记只用于不自己认证的反向代理站点，其余情况保存时关闭。
func TestTrustBackendAuthValidate(t *testing.T) {
	ok := Site{Type: TypeProxy, Targets: []string{"http://127.0.0.1:1"}, TrustBackendAuth: true}
	if err := ok.validateAccess(); err != nil || !ok.TrustBackendAuth {
		t.Fatalf("反向代理站点应保留: %v %v", err, ok.TrustBackendAuth)
	}
	for _, s := range []Site{
		{Type: TypeText, TrustBackendAuth: true},
		{Type: TypeProxy, TrustBackendAuth: true, WebAuth: true, Users: []string{"a:b"}},
		{Type: TypeProxy, TrustBackendAuth: true, ForwardAuth: ForwardAuth{URL: "http://auth.test"}},
	} {
		if err := s.validateAccess(); err != nil || s.TrustBackendAuth {
			t.Fatalf("应关闭信任后端认证标记: %+v %v", s, err)
		}
	}
}

// 外层记住的凭据数有上限。
func TestBackendTrustBound(t *testing.T) {
	var b backendTrust
	now := time.Now().Unix()
	for i := 0; i < backendTrustMax+5; i++ {
		b.remember(credHash(strings.Repeat("x", i)), now+60, now)
	}
	if n := len(b.m); n == 0 || n > backendTrustMax {
		t.Fatalf("条数应在上限内: %d", n)
	}
}

// WebSocket 升级的 101 响应不经过 WriteHeader：后端写出的标记同样要在接管连接前去掉，不能传给访客。
func TestAuthMarkerStrippedOnUpgrade(t *testing.T) {
	key := []byte(testMarkerKey)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, brw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = brw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n" +
			authMarkerHeader + ": " + signAuthMarker(key, credSession("x"), time.Now()) + "\r\n\r\n")
		_ = brw.Flush()
	}))
	t.Cleanup(backend.Close)
	for _, site := range []Site{
		// 外层：信任后端标记的反代站点
		{Enable: true, Key: "edge", Domains: []string{"ws.test"}, Targets: []string{backend.URL}, TrustBackendAuth: true},
		// 内层：信任已登录用户的站点，应用写出的标记要去掉
		{Enable: true, Key: "app", Domains: []string{"ws.test"}, Targets: []string{backend.URL}, BasicAuth: true, Users: []string{"alice:pw"}, TrustAuthUsers: true, AuthOptional: true},
	} {
		addr, run := startRunner(t, Listener{Key: site.Key, Sites: []Site{site}}, nil, nil)
		run.auth.marker.set(testMarkerKey)
		c, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(c, "GET /ws HTTP/1.1\r\nHost: ws.test\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
		resp, err := http.ReadResponse(bufio.NewReader(c), nil)
		c.Close()
		if err != nil || resp.StatusCode != http.StatusSwitchingProtocols {
			t.Fatalf("%s 升级失败: %v %v", site.Key, resp, err)
		}
		if v := resp.Header.Get(authMarkerHeader); v != "" {
			t.Fatalf("%s 的 101 响应不应带后端写出的标记: %q", site.Key, v)
		}
	}
}
