package thirdauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAllowed(t *testing.T) {
	entries := []string{"github:15172899 # VanJay", "Google:A@B.c", "qq:*"}
	cases := []struct {
		id   Identity
		want bool
	}{
		{Identity{Provider: "github", Primary: "renamed", Aliases: []string{"15172899"}}, true},
		{Identity{Provider: "github", Primary: "VanJay"}, false},
		{Identity{Provider: "google", Primary: "a@b.c"}, true},
		{Identity{Provider: "google", Primary: "sub1", Aliases: []string{}}, false},
		{Identity{Provider: "qq", Primary: "anything"}, true},
		{Identity{Provider: "weibo", Primary: "x"}, false},
	}
	for _, c := range cases {
		if got := Allowed(entries, c.id); got != c.want {
			t.Errorf("%+v: got %v want %v", c.id, got, c.want)
		}
	}
	if err := ValidateEntries([]string{"github"}); err == nil {
		t.Error("缺少标识应校验失败")
	}
	if p, v, ok := ParseEntry("qq:UID_X  # 备注"); !ok || p != "qq" || v != "UID_X" {
		t.Errorf("备注应被忽略: %q %q %v", p, v, ok)
	}
}

func TestLuckyResolve(t *testing.T) {
	ids := LuckyIdentities{List: []LuckyIdentity{
		{Key: "a", Enable: true, Type: "GitHub", ID: "1", Name: "me"},
		{Key: "b", Enable: false, Type: "qq", ID: "2"},
		{Key: "c", Enable: true, Type: "google", ID: "3", Name: "n", EMail: "e@x.y"},
	}}
	got, missing := ids.Resolve([]string{"a", "b", "zzz", "c"}, false)
	if strings.Join(got, "|") != "github:1 # me|google:3 # n e@x.y" || missing != 1 {
		t.Errorf("按 Key 解析: %q missing=%d", got, missing)
	}
	all, _ := ids.Resolve(nil, true)
	if len(all) != 2 {
		t.Errorf("全部身份应只含启用项: %q", all)
	}
}

func TestQQExchange(t *testing.T) {
	var gotRedirect string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			gotRedirect = r.URL.Query().Get("redirect_uri")
			if r.URL.Query().Get("client_secret") != "sec" || r.URL.Query().Get("code") != "c1" {
				_ = json.NewEncoder(w).Encode(map[string]any{"error": 100010, "error_description": "bad"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "AT", "expires_in": 7776000})
		case "/me":
			if r.URL.Query().Get("access_token") != "AT" {
				_ = json.NewEncoder(w).Encode(map[string]any{"error": 100016})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"client_id": "id", "openid": "UID_ABC"})
		}
	}))
	defer srv.Close()
	old := Endpoints
	Endpoints.QQ.Authorize, Endpoints.QQ.Token, Endpoints.QQ.Me = srv.URL+"/authorize", srv.URL+"/token", srv.URL+"/me"
	defer func() { Endpoints = old }()

	p := Provider{Key: "qq", Type: TypeQQ, ClientID: "id", ClientSecret: "sec"}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	au, err := p.AuthorizeURL(context.Background(), "https://x/cb", "st")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(au)
	if u.Query().Get("response_type") != "code" || u.Query().Get("scope") != "get_user_info" || u.Query().Get("state") != "st" {
		t.Errorf("QQ 授权地址参数错误: %s", au)
	}
	id, err := p.Exchange(context.Background(), "c1", "https://x/cb")
	if err != nil || id.String() != "qq:UID_ABC" || gotRedirect != "https://x/cb" {
		t.Fatalf("QQ 换取身份: %+v %v", id, err)
	}
	if _, err := p.Exchange(context.Background(), "bad", "https://x/cb"); err == nil {
		t.Error("授权码错误应失败")
	}
}

func TestGoogleDefaultsIssuer(t *testing.T) {
	p := Provider{Key: "google", Type: TypeGoogle, ClientID: "i", ClientSecret: "s"}
	if err := p.Validate(); err != nil || p.Issuer != GoogleIssuer {
		t.Errorf("Google 应自动使用固定 Issuer: %v %q", err, p.Issuer)
	}
}

func TestGoogleUsesBuiltinEndpoints(t *testing.T) {
	p := Provider{Key: "google", Type: TypeGoogle, ClientID: "i", ClientSecret: "s"}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	// 校验后 Issuer 已被填充，仍应使用内置端点而不访问发现文档
	u, err := p.AuthorizeURL(context.Background(), "https://g.example.com/cb", "st")
	if err != nil || !strings.HasPrefix(u, googleConfig.AuthorizationEndpoint+"?") || !strings.Contains(u, "prompt=select_account") {
		t.Errorf("Google 应使用内置授权端点: %v %q", err, u)
	}
}

func TestProviderProxy(t *testing.T) {
	// 代理服务器直接应答：提供方地址使用不可解析的域名，只有经过代理才能成功
	var proxied []string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied = append(proxied, r.URL.Host+r.URL.Path)
		switch r.URL.Path {
		case "/token":
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "AT"})
		case "/me":
			_ = json.NewEncoder(w).Encode(map[string]string{"openid": "UID_P"})
		}
	}))
	defer proxy.Close()
	old := Endpoints
	Endpoints.QQ.Token, Endpoints.QQ.Me = "http://qq.invalid/token", "http://qq.invalid/me"
	defer func() { Endpoints = old }()

	p := Provider{Key: "qq", Type: TypeQQ, ClientID: "id", ClientSecret: "sec", Proxy: " " + proxy.URL + " "}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	id, err := p.Exchange(context.Background(), "c1", "https://x/cb")
	if err != nil || id.String() != "qq:UID_P" || strings.Join(proxied, ",") != "qq.invalid/token,qq.invalid/me" {
		t.Fatalf("应经代理换取身份: %+v %v %q", id, err, proxied)
	}

	bad := Provider{Key: "qq", Type: TypeQQ, ClientID: "id", ClientSecret: "sec", Proxy: "127.0.0.1:7890"}
	if err := bad.Validate(); err == nil {
		t.Error("缺少协议的代理地址应校验失败")
	}
}

// TestLoginSecrets 验证 Secrets 按登录方式标识返回 Client Secret 明文，Masked 不影响原配置。
func TestLoginSecrets(t *testing.T) {
	c := LoginConfig{Providers: []Provider{{Key: "github", ClientSecret: "s1"}, {Key: "corp", ClientSecret: "s2"}}}
	if c.Masked().Providers[0].ClientSecret != "********" {
		t.Fatal("Masked 应脱敏 Client Secret")
	}
	got := c.Secrets()
	if len(got) != 2 || got["github"] != "s1" || got["corp"] != "s2" {
		t.Fatalf("应按标识返回 Client Secret 明文: %v", got)
	}
}

// 身份表来自导入的文件：通配、空标识以及会被当成备注或拆成多项的写法都不能转换为允许列表项。
func TestLuckyEntryRejectsWildcard(t *testing.T) {
	for _, id := range []LuckyIdentity{
		{Type: "github", ID: "*"},
		{Type: "github", ID: ""},
		{Type: "", ID: "1"},
		{Type: "github", ID: " * "},
		{Type: "github", ID: "1 2"},
		{Type: "github", ID: "1\ngithub:*"},
		{Type: "github", ID: "1\t"},
		{Type: "github", ID: "1#x"},
		{Type: "github:*", ID: "1"},
		{Type: "git hub", ID: "1"},
	} {
		if e, ok := id.Entry(); ok {
			t.Errorf("%+v 不应转换为允许列表项，实际 %q", id, e)
		}
	}
	e, ok := LuckyIdentity{Type: "GitHub", ID: "15172899", Name: "a\nb", EMail: "x@y.z"}.Entry()
	if !ok || e != "github:15172899 # a b x@y.z" {
		t.Fatalf("有效身份: %q %v", e, ok)
	}
	// 含 * 的普通标识（不是单独一个 *）不受影响
	if e, ok := (LuckyIdentity{Type: "qq", ID: "UID*X"}).Entry(); !ok || e != "qq:UID*X" {
		t.Fatalf("含星号的标识: %q %v", e, ok)
	}
	ids := LuckyIdentities{List: []LuckyIdentity{
		{Key: "a", Enable: true, Type: "github", ID: "1"},
		{Key: "b", Enable: true, Type: "github", ID: "*"},
		{Key: "c", Enable: false, Type: "github", ID: "*"},
	}}
	got, missing := ids.Resolve(nil, true)
	if strings.Join(got, "|") != "github:1" || missing != 1 {
		t.Fatalf("解析全部身份应跳过通配: %q missing=%d", got, missing)
	}
	got, missing = ids.Resolve([]string{"a", "b", "c", "zz"}, false)
	if strings.Join(got, "|") != "github:1" || missing != 2 {
		t.Fatalf("按 Key 解析应跳过通配: %q missing=%d", got, missing)
	}
}
