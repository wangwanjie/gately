package tailscale

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// newModule 在 dir 中初始化模块（不启动）。
func newModule(t *testing.T, dir string) *Module {
	t.Helper()
	store, err := config.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := New()
	if err := m.Init(module.Env{Store: store, Logs: logbuf.New(500, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	return m
}

// TestReset 验证恢复默认配置会清空实例与程序设置并删除默认状态目录（含迁移自旧版的目录），自定义状态目录保留。
func TestReset(t *testing.T) {
	dir := t.TempDir()
	// 旧版单实例配置，迁移为使用 <配置目录>/tailscale 的实例
	if err := os.WriteFile(filepath.Join(dir, "tailscale.json"), []byte(`{"enable":false,"hostname":"nas","authKey":"tskey-x","installProxy":"http://127.0.0.1:7890"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	m := newModule(t, dir)
	legacy := filepath.Join(dir, "tailscale")
	_ = os.MkdirAll(legacy, 0o700)
	_ = os.WriteFile(filepath.Join(legacy, "tailscaled.state"), []byte("{}"), 0o600)
	custom := filepath.Join(t.TempDir(), "ts")
	_ = os.MkdirAll(custom, 0o700)
	m.cfg.Instances = append(m.cfg.Instances,
		Instance{Key: "k2", Name: "b", Options: DefaultOptions()},
		Instance{Key: "k3", Name: "c", Options: Options{ManageDaemon: true, StateDir: custom}},
	)
	_ = os.MkdirAll(filepath.Join(legacy, "k2"), 0o700)

	m.mu.Lock()
	err := m.resetLocked()
	m.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(m.cfg.Instances) != 0 || m.cfg.Settings != (Settings{}) {
		t.Errorf("配置未恢复默认: %+v", m.cfg)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Errorf("默认状态目录应被删除: %v", err)
	}
	if _, err := os.Stat(custom); err != nil {
		t.Errorf("自定义状态目录应保留: %v", err)
	}
	// 恢复默认后保存为新格式，不会再次迁移出实例
	m2 := newModule(t, dir)
	if len(m2.cfg.Instances) != 0 {
		t.Errorf("恢复默认后不应再迁移出实例: %+v", m2.cfg.Instances)
	}
}

type testRouter struct{ mux *http.ServeMux }

func (r testRouter) HandleFunc(pattern string, h http.HandlerFunc) {
	method, path, _ := strings.Cut(pattern, " ")
	r.mux.HandleFunc(method+" /api/tailscale"+path, h)
}

// serve 注册模块接口并返回测试服务。
func serve(t *testing.T, m *Module) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	m.Routes(testRouter{mux})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestSecrets 验证查看实例认证密钥明文的接口返回真实密钥，列表中的密钥已脱敏。
func TestSecrets(t *testing.T) {
	m := newModule(t, t.TempDir())
	m.cfg.Instances = []Instance{{Key: "k1", Name: "a", Options: Options{ManageDaemon: true, AuthKey: "tskey-x"}}}
	srv := serve(t, m)
	var got map[string]string
	if code := doJSON(t, "GET", srv.URL+"/api/tailscale/instances/k1/secrets", nil, &got); code != 200 || got["authKey"] != "tskey-x" {
		t.Fatalf("应返回认证密钥明文: %d %v", code, got)
	}
	var list []map[string]any
	doJSON(t, "GET", srv.URL+"/api/tailscale/instances", nil, &list)
	if len(list) != 1 || list[0]["authKey"] != "********" {
		t.Fatalf("列表中的密钥应脱敏: %v", list)
	}
}

func doJSON(t *testing.T, method, url string, body, out any) int {
	t.Helper()
	var r io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		r = strings.NewReader(string(data))
	}
	req, _ := http.NewRequest(method, url, r)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}
