//go:build linux || darwin

package smb

import (
	"testing"

	"github.com/wangwanjie/gately/internal/fileusers"
)

// TestSMBDisabledUsers 验证停用的本地用户与共享用户即使密码正确也无法登录 SMB，停用的本地用户不回落到同名共享用户；
// 停用共享用户后服务重新加载，重新启用后恢复登录。
func TestSMBDisabledUsers(t *testing.T) {
	dir := t.TempDir()
	m, port := startTestModule(t, Settings{
		Users: []User{
			{Name: "on", Password: "p1", Root: dir},
			{Name: "off", Password: "p2", Root: dir, Disabled: true},
			{Name: "dup", Password: "local", Root: dir, Disabled: true},
		},
		SharedUsers: true,
	})
	if err := fileusers.Default.Save([]fileusers.User{
		{Name: "sam", Password: "ps", Root: dir},
		{Name: "DUP", Password: "shared", Root: dir},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := dial(t, port, "on", "p1"); err != nil {
		t.Fatalf("启用的用户应能登录: %v", err)
	}
	if _, err := dial(t, port, "sam", "ps"); err != nil {
		t.Fatalf("启用的共享用户应能登录: %v", err)
	}
	for _, c := range []struct{ user, pass, why string }{
		{"off", "p2", "停用的本地用户"},
		{"dup", "local", "停用的本地用户"},
		{"DUP", "shared", "停用本地用户的同名共享用户"},
	} {
		if _, err := dial(t, port, c.user, c.pass); err == nil {
			t.Errorf("%s %s 不应能登录", c.why, c.user)
		}
	}
	if err := fileusers.Default.Save([]fileusers.User{{Name: "sam", Password: "ps", Root: dir, Disabled: true}}); err != nil {
		t.Fatal(err)
	}
	if !hasLog(m, "共享用户已修改") {
		t.Fatal("停用共享用户后应重新加载 SMB 服务")
	}
	if _, err := dial(t, port, "sam", "ps"); err == nil {
		t.Fatal("停用的共享用户不应能登录")
	}
	if err := fileusers.Default.Save([]fileusers.User{{Name: "sam", Password: "ps", Root: dir}}); err != nil {
		t.Fatal(err)
	}
	if _, err := dial(t, port, "sam", "ps"); err != nil {
		t.Fatalf("重新启用的共享用户应能登录: %v", err)
	}
}

// TestSMBDisabledCountAsUsers 验证停用的用户仍算作已添加的用户：全部停用时仍可启用服务，
// 只靠共享用户启用时也可以停用最后一个共享用户；停用用户不生成账号。
func TestSMBDisabledCountAsUsers(t *testing.T) {
	dir := t.TempDir()
	s := Settings{Enable: true, Users: []User{{Name: "a", Password: "p", Root: dir, Disabled: true}}}
	if err := s.Validate(); err != nil {
		t.Fatalf("全部本地用户停用时仍应可以启用: %v", err)
	}
	accounts, _, disabled := buildAccountsCount(s, nil)
	if len(accounts) != 0 || disabled != 1 {
		t.Fatalf("停用的本地用户不应生成账号: %d %d", len(accounts), disabled)
	}
	m, _ := startTestModule(t, Settings{SharedUsers: true, Users: []User{{Name: "tmp", Password: "p", Root: dir}}})
	if err := fileusers.Default.Save([]fileusers.User{{Name: "s", Password: "p", Root: dir}}); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.settings.Users = nil
	m.mu.Unlock()
	if err := fileusers.Default.Save([]fileusers.User{{Name: "s", Password: "p", Root: dir, Disabled: true}}); err != nil {
		t.Fatalf("停用最后一个共享用户不应被拦截: %v", err)
	}
	if err := fileusers.Default.Save(nil); err == nil {
		t.Fatal("删光共享用户仍应被拦截")
	}
}
