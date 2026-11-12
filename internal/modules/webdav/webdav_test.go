package webdav

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/fileusers"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/mountfs"
	"golang.org/x/crypto/bcrypt"
)

func TestWebDAVUsers(t *testing.T) {
	rootA, rootB := t.TempDir(), t.TempDir()
	_ = os.WriteFile(filepath.Join(rootB, "readme.txt"), []byte("hello"), 0o644)
	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	st, _ := config.Open(t.TempDir())
	m := New(nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(20, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	m.settings = Settings{Enable: true, ListenIP: "127.0.0.1", Port: port, Users: []User{
		{Name: "rw", Password: "p1", Root: rootA},
		{Name: "ro", Password: "p2", Root: rootB, ReadOnly: true},
		{Name: "multi", Password: "p3", Mounts: []mountfs.Mount{{Name: "a", Path: rootA}, {Name: "b", Path: rootB, ReadOnly: true}}},
	}}
	if err := m.settings.Validate(); err != nil {
		t.Fatal(err)
	}
	m.restartLocked()
	defer m.Stop()
	base := "http://127.0.0.1:" + strconv.Itoa(port)
	do := func(method, path, user, pass, body string) int {
		req, _ := http.NewRequest(method, base+path, strings.NewReader(body))
		if user != "" {
			req.SetBasicAuth(user, pass)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if c := do("PUT", "/a.txt", "rw", "bad", "x"); c != 401 {
		t.Fatalf("错误密码应 401: %d", c)
	}
	if c := do("PUT", "/a.txt", "rw", "p1", "data"); c != 201 {
		t.Fatalf("读写用户上传失败: %d", c)
	}
	if b, _ := os.ReadFile(filepath.Join(rootA, "a.txt")); string(b) != "data" {
		t.Fatal("文件应写入用户目录")
	}
	if c := do("GET", "/readme.txt", "ro", "p2", ""); c != 200 {
		t.Fatalf("只读用户应能读取: %d", c)
	}
	if c := do("PUT", "/new.txt", "ro", "p2", "x"); c == 201 {
		t.Fatal("只读用户不应能写入")
	}
	if c := do("GET", "/a.txt", "ro", "p2", ""); c != 404 {
		t.Fatalf("用户之间目录应隔离: %d", c)
	}

	// 多目录用户：根目录列出挂载名称，各挂载按自身的只读设置读写
	req, _ := http.NewRequest("PROPFIND", base+"/", nil)
	req.Header.Set("Depth", "1")
	req.SetBasicAuth("multi", "p3")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	listing, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 207 || !strings.Contains(string(listing), "<D:href>/a/</D:href>") || !strings.Contains(string(listing), "<D:href>/b/</D:href>") {
		t.Fatalf("根目录应列出挂载: %d %s", resp.StatusCode, listing)
	}
	if c := do("GET", "/b/readme.txt", "multi", "p3", ""); c != 200 {
		t.Fatalf("应能读取只读挂载: %d", c)
	}
	if c := do("PUT", "/a/m.txt", "multi", "p3", "m"); c != 201 {
		t.Fatalf("应能写入可写挂载: %d", c)
	}
	if c := do("PUT", "/b/m.txt", "multi", "p3", "m"); c == 201 {
		t.Fatal("只读挂载不应能写入")
	}
	if c := do("PUT", "/m.txt", "multi", "p3", "m"); c == 201 {
		t.Fatal("虚拟根目录不应能写入")
	}
}

// TestSettingsSecrets 验证查看本地用户密码明文的接口按用户名返回原始密码，而设置接口仍返回占位符。
func TestSettingsSecrets(t *testing.T) {
	st, _ := config.Open(t.TempDir())
	m := New(nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(20, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.settings = Settings{Users: []User{{Name: "alice", Password: "p1", Root: "/a"}, {Name: "bob", Password: "p2", Root: "/b"}}}
	m.mu.Unlock()
	mux := http.NewServeMux()
	m.Routes(secretsRouter{mux})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/settings/secrets", nil))
	var out map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || len(out) != 2 || out["alice"] != "p1" || out["bob"] != "p2" {
		t.Fatalf("应按用户名返回密码明文: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/settings", nil))
	if strings.Contains(rec.Body.String(), "p1") {
		t.Fatalf("设置接口不应返回密码明文: %s", rec.Body.String())
	}
}

// secretsRouter 把模块接口直接注册到 ServeMux，用于测试。
type secretsRouter struct{ *http.ServeMux }

func (r secretsRouter) HandleFunc(pattern string, h http.HandlerFunc) {
	r.ServeMux.HandleFunc(pattern, h)
}

// 刚启动就重启时，旧监听可能还没被后台的 Serve 登记，停止时要同步关闭它，否则同一端口重新监听会报端口已被占用。
func TestWebDAVImmediateRestart(t *testing.T) {
	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	st, _ := config.Open(t.TempDir())
	m := New(nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(20, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	m.settings = Settings{Enable: true, ListenIP: "127.0.0.1", Port: port, Users: []User{{Name: "u", Password: "p", Root: t.TempDir()}}}
	defer m.Stop()
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := 0; i < 200; i++ {
		m.restartLocked()
		if m.err != "" {
			t.Fatalf("第 %d 次重启失败: %s", i+1, m.err)
		}
	}
}

// 同一来源连续猜错密码达到上限后，即使随后给出正确密码也返回 429，其他来源不受影响。
func TestServeThrottle(t *testing.T) {
	m := &Module{handlers: map[string]*userHandler{}, logs: logbuf.New(20, log.New(io.Discard, "", 0))}
	m.handlers["u"] = newUserHandler(User{Name: "u", Password: "good", Root: t.TempDir()})
	do := func(remote, pass string) int {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = remote
		req.SetBasicAuth("u", pass)
		w := httptest.NewRecorder()
		m.serve(w, req)
		return w.Code
	}
	for i := 0; i < fileusers.ThrottleMaxFailures; i++ {
		if c := do("198.51.100.5:1000", "bad"); c != 401 {
			t.Fatalf("错误密码应 401: %d", c)
		}
	}
	if c := do("198.51.100.5:1001", "good"); c != 429 {
		t.Fatalf("失败次数达到上限后应 429: %d", c)
	}
	if c := do("198.51.100.6:1000", "good"); c == 401 || c == 429 {
		t.Fatalf("其他来源不应受影响: %d", c)
	}
}

// 近期验证通过的凭据不占限流名额：同一来源有多个正在校验的请求时，凭据正确的请求不被误拒。
func TestServeThrottleKnownGood(t *testing.T) {
	m := &Module{handlers: map[string]*userHandler{}, logs: logbuf.New(20, log.New(io.Discard, "", 0))}
	hash, err := bcrypt.GenerateFromPassword([]byte("good"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	m.handlers["u"] = newUserHandler(User{Name: "u", Password: string(hash), Root: t.TempDir()})
	do := func(pass string) int {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "198.51.100.7:1000"
		req.SetBasicAuth("u", pass)
		w := httptest.NewRecorder()
		m.serve(w, req)
		return w.Code
	}
	if c := do("good"); c == 401 || c == 429 {
		t.Fatalf("首次正确密码应通过: %d", c)
	}
	// 模拟同一来源有大量并发的校验正在进行
	for i := 0; i < fileusers.ThrottleMaxFailures; i++ {
		m.throttle.Reserve("198.51.100.7")
	}
	if c := do("good"); c == 401 || c == 429 {
		t.Fatalf("近期验证通过的凭据不应被进行中的校验挡住: %d", c)
	}
	if c := do("bad"); c != 429 {
		t.Fatalf("未缓存的凭据仍按名额限流: %d", c)
	}
}
