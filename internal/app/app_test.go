package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"github.com/wangwanjie/gately/internal/backup"
	"io"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/i18n"
	"github.com/wangwanjie/gately/internal/luckyimport"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/portforward"
	"github.com/wangwanjie/gately/internal/modules/ssl"
	"github.com/wangwanjie/gately/internal/webauthn"
	"github.com/wangwanjie/gately/internal/webauthn/webauthntest"
)

// newTestServer 启动带端口转发模块的测试实例，并以已知密码登录。
func newTestServer(t *testing.T) (*httptest.Server, *http.Client) {
	t.Helper()
	dir := t.TempDir()
	a, err := New(Options{ConfigDir: dir, Modules: []module.Module{portforward.New()}})
	if err != nil {
		t.Fatal(err)
	}
	a.logger = log.New(io.Discard, "", 0)
	// 换成不落盘的账号，初始密码为默认密码，便于测试登录
	a.auth = auth.NewManager(auth.Account{}, func(auth.Account) error { return nil })
	if _, err := a.auth.EnsureInitialized(); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	return srv, client
}

func doJSON(t *testing.T, c *http.Client, method, url string, body any) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, r)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

func TestAPIRequiresLogin(t *testing.T) {
	srv, c := newTestServer(t)
	if code, _ := doJSON(t, c, "GET", srv.URL+"/api/status", nil); code != http.StatusUnauthorized {
		t.Fatalf("未登录应返回 401，实际 %d", code)
	}
	if code, _ := doJSON(t, c, "GET", srv.URL+"/api/portforward/rules", nil); code != http.StatusUnauthorized {
		t.Fatalf("模块接口未登录应返回 401，实际 %d", code)
	}
	if code, _ := doJSON(t, c, "POST", srv.URL+"/api/login", map[string]string{"username": "admin", "password": "x"}); code != http.StatusUnauthorized {
		t.Fatalf("错误密码应返回 401，实际 %d", code)
	}
}

func TestPortForwardCRUDAndImport(t *testing.T) {
	dir := t.TempDir()
	pf := portforward.New()
	a, err := New(Options{ConfigDir: dir, Modules: []module.Module{pf}})
	if err != nil {
		t.Fatal(err)
	}
	a.logger = log.New(io.Discard, "", 0)
	if err := a.auth.ChangeCredentials(mustInitialPassword(t, a), "admin", "test-password"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}

	if code, body := doJSON(t, c, "POST", srv.URL+"/api/login", map[string]string{"username": "admin", "password": "test-password"}); code != 200 {
		t.Fatalf("登录失败 %d %s", code, body)
	}

	port := freeTCPPort(t)
	rule := map[string]any{
		"name": "web", "enable": false, "protocols": []string{"tcp4"},
		"listenAddress": "127.0.0.1", "listenPorts": strconv.Itoa(port),
		"targetAddresses": []string{"127.0.0.1"}, "targetPorts": "80",
	}
	code, body := doJSON(t, c, "POST", srv.URL+"/api/portforward/rules", rule)
	if code != 200 {
		t.Fatalf("创建失败 %d %s", code, body)
	}
	var created portforward.Rule
	_ = json.Unmarshal(body, &created)
	if created.Key == "" {
		t.Fatal("应生成 key")
	}

	// 非法规则应被拒绝
	bad := map[string]any{"protocols": []string{"icmp"}, "listenPorts": "1", "targetAddresses": []string{"x"}}
	if code, _ := doJSON(t, c, "POST", srv.URL+"/api/portforward/rules", bad); code != http.StatusBadRequest {
		t.Fatalf("非法规则应返回 400，实际 %d", code)
	}

	// 启用后应处于运行状态
	if code, body := doJSON(t, c, "PUT", srv.URL+"/api/portforward/rules/"+created.Key+"/enable", map[string]bool{"enable": true}); code != 200 {
		t.Fatalf("启用失败 %d %s", code, body)
	}
	// 模块未经 Run 启动时没有运行上下文，这里只验证配置被持久化
	_, body = doJSON(t, c, "GET", srv.URL+"/api/portforward/rules", nil)
	var views []portforward.RuleView
	_ = json.Unmarshal(body, &views)
	if len(views) != 1 || !views[0].Enable {
		t.Fatalf("列表异常: %s", body)
	}

	// 导入明文 JSON 形式的 Lucky 配置（预览）
	lucky := `{"PortForwardsRuleList":[{"Name":"ssh","Enable":false,"ForwardTypes":["tcp"],"ListenPorts":"6022","TargetAddressList":["192.168.1.2"],"TargetPorts":"22","Options":{}}]}`
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("files", "lucky_portforward.lkcf")
	_, _ = fw.Write([]byte(lucky))
	_ = mw.Close()
	req, _ := http.NewRequest("POST", srv.URL+"/api/import/lucky?dryRun=1", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !bytes.Contains(body, []byte(`"imported":1`)) {
		t.Fatalf("导入预览异常 %d %s", resp.StatusCode, body)
	}

	if code, _ := doJSON(t, c, "DELETE", srv.URL+"/api/portforward/rules/"+created.Key, nil); code != 200 {
		t.Fatalf("删除失败 %d", code)
	}
}

// mustInitialPassword 重置为已知的初始密码并返回。
func mustInitialPassword(t *testing.T, a *App) string {
	t.Helper()
	a.auth = auth.NewManager(auth.Account{}, func(acc auth.Account) error { return a.store.Save(accountName, acc) })
	p, err := a.auth.EnsureInitialized()
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestCrossOriginRejected(t *testing.T) {
	srv, c := newTestServer(t)
	req, _ := http.NewRequest("POST", srv.URL+"/api/login", strings.NewReader(`{"username":"a","password":"b"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://evil.example:8080")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("跨源请求应被拒绝: %d", resp.StatusCode)
	}
	// 简单请求（text/plain）不能绕过 JSON 要求
	req, _ = http.NewRequest("POST", srv.URL+"/api/login", strings.NewReader(`{"username":"a","password":"b"}`))
	req.Header.Set("Content-Type", "text/plain")
	resp, _ = c.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("非 JSON 请求体应被拒绝: %d", resp.StatusCode)
	}
	// 同源请求放行
	req, _ = http.NewRequest("POST", srv.URL+"/api/logout", nil)
	req.Header.Set("Origin", srv.URL)
	resp, _ = c.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("同源请求应放行: %d", resp.StatusCode)
	}
}

// loginApp 创建带端口转发模块的实例并以已知密码登录，返回实例、服务地址和客户端。
func loginApp(t *testing.T) (*App, *httptest.Server, *http.Client) {
	t.Helper()
	dir := t.TempDir()
	a, err := New(Options{ConfigDir: dir, Modules: []module.Module{portforward.New()}})
	if err != nil {
		t.Fatal(err)
	}
	a.logger = log.New(io.Discard, "", 0)
	if err := a.auth.ChangeCredentials(mustInitialPassword(t, a), "admin", "test-password"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if code, body := doJSON(t, c, "POST", srv.URL+"/api/login", map[string]string{"username": "admin", "password": "test-password"}); code != 200 {
		t.Fatalf("登录失败 %d %s", code, body)
	}
	return a, srv, c
}

func uploadFiles(t *testing.T, c *http.Client, url, field string, files map[string][]byte) (int, []byte) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for name, data := range files {
		fw, _ := mw.CreateFormFile(field, name)
		_, _ = fw.Write(data)
	}
	_ = mw.Close()
	req, _ := http.NewRequest("POST", url, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(content))
	}
	_ = zw.Close()
	return buf.Bytes()
}

func TestImportZipKeepEnabledAndDedupe(t *testing.T) {
	_, srv, c := loginApp(t)
	port := freeTCPPort(t)
	pf := `{"PortForwardsRuleList":[{"Name":"web","Enable":true,"ForwardTypes":["tcp4"],"ListenAddress":"127.0.0.1","ListenPorts":"` + strconv.Itoa(port) + `","TargetAddressList":["127.0.0.1"],"TargetPorts":"80","Options":{}}]}`
	base := `{"AdminWebListenPort":16601,"AllowInternetaccess":true,"MaxConsecutiveLoginFailures":8,"TimeZone":"Asia/Shanghai","SafeURL":"secret"}`
	archive := zipOf(t, map[string]string{
		"lucky_conf/lucky_portforward.lkcf": pf,
		"lucky_conf/lucky_base.lkcf":        base,
		"lucky_conf/readme.txt":             "ignored",
		"__MACOSX/._lucky_base.lkcf":        "junk",
	})
	code, body := uploadFiles(t, c, srv.URL+"/api/import/lucky?enable=1", "files", map[string][]byte{"backup.zip": archive})
	if code != 200 || !bytes.Contains(body, []byte(`"imported":1`)) || bytes.Contains(body, []byte(`"error"`)) {
		t.Fatalf("zip 导入失败 %d %s", code, body)
	}
	_, body = doJSON(t, c, "GET", srv.URL+"/api/portforward/rules", nil)
	var views []portforward.RuleView
	_ = json.Unmarshal(body, &views)
	if len(views) != 1 || !views[0].Enable {
		t.Fatalf("开启“保留启用状态”后规则应为启用: %s", body)
	}
	// 基础设置：登录失败上限与时区被迁移，管理端口与安全入口不迁移
	_, body = doJSON(t, c, "GET", srv.URL+"/api/system/settings", nil)
	var s settingsView
	_ = json.Unmarshal(body, &s)
	if s.MaxLoginFailures != 8 || s.TimeZone != "Asia/Shanghai" || s.SafePath != "" || s.Listen != DefaultListen {
		t.Fatalf("基础设置迁移错误: %s", body)
	}
	// 重复导入应跳过同名规则
	code, body = uploadFiles(t, c, srv.URL+"/api/import/lucky", "files", map[string][]byte{"lucky_portforward.lkcf": []byte(pf)})
	if code != 200 || !bytes.Contains(body, []byte("已存在同名条目")) {
		t.Fatalf("重复导入应跳过: %s", body)
	}
	_, body = doJSON(t, c, "GET", srv.URL+"/api/portforward/rules", nil)
	_ = json.Unmarshal(body, &views)
	if len(views) != 1 {
		t.Fatalf("重复导入不应产生重复规则: %d", len(views))
	}
	// 压缩包中没有配置文件时报错
	_, body = uploadFiles(t, c, srv.URL+"/api/import/lucky", "files", map[string][]byte{"empty.zip": zipOf(t, map[string]string{"a.txt": "x"})})
	if !bytes.Contains(body, []byte("没有找到")) {
		t.Fatalf("空压缩包应提示: %s", body)
	}
}

func TestSettingsSafePathAndToken(t *testing.T) {
	_, srv, c := loginApp(t)
	_, body := doJSON(t, c, "GET", srv.URL+"/api/system/settings", nil)
	var s settingsView
	_ = json.Unmarshal(body, &s)
	s.SafePath = "my-entry"
	s.SessionHours = 24
	if code, body := doJSON(t, c, "PUT", srv.URL+"/api/system/settings", s); code != 200 {
		t.Fatalf("保存设置失败 %d %s", code, body)
	}
	_, body = doJSON(t, c, "GET", srv.URL+"/api/system/settings", nil)
	_ = json.Unmarshal(body, &s)
	if s.SafePath != "/my-entry" || s.SessionHours != 24 {
		t.Fatalf("设置未生效: %s", body)
	}
	// 通过令牌接口创建只读状态令牌
	_, body = doJSON(t, c, "POST", srv.URL+"/api/tokens", map[string]any{"name": "测试", "scopes": []string{"status:read"}})
	var created struct {
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(body, &created)
	if !strings.HasPrefix(created.Secret, "gtl_") {
		t.Fatalf("创建令牌失败: %s", body)
	}

	// 新浏览器：未经过安全入口时首页与登录接口均返回 404
	jar, _ := cookiejar.New(nil)
	fresh := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if code, _ := doJSON(t, fresh, "GET", srv.URL+"/", nil); code != 404 {
		t.Fatalf("未经过安全入口应返回 404，实际 %d", code)
	}
	if code, _ := doJSON(t, fresh, "POST", srv.URL+"/api/login", map[string]string{"username": "admin", "password": "test-password"}); code != 404 {
		t.Fatalf("未经过安全入口时登录接口应返回 404，实际 %d", code)
	}
	if code, _ := doJSON(t, fresh, "GET", srv.URL+"/my-entry", nil); code != 302 {
		t.Fatalf("访问安全入口应跳转，实际 %d", code)
	}
	if code, _ := doJSON(t, fresh, "POST", srv.URL+"/api/login", map[string]string{"username": "admin", "password": "test-password"}); code != 200 {
		t.Fatalf("经过安全入口后应能登录，实际 %d", code)
	}

	// API 令牌可直接调用接口（不需要安全入口与会话）
	req, _ := http.NewRequest("GET", srv.URL+"/api/system/info", nil)
	req.Header.Set("Authorization", "Bearer "+created.Secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("API 令牌应可访问，实际 %d", resp.StatusCode)
	}
	req.Header.Set("Authorization", "Bearer wrong-token-wrong-token")
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("错误令牌应被安全入口拦截，实际 %d", resp.StatusCode)
	}

	// 非法设置被拒绝
	s.SafePath = "/api"
	if code, _ := doJSON(t, c, "PUT", srv.URL+"/api/system/settings", s); code != 400 {
		t.Fatalf("非法安全入口应被拒绝，实际 %d", code)
	}
}

func TestBackupRestore(t *testing.T) {
	a, srv, c := loginApp(t)
	rule := map[string]any{"name": "web", "enable": false, "protocols": []string{"tcp4"}, "listenAddress": "127.0.0.1",
		"listenPorts": strconv.Itoa(freeTCPPort(t)), "targetAddresses": []string{"127.0.0.1"}, "targetPorts": "80"}
	if code, body := doJSON(t, c, "POST", srv.URL+"/api/portforward/rules", rule); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	resp, err := c.Get(srv.URL + "/api/system/backup")
	if err != nil {
		t.Fatal(err)
	}
	backup, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	zr, err := zip.NewReader(bytes.NewReader(backup), int64(len(backup)))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	if !names["account.json"] || !names["portforward.json"] {
		t.Fatalf("备份内容不完整: %v", names)
	}
	// 恢复会触发重启信号
	code, body := uploadFiles(t, c, srv.URL+"/api/system/restore", "file", map[string][]byte{"b.zip": backup})
	if code != 200 || !bytes.Contains(body, []byte(`"restored"`)) {
		t.Fatalf("恢复失败 %d %s", code, body)
	}
	select {
	case <-a.restartCh:
	case <-time.After(3 * time.Second):
		t.Fatal("恢复后应请求重启")
	}
	// 缺少账号文件的压缩包被拒绝
	code, _ = uploadFiles(t, c, srv.URL+"/api/system/restore", "file", map[string][]byte{"b.zip": zipOf(t, map[string]string{"x.json": "{}"})})
	if code != 400 {
		t.Fatalf("非 Gately 备份应被拒绝，实际 %d", code)
	}
}

func TestIsPrivate(t *testing.T) {
	for ip, want := range map[string]bool{"192.168.1.2": true, "10.0.0.1": true, "100.64.0.3": true, "127.0.0.1": true, "fd00::1": true, "8.8.8.8": false, "2408:8000::1": false} {
		if isPrivate(ip) != want {
			t.Errorf("isPrivate(%s) != %v", ip, want)
		}
	}
}

// 重启时应先停止全部模块（含有后台协程的证书模块），再重新执行进程
func TestRunRestartStopsModules(t *testing.T) {
	called := make(chan struct{})
	old := restartProcess
	restartProcess = func() { close(called) }
	defer func() { restartProcess = old }()
	certs := ssl.New()
	a, err := New(Options{ConfigDir: t.TempDir(), Listen: "127.0.0.1:" + strconv.Itoa(freeTCPPort(t)), Modules: []module.Module{certs, portforward.New()}})
	if err != nil {
		t.Fatal(err)
	}
	a.logger = log.New(io.Discard, "", 0)
	done := make(chan error, 1)
	go func() { done <- a.Run(context.Background()) }()
	time.Sleep(300 * time.Millisecond)
	a.scheduleRestart()
	select {
	case <-called:
	case <-time.After(15 * time.Second):
		t.Fatal("重启卡住：模块未能停止")
	}
	<-done
}

// TestRestoreEncryptedBackup 验证加密备份缺口令返回 422，口令正确时可恢复。
func TestRestoreEncryptedBackup(t *testing.T) {
	a, srv, c := loginApp(t)
	enc, err := backup.Encrypt(zipOf(t, map[string]string{"account.json": `{"user":"x"}`}), "pw")
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := uploadFiles(t, c, srv.URL+"/api/system/restore", "file", map[string][]byte{"b.zip.enc": enc}); code != 422 {
		t.Fatalf("缺少口令应返回 422，实际 %d", code)
	}
	post := func(password string) int {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", "b.zip.enc")
		_, _ = fw.Write(enc)
		_ = mw.WriteField("password", password)
		_ = mw.Close()
		req, _ := http.NewRequest("POST", srv.URL+"/api/system/restore", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := post("bad"); code != 400 {
		t.Fatalf("错误口令应返回 400，实际 %d", code)
	}
	if code := post("pw"); code != 200 {
		t.Fatalf("正确口令应恢复成功，实际 %d", code)
	}
	select {
	case <-a.restartCh:
	case <-time.After(3 * time.Second):
		t.Fatal("恢复后应请求重启")
	}
}

func TestDiagnosticsBundle(t *testing.T) {
	a, srv, c := loginApp(t)
	rule := map[string]any{"name": "家里的NAS-secret-name", "enable": false, "protocols": []string{"tcp4"}, "listenAddress": "127.0.0.1",
		"listenPorts": strconv.Itoa(freeTCPPort(t)), "targetAddresses": []string{"192.168.77.88"}, "targetPorts": "80"}
	if code, body := doJSON(t, c, "POST", srv.URL+"/api/portforward/rules", rule); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	a.sysLogs.Add("warn", "ddns", "更新 nas.myhome-secret.com 到 203.0.113.9 失败 password=hunter2")

	resp, err := c.Get(srv.URL + "/api/system/diagnostics")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Disposition"), "gately-diagnostics-") {
		t.Fatalf("下载诊断包失败 %d", resp.StatusCode)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	var all strings.Builder
	for _, f := range zr.File {
		names[f.Name] = true
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		all.Write(b)
	}
	for _, want := range []string{"README.txt", "summary.json", "modules.json", "config/portforward.json", "logs/system.log", "logs/portforward.log"} {
		if !names[want] {
			t.Errorf("诊断包缺少 %s: %v", want, names)
		}
	}
	for name := range names {
		if strings.Contains(name, "account") || strings.Contains(name, "tokens") {
			t.Errorf("诊断包不应包含 %s", name)
		}
	}
	content := all.String()
	for _, leak := range []string{"secret-name", "myhome-secret", "203.0.113.9", "192.168.77.88", "hunter2", "test-password", "authSecret\": \"a", a.opts.ConfigDir, "$2a$"} {
		if strings.Contains(content, leak) {
			t.Errorf("诊断包泄露了 %q", leak)
		}
	}

	// 不含日志
	resp, _ = c.Get(srv.URL + "/api/system/diagnostics?logs=0")
	data, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	zr, _ = zip.NewReader(bytes.NewReader(data), int64(len(data)))
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "logs/") {
			t.Errorf("logs=0 时不应包含 %s", f.Name)
		}
	}

	// 预览
	code, body := doJSON(t, c, "GET", srv.URL+"/api/system/diagnostics?preview=1", nil)
	if code != 200 || !bytes.Contains(body, []byte(`"summary.json"`)) {
		t.Fatalf("预览失败 %d %s", code, body)
	}
}

// 审计日志记录登录与变更，配置历史支持比较与回滚
func TestAuditAndHistory(t *testing.T) {
	a, srv, c := loginApp(t)
	mk := func(name string) map[string]any {
		return map[string]any{"name": name, "enable": false, "protocols": []string{"tcp4"}, "listenAddress": "127.0.0.1",
			"listenPorts": strconv.Itoa(freeTCPPort(t)), "targetAddresses": []string{"127.0.0.1"}, "targetPorts": "80"}
	}
	for _, n := range []string{"rule-a", "rule-b"} {
		if code, body := doJSON(t, c, "POST", srv.URL+"/api/portforward/rules", mk(n)); code != 200 {
			t.Fatalf("%d %s", code, body)
		}
	}

	code, body := doJSON(t, c, "GET", srv.URL+"/api/audit", nil)
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	var res struct {
		Entries []struct {
			Category, Module, Method, Actor string
		}
		Total int
	}
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatal(err)
	}
	var logins, changes int
	for _, e := range res.Entries {
		switch {
		case e.Category == "login":
			logins++
		case e.Category == "change" && e.Module == "portforward" && e.Method == "POST":
			changes++
		}
	}
	if logins < 1 || changes != 2 {
		t.Fatalf("审计记录不符: logins=%d changes=%d %s", logins, changes, body)
	}

	// 两次保存之间产生了一个历史版本（只含 rule-a）
	code, body = doJSON(t, c, "GET", srv.URL+"/api/history", nil)
	var files []struct {
		Name     string
		Versions []struct{ ID string }
	}
	if err := json.Unmarshal(body, &files); err != nil || code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	var id string
	for _, f := range files {
		if f.Name == "portforward" && len(f.Versions) > 0 {
			id = f.Versions[0].ID
		}
	}
	if id == "" {
		t.Fatalf("应有 portforward 的历史版本: %s", body)
	}
	code, body = doJSON(t, c, "GET", srv.URL+"/api/history/portforward/"+id+"/diff", nil)
	if code != 200 || !bytes.Contains(body, []byte("rule-b")) {
		t.Fatalf("差异应包含新增的 rule-b: %d %s", code, body)
	}
	if code, _ = doJSON(t, c, "GET", srv.URL+"/api/history/portforward/bad-id/diff", nil); code != 404 {
		t.Fatalf("非法版本应返回 404，实际 %d", code)
	}
	if code, body = doJSON(t, c, "POST", srv.URL+"/api/history/portforward/"+id+"/restore", nil); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	cur, _ := a.store.ReadRaw("portforward")
	if bytes.Contains(cur, []byte("rule-b")) {
		t.Fatal("回滚后不应再包含 rule-b")
	}
	select {
	case <-a.restartCh:
	case <-time.After(3 * time.Second):
		t.Fatal("回滚后应请求重启")
	}
}

// 全局指标接口返回进程状态，并可通过 metrics:read 令牌访问
func TestGlobalMetrics(t *testing.T) {
	_, srv, c := loginApp(t)
	resp, err := c.Get(srv.URL + "/api/metrics")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	text := string(data)
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain") {
		t.Fatalf("状态 %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	for _, want := range []string{"gately_build_info{", "gately_uptime_seconds ", "# TYPE gately_goroutines gauge"} {
		if !strings.Contains(text, want) {
			t.Errorf("指标缺少 %q:\n%s", want, text)
		}
	}
	// 令牌只需 metrics:read 权限
	code, body := doJSON(t, c, "POST", srv.URL+"/api/tokens", map[string]any{"name": "prom", "scopes": []string{"metrics:read"}})
	if code != 200 {
		t.Fatalf("创建令牌失败 %d %s", code, body)
	}
	var created struct{ Secret string }
	_ = json.Unmarshal(body, &created)
	req, _ := http.NewRequest("GET", srv.URL+"/api/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+created.Secret)
	r2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	r2.Body.Close()
	if r2.StatusCode != 200 {
		t.Fatalf("metrics:read 令牌应能抓取指标，实际 %d", r2.StatusCode)
	}
	req, _ = http.NewRequest("GET", srv.URL+"/api/portforward/rules", nil)
	req.Header.Set("Authorization", "Bearer "+created.Secret)
	r3, _ := http.DefaultClient.Do(req)
	r3.Body.Close()
	if r3.StatusCode != 403 {
		t.Fatalf("metrics:read 令牌不应访问其他模块，实际 %d", r3.StatusCode)
	}
}

// 只读账号可查看但不能修改，也不能访问高危接口；重置密码或删除后会话失效
func TestReadonlyUser(t *testing.T) {
	_, srv, admin := loginApp(t)
	if code, body := doJSON(t, admin, "POST", srv.URL+"/api/account/users", map[string]string{"username": "viewer", "password": "viewer-pass-1"}); code != 200 {
		t.Fatalf("创建只读账号失败 %d %s", code, body)
	}
	if code, _ := doJSON(t, admin, "POST", srv.URL+"/api/account/users", map[string]string{"username": "viewer", "password": "viewer-pass-1"}); code != 400 {
		t.Fatalf("重复用户名应被拒绝，实际 %d", code)
	}
	jar, _ := cookiejar.New(nil)
	ro := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if code, body := doJSON(t, ro, "POST", srv.URL+"/api/login", map[string]string{"username": "viewer", "password": "viewer-pass-1"}); code != 200 {
		t.Fatalf("只读账号登录失败 %d %s", code, body)
	}
	if code, body := doJSON(t, ro, "GET", srv.URL+"/api/session", nil); code != 200 || !bytes.Contains(body, []byte(`"readonly"`)) {
		t.Fatalf("会话应返回只读角色: %d %s", code, body)
	}
	if code, _ := doJSON(t, ro, "GET", srv.URL+"/api/portforward/rules", nil); code != 200 {
		t.Fatalf("只读账号应能查看规则，实际 %d", code)
	}
	rule := map[string]any{"name": "x", "enable": false, "protocols": []string{"tcp4"}, "listenAddress": "127.0.0.1",
		"listenPorts": strconv.Itoa(freeTCPPort(t)), "targetAddresses": []string{"127.0.0.1"}, "targetPorts": "80"}
	for _, c := range []struct{ method, path string }{
		{"POST", "/api/portforward/rules"},
		{"GET", "/api/audit"},
		{"GET", "/api/system/backup"},
		{"GET", "/api/account/users"},
		{"GET", "/api/tokens"},
		{"POST", "/api/system/restart"},
		// 只读账号不能查看密钥明文
		{"GET", "/api/frp/clients/abc/secrets"},
		{"GET", "/api/ddns/tasks/abc/secrets"},
		// 批量启停、删除与站点批量操作属于写操作
		{"PUT", "/api/portforward/rules-enable"},
		{"POST", "/api/portforward/rules-delete"},
		{"POST", "/api/webservice/sites/batch"},
	} {
		var body any
		if c.method == "POST" {
			body = rule
		}
		if code, _ := doJSON(t, ro, c.method, srv.URL+c.path, body); code != 403 {
			t.Errorf("只读账号访问 %s %s 应返回 403，实际 %d", c.method, c.path, code)
		}
	}
	// Web 服务访问日志的“后台用户”标记只认管理员会话，只读账号的 Cookie 不能用来隐藏请求
	u, _ := url.Parse(srv.URL)
	tagged := func(c *http.Client) string {
		req := httptest.NewRequest("GET", "/", nil)
		for _, ck := range c.Jar.Cookies(u) {
			req.AddCookie(ck)
		}
		return httpx.SessionUser(req)
	}
	if got := tagged(ro); got != "" {
		t.Errorf("只读账号的会话不应标记为后台请求: %q", got)
	}
	if got := tagged(admin); got == "" {
		t.Error("管理员会话应标记为后台请求")
	}
	// 重置密码后，旧会话立即失效
	if code, _ := doJSON(t, admin, "PUT", srv.URL+"/api/account/users/viewer", map[string]string{"password": "viewer-pass-2"}); code != 200 {
		t.Fatalf("重置密码失败 %d", code)
	}
	if code, _ := doJSON(t, ro, "GET", srv.URL+"/api/session", nil); code != 401 {
		t.Fatalf("重置密码后旧会话应失效，实际 %d", code)
	}
	if code, _ := doJSON(t, ro, "POST", srv.URL+"/api/login", map[string]string{"username": "viewer", "password": "viewer-pass-2"}); code != 200 {
		t.Fatalf("新密码登录失败 %d", code)
	}
	if code, _ := doJSON(t, admin, "DELETE", srv.URL+"/api/account/users/viewer", nil); code != 200 {
		t.Fatalf("删除失败 %d", code)
	}
	if code, _ := doJSON(t, ro, "GET", srv.URL+"/api/session", nil); code != 401 {
		t.Fatalf("删除账号后会话应失效，实际 %d", code)
	}
	if code, _ := doJSON(t, admin, "GET", srv.URL+"/api/session", nil); code != 200 {
		t.Fatalf("管理员会话不应受影响，实际 %d", code)
	}
}

// Passkey：注册需要当前密码，登录无需密码，挑战只能使用一次，删除后不能再登录
func TestPasskeyLifecycle(t *testing.T) {
	_, srv, admin := loginApp(t)
	host := strings.TrimPrefix(srv.URL, "http://")
	origin := srv.URL
	rpID := webauthn.RPID(host)
	key := webauthntest.New()

	// 未注册时不能开始登录
	if code, _ := doJSON(t, http.DefaultClient, "POST", srv.URL+"/api/login/passkey/begin", map[string]string{}); code != 400 {
		t.Fatalf("未注册 Passkey 时应返回 400，实际 %d", code)
	}

	code, body := doJSON(t, admin, "POST", srv.URL+"/api/account/passkeys/begin", map[string]string{})
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	var opts struct{ Challenge string }
	_ = json.Unmarshal(body, &opts)
	reg := map[string]any{"name": "测试设备", "password": "wrong-password", "challenge": opts.Challenge, "credential": key.Register(rpID, opts.Challenge, origin)}
	if code, _ := doJSON(t, admin, "POST", srv.URL+"/api/account/passkeys", reg); code != 400 {
		t.Fatalf("密码错误时注册应失败，实际 %d", code)
	}
	// 挑战已被消耗，需重新开始
	_, body = doJSON(t, admin, "POST", srv.URL+"/api/account/passkeys/begin", map[string]string{})
	_ = json.Unmarshal(body, &opts)
	reg = map[string]any{"name": "测试设备", "password": "test-password", "challenge": opts.Challenge, "credential": key.Register(rpID, opts.Challenge, origin)}
	if code, body := doJSON(t, admin, "POST", srv.URL+"/api/account/passkeys", reg); code != 200 {
		t.Fatalf("注册失败 %d %s", code, body)
	}

	login := func(c *http.Client) int {
		code, body := doJSON(t, c, "POST", srv.URL+"/api/login/passkey/begin", map[string]string{})
		if code != 200 {
			t.Fatalf("开始登录失败 %d %s", code, body)
		}
		var lo struct{ Challenge string }
		_ = json.Unmarshal(body, &lo)
		finish := map[string]any{"challenge": lo.Challenge, "credential": key.Assert(rpID, lo.Challenge, origin)}
		code, _ = doJSON(t, c, "POST", srv.URL+"/api/login/passkey/finish", finish)
		if code == 200 {
			// 同一个挑战不能重放
			if code2, _ := doJSON(t, c, "POST", srv.URL+"/api/login/passkey/finish", finish); code2 != 400 {
				t.Errorf("挑战重放应被拒绝，实际 %d", code2)
			}
		}
		return code
	}
	jar, _ := cookiejar.New(nil)
	fresh := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if code := login(fresh); code != 200 {
		t.Fatalf("Passkey 登录失败 %d", code)
	}
	if code, body := doJSON(t, fresh, "GET", srv.URL+"/api/session", nil); code != 200 || !bytes.Contains(body, []byte(`"admin"`)) {
		t.Fatalf("Passkey 登录后会话无效 %d %s", code, body)
	}

	// 删除后不能再登录
	_, body = doJSON(t, admin, "GET", srv.URL+"/api/account/passkeys", nil)
	var list []struct{ ID string }
	_ = json.Unmarshal(body, &list)
	if len(list) != 1 {
		t.Fatalf("应有 1 个 Passkey: %s", body)
	}
	if code, _ := doJSON(t, admin, "DELETE", srv.URL+"/api/account/passkeys/"+list[0].ID, map[string]string{"password": "test-password"}); code != 200 {
		t.Fatalf("删除失败 %d", code)
	}
	if code, _ := doJSON(t, http.DefaultClient, "POST", srv.URL+"/api/login/passkey/begin", map[string]string{}); code != 400 {
		t.Fatalf("删除后不应再能开始登录，实际 %d", code)
	}
}

// 全新安装需要首次引导，完成后不再提示
func TestOnboarding(t *testing.T) {
	_, srv, c := loginApp(t)
	code, body := doJSON(t, c, "GET", srv.URL+"/api/system/onboarding", nil)
	if code != 200 || !bytes.Contains(body, []byte(`"pending":true`)) {
		t.Fatalf("全新安装应提示引导: %d %s", code, body)
	}
	if code, _ := doJSON(t, c, "POST", srv.URL+"/api/system/onboarding/dismiss", map[string]string{}); code != 200 {
		t.Fatalf("完成引导失败 %d", code)
	}
	_, body = doJSON(t, c, "GET", srv.URL+"/api/system/onboarding", nil)
	if !bytes.Contains(body, []byte(`"pending":false`)) {
		t.Fatalf("完成后不应再提示: %s", body)
	}
}

// 声明式配置：启动时覆盖配置目录，旧内容进入历史；禁止覆盖账号文件；可指定初始密码
func TestApplyDirAndInitialPassword(t *testing.T) {
	dir := t.TempDir()
	applyDir := t.TempDir()
	rule := `{"settings":{},"rules":[{"key":"r1","name":"from-git","enable":false,"protocols":["tcp4"],"listenAddress":"127.0.0.1","listenPorts":"` + strconv.Itoa(freeTCPPort(t)) + `","targetAddresses":["127.0.0.1"],"targetPorts":"80"}]}`
	if err := os.WriteFile(filepath.Join(applyDir, "portforward.json"), []byte(rule), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := New(Options{ConfigDir: dir, ApplyDir: applyDir, InitialPassword: "bootstrap-pass", Modules: []module.Module{portforward.New()}})
	if err != nil {
		t.Fatal(err)
	}
	a.logger = log.New(io.Discard, "", 0)
	if _, _, err := a.auth.Login("127.0.0.1", "admin", "bootstrap-pass"); err != nil {
		t.Fatalf("应能用指定的初始密码登录: %v", err)
	}
	cur, _ := a.store.ReadRaw("portforward")
	if !bytes.Contains(cur, []byte("from-git")) {
		t.Fatalf("声明式配置未生效: %s", cur)
	}
	if err := os.WriteFile(filepath.Join(applyDir, "account.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Options{ConfigDir: dir, ApplyDir: applyDir}); err == nil {
		t.Fatal("不应允许覆盖 account.json")
	}
	_ = os.Remove(filepath.Join(applyDir, "account.json"))
	if err := os.WriteFile(filepath.Join(applyDir, "bad.json"), []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Options{ConfigDir: dir, ApplyDir: applyDir}); err == nil {
		t.Fatal("非法 JSON 应报错")
	}
}

// Lucky 3.0 的默认 DNS 列表含 default 与 doh3@，前者应忽略，后者按 doh@ 导入
func TestImportLuckyBaseDNSFormats(t *testing.T) {
	a, err := New(Options{ConfigDir: t.TempDir(), Modules: []module.Module{}})
	if err != nil {
		t.Fatal(err)
	}
	a.logger = log.New(io.Discard, "", 0)
	doc := luckyimport.Document{"AdminWebListenPort": json.RawMessage(`16601`), "CustomDNSList": json.RawMessage(`"114.114.114.114\ndefault\ndoh3@223.5.5.5"`)}
	rep := a.importLuckyBase(doc, luckyimport.Options{})
	if rep == nil {
		t.Fatal("应识别为基础设置")
	}
	for _, w := range rep.Warnings {
		if strings.Contains(w, "DNS") {
			t.Fatalf("不应有 DNS 警告: %s", w)
		}
	}
	got := a.settingsSnapshot().DNSServers
	if len(got) != 2 || got[0] != "114.114.114.114" || got[1] != "doh@223.5.5.5" {
		t.Fatalf("DNS 导入结果错误: %v", got)
	}
}

// 命令行 -listen 覆盖（如 OpenWrt 的 LuCI 设置了端口）期间：没改监听地址时配置文件保持原值，
// 在界面上改了则写入配置文件，运行中仍以覆盖值为准，去掉覆盖后生效
func TestSettingsListenOverride(t *testing.T) {
	a, srv, c := loginApp(t)
	const override = "127.0.0.1:17001"
	a.settingsMu.Lock()
	a.opts.Listen = override
	a.settings.Listen = override
	a.settingsMu.Unlock()
	fileListen := func() string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(a.opts.ConfigDir, settingsName+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var s struct {
			Listen string `json:"listen"`
		}
		_ = json.Unmarshal(b, &s)
		return s.Listen
	}

	_, body := doJSON(t, c, "GET", srv.URL+"/api/system/settings", nil)
	var s settingsView
	_ = json.Unmarshal(body, &s)
	if s.Listen != override {
		t.Fatalf("覆盖期间界面应显示实际监听地址: %s", s.Listen)
	}
	s.SessionHours = 48
	if code, body := doJSON(t, c, "PUT", srv.URL+"/api/system/settings", s); code != 200 {
		t.Fatalf("保存设置失败 %d %s", code, body)
	}
	if got := fileListen(); got != DefaultListen {
		t.Fatalf("未修改监听地址时配置文件应保持原值 %s，实际 %s", DefaultListen, got)
	}

	s.Listen = "192.168.1.1:17500"
	if code, body := doJSON(t, c, "PUT", srv.URL+"/api/system/settings", s); code != 200 {
		t.Fatalf("保存设置失败 %d %s", code, body)
	}
	if got := fileListen(); got != "192.168.1.1:17500" {
		t.Fatalf("界面上修改的监听地址应写入配置文件，实际 %s", got)
	}
	_, body = doJSON(t, c, "GET", srv.URL+"/api/system/settings", nil)
	_ = json.Unmarshal(body, &s)
	if s.Listen != override || s.SessionHours != 48 {
		t.Fatalf("运行中仍应以覆盖值为准: %s", body)
	}
	// 再保存一次不改监听地址，配置文件中的新地址不能被覆盖值冲掉
	if code, body := doJSON(t, c, "PUT", srv.URL+"/api/system/settings", s); code != 200 {
		t.Fatalf("保存设置失败 %d %s", code, body)
	}
	if got := fileListen(); got != "192.168.1.1:17500" {
		t.Fatalf("再次保存后配置文件中的地址应保持，实际 %s", got)
	}
}

// 登录时记录界面语言并写入设置文件；保存设置时忽略客户端提交的界面语言。
func TestLoginRemembersUILanguage(t *testing.T) {
	for _, k := range []string{"GATELY_LANG", "LC_ALL", "LC_MESSAGES", "LANG"} {
		t.Setenv(k, "")
	}
	t.Cleanup(func() { i18n.SetUILang("") })
	a, srv, c := loginApp(t)
	req, _ := http.NewRequest("POST", srv.URL+"/api/login", strings.NewReader(`{"username":"admin","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", "zh-CN")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("登录失败 %d", resp.StatusCode)
	}
	if got := a.settingsSnapshot().UILanguage; got != "zh-CN" {
		t.Fatalf("界面语言未记录: %q", got)
	}
	if got := i18n.ServerLang(); got != "zh-CN" {
		t.Fatalf("服务端语言应跟随界面语言: %s", got)
	}
	var saved Settings
	if _, err := a.store.Load(settingsName, &saved); err != nil || saved.UILanguage != "zh-CN" {
		t.Fatalf("界面语言未写入设置文件: %q %v", saved.UILanguage, err)
	}
	_, body := doJSON(t, c, "GET", srv.URL+"/api/system/settings", nil)
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		t.Fatal(err)
	}
	data["uiLanguage"] = "en"
	if code, out := doJSON(t, c, "PUT", srv.URL+"/api/system/settings", data); code != 200 {
		t.Fatalf("保存设置失败 %d %s", code, out)
	}
	if got := a.settingsSnapshot().UILanguage; got != "zh-CN" {
		t.Fatalf("保存设置不应改写界面语言: %q", got)
	}
}

// postWithLang 以指定 Accept-Language 发送 JSON 请求，返回状态码。
func postWithLang(t *testing.T, c *http.Client, method, url, lang, body string) int {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", lang)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// 管理员登录后在界面上切换语言时记录界面语言；只读账号与 API 令牌不能改，只读账号登录也不记录。
func TestSwitchUILanguage(t *testing.T) {
	for _, k := range []string{"GATELY_LANG", "LC_ALL", "LC_MESSAGES", "LANG"} {
		t.Setenv(k, "")
	}
	t.Cleanup(func() { i18n.SetUILang("") })
	a, srv, admin := loginApp(t)
	if code := postWithLang(t, admin, "PUT", srv.URL+"/api/account/ui-language", "en", `{"language":"zh-CN"}`); code != 200 {
		t.Fatalf("切换界面语言失败 %d", code)
	}
	if got := a.settingsSnapshot().UILanguage; got != "zh-CN" || i18n.ServerLang() != "zh-CN" {
		t.Fatalf("界面语言未生效: %q %s", got, i18n.ServerLang())
	}
	var saved Settings
	if _, err := a.store.Load(settingsName, &saved); err != nil || saved.UILanguage != "zh-CN" {
		t.Fatalf("界面语言未写入设置文件: %q %v", saved.UILanguage, err)
	}
	if code := postWithLang(t, admin, "PUT", srv.URL+"/api/account/ui-language", "zh-CN", `{"language":"xx"}`); code != 400 {
		t.Fatalf("不支持的语言应返回 400，实际 %d", code)
	}

	// 只读账号：登录不记录界面语言，也不能调用接口
	if code, body := doJSON(t, admin, "POST", srv.URL+"/api/account/users", map[string]string{"username": "viewer", "password": "viewer-pass-1"}); code != 200 {
		t.Fatalf("创建只读账号失败 %d %s", code, body)
	}
	jar, _ := cookiejar.New(nil)
	ro := &http.Client{Jar: jar}
	if code := postWithLang(t, ro, "POST", srv.URL+"/api/login", "ja", `{"username":"viewer","password":"viewer-pass-1"}`); code != 200 {
		t.Fatalf("只读账号登录失败 %d", code)
	}
	if code := postWithLang(t, ro, "PUT", srv.URL+"/api/account/ui-language", "ja", `{"language":"ja"}`); code != 403 {
		t.Fatalf("只读账号不应能切换服务端语言，实际 %d", code)
	}

	// API 令牌不能访问账号接口
	_, body := doJSON(t, admin, "POST", srv.URL+"/api/tokens", map[string]any{"name": "测试", "scopes": []string{"status:read"}})
	var created struct {
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(body, &created)
	req, _ := http.NewRequest("PUT", srv.URL+"/api/account/ui-language", strings.NewReader(`{"language":"ko"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+created.Secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("API 令牌不应能切换服务端语言，实际 %d", resp.StatusCode)
	}
	if got := a.settingsSnapshot().UILanguage; got != "zh-CN" {
		t.Fatalf("界面语言不应被只读账号或令牌改变: %q", got)
	}
}
