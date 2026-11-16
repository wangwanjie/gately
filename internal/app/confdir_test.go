package app

import (
	"context"
	"encoding/json"
	"io"
	"log"
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
	"time"

	"github.com/wangwanjie/gately/internal/modules/registry"
)

// fullApp 创建带全部模块的实例并以管理员登录。prep 在创建实例前调用，用来写入“升级前已保存”的配置文件；
// start 列出需要真正启动的模块（其余模块只注册接口，不监听端口）。
func fullApp(t *testing.T, dir string, prep func(), start ...string) (*App, *httptest.Server, *http.Client) {
	t.Helper()
	// 测试会核对接口返回的中文提示：固定服务端语言为源语言，不随运行测试的机器的 LANG 变化
	t.Setenv("GATELY_LANG", "zh-CN")
	if prep != nil {
		prep()
	}
	mods, certs := registry.New()
	a, err := New(Options{ConfigDir: dir, Certs: certs, Modules: mods})
	if err != nil {
		t.Fatal(err)
	}
	a.logger = log.New(io.Discard, "", 0)
	if err := a.auth.ChangeCredentials(mustInitialPassword(t, a), "admin", "test-password"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	for _, e := range a.modules {
		for _, id := range start {
			if e.mod.ID() == id {
				if err := e.mod.Start(ctx); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = e.mod.Stop() })
			}
		}
	}
	t.Cleanup(cancel)
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if code, body := doJSON(t, c, "POST", srv.URL+"/api/login", map[string]string{"username": "admin", "password": "test-password"}); code != 200 {
		t.Fatalf("登录失败 %d %s", code, body)
	}
	return a, srv, c
}

// leaksAccount 判断响应里是否出现了账号文件的内容。
func leaksAccount(a *App, body string) bool {
	return strings.Contains(body, "sessionSecret") || strings.Contains(body, "passwordHash") || strings.Contains(body, a.auth.EntrySecret())
}

// volumeLayout 建立“整盘共享”的目录：base/vol1 下有 media/hello.txt 与配置目录 1000/docker/gately。
// 配置目录由 New 创建（account.json 为真实账号文件）。
func volumeLayout(t *testing.T) (vol, conf, media string) {
	t.Helper()
	vol = filepath.Join(t.TempDir(), "vol1")
	conf = filepath.Join(vol, "1000", "docker", "gately")
	media = filepath.Join(vol, "media")
	if err := os.MkdirAll(media, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(media, "hello.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	return vol, conf, media
}

// siteGet 以 host 请求 Web 服务监听上的 path。
func siteGet(t *testing.T, port int, host, path string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest("GET", "http://127.0.0.1:"+strconv.Itoa(port)+path, nil)
	if host != "" {
		req.Host = host
	}
	var resp *http.Response
	var err error
	// 监听在保存后异步启动，稍等
	for i := 0; i < 50; i++ {
		if resp, err = http.DefaultClient.Do(req); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp.StatusCode, string(b)
}

// 只有某个模块写权限的令牌不能借存储挂载或静态站点读到配置目录里的 account.json（其中有会话签名密钥）：
// 目录就是配置目录或在其中时拒绝保存；目录包含配置目录（上级目录、整盘、根目录）时可以保存，但配置目录读不到、看不到。
func TestTokenCannotExposeConfigDir(t *testing.T) {
	vol, conf, media := volumeLayout(t)
	a, srv, c := fullApp(t, conf, nil, "webservice")
	if _, err := os.Stat(filepath.Join(conf, "account.json")); err != nil {
		t.Fatalf("测试前提：配置目录里应有 account.json: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(conf, "history"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, storageTok := createToken(t, srv.URL, c, map[string]any{"name": "st", "scopes": []string{"storage:write"}})
	_, webTok := createToken(t, srv.URL, c, map[string]any{"name": "ws", "scopes": []string{"webservice:write"}})

	inside := map[string]string{
		"配置目录":     conf,
		"配置目录内":    filepath.Join(conf, "history"),
		"含 .. 的写法": filepath.Join(media, "..", "1000", "docker", "gately"),
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(vol, "link")
		if err := os.Symlink(conf, link); err != nil {
			t.Fatal(err)
		}
		inside["指向配置目录的符号链接"] = link
	}
	for name, dir := range inside {
		// 链路一：本地挂载点 → 文件管理下载
		code, body := tokenRequest(t, srv.URL, "POST", "/api/storage/mounts", storageTok,
			map[string]any{"name": "cfg", "type": "local", "enable": true, "path": dir})
		if code != 400 || !strings.Contains(body, "配置目录") {
			t.Errorf("挂载%s（%s）应被拒绝: %d %s", name, dir, code, body)
		}
		// 链路二：默认站点为静态目录的监听 → 直接下载
		code, body = tokenRequest(t, srv.URL, "POST", "/api/webservice/listeners", webTok, map[string]any{
			"name": "leak", "enable": true, "network": "tcp4", "listenIp": "127.0.0.1", "port": freeTCPPort(t),
			"default": map[string]any{"enable": true, "type": "static", "root": dir, "listDir": true},
		})
		if code != 400 || !strings.Contains(body, "配置目录") {
			t.Errorf("以%s（%s）为站点目录应被拒绝: %d %s", name, dir, code, body)
		}
	}
	if code, body := tokenRequest(t, srv.URL, "GET", "/api/storage/mounts", storageTok, nil); code != 200 || strings.TrimSpace(body) != "[]" {
		t.Fatalf("被拒绝的挂载点不应保存下来: %d %s", code, body)
	}

	// 审查报告的链路改用上级目录、整盘与根目录：可以保存，但读不到账号文件
	real, _ := filepath.EvalSymlinks(conf)
	contains := map[string][]string{
		vol:                        {"/1000/docker/gately/account.json", "/media/../1000/docker/gately/account.json", "/1000/docker/GATELY/account.json"},
		filepath.Dir(conf):         {"/gately/account.json"},
		string(filepath.Separator): {conf + "/account.json", real + "/account.json"},
	}
	if runtime.GOOS != "windows" {
		contains[vol] = append(contains[vol], "/link/account.json")
	}
	for dir, paths := range contains {
		code, body := tokenRequest(t, srv.URL, "POST", "/api/storage/mounts", storageTok,
			map[string]any{"name": "vol", "type": "local", "enable": true, "path": dir})
		if code != 200 {
			t.Fatalf("包含配置目录的目录 %s 应能挂载: %d %s", dir, code, body)
		}
		var mt struct{ Key string }
		_ = json.Unmarshal([]byte(body), &mt)
		for _, p := range paths {
			code, body := tokenRequest(t, srv.URL, "GET", "/api/storage/files/"+mt.Key+"/download?path="+url.QueryEscape(p), storageTok, nil)
			if code == 200 || leaksAccount(a, body) {
				t.Errorf("挂载 %s 后读取 %s 不应成功: %d %s", dir, p, code, body)
			}
		}
		if dir == vol {
			if code, body := tokenRequest(t, srv.URL, "GET", "/api/storage/files/"+mt.Key+"/download?path=/media/hello.txt", storageTok, nil); code != 200 || body != "hello" {
				t.Fatalf("整盘挂载中的普通文件应可读: %d %s", code, body)
			}
			if code, body := tokenRequest(t, srv.URL, "GET", "/api/storage/files/"+mt.Key+"/list?path=/1000/docker", storageTok, nil); code != 200 || strings.Contains(body, `"gately"`) {
				t.Fatalf("列表中不应出现配置目录: %d %s", code, body)
			}
		}
	}
	port := freeTCPPort(t)
	code, body := tokenRequest(t, srv.URL, "POST", "/api/webservice/listeners", webTok, map[string]any{
		"name": "vol", "enable": true, "network": "tcp4", "listenIp": "127.0.0.1", "port": port,
		"default": map[string]any{"enable": true, "type": "static", "root": vol, "listDir": true},
	})
	if code != 200 {
		t.Fatalf("包含配置目录的站点目录应能保存: %d %s", code, body)
	}
	if code, body := siteGet(t, port, "", "/media/hello.txt"); code != 200 || body != "hello" {
		t.Fatalf("站点中的普通文件应可访问: %d %s", code, body)
	}
	for _, p := range []string{"/1000/docker/gately/account.json", "/1000/docker/gately/", "/link/account.json", "/1000/docker/GATELY/account.json"} {
		if code, body := siteGet(t, port, "", p); code == 200 || leaksAccount(a, body) {
			t.Errorf("站点 %s 不应提供: %d %s", p, code, body)
		}
	}
	if _, body := siteGet(t, port, "", "/1000/docker/"); strings.Contains(body, `href="gately/"`) {
		t.Fatalf("站点目录列表中不应出现配置目录: %s", body)
	}
}

// 升级前已经保存的挂载点与静态站点：程序照常启动；目录在配置目录中的不再提供任何内容，
// 包含配置目录的照常提供，只是配置目录读不到。
func TestExistingConfigDirExposureBlocked(t *testing.T) {
	vol, conf, _ := volumeLayout(t)
	if err := os.MkdirAll(conf, 0o700); err != nil {
		t.Fatal(err)
	}
	port := freeTCPPort(t)
	write := func(name string, v any) {
		b, _ := json.Marshal(v)
		if err := os.WriteFile(filepath.Join(conf, name), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	a, srv, c := fullApp(t, conf, func() {
		write("storage.json", map[string]any{"mounts": []map[string]any{
			{"key": "old", "name": "cfg", "type": "local", "enable": true, "path": conf},
			{"key": "up", "name": "up", "type": "local", "enable": true, "path": vol},
		}})
		write("webservice.json", map[string]any{"listeners": []map[string]any{{
			"key": "l1", "name": "leak", "enable": true, "network": "tcp4", "listenIp": "127.0.0.1", "port": port,
			"default": map[string]any{"key": "default", "enable": true, "type": "static", "root": conf, "listDir": true},
			"sites":   []map[string]any{{"key": "s1", "enable": true, "domains": []string{"up.test"}, "type": "static", "root": vol, "listDir": true}},
		}}})
	}, "webservice")
	_, storageTok := createToken(t, srv.URL, c, map[string]any{"name": "st", "scopes": []string{"storage:write"}})

	for _, p := range []string{"/api/storage/files/old/download?path=/account.json", "/api/storage/files/old/list?path=/"} {
		code, body := tokenRequest(t, srv.URL, "GET", p, storageTok, nil)
		if code != 400 || leaksAccount(a, body) {
			t.Fatalf("%s 应被拒绝: %d %s", p, code, body)
		}
		// 管理员会话同样读不到：限制作用在挂载点上，与身份无关
		if code, body := doJSON(t, c, "GET", srv.URL+p, nil); code != 400 || leaksAccount(a, string(body)) {
			t.Fatalf("%s（管理员）应被拒绝: %d %s", p, code, body)
		}
	}
	for _, p := range []string{"/api/storage/files/up/download?path=/1000/docker/gately/account.json", "/api/storage/files/up/list?path=/1000/docker/gately"} {
		if code, body := doJSON(t, c, "GET", srv.URL+p, nil); code != 404 || leaksAccount(a, string(body)) {
			t.Fatalf("%s 应按不存在处理: %d %s", p, code, body)
		}
	}
	if code, body := doJSON(t, c, "GET", srv.URL+"/api/storage/files/up/download?path=/media/hello.txt", nil); code != 200 || string(body) != "hello" {
		t.Fatalf("包含配置目录的存量挂载点应照常可用: %d %s", code, body)
	}
	for _, r := range [][3]string{
		{"", "/account.json", "403"}, {"", "/", "403"}, {"", "/tokens.json", "403"},
		{"up.test", "/1000/docker/gately/account.json", "404"}, {"up.test", "/1000/docker/gately/", "404"},
		{"up.test", "/media/hello.txt", "200"},
	} {
		code, body := siteGet(t, port, r[0], r[1])
		if strconv.Itoa(code) != r[2] || leaksAccount(a, body) {
			t.Fatalf("站点 %s%s 应返回 %s: %d %s", r[0], r[1], r[2], code, body)
		}
	}
	// 管理员能在日志里看到原因
	_, logs := doJSON(t, c, "GET", srv.URL+"/api/storage/logs", nil)
	if !strings.Contains(string(logs), "配置目录重叠") || !strings.Contains(string(logs), "配置目录已自动隐藏") {
		t.Fatalf("存储挂载日志应说明两种情况: %s", logs)
	}
	_, logs = doJSON(t, c, "GET", srv.URL+"/api/webservice/logs", nil)
	if !strings.Contains(string(logs), "配置目录重叠") || !strings.Contains(string(logs), "配置目录已自动隐藏") {
		t.Fatalf("Web 服务日志应说明两种情况: %s", logs)
	}
}
