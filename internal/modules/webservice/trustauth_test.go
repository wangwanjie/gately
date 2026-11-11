package webservice

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/webprobe"
)

// loginCookie 用网页认证登录站点，返回写出的会话 Cookie（"名称=值"）。
func loginCookie(t *testing.T, addr, host, user, pass string) string {
	t.Helper()
	form := url.Values{"username": {user}, "password": {pass}, "next": {"/"}}
	req, _ := http.NewRequest("POST", "http://"+addr+authPath+"/login", strings.NewReader(form.Encode()))
	req.Host = host
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := noRedirect().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	for _, c := range resp.Cookies() {
		if strings.HasPrefix(c.Name, "gately_site_") || c.Name == ssoCookie {
			return c.Name + "=" + c.Value
		}
	}
	t.Fatalf("登录 %s 没有写出会话 Cookie: %d", host, resp.StatusCode)
	return ""
}

// 开启信任已认证用户的站点：带有效会话 Cookie 的请求不经过 WAF；未登录、伪造会话、
// 别的站点的会话、未开启选项的站点仍被 WAF 拦截；已封禁的地址即使带会话也被拒绝。
func TestTrustAuthUsersSkipsWAFWithSession(t *testing.T) {
	browser := map[string]string{"User-Agent": "Mozilla/5.0", "Accept": "text/html"}
	l := Listener{Sites: []Site{
		{Enable: true, Key: "on", Domains: []string{"on.test"}, Type: TypeText, Text: "ok", WebAuth: true, Users: []string{"alice:pw"}, TrustAuthUsers: true, WAF: "w"},
		{Enable: true, Key: "off", Domains: []string{"off.test"}, Type: TypeText, Text: "ok", WebAuth: true, Users: []string{"alice:pw"}, WAF: "w"},
		{Enable: true, Key: "other", Domains: []string{"other.test"}, Type: TypeText, Text: "ok", WebAuth: true, Users: []string{"alice:pw"}, TrustAuthUsers: true, WAF: "w"},
	}}
	addr, r := startRunnerWAF(t, l, blockingWAF{})
	r.auth.bans = banlist.New()

	on := loginCookie(t, addr, "on.test", "alice", "pw")
	off := loginCookie(t, addr, "off.test", "alice", "pw")
	with := func(cookie string) map[string]string {
		h := map[string]string{"Cookie": cookie}
		for k, v := range browser {
			h[k] = v
		}
		return h
	}
	if resp, body := get(t, noRedirect(), addr, "on.test", "/attack", with(on)); resp.StatusCode != http.StatusOK || body != "ok" {
		t.Fatalf("已登录用户应跳过 WAF: %d %q", resp.StatusCode, body)
	}
	if resp, _ := get(t, noRedirect(), addr, "on.test", "/attack", browser); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("未登录的请求应被 WAF 拦截: %d", resp.StatusCode)
	}
	// 篡改签名的会话不算登录
	forged := on[:len(on)-2] + "xx"
	if resp, _ := get(t, noRedirect(), addr, "on.test", "/attack", with(forged)); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("伪造的会话应被 WAF 拦截: %d", resp.StatusCode)
	}
	// 站点会话按站点隔离：on.test 的会话换成 other.test 的 Cookie 名也不算本站点的认证
	_, value, _ := strings.Cut(on, "=")
	otherName := cookieName(Site{scope: l.Key + "/other"})
	if resp, _ := get(t, noRedirect(), addr, "other.test", "/attack", with(otherName+"="+value)); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("别的站点的会话应被 WAF 拦截: %d", resp.StatusCode)
	}
	// 未开启选项的站点：登录用户照常经过 WAF
	if resp, _ := get(t, noRedirect(), addr, "off.test", "/attack", with(off)); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("未开启信任的站点应照常检查: %d", resp.StatusCode)
	}
	// 封禁优先于信任：被封禁的地址带会话也被拒绝
	if _, _, err := r.auth.bans.Ban("127.0.0.1", banlist.Options{Duration: time.Minute, Force: true}); err != nil {
		t.Fatal(err)
	}
	if resp, _ := get(t, noRedirect(), addr, "on.test", "/", with(on)); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("被封禁的地址应被拒绝: %d", resp.StatusCode)
	}
}

// 基本认证：首个带凭据的请求照常经过 WAF，验证通过后同一凭据的请求跳过 WAF；
// 错误密码、修改密码后的旧凭据、删除的用户都不再信任。
func TestTrustAuthUsersBasicAuth(t *testing.T) {
	l := Listener{Sites: []Site{
		{Enable: true, Key: "b", Domains: []string{"b.test"}, Type: TypeText, Text: "ok", BasicAuth: true, Users: []string{"bob:pw"}, TrustAuthUsers: true, WAF: "w"},
	}}
	addr, _ := startRunnerWAF(t, l, blockingWAF{})
	good := basicHeader("bob", "pw")
	if resp, _ := get(t, noRedirect(), addr, "b.test", "/attack", good); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("尚未验证过的凭据应先经过 WAF: %d", resp.StatusCode)
	}
	if resp, body := get(t, noRedirect(), addr, "b.test", "/", good); resp.StatusCode != http.StatusOK || body != "ok" {
		t.Fatalf("基本认证应通过: %d %q", resp.StatusCode, body)
	}
	if resp, body := get(t, noRedirect(), addr, "b.test", "/attack", good); resp.StatusCode != http.StatusOK || body != "ok" {
		t.Fatalf("验证过的凭据应跳过 WAF: %d %q", resp.StatusCode, body)
	}
	bad := basicHeader("bob", "wrong")
	if resp, _ := get(t, noRedirect(), addr, "b.test", "/attack", bad); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("错误密码应被 WAF 拦截: %d", resp.StatusCode)
	}
}

// 基本认证凭据缓存随用户凭据变化失效，并按站点作用域隔离；共享用户启用 TOTP 后不再信任。
func TestTrustedBasicInvalidation(t *testing.T) {
	a := newAuthenticator("secret")
	site := Site{Key: "s", scope: "l/s", BasicAuth: true, Users: []string{"bob:pw"}, TrustAuthUsers: true, SharedUsers: true}
	req := func(user, pass string) *http.Request {
		r, _ := http.NewRequest("GET", "http://x/", nil)
		r.SetBasicAuth(user, pass)
		return r
	}
	a.rememberBasic(site, "bob", "pw", false)
	if !a.trustedBasic(req("bob", "pw"), site) {
		t.Fatal("验证过的凭据应被信任")
	}
	if a.trustedBasic(req("bob", "other"), site) {
		t.Fatal("别的密码不应被信任")
	}
	changed := site
	changed.Users = []string{"bob:new"}
	if a.trustedBasic(req("bob", "pw"), changed) {
		t.Fatal("修改密码后旧凭据不应被信任")
	}
	removed := site
	removed.Users = nil
	if a.trustedBasic(req("bob", "pw"), removed) {
		t.Fatal("删除用户后旧凭据不应被信任")
	}
	other := site
	other.scope = "l/other"
	if a.trustedBasic(req("bob", "pw"), other) {
		t.Fatal("别的站点验证过的凭据不应被信任")
	}
	// 未开启信任的站点不记录凭据
	plain := site
	plain.TrustAuthUsers = false
	plain.scope = "l/plain"
	a.rememberBasic(plain, "bob", "pw", false)
	if a.trustedBasic(req("bob", "pw"), plain) {
		t.Fatal("未开启信任的站点不应记录凭据")
	}

	// 共享用户：TOTP 启用后基本认证本身就不能登录，缓存同样失效
	users, err := normalizeAuthUsers([]AuthUser{{Name: "carol", Password: "pw"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	a.shared.set(users, "")
	a.rememberBasic(site, "carol", "pw", true)
	if !a.trustedBasic(req("carol", "pw"), site) {
		t.Fatal("共享用户验证过的凭据应被信任")
	}
	withTOTP := []AuthUser{users[0]}
	withTOTP[0].TOTPSecret = "JBSWY3DPEHPK3PXP"
	a.shared.set(withTOTP, "")
	if a.trustedBasic(req("carol", "pw"), site) {
		t.Fatal("共享用户启用 TOTP 后不应再信任基本认证凭据")
	}
	noShared := site
	noShared.SharedUsers = false
	a.shared.set(users, "")
	if a.trustedBasic(req("carol", "pw"), noShared) {
		t.Fatal("站点关闭共享用户后不应信任共享用户凭据")
	}
}

// 缓存只增长到上限，满了先清过期项，仍满则整体清空。
func TestTrustedBasicCacheBound(t *testing.T) {
	a := newAuthenticator("secret")
	site := Site{Key: "s", scope: "l/s", BasicAuth: true, TrustAuthUsers: true}
	for i := 0; i < trustBasicMax+10; i++ {
		u := fmt.Sprintf("u%d", i)
		site.Users = []string{u + ":pw"}
		a.rememberBasic(site, u, "pw", false)
	}
	if n := len(a.trustBasic.m); n == 0 || n > trustBasicMax {
		t.Fatalf("缓存条数应在上限内: %d", n)
	}
}

// 已认证用户的请求不计入路径探测（网页会话与首次基本认证都算）；未登录、未开启选项的站点、
// 默认站点（未知域名）照常上报。
func TestTrustAuthUsersSkipsWebProbe(t *testing.T) {
	var mu sync.Mutex
	var hits []webprobe.Hit
	webprobe.SetHandler(func(h webprobe.Hit) {
		mu.Lock()
		hits = append(hits, h)
		mu.Unlock()
	})
	t.Cleanup(func() { webprobe.SetHandler(nil) })
	// paths 等到至少 n 条上报后返回全部上报的 域名+路径；站点在响应写出后的 defer 中上报，客户端收到响应时可能还没上报
	paths := func(n int) []string {
		t.Helper()
		for i := 0; i < 200; i++ {
			mu.Lock()
			if len(hits) >= n {
				out := make([]string, 0, len(hits))
				for _, h := range hits {
					out = append(out, h.Host+h.Path)
				}
				mu.Unlock()
				return out
			}
			mu.Unlock()
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("没有收到 %d 条上报", n)
		return nil
	}

	l := Listener{
		Sites: []Site{
			{Enable: true, Key: "on", Domains: []string{"on.test"}, Type: TypeText, Text: "nope", StatusCode: 404, WebAuth: true, Users: []string{"alice:pw"}, TrustAuthUsers: true},
			{Enable: true, Key: "off", Domains: []string{"off.test"}, Type: TypeText, Text: "nope", StatusCode: 404, WebAuth: true, Users: []string{"alice:pw"}},
		},
		Default: Site{Key: "default", Enable: true, Type: TypeText, Text: "nope", StatusCode: 404, BasicAuth: true, Users: []string{"alice:pw"}, TrustAuthUsers: true},
	}
	addr, _ := startRunner(t, l, nil, nil)
	on := loginCookie(t, addr, "on.test", "alice", "pw")
	off := loginCookie(t, addr, "off.test", "alice", "pw")
	// 登录请求本身会上报（POST 登录页），记下基数
	base := len(paths(2))

	get(t, noRedirect(), addr, "on.test", "/.env", map[string]string{"Cookie": on})
	// 首个基本认证请求：WAF 之前尚未缓存，但经 Gately 认证通过，同样不计入
	get(t, noRedirect(), addr, "on.test", "/basic-first", basicHeader("alice", "pw"))
	get(t, noRedirect(), addr, "on.test", "/anon", nil)
	get(t, noRedirect(), addr, "off.test", "/off", map[string]string{"Cookie": off})
	get(t, noRedirect(), addr, "unknown.test", "/unknown", basicHeader("alice", "pw"))
	got := strings.Join(paths(base + 3)[base:], ",")
	if got != "on.test/anon,off.test/off,unknown.test/unknown" {
		t.Fatalf("上报不符: %s", got)
	}
}

// 没有启用基本认证或网页认证时保存会关闭信任已认证用户（包括只开了转发认证）。
func TestTrustAuthUsersRequiresGatelyAuth(t *testing.T) {
	s := Site{TrustAuthUsers: true, ForwardAuth: ForwardAuth{URL: "http://auth.test/verify"}}
	if err := s.validateAccess(); err != nil {
		t.Fatal(err)
	}
	if s.TrustAuthUsers {
		t.Fatal("没有 Gately 认证时应关闭信任已认证用户")
	}
	s = Site{TrustAuthUsers: true, WebAuth: true, Users: []string{"a:b"}}
	if err := s.validateAccess(); err != nil || !s.TrustAuthUsers {
		t.Fatalf("启用网页认证时应保留: %v %v", err, s.TrustAuthUsers)
	}
}

// 第三方登录：允许列表逐个列出的身份跳过 WAF；只靠 提供方:* 通配进来的身份可以登录站点，但不被信任，
// 否则任何人注册一个账号就能对该站点关闭 WAF。
func TestTrustAuthUsersWildcardThirdParty(t *testing.T) {
	browser := map[string]string{"User-Agent": "Mozilla/5.0", "Accept": "text/html"}
	l := Listener{Sites: []Site{
		{Enable: true, Key: "exact", Domains: []string{"exact.test"}, Type: TypeText, Text: "ok", WebAuth: true, OAuthUsers: []string{"github:12345 #alice"}, TrustAuthUsers: true, WAF: "w"},
		{Enable: true, Key: "any", Domains: []string{"any.test"}, Type: TypeText, Text: "ok", WebAuth: true, OAuthUsers: []string{"github:*", "google:bob@example.com"}, TrustAuthUsers: true, WAF: "w"},
	}}
	addr, r := startRunnerWAF(t, l, blockingWAF{})
	session := func(key, host, user string, aliases ...string) map[string]string {
		v := r.auth.signBlob("session", siteSession{User: user, Host: host, Scope: l.Key + "/" + key, Aliases: aliases}, time.Hour)
		h := map[string]string{"Cookie": cookieName(Site{scope: l.Key + "/" + key}) + "=" + v}
		for k, val := range browser {
			h[k] = val
		}
		return h
	}
	cases := []struct {
		name, host string
		hdr        map[string]string
		want       int
	}{
		{"逐个列出的身份", "exact.test", session("exact", "exact.test", "github:12345"), http.StatusOK},
		{"别名匹配的身份", "exact.test", session("exact", "exact.test", "github:999", "12345"), http.StatusOK},
		{"通配允许的身份", "any.test", session("any", "any.test", "github:777"), http.StatusForbidden},
		{"通配站点里逐个列出的身份", "any.test", session("any", "any.test", "google:bob@example.com"), http.StatusOK},
		{"不在允许列表的身份", "exact.test", session("exact", "exact.test", "github:777"), http.StatusForbidden},
	}
	for _, c := range cases {
		if resp, _ := get(t, noRedirect(), addr, c.host, "/attack", c.hdr); resp.StatusCode != c.want {
			t.Errorf("%s: 状态码 %d，期望 %d", c.name, resp.StatusCode, c.want)
		}
	}
	// 通配身份仍可正常访问站点（只是要经过 WAF）
	if resp, body := get(t, noRedirect(), addr, "any.test", "/", session("any", "any.test", "github:777")); resp.StatusCode != http.StatusOK || body != "ok" {
		t.Fatalf("通配身份应能登录访问: %d %q", resp.StatusCode, body)
	}
}

// 只靠 提供方:* 通配进来的第三方身份认证通过后仍计入路径探测，也不给外层 Gately 签发认证标记；
// 逐个列出的身份不计入并签发标记。
func TestTrustAuthUsersWildcardProbeAndMarker(t *testing.T) {
	var mu sync.Mutex
	var hits []string
	webprobe.SetHandler(func(h webprobe.Hit) {
		mu.Lock()
		hits = append(hits, h.Path)
		mu.Unlock()
	})
	t.Cleanup(func() { webprobe.SetHandler(nil) })
	l := Listener{Key: "w", Sites: []Site{
		{Enable: true, Key: "any", Domains: []string{"any.test"}, Type: TypeText, Text: "nope", StatusCode: 404, WebAuth: true,
			OAuthUsers: []string{"github:*", "google:bob@example.com"}, TrustAuthUsers: true},
	}}
	addr, r := startRunner(t, l, nil, nil)
	r.auth.marker.set(testMarkerKey)
	session := func(user string) map[string]string {
		v := r.auth.signBlob("session", siteSession{User: user, Host: "any.test", Scope: "w/any"}, time.Hour)
		return map[string]string{"Cookie": cookieName(Site{scope: "w/any"}) + "=" + v, "User-Agent": "Mozilla/5.0", "Accept": "text/html"}
	}
	wild, _ := get(t, noRedirect(), addr, "any.test", "/wild", session("github:777"))
	listed, _ := get(t, noRedirect(), addr, "any.test", "/listed", session("google:bob@example.com"))
	if wild.StatusCode != http.StatusNotFound || listed.StatusCode != http.StatusNotFound {
		t.Fatalf("两种身份都应通过认证: %d %d", wild.StatusCode, listed.StatusCode)
	}
	if wild.Header.Get(authMarkerHeader) != "" || listed.Header.Get(authMarkerHeader) == "" {
		t.Fatalf("只应给逐个列出的身份签发标记: %q %q", wild.Header.Get(authMarkerHeader), listed.Header.Get(authMarkerHeader))
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		got := strings.Join(hits, ",")
		mu.Unlock()
		if got == "/wild" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("应只上报通配身份的请求: %s", got)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
