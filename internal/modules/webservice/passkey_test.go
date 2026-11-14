package webservice

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/webauthn"
	"github.com/wangwanjie/gately/internal/webauthn/webauthntest"
)

// pkEnv 是 Passkey 测试环境：站点 a.test、b.test（均使用共享用户）与站点 a.example.test、b.example.test，
// 共享用户保存在内存中，模拟模块的 saveSharedPasskeys（原子校验与写入）。
type pkEnv struct {
	t     *testing.T
	addr  string
	a     *authenticator
	mu    sync.Mutex
	users []AuthUser
	saves int
	sso   string
}

func newPKEnv(t *testing.T, users []AuthUser, sso string) *pkEnv {
	t.Helper()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("backend")) }))
	t.Cleanup(backend.Close)
	mk := func(key string, domain string) Site {
		return Site{Enable: true, Key: key, Domains: []string{domain}, Targets: []string{backend.URL}, WebAuth: true, SharedUsers: true}
	}
	l := Listener{Sites: []Site{mk("a", "a.test"), mk("b", "b.test"), mk("ax", "a.example.test"), mk("bx", "b.example.test")}}
	addr, r := startRunner(t, l, nil, nil)
	e := &pkEnv{t: t, addr: addr, a: r.auth, sso: sso}
	norm, err := normalizeAuthUsers(users, nil)
	if err != nil {
		t.Fatal(err)
	}
	e.users = norm
	e.a.shared.set(e.users, sso)
	e.a.passkeyOn.Store(true)
	e.a.savePasskeys = func(id string, mutate func(all []AuthUser, i int) ([]webauthn.Credential, error)) error {
		e.mu.Lock()
		defer e.mu.Unlock()
		for i := range e.users {
			if e.users[i].ID != id {
				continue
			}
			list, err := mutate(e.users, i)
			if err == errNoPasskeyChange {
				return nil
			}
			if err != nil {
				return err
			}
			next := append([]AuthUser{}, e.users...)
			next[i].Passkeys = list
			e.users = next
			e.saves++
			e.a.shared.set(e.users, e.sso)
			return nil
		}
		return errPasswordChanged
	}
	return e
}

// post 向 host 的 Passkey 接口发送 JSON；origin 为空时不带 Origin 头。
func (e *pkEnv) post(host, sub, origin string, body any) (*http.Response, map[string]any) {
	e.t.Helper()
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "http://"+e.addr+authPath+passkeyPath+sub, bytes.NewReader(raw))
	req.Host = host
	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := noRedirect().Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return resp, out
}

// token 在 host 上验证身份并返回管理令牌。
func (e *pkEnv) token(host, user, pass, code string) string {
	e.t.Helper()
	resp, out := e.post(host, "/manage/auth", "http://"+host, map[string]string{"username": user, "password": pass, "code": code})
	if resp.StatusCode != 200 {
		e.t.Fatalf("验证身份失败: %d %v", resp.StatusCode, out)
	}
	return out["token"].(string)
}

// register 用 key 在 host 上注册 Passkey，返回 HTTP 响应与内容。
func (e *pkEnv) register(host, token, name string, key *webauthntest.SoftKey) (*http.Response, map[string]any) {
	e.t.Helper()
	origin := "http://" + host
	resp, opts := e.post(host, "/manage/begin", origin, map[string]string{"token": token})
	if resp.StatusCode != 200 {
		return resp, opts
	}
	rp := opts["rp"].(map[string]any)["id"].(string)
	ch := opts["challenge"].(string)
	return e.post(host, "/manage/finish", origin, map[string]any{"token": token, "name": name, "challenge": ch, "credential": key.Register(rp, ch, origin)})
}

// login 用 key 在 host 上登录，返回完成接口的响应与会话 Cookie。
func (e *pkEnv) login(host string, key *webauthntest.SoftKey) (*http.Response, map[string]any) {
	e.t.Helper()
	resp, opts := e.post(host, "/login/begin", "http://"+host, map[string]string{})
	if resp.StatusCode != 200 {
		e.t.Fatalf("login/begin 失败: %d %v", resp.StatusCode, opts)
	}
	ch := opts["challenge"].(string)
	return e.post(host, "/login/finish", "http://"+host, map[string]any{"challenge": ch, "next": "/dash", "credential": key.Assert(opts["rpId"].(string), ch, "http://"+host)})
}

func (e *pkEnv) cookieWorks(host string, resp *http.Response) bool {
	e.t.Helper()
	req, _ := http.NewRequest("GET", "http://"+e.addr+"/", nil)
	req.Host = host
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "text/html")
	for _, c := range resp.Cookies() {
		req.AddCookie(c)
	}
	r, err := noRedirect().Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	b, _ := io.ReadAll(r.Body)
	r.Body.Close()
	return r.StatusCode == 200 && string(b) == "backend"
}

func (e *pkEnv) stored(name string) []webauthn.Credential {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, u := range e.users {
		if u.Name == name {
			return u.Passkeys
		}
	}
	return nil
}

var pkAlice = AuthUser{Name: "alice", Password: "alice-password"}

// 功能关闭时页面、接口与登录页入口都不存在。
func TestPasskeyDisabled(t *testing.T) {
	e := newPKEnv(t, []AuthUser{pkAlice}, "")
	e.a.passkeyOn.Store(false)
	for _, sub := range []string{"/login/begin", "/manage/auth", "/manage/begin"} {
		if resp, _ := e.post("a.test", sub, "http://a.test", map[string]string{}); resp.StatusCode != 404 {
			t.Fatalf("关闭时 %s 应返回 404: %d", sub, resp.StatusCode)
		}
	}
	browser := map[string]string{"User-Agent": "Mozilla/5.0", "Accept": "text/html"}
	if resp, _ := get(t, noRedirect(), e.addr, "a.test", authPath+passkeyPath, browser); resp.StatusCode != 404 {
		t.Fatalf("关闭时页面应返回 404: %d", resp.StatusCode)
	}
	if _, page := get(t, noRedirect(), e.addr, "a.test", authPath+"/login", browser); strings.Contains(page, `id="pk-login"`) {
		t.Fatal("关闭时登录页不应显示 Passkey 入口")
	}
	// 开启后登录页显示入口与管理链接，管理页可访问
	e.a.passkeyOn.Store(true)
	if _, page := get(t, noRedirect(), e.addr, "a.test", authPath+"/login", browser); !strings.Contains(page, `id="pk-login"`) || !strings.Contains(page, `href="`+authPath+passkeyPath+`"`) {
		t.Fatal("开启后登录页应显示 Passkey 入口")
	}
	if resp, page := get(t, noRedirect(), e.addr, "a.test", authPath+passkeyPath, browser); resp.StatusCode != 200 || !strings.Contains(page, `id="pk-auth"`) {
		t.Fatalf("管理页内容错误: %d", resp.StatusCode)
	}
}

// 站点没有开启共享用户时 Passkey 不可用。
func TestPasskeyNeedsSharedUsers(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer backend.Close()
	addr, r := startRunner(t, Listener{Sites: []Site{{Enable: true, Key: "l", Domains: []string{"l.test"}, Targets: []string{backend.URL}, WebAuth: true, Users: []string{"u:p"}}}}, nil, nil)
	r.auth.passkeyOn.Store(true)
	req, _ := http.NewRequest("POST", "http://"+addr+authPath+passkeyPath+"/login/begin", strings.NewReader("{}"))
	req.Host = "l.test"
	req.Header.Set("Origin", "http://l.test")
	req.Header.Set("Content-Type", "application/json")
	resp, err := noRedirect().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("没有共享用户的站点应返回 404: %d", resp.StatusCode)
	}
}

// 注册、登录、签名计数落盘：启用 TOTP 的用户用 Passkey 登录不再要求动态验证码。
func TestPasskeyRegisterAndLogin(t *testing.T) {
	secret := auth.NewTOTPSecret()
	e := newPKEnv(t, []AuthUser{{Name: "alice", Password: "alice-password", TOTPSecret: secret}}, "")
	key := webauthntest.New()

	// 密码或验证码错误不能换得令牌
	if resp, _ := e.post("a.test", "/manage/auth", "http://a.test", map[string]string{"username": "alice", "password": "bad"}); resp.StatusCode != 401 {
		t.Fatalf("密码错误应返回 401: %d", resp.StatusCode)
	}
	if resp, _ := e.post("a.test", "/manage/auth", "http://a.test", map[string]string{"username": "alice", "password": "alice-password"}); resp.StatusCode != 401 {
		t.Fatalf("缺少验证码应返回 401: %d", resp.StatusCode)
	}
	tok := e.token("a.test", "alice", "alice-password", totpNow(secret))

	resp, out := e.register("a.test", tok, "我的手机", key)
	if resp.StatusCode != 200 {
		t.Fatalf("注册失败: %d %v", resp.StatusCode, out)
	}
	list := out["passkeys"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["name"] != "我的手机" || list[0].(map[string]any)["x"] != nil {
		t.Fatalf("注册结果不应含公钥坐标: %v", list)
	}
	if got := e.stored("alice"); len(got) != 1 || got[0].RPID != "a.test" {
		t.Fatalf("凭据应记录 RP ID: %+v", got)
	}
	// 同一验证器重复注册被拒绝（排除列表由浏览器处理，服务端按凭据 ID 唯一兜底）
	if resp, _ := e.register("a.test", tok, "again", key); resp.StatusCode != 400 {
		t.Fatalf("重复注册应返回 400: %d", resp.StatusCode)
	}

	// 登录：无需密码与验证码，会话可访问站点，next 原样返回站内地址
	resp, out = e.login("a.test", key)
	if resp.StatusCode != 200 || out["next"] != "/dash" {
		t.Fatalf("Passkey 登录失败: %d %v", resp.StatusCode, out)
	}
	if !e.cookieWorks("a.test", resp) {
		t.Fatal("登录后的会话应能访问站点")
	}
	// 其他站点没有这个会话（未开启单点登录）
	if e.cookieWorks("b.test", resp) {
		t.Fatal("未开启单点登录时会话不应在其他站点有效")
	}
	// 签名计数已落盘
	if got := e.stored("alice")[0]; got.SignCount != 1 || got.LastUsed.IsZero() {
		t.Fatalf("签名计数与最近使用时间应落盘: %+v", got)
	}
}

// 挑战只能使用一次；签名计数回退（克隆的凭据）被拒绝；失败不签发会话。
func TestPasskeyReplayAndClone(t *testing.T) {
	e := newPKEnv(t, []AuthUser{pkAlice}, "")
	key := webauthntest.New()
	tok := e.token("a.test", "alice", "alice-password", "")
	if resp, out := e.register("a.test", tok, "k", key); resp.StatusCode != 200 {
		t.Fatalf("注册失败: %v", out)
	}
	_, opts := e.post("a.test", "/login/begin", "http://a.test", map[string]string{})
	ch := opts["challenge"].(string)
	body := map[string]any{"challenge": ch, "credential": key.Assert("a.test", ch, "http://a.test")}
	if resp, out := e.post("a.test", "/login/finish", "http://a.test", body); resp.StatusCode != 200 {
		t.Fatalf("首次应成功: %v", out)
	}
	resp, _ := e.post("a.test", "/login/finish", "http://a.test", body)
	if resp.StatusCode != 401 || len(resp.Cookies()) != 0 {
		t.Fatalf("重放同一断言应失败且不签发会话: %d", resp.StatusCode)
	}
	// 克隆的验证器计数落后：服务端已记录 1，新断言计数 1 不再递增
	key.SetCount(0)
	if resp, _ := e.login("a.test", key); resp.StatusCode != 401 {
		t.Fatalf("计数未递增应被拒绝: %d", resp.StatusCode)
	}
	// 计数继续递增则恢复正常
	key.SetCount(5)
	if resp, _ := e.login("a.test", key); resp.StatusCode != 200 {
		t.Fatalf("计数递增应成功: %d", resp.StatusCode)
	}
}

// 没有签名计数的同步型 Passkey（计数恒为 0）可以反复登录，且一天内只写一次配置（刷新最近使用时间）。
func TestPasskeyZeroCounterNoRewrite(t *testing.T) {
	e := newPKEnv(t, []AuthUser{pkAlice}, "")
	key := webauthntest.New()
	key.SetZeroCounter(true)
	tok := e.token("a.test", "alice", "alice-password", "")
	if resp, out := e.register("a.test", tok, "sync", key); resp.StatusCode != 200 {
		t.Fatalf("注册失败: %v", out)
	}
	saves := e.saves
	for i := 0; i < 3; i++ {
		if resp, out := e.login("a.test", key); resp.StatusCode != 200 {
			t.Fatalf("第 %d 次登录失败: %d %v", i+1, resp.StatusCode, out)
		}
	}
	if e.saves != saves+1 {
		t.Fatalf("三次登录应只写一次配置: %d -> %d", saves, e.saves)
	}
	if e.stored("alice")[0].LastUsed.IsZero() {
		t.Fatal("应记录最近使用时间")
	}
}

// 来源与域名的绑定：缺少 Origin、跨站 Origin 被拒绝；挑战不能跨域名使用；凭据不能在其他域名使用。
func TestPasskeyOriginAndBinding(t *testing.T) {
	e := newPKEnv(t, []AuthUser{pkAlice}, "")
	key := webauthntest.New()
	tok := e.token("a.test", "alice", "alice-password", "")
	if resp, _ := e.register("a.test", tok, "k", key); resp.StatusCode != 200 {
		t.Fatal("注册失败")
	}
	for _, origin := range []string{"", "null", "http://evil.test", "https://a.test.evil.test"} {
		if resp, _ := e.post("a.test", "/login/begin", origin, map[string]string{}); resp.StatusCode != 403 {
			t.Fatalf("Origin %q 应被拒绝: %d", origin, resp.StatusCode)
		}
		if resp, _ := e.post("a.test", "/manage/auth", origin, map[string]string{"username": "alice", "password": "alice-password"}); resp.StatusCode != 403 {
			t.Fatalf("Origin %q 的验证身份应被拒绝: %d", origin, resp.StatusCode)
		}
	}
	// 在 a.test 取得的挑战拿到 b.test 使用
	_, opts := e.post("a.test", "/login/begin", "http://a.test", map[string]string{})
	ch := opts["challenge"].(string)
	resp, _ := e.post("b.test", "/login/finish", "http://b.test", map[string]any{"challenge": ch, "credential": key.Assert("b.test", ch, "http://b.test")})
	if resp.StatusCode != 401 {
		t.Fatalf("挑战不能跨域名使用: %d", resp.StatusCode)
	}
	// 同一凭据在 b.test 上用 b.test 的挑战也不行（RP ID 不同）
	if resp, _ := e.login("b.test", key); resp.StatusCode != 401 {
		t.Fatalf("凭据不能在其他域名使用: %d", resp.StatusCode)
	}
	// 页面、断言中的 RP ID 与访问域名不符
	_, opts = e.post("a.test", "/login/begin", "http://a.test", map[string]string{})
	ch = opts["challenge"].(string)
	resp, _ = e.post("a.test", "/login/finish", "http://a.test", map[string]any{"challenge": ch, "credential": key.Assert("evil.test", ch, "http://a.test")})
	if resp.StatusCode != 401 {
		t.Fatalf("RP ID 不符应被拒绝: %d", resp.StatusCode)
	}
	// 断言里的 origin 与访问域名不符
	_, opts = e.post("a.test", "/login/begin", "http://a.test", map[string]string{})
	ch = opts["challenge"].(string)
	resp, _ = e.post("a.test", "/login/finish", "http://a.test", map[string]any{"challenge": ch, "credential": key.Assert("a.test", ch, "http://evil.test")})
	if resp.StatusCode != 401 {
		t.Fatalf("clientData 来源不符应被拒绝: %d", resp.StatusCode)
	}
	// 未知凭据
	if resp, _ := e.login("a.test", webauthntest.New()); resp.StatusCode != 401 {
		t.Fatalf("未注册的凭据应被拒绝: %d", resp.StatusCode)
	}
}

// 配置单点登录域后，凭据的 RP ID 为该域：在一个站点注册，各站点通用，会话也跨站有效。
func TestPasskeySSODomain(t *testing.T) {
	e := newPKEnv(t, []AuthUser{pkAlice}, "example.test")
	key := webauthntest.New()
	tok := e.token("a.example.test", "alice", "alice-password", "")
	if resp, out := e.register("a.example.test", tok, "k", key); resp.StatusCode != 200 {
		t.Fatalf("注册失败: %v", out)
	}
	if got := e.stored("alice")[0].RPID; got != "example.test" {
		t.Fatalf("RP ID 应为单点登录域: %s", got)
	}
	resp, out := e.login("b.example.test", key)
	if resp.StatusCode != 200 {
		t.Fatalf("单点登录域下其他站点也应可登录: %d %v", resp.StatusCode, out)
	}
	if !e.cookieWorks("a.example.test", resp) || !e.cookieWorks("b.example.test", resp) {
		t.Fatal("单点登录会话应在域内各站点有效")
	}
	// 单点登录域调整后，旧凭据 RP ID 不再匹配
	e.sso = "other.test"
	e.a.shared.set(e.users, e.sso)
	if resp, _ := e.login("a.example.test", key); resp.StatusCode != 401 {
		t.Fatalf("单点登录域变化后旧凭据应失效: %d", resp.StatusCode)
	}
}

// 管理令牌：改密码后失效、跨域名失效；数量上限；删除后不能再登录；删除自己以外的凭据返回 404。
func TestPasskeyManageLifecycle(t *testing.T) {
	e := newPKEnv(t, []AuthUser{pkAlice, {Name: "bob", Password: "bob-password"}}, "")
	tok := e.token("a.test", "alice", "alice-password", "")
	// 令牌只在签发的域名有效
	if resp, _ := e.post("b.test", "/manage/begin", "http://b.test", map[string]string{"token": tok}); resp.StatusCode != 401 {
		t.Fatalf("令牌不能跨域名使用: %d", resp.StatusCode)
	}
	if resp, _ := e.post("a.test", "/manage/begin", "http://a.test", map[string]string{"token": tok + "x"}); resp.StatusCode != 401 {
		t.Fatalf("伪造令牌应被拒绝: %d", resp.StatusCode)
	}
	keys := make([]*webauthntest.SoftKey, maxUserPasskeys)
	for i := range keys {
		keys[i] = webauthntest.New()
		if resp, out := e.register("a.test", tok, "", keys[i]); resp.StatusCode != 200 {
			t.Fatalf("第 %d 个注册失败: %v", i+1, out)
		}
	}
	if resp, out := e.register("a.test", tok, "extra", webauthntest.New()); resp.StatusCode != 400 {
		t.Fatalf("超过上限应被拒绝: %d %v", resp.StatusCode, out)
	}
	if got := e.stored("alice"); got[0].Name != "Passkey" {
		t.Fatalf("名称留空应为 Passkey: %q", got[0].Name)
	}
	// bob 删除不了 alice 的凭据
	bobTok := e.token("a.test", "bob", "bob-password", "")
	if resp, _ := e.post("a.test", "/manage/delete", "http://a.test", map[string]string{"token": bobTok, "id": keys[0].ID()}); resp.StatusCode != 404 {
		t.Fatalf("不能删除他人的 Passkey: %d", resp.StatusCode)
	}
	if len(e.stored("alice")) != maxUserPasskeys {
		t.Fatal("他人的删除请求不应生效")
	}
	// 凭据 ID 全局唯一：bob 不能注册 alice 已有的验证器
	if resp, _ := e.register("a.test", bobTok, "dup", keys[1]); resp.StatusCode != 400 {
		t.Fatalf("他人已注册的验证器应被拒绝: %d", resp.StatusCode)
	}
	// 删除后不能再登录
	resp, out := e.post("a.test", "/manage/delete", "http://a.test", map[string]string{"token": tok, "id": keys[0].ID()})
	if resp.StatusCode != 200 || len(out["passkeys"].([]any)) != maxUserPasskeys-1 {
		t.Fatalf("删除失败: %d %v", resp.StatusCode, out)
	}
	if resp, _ := e.login("a.test", keys[0]); resp.StatusCode != 401 {
		t.Fatalf("已删除的 Passkey 不能登录: %d", resp.StatusCode)
	}
	if resp, _ := e.login("a.test", keys[1]); resp.StatusCode != 200 {
		t.Fatalf("其余 Passkey 仍可登录: %d", resp.StatusCode)
	}
	// 修改密码后令牌失效（凭据指纹变化）
	e.mu.Lock()
	e.users[0].Password = "$2a$04$" + strings.Repeat("a", 53)
	e.a.shared.set(e.users, "")
	e.mu.Unlock()
	if resp, _ := e.post("a.test", "/manage/begin", "http://a.test", map[string]string{"token": tok}); resp.StatusCode != 401 {
		t.Fatalf("改密后令牌应失效: %d", resp.StatusCode)
	}
}

// 管理令牌的名称限制：过长与含控制字符被拒绝。
func TestPasskeyNameLimits(t *testing.T) {
	for in, ok := range map[string]bool{"": true, "  手机 ": true, strings.Repeat("a", 40): true, strings.Repeat("a", 41): false, "a\nb": false, "a\x00": false} {
		if _, got := cleanPasskeyName(in); got != ok {
			t.Fatalf("名称 %q: 期望 %v", in, ok)
		}
	}
	if n, _ := cleanPasskeyName("  x "); n != "x" {
		t.Fatalf("应去掉首尾空白: %q", n)
	}
}

// 失败登录与密码登录共用限流：连续失败达到上限后返回 429，之后即使凭据正确也被拒绝。
func TestPasskeyFailuresRateLimited(t *testing.T) {
	e := newPKEnv(t, []AuthUser{pkAlice}, "")
	key := webauthntest.New()
	tok := e.token("a.test", "alice", "alice-password", "")
	e.register("a.test", tok, "k", key)
	unknown := webauthntest.New()
	var last int
	for i := 0; i < maxFailures+2; i++ {
		_, opts := e.post("a.test", "/login/begin", "http://a.test", map[string]string{})
		ch := opts["challenge"].(string)
		resp, _ := e.post("a.test", "/login/finish", "http://a.test", map[string]any{"challenge": ch, "credential": unknown.Assert("a.test", ch, "http://a.test")})
		last = resp.StatusCode
	}
	if last != 429 {
		t.Fatalf("连续失败后应限流: %d", last)
	}
}

// 挑战数量受总量与按来源的上限保护，用完后返回 429；过期与已使用的挑战被回收。
func TestPasskeyChallengeCaps(t *testing.T) {
	var c passkeyChallenges
	for i := 0; i < passkeyMaxPerIP; i++ {
		if _, ok := c.issue(pkLogin, "", "a.test", "1.1.1.1"); !ok {
			t.Fatalf("第 %d 个挑战应成功", i+1)
		}
	}
	if _, ok := c.issue(pkLogin, "", "a.test", "1.1.1.1"); ok {
		t.Fatal("同一来源超过上限应被拒绝")
	}
	if _, ok := c.issue(pkLogin, "", "a.test", "2.2.2.2"); !ok {
		t.Fatal("其他来源不受影响")
	}
	// 使用后归还配额
	ch, _ := c.issue(pkLogin, "", "a.test", "3.3.3.3")
	if _, ok := c.consume(ch, pkRegister, "a.test"); ok {
		t.Fatal("用途不符应失败")
	}
	if _, ok := c.consume(ch, pkLogin, "a.test"); ok {
		t.Fatal("挑战在一次（即使失败的）消费后即作废")
	}
	// 总量上限
	var d passkeyChallenges
	for i := 0; i < passkeyMaxPending; i++ {
		d.issue(pkLogin, "", "h", "ip"+string(rune('a'+i%26))+strings.Repeat("x", i/26))
	}
	if _, ok := d.issue(pkLogin, "", "h", "fresh"); ok {
		t.Fatal("总量用尽后应拒绝")
	}
	if d.pending() > passkeyMaxPending {
		t.Fatalf("挂起数量超过上限: %d", d.pending())
	}
}

// IP 地址不能作为 RP ID：登录页不显示入口，接口返回错误。
func TestPasskeyRejectsIPHost(t *testing.T) {
	a := newAuthenticator("s")
	a.passkeyOn.Store(true)
	for _, host := range []string{"127.0.0.1:8080", "[::1]:8080", "192.168.1.2"} {
		r := httptest.NewRequest("GET", "/", nil)
		r.Host = host
		if _, err := a.passkeyRP(r); err == nil {
			t.Fatalf("%s 不应可作为 RP ID", host)
		}
		if a.passkeyOffered(r, Site{SharedUsers: true}) {
			t.Fatalf("%s 的登录页不应显示 Passkey 入口", host)
		}
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.Host = "Example.COM:8443"
	if rp, err := a.passkeyRP(r); err != nil || rp != "example.com" {
		t.Fatalf("域名应规范化: %q %v", rp, err)
	}
}

// 保存共享用户列表时 Passkey 只沿用已保存的值，提交的内容被忽略；改名后保留；读取时脱敏。
func TestPasskeyKeptByNormalize(t *testing.T) {
	cred := webauthn.Credential{ID: "cid", Name: "k", RPID: "a.test", X: "xx", Y: "yy", SignCount: 7}
	old, err := normalizeAuthUsers([]AuthUser{{Name: "alice", Password: "alice-password"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	old[0].Passkeys = []webauthn.Credential{cred}
	// 提交者夹带伪造的凭据，同时改名
	forged := webauthn.Credential{ID: "forged", Name: "evil"}
	in := []AuthUser{{ID: old[0].ID, Name: "alice2", Password: "******", Passkeys: []webauthn.Credential{forged}}}
	in[0].Password = ""
	out, err := normalizeAuthUsers(in, old)
	if err != nil {
		t.Fatal(err)
	}
	if len(out[0].Passkeys) != 1 || out[0].Passkeys[0].ID != "cid" {
		t.Fatalf("应沿用已保存的 Passkey，忽略提交的内容: %+v", out[0].Passkeys)
	}
	// 新用户（无 ID）不能带入 Passkey
	out, err = normalizeAuthUsers([]AuthUser{{Name: "mallory", Password: "mallory-password", Passkeys: []webauthn.Credential{forged}}}, old)
	if err != nil || len(out[0].Passkeys) != 0 {
		t.Fatalf("新用户不应带入 Passkey: %+v %v", out[0].Passkeys, err)
	}
	m := maskAuthUsers(old)
	if g := m[0].Passkeys[0]; g.X != "" || g.Y != "" || g.SignCount != 0 || g.ID != "cid" || g.Name != "k" {
		t.Fatalf("脱敏应去掉公钥与计数: %+v", g)
	}
	if old[0].Passkeys[0].X != "xx" {
		t.Fatal("脱敏不应修改原数据")
	}
}

// pkModule 创建带一个共享用户与两个站点的真实模块，用于验证持久化与管理接口。
func pkModule(t *testing.T) *Module {
	t.Helper()
	st, _ := config.Open(t.TempDir())
	m := New(nil, nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	users, err := normalizeAuthUsers([]AuthUser{pkAlice, {Name: "bob", Password: "bob-password"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.cfg.Settings.AuthUsers = users
	m.cfg.Settings.PasskeyLogin = true
	m.auth.shared.set(users, "")
	m.auth.passkeyOn.Store(true)
	return m
}

// 模块保存 Passkey：写入配置文件、刷新运行时索引；保存用户列表（PUT /users）时不丢失也不被覆盖；管理员可删除；用户被删后拒绝。
func TestPasskeyModulePersistence(t *testing.T) {
	m := pkModule(t)
	alice := m.cfg.Settings.AuthUsers[0]
	cred := webauthn.Credential{ID: "c1", Name: "手机", RPID: "a.test", X: "x", Y: "y", CreatedAt: time.Now()}
	add := func(c webauthn.Credential) error {
		return m.saveSharedPasskeys(alice.ID, func(all []AuthUser, i int) ([]webauthn.Credential, error) {
			return append(append([]webauthn.Credential{}, all[i].Passkeys...), c), nil
		})
	}
	if err := add(cred); err != nil {
		t.Fatal(err)
	}
	if name, ok := m.auth.shared.get().byCred["c1"]; !ok || name != "alice" {
		t.Fatal("运行时索引应包含新凭据")
	}
	// 配置文件中保存了凭据（重新读取）
	raw, err := os.ReadFile(filepath.Join(m.store.Dir(), ID+".json"))
	if err != nil || !strings.Contains(string(raw), `"rpId": "a.test"`) {
		t.Fatalf("配置文件应包含 Passkey: %v", err)
	}
	// 没有改动不写盘；回调报错不改动配置
	if err := m.saveSharedPasskeys(alice.ID, func([]AuthUser, int) ([]webauthn.Credential, error) { return nil, errNoPasskeyChange }); err != nil {
		t.Fatal(err)
	}
	if err := m.saveSharedPasskeys(alice.ID, func([]AuthUser, int) ([]webauthn.Credential, error) { return nil, errPasskeyDup }); err != errPasskeyDup {
		t.Fatalf("应透传回调错误: %v", err)
	}
	if len(m.cfg.Settings.AuthUsers[0].Passkeys) != 1 {
		t.Fatal("失败的回调不应修改配置")
	}
	if err := m.saveSharedPasskeys("missing", func([]AuthUser, int) ([]webauthn.Credential, error) { return nil, nil }); err != errPasswordChanged {
		t.Fatalf("用户不存在应返回 errPasswordChanged: %v", err)
	}

	// 管理页保存用户列表：提交里没有、或伪造 Passkey 都不影响已保存的
	put := func(users []AuthUser) {
		raw, _ := json.Marshal(UsersRequest{UsersSettings: UsersSettings{AuthUsers: users, PasskeyLogin: true}})
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/users", bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		m.handlePutUsers(rec, r)
		if rec.Code != 200 {
			t.Fatalf("保存失败: %d %s", rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), `"x":`) {
			t.Fatal("接口响应不应包含公钥坐标")
		}
	}
	put([]AuthUser{{ID: alice.ID, Name: "alice", Password: crud.SecretMask, Passkeys: []webauthn.Credential{{ID: "forged"}}}, {ID: m.cfg.Settings.AuthUsers[1].ID, Name: "bob", Password: crud.SecretMask}})
	got := m.cfg.Settings.AuthUsers[0].Passkeys
	if len(got) != 1 || got[0].ID != "c1" || got[0].X != "x" {
		t.Fatalf("保存用户列表应保留原 Passkey 且忽略提交的内容: %+v", got)
	}
	// GET 脱敏
	rec := httptest.NewRecorder()
	m.mu.Lock()
	v := m.usersViewLocked()
	m.mu.Unlock()
	if p := v.AuthUsers[0].Passkeys; len(p) != 1 || p[0].X != "" || p[0].SignCount != 0 {
		t.Fatalf("读取应脱敏: %+v", p)
	}
	_ = rec

	// 管理员删除
	del := func(id, cid string) int {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/users/"+id+"/passkeys/"+cid, nil)
		r.SetPathValue("id", id)
		r.SetPathValue("cred", cid)
		m.handleDeleteUserPasskey(rec, r)
		return rec.Code
	}
	if c := del("nope", "c1"); c != 404 {
		t.Fatalf("用户不存在应返回 404: %d", c)
	}
	if c := del(alice.ID, "nope"); c != 404 {
		t.Fatalf("Passkey 不存在应返回 404: %d", c)
	}
	if c := del(alice.ID, "c1"); c != 200 {
		t.Fatalf("删除失败: %d", c)
	}
	if _, ok := m.auth.shared.get().byCred["c1"]; ok || len(m.cfg.Settings.AuthUsers[0].Passkeys) != 0 {
		t.Fatal("删除后索引与配置都应清除")
	}
}

// 全局设置接口省略 passkeyLogin 时保留原值，提供时更新；认证用户页保存时随请求更新。
func TestPasskeySettingKept(t *testing.T) {
	m := pkModule(t)
	m.cfg.Settings.PasskeyLogin = true
	body := func(extra string) string {
		return `{"trustedProxies":[],"oauthProviders":[]` + extra + `}`
	}
	put := func(b string) {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/settings", strings.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
		m.handlePutSettings(rec, r)
		if rec.Code != 200 {
			t.Fatalf("保存失败: %d %s", rec.Code, rec.Body.String())
		}
	}
	put(body(""))
	if !m.cfg.Settings.PasskeyLogin || !m.auth.passkeyOn.Load() {
		t.Fatal("省略时应保留 passkeyLogin")
	}
	put(body(`,"passkeyLogin":false`))
	if m.cfg.Settings.PasskeyLogin || m.auth.passkeyOn.Load() {
		t.Fatal("提供 false 时应关闭并立即生效")
	}
}

// OIDC 签发者域名的登录页同样提供 Passkey：在签发者域名上注册后，用 Passkey 登录得到的会话可以继续授权并拿到授权码。
func TestPasskeyOnOIDCIssuerLogin(t *testing.T) {
	f := newOIDCFixture(t, "idp.test", "")
	f.r.auth.passkeyOn.Store(true)
	var mu sync.Mutex
	f.r.auth.savePasskeys = func(id string, mutate func(all []AuthUser, i int) ([]webauthn.Credential, error)) error {
		mu.Lock()
		defer mu.Unlock()
		for i := range f.users {
			if f.users[i].ID == id {
				list, err := mutate(f.users, i)
				if err != nil {
					return err
				}
				next := append([]AuthUser{}, f.users...)
				next[i].Passkeys = list
				f.users = next
				f.r.auth.shared.set(next, "")
				return nil
			}
		}
		return errPasswordChanged
	}
	post := func(sub string, body any) (*http.Response, map[string]any) {
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", "http://"+f.addr+oidcPath+passkeyPath+sub, bytes.NewReader(raw))
		req.Host = f.host
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://"+f.host)
		resp, err := f.c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var out map[string]any
		_ = json.Unmarshal(b, &out)
		return resp, out
	}
	origin := "http://" + f.host
	// 登录页显示入口并指向 OIDC 路径，管理页可打开
	if _, page := f.do("GET", oidcLoginPath, nil, map[string]string{"Accept": "text/html"}); !strings.Contains(page, `id="pk-login"`) || !strings.Contains(page, `data-base="`+oidcPath+passkeyPath+`"`) {
		t.Fatal("OIDC 登录页应显示 Passkey 入口")
	}
	if resp, page := f.do("GET", oidcPath+passkeyPath, nil, map[string]string{"Accept": "text/html"}); resp.StatusCode != 200 || !strings.Contains(page, `id="pk-auth"`) {
		t.Fatalf("OIDC 签发者域名上的管理页应可访问: %d", resp.StatusCode)
	}
	// 注册（alice 未启用 TOTP）
	_, out := post("/manage/auth", map[string]string{"username": "alice", "password": "alice-pw"})
	token, _ := out["token"].(string)
	if token == "" {
		t.Fatalf("验证身份失败: %v", out)
	}
	key := webauthntest.New()
	_, opts := post("/manage/begin", map[string]string{"token": token})
	ch := opts["challenge"].(string)
	if resp, out := post("/manage/finish", map[string]any{"token": token, "name": "k", "challenge": ch, "credential": key.Register(f.host, ch, origin)}); resp.StatusCode != 200 {
		t.Fatalf("注册失败: %v", out)
	}
	// 登录：next 指向授权地址，登录后带会话授权得到授权码
	q := authorizeQuery("gitea", oidcTestRedirect)
	next := oidcAuthorizePath + "?" + q.Encode()
	_, opts = post("/login/begin", map[string]string{})
	ch = opts["challenge"].(string)
	resp, out := post("/login/finish", map[string]any{"challenge": ch, "next": next, "credential": key.Assert(f.host, ch, origin)})
	if resp.StatusCode != 200 || out["next"] != next {
		t.Fatalf("登录失败: %d %v", resp.StatusCode, out)
	}
	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Value != "" {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("登录后应有会话 Cookie")
	}
	if code := f.code(q, cookie); code == "" {
		t.Fatal("应拿到授权码")
	}
}

// 注册挑战绑定下发时的用户：用 bob 的挑战配 alice 的令牌注册会被拒绝。
func TestPasskeyRegisterChallengeBoundToUser(t *testing.T) {
	e := newPKEnv(t, []AuthUser{pkAlice, {Name: "bob", Password: "bob-password"}}, "")
	aliceTok := e.token("a.test", "alice", "alice-password", "")
	bobTok := e.token("a.test", "bob", "bob-password", "")
	_, opts := e.post("a.test", "/manage/begin", "http://a.test", map[string]string{"token": bobTok})
	ch := opts["challenge"].(string)
	key := webauthntest.New()
	resp, _ := e.post("a.test", "/manage/finish", "http://a.test", map[string]any{"token": aliceTok, "name": "x", "challenge": ch, "credential": key.Register("a.test", ch, "http://a.test")})
	if resp.StatusCode != 400 || len(e.stored("alice")) != 0 {
		t.Fatalf("他人的注册挑战应被拒绝: %d", resp.StatusCode)
	}
}

// 同一 RP ID 下挑战仍绑定下发时的域名：在 a 站点取得的挑战不能拿到 b 站点完成登录。
func TestPasskeyChallengeBoundToHostSameRP(t *testing.T) {
	e := newPKEnv(t, []AuthUser{pkAlice}, "example.test")
	key := webauthntest.New()
	tok := e.token("a.example.test", "alice", "alice-password", "")
	if resp, _ := e.register("a.example.test", tok, "k", key); resp.StatusCode != 200 {
		t.Fatal("注册失败")
	}
	_, opts := e.post("a.example.test", "/login/begin", "http://a.example.test", map[string]string{})
	ch := opts["challenge"].(string)
	resp, _ := e.post("b.example.test", "/login/finish", "http://b.example.test", map[string]any{"challenge": ch, "credential": key.Assert("example.test", ch, "http://b.example.test")})
	if resp.StatusCode != 401 {
		t.Fatalf("挑战不能跨站点使用: %d", resp.StatusCode)
	}
}

// 并发或重放时，落盘环节原子地拒绝不大于已记录值的签名计数。
func TestPasskeyPersistRejectsStaleCounter(t *testing.T) {
	e := newPKEnv(t, []AuthUser{pkAlice}, "")
	e.users[0].Passkeys = []webauthn.Credential{{ID: "c", RPID: "a.test", SignCount: 5}}
	e.a.shared.set(e.users, "")
	u := e.users[0]
	if err := e.a.persistPasskeyUse(u, webauthn.Credential{ID: "c", SignCount: 6}); err != nil {
		t.Fatal(err)
	}
	for _, n := range []uint32{6, 3} {
		if err := e.a.persistPasskeyUse(u, webauthn.Credential{ID: "c", SignCount: n}); err != errPasskeyReplay {
			t.Fatalf("计数 %d 应被拒绝: %v", n, err)
		}
	}
}
