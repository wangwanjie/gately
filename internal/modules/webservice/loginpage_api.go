package webservice

import (
	"bytes"
	"net/http"

	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/i18n"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/webpage"
)

// 网页认证页面外观的管理接口：读取与保存全局外观设置，按尚未保存的设置生成各样式的预览。

// LoginPageView 为 GET/PUT /loginpage 的响应。
type LoginPageView struct {
	LoginPage LoginPage `json:"loginPage"`
	// Styles 为全部内置样式 ID（不含默认样式），顺序即展示顺序
	Styles []string `json:"styles"`
	// SiteStyles 为单独设置了样式的站点数，按样式 ID 统计（default 为默认样式）
	SiteStyles map[string]int `json:"siteStyles"`
}

// LoginPreviewRequest 为 POST /loginpage/preview 的请求体。
type LoginPreviewRequest struct {
	// LoginPage 为要预览的外观设置（可以尚未保存）
	LoginPage LoginPage `json:"loginPage"`
	// Styles 为要生成预览的样式，default 为默认样式；为空时生成默认样式与全部内置样式
	Styles []string `json:"styles"`
	// State 为页面状态：login（登录表单，默认）、totp（动态验证码对话框）或 error（登录失败提示）
	State string `json:"state"`
}

// LoginStylePreview 是一种样式的预览页面。
type LoginStylePreview struct {
	ID   string `json:"id"`
	HTML string `json:"html"`
}

func (m *Module) registerLoginPage(r module.Router) {
	r.HandleFunc("GET /loginpage", func(w http.ResponseWriter, req *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		httpx.OK(w, m.loginPageViewLocked())
	})
	r.HandleFunc("PUT /loginpage", m.handlePutLoginPage)
	r.HandleFunc("POST /loginpage/preview", m.handleLoginPreview)
	module.DocOf(r, "GET /loginpage", module.Doc{Summary: "读取网页认证页面（登录、修改密码、Passkey 管理）的外观设置与内置样式列表", Resp: &LoginPageView{}})
	module.DocOf(r, "PUT /loginpage", module.Doc{Summary: "保存网页认证页面的外观：样式、主色、背景图、Logo、标题、说明、页脚标识与自定义 CSS", Req: &LoginPage{}, Resp: &LoginPageView{}})
	module.DocOf(r, "POST /loginpage/preview", module.Doc{Summary: "按（可以尚未保存的）外观设置生成登录页预览，styles 为空时生成默认样式与全部内置样式", Req: &LoginPreviewRequest{}, Resp: []LoginStylePreview{}})
}

func (m *Module) loginPageViewLocked() LoginPageView {
	v := LoginPageView{LoginPage: m.cfg.LoginPage, Styles: webpage.LoginSkinIDs(), SiteStyles: map[string]int{}}
	for _, l := range m.cfg.Listeners {
		for _, s := range l.Sites {
			if s.LoginStyle != "" {
				v.SiteStyles[s.LoginStyle]++
			}
		}
		if l.Default.LoginStyle != "" {
			v.SiteStyles[l.Default.LoginStyle]++
		}
	}
	return v
}

func (m *Module) handlePutLoginPage(w http.ResponseWriter, r *http.Request) {
	var p LoginPage
	if err := httpx.ReadJSON(r, &p); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if err := p.validate(); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	old := m.cfg.LoginPage
	m.cfg.LoginPage = p
	if err := m.saveLocked(); err != nil {
		m.cfg.LoginPage = old
		httpx.Error(w, http.StatusInternalServerError, "%v", err)
		return
	}
	_ = m.auth.look.set(p)
	httpx.OK(w, m.loginPageViewLocked())
}

// previewHost 为预览页面中显示的示例域名。
const previewHost = "app.example.com"

func (m *Module) handleLoginPreview(w http.ResponseWriter, r *http.Request) {
	var req LoginPreviewRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if err := req.LoginPage.validate(); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	ids := req.Styles
	if len(ids) == 0 {
		ids = append([]string{LoginStyleDefault}, webpage.LoginSkinIDs()...)
	}
	if len(ids) > 32 {
		httpx.Error(w, http.StatusBadRequest, "参数无效")
		return
	}
	for _, id := range ids {
		if id != LoginStyleDefault && !webpage.HasLoginSkin(id) {
			httpx.Error(w, http.StatusBadRequest, "未知的登录页样式 %q", id)
			return
		}
	}
	var c lookHolder
	_ = c.set(req.LoginPage)
	lang := i18n.RequestLang(r)
	out := make([]LoginStylePreview, 0, len(ids))
	for _, id := range ids {
		o := lookOptions(c.get(), id, authPath, previewHost, lang, true)
		out = append(out, LoginStylePreview{ID: id, HTML: string(webpage.ForLoginPreview(renderLoginSample(o, req.State, lang)))})
	}
	httpx.OK(w, out)
}

// renderLoginSample 按外观生成示例登录页：用户名密码登录、修改密码入口，state 为 totp 时打开动态验证码对话框，
// 为 error 时显示登录失败提示。
func renderLoginSample(o webpage.LookOptions, state, lang string) []byte {
	data := map[string]any{
		"Host": previewHost, "Action": authPath + "/login", "Next": "/", "HasPassword": true,
		"PasswordURL": authPath + passwordSuffix, "StartURL": oauthStart,
		"PageTitle": i18n.Tr(lang, loginTexts["pageTitle"]) + " · " + previewHost,
		"Look":      webpage.BuildLook(o),
	}
	switch state {
	case "totp":
		data["Pending"], data["Username"] = "preview", "alice"
	case "error":
		data["Error"], data["Username"] = i18n.Tr(lang, "用户名或密码错误"), "alice"
	}
	var buf bytes.Buffer
	webpage.RenderTo(&buf, "ws-login", lang, loginTexts, data)
	return buf.Bytes()
}
