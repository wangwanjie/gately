package webservice

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strconv"
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
	// 旧版的用户名请求头保存时并入身份请求头
	if ok.UserHeader != "" || len(ok.IdentityHeaders) != 1 || ok.IdentityHeaders[0] != (IdentityHeader{Header: "X-Remote-User", Field: IdentityUser}) ||
		len(ok.UserRoutes[0].Users) != 1 || ok.UserRoutes[0].Users[0] != "a" {
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

// identityBackend 返回后端收到的身份请求头（含下划线写法的变体），按请求头名排序，形如 Remote-Email=a@x|Remote-User=a。
func identityBackend(t *testing.T, headers ...string) *httptest.Server {
	t.Helper()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got []string
		for k, vs := range r.Header {
			for _, h := range headers {
				if headerVariantKey(k) == headerVariantKey(h) {
					for _, v := range vs {
						got = append(got, k+"="+v)
					}
				}
			}
		}
		sort.Strings(got)
		_, _ = w.Write([]byte(strings.Join(got, "|")))
	}))
	t.Cleanup(b.Close)
	return b
}

// allIdentityHeaders 为四种字段各一条的身份请求头。
var allIdentityHeaders = []IdentityHeader{
	{Header: "remote-user", Field: IdentityUser}, {Header: "Remote-Email", Field: IdentityEmail},
	{Header: "Remote-Name", Field: IdentityName}, {Header: "Remote-Groups", Field: IdentityGroups},
}

// forgedIdentity 在 hdr 中加入访客伪造的身份头，含下划线写法的变体。
func forgedIdentity(hdr map[string]string) map[string]string {
	for _, h := range []string{"Remote-User", "Remote-Email", "Remote-Name", "Remote-Groups", "Remote_Email", "Remote_Groups"} {
		hdr[h] = "forged"
	}
	return hdr
}

// TestIdentityHeadersSharedUsers 覆盖网页认证下传递身份信息：共享用户传用户名、邮箱、显示名与分组，
// 同名站点用户与第三方登录只传用户名，未填写的信息不传；访客伪造的身份头（含下划线变体）在任何路径上都被删除；
// 修改共享用户的信息后已登录的会话立即传新值。
func TestIdentityHeadersSharedUsers(t *testing.T) {
	backend := identityBackend(t, "Remote-User", "Remote-Email", "Remote-Name", "Remote-Groups")
	l := Listener{Sites: []Site{{
		Enable: true, Key: "fx", Domains: []string{"fx.test"}, Targets: []string{backend.URL},
		WebAuth: true, SharedUsers: true, Users: []string{"bob:site-pw"}, OAuthUsers: []string{"github:alice"},
		AuthOptional: true, Rules: []PathRule{{Prefix: "/", Auth: RuleAuthOn}, {Prefix: "/public/", Auth: RuleAuthOff}},
		IdentityHeaders: allIdentityHeaders,
	}}}
	addr, rn := startRunner(t, l, nil, nil)
	users, err := normalizeAuthUsers([]AuthUser{
		{Name: "alice", Password: "alice-pw", Email: "alice@example.com", DisplayName: "Alice Liddell", Groups: []string{"admins", "dev"}},
		{Name: "bob", Password: "shared-pw", Email: "bob@example.com", Groups: []string{"users"}},
		{Name: "carol", Password: "carol-pw"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rn.auth.shared.set(users, "")
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
	aliceWant := "Remote-Email=alice@example.com|Remote-Groups=admins,dev|Remote-Name=Alice Liddell|Remote-User=alice"
	cases := []struct {
		name, path string
		hdr        map[string]string
		want       string
	}{
		{"共享用户会话", "/", map[string]string{"Cookie": alice}, aliceWant},
		{"共享用户会话访问公开路径", "/public/x", map[string]string{"Cookie": alice}, aliceWant},
		{"未填写信息的共享用户", "/", map[string]string{"Cookie": carol}, "Remote-User=carol"},
		{"第三方登录", "/", map[string]string{"Cookie": github}, "Remote-User=github:1001"},
		{"同名站点用户的基本认证", "/", basicHeader("bob", "site-pw"), "Remote-User=bob"},
		{"共享用户的基本认证", "/", basicHeader("bob", "shared-pw"), "Remote-Email=bob@example.com|Remote-Groups=users|Remote-User=bob"},
		{"未登录访问公开路径", "/public/x", map[string]string{}, ""},
		// 访客在 Connection 中列出身份头，反向代理会把它们当逐跳头删掉；Gately 写入的身份头仍要到达后端
		{"Connection 列出身份头", "/", map[string]string{"Cookie": alice, "Connection": "keep-alive, Remote-Email, remote_groups", "Proxy-Connection": "Remote-Name"}, aliceWant},
	}
	for _, c := range cases {
		resp, body := get(t, noRedirect(), addr, "fx.test", c.path, forgedIdentity(c.hdr))
		if resp.StatusCode != http.StatusOK || body != c.want {
			t.Fatalf("%s: 状态 %d，后端收到 %q，期望 %q", c.name, resp.StatusCode, body, c.want)
		}
	}
	// 未登录访问需要认证的路径被拦下，伪造的身份头到不了后端
	if resp, _ := get(t, noRedirect(), addr, "fx.test", "/", forgedIdentity(map[string]string{})); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("未登录应 401，实际 %d", resp.StatusCode)
	}
	// 修改共享用户的信息后，已登录的会话立即传新值；清空的信息不再传
	users[0].Email, users[0].DisplayName, users[0].Groups = "alice@new.example.com", "", []string{"ops"}
	rn.auth.shared.set(users, "")
	if _, body := get(t, noRedirect(), addr, "fx.test", "/", forgedIdentity(map[string]string{"Cookie": alice})); body != "Remote-Email=alice@new.example.com|Remote-Groups=ops|Remote-User=alice" {
		t.Fatalf("修改信息后得到 %q", body)
	}
}

// TestIdentityHeadersForwardAuth 覆盖转发认证：认证服务返回的 Remote-* 写入身份请求头；
// 未复制的 Remote-* 是访客自带的，不可信，不写入；认证服务没给用户名时其他信息也不传。
func TestIdentityHeadersForwardAuth(t *testing.T) {
	authz := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Cookie") {
		case "session=ok":
			w.Header().Set("Remote-User", "alice")
		case "session=nouser":
		default:
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Remote-Email", "alice@example.com")
		w.Header().Set("Remote-Name", "Alice")
		w.Header().Set("Remote-Groups", "admins,dev")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(authz.Close)
	backend := identityBackend(t, "X-User", "X-Email", "X-Name", "X-Groups")
	ids := []IdentityHeader{{Header: "X-User", Field: IdentityUser}, {Header: "X-Email", Field: IdentityEmail}, {Header: "X-Name", Field: IdentityName}, {Header: "X-Groups", Field: IdentityGroups}}
	l := Listener{Sites: []Site{
		{Enable: true, Domains: []string{"fa.test"}, Targets: []string{backend.URL}, ForwardAuth: ForwardAuth{URL: authz.URL}, IdentityHeaders: ids},
		{Enable: true, Domains: []string{"user-only.test"}, Targets: []string{backend.URL},
			ForwardAuth: ForwardAuth{URL: authz.URL, CopyHeaders: []string{"Remote-User"}}, IdentityHeaders: ids},
	}}
	addr, _ := startRunner(t, l, nil, nil)
	cases := []struct {
		host, cookie, want string
	}{
		{"fa.test", "session=ok", "X-Email=alice@example.com|X-Groups=admins,dev|X-Name=Alice|X-User=alice"},
		{"fa.test", "session=nouser", ""},
		{"user-only.test", "session=ok", "X-User=alice"},
	}
	for _, c := range cases {
		hdr := forgedIdentity(map[string]string{"Cookie": c.cookie, "X-Email": "forged", "X_Groups": "forged", "X-Name": "forged", "Connection": "X-User, X-Email"})
		resp, body := get(t, noRedirect(), addr, c.host, "/", hdr)
		if resp.StatusCode != http.StatusOK || body != c.want {
			t.Fatalf("%s %s: 状态 %d，后端收到 %q，期望 %q", c.host, c.cookie, resp.StatusCode, body, c.want)
		}
	}
}

// TestValidateIdentityHeaders 覆盖身份请求头的整理、旧配置合并与校验。
func TestValidateIdentityHeaders(t *testing.T) {
	base := func() Site {
		return Site{Domains: []string{"a.test"}, Targets: []string{"http://127.0.0.1:1"}, WebAuth: true, SharedUsers: true,
			IdentityHeaders: []IdentityHeader{{Header: " remote-email ", Field: " email "}}}
	}
	ok := base()
	ok.UserHeader = "x-remote-user"
	if err := ok.validate(false); err != nil {
		t.Fatal(err)
	}
	want := []IdentityHeader{{Header: "X-Remote-User", Field: IdentityUser}, {Header: "Remote-Email", Field: IdentityEmail}}
	if ok.UserHeader != "" || !reflect.DeepEqual(ok.IdentityHeaders, want) {
		t.Fatalf("整理结果错误: %q %+v", ok.UserHeader, ok.IdentityHeaders)
	}
	// 旧版用户名请求头已作为用户名行在列表中时不重复添加
	dup := base()
	dup.UserHeader = "x_remote_user"
	dup.IdentityHeaders = append(dup.IdentityHeaders, IdentityHeader{Header: "X-Remote-User", Field: IdentityUser})
	if err := dup.validate(false); err != nil || len(dup.IdentityHeaders) != 2 {
		t.Fatalf("不应重复添加: %v %+v", err, dup.IdentityHeaders)
	}
	// 只传用户名时站点用户即可，邮箱等需要共享用户或转发认证
	siteOnly := Site{Domains: []string{"a.test"}, Targets: []string{"http://127.0.0.1:1"}, BasicAuth: true, Users: []string{"a:b"},
		IdentityHeaders: []IdentityHeader{{Header: "X-User", Field: IdentityUser}}}
	if err := siteOnly.validate(false); err != nil {
		t.Fatal(err)
	}
	fwd := Site{Domains: []string{"a.test"}, Targets: []string{"http://127.0.0.1:1"}, ForwardAuth: ForwardAuth{URL: "http://127.0.0.1:9091/"},
		IdentityHeaders: []IdentityHeader{{Header: "X-Groups", Field: IdentityGroups}}}
	if err := fwd.validate(false); err != nil {
		t.Fatalf("转发认证应可传递分组: %v", err)
	}
	empty := base()
	empty.IdentityHeaders = []IdentityHeader{}
	if err := empty.validate(false); err != nil || empty.IdentityHeaders != nil {
		t.Fatalf("空列表应整理为 nil: %v %+v", err, empty.IdentityHeaders)
	}
	add := func(s *Site, h, f string) {
		s.IdentityHeaders = append(s.IdentityHeaders, IdentityHeader{Header: h, Field: f})
	}
	bad := map[string]func(s *Site){
		"未启用认证":      func(s *Site) { s.WebAuth, s.SharedUsers = false, false },
		"非反向代理":      func(s *Site) { s.Type, s.Targets, s.Root = TypeStatic, nil, "/tmp" },
		"只有站点用户时传邮箱": func(s *Site) { s.SharedUsers, s.Users = false, []string{"a:b"} },
		"只有站点用户时传分组": func(s *Site) {
			s.SharedUsers, s.Users, s.IdentityHeaders = false, []string{"a:b"}, nil
			add(s, "X-G", IdentityGroups)
		},
		"缺少请求头名":      func(s *Site) { add(s, " ", IdentityUser) },
		"非法名称":        func(s *Site) { add(s, "Remote User", IdentityUser) },
		"禁止的头":        func(s *Site) { add(s, "authorization", IdentityUser) },
		"禁止的头的下划线写法":  func(s *Site) { add(s, "X_Forwarded_For", IdentityUser) },
		"未知字段":        func(s *Site) { add(s, "X-Role", "role") },
		"缺少字段":        func(s *Site) { add(s, "X-Role", "") },
		"重复请求头":       func(s *Site) { add(s, "REMOTE-EMAIL", IdentityName) },
		"重复请求头的下划线写法": func(s *Site) { add(s, "Remote_Email", IdentityName) },
		"与旧版用户名请求头冲突": func(s *Site) { s.UserHeader = "X-User"; add(s, "x_user", IdentityName) },
		"与站点请求头设置冲突":  func(s *Site) { s.RequestHeaders = []string{"Remote-Email: admin@example.com"} },
		"与站点删除请求头冲突":  func(s *Site) { s.RequestHeaders = []string{"remote_email:"} },
		"与分路径规则请求头冲突": func(s *Site) {
			s.Locations = []Location{{Prefix: "/api/", RequestHeaders: []string{"Remote-Email: x"}}}
		},
		"超过条数上限": func(s *Site) {
			for i := 0; i < maxIdentityHeaders; i++ {
				add(s, "X-Id-"+strconv.Itoa(i), IdentityUser)
			}
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

// TestIdentityHeaderCacheTagAndValues 覆盖缓存键按传出的值区分且不会被含分隔符的值拼成相同的标记，
// 非法值不写入请求头，未识别出身份时不传任何信息，以及未重新保存的旧配置在运行时仍传递并删除用户名头。
func TestIdentityHeaderCacheTagAndValues(t *testing.T) {
	h := &siteHandler{site: Site{IdentityHeaders: []IdentityHeader{{Header: "Remote-User", Field: IdentityUser}, {Header: "Remote-Email", Field: IdentityEmail}, {Header: "Remote-Groups", Field: IdentityGroups}}}}
	do := func(v visitor) *http.Request {
		return h.withIdentity(httptest.NewRequest("GET", "http://a.test/x", nil), v)
	}
	bob := visitor{names: []string{"bob"}}
	sharedBob := visitor{names: []string{"bob"}, email: "bob@example.com"}
	if cacheKey(do(bob)) == cacheKey(do(sharedBob)) {
		t.Fatal("邮箱不同的同名用户缓存键应不同")
	}
	// 值里含有标记的分隔符时也不能与其他身份拼出相同的标记
	a := visitor{names: []string{"a"}, email: "x|2:y"}
	b := visitor{names: []string{"a"}, email: "x", groups: "y"}
	if cacheKey(do(a)) == cacheKey(do(b)) {
		t.Fatal("含分隔符的值不应与其他身份的缓存键相同")
	}
	r := do(visitor{names: []string{"bob"}, email: "bob@example.com\r\nX-Admin: 1", groups: "g1,g2"})
	if _, ok := r.Header["Remote-Email"]; ok || r.Header.Get("X-Admin") != "" || r.Header.Get("Remote-Groups") != "g1,g2" || r.Header.Get("Remote-User") != "bob" {
		t.Fatalf("非法值不应写入请求头，其他值照常: %v", r.Header)
	}
	if r := do(visitor{email: "x@example.com"}); len(r.Header) != 0 {
		t.Fatalf("未识别出用户时不应传身份信息: %v", r.Header)
	}
	// 旧配置只有 UserHeader 且未经校验（加载配置不校验）时仍生效
	legacy := &siteHandler{site: Site{UserHeader: "X-Remote-User"}}
	if !legacy.usesIdentity() {
		t.Fatal("旧版用户名请求头应启用身份识别")
	}
	req := httptest.NewRequest("GET", "http://a.test/x", nil)
	req.Header["X_remote_user"] = []string{"mallory"}
	legacy.stripIdentityHeaders(req)
	req = legacy.withIdentity(req, bob)
	if len(req.Header) != 1 || req.Header.Get("X-Remote-User") != "bob" {
		t.Fatalf("旧版用户名请求头处理错误: %v", req.Header)
	}
}
