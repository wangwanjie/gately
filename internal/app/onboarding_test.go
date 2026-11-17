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
	"testing"

	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/portforward"
)

// startAppIn 在指定配置目录启动实例并以管理员登录；目录里没有账号时先改掉初始密码。
func startAppIn(t *testing.T, dir string) (*App, *httptest.Server, *http.Client) {
	t.Helper()
	a, err := New(Options{ConfigDir: dir, Modules: []module.Module{portforward.New()}})
	if err != nil {
		t.Fatal(err)
	}
	a.logger = log.New(io.Discard, "", 0)
	if a.auth.UsingDefaultPassword() {
		if err := a.auth.ChangeCredentials(mustInitialPassword(t, a), "admin", "test-password"); err != nil {
			t.Fatal(err)
		}
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	if code, body := doJSON(t, c, "POST", srv.URL+"/api/login", map[string]string{"username": "admin", "password": "test-password"}); code != 200 {
		t.Fatalf("登录失败 %d %s", code, body)
	}
	return a, srv, c
}

func getOnboarding(t *testing.T, c *http.Client, base string) onboardingView {
	t.Helper()
	code, body := doJSON(t, c, "GET", base+"/api/system/onboarding", nil)
	if code != 200 {
		t.Fatalf("读取引导状态失败 %d %s", code, body)
	}
	var v onboardingView
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// 全新安装（空配置目录）首次登录需要选择使用场景；重启后仍保留，选择后不再询问
func TestOnboardingSceneFreshInstall(t *testing.T) {
	dir := t.TempDir()
	_, srv, c := startAppIn(t, dir)
	v := getOnboarding(t, c, srv.URL)
	if !v.Pending || !v.ScenePending {
		t.Fatalf("全新安装应询问使用场景: %+v", v)
	}
	srv.Close()

	// 重启（目录里已有配置文件）不影响尚未完成的场景选择
	_, srv2, c2 := startAppIn(t, dir)
	if v := getOnboarding(t, c2, srv2.URL); !v.ScenePending {
		t.Fatalf("重启后仍应询问使用场景: %+v", v)
	}
	if code, body := doJSON(t, c2, "POST", srv2.URL+"/api/system/onboarding/scenes", map[string]any{"scenes": []string{"web", "ddns", "web"}}); code != 200 {
		t.Fatalf("记录场景失败 %d %s", code, body)
	}
	v = getOnboarding(t, c2, srv2.URL)
	if v.ScenePending || !v.Pending || len(v.Scenes) != 2 || v.Scenes[0] != "web" || v.Scenes[1] != "ddns" {
		t.Fatalf("选择场景后应结束场景选择、保留清单并去重: %+v", v)
	}
	if code, _ := doJSON(t, c2, "POST", srv2.URL+"/api/system/onboarding/scenes", map[string]any{"scenes": []string{"bogus"}}); code != 400 {
		t.Fatalf("未知场景应被拒绝，实际 %d", code)
	}
}

// 升级的实例（配置目录里已有配置文件）不询问使用场景，即使账号文件是新建的
func TestOnboardingSceneUpgradeSkipped(t *testing.T) {
	for _, name := range []string{"gately.json", "portforward.json", "webservice.json"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, name), []byte("{}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			_, srv, c := startAppIn(t, dir)
			if v := getOnboarding(t, c, srv.URL); v.ScenePending {
				t.Fatalf("已有配置文件时不应询问使用场景: %+v", v)
			}
			if _, err := os.Stat(filepath.Join(dir, onboardingName+".json")); !os.IsNotExist(err) {
				t.Fatalf("升级的实例不应写入引导进度文件: %v", err)
			}
		})
	}
}

// 场景选择要求账号上的首次引导标记仍在：关闭引导（或从备份恢复了已完成引导的账号）后不再询问
func TestOnboardingDismissEndsScene(t *testing.T) {
	_, srv, c := startAppIn(t, t.TempDir())
	if code, body := doJSON(t, c, "POST", srv.URL+"/api/system/onboarding/dismiss", map[string]any{}); code != 200 {
		t.Fatalf("关闭引导失败 %d %s", code, body)
	}
	if v := getOnboarding(t, c, srv.URL); v.Pending || v.ScenePending {
		t.Fatalf("关闭引导后不应再显示: %+v", v)
	}
}

// 手动标记完成的清单项去重、校验编号
func TestOnboardingDone(t *testing.T) {
	_, srv, c := startAppIn(t, t.TempDir())
	for range 2 {
		if code, body := doJSON(t, c, "POST", srv.URL+"/api/system/onboarding/done", map[string]string{"item": "check"}); code != 200 {
			t.Fatalf("标记完成失败 %d %s", code, body)
		}
	}
	if v := getOnboarding(t, c, srv.URL); len(v.Done) != 1 || v.Done[0] != "check" {
		t.Fatalf("应记录一次 check: %+v", v)
	}
	for _, bad := range []string{"", "Check", "../x", "a b"} {
		if code, _ := doJSON(t, c, "POST", srv.URL+"/api/system/onboarding/done", map[string]string{"item": bad}); code != 400 {
			t.Fatalf("非法编号 %q 应被拒绝，实际 %d", bad, code)
		}
	}
}
