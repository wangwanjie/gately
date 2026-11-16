package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/webauthn"
	"github.com/wangwanjie/gately/internal/webauthn/webauthntest"
)

// 挑战池：单个来源有限额，总量满时淘汰最早的挑战而不是拒绝所有人。
func TestChallengeStoreQuota(t *testing.T) {
	var s challengeStore
	var mine []string
	for i := 0; i < passkeyMaxPerSource; i++ {
		c, ok := s.issue("203.0.113.9")
		if !ok {
			t.Fatalf("第 %d 个挑战应能下发", i+1)
		}
		mine = append(mine, c)
	}
	if _, ok := s.issue("203.0.113.9"); ok {
		t.Fatal("同一来源超过限额后应被拒绝")
	}
	// IPv6 按 /64 计：同一网段换地址不能绕过
	for i := 0; i < passkeyMaxPerSource; i++ {
		if _, ok := s.issue("2001:db8:1:2::" + strconv.Itoa(i+1)); !ok {
			t.Fatalf("IPv6 第 %d 个挑战应能下发", i+1)
		}
	}
	if _, ok := s.issue("2001:db8:1:2:ffff::1"); ok {
		t.Fatal("同一 /64 内换地址不应绕过限额")
	}
	// 其他来源不受影响
	other, ok := s.issue("198.51.100.7")
	if !ok {
		t.Fatal("其他来源应不受影响")
	}
	// 用掉一个挑战即归还名额；每个挑战只能用一次
	if !s.consume(mine[0]) || s.consume(mine[0]) {
		t.Fatal("挑战应能使用且只能使用一次")
	}
	if _, ok := s.issue("203.0.113.9"); !ok {
		t.Fatal("用掉挑战后应归还该来源的名额")
	}
	// 大量来源把总量占满：新的请求仍能拿到挑战，被淘汰的是最早的那个
	for i := 0; len(s.pending) < passkeyMaxPending; i++ {
		if _, ok := s.issue("10." + strconv.Itoa(i/65536%256) + "." + strconv.Itoa(i/256%256) + "." + strconv.Itoa(i%256)); !ok {
			t.Fatalf("不同来源的挑战应能下发（第 %d 个）", i)
		}
	}
	fresh, ok := s.issue("192.0.2.200")
	if !ok || len(s.pending) != passkeyMaxPending {
		t.Fatalf("总量满时应淘汰旧挑战后下发新的: %v %d", ok, len(s.pending))
	}
	if !s.consume(fresh) || !s.consume(other) {
		t.Fatal("刚下发的挑战与未被淘汰的挑战应仍可使用")
	}
	// 来源未知（空）时不计来源配额，只受总量约束
	var u challengeStore
	for i := 0; i < passkeyMaxPerSource*4; i++ {
		if _, ok := u.issue(""); !ok {
			t.Fatal("来源未知时不应按来源限额拒绝")
		}
	}
	if len(u.perSrc) != 0 {
		t.Fatalf("来源未知的挑战不应占用来源配额: %v", u.perSrc)
	}
	// 过期的挑战不能使用，并在之后被清理
	var e challengeStore
	c, _ := e.issue("203.0.113.1")
	e.pending[c] = pendingChallenge{exp: time.Now().Add(-time.Second), src: "203.0.113.1"}
	if e.consume(c) {
		t.Fatal("过期的挑战不应通过")
	}
}

// 未登录的人反复请求登录挑战，不能让管理员的 Passkey 登录与 Passkey 注册不可用。
func TestPasskeyBeginFloodDoesNotLockOutAdmin(t *testing.T) {
	a, srv, admin := loginApp(t)
	h := a.Handler()
	host := strings.TrimPrefix(srv.URL, "http://")
	rpID := webauthn.RPID(host)
	key := webauthntest.New()
	_, body := doJSON(t, admin, "POST", srv.URL+"/api/account/passkeys/begin", map[string]string{})
	var opts struct{ Challenge string }
	_ = json.Unmarshal(body, &opts)
	reg := map[string]any{"name": "k", "password": "test-password", "challenge": opts.Challenge, "credential": key.Register(rpID, opts.Challenge, srv.URL)}
	if code, body := doJSON(t, admin, "POST", srv.URL+"/api/account/passkeys", reg); code != 200 {
		t.Fatalf("注册失败 %d %s", code, body)
	}

	begin := func(remote string) int {
		req := httptest.NewRequest("POST", "/api/login/passkey/begin", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	// 一个来源连续请求 300 次：超过限额后只有它自己被拒绝
	rejected := 0
	for i := 0; i < 300; i++ {
		if begin("203.0.113.50:4000") == http.StatusTooManyRequests {
			rejected++
		}
	}
	if rejected != 300-passkeyMaxPerSource {
		t.Fatalf("同一来源应只有 %d 次成功，实际被拒绝 %d 次", passkeyMaxPerSource, rejected)
	}
	// 再有 300 个不同来源各请求一次
	for i := 0; i < 300; i++ {
		if code := begin("198.51.100." + strconv.Itoa(i%250+1) + ":" + strconv.Itoa(5000+i)); code != 200 && code != http.StatusTooManyRequests {
			t.Fatalf("意外的状态码 %d", code)
		}
	}
	// 管理员从自己的地址照常用 Passkey 登录
	code, body := doJSON(t, http.DefaultClient, "POST", srv.URL+"/api/login/passkey/begin", map[string]string{})
	if code != 200 {
		t.Fatalf("管理员应仍能开始 Passkey 登录: %d %s", code, body)
	}
	var lo struct{ Challenge string }
	_ = json.Unmarshal(body, &lo)
	finish := map[string]any{"challenge": lo.Challenge, "credential": key.Assert(rpID, lo.Challenge, srv.URL)}
	if code, body := doJSON(t, http.DefaultClient, "POST", srv.URL+"/api/login/passkey/finish", finish); code != 200 {
		t.Fatalf("管理员 Passkey 登录应成功: %d %s", code, body)
	}
	// 已登录的管理员注册新的 Passkey 不受登录挑战数量影响
	if code, body := doJSON(t, admin, "POST", srv.URL+"/api/account/passkeys/begin", map[string]string{}); code != 200 {
		t.Fatalf("管理员应仍能开始注册 Passkey: %d %s", code, body)
	}
}

// 经统一网关登录时按飞牛用户限流：一个飞牛用户输错密码不会把其他人挡在外面，轮换 X-Real-IP 也绕不过锁定；
// 登录失败事件不带可以伪造的地址。
func TestGatewayLoginLimitPerFnOSUser(t *testing.T) {
	a, h := newOriginApp(t, t.TempDir(), true)
	var mu sync.Mutex
	var failed []events.Event
	cancel := events.Subscribe(func(e events.Event) {
		if e.Type == events.AuthFailed {
			mu.Lock()
			failed = append(failed, e)
			mu.Unlock()
		}
	})
	defer cancel()
	login := func(fnUser, realIP, password string) int {
		req := httptest.NewRequest("POST", "http://nas.example.com/app/gately/api/login", strings.NewReader(`{"username":"admin","password":"`+password+`"}`))
		req.Host = "nas.example.com"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", gwLegit)
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		if fnUser != "" {
			req.Header.Set("X-Trim-Username", fnUser)
		}
		if realIP != "" {
			req.Header.Set("X-Real-IP", realIP)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	maxFail := a.auth.Snapshot().MaxFail
	// 飞牛用户 mallory 不断输错密码，每次换一个 X-Real-IP
	for i := 0; i < maxFail; i++ {
		if code := login("mallory", "203.0.113."+strconv.Itoa(i+1), "wrong"); code != http.StatusUnauthorized {
			t.Fatalf("第 %d 次错误密码应返回 401，实际 %d", i+1, code)
		}
	}
	for i := 0; i < 5; i++ {
		if code := login("mallory", "198.51.100."+strconv.Itoa(i+1), "wrong"); code != http.StatusTooManyRequests {
			t.Fatalf("达到上限后换 X-Real-IP 不应绕过锁定，实际 %d", code)
		}
	}
	if code := login("mallory", "", "test-password"); code != http.StatusTooManyRequests {
		t.Fatalf("被锁定的飞牛用户即使密码正确也应被拒绝，实际 %d", code)
	}
	// 其他飞牛用户（管理员本人）不受影响，浏览器直连网关时没有客户端地址
	if code := login("boss", "", "test-password"); code != 200 {
		t.Fatalf("其他飞牛用户应能正常登录，实际 %d", code)
	}
	// 事件总线异步投递，稍等
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(failed)
		mu.Unlock()
		if n >= maxFail {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(failed) < maxFail {
		t.Fatalf("应收到登录失败事件，实际 %d 个", len(failed))
	}
	for _, e := range failed {
		if ip, ok := e.Fields["ip"]; ok {
			t.Fatalf("经网关的登录失败事件不应带地址（可伪造，会被入侵防护用来封禁）: %q", ip)
		}
	}
}

// 直接访问时登录失败事件照常带上客户端地址，入侵防护的“Gately 管理后台”规则据此计数。
func TestDirectLoginFailureEventKeepsIP(t *testing.T) {
	a, _, _ := loginApp(t)
	h := a.Handler()
	got := make(chan events.Event, 4)
	cancel := events.Subscribe(func(e events.Event) {
		if e.Type == events.AuthFailed {
			select {
			case got <- e:
			default:
			}
		}
	})
	defer cancel()
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"username":"admin","password":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.77:5555"
	h.ServeHTTP(httptest.NewRecorder(), req)
	select {
	case e := <-got:
		if e.Fields["ip"] != "203.0.113.77" {
			t.Fatalf("直接访问的登录失败事件应带客户端地址: %v", e.Fields)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("没有收到登录失败事件")
	}
}
