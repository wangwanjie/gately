package webservice

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/geoip"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/webpage"
)

// OIDCSettings 是内置 OIDC 身份提供方的配置：把共享用户以 OpenID Connect 协议提供给 Gitea、OpenList 等应用登录，
// 相当于 Authelia 的 identity_providers.oidc。
type OIDCSettings struct {
	Enable bool `json:"enable"`
	// Issuer 为签发者地址（只含协议、域名与可选端口），例如 https://auth.example.com，需由 Gately 的某个监听提供
	Issuer  string       `json:"issuer"`
	Clients []OIDCClient `json:"clients"`
	// SigningKey 为 PEM 格式的 RSA 私钥，首次启用时自动生成；接口不返回
	SigningKey string `json:"signingKey,omitempty"`
}

// OIDCClient 是一个接入的应用（OIDC 客户端）。
type OIDCClient struct {
	// ID 即应用中填写的 Client ID
	ID   string `json:"id"`
	Name string `json:"name"`
	// Secret 为客户端密钥的 bcrypt 哈希；接口提交明文时自动哈希，读取时脱敏。公共客户端没有密钥
	Secret string `json:"secret"`
	// Public 为 true 表示公共客户端（单页应用、手机 App），不使用密钥，必须使用 PKCE
	Public bool `json:"public"`
	// RedirectURIs 为允许的回调地址，必须与应用发来的 redirect_uri 完全一致
	RedirectURIs []string `json:"redirectUris"`
	// Allow 为允许登录的共享用户，每项为用户名或 group:分组名；留空表示全部共享用户
	Allow  []string `json:"allow"`
	Remark string   `json:"remark"`
}

// OIDC 协议的保留路径与令牌时长
const (
	oidcPath          = "/__gately_oidc"
	oidcDiscoveryPath = "/.well-known/openid-configuration"
	oidcAuthorizePath = oidcPath + "/authorize"
	oidcTokenPath     = oidcPath + "/token"
	oidcUserinfoPath  = oidcPath + "/userinfo"
	oidcJWKSPath      = oidcPath + "/jwks"
	oidcLogoutPath    = oidcPath + "/logout"
	oidcLoginPath     = oidcPath + "/login"

	// oidcReauthParam 为强制重新登录（prompt=login、max_age）时附加在授权地址上的签名标记
	oidcReauthParam = "gately_reauth"

	oidcCodeTTL    = 2 * time.Minute
	oidcTokenTTL   = time.Hour
	oidcRefreshTTL = 30 * 24 * time.Hour
	// oidcMinSecret 为明文客户端密钥的最短长度
	oidcMinSecret = 16
	// oidcMaxForm 为各接口请求体的大小上限，oidcMaxParam 为 state、nonce 的长度上限
	oidcMaxForm  = 64 << 10
	oidcMaxParam = 512
	// oidcReauthTTL 为强制重新登录标记的有效期，需在此时间内完成登录
	oidcReauthTTL = 30 * time.Minute
)

// oidcScopes 为支持的 scope；应用请求的其他 scope 会被忽略。
var oidcScopes = []string{"openid", "profile", "email", "groups", "offline_access"}

// oidcEnv 是 OIDC 身份提供方的运行时副本，设置变更时整体替换。
type oidcEnv struct {
	issuer string
	host   string
	// secure 为 true 表示签发者地址为 https，协议路径不在明文连接上提供
	secure  bool
	key     *rsa.PrivateKey
	kid     string
	clients map[string]OIDCClient
}

// oidcHolder 保存当前生效的 OIDC 配置，nil 表示未启用。
type oidcHolder struct{ p atomic.Pointer[oidcEnv] }

func (h *oidcHolder) get() *oidcEnv { return h.p.Load() }

// set 按配置构建运行时副本；未启用或密钥无效时停用。
func (h *oidcHolder) set(s OIDCSettings) error {
	if !s.Enable || s.Issuer == "" {
		h.p.Store(nil)
		return nil
	}
	key, err := parseOIDCKey(s.SigningKey)
	if err != nil {
		h.p.Store(nil)
		return err
	}
	u, err := url.Parse(s.Issuer)
	if err != nil {
		h.p.Store(nil)
		return err
	}
	env := &oidcEnv{issuer: strings.TrimRight(s.Issuer, "/"), host: hostOnly(u.Host), secure: u.Scheme == "https", key: key, kid: jwkThumbprint(&key.PublicKey), clients: map[string]OIDCClient{}}
	for _, c := range s.Clients {
		env.clients[c.ID] = c
	}
	h.p.Store(env)
	return nil
}

// ---- 配置校验 ----

// newOIDCKey 生成 2048 位 RSA 签名密钥（PKCS#8 PEM）。
func newOIDCKey() (string, error) {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", err
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), nil
}

func parseOIDCKey(s string) (*rsa.PrivateKey, error) {
	b, _ := pem.Decode([]byte(s))
	if b == nil {
		return nil, errors.New("OIDC 签名密钥无效")
	}
	if k, err := x509.ParsePKCS1PrivateKey(b.Bytes); err == nil {
		return k, nil
	}
	k, err := x509.ParsePKCS8PrivateKey(b.Bytes)
	if err != nil {
		return nil, errors.New("OIDC 签名密钥无效")
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("OIDC 签名密钥需为 RSA 私钥")
	}
	return rk, nil
}

// validOIDCClientID 限制 Client ID 只含 URL 安全字符，避免在 Basic 认证与表单中产生歧义。
func validOIDCClientID(id string) bool {
	if id == "" || len(id) > 100 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-._~", c)) {
			return false
		}
	}
	return true
}

// normalizeOIDC 校验 OIDC 设置：签发者地址、客户端与回调地址；明文密钥哈希保存，脱敏占位符恢复旧值，签名密钥沿用旧值或首次生成。
func normalizeOIDC(s *OIDCSettings, old OIDCSettings) error {
	s.SigningKey = old.SigningKey
	s.Issuer = strings.TrimRight(strings.TrimSpace(s.Issuer), "/")
	if s.Issuer != "" {
		u, err := url.Parse(s.Issuer)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("OIDC 签发者地址需为 http(s)://域名[:端口]，不能带路径")
		}
		s.Issuer = u.Scheme + "://" + strings.ToLower(u.Host)
	}
	if s.Enable && s.Issuer == "" {
		return errors.New("启用 OIDC 身份提供方需要填写签发者地址")
	}
	// 未修改密钥的机密客户端先统一成占位符，再按 Client ID 恢复原密钥；改了 Client ID 时按位置对应原条目
	for i := range s.Clients {
		if c := &s.Clients[i]; !c.Public && c.Secret == "" {
			c.Secret = crud.SecretMask
		}
	}
	crud.KeepSecrets(old.Clients, s.Clients, func(c *OIDCClient) string { return c.ID }, func(c *OIDCClient) *string { return &c.Secret })
	seen := map[string]bool{}
	for i := range s.Clients {
		c := &s.Clients[i]
		c.ID = strings.TrimSpace(c.ID)
		if !validOIDCClientID(c.ID) {
			return fmt.Errorf("OIDC 客户端 ID %q 无效（只能包含字母、数字和 - . _ ~）", c.ID)
		}
		if seen[c.ID] {
			return fmt.Errorf("OIDC 客户端 ID %s 重复", c.ID)
		}
		seen[c.ID] = true
		c.Name = strings.TrimSpace(c.Name)
		c.Remark = strings.TrimSpace(c.Remark)
		if c.Public {
			c.Secret = ""
		} else {
			switch {
			case c.Secret == "" || c.Secret == crud.SecretMask:
				// 占位符没能对应到原客户端（新增客户端未填写密钥）
				c.Secret = ""
			case strings.HasPrefix(c.Secret, "$2"):
				if _, err := bcrypt.Cost([]byte(c.Secret)); err != nil {
					return fmt.Errorf("OIDC 客户端 %s 的密钥哈希无效", c.ID)
				}
			default:
				if len(c.Secret) < oidcMinSecret {
					return fmt.Errorf("OIDC 客户端 %s 的密钥至少需要 %d 个字符", c.ID, oidcMinSecret)
				}
				h, err := bcrypt.GenerateFromPassword([]byte(c.Secret), bcrypt.DefaultCost)
				if err != nil {
					return err
				}
				c.Secret = string(h)
			}
			if c.Secret == "" {
				return fmt.Errorf("请为 OIDC 客户端 %s 设置密钥", c.ID)
			}
		}
		c.RedirectURIs = uniqueList(cleanList(c.RedirectURIs))
		if len(c.RedirectURIs) == 0 {
			return fmt.Errorf("OIDC 客户端 %s 至少需要一个回调地址", c.ID)
		}
		for _, r := range c.RedirectURIs {
			u, err := url.Parse(r)
			if err != nil || u.Scheme == "" || u.Fragment != "" || ((u.Scheme == "http" || u.Scheme == "https") && u.Host == "") {
				return fmt.Errorf("OIDC 客户端 %s 的回调地址 %q 无效", c.ID, r)
			}
		}
		c.Allow = uniqueList(cleanList(c.Allow))
		for _, a := range c.Allow {
			if strings.ContainsAny(a, " \t") || a == "group:" {
				return fmt.Errorf("OIDC 客户端 %s 的允许用户 %q 无效", c.ID, a)
			}
		}
	}
	if s.Enable && s.SigningKey == "" {
		k, err := newOIDCKey()
		if err != nil {
			return err
		}
		s.SigningKey = k
	}
	return nil
}

// maskOIDC 返回脱敏后的 OIDC 设置，用于接口输出。
func maskOIDC(s OIDCSettings) OIDCSettings {
	s.SigningKey = ""
	s.Clients = append([]OIDCClient{}, s.Clients...)
	for i := range s.Clients {
		s.Clients[i].Secret = crud.Mask(s.Clients[i].Secret)
	}
	return s
}

// ---- JWT（RS256） ----

var b64 = base64.RawURLEncoding

func (e *oidcEnv) signJWT(typ string, claims map[string]any) string {
	h, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": e.kid, "typ": typ})
	c, _ := json.Marshal(claims)
	in := b64.EncodeToString(h) + "." + b64.EncodeToString(c)
	sum := sha256.Sum256([]byte(in))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, e.key, crypto.SHA256, sum[:])
	return in + "." + b64.EncodeToString(sig)
}

// verifyJWT 校验本签发者签出的 JWT：签名、类型、签发者与有效期。
func (e *oidcEnv) verifyJWT(token, typ string) (map[string]any, error) {
	return e.parseJWT(token, typ, true)
}

// parseJWT 校验签名、类型与签发者；checkExp 为 false 时不检查有效期（退出时的 id_token_hint 允许已过期）。
func (e *oidcEnv) parseJWT(token, typ string, checkExp bool) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed token")
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("malformed token")
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(&e.key.PublicKey, crypto.SHA256, sum[:], sig) != nil {
		return nil, errors.New("invalid signature")
	}
	var h struct{ Alg, Typ string }
	hb, _ := b64.DecodeString(parts[0])
	if json.Unmarshal(hb, &h) != nil || h.Alg != "RS256" || h.Typ != typ {
		return nil, errors.New("unexpected token type")
	}
	var c map[string]any
	cb, _ := b64.DecodeString(parts[1])
	if json.Unmarshal(cb, &c) != nil {
		return nil, errors.New("malformed token")
	}
	if c["iss"] != e.issuer {
		return nil, errors.New("unexpected issuer")
	}
	exp, _ := c["exp"].(float64)
	if checkExp && int64(exp) < time.Now().Unix() {
		return nil, errors.New("token expired")
	}
	return c, nil
}

// jwk 返回公钥的 JWK 表示。
func (e *oidcEnv) jwk() map[string]string {
	pub := &e.key.PublicKey
	return map[string]string{"kty": "RSA", "use": "sig", "alg": "RS256", "kid": e.kid,
		"n": b64.EncodeToString(pub.N.Bytes()), "e": b64.EncodeToString(big.NewInt(int64(pub.E)).Bytes())}
}

// jwkThumbprint 按 RFC 7638 计算公钥指纹，用作 kid：换密钥后应用能据此识别。
func jwkThumbprint(pub *rsa.PublicKey) string {
	s := fmt.Sprintf(`{"e":"%s","kty":"RSA","n":"%s"}`, b64.EncodeToString(big.NewInt(int64(pub.E)).Bytes()), b64.EncodeToString(pub.N.Bytes()))
	sum := sha256.Sum256([]byte(s))
	return b64.EncodeToString(sum[:])
}

// ---- 协议处理 ----

// oidcCode 是授权码的内容，签名后作为 code 返回给应用，只能兑换一次。
type oidcCode struct {
	Client   string `json:"c"`
	Redirect string `json:"r"`
	// User 为共享用户的不可变 ID
	User      string `json:"u"`
	Cred      string `json:"k"`
	Scope     string `json:"s"`
	Nonce     string `json:"n,omitempty"`
	Challenge string `json:"p,omitempty"`
	AuthTime  int64  `json:"t"`
}

// oidcRefresh 是刷新令牌的内容；用户修改密码或 TOTP、客户端被删除后失效。
type oidcRefresh struct {
	Client string `json:"c"`
	// User 为共享用户的不可变 ID
	User     string `json:"u"`
	Cred     string `json:"k"`
	Scope    string `json:"s"`
	AuthTime int64  `json:"t"`
}

// oidcTarget 判断请求是否为签发者域名上的 OIDC 协议路径：是则返回当前配置，否则返回 nil，交给站点继续处理。
func (a *authenticator) oidcTarget(r *http.Request) *oidcEnv {
	e := a.oidc.get()
	if e == nil || hostOnly(r.Host) != e.host {
		return nil
	}
	p := r.URL.Path
	if p != oidcDiscoveryPath && p != oidcPath && !strings.HasPrefix(p, oidcPath+"/") {
		return nil
	}
	return e
}

// requestIsHTTPS 判断访客是否经 HTTPS 访问：TLS 直连，或由反向代理以 X-Forwarded-Proto: https 转发，
// 转发方需是本机、内网地址、全局设置中的可信代理，或经可信 PROXY 来源进入的连接（如公网 Gately 解开 TLS 后经 frp 隧道以明文转给本机）。
// 转发方按原始对端判断：客户端地址可能已按可信代理的请求头改写为访客地址。
// 浏览器自身不会发送该头，其他公网来源带上它也不被采信。
func requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
		return false
	}
	peer := peerOf(r)
	return peer.trusted || geoip.IsLocal(peer.addr) || viaTrustedProxy(r)
}

// serveOIDC 在签发者域名上处理 OIDC 协议路径（调用方已通过 oidcTarget 确认，并执行过签发者域名所在站点的访问控制）。
func (a *authenticator) serveOIDC(w http.ResponseWriter, r *http.Request, e *oidcEnv, log *logbuf.Logger) {
	p := r.URL.Path
	if a.banned(clientAddr(r).Addr()) {
		http.Error(w, "403 Forbidden", http.StatusForbidden)
		return
	}
	// 发现文档与公钥可以缓存，其余接口涉及登录态与令牌，禁止缓存
	if p == oidcDiscoveryPath || p == oidcJWKSPath {
		w.Header().Set("Cache-Control", "public, max-age=300")
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
	// 签发者为 https 时不在明文连接上提供登录页与令牌接口，避免密码与令牌明文传输、会话 Cookie 缺少 Secure：
	// GET、HEAD 永久跳转到签发者地址上的同一路径，其余请求直接拒绝
	if e.secure && !requestIsHTTPS(r) {
		switch {
		case r.Method == http.MethodGet || r.Method == http.MethodHead:
			http.Redirect(w, r, e.issuer+r.URL.RequestURI(), http.StatusMovedPermanently)
		case p == oidcTokenPath:
			oauthError(w, http.StatusBadRequest, "invalid_request", "the issuer requires HTTPS")
		default:
			http.NotFound(w, r)
		}
		return
	}
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, oidcMaxForm)
	}
	lg := log.With(logbuf.Fields{logbuf.FieldIP: clientAddr(r).Addr().String(), logbuf.FieldHost: e.host, logbuf.FieldMethod: r.Method, logbuf.FieldPath: p})
	// Passkey 登录与注册接口、管理页同样由 handleLogin 按路径分发
	if p == oidcPath+passkeyPath || strings.HasPrefix(p, oidcPath+passkeyPath+"/") {
		a.handleLogin(w, r, e.loginSite(r), clientAddr(r).Addr().String())
		return
	}
	switch p {
	case oidcDiscoveryPath:
		a.oidcCORS(w, r)
		writeJSON(w, http.StatusOK, e.discovery())
	case oidcJWKSPath:
		a.oidcCORS(w, r)
		writeJSON(w, http.StatusOK, map[string]any{"keys": []any{e.jwk()}})
	case oidcAuthorizePath:
		a.oidcAuthorize(w, r, e, lg)
	case oidcTokenPath:
		a.oidcCORS(w, r)
		a.oidcToken(w, r, e, lg)
	case oidcUserinfoPath:
		a.oidcCORS(w, r)
		a.oidcUserinfo(w, r, e)
	case oidcLogoutPath:
		a.oidcLogout(w, r, e)
	case oidcLoginPath, oidcPath + passwordSuffix:
		// 登录页与自助修改密码页都由 handleLogin 按路径分发
		a.handleLogin(w, r, e.loginSite(r), clientAddr(r).Addr().String())
	default:
		http.NotFound(w, r)
	}
}

// oidcCORS 允许浏览器中的单页应用跨域读取发现文档、公钥并兑换令牌（这些接口不依赖 Cookie）。
func (a *authenticator) oidcCORS(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") == "" {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// oauthError 按 RFC 6749 输出令牌接口与用户信息接口的错误。
func oauthError(w http.ResponseWriter, status int, code, desc string) {
	if status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer error=%q, error_description=%q`, code, desc))
	}
	writeJSON(w, status, map[string]string{"error": code, "error_description": desc})
}

func (e *oidcEnv) discovery() map[string]any {
	return map[string]any{
		"issuer":                                e.issuer,
		"authorization_endpoint":                e.issuer + oidcAuthorizePath,
		"token_endpoint":                        e.issuer + oidcTokenPath,
		"userinfo_endpoint":                     e.issuer + oidcUserinfoPath,
		"jwks_uri":                              e.issuer + oidcJWKSPath,
		"end_session_endpoint":                  e.issuer + oidcLogoutPath,
		"scopes_supported":                      oidcScopes,
		"response_types_supported":              []string{"code"},
		"response_modes_supported":              []string{"query"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post", "none"},
		"code_challenge_methods_supported":      []string{"S256"},
		"claims_supported":                      []string{"iss", "sub", "aud", "exp", "iat", "auth_time", "nonce", "azp", "at_hash", "name", "preferred_username", "nickname", "email", "email_verified", "groups"},
		"request_parameter_supported":           false,
		// 授权响应（含错误响应）都带 iss 参数（RFC 9207），应用可据此防御混淆攻击
		"authorization_response_iss_parameter_supported": true,
		"claims_parameter_supported":                     false,
	}
}

// loginSite 返回签发者域名上的虚拟站点：只接受共享用户，登录页挂在 OIDC 路径下，会话与普通站点隔离（单点登录域除外）。
func (e *oidcEnv) loginSite(r *http.Request) Site {
	s := Site{Key: "__oidc", WebAuth: true, SharedUsers: true, scope: "__oidc", authBase: oidcPath}
	// 只显示已登记客户端的名称，避免通过链接参数在登录页上展示任意文字
	if c, ok := e.clients[loginClientID(r)]; ok {
		s.loginClient = c.displayName()
	}
	return s
}

// loginClientID 返回登录页对应的客户端：取链接参数 client，提交表单（密码错误、输入验证码）时从 next 指向的授权地址中取 client_id。
func loginClientID(r *http.Request) string {
	if id := r.URL.Query().Get("client"); id != "" {
		return id
	}
	next := r.URL.Query().Get("next")
	if r.Method == http.MethodPost {
		next = r.FormValue("next")
	}
	if q, ok := strings.CutPrefix(next, oidcAuthorizePath+"?"); ok {
		if v, err := url.ParseQuery(q); err == nil {
			return v.Get("client_id")
		}
	}
	return ""
}

// grantedScope 返回应用请求的 scope 中受支持的部分，保持 openid 在首位。
func grantedScope(req string) string {
	var out []string
	for _, s := range oidcScopes {
		if slices.Contains(strings.Fields(req), s) {
			out = append(out, s)
		}
	}
	return strings.Join(out, " ")
}

func hasScope(scope, s string) bool { return slices.Contains(strings.Fields(scope), s) }

// clientAllows 判断共享用户是否可登录该客户端：允许列表为空表示全部共享用户，group: 前缀按分组匹配。
func clientAllows(c OIDCClient, u AuthUser) bool {
	if len(c.Allow) == 0 {
		return true
	}
	for _, a := range c.Allow {
		if g, ok := strings.CutPrefix(a, "group:"); ok {
			if slices.Contains(u.Groups, g) {
				return true
			}
		} else if a == u.Name {
			return true
		}
	}
	return false
}

func (c OIDCClient) displayName() string {
	if c.Name != "" {
		return c.Name
	}
	return c.ID
}

// oidcReauth 是强制重新登录时附加在授权地址上的标记：要求登录时间晚于 Since（秒），签名防止访客伪造以跳过重新登录。
// 同一秒内完成的登录无法与之前的会话区分，按未重新登录处理。
type oidcReauth struct {
	Since int64 `json:"t"`
}

// oidcAuthorize 处理授权请求：校验客户端与回调地址，未登录时转到登录页，登录后签发授权码跳回应用。
func (a *authenticator) oidcAuthorize(w http.ResponseWriter, r *http.Request, e *oidcEnv, lg *logbuf.Logger) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "405 Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	// 只拒绝超出大小上限的请求体；其余解析错误（如查询串中的分号）沿用能解析出的参数
	if err := r.ParseForm(); err != nil {
		if mbe := (*http.MaxBytesError)(nil); errors.As(err, &mbe) {
			oidcErrorPage(w, r, "请求无效", "授权请求过大，请回到应用重新发起登录。", "")
			return
		}
	}
	q := r.Form
	c, ok := e.clients[q.Get("client_id")]
	if !ok {
		oidcErrorPage(w, r, "应用未登记", "这个应用的 Client ID 没有在 Gately 中登记，请在 Web 服务设置的 OIDC 身份提供方中添加客户端。", q.Get("client_id"))
		return
	}
	redirect := q.Get("redirect_uri")
	if !slices.Contains(c.RedirectURIs, redirect) {
		oidcErrorPage(w, r, "回调地址不匹配", "应用发来的回调地址不在该客户端的允许列表中，请把下面的地址原样加入客户端的回调地址。", redirect)
		return
	}
	// 回调地址可信之后，其余错误按协议带回应用
	fail := func(code, desc string) {
		u, _ := url.Parse(redirect)
		v := u.Query()
		v.Set("error", code)
		v.Set("error_description", desc)
		if st := q.Get("state"); st != "" {
			v.Set("state", st)
		}
		v.Set("iss", e.issuer)
		u.RawQuery = v.Encode()
		http.Redirect(w, r, u.String(), http.StatusFound)
	}
	if len(q.Get("state")) > oidcMaxParam || len(q.Get("nonce")) > oidcMaxParam {
		// 过长的 state 不再带回应用
		q.Del("state")
		fail("invalid_request", "state or nonce is too long")
		return
	}
	if q.Get("request") != "" || q.Get("request_uri") != "" {
		fail("request_not_supported", "request objects are not supported")
		return
	}
	if q.Get("response_type") != "code" {
		fail("unsupported_response_type", "only response_type=code is supported")
		return
	}
	if m := q.Get("response_mode"); m != "" && m != "query" {
		fail("invalid_request", "only response_mode=query is supported")
		return
	}
	scope := grantedScope(q.Get("scope"))
	if !hasScope(scope, "openid") {
		fail("invalid_scope", "the openid scope is required")
		return
	}
	if c.Public {
		// 公共客户端无法保管刷新令牌，也无法安全轮换，不授予 offline_access
		scope = withoutScope(scope, "offline_access")
	}
	challenge := q.Get("code_challenge")
	if challenge != "" {
		if q.Get("code_challenge_method") != "S256" {
			fail("invalid_request", "only code_challenge_method=S256 is supported")
			return
		}
		if len(challenge) != 43 || !pkceChars(challenge) {
			fail("invalid_request", "invalid code_challenge")
			return
		}
	} else if c.Public {
		fail("invalid_request", "public clients must use PKCE")
		return
	}
	prompt := strings.Fields(q.Get("prompt"))
	none := slices.Contains(prompt, "none")
	if none && len(prompt) > 1 {
		fail("invalid_request", "prompt=none cannot be combined with other values")
		return
	}
	maxAge := int64(-1)
	if v := q.Get("max_age"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			fail("invalid_request", "invalid max_age")
			return
		}
		maxAge = n
	}
	// 去掉旧的重新登录标记，跳转登录页时按需附加新的
	var reauth oidcReauth
	reauthOK := a.verifyBlob("oidc-reauth", q.Get(oidcReauthParam), &reauth) == nil
	q.Del(oidcReauthParam)
	toLogin := func(marker bool) {
		// 登录后回到授权地址继续；POST 发起的授权请求改写为等价的 GET 地址
		if marker {
			q.Set(oidcReauthParam, a.signBlob("oidc-reauth", oidcReauth{Since: time.Now().Unix()}, oidcReauthTTL))
		}
		next := oidcAuthorizePath + "?" + q.Encode()
		http.Redirect(w, r, oidcLoginPath+"?next="+url.QueryEscape(next)+"&client="+url.QueryEscape(c.ID), http.StatusFound)
	}
	u, authTime, logged := a.oidcSessionUser(r, e)
	if !logged {
		if none {
			fail("login_required", "the user is not logged in")
			return
		}
		toLogin(false)
		return
	}
	// prompt=login 或超过 max_age 时要求重新输入密码；带着标记回来且登录时间晚于标记签发的那一秒，说明刚刚重新登录过
	fresh := reauthOK && authTime > reauth.Since
	if !fresh && (slices.Contains(prompt, "login") || (maxAge >= 0 && time.Now().Unix()-authTime > maxAge)) {
		if none {
			fail("login_required", "re-authentication is required")
			return
		}
		toLogin(true)
		return
	}
	if !clientAllows(c, u) {
		lg.With(logbuf.Fields{logbuf.FieldUser: u.Name}).Warnf("OIDC：%s 不在应用 %s 的允许列表中", u.Name, c.displayName())
		if none {
			// 静默登录不显示页面，按协议告知应用
			fail("access_denied", "the user is not allowed to use this application")
			return
		}
		// 提供“换个账号登录”：退出签发者上的登录后回到同一个授权请求
		switchURL := oidcLogoutPath + "?switch=" + url.QueryEscape(oidcAuthorizePath+"?"+q.Encode())
		webpage.Error(w, r, webpage.Problem{Code: http.StatusForbidden, Title: "没有使用该应用的权限", Lock: true,
			Detail: "当前登录的共享用户不在该客户端的允许列表中，请联系管理员授权，或退出后换一个账号登录。",
			Hint:   u.Name + " → " + c.displayName(), Action: switchURL, ActionText: "换个账号登录"})
		return
	}
	code := a.signBlob("oidc-code", oidcCode{Client: c.ID, Redirect: redirect, User: u.ID, Cred: a.sharedCred(u), Scope: scope,
		Nonce: q.Get("nonce"), Challenge: challenge, AuthTime: authTime}, oidcCodeTTL)
	lg.With(logbuf.Fields{logbuf.FieldUser: u.Name}).Infof("OIDC：%s 登录应用 %s", u.Name, c.displayName())
	target, _ := url.Parse(redirect)
	v := target.Query()
	v.Set("code", code)
	if st := q.Get("state"); st != "" {
		v.Set("state", st)
	}
	v.Set("iss", e.issuer)
	target.RawQuery = v.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound)
}

// withoutScope 从 scope 中去掉指定项。
func withoutScope(scope, drop string) string {
	return strings.Join(slices.DeleteFunc(strings.Fields(scope), func(s string) bool { return s == drop }), " ")
}

// pkceChars 判断字符串是否只含 RFC 7636 允许的字符（A-Z a-z 0-9 - . _ ~）。
func pkceChars(s string) bool {
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-._~", c)) {
			return false
		}
	}
	return true
}

// oidcErrorPage 在无法安全跳回应用时直接向用户展示错误，extra 为附加的原始信息（如收到的回调地址）。
func oidcErrorPage(w http.ResponseWriter, r *http.Request, title, detail, extra string) {
	webpage.Error(w, r, webpage.Problem{Code: http.StatusBadRequest, Title: title, Detail: detail, Hint: extra})
}

// oidcSessionUser 返回签发者域名上已登录的共享用户及登录时间（单点登录 Cookie 或签发者自身的会话）。
// 两种会话同时有效时取最近登录的一个，重新登录后的登录时间才能被 prompt=login、max_age 识别。
func (a *authenticator) oidcSessionUser(r *http.Request, e *oidcEnv) (AuthUser, int64, bool) {
	s := e.loginSite(r)
	var best AuthUser
	var bestTime int64
	found := false
	for _, name := range []string{cookieName(s), ssoCookie} {
		c, err := r.Cookie(name)
		if err != nil {
			continue
		}
		var sess siteSession
		exp, err := a.verifyBlobExp("session", c.Value, &sess)
		if err != nil || !sess.Shared || !a.validShared(r, s, &sess) {
			continue
		}
		if t := exp - int64(sessionTTL/time.Second); !found || t > bestTime {
			best, bestTime, found = a.shared.get().users[sess.User], t, true
		}
	}
	return best, bestTime, found
}

// clientAuth 校验令牌接口的客户端身份：client_secret_basic、client_secret_post，或公共客户端只带 client_id。
func (a *authenticator) clientAuth(r *http.Request, e *oidcEnv) (OIDCClient, string, bool) {
	id, secret, basic := r.BasicAuth()
	if basic {
		// RFC 6749 2.3.1：Basic 认证中的 ID 与密钥先做 application/x-www-form-urlencoded 编码
		if v, err := url.QueryUnescape(id); err == nil {
			id = v
		}
		if v, err := url.QueryUnescape(secret); err == nil {
			secret = v
		}
	} else {
		id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	c, ok := e.clients[id]
	if !ok {
		_ = bcrypt.CompareHashAndPassword(dummyHash(), []byte(secret))
		return OIDCClient{}, id, false
	}
	if c.Public {
		return c, id, secret == ""
	}
	return c, id, secret != "" && bcrypt.CompareHashAndPassword([]byte(c.Secret), []byte(secret)) == nil
}

// oidcToken 处理令牌接口：用授权码或刷新令牌换取 ID Token 与 Access Token。
func (a *authenticator) oidcToken(w http.ResponseWriter, r *http.Request, e *oidcEnv, lg *logbuf.Logger) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		oauthError(w, http.StatusMethodNotAllowed, "invalid_request", "use POST")
		return
	}
	// 请求体大小已在 serveOIDC 中限制
	if err := r.ParseForm(); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", "invalid form body")
		return
	}
	key := limitKey(clientAddr(r).Addr().String())
	if !a.reserve(key) {
		oauthError(w, http.StatusTooManyRequests, "slow_down", "too many failed attempts")
		return
	}
	c, id, ok := a.clientAuth(r, e)
	a.release(key, ok)
	if !ok {
		lg.Warnf("OIDC：客户端 %s 认证失败，请检查 Client ID 与 Client Secret", id)
		w.Header().Set("WWW-Authenticate", `Basic realm="Gately"`)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_client", "error_description": "client authentication failed"})
		return
	}
	switch r.PostForm.Get("grant_type") {
	case "authorization_code":
		a.oidcExchangeCode(w, r, e, c, lg)
	case "refresh_token":
		a.oidcRefreshToken(w, r, e, c, lg)
	default:
		oauthError(w, http.StatusBadRequest, "unsupported_grant_type", "only authorization_code and refresh_token are supported")
	}
}

func (a *authenticator) oidcExchangeCode(w http.ResponseWriter, r *http.Request, e *oidcEnv, c OIDCClient, lg *logbuf.Logger) {
	raw := r.PostForm.Get("code")
	var code oidcCode
	if err := a.verifyBlob("oidc-code", raw, &code); err != nil {
		grantError(w, lg, c, "invalid or expired authorization code")
		return
	}
	if code.Client != c.ID || code.Redirect != r.PostForm.Get("redirect_uri") {
		grantError(w, lg, c, "authorization code was issued to another client or redirect_uri")
		return
	}
	verifier := r.PostForm.Get("code_verifier")
	if code.Challenge != "" || verifier != "" {
		sum := sha256.Sum256([]byte(verifier))
		if len(verifier) < 43 || len(verifier) > 128 || !pkceChars(verifier) || subtle.ConstantTimeCompare([]byte(b64.EncodeToString(sum[:])), []byte(code.Challenge)) != 1 {
			grantError(w, lg, c, "PKCE verification failed")
			return
		}
	}
	if !a.consumeOnce("oidc-code|"+raw, oidcCodeTTL) {
		grantError(w, lg, c, "authorization code has already been used")
		return
	}
	u, ok := a.oidcUser(code.User, code.Cred)
	if !ok || !clientAllows(c, u) {
		grantError(w, lg, c, "the user is no longer allowed")
		return
	}
	a.issueTokens(w, e, c, u, code.Scope, code.Nonce, code.AuthTime, true)
}

func (a *authenticator) oidcRefreshToken(w http.ResponseWriter, r *http.Request, e *oidcEnv, c OIDCClient, lg *logbuf.Logger) {
	if c.Public {
		// 公共客户端不签发刷新令牌（无法轮换，泄露后可长期使用）
		grantError(w, lg, c, "refresh tokens are not issued to public clients")
		return
	}
	var rt oidcRefresh
	if err := a.verifyBlob("oidc-refresh", r.PostForm.Get("refresh_token"), &rt); err != nil || rt.Client != c.ID {
		grantError(w, lg, c, "invalid or expired refresh token")
		return
	}
	u, ok := a.oidcUser(rt.User, rt.Cred)
	if !ok || !clientAllows(c, u) {
		grantError(w, lg, c, "the user is no longer allowed")
		return
	}
	scope := rt.Scope
	// 刷新时可以缩小 scope，不能扩大
	if req := r.PostForm.Get("scope"); req != "" {
		var narrowed []string
		for _, s := range strings.Fields(grantedScope(req)) {
			if hasScope(rt.Scope, s) {
				narrowed = append(narrowed, s)
			}
		}
		scope = strings.Join(narrowed, " ")
	}
	// 刷新令牌不轮换：继续使用原令牌，到期或用户凭据变化后失效
	a.issueTokens(w, e, c, u, scope, "", rt.AuthTime, false)
}

// grantError 返回 invalid_grant 并记录日志，便于排查应用兑换令牌失败的原因。
func grantError(w http.ResponseWriter, lg *logbuf.Logger, c OIDCClient, desc string) {
	lg.Warnf("OIDC：应用 %s 兑换令牌失败：%s", c.displayName(), desc)
	oauthError(w, http.StatusBadRequest, "invalid_grant", desc)
}

// oidcUser 按 ID 返回凭据指纹仍然一致的共享用户：修改密码或 TOTP、改名、删除用户后，
// 已签发的授权码、刷新令牌与 Access Token 失效。
func (a *authenticator) oidcUser(id, cred string) (AuthUser, bool) {
	u, ok := a.shared.get().userByID(id)
	if !ok || subtle.ConstantTimeCompare([]byte(cred), []byte(a.sharedCred(u))) != 1 {
		return AuthUser{}, false
	}
	return u, true
}

// subject 返回用户在 OIDC 中的 sub：不可变的随机 ID，改名后不变，删除后再建同名用户也不会相同。
func (u AuthUser) subject() string {
	if u.ID != "" {
		return u.ID
	}
	return u.Name
}

// userClaims 按 scope 返回用户声明。
func userClaims(u AuthUser, scope string) map[string]any {
	m := map[string]any{"sub": u.subject()}
	if hasScope(scope, "profile") {
		name := u.DisplayName
		if name == "" {
			name = u.Name
		}
		m["name"], m["preferred_username"], m["nickname"] = name, u.Name, u.Name
	}
	if hasScope(scope, "email") && u.Email != "" {
		// 邮箱由管理员填写，视为已验证（Gitea 等应用依据它自动关联账号）
		m["email"], m["email_verified"] = u.Email, true
	}
	if hasScope(scope, "groups") {
		g := u.Groups
		if g == nil {
			g = []string{}
		}
		m["groups"] = g
	}
	return m
}

// issueTokens 签发 Access Token、ID Token，以及请求了 offline_access 时的刷新令牌。
func (a *authenticator) issueTokens(w http.ResponseWriter, e *oidcEnv, c OIDCClient, u AuthUser, scope, nonce string, authTime int64, newRefresh bool) {
	now := time.Now()
	exp := now.Add(oidcTokenTTL).Unix()
	// gately_cred 为用户凭据指纹：修改密码或 TOTP 后，用户信息接口不再接受此前签发的 Access Token
	access := e.signJWT("at+jwt", map[string]any{"iss": e.issuer, "sub": u.subject(), "aud": c.ID, "client_id": c.ID,
		"scope": scope, "iat": now.Unix(), "exp": exp, "jti": randomNonce(), "gately_cred": a.sharedCred(u)})
	atHash := sha256.Sum256([]byte(access))
	claims := userClaims(u, scope)
	claims["iss"], claims["aud"], claims["azp"] = e.issuer, c.ID, c.ID
	claims["iat"], claims["exp"], claims["at_hash"] = now.Unix(), exp, b64.EncodeToString(atHash[:16])
	if authTime > 0 {
		claims["auth_time"] = authTime
	}
	if nonce != "" {
		claims["nonce"] = nonce
	}
	resp := map[string]any{"access_token": access, "token_type": "Bearer", "expires_in": int(oidcTokenTTL / time.Second),
		"id_token": e.signJWT("JWT", claims), "scope": scope}
	// 只给机密客户端签发刷新令牌；公共客户端的 scope 在授权时已去掉 offline_access
	if newRefresh && !c.Public && hasScope(scope, "offline_access") {
		resp["refresh_token"] = a.signBlob("oidc-refresh", oidcRefresh{Client: c.ID, User: u.ID, Cred: a.sharedCred(u), Scope: scope, AuthTime: authTime}, oidcRefreshTTL)
	}
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, http.StatusOK, resp)
}

// oidcUserinfo 用 Access Token 返回用户声明。
func (a *authenticator) oidcUserinfo(w http.ResponseWriter, r *http.Request, e *oidcEnv) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	token := ""
	// Bearer 方案名不区分大小写（RFC 7235）
	if scheme, cred, ok := strings.Cut(r.Header.Get("Authorization"), " "); ok && strings.EqualFold(scheme, "Bearer") {
		token = cred
	} else if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			oauthError(w, http.StatusBadRequest, "invalid_request", "invalid form body")
			return
		}
		token = r.PostForm.Get("access_token")
	}
	claims, err := e.verifyJWT(strings.TrimSpace(token), "at+jwt")
	if err != nil {
		oauthError(w, http.StatusUnauthorized, "invalid_token", err.Error())
		return
	}
	sub, _ := claims["sub"].(string)
	cred, _ := claims["gately_cred"].(string)
	cid, _ := claims["client_id"].(string)
	scope, _ := claims["scope"].(string)
	u, ok := a.oidcUser(sub, cred)
	c, cok := e.clients[cid]
	if !ok || !cok || !clientAllows(c, u) {
		oauthError(w, http.StatusUnauthorized, "invalid_token", "the token has been revoked")
		return
	}
	writeJSON(w, http.StatusOK, userClaims(u, scope))
}

// oidcLogout 处理应用发起的退出：清除签发者上的登录态（含单点登录 Cookie），回调地址属于该客户端时跳回应用。
// 客户端由签名有效的 id_token_hint（允许已过期）或 client_id 确定，两者同时提供时必须一致。
func (a *authenticator) oidcLogout(w http.ResponseWriter, r *http.Request, e *oidcEnv) {
	_ = r.ParseForm()
	s := e.loginSite(r)
	http.SetCookie(w, &http.Cookie{Name: cookieName(s), Value: "", Path: "/", MaxAge: -1})
	if domain := a.shared.get().ssoFor(hostOnly(r.Host)); domain != "" {
		http.SetCookie(w, &http.Cookie{Name: ssoCookie, Value: "", Path: "/", Domain: domain, MaxAge: -1})
	}
	// 从“没有使用该应用的权限”页换账号：退出后带着原授权请求回到登录页
	if sw := r.Form.Get("switch"); strings.HasPrefix(sw, oidcAuthorizePath+"?") && safeNext(sw) == sw {
		http.Redirect(w, r, oidcLoginPath+"?next="+url.QueryEscape(sw), http.StatusFound)
		return
	}
	if target := r.Form.Get("post_logout_redirect_uri"); target != "" {
		c, ok := e.logoutClient(r.Form.Get("id_token_hint"), r.Form.Get("client_id"))
		// 回调地址需为该客户端登记的回调地址，或与其中之一同源
		if ok && (slices.Contains(c.RedirectURIs, target) || sameOriginAny(target, c.RedirectURIs)) {
			u, _ := url.Parse(target)
			if st := r.Form.Get("state"); st != "" && len(st) <= oidcMaxParam {
				v := u.Query()
				v.Set("state", st)
				u.RawQuery = v.Encode()
			}
			http.Redirect(w, r, u.String(), http.StatusFound)
			return
		}
	}
	http.Redirect(w, r, oidcLoginPath, http.StatusFound)
}

// logoutClient 确定退出请求所属的客户端：id_token_hint 需为本签发者签出的 ID Token（不检查有效期），
// 与 client_id 同时提供时两者必须一致；签名无效或不一致时不跳回任何应用。
func (e *oidcEnv) logoutClient(hint, cid string) (OIDCClient, bool) {
	if hint != "" {
		claims, err := e.parseJWT(hint, "JWT", false)
		if err != nil {
			return OIDCClient{}, false
		}
		aud, _ := claims["aud"].(string)
		if aud == "" || (cid != "" && cid != aud) {
			return OIDCClient{}, false
		}
		cid = aud
	}
	if cid == "" {
		return OIDCClient{}, false
	}
	c, ok := e.clients[cid]
	return c, ok
}

// sameOriginAny 判断地址是否与任一回调地址同源（协议、域名、端口一致）。
func sameOriginAny(target string, list []string) bool {
	t, err := url.Parse(target)
	if err != nil || t.Host == "" {
		return false
	}
	for _, l := range list {
		if u, err := url.Parse(l); err == nil && strings.EqualFold(u.Scheme, t.Scheme) && strings.EqualFold(u.Host, t.Host) {
			return true
		}
	}
	return false
}
