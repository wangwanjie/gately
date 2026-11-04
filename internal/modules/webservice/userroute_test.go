package webservice

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// userBackend 返回 名称|X-Remote-User 请求头，便于同时检查分到哪个后端与传递的用户名。
func userBackend(t *testing.T, name string) *httptest.Server {
	t.Helper()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=60")
		_, _ = w.Write([]byte(name + "|" + r.Header.Get("X-Remote-User")))
	}))
	t.Cleanup(b.Close)
	return b
}

func basicHeader(user, pass string) map[string]string {
	return map[string]string{"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))}
}

// TestUserRoutesBasicAuth 覆盖基本认证下按用户分流、与分路径规则的优先级、用户名请求头与防伪造、公开路径。
func TestUserRoutesBasicAuth(t *testing.T) {
	main, admin, vip, api := userBackend(t, "main"), userBackend(t, "admin"), userBackend(t, "vip"), userBackend(t, "api")
	l := Listener{Sites: []Site{{
		Enable: true, Domains: []string{"app.test"}, Targets: []string{main.URL},
		BasicAuth: true, Users: []string{"admin:pw", "vip_user:pw", "bob:pw"},
		AuthOptional: true, Rules: []PathRule{{Prefix: "/", Auth: RuleAuthOn}, {Prefix: "/public/", Auth: RuleAuthOff}},
		Locations: []Location{
			{Prefix: "/api/", Targets: []string{api.URL}},
			// 只追加请求头的路径沿用后端，按用户分流时应改用该用户的后端
			{Prefix: "/hdr/", RequestHeaders: []string{"X-Loc: 1"}},
		},
		UserRoutes: []UserRoute{{Users: []string{"admin"}, Targets: []string{admin.URL}}, {Users: []string{"vip_user"}, Targets: []string{vip.URL}}},
		UserHeader: "x-remote-user",
	}}}
	addr, _ := startRunner(t, l, nil, nil)
	cases := []struct {
		user, path, want string
	}{
		{"admin", "/", "admin|admin"},
		{"vip_user", "/", "vip|vip_user"},
		{"bob", "/", "main|bob"},
		{"admin", "/api/x", "api|admin"},
		{"admin", "/hdr/x", "admin|admin"},
		{"bob", "/hdr/x", "main|bob"},
	}
	for _, c := range cases {
		hdr := basicHeader(c.user, "pw")
		hdr["X-Remote-User"] = "mallory"
		if _, body := get(t, http.DefaultClient, addr, "app.test", c.path, hdr); body != c.want {
			t.Fatalf("%s 访问 %s 得到 %q，期望 %q", c.user, c.path, body, c.want)
		}
	}
	// 公开路径的未登录访客使用站点后端，伪造的用户名头被删除
	if _, body := get(t, http.DefaultClient, addr, "app.test", "/public/a", map[string]string{"X-Remote-User": "admin"}); body != "main|" {
		t.Fatalf("公开路径得到 %q", body)
	}
	// 密码错误仍需认证
	if resp, _ := get(t, http.DefaultClient, addr, "app.test", "/", basicHeader("admin", "bad")); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("密码错误应 401，实际 %d", resp.StatusCode)
	}
}

// TestUserRoutesWebSession 覆盖网页登录会话（含第三方登录的别名）按用户分流，以及公开路径上已登录访客的分流。
func TestUserRoutesWebSession(t *testing.T) {
	main, admin := userBackend(t, "main"), userBackend(t, "admin")
	l := Listener{Sites: []Site{{
		Enable: true, Domains: []string{"web.test"}, Targets: []string{main.URL},
		WebAuth: true, OAuthUsers: []string{"github:alice"},
		AuthOptional: true, Rules: []PathRule{{Prefix: "/private/", Auth: RuleAuthOn}},
		UserRoutes: []UserRoute{{Users: []string{"github:alice"}, Targets: []string{admin.URL}}},
	}}}
	addr, rn := startRunner(t, l, nil, nil)
	h := rn.route("web.test")
	// 第三方登录会话的主标识为稳定 ID，用户名作为别名
	req := httptest.NewRequest("GET", "http://web.test/", nil)
	rec := httptest.NewRecorder()
	h.auth.issueSession(rec, req, h.site, "github:1001", "alice")
	cookie := rec.Result().Cookies()[0]
	hdr := map[string]string{"Cookie": cookie.Name + "=" + cookie.Value}
	for _, path := range []string{"/private/x", "/"} {
		if _, body := get(t, noRedirect(), addr, "web.test", path, hdr); !strings.HasPrefix(body, "admin|") {
			t.Fatalf("已登录访问 %s 得到 %q，期望分到 admin", path, body)
		}
	}
	if _, body := get(t, noRedirect(), addr, "web.test", "/", nil); !strings.HasPrefix(body, "main|") {
		t.Fatalf("未登录访问公开路径得到 %q", body)
	}
}

// TestUserRoutesForwardAuth 覆盖转发认证返回的 Remote-User 参与分流，访客伪造的不生效。
func TestUserRoutesForwardAuth(t *testing.T) {
	var seen http.Header
	authz := fakeAuthelia(t, &seen)
	main, admin := userBackend(t, "main"), userBackend(t, "admin")
	l := Listener{Sites: []Site{{
		Enable: true, Domains: []string{"fa.test"}, Targets: []string{main.URL},
		ForwardAuth: ForwardAuth{URL: authz.URL},
		UserRoutes:  []UserRoute{{Users: []string{"alice"}, Targets: []string{admin.URL}}, {Users: []string{"mallory"}, Targets: []string{main.URL}}},
	}}}
	addr, _ := startRunner(t, l, nil, nil)
	if _, body := get(t, noRedirect(), addr, "fa.test", "/", map[string]string{"Cookie": "session=ok", "Remote-User": "mallory"}); !strings.HasPrefix(body, "admin|") {
		t.Fatalf("转发认证用户 alice 应分到 admin，得到 %q", body)
	}
}

func TestValidateUserRoutes(t *testing.T) {
	base := func() Site {
		return Site{Domains: []string{"a.test"}, Targets: []string{"http://127.0.0.1:1"}, BasicAuth: true, Users: []string{"a:b"}}
	}
	ok := base()
	ok.UserRoutes = []UserRoute{{Users: []string{" a ", ""}, Targets: []string{"http://127.0.0.1:2"}}}
	ok.UserHeader = " x-remote-user "
	if err := ok.validate(false); err != nil {
		t.Fatal(err)
	}
	if ok.UserHeader != "X-Remote-User" || len(ok.UserRoutes[0].Users) != 1 || ok.UserRoutes[0].Users[0] != "a" {
		t.Fatalf("整理结果错误: %+v", ok)
	}
	bad := map[string]func(s *Site){
		"未启用认证": func(s *Site) {
			s.BasicAuth = false
			s.UserRoutes = []UserRoute{{Users: []string{"a"}, Targets: []string{"http://x"}}}
		},
		"缺少用户": func(s *Site) { s.UserRoutes = []UserRoute{{Targets: []string{"http://x"}}} },
		"缺少后端": func(s *Site) { s.UserRoutes = []UserRoute{{Users: []string{"a"}}} },
		"非法后端": func(s *Site) { s.UserRoutes = []UserRoute{{Users: []string{"a"}, Targets: []string{"ftp://x"}}} },
		"用户重复": func(s *Site) {
			s.UserRoutes = []UserRoute{{Users: []string{"a"}, Targets: []string{"http://x"}}, {Users: []string{"a"}, Targets: []string{"http://y"}}}
		},
		"非法请求头": func(s *Site) { s.UserHeader = "X User" },
		"禁用请求头": func(s *Site) { s.UserHeader = "host" },
		"非反向代理": func(s *Site) {
			s.Type, s.Root = TypeStatic, "/tmp"
			s.UserHeader = "X-User"
		},
	}
	for name, mod := range bad {
		s := base()
		mod(&s)
		if err := s.validate(false); err == nil {
			t.Errorf("%s 应校验失败", name)
		}
	}
}

// TestUserRouteCacheKey 覆盖响应缓存键按分流规则与传递的用户名区分。
// 带 Cookie 或 Authorization 的请求本就不缓存，这里防的是转发认证按其他方式（如来源 IP）放行的情况
func TestUserRouteCacheKey(t *testing.T) {
	h := &siteHandler{site: Site{UserHeader: "X-Remote-User"}, userRoutes: []*userRouteHandler{{users: map[string]bool{"admin": true}}}}
	key := func(names ...string) string {
		r := h.withIdentity(httptest.NewRequest("GET", "http://a.test/x", nil), visitor{names: names})
		return cacheKey(r)
	}
	if key("admin") == key("bob") || key("bob") == key("carol") || key() == key("bob") {
		t.Fatal("不同用户的缓存键应不同")
	}
	h.site.UserHeader = ""
	if key("bob") != key("carol") || key("admin") == key("bob") {
		t.Fatal("未传递用户名时缓存键只按分流规则区分")
	}
}

// TestUserRouteThirdPartyCase 覆盖第三方登录标识与允许名单一样不区分大小写，站点用户名区分大小写。
func TestUserRouteThirdPartyCase(t *testing.T) {
	users := map[string]bool{routeUserKey("GitHub:Alice"): true, routeUserKey("Bob"): true}
	if !matchUserRoute(users, []string{"github:1001", "github:alice"}) {
		t.Fatal("第三方标识应不区分大小写匹配")
	}
	if matchUserRoute(users, []string{"bob"}) || !matchUserRoute(users, []string{"Bob"}) {
		t.Fatal("站点用户名应区分大小写")
	}
	s := Site{Domains: []string{"a.test"}, Targets: []string{"http://127.0.0.1:1"}, BasicAuth: true, Users: []string{"a:b"},
		UserRoutes: []UserRoute{{Users: []string{"github:A"}, Targets: []string{"http://x"}}, {Users: []string{"GITHUB:a"}, Targets: []string{"http://y"}}}}
	if err := s.validate(false); err == nil {
		t.Fatal("仅大小写不同的第三方标识应视为重复")
	}
}

// TestDelHeaderVariants 覆盖删除用户名请求头时一并删除下划线变体，其他请求头保留。
func TestDelHeaderVariants(t *testing.T) {
	h := http.Header{}
	h.Set("X-Remote-User", "a")
	h["X_remote_user"] = []string{"b"}
	h["x_Remote-User"] = []string{"c"}
	h.Set("X-Remote-Users", "keep")
	delHeaderVariants(h, "X-Remote-User")
	if len(h) != 1 || h.Get("X-Remote-Users") != "keep" {
		t.Fatalf("删除结果错误: %v", h)
	}
	f := &forwardAuther{headers: []string{"Remote-User"}}
	r := httptest.NewRequest("GET", "http://a.test/", nil)
	r.Header["Remote_user"] = []string{"mallory"}
	f.stripIdentity(r)
	if len(r.Header) != 0 {
		t.Fatalf("转发认证应删除伪造身份头的下划线变体: %v", r.Header)
	}
}

// emailBackend 返回后端收到的所有邮箱头（含下划线写法的变体），便于检查伪造的头是否都被删除。
func emailBackend(t *testing.T, header string) *httptest.Server {
	t.Helper()
	want := strings.ReplaceAll(header, "_", "-")
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got []string
		for k, vs := range r.Header {
			if strings.EqualFold(strings.ReplaceAll(k, "_", "-"), want) {
				got = append(got, vs...)
			}
		}
		_, _ = w.Write([]byte(strings.Join(got, ",")))
	}))
	t.Cleanup(b.Close)
	return b
}

// TestEmailHeaderSharedUsers 覆盖网页认证下传递邮箱：只有共享用户带邮箱，同名站点用户与第三方登录不带，
// 访客伪造的邮箱头（含下划线变体）在任何路径上都被删除，修改共享用户邮箱后立即生效。
func TestEmailHeaderSharedUsers(t *testing.T) {
	backend := emailBackend(t, "Remote-Email")
	l := Listener{Sites: []Site{{
		Enable: true, Key: "fx", Domains: []string{"fx.test"}, Targets: []string{backend.URL},
		WebAuth: true, SharedUsers: true, Users: []string{"bob:site-pw"}, OAuthUsers: []string{"github:alice"},
		AuthOptional: true, Rules: []PathRule{{Prefix: "/", Auth: RuleAuthOn}, {Prefix: "/public/", Auth: RuleAuthOff}},
		EmailHeader: "remote-email",
	}}}
	addr, rn := startRunner(t, l, nil, nil)
	setUsers := func(users ...AuthUser) []AuthUser {
		t.Helper()
		out, err := normalizeAuthUsers(users, nil)
		if err != nil {
			t.Fatal(err)
		}
		rn.auth.shared.set(out, "")
		return out
	}
	users := setUsers(
		AuthUser{Name: "alice", Password: "alice-pw", Email: "alice@example.com"},
		AuthUser{Name: "bob", Password: "shared-pw", Email: "bob@example.com"},
		AuthUser{Name: "carol", Password: "carol-pw"},
	)
	h := rn.route("fx.test")
	cookieFor := func(issue func(w http.ResponseWriter, r *http.Request)) string {
		rec := httptest.NewRecorder()
		issue(rec, httptest.NewRequest("GET", "http://fx.test/", nil))
		c := rec.Result().Cookies()[0]
		return c.Name + "=" + c.Value
	}
	alice := cookieFor(func(w http.ResponseWriter, r *http.Request) { h.auth.issueSharedSession(w, r, h.site, users[0]) })
	carol := cookieFor(func(w http.ResponseWriter, r *http.Request) { h.auth.issueSharedSession(w, r, h.site, users[2]) })
	github := cookieFor(func(w http.ResponseWriter, r *http.Request) {
		h.auth.issueSession(w, r, h.site, "github:1001", "alice")
	})
	forged := func(hdr map[string]string) map[string]string {
		hdr["Remote-Email"] = "admin@example.com"
		hdr["Remote_Email"] = "admin@example.com"
		return hdr
	}
	cases := []struct {
		name, path string
		hdr        map[string]string
		want       string
	}{
		{"共享用户会话", "/", forged(map[string]string{"Cookie": alice}), "alice@example.com"},
		{"共享用户会话访问公开路径", "/public/x", forged(map[string]string{"Cookie": alice}), "alice@example.com"},
		{"未填写邮箱的共享用户", "/", forged(map[string]string{"Cookie": carol}), ""},
		{"第三方登录", "/", forged(map[string]string{"Cookie": github}), ""},
		{"同名站点用户的基本认证", "/", forged(basicHeader("bob", "site-pw")), ""},
		{"共享用户的基本认证", "/", forged(basicHeader("bob", "shared-pw")), "bob@example.com"},
		{"未登录访问公开路径", "/public/x", forged(map[string]string{}), ""},
	}
	for _, c := range cases {
		resp, body := get(t, noRedirect(), addr, "fx.test", c.path, c.hdr)
		if resp.StatusCode != http.StatusOK || body != c.want {
			t.Fatalf("%s: 状态 %d，后端收到邮箱 %q，期望 %q", c.name, resp.StatusCode, body, c.want)
		}
	}
	// 未登录访问需要认证的路径被拦下，伪造的邮箱头到不了后端
	if resp, _ := get(t, noRedirect(), addr, "fx.test", "/", forged(map[string]string{})); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("未登录应 401，实际 %d", resp.StatusCode)
	}
	// 修改共享用户的邮箱后，已登录的会话立即传新邮箱
	users[0].Email = "alice@new.example.com"
	rn.auth.shared.set(users, "")
	if _, body := get(t, noRedirect(), addr, "fx.test", "/", map[string]string{"Cookie": alice}); body != "alice@new.example.com" {
		t.Fatalf("修改邮箱后得到 %q", body)
	}
}

// TestEmailHeaderForwardAuth 覆盖转发认证：认证服务返回的 Remote-Email 写入邮箱头；
// 未复制 Remote-Email 时访客自带的不可信，不写入；认证服务没给用户名时也不传邮箱。
func TestEmailHeaderForwardAuth(t *testing.T) {
	authz := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Cookie") {
		case "session=ok":
			w.Header().Set("Remote-User", "alice")
			w.Header().Set("Remote-Email", "alice@example.com")
		case "session=nouser":
			w.Header().Set("Remote-Email", "alice@example.com")
		default:
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(authz.Close)
	backend := emailBackend(t, "X-Email")
	l := Listener{Sites: []Site{
		{Enable: true, Domains: []string{"fa.test"}, Targets: []string{backend.URL}, ForwardAuth: ForwardAuth{URL: authz.URL}, EmailHeader: "X-Email"},
		{Enable: true, Domains: []string{"user-only.test"}, Targets: []string{backend.URL},
			ForwardAuth: ForwardAuth{URL: authz.URL, CopyHeaders: []string{"Remote-User"}}, EmailHeader: "X-Email"},
	}}
	addr, _ := startRunner(t, l, nil, nil)
	cases := []struct {
		host, cookie, want string
	}{
		{"fa.test", "session=ok", "alice@example.com"},
		{"fa.test", "session=nouser", ""},
		{"user-only.test", "session=ok", ""},
	}
	for _, c := range cases {
		hdr := map[string]string{"Cookie": c.cookie, "Remote-Email": "admin@example.com", "X-Email": "admin@example.com", "X_Email": "admin@example.com"}
		resp, body := get(t, noRedirect(), addr, c.host, "/", hdr)
		if resp.StatusCode != http.StatusOK || body != c.want {
			t.Fatalf("%s %s: 状态 %d，后端收到邮箱 %q，期望 %q", c.host, c.cookie, resp.StatusCode, body, c.want)
		}
	}
}

// TestValidateEmailHeader 覆盖邮箱请求头的整理与校验。
func TestValidateEmailHeader(t *testing.T) {
	base := func() Site {
		return Site{Domains: []string{"a.test"}, Targets: []string{"http://127.0.0.1:1"}, WebAuth: true, SharedUsers: true, EmailHeader: " remote-email "}
	}
	ok := base()
	if err := ok.validate(false); err != nil {
		t.Fatal(err)
	}
	if ok.EmailHeader != "Remote-Email" {
		t.Fatalf("请求头未规范化: %q", ok.EmailHeader)
	}
	fwd := Site{Domains: []string{"a.test"}, Targets: []string{"http://127.0.0.1:1"}, ForwardAuth: ForwardAuth{URL: "http://127.0.0.1:9091/"}, EmailHeader: "X-Email"}
	if err := fwd.validate(false); err != nil {
		t.Fatalf("转发认证应可传递邮箱: %v", err)
	}
	bad := map[string]func(s *Site){
		"未启用认证":       func(s *Site) { s.WebAuth, s.SharedUsers = false, false },
		"只有站点用户":      func(s *Site) { s.SharedUsers, s.Users = false, []string{"a:b"} },
		"非反向代理":       func(s *Site) { s.Type, s.Targets, s.Root = TypeStatic, nil, "/tmp" },
		"非法名称":        func(s *Site) { s.EmailHeader = "Remote Email" },
		"禁止的头":        func(s *Site) { s.EmailHeader = "authorization" },
		"与用户名头相同":     func(s *Site) { s.UserHeader, s.EmailHeader = "X-User", "x-user" },
		"与用户名头仅下划线不同": func(s *Site) { s.UserHeader, s.EmailHeader = "X-User", "X_User" },
	}
	for name, mod := range bad {
		s := base()
		mod(&s)
		if err := s.validate(false); err == nil {
			t.Errorf("%s 应校验失败", name)
		}
	}
}

// TestEmailHeaderCacheKeyAndValue 覆盖缓存键按邮箱区分（同名的站点用户与共享用户不串用），非法邮箱值不写入请求头。
func TestEmailHeaderCacheKeyAndValue(t *testing.T) {
	h := &siteHandler{site: Site{UserHeader: "X-Remote-User", EmailHeader: "Remote-Email"}}
	do := func(v visitor) *http.Request {
		return h.withIdentity(httptest.NewRequest("GET", "http://a.test/x", nil), v)
	}
	bob := visitor{names: []string{"bob"}}
	sharedBob := visitor{names: []string{"bob"}, email: "bob@example.com"}
	if cacheKey(do(bob)) == cacheKey(do(sharedBob)) {
		t.Fatal("邮箱不同的同名用户缓存键应不同")
	}
	if got := do(sharedBob).Header.Get("Remote-Email"); got != "bob@example.com" {
		t.Fatalf("邮箱头为 %q", got)
	}
	r := do(visitor{names: []string{"bob"}, email: "bob@example.com\r\nX-Admin: 1"})
	if _, ok := r.Header["Remote-Email"]; ok || r.Header.Get("X-Admin") != "" {
		t.Fatalf("非法邮箱不应写入请求头: %v", r.Header)
	}
	if _, ok := do(visitor{email: "x@example.com"}).Header["Remote-Email"]; ok {
		t.Fatal("未识别出用户时不应传邮箱")
	}
}
