package webservice

import (
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/fileusers"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// fakeFiles 模拟文件服务共享用户的自助改密。
type fakeFiles struct {
	mu    sync.Mutex
	on    bool
	users map[string]string
}

func (f *fakeFiles) SelfService() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.on
}

func (f *fakeFiles) ChangePassword(name, current, next string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.on {
		return fileusers.ErrSelfServiceOff
	}
	if p, ok := f.users[name]; !ok || p != current {
		return fileusers.ErrBadCredentials
	}
	f.users[name] = next
	return nil
}

// passwordEnv 启动一个开启共享用户的站点与一个只有本地用户的站点，返回发送改密表单的函数。
func passwordEnv(t *testing.T) (addr string, r *listenerRunner, post func(host string, form url.Values, hdr map[string]string) (*http.Response, string)) {
	t.Helper()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("backend")) }))
	t.Cleanup(backend.Close)
	l := Listener{Sites: []Site{
		{Enable: true, Key: "a", Domains: []string{"a.test"}, Targets: []string{backend.URL}, WebAuth: true, SharedUsers: true},
		{Enable: true, Key: "local", Domains: []string{"local.test"}, Targets: []string{backend.URL}, WebAuth: true, Users: []string{"u:p"}},
	}}
	addr, r = startRunner(t, l, nil, nil)
	client := noRedirect()
	post = func(host string, form url.Values, hdr map[string]string) (*http.Response, string) {
		req, _ := http.NewRequest("POST", "http://"+addr+authPath+passwordSuffix, strings.NewReader(form.Encode()))
		req.Host = host
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, string(b)
	}
	return addr, r, post
}

// useSharedUsers 设置共享用户，并模拟模块保存新密码。
func useSharedUsers(t *testing.T, a *authenticator, users []AuthUser) {
	t.Helper()
	users, err := normalizeAuthUsers(users, nil)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	a.shared.set(users, nil)
	a.changeShared = func(u AuthUser, hash string) error {
		mu.Lock()
		defer mu.Unlock()
		for i := range users {
			if users[i].ID == u.ID && users[i].Password == u.Password {
				users[i].Password = hash
				a.shared.set(users, nil)
				return nil
			}
		}
		return errPasswordChanged
	}
}

func TestSelfServicePasswordAuthUsers(t *testing.T) {
	addr, r, post := passwordEnv(t)
	a := r.auth
	secret := auth.NewTOTPSecret()
	useSharedUsers(t, a, []AuthUser{{Name: "alice", Password: "old-password"}, {Name: "bob", Password: "bob-password", TOTPSecret: secret}})
	var changed []string
	a.onPasswordChanged = func(kind, name, ip string) { changed = append(changed, kind+":"+name) }
	files := &fakeFiles{users: map[string]string{}}
	a.files = files
	client := noRedirect()
	browser := map[string]string{"User-Agent": "Mozilla/5.0", "Accept": "text/html"}

	// 两种开关都关闭时页面不存在，登录页也不显示入口
	if resp, _ := get(t, client, addr, "a.test", authPath+passwordSuffix, browser); resp.StatusCode != 404 {
		t.Fatalf("未开启时应返回 404: %d", resp.StatusCode)
	}
	if _, page := get(t, client, addr, "a.test", authPath+"/login", browser); strings.Contains(page, passwordSuffix) {
		t.Fatal("未开启时登录页不应显示修改密码入口")
	}
	a.selfAuth.Store(true)
	resp, page := get(t, client, addr, "a.test", authPath+passwordSuffix, browser)
	if resp.StatusCode != 200 || !strings.Contains(page, `name="current"`) || !strings.Contains(page, `name="code"`) || strings.Contains(page, `type="radio"`) {
		t.Fatalf("改密页面内容错误: %d", resp.StatusCode)
	}
	if _, page := get(t, client, addr, "a.test", authPath+"/login", browser); !strings.Contains(page, `href="`+authPath+passwordSuffix+`"`) {
		t.Fatal("登录页应显示修改密码入口")
	}
	// 没有开启共享用户的站点上，共享用户不能改密
	if resp, _ := get(t, client, addr, "local.test", authPath+passwordSuffix, browser); resp.StatusCode != 404 {
		t.Fatalf("未开启共享用户的站点应返回 404: %d", resp.StatusCode)
	}

	// 先登录拿到会话，改密后旧会话失效
	form := url.Values{"username": {"alice"}, "password": {"old-password"}, "next": {"/"}}
	req, _ := http.NewRequest("POST", "http://"+addr+authPath+"/login", strings.NewReader(form.Encode()))
	req.Host = "a.test"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	login, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	login.Body.Close()
	session := login.Cookies()[0]
	cookieHdr := map[string]string{"Cookie": session.Name + "=" + session.Value}
	if resp, body := get(t, client, addr, "a.test", "/", cookieHdr); resp.StatusCode != 200 || body != "backend" {
		t.Fatalf("登录后应能访问: %d", resp.StatusCode)
	}

	cases := []struct {
		form url.Values
		code int
		want string
	}{
		{url.Values{"username": {"alice"}, "current": {"bad"}, "next": {"new-password-1"}, "confirm": {"new-password-1"}}, 401, "用户名或当前密码错误"},
		{url.Values{"username": {"alice"}, "current": {"old-password"}, "next": {"new-password-1"}, "confirm": {"new-password-2"}}, 400, "两次输入的新密码不一致"},
		{url.Values{"username": {"alice"}, "current": {"old-password"}, "next": {"short"}, "confirm": {"short"}}, 400, "新密码长度"},
		{url.Values{"username": {"alice"}, "current": {"old-password"}, "next": {"old-password"}, "confirm": {"old-password"}}, 400, "不能与当前密码相同"},
		{url.Values{"username": {"bob"}, "current": {"bob-password"}, "next": {"new-password-1"}, "confirm": {"new-password-1"}}, 401, "请输入动态验证码"},
	}
	for i, c := range cases {
		resp, body := post("a.test", c.form, nil)
		if resp.StatusCode != c.code || !strings.Contains(body, c.want) {
			t.Errorf("用例 %d: 期望 %d %q，实际 %d", i, c.code, c.want, resp.StatusCode)
		}
	}
	// 其他网站代为提交的表单直接拒绝
	ok := url.Values{"username": {"alice"}, "current": {"old-password"}, "next": {"new-password-1"}, "confirm": {"new-password-1"}}
	if resp, _ := post("a.test", ok, map[string]string{"Origin": "https://evil.test"}); resp.StatusCode != 403 {
		t.Fatalf("跨站提交应返回 403: %d", resp.StatusCode)
	}
	resp, body := post("a.test", ok, nil)
	if resp.StatusCode != 200 || !strings.Contains(body, "密码已修改") {
		t.Fatalf("改密失败: %d %s", resp.StatusCode, body)
	}
	cleared := false
	for _, c := range resp.Cookies() {
		if c.Name == session.Name && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("改密后应清除本站会话 Cookie")
	}
	if resp, _ := get(t, client, addr, "a.test", "/", map[string]string{"Cookie": cookieHdr["Cookie"], "User-Agent": "Mozilla/5.0", "Accept": "text/html"}); resp.StatusCode != 302 {
		t.Fatalf("改密后旧会话应失效: %d", resp.StatusCode)
	}
	if _, ok := a.shared.get().checkPassword("alice", "new-password-1"); !ok {
		t.Fatal("新密码应生效")
	}
	if _, ok := a.shared.get().checkPassword("alice", "old-password"); ok {
		t.Fatal("旧密码应失效")
	}
	// 开启了 TOTP 的用户需要动态验证码
	bob := url.Values{"username": {"bob"}, "current": {"bob-password"}, "code": {totpNow(secret)}, "next": {"new-password-1"}, "confirm": {"new-password-1"}}
	if resp, body := post("a.test", bob, nil); resp.StatusCode != 200 || !strings.Contains(body, "密码已修改") {
		t.Fatalf("带验证码改密失败: %d", resp.StatusCode)
	}
	if strings.Join(changed, ",") != "auth:alice,auth:bob" {
		t.Fatalf("应记录改密: %v", changed)
	}
}

// TestSelfServicePasswordFileUsers 验证文件服务账号改密，以及两种账号同时开启时按选择的类型修改。
func TestSelfServicePasswordFileUsers(t *testing.T) {
	addr, r, post := passwordEnv(t)
	a := r.auth
	useSharedUsers(t, a, []AuthUser{{Name: "alice", Password: "web-password"}})
	files := &fakeFiles{on: true, users: map[string]string{"alice": "file-password"}}
	a.files = files
	client := noRedirect()
	browser := map[string]string{"User-Agent": "Mozilla/5.0", "Accept": "text/html"}

	// 只开启文件服务账号：任何开启网页认证的站点都提供页面，不显示验证码与类型切换
	resp, page := get(t, client, addr, "local.test", authPath+passwordSuffix, browser)
	if resp.StatusCode != 200 || strings.Contains(page, `name="code"`) || strings.Contains(page, `type="radio"`) || !strings.Contains(page, `name="kind" value="file"`) {
		t.Fatalf("文件服务账号页面内容错误: %d", resp.StatusCode)
	}
	// 未开启共享认证用户时，提交 auth 类型按文件服务账号处理
	form := url.Values{"kind": {"auth"}, "username": {"alice"}, "current": {"web-password"}, "next": {"new-password-1"}, "confirm": {"new-password-1"}}
	if resp, _ := post("a.test", form, nil); resp.StatusCode != 401 {
		t.Fatalf("应按文件服务账号校验: %d", resp.StatusCode)
	}
	a.selfAuth.Store(true)
	if _, page := get(t, client, addr, "a.test", authPath+passwordSuffix+"?kind=file", browser); !strings.Contains(page, `value="file" checked`) || !strings.Contains(page, `type="radio"`) {
		t.Fatal("两种账号都开启时应显示类型切换并按参数选中")
	}
	form = url.Values{"kind": {"file"}, "username": {"alice"}, "current": {"file-password"}, "next": {"new-password-1"}, "confirm": {"new-password-1"}}
	resp, body := post("a.test", form, nil)
	if resp.StatusCode != 200 || !strings.Contains(body, "密码已修改") || strings.Contains(body, authPath+"/login") {
		t.Fatalf("文件服务账号改密失败或显示了网页登录入口: %d", resp.StatusCode)
	}
	if files.users["alice"] != "new-password-1" {
		t.Fatal("文件服务账号的新密码应保存")
	}
	if _, ok := a.shared.get().checkPassword("alice", "web-password"); !ok {
		t.Fatal("同名的网页登录账号不应受影响")
	}
}

// TestSelfServicePasswordRateLimit 验证改密与登录共用失败次数限制。
func TestSelfServicePasswordRateLimit(t *testing.T) {
	_, r, post := passwordEnv(t)
	useSharedUsers(t, r.auth, []AuthUser{{Name: "alice", Password: "old-password"}})
	r.auth.selfAuth.Store(true)
	r.auth.files = &fakeFiles{}
	bad := url.Values{"username": {"alice"}, "current": {"bad"}, "next": {"new-password-1"}, "confirm": {"new-password-1"}}
	for i := 0; i < maxFailures; i++ {
		post("a.test", bad, nil)
	}
	good := url.Values{"username": {"alice"}, "current": {"old-password"}, "next": {"new-password-1"}, "confirm": {"new-password-1"}}
	if resp, body := post("a.test", good, nil); resp.StatusCode != 429 || !strings.Contains(body, "失败次数过多") {
		t.Fatalf("失败次数过多后应拒绝: %d", resp.StatusCode)
	}
}

// TestSelfServicePasswordOIDC 验证 OIDC 签发者域名上的改密页面。
func TestSelfServicePasswordOIDC(t *testing.T) {
	a := newAuthenticator("secret")
	a.files = &fakeFiles{}
	useSharedUsers(t, a, []AuthUser{{Name: "alice", Password: "old-password"}})
	a.selfAuth.Store(true)
	e := &oidcEnv{host: "id.test", issuer: "http://id.test"}
	req := httptest.NewRequest("GET", "http://id.test"+oidcPath+passwordSuffix, nil)
	rec := httptest.NewRecorder()
	a.serveOIDC(rec, req, e, logbuf.New(10, log.New(io.Discard, "", 0)).Logger("t"))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `action="`+oidcPath+passwordSuffix+`"`) {
		t.Fatalf("OIDC 签发者域名上应提供改密页面: %d", rec.Code)
	}
}

// TestChangeSharedPassword 验证模块保存自助修改的密码，并拒绝期间被管理员修改过的用户。
func TestChangeSharedPassword(t *testing.T) {
	st, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := New(nil, nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(10, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	users, _ := normalizeAuthUsers([]AuthUser{{Name: "alice", Password: "old-password"}}, nil)
	m.cfg.Settings.AuthUsers = users
	m.auth.shared.set(users, nil)
	u := users[0]
	if err := m.changeSharedPassword(u, "$2a$new"); err != nil {
		t.Fatal(err)
	}
	if m.cfg.Settings.AuthUsers[0].Password != "$2a$new" || m.auth.shared.get().users["alice"].Password != "$2a$new" {
		t.Fatal("新密码应保存并生效")
	}
	// u 仍是旧密码：视为期间被修改过
	if err := m.changeSharedPassword(u, "$2a$other"); !errors.Is(err, errPasswordChanged) {
		t.Fatalf("密码已变化时应拒绝: %v", err)
	}
	var saved fileConfig
	if _, err := st.Load(ID, &saved); err != nil || saved.Settings.AuthUsers[0].Password != "$2a$new" {
		t.Fatalf("应写入配置文件: %v", err)
	}
}

// failingFiles 模拟保存时被服务检查拒绝的文件服务账号（当前密码正确）。
type failingFiles struct{ fakeFiles }

func (f *failingFiles) ChangePassword(name, current, next string) error {
	if err := f.fakeFiles.ChangePassword(name, current, next); err != nil {
		return err
	}
	return errors.New("共享用户 bob 的目录需为绝对路径")
}

// TestSelfServicePasswordCooldown 验证同一账号改密后 1 分钟内不能再改，其他账号不受影响；
// 保存失败的详细原因只写日志，访客看到统一提示。
func TestSelfServicePasswordCooldown(t *testing.T) {
	_, r, post := passwordEnv(t)
	a := r.auth
	files := &fakeFiles{on: true, users: map[string]string{"alice": "file-password", "carol": "carol-password"}}
	a.files = files
	form := func(name, cur, next string) url.Values {
		return url.Values{"kind": {"file"}, "username": {name}, "current": {cur}, "next": {next}, "confirm": {next}}
	}
	if resp, _ := post("a.test", form("alice", "file-password", "new-password-1"), nil); resp.StatusCode != 200 {
		t.Fatalf("首次改密应成功: %d", resp.StatusCode)
	}
	// 用户名大小写不同也视为同一账号
	if resp, body := post("a.test", form("Alice", "new-password-1", "new-password-2"), nil); resp.StatusCode != 429 || !strings.Contains(body, "密码刚修改过") {
		t.Fatalf("冷却期内应拒绝: %d", resp.StatusCode)
	}
	if files.users["alice"] != "new-password-1" {
		t.Fatal("冷却期内不应保存新密码")
	}
	if resp, _ := post("a.test", form("carol", "carol-password", "new-password-3"), nil); resp.StatusCode != 200 {
		t.Fatalf("其他账号不受冷却影响: %d", resp.StatusCode)
	}
	// 冷却结束后可以再改
	a.pwChangedAt.Store(cooldownKey(kindFile, "alice"), time.Now().Add(-passwordCooldown-time.Second))
	if resp, _ := post("a.test", form("alice", "new-password-1", "new-password-2"), nil); resp.StatusCode != 200 {
		t.Fatalf("冷却结束后应允许改密: %d", resp.StatusCode)
	}

	var logged error
	a.onPasswordFailed = func(_, _, _ string, err error) { logged = err }
	a.files = &failingFiles{fakeFiles{on: true, users: map[string]string{"dave": "dave-password"}}}
	resp, body := post("a.test", form("dave", "dave-password", "new-password-4"), nil)
	if resp.StatusCode != 500 || !strings.Contains(body, "保存失败，请联系管理员") || strings.Contains(body, "bob") {
		t.Fatalf("保存失败时应只显示统一提示: %d", resp.StatusCode)
	}
	if logged == nil || !strings.Contains(logged.Error(), "bob") {
		t.Fatalf("详细原因应写入日志: %v", logged)
	}
}
