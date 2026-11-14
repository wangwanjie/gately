package webservice

import (
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/i18n"
	"github.com/wangwanjie/gately/internal/webauthn"
	"github.com/wangwanjie/gately/internal/webpage"
)

// 共享用户 Passkey：开启设置 passkeyLogin 后，共享用户在认证保留路径下的 /passkey 页面自助注册，
// 并在登录页（站点与 OIDC 签发者域名）用 Passkey 免输密码与验证码登录。
//
// 凭据只保存公钥，存放在共享用户（AuthUser.Passkeys）上；RP ID 为单点登录域（站点在该域下时，
// 一次注册各站点通用）或站点域名。登录使用可发现凭据（不先输入用户名），强制用户验证（指纹、面容或 PIN），
// 因此与管理后台的 Passkey 登录一样不再要求动态验证码。
//
// 安全要点：
//   - 挑战随机、一次性、3 分钟有效，并绑定用途（登录/注册）、域名与注册的用户；挂起数量全局与按来源地址都有上限
//   - 登录失败与密码登录共用限流与自动封禁；跨站请求（Origin 缺失或与域名不一致）一律拒绝
//   - 注册、查看与删除先在页面验证用户名、密码与动态验证码，换得 5 分钟有效、绑定凭据指纹的令牌
//   - 签名计数必须递增（有计数的验证器），并按原子方式落盘，克隆的凭据无法重放

const (
	// passkeyPath 是认证保留路径下 Passkey 页面与接口的后缀
	passkeyPath = "/passkey"
	// passkeyChallengeTTL 为挑战有效期，passkeyStepTTL 为验证身份后管理令牌的有效期
	passkeyChallengeTTL = 3 * time.Minute
	passkeyStepTTL      = 5 * time.Minute
	// passkeyMaxPending 为同时挂起的挑战总数，passkeyMaxPerIP 为单个来源（IPv6 按 /64）的上限
	passkeyMaxPending = 4096
	passkeyMaxPerIP   = 16
	// maxUserPasskeys 为每个共享用户可注册的 Passkey 数量
	maxUserPasskeys = 10
	// passkeyBodyLimit 限制 Passkey 接口的请求体
	passkeyBodyLimit = 64 << 10
	// passkeyTouchEvery 为没有签名计数的凭据（同步型 Passkey 总是 0）刷新最近使用时间的最短间隔，避免每次登录都写配置
	passkeyTouchEvery = 24 * time.Hour
)

// 挑战的用途
const (
	pkLogin    = "login"
	pkRegister = "register"
)

type passkeyChallenge struct {
	exp  time.Time
	kind string
	// user 为注册挑战对应用户的 ID，登录挑战为空
	user string
	host string
	// ip 为来源限流键，用于归还按来源的配额
	ip string
}

// passkeyChallenges 保存已下发、尚未使用的 WebAuthn 挑战，每个挑战只能使用一次。
type passkeyChallenges struct {
	mu    sync.Mutex
	m     map[string]passkeyChallenge
	perIP map[string]int
}

// issue 下发挑战；总量或来源配额用尽时返回 false。
func (c *passkeyChallenges) issue(kind, user, host, ip string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if c.m == nil {
		c.m, c.perIP = map[string]passkeyChallenge{}, map[string]int{}
	}
	if len(c.m) >= passkeyMaxPending/2 {
		// 表较大时先清理过期项
		c.sweepLocked(now)
	}
	if len(c.m) >= passkeyMaxPending || c.perIP[ip] >= passkeyMaxPerIP {
		c.sweepLocked(now)
		if len(c.m) >= passkeyMaxPending || c.perIP[ip] >= passkeyMaxPerIP {
			return "", false
		}
	}
	ch := webauthn.NewChallenge()
	c.m[ch] = passkeyChallenge{exp: now.Add(passkeyChallengeTTL), kind: kind, user: user, host: host, ip: ip}
	c.perIP[ip]++
	return ch, true
}

func (c *passkeyChallenges) sweepLocked(now time.Time) {
	for k, v := range c.m {
		if now.After(v.exp) {
			c.dropLocked(k, v)
		}
	}
}

func (c *passkeyChallenges) dropLocked(k string, v passkeyChallenge) {
	delete(c.m, k)
	if c.perIP[v.ip]--; c.perIP[v.ip] <= 0 {
		delete(c.perIP, v.ip)
	}
}

// consume 校验并移除挑战：用途与域名必须与下发时一致，同一挑战只能使用一次。返回注册挑战对应的用户 ID。
func (c *passkeyChallenges) consume(ch, kind, host string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[ch]
	if !ok {
		return "", false
	}
	c.dropLocked(ch, v)
	if time.Now().After(v.exp) || v.kind != kind || v.host != host {
		return "", false
	}
	return v.user, true
}

// pending 返回挂起的挑战数量（测试用）。
func (c *passkeyChallenges) pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.m)
}

// PasskeyInfo 是 Passkey 对外显示的信息。
type PasskeyInfo struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	LastUsed  time.Time `json:"lastUsed,omitzero"`
}

// maskPasskeys 返回脱敏后的 Passkey：不含公钥坐标与签名计数，只保留标识、名称与时间。
func maskPasskeys(list []webauthn.Credential) []webauthn.Credential {
	if len(list) == 0 {
		return nil
	}
	out := make([]webauthn.Credential, len(list))
	for i, c := range list {
		out[i] = webauthn.Credential{ID: c.ID, Name: c.Name, CreatedAt: c.CreatedAt, LastUsed: c.LastUsed}
	}
	return out
}

func passkeyInfos(list []webauthn.Credential) []PasskeyInfo {
	out := make([]PasskeyInfo, 0, len(list))
	for _, c := range list {
		out = append(out, PasskeyInfo{ID: c.ID, Name: c.Name, CreatedAt: c.CreatedAt, LastUsed: c.LastUsed})
	}
	return out
}

// passkeyRP 返回访问域名对应的 RP ID：站点在单点登录域下时为该域（一次注册各站点通用），否则为站点域名。
// IP 地址不能作为 RP ID，浏览器会拒绝，需要用域名访问。
func (a *authenticator) passkeyRP(r *http.Request) (string, error) {
	h := hostOnly(r.Host)
	if h == "" || net.ParseIP(h) != nil {
		return "", errors.New("Passkey 需要通过域名访问，不支持 IP 地址")
	}
	if d := a.shared.get().ssoFor(h); d != "" {
		return d, nil
	}
	return h, nil
}

// passkeyOrigin 要求请求带有同域名的 Origin 头（浏览器对 JSON 的 POST 总会带），返回 Origin 中的主机（含端口）用于校验 clientData。
// 经反向代理转发导致 Host 缺少端口时，以 Origin 为准；域名部分仍必须与请求一致。
func passkeyOrigin(r *http.Request) (string, bool) {
	o := r.Header.Get("Origin")
	if o == "" || o == "null" {
		return "", false
	}
	u, err := url.Parse(o)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || hostOnly(u.Host) != hostOnly(r.Host) {
		return "", false
	}
	return u.Host, true
}

// pkStep 是在 Passkey 页面验证身份后签发的管理令牌，绑定用户凭据指纹、域名与站点；改密码或 TOTP 后立即失效。
type pkStep struct {
	User  string `json:"u"`
	Cred  string `json:"c"`
	Host  string `json:"h"`
	Scope string `json:"s"`
}

func (a *authenticator) pkStepUser(token, host string, s Site) (AuthUser, bool) {
	var st pkStep
	if err := a.verifyBlob("pk-step", token, &st); err != nil || st.Host != host || st.Scope != s.scope {
		return AuthUser{}, false
	}
	env := a.shared.get()
	name, ok := env.byID[st.User]
	if !ok {
		return AuthUser{}, false
	}
	u := env.users[name]
	if !hmac.Equal([]byte(st.Cred), []byte(a.sharedCred(u))) {
		return AuthUser{}, false
	}
	return u, true
}

// pkError 写出 JSON 错误，提示按访客语言翻译。
func pkError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": i18n.Tr(i18n.RequestLang(r), msg)})
}

func pkJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

type pubKeyCred struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// handlePasskey 分发认证保留路径下 /passkey 开头的请求：管理页面与登录、注册接口。
func (a *authenticator) handlePasskey(w http.ResponseWriter, r *http.Request, s Site, ip string) {
	if !a.passkeyOn.Load() || !s.SharedUsers {
		http.NotFound(w, r)
		return
	}
	base := s.authPrefix() + passkeyPath
	sub := strings.TrimPrefix(r.URL.Path, base)
	if sub == "" {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "405 Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		a.passkeyPage(w, r, s)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "405 Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, passkeyBodyLimit)
	// 浏览器对 JSON 的 POST 总会带 Origin；缺失或与域名不一致的请求（跨站、脚本伪造）不处理，也不消耗限流次数
	origin, ok := passkeyOrigin(r)
	if !ok {
		pkError(w, r, http.StatusForbidden, "请求来源无效，请刷新页面后重试")
		return
	}
	rp, err := a.passkeyRP(r)
	if err != nil {
		pkError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	switch sub {
	case "/login/begin":
		a.passkeyLoginBegin(w, r, rp, ip)
	case "/login/finish":
		a.passkeyLoginFinish(w, r, s, rp, origin, ip)
	case "/manage/auth":
		a.passkeyManageAuth(w, r, s, ip)
	case "/manage/begin":
		a.passkeyRegisterBegin(w, r, s, rp, ip)
	case "/manage/finish":
		a.passkeyRegisterFinish(w, r, s, rp, origin, ip)
	case "/manage/delete":
		a.passkeyDelete(w, r, s, ip)
	default:
		http.NotFound(w, r)
	}
}

// passkeyLoginBegin 下发登录参数：不指定凭据，由验证器列出该域名下可用的 Passkey（可发现凭据）。
func (a *authenticator) passkeyLoginBegin(w http.ResponseWriter, r *http.Request, rp, ip string) {
	ch, ok := a.pkChal.issue(pkLogin, "", hostOnly(r.Host), limitKey(ip))
	if !ok {
		pkError(w, r, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
		return
	}
	pkJSON(w, map[string]any{
		"challenge":        ch,
		"rpId":             rp,
		"userVerification": "required",
		"timeout":          120000,
		"allowCredentials": []pubKeyCred{},
	})
}

type passkeyLoginRequest struct {
	Challenge  string                     `json:"challenge"`
	Credential webauthn.AssertionResponse `json:"credential"`
	Next       string                     `json:"next"`
}

// passkeyLoginFinish 校验断言并签发共享用户会话。任何失败都只返回统一提示，并计入失败次数。
func (a *authenticator) passkeyLoginFinish(w http.ResponseWriter, r *http.Request, s Site, rp, origin, ip string) {
	var req passkeyLoginRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		pkError(w, r, http.StatusBadRequest, "请求格式错误")
		return
	}
	key := limitKey(ip)
	if !a.reserve(key) {
		pkError(w, r, http.StatusTooManyRequests, "失败次数过多，请稍后再试")
		return
	}
	u, cred, err := a.passkeyVerifyLogin(r, req, rp, origin)
	if err == nil {
		// 落盘新的签名计数；重放（计数未递增）在这里被原子地拒绝，其余保存失败只记日志，不影响登录
		if perr := a.persistPasskeyUse(u, cred); errors.Is(perr, errPasskeyReplay) {
			err = fmt.Errorf("用户 %s: %w", u.Name, perr)
		} else if perr != nil {
			a.passkeyEvent(fmt.Sprintf("共享用户 %s 的 Passkey 使用记录保存失败: %v", u.Name, perr))
		}
	}
	a.release(key, err == nil)
	if err != nil {
		a.passkeyEvent(fmt.Sprintf("来自 %s 的共享用户 Passkey 登录失败: %v", ip, err))
		pkError(w, r, http.StatusUnauthorized, "Passkey 验证失败，请重试或改用密码登录")
		return
	}
	a.issueSharedSession(w, r, s, u)
	a.passkeyEvent(fmt.Sprintf("共享用户 %s 使用 Passkey 从 %s 登录", u.Name, ip))
	pkJSON(w, map[string]string{"next": safeNext(req.Next)})
}

// errPasskeyReplay 表示签名计数没有递增，凭据可能被克隆。
var errPasskeyReplay = errors.New("签名计数未递增，凭据可能被克隆")

// passkeyVerifyLogin 校验挑战、查找凭据所属用户并验证断言，返回用户与带有新签名计数的凭据副本。
func (a *authenticator) passkeyVerifyLogin(r *http.Request, req passkeyLoginRequest, rp, origin string) (AuthUser, webauthn.Credential, error) {
	if _, ok := a.pkChal.consume(req.Challenge, pkLogin, hostOnly(r.Host)); !ok {
		return AuthUser{}, webauthn.Credential{}, errors.New("挑战不存在或已过期")
	}
	env := a.shared.get()
	name, ok := env.byCred[req.Credential.ID]
	if !ok {
		return AuthUser{}, webauthn.Credential{}, errors.New("凭据未注册")
	}
	u := env.users[name]
	i := slices.IndexFunc(u.Passkeys, func(c webauthn.Credential) bool { return c.ID == req.Credential.ID })
	if i < 0 {
		return AuthUser{}, webauthn.Credential{}, errors.New("凭据未注册")
	}
	cred := u.Passkeys[i]
	// 凭据只能在注册时的 RP ID 下使用：单点登录域变更后旧凭据不再有效，需要重新注册
	if cred.RPID != rp {
		return AuthUser{}, webauthn.Credential{}, errors.New("凭据注册于其他域名")
	}
	count, err := webauthn.VerifyAssertionRP(&cred, req.Credential, req.Challenge, origin, rp)
	if err != nil {
		return AuthUser{}, webauthn.Credential{}, err
	}
	cred.SignCount = count
	return u, cred, nil
}

// persistPasskeyUse 记录凭据的使用：签名计数变化时必须落盘（并拒绝重放）；没有计数的凭据最多每天刷新一次最近使用时间。
func (a *authenticator) persistPasskeyUse(u AuthUser, used webauthn.Credential) error {
	if a.savePasskeys == nil {
		return nil
	}
	now := time.Now()
	return a.savePasskeys(u.ID, func(all []AuthUser, i int) ([]webauthn.Credential, error) {
		list := all[i].Passkeys
		j := slices.IndexFunc(list, func(c webauthn.Credential) bool { return c.ID == used.ID })
		if j < 0 {
			return nil, errors.New("Passkey 已被删除")
		}
		cur := list[j]
		counted := used.SignCount != 0 || cur.SignCount != 0
		if counted && used.SignCount <= cur.SignCount {
			return nil, errPasskeyReplay
		}
		if !counted && now.Sub(cur.LastUsed) < passkeyTouchEvery {
			return nil, errNoPasskeyChange
		}
		next := slices.Clone(list)
		next[j].SignCount, next[j].LastUsed = used.SignCount, now
		return next, nil
	})
}

// 保存回调返回的错误：errNoPasskeyChange 表示无需写入，其余为可以直接告诉访客的原因
var (
	errNoPasskeyChange = errors.New("无改动")
	errPasskeyLimit    = errors.New("Passkey 数量已达上限")
	errPasskeyDup      = errors.New("该 Passkey 已注册")
)

type passkeyAuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Code     string `json:"code"`
}

// passkeyManageAuth 验证用户名、密码与动态验证码（与自助改密相同的限流与封禁），成功后返回管理令牌与已注册的 Passkey。
func (a *authenticator) passkeyManageAuth(w http.ResponseWriter, r *http.Request, s Site, ip string) {
	var req passkeyAuthRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		pkError(w, r, http.StatusBadRequest, "请求格式错误")
		return
	}
	name := strings.TrimSpace(req.Username)
	if name == "" || req.Password == "" {
		pkError(w, r, http.StatusBadRequest, "请输入用户名和密码")
		return
	}
	key := limitKey(ip)
	if !a.reserve(key) {
		pkError(w, r, http.StatusTooManyRequests, "失败次数过多，请稍后再试")
		return
	}
	u, ok := a.shared.get().checkPassword(name, req.Password)
	if !ok {
		a.release(key, false)
		pkError(w, r, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	if u.TOTPSecret != "" {
		if msg := a.checkTOTP(u, req.Code); msg != "" {
			a.release(key, false)
			pkError(w, r, http.StatusUnauthorized, msg)
			return
		}
	}
	a.release(key, true)
	token := a.signBlob("pk-step", pkStep{User: u.ID, Cred: a.sharedCred(u), Host: hostOnly(r.Host), Scope: s.scope}, passkeyStepTTL)
	pkJSON(w, map[string]any{"token": token, "username": u.Name, "passkeys": passkeyInfos(u.Passkeys)})
}

type passkeyTokenRequest struct {
	Token string `json:"token"`
}

// passkeyRegisterBegin 下发注册参数（navigator.credentials.create 的 publicKey）。
func (a *authenticator) passkeyRegisterBegin(w http.ResponseWriter, r *http.Request, s Site, rp, ip string) {
	var req passkeyTokenRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		pkError(w, r, http.StatusBadRequest, "请求格式错误")
		return
	}
	u, ok := a.pkStepUser(req.Token, hostOnly(r.Host), s)
	if !ok {
		pkError(w, r, http.StatusUnauthorized, "验证已超时，请重新验证身份")
		return
	}
	if len(u.Passkeys) >= maxUserPasskeys {
		pkError(w, r, http.StatusBadRequest, "Passkey 数量已达上限")
		return
	}
	ch, ok := a.pkChal.issue(pkRegister, u.ID, hostOnly(r.Host), limitKey(ip))
	if !ok {
		pkError(w, r, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
		return
	}
	exclude := []pubKeyCred{}
	for _, c := range u.Passkeys {
		exclude = append(exclude, pubKeyCred{Type: "public-key", ID: c.ID})
	}
	display := u.DisplayName
	if display == "" {
		display = u.Name
	}
	pkJSON(w, map[string]any{
		"challenge": ch,
		"rp":        map[string]string{"id": rp, "name": "Gately"},
		// 用户句柄用不可变的随机 ID，不含用户名等信息
		"user":                   map[string]string{"id": base64.RawURLEncoding.EncodeToString([]byte(u.ID)), "name": u.Name, "displayName": display},
		"pubKeyCredParams":       []map[string]any{{"type": "public-key", "alg": -7}},
		"excludeCredentials":     exclude,
		"authenticatorSelection": map[string]string{"residentKey": "required", "userVerification": "required"},
		"attestation":            "none",
		"timeout":                120000,
	})
}

type passkeyRegisterRequest struct {
	Token      string                        `json:"token"`
	Name       string                        `json:"name"`
	Challenge  string                        `json:"challenge"`
	Credential webauthn.RegistrationResponse `json:"credential"`
}

// cleanPasskeyName 整理 Passkey 名称：去首尾空白，不含控制字符，最多 40 个字符，留空为 Passkey。
func cleanPasskeyName(n string) (string, bool) {
	n = strings.TrimSpace(n)
	if n == "" {
		return "Passkey", true
	}
	if len([]rune(n)) > 40 || strings.ContainsFunc(n, unicode.IsControl) {
		return "", false
	}
	return n, true
}

func (a *authenticator) passkeyRegisterFinish(w http.ResponseWriter, r *http.Request, s Site, rp, origin, ip string) {
	var req passkeyRegisterRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		pkError(w, r, http.StatusBadRequest, "请求格式错误")
		return
	}
	u, ok := a.pkStepUser(req.Token, hostOnly(r.Host), s)
	if !ok {
		pkError(w, r, http.StatusUnauthorized, "验证已超时，请重新验证身份")
		return
	}
	name, ok := cleanPasskeyName(req.Name)
	if !ok {
		pkError(w, r, http.StatusBadRequest, "名称过长或包含控制字符")
		return
	}
	// 挑战必须是为该用户下发的注册挑战
	if owner, ok := a.pkChal.consume(req.Challenge, pkRegister, hostOnly(r.Host)); !ok || owner != u.ID {
		pkError(w, r, http.StatusBadRequest, "挑战已过期，请重新开始")
		return
	}
	cred, err := webauthn.ParseRegistrationRP(req.Credential, req.Challenge, origin, rp, name)
	if err != nil {
		a.passkeyEvent(fmt.Sprintf("共享用户 %s 从 %s 注册 Passkey 失败: %v", u.Name, ip, err))
		pkError(w, r, http.StatusBadRequest, "Passkey 注册失败，请重试")
		return
	}
	if a.savePasskeys == nil {
		pkError(w, r, http.StatusInternalServerError, "保存失败，请联系管理员")
		return
	}
	var updated []webauthn.Credential
	err = a.savePasskeys(u.ID, func(all []AuthUser, i int) ([]webauthn.Credential, error) {
		if all[i].Name != u.Name || a.sharedCred(all[i]) != a.sharedCred(u) {
			return nil, errPasswordChanged
		}
		if len(all[i].Passkeys) >= maxUserPasskeys {
			return nil, errPasskeyLimit
		}
		// 凭据 ID 在所有共享用户中唯一，避免登录时匹配到别人的账号
		for _, o := range all {
			if slices.ContainsFunc(o.Passkeys, func(c webauthn.Credential) bool { return c.ID == cred.ID }) {
				return nil, errPasskeyDup
			}
		}
		updated = append(slices.Clone(all[i].Passkeys), *cred)
		return updated, nil
	})
	if err != nil {
		a.passkeySaveError(w, r, u, ip, err)
		return
	}
	a.passkeyEvent(fmt.Sprintf("共享用户 %s 从 %s 注册了 Passkey: %s", u.Name, ip, name))
	pkJSON(w, map[string]any{"passkeys": passkeyInfos(updated)})
}

type passkeyDeleteRequest struct {
	Token string `json:"token"`
	ID    string `json:"id"`
}

func (a *authenticator) passkeyDelete(w http.ResponseWriter, r *http.Request, s Site, ip string) {
	var req passkeyDeleteRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		pkError(w, r, http.StatusBadRequest, "请求格式错误")
		return
	}
	u, ok := a.pkStepUser(req.Token, hostOnly(r.Host), s)
	if !ok {
		pkError(w, r, http.StatusUnauthorized, "验证已超时，请重新验证身份")
		return
	}
	if a.savePasskeys == nil {
		pkError(w, r, http.StatusInternalServerError, "保存失败，请联系管理员")
		return
	}
	var updated []webauthn.Credential
	err := a.savePasskeys(u.ID, func(all []AuthUser, i int) ([]webauthn.Credential, error) {
		if all[i].Name != u.Name || a.sharedCred(all[i]) != a.sharedCred(u) {
			return nil, errPasswordChanged
		}
		j := slices.IndexFunc(all[i].Passkeys, func(c webauthn.Credential) bool { return c.ID == req.ID })
		if j < 0 {
			return nil, errPasskeyNotFound
		}
		updated = slices.Delete(slices.Clone(all[i].Passkeys), j, j+1)
		return updated, nil
	})
	if err != nil {
		a.passkeySaveError(w, r, u, ip, err)
		return
	}
	a.passkeyEvent(fmt.Sprintf("共享用户 %s 从 %s 删除了一个 Passkey", u.Name, ip))
	pkJSON(w, map[string]any{"passkeys": passkeyInfos(updated)})
}

// errPasskeyNotFound 表示要删除的 Passkey 不存在。
var errPasskeyNotFound = errors.New("Passkey 不存在")

// passkeySaveError 把保存回调的错误转成访客提示；其余错误只给统一提示，详细原因写日志。
func (a *authenticator) passkeySaveError(w http.ResponseWriter, r *http.Request, u AuthUser, ip string, err error) {
	switch {
	case errors.Is(err, errPasswordChanged):
		pkError(w, r, http.StatusUnauthorized, "验证已超时，请重新验证身份")
	case errors.Is(err, errPasskeyNotFound):
		pkError(w, r, http.StatusNotFound, err.Error())
	case errors.Is(err, errPasskeyLimit), errors.Is(err, errPasskeyDup):
		pkError(w, r, http.StatusBadRequest, err.Error())
	default:
		a.passkeyEvent(fmt.Sprintf("共享用户 %s 从 %s 保存 Passkey 时失败: %v", u.Name, ip, err))
		pkError(w, r, http.StatusInternalServerError, "保存失败，请联系管理员")
	}
}

func (a *authenticator) passkeyEvent(msg string) {
	if a.onPasskeyEvent != nil {
		a.onPasskeyEvent(msg)
	}
}

// passkeyOffered 判断登录页是否展示 Passkey 入口：功能开启、站点使用共享用户，且访问地址可作为 RP ID（不是 IP）。
func (a *authenticator) passkeyOffered(r *http.Request, s Site) bool {
	if !a.passkeyOn.Load() || !s.SharedUsers {
		return false
	}
	_, err := a.passkeyRP(r)
	return err == nil
}

// passkeyPage 输出 Passkey 管理页面，全部交互由页面脚本调用上面的接口完成。
func (a *authenticator) passkeyPage(w http.ResponseWriter, r *http.Request, s Site) {
	lang := i18n.RequestLang(r)
	host := hostOnly(r.Host)
	base := s.authPrefix()
	data := map[string]any{"Host": host, "Base": base + passkeyPath, "LoginURL": base + "/login"}
	if _, err := a.passkeyRP(r); err != nil {
		data["Error"] = i18n.Tr(lang, err.Error())
	}
	data["PageTitle"] = i18n.Tr(lang, passkeyTexts["pageTitle"]) + " · " + host
	webpage.Render(w, http.StatusOK, "ws-passkey", lang, passkeyTexts, data)
}
