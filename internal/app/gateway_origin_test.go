package app

import (
	"bytes"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/portforward"
)

// gwReq 描述一个模拟 fnOS 网关转发的请求：Host 不带端口。
type gwReq struct {
	method, path string
	origin       string
	// site 为浏览器写入的 Sec-Fetch-Site，空串表示不带（旧浏览器、部分浏览器的 WebSocket 握手、命令行工具）
	site   string
	cookie string
	body   string
	// ws 为 true 时带上 WebSocket 握手的请求头
	ws bool
	// multipart 为 true 时以表单方式提交（不触发浏览器预检的写请求）
	multipart bool
	// page 为 true 时带上管理界面的 X-Gately-Page 请求头
	page bool
}

func (q gwReq) do(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	var body io.Reader
	ctype := ""
	switch {
	case q.multipart:
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", "x.zip")
		_, _ = fw.Write([]byte("not a zip"))
		_ = mw.Close()
		body, ctype = &buf, mw.FormDataContentType()
	case q.body != "":
		body, ctype = strings.NewReader(q.body), "application/json"
	}
	req := httptest.NewRequest(q.method, "http://nas.example.com/app/gately"+q.path, body)
	req.Host = "nas.example.com"
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	if q.origin != "" {
		req.Header.Set("Origin", q.origin)
	}
	if q.site != "" {
		req.Header.Set("Sec-Fetch-Site", q.site)
	}
	if q.cookie != "" {
		req.Header.Set("Cookie", q.cookie)
	}
	if q.page {
		req.Header.Set(gatewayPageHeader, "1")
	}
	if q.ws {
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Upgrade", "websocket")
		req.Header.Set("Sec-WebSocket-Version", "13")
		req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == "gately_token" && ck.Value != "" {
			return "gately_token=" + ck.Value
		}
	}
	t.Fatalf("响应里没有会话 Cookie: %d %s", rec.Code, rec.Body.String())
	return ""
}

const (
	gwLegit = "https://nas.example.com:5667"
	// 同一台主机上其他端口的页面：主机名相同，浏览器会带上会话 Cookie
	gwOtherPort = "http://nas.example.com:8096"
	// 默认端口的来源不带端口号，与网关转发来的 Host 字面相同
	gwDefaultPort = "https://nas.example.com"
)

// newOriginApp 在 dir 上创建实例并返回经网关访问的处理器。setup 为 true 时初始化管理员账号（admin / test-password），
// 为 false 时沿用 dir 中已有的账号，用来模拟重启。
func newOriginApp(t *testing.T, dir string, setup bool) (*App, http.Handler) {
	t.Helper()
	a, err := New(Options{ConfigDir: dir, Modules: []module.Module{portforward.New()}})
	if err != nil {
		t.Fatal(err)
	}
	a.logger = log.New(io.Discard, "", 0)
	if setup {
		// 须在创建处理器之前：mustInitialPassword 会换掉账号管理器
		if err := a.auth.ChangeCredentials(mustInitialPassword(t, a), "admin", "test-password"); err != nil {
			t.Fatal(err)
		}
	}
	return a, a.gatewayHandler("/app/gately", a.Handler())
}

// 经统一网关访问时，同主机其他端口的页面不能借管理员的会话发起写请求或建立 WebSocket：
// 只比主机名会把它们当成管理界面自己。
func TestGatewayRejectsOtherPortOrigin(t *testing.T) {
	dir := t.TempDir()
	a, h := newOriginApp(t, dir, true)
	// 现代浏览器登录：Origin 带端口，浏览器标明同源
	rec := gwReq{method: "POST", path: "/api/login", origin: gwLegit, site: "same-origin", body: `{"username":"admin","password":"test-password"}`}.do(t, h)
	if rec.Code != 200 {
		t.Fatalf("经网关登录失败: %d %s", rec.Code, rec.Body.String())
	}
	admin := sessionCookie(t, rec)

	wsPath := "/api/portforward/rules/x/ws"
	attacks := []gwReq{
		// 其他端口的页面：浏览器标明 same-site
		{method: "POST", path: "/api/system/restore", origin: gwOtherPort, site: "same-site", cookie: admin, multipart: true},
		{method: "DELETE", path: "/api/portforward/logs", origin: gwOtherPort, site: "same-site", cookie: admin},
		{method: "GET", path: wsPath, origin: gwOtherPort, site: "same-site", cookie: admin, ws: true},
		// 不带 Sec-Fetch-Site 的浏览器或握手：其他端口的来源没有确认过
		{method: "POST", path: "/api/system/restore", origin: gwOtherPort, cookie: admin, multipart: true},
		{method: "DELETE", path: "/api/portforward/logs", origin: gwOtherPort, cookie: admin},
		{method: "GET", path: wsPath, origin: gwOtherPort, cookie: admin, ws: true},
		// 默认端口上的页面，来源与转发来的 Host 字面相同
		{method: "DELETE", path: "/api/portforward/logs", origin: gwDefaultPort, cookie: admin},
		{method: "GET", path: wsPath, origin: gwDefaultPort, cookie: admin, ws: true},
		{method: "DELETE", path: "/api/portforward/logs", origin: gwDefaultPort, site: "same-site", cookie: admin},
		// 其他站点
		{method: "DELETE", path: "/api/portforward/logs", origin: "https://evil.example", site: "cross-site", cookie: admin},
		{method: "GET", path: wsPath, origin: "https://evil.example", cookie: admin, ws: true},
		// 凭飞牛登录态登录不需要凭据，也不能由别的页面触发
		{method: "POST", path: "/api/login/fnos", origin: gwOtherPort, site: "same-site"},
	}
	for _, q := range attacks {
		if rec := q.do(t, h); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s（Origin=%s，Sec-Fetch-Site=%q）应被拒绝，实际 %d %s", q.method, q.path, q.origin, q.site, rec.Code, rec.Body.String())
		}
	}

	// 管理界面自己的请求照常通过
	if rec := (gwReq{method: "DELETE", path: "/api/portforward/logs", origin: gwLegit, site: "same-origin", cookie: admin}).do(t, h); rec.Code != 200 {
		t.Fatalf("同源写请求应通过: %d %s", rec.Code, rec.Body.String())
	}
	// 来源已确认：之后不带 Sec-Fetch-Site 的请求（WebSocket 握手）也接受，越过来源检查后到达接口本身
	if rec := (gwReq{method: "DELETE", path: "/api/portforward/logs", origin: gwLegit, cookie: admin}).do(t, h); rec.Code != 200 {
		t.Fatalf("已确认来源的写请求应通过: %d %s", rec.Code, rec.Body.String())
	}
	if rec := (gwReq{method: "GET", path: wsPath, origin: gwLegit, cookie: admin, ws: true}).do(t, h); rec.Code == http.StatusForbidden {
		t.Fatalf("已确认来源的 WebSocket 握手不应被来源检查拒绝: %d %s", rec.Code, rec.Body.String())
	}
	// 没有 Origin 的请求（命令行工具）与普通 GET 不受影响
	if rec := (gwReq{method: "GET", path: "/api/session", cookie: admin}).do(t, h); rec.Code != 200 {
		t.Fatalf("不带 Origin 的读取应通过: %d", rec.Code)
	}
	if rec := (gwReq{method: "GET", path: "/api/session", origin: gwOtherPort, site: "same-site", cookie: admin}).do(t, h); rec.Code != 200 {
		t.Fatalf("普通 GET 不做来源检查（浏览器不会把响应交给其他来源的页面）: %d", rec.Code)
	}
	// 攻击请求没有让别的来源变成“已确认”
	for _, o := range []string{gwOtherPort, gwDefaultPort, "https://evil.example"} {
		key, _ := originKey(o)
		if a.gatewayOriginKnown(key) {
			t.Errorf("%s 不应被记为已确认的来源", o)
		}
	}

	// 已确认的来源保存在配置目录，重启后仍然有效
	a2, h2 := newOriginApp(t, dir, false)
	key, _ := originKey(gwLegit)
	if !a2.gatewayOriginKnown(key) {
		t.Fatal("重启后应保留已确认的来源")
	}
	if rec := (gwReq{method: "GET", path: wsPath, origin: gwLegit, cookie: admin, ws: true}).do(t, h2); rec.Code == http.StatusForbidden {
		t.Fatalf("重启后已确认来源的 WebSocket 握手不应被拒绝: %d", rec.Code)
	}
	if rec := (gwReq{method: "GET", path: wsPath, origin: gwOtherPort, cookie: admin, ws: true}).do(t, h2); rec.Code != http.StatusForbidden {
		t.Fatalf("重启后其他端口的握手仍应被拒绝: %d", rec.Code)
	}
}

// 来源只能由管理员确认：只读账号可以不经浏览器自己构造请求头，不能替别的端口“确认”来源。
func TestGatewayOriginLearnedOnlyFromAdmin(t *testing.T) {
	a, h := newOriginApp(t, t.TempDir(), true)
	if err := a.auth.AddUser("viewer", "tide-Lamp-47-oak"); err != nil {
		t.Fatal(err)
	}
	known := func(o string) bool {
		key, _ := originKey(o)
		return a.gatewayOriginKnown(key)
	}
	// 只读账号以伪造的来源登录（不带 Sec-Fetch-Site，如同旧浏览器），再带着会话伪造“同源”标记
	rec := gwReq{method: "POST", path: "/api/login", origin: gwOtherPort, body: `{"username":"viewer","password":"tide-Lamp-47-oak"}`}.do(t, h)
	if rec.Code != 200 {
		t.Fatalf("只读账号登录失败: %d %s", rec.Code, rec.Body.String())
	}
	viewer := sessionCookie(t, rec)
	gwReq{method: "PUT", path: "/api/account/ui-language", origin: gwOtherPort, site: "same-origin", cookie: viewer, body: `{"language":"zh-CN"}`}.do(t, h)
	if known(gwOtherPort) {
		t.Fatal("只读账号不能让来源变成已确认")
	}
	// 错误密码、飞牛登录态登录同样不能
	gwReq{method: "POST", path: "/api/login", origin: gwOtherPort, body: `{"username":"admin","password":"wrong"}`}.do(t, h)
	a.settingsMu.Lock()
	a.settings.GatewaySSO = true
	a.settingsMu.Unlock()
	req := httptest.NewRequest("POST", "http://nas.example.com/app/gately/api/login/fnos", nil)
	req.Host = "nas.example.com"
	req.Header.Set("Origin", gwOtherPort)
	req.Header.Set("X-Trim-Isadmin", "true")
	req.Header.Set("X-Trim-Username", "boss")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("飞牛登录态登录应成功（不带 Sec-Fetch-Site 的浏览器）: %d %s", rec.Code, rec.Body.String())
	}
	sso := sessionCookie(t, rec)
	if known(gwOtherPort) {
		t.Fatal("飞牛登录态登录不需要凭据，不能据此确认来源")
	}
	// 这样得到的管理员会话，在来源被确认前同样不能用于不带 Sec-Fetch-Site 的写请求与握手
	if rec := (gwReq{method: "DELETE", path: "/api/portforward/logs", origin: gwOtherPort, cookie: sso}).do(t, h); rec.Code != http.StatusForbidden {
		t.Fatalf("未确认来源的写请求应被拒绝: %d", rec.Code)
	}

	// 旧浏览器（不带 Sec-Fetch-Site）里管理员用密码登录：登录成功即确认来源
	rec = gwReq{method: "POST", path: "/api/login", origin: gwLegit, body: `{"username":"admin","password":"test-password"}`}.do(t, h)
	if rec.Code != 200 || !known(gwLegit) {
		t.Fatalf("管理员密码登录成功应确认来源: %d %v", rec.Code, known(gwLegit))
	}
	admin := sessionCookie(t, rec)
	if rec := (gwReq{method: "DELETE", path: "/api/portforward/logs", origin: gwLegit, cookie: admin}).do(t, h); rec.Code != 200 {
		t.Fatalf("旧浏览器登录后的写请求应通过: %d %s", rec.Code, rec.Body.String())
	}
	if known(gwOtherPort) {
		t.Fatal("其他端口不应被确认")
	}
}

// 经 HTTP 地址访问（如飞牛的 5000 端口、飞牛 App 的 HTTP 模式）时浏览器不发 Sec-Fetch-Site：管理界面的请求带着
// X-Gately-Page，凭飞牛登录态登录后的写请求照常通过，并为之后的 WebSocket 握手确认来源；别的端口的页面带不上这个
// 请求头（须先通过 CORS 预检），仍被拒绝。
func TestGatewayPlainHTTPPageHeader(t *testing.T) {
	a, h := newOriginApp(t, t.TempDir(), true)
	if err := a.auth.AddUser("viewer", "tide-Lamp-47-oak"); err != nil {
		t.Fatal(err)
	}
	a.settingsMu.Lock()
	a.settings.GatewaySSO = true
	a.settingsMu.Unlock()
	const httpOrigin = "http://nas.example.com:5000"
	known := func(o string) bool {
		key, _ := originKey(o)
		return a.gatewayOriginKnown(key)
	}
	// 打开首页时凭飞牛登录态自动登录
	req := httptest.NewRequest("GET", "http://nas.example.com/app/gately/", nil)
	req.Host = "nas.example.com"
	req.Header.Set("X-Trim-Isadmin", "true")
	req.Header.Set("X-Trim-Username", "boss")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	sso := sessionCookie(t, rec)

	wsPath := "/api/portforward/rules/x/ws"
	// 修复前：不带 Sec-Fetch-Site、来源未确认的写请求一律被拒（修改设置提示“拒绝跨源请求”）
	if rec := (gwReq{method: "DELETE", path: "/api/portforward/logs", origin: httpOrigin, cookie: sso}).do(t, h); rec.Code != http.StatusForbidden {
		t.Fatalf("不带 X-Gately-Page 的写请求应被拒绝: %d", rec.Code)
	}
	if rec := (gwReq{method: "GET", path: wsPath, origin: httpOrigin, cookie: sso, ws: true}).do(t, h); rec.Code != http.StatusForbidden {
		t.Fatalf("来源确认前的 WebSocket 握手应被拒绝: %d", rec.Code)
	}
	if rec := (gwReq{method: "DELETE", path: "/api/portforward/logs", origin: httpOrigin, cookie: sso, page: true}).do(t, h); rec.Code != 200 {
		t.Fatalf("管理界面经 HTTP 发出的写请求应通过: %d %s", rec.Code, rec.Body.String())
	}
	if !known(httpOrigin) {
		t.Fatal("带管理员会话与 X-Gately-Page 的请求应确认来源")
	}
	if rec := (gwReq{method: "GET", path: wsPath, origin: httpOrigin, cookie: sso, ws: true}).do(t, h); rec.Code == http.StatusForbidden {
		t.Fatalf("来源确认后的 WebSocket 握手不应被拒绝: %d %s", rec.Code, rec.Body.String())
	}
	// 主机名不同的来源，带着请求头也不行；只读账号带着请求头不能确认来源
	if rec := (gwReq{method: "DELETE", path: "/api/portforward/logs", origin: "http://evil.example:5000", cookie: sso, page: true}).do(t, h); rec.Code != http.StatusForbidden {
		t.Fatalf("主机名不同的来源应被拒绝: %d", rec.Code)
	}
	rec = gwReq{method: "POST", path: "/api/login", origin: gwOtherPort, page: true, body: `{"username":"viewer","password":"tide-Lamp-47-oak"}`}.do(t, h)
	if rec.Code != 200 {
		t.Fatalf("只读账号登录失败: %d %s", rec.Code, rec.Body.String())
	}
	viewer := sessionCookie(t, rec)
	gwReq{method: "PUT", path: "/api/account/ui-language", origin: gwOtherPort, page: true, cookie: viewer, body: `{"language":"zh-CN"}`}.do(t, h)
	if known(gwOtherPort) {
		t.Fatal("只读账号不能让来源变成已确认")
	}
	// 别的端口的页面带不上自定义请求头：管理员会话下的写请求与握手仍被拒绝
	if rec := (gwReq{method: "DELETE", path: "/api/portforward/logs", origin: gwOtherPort, cookie: sso}).do(t, h); rec.Code != http.StatusForbidden {
		t.Fatalf("其他端口的写请求应被拒绝: %d", rec.Code)
	}
	if rec := (gwReq{method: "GET", path: wsPath, origin: gwOtherPort, cookie: sso, ws: true}).do(t, h); rec.Code != http.StatusForbidden {
		t.Fatalf("其他端口的握手应被拒绝: %d", rec.Code)
	}
	// 拒绝时按来源记一条日志，写明判断依据
	found := false
	for _, e := range a.sysLogs.Since(0) {
		if strings.Contains(e.Message, "来源未确认被拒绝") && strings.Contains(e.Message, gwOtherPort) {
			found = true
		}
	}
	if !found {
		t.Fatal("被拒绝的来源应记入系统日志")
	}
}
