package webservice

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// loginSite 返回一个用 bcrypt 保存密码的网页认证站点
func loginSite(t *testing.T) Site {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte("right-pw"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return Site{Enable: true, Key: "rl", Domains: []string{"rl.test"}, WebAuth: true, Users: []string{"alice:" + string(h)}}
}

// postLogin 以同一来源 IP 提交一次登录表单，返回状态码与页面内容
func postLogin(a *authenticator, s Site, pass string) (int, string) {
	form := url.Values{"username": {"alice"}, "password": {pass}, "next": {"/"}}
	req := httptest.NewRequest(http.MethodPost, "http://rl.test"+s.authPrefix()+"/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	a.handleLogin(w, req, s, "198.51.100.7")
	return w.Code, w.Body.String()
}

// failures 返回某来源已记录的失败次数
func failures(a *authenticator, key string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if f := a.fails[key]; f != nil {
		return f.n
	}
	return 0
}

// TestConcurrentCorrectLoginsNotLimited 验证同一来源并发的正确登录全部成功，不被当作失败过多，也不累加失败次数。
func TestConcurrentCorrectLoginsNotLimited(t *testing.T) {
	a := newAuthenticator("s")
	s := loginSite(t)
	var wg sync.WaitGroup
	var ok, other atomic.Int64
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if code, _ := postLogin(a, s, "right-pw"); code == http.StatusFound {
				ok.Add(1)
			} else {
				other.Add(1)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 30 {
		t.Fatalf("30 次并发正确登录应全部成功, 成功 %d 其他 %d", ok.Load(), other.Load())
	}
	if n := failures(a, "198.51.100.7"); n != 0 {
		t.Fatalf("正确登录不应计入失败次数: %d", n)
	}
}

// TestConcurrentWrongLoginsCapped 验证并发的错误登录最多校验 maxFailures 次，之后提示失败次数过多。
func TestConcurrentWrongLoginsCapped(t *testing.T) {
	a := newAuthenticator("s")
	s := loginSite(t)
	var wg sync.WaitGroup
	var wrong, locked atomic.Int64
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, body := postLogin(a, s, "bad-pw")
			switch {
			case strings.Contains(body, "失败次数过多"):
				locked.Add(1)
			case strings.Contains(body, "用户名或密码错误"):
				wrong.Add(1)
			}
		}()
	}
	wg.Wait()
	if wrong.Load() != maxFailures || locked.Load() != 30-maxFailures {
		t.Fatalf("应恰好校验 %d 次错误密码, 实际错误 %d 锁定 %d", maxFailures, wrong.Load(), locked.Load())
	}
	if n := failures(a, "198.51.100.7"); n != maxFailures {
		t.Fatalf("失败次数应为 %d: %d", maxFailures, n)
	}
	// 锁定期内正确密码同样被拒绝
	if code, body := postLogin(a, s, "right-pw"); code == http.StatusFound || !strings.Contains(body, "失败次数过多") {
		t.Fatalf("锁定期内应拒绝登录: %d", code)
	}
}

// TestConcurrentMixedLogins 验证正确与错误登录混合并发时：正确登录在锁定前都能通过，错误校验总数不超过上限。
func TestConcurrentMixedLogins(t *testing.T) {
	a := newAuthenticator("s")
	s := loginSite(t)
	var wg sync.WaitGroup
	var good, wrong, locked atomic.Int64
	// 错误登录少于上限：所有正确登录都应成功
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pass := "right-pw"
			if i%5 == 0 {
				pass = "bad-pw"
			}
			code, body := postLogin(a, s, pass)
			switch {
			case code == http.StatusFound:
				good.Add(1)
			case strings.Contains(body, "失败次数过多"):
				locked.Add(1)
			case strings.Contains(body, "用户名或密码错误"):
				wrong.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if good.Load() != 24 || wrong.Load() != 6 || locked.Load() != 0 {
		t.Fatalf("混合登录结果不符: 成功 %d 错误 %d 锁定 %d", good.Load(), wrong.Load(), locked.Load())
	}
	// 错误登录超过上限：错误校验总数仍不超过上限
	a2 := newAuthenticator("s")
	good.Store(0)
	wrong.Store(0)
	locked.Store(0)
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pass := "bad-pw"
			if i%2 == 0 {
				pass = "right-pw"
			}
			code, body := postLogin(a2, s, pass)
			switch {
			case code == http.StatusFound:
				good.Add(1)
			case strings.Contains(body, "失败次数过多"):
				locked.Add(1)
			case strings.Contains(body, "用户名或密码错误"):
				wrong.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if wrong.Load() > maxFailures || good.Load()+wrong.Load()+locked.Load() != 40 {
		t.Fatalf("错误校验不应超过上限: 成功 %d 错误 %d 锁定 %d", good.Load(), wrong.Load(), locked.Load())
	}
	if n := failures(a2, "198.51.100.7"); n != int(wrong.Load()) {
		t.Fatalf("失败次数应等于错误校验数 %d: %d", wrong.Load(), n)
	}
}

// TestReserveInflightCap 验证同一来源同时进行的校验不超过 maxInflight，空位释放后排队的请求继续。
func TestReserveInflightCap(t *testing.T) {
	a := newAuthenticator("s")
	ctx := context.Background()
	for i := 0; i < maxInflight; i++ {
		if a.reserve(ctx, "k") != reserveOK {
			t.Fatal("并发上限内应占位成功")
		}
	}
	got := make(chan reserveResult, 1)
	go func() {
		got <- a.reserve(ctx, "k")
	}()
	select {
	case r := <-got:
		t.Fatalf("达到并发上限时应排队等待, 却返回 %v", r)
	case <-time.After(50 * time.Millisecond):
	}
	a.release("k", true)
	select {
	case r := <-got:
		if r != reserveOK {
			t.Fatalf("空位释放后应占位成功: %v", r)
		}
	case <-time.After(time.Second):
		t.Fatal("空位释放后排队的请求应被唤醒")
	}
}

// TestReserveBusyNotCountedAsFailure 验证排队超时（这里用取消请求模拟）与队列已满时返回 reserveBusy，且不累加失败次数。
func TestReserveBusyNotCountedAsFailure(t *testing.T) {
	a := newAuthenticator("s")
	for i := 0; i < maxInflight; i++ {
		a.reserve(context.Background(), "k")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if r := a.reserve(ctx, "k"); r != reserveBusy {
		t.Fatalf("等待被取消时应返回 reserveBusy: %v", r)
	}
	// 排队请求达到上限后，新请求立即返回 reserveBusy
	a.mu.Lock()
	a.fails["k"].waiting = maxAuthWaiting
	a.mu.Unlock()
	start := time.Now()
	if r := a.reserve(context.Background(), "k"); r != reserveBusy {
		t.Fatalf("排队已满时应返回 reserveBusy: %v", r)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("排队已满时应立即返回")
	}
	a.mu.Lock()
	a.fails["k"].waiting = 0
	a.mu.Unlock()
	if n := failures(a, "k"); n != 0 {
		t.Fatalf("请求过多不应计入失败次数: %d", n)
	}
	if reserveDenyText(reserveBusy) == reserveDenyText(reserveLocked) {
		t.Fatal("请求过多与失败次数过多的提示应不同")
	}
}
