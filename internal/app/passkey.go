package app

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/audit"
	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/webauthn"
)

// 挑战的有效期与同时挂起的上限。登录接口未认证，要限制数量防止被刷爆内存；但总量用尽时不能拒绝所有人，
// 否则任何人连续请求几百次就能让 Passkey 登录一直不可用（关闭了密码登录的实例只剩命令行重置）。
// 做法与站点 Passkey 一致：每个来源单独限额，总量满时淘汰最早的挑战。
const (
	passkeyChallengeTTL = 3 * time.Minute
	// passkeyMaxPending 为同时挂起的挑战总数，passkeyMaxPerSource 为单个来源（IPv6 按 /64）的上限
	passkeyMaxPending   = 4096
	passkeyMaxPerSource = 16
)

type pendingChallenge struct {
	exp time.Time
	// src 为来源限流键，用于归还按来源的配额；来源未知时为空，不计配额
	src string
}

// challengeStore 保存已下发、尚未使用的 WebAuthn 挑战，每个挑战只能使用一次。
type challengeStore struct {
	mu      sync.Mutex
	pending map[string]pendingChallenge
	perSrc  map[string]int
}

// challengeSource 把客户端地址归并为限额用的键：IPv6 按 /64 聚合，无法解析的值原样使用。
func challengeSource(src string) string {
	a, err := netip.ParseAddr(src)
	if err != nil {
		return src
	}
	a = a.Unmap().WithZone("")
	if a.Is6() {
		return netip.PrefixFrom(a, 64).Masked().String()
	}
	return a.String()
}

// issue 为来源 src 下发挑战；该来源挂起的挑战已达上限时返回 false。src 为空（来源未知）时不计来源配额。
func (s *challengeStore) issue(src string) (string, bool) {
	src = challengeSource(src)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if s.pending == nil {
		s.pending, s.perSrc = map[string]pendingChallenge{}, map[string]int{}
	}
	if len(s.pending) >= passkeyMaxPending/2 || (src != "" && s.perSrc[src] >= passkeyMaxPerSource) {
		for k, v := range s.pending {
			if now.After(v.exp) {
				s.dropLocked(k, v)
			}
		}
	}
	if src != "" && s.perSrc[src] >= passkeyMaxPerSource {
		return "", false
	}
	if len(s.pending) >= passkeyMaxPending {
		// 总量满：淘汰最早到期的挑战。正常登录从取挑战到提交只有几秒，要挤掉它得在这几秒内再发几千个请求
		var (
			found  bool
			oldest string
			oldV   pendingChallenge
		)
		for k, v := range s.pending {
			if !found || v.exp.Before(oldV.exp) {
				found, oldest, oldV = true, k, v
			}
		}
		if found {
			s.dropLocked(oldest, oldV)
		}
	}
	c := webauthn.NewChallenge()
	s.pending[c] = pendingChallenge{exp: now.Add(passkeyChallengeTTL), src: src}
	if src != "" {
		s.perSrc[src]++
	}
	return c, true
}

func (s *challengeStore) dropLocked(k string, v pendingChallenge) {
	delete(s.pending, k)
	if v.src == "" {
		return
	}
	if s.perSrc[v.src]--; s.perSrc[v.src] <= 0 {
		delete(s.perSrc, v.src)
	}
}

// consume 校验并移除挑战。
func (s *challengeStore) consume(c string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.pending[c]
	if !ok {
		return false
	}
	s.dropLocked(c, v)
	return time.Now().Before(v.exp)
}

// userHandle 是管理员在验证器中的稳定标识（不含任何隐私信息）。
func passkeyUserHandle() string {
	sum := sha256.Sum256([]byte("gately-admin"))
	return base64.RawURLEncoding.EncodeToString(sum[:16])
}

type pubKeyCred struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// ---- 管理（需要登录会话） ----

func (a *App) handlePasskeyList(w http.ResponseWriter, r *http.Request) {
	httpx.OK(w, a.auth.Passkeys())
}

// handlePasskeyBegin 下发注册所需的参数（对应 navigator.credentials.create 的 publicKey）。
func (a *App) handlePasskeyBegin(w http.ResponseWriter, r *http.Request) {
	// 注册挑战单独存放：未登录的请求再多，也挤不掉管理员正在进行的注册
	challenge, ok := a.passkeyRegChallenges.issue("")
	if !ok {
		httpx.Error(w, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
		return
	}
	exclude := []pubKeyCred{}
	for _, id := range a.auth.PasskeyIDs() {
		exclude = append(exclude, pubKeyCred{Type: "public-key", ID: id})
	}
	name := a.auth.Username()
	httpx.OK(w, map[string]any{
		"challenge":              challenge,
		"rp":                     map[string]string{"id": webauthn.RPID(r.Host), "name": "Gately"},
		"user":                   map[string]string{"id": passkeyUserHandle(), "name": name, "displayName": name},
		"pubKeyCredParams":       []map[string]any{{"type": "public-key", "alg": -7}},
		"excludeCredentials":     exclude,
		"authenticatorSelection": map[string]string{"residentKey": "required", "userVerification": "required"},
		"attestation":            "none",
		"timeout":                120000,
	})
}

type passkeyRegisterRequest struct {
	Name       string                        `json:"name"`
	Password   string                        `json:"password"`
	Challenge  string                        `json:"challenge"`
	Credential webauthn.RegistrationResponse `json:"credential"`
}

func (a *App) handlePasskeyRegister(w http.ResponseWriter, r *http.Request) {
	var req passkeyRegisterRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if !a.passkeyRegChallenges.consume(req.Challenge) {
		httpx.Error(w, http.StatusBadRequest, "挑战已过期，请重新开始")
		return
	}
	name := req.Name
	if name == "" {
		name = "Passkey"
	}
	if len([]rune(name)) > 40 {
		httpx.Error(w, http.StatusBadRequest, "名称过长")
		return
	}
	cred, err := webauthn.ParseRegistration(req.Credential, req.Challenge, r.Host, name)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if err := a.auth.AddPasskey(req.Password, cred); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	a.sysLogs.Add(logbuf.LevelInfo, "auth", fmt.Sprintf("已注册 Passkey: %s", name))
	httpx.OK(w, a.auth.Passkeys())
}

type passkeyDeleteRequest struct {
	Password string `json:"password"`
}

func (a *App) handlePasskeyDelete(w http.ResponseWriter, r *http.Request) {
	var req passkeyDeleteRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	id := r.PathValue("id")
	if err := a.auth.RemovePasskey(req.Password, id); err != nil {
		if errors.Is(err, auth.ErrPasskeyNotFound) {
			httpx.Error(w, http.StatusNotFound, "%v", err)
			return
		}
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	a.sysLogs.Add(logbuf.LevelWarn, "auth", "已删除 Passkey")
	httpx.OK(w, a.auth.Passkeys())
}

// ---- 登录（公开接口） ----

// handlePasskeyLoginBegin 下发登录参数（navigator.credentials.get 的 publicKey）；不指定凭据，由验证器列出可用的 Passkey。
func (a *App) handlePasskeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	if len(a.auth.PasskeyIDs()) == 0 {
		httpx.Error(w, http.StatusBadRequest, "尚未注册 Passkey")
		return
	}
	src, _ := a.loginSource(r, httpx.ClientIP(r))
	challenge, ok := a.passkeyChallenges.issue(src)
	if !ok {
		httpx.Error(w, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
		return
	}
	httpx.OK(w, map[string]any{
		"challenge":        challenge,
		"rpId":             webauthn.RPID(r.Host),
		"userVerification": "required",
		"timeout":          120000,
		"allowCredentials": []pubKeyCred{},
	})
}

type passkeyLoginRequest struct {
	Challenge  string                     `json:"challenge"`
	Credential webauthn.AssertionResponse `json:"credential"`
}

func (a *App) handlePasskeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	var req passkeyLoginRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	ip := httpx.ClientIP(r)
	src, ipTrusted := a.loginSource(r, ip)
	if !a.passkeyChallenges.consume(req.Challenge) {
		httpx.Error(w, http.StatusBadRequest, "挑战已过期，请重新开始")
		return
	}
	token, exp, err := a.auth.LoginPasskey(src, req.Credential.ID, func(c webauthn.Credential) (uint32, error) {
		return webauthn.VerifyAssertion(&c, req.Credential, req.Challenge, r.Host)
	})
	if err != nil {
		a.sysLogs.Add(logbuf.LevelWarn, "auth", fmt.Sprintf("来自 %s 的 Passkey 登录失败: %v", ip, err))
		a.audit.Record(audit.Entry{Actor: a.auth.Username(), IP: ip, Via: audit.Via(r), Category: audit.CategoryLogin, Method: "POST", Path: "/api/login/passkey/finish", Status: http.StatusUnauthorized, Detail: err.Error()})
		events.Publish(events.Event{Type: events.AuthFailed, Source: "管理后台", Level: events.LevelWarn,
			Title: "管理后台登录失败", Message: fmt.Sprintf("来自 %s 的登录失败：%v", ip, err), Fields: authFailedFields(ip, ipTrusted)})
		status := http.StatusUnauthorized
		if errors.Is(err, auth.ErrLocked) || errors.Is(err, auth.ErrBusy) {
			status = http.StatusTooManyRequests
		}
		httpx.Error(w, status, "%v", err)
		return
	}
	user := a.auth.Username()
	// 先记录界面语言，随后的登录日志与通知即按该语言输出
	a.rememberUILanguage(r)
	a.sysLogs.Add(logbuf.LevelInfo, "auth", fmt.Sprintf("%s 使用 Passkey 从 %s 登录", user, ip))
	a.audit.Record(audit.Entry{Actor: user, IP: ip, Via: audit.Via(r), Category: audit.CategoryLogin, Method: "POST", Path: "/api/login/passkey/finish", Status: http.StatusOK, Detail: "Passkey 登录"})
	events.Publish(events.Event{Type: events.LoginSuccessful, Source: "管理后台", Level: events.LevelInfo,
		Title: "管理后台登录成功", Message: fmt.Sprintf("%s 从 %s 登录", user, ip), Fields: map[string]string{"ip": ip, "user": user}})
	a.noteLoginIP(ip, user, "Passkey")
	a.learnGatewayLoginOrigin(r)
	http.SetCookie(w, &http.Cookie{
		Name: auth.CookieName, Value: token, Path: "/", Expires: exp,
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: cookieSecure(r),
	})
	httpx.OK(w, map[string]any{"username": user, "expires": exp})
}
