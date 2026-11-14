package app

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"

	"net/http"

	"github.com/wangwanjie/gately/internal/audit"
	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// ---- 陌生来源登录失败预算 ----

// applyLoginBudget 按设置启用陌生来源的全局登录失败预算：来源是否“陌生”以成功登录过管理后台的地址为准。
func (a *App) applyLoginBudget(s Settings) {
	limit := s.LoginGlobalLimit
	if limit == 0 {
		limit = auth.DefaultGlobalLimit
	}
	if limit < 0 || a.loginIPs == nil {
		a.auth.SetGlobalLimit(0, nil, nil)
		return
	}
	a.auth.SetGlobalLimit(limit, a.loginIPs.IsKnown, a.onLoginBudgetTrip)
}

// onLoginBudgetTrip 在预算首次耗尽时记录日志并发布告警事件（窗口期内只触发一次）。
func (a *App) onLoginBudgetTrip(fails int) {
	msg := fmt.Sprintf("10 分钟内来自陌生地址的登录失败已达 %d 次，疑似分布式爆破；此后陌生地址的登录暂时受限，曾成功登录过的地址不受影响", fails)
	a.sysLogs.Add(logbuf.LevelWarn, "auth", msg)
	events.Publish(events.Event{Type: events.AuthFlood, Source: "管理后台", Level: events.LevelWarn, Title: "管理后台疑似遭到分布式爆破", Message: msg})
}

// ---- 安全评分 ----

// 评分项的状态。
const (
	secOK   = "ok"
	secWarn = "warn"
	secRisk = "risk"
)

// secItem 是一项安全检查：Points 为该项已扣分数，Max 为该项满分。文案与修复入口由前端按 ID 提供。
type secItem struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Points int    `json:"points"`
	Max    int    `json:"max"`
}

// securityStatus 是管理端安全评分。
type securityStatus struct {
	// Score 为 0~100 的得分，Level 为 good（良好）、warn（建议加固）、risk（存在高风险）
	Score int    `json:"score"`
	Level string `json:"level"`
	// Exposed 表示管理端可能暴露在公网：监听了非回环地址且没有限制为仅内网访问，或当前请求本身来自公网
	Exposed       bool      `json:"exposed"`
	PasswordIssue string    `json:"passwordIssue"`
	Items         []secItem `json:"items"`
}

// commonAdminNames 是最常被爆破尝试的管理员用户名。
var commonAdminNames = map[string]bool{"admin": true, "administrator": true, "root": true, "gately": true, "user": true, "test": true}

// listenExposed 判断监听地址是否对外：空主机、通配地址与非回环地址都视为对外。
func listenExposed(listen string) bool {
	host := listen
	if h, _, err := net.SplitHostPort(listen); err == nil {
		host = h
	} else if i := strings.LastIndex(listen, ":"); i >= 0 && !strings.Contains(listen[:i], ":") {
		host = listen[:i]
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return true
	}
	if strings.EqualFold(host, "localhost") {
		return false
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return !ip.IsLoopback()
	}
	return true
}

// requestFromPublic 判断请求的客户端地址是否来自公网：此时管理端必然可从公网访问。
func requestFromPublic(r *http.Request) bool {
	ip, err := netip.ParseAddr(httpx.ClientIP(r))
	if err != nil {
		return false
	}
	ip = ip.Unmap()
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !isCGNAT(ip)
}

// isCGNAT 判断是否运营商级 NAT 地址段 100.64.0.0/10（Tailscale 等组网也使用）。
func isCGNAT(ip netip.Addr) bool {
	return netip.MustParsePrefix("100.64.0.0/10").Contains(ip)
}

// computeSecurity 汇总当前配置得出安全评分。纯函数，便于单元测试。
//
// 满分 100，各项满分：密码 35、二次验证 20、安全入口 15、锁定策略 10、HTTPS 10、陌生来源预算 5、用户名 5。
// 管理端没有暴露到公网（仅内网或监听回环）时，安全入口、HTTPS 与用户名三项视为满分。
func computeSecurity(snap auth.Snapshot, s Settings, exposed, secure bool) securityStatus {
	st := securityStatus{Exposed: exposed, PasswordIssue: snap.PasswordIssue}
	add := func(id string, max, points int, status string) {
		if points <= 0 {
			points, status = 0, secOK
		}
		st.Items = append(st.Items, secItem{ID: id, Status: status, Points: points, Max: max})
	}

	// 密码：默认密码或强度不足都是高风险；关闭密码登录后密码不再能被用来登录（Snapshot 已把问题清空）
	if snap.PasswordIssue != "" {
		add("password", 35, 35, secRisk)
	} else {
		add("password", 35, 0, secOK)
	}

	// 二次验证：TOTP 或 Passkey 任一即可；关闭密码登录（仅 Passkey）同样满分
	if snap.TOTP || snap.Passkeys > 0 {
		add("twoFactor", 20, 0, secOK)
	} else if exposed {
		add("twoFactor", 20, 20, secWarn)
	} else {
		add("twoFactor", 20, 10, secWarn)
	}

	if s.SafePath != "" || !exposed {
		add("entry", 15, 0, secOK)
	} else {
		add("entry", 15, 15, secWarn)
	}

	// 锁定策略：被改得过松（失败上限过高或锁定时间过短）同样扣分
	if snap.MaxFail > 10 || snap.LockDuration < 5*time.Minute {
		add("lockout", 10, 10, secWarn)
	} else {
		add("lockout", 10, 0, secOK)
	}

	if exposed && !secure {
		add("https", 10, 10, secWarn)
	} else {
		add("https", 10, 0, secOK)
	}

	if s.LoginGlobalLimit < 0 {
		add("globalLimit", 5, 5, secWarn)
	} else {
		add("globalLimit", 5, 0, secOK)
	}

	if exposed && commonAdminNames[strings.ToLower(snap.Username)] {
		add("username", 5, 5, secWarn)
	} else {
		add("username", 5, 0, secOK)
	}

	score := 100
	for _, it := range st.Items {
		score -= it.Points
	}
	if score < 0 {
		score = 0
	}
	st.Score = score
	switch {
	case snap.PasswordIssue != "" || (exposed && score < 60):
		st.Level = secRisk
	case score < 90:
		st.Level = secWarn
	default:
		st.Level = "good"
	}
	return st
}

func (a *App) handleSecurityStatus(w http.ResponseWriter, r *http.Request) {
	s := a.settingsSnapshot()
	exposed := (listenExposed(s.Listen) && !s.LANOnly) || requestFromPublic(r)
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	httpx.OK(w, computeSecurity(a.auth.Snapshot(), s, exposed, secure))
}

// ---- 密码强度预检 ----

type passwordCheckRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// handlePasswordCheck 预检候选密码的强度，供界面在输入时即时提示。只返回原因代码，不记录也不回显密码；
// 不涉及 bcrypt 与账号状态，登录会话内可随意调用。
func (a *App) handlePasswordCheck(w http.ResponseWriter, r *http.Request) {
	var req passwordCheckRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if strings.TrimSpace(req.Username) == "" {
		req.Username = a.auth.Username()
	}
	err := auth.CheckPasswordStrength(req.Username, req.Password)
	var we *auth.WeakError
	if errors.As(err, &we) {
		httpx.OK(w, map[string]any{"ok": false, "code": we.Code})
		return
	}
	httpx.OK(w, map[string]any{"ok": true})
}

// ---- 关闭密码登录 ----

type passwordLoginRequest struct {
	Password string `json:"password"`
	Disabled bool   `json:"disabled"`
}

func (a *App) handlePasswordLoginGet(w http.ResponseWriter, r *http.Request) {
	httpx.OK(w, map[string]any{"disabled": a.auth.PasswordLoginOff(), "passkeys": len(a.auth.Passkeys())})
}

// handlePasswordLoginSet 关闭或重新开启管理员的密码登录。关闭后只能用 Passkey 或第三方登录，
// 管理员账号的密码无法再被爆破；丢失 Passkey 设备时可在服务器上执行 gately reset-password 重新开启。
func (a *App) handlePasswordLoginSet(w http.ResponseWriter, r *http.Request) {
	var req passwordLoginRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if err := a.auth.SetPasswordLoginOff(req.Password, req.Disabled); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	msg := "管理员已重新开启密码登录"
	if req.Disabled {
		msg = "管理员已关闭密码登录，只能使用 Passkey 或第三方登录"
	}
	a.sysLogs.Add(logbuf.LevelWarn, "auth", msg)
	a.audit.Record(audit.Entry{Actor: auth.UserFromContext(r.Context()), IP: httpx.ClientIP(r), Via: audit.Via(r), Category: audit.CategoryChange,
		Method: r.Method, Path: r.URL.Path, Status: http.StatusOK, Detail: msg})
	httpx.OK(w, map[string]any{"disabled": a.auth.PasswordLoginOff()})
}
