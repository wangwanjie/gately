package webservice

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 按用户分流用于非反向代理站点（#79）：站点类型即未命中规则时的默认处理方式，命中规则的用户反向代理到规则的后端。

// nonProxySite 返回启用基本认证、admin 分流到 backend 的指定类型站点，/public/ 为公开路径。
func nonProxySite(typ, backend string, mod func(s *Site)) Site {
	s := Site{
		Enable: true, Domains: []string{"np.test"}, Type: typ,
		BasicAuth: true, Users: []string{"admin:pw", "bob:pw"},
		AuthOptional: true, Rules: []PathRule{{Prefix: "/", Auth: RuleAuthOn}, {Prefix: "/public/", Auth: RuleAuthOff}},
		UserRoutes:      []UserRoute{{Users: []string{"admin"}, Targets: []string{backend}}},
		IdentityHeaders: []IdentityHeader{{Header: "X-Remote-User", Field: IdentityUser}},
	}
	if mod != nil {
		mod(&s)
	}
	return s
}

// TestUserRoutesNonProxyTypes 覆盖重定向、静态文件、固定文本站点上的按用户分流：
// 命中规则的用户到达后端并带上身份请求头，未命中的用户与公开路径上的未登录访客得到站点类型本身的响应，认证失败仍为 401。
func TestUserRoutesNonProxyTypes(t *testing.T) {
	admin := userBackend(t, "admin")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("static-home"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "public"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "public", "index.html"), []byte("static-public"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		site Site
		// check 检查未命中规则的访客得到的响应，path 为访问路径
		check func(t *testing.T, resp *http.Response, body, path string)
	}{
		{
			name: "重定向",
			site: nonProxySite(TypeRedirect, admin.URL, func(s *Site) { s.RedirectTo, s.RedirectCode = "https://example.com", 307 }),
			check: func(t *testing.T, resp *http.Response, body, path string) {
				if resp.StatusCode != http.StatusTemporaryRedirect || !strings.HasPrefix(resp.Header.Get("Location"), "https://example.com") {
					t.Fatalf("%s 应跳转，实际 %d %q", path, resp.StatusCode, resp.Header.Get("Location"))
				}
			},
		},
		{
			name: "静态文件",
			site: nonProxySite(TypeStatic, admin.URL, func(s *Site) { s.Root = root }),
			check: func(t *testing.T, resp *http.Response, body, path string) {
				if resp.StatusCode != http.StatusOK || !strings.HasPrefix(body, "static-") {
					t.Fatalf("%s 应返回静态文件，实际 %d %q", path, resp.StatusCode, body)
				}
			},
		},
		{
			name: "固定文本",
			site: nonProxySite(TypeText, admin.URL, func(s *Site) { s.Text, s.StatusCode = "no access", http.StatusForbidden }),
			check: func(t *testing.T, resp *http.Response, body, path string) {
				if resp.StatusCode != http.StatusForbidden || body != "no access" {
					t.Fatalf("%s 应返回固定文本，实际 %d %q", path, resp.StatusCode, body)
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			addr, _ := startRunner(t, Listener{Sites: []Site{c.site}}, nil, nil)
			// 命中规则的用户到达后端，伪造的身份头被替换为认证得到的用户名
			hdr := basicHeader("admin", "pw")
			hdr["X-Remote-User"] = "mallory"
			if resp, body := get(t, noRedirect(), addr, "np.test", "/", hdr); resp.StatusCode != http.StatusOK || body != "admin|admin" {
				t.Fatalf("admin 应分到后端，实际 %d %q", resp.StatusCode, body)
			}
			// 未命中规则的已登录用户按站点类型处理
			resp, body := get(t, noRedirect(), addr, "np.test", "/", basicHeader("bob", "pw"))
			c.check(t, resp, body, "/")
			// 公开路径上的未登录访客按站点类型处理
			resp, body = get(t, noRedirect(), addr, "np.test", "/public/", map[string]string{"X-Remote-User": "admin"})
			c.check(t, resp, body, "/public/")
			// 未登录或密码错误仍需认证，不会落到站点类型的响应或后端
			for _, h := range []map[string]string{nil, basicHeader("admin", "bad")} {
				if resp, _ := get(t, noRedirect(), addr, "np.test", "/", h); resp.StatusCode != http.StatusUnauthorized {
					t.Fatalf("未通过认证应 401，实际 %d", resp.StatusCode)
				}
			}
		})
	}
}

// TestUserRoutesNonProxyBackendStatus 覆盖非反向代理站点的分流后端出现在后端状态中，站点自身没有后端组。
func TestUserRoutesNonProxyBackendStatus(t *testing.T) {
	admin := userBackend(t, "admin")
	s := nonProxySite(TypeText, admin.URL, func(s *Site) { s.Text = "x" })
	if err := s.validate(false); err != nil {
		t.Fatal(err)
	}
	h, err := newSiteHandler(s, nil, newAuthenticator("secret"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if h.pool != nil || len(h.pools()) != 1 || h.cache != nil {
		t.Fatalf("应只有一组分流后端且无缓存: pool=%v pools=%d cache=%v", h.pool, len(h.pools()), h.cache)
	}
	st := h.upstreamStatus()
	if len(st) != 1 || st[0].URL != admin.URL {
		t.Fatalf("后端状态错误: %+v", st)
	}
	// 拓扑中未运行站点的配置后端只列出分流后端，不把隐藏的站点后端算进去
	s.Targets = []string{"http://127.0.0.1:9"}
	ups := configUpstreams(s)
	if len(ups) != 1 || ups[0].Scope != scopeUser || ups[0].Match != "admin" {
		t.Fatalf("拓扑后端错误: %+v", ups)
	}
	s.UserRoutes = nil
	if ups := configUpstreams(s); len(ups) != 0 {
		t.Fatalf("没有分流规则的非反向代理站点不应有后端: %+v", ups)
	}
}

// TestValidateUserRoutesNonProxy 覆盖非反向代理站点保存按用户分流与身份请求头的校验。
func TestValidateUserRoutesNonProxy(t *testing.T) {
	base := func(typ string) Site {
		s := Site{Domains: []string{"a.test"}, Type: typ, WebAuth: true, Users: []string{"a:b"},
			UserRoutes:      []UserRoute{{Users: []string{"a"}, Targets: []string{"http://127.0.0.1:2"}}},
			IdentityHeaders: []IdentityHeader{{Header: "Remote-User", Field: IdentityUser}}}
		switch typ {
		case TypeRedirect:
			s.RedirectTo = "https://example.com"
		case TypeStatic:
			s.Root = "/tmp"
		case TypePanel:
			s.Panel = "p1"
		case TypeText:
			s.Text = "x"
		}
		return s
	}
	for _, typ := range []string{TypeRedirect, TypeStatic, TypePanel, TypeText} {
		s := base(typ)
		if err := s.validate(false); err != nil {
			t.Fatalf("%s 站点按用户分流应可保存: %v", typ, err)
		}
		// 没有分流规则时身份信息无处可传
		s = base(typ)
		s.UserRoutes = nil
		if err := s.validate(false); err == nil {
			t.Fatalf("%s 站点没有分流规则时不应能传递身份信息", typ)
		}
		// 分流仍需先启用认证
		s = base(typ)
		s.WebAuth = false
		if err := s.validate(false); err == nil {
			t.Fatalf("%s 站点未启用认证时不应能按用户分流", typ)
		}
		// 分流规则的后端仍需合法
		s = base(typ)
		s.UserRoutes[0].Targets = []string{"ftp://x"}
		if err := s.validate(false); err == nil {
			t.Fatalf("%s 站点分流规则后端非法时应校验失败", typ)
		}
	}
	// 分路径后端仍只用于反向代理站点
	s := base(TypeStatic)
	s.Locations = []Location{{Prefix: "/api/", Targets: []string{"http://127.0.0.1:3"}}}
	if err := s.validate(false); err == nil {
		t.Fatal("非反向代理站点不应能配置分路径后端")
	}
}

// TestUserRoutesCloseSiteIgnored 覆盖断开连接站点不经过认证：即使配置里残留分流规则，也直接断开，不会把请求转给后端。
func TestUserRoutesCloseSiteIgnored(t *testing.T) {
	admin := userBackend(t, "admin")
	s := nonProxySite(TypeClose, admin.URL, nil)
	addr, _ := startRunner(t, Listener{Sites: []Site{s}}, nil, nil)
	req, _ := http.NewRequest("GET", "http://"+addr+"/", nil)
	req.Host = "np.test"
	req.SetBasicAuth("admin", "pw")
	if resp, err := http.DefaultClient.Do(req); err == nil {
		resp.Body.Close()
		t.Fatalf("断开连接站点应直接断开，实际 %d", resp.StatusCode)
	}
}
