//go:build linux || darwin

package smb

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/fileusers"
)

// enableSelfService 经共享用户的保存接口保存 users 并打开自助修改密码。
func enableSelfService(t *testing.T, users string) {
	t.Helper()
	req := httptest.NewRequest("PUT", "/fileusers", strings.NewReader(`{"selfService":true,"users":`+users+`}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	fileusers.Default.HandlePut(nil)(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("保存共享用户失败: %d %s", w.Code, w.Body.String())
	}
	t.Cleanup(func() {
		req := httptest.NewRequest("PUT", "/fileusers", strings.NewReader(`{"selfService":false,"users":[]}`))
		req.Header.Set("Content-Type", "application/json")
		fileusers.Default.HandlePut(nil)(httptest.NewRecorder(), req)
	})
}

// TestSelfServicePasswordChangeKeepsService：共享用户自助修改密码后服务不重启，其他用户的连接与认证失败计数都保留，
// 只有改了密码的用户已登录的连接被断开；新密码立即生效，旧密码失效。
func TestSelfServicePasswordChangeKeepsService(t *testing.T) {
	bobDir, carolDir := t.TempDir(), t.TempDir()
	m, port := startTestModule(t, Settings{Users: []User{{Name: "local", Password: "lp", Root: t.TempDir()}}, SharedUsers: true})
	enableSelfService(t, `[{"name":"bob","password":"bob-pass-1","root":"`+bobDir+`"},{"name":"carol","password":"carol-pass-1","root":"`+carolDir+`"}]`)
	m.mu.Lock()
	srvBefore := m.srv
	m.mu.Unlock()

	bob, err := dial(t, port, "bob", "bob-pass-1")
	if err != nil {
		t.Fatalf("bob 登录失败: %v", err)
	}
	carol, err := dial(t, port, "carol", "carol-pass-1")
	if err != nil {
		t.Fatalf("carol 登录失败: %v", err)
	}
	// 同一来源先失败到差一次就被限流
	for i := 0; i < fileusers.ThrottleMaxFailures-1; i++ {
		if _, err := dial(t, port, "local", "wrong"); err == nil {
			t.Fatal("错误密码应登录失败")
		}
	}

	if err := fileusers.Default.ChangePassword("carol", "carol-pass-1", "carol-pass-2"); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	srvAfter := m.srv
	m.mu.Unlock()
	if srvAfter != srvBefore {
		t.Fatal("自助修改密码不应重启 SMB 服务")
	}
	if !hasLog(m, "共享用户 carol 的密码已修改，已断开其 1 个连接") {
		t.Fatal("应记录修改密码并断开该用户的连接")
	}
	// bob 的连接不受影响
	if _, err := bob.ListSharenames(); err != nil {
		t.Fatalf("其他用户的连接不应被断开: %v", err)
	}
	// carol 的旧连接被断开
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := carol.ListSharenames(); err != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("改了密码的用户已登录的连接应被断开")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := dial(t, port, "carol", "carol-pass-2"); err != nil {
		t.Fatalf("新密码应立即生效: %v", err)
	}
	// 失败计数未被清空：再失败一次即达到上限，正确密码也被拒绝
	if _, err := dial(t, port, "carol", "carol-pass-1"); err == nil {
		t.Fatal("旧密码应失效")
	}
	if _, err := dial(t, port, "local", "lp"); err == nil {
		t.Fatal("修改密码不应清空来源的认证失败计数")
	}
}
