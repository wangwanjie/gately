package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/audit"
	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/luckyimport"
	"github.com/wangwanjie/gately/internal/thirdauth"
)

// 管理界面第三方登录
const (
	loginStateKind = "login-state"
	loginStateTTL  = 10 * time.Minute
	// loginNonceCookie 把登录流程绑定到发起登录的浏览器
	loginNonceCookie = "gately_login_nonce"
	loginNoncePath   = "/api/login/oauth"
)

// loginState 在跳转身份提供方时携带，回调时用于定位提供方并校验浏览器。
type loginState struct {
	Provider string `json:"p"`
	Nonce    string `json:"n"`
}

// usedStates 记录已使用的 state，防止回调被重放。
type usedStates struct {
	mu sync.Mutex
	m  map[string]time.Time
}

// consume 首次出现返回 true；同时清理已过期的记录。
func (u *usedStates) consume(key string, ttl time.Duration) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	now := time.Now()
	for k, exp := range u.m {
		if now.After(exp) {
			delete(u.m, k)
		}
	}
	if _, seen := u.m[key]; seen {
		return false
	}
	if u.m == nil {
		u.m = map[string]time.Time{}
	}
	u.m[key] = now.Add(ttl)
	return true
}

// loginSecret 由会话密钥派生，避免 state 与会话令牌共用同一把密钥。
func (a *App) loginSecret() []byte {
	m := hmac.New(sha256.New, []byte(a.auth.EntrySecret()))
	m.Write([]byte("third-login"))
	return m.Sum(nil)
}

// handleThirdProviders 返回登录页可用的第三方登录方式。
func (a *App) handleThirdProviders(w http.ResponseWriter, r *http.Request) {
	type item struct {
		Key  string `json:"key"`
		Name string `json:"name"`
		Type string `json:"type"`
		// URL 为发起登录的完整地址；回调地址所在主机与当前访问主机不同时指向回调主机
		URL string `json:"url"`
	}
	cfg := a.settingsSnapshot().ThirdLogin
	out := []item{}
	if cfg.Enable && cfg.CallbackBase != "" {
		for _, p := range thirdauth.Providers(cfg.Allowed, cfg.ProviderMap()) {
			out = append(out, item{Key: p.Key, Name: p.DisplayName(), Type: p.Type, URL: cfg.CallbackBase + thirdauth.StartPath + "?p=" + url.QueryEscape(p.Key)})
		}
	}
	resp := map[string]any{"providers": out}
	// 经统一网关访问且可用飞牛登录态登录时，登录页显示“使用飞牛账号登录”
	if user, ok := a.gatewaySSOUser(r); ok {
		resp["fnosUser"] = user
	}
	httpx.OK(w, resp)
}

// loginFailure 把失败原因带回登录页展示。
func loginFailure(w http.ResponseWriter, r *http.Request, msg string) {
	// 前端使用 hash 路由
	http.Redirect(w, r, "/#/login?oauthError="+url.QueryEscape(msg), http.StatusFound)
}

// handleThirdStart 发起第三方登录。
func (a *App) handleThirdStart(w http.ResponseWriter, r *http.Request) {
	cfg := a.settingsSnapshot().ThirdLogin
	p, ok := cfg.ProviderMap()[r.URL.Query().Get("p")]
	if !cfg.Enable || !ok {
		loginFailure(w, r, "第三方登录未启用")
		return
	}
	// 登录必须在回调基础地址的主机上发起，浏览器才会带着发起时写入的 nonce 回到回调
	if base, err := url.Parse(cfg.CallbackBase); err != nil || !strings.EqualFold(base.Host, r.Host) {
		http.Error(w, "请通过 "+cfg.CallbackBase+" 访问管理界面后再使用第三方登录", http.StatusBadRequest)
		return
	}
	nonce := thirdauth.RandomNonce()
	state := thirdauth.Sign(a.loginSecret(), loginStateKind, loginState{Provider: p.Key, Nonce: nonce}, loginStateTTL)
	target, err := p.AuthorizeURL(r.Context(), cfg.CallbackURL(), state)
	if err != nil {
		loginFailure(w, r, "无法连接身份提供方: "+err.Error())
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: loginNonceCookie, Value: nonce, Path: loginNoncePath, MaxAge: int(loginStateTTL / time.Second),
		HttpOnly: true, Secure: strings.HasPrefix(cfg.CallbackBase, "https://"), SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, target, http.StatusFound)
}

// handleThirdCallback 处理身份提供方的回调：校验 state、换取身份、检查允许列表后签发管理会话。
func (a *App) handleThirdCallback(w http.ResponseWriter, r *http.Request) {
	cfg := a.settingsSnapshot().ThirdLogin
	ip := httpx.ClientIP(r)
	rawState := r.URL.Query().Get("state")
	var st loginState
	if err := thirdauth.Verify(a.loginSecret(), loginStateKind, rawState, &st); err != nil {
		loginFailure(w, r, "登录状态无效: "+err.Error())
		return
	}
	if !a.loginUsed.consume(rawState, loginStateTTL) {
		loginFailure(w, r, "登录状态已使用，请重新登录")
		return
	}
	// state 必须由发起登录的同一浏览器使用，防止把他人的授权码注入受害者浏览器
	c, err := r.Cookie(loginNonceCookie)
	if err != nil || !hmac.Equal([]byte(c.Value), []byte(st.Nonce)) {
		loginFailure(w, r, "登录状态与当前浏览器不匹配，请重新登录")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: loginNonceCookie, Value: "", Path: loginNoncePath, MaxAge: -1})
	p, ok := cfg.ProviderMap()[st.Provider]
	if !cfg.Enable || !ok {
		loginFailure(w, r, "第三方登录配置已变化，请重试")
		return
	}
	if e := r.URL.Query().Get("error"); e != "" {
		loginFailure(w, r, "身份提供方拒绝登录: "+e)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), thirdauth.HTTPTimeout)
	defer cancel()
	id, err := p.Exchange(ctx, r.URL.Query().Get("code"), cfg.CallbackURL())
	if err != nil {
		a.sysLogs.Add(logbuf.LevelWarn, "auth", "来自 "+ip+" 的第三方登录失败: "+err.Error())
		loginFailure(w, r, "登录失败: "+err.Error())
		return
	}
	if !thirdauth.Allowed(cfg.Allowed, id) {
		a.sysLogs.Add(logbuf.LevelWarn, "auth", "来自 "+ip+" 的第三方登录被拒绝，账号 "+id.String())
		a.audit.Record(audit.Entry{Actor: id.String(), IP: ip, Via: audit.Via(r), Category: audit.CategoryLogin, Method: "GET", Path: thirdauth.CallbackPath, Status: http.StatusForbidden, Detail: "第三方登录被拒绝"})
		// 提示完整标识，方便把它加入允许列表（例如 QQ 的 openid 对每个应用不同）
		loginFailure(w, r, "账号 "+id.String()+" 不在允许登录的身份列表中")
		return
	}
	if a.auth.TOTPEnabled() && !cfg.SkipTOTP {
		loginFailure(w, r, "已启用两步验证：请使用密码登录，或在设置中允许第三方登录跳过两步验证")
		return
	}
	token, exp := a.auth.Issue()
	// 整页跳转带的是浏览器自身的语言，不代表界面语言，不在这里记录；进入管理界面后由前端上报
	a.sysLogs.Add(logbuf.LevelInfo, "auth", "通过第三方登录 "+id.String()+" 从 "+ip+" 登录")
	a.audit.Record(audit.Entry{Actor: id.String(), IP: ip, Via: audit.Via(r), Category: audit.CategoryLogin, Method: "GET", Path: thirdauth.CallbackPath, Status: http.StatusFound, Detail: "第三方登录"})
	a.noteLoginIP(ip, id.String(), "第三方登录")
	http.SetCookie(w, &http.Cookie{
		Name: auth.CookieName, Value: token, Path: "/", Expires: exp,
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil || strings.HasPrefix(cfg.CallbackBase, "https://"),
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

// ---- 从 Lucky 迁移 ----

// importLuckyIdentities 导入 Lucky 第三方登录管理中的身份表（lucky_thirdPartyAuthManager）。
// 身份表保存在本地，之后导入站点与基础设置时按 Key 解析出允许的身份。
func (a *App) importLuckyIdentities(doc luckyimport.Document, ids *thirdauth.LuckyIdentities, opts luckyimport.Options) *luckyimport.Report {
	raw, ok := doc["List"]
	if _, hasConf := doc["Config"]; !ok || !hasConf {
		return nil
	}
	var list []thirdauth.LuckyIdentity
	if err := json.Unmarshal(raw, &list); err != nil || len(list) == 0 || list[0].Key == "" || list[0].Type == "" || list[0].ID == "" {
		return nil
	}
	rep := &luckyimport.Report{Module: "thirdauth", Title: "第三方登录身份", Found: len(list)}
	index := map[string]int{}
	for i, id := range ids.List {
		index[id.Key] = i
	}
	hasQQ := false
	var unsupported []string
	for _, id := range list {
		id.Type = strings.ToLower(id.Type)
		switch id.Type {
		case thirdauth.TypeGitHub, thirdauth.TypeGoogle:
		case thirdauth.TypeQQ:
			hasQQ = true
		default:
			unsupported = append(unsupported, id.Type)
		}
		if i, exists := index[id.Key]; exists {
			ids.List[i] = id
		} else {
			index[id.Key] = len(ids.List)
			ids.List = append(ids.List, id)
		}
		rep.Imported++
	}
	if len(unsupported) > 0 {
		rep.Warnings = append(rep.Warnings, "暂不支持的登录类型（仍记录身份，但无法用于登录）："+strings.Join(unsupported, "、"))
	}
	if hasQQ {
		rep.Warnings = append(rep.Warnings, "QQ 的 openid 对每个 QQ 互联应用不同：换成你自己的 QQ 应用后，首次登录会提示新的账号标识，把它加入允许列表即可")
	}
	rep.Warnings = append(rep.Warnings, "身份已记录；站点与管理后台引用的身份会在导入反向代理与基础设置时迁移，登录前需自行配置 OAuth 应用")
	if opts.DryRun {
		return rep
	}
	if err := ids.Save(a.store); err != nil {
		rep.Imported = 0
		rep.Warnings = append(rep.Warnings, "保存失败: "+err.Error())
	}
	return rep
}

// mergeLuckyThirdLogin 把 Lucky 基础设置中的管理后台第三方登录迁移到设置里。
// OAuth 应用需要用户自己申请，因此只迁移允许的身份与跳过两步验证的选项，不会自动启用。
func (a *App) mergeLuckyThirdLogin(s *Settings, lb luckyBase, opts luckyimport.Options, rep *luckyimport.Report) {
	keys := luckyKeys(lb.ThirdAuthLoginUserList)
	if !lb.EnableThirdAuthLogin && !lb.AllowAllThirdAuthUsers && len(keys) == 0 {
		return
	}
	if opts.ResolveThirdParty == nil {
		rep.Warnings = append(rep.Warnings, "Lucky 开启了管理后台第三方登录，但未导入身份表：请同时导入 lucky_thirdPartyAuthManager.lkcf 后再导入基础设置")
		return
	}
	entries, missing := opts.ResolveThirdParty(keys, lb.AllowAllThirdAuthUsers)
	have := map[string]bool{}
	for _, e := range s.ThirdLogin.Allowed {
		have[e] = true
	}
	for _, e := range entries {
		if !have[e] {
			s.ThirdLogin.Allowed = append(s.ThirdLogin.Allowed, e)
			have[e] = true
		}
	}
	s.ThirdLogin.SkipTOTP = s.ThirdLogin.SkipTOTP || lb.ThirdAuthLoginSkipTwoFA
	msg := fmt.Sprintf("管理后台第三方登录：已迁移 %d 个允许的身份，需在「系统设置 → 第三方登录」填写你自己的 OAuth 应用与回调基础地址后启用", len(entries))
	if lb.ThirdAuthLoginSkipTwoFA {
		msg += "；已沿用“第三方登录跳过两步验证”"
	}
	rep.Warnings = append(rep.Warnings, msg)
	if missing > 0 {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("管理后台引用的 %d 个第三方身份在身份表中找不到，已忽略", missing))
	}
}

// luckyKeys 把 JSON 解码出的任意值转换为字符串列表。
func luckyKeys(v any) []string {
	arr, _ := v.([]any)
	var out []string
	for _, x := range arr {
		if s, ok := x.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}
