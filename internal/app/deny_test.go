package app

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/httpx"
)

// putSettings 读取当前设置，经 edit 修改后保存，返回状态码与响应。
func putSettings(t *testing.T, c *http.Client, base string, edit func(*settingsView)) (int, []byte) {
	t.Helper()
	_, body := doJSON(t, c, "GET", base+"/api/system/settings", nil)
	var s settingsView
	if err := json.Unmarshal(body, &s); err != nil {
		t.Fatal(err)
	}
	edit(&s)
	return doJSON(t, c, "PUT", base+"/api/system/settings", s)
}

// shortHold 缩短挂起时长，测试结束后恢复。
func shortHold(t *testing.T, d time.Duration) {
	t.Helper()
	old := denyHoldTimeout
	denyHoldTimeout = d
	t.Cleanup(func() { denyHoldTimeout = old })
}

// denyResult 是一次请求的结果：连接被中止时 err 非空。
type denyResult struct {
	code    int
	body    string
	header  http.Header
	err     error
	elapsed time.Duration
}

// probe 以新的浏览器（无 Cookie）请求 path；fromIP 非空时经本机代理头伪装为该来源。
func probe(t *testing.T, base, method, path, fromIP string, timeout time.Duration) denyResult {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		// 每次新建连接，避免复用到被中止的连接
		Transport: &http.Transport{DisableKeepAlives: true}}
	req, _ := http.NewRequest(method, base+path, nil)
	req.Header.Set("Accept", "text/html")
	// 显式声明压缩后 Transport 不再透明解压，可以检查拦截响应是否被压缩
	req.Header.Set("Accept-Encoding", "br, gzip")
	if fromIP != "" {
		req.Header.Set("X-Real-IP", fromIP)
	}
	start := time.Now()
	resp, err := c.Do(req)
	r := denyResult{err: err, elapsed: time.Since(start)}
	if err != nil {
		return r
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	r.code, r.body, r.header = resp.StatusCode, string(b), resp.Header
	return r
}

// noFingerprint 断言响应中不含产品特征。
func noFingerprint(t *testing.T, label string, r denyResult) {
	t.Helper()
	all := strings.ToLower(r.body)
	for k, v := range r.header {
		all += "\n" + strings.ToLower(k) + ": " + strings.ToLower(strings.Join(v, ","))
	}
	for _, w := range []string{"gately", "安全入口", "entry", "safepath"} {
		if strings.Contains(all, w) {
			t.Fatalf("%s: 响应含特征 %q:\n%s", label, w, all)
		}
	}
	if r.header.Get("Set-Cookie") != "" {
		t.Fatalf("%s: 拦截响应不应设置 Cookie", label)
	}
	// 管理后台统一的安全响应头与压缩都是可识别的特征
	for _, h := range []string{"X-Frame-Options", "Referrer-Policy", "X-Content-Type-Options", "Content-Encoding", "Vary"} {
		if v := r.header.Get(h); v != "" {
			t.Fatalf("%s: 拦截响应不应带 %s: %s", label, h, v)
		}
	}
}

func TestDenySettingsValidate(t *testing.T) {
	_, srv, c := loginApp(t)
	cases := []struct {
		name string
		edit func(*settingsView)
		ok   bool
	}{
		{"未知方式", func(s *settingsView) { s.DenyMode = "rst" }, false},
		{"大小写不同", func(s *settingsView) { s.DenyMode = "Close" }, true},
		{"状态码不在列表", func(s *settingsView) { s.DenyMode = denyModeCustom; s.DenyStatus = 302 }, false},
		{"状态码越界", func(s *settingsView) { s.DenyMode = denyModeCustom; s.DenyStatus = 1000 }, false},
		{"页面过大", func(s *settingsView) { s.DenyMode = denyModeCustom; s.DenyPage = strings.Repeat("a", maxDenyPage+1) }, false},
		// 非自定义方式下页面内容同样受大小限制，不能借此写入超大配置
		{"非自定义方式页面过大", func(s *settingsView) { s.DenyMode = denyModeBlank; s.DenyPage = strings.Repeat("a", maxDenyPage+1) }, false},
		{"页面恰好上限", func(s *settingsView) { s.DenyMode = denyModeCustom; s.DenyPage = strings.Repeat("a", maxDenyPage) }, true},
		{"前后空格", func(s *settingsView) { s.DenyMode = " drop " }, true},
		{"状态码缺省", func(s *settingsView) { s.DenyMode = denyModeCustom; s.DenyStatus = 0 }, true},
	}
	for _, tc := range cases {
		code, body := putSettings(t, c, srv.URL, tc.edit)
		if (code == 200) != tc.ok {
			t.Fatalf("%s: 期望 ok=%v，实际 %d %s", tc.name, tc.ok, code, body)
		}
	}
	_, body := doJSON(t, c, "GET", srv.URL+"/api/system/settings", nil)
	var s settingsView
	_ = json.Unmarshal(body, &s)
	if s.DenyMode != denyModeCustom || s.DenyStatus != http.StatusOK {
		t.Fatalf("状态码缺省应为 200: %+v", s.Settings)
	}
	if code, _ := putSettings(t, c, srv.URL, func(s *settingsView) { s.DenyMode = " drop " }); code != 200 {
		t.Fatal(code)
	}
	_, body = doJSON(t, c, "GET", srv.URL+"/api/system/settings", nil)
	_ = json.Unmarshal(body, &s)
	if s.DenyMode != denyModeDrop {
		t.Fatalf("方式应去除空格: %q", s.DenyMode)
	}
}

// TestDenyModes 覆盖每种响应方式在三种拦截（安全入口、仅内网访问、来源被封禁）下的表现，
// 以及放行的请求（通过入口、内网来源、有效令牌）不受影响。
func TestDenyModes(t *testing.T) {
	shortHold(t, 400*time.Millisecond)
	_, srv, c := loginApp(t)
	const page = `<!doctype html><title>Welcome</title><h1>It works!</h1><script>parent.x=1</script>`
	// 只读状态令牌，用来确认有效令牌不受安全入口影响
	_, body := doJSON(t, c, "POST", srv.URL+"/api/tokens", map[string]any{"name": "t", "scopes": []string{"status:read"}})
	var tok struct {
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(body, &tok)

	const banned = "198.51.100.23"
	if _, _, err := banlist.Default.Ban(banned, banlist.Options{Duration: time.Minute, Force: true}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { banlist.Default.Unban(banned) })

	type trigger struct {
		name    string
		path    string
		from    string
		safe    string
		lanOnly bool
	}
	triggers := []trigger{
		{"安全入口首页", "/", "", "/entry-x1", false},
		{"安全入口接口", "/api/login", "", "/entry-x1", false},
		{"安全入口错误令牌", "/api/system/info?token=gtl_bad", "", "/entry-x1", false},
		{"仅内网首页", "/", "203.0.113.9", "", true},
		{"仅内网接口", "/api/status", "203.0.113.9", "", true},
		// 仅内网 + 安全入口：先于安全入口拦截，入口路径本身也不放行公网来源
		{"仅内网访问入口路径", "/entry-x1", "203.0.113.9", "/entry-x1", true},
		{"封禁来源", "/", banned, "", false},
	}
	modes := []string{denyModeDefault, denyModeNotFound, denyModeBlank, denyModeClose, denyModeDrop, denyModeCustom}
	for _, mode := range modes {
		for _, tg := range triggers {
			label := "方式 " + mode + " / " + tg.name
			if code, body := putSettings(t, c, srv.URL, func(s *settingsView) {
				s.DenyMode, s.DenyStatus, s.DenyPage = mode, http.StatusServiceUnavailable, page
				s.SafePath, s.LANOnly = tg.safe, tg.lanOnly
			}); code != 200 {
				t.Fatalf("%s: 保存失败 %d %s", label, code, body)
			}
			r := probe(t, srv.URL, "GET", tg.path, tg.from, 3*time.Second)
			switch mode {
			case denyModeDefault:
				if tg.safe != "" && !tg.lanOnly {
					// 提示页是 HTML（正文已压缩，按类型判断），不是纯文本 404
					if r.code != 404 || !strings.HasPrefix(r.header.Get("Content-Type"), "text/html") {
						t.Fatalf("%s: 默认应为 404 提示页，实际 %d %v", label, r.code, r.err)
					}
				} else if r.code != 403 || !strings.HasPrefix(r.header.Get("Content-Type"), "text/plain") {
					t.Fatalf("%s: 默认应为 403，实际 %d %q %v", label, r.code, r.body, r.err)
				}
			case denyModeNotFound:
				if r.code != 404 || r.body != notFoundPage {
					t.Fatalf("%s: 应为普通 404，实际 %d %q %v", label, r.code, r.body, r.err)
				}
				noFingerprint(t, label, r)
			case denyModeBlank:
				if r.code != 200 || r.body != "" {
					t.Fatalf("%s: 应为空白页，实际 %d %q %v", label, r.code, r.body, r.err)
				}
				noFingerprint(t, label, r)
			case denyModeClose, denyModeDrop:
				if tg.from != "" {
					// 经本机代理头转发的请求按普通 404 处理，避免前置代理返回自己的 502 页面
					if r.code != 404 || r.body != notFoundPage {
						t.Fatalf("%s: 经代理转发时应降级为普通 404，实际 %d %q %v", label, r.code, r.body, r.err)
					}
					noFingerprint(t, label, r)
					continue
				}
				if r.err == nil {
					t.Fatalf("%s: 不应有响应，实际返回 %d", label, r.code)
				}
				if mode == denyModeClose && r.elapsed > time.Second {
					t.Fatalf("%s: 断开应立即发生，用时 %v", label, r.elapsed)
				}
				if mode == denyModeDrop && r.elapsed < 350*time.Millisecond {
					t.Fatalf("%s: 应挂起到超时，只用了 %v", label, r.elapsed)
				}
			case denyModeCustom:
				if r.code != 503 || r.body != page {
					t.Fatalf("%s: 应返回自定义页面，实际 %d %q %v", label, r.code, r.body, r.err)
				}
				csp := r.header.Get("Content-Security-Policy")
				if !strings.HasPrefix(csp, "sandbox;") || !strings.Contains(csp, "default-src 'none'") {
					t.Fatalf("%s: 自定义页面必须在沙箱中、禁止脚本: %q", label, csp)
				}
				if !strings.HasPrefix(r.header.Get("Content-Type"), "text/html") {
					t.Fatalf("%s: Content-Type %q", label, r.header.Get("Content-Type"))
				}
				noFingerprint(t, label, r)
			}
		}

		// 放行的请求不受响应方式影响
		label := "方式 " + mode
		if code, _ := putSettings(t, c, srv.URL, func(s *settingsView) {
			s.DenyMode, s.SafePath, s.LANOnly = mode, "/entry-x1", true
		}); code != 200 {
			t.Fatal(code)
		}
		jar, _ := cookiejar.New(nil)
		fresh := &http.Client{Jar: jar, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		if code, _ := doJSON(t, fresh, "GET", srv.URL+"/entry-x1", nil); code != 302 {
			t.Fatalf("%s: 内网访问入口应跳转，实际 %d", label, code)
		}
		if code, body := doJSON(t, fresh, "POST", srv.URL+"/api/login", map[string]string{"username": "admin", "password": "test-password"}); code != 200 {
			t.Fatalf("%s: 通过入口后应能登录，实际 %d %s", label, code, body)
		}
		req, _ := http.NewRequest("GET", srv.URL+"/api/system/info", nil)
		req.Header.Set("Authorization", "Bearer "+tok.Secret)
		resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
		if err != nil {
			t.Fatalf("%s: 有效令牌请求失败: %v", label, err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("%s: 有效令牌应放行，实际 %d", label, resp.StatusCode)
		}
		// 已登录会话（修改设置的这个客户端）不受影响
		if code, _ := doJSON(t, c, "GET", srv.URL+"/api/system/settings", nil); code != 200 {
			t.Fatalf("%s: 已登录会话应放行，实际 %d", label, code)
		}
	}
}

// TestDenyHeadRequest 确认 HEAD 请求同样按方式处理，且不返回正文。
func TestDenyHeadRequest(t *testing.T) {
	_, srv, c := loginApp(t)
	if code, _ := putSettings(t, c, srv.URL, func(s *settingsView) {
		s.DenyMode, s.DenyStatus, s.DenyPage, s.SafePath = denyModeCustom, 404, "<p>nope</p>", "/entry-h"
	}); code != 200 {
		t.Fatal(code)
	}
	r := probe(t, srv.URL, "HEAD", "/", "", 3*time.Second)
	if r.err != nil || r.code != 404 || r.body != "" {
		t.Fatalf("HEAD 应返回 404 且无正文: %d %q %v", r.code, r.body, r.err)
	}
}

// TestDenyCustomBadStatusFromFile 模拟手工改坏配置文件：状态码无效时按 200 返回，不能让处理器崩溃。
func TestDenyCustomBadStatusFromFile(t *testing.T) {
	a, srv, _ := loginApp(t)
	a.settingsMu.Lock()
	a.settings.SafePath, a.settings.DenyMode, a.settings.DenyStatus, a.settings.DenyPage = "/entry-b", denyModeCustom, 99, "ok"
	a.settingsMu.Unlock()
	r := probe(t, srv.URL, "GET", "/", "", 3*time.Second)
	if r.err != nil || r.code != 200 || r.body != "ok" {
		t.Fatalf("无效状态码应按 200 返回: %d %q %v", r.code, r.body, r.err)
	}
	// 未知方式按默认处理（安全入口提示页）
	a.settingsMu.Lock()
	a.settings.DenyMode = "weird"
	a.settingsMu.Unlock()
	if r := probe(t, srv.URL, "GET", "/", "", 3*time.Second); r.code != 404 || !strings.HasPrefix(r.header.Get("Content-Type"), "text/html") {
		t.Fatalf("未知方式应按默认处理: %d %v", r.code, r.err)
	}
}

// TestDenyDropSlotsFull 确认挂起名额用尽时立即断开，挂起不会无限累积；服务停止时挂起的请求立即结束。
func TestDenyDropSlotsFull(t *testing.T) {
	shortHold(t, 5*time.Second)
	a, srv, c := loginApp(t)
	if code, _ := putSettings(t, c, srv.URL, func(s *settingsView) { s.DenyMode, s.SafePath = denyModeDrop, "/entry-d" }); code != 200 {
		t.Fatal(code)
	}
	// 占满所有名额
	n := 0
	for len(denyHoldSlots) < cap(denyHoldSlots) {
		denyHoldSlots <- struct{}{}
		n++
	}
	r := probe(t, srv.URL, "GET", "/", "", 3*time.Second)
	for range n {
		<-denyHoldSlots
	}
	if r.err == nil || r.elapsed > time.Second {
		t.Fatalf("名额用尽时应立即断开: %v %v", r.err, r.elapsed)
	}

	done := make(chan denyResult, 1)
	go func() {
		done <- probe(t, srv.URL, "GET", "/", "", 4*time.Second)
	}()
	time.Sleep(200 * time.Millisecond)
	a.holdCancel()
	select {
	case r := <-done:
		if r.err == nil || r.elapsed > 2*time.Second {
			t.Fatalf("服务停止时挂起应立即结束: %v %v", r.err, r.elapsed)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("服务停止后挂起的请求没有结束")
	}
}

// fakeRemote 把真实连接的对端地址伪装成指定地址，用于测试接受连接时的判断。
type fakeRemote struct {
	net.Conn
	remote net.Addr
}

func (f fakeRemote) RemoteAddr() net.Addr { return f.remote }

// tcpPair 返回一对已连接的 TCP 连接（server 端、client 端）。
func tcpPair(t *testing.T) (*net.TCPConn, net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	server, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		client.Close()
		server.Close()
	})
	return server.(*net.TCPConn), client
}

func TestDenyConnAtAccept(t *testing.T) {
	shortHold(t, 400*time.Millisecond)
	a, _, _ := loginApp(t)
	public := &net.TCPAddr{IP: net.ParseIP("203.0.113.50"), Port: 40000}
	lan := &net.TCPAddr{IP: net.ParseIP("192.168.1.20"), Port: 40000}
	const banned = "198.51.100.77"
	if _, _, err := banlist.Default.Ban(banned, banlist.Options{Duration: time.Minute, Force: true}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { banlist.Default.Unban(banned) })
	set := func(mode string, lanOnly bool, proxies ...string) {
		a.settingsMu.Lock()
		a.settings.DenyMode, a.settings.LANOnly, a.settings.TrustedProxies = mode, lanOnly, proxies
		a.settingsMu.Unlock()
		a.applyRuntime()
	}
	t.Cleanup(func() { httpx.SetProxyTrust(true, nil) })

	cases := []struct {
		name    string
		mode    string
		lanOnly bool
		proxies []string
		remote  net.Addr
		deny    bool
	}{
		{"断开+仅内网+公网", denyModeClose, true, nil, public, true},
		{"不响应+仅内网+公网", denyModeDrop, true, nil, public, true},
		{"断开+仅内网+内网", denyModeClose, true, nil, lan, false},
		{"断开+未开仅内网+公网", denyModeClose, false, nil, public, false},
		{"断开+封禁来源", denyModeClose, false, nil, &net.TCPAddr{IP: net.ParseIP(banned), Port: 1}, true},
		{"断开+封禁 IPv4 映射地址", denyModeClose, false, nil, &net.TCPAddr{IP: net.ParseIP("::ffff:" + banned), Port: 1}, true},
		// 公网可信代理后面可能是内网用户，要交给 HTTP 层按请求头判断
		{"断开+仅内网+可信代理", denyModeClose, true, []string{"203.0.113.0/24"}, public, false},
		{"默认方式", denyModeDefault, true, nil, public, false},
		{"普通 404", denyModeNotFound, true, nil, public, false},
		{"自定义页面", denyModeCustom, true, nil, public, false},
	}
	for _, tc := range cases {
		set(tc.mode, tc.lanOnly, tc.proxies...)
		server, client := tcpPair(t)
		got := a.denyConn(fakeRemote{Conn: server, remote: tc.remote})
		if got != tc.deny {
			t.Fatalf("%s: 期望 deny=%v，实际 %v", tc.name, tc.deny, got)
		}
		if !got {
			continue
		}
		_, _ = client.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n"))
		_ = client.SetReadDeadline(time.Now().Add(3 * time.Second))
		start := time.Now()
		n, err := client.Read(make([]byte, 64))
		elapsed := time.Since(start)
		if n != 0 || err == nil {
			t.Fatalf("%s: 不应收到任何数据: n=%d err=%v", tc.name, n, err)
		}
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			t.Fatalf("%s: 连接应被关闭，实际读超时", tc.name)
		}
		if tc.mode == denyModeClose && elapsed > time.Second {
			t.Fatalf("%s: 应立即断开，用时 %v", tc.name, elapsed)
		}
		if tc.mode == denyModeDrop && elapsed < 350*time.Millisecond {
			t.Fatalf("%s: 应挂起到超时，只用了 %v", tc.name, elapsed)
		}
	}
}

// TestDenyListenerHoldLimits 覆盖接受连接时挂起的上限：名额满时直接断开、读满上限后断开、服务停止时立即结束。
func TestDenyListenerHoldLimits(t *testing.T) {
	shortHold(t, 10*time.Second)
	a, _, _ := loginApp(t)
	a.settingsMu.Lock()
	a.settings.DenyMode, a.settings.LANOnly = denyModeDrop, true
	a.settingsMu.Unlock()
	public := &net.TCPAddr{IP: net.ParseIP("203.0.113.51"), Port: 1}
	readEnd := func(c net.Conn) (time.Duration, error) {
		_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
		start := time.Now()
		_, err := c.Read(make([]byte, 8))
		return time.Since(start), err
	}

	// 名额满
	n := 0
	for len(denyHoldSlots) < cap(denyHoldSlots) {
		denyHoldSlots <- struct{}{}
		n++
	}
	server, client := tcpPair(t)
	if !a.denyConn(fakeRemote{Conn: server, remote: public}) {
		t.Fatal("应拒绝")
	}
	for range n {
		<-denyHoldSlots
	}
	if d, err := readEnd(client); err == nil || d > time.Second {
		t.Fatalf("名额满时应立即断开: %v %v", d, err)
	}

	// 发送超过上限的数据后断开
	server, client = tcpPair(t)
	a.denyConn(fakeRemote{Conn: server, remote: public})
	go func() {
		_, _ = client.Write(make([]byte, denyHoldReadLimit+1024))
	}()
	if d, err := readEnd(client); err == nil || d > 3*time.Second {
		t.Fatalf("读满上限应断开: %v %v", d, err)
	}

	// 服务停止
	server, client = tcpPair(t)
	a.denyConn(fakeRemote{Conn: server, remote: public})
	time.AfterFunc(200*time.Millisecond, a.holdCancel)
	if d, err := readEnd(client); err == nil || d > 3*time.Second {
		t.Fatalf("服务停止时应结束挂起: %v %v", d, err)
	}
	// 名额全部归还
	deadline := time.Now().Add(2 * time.Second)
	for len(denyHoldSlots) != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if len(denyHoldSlots) != 0 {
		t.Fatalf("挂起结束后名额应归还，剩余 %d", len(denyHoldSlots))
	}
}

// TestDenyListenerRealAccept 用真实监听器确认包装后本机连接照常交给 HTTP 服务。
func TestDenyListenerRealAccept(t *testing.T) {
	a, _, _ := loginApp(t)
	a.settingsMu.Lock()
	a.settings.DenyMode, a.settings.LANOnly = denyModeClose, true
	a.settingsMu.Unlock()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: a.Handler()}
	go srv.Serve(&denyListener{Listener: ln, a: a})
	t.Cleanup(func() { srv.Close() })
	resp, err := http.Get("http://" + ln.Addr().String() + "/api/system/onboarding")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("本机连接应照常处理（未登录 401），实际 %d", resp.StatusCode)
	}
	// 本机代理转发的公网来源在 HTTP 层处理，断开方式降级为普通 404，避免代理返回自己的错误页
	req, _ := http.NewRequest("GET", "http://"+ln.Addr().String()+"/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.60")
	resp, err = (&http.Client{Transport: &http.Transport{DisableKeepAlives: true}}).Do(req)
	if err != nil {
		t.Fatalf("代理转发的请求应得到 404: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("代理转发的请求应得到 404，实际 %d", resp.StatusCode)
	}
	// 本机直连（不经代理）未通过安全入口时照常断开
	a.settingsMu.Lock()
	a.settings.LANOnly, a.settings.SafePath = false, "/entry-r"
	a.settingsMu.Unlock()
	if _, err := (&http.Client{Transport: &http.Transport{DisableKeepAlives: true}}).Get("http://" + ln.Addr().String() + "/"); err == nil {
		t.Fatal("直连且未通过入口应被断开")
	}
}

// TestDenyHTTP2 确认 HTTPS 管理端口协商 HTTP/2 时各方式同样生效：断开与不响应在 HTTP/2 下重置请求的流。
func TestDenyHTTP2(t *testing.T) {
	shortHold(t, 400*time.Millisecond)
	a, _, _ := loginApp(t)
	srv := httptest.NewUnstartedServer(a.Handler())
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	client := srv.Client()
	client.Timeout = 3 * time.Second
	for _, mode := range []string{denyModeNotFound, denyModeClose, denyModeDrop, denyModeCustom} {
		a.settingsMu.Lock()
		a.settings.SafePath, a.settings.DenyMode, a.settings.DenyStatus, a.settings.DenyPage = "/entry-h2", mode, 404, "<p>x</p>"
		a.settingsMu.Unlock()
		start := time.Now()
		resp, err := client.Get(srv.URL + "/")
		elapsed := time.Since(start)
		switch mode {
		case denyModeClose, denyModeDrop:
			if err == nil {
				resp.Body.Close()
				t.Fatalf("%s: HTTP/2 下不应有响应，实际 %d", mode, resp.StatusCode)
			}
			if mode == denyModeDrop && elapsed < 350*time.Millisecond {
				t.Fatalf("drop: 应挂起到超时，只用了 %v", elapsed)
			}
		default:
			if err != nil {
				t.Fatalf("%s: %v", mode, err)
			}
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.ProtoMajor != 2 || resp.StatusCode != 404 || len(b) == 0 {
				t.Fatalf("%s: proto=%d code=%d body=%q", mode, resp.ProtoMajor, resp.StatusCode, b)
			}
		}
	}
	// 断开只重置被拦截的流，同一 HTTP/2 连接上的后续请求不受影响
	a.settingsMu.Lock()
	a.settings.SafePath, a.settings.DenyMode = "/entry-h2", denyModeClose
	a.settingsMu.Unlock()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if resp, err := client.Get(srv.URL + "/entry-h2"); err != nil {
		t.Fatalf("同一连接访问入口失败: %v", err)
	} else {
		resp.Body.Close()
		if resp.StatusCode != http.StatusFound || resp.ProtoMajor != 2 {
			t.Fatalf("入口应放行，实际 %d", resp.StatusCode)
		}
	}
}

// TestDenyHoldPerConn 确认一条 HTTP/2 连接同时最多挂起一个请求，其余流立即重置，不能靠一条连接占满全部名额。
func TestDenyHoldPerConn(t *testing.T) {
	shortHold(t, 1500*time.Millisecond)
	a, _, _ := loginApp(t)
	a.settingsMu.Lock()
	a.settings.SafePath, a.settings.DenyMode = "/entry-pc", denyModeDrop
	a.settingsMu.Unlock()
	srv := httptest.NewUnstartedServer(a.Handler())
	srv.Config.ConnContext = denyConnContext
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	client := srv.Client()
	client.Timeout = 5 * time.Second
	// 先建立连接，保证后续请求复用同一条 HTTP/2 连接
	if resp, err := client.Get(srv.URL + "/entry-pc"); err == nil {
		resp.Body.Close()
	}
	first := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		_, _ = client.Get(srv.URL + "/")
		first <- time.Since(start)
	}()
	time.Sleep(300 * time.Millisecond)
	if len(denyHoldSlots) != 1 {
		t.Fatalf("第一个请求应占用 1 个挂起名额，实际 %d", len(denyHoldSlots))
	}
	start := time.Now()
	_, err := client.Get(srv.URL + "/")
	if err == nil || time.Since(start) > 700*time.Millisecond {
		t.Fatalf("同一连接上的第二个请求应立即重置: %v %v", err, time.Since(start))
	}
	if d := <-first; d < 1200*time.Millisecond {
		t.Fatalf("第一个请求应挂起到超时，只用了 %v", d)
	}
}

// fakeCerts 让管理界面认为已有证书，用于测试强制 HTTPS。
type fakeCerts struct{}

func (fakeCerts) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return nil, errors.New("none")
}
func (fakeCerts) HasCertificate(string) bool { return true }

// TestDenyBeforeForceHTTPS 确认隐藏方式下未越过安全入口的请求不会收到强制 HTTPS 跳转，入口路径与默认方式保持原样。
func TestDenyBeforeForceHTTPS(t *testing.T) {
	a, srv, _ := loginApp(t)
	a.opts.Certs = fakeCerts{}
	a.httpsReady.Store(true)
	set := func(mode string) {
		a.settingsMu.Lock()
		a.settings.SafePath, a.settings.DenyMode, a.settings.ForceHTTPS, a.settings.HTTPSListen = "/entry-fh", mode, true, ":16443"
		a.settingsMu.Unlock()
	}
	set(denyModeNotFound)
	if r := probe(t, srv.URL, "GET", "/", "", 3*time.Second); r.code != 404 || r.body != notFoundPage {
		t.Fatalf("隐藏方式下未通过入口应直接 404，实际 %d %q", r.code, r.header.Get("Location"))
	}
	if r := probe(t, srv.URL, "GET", "/entry-fh", "", 3*time.Second); r.code != 302 || !strings.HasPrefix(r.header.Get("Location"), "https://") {
		t.Fatalf("入口路径应跳转到 HTTPS，实际 %d %q", r.code, r.header.Get("Location"))
	}
	set(denyModeDefault)
	if r := probe(t, srv.URL, "GET", "/", "", 3*time.Second); r.code != 302 {
		t.Fatalf("默认方式保持原有行为（先跳转 HTTPS），实际 %d", r.code)
	}
}

// TestDenyNormalizeLegacy 确认手工写成大写的方式与非自定义方式下的无效状态码在保存时被规范化，不阻止保存其他设置。
func TestDenyNormalizeLegacy(t *testing.T) {
	s := Settings{DenyMode: " Close ", DenyStatus: 302}
	if err := s.Validate(); err != nil {
		t.Fatalf("应规范化后通过: %v", err)
	}
	if s.DenyMode != denyModeClose || s.DenyStatus != http.StatusOK {
		t.Fatalf("规范化结果 %q %d", s.DenyMode, s.DenyStatus)
	}
	s = Settings{DenyMode: denyModeCustom, DenyStatus: 302}
	if err := s.Validate(); err == nil {
		t.Fatal("自定义页面的无效状态码应被拒绝")
	}
}
