package app

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"

	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/portforward"
	"github.com/wangwanjie/gately/internal/modules/webterminal"
)

func TestPrefixCookiePath(t *testing.T) {
	cases := map[string]string{
		"gately_token=abc; Path=/; HttpOnly":                   "gately_token=abc; Path=/app/gately; HttpOnly",
		"n=1; Path=/api/login/oauth; Max-Age=600":              "n=1; Path=/app/gately/api/login/oauth; Max-Age=600",
		"Path=/; Path=/":                                       "Path=/; Path=/app/gately",
		"x=1; HttpOnly":                                        "x=1; HttpOnly",
		"x=1; path=/; Secure":                                  "x=1; Path=/app/gately; Secure",
		"x=1; Domain=example.com; Path=relative; SameSite=Lax": "x=1; Domain=example.com; Path=relative; SameSite=Lax",
	}
	for in, want := range cases {
		if got := prefixCookiePath(in, "/app/gately"); got != want {
			t.Errorf("prefixCookiePath(%q) = %q，期望 %q", in, got, want)
		}
	}
}

func TestNormalizeGatewayPrefix(t *testing.T) {
	for in, want := range map[string]string{"": "", "/": "", "/app/gately/": "/app/gately", " /x ": "/x"} {
		got, err := normalizeGatewayPrefix(in)
		if err != nil || got != want {
			t.Errorf("normalizeGatewayPrefix(%q) = %q, %v，期望 %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"app", "/a?b", `/a"b`} {
		if _, err := normalizeGatewayPrefix(bad); err == nil {
			t.Errorf("normalizeGatewayPrefix(%q) 应报错", bad)
		}
	}
}

func TestListenUnix(t *testing.T) {
	// macOS 的 Socket 路径上限较短，使用短目录
	dir, err := os.MkdirTemp("", "gw")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "g.sock")
	ln, err := listenUnix(path)
	if err != nil {
		t.Fatal(err)
	}
	// 模拟异常退出遗留的 Socket 文件：关闭时不删除
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	_ = ln.Close()
	ln, err = listenUnix(path)
	if err != nil {
		t.Fatalf("遗留的 Socket 文件应被替换: %v", err)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o660 {
		t.Fatalf("Socket 权限应为 0660，实际 %v", fi.Mode().Perm())
	}
	_ = ln.Close()

	regular := filepath.Join(dir, "file")
	_ = os.WriteFile(regular, []byte("x"), 0o600)
	if _, err := listenUnix(regular); err == nil {
		t.Fatal("同名普通文件不应被覆盖")
	}
}

// newGatewayApp 创建带最小前端文件的实例，返回实例、直接访问的服务和经网关访问的服务。
func newGatewayApp(t *testing.T) (*App, *httptest.Server, *httptest.Server) {
	t.Helper()
	web := fstest.MapFS{
		"index.html":  {Data: []byte(`<!doctype html><html><head><title>Gately</title><script type="module" src="./assets/a.js"></script></head><body></body></html>`)},
		"assets/a.js": {Data: []byte("console.log(1)")},
		"favicon.svg": {Data: []byte("<svg/>")},
	}
	a, err := New(Options{ConfigDir: t.TempDir(), Web: web, Modules: []module.Module{portforward.New()}})
	if err != nil {
		t.Fatal(err)
	}
	a.logger = log.New(io.Discard, "", 0)
	if err := a.auth.ChangeCredentials(mustInitialPassword(t, a), "admin", "test-password"); err != nil {
		t.Fatal(err)
	}
	handler := a.Handler()
	direct := httptest.NewServer(handler)
	t.Cleanup(direct.Close)
	gw := httptest.NewServer(a.gatewayHandler("/app/gately", handler))
	t.Cleanup(gw.Close)
	return a, direct, gw
}

// gatewayRequest 模拟 fnOS 网关转发的请求：Host 不带端口，浏览器的 Origin 带端口。
func gatewayRequest(t *testing.T, c *http.Client, method, url, body, realIP string) *http.Response {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, url, r)
	req.Host = "nas.example.com"
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "https://nas.example.com:4433")
	}
	if realIP != "" {
		req.Header.Set("X-Real-IP", realIP)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestGatewayServesPrefixedUI(t *testing.T) {
	a, direct, gw := newGatewayApp(t)
	// 开启安全入口：直接访问需要先经过入口，经网关访问不受影响
	a.settingsMu.Lock()
	a.settings.SafePath = "/entry"
	a.settingsMu.Unlock()

	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	resp := gatewayRequest(t, c, "GET", gw.URL+"/app/gately", "", "")
	if resp.StatusCode != http.StatusMovedPermanently || resp.Header.Get("Location") != "/app/gately/" {
		t.Fatalf("前缀不带斜杠时应跳转到 /app/gately/，实际 %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}

	resp = gatewayRequest(t, c, "GET", gw.URL+"/app/gately/", "", "")
	page, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(string(page), `<meta name="gately-base" content="/app/gately" />`) {
		t.Fatalf("经网关打开首页应注入路径前缀，实际 %d %s", resp.StatusCode, page)
	}
	if !strings.Contains(string(page), `src="/app/gately/assets/a.js"`) {
		t.Fatalf("资源路径应改写为带前缀的绝对路径: %s", page)
	}
	if resp = gatewayRequest(t, c, "GET", gw.URL+"/app/gately/assets/a.js", "", ""); resp.StatusCode != 200 {
		t.Fatalf("静态资源应可访问，实际 %d", resp.StatusCode)
	}

	// 直接访问的行为不变：仍需安全入口，首页不注入前缀，资源路径与以往一致
	resp, err := http.Get(direct.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("直接访问未经过安全入口应返回 404，实际 %d", resp.StatusCode)
	}
	a.settingsMu.Lock()
	a.settings.SafePath = ""
	a.settingsMu.Unlock()
	resp, err = http.Get(direct.URL + "/some/deep/path")
	if err != nil {
		t.Fatal(err)
	}
	page, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(page), "gately-base") || !strings.Contains(string(page), `src="/assets/a.js"`) {
		t.Fatalf("直接访问的首页应与以往一致: %s", page)
	}
	a.settingsMu.Lock()
	a.settings.SafePath = "/entry"
	a.settingsMu.Unlock()

	// 登录：Origin 带端口也能通过跨源校验，会话 Cookie 限定在前缀路径下
	resp = gatewayRequest(t, c, "POST", gw.URL+"/app/gately/api/login", `{"username":"admin","password":"test-password"}`, "")
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("经网关登录失败 %d %s", resp.StatusCode, body)
	}
	if sc := resp.Header.Get("Set-Cookie"); !strings.Contains(sc, "Path=/app/gately;") && !strings.HasSuffix(sc, "Path=/app/gately") {
		t.Fatalf("会话 Cookie 路径应带前缀: %q", sc)
	}
	if resp = gatewayRequest(t, c, "GET", gw.URL+"/app/gately/api/session", "", ""); resp.StatusCode != 200 {
		t.Fatalf("登录后应能访问接口，实际 %d", resp.StatusCode)
	}
	// 跨源请求仍被拒绝
	req, _ := http.NewRequest("POST", gw.URL+"/app/gately/api/logout", nil)
	req.Host = "nas.example.com"
	req.Header.Set("Origin", "https://evil.example.com")
	if resp, err = c.Do(req); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("经网关的跨源请求应被拒绝，实际 %d", resp.StatusCode)
	}
}

func TestGatewayClientIPAndLANOnly(t *testing.T) {
	a, direct, gw := newGatewayApp(t)
	a.settingsMu.Lock()
	a.settings.LANOnly = true
	a.settingsMu.Unlock()
	c := &http.Client{}
	// 经飞牛网关的请求由飞牛负责访问控制，不受“仅内网访问”约束：网关给出的地址可伪造，直连时还为空
	for _, ip := range []string{"203.0.113.9", "192.168.1.5", ""} {
		if resp := gatewayRequest(t, c, "GET", gw.URL+"/app/gately/", "", ip); resp.StatusCode != 200 {
			t.Fatalf("经网关访问不应受仅内网访问限制（X-Real-IP=%q），实际 %d", ip, resp.StatusCode)
		}
	}
	// 直接访问 TCP 端口仍然遵守“仅内网访问”（本机代理转发的外网地址被拒绝）
	req, _ := http.NewRequest("GET", direct.URL+"/", nil)
	req.Header.Set("X-Real-IP", "203.0.113.9")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("直接访问时外网地址应被拒绝，实际 %d", resp.StatusCode)
	}

	// 客户端地址只用于展示：取 X-Real-IP，忽略网关自身的回环地址
	for _, tc := range []struct{ realIP, xff, want string }{
		{"203.0.113.9", "127.0.0.1", "203.0.113.9:0"},
		{"", "127.0.0.1", ""},
		{"", "198.51.100.7", "198.51.100.7:0"},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		if tc.realIP != "" {
			r.Header.Set("X-Real-IP", tc.realIP)
		}
		r.Header.Set("X-Forwarded-For", tc.xff)
		if got := gatewayClientAddr(r); got != tc.want {
			t.Errorf("gatewayClientAddr(X-Real-IP=%q, XFF=%q) = %q，期望 %q", tc.realIP, tc.xff, got, tc.want)
		}
	}
}

func TestGatewayWriterRewritesRedirects(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/x" {
			t.Errorf("前缀应被去掉，实际路径 %q", r.URL.Path)
		}
		if p, ok := gatewayPrefix(r); !ok || p != "/app/gately" {
			t.Errorf("请求上下文应标记网关前缀，实际 %q %v", p, ok)
		}
		http.SetCookie(w, &http.Cookie{Name: "n", Value: "1", Path: "/api/login/oauth"})
		http.Redirect(w, r, "/#/login?e=1", http.StatusFound)
	})
	a := &App{sysLogs: logbuf.New(10, log.New(io.Discard, "", 0))}
	h := a.gatewayHandler("/app/gately", next)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/app/gately/api/x", nil))
	if loc := rec.Header().Get("Location"); loc != "/app/gately/#/login?e=1" {
		t.Fatalf("站内重定向应补前缀，实际 %q", loc)
	}
	if sc := rec.Header().Get("Set-Cookie"); !strings.Contains(sc, "Path=/app/gately/api/login/oauth") {
		t.Fatalf("Cookie 路径应补前缀，实际 %q", sc)
	}

	// 网关已去掉前缀时同样可用
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/x", nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("不带前缀的请求也应处理，实际 %d", rec.Code)
	}
}

func TestGatewayWebSocket(t *testing.T) {
	// 网关包装的 ResponseWriter 需要支持 Hijack，WebSocket 才能升级
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer ws.CloseNow()
		typ, msg, err := ws.Read(r.Context())
		if err == nil {
			_ = ws.Write(r.Context(), typ, append([]byte("echo:"), msg...))
		}
	})
	a := &App{sysLogs: logbuf.New(10, log.New(io.Discard, "", 0))}
	srv := httptest.NewServer(a.gatewayHandler("/app/gately", next))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/app/gately/api/ws", nil)
	if err != nil {
		t.Fatalf("经网关建立 WebSocket 失败: %v", err)
	}
	defer ws.CloseNow()
	if err := ws.Write(ctx, websocket.MessageText, []byte("hi")); err != nil {
		t.Fatal(err)
	}
	if _, msg, err := ws.Read(ctx); err != nil || string(msg) != "echo:hi" {
		t.Fatalf("WebSocket 回显异常: %q %v", msg, err)
	}
}

// fnosRequest 模拟 fnOS 网关在校验登录态后转发的请求，附带 X-Trim-* 用户信息。
func fnosRequest(t *testing.T, c *http.Client, method, url string, admin bool) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, url, nil)
	req.Host = "nas.example.com"
	req.Header.Set("X-Real-IP", "192.168.1.5")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Trim-Username", "alice")
	req.Header.Set("X-Trim-Userid", "1000")
	req.Header.Set("X-Trim-Isadmin", strconv.FormatBool(admin))
	if method == "POST" {
		req.Header.Set("Origin", "https://nas.example.com:4433")
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestGatewayAutoLogin(t *testing.T) {
	a, direct, gw := newGatewayApp(t)
	newClient := func() *http.Client {
		jar, _ := cookiejar.New(nil)
		return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}

	// 默认关闭：飞牛管理员打开首页也不会自动登录
	c := newClient()
	if resp := fnosRequest(t, c, "GET", gw.URL+"/app/gately/", true); resp.Header.Get("Set-Cookie") != "" {
		t.Fatal("未开启时不应自动登录")
	}
	if resp := fnosRequest(t, c, "POST", gw.URL+"/app/gately/api/login/fnos", true); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("未开启时飞牛登录接口应返回 403，实际 %d", resp.StatusCode)
	}

	a.settingsMu.Lock()
	a.settings.GatewaySSO = true
	a.settingsMu.Unlock()

	// 普通飞牛用户不会自动登录，登录页也不显示飞牛登录
	if resp := fnosRequest(t, c, "GET", gw.URL+"/app/gately/", false); resp.Header.Get("Set-Cookie") != "" {
		t.Fatal("普通飞牛用户不应自动登录")
	}
	resp := fnosRequest(t, c, "GET", gw.URL+"/app/gately/api/login/providers", false)
	if body, _ := io.ReadAll(resp.Body); strings.Contains(string(body), "fnosUser") {
		t.Fatalf("普通飞牛用户不应看到飞牛登录: %s", body)
	}

	// 飞牛管理员打开首页即登录，Cookie 限定在前缀路径下且仅经 HTTPS 发送
	resp = fnosRequest(t, c, "GET", gw.URL+"/app/gately/", true)
	sc := resp.Header.Get("Set-Cookie")
	if !strings.Contains(sc, "gately_token=") || !strings.Contains(sc, "Path=/app/gately") || !strings.Contains(sc, "Secure") {
		t.Fatalf("飞牛管理员应自动登录: %q", sc)
	}
	// 测试服务是 HTTP，带 Secure 的 Cookie 不会被 cookiejar 回传，这里手动带上
	token := strings.TrimPrefix(strings.Split(sc, ";")[0], "gately_token=")
	req, _ := http.NewRequest("GET", gw.URL+"/app/gately/api/session", nil)
	req.AddCookie(&http.Cookie{Name: "gately_token", Value: token})
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != 200 {
		t.Fatalf("自动登录后的会话应可用: %v", err)
	} else {
		resp.Body.Close()
	}
	// 已有有效会话时不重复签发
	req, _ = http.NewRequest("GET", gw.URL+"/app/gately/", nil)
	req.Header.Set("X-Trim-Username", "alice")
	req.Header.Set("X-Trim-Isadmin", "true")
	req.AddCookie(&http.Cookie{Name: "gately_token", Value: token})
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.Header.Get("Set-Cookie") != "" {
		t.Fatalf("已有会话时不应重复签发: %v", err)
	} else {
		resp.Body.Close()
	}

	// 整页跳转带的是浏览器语言，自动登录不改写已记录的界面语言
	a.settingsMu.Lock()
	a.settings.UILanguage = "zh-CN"
	a.settingsMu.Unlock()
	req, _ = http.NewRequest("GET", gw.URL+"/app/gately/", nil)
	req.Header.Set("X-Trim-Username", "alice")
	req.Header.Set("X-Trim-Isadmin", "true")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if resp, err := http.DefaultClient.Do(req); err != nil || !strings.Contains(resp.Header.Get("Set-Cookie"), "gately_token=") {
		t.Fatalf("飞牛管理员应自动登录: %v", err)
	} else {
		resp.Body.Close()
	}
	if got := a.settingsSnapshot().UILanguage; got != "zh-CN" {
		t.Fatalf("自动登录不应按浏览器语言改写界面语言: %q", got)
	}

	// 退出登录后，登录页显示飞牛登录，点击即可登录
	resp = fnosRequest(t, newClient(), "GET", gw.URL+"/app/gately/api/login/providers", true)
	if body, _ := io.ReadAll(resp.Body); !strings.Contains(string(body), `"fnosUser":"alice"`) {
		t.Fatalf("登录页应提供飞牛登录: %s", body)
	}
	if resp = fnosRequest(t, newClient(), "POST", gw.URL+"/app/gately/api/login/fnos", true); resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Set-Cookie"), "gately_token=") {
		t.Fatalf("飞牛登录接口应签发会话，实际 %d", resp.StatusCode)
	}

	// 直接访问 TCP 监听时伪造的 X-Trim-* 请求头无效
	dc := newClient()
	if resp = fnosRequest(t, dc, "GET", direct.URL+"/", true); resp.Header.Get("Set-Cookie") != "" {
		t.Fatal("直接访问时不应信任 X-Trim-* 请求头")
	}
	if resp = fnosRequest(t, dc, "POST", direct.URL+"/api/login/fnos", true); resp.StatusCode == 200 {
		t.Fatal("直接访问时飞牛登录接口不应可用")
	}

	// 开启仅内网访问不影响经网关的自动登录（访问控制由飞牛负责）
	a.settingsMu.Lock()
	a.settings.LANOnly = true
	a.settingsMu.Unlock()
	if resp = fnosRequest(t, newClient(), "GET", gw.URL+"/app/gately/", true); !strings.Contains(resp.Header.Get("Set-Cookie"), "gately_token=") {
		t.Fatal("开启仅内网访问时经网关仍应自动登录")
	}
}

// TestGatewayTerminalWebSocket 经统一网关的 Unix Socket 打开本机 Shell（issue #31）：
// 请求要经过 a.Handler() 的全部中间件（审计、翻译等），各层包装都要能把 Hijack 传到底层连接，
// 否则写出 101 后接管失败，浏览器一直停在“连接中”。简体中文与其他语言的中间件链不同，两种都要覆盖。
func TestGatewayTerminalWebSocket(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("本机 Shell 需要 Unix 伪终端")
	}
	// 终端会话在 WebSocket 断开后异步结束并写日志、会话记录：删除临时目录前先停止模块与落盘，
	// 删除时仍可能撞上最后一次异步写入，所以不用 t.TempDir（删除失败即判测试失败），改为重试几次
	cfgDir, err := os.MkdirTemp("", "gwterm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for i := 0; i < 20; i++ {
			if os.RemoveAll(cfgDir) == nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Logf("临时目录 %s 未能删除", cfgDir)
	})
	a, err := New(Options{ConfigDir: cfgDir, Modules: []module.Module{webterminal.New()}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		a.stopModules()
		a.closeLogs()
	})
	a.logger = log.New(io.Discard, "", 0)
	if err := a.auth.ChangeCredentials(mustInitialPassword(t, a), "admin", "test-password"); err != nil {
		t.Fatal(err)
	}
	handler := a.Handler()
	direct := httptest.NewServer(handler)
	t.Cleanup(direct.Close)

	// 与 fpk 部署一致：网关处理器监听在真实的 Unix Socket 上（路径过长会超出 Socket 地址上限，放在系统临时目录）
	dir, err := os.MkdirTemp("", "gw")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sockPath := filepath.Join(dir, "g.sock")
	ln, err := listenUnix(sockPath)
	if err != nil {
		t.Fatal(err)
	}
	gwSrv := &http.Server{Handler: a.gatewayHandler("/app/gately", handler)}
	go func() { _ = gwSrv.Serve(ln) }()
	t.Cleanup(func() { _ = gwSrv.Close() })

	// 通过直接访问登录并准备一个本机 Shell 连接
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	if code, body := doJSON(t, c, "POST", direct.URL+"/api/login", map[string]string{"username": "admin", "password": "test-password"}); code != 200 {
		t.Fatalf("登录失败 %d %s", code, body)
	}
	if code, body := doJSON(t, c, "POST", direct.URL+"/api/webterminal/security/agreement", nil); code != 200 {
		t.Fatalf("接受协议失败 %d %s", code, body)
	}
	if code, body := doJSON(t, c, "PUT", direct.URL+"/api/webterminal/settings", map[string]any{"allowLocalShell": true, "bufferSize": 64, "heartbeatInterval": 30, "maxSessions": 5}); code != 200 {
		t.Fatalf("开启本机 Shell 失败 %d %s", code, body)
	}
	code, body := doJSON(t, c, "POST", direct.URL+"/api/webterminal/connections", map[string]any{"name": "local", "type": "local", "enable": true, "local": map[string]any{"shell": "/bin/sh"}})
	if code != 200 && code != 201 {
		t.Fatalf("创建本机连接失败 %d %s", code, body)
	}
	var created struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(body, &created); err != nil || created.Key == "" {
		t.Fatalf("创建连接的响应缺少 key: %s", body)
	}
	u, _ := url.Parse(direct.URL)
	var token string
	for _, ck := range jar.Cookies(u) {
		if ck.Name == "gately_token" {
			token = ck.Value
		}
	}
	if token == "" {
		t.Fatal("登录后没有拿到会话 Cookie")
	}

	// 客户端经 Unix Socket 连接，模拟 fnOS 网关转发：Host 不带端口，Origin 带端口
	unixClient := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", sockPath)
		},
	}}
	for _, lang := range []string{"zh-CN", "en"} {
		t.Run(lang, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			h := http.Header{}
			h.Set("Cookie", "gately_token="+token)
			h.Set("Origin", "https://nas.example.com:4433")
			h.Set("Accept-Language", lang)
			ws, _, err := websocket.Dial(ctx, "ws://nas.example.com/app/gately/api/webterminal/connections/"+created.Key+"/ws?cols=80&rows=24",
				&websocket.DialOptions{HTTPClient: unixClient, HTTPHeader: h, Host: "nas.example.com"})
			if err != nil {
				t.Fatalf("经网关建立终端 WebSocket 失败: %v", err)
			}
			defer ws.CloseNow()
			// 服务端先发会话 ID（文本帧），再转发 Shell 输出（二进制帧）
			if typ, msg, err := ws.Read(ctx); err != nil || typ != websocket.MessageText || !strings.Contains(string(msg), `"t":"s"`) {
				t.Fatalf("应先收到会话 ID，实际 %v %q %v", typ, msg, err)
			}
			if err := ws.Write(ctx, websocket.MessageText, []byte(`{"t":"i","d":"echo gately-$((6*7))\n"}`)); err != nil {
				t.Fatal(err)
			}
			var out strings.Builder
			for !strings.Contains(out.String(), "gately-42") {
				typ, msg, err := ws.Read(ctx)
				if err != nil {
					t.Fatalf("没有收到 Shell 输出: %v，已收到 %q", err, out.String())
				}
				if typ == websocket.MessageBinary {
					out.Write(msg)
				}
			}
		})
	}
}
