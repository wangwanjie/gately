package app

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/portforward"
)

// startAbsentApp 启动只编译了端口转发模块的实例，模拟去掉网络唤醒模块的定制构建。
func startAbsentApp(t *testing.T, dir string, absent []module.Info) (*httptest.Server, *http.Client) {
	t.Helper()
	a, err := New(Options{
		ConfigDir:       dir,
		Modules:         []module.Module{portforward.New()},
		Absent:          absent,
		InitialPassword: "test-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	a.logger = log.New(io.Discard, "", 0)
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	if code, body := doJSON(t, c, "POST", srv.URL+"/api/login", map[string]string{"username": "admin", "password": "test-password"}); code != 200 {
		t.Fatalf("登录失败 %d %s", code, body)
	}
	return srv, c
}

func TestAbsentModules(t *testing.T) {
	dir := t.TempDir()
	// 完整构建留下的网络唤醒配置与“关闭网络唤醒”的设置
	wolConfig := []byte(`{"devices":[{"key":"d1","name":"NAS","mac":"00:11:22:33:44:55"}]}`)
	if err := os.WriteFile(filepath.Join(dir, "wol.json"), wolConfig, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gately.json"), []byte(`{"disabledModules":["wol"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	srv, c := startAbsentApp(t, dir, []module.Info{{ID: "wol", Title: "网络唤醒"}})

	// 会话与状态中标出未编译的模块，状态中带构建内容
	_, body := doJSON(t, c, "GET", srv.URL+"/api/session", nil)
	var sess struct {
		AbsentModules []string `json:"absentModules"`
	}
	_ = json.Unmarshal(body, &sess)
	if !slices.Equal(sess.AbsentModules, []string{"wol"}) {
		t.Fatalf("会话中应列出未编译的模块: %s", body)
	}
	_, body = doJSON(t, c, "GET", srv.URL+"/api/status", nil)
	var st struct {
		Build buildInfo `json:"build"`
	}
	_ = json.Unmarshal(body, &st)
	if !st.Build.Custom || len(st.Build.Absent) != 1 || st.Build.Absent[0].ID != "wol" {
		t.Fatalf("状态中应标明定制构建: %s", body)
	}

	// 未编译模块的接口返回 404 与明确的说明（测试请求不带 Accept-Language，返回英文）
	code, body := doJSON(t, c, "GET", srv.URL+"/api/wol/devices", nil)
	if code != http.StatusNotFound || !strings.Contains(string(body), "not included in this build") {
		t.Fatalf("未编译模块的接口应返回 404: %d %s", code, body)
	}

	// 设置页只提交本次编译的模块开关，保存后未编译模块原来的“关闭”仍然保留
	_, body = doJSON(t, c, "GET", srv.URL+"/api/system/settings", nil)
	var s settingsView
	_ = json.Unmarshal(body, &s)
	s.DisabledModules = nil
	if code, body := doJSON(t, c, "PUT", srv.URL+"/api/system/settings", s); code != 200 {
		t.Fatalf("保存设置失败 %d %s", code, body)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "gately.json"))
	if !strings.Contains(string(raw), `"wol"`) {
		t.Fatalf("未编译模块的开关应保留: %s", raw)
	}
	// 显式提交未编译模块的开关也是合法的
	s.DisabledModules = []string{"wol"}
	if code, body := doJSON(t, c, "PUT", srv.URL+"/api/system/settings", s); code != 200 {
		t.Fatalf("提交未编译模块的开关应被接受 %d %s", code, body)
	}

	// 未编译模块仍是合法的权限域，带它权限的令牌可以创建，换回完整构建后继续有效
	createToken(t, srv.URL, c, map[string]any{"name": "唤醒", "scopes": []string{"wol:read", "portforward:read"}})

	// 未编译模块的配置文件不被改动
	if got, _ := os.ReadFile(filepath.Join(dir, "wol.json")); string(got) != string(wolConfig) {
		t.Fatalf("未编译模块的配置文件被改动: %s", got)
	}
}
