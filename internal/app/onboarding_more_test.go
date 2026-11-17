package app

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"testing"
)

// 跳过场景选择（空列表）结束场景选择，清单保留，场景为空数组而不是 null。
func TestOnboardingSkipScenes(t *testing.T) {
	_, srv, c := startAppIn(t, t.TempDir())
	code, body := doJSON(t, c, "POST", srv.URL+"/api/system/onboarding/scenes", map[string]any{"scenes": []string{}})
	if code != 200 {
		t.Fatalf("跳过选择失败 %d %s", code, body)
	}
	v := getOnboarding(t, c, srv.URL)
	if v.ScenePending || !v.Pending || v.Scenes == nil || len(v.Scenes) != 0 {
		t.Fatalf("跳过后应结束场景选择并保留清单: %+v", v)
	}
	// 非法 JSON 被拒绝
	req, _ := http.NewRequest("POST", srv.URL+"/api/system/onboarding/scenes", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("空请求体应被拒绝，实际 %d", resp.StatusCode)
	}
}

// 手动标记完成的清单项有数量上限，已记录的项重复标记不受上限影响。
func TestOnboardingDoneLimit(t *testing.T) {
	_, srv, c := startAppIn(t, t.TempDir())
	for i := range maxOnboardingDone {
		if code, body := doJSON(t, c, "POST", srv.URL+"/api/system/onboarding/done", map[string]string{"item": fmt.Sprintf("item-%d", i)}); code != 200 {
			t.Fatalf("第 %d 项标记失败 %d %s", i, code, body)
		}
	}
	if code, _ := doJSON(t, c, "POST", srv.URL+"/api/system/onboarding/done", map[string]string{"item": "one-more"}); code != 400 {
		t.Fatalf("超过上限应被拒绝，实际 %d", code)
	}
	if code, _ := doJSON(t, c, "POST", srv.URL+"/api/system/onboarding/done", map[string]string{"item": "item-0"}); code != 200 {
		t.Fatalf("已记录的项重复标记应成功，实际 %d", code)
	}
	if v := getOnboarding(t, c, srv.URL); len(v.Done) != maxOnboardingDone {
		t.Fatalf("应只记录 %d 项: %d", maxOnboardingDone, len(v.Done))
	}
}

// 只读账号既不能读取引导状态（界面据此不显示引导），也不能修改场景、清单或关闭引导。
func TestOnboardingReadonlyCannotWrite(t *testing.T) {
	_, srv, admin := startAppIn(t, t.TempDir())
	if code, body := doJSON(t, admin, "POST", srv.URL+"/api/account/users", map[string]string{"username": "viewer", "password": "tide-Lamp-47-oak"}); code != 200 {
		t.Fatalf("创建只读账号失败 %d %s", code, body)
	}
	jar, _ := cookiejar.New(nil)
	ro := &http.Client{Jar: jar}
	if code, body := doJSON(t, ro, "POST", srv.URL+"/api/login", map[string]string{"username": "viewer", "password": "tide-Lamp-47-oak"}); code != 200 {
		t.Fatalf("只读账号登录失败 %d %s", code, body)
	}
	if code, _ := doJSON(t, ro, "GET", srv.URL+"/api/system/onboarding", nil); code != 403 {
		t.Fatalf("只读账号不应读取引导状态，实际 %d", code)
	}
	for path, body := range map[string]any{
		"/api/system/onboarding/scenes":  map[string]any{"scenes": []string{"web"}},
		"/api/system/onboarding/done":    map[string]string{"item": "check"},
		"/api/system/onboarding/dismiss": map[string]any{},
	} {
		if code, _ := doJSON(t, ro, "POST", srv.URL+path, body); code != 403 {
			t.Errorf("只读账号 POST %s 应被拒绝，实际 %d", path, code)
		}
	}
	if v := getOnboarding(t, admin, srv.URL); !v.ScenePending || len(v.Done) != 0 {
		t.Fatalf("只读账号的请求不应改动引导进度: %+v", v)
	}
}
