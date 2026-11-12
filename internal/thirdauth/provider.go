// Package thirdauth 实现自建的第三方登录（GitHub、Google、QQ 与通用 OIDC）。
// 使用用户自己申请的 OAuth 应用，不经过任何中转服务器；管理界面登录与 Web 服务站点认证共用本包。
package thirdauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/outbound"
)

// 提供方类型
const (
	TypeGitHub = "github"
	TypeGoogle = "google"
	TypeQQ     = "qq"
	TypeOIDC   = "oidc"
)

// GoogleIssuer 是 Google 类型固定使用的 OIDC 签发者。
const GoogleIssuer = "https://accounts.google.com"

// Provider 是一个第三方登录配置。
type Provider struct {
	// Key 同时作为身份前缀，例如 key 为 github 时身份写作 github:用户名
	Key          string `json:"key"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	// Issuer 为 OIDC 签发者地址，例如 https://accounts.google.com（Google 类型自动填充）
	Issuer string `json:"issuer"`
	// Proxy 为服务器访问该提供方（换取令牌、查询用户信息、读取发现文档）时使用的代理，
	// 支持 http://、https://、socks5://，留空直连。只作用于本登录方式，不影响 Gately 其他功能
	Proxy string `json:"proxy"`
}

// Endpoints 是各提供方的固定端点，测试时可替换。
var Endpoints = struct {
	GitHub struct{ Authorize, Token, User string }
	QQ     struct{ Authorize, Token, Me string }
}{
	GitHub: struct{ Authorize, Token, User string }{
		Authorize: "https://github.com/login/oauth/authorize",
		Token:     "https://github.com/login/oauth/access_token",
		User:      "https://api.github.com/user",
	},
	QQ: struct{ Authorize, Token, Me string }{
		Authorize: "https://graph.qq.com/oauth2.0/authorize",
		Token:     "https://graph.qq.com/oauth2.0/token",
		Me:        "https://graph.qq.com/oauth2.0/me",
	},
}

// Identity 是登录成功后得到的身份。
type Identity struct {
	// Provider 为提供方 Key
	Provider string
	// Primary 为展示与提示用的主标识：GitHub 为登录名，OIDC 为已验证邮箱（缺失时为 sub），QQ 为 openid
	Primary string
	// Aliases 为同一账号的其他可匹配标识（GitHub 数字 ID、OIDC 的 sub 等）。
	// 按稳定 ID 授权比按用户名更安全，用户名可被改名后由他人重新注册
	Aliases []string
}

// String 返回 提供方:主标识。
func (i Identity) String() string {
	return i.Provider + ":" + i.Primary
}

// Validate 校验并规范化提供方配置。
func (p *Provider) Validate() error {
	p.Key = strings.ToLower(strings.TrimSpace(p.Key))
	if p.Key == "" || strings.ContainsAny(p.Key, ": /#") {
		return errors.New("第三方登录的标识不能为空，且不能包含冒号、空格、斜杠或 #")
	}
	if p.ClientID == "" || p.ClientSecret == "" {
		return fmt.Errorf("第三方登录 %s 缺少 Client ID 或 Secret", p.Key)
	}
	p.Proxy = strings.TrimSpace(p.Proxy)
	if p.Proxy != "" {
		u, err := url.Parse(p.Proxy)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" && u.Scheme != "socks5h") {
			return fmt.Errorf("第三方登录 %s 的代理地址需为 http://、https:// 或 socks5:// 开头的地址，例如 http://127.0.0.1:7890", p.Key)
		}
	}
	switch p.Type {
	case TypeGitHub, TypeQQ:
	case TypeGoogle:
		p.Issuer = GoogleIssuer
	case TypeOIDC:
		if !strings.HasPrefix(p.Issuer, "https://") {
			return fmt.Errorf("第三方登录 %s 的 Issuer 需为 https 地址", p.Key)
		}
	default:
		return fmt.Errorf("未知的第三方登录类型 %q", p.Type)
	}
	return nil
}

// DisplayName 返回登录按钮上的名称。
func (p Provider) DisplayName() string {
	if p.Name != "" {
		return p.Name
	}
	return p.Key
}

// client 返回访问提供方使用的 HTTP 客户端：配置了代理时走代理，否则沿用默认客户端。
func (p Provider) client() *http.Client {
	if p.Proxy == "" {
		return outbound.DefaultClient()
	}
	u, err := url.Parse(p.Proxy)
	if err != nil {
		// Validate 已校验过地址，这里仅防御异常配置
		return outbound.DefaultClient()
	}
	return &http.Client{Transport: outbound.ProxyTransport(u)}
}

func (p Provider) isOIDC() bool {
	return p.Type == TypeOIDC || p.Type == TypeGoogle
}

func (p Provider) issuer() string {
	if p.Type == TypeGoogle && p.Issuer == "" {
		return GoogleIssuer
	}
	return p.Issuer
}

// oidcConfig 是 OIDC 发现文档中用到的端点。
type oidcConfig struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
}

var (
	oidcCacheMu sync.Mutex
	oidcCache   = map[string]oidcConfig{}
)

// googleConfig 是 Google 固定的 OIDC 端点；内置后无需在发起登录时由服务器访问 accounts.google.com，
// 服务器无法直连 Google 的网络环境下，点击登录也能立即跳转到授权页。
var googleConfig = oidcConfig{
	AuthorizationEndpoint: "https://accounts.google.com/o/oauth2/v2/auth",
	TokenEndpoint:         "https://oauth2.googleapis.com/token",
	UserinfoEndpoint:      "https://openidconnect.googleapis.com/v1/userinfo",
}

// oidcEndpoints 返回提供方的 OIDC 端点：Google 类型始终使用内置端点，其余走发现文档。
// Validate 会把 Google 的 Issuer 固定填为 accounts.google.com，因此这里只按类型判断，不能依赖 Issuer 是否为空。
func (p Provider) oidcEndpoints(ctx context.Context) (oidcConfig, error) {
	if p.Type == TypeGoogle {
		return googleConfig, nil
	}
	return discoverOIDC(ctx, p.client(), p.issuer())
}

// discoveryTimeout 是读取发现文档的超时，避免身份提供方不可达时登录请求一直挂起。
const discoveryTimeout = 10 * time.Second

func discoverOIDC(ctx context.Context, hc *http.Client, issuer string) (oidcConfig, error) {
	ctx, cancel := context.WithTimeout(ctx, discoveryTimeout)
	defer cancel()
	oidcCacheMu.Lock()
	c, ok := oidcCache[issuer]
	oidcCacheMu.Unlock()
	if ok {
		return c, nil
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(issuer, "/")+"/.well-known/openid-configuration", nil)
	resp, err := hc.Do(req)
	if err != nil {
		return c, err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&c); err != nil {
		return c, err
	}
	if c.AuthorizationEndpoint == "" || c.TokenEndpoint == "" || c.UserinfoEndpoint == "" {
		return c, errors.New("OIDC 发现文档不完整")
	}
	oidcCacheMu.Lock()
	oidcCache[issuer] = c
	oidcCacheMu.Unlock()
	return c, nil
}

// AuthorizeURL 生成跳转到身份提供方的地址。
func (p Provider) AuthorizeURL(ctx context.Context, redirect, state string) (string, error) {
	v := url.Values{"client_id": {p.ClientID}, "redirect_uri": {redirect}, "state": {state}}
	switch {
	case p.Type == TypeGitHub:
		v.Set("scope", "read:user user:email")
		return Endpoints.GitHub.Authorize + "?" + v.Encode(), nil
	case p.Type == TypeQQ:
		v.Set("response_type", "code")
		v.Set("scope", "get_user_info")
		return Endpoints.QQ.Authorize + "?" + v.Encode(), nil
	default:
		c, err := p.oidcEndpoints(ctx)
		if err != nil {
			return "", err
		}
		v.Set("response_type", "code")
		v.Set("scope", "openid email profile")
		if p.Type == TypeGoogle {
			// 每次都弹出 Google 账号选择页，便于在多个账号间切换
			v.Set("prompt", "select_account")
		}
		return c.AuthorizationEndpoint + "?" + v.Encode(), nil
	}
}

// Exchange 用授权码换取访问令牌并查询身份。
func (p Provider) Exchange(ctx context.Context, code, redirect string) (Identity, error) {
	switch {
	case p.Type == TypeQQ:
		return p.exchangeQQ(ctx, code, redirect)
	case p.isOIDC():
		c, err := p.oidcEndpoints(ctx)
		if err != nil {
			return Identity{}, err
		}
		tok, err := p.fetchToken(ctx, c.TokenEndpoint, code, redirect)
		if err != nil {
			return Identity{}, err
		}
		return p.oidcIdentity(ctx, c.UserinfoEndpoint, tok)
	default:
		tok, err := p.fetchToken(ctx, Endpoints.GitHub.Token, code, redirect)
		if err != nil {
			return Identity{}, err
		}
		return p.githubIdentity(ctx, tok)
	}
}

// fetchToken 以标准 authorization_code 流程换取访问令牌（GitHub 与 OIDC 共用）。
func (p Provider) fetchToken(ctx context.Context, tokenURL, code, redirect string) (string, error) {
	form := url.Values{"client_id": {p.ClientID}, "client_secret": {p.ClientSecret}, "code": {code}, "redirect_uri": {redirect}, "grant_type": {"authorization_code"}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := p.client().Do(req)
	if err != nil {
		return "", err
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		Desc        string `json:"error_description"`
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&tok)
	resp.Body.Close()
	if err != nil {
		return "", fmt.Errorf("解析令牌响应失败: %w", err)
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("获取令牌失败: %s %s", tok.Error, tok.Desc)
	}
	return tok.AccessToken, nil
}

// getJSON 以 Bearer 令牌（可为空）请求并解析 JSON。
func getJSON(ctx context.Context, hc *http.Client, target, bearer string, out any) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}

func (p Provider) githubIdentity(ctx context.Context, token string) (Identity, error) {
	var u struct {
		Login string `json:"login"`
		ID    int64  `json:"id"`
	}
	if err := getJSON(ctx, p.client(), Endpoints.GitHub.User, token, &u); err != nil {
		return Identity{}, fmt.Errorf("解析用户信息失败: %w", err)
	}
	if u.Login == "" {
		return Identity{}, errors.New("未获取到 GitHub 用户名")
	}
	id := Identity{Provider: p.Key, Primary: u.Login}
	if u.ID != 0 {
		id.Aliases = []string{strconv.FormatInt(u.ID, 10)}
	}
	return id, nil
}

func (p Provider) oidcIdentity(ctx context.Context, userinfo, token string) (Identity, error) {
	var u struct {
		Email         string          `json:"email"`
		EmailVerified json.RawMessage `json:"email_verified"`
		Sub           string          `json:"sub"`
	}
	if err := getJSON(ctx, p.client(), userinfo, token, &u); err != nil {
		return Identity{}, fmt.Errorf("解析用户信息失败: %w", err)
	}
	if u.Sub == "" && u.Email == "" {
		return Identity{}, errors.New("未获取到用户标识")
	}
	// 未验证的邮箱不可信，只使用 sub；部分提供方把 email_verified 编码为字符串，只有明确为 true 时才采用邮箱
	verified := string(u.EmailVerified) == "true" || string(u.EmailVerified) == `"true"`
	if u.Email != "" && verified {
		id := Identity{Provider: p.Key, Primary: strings.ToLower(u.Email)}
		if u.Sub != "" {
			id.Aliases = []string{u.Sub}
		}
		return id, nil
	}
	if u.Sub == "" {
		return Identity{}, errors.New("未获取到用户标识")
	}
	return Identity{Provider: p.Key, Primary: u.Sub}, nil
}

// exchangeQQ 走 QQ 互联流程：换取令牌后通过 /me 取得 openid。
// openid 对每个 QQ 互联应用唯一，换用自己的应用后与 Lucky 中转应用的值不同，需要重新授权一次。
func (p Provider) exchangeQQ(ctx context.Context, code, redirect string) (Identity, error) {
	q := url.Values{"grant_type": {"authorization_code"}, "client_id": {p.ClientID}, "client_secret": {p.ClientSecret},
		"code": {code}, "redirect_uri": {redirect}, "fmt": {"json"}}
	var tok struct {
		AccessToken string `json:"access_token"`
		Error       int    `json:"error"`
		Desc        string `json:"error_description"`
	}
	if err := getJSON(ctx, p.client(), Endpoints.QQ.Token+"?"+q.Encode(), "", &tok); err != nil {
		return Identity{}, fmt.Errorf("解析令牌响应失败: %w", err)
	}
	if tok.AccessToken == "" {
		return Identity{}, fmt.Errorf("获取令牌失败: %d %s", tok.Error, tok.Desc)
	}
	var me struct {
		OpenID string `json:"openid"`
		Error  int    `json:"error"`
		Desc   string `json:"error_description"`
	}
	if err := getJSON(ctx, p.client(), Endpoints.QQ.Me+"?"+url.Values{"access_token": {tok.AccessToken}, "fmt": {"json"}}.Encode(), "", &me); err != nil {
		return Identity{}, fmt.Errorf("解析用户信息失败: %w", err)
	}
	if me.OpenID == "" {
		return Identity{}, fmt.Errorf("未获取到 QQ openid: %d %s", me.Error, me.Desc)
	}
	return Identity{Provider: p.Key, Primary: me.OpenID}, nil
}

// HTTPTimeout 是一次登录回调中访问身份提供方的总超时。
const HTTPTimeout = 20 * time.Second
