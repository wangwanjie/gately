package webservice

import (
	"compress/gzip"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/webpage"
)

func TestParseStatusCodes(t *testing.T) {
	cases := []struct {
		in, norm string
		has, not []int
		bad      bool
	}{
		{in: "404, 500-599", norm: "404, 500-599", has: []int{404, 500, 502, 599}, not: []int{403, 405, 499}},
		{in: "5xx，404；404 4XX", norm: "500-599, 404, 400-499", has: []int{400, 450, 599}},
		{in: " 502\n503\t504 ", norm: "502, 503, 504", has: []int{502, 503, 504}, not: []int{500}},
		{in: "", norm: ""},
		{in: "399", bad: true},
		{in: "600", bad: true},
		{in: "200", bad: true},
		{in: "500-450", bad: true},
		{in: "abc", bad: true},
		{in: "3xx", bad: true},
		{in: "404-", bad: true},
		{in: strings.Repeat("404,", maxInterceptTokens+1), bad: true},
	}
	for _, c := range cases {
		set, norm, err := parseStatusCodes(c.in)
		if c.bad {
			if err == nil {
				t.Errorf("%q 应报错", c.in)
			}
			continue
		}
		if err != nil || norm != c.norm {
			t.Errorf("%q: 得到 %q %v，期望 %q", c.in, norm, err, c.norm)
			continue
		}
		for _, h := range c.has {
			if !set[h] {
				t.Errorf("%q 应包含 %d", c.in, h)
			}
		}
		for _, n := range c.not {
			if set[n] {
				t.Errorf("%q 不应包含 %d", c.in, n)
			}
		}
	}
}

func TestErrorPageOptionsValidate(t *testing.T) {
	ok := []ErrorPageOptions{
		{},
		{Style: "terminal"},
		{Style: " aurora "},
		{Style: ErrorStyleCustom, Page: "p1"},
		{Intercept: true, Codes: "404"},
		{Style: "minimal", Page: "残留", Codes: "5xx"},
	}
	for _, o := range ok {
		if err := o.validate(); err != nil {
			t.Errorf("%+v 应通过: %v", o, err)
		}
		if o.Style != ErrorStyleCustom && o.Page != "" {
			t.Errorf("非自定义样式应清空页面引用: %+v", o)
		}
	}
	bad := []ErrorPageOptions{
		{Style: "nope"},
		{Style: ErrorStyleCustom},
		{Intercept: true},
		{Intercept: true, Codes: "302"},
		{Intercept: true, Codes: "abc"},
	}
	for _, o := range bad {
		if err := o.validate(); err == nil {
			t.Errorf("%+v 应报错", o)
		}
	}
	// 站点与默认站点校验时一并检查；断开连接类型不校验；默认站点停用时仍检查
	l := Listener{Port: 80, Sites: []Site{{Domains: []string{"a.com"}, Type: TypeStatic, Root: "/tmp", ErrorPage: ErrorPageOptions{Style: "nope"}}}}
	if err := l.Validate(); err == nil {
		t.Fatal("站点的错误页设置无效时应报错")
	}
	l.Sites[0].Type = TypeClose
	if err := l.Validate(); err != nil {
		t.Fatalf("断开连接类型不应校验错误页: %v", err)
	}
	l.Default.ErrorPage = ErrorPageOptions{Style: "nope"}
	if err := l.Validate(); err == nil {
		t.Fatal("默认站点停用时也应校验错误页设置")
	}
}

func writeFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// 页面文件只接受扩展名为 .html/.htm 的普通文件（含符号链接的目标），且内容须为可用的 HTML。
func TestReadErrorPageFile(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "50x.html")
	writeFile(t, good, `<meta name="viewport" content="width=device-width"><h1>{code}</h1><script>alert(1)</script>`)
	page, rep, st, err := readErrorPageFile(good)
	if err != nil || st == nil || strings.Contains(page, "<script") || len(rep.Removed) == 0 {
		t.Fatalf("正常页面: %v %+v\n%s", err, rep, page)
	}
	if _, _, _, err := readErrorPageFile(filepath.Join(dir, "x.HTM")); err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("不存在的文件: %v", err)
	}
	secret := filepath.Join(dir, "secret.txt")
	writeFile(t, secret, "<h1>password=123</h1>")
	link := filepath.Join(dir, "link.html")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatal(err)
	}
	okLink := filepath.Join(dir, "ok.htm")
	if err := os.Symlink(good, okLink); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := readErrorPageFile(okLink); err != nil {
		t.Fatalf("指向 HTML 文件的符号链接应可用: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "d.html"), 0o755); err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(dir, "plain.html")
	writeFile(t, plain, "root:x:0:0:root:/root:/bin/bash\n")
	big := filepath.Join(dir, "big.html")
	writeFile(t, big, "<h1>x</h1>"+strings.Repeat("a", webpage.MaxCustomPage))
	for name, p := range map[string]string{
		"相对路径":        "50x.html",
		"非 HTML 扩展名":  secret,
		"指向非 HTML 文件": link,
		"目录":          filepath.Join(dir, "d.html"),
		"不是 HTML 内容":  plain,
		"超过上限":        big,
		"空路径":         "",
		"系统文件":        "/etc/passwd",
	} {
		if _, _, _, err := readErrorPageFile(p); err == nil {
			t.Errorf("%s: 应拒绝", name)
		}
	}
}

// 文件页面变化后自动重新读取；文件不可用时退回默认样式，同一原因只记录一次日志。
func TestErrorPageStoreFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "e.html")
	writeFile(t, p, "<h1>v1 {code}</h1>")
	s := newErrorPageStore()
	var warns []string
	s.warn = func(f string, a ...any) { warns = append(warns, f) }
	s.set([]ErrorPage{{Key: "f", Name: "文件", Source: ErrorPageFile, File: p}, {Key: "i", Name: "内联", Source: ErrorPageInline, HTML: "<h1>inline</h1><script>x</script>"}})
	if page, ok := s.page("f"); !ok || !strings.Contains(page, "v1") {
		t.Fatalf("应读到 v1: %q", page)
	}
	// 配置中的内联页面即使被手工改过也会再清理一遍
	if page, ok := s.page("i"); !ok || strings.Contains(page, "<script") {
		t.Fatalf("内联页面应再次清理: %q", page)
	}
	writeFile(t, p, "<h1>version2 {code}</h1>")
	_ = os.Chtimes(p, time.Now(), time.Now().Add(time.Minute))
	if page, ok := s.page("f"); !ok || !strings.Contains(page, "version2") {
		t.Fatalf("文件变化后应重新读取: %q", page)
	}
	_ = os.Remove(p)
	for range 3 {
		if _, ok := s.page("f"); ok {
			t.Fatal("文件删除后应不可用")
		}
	}
	if len(warns) != 1 || s.fileError("f") == "" {
		t.Fatalf("同一原因应只记录一次: %v", warns)
	}
	writeFile(t, p, "<h1>back</h1>")
	if page, ok := s.page("f"); !ok || !strings.Contains(page, "back") || s.fileError("f") != "" {
		t.Fatalf("文件恢复后应可用: %q", page)
	}
	if _, ok := s.page("none"); ok {
		t.Fatal("不存在的页面应不可用")
	}
}

// errBackend 是按路径返回各种状态码的后端。
func errBackend(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/404":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			_, _ = io.WriteString(w, `{"error":"backend-404"}`)
		case "/500":
			// 带压缩编码、CSP 与 Cookie 的错误响应：替换后压缩编码与 CSP 不应保留，Cookie 保留
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Content-Security-Policy", "default-src 'self'")
			w.Header().Set("Set-Cookie", "sid=1; Path=/")
			w.Header().Set("Etag", `"abc"`)
			w.WriteHeader(500)
			zw := gzip.NewWriter(w)
			_, _ = io.WriteString(zw, "backend-500 stack trace")
			_ = zw.Close()
		case "/503":
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(503)
			_, _ = io.WriteString(w, "backend-503")
		case "/418":
			w.WriteHeader(418)
			_, _ = io.WriteString(w, "backend-418")
		default:
			_, _ = io.WriteString(w, "backend-ok")
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

var (
	htmlAccept = map[string]string{"Accept": "text/html,application/xhtml+xml,*/*;q=0.8", "Accept-Language": "zh-CN"}
	jsonAccept = map[string]string{"Accept": "application/json"}
)

// 拦截后端错误：浏览器打开页面的请求按设置的样式替换，接口请求、未列出的状态码与成功响应保持后端原样。
func TestInterceptBackendErrors(t *testing.T) {
	be := errBackend(t)
	site := Site{Enable: true, Domains: []string{"app.test"}, Type: TypeProxy, Targets: []string{be.URL},
		ErrorPage: ErrorPageOptions{Style: "terminal", Intercept: true, Codes: "404, 500-599"}}
	addr, _ := startRunner(t, Listener{Sites: []Site{site}}, nil, nil)
	c := &http.Client{Transport: &http.Transport{DisableCompression: true}}

	resp, body := get(t, c, addr, "app.test", "/404", htmlAccept)
	if resp.StatusCode != 404 || strings.Contains(body, "backend-404") || !strings.Contains(body, "HTTP/1.1 404 Not Found") || !strings.Contains(body, "页面不存在") {
		t.Fatalf("404 应替换为终端风格: %d %s", resp.StatusCode, body)
	}
	if resp.Header.Get("Content-Security-Policy") != webpage.ErrorPageCSP || resp.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("替换后的响应头: %v", resp.Header)
	}
	resp, body = get(t, c, addr, "app.test", "/500", htmlAccept)
	if resp.StatusCode != 500 || strings.Contains(body, "stack trace") || resp.Header.Get("Content-Encoding") != "" || resp.Header.Get("Etag") != "" {
		t.Fatalf("500 应替换且去掉压缩编码与 Etag: %d %v %s", resp.StatusCode, resp.Header, body)
	}
	if resp.Header.Get("Set-Cookie") != "sid=1; Path=/" || resp.Header.Get("Content-Security-Policy") != webpage.ErrorPageCSP {
		t.Fatalf("应保留 Cookie、覆盖后端的 CSP: %v", resp.Header)
	}
	resp, _ = get(t, c, addr, "app.test", "/503", htmlAccept)
	if resp.StatusCode != 503 || resp.Header.Get("Retry-After") != "120" {
		t.Fatalf("503 应保留 Retry-After: %v", resp.Header)
	}
	// 接口请求不替换
	resp, body = get(t, c, addr, "app.test", "/404", jsonAccept)
	if resp.StatusCode != 404 || body != `{"error":"backend-404"}` {
		t.Fatalf("接口请求应保持原样: %d %s", resp.StatusCode, body)
	}
	// 没有 Accept 的命令行请求不替换
	resp, body = get(t, c, addr, "app.test", "/404", nil)
	if body != `{"error":"backend-404"}` {
		t.Fatalf("无 Accept 的请求应保持原样: %s", body)
	}
	// 未列出的状态码与成功响应不替换
	if _, body = get(t, c, addr, "app.test", "/418", htmlAccept); body != "backend-418" {
		t.Fatalf("418 不在拦截列表中: %s", body)
	}
	if _, body = get(t, c, addr, "app.test", "/", htmlAccept); body != "backend-ok" {
		t.Fatalf("成功响应: %s", body)
	}
	// HEAD 请求只返回状态码与响应头
	req, _ := http.NewRequest("HEAD", "http://"+addr+"/404", nil)
	req.Host = "app.test"
	req.Header.Set("Accept", "text/html")
	hr, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	hb, _ := io.ReadAll(hr.Body)
	hr.Body.Close()
	if hr.StatusCode != 404 || len(hb) != 0 || hr.Header.Get("Content-Security-Policy") != webpage.ErrorPageCSP {
		t.Fatalf("HEAD: %d %q %v", hr.StatusCode, hb, hr.Header)
	}
}

// 拦截与压缩同时开启：替换后的页面由 Gately 重新压缩，客户端解压后得到完整页面。
func TestInterceptWithCompression(t *testing.T) {
	be := errBackend(t)
	site := Site{Enable: true, Domains: []string{"app.test"}, Type: TypeProxy, Targets: []string{be.URL}, Compress: true,
		ErrorPage: ErrorPageOptions{Style: "minimal", Intercept: true, Codes: "5xx"}}
	addr, _ := startRunner(t, Listener{Sites: []Site{site}}, nil, nil)
	resp, body := get(t, http.DefaultClient, addr, "app.test", "/500", htmlAccept)
	if resp.StatusCode != 500 || !strings.Contains(body, "服务器内部错误") || !strings.Contains(body, "</html>") {
		t.Fatalf("压缩后的错误页: %d %s", resp.StatusCode, body)
	}
}

// Gately 自己生成的错误页：连不上后端的 502、静态文件的 404、未匹配站点的 404 都按设置的样式输出；
// 非浏览器请求仍为纯文本，未设置样式的站点保持原有的默认错误页。
func TestGeneratedErrorPagesStyled(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	dead := "http://" + ln.Addr().String()
	ln.Close()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "index.html"), "home")
	rootOwn := t.TempDir()
	writeFile(t, filepath.Join(rootOwn, "404.html"), "own-404")
	l := Listener{
		Sites: []Site{
			{Enable: true, Domains: []string{"down.test"}, Type: TypeProxy, Targets: []string{dead}, ErrorPage: ErrorPageOptions{Style: "status"}},
			{Enable: true, Domains: []string{"plain.test"}, Type: TypeProxy, Targets: []string{dead}},
			{Enable: true, Domains: []string{"static.test"}, Type: TypeStatic, Root: root, ErrorPage: ErrorPageOptions{Style: "paper"}},
			{Enable: true, Domains: []string{"own.test"}, Type: TypeStatic, Root: rootOwn, NotFoundPage: "404.html", ErrorPage: ErrorPageOptions{Style: "paper"}},
		},
		// 默认站点停用，其错误页设置仍用于未匹配任何站点的 404
		Default: Site{Enable: false, ErrorPage: ErrorPageOptions{Style: "neon"}},
	}
	addr, _ := startRunner(t, l, nil, nil)
	c := http.DefaultClient

	resp, body := get(t, c, addr, "down.test", "/x", htmlAccept)
	if resp.StatusCode != 502 || !strings.Contains(body, `class="chain"`) || !strings.Contains(body, "无法连接到后端服务") || resp.Header.Get("Content-Security-Policy") != webpage.ErrorPageCSP {
		t.Fatalf("502 应为状态链路风格: %d %v %s", resp.StatusCode, resp.Header, body)
	}
	if resp, body = get(t, c, addr, "down.test", "/x", jsonAccept); resp.StatusCode != 502 || strings.Contains(body, "<html") {
		t.Fatalf("非浏览器请求应为纯文本: %s", body)
	}
	// 未设置样式：保持原有默认错误页，不附加 CSP
	if resp, body = get(t, c, addr, "plain.test", "/x", htmlAccept); resp.StatusCode != 502 || !strings.Contains(body, `class="card err"`) || resp.Header.Get("Content-Security-Policy") != "" {
		t.Fatalf("默认样式: %v %s", resp.Header, body)
	}
	if resp, body = get(t, c, addr, "static.test", "/missing", htmlAccept); resp.StatusCode != 404 || !strings.Contains(body, `class="lead"`) {
		t.Fatalf("静态站点 404 应为报刊风格: %d %s", resp.StatusCode, body)
	}
	// 站点自带的 404 页面优先
	if resp, body = get(t, c, addr, "own.test", "/missing", htmlAccept); resp.StatusCode != 404 || body != "own-404" {
		t.Fatalf("应使用站点自带的 404 页面: %s", body)
	}
	if resp, body = get(t, c, addr, "unknown.test", "/", htmlAccept); resp.StatusCode != 404 || !strings.Contains(body, `class="floor"`) {
		t.Fatalf("未匹配站点的 404 应为霓虹风格: %d %s", resp.StatusCode, body)
	}
	if resp, body = get(t, c, addr, "unknown.test", "/", nil); resp.StatusCode != 404 || strings.Contains(body, "<html") {
		t.Fatalf("未匹配站点的非浏览器请求应为纯文本: %s", body)
	}
}

// 自定义页面：替换占位符输出；页面被删除或不可用时退回默认样式；拦截时使用默认样式也会附加 CSP。
func TestCustomErrorPageServing(t *testing.T) {
	be := errBackend(t)
	l := Listener{Sites: []Site{
		{Enable: true, Domains: []string{"c.test"}, Type: TypeProxy, Targets: []string{be.URL},
			ErrorPage: ErrorPageOptions{Style: ErrorStyleCustom, Page: "p1", Intercept: true, Codes: "404"}},
		{Enable: true, Domains: []string{"d.test"}, Type: TypeProxy, Targets: []string{be.URL},
			ErrorPage: ErrorPageOptions{Intercept: true, Codes: "404"}},
	}}
	addr, r := startRunner(t, l, nil, nil)
	r.auth.errPages.set([]ErrorPage{{Key: "p1", Name: "品牌页", Source: ErrorPageInline, HTML: `<title>{code}</title><h1 class="brand">{code} {title}</h1><p>{host}{path}</p>`}})
	resp, body := get(t, http.DefaultClient, addr, "c.test", "/404", htmlAccept)
	if resp.StatusCode != 404 || !strings.Contains(body, `<h1 class="brand">404 页面不存在</h1>`) || !strings.Contains(body, "c.test/404") {
		t.Fatalf("自定义页面: %d %s", resp.StatusCode, body)
	}
	r.auth.errPages.set(nil)
	if _, body = get(t, http.DefaultClient, addr, "c.test", "/404", htmlAccept); !strings.Contains(body, `class="card err"`) {
		t.Fatalf("页面不存在时应退回默认样式: %s", body)
	}
	resp, body = get(t, http.DefaultClient, addr, "d.test", "/404", htmlAccept)
	if !strings.Contains(body, `class="card err"`) || resp.Header.Get("Content-Security-Policy") != webpage.ErrorPageCSP {
		t.Fatalf("默认样式的拦截页: %v %s", resp.Header, body)
	}
}

// 拦截只对反向代理站点生效。
func TestErrorPagerProxyOnly(t *testing.T) {
	store := newErrorPageStore()
	if e := newErrorPager(Site{Type: TypeText, ErrorPage: ErrorPageOptions{Intercept: true, Codes: "404"}}, store); e != nil {
		t.Fatal("非反向代理站点不应拦截")
	}
	if e := newErrorPager(Site{Type: TypeProxy, ErrorPage: ErrorPageOptions{Intercept: true, Codes: "404"}}, store); e == nil || !e.codes[404] {
		t.Fatal("反向代理站点应拦截")
	}
	if e := newErrorPager(Site{Type: TypeProxy}, store); e != nil {
		t.Fatal("默认样式且不拦截时应为 nil")
	}
	var e *errorPager
	resp := &http.Response{StatusCode: 404, Request: httptest.NewRequest("GET", "/", nil)}
	if e.intercepts(resp) {
		t.Fatal("nil 不拦截")
	}
}

// 页面库接口：新增时清理并返回报告、列表不含内容、重名与数量上限、被引用时不能删除、监听引用不存在的页面时拒绝保存。
func TestErrorPagesAPI(t *testing.T) {
	m, do := newCopyModule(t)
	code, out := do("POST", "/api/webservice/errorpages", ErrorPage{Name: "品牌", Source: ErrorPageInline, HTML: `<h1 onclick="x()">{code}</h1><script>alert(1)</script>`})
	if code != 200 {
		t.Fatalf("新增: %d %v", code, out)
	}
	page := out["page"].(map[string]any)
	key := page["key"].(string)
	rep := out["report"].(map[string]any)
	if len(rep["removed"].([]any)) < 2 || page["size"].(float64) == 0 {
		t.Fatalf("应报告清理内容: %v", out)
	}
	if got := m.cfg.ErrorPages[0].HTML; strings.Contains(got, "<script") || strings.Contains(got, "onclick") {
		t.Fatalf("保存的应是清理后的页面: %s", got)
	}
	if code, out = do("POST", "/api/webservice/errorpages", ErrorPage{Name: "品牌", Source: ErrorPageInline, HTML: "<h1>x</h1>"}); code != 400 {
		t.Fatalf("重名应拒绝: %d %v", code, out)
	}
	for _, bad := range []ErrorPage{
		{Name: "", Source: ErrorPageInline, HTML: "<h1>x</h1>"},
		{Name: "a", Source: "url", HTML: "<h1>x</h1>"},
		{Name: "a", Source: ErrorPageInline, HTML: "plain text"},
		{Name: "a", Source: ErrorPageFile, File: "/etc/passwd"},
	} {
		if code, _ := do("POST", "/api/webservice/errorpages", bad); code != 400 {
			t.Errorf("%+v 应拒绝", bad)
		}
	}
	// 列表不含页面内容
	rec := httptest.NewRecorder()
	m.handleListErrorPages(rec, httptest.NewRequest("GET", "/", nil))
	if strings.Contains(rec.Body.String(), "<h1") || !strings.Contains(rec.Body.String(), key) {
		t.Fatalf("列表: %s", rec.Body.String())
	}
	// 读取单个页面含内容；修改保留 key
	if code, out = do("GET", "/api/webservice/errorpages/"+key, nil); code != 200 || !strings.Contains(out["html"].(string), "<h1>") {
		t.Fatalf("读取: %d %v", code, out)
	}
	if code, out = do("PUT", "/api/webservice/errorpages/"+key, ErrorPage{Name: "品牌2", Source: ErrorPageInline, HTML: "<h1>{title}</h1>"}); code != 200 || m.cfg.ErrorPages[0].Key != key || m.cfg.ErrorPages[0].Name != "品牌2" {
		t.Fatalf("修改: %d %v", code, out)
	}
	if code, _ = do("PUT", "/api/webservice/errorpages/none", ErrorPage{Name: "x", Source: ErrorPageInline, HTML: "<h1>x</h1>"}); code != 404 {
		t.Fatalf("修改不存在的页面: %d", code)
	}

	// 监听引用不存在的页面时拒绝保存，引用存在的页面可以保存
	l := Listener{Name: "主", Port: 18090, Sites: []Site{{Enable: true, Domains: []string{"a.test"}, Type: TypeRedirect, RedirectTo: "https://x.example.com",
		ErrorPage: ErrorPageOptions{Style: ErrorStyleCustom, Page: "missing"}}}}
	if code, out = do("POST", "/api/webservice/listeners", l); code != 400 {
		t.Fatalf("引用不存在的页面应拒绝: %d %v", code, out)
	}
	l.Default.ErrorPage = ErrorPageOptions{Style: ErrorStyleCustom, Page: "missing"}
	l.Sites[0].ErrorPage.Page = key
	if code, out = do("POST", "/api/webservice/listeners", l); code != 400 {
		t.Fatalf("默认站点引用不存在的页面应拒绝: %d %v", code, out)
	}
	l.Default.ErrorPage.Page = key
	if code, out = do("POST", "/api/webservice/listeners", l); code != 200 {
		t.Fatalf("引用存在的页面: %d %v", code, out)
	}
	// 被引用时不能删除，并列出引用方
	code, out = do("DELETE", "/api/webservice/errorpages/"+key, nil)
	if code != 400 || !strings.Contains(out["error"].(string), "主 / a.test") || !strings.Contains(out["error"].(string), "默认站点") {
		t.Fatalf("被引用时应拒绝删除: %d %v", code, out)
	}
	rec = httptest.NewRecorder()
	m.handleListErrorPages(rec, httptest.NewRequest("GET", "/", nil))
	if !strings.Contains(rec.Body.String(), "usedBy") {
		t.Fatalf("列表应带引用方: %s", rec.Body.String())
	}
	m.mu.Lock()
	m.cfg.Listeners = nil
	m.mu.Unlock()
	if code, out = do("DELETE", "/api/webservice/errorpages/"+key, nil); code != 200 || len(m.cfg.ErrorPages) != 0 {
		t.Fatalf("删除: %d %v", code, out)
	}
	if _, ok := m.auth.errPages.page(key); ok {
		t.Fatal("删除后运行时页面库应同步移除")
	}
	// 数量上限
	for i := range maxErrorPages {
		if code, out = do("POST", "/api/webservice/errorpages", ErrorPage{Name: "p" + string(rune('A'+i)), Source: ErrorPageInline, HTML: "<h1>x</h1>"}); code != 200 {
			t.Fatalf("第 %d 个: %d %v", i, code, out)
		}
	}
	if code, _ = do("POST", "/api/webservice/errorpages", ErrorPage{Name: "over", Source: ErrorPageInline, HTML: "<h1>x</h1>"}); code != 400 {
		t.Fatalf("超过上限应拒绝: %d", code)
	}
}

// 检查与预览接口：返回报告与替换了示例值的预览，不修改配置；文件页面检查文件内容。
func TestErrorPageCheckAndPreview(t *testing.T) {
	m, do := newCopyModule(t)
	code, out := do("POST", "/api/webservice/errorpages/check", ErrorPageCheckRequest{Source: ErrorPageInline, HTML: `<h1>{code} {host}</h1><img src="https://cdn.example.com/x.png">`, Code: 404})
	if code != 200 || !strings.Contains(out["preview"].(string), "404 "+errorPageSampleHost) || !strings.Contains(out["preview"].(string), `http-equiv="Content-Security-Policy"`) {
		t.Fatalf("检查: %d %v", code, out)
	}
	warns := out["report"].(map[string]any)["warnings"].([]any)
	if len(warns) == 0 || len(m.cfg.ErrorPages) != 0 {
		t.Fatalf("应有提示且不保存: %v", out)
	}
	p := filepath.Join(t.TempDir(), "e.html")
	writeFile(t, p, "<h1>from-file {code}</h1>")
	if code, out = do("POST", "/api/webservice/errorpages/check", ErrorPageCheckRequest{Source: ErrorPageFile, File: p}); code != 200 || !strings.Contains(out["preview"].(string), "from-file 502") {
		t.Fatalf("文件检查: %d %v", code, out)
	}
	if code, _ = do("POST", "/api/webservice/errorpages/check", ErrorPageCheckRequest{Source: ErrorPageFile, File: p + ".missing.html"}); code != 400 {
		t.Fatalf("不存在的文件应报错: %d", code)
	}
	// 样式预览：默认样式与 10 种内置风格
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/webservice/errorpages/styles?code=404", nil)
	m.handleErrorStyles(rec, req)
	if n := strings.Count(rec.Body.String(), `"id":`); n != 11 {
		t.Fatalf("应有 11 个样式预览，实际 %d", n)
	}
	// 自定义页面预览；文件被删除后返回原因
	code, out = do("POST", "/api/webservice/errorpages", ErrorPage{Name: "文件页", Source: ErrorPageFile, File: p})
	if code != 200 {
		t.Fatalf("新增文件页面: %d %v", code, out)
	}
	key := out["page"].(map[string]any)["key"].(string)
	if code, out = do("GET", "/api/webservice/errorpages/"+key+"/preview?code=503", nil); code != 200 || !strings.Contains(out["html"].(string), "from-file 503") {
		t.Fatalf("预览: %d %v", code, out)
	}
	_ = os.Remove(p)
	if code, out = do("GET", "/api/webservice/errorpages/"+key+"/preview", nil); code != 400 || !strings.Contains(out["error"].(string), "不存在") {
		t.Fatalf("文件删除后预览应报错: %d %v", code, out)
	}
	rec = httptest.NewRecorder()
	m.handleListErrorPages(rec, httptest.NewRequest("GET", "/", nil))
	if !strings.Contains(rec.Body.String(), `"error":`) {
		t.Fatalf("列表应显示文件不可用: %s", rec.Body.String())
	}
}

// 批量设置错误页：规范化后与现有设置比较，引用不存在的页面时整体拒绝。
func TestBatchSetErrorPage(t *testing.T) {
	m := batchModule(t)
	// batchModule 的站点都是固定文本，没有错误页；改成重定向站点
	m.cfg.Listeners[0].Sites[0].Type, m.cfg.Listeners[0].Sites[0].RedirectTo = TypeRedirect, "https://x.example.com"
	m.cfg.Listeners[0].Sites[0].ErrorPage = ErrorPageOptions{Intercept: true, Codes: "404"}
	opt := &ErrorPageStyle{Style: " blueprint "}
	code, res, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "a", "l1", "default"), Action: BatchSet, Options: &SiteBatchOptions{ErrorPage: opt}})
	if code != 200 || res.Changed != 2 || m.siteOf(t, "l1", "a").ErrorPage.Style != "blueprint" || m.siteOf(t, "l1", "default").ErrorPage.Style != "blueprint" {
		t.Fatalf("批量设置: %d %s", code, body)
	}
	if code, res, body = m.batch(t, SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchSet, Options: &SiteBatchOptions{ErrorPage: &ErrorPageStyle{Style: "blueprint"}}}); code != 200 || res.Unchanged != 1 {
		t.Fatalf("相同设置应计为未变化: %d %s", code, body)
	}
	if code, _, body = m.batch(t, SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchSet, Options: &SiteBatchOptions{ErrorPage: &ErrorPageStyle{Style: ErrorStyleCustom, Page: "none"}}}); code != 400 {
		t.Fatalf("引用不存在的页面应拒绝: %d %s", code, body)
	}
	if code, _, body = m.batch(t, SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchSet, Options: &SiteBatchOptions{ErrorPage: &ErrorPageStyle{Style: "nope"}}}); code != 400 {
		t.Fatalf("未知样式应拒绝: %d %s", code, body)
	}
	// 批量设置样式不影响站点各自的拦截设置
	if ep := m.siteOf(t, "l1", "a").ErrorPage; !ep.Intercept || ep.Codes != "404" {
		t.Fatalf("拦截设置应保留: %+v", ep)
	}
	if m.siteOf(t, "l1", "a").ErrorPage.Style != "blueprint" {
		t.Fatal("失败的批量操作不应修改站点")
	}
}

// statusProblem 为常见状态码给出标题，其余按 4xx/5xx 归类。
func TestStatusProblem(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	if p := statusProblem(404, r); p.Title != "页面不存在" || p.Home {
		t.Fatalf("首页的 404 不显示返回首页: %+v", p)
	}
	if p := statusProblem(599, httptest.NewRequest("GET", "/a", nil)); p.Title != "服务器错误" || !p.Home {
		t.Fatalf("599: %+v", p)
	}
	if p := statusProblem(451, nil); p.Title != "请求未能完成" {
		t.Fatalf("451: %+v", p)
	}
}

// 拦截时按访客的原始请求生成页面与判断是否拦截：后端地址带路径前缀、使用后端 Host、
// 站点改写了 Accept 时，页面中的域名与路径仍是访客看到的地址，后端内部路径不会显示出来。
func TestInterceptUsesVisitorRequest(t *testing.T) {
	be := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(404) }))
	t.Cleanup(be.Close)
	site := Site{Enable: true, Domains: []string{"app.test"}, Type: TypeProxy, Targets: []string{be.URL + "/internal-base"}, UseTargetHost: true,
		RequestHeaders: []string{"Accept: application/json"},
		ErrorPage:      ErrorPageOptions{Style: ErrorStyleCustom, Page: "p", Intercept: true, Codes: "404"}}
	addr, r := startRunner(t, Listener{Sites: []Site{site}}, nil, nil)
	r.auth.errPages.set([]ErrorPage{{Key: "p", Name: "p", Source: ErrorPageInline, HTML: `<p id="v">{host}{path}</p>`}})
	resp, body := get(t, http.DefaultClient, addr, "app.test", "/visitor", htmlAccept)
	if resp.StatusCode != 404 || !strings.Contains(body, `<p id="v">app.test/visitor</p>`) || strings.Contains(body, "internal-base") {
		t.Fatalf("应显示访客地址: %d %s", resp.StatusCode, body)
	}
}

// 文件内容无效时缓存失败结果：文件不变时不再重新读取，文件变化后重新读取；文件被删除后按间隔重试。
func TestErrorPageStoreCachesFailure(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "e.html")
	writeFile(t, p, "plain text, not html")
	s := newErrorPageStore()
	s.set([]ErrorPage{{Key: "f", Name: "f", Source: ErrorPageFile, File: p}})
	if _, ok := s.page("f"); ok {
		t.Fatal("无效内容应不可用")
	}
	sp := s.pages["f"]
	first := sp.failedAt
	time.Sleep(10 * time.Millisecond)
	if _, ok := s.page("f"); ok || !sp.failedAt.Equal(first) {
		t.Fatal("文件未变时应直接使用缓存的失败结果")
	}
	writeFile(t, p, "<h1>fixed {code}</h1>")
	_ = os.Chtimes(p, time.Now(), time.Now().Add(time.Minute))
	if page, ok := s.page("f"); !ok || !strings.Contains(page, "fixed") {
		t.Fatal("文件变化后应重新读取")
	}
	// 内容与修改时间、大小都相同、但换成了另一个文件（cp -p 一类的替换）时同样重新读取
	repl := filepath.Join(dir, "r.html")
	writeFile(t, repl, "<h1>fixeD {code}</h1>")
	st, _ := os.Stat(p)
	_ = os.Chtimes(repl, st.ModTime(), st.ModTime())
	if err := os.Rename(repl, p); err != nil {
		t.Fatal(err)
	}
	if page, ok := s.page("f"); !ok || !strings.Contains(page, "fixeD") {
		t.Fatal("文件被替换后应重新读取")
	}
	// 文件被删除：重试间隔内不再访问文件，failedAt 保持不变；文件恢复后立即可用
	_ = os.Remove(p)
	if _, ok := s.page("f"); ok {
		t.Fatal("删除后应不可用")
	}
	missingAt := sp.failedAt
	time.Sleep(10 * time.Millisecond)
	if _, ok := s.page("f"); ok || !sp.failedAt.Equal(missingAt) {
		t.Fatal("重试间隔内不应重新读取")
	}
	writeFile(t, p, "<h1>back</h1>")
	if page, ok := s.page("f"); !ok || !strings.Contains(page, "back") {
		t.Fatal("文件恢复后应立即可用")
	}
}

// 未开启拦截时写错的状态码直接丢弃（界面上没有输入框可改）；断开连接与固定文本站点清空错误页设置，
// 不再引用自定义页面；批量设置跳过这两类站点；默认站点在引用方列表中用标记表示。
func TestErrorPageCleanups(t *testing.T) {
	o := ErrorPageOptions{Codes: "abc"}
	if err := o.validate(); err != nil || o.Codes != "" {
		t.Fatalf("未开启拦截时应丢弃无效状态码: %v %+v", err, o)
	}
	o = ErrorPageOptions{Codes: "5xx"}
	if err := o.validate(); err != nil || o.Codes != "500-599" {
		t.Fatalf("有效的状态码保留: %+v", o)
	}
	l := Listener{Port: 80, Sites: []Site{
		{Domains: []string{"a.test"}, Type: TypeText, ErrorPage: ErrorPageOptions{Style: ErrorStyleCustom, Page: "x"}},
		{Domains: []string{"b.test"}, Type: TypeClose, ErrorPage: ErrorPageOptions{Style: "neon"}},
	}, Default: Site{Type: TypeClose, ErrorPage: ErrorPageOptions{Style: "neon"}}}
	if err := l.Validate(); err != nil {
		t.Fatal(err)
	}
	if l.Sites[0].ErrorPage != (ErrorPageOptions{}) || l.Sites[1].ErrorPage != (ErrorPageOptions{}) || l.Default.ErrorPage.Style != "neon" {
		t.Fatalf("清理结果: %+v %+v %+v", l.Sites[0].ErrorPage, l.Sites[1].ErrorPage, l.Default.ErrorPage)
	}

	m := batchModule(t)
	code, res, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "a", "l1", "c"), Action: BatchSet, Options: &SiteBatchOptions{ErrorPage: &ErrorPageStyle{Style: "neon"}}})
	if code != 200 || res.Changed != 0 || res.Unchanged != 2 || m.siteOf(t, "l1", "a").ErrorPage.Style != "" {
		t.Fatalf("固定文本站点应跳过: %d %s", code, body)
	}

	m.cfg.ErrorPages = []ErrorPage{{Key: "p", Name: "p", Source: ErrorPageInline, HTML: "<h1>x</h1>"}}
	m.cfg.Listeners[0].Default.ErrorPage = ErrorPageOptions{Style: ErrorStyleCustom, Page: "p"}
	if users := m.errorPageUsersLocked("p"); len(users) != 1 || users[0] != "main / "+defaultSiteMarker {
		t.Fatalf("引用方: %v", users)
	}
}
