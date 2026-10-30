package auth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// RFC 6238 附录 B 的 SHA1 测试向量（密钥为 ASCII "12345678901234567890"，取 8 位中的后 6 位）
func TestTOTPRFCVector(t *testing.T) {
	secret := b32.EncodeToString([]byte("12345678901234567890"))
	cases := map[int64]string{59: "287082", 1111111109: "081804", 1234567890: "005924", 2000000000: "279037"}
	for ts, want := range cases {
		got, err := totpCode(secret, ts/totpPeriod)
		if err != nil || got != want {
			t.Errorf("t=%d got %s want %s", ts, got, want)
		}
		if !ValidateTOTP(secret, want, time.Unix(ts, 0)) {
			t.Errorf("t=%d 校验失败", ts)
		}
	}
	if ValidateTOTP(secret, "000000", time.Unix(59, 0)) {
		t.Error("错误验证码不应通过")
	}
	if !strings.HasPrefix(TOTPURL(secret, "admin"), "otpauth://totp/Gately:admin?") {
		t.Error("otpauth 地址格式错误")
	}
}

func TestLoginWithTOTP(t *testing.T) {
	m, pass := newTestManager(t)
	secret := NewTOTPSecret()
	if err := m.SetTOTP("wrong", "", secret); err == nil {
		t.Fatal("密码错误时不应启用")
	}
	if err := m.SetTOTP(pass, "", secret); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Login("1.1.1.1", "admin", pass); !errors.Is(err, ErrTOTPRequired) {
		t.Fatalf("应要求验证码: %v", err)
	}
	if _, _, err := m.Login("1.1.1.1", "admin", pass, "123456"); err == nil {
		t.Fatal("错误验证码应失败")
	}
	code, _ := totpCode(secret, time.Now().Unix()/totpPeriod)
	token, _, err := m.Login("1.1.1.1", "admin", pass, code)
	if err != nil {
		t.Fatal(err)
	}
	// 同一验证码不能重放
	if _, _, err := m.Login("1.1.1.2", "admin", pass, code); err == nil {
		t.Fatal("验证码不应能重复使用")
	}
	if err := m.SetTOTP(pass, "", ""); err == nil {
		t.Fatal("关闭两步验证应要求验证码")
	}
	// 下一个时间窗口的验证码
	m.now = func() time.Time { return time.Now().Add(totpPeriod * time.Second) }
	next, _ := totpCode(secret, m.now().Unix()/totpPeriod)
	if err := m.SetTOTP(pass, next, ""); err != nil || m.TOTPEnabled() {
		t.Fatalf("关闭两步验证失败: %v", err)
	}
	if _, ok := m.Verify(token); ok {
		t.Fatal("切换两步验证后旧令牌应失效")
	}
}

// 密码正确但不带验证码的请求不能清零失败计数，否则攻击者可在每 4 次猜错之间插入一次来无限爆破验证码。
func TestTOTPCodelessRequestKeepsFailures(t *testing.T) {
	m, pass := newTestManager(t)
	if err := m.SetTOTP(pass, "", NewTOTPSecret()); err != nil {
		t.Fatal(err)
	}
	for round := 0; round < 3; round++ {
		for i := 0; i < maxFailures-1; i++ {
			if _, _, err := m.Login("9.9.9.9", "admin", pass, "000000"); errors.Is(err, ErrLocked) {
				return
			}
		}
		if _, _, err := m.Login("9.9.9.9", "admin", pass); errors.Is(err, ErrLocked) {
			return
		}
	}
	t.Fatal("交错发送不带验证码的请求后仍未被锁定")
}
