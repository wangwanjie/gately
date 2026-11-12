package auth

import (
	"testing"
	"time"
)

// 登出后该令牌立即失效，同一时刻签发的其他令牌不受影响。
func TestRevokeToken(t *testing.T) {
	m, pass := newTestManager(t)
	a, _, err := m.Login("1.1.1.1", "admin", pass)
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := m.Login("1.1.1.2", "admin", pass)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Revoke(a); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Verify(a); ok {
		t.Fatal("登出后的令牌应被拒绝")
	}
	if _, ok := m.Verify(b); !ok {
		t.Fatal("其他会话不应受影响")
	}
	// 重复吊销与吊销无效令牌都不报错、不增加记录
	if err := m.Revoke(a); err != nil {
		t.Fatal(err)
	}
	if err := m.Revoke("garbage.token"); err != nil {
		t.Fatal(err)
	}
	if n := len(m.acc.Revoked); n != 1 {
		t.Fatalf("吊销记录应为 1 条，实际 %d", n)
	}
}

// 退出所有设备使全部旧令牌失效，之后新签发的令牌可用。
func TestRevokeAll(t *testing.T) {
	m, pass := newTestManager(t)
	a, _, _ := m.Login("1.1.1.1", "admin", pass)
	b, _ := m.Issue()
	if err := m.RevokeAll(); err != nil {
		t.Fatal(err)
	}
	for _, tk := range []string{a, b} {
		if _, ok := m.Verify(tk); ok {
			t.Fatal("退出所有设备后旧令牌应失效")
		}
	}
	c, _ := m.Issue()
	if _, ok := m.Verify(c); !ok {
		t.Fatal("新签发的令牌应有效")
	}
}

// 吊销状态随账号持久化，重启（用保存的账号重建管理器）后仍然有效。
func TestRevokePersistsAcrossRestart(t *testing.T) {
	var saved Account
	m := NewManager(Account{}, func(a Account) error { saved = a; return nil })
	pass, err := m.EnsureInitialized()
	if err != nil {
		t.Fatal(err)
	}
	a, _, _ := m.Login("1.1.1.1", "admin", pass)
	b, _, _ := m.Login("1.1.1.2", "admin", pass)
	if err := m.Revoke(a); err != nil {
		t.Fatal(err)
	}
	m2 := NewManager(saved, func(a Account) error { saved = a; return nil })
	if _, ok := m2.Verify(a); ok {
		t.Fatal("重启后已登出的令牌仍应失效")
	}
	if _, ok := m2.Verify(b); !ok {
		t.Fatal("重启后未登出的令牌应仍有效")
	}
	if err := m2.RevokeAll(); err != nil {
		t.Fatal(err)
	}
	m3 := NewManager(saved, func(Account) error { return nil })
	if _, ok := m3.Verify(b); ok {
		t.Fatal("重启后退出所有设备仍应生效")
	}
}

// 吊销列表有界：已过期的记录被清理，列表满时提升纪元并清空。
func TestRevokeListBounded(t *testing.T) {
	m, _ := newTestManager(t)
	var tokens []string
	for i := 0; i < maxRevoked+5; i++ {
		tk, _ := m.Issue()
		tokens = append(tokens, tk)
	}
	for _, tk := range tokens {
		if err := m.Revoke(tk); err != nil {
			t.Fatal(err)
		}
		if n := len(m.acc.Revoked); n > maxRevoked {
			t.Fatalf("吊销列表超过上限: %d", n)
		}
	}
	// 溢出后全部旧令牌（包括已被淘汰记录的）依然无效
	for _, tk := range tokens {
		if _, ok := m.Verify(tk); ok {
			t.Fatal("溢出后旧令牌应全部无效")
		}
	}
	// 过期记录被清理
	tk, _ := m.Issue()
	if err := m.Revoke(tk); err != nil {
		t.Fatal(err)
	}
	m.now = func() time.Time { return time.Now().Add(200 * time.Hour) }
	tk2, _ := m.Issue()
	if err := m.Revoke(tk2); err != nil {
		t.Fatal(err)
	}
	if n := len(m.acc.Revoked); n != 1 {
		t.Fatalf("过期记录应被清理，剩余 %d", n)
	}
}

// 只读账号反复登录、登出不能填满吊销列表而触发提升纪元，管理员会话不受影响。
func TestReadonlyRevokeCannotEvictAdmins(t *testing.T) {
	m, pass := newTestManager(t)
	admin, _, err := m.Login("1.1.1.1", "admin", pass)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.AddUser("viewer", "viewer-pass-123"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Login("2.2.2.2", "viewer", "viewer-pass-123"); err != nil {
		t.Fatal(err)
	}
	// 直接签发只读令牌，省去上千次 bcrypt 登录
	var viewerHash string
	for _, u := range m.acc.Users {
		if u.Username == "viewer" {
			viewerHash = u.PasswordHash
		}
	}
	for i := 0; i < maxRevoked+10; i++ {
		tk := m.sign(m.acc, time.Now().Add(time.Hour), "viewer", RoleReadonly, viewerHash)
		if err := m.Revoke(tk); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := m.Verify(admin); !ok {
		t.Fatal("只读账号登出不应让管理员会话失效")
	}
	if n := len(m.acc.Revoked); n > maxRevoked/2 {
		t.Fatalf("只读账号的吊销记录不应超过一半容量: %d", n)
	}
	// 管理员登出照常记录
	if err := m.Revoke(admin); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Verify(admin); ok {
		t.Fatal("管理员登出后令牌应失效")
	}
}
