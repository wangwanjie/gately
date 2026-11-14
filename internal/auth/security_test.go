package auth

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/webauthn"
)

// strongPass 是满足强度要求的测试密码。
const strongPass = "Zm3$kq9Lr!vT"

// newStrongManager 创建管理员密码已改为强密码的管理器。
func newStrongManager(t *testing.T) *Manager {
	t.Helper()
	m, pass := newTestManager(t)
	if err := m.ChangeCredentials(pass, "admin", strongPass); err != nil {
		t.Fatal(err)
	}
	return m
}

// clock 是可推进的测试时钟。
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func useClock(m *Manager) *clock {
	c := &clock{t: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)}
	m.now = c.Now
	return c
}

// ---- 弱密码标记 ----

func TestPasswordIssueDefaultThenChange(t *testing.T) {
	m, pass := newTestManager(t)
	if got := m.PasswordIssue(); got != "default" {
		t.Fatalf("新装应为 default，得到 %q", got)
	}
	// 默认密码登录成功不改变 default 状态
	if _, _, err := m.Login("1.1.1.1", "admin", pass); err != nil {
		t.Fatal(err)
	}
	if got := m.PasswordIssue(); got != "default" {
		t.Fatalf("登录后应仍为 default，得到 %q", got)
	}
	if err := m.ChangeCredentials(pass, "admin", strongPass); err != nil {
		t.Fatal(err)
	}
	if got := m.PasswordIssue(); got != "" {
		t.Fatalf("改密后应无问题，得到 %q", got)
	}
}

func TestChangeCredentialsRejectsWeak(t *testing.T) {
	m, pass := newTestManager(t)
	for _, weakPass := range []string{"short", "password123", "gately.cn", "12345678901", "admin-Router-9x"} {
		if err := m.ChangeCredentials(pass, "admin", weakPass); err == nil {
			t.Errorf("弱密码 %q 应被拒绝", weakPass)
		}
	}
	// 与原密码相同
	if err := m.ChangeCredentials(strongPassOrDefault(m, pass), "admin", pass); err == nil {
		t.Error("新密码与默认密码相同应被拒绝")
	}
	// 被拒绝的修改不改变账号
	if _, _, err := m.Login("1.1.1.1", "admin", pass); err != nil {
		t.Fatalf("拒绝后原密码应仍可登录: %v", err)
	}
}

func strongPassOrDefault(_ *Manager, pass string) string { return pass }

func TestChangeCredentialsSameAsOld(t *testing.T) {
	m := newStrongManager(t)
	if err := m.ChangeCredentials(strongPass, "admin", strongPass); err == nil {
		t.Fatal("新密码与原密码相同应被拒绝")
	}
}

// 存量账号：已有的弱密码在下次密码登录成功时被识别，改强后清除。
func TestLegacyWeakPasswordDetectedAtLogin(t *testing.T) {
	var saved Account
	m := NewManager(Account{}, func(a Account) error { saved = a; return nil })
	m.SetBootstrapPassword("Password2024!") // 初始密码不够强
	if _, err := m.EnsureInitialized(); err != nil {
		t.Fatal(err)
	}
	if got := m.PasswordIssue(); got != "weak" {
		t.Fatalf("弱的初始密码应标记 weak，得到 %q", got)
	}
	if !saved.WeakPassword {
		t.Fatal("弱密码标记应持久化")
	}
	// 模拟升级前创建的账号：标记为空，但密码是弱的
	acc := saved
	acc.WeakPassword = false
	m2 := NewManager(acc, func(a Account) error { saved = a; return nil })
	if got := m2.PasswordIssue(); got != "" {
		t.Fatalf("旧账号尚未登录时无法判定，应为空，得到 %q", got)
	}
	if _, _, err := m2.Login("1.1.1.1", "admin", "Password2024!"); err != nil {
		t.Fatal(err)
	}
	if got := m2.PasswordIssue(); got != "weak" {
		t.Fatalf("登录后应识别为 weak，得到 %q", got)
	}
	if !saved.WeakPassword {
		t.Fatal("识别结果应持久化")
	}
	if err := m2.ChangeCredentials("Password2024!", "admin", strongPass); err != nil {
		t.Fatal(err)
	}
	if m2.PasswordIssue() != "" || saved.WeakPassword {
		t.Fatal("改成强密码后应清除标记")
	}
}

func TestStrongBootstrapPasswordNotFlagged(t *testing.T) {
	m := NewManager(Account{}, func(Account) error { return nil })
	if err := m.SetBootstrapPassword(strongPass); err != nil {
		t.Fatal(err)
	}
	initial, err := m.EnsureInitialized()
	if err != nil || initial != "" {
		t.Fatalf("指定初始密码不应返回默认密码: %q %v", initial, err)
	}
	if m.PasswordIssue() != "" {
		t.Fatal("强初始密码不应有问题")
	}
}

func TestAddUserAndSetPasswordRejectWeak(t *testing.T) {
	m := newStrongManager(t)
	if err := m.AddUser("viewer", "viewer12345"); err == nil {
		t.Error("含用户名的密码应被拒绝")
	}
	if err := m.AddUser("viewer", "short"); err == nil {
		t.Error("过短密码应被拒绝")
	}
	if err := m.AddUser("viewer", "tide-Lamp-47-oak"); err != nil {
		t.Fatal(err)
	}
	if err := m.SetUserPassword("viewer", "12345678901"); err == nil {
		t.Error("重置为弱密码应被拒绝")
	}
	if err := m.SetUserPassword("viewer", "oak-Lamp-91-tide"); err != nil {
		t.Fatal(err)
	}
}

func TestNewPasswordResetRejectsWeak(t *testing.T) {
	if _, err := NewPasswordReset("password123"); err == nil {
		t.Error("自定义弱密码不应被接受")
	}
	if _, err := NewPasswordReset(""); err != nil {
		t.Errorf("空密码表示重置为默认密码: %v", err)
	}
	if _, err := NewPasswordReset(strongPass); err != nil {
		t.Errorf("强密码应被接受: %v", err)
	}
}

// ---- 递增锁定 ----

func failN(m *Manager, ip string, n int) {
	for i := 0; i < n; i++ {
		_, _, _ = m.Login(ip, "admin", "wrong")
	}
}

func TestLockDurationEscalates(t *testing.T) {
	m := newStrongManager(t)
	c := useClock(m)
	ip := "7.7.7.7"
	want := []time.Duration{15 * time.Minute, 30 * time.Minute, 60 * time.Minute, 120 * time.Minute}
	for i, d := range want {
		failN(m, ip, maxFailures)
		// 锁定期内即使密码正确也被拒绝
		if _, _, err := m.Login(ip, "admin", strongPass); !errors.Is(err, ErrLocked) {
			t.Fatalf("第 %d 轮应被锁定: %v", i+1, err)
		}
		c.Advance(d - time.Second)
		if _, _, err := m.Login(ip, "admin", strongPass); !errors.Is(err, ErrLocked) {
			t.Fatalf("第 %d 轮锁定未到期应仍被拒绝（锁定 %v）: %v", i+1, d, err)
		}
		c.Advance(2 * time.Second)
		if i < len(want)-1 {
			// 到期后重新累计失败
			continue
		}
	}
	// 到期后密码正确可登录
	if _, _, err := m.Login(ip, "admin", strongPass); err != nil {
		t.Fatalf("锁定到期后应可登录: %v", err)
	}
}

func TestLockDurationCapped(t *testing.T) {
	m := newStrongManager(t)
	c := useClock(m)
	ip := "7.7.7.8"
	var longest time.Duration
	for i := 0; i < 12; i++ {
		failN(m, ip, maxFailures)
		m.mu.RLock()
		st := m.fails[limitKey(ip)]
		d := st.lockedUntil.Sub(st.lastStrike)
		until := st.lockedUntil
		m.mu.RUnlock()
		if d > maxLockDuration {
			t.Fatalf("第 %d 轮锁定时长超过上限: %v", i+1, d)
		}
		if d > longest {
			longest = d
		}
		c.Advance(until.Sub(c.Now()) + time.Second)
	}
	if longest < 8*time.Hour {
		t.Fatalf("多轮后锁定时长应明显增长，最长只有 %v", longest)
	}
}

func TestStrikesDecayAfterMemory(t *testing.T) {
	m := newStrongManager(t)
	c := useClock(m)
	ip := "7.7.7.9"
	failN(m, ip, maxFailures)
	c.Advance(lockDuration + time.Second)
	failN(m, ip, maxFailures) // 第二次：30 分钟
	c.Advance(strikeMemory + time.Hour)
	failN(m, ip, maxFailures) // 记录过期，重新从 15 分钟算
	m.mu.RLock()
	st := m.fails[limitKey(ip)]
	m.mu.RUnlock()
	if st.strikes != 1 {
		t.Fatalf("记录过期后应重新计数，strikes=%d", st.strikes)
	}
	if d := st.lockedUntil.Sub(c.Now()); d > lockDuration {
		t.Fatalf("重新计数后锁定时长应回到基础值: %v", d)
	}
}

// 递增记录不能被一次成功登录洗掉：攻击者拿到过一次有效凭据也不应重置历史。
func TestStrikesSurviveSuccess(t *testing.T) {
	m := newStrongManager(t)
	c := useClock(m)
	ip := "7.7.7.10"
	failN(m, ip, maxFailures)
	c.Advance(lockDuration + time.Second)
	if _, _, err := m.Login(ip, "admin", strongPass); err != nil {
		t.Fatal(err)
	}
	failN(m, ip, maxFailures)
	m.mu.RLock()
	st := m.fails[limitKey(ip)]
	m.mu.RUnlock()
	if st.strikes != 2 {
		t.Fatalf("成功登录后再次触发锁定应为第 2 次，strikes=%d", st.strikes)
	}
}

// 持有只读账号的人不能靠登录只读账号来清零管理员密码的失败计数。
func TestReadonlyLoginDoesNotResetFailCount(t *testing.T) {
	m := newStrongManager(t)
	if err := m.AddUser("viewer", "tide-Lamp-47-oak"); err != nil {
		t.Fatal(err)
	}
	ip := "7.7.7.11"
	for round := 0; round < 3; round++ {
		failN(m, ip, 2)
		if _, _, err := m.Login(ip, "viewer", "tide-Lamp-47-oak"); err != nil && !errors.Is(err, ErrLocked) {
			t.Fatal(err)
		}
	}
	// 6 次失败穿插只读登录：应已触发锁定
	if _, _, err := m.Login(ip, "admin", strongPass); !errors.Is(err, ErrLocked) {
		t.Fatalf("穿插只读登录不应清零失败计数，管理员应被锁定: %v", err)
	}
}

// 限流表有上限：直接走预占/释放，避免为两万个地址各做一次 bcrypt。
func TestTrackedMapBounded(t *testing.T) {
	m := newStrongManager(t)
	useClock(m)
	var rejected int
	for i := 0; i < maxTracked+500; i++ {
		key := fmt.Sprintf("10.%d.%d.%d", i>>16&255, i>>8&255, i&255)
		if err := m.reserve(key, false); err != nil {
			rejected++
			continue
		}
		m.release(key, false, false)
	}
	m.mu.RLock()
	n := len(m.fails)
	m.mu.RUnlock()
	if n > maxTracked {
		t.Fatalf("限流表超过上限: %d", n)
	}
	if rejected != 500 {
		t.Fatalf("表满后新来源应被拒绝，实际拒绝 %d", rejected)
	}
	// 已在表里的来源不受影响：能继续被判断为锁定或放行
	if err := m.reserve("10.0.0.0", false); err != nil && !errors.Is(err, ErrLocked) {
		t.Fatalf("已跟踪来源应得到锁定判断而非繁忙: %v", err)
	}
}

// ---- 陌生来源全局失败预算 ----

func knownSet(ips ...string) func(string) bool {
	set := map[string]bool{}
	for _, ip := range ips {
		set[ip] = true
	}
	return func(ip string) bool { return set[ip] }
}

func TestGlobalBudgetBlocksUnknownOnly(t *testing.T) {
	m := newStrongManager(t)
	c := useClock(m)
	var tripped atomic.Int32
	trips := make(chan int, 4)
	m.SetGlobalLimit(6, knownSet("9.9.9.9"), func(n int) { tripped.Add(1); trips <- n })

	// 三个陌生地址各失败两次（都未触及单 IP 上限）耗尽全局预算
	for _, ip := range []string{"20.0.0.1", "20.0.0.2", "20.0.0.3"} {
		failN(m, ip, 2)
	}
	// 新的陌生地址即使密码正确也被拒绝
	if _, _, err := m.Login("20.0.0.4", "admin", strongPass); !errors.Is(err, ErrBusy) {
		t.Fatalf("陌生地址应因全局预算耗尽被拒: %v", err)
	}
	select {
	case n := <-trips:
		if n != 6 {
			t.Errorf("回调应报告失败数 6，得到 %d", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("预算耗尽应触发回调")
	}
	// 窗口内重复被拒不重复触发回调
	_, _, _ = m.Login("20.0.0.5", "admin", strongPass)
	time.Sleep(50 * time.Millisecond)
	if tripped.Load() != 1 {
		t.Fatalf("窗口内只应触发一次回调，实际 %d 次", tripped.Load())
	}
	// 常用地址不受影响
	if _, _, err := m.Login("9.9.9.9", "admin", strongPass); err != nil {
		t.Fatalf("常用地址应可登录: %v", err)
	}
	// 窗口过后恢复
	c.Advance(globalWindow + time.Second)
	if _, _, err := m.Login("20.0.0.4", "admin", strongPass); err != nil {
		t.Fatalf("窗口过后陌生地址应可登录: %v", err)
	}
	// 新窗口里可再次触发回调
	failN(m, "20.1.0.1", 2)
	failN(m, "20.1.0.2", 2)
	failN(m, "20.1.0.3", 2)
	_, _, _ = m.Login("20.1.0.4", "admin", strongPass)
	select {
	case <-trips:
	case <-time.After(2 * time.Second):
		t.Fatal("新窗口应能再次触发回调")
	}
}

func TestGlobalBudgetOffByDefault(t *testing.T) {
	m := newStrongManager(t)
	useClock(m)
	// 未启用时陌生地址不受预算约束
	for i := 0; i < 10; i++ {
		failN(m, fmt.Sprintf("21.0.0.%d", i), 2)
	}
	if _, _, err := m.Login("21.0.1.1", "admin", strongPass); err != nil {
		t.Fatalf("未启用预算时应可登录: %v", err)
	}
	// limit<=0 同样关闭
	m.SetGlobalLimit(0, knownSet(), nil)
	for i := 0; i < 10; i++ {
		failN(m, fmt.Sprintf("21.0.2.%d", i), 2)
	}
	if _, _, err := m.Login("21.0.3.1", "admin", strongPass); err != nil {
		t.Fatalf("limit=0 时应可登录: %v", err)
	}
}

// 成功登录不消耗预算；缺验证码（pending）也不消耗预算。
func TestGlobalBudgetSuccessAndPendingDoNotConsume(t *testing.T) {
	m := newStrongManager(t)
	useClock(m)
	m.SetGlobalLimit(3, knownSet(), nil)
	for i := 0; i < 10; i++ {
		ip := fmt.Sprintf("22.0.0.%d", i)
		if _, _, err := m.Login(ip, "admin", strongPass); err != nil {
			t.Fatalf("成功登录不应消耗预算: %v", err)
		}
	}
}

// 并发的陌生来源失败不能绕过预算：预占名额按并发数计入。
func TestGlobalBudgetConcurrent(t *testing.T) {
	m := newStrongManager(t)
	useClock(m)
	m.SetGlobalLimit(5, knownSet(), nil)
	var wg sync.WaitGroup
	var busy, invalid atomic.Int32
	for i := 0; i < 60; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := m.Login(fmt.Sprintf("23.0.%d.1", i), "admin", "wrong")
			switch {
			case errors.Is(err, ErrBusy):
				busy.Add(1)
			case errors.Is(err, ErrInvalidCredentials):
				invalid.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if invalid.Load() > 5 {
		t.Fatalf("预算 5 次，实际执行了 %d 次密码校验", invalid.Load())
	}
	if busy.Load() == 0 {
		t.Fatal("超出预算的请求应被拒绝")
	}
}

func TestGlobalBudgetAppliesToPasskey(t *testing.T) {
	m := newStrongManager(t)
	useClock(m)
	m.SetGlobalLimit(2, knownSet(), nil)
	verify := func(webauthn.Credential) (uint32, error) { return 0, errors.New("断言无效") }
	_, _, _ = m.LoginPasskey("24.0.0.1", "nope", verify)
	_, _, _ = m.LoginPasskey("24.0.0.2", "nope", verify)
	if _, _, err := m.LoginPasskey("24.0.0.3", "nope", verify); !errors.Is(err, ErrBusy) {
		t.Fatalf("Passkey 登录同样受全局预算限制: %v", err)
	}
}

// ---- 关闭密码登录 ----

func addTestPasskey(t *testing.T, m *Manager, id string) {
	t.Helper()
	if err := m.AddPasskey(strongPass, &webauthn.Credential{ID: id, Name: id}); err != nil {
		t.Fatal(err)
	}
}

func TestPasswordLoginOffRequiresPasskey(t *testing.T) {
	m := newStrongManager(t)
	if err := m.SetPasswordLoginOff(strongPass, true); err == nil {
		t.Fatal("没有 Passkey 时不能关闭密码登录")
	}
	addTestPasskey(t, m, "k1")
	if err := m.SetPasswordLoginOff("wrong-password-1", true); err == nil {
		t.Fatal("当前密码错误时不能修改")
	}
	if err := m.SetPasswordLoginOff(strongPass, true); err != nil {
		t.Fatal(err)
	}
	if !m.PasswordLoginOff() {
		t.Fatal("应已关闭密码登录")
	}
	// 密码正确也无法登录，且错误与密码错误一致，不泄露账号存在与否
	_, _, err := m.Login("1.1.1.1", "admin", strongPass)
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("应返回统一的凭据错误: %v", err)
	}
	_, _, err2 := m.Login("1.1.1.2", "nobody", strongPass)
	if err.Error() != err2.Error() {
		t.Fatalf("关闭密码登录后的错误应与账号不存在一致: %q vs %q", err, err2)
	}
	// 重新开启
	if err := m.SetPasswordLoginOff(strongPass, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Login("1.1.1.3", "admin", strongPass); err != nil {
		t.Fatalf("重新开启后应可登录: %v", err)
	}
}

func TestPasswordLoginOffKeepsReadonlyAndPasskey(t *testing.T) {
	m := newStrongManager(t)
	if err := m.AddUser("viewer", "tide-Lamp-47-oak"); err != nil {
		t.Fatal(err)
	}
	addTestPasskey(t, m, "k1")
	if err := m.SetPasswordLoginOff(strongPass, true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Login("1.1.1.1", "viewer", "tide-Lamp-47-oak"); err != nil {
		t.Fatalf("只读账号不受影响: %v", err)
	}
	ok := func(webauthn.Credential) (uint32, error) { return 1, nil }
	if _, _, err := m.LoginPasskey("1.1.1.2", "k1", ok); err != nil {
		t.Fatalf("Passkey 登录应可用: %v", err)
	}
}

func TestPasswordLoginOffBlocksRemovingLastPasskey(t *testing.T) {
	m := newStrongManager(t)
	addTestPasskey(t, m, "k1")
	addTestPasskey(t, m, "k2")
	if err := m.SetPasswordLoginOff(strongPass, true); err != nil {
		t.Fatal(err)
	}
	if err := m.RemovePasskey(strongPass, "k1"); err != nil {
		t.Fatalf("还剩一个时可以删除: %v", err)
	}
	if err := m.RemovePasskey(strongPass, "k2"); err == nil {
		t.Fatal("不能删除最后一个 Passkey")
	}
	if err := m.SetPasswordLoginOff(strongPass, false); err != nil {
		t.Fatal(err)
	}
	if err := m.RemovePasskey(strongPass, "k2"); err != nil {
		t.Fatalf("重新开启密码登录后可以删除: %v", err)
	}
}

// 命令行重置密码是找回访问权限的途径：必须同时重新开启密码登录。
func TestResetReenablesPasswordLogin(t *testing.T) {
	m := newStrongManager(t)
	addTestPasskey(t, m, "k1")
	if err := m.SetPasswordLoginOff(strongPass, true); err != nil {
		t.Fatal(err)
	}
	req, err := NewPasswordReset("Tq8!wLm3-zRv9")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.ApplyReset(req); err != nil {
		t.Fatal(err)
	}
	if m.PasswordLoginOff() {
		t.Fatal("重置密码后应重新开启密码登录")
	}
	if _, _, err := m.Login("1.1.1.1", "admin", "Tq8!wLm3-zRv9"); err != nil {
		t.Fatalf("重置后应可用新密码登录: %v", err)
	}
	if m.PasswordIssue() != "" {
		t.Fatalf("重置为强密码后不应有问题: %q", m.PasswordIssue())
	}
}

func TestSnapshot(t *testing.T) {
	m := newStrongManager(t)
	m.SetLimits(7, 20*time.Minute)
	addTestPasskey(t, m, "k1")
	s := m.Snapshot()
	if s.Username != "admin" || s.PasswordIssue != "" || s.Passkeys != 1 || s.TOTP || s.MaxFail != 7 || s.LockDuration != 20*time.Minute {
		t.Fatalf("快照不符: %+v", s)
	}
}
