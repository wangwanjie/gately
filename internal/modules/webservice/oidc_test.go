package webservice

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/thirdauth"
)

const (
	oidcTestSecret   = "gitea-secret-0123456789"
	oidcTestRedirect = "https://git.test/user/oauth2/gately/callback"
	oidcTestSPA      = "https://spa.test/cb"
)

// oidcFixture 启动一个带共享用户与 OIDC 身份提供方的监听器，issuer 为签发者域名（请求通过 Host 头指向它）。
type oidcFixture struct {
	t      *testing.T
	addr   string
	r      *listenerRunner
	c      *http.Client
	issuer string
	host   string
	set    OIDCSettings
	users  []AuthUser
	// carolTOTP 为启用 TOTP 的用户 carol 的密钥
	carolTOTP string
}

// newOIDCFixture 创建测试环境；issuerHost 为空时签发者地址使用监听器的真实地址（供真实 HTTP 客户端访问）。
func newOIDCFixture(t *testing.T, issuerHost, ssoDomain string) *oidcFixture {
	t.Helper()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("backend:" + r.URL.Path))
	}))
	t.Cleanup(backend.Close)
	l := Listener{Sites: []Site{
		{Enable: true, Key: "app", Domains: []string{"app.test"}, Targets: []string{backend.URL}},
		{Enable: true, Key: "sso", Domains: []string{"a.sso.test"}, Targets: []string{backend.URL}, WebAuth: true, SharedUsers: true},
	}}
	addr, r := startRunner(t, l, nil, nil)
	f := &oidcFixture{t: t, addr: addr, r: r, c: noRedirect(), carolTOTP: auth.NewTOTPSecret()}
	users, err := normalizeAuthUsers([]AuthUser{
		{Name: "alice", Password: "alice-pw", Email: "alice@example.com", DisplayName: "Alice Liddell", Groups: []string{"admins", "dev"}},
		{Name: "bob", Password: "bob-pw"},
		{Name: "carol", Password: "carol-pw", TOTPSecret: f.carolTOTP, Groups: []string{"admins"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	f.users = users
	r.auth.shared.set(users, ssoList(ssoDomain))
	f.host = issuerHost
	if issuerHost == "" {
		f.host = addr
	}
	f.issuer = "http://" + f.host
	f.set = OIDCSettings{Enable: true, Issuer: f.issuer, Clients: []OIDCClient{
		{ID: "gitea", Name: "Gitea", Secret: oidcTestSecret, RedirectURIs: []string{oidcTestRedirect}},
		{ID: "spa", Name: "SPA", Public: true, RedirectURIs: []string{oidcTestSPA}},
		{ID: "admin-only", Secret: oidcTestSecret, RedirectURIs: []string{oidcTestRedirect}, Allow: []string{"group:admins", "bob"}},
		{ID: "dev-only", Secret: oidcTestSecret, RedirectURIs: []string{oidcTestRedirect}, Allow: []string{"group:dev"}},
	}}
	if err := normalizeOIDC(&f.set, OIDCSettings{}); err != nil {
		t.Fatal(err)
	}
	if err := r.auth.oidc.set(f.set); err != nil {
		t.Fatal(err)
	}
	return f
}

// do 向签发者域名发送请求（不跟随跳转）。
func (f *oidcFixture) do(method, path string, body url.Values, hdr map[string]string, cookies ...*http.Cookie) (*http.Response, string) {
	f.t.Helper()
	var rd io.Reader
	if body != nil {
		rd = strings.NewReader(body.Encode())
	}
	req, _ := http.NewRequest(method, "http://"+f.addr+path, rd)
	req.Host = f.host
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := f.c.Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(b)
}

// login 在签发者的登录页登录，返回会话 Cookie。
func (f *oidcFixture) login(user, pass string) *http.Cookie {
	f.t.Helper()
	resp, _ := f.do("POST", oidcLoginPath, url.Values{"username": {user}, "password": {pass}, "next": {"/"}}, nil)
	if resp.StatusCode != http.StatusFound {
		f.t.Fatalf("登录 %s 失败: %d", user, resp.StatusCode)
	}
	for _, c := range resp.Cookies() {
		if c.Value != "" {
			return c
		}
	}
	f.t.Fatal("登录后没有会话 Cookie")
	return nil
}

func authorizeQuery(client, redirect string, extra ...string) url.Values {
	v := url.Values{"response_type": {"code"}, "client_id": {client}, "redirect_uri": {redirect}, "scope": {"openid profile email groups"}, "state": {"st-1"}, "nonce": {"n-1"}}
	for i := 0; i+1 < len(extra); i += 2 {
		v.Set(extra[i], extra[i+1])
	}
	return v
}

// authorize 发起授权请求，返回跳转地址。
func (f *oidcFixture) authorize(q url.Values, cookies ...*http.Cookie) (*http.Response, *url.URL) {
	f.t.Helper()
	resp, _ := f.do("GET", oidcAuthorizePath+"?"+q.Encode(), nil, map[string]string{"User-Agent": "Mozilla/5.0", "Accept": "text/html"}, cookies...)
	loc, _ := url.Parse(resp.Header.Get("Location"))
	return resp, loc
}

// code 发起授权请求并取出授权码。
func (f *oidcFixture) code(q url.Values, cookie *http.Cookie) string {
	f.t.Helper()
	resp, loc := f.authorize(q, cookie)
	if resp.StatusCode != http.StatusFound || loc.Query().Get("code") == "" {
		f.t.Fatalf("授权应返回授权码: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	if loc.Query().Get("state") != q.Get("state") || loc.Query().Get("iss") != f.issuer {
		f.t.Fatalf("回调应带回 state 与 iss: %s", loc)
	}
	return loc.Query().Get("code")
}

// token 调用令牌接口；user 非空时使用 client_secret_basic。
func (f *oidcFixture) token(form url.Values, user, pass string) (int, map[string]any) {
	f.t.Helper()
	req, _ := http.NewRequest("POST", "http://"+f.addr+oidcTokenPath, strings.NewReader(form.Encode()))
	req.Host = f.host
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if user != "" {
		req.SetBasicAuth(url.QueryEscape(user), url.QueryEscape(pass))
	}
	resp, err := f.c.Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// verify 用 go-jose 按 JWKS 独立校验 JWT 签名并返回声明，确认产出的令牌能被标准库识别。
func (f *oidcFixture) verify(token string) map[string]any {
	f.t.Helper()
	_, body := f.do("GET", oidcJWKSPath, nil, nil)
	var set jose.JSONWebKeySet
	if err := json.Unmarshal([]byte(body), &set); err != nil || len(set.Keys) != 1 {
		f.t.Fatalf("JWKS 无效: %v %s", err, body)
	}
	sig, err := jose.ParseSigned(token, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		f.t.Fatal(err)
	}
	if sig.Signatures[0].Header.KeyID != set.Keys[0].KeyID {
		f.t.Fatal("JWT 的 kid 与 JWKS 不一致")
	}
	payload, err := sig.Verify(set.Keys[0].Key)
	if err != nil {
		f.t.Fatalf("JWT 签名校验失败: %v", err)
	}
	var claims map[string]any
	_ = json.Unmarshal(payload, &claims)
	return claims
}

func pkcePair() (verifier, challenge string) {
	verifier = strings.Repeat("v3rifier-", 6)
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

func TestOIDCNormalize(t *testing.T) {
	ok := OIDCSettings{Enable: true, Issuer: "https://Auth.Example.com/", Clients: []OIDCClient{
		{ID: "gitea", Secret: oidcTestSecret, RedirectURIs: []string{oidcTestRedirect, " ", oidcTestRedirect}},
		{ID: "app", Public: true, Secret: "ignored", RedirectURIs: []string{"com.example.app:/cb"}},
	}}
	if err := normalizeOIDC(&ok, OIDCSettings{}); err != nil {
		t.Fatal(err)
	}
	if ok.Issuer != "https://auth.example.com" || ok.SigningKey == "" {
		t.Fatalf("签发者应规范化并生成签名密钥: %q", ok.Issuer)
	}
	if !strings.HasPrefix(ok.Clients[0].Secret, "$2") || ok.Clients[1].Secret != "" {
		t.Fatal("客户端密钥应哈希保存，公共客户端不保存密钥")
	}
	if len(ok.Clients[0].RedirectURIs) != 1 {
		t.Fatal("回调地址应去重并去掉空行")
	}
	// 读取时脱敏，提交占位符保留旧值，签名密钥沿用
	masked := maskOIDC(ok)
	if masked.SigningKey != "" || masked.Clients[0].Secret != crud.SecretMask || ok.Clients[0].Secret == crud.SecretMask {
		t.Fatal("接口输出应隐藏签名密钥与客户端密钥，且不影响原配置")
	}
	again := masked
	if err := normalizeOIDC(&again, ok); err != nil {
		t.Fatal(err)
	}
	if again.Clients[0].Secret != ok.Clients[0].Secret || again.SigningKey != ok.SigningKey {
		t.Fatal("提交脱敏占位符应保留原密钥")
	}
	// 未启用时不生成签名密钥，老配置保存后保持原样
	off := OIDCSettings{}
	if err := normalizeOIDC(&off, OIDCSettings{}); err != nil || off.SigningKey != "" {
		t.Fatalf("未启用时不应生成密钥: %v", err)
	}
	bad := []OIDCSettings{
		{Enable: true},
		{Issuer: "https://auth.example.com/oidc"},
		{Issuer: "ftp://auth.example.com"},
		{Issuer: "https://auth.example.com?x=1"},
		{Clients: []OIDCClient{{ID: "a b", Secret: oidcTestSecret, RedirectURIs: []string{oidcTestRedirect}}}},
		{Clients: []OIDCClient{{ID: "a", Secret: oidcTestSecret, RedirectURIs: []string{oidcTestRedirect}}, {ID: "a", Secret: oidcTestSecret, RedirectURIs: []string{oidcTestRedirect}}}},
		{Clients: []OIDCClient{{ID: "a", Secret: "short", RedirectURIs: []string{oidcTestRedirect}}}},
		{Clients: []OIDCClient{{ID: "a", RedirectURIs: []string{oidcTestRedirect}}}},
		{Clients: []OIDCClient{{ID: "a", Secret: oidcTestSecret}}},
		{Clients: []OIDCClient{{ID: "a", Secret: oidcTestSecret, RedirectURIs: []string{"/relative"}}}},
		{Clients: []OIDCClient{{ID: "a", Secret: oidcTestSecret, RedirectURIs: []string{"https://x.test/cb#frag"}}}},
		{Clients: []OIDCClient{{ID: "a", Secret: "$2a$bad", RedirectURIs: []string{oidcTestRedirect}}}},
		{Clients: []OIDCClient{{ID: "a", Secret: oidcTestSecret, RedirectURIs: []string{oidcTestRedirect}, Allow: []string{"group:"}}}},
	}
	for i, s := range bad {
		if err := normalizeOIDC(&s, OIDCSettings{}); err == nil {
			t.Errorf("第 %d 项无效配置应被拒绝", i)
		}
	}
}

func TestSharedUserProfileFields(t *testing.T) {
	users, err := normalizeAuthUsers([]AuthUser{{Name: "a", Password: "pw", Email: " a@x.test ", DisplayName: " A ", Groups: []string{"g1", " ", "g1", "g2"}}, {Name: "b", Password: "pw"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if users[0].Email != "a@x.test" || users[0].DisplayName != "A" || len(users[0].Groups) != 2 || users[1].Groups != nil {
		t.Fatalf("身份字段应规范化: %+v", users)
	}
	// 老配置没有这些字段，序列化后也不应多出空字段
	b, _ := json.Marshal(users[1])
	if strings.Contains(string(b), "email") || strings.Contains(string(b), "groups") {
		t.Fatalf("未填写的身份字段不应写入配置: %s", b)
	}
	for _, u := range []AuthUser{{Name: "a", Password: "pw", Email: "nope"}, {Name: "a", Password: "pw", Groups: []string{"a b"}}} {
		if _, err := normalizeAuthUsers([]AuthUser{u}, nil); err == nil {
			t.Errorf("无效的身份字段应被拒绝: %+v", u)
		}
	}
	// 修改身份字段不影响已有会话（凭据指纹只含密码与 TOTP）
	a := newAuthenticator("s")
	changed := users[0]
	changed.Email, changed.Groups = "new@x.test", nil
	if a.sharedCred(users[0]) != a.sharedCred(changed) {
		t.Fatal("修改邮箱或分组不应使会话失效")
	}
}

func TestOIDCDiscoveryAndRouting(t *testing.T) {
	f := newOIDCFixture(t, "auth.test", "")
	resp, body := f.do("GET", oidcDiscoveryPath, nil, map[string]string{"Origin": "https://spa.test"})
	var d map[string]any
	if resp.StatusCode != 200 || json.Unmarshal([]byte(body), &d) != nil {
		t.Fatalf("发现文档: %d %s", resp.StatusCode, body)
	}
	if d["issuer"] != f.issuer || d["authorization_endpoint"] != f.issuer+oidcAuthorizePath || d["jwks_uri"] != f.issuer+oidcJWKSPath {
		t.Fatalf("发现文档的地址不正确: %v", d)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("发现文档应允许跨域读取")
	}
	// 其他域名上的同名路径照常交给站点，不被拦截
	if _, body := get(t, f.c, f.addr, "app.test", oidcDiscoveryPath, nil); body != "backend:"+oidcDiscoveryPath {
		t.Fatalf("非签发者域名不应被拦截: %s", body)
	}
	if _, body := get(t, f.c, f.addr, "app.test", oidcTokenPath, nil); body != "backend:"+oidcTokenPath {
		t.Fatalf("非签发者域名不应被拦截: %s", body)
	}
	// 签发者域名上的其他路径照常路由（这里没有站点，返回 404）
	if resp, _ := f.do("GET", "/other", nil, nil); resp.StatusCode != 404 {
		t.Fatalf("签发者域名的普通路径应按站点路由: %d", resp.StatusCode)
	}
	if resp, _ := f.do("GET", oidcPath+"/nope", nil, nil); resp.StatusCode != 404 {
		t.Fatalf("未知的 OIDC 路径应返回 404: %d", resp.StatusCode)
	}
	// 停用后不再拦截
	_ = f.r.auth.oidc.set(OIDCSettings{})
	if resp, _ := f.do("GET", oidcDiscoveryPath, nil, nil); resp.StatusCode != 404 {
		t.Fatalf("停用后不应提供发现文档: %d", resp.StatusCode)
	}
}

func TestOIDCAuthorizationCodeFlow(t *testing.T) {
	f := newOIDCFixture(t, "auth.test", "")
	q := authorizeQuery("gitea", oidcTestRedirect)
	// 未登录：跳到签发者的登录页，登录后回到授权地址
	resp, loc := f.authorize(q)
	if resp.StatusCode != 302 || loc.Path != oidcLoginPath || !strings.HasPrefix(loc.Query().Get("next"), oidcAuthorizePath+"?") {
		t.Fatalf("未登录应跳转登录页: %d %s", resp.StatusCode, loc)
	}
	_, page := f.do("GET", loc.RequestURI(), nil, map[string]string{"User-Agent": "Mozilla/5.0", "Accept": "text/html"})
	if !strings.Contains(page, "Gitea") || !strings.Contains(page, `action="`+oidcLoginPath+`"`) {
		t.Fatal("登录页应显示应用名称并提交到 OIDC 登录路径")
	}
	if _, page := f.do("GET", oidcLoginPath+"?client=<b>evil</b>", nil, map[string]string{"User-Agent": "Mozilla/5.0", "Accept": "text/html"}); strings.Contains(page, "evil") {
		t.Fatal("登录页只应显示已登记客户端的名称")
	}
	// 密码错误重新显示登录页时仍显示应用名（从 next 中的 client_id 取得）
	_, page = f.do("POST", oidcLoginPath, url.Values{"username": {"alice"}, "password": {"bad"}, "next": {loc.Query().Get("next")}}, map[string]string{"User-Agent": "Mozilla/5.0", "Accept": "text/html"})
	if !strings.Contains(page, "Gitea") {
		t.Fatal("密码错误后登录页应继续显示应用名称")
	}
	// 登录成功后回到 next 指向的授权地址
	resp, _ = f.do("POST", oidcLoginPath, url.Values{"username": {"alice"}, "password": {"alice-pw"}, "next": {loc.Query().Get("next")}}, nil)
	if resp.StatusCode != 302 || !strings.HasPrefix(resp.Header.Get("Location"), oidcAuthorizePath+"?") {
		t.Fatalf("登录后应回到授权地址: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	cookie := resp.Cookies()[0]
	code := f.code(q, cookie)

	// 错误的密钥、回调地址都不能兑换
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {oidcTestRedirect}}
	if st, out := f.token(form, "gitea", "wrong-secret-0000000"); st != 401 || out["error"] != "invalid_client" {
		t.Fatalf("错误的客户端密钥应返回 invalid_client: %d %v", st, out)
	}
	bad := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {"https://git.test/other"}}
	if st, out := f.token(bad, "gitea", oidcTestSecret); st != 400 || out["error"] != "invalid_grant" {
		t.Fatalf("回调地址不一致应返回 invalid_grant: %d %v", st, out)
	}
	if st, out := f.token(form, "admin-only", oidcTestSecret); st != 400 || out["error"] != "invalid_grant" {
		t.Fatalf("授权码不能被其他客户端兑换: %d %v", st, out)
	}
	st, out := f.token(form, "gitea", oidcTestSecret)
	if st != 200 || out["token_type"] != "Bearer" || out["refresh_token"] != nil {
		t.Fatalf("兑换授权码: %d %v", st, out)
	}
	// 授权码只能用一次
	if st, out := f.token(form, "gitea", oidcTestSecret); st != 400 || out["error"] != "invalid_grant" {
		t.Fatalf("授权码重放应失败: %d %v", st, out)
	}
	claims := f.verify(out["id_token"].(string))
	// sub 为共享用户不可变的随机 ID，用户名在 preferred_username 中
	aliceID := f.users[0].ID
	want := map[string]any{"iss": f.issuer, "aud": "gitea", "azp": "gitea", "sub": aliceID, "nonce": "n-1", "email": "alice@example.com",
		"email_verified": true, "name": "Alice Liddell", "preferred_username": "alice"}
	for k, v := range want {
		if claims[k] != v {
			t.Errorf("ID Token 的 %s = %v，期望 %v", k, claims[k], v)
		}
	}
	if g, _ := claims["groups"].([]any); len(g) != 2 || g[0] != "admins" {
		t.Errorf("ID Token 应包含分组: %v", claims["groups"])
	}
	exp, _ := claims["exp"].(float64)
	if d := time.Until(time.Unix(int64(exp), 0)); d < 50*time.Minute || d > 61*time.Minute {
		t.Errorf("ID Token 有效期应约为 1 小时: %v", d)
	}
	access := out["access_token"].(string)
	sum := sha256.Sum256([]byte(access))
	if claims["at_hash"] != base64.RawURLEncoding.EncodeToString(sum[:16]) {
		t.Error("at_hash 与 Access Token 不一致")
	}
	if ac := f.verify(access); ac["client_id"] != "gitea" || ac["sub"] != aliceID {
		t.Errorf("Access Token 声明不正确: %v", ac)
	}

	// 用户信息接口
	resp, body := f.do("GET", oidcUserinfoPath, nil, map[string]string{"Authorization": "Bearer " + access})
	var info map[string]any
	_ = json.Unmarshal([]byte(body), &info)
	if resp.StatusCode != 200 || info["sub"] != aliceID || info["email"] != "alice@example.com" {
		t.Fatalf("用户信息: %d %s", resp.StatusCode, body)
	}
	if resp, _ := f.do("GET", oidcUserinfoPath, nil, map[string]string{"Authorization": "Bearer " + out["id_token"].(string)}); resp.StatusCode != 401 {
		t.Fatal("ID Token 不能当作 Access Token 使用")
	}
	if resp, _ := f.do("GET", oidcUserinfoPath, nil, map[string]string{"Authorization": "Bearer " + access + "x"}); resp.StatusCode != 401 || !strings.Contains(resp.Header.Get("WWW-Authenticate"), "invalid_token") {
		t.Fatal("篡改的 Access Token 应返回 401")
	}

	// client_secret_post 同样可用；scope 只给 openid 时不返回邮箱等信息
	code = f.code(authorizeQuery("gitea", oidcTestRedirect, "scope", "openid unknown"), cookie)
	st, out = f.token(url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {oidcTestRedirect}, "client_id": {"gitea"}, "client_secret": {oidcTestSecret}}, "", "")
	if st != 200 || out["scope"] != "openid" {
		t.Fatalf("client_secret_post: %d %v", st, out)
	}
	if c := f.verify(out["id_token"].(string)); c["email"] != nil || c["groups"] != nil || c["name"] != nil {
		t.Fatalf("未请求的 scope 不应返回对应声明: %v", c)
	}
}

func TestOIDCAuthorizeErrors(t *testing.T) {
	f := newOIDCFixture(t, "auth.test", "")
	cookie := f.login("alice", "alice-pw")
	// 客户端未登记或回调地址不匹配：不能跳回，直接显示错误页
	for _, q := range []url.Values{authorizeQuery("nope", oidcTestRedirect), authorizeQuery("gitea", "https://evil.test/cb")} {
		resp, _ := f.authorize(q, cookie)
		if resp.StatusCode != 400 || resp.Header.Get("Location") != "" {
			t.Fatalf("无效客户端或回调地址不应跳转: %d %s", resp.StatusCode, resp.Header.Get("Location"))
		}
	}
	cases := map[string]url.Values{
		"unsupported_response_type": authorizeQuery("gitea", oidcTestRedirect, "response_type", "token"),
		"invalid_scope":             authorizeQuery("gitea", oidcTestRedirect, "scope", "profile email"),
		"invalid_request":           authorizeQuery("gitea", oidcTestRedirect, "response_mode", "form_post"),
		"request_not_supported":     authorizeQuery("gitea", oidcTestRedirect, "request", "eyJ"),
	}
	for want, q := range cases {
		resp, loc := f.authorize(q, cookie)
		if resp.StatusCode != 302 || loc.Query().Get("error") != want || loc.Query().Get("state") != "st-1" || loc.Query().Get("code") != "" {
			t.Errorf("期望错误 %s: %d %s", want, resp.StatusCode, loc)
		}
	}
	// prompt=none 且未登录：直接返回 login_required，不显示登录页
	if _, loc := f.authorize(authorizeQuery("gitea", oidcTestRedirect, "prompt", "none")); loc.Query().Get("error") != "login_required" {
		t.Fatalf("prompt=none 未登录应返回 login_required: %s", loc)
	}
	// 允许列表：按用户名或分组；被拒绝时页面提供“换个账号登录”，退出后带着原授权请求回到登录页
	bobCookie := f.login("bob", "bob-pw")
	denied := authorizeQuery("dev-only", oidcTestRedirect)
	resp, body := f.do("GET", oidcAuthorizePath+"?"+denied.Encode(), nil, map[string]string{"User-Agent": "Mozilla/5.0", "Accept": "text/html"}, bobCookie)
	if resp.StatusCode != 403 || !strings.Contains(body, oidcLogoutPath+"?switch=") {
		t.Fatalf("不在允许列表的用户应被拒绝并可换账号: %d", resp.StatusCode)
	}
	resp, _ = f.do("GET", oidcLogoutPath+"?switch="+url.QueryEscape(oidcAuthorizePath+"?"+denied.Encode()), nil, nil, bobCookie)
	if loc, _ := url.Parse(resp.Header.Get("Location")); loc == nil || loc.Path != oidcLoginPath || loc.Query().Get("next") != oidcAuthorizePath+"?"+denied.Encode() {
		t.Fatalf("换账号应退出后回到登录页并保留授权请求: %s", resp.Header.Get("Location"))
	}
	if resp, _ := f.do("GET", oidcLogoutPath+"?switch="+url.QueryEscape("https://evil.test/"), nil, nil); resp.Header.Get("Location") != oidcLoginPath {
		t.Fatal("换账号只能回到本签发者的授权地址")
	}
	f.code(authorizeQuery("admin-only", oidcTestRedirect), f.login("bob", "bob-pw"))
	f.code(authorizeQuery("dev-only", oidcTestRedirect), cookie)
	if resp, _ := f.do("POST", oidcAuthorizePath, url.Values{"client_id": {"gitea"}}, nil); resp.StatusCode != 400 {
		t.Fatalf("缺少回调地址应显示错误: %d", resp.StatusCode)
	}
	// POST 发起的授权请求同样支持，未登录时转成等价的 GET 地址
	resp, _ = f.do("POST", oidcAuthorizePath, authorizeQuery("gitea", oidcTestRedirect), nil)
	if loc, _ := url.Parse(resp.Header.Get("Location")); resp.StatusCode != 302 || loc.Path != oidcLoginPath {
		t.Fatalf("POST 授权请求未登录应跳转登录页: %d", resp.StatusCode)
	}
}

func TestOIDCPKCE(t *testing.T) {
	f := newOIDCFixture(t, "auth.test", "")
	cookie := f.login("alice", "alice-pw")
	// 公共客户端必须使用 PKCE，且只接受 S256
	if _, loc := f.authorize(authorizeQuery("spa", oidcTestSPA), cookie); loc.Query().Get("error") != "invalid_request" {
		t.Fatalf("公共客户端缺少 PKCE 应被拒绝: %s", loc)
	}
	verifier, challenge := pkcePair()
	if _, loc := f.authorize(authorizeQuery("spa", oidcTestSPA, "code_challenge", challenge, "code_challenge_method", "plain"), cookie); loc.Query().Get("error") != "invalid_request" {
		t.Fatalf("plain 方法应被拒绝: %s", loc)
	}
	code := f.code(authorizeQuery("spa", oidcTestSPA, "code_challenge", challenge, "code_challenge_method", "S256"), cookie)
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {oidcTestSPA}, "client_id": {"spa"}}
	if st, out := f.token(form, "", ""); st != 400 || out["error"] != "invalid_grant" {
		t.Fatalf("缺少 code_verifier 应失败: %d %v", st, out)
	}
	form.Set("code_verifier", strings.Repeat("x", 50))
	if st, _ := f.token(form, "", ""); st != 400 {
		t.Fatal("错误的 code_verifier 应失败")
	}
	// 校验失败不消耗授权码，正确的 verifier 仍可兑换
	form.Set("code_verifier", verifier)
	if st, out := f.token(form, "", ""); st != 200 || out["id_token"] == nil {
		t.Fatalf("正确的 code_verifier 应兑换成功: %d %v", st, out)
	}
	// 公共客户端不能携带密钥冒充
	code = f.code(authorizeQuery("spa", oidcTestSPA, "code_challenge", challenge, "code_challenge_method", "S256"), cookie)
	form.Set("code", code)
	form.Set("client_secret", "anything")
	if st, _ := f.token(form, "", ""); st != 401 {
		t.Fatal("公共客户端带密钥应被拒绝")
	}
	// 机密客户端也可以叠加 PKCE
	code = f.code(authorizeQuery("gitea", oidcTestRedirect, "code_challenge", challenge, "code_challenge_method", "S256"), cookie)
	if st, _ := f.token(url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {oidcTestRedirect}}, "gitea", oidcTestSecret); st != 400 {
		t.Fatal("授权时使用了 PKCE，兑换时必须提供 code_verifier")
	}
}

func TestOIDCRefreshAndRevocation(t *testing.T) {
	f := newOIDCFixture(t, "auth.test", "")
	cookie := f.login("alice", "alice-pw")
	code := f.code(authorizeQuery("gitea", oidcTestRedirect, "scope", "openid email offline_access"), cookie)
	st, out := f.token(url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {oidcTestRedirect}}, "gitea", oidcTestSecret)
	rt, _ := out["refresh_token"].(string)
	if st != 200 || rt == "" {
		t.Fatalf("请求 offline_access 时应签发刷新令牌: %d %v", st, out)
	}
	refresh := func(client, scope string) (int, map[string]any) {
		form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {rt}}
		if scope != "" {
			form.Set("scope", scope)
		}
		return f.token(form, client, oidcTestSecret)
	}
	st, out = refresh("gitea", "")
	if st != 200 || out["access_token"] == nil || out["scope"] != "openid email offline_access" {
		t.Fatalf("刷新令牌: %d %v", st, out)
	}
	if c := f.verify(out["id_token"].(string)); c["email"] != "alice@example.com" || c["nonce"] != nil {
		t.Fatalf("刷新得到的 ID Token 不正确: %v", c)
	}
	// 可以缩小 scope，不能扩大
	if st, out := refresh("gitea", "openid profile"); st != 200 || out["scope"] != "openid" {
		t.Fatalf("刷新时 scope 只能缩小: %d %v", st, out)
	}
	if st, out := refresh("admin-only", ""); st != 400 || out["error"] != "invalid_grant" {
		t.Fatalf("刷新令牌不能被其他客户端使用: %d %v", st, out)
	}
	access := out["access_token"].(string)

	// 修改密码后：已签发的授权码与刷新令牌失效
	pending := f.code(authorizeQuery("gitea", oidcTestRedirect), cookie)
	users := append([]AuthUser{}, f.users...)
	users[0].Password = "$2a$10$" + strings.Repeat("a", 53)
	f.r.auth.shared.set(users, nil)
	if st, _ := f.token(url.Values{"grant_type": {"authorization_code"}, "code": {pending}, "redirect_uri": {oidcTestRedirect}}, "gitea", oidcTestSecret); st != 400 {
		t.Fatal("修改密码后授权码应失效")
	}
	if st, _ := refresh("gitea", ""); st != 400 {
		t.Fatal("修改密码后刷新令牌应失效")
	}
	if resp, _ := f.authorize(authorizeQuery("gitea", oidcTestRedirect), cookie); !strings.Contains(resp.Header.Get("Location"), oidcLoginPath) {
		t.Fatal("修改密码后签发者上的会话应失效")
	}
	// 删除客户端后 Access Token 不能再查询用户信息
	f.r.auth.shared.set(f.users, nil)
	set := f.set
	set.Clients = set.Clients[1:]
	_ = f.r.auth.oidc.set(set)
	if resp, _ := f.do("GET", oidcUserinfoPath, nil, map[string]string{"Authorization": "Bearer " + access}); resp.StatusCode != 401 {
		t.Fatal("客户端删除后 Access Token 应失效")
	}
}

func TestOIDCTOTPAndSSO(t *testing.T) {
	f := newOIDCFixture(t, "auth.sso.test", "sso.test")
	// 在单点登录域内的任一站点登录后，签发者无需再次登录（carol 启用了 TOTP）
	form := url.Values{"username": {"carol"}, "password": {"carol-pw"}, "code": {totpNow(f.carolTOTP)}, "next": {"/"}}
	req, _ := http.NewRequest("POST", "http://"+f.addr+authPath+"/login", strings.NewReader(form.Encode()))
	req.Host = "a.sso.test"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := f.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	var sso *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == ssoCookie {
			sso = c
		}
	}
	if sso == nil {
		t.Fatalf("站点登录应写出单点登录 Cookie: %d", resp.StatusCode)
	}
	code := f.code(authorizeQuery("admin-only", oidcTestRedirect), sso)
	st, out := f.token(url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {oidcTestRedirect}}, "admin-only", oidcTestSecret)
	if st != 200 {
		t.Fatalf("单点登录后兑换: %d %v", st, out)
	}
	c := f.verify(out["id_token"].(string))
	if c["sub"] != f.users[2].ID || c["preferred_username"] != "carol" {
		t.Fatalf("ID Token 用户不正确: %v", c)
	}
	if at, _ := c["auth_time"].(float64); time.Since(time.Unix(int64(at), 0)) > time.Minute {
		t.Fatalf("auth_time 应为登录时间: %v", c["auth_time"])
	}
	// 签发者登录页同样要求 TOTP：只提交密码会进入验证码步骤
	_, body := f.do("POST", oidcLoginPath, url.Values{"username": {"carol"}, "password": {"carol-pw"}}, nil)
	if !strings.Contains(body, `name="pending"`) {
		t.Fatal("启用 TOTP 的用户在签发者登录时应输入验证码")
	}
	// 应用发起退出：清除单点登录 Cookie，同源的回调地址可以跳回
	resp, _ = f.do("GET", oidcLogoutPath+"?client_id=gitea&post_logout_redirect_uri="+url.QueryEscape("https://git.test/")+"&state=s9", nil, nil, sso)
	cleared := false
	for _, ck := range resp.Cookies() {
		if ck.Name == ssoCookie && ck.MaxAge < 0 && ck.Domain == "sso.test" {
			cleared = true
		}
	}
	if resp.Header.Get("Location") != "https://git.test/?state=s9" || !cleared {
		t.Fatalf("退出应清除单点登录并跳回应用: %s %v", resp.Header.Get("Location"), resp.Cookies())
	}
	if resp, _ := f.do("GET", oidcLogoutPath+"?client_id=gitea&post_logout_redirect_uri="+url.QueryEscape("https://evil.test/"), nil, nil); resp.Header.Get("Location") != oidcLoginPath {
		t.Fatal("退出后的跳转地址需与客户端回调同源")
	}
}

func TestOIDCTokenRateLimit(t *testing.T) {
	f := newOIDCFixture(t, "auth.test", "")
	form := url.Values{"grant_type": {"authorization_code"}, "code": {"x"}, "redirect_uri": {oidcTestRedirect}}
	for i := 0; i < maxFailures; i++ {
		if st, _ := f.token(form, "gitea", "wrong-secret-0000000"); st != 401 {
			t.Fatalf("第 %d 次错误密钥应返回 401: %d", i+1, st)
		}
	}
	if st, _ := f.token(form, "gitea", oidcTestSecret); st != 429 {
		t.Fatalf("连续失败后应限流: %d", st)
	}
}

// TestOIDCWithGenericClient 用 Gately 自带的 OIDC 客户端（第三方登录）走一遍发现、授权、换取令牌与用户信息，
// 验证与标准客户端的互通。
func TestOIDCWithGenericClient(t *testing.T) {
	f := newOIDCFixture(t, "", "")
	p := thirdauth.Provider{Key: "gately", Type: "oidc", ClientID: "gitea", ClientSecret: oidcTestSecret, Issuer: f.issuer}
	// 测试环境只有 HTTP，跳过客户端对 https 签发者的校验
	ctx := context.Background()
	target, err := p.AuthorizeURL(ctx, oidcTestRedirect, "st-1")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(target)
	code := f.code(u.Query(), f.login("alice", "alice-pw"))
	id, err := p.Exchange(ctx, code, oidcTestRedirect)
	if err != nil {
		t.Fatal(err)
	}
	if id.Primary != "alice@example.com" || len(id.Aliases) != 1 || id.Aliases[0] != f.users[0].ID {
		t.Fatalf("通用客户端得到的身份不正确: %+v", id)
	}
}

// TestOIDCSettingsAPI 验证设置接口：保存后脱敏、签名密钥与客户端密钥沿用、重启后照常生效，未启用时不影响老配置。
func TestOIDCSettingsAPI(t *testing.T) {
	dir := t.TempDir()
	st, _ := config.Open(dir)
	m := New(nil, nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	if m.auth.oidc.get() != nil || m.cfg.Settings.OIDC.SigningKey != "" {
		t.Fatal("老配置不应启用 OIDC 或生成密钥")
	}
	put := func(s Settings) (int, Settings) {
		raw, _ := json.Marshal(s)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("PUT", "/settings", strings.NewReader(string(raw)))
		req.Header.Set("Content-Type", "application/json")
		m.handlePutSettings(rec, req)
		var out Settings
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	s := Settings{Enable: true, AuthUsers: []AuthUser{{Name: "alice", Password: "pw", Email: "a@x.test"}},
		OIDC: OIDCSettings{Enable: true, Issuer: "https://auth.x.test", Clients: []OIDCClient{{ID: "gitea", Secret: oidcTestSecret, RedirectURIs: []string{oidcTestRedirect}}}}}
	code, out := put(s)
	if code != 200 || out.OIDC.SigningKey != "" || out.OIDC.Clients[0].Secret != crud.SecretMask {
		t.Fatalf("保存后应返回脱敏的设置: %d %+v", code, out.OIDC)
	}
	key, secret := m.cfg.Settings.OIDC.SigningKey, m.cfg.Settings.OIDC.Clients[0].Secret
	env := m.auth.oidc.get()
	if key == "" || !strings.HasPrefix(secret, "$2") || env == nil || env.issuer != "https://auth.x.test" {
		t.Fatal("保存后应生成密钥并立即生效")
	}
	// 原样提交接口返回的设置：密钥不变
	out.AuthUsers[0].Password = crud.SecretMask
	if code, _ := put(out); code != 200 || m.cfg.Settings.OIDC.SigningKey != key || m.cfg.Settings.OIDC.Clients[0].Secret != secret {
		t.Fatalf("重复保存不应更换密钥: %d", code)
	}
	// 无效设置被拒绝，原配置保持
	bad := out
	bad.OIDC.Issuer = "https://auth.x.test/path"
	if code, _ := put(bad); code != 400 || m.auth.oidc.get() == nil {
		t.Fatalf("无效的签发者应被拒绝: %d", code)
	}
	// 重新加载：同一密钥继续生效
	m2 := New(nil, nil)
	if err := m2.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	if e := m2.auth.oidc.get(); e == nil || e.kid != env.kid {
		t.Fatal("重启后应使用同一签名密钥")
	}
	// 关闭后停止服务，密钥保留以便再次启用
	out.OIDC.Enable = false
	if code, _ := put(out); code != 200 || m.auth.oidc.get() != nil || m.cfg.Settings.OIDC.SigningKey != key {
		t.Fatalf("关闭后应停止服务并保留密钥: %d", code)
	}
}

// reloadSites 在监听器上追加站点并热替换站点表。
func (f *oidcFixture) reloadSites(extra ...Site) {
	f.t.Helper()
	cfg := f.r.cfg
	cfg.Sites = append(append([]Site{}, cfg.Sites...), extra...)
	if err := cfg.Validate(); err != nil {
		f.t.Fatal(err)
	}
	if err := f.r.reload(cfg, nil); err != nil {
		f.t.Fatal(err)
	}
}

// setIssuer 更换签发者地址（域名不变）。
func (f *oidcFixture) setIssuer(issuer string) {
	f.t.Helper()
	f.set.Issuer = issuer
	if err := f.r.auth.oidc.set(f.set); err != nil {
		f.t.Fatal(err)
	}
	f.issuer = issuer
}

// exchange 用授权码换取令牌（gitea 客户端，client_secret_basic）。
func (f *oidcFixture) exchange(code string) map[string]any {
	f.t.Helper()
	st, out := f.token(url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {oidcTestRedirect}}, "gitea", oidcTestSecret)
	if st != 200 {
		f.t.Fatalf("兑换授权码: %d %v", st, out)
	}
	return out
}

// TestOIDCHTTPSIssuer 验证签发者为 https 时，明文连接上的协议路径跳转到 https 或被拒绝，不显示登录页、不签发令牌。
func TestOIDCHTTPSIssuer(t *testing.T) {
	f := newOIDCFixture(t, "auth.test", "")
	f.setIssuer("https://auth.test")
	q := authorizeQuery("gitea", oidcTestRedirect)
	resp, _ := f.do("GET", oidcAuthorizePath+"?"+q.Encode(), nil, nil)
	if resp.StatusCode != http.StatusMovedPermanently || resp.Header.Get("Location") != "https://auth.test"+oidcAuthorizePath+"?"+q.Encode() {
		t.Fatalf("明文 GET 应永久跳转到签发者地址: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	if resp, _ := f.do("GET", oidcDiscoveryPath, nil, nil); resp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("发现文档同样应跳转: %d", resp.StatusCode)
	}
	// 明文提交密码被拒绝，也不写出会话 Cookie
	resp, _ = f.do("POST", oidcLoginPath, url.Values{"username": {"alice"}, "password": {"alice-pw"}, "next": {"/"}}, nil)
	if resp.StatusCode != http.StatusNotFound || len(resp.Cookies()) != 0 {
		t.Fatalf("明文登录应被拒绝: %d %v", resp.StatusCode, resp.Cookies())
	}
	st, out := f.token(url.Values{"grant_type": {"authorization_code"}, "code": {"x"}}, "gitea", oidcTestSecret)
	if st != http.StatusBadRequest || out["error"] != "invalid_request" {
		t.Fatalf("明文令牌请求应返回 invalid_request: %d %v", st, out)
	}
	// 本机或内网的反向代理已终止 TLS、以 X-Forwarded-Proto: https 转发时照常提供
	resp, body := f.do("GET", oidcDiscoveryPath, nil, map[string]string{"X-Forwarded-Proto": "https"})
	if resp.StatusCode != 200 || !strings.Contains(body, `"issuer":"https://auth.test"`) {
		t.Fatalf("经 HTTPS 反向代理转发时应提供发现文档: %d %s", resp.StatusCode, body)
	}
	if !strings.Contains(resp.Header.Get("Cache-Control"), "max-age=300") {
		t.Fatalf("发现文档应允许缓存: %s", resp.Header.Get("Cache-Control"))
	}
}

// TestOIDCIssuerSiteAccessControl 验证签发者域名配有站点时，该站点的 IP 访问控制与限流同样作用于 OIDC 协议路径。
func TestOIDCIssuerSiteAccessControl(t *testing.T) {
	f := newOIDCFixture(t, "auth.test", "")
	if resp, _ := f.do("GET", oidcDiscoveryPath, nil, nil); resp.StatusCode != 200 {
		t.Fatalf("没有站点时照常提供: %d", resp.StatusCode)
	}
	f.reloadSites(Site{Enable: true, Key: "auth", Domains: []string{"auth.test"}, Targets: []string{"http://127.0.0.1:1"}, IPMode: IPModeBlacklist, IPList: []string{"127.0.0.1"}})
	for _, p := range []string{oidcDiscoveryPath, oidcAuthorizePath + "?" + authorizeQuery("gitea", oidcTestRedirect).Encode(), oidcLoginPath} {
		if resp, _ := f.do("GET", p, nil, nil); resp.StatusCode != http.StatusForbidden {
			t.Fatalf("黑名单中的地址访问 %s 应被拒绝: %d", p, resp.StatusCode)
		}
	}
	if st, _ := f.token(url.Values{"grant_type": {"authorization_code"}}, "gitea", oidcTestSecret); st != http.StatusForbidden {
		t.Fatalf("黑名单中的地址不能调用令牌接口: %d", st)
	}
	// 站点限流同样生效
	f.r.cfg.Sites = f.r.cfg.Sites[:len(f.r.cfg.Sites)-1]
	f.reloadSites(Site{Enable: true, Key: "auth", Domains: []string{"auth.test"}, Targets: []string{"http://127.0.0.1:1"}, RateLimit: RateLimit{Enable: true, Rate: 0.001, Burst: 1}})
	if resp, _ := f.do("GET", oidcDiscoveryPath, nil, nil); resp.StatusCode != 200 {
		t.Fatalf("限流额度内应正常访问: %d", resp.StatusCode)
	}
	if resp, _ := f.do("GET", oidcDiscoveryPath, nil, nil); resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("超过站点限流应返回 429: %d", resp.StatusCode)
	}
}

// TestOIDCAccessTokenRevocation 验证修改密码后已签发的 Access Token 不能再查询用户信息，Bearer 不区分大小写。
func TestOIDCAccessTokenRevocation(t *testing.T) {
	f := newOIDCFixture(t, "auth.test", "")
	out := f.exchange(f.code(authorizeQuery("gitea", oidcTestRedirect), f.login("alice", "alice-pw")))
	access := out["access_token"].(string)
	if resp, _ := f.do("GET", oidcUserinfoPath, nil, map[string]string{"Authorization": "bearer " + access}); resp.StatusCode != 200 {
		t.Fatalf("小写 bearer 应被接受: %d", resp.StatusCode)
	}
	users := append([]AuthUser{}, f.users...)
	users[0].Password = "$2a$10$" + strings.Repeat("b", 53)
	f.r.auth.shared.set(users, nil)
	resp, _ := f.do("GET", oidcUserinfoPath, nil, map[string]string{"Authorization": "Bearer " + access})
	if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(resp.Header.Get("WWW-Authenticate"), "invalid_token") {
		t.Fatalf("修改密码后 Access Token 应失效: %d", resp.StatusCode)
	}
	if resp, _ := f.do("GET", oidcUserinfoPath, nil, nil); resp.StatusCode != http.StatusUnauthorized || !strings.HasPrefix(resp.Header.Get("WWW-Authenticate"), "Bearer") {
		t.Fatalf("缺少令牌应返回 401 与 WWW-Authenticate: %d", resp.StatusCode)
	}
}

// TestOIDCPromptLoginAndMaxAge 验证 prompt=login 与 max_age 要求重新登录，重新登录后不再循环跳转。
func TestOIDCPromptLoginAndMaxAge(t *testing.T) {
	f := newOIDCFixture(t, "auth.test", "")
	old := f.login("alice", "alice-pw")
	// 已登录且未要求重新登录：直接签发授权码；max_age 足够大时同样
	f.code(authorizeQuery("gitea", oidcTestRedirect, "max_age", "3600"), old)
	for _, extra := range [][]string{{"prompt", "login"}, {"max_age", "0"}} {
		q := authorizeQuery("gitea", oidcTestRedirect, extra...)
		resp, loc := f.authorize(q, old)
		next := loc.Query().Get("next")
		if resp.StatusCode != 302 || loc.Path != oidcLoginPath || !strings.Contains(next, oidcReauthParam+"=") {
			t.Fatalf("%v 应要求重新登录: %d %s", extra, resp.StatusCode, loc)
		}
		// 不重新登录、直接带着标记回来：仍要求登录
		if _, again := f.authorize(mustQuery(t, next), old); again.Path != oidcLoginPath {
			t.Fatalf("%v 未重新登录不应签发授权码: %s", extra, again)
		}
		// 伪造的标记无效
		forged := mustQuery(t, next)
		forged.Set(oidcReauthParam, "forged")
		if _, again := f.authorize(forged, old); again.Path != oidcLoginPath {
			t.Fatalf("伪造的重新登录标记不应生效: %s", again)
		}
		// 重新登录（晚于标记签发的那一秒）后回到授权地址，签发授权码，不再循环
		time.Sleep(1100 * time.Millisecond)
		resp, _ = f.do("POST", oidcLoginPath, url.Values{"username": {"alice"}, "password": {"alice-pw"}, "next": {next}}, nil)
		if resp.StatusCode != 302 || resp.Header.Get("Location") != next {
			t.Fatalf("重新登录后应回到授权地址: %d %s", resp.StatusCode, resp.Header.Get("Location"))
		}
		fresh := resp.Cookies()[0]
		code := f.code(mustQuery(t, next), fresh)
		c := f.verify(f.exchange(code)["id_token"].(string))
		if at, _ := c["auth_time"].(float64); time.Since(time.Unix(int64(at), 0)) > 5*time.Second {
			t.Fatalf("auth_time 应为重新登录的时间: %v", c["auth_time"])
		}
	}
	// prompt=none 不能与 login 同时使用；需要重新登录时静默请求返回 login_required
	if _, loc := f.authorize(authorizeQuery("gitea", oidcTestRedirect, "prompt", "none login"), old); loc.Query().Get("error") != "invalid_request" {
		t.Fatalf("prompt=none login 应返回 invalid_request: %s", loc)
	}
	if _, loc := f.authorize(authorizeQuery("gitea", oidcTestRedirect, "prompt", "none", "max_age", "0"), old); loc.Query().Get("error") != "login_required" {
		t.Fatalf("prompt=none 且超过 max_age 应返回 login_required: %s", loc)
	}
	if _, loc := f.authorize(authorizeQuery("gitea", oidcTestRedirect, "max_age", "-1"), old); loc.Query().Get("error") != "invalid_request" {
		t.Fatalf("无效的 max_age 应返回 invalid_request: %s", loc)
	}
}

func mustQuery(t *testing.T, uri string) url.Values {
	t.Helper()
	u, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query()
}

// TestOIDCPublicClientNoRefresh 验证公共客户端不获得刷新令牌，也不能使用刷新授权。
func TestOIDCPublicClientNoRefresh(t *testing.T) {
	f := newOIDCFixture(t, "auth.test", "")
	verifier, challenge := pkcePair()
	code := f.code(authorizeQuery("spa", oidcTestSPA, "scope", "openid offline_access", "code_challenge", challenge, "code_challenge_method", "S256"), f.login("alice", "alice-pw"))
	st, out := f.token(url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {oidcTestSPA}, "client_id": {"spa"}, "code_verifier": {verifier}}, "", "")
	if st != 200 || out["refresh_token"] != nil || out["scope"] != "openid" {
		t.Fatalf("公共客户端不应获得刷新令牌: %d %v", st, out)
	}
	if st, out := f.token(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"x"}, "client_id": {"spa"}}, "", ""); st != 400 || out["error"] != "invalid_grant" {
		t.Fatalf("公共客户端的刷新请求应被拒绝: %d %v", st, out)
	}
}

// TestSharedUserIDs 验证共享用户的不可变 ID：自动生成、改名保留（连同密码与 TOTP）、删除后同名新建得到新 ID。
func TestSharedUserIDs(t *testing.T) {
	totp := auth.NewTOTPSecret()
	users, err := normalizeAuthUsers([]AuthUser{{Name: "alice", Password: "pw", TOTPSecret: totp}, {Name: "bob", Password: "pw"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	alice := users[0]
	if !validAuthUserID(alice.ID) || alice.ID == users[1].ID {
		t.Fatalf("应为每个用户生成不同的 ID: %+v", users)
	}
	// 改名且不重新输入密码：按 ID 保留原密码、TOTP 与 ID
	masked := maskAuthUsers(users)
	masked[0].Name = "alice2"
	renamed, err := normalizeAuthUsers(masked, users)
	if err != nil {
		t.Fatal(err)
	}
	if renamed[0].ID != alice.ID || renamed[0].Password != alice.Password || renamed[0].TOTPSecret != totp {
		t.Fatalf("改名后应保留 ID、密码与 TOTP: %+v", renamed[0])
	}
	// 删除后再建同名用户：新用户不带 ID，得到新的 ID，也不继承密码
	recreated, err := normalizeAuthUsers([]AuthUser{maskAuthUsers(users)[1], {Name: "alice", Password: "new-pw"}}, users)
	if err != nil {
		t.Fatal(err)
	}
	if recreated[1].ID == alice.ID || recreated[1].Password == alice.Password || recreated[1].TOTPSecret != "" {
		t.Fatalf("同名新用户应得到新的 ID: %+v", recreated[1])
	}
	if _, err := normalizeAuthUsers([]AuthUser{maskAuthUsers(users)[1], {Name: "alice"}}, users); err == nil {
		t.Fatal("同名新用户需要设置密码")
	}
	// 不认识 ID 的老调用方（整个列表都没有 ID）：按用户名对应
	legacy, err := normalizeAuthUsers([]AuthUser{{Name: "alice", Password: crud.SecretMask, TOTPSecret: crud.SecretMask}}, users)
	if err != nil || legacy[0].ID != alice.ID || legacy[0].Password != alice.Password {
		t.Fatalf("没有 ID 的列表应按用户名保留原用户: %v %+v", err, legacy)
	}
	for _, bad := range [][]AuthUser{
		{{ID: "a b", Name: "x", Password: "pw"}},
		{{ID: "same", Name: "x", Password: "pw"}, {ID: "same", Name: "y", Password: "pw"}},
	} {
		if _, err := normalizeAuthUsers(bad, nil); err == nil {
			t.Errorf("无效或重复的 ID 应被拒绝: %+v", bad)
		}
	}
}

// TestOIDCSubjectStable 验证 sub 为不可变 ID：改名后不变，删除后再建同名用户得到不同的 sub。
func TestOIDCSubjectStable(t *testing.T) {
	f := newOIDCFixture(t, "auth.test", "")
	sub := func(user, pass string) any {
		t.Helper()
		return f.verify(f.exchange(f.code(authorizeQuery("gitea", oidcTestRedirect), f.login(user, pass)))["id_token"].(string))["sub"]
	}
	before := sub("alice", "alice-pw")
	masked := maskAuthUsers(f.users)
	masked[0].Name = "alice-renamed"
	renamed, err := normalizeAuthUsers(masked, f.users)
	if err != nil {
		t.Fatal(err)
	}
	f.r.auth.shared.set(renamed, nil)
	if got := sub("alice-renamed", "alice-pw"); got != before {
		t.Fatalf("改名后 sub 应保持不变: %v → %v", before, got)
	}
	// 删除后再建同名用户
	again, err := normalizeAuthUsers([]AuthUser{masked[1], masked[2], {Name: "alice", Password: "alice-pw"}}, renamed)
	if err != nil {
		t.Fatal(err)
	}
	f.r.auth.shared.set(again, nil)
	if got := sub("alice", "alice-pw"); got == before {
		t.Fatal("删除后同名新建的用户不应继承原用户的 sub")
	}
}

// TestSharedUserIDsOnLoad 验证老配置加载时为共享用户补生成 ID 并立即保存，重启后 ID 不变。
func TestSharedUserIDsOnLoad(t *testing.T) {
	dir := t.TempDir()
	st, _ := config.Open(dir)
	users, _ := normalizeAuthUsers([]AuthUser{{Name: "alice", Password: "pw"}}, nil)
	users[0].ID = ""
	if err := st.Save(ID, fileConfig{Settings: Settings{Enable: true, AuthSecret: "s", AuthUsers: users}}); err != nil {
		t.Fatal(err)
	}
	load := func() string {
		m := New(nil, nil)
		if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
			t.Fatal(err)
		}
		return m.cfg.Settings.AuthUsers[0].ID
	}
	first := load()
	if first == "" || load() != first {
		t.Fatalf("加载时应补生成并保存 ID: %q", first)
	}
}

// TestOIDCClientRenameKeepsSecret 验证修改 Client ID 而不重新输入密钥时保留原密钥。
func TestOIDCClientRenameKeepsSecret(t *testing.T) {
	old := OIDCSettings{Enable: true, Issuer: "https://auth.x.test", Clients: []OIDCClient{{ID: "gitea", Secret: oidcTestSecret, RedirectURIs: []string{oidcTestRedirect}}}}
	if err := normalizeOIDC(&old, OIDCSettings{}); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{crud.SecretMask, ""} {
		s := maskOIDC(old)
		s.Clients[0].ID, s.Clients[0].Secret = "gitea2", secret
		if err := normalizeOIDC(&s, old); err != nil || s.Clients[0].Secret != old.Clients[0].Secret {
			t.Fatalf("改 Client ID 后应保留原密钥（提交 %q）: %v", secret, err)
		}
	}
	// 新增客户端不填写密钥仍然报错
	s := maskOIDC(old)
	s.Clients = append(s.Clients, OIDCClient{ID: "new", RedirectURIs: []string{oidcTestRedirect}})
	if err := normalizeOIDC(&s, old); err == nil {
		t.Fatal("新增的机密客户端需要设置密钥")
	}
}

// TestOIDCLogoutRedirect 验证退出后的跳转：客户端由签名有效的 id_token_hint 或 client_id 确定，两者需一致。
func TestOIDCLogoutRedirect(t *testing.T) {
	f := newOIDCFixture(t, "auth.test", "")
	idToken := f.exchange(f.code(authorizeQuery("gitea", oidcTestRedirect), f.login("alice", "alice-pw")))["id_token"].(string)
	logout := func(params url.Values) string {
		resp, _ := f.do("GET", oidcLogoutPath+"?"+params.Encode(), nil, nil)
		return resp.Header.Get("Location")
	}
	target := "https://git.test/"
	if loc := logout(url.Values{"id_token_hint": {idToken}, "post_logout_redirect_uri": {target}}); loc != target {
		t.Fatalf("签名有效的 id_token_hint 应确定客户端: %s", loc)
	}
	if loc := logout(url.Values{"id_token_hint": {idToken}, "client_id": {"spa"}, "post_logout_redirect_uri": {oidcTestSPA}}); loc != oidcLoginPath {
		t.Fatalf("id_token_hint 与 client_id 不一致时不应跳回: %s", loc)
	}
	// 伪造的 id_token_hint（未签名，aud 指向 gitea）不被采信
	parts := strings.Split(idToken, ".")
	forged := parts[0] + "." + parts[1] + "." + base64.RawURLEncoding.EncodeToString([]byte("bad"))
	if loc := logout(url.Values{"id_token_hint": {forged}, "post_logout_redirect_uri": {target}}); loc != oidcLoginPath {
		t.Fatalf("签名无效的 id_token_hint 不应跳回: %s", loc)
	}
	if loc := logout(url.Values{"post_logout_redirect_uri": {target}}); loc != oidcLoginPath {
		t.Fatalf("无法确定客户端时不应跳回: %s", loc)
	}
	if loc := logout(url.Values{"client_id": {"spa"}, "post_logout_redirect_uri": {oidcTestSPA}}); loc != oidcTestSPA {
		t.Fatalf("登记过的回调地址应可跳回: %s", loc)
	}
}

// TestOIDCPromptNoneDenied 验证静默登录时用户不在允许列表中返回 access_denied，而不是显示页面。
func TestOIDCPromptNoneDenied(t *testing.T) {
	f := newOIDCFixture(t, "auth.test", "")
	resp, loc := f.authorize(authorizeQuery("dev-only", oidcTestRedirect, "prompt", "none"), f.login("bob", "bob-pw"))
	if resp.StatusCode != 302 || loc.Query().Get("error") != "access_denied" || loc.Query().Get("state") != "st-1" {
		t.Fatalf("prompt=none 且无权限应返回 access_denied: %d %s", resp.StatusCode, loc)
	}
}

// TestLoginCSRF 验证其他网站代为提交的登录表单被拒绝。
func TestLoginCSRF(t *testing.T) {
	f := newOIDCFixture(t, "auth.test", "")
	form := url.Values{"username": {"alice"}, "password": {"alice-pw"}, "next": {"/"}}
	for _, hdr := range []map[string]string{{"Origin": "https://evil.test"}, {"Origin": "null"}, {"Referer": "https://evil.test/page"}} {
		resp, _ := f.do("POST", oidcLoginPath, form, hdr)
		if resp.StatusCode != http.StatusForbidden || len(resp.Cookies()) != 0 {
			t.Fatalf("跨站提交的登录表单应被拒绝 %v: %d", hdr, resp.StatusCode)
		}
	}
	for _, hdr := range []map[string]string{{"Origin": "https://auth.test"}, {"Referer": "https://auth.test" + oidcLoginPath}, nil} {
		if resp, _ := f.do("POST", oidcLoginPath, form, hdr); resp.StatusCode != http.StatusFound {
			t.Fatalf("同源提交应正常登录 %v: %d", hdr, resp.StatusCode)
		}
	}
	// 普通站点的登录页同样检查
	req, _ := http.NewRequest("POST", "http://"+f.addr+authPath+"/login", strings.NewReader(form.Encode()))
	req.Host = "a.sso.test"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://evil.test")
	resp, err := f.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("站点登录页也应拒绝跨站提交: %d", resp.StatusCode)
	}
}

// TestOIDCInputLimits 验证发现文档声明 iss 参数、state 长度上限与 PKCE verifier 字符集。
func TestOIDCInputLimits(t *testing.T) {
	f := newOIDCFixture(t, "auth.test", "")
	_, body := f.do("GET", oidcDiscoveryPath, nil, nil)
	if !strings.Contains(body, `"authorization_response_iss_parameter_supported":true`) {
		t.Fatalf("发现文档应声明授权响应带 iss: %s", body)
	}
	cookie := f.login("alice", "alice-pw")
	_, loc := f.authorize(authorizeQuery("gitea", oidcTestRedirect, "state", strings.Repeat("s", oidcMaxParam+1)), cookie)
	if loc.Query().Get("error") != "invalid_request" || loc.Query().Get("state") != "" {
		t.Fatalf("过长的 state 应返回 invalid_request 且不带回: %s", loc)
	}
	if _, loc := f.authorize(authorizeQuery("gitea", oidcTestRedirect, "nonce", strings.Repeat("n", oidcMaxParam+1)), cookie); loc.Query().Get("error") != "invalid_request" {
		t.Fatalf("过长的 nonce 应返回 invalid_request: %s", loc)
	}
	// verifier 含 RFC 7636 不允许的字符时，即使哈希一致也拒绝
	verifier := strings.Repeat("v!", 25)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	code := f.code(authorizeQuery("spa", oidcTestSPA, "code_challenge", challenge, "code_challenge_method", "S256"), cookie)
	if st, _ := f.token(url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {oidcTestSPA}, "client_id": {"spa"}, "code_verifier": {verifier}}, "", ""); st != 400 {
		t.Fatalf("非法字符的 code_verifier 应被拒绝: %d", st)
	}
	// 请求体超过上限
	big := url.Values{"grant_type": {"authorization_code"}, "pad": {strings.Repeat("x", oidcMaxForm)}}
	if st, out := f.token(big, "gitea", oidcTestSecret); st != 400 || out["error"] != "invalid_request" {
		t.Fatalf("过大的请求体应被拒绝: %d %v", st, out)
	}
	if resp, _ := f.do("GET", oidcAuthorizePath+"?"+authorizeQuery("gitea", oidcTestRedirect).Encode(), nil, nil, cookie); !strings.Contains(resp.Header.Get("Cache-Control"), "no-store") {
		t.Fatal("授权接口不应被缓存")
	}
}
