package webservice

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/wangwanjie/gately/internal/fileusers"
	"github.com/wangwanjie/gately/internal/i18n"
	"github.com/wangwanjie/gately/internal/webpage"
)

// 自助修改密码页面挂在认证保留路径下：普通站点为 /__gately_auth/password，OIDC 签发者域名为 /__gately_oidc/password。
// 共享认证用户（含通过内置 OIDC 身份提供方登录的用户）与文件服务共享用户各有开关，都关闭时页面返回 404。
// 修改后共享认证用户的凭据指纹随之变化，各站点会话与已签发的 OIDC 令牌立即失效，需要用新密码重新登录。
const passwordSuffix = "/password"

// 自助修改密码的账号类型。
const (
	kindAuth = "auth"
	kindFile = "file"
)

// fileUserStore 是文件服务共享用户的自助改密接口，默认为 fileusers.Default，测试中替换。
type fileUserStore interface {
	SelfService() bool
	ChangePassword(name, current, next string) error
}

// selfServiceKinds 返回站点上可以自助修改密码的账号类型：共享认证用户要求站点开启了“使用共享用户”。
func (a *authenticator) selfServiceKinds(s Site) (auth, file bool) {
	auth = a.selfAuth.Load() && s.SharedUsers
	file = a.files != nil && a.files.SelfService()
	return auth, file
}

// passwordSrc 是自助修改密码页面的主体。模板内不写中文，文案见 passwordTexts。
// 两种账号都开启时显示类型切换，选择文件服务账号时隐藏动态验证码输入框（不依赖脚本也能提交，服务端会忽略该字段）。
const passwordSrc = `{{define "body"}}<section class="card">
{{if .Done}}<div class="icon soft"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="m9 12 2 2 4-4"/></svg></div>
<h1>{{.T.doneTitle}}</h1>
<p class="sub">{{if eq .Kind "auth"}}{{.T.doneAuth}}{{else}}{{.T.doneFile}}{{end}}</p>
<div class="chip"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="8" r="5"/><path d="M20 21a8 8 0 0 0-16 0"/></svg><span>{{.Username}}</span></div>
{{if .LoginURL}}<a class="btn primary" href="{{.LoginURL}}">{{.T.toLogin}}</a>{{end}}
{{else}}<div class="icon"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2.586 17.414A2 2 0 0 0 2 18.828V21a1 1 0 0 0 1 1h3a1 1 0 0 0 1-1v-1a1 1 0 0 1 1-1h1a1 1 0 0 0 1-1v-1a1 1 0 0 1 1-1h.172a2 2 0 0 0 1.414-.586l.814-.814a6.5 6.5 0 1 0-4-4z"/><circle cx="16.5" cy="7.5" r=".5" fill="currentColor"/></svg></div>
<h1>{{.T.title}}</h1>
<p class="sub">{{.T.subtitle}}</p>
<div class="chip"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20"/><path d="M2 12h20"/></svg><span>{{.Host}}</span></div>
{{if .Error}}<div class="alert" role="alert"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M12 8v4"/><path d="M12 16h.01"/></svg><span>{{.Error}}</span></div>{{end}}
<form method="post" action="{{.Action}}" data-once>
{{if and .AuthKind .FileKind}}<div class="seg" role="radiogroup">
<label><input type="radio" name="kind" value="auth"{{if eq .Kind "auth"}} checked{{end}}><span>{{.T.kindAuth}}</span></label>
<label><input type="radio" name="kind" value="file"{{if eq .Kind "file"}} checked{{end}}><span>{{.T.kindFile}}</span></label>
</div>{{else}}<input type="hidden" name="kind" value="{{.Kind}}">{{end}}
<div class="fields">
<label class="f"><span>{{.T.username}}</span><div class="ctl"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="8" r="5"/><path d="M20 21a8 8 0 0 0-16 0"/></svg><input name="username" value="{{.Username}}" placeholder="{{.T.usernamePh}}" autocomplete="username" autocapitalize="none" spellcheck="false" required{{if not .Username}} autofocus{{end}}></div></label>
<label class="f"><span>{{.T.current}}</span><div class="ctl"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="18" height="11" x="3" y="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg><input name="current" type="password" placeholder="{{.T.currentPh}}" autocomplete="current-password" required{{if .Username}} autofocus{{end}}></div></label>
{{if .AuthKind}}<label class="f" id="code-field"{{if eq .Kind "file"}} hidden{{end}}><span>{{.T.code}}</span><div class="ctl"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/></svg><input name="code" inputmode="numeric" pattern="[0-9]{6}" maxlength="6" placeholder="{{.T.codePh}}" autocomplete="one-time-code"></div></label>{{end}}
<label class="f"><span>{{.T.next}}</span><div class="ctl"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="18" height="11" x="3" y="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 9.9-1"/></svg><input name="next" type="password" placeholder="{{.T.nextPh}}" autocomplete="new-password" minlength="8" maxlength="128" required></div></label>
<label class="f"><span>{{.T.confirm}}</span><div class="ctl"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="18" height="11" x="3" y="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 9.9-1"/></svg><input name="confirm" type="password" placeholder="{{.T.confirmPh}}" autocomplete="new-password" minlength="8" maxlength="128" required></div></label>
</div>
<button class="btn primary" type="submit">{{.T.submit}}</button>
</form>
{{if .LoginURL}}<a class="back" href="{{.LoginURL}}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m12 19-7-7 7-7"/><path d="M19 12H5"/></svg>{{.T.backLogin}}</a>{{end}}
{{end}}</section>
<style>
.seg{display:grid;grid-template-columns:1fr 1fr;gap:4px;margin-top:22px;padding:4px;border-radius:12px;background:var(--input);border:1px solid var(--line)}
.seg label{position:relative;cursor:pointer}
.seg input{position:absolute;opacity:0;pointer-events:none}
.seg span{display:block;padding:7px 8px;border-radius:9px;text-align:center;font-size:13.5px;font-weight:500;color:var(--muted);transition:background .15s,color .15s}
.seg input:checked+span{background:var(--card);color:var(--brand);font-weight:600;box-shadow:0 1px 3px rgba(16,40,34,.12),inset 0 0 0 1px var(--brand-soft)}
.seg input:focus-visible+span{box-shadow:0 0 0 3px var(--brand-soft)}
.back{display:inline-flex;align-items:center;gap:6px;margin-top:16px;padding:4px 8px;border-radius:8px;color:var(--muted);text-decoration:none;font-size:13px}
.back:hover{color:var(--text);background:var(--brand-soft)}
.back svg{width:15px;height:15px}
[hidden]{display:none!important}
</style>
<script>
(function(){
  document.querySelectorAll('form[data-once]').forEach(function(f){
    f.addEventListener('submit',function(){var b=f.querySelector('button[type=submit]');if(b){setTimeout(function(){b.disabled=true;},0);}});
  });
  var code=document.getElementById('code-field');
  document.querySelectorAll('input[name=kind][type=radio]').forEach(function(r){
    r.addEventListener('change',function(){if(code){code.hidden=r.value!=='auth';}});
  });
})();
</script>{{end}}`

// passwordTexts 为自助修改密码页面的文案，中文为翻译键。
var passwordTexts = map[string]string{
	"pageTitle":  "修改密码",
	"title":      "修改密码",
	"subtitle":   "输入当前密码并设置新密码",
	"kindAuth":   "网页登录账号",
	"kindFile":   "文件服务账号",
	"username":   "用户名",
	"usernamePh": "请输入用户名",
	"current":    "当前密码",
	"currentPh":  "请输入当前密码",
	"code":       "动态验证码",
	"codePh":     "已开启两步验证时填写",
	"next":       "新密码",
	"nextPh":     "至少 8 个字符",
	"confirm":    "确认新密码",
	"confirmPh":  "再次输入新密码",
	"submit":     "修改密码",
	"backLogin":  "返回登录",
	"doneTitle":  "密码已修改",
	"doneAuth":   "所有设备上的登录已失效，请使用新密码重新登录",
	"doneFile":   "WebDAV、FTP、SMB 请使用新密码重新连接",
	"toLogin":    "前往登录",
}

func init() {
	webpage.MustRegister("ws-password", passwordSrc)
}

// handlePassword 处理自助修改密码页面。
func (a *authenticator) handlePassword(w http.ResponseWriter, r *http.Request, s Site, ip string) {
	authKind, fileKind := a.selfServiceKinds(s)
	if !authKind && !fileKind {
		http.NotFound(w, r)
		return
	}
	host := hostOnly(r.Host)
	base := s.authPrefix()
	kind := r.URL.Query().Get("kind")
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		kind = r.PostForm.Get("kind")
	}
	if (kind != kindAuth || !authKind) && (kind != kindFile || !fileKind) {
		kind = kindFile
		if authKind {
			kind = kindAuth
		}
	}
	data := map[string]any{"Host": host, "Action": base + passwordSuffix, "AuthKind": authKind, "FileKind": fileKind, "Kind": kind}
	if authKind {
		data["LoginURL"] = base + "/login"
	}
	status := http.StatusOK
	if r.Method == http.MethodPost {
		name := strings.TrimSpace(r.PostForm.Get("username"))
		data["Username"] = name
		msg, code := a.changePassword(w, r, s, ip, kind, name)
		if msg == "" {
			data["Done"] = true
			if kind != kindAuth {
				delete(data, "LoginURL")
			}
		} else {
			data["Error"], status = msg, code
		}
	} else if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "405 Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	lang := i18n.RequestLang(r)
	if msg, ok := data["Error"].(string); ok {
		data["Error"] = i18n.Tr(lang, msg)
	}
	data["PageTitle"] = i18n.Tr(lang, passwordTexts["pageTitle"]) + " · " + host
	webpage.Render(w, status, "ws-password", lang, passwordTexts, data)
}

// changePassword 校验表单并修改密码，返回给访客的错误提示（空表示成功）与状态码。
// 失败次数与登录页共用限流与自动封禁：当前密码或动态验证码错误计为一次失败。
func (a *authenticator) changePassword(w http.ResponseWriter, r *http.Request, s Site, ip, kind, name string) (string, int) {
	if !sameOriginPost(r) {
		return "请求来源无效，请刷新页面后重试", http.StatusForbidden
	}
	current, next := r.PostForm.Get("current"), r.PostForm.Get("next")
	if name == "" || current == "" {
		return "请输入用户名和当前密码", http.StatusBadRequest
	}
	if next != r.PostForm.Get("confirm") {
		return "两次输入的新密码不一致", http.StatusBadRequest
	}
	if err := fileusers.ValidateNewPassword(current, next); err != nil {
		return err.Error(), http.StatusBadRequest
	}
	if a.inCooldown(kind, name) {
		return "密码刚修改过，请 1 分钟后再试", http.StatusTooManyRequests
	}
	key := limitKey(ip)
	if res := a.reserve(r.Context(), key); res != reserveOK {
		return reserveDenyText(res), http.StatusTooManyRequests
	}
	if kind == kindFile {
		err := a.files.ChangePassword(name, current, next)
		a.release(key, !errors.Is(err, fileusers.ErrBadCredentials))
		if err != nil {
			switch {
			case errors.Is(err, fileusers.ErrBadCredentials):
				return err.Error(), http.StatusUnauthorized
			case errors.Is(err, fileusers.ErrBadNewPassword), errors.Is(err, fileusers.ErrSelfServiceOff):
				return err.Error(), http.StatusBadRequest
			}
			return a.saveFailed(kindFile, name, ip, err)
		}
		a.passwordChanged(kindFile, name, ip)
		return "", http.StatusOK
	}
	u, ok := a.shared.get().checkPassword(name, current)
	if !ok {
		a.release(key, false)
		return "用户名或当前密码错误", http.StatusUnauthorized
	}
	if u.TOTPSecret != "" {
		if msg := a.checkTOTP(u, r.PostForm.Get("code")); msg != "" {
			a.release(key, false)
			return msg, http.StatusUnauthorized
		}
	}
	a.release(key, true)
	h, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		return a.saveFailed(kindAuth, name, ip, err)
	}
	if a.changeShared == nil {
		return a.saveFailed(kindAuth, name, ip, errors.New("未设置保存方法"))
	}
	if err := a.changeShared(u, string(h)); err != nil {
		if errors.Is(err, errPasswordChanged) {
			return err.Error(), http.StatusUnauthorized
		}
		return a.saveFailed(kindAuth, name, ip, err)
	}
	// 旧会话已随凭据指纹失效，顺手清掉本站与单点登录的 Cookie
	http.SetCookie(w, &http.Cookie{Name: cookieName(s), Value: "", Path: "/", MaxAge: -1})
	if domain := a.shared.get().ssoFor(hostOnly(r.Host)); domain != "" {
		http.SetCookie(w, &http.Cookie{Name: ssoCookie, Value: "", Path: "/", Domain: domain, MaxAge: -1})
	}
	a.passwordChanged(kindAuth, name, ip)
	return "", http.StatusOK
}

// passwordCooldown 是同一账号两次自助修改密码的最短间隔。文件服务账号每次修改都会让 FTP 重启、SMB 重新载入，
// 不加限制时知道自己密码的用户可以来回修改，反复断开其他人的连接。
const passwordCooldown = time.Minute

// cooldownKey 返回自助改密冷却记录的键。
func cooldownKey(kind, name string) string {
	return kind + "\x00" + strings.ToLower(name)
}

// inCooldown 判断账号是否刚自助修改过密码。
func (a *authenticator) inCooldown(kind, name string) bool {
	v, ok := a.pwChangedAt.Load(cooldownKey(kind, name))
	return ok && time.Since(v.(time.Time)) < passwordCooldown
}

// saveFailed 记录保存失败的详细原因，访客只看到统一提示，避免泄露其他用户或服务的配置。
func (a *authenticator) saveFailed(kind, name, ip string, err error) (string, int) {
	if a.onPasswordFailed != nil {
		a.onPasswordFailed(kind, name, ip, err)
	}
	return "保存失败，请联系管理员", http.StatusInternalServerError
}

// passwordChanged 记录自助修改密码的日志，并开始该账号的冷却时间。
func (a *authenticator) passwordChanged(kind, name, ip string) {
	a.pwChangedAt.Store(cooldownKey(kind, name), time.Now())
	if a.onPasswordChanged != nil {
		a.onPasswordChanged(kind, name, ip)
	}
}

// errPasswordChanged 表示校验之后、保存之前共享用户的密码已被修改或用户已被删除。
var errPasswordChanged = errors.New("用户名或当前密码错误")
