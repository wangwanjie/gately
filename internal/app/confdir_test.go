package app

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

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

// 只有某个模块写权限的令牌不能借存储挂载或静态站点读到配置目录里的 account.json（其中有会话签名密钥）。
func TestTokenCannotExposeConfigDir(t *testing.T) {
	base := t.TempDir()
	conf := filepath.Join(base, "conf")
	share := filepath.Join(base, "share")
	if err := os.MkdirAll(share, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(share, "hello.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, srv, c := fullApp(t, conf, nil)
	if _, err := os.Stat(filepath.Join(conf, "account.json")); err != nil {
		t.Fatalf("测试前提：配置目录里应有 account.json: %v", err)
	}
	_, storageTok := createToken(t, srv.URL, c, map[string]any{"name": "st", "scopes": []string{"storage:write"}})
	_, webTok := createToken(t, srv.URL, c, map[string]any{"name": "ws", "scopes": []string{"webservice:write"}})

	dirs := map[string]string{
		"配置目录":     conf,
		"包含配置目录":   base,
		"含 .. 的写法": filepath.Join(share, "..", "conf"),
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(base, "link")
		if err := os.Symlink(conf, link); err != nil {
			t.Fatal(err)
		}
		dirs["指向配置目录的符号链接"] = link
	}
	for name, dir := range dirs {
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
	if code, body := tokenRequest(t, srv.URL, "GET", "/api/webservice/listeners", webTok, nil); code != 200 || strings.TrimSpace(body) != "[]" {
		t.Fatalf("被拒绝的监听不应保存下来: %d %s", code, body)
	}
	// 其他目录不受影响
	code, body := tokenRequest(t, srv.URL, "POST", "/api/storage/mounts", storageTok,
		map[string]any{"name": "share", "type": "local", "enable": true, "path": share})
	if code != 200 {
		t.Fatalf("无关目录应能挂载: %d %s", code, body)
	}
	var mt struct{ Key string }
	_ = json.Unmarshal([]byte(body), &mt)
	if code, body := tokenRequest(t, srv.URL, "GET", "/api/storage/files/"+mt.Key+"/download?path=/hello.txt", storageTok, nil); code != 200 || body != "hello" {
		t.Fatalf("无关目录应能下载: %d %s", code, body)
	}
	// 经无关挂载点用 .. 也走不到配置目录
	for _, p := range []string{"/../conf/account.json", "/..%2fconf%2faccount.json", "/%2e%2e/conf/account.json"} {
		code, body := tokenRequest(t, srv.URL, "GET", "/api/storage/files/"+mt.Key+"/download?path="+p, storageTok, nil)
		if code == 200 || leaksAccount(a, body) {
			t.Fatalf("经 %s 不应读到账号文件: %d %s", p, code, body)
		}
	}
}

// 升级前已经保存的、指向配置目录的挂载点与静态站点：程序照常启动，但它们不再提供任何内容。
func TestExistingConfigDirExposureBlocked(t *testing.T) {
	base := t.TempDir()
	conf := filepath.Join(base, "conf")
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
			{"key": "up", "name": "up", "type": "local", "enable": true, "path": base},
		}})
		write("webservice.json", map[string]any{"listeners": []map[string]any{{
			"key": "l1", "name": "leak", "enable": true, "network": "tcp4", "listenIp": "127.0.0.1", "port": port,
			"default": map[string]any{"key": "default", "enable": true, "type": "static", "root": conf, "listDir": true},
			"sites":   []map[string]any{{"key": "s1", "enable": true, "domains": []string{"up.test"}, "type": "static", "root": base, "listDir": true}},
		}}})
	}, "webservice")
	_, storageTok := createToken(t, srv.URL, c, map[string]any{"name": "st", "scopes": []string{"storage:write"}})

	for _, p := range []string{
		"/api/storage/files/old/download?path=/account.json",
		"/api/storage/files/old/list?path=/",
		"/api/storage/files/up/download?path=/conf/account.json",
		"/api/storage/files/up/list?path=/conf",
	} {
		code, body := tokenRequest(t, srv.URL, "GET", p, storageTok, nil)
		if code != 400 || leaksAccount(a, body) {
			t.Fatalf("%s 应被拒绝: %d %s", p, code, body)
		}
		// 管理员会话同样读不到：限制作用在挂载点上，与身份无关
		if code, body := doJSON(t, c, "GET", srv.URL+p, nil); code != 400 || leaksAccount(a, string(body)) {
			t.Fatalf("%s（管理员）应被拒绝: %d %s", p, code, body)
		}
	}
	site := "http://127.0.0.1:" + strconv.Itoa(port)
	for _, r := range [][2]string{
		{"", "/account.json"}, {"", "/"}, {"", "/tokens.json"},
		{"up.test", "/conf/account.json"}, {"up.test", "/conf/"},
	} {
		req, _ := http.NewRequest("GET", site+r[1], nil)
		if r[0] != "" {
			req.Host = r[0]
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 403 || leaksAccount(a, string(b)) {
			t.Fatalf("站点 %s%s 应返回 403: %d %s", r[0], r[1], resp.StatusCode, b)
		}
	}
	// 管理员能在日志里看到原因
	_, logs := doJSON(t, c, "GET", srv.URL+"/api/storage/logs", nil)
	if !strings.Contains(string(logs), "配置目录重叠") {
		t.Fatalf("存储挂载日志应提示目录与配置目录重叠: %s", logs)
	}
	_, logs = doJSON(t, c, "GET", srv.URL+"/api/webservice/logs", nil)
	if !strings.Contains(string(logs), "配置目录重叠") {
		t.Fatalf("Web 服务日志应提示目录与配置目录重叠: %s", logs)
	}
}
