package app

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/audit"
	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/webauthn"
)

// 挑战的有效期与同时挂起的上限（登录接口未认证，限制数量防止被刷爆内存）
const (
	passkeyChallengeTTL = 3 * time.Minute
	passkeyMaxPending   = 200
)

// challengeStore 保存已下发、尚未使用的 WebAuthn 挑战，每个挑战只能使用一次。
type challengeStore struct {
	mu      sync.Mutex
	pending map[string]time.Time
}

func (s *challengeStore) issue() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, exp := range s.pending {
		if now.After(exp) {
			delete(s.pending, k)
		}
	}
	if s.pending == nil {
		s.pending = map[string]time.Time{}
	}
	if len(s.pending) >= passkeyMaxPending {
		return "", false
	}
	c := webauthn.NewChallenge()
	s.pending[c] = now.Add(passkeyChallengeTTL)
	return c, true
}

// consume 校验并移除挑战。
func (s *challengeStore) consume(c string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.pending[c]
	delete(s.pending, c)
	return ok && time.Now().Before(exp)
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
	challenge, ok := a.passkeyChallenges.issue()
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
	if !a.passkeyChallenges.consume(req.Challenge) {
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
	challenge, ok := a.passkeyChallenges.issue()
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
	if !a.passkeyChallenges.consume(req.Challenge) {
		httpx.Error(w, http.StatusBadRequest, "挑战已过期，请重新开始")
		return
	}
	token, exp, err := a.auth.LoginPasskey(ip, req.Credential.ID, func(c webauthn.Credential) (uint32, error) {
		return webauthn.VerifyAssertion(&c, req.Credential, req.Challenge, r.Host)
	})
	if err != nil {
		a.sysLogs.Add(logbuf.LevelWarn, "auth", fmt.Sprintf("来自 %s 的 Passkey 登录失败: %v", ip, err))
		a.audit.Record(audit.Entry{Actor: a.auth.Username(), IP: ip, Via: audit.Via(r), Category: audit.CategoryLogin, Method: "POST", Path: "/api/login/passkey/finish", Status: http.StatusUnauthorized, Detail: err.Error()})
		events.Publish(events.Event{Type: events.AuthFailed, Source: "管理后台", Level: events.LevelWarn,
			Title: "管理后台登录失败", Message: fmt.Sprintf("来自 %s 的登录失败：%v", ip, err), Fields: map[string]string{"ip": ip}})
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
	http.SetCookie(w, &http.Cookie{
		Name: auth.CookieName, Value: token, Path: "/", Expires: exp,
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil,
	})
	httpx.OK(w, map[string]any{"username": user, "expires": exp})
}
