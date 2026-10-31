package webservice

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/fileusers"
	"github.com/wangwanjie/gately/internal/i18n"
	"github.com/wangwanjie/gately/internal/thirdauth"
	"github.com/wangwanjie/gately/internal/webpage"
)

// 网页认证使用的保留路径与会话时长
const (
	authPath     = "/__gately_auth"
	sessionTTL   = 7 * 24 * time.Hour
	maxFailures  = 10
	failureReset = 15 * time.Minute
)

// authenticator 处理站点的基本认证与网页认证（登录页 + 签名 Cookie）。
type authenticator struct {
	secret []byte
	mu     sync.Mutex
	fails  map[string]*failCount
	// used 记录已使用的一次性令牌（OAuth state 与票据）及其过期时间
	used map[string]int64
	// oauth 为第三方登录配置，设置变更时整体替换
	oauthMu sync.RWMutex
	oauth   oauthEnv

	// bans 为共享封禁表；banMu 保护自动封禁时长；shareUses 记录分享链接已兑换次数（受 mu 保护）
	bans      *banlist.Store
	banMu     sync.Mutex
	autoBan   time.Duration
	shareUses map[string]int

	// shared 为共享认证用户与单点登录域，设置变更时整体替换
	shared sharedStore
	// oidc 为内置 OIDC 身份提供方的配置，未启用时为空
	oidc oidcHolder

	// selfAuth 为是否允许共享认证用户自助修改密码；files 为文件服务共享用户（自助改密开关保存在 fileusers.json）
	selfAuth atomic.Bool
	files    fileUserStore
	// changeShared 保存共享认证用户的新密码哈希（u 为校验通过时的用户），由模块设置
	changeShared func(u AuthUser, hash string) error
	// onPasswordChanged 在自助修改密码成功后调用，用于记录日志
	onPasswordChanged func(kind, name, ip string)
	// onPasswordFailed 在当前密码校验通过、但保存失败时调用，记录详细原因
	onPasswordFailed func(kind, name, ip string, err error)
	// pwChangedAt 记录各账号最近一次自助修改密码的时间（键见 cooldownKey），用于冷却
	pwChangedAt sync.Map
}

func (a *authenticator) setOAuth(env oauthEnv) {
	a.oauthMu.Lock()
	a.oauth = env
	a.oauthMu.Unlock()
}

func (a *authenticator) oauthEnv() oauthEnv {
	a.oauthMu.RLock()
	defer a.oauthMu.RUnlock()
	return a.oauth
}

type failCount struct {
	n        int
	inflight int
	first    time.Time
}

func newAuthenticator(secret string) *authenticator {
	return &authenticator{secret: []byte(secret), fails: map[string]*failCount{}, used: map[string]int64{},
		bans: banlist.Default, shareUses: map[string]int{}, files: fileusers.Default}
}

// verifyUser 校验用户名密码，密码支持明文（导入的配置）与 bcrypt 哈希。
func verifyUser(users []string, name, pass string) bool {
	ok := false
	for _, u := range users {
		un, up, _ := strings.Cut(u, ":")
		if subtle.ConstantTimeCompare([]byte(un), []byte(name)) != 1 {
			continue
		}
		if strings.HasPrefix(up, "$2") {
			ok = bcrypt.CompareHashAndPassword([]byte(up), []byte(pass)) == nil
		} else {
			ok = subtle.ConstantTimeCompare([]byte(up), []byte(pass)) == 1
		}
		if ok {
			return true
		}
	}
	return false
}

// limitKey 返回限流键：IPv6 按 /64 聚合，避免攻击者轮换同一网段内的地址绕过限制。
func limitKey(ip string) string {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}
	a = a.Unmap()
	if a.Is6() {
		p, _ := a.Prefix(64)
		return p.String()
	}
	return a.String()
}

// reserve 为一次密码校验占位；失败次数加上进行中的校验达到上限时拒绝，防止并发绕过。
func (a *authenticator) reserve(key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	// 表过大时清理过期项，防止大量来源地址撑大内存
	if len(a.fails) > 4096 {
		for k, f := range a.fails {
			if f.inflight == 0 && now.Sub(f.first) > failureReset {
				delete(a.fails, k)
			}
		}
	}
	f := a.fails[key]
	if f == nil || (f.inflight == 0 && now.Sub(f.first) > failureReset) {
		f = &failCount{first: now}
		a.fails[key] = f
	}
	if f.n+f.inflight >= maxFailures {
		return false
	}
	f.inflight++
	return true
}

// release 结束占位并记录结果；成功不清零计数，避免交替使用正确密码重置限流。
func (a *authenticator) release(key string, ok bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	f := a.fails[key]
	if f == nil {
		return
	}
	f.inflight--
	if !ok {
		f.n++
		// 恰好达到上限时触发一次自动封禁
		if f.n == maxFailures {
			a.autoBanFor(key)
		}
	}
}

// consumeOnce 标记一次性令牌已使用，重复使用返回 false。
func (a *authenticator) consumeOnce(token string, ttl time.Duration) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now().Unix()
	for k, exp := range a.used {
		if exp < now {
			delete(a.used, k)
		}
	}
	if _, seen := a.used[token]; seen {
		return false
	}
	a.used[token] = now + int64(ttl/time.Second) + 1
	return true
}

// cookieName 按 监听器/站点 区分 Cookie，避免同域名下不同站点或端口共享登录态。
func cookieName(s Site) string {
	scope := s.scope
	if scope == "" {
		scope = s.Key
	}
	sum := sha256.Sum256([]byte(scope))
	return "gately_site_" + hex.EncodeToString(sum[:6])
}

// siteSession 是网页认证会话 Cookie 的内容。
type siteSession struct {
	User  string `json:"u"`
	Host  string `json:"h"`
	Scope string `json:"s"`
	// Cred 为密码用户的凭据指纹，修改密码后旧会话失效
	Cred string `json:"c,omitempty"`
	// Aliases 为第三方登录身份的其他标识，校验允许列表时与 User 一起匹配
	Aliases []string `json:"a,omitempty"`
	// Shared 表示共享认证用户的会话（Cred 为共享用户凭据指纹，Scope 为 sso 时跨站有效）
	Shared bool `json:"sh,omitempty"`
}

// credVersion 返回用户当前凭据的指纹（HMAC，Cookie 中不暴露密码哈希）。
func (a *authenticator) credVersion(users []string, name string) string {
	for _, u := range users {
		if un, _, _ := strings.Cut(u, ":"); un == name {
			m := hmac.New(sha256.New, a.secret)
			m.Write([]byte("cred|" + u))
			return hex.EncodeToString(m.Sum(nil)[:8])
		}
	}
	return ""
}

// issueSession 写出站点会话 Cookie。
func (a *authenticator) issueSession(w http.ResponseWriter, r *http.Request, s Site, user string, aliases ...string) {
	exp := time.Now().Add(sessionTTL)
	sess := siteSession{User: user, Host: hostOnly(r.Host), Scope: s.scope, Cred: a.credVersion(s.Users, user), Aliases: aliases}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName(s), Value: a.signBlob("session", sess, sessionTTL),
		Path: "/", Expires: exp, HttpOnly: true, Secure: requestIsHTTPS(r), SameSite: http.SameSiteLaxMode,
	})
}

// validCookie 校验会话 Cookie；用户被删除或密码修改后会话立即失效。
func (a *authenticator) validCookie(r *http.Request, s Site) bool {
	_, ok := a.cookieSession(r, s)
	return ok
}

// cookieSession 返回站点会话 Cookie 中通过校验的会话。
func (a *authenticator) cookieSession(r *http.Request, s Site) (*siteSession, bool) {
	c, err := r.Cookie(cookieName(s))
	if err != nil {
		return nil, false
	}
	var sess siteSession
	if err := a.verifyBlob("session", c.Value, &sess); err != nil {
		return nil, false
	}
	if sess.Shared {
		return &sess, s.SharedUsers && a.validShared(r, s, &sess)
	}
	if sess.Host != hostOnly(r.Host) || sess.Scope != s.scope {
		return nil, false
	}
	for _, u := range s.Users {
		if name, _, _ := strings.Cut(u, ":"); name == sess.User {
			return &sess, sess.Cred != "" && hmac.Equal([]byte(sess.Cred), []byte(a.credVersion(s.Users, sess.User)))
		}
	}
	// 第三方登录的会话用户为 提供方:标识，允许列表变化后立即失效
	if provider, id, ok := strings.Cut(sess.User, ":"); ok && sess.Cred == "" {
		return &sess, identityAllowed(s, thirdauth.Identity{Provider: provider, Primary: id, Aliases: sess.Aliases})
	}
	return nil, false
}

// sessionNames 返回会话对应的身份标识：第一项为主标识（用户名或 提供方:标识），第三方登录另附 提供方:其他标识
func sessionNames(sess *siteSession) []string {
	names := []string{sess.User}
	if provider, _, ok := strings.Cut(sess.User, ":"); ok && !sess.Shared && sess.Cred == "" {
		for _, alias := range sess.Aliases {
			names = append(names, provider+":"+alias)
		}
	}
	return names
}

// sessionIdentity 只读取已登录的会话（站点 Cookie 或单点登录 Cookie），不要求登录，用于无需认证的路径识别用户
func (a *authenticator) sessionIdentity(r *http.Request, s Site) []string {
	if !s.WebAuth {
		return nil
	}
	if sess, ok := a.cookieSession(r, s); ok {
		return sessionNames(sess)
	}
	if s.SharedUsers {
		if sess, ok := a.ssoSession(r, s); ok {
			return sessionNames(sess)
		}
	}
	return nil
}

// isBrowser 粗略判断是否为浏览器访问，非浏览器回退到基本认证。
func isBrowser(r *http.Request) bool {
	ua := r.UserAgent()
	return strings.Contains(ua, "Mozilla/") && strings.Contains(r.Header.Get("Accept"), "text/html")
}

// check 执行认证，返回 true 表示放行；否则已写出响应。
func (a *authenticator) check(w http.ResponseWriter, r *http.Request, s Site) bool {
	_, ok := a.checkUser(w, r, s)
	return ok
}

// checkUser 与 check 相同，放行时同时返回访客的身份标识（见 sessionNames），基本认证为用户名。
func (a *authenticator) checkUser(w http.ResponseWriter, r *http.Request, s Site) ([]string, bool) {
	ip := clientAddr(r).Addr().String()
	if s.WebAuth {
		switch r.URL.Path {
		case oauthStart:
			a.handleOAuthStart(w, r, s, a.oauthEnv())
			return nil, false
		case oauthFinish:
			a.handleOAuthFinish(w, r, s)
			return nil, false
		}
		if strings.HasPrefix(r.URL.Path, authPath) {
			a.handleLogin(w, r, s, ip)
			return nil, false
		}
		if sess, ok := a.cookieSession(r, s); ok {
			return sessionNames(sess), true
		}
		if s.SharedUsers {
			if sess, ok := a.ssoSession(r, s); ok {
				return sessionNames(sess), true
			}
		}
		if isBrowser(r) {
			target := authPath + "/login?next=" + url.QueryEscape(r.URL.RequestURI())
			http.Redirect(w, r, target, http.StatusFound)
			return nil, false
		}
	}
	// 基本认证（网页认证对非浏览器的回退）；只配置了第三方身份时非浏览器无法登录
	if len(s.Users) == 0 && !s.SharedUsers {
		http.Error(w, "401 Unauthorized", http.StatusUnauthorized)
		return nil, false
	}
	if name, pass, ok := r.BasicAuth(); ok {
		key := limitKey(ip)
		if !a.reserve(key) {
			http.Error(w, "429 Too Many Requests", http.StatusTooManyRequests)
			return nil, false
		}
		valid := verifyUser(s.Users, name, pass)
		if !valid && s.SharedUsers {
			// 基本认证无法输入动态验证码，启用了 TOTP 的共享用户只能通过网页登录
			if u, ok := a.shared.get().checkPassword(name, pass); ok && u.TOTPSecret == "" {
				valid = true
			}
		}
		a.release(key, valid)
		if valid {
			return []string{name}, true
		}
	}
	w.Header().Set("WWW-Authenticate", `Basic realm="Gately", charset="UTF-8"`)
	http.Error(w, "401 Unauthorized", http.StatusUnauthorized)
	return nil, false
}

// loginSrc 是网页认证登录页的主体（外壳与样式见 webpage 包）。设置了 Pending 时在登录卡片上弹出动态验证码对话框。
// 模板内不写中文（包括脚本注释），否则整段会被 i18nsync 当作翻译键；脚本依次处理：显示/隐藏密码、
// 提交后禁用按钮防止重复提交、验证码只保留数字且输满 6 位自动提交。页面不依赖脚本也能正常登录。
// 验证码对话框打开时锁定页面滚动（body:has(.modal)），滚轮和触摸滑动不穿透到下方的登录卡片；屏幕过矮时对话框自身可滚动。
const loginSrc = `{{define "body"}}<section class="card login"{{if .Pending}} aria-hidden="true"{{end}}>
<div class="icon"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="16" r="1"/><rect x="3" y="10" width="18" height="12" rx="2"/><path d="M7 10V7a5 5 0 0 1 10 0v3"/></svg></div>
<h1>{{.T.title}}</h1>
<p class="sub">{{if .Client}}{{printf .T.clientSubtitle .Client}}{{else}}{{.T.subtitle}}{{end}}</p>
<div class="chip"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20"/><path d="M2 12h20"/></svg><span>{{.Host}}</span></div>
{{if and .Error (not .Pending)}}<div class="alert" role="alert"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M12 8v4"/><path d="M12 16h.01"/></svg><span>{{.Error}}</span></div>{{end}}
{{if .HasPassword}}<form method="post" action="{{.Action}}" data-once>
<div class="fields">
<label class="f"><span>{{.T.username}}</span><div class="ctl"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="8" r="5"/><path d="M20 21a8 8 0 0 0-16 0"/></svg><input name="username" value="{{.Username}}" placeholder="{{.T.usernamePh}}" autocomplete="username" autocapitalize="none" spellcheck="false" required{{if not .Username}} autofocus{{end}}></div></label>
<label class="f"><span>{{.T.password}}</span><div class="ctl"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="18" height="11" x="3" y="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg><input id="pw" name="password" type="password" placeholder="{{.T.passwordPh}}" autocomplete="current-password" required{{if .Username}} autofocus{{end}}><button type="button" class="eye" id="eye" title="{{.T.showPassword}}" aria-label="{{.T.showPassword}}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2.06 12.35a1 1 0 0 1 0-.7 10.75 10.75 0 0 1 19.88 0 1 1 0 0 1 0 .7 10.75 10.75 0 0 1-19.88 0"/><circle cx="12" cy="12" r="3"/></svg></button></div></label>
</div>
<input type="hidden" name="next" value="{{.Next}}">
<button class="btn primary" type="submit">{{.T.submit}}<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14"/><path d="m12 5 7 7-7 7"/></svg></button>
</form>{{end}}
{{if .PasswordURL}}<a class="back" href="{{.PasswordURL}}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2.586 17.414A2 2 0 0 0 2 18.828V21a1 1 0 0 0 1 1h3a1 1 0 0 0 1-1v-1a1 1 0 0 1 1-1h1a1 1 0 0 0 1-1v-1a1 1 0 0 1 1-1h.172a2 2 0 0 0 1.414-.586l.814-.814a6.5 6.5 0 1 0-4-4z"/><circle cx="16.5" cy="7.5" r=".5" fill="currentColor"/></svg>{{.T.changePassword}}</a>{{end}}
{{if .Providers}}{{if .HasPassword}}<div class="or">{{.T.or}}</div>{{else}}<div class="gap"></div>{{end}}
<div class="oauth">{{range .Providers}}<a class="btn ghost" href="{{$.StartURL}}?p={{.Key}}&next={{$.Next}}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2.586 17.414A2 2 0 0 0 2 18.828V21a1 1 0 0 0 1 1h3a1 1 0 0 0 1-1v-1a1 1 0 0 1 1-1h1a1 1 0 0 0 1-1v-1a1 1 0 0 1 1-1h.172a2 2 0 0 0 1.414-.586l.814-.814a6.5 6.5 0 1 0-4-4z"/><circle cx="16.5" cy="7.5" r=".5" fill="currentColor"/></svg>{{printf $.T.loginWith .Name}}</a>{{end}}</div>{{end}}
</section>
{{if .Pending}}<div class="modal" role="dialog" aria-modal="true" aria-labelledby="totp-title">
<form class="card sheet" method="post" action="{{.Action}}" data-once>
<div class="icon"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/><path d="m9 12 2 2 4-4"/></svg></div>
<h1 id="totp-title">{{.T.totpTitle}}</h1>
<p class="sub">{{.T.totpPrompt}}</p>
<div class="chip"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="8" r="5"/><path d="M20 21a8 8 0 0 0-16 0"/></svg><span>{{.Username}}</span></div>
{{if .Error}}<div class="alert" role="alert"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M12 8v4"/><path d="M12 16h.01"/></svg><span>{{.Error}}</span></div>{{end}}
<input class="otp{{if .Error}} shake{{end}}" id="otp" name="code" inputmode="numeric" pattern="[0-9]{6}" autocomplete="one-time-code" maxlength="6" placeholder="000000" required autofocus aria-label="{{.T.code}}">
<input type="hidden" name="pending" value="{{.Pending}}">
<input type="hidden" name="next" value="{{.Next}}">
<button class="btn primary" type="submit">{{.T.verify}}</button>
<p class="tip">{{.T.totpAuto}}</p>
<a class="back" href="{{.Action}}?next={{.Next}}&amp;u={{.Username}}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m12 19-7-7 7-7"/><path d="M19 12H5"/></svg>{{.T.back}}</a>
</form></div>{{end}}
<style>
.login .gap{height:24px}
body:has(.modal){overflow:hidden}
.modal{position:fixed;inset:0;z-index:10;display:grid;place-items:center;padding:16px;overflow:auto;overscroll-behavior:contain;background:rgba(8,18,16,.42);-webkit-backdrop-filter:blur(8px);backdrop-filter:blur(8px);animation:fade .2s ease-out both}
@keyframes fade{from{opacity:0}to{opacity:1}}
.sheet{max-width:380px;text-align:center;animation:pop .25s cubic-bezier(.2,.9,.3,1.2) both}
@keyframes pop{from{opacity:0;transform:scale(.94)}to{opacity:1;transform:none}}
.sheet .icon{margin:0 auto}
.sheet .sub{text-wrap:balance}
.sheet .alert{text-align:left}
.otp{display:block;width:100%;height:60px;margin-top:22px;padding:0 0 0 .55em;border:1px solid var(--line);border-radius:14px;background:var(--input);color:var(--text);text-align:center;font:650 30px/1 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;letter-spacing:.55em;outline:none;transition:border-color .15s,box-shadow .15s}
.otp::placeholder{color:var(--line)}
.otp:focus{border-color:var(--brand);background:var(--card);box-shadow:0 0 0 4px var(--brand-soft)}
.otp.shake{border-color:var(--danger);animation:shake .4s}
@keyframes shake{20%,60%{transform:translateX(-6px)}40%,80%{transform:translateX(6px)}}
.sheet .btn.primary{margin-top:16px}
.tip{margin:12px 0 0;color:var(--faint);font-size:12.5px}
.back{display:inline-flex;align-items:center;gap:6px;margin-top:14px;padding:4px 8px;border-radius:8px;color:var(--muted);text-decoration:none;font-size:13px}
.back:hover{color:var(--text);background:var(--brand-soft)}
.back svg{width:15px;height:15px}
</style>
<script>
(function(){
  var eye=document.getElementById('eye'),pw=document.getElementById('pw');
  if(eye&&pw){eye.addEventListener('click',function(){pw.type=pw.type==='password'?'text':'password';pw.focus();});}
  document.querySelectorAll('form[data-once]').forEach(function(f){
    f.addEventListener('submit',function(){var b=f.querySelector('button[type=submit]');if(b){setTimeout(function(){b.disabled=true;},0);}});
  });
  var otp=document.getElementById('otp');
  if(otp){
    otp.focus();
    otp.addEventListener('input',function(){
      otp.classList.remove('shake');
      var v=otp.value.replace(/\D/g,'').slice(0,6);
      if(v!==otp.value){otp.value=v;}
      if(v.length===6){otp.form.requestSubmit?otp.form.requestSubmit():otp.form.submit();}
    });
  }
})();
</script>{{end}}`

// loginTexts 为登录页文案，中文为翻译键。
var loginTexts = map[string]string{
	"pageTitle":      "登录",
	"title":          "登录以继续访问",
	"subtitle":       "此站点已开启访问保护，请先验证身份",
	"clientSubtitle": "验证身份后将继续登录 %s",
	"username":       "用户名",
	"usernamePh":     "请输入用户名",
	"password":       "密码",
	"passwordPh":     "请输入密码",
	"showPassword":   "显示或隐藏密码",
	"submit":         "登录",
	"or":             "或使用以下方式",
	"loginWith":      "使用 %s 登录",
	"totpTitle":      "两步验证",
	"totpPrompt":     "打开验证器应用，输入当前显示的 6 位动态验证码",
	"code":           "动态验证码",
	"verify":         "验证",
	"totpAuto":       "输满 6 位后自动提交",
	"back":           "返回重新登录",
	"changePassword": "修改密码",
}

func init() {
	webpage.MustRegister("ws-login", loginSrc)
}

// safeNext 只允许站内相对路径，防止开放重定向。
func safeNext(next string) string {
	if next == "" || next[0] != '/' || strings.ContainsRune(next, '\\') {
		return "/"
	}
	if len(next) > 1 && next[1] == '/' {
		return "/"
	}
	for _, c := range next {
		if c < 0x20 || c == 0x7f {
			return "/"
		}
	}
	u, err := url.Parse(next)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil || strings.HasPrefix(u.Path, authPath) {
		return "/"
	}
	return next
}

// loginProvider 是登录页展示的第三方登录入口。
type loginProvider struct{ Key, Name string }

// siteProviders 返回站点允许、且已配置的第三方登录提供方。
func (a *authenticator) siteProviders(s Site) []loginProvider {
	env := a.oauthEnv()
	if env.callbackBase == "" {
		return nil
	}
	var out []loginProvider
	for _, p := range thirdauth.Providers(s.OAuthUsers, env.providers) {
		out = append(out, loginProvider{Key: p.Key, Name: p.DisplayName()})
	}
	return out
}

// totpStep 是密码校验通过、等待输入动态验证码时签发的凭据：验证码步骤无需再次提交密码。
// 绑定用户凭据指纹、域名与站点，修改密码或 TOTP 后立即失效。
type totpStep struct {
	User  string `json:"u"`
	Cred  string `json:"c"`
	Host  string `json:"h"`
	Scope string `json:"s"`
}

// totpStepTTL 为输入动态验证码的时限。
const totpStepTTL = 5 * time.Minute

func (a *authenticator) handleLogin(w http.ResponseWriter, r *http.Request, s Site, ip string) {
	host := hostOnly(r.Host)
	base := s.authPrefix()
	if r.URL.Path == base+passwordSuffix {
		a.handlePassword(w, r, s, ip)
		return
	}
	data := map[string]any{
		"Host": host, "Action": base + "/login", "Next": safeNext(r.URL.Query().Get("next")), "Client": s.loginClient,
		"HasPassword": len(s.Users) > 0 || s.SharedUsers, "Providers": a.siteProviders(s), "StartURL": oauthStart,
		// 从验证码对话框返回时带回用户名，免得重新输入
		"Username": r.URL.Query().Get("u"),
	}
	if authKind, _ := a.selfServiceKinds(s); authKind {
		data["PasswordURL"] = base + passwordSuffix
	}
	if r.URL.Path == base+"/logout" {
		http.SetCookie(w, &http.Cookie{Name: cookieName(s), Value: "", Path: "/", MaxAge: -1})
		if domain := a.shared.get().ssoFor(host); domain != "" && s.SharedUsers {
			// 单点登录的会话写在父域，退出时一并清除
			http.SetCookie(w, &http.Cookie{Name: ssoCookie, Value: "", Path: "/", Domain: domain, MaxAge: -1})
		}
		http.Redirect(w, r, base+"/login", http.StatusFound)
		return
	}
	status := http.StatusOK
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		data["Next"] = safeNext(r.PostForm.Get("next"))
		next, _ := data["Next"].(string)
		key := limitKey(ip)
		if !sameOriginPost(r) {
			// 其他网站代为提交的登录表单（登录 CSRF）：不校验密码，也不计入失败次数
			data["Error"] = "请求来源无效，请刷新登录页后重试"
			status = http.StatusForbidden
		} else if !a.reserve(key) {
			data["Error"] = "失败次数过多，请稍后再试"
		} else if token := r.PostForm.Get("pending"); token != "" && s.SharedUsers {
			// 第二步：校验动态验证码
			u, msg, expired := a.finishTOTPStep(token, host, s, r.PostForm.Get("code"))
			a.release(key, msg == "")
			if msg == "" {
				a.issueSharedSession(w, r, s, u)
				http.Redirect(w, r, next, http.StatusFound)
				return
			}
			data["Error"] = msg
			if !expired {
				// 验证码错误时保持在验证码对话框，凭据仍在有效期内可继续输入
				data["Pending"], data["Username"] = token, u.Name
			}
		} else {
			user := r.PostForm.Get("username")
			pass := r.PostForm.Get("password")
			valid := len(s.Users) > 0 && verifyUser(s.Users, user, pass)
			msg := "用户名或密码错误"
			var shared AuthUser
			needCode := false
			if !valid && s.SharedUsers {
				var errMsg string
				shared, errMsg, needCode = a.loginShared(user, pass, r.PostForm.Get("code"))
				valid = errMsg == ""
				if !valid {
					msg = errMsg
				}
			}
			// 密码正确但需要验证码：本次校验不计入失败次数，转入验证码对话框
			a.release(key, valid || needCode)
			switch {
			case valid:
				if shared.Name != "" {
					a.issueSharedSession(w, r, s, shared)
				} else {
					a.issueSession(w, r, s, user)
				}
				http.Redirect(w, r, next, http.StatusFound)
				return
			case needCode:
				data["Pending"] = a.signBlob("totp-step", totpStep{User: shared.Name, Cred: a.sharedCred(shared), Host: host, Scope: s.scope}, totpStepTTL)
				data["Username"] = shared.Name
			default:
				data["Error"] = msg
				data["Username"] = user
			}
		}
	}
	if _, failed := data["Error"]; failed && status == http.StatusOK {
		status = http.StatusUnauthorized
	}
	lang := i18n.RequestLang(r)
	if msg, ok := data["Error"].(string); ok {
		data["Error"] = i18n.Tr(lang, msg)
	}
	data["PageTitle"] = i18n.Tr(lang, loginTexts["pageTitle"]) + " · " + host
	webpage.Render(w, status, "ws-login", lang, loginTexts, data)
}

// sameOriginPost 判断表单是否由本域名的页面提交：有 Origin 头时其域名需与请求的域名一致（Origin: null 视为跨站），
// 没有 Origin 时再看 Referer；两者都没有（老旧客户端、脚本）时放行，以免影响非浏览器的登录。
func sameOriginPost(r *http.Request) bool {
	src := r.Header.Get("Origin")
	if src == "" {
		src = r.Header.Get("Referer")
		if src == "" {
			return true
		}
	}
	u, err := url.Parse(src)
	if err != nil || u.Host == "" {
		return false
	}
	return hostOnly(u.Host) == hostOnly(r.Host)
}

// finishTOTPStep 校验验证码步骤的凭据与动态验证码；expired 为 true 表示凭据无效或已过期，需要重新输入密码。
func (a *authenticator) finishTOTPStep(token, host string, s Site, code string) (u AuthUser, msg string, expired bool) {
	var st totpStep
	if err := a.verifyBlob("totp-step", token, &st); err != nil || st.Host != host || st.Scope != s.scope {
		return AuthUser{}, "验证已超时，请重新登录", true
	}
	u, ok := a.shared.get().users[st.User]
	if !ok || u.TOTPSecret == "" || !hmac.Equal([]byte(st.Cred), []byte(a.sharedCred(u))) {
		return AuthUser{}, "验证已超时，请重新登录", true
	}
	if msg := a.checkTOTP(u, code); msg != "" {
		return u, msg, false
	}
	return u, "", false
}

// hostOnly 去掉 Host 中的端口。
func hostOnly(h string) string {
	h = strings.ToLower(h)
	if i := strings.LastIndexByte(h, ':'); i > 0 && !strings.HasSuffix(h, "]") {
		return strings.Trim(h[:i], "[]")
	}
	return strings.Trim(h, "[]")
}
