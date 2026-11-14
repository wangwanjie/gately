package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/portforward"
	"github.com/wangwanjie/gately/internal/webauthn"
	"github.com/wangwanjie/gately/internal/webauthn/webauthntest"
)

// newAccountApp 以指定的账号状态启动测试实例（账号落盘到临时目录，初始化后返回）。
// 返回的客户端尚未登录。
func newAccountApp(t *testing.T, setup func(m *auth.Manager)) (*App, *httptest.Server, func() *http.Client) {
	t.Helper()
	a, err := New(Options{ConfigDir: t.TempDir(), Modules: []module.Module{portforward.New()}})
	if err != nil {
		t.Fatal(err)
	}
	a.logger = log.New(io.Discard, "", 0)
	a.auth = auth.NewManager(auth.Account{}, func(acc auth.Account) error { return a.store.Save(accountName, acc) })
	if setup != nil {
		setup(a.auth)
	}
	if _, err := a.auth.EnsureInitialized(); err != nil {
		t.Fatal(err)
	}
	a.applyRuntime()
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	newClient := func() *http.Client {
		jar, _ := cookiejar.New(nil)
		return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return a, srv, newClient
}

func mustLogin(t *testing.T, c *http.Client, base, user, pass string) {
	t.Helper()
	if code, body := doJSON(t, c, "POST", base+"/api/login", map[string]string{"username": user, "password": pass}); code != 200 {
		t.Fatalf("登录失败 %d %s", code, body)
	}
}

// doWith 发送带额外请求头的请求。
func doWith(t *testing.T, c *http.Client, method, url string, hdr map[string]string, body any) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, r)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

func sessionOf(t *testing.T, c *http.Client, base string) map[string]any {
	t.Helper()
	code, body := doJSON(t, c, "GET", base+"/api/session", nil)
	if code != 200 {
		t.Fatalf("读取会话失败 %d %s", code, body)
	}
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	return m
}

// ---- 默认密码 / 弱密码：强制修改 ----

// 新装实例使用默认密码：读取放行，写操作被拒绝；修改为强密码后恢复。
func TestDefaultPasswordForcesChange(t *testing.T) {
	_, srv, newClient := newAccountApp(t, nil)
	c := newClient()
	mustLogin(t, c, srv.URL, "admin", auth.DefaultPassword)
	s := sessionOf(t, c, srv.URL)
	if s["passwordIssue"] != "default" || s["defaultPassword"] != true {
		t.Fatalf("会话应标明默认密码: %v", s)
	}
	rule := map[string]any{"name": "web", "enable": false, "protocols": []string{"tcp4"}, "listenAddress": "127.0.0.1", "listenPorts": "39001", "targetAddresses": []string{"127.0.0.1"}, "targetPorts": "80"}
	if code, _ := doJSON(t, c, "GET", srv.URL+"/api/portforward/rules", nil); code != 200 {
		t.Fatalf("读取应放行，实际 %d", code)
	}
	code, body := doJSON(t, c, "POST", srv.URL+"/api/portforward/rules", rule)
	if code != http.StatusForbidden {
		t.Fatalf("默认密码下的普通写操作应被拒绝: %d %s", code, body)
	}
	// 允许的操作：预检密码强度、界面语言
	if code, _ := doJSON(t, c, "POST", srv.URL+"/api/account/password-check", map[string]string{"password": "x"}); code != 200 {
		t.Fatalf("密码预检应放行，实际 %d", code)
	}
	// 弱密码被拒绝，原密码仍有效
	for _, weak := range []string{"short", "password123", "gately.cn", "admin-Router-9x"} {
		if code, _ := doJSON(t, c, "PUT", srv.URL+"/api/account", map[string]string{"oldPassword": auth.DefaultPassword, "username": "admin", "newPassword": weak}); code != 400 {
			t.Fatalf("弱密码 %q 应返回 400，实际 %d", weak, code)
		}
	}
	if code, body := doJSON(t, c, "PUT", srv.URL+"/api/account", map[string]string{"oldPassword": auth.DefaultPassword, "username": "admin", "newPassword": "Zm3$kq9Lr!vT"}); code != 200 {
		t.Fatalf("改为强密码应成功: %d %s", code, body)
	}
	// 旧会话已失效，用新密码登录后写操作放行
	if code, _ := doJSON(t, c, "GET", srv.URL+"/api/session", nil); code != http.StatusUnauthorized {
		t.Fatalf("改密后旧会话应失效，实际 %d", code)
	}
	c2 := newClient()
	mustLogin(t, c2, srv.URL, "admin", "Zm3$kq9Lr!vT")
	if s := sessionOf(t, c2, srv.URL); s["passwordIssue"] != "" || s["defaultPassword"] != false {
		t.Fatalf("改密后不应再有密码问题: %v", s)
	}
	if code, body := doJSON(t, c2, "POST", srv.URL+"/api/portforward/rules", rule); code != 200 {
		t.Fatalf("改密后写操作应放行: %d %s", code, body)
	}
}

// 存量账号用弱密码登录：识别为 weak 并同样强制修改，改成强密码后恢复。
func TestWeakPasswordForcesChange(t *testing.T) {
	_, srv, newClient := newAccountApp(t, func(m *auth.Manager) { _ = m.SetBootstrapPassword("Password2024!") })
	c := newClient()
	mustLogin(t, c, srv.URL, "admin", "Password2024!")
	s := sessionOf(t, c, srv.URL)
	if s["passwordIssue"] != "weak" || s["defaultPassword"] != false {
		t.Fatalf("应识别为弱密码: %v", s)
	}
	if code, body := doJSON(t, c, "POST", srv.URL+"/api/tokens", map[string]any{"name": "ci", "scopes": []string{"*:write"}}); code != http.StatusForbidden {
		t.Fatalf("弱密码下创建令牌应被拒绝: %d %s", code, body)
	}
	if code, _ := doJSON(t, c, "PUT", srv.URL+"/api/account", map[string]string{"oldPassword": "Password2024!", "username": "admin", "newPassword": "Zm3$kq9Lr!vT"}); code != 200 {
		t.Fatal("改密失败")
	}
	c2 := newClient()
	mustLogin(t, c2, srv.URL, "admin", "Zm3$kq9Lr!vT")
	if s := sessionOf(t, c2, srv.URL); s["passwordIssue"] != "" {
		t.Fatalf("改密后不应再有问题: %v", s)
	}
	if code, body := doJSON(t, c2, "POST", srv.URL+"/api/tokens", map[string]any{"name": "ci", "scopes": []string{"portforward:write"}}); code != 200 {
		t.Fatalf("改密后创建令牌应成功: %d %s", code, body)
	}
}

// 守卫按调用方类型区分：登录会话拒绝一切写，令牌只拒绝安全配置写。
func TestGuardTokenScope(t *testing.T) {
	a, _, _ := newAccountApp(t, func(m *auth.Manager) { _ = m.SetBootstrapPassword("Password2024!") })
	if a.auth.PasswordIssue() != "weak" {
		t.Fatal("前置：账号应为弱密码状态")
	}
	var reached bool
	h := a.defaultPasswordGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	cases := []struct {
		name, user, method, path string
		want                     int // 0 表示放行
	}{
		{"会话 GET 放行", "admin", "GET", "/api/portforward/rules", 0},
		{"会话普通写拒绝", "admin", "POST", "/api/portforward/rules", 403},
		{"会话改密放行", "admin", "PUT", "/api/account", 0},
		{"会话预检放行", "admin", "POST", "/api/account/password-check", 0},
		{"会话语言放行", "admin", "PUT", "/api/account/ui-language", 0},
		{"会话安全配置拒绝", "admin", "PUT", "/api/system/settings", 403},
		{"会话删除拒绝", "admin", "DELETE", "/api/portforward/rules/x", 403},
		{"令牌普通写放行", "api-token:ci", "POST", "/api/portforward/rules", 0},
		{"令牌安全配置写拒绝", "api-token:ci", "PUT", "/api/system/settings", 403},
		{"令牌令牌管理写拒绝", "api-token:ci", "POST", "/api/tokens", 403},
	}
	for _, c := range cases {
		reached = false
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(c.method, c.path, nil)
		req = req.WithContext(auth.WithUser(req.Context(), c.user))
		h.ServeHTTP(rec, req)
		if c.want == 0 && !reached {
			t.Errorf("%s: 应放行，得到 %d", c.name, rec.Code)
		}
		if c.want != 0 && (reached || rec.Code != c.want) {
			t.Errorf("%s: 应返回 %d，得到 %d reached=%v", c.name, c.want, rec.Code, reached)
		}
	}
}

// ---- 安全评分 ----

func TestComputeSecurity(t *testing.T) {
	strong := auth.Snapshot{Username: "boss", MaxFail: 5, LockDuration: 15 * time.Minute, TOTP: true}
	ok := Settings{SafePath: "/x7k2p9", Listen: ":16680"}
	item := func(st securityStatus, id string) secItem {
		for _, it := range st.Items {
			if it.ID == id {
				return it
			}
		}
		t.Fatalf("缺少检查项 %s", id)
		return secItem{}
	}
	// 全部到位：满分
	if st := computeSecurity(strong, ok, true, true); st.Score != 100 || st.Level != "good" {
		t.Fatalf("应满分: %+v", st)
	}
	// 默认密码：高风险
	def := strong
	def.PasswordIssue = "default"
	if st := computeSecurity(def, ok, true, true); st.Level != "risk" || st.Score != 65 || item(st, "password").Status != secRisk {
		t.Fatalf("默认密码应高风险且扣 35: %+v", st)
	}
	// 公网暴露且没有任何加固：高风险
	bare := auth.Snapshot{Username: "admin", MaxFail: 5, LockDuration: 15 * time.Minute}
	st := computeSecurity(bare, Settings{Listen: ":16680"}, true, false)
	if st.Level != "risk" || st.Score != 100-20-15-10-5 {
		t.Fatalf("裸奔应高风险: %+v", st)
	}
	for _, id := range []string{"twoFactor", "entry", "https", "username"} {
		if item(st, id).Status != secWarn {
			t.Errorf("%s 应为 warn", id)
		}
	}
	// 仅内网：安全入口、HTTPS、用户名视为满分，二次验证减半扣分，不触发高风险
	st = computeSecurity(bare, Settings{Listen: ":16680", LANOnly: true}, false, false)
	if st.Score != 90 || st.Level != "good" {
		t.Fatalf("仅内网应 90 分良好: %+v", st)
	}
	if item(st, "entry").Points != 0 || item(st, "https").Points != 0 || item(st, "username").Points != 0 || item(st, "twoFactor").Points != 10 {
		t.Fatalf("仅内网各项扣分不符: %+v", st.Items)
	}
	// 锁定策略过松、关闭陌生来源预算
	loose := strong
	loose.MaxFail = 50
	st = computeSecurity(loose, Settings{SafePath: "/x7k2p9", LoginGlobalLimit: -1}, true, true)
	if item(st, "lockout").Points != 10 || item(st, "globalLimit").Points != 5 || st.Score != 85 || st.Level != "warn" {
		t.Fatalf("宽松策略应扣分: %+v", st)
	}
	short := strong
	short.LockDuration = time.Minute
	if st := computeSecurity(short, ok, true, true); item(st, "lockout").Points != 10 {
		t.Fatal("锁定时间过短应扣分")
	}
	// Passkey 同样算二次验证
	pk := bare
	pk.Passkeys = 1
	if st := computeSecurity(pk, Settings{Listen: ":16680", LANOnly: true}, false, false); item(st, "twoFactor").Points != 0 {
		t.Fatal("有 Passkey 应满足二次验证")
	}
	// 得分不为负
	worst := auth.Snapshot{Username: "root", PasswordIssue: "weak", MaxFail: 99, LockDuration: time.Second}
	if st := computeSecurity(worst, Settings{LoginGlobalLimit: -1}, true, false); st.Score < 0 || st.Level != "risk" {
		t.Fatalf("最差情况: %+v", st)
	}
}

func TestListenExposed(t *testing.T) {
	cases := map[string]bool{
		":16680": true, "0.0.0.0:16680": true, "[::]:16680": true, "192.168.1.2:80": true, "example.com:80": true, "": true,
		"127.0.0.1:16680": false, "[::1]:16680": false, "localhost:16680": false,
	}
	for in, want := range cases {
		if got := listenExposed(in); got != want {
			t.Errorf("listenExposed(%q) = %v，期望 %v", in, got, want)
		}
	}
}

func TestRequestFromPublic(t *testing.T) {
	cases := map[string]bool{
		"8.8.8.8:1234": true, "[2400:cb00::1]:1": true, "192.168.1.5:1": false, "10.1.2.3:1": false, "172.16.0.9:1": false,
		"100.64.0.5:1": false, "127.0.0.1:1": false, "169.254.1.1:1": false, "[fe80::1]:1": false, "[fd00::1]:1": false,
	}
	for addr, want := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = addr
		if got := requestFromPublic(r); got != want {
			t.Errorf("requestFromPublic(%s) = %v，期望 %v", addr, got, want)
		}
	}
}

func TestSecurityStatusEndpoint(t *testing.T) {
	_, srv, newClient := newAccountApp(t, nil)
	if code, _ := doJSON(t, newClient(), "GET", srv.URL+"/api/system/security", nil); code != http.StatusUnauthorized {
		t.Fatalf("未登录应返回 401，实际 %d", code)
	}
	c := newClient()
	mustLogin(t, c, srv.URL, "admin", auth.DefaultPassword)
	code, body := doJSON(t, c, "GET", srv.URL+"/api/system/security", nil)
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	var st securityStatus
	if err := json.Unmarshal(body, &st); err != nil {
		t.Fatal(err)
	}
	if st.PasswordIssue != "default" || st.Level != "risk" || len(st.Items) != 7 {
		t.Fatalf("默认密码实例的评分不符: %s", body)
	}
	// 响应中不含任何凭据字段
	for _, bad := range []string{"passwordHash", "sessionSecret", "totpSecret"} {
		if strings.Contains(string(body), bad) {
			t.Errorf("安全评分不应包含 %s", bad)
		}
	}
	// 来自公网的请求必然被判定为暴露（本机是受信任代理，其 X-Real-IP 被采信）
	code, body = doWith(t, c, "GET", srv.URL+"/api/system/security", map[string]string{"X-Real-IP": "8.8.4.4"}, nil)
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	var pub securityStatus
	_ = json.Unmarshal(body, &pub)
	if !pub.Exposed {
		t.Fatalf("公网来源应判定为暴露: %s", body)
	}
}

// ---- 密码预检 ----

func TestPasswordCheckEndpoint(t *testing.T) {
	_, srv, c := loginApp(t)
	cases := []struct {
		user, pass, code string
	}{
		{"", "short", "tooShort"},
		{"", "password123", "common"},
		{"", "admin-Router-9x", "username"},
		{"bob", "admin-Router-9x", ""},
		{"", "MyGately-2026-xyz", "product"},
		{"", "abcdefghijkl", "sequence"},
		{"", "Zm3$kq9Lr!vT", ""},
	}
	for _, tc := range cases {
		code, body := doJSON(t, c, "POST", srv.URL+"/api/account/password-check", map[string]string{"username": tc.user, "password": tc.pass})
		if code != 200 {
			t.Fatalf("%q: %d %s", tc.pass, code, body)
		}
		var r struct {
			OK   bool   `json:"ok"`
			Code string `json:"code"`
		}
		_ = json.Unmarshal(body, &r)
		if tc.code == "" && !r.OK || tc.code != "" && (r.OK || r.Code != tc.code) {
			t.Errorf("%q: 期望 %q，得到 %s", tc.pass, tc.code, body)
		}
		if strings.Contains(string(body), tc.pass) {
			t.Errorf("响应不应回显密码: %s", body)
		}
	}
	if code, _ := doJSON(t, http.DefaultClient, "POST", srv.URL+"/api/account/password-check", map[string]string{"password": "x"}); code != http.StatusUnauthorized {
		t.Fatalf("未登录应返回 401，实际 %d", code)
	}
}

// ---- 关闭密码登录 ----

func TestPasswordLoginSwitch(t *testing.T) {
	a, srv, admin := loginApp(t)
	host := strings.TrimPrefix(srv.URL, "http://")
	rpID := webauthn.RPID(host)
	key := webauthntest.New()

	providers := func() bool {
		_, body := doJSON(t, http.DefaultClient, "GET", srv.URL+"/api/login/providers", nil)
		var r struct {
			PasswordLogin bool `json:"passwordLogin"`
		}
		_ = json.Unmarshal(body, &r)
		return r.PasswordLogin
	}
	if !providers() {
		t.Fatal("默认应允许密码登录")
	}
	// 未注册 Passkey 不能关闭
	if code, _ := doJSON(t, admin, "PUT", srv.URL+"/api/account/password-login", map[string]any{"password": "test-password", "disabled": true}); code != 400 {
		t.Fatalf("没有 Passkey 不能关闭，实际 %d", code)
	}
	// 注册 Passkey
	_, body := doJSON(t, admin, "POST", srv.URL+"/api/account/passkeys/begin", map[string]string{})
	var opts struct{ Challenge string }
	_ = json.Unmarshal(body, &opts)
	reg := map[string]any{"name": "k", "password": "test-password", "challenge": opts.Challenge, "credential": key.Register(rpID, opts.Challenge, srv.URL)}
	if code, body := doJSON(t, admin, "POST", srv.URL+"/api/account/passkeys", reg); code != 200 {
		t.Fatalf("注册失败 %d %s", code, body)
	}
	if code, _ := doJSON(t, admin, "PUT", srv.URL+"/api/account/password-login", map[string]any{"password": "wrong-pass-1234", "disabled": true}); code != 400 {
		t.Fatalf("密码错误不能修改，实际 %d", code)
	}
	if code, body := doJSON(t, admin, "PUT", srv.URL+"/api/account/password-login", map[string]any{"password": "test-password", "disabled": true}); code != 200 {
		t.Fatalf("关闭失败 %d %s", code, body)
	}
	if providers() {
		t.Fatal("关闭后登录页应隐藏密码表单")
	}
	code1, body1 := doJSON(t, http.DefaultClient, "POST", srv.URL+"/api/login", map[string]string{"username": "admin", "password": "test-password"})
	code2, body2 := doJSON(t, http.DefaultClient, "POST", srv.URL+"/api/login", map[string]string{"username": "nobody", "password": "test-password"})
	if code1 != 401 || code2 != 401 || string(body1) != string(body2) {
		t.Fatalf("关闭后密码登录应与账号不存在返回完全一致的响应: %d %s / %d %s", code1, body1, code2, body2)
	}
	// Passkey 仍可登录
	jar, _ := cookiejar.New(nil)
	fresh := &http.Client{Jar: jar}
	_, body = doJSON(t, fresh, "POST", srv.URL+"/api/login/passkey/begin", map[string]string{})
	var lo struct{ Challenge string }
	_ = json.Unmarshal(body, &lo)
	finish := map[string]any{"challenge": lo.Challenge, "credential": key.Assert(rpID, lo.Challenge, srv.URL)}
	if code, body := doJSON(t, fresh, "POST", srv.URL+"/api/login/passkey/finish", finish); code != 200 {
		t.Fatalf("Passkey 登录失败 %d %s", code, body)
	}
	// 不能删除最后一个 Passkey
	_, body = doJSON(t, admin, "GET", srv.URL+"/api/account/passkeys", nil)
	var list []struct{ ID string }
	_ = json.Unmarshal(body, &list)
	if code, _ := doJSON(t, admin, "DELETE", srv.URL+"/api/account/passkeys/"+list[0].ID, map[string]string{"password": "test-password"}); code != 400 {
		t.Fatalf("不能删除最后一个 Passkey，实际 %d", code)
	}
	// 命令行重置密码会重新开启
	req, _ := auth.NewPasswordReset("Tq8!wLm3-zRv9")
	if err := a.auth.ApplyReset(req); err != nil {
		t.Fatal(err)
	}
	if !providers() {
		t.Fatal("重置密码后应重新允许密码登录")
	}
}

// ---- 陌生来源全局预算 ----

func TestGlobalBudgetEndpoint(t *testing.T) {
	_, srv, newClient := newAccountApp(t, func(m *auth.Manager) { _ = m.SetBootstrapPassword("Zm3$kq9Lr!vT") })
	// 先用本机（127.0.0.1）成功登录一次，使其成为常用地址
	good := newClient()
	mustLogin(t, good, srv.URL, "admin", "Zm3$kq9Lr!vT")
	// 缩小预算便于测试：通过设置接口修改
	code, body := doJSON(t, good, "GET", srv.URL+"/api/system/settings", nil)
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	var s map[string]any
	_ = json.Unmarshal(body, &s)
	s["loginGlobalLimit"] = 3
	if code, body := doJSON(t, good, "PUT", srv.URL+"/api/system/settings", s); code != 200 {
		t.Fatalf("保存设置失败 %d %s", code, body)
	}
	unknown := func(ip, pass string) (int, string) {
		code, body := doWith(t, newClient(), "POST", srv.URL+"/api/login", map[string]string{"X-Real-IP": ip}, map[string]string{"username": "admin", "password": pass})
		return code, string(body)
	}
	for i, ip := range []string{"203.0.113.1", "203.0.113.2", "203.0.113.3"} {
		if code, body := unknown(ip, "wrong-pass-"+ip); code != 401 {
			t.Fatalf("第 %d 次失败应为 401: %d %s", i+1, code, body)
		}
	}
	// 预算耗尽：陌生地址即使密码正确也返回 429
	code2, body2 := unknown("203.0.113.9", "Zm3$kq9Lr!vT")
	if code2 != http.StatusTooManyRequests {
		t.Fatalf("预算耗尽后陌生地址应返回 429: %d %s", code2, body2)
	}
	// 常用地址（本机）不受影响
	mustLogin(t, newClient(), srv.URL, "admin", "Zm3$kq9Lr!vT")
}

func TestGlobalBudgetSettingValidation(t *testing.T) {
	_, srv, c := loginApp(t)
	code, body := doJSON(t, c, "GET", srv.URL+"/api/system/settings", nil)
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	var s map[string]any
	_ = json.Unmarshal(body, &s)
	s["loginGlobalLimit"] = 100000
	if code, _ := doJSON(t, c, "PUT", srv.URL+"/api/system/settings", s); code != 400 {
		t.Fatalf("过大的预算应被拒绝，实际 %d", code)
	}
	s["loginGlobalLimit"] = -1
	if code, body := doJSON(t, c, "PUT", srv.URL+"/api/system/settings", s); code != 200 {
		t.Fatalf("负数表示关闭，应可保存: %d %s", code, body)
	}
}
