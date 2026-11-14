package auth

import (
	"strings"
	"testing"
	"time"
)

// 管理员会话可以签发桥接凭据，凭据校验通过并返回管理员用户名，过期时间与会话相同。
func TestBridgeIssueAndVerify(t *testing.T) {
	m, pass := newTestManager(t)
	token, texp, err := m.Login("1.1.1.1", "admin", pass)
	if err != nil {
		t.Fatal(err)
	}
	v, exp, ok := m.IssueBridge(token)
	if !ok || v == "" {
		t.Fatal("管理员会话应能签发桥接凭据")
	}
	if !exp.Equal(time.Unix(texp.Unix(), 0)) {
		t.Fatalf("凭据过期时间应与会话相同: %v vs %v", exp, texp)
	}
	if user, ok := m.VerifyBridge(v); !ok || user != "admin" {
		t.Fatalf("凭据应校验通过: %q %v", user, ok)
	}
}

// 桥接凭据与会话令牌互不通用：凭据不能当会话令牌使用，会话令牌也不能当凭据使用。
func TestBridgeNotInterchangeableWithToken(t *testing.T) {
	m, pass := newTestManager(t)
	token, _, _ := m.Login("1.1.1.1", "admin", pass)
	v, _, _ := m.IssueBridge(token)
	if _, ok := m.Identify(v); ok {
		t.Fatal("桥接凭据不应被当作后台会话令牌")
	}
	if _, ok := m.VerifyBridge(token); ok {
		t.Fatal("后台会话令牌不应被当作桥接凭据")
	}
}

// 没有有效会话、被伪造或篡改的内容不能签发或通过校验。
func TestBridgeRejectsInvalid(t *testing.T) {
	m, pass := newTestManager(t)
	for _, tk := range []string{"", "garbage", "a.b", "x.y.z"} {
		if _, _, ok := m.IssueBridge(tk); ok {
			t.Fatalf("无效令牌 %q 不应签发凭据", tk)
		}
		if _, ok := m.VerifyBridge(tk); ok {
			t.Fatalf("无效凭据 %q 不应通过", tk)
		}
	}
	token, _, _ := m.Login("1.1.1.1", "admin", pass)
	v, _, _ := m.IssueBridge(token)
	// 篡改负载或签名
	p, sig, _ := strings.Cut(v, ".")
	for _, bad := range []string{p + "." + sig[:len(sig)-2] + "xx", p[:len(p)-2] + "xx." + sig, "." + sig, p + "."} {
		if _, ok := m.VerifyBridge(bad); ok {
			t.Fatalf("被篡改的凭据不应通过: %q", bad)
		}
	}
	// 另一个实例（密钥不同）签发的凭据不通过
	other, opass := newTestManager(t)
	otoken, _, _ := other.Login("1.1.1.1", "admin", opass)
	ov, _, _ := other.IssueBridge(otoken)
	if _, ok := m.VerifyBridge(ov); ok {
		t.Fatal("其他实例签发的凭据不应通过")
	}
}

// 只读账号的会话不能签发凭据：只读账号不应获得站点的免认证权限。
func TestBridgeNotForReadOnly(t *testing.T) {
	m, pass := newTestManager(t)
	if err := m.AddUser("viewer", "viewer-pass-1"); err != nil {
		t.Fatal(err)
	}
	token, _, err := m.Login("1.1.1.2", "viewer", "viewer-pass-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Identify(token); !ok {
		t.Fatal("只读账号应能登录")
	}
	if _, _, ok := m.IssueBridge(token); ok {
		t.Fatal("只读账号不应签发桥接凭据")
	}
	_ = pass
}

// 登出（吊销会话）后凭据立即失效；其他会话签发的凭据不受影响。
func TestBridgeRevokedWithSession(t *testing.T) {
	m, pass := newTestManager(t)
	a, _, _ := m.Login("1.1.1.1", "admin", pass)
	b, _, _ := m.Login("1.1.1.2", "admin", pass)
	va, _, _ := m.IssueBridge(a)
	vb, _, _ := m.IssueBridge(b)
	if err := m.Revoke(a); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.VerifyBridge(va); ok {
		t.Fatal("登出后凭据应失效")
	}
	if _, ok := m.VerifyBridge(vb); !ok {
		t.Fatal("其他会话的凭据不应受影响")
	}
	// 已吊销的会话不能再签发凭据
	if _, _, ok := m.IssueBridge(a); ok {
		t.Fatal("已登出的会话不应再签发凭据")
	}
}

// 退出所有设备、修改密码后凭据失效。
func TestBridgeInvalidatedByEpochAndPassword(t *testing.T) {
	m, pass := newTestManager(t)
	a, _, _ := m.Login("1.1.1.1", "admin", pass)
	va, _, _ := m.IssueBridge(a)
	if err := m.RevokeAll(); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.VerifyBridge(va); ok {
		t.Fatal("退出所有设备后凭据应失效")
	}

	m2, pass2 := newTestManager(t)
	b, _, _ := m2.Login("1.1.1.1", "admin", pass2)
	vb, _, _ := m2.IssueBridge(b)
	if err := m2.ChangeCredentials(pass2, "admin", "a-brand-new-pass-1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := m2.VerifyBridge(vb); ok {
		t.Fatal("修改密码后凭据应失效")
	}
}

// 会话过期后凭据随之失效。
func TestBridgeExpires(t *testing.T) {
	m, pass := newTestManager(t)
	a, _, _ := m.Login("1.1.1.1", "admin", pass)
	v, _, _ := m.IssueBridge(a)
	base := time.Now()
	m.now = func() time.Time { return base.Add(48 * time.Hour * 365) }
	if _, ok := m.VerifyBridge(v); ok {
		t.Fatal("会话过期后凭据应失效")
	}
}

func BenchmarkBridgeVerify(b *testing.B) {
	m := NewManager(Account{}, func(Account) error { return nil })
	pass, _ := m.EnsureInitialized()
	token, _, _ := m.Login("1.1.1.1", "admin", pass)
	v, _, _ := m.IssueBridge(token)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := m.VerifyBridge(v); !ok {
			b.Fatal("校验失败")
		}
	}
}
