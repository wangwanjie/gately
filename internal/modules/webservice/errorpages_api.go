package webservice

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/i18n"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/webpage"
)

// ErrorPageView 是页面库列表中的条目：不含页面内容，附带引用方与文件状态。
type ErrorPageView struct {
	Key       string    `json:"key"`
	Name      string    `json:"name"`
	Source    string    `json:"source"`
	File      string    `json:"file,omitempty"`
	Remark    string    `json:"remark"`
	Size      int       `json:"size"`
	UpdatedAt time.Time `json:"updatedAt,omitzero"`
	// UsedBy 为引用该页面的站点（监听名 / 站点名）
	UsedBy []string `json:"usedBy,omitempty"`
	// Error 为 file 页面当前不可用的原因（文件被删除、内容无效等），此时引用它的站点改用默认样式
	Error string `json:"error,omitempty"`
}

// ErrorPageSaved 是新增或修改页面后的结果：保存后的条目与检查报告。
type ErrorPageSaved struct {
	Page   ErrorPageView       `json:"page"`
	Report webpage.CheckReport `json:"report"`
}

// ErrorPageCheckRequest 是检查页面的请求：与页面条目相同的来源与内容，不保存。
type ErrorPageCheckRequest struct {
	Source string `json:"source"`
	HTML   string `json:"html,omitempty"`
	File   string `json:"file,omitempty"`
	// Code 为预览使用的状态码，默认 502
	Code int `json:"code,omitempty"`
}

// ErrorPageCheckResult 是检查结果与按示例错误生成的预览页面。
type ErrorPageCheckResult struct {
	Report webpage.CheckReport `json:"report"`
	// Preview 为替换占位符后的完整页面，管理界面放在 sandbox iframe 中显示
	Preview string `json:"preview"`
}

// ErrorStylePreview 是一种样式的预览。ID 为空表示默认样式。
type ErrorStylePreview struct {
	ID   string `json:"id"`
	HTML string `json:"html"`
}

// errorPageSampleHost 为预览中显示的示例域名。
const errorPageSampleHost = "www.example.com"

// sampleProblem 返回预览用的示例错误：404 为页面不存在，其余为连不上后端的 5xx 或对应状态码的说明。
func sampleProblem(code int) webpage.Problem {
	if code == http.StatusBadGateway {
		return webpage.Problem{Code: code, Title: "无法连接到后端服务", Detail: "站点的后端服务没有响应，可能未启动或网络不可达。请稍后重试。", Home: true}
	}
	return statusProblem(code, nil)
}

// previewRequest 构造生成预览所用的请求：沿用管理员的语言，域名与路径为示例值。
func previewRequest(r *http.Request) *http.Request {
	pr := httptest.NewRequest(http.MethodGet, "http://"+errorPageSampleHost+"/missing/page", nil)
	pr.Header.Set("Accept-Language", r.Header.Get("Accept-Language"))
	pr.Header.Set("Accept", "text/html")
	return pr
}

// previewCode 读取预览的状态码，只允许 400~599，默认 502。
func previewCode(s string) int {
	if c, err := strconv.Atoi(s); err == nil && c >= 400 && c <= 599 {
		return c
	}
	return http.StatusBadGateway
}

func (m *Module) findErrorPageLocked(key string) *ErrorPage {
	for i := range m.cfg.ErrorPages {
		if m.cfg.ErrorPages[i].Key == key {
			return &m.cfg.ErrorPages[i]
		}
	}
	return nil
}

// errorPageUsersLocked 返回引用页面的站点（含默认站点）。
func (m *Module) errorPageUsersLocked(key string) []string {
	var out []string
	for i := range m.cfg.Listeners {
		l := &m.cfg.Listeners[i]
		for j := range l.Sites {
			if s := &l.Sites[j]; s.ErrorPage.Style == ErrorStyleCustom && s.ErrorPage.Page == key {
				out = append(out, listenerSiteLabel(l, s))
			}
		}
		if d := l.Default.ErrorPage; d.Style == ErrorStyleCustom && d.Page == key {
			out = append(out, defaultSiteLabel(l, defaultSiteMarker))
		}
	}
	return out
}

// defaultSiteMarker 在页面库列表的 usedBy 中代表默认站点，由管理界面换成界面语言的名称
// （usedBy 中的监听名、站点名是用户内容，接口不做翻译）。
const defaultSiteMarker = "@default"

// defaultSiteLabel 返回“监听名 / name”形式的默认站点描述。
func defaultSiteLabel(l *Listener, name string) string {
	label := l.Name
	if label == "" {
		label = ":" + strconv.Itoa(l.Port)
	}
	return label + " / " + name
}

// checkErrorPageRefsLocked 检查监听器中引用的自定义错误页面都存在。
func (m *Module) checkErrorPageRefsLocked(l *Listener) error {
	check := func(o ErrorPageOptions, label string) error {
		if o.Style == ErrorStyleCustom && m.findErrorPageLocked(o.Page) == nil {
			return fmt.Errorf("站点 %s 引用的自定义错误页面不存在", label)
		}
		return nil
	}
	for i := range l.Sites {
		if err := check(l.Sites[i].ErrorPage, listenerSiteLabel(l, &l.Sites[i])); err != nil {
			return err
		}
	}
	return check(l.Default.ErrorPage, defaultSiteLabel(l, "默认站点"))
}

// checkRefsLocked 检查监听器引用的 IP 列表与自定义错误页面都存在。
func (m *Module) checkRefsLocked(l *Listener) error {
	if err := m.checkIPSetsLocked(l); err != nil {
		return err
	}
	return m.checkErrorPageRefsLocked(l)
}

func (m *Module) viewErrorPageLocked(p *ErrorPage) ErrorPageView {
	v := ErrorPageView{Key: p.Key, Name: p.Name, Source: p.Source, File: p.File, Remark: p.Remark, Size: len(p.HTML), UpdatedAt: p.UpdatedAt, UsedBy: m.errorPageUsersLocked(p.Key)}
	if p.Source == ErrorPageFile {
		v.Error = m.auth.errPages.fileError(p.Key)
		if page, ok := m.auth.errPages.page(p.Key); ok {
			v.Size = len(page)
		}
	}
	return v
}

// registerErrorPages 注册页面库与样式预览接口。
func (m *Module) registerErrorPages(r module.Router) {
	r.HandleFunc("GET /errorpages", m.handleListErrorPages)
	r.HandleFunc("POST /errorpages", m.handleSaveErrorPage)
	r.HandleFunc("GET /errorpages/{key}", m.handleGetErrorPage)
	r.HandleFunc("PUT /errorpages/{key}", m.handleSaveErrorPage)
	r.HandleFunc("DELETE /errorpages/{key}", m.handleDeleteErrorPage)
	r.HandleFunc("POST /errorpages/check", m.handleCheckErrorPage)
	r.HandleFunc("GET /errorpages/styles", m.handleErrorStyles)
	r.HandleFunc("GET /errorpages/{key}/preview", m.handleErrorPagePreview)
	module.DocOf(r, "GET /errorpages", module.Doc{Summary: "自定义错误页面列表（不含页面内容）", Resp: []ErrorPageView{}})
	module.DocOf(r, "POST /errorpages", module.Doc{Summary: "新增自定义错误页面：上传的页面清理后保存，文件页面检查文件是否可用", Req: &ErrorPage{}, Resp: &ErrorPageSaved{}})
	module.DocOf(r, "GET /errorpages/{key}", module.Doc{Summary: "读取自定义错误页面（含页面内容）", Resp: &ErrorPage{}})
	module.DocOf(r, "PUT /errorpages/{key}", module.Doc{Summary: "修改自定义错误页面", Req: &ErrorPage{}, Resp: &ErrorPageSaved{}})
	module.DocOf(r, "DELETE /errorpages/{key}", module.Doc{Summary: "删除自定义错误页面（仍被站点引用时拒绝）"})
	module.DocOf(r, "POST /errorpages/check", module.Doc{Summary: "检查页面的安全性与可用性并生成预览，不保存", Req: &ErrorPageCheckRequest{}, Resp: &ErrorPageCheckResult{}})
	module.DocOf(r, "GET /errorpages/styles", module.Doc{Summary: "默认样式与全部内置风格的预览，?code= 指定状态码（默认 502）", Resp: []ErrorStylePreview{}})
	module.DocOf(r, "GET /errorpages/{key}/preview", module.Doc{Summary: "自定义错误页面的预览，?code= 指定状态码（默认 502）", Resp: &ErrorStylePreview{}})
}

func (m *Module) handleListErrorPages(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ErrorPageView, 0, len(m.cfg.ErrorPages))
	for i := range m.cfg.ErrorPages {
		out = append(out, m.viewErrorPageLocked(&m.cfg.ErrorPages[i]))
	}
	httpx.OK(w, out)
}

func (m *Module) handleGetErrorPage(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.findErrorPageLocked(r.PathValue("key"))
	if p == nil {
		httpx.Error(w, http.StatusNotFound, "自定义错误页面不存在")
		return
	}
	httpx.OK(w, p)
}

// handleSaveErrorPage 新增（POST）或修改（PUT /{key}）页面。页面的检查（清理、读取文件）在持锁前完成，
// 读取较慢的文件不会阻塞其他配置操作。
func (m *Module) handleSaveErrorPage(w http.ResponseWriter, r *http.Request) {
	var p ErrorPage
	if err := httpx.ReadJSON(r, &p); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	rep, err := p.validate()
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	key := r.PathValue("key")
	m.mu.Lock()
	defer m.mu.Unlock()
	if key == "" {
		if len(m.cfg.ErrorPages) >= maxErrorPages {
			httpx.Error(w, http.StatusBadRequest, "自定义错误页面最多 %d 个", maxErrorPages)
			return
		}
		p.Key = crud.NewKey()
	} else if m.findErrorPageLocked(key) == nil {
		httpx.Error(w, http.StatusNotFound, "自定义错误页面不存在")
		return
	} else {
		p.Key = key
	}
	if slices.ContainsFunc(m.cfg.ErrorPages, func(x ErrorPage) bool { return x.Key != p.Key && x.Name == p.Name }) {
		httpx.Error(w, http.StatusBadRequest, "已有同名的自定义错误页面")
		return
	}
	p.UpdatedAt = time.Now().UTC()
	old := slices.Clone(m.cfg.ErrorPages)
	if key == "" {
		m.cfg.ErrorPages = append(m.cfg.ErrorPages, p)
	} else {
		*m.findErrorPageLocked(key) = p
	}
	if err := m.saveLocked(); err != nil {
		m.cfg.ErrorPages = old
		httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
		return
	}
	m.auth.errPages.set(m.cfg.ErrorPages)
	httpx.OK(w, i18n.Localize(r, ErrorPageSaved{Page: m.viewErrorPageLocked(m.findErrorPageLocked(p.Key)), Report: rep}, "removed"))
}

func (m *Module) handleDeleteErrorPage(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := r.PathValue("key")
	if m.findErrorPageLocked(key) == nil {
		httpx.Error(w, http.StatusNotFound, "自定义错误页面不存在")
		return
	}
	if users := m.errorPageUsersLocked(key); len(users) > 0 {
		httpx.Error(w, http.StatusBadRequest, "自定义错误页面仍被 %s 引用，请先修改这些站点的错误页设置", strings.ReplaceAll(strings.Join(users, "、"), defaultSiteMarker, "默认站点"))
		return
	}
	old := slices.Clone(m.cfg.ErrorPages)
	m.cfg.ErrorPages = slices.DeleteFunc(m.cfg.ErrorPages, func(p ErrorPage) bool { return p.Key == key })
	if err := m.saveLocked(); err != nil {
		m.cfg.ErrorPages = old
		httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
		return
	}
	m.auth.errPages.set(m.cfg.ErrorPages)
	httpx.OK(w, nil)
}

// handleCheckErrorPage 检查上传内容或文件路径，返回检查报告与预览，不修改配置。
func (m *Module) handleCheckErrorPage(w http.ResponseWriter, r *http.Request) {
	var req ErrorPageCheckRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	// 名称在检查时无关紧要，用占位名称通过校验
	p := ErrorPage{Name: "-", Source: req.Source, HTML: req.HTML, File: req.File}
	rep, err := p.validate()
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	page := p.HTML
	if p.Source == ErrorPageFile {
		if page, _, _, err = readErrorPageFile(p.File); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
	}
	code := req.Code
	if code < 400 || code > 599 {
		code = http.StatusBadGateway
	}
	preview := webpage.ForPreview(webpage.RenderCustom(page, previewRequest(r), sampleProblem(code)))
	httpx.OK(w, i18n.Localize(r, ErrorPageCheckResult{Report: rep, Preview: string(preview)}, "removed"))
}

// handleErrorStyles 返回默认样式与全部内置风格按示例错误生成的页面，供管理界面展示缩略图与预览。
func (m *Module) handleErrorStyles(w http.ResponseWriter, r *http.Request) {
	pr := previewRequest(r)
	p := sampleProblem(previewCode(r.URL.Query().Get("code")))
	ids := append([]string{ErrorStyleDefault}, webpage.StyleIDs()...)
	out := make([]ErrorStylePreview, 0, len(ids))
	for _, id := range ids {
		out = append(out, ErrorStylePreview{ID: id, HTML: string(webpage.ForPreview(webpage.RenderError(pr, p, id)))})
	}
	httpx.OK(w, out)
}

// handleErrorPagePreview 返回自定义页面按示例错误生成的页面；文件页面不可用时返回原因。
func (m *Module) handleErrorPagePreview(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	m.mu.Lock()
	exists := m.findErrorPageLocked(key) != nil
	m.mu.Unlock()
	if !exists {
		httpx.Error(w, http.StatusNotFound, "自定义错误页面不存在")
		return
	}
	page, ok := m.auth.errPages.page(key)
	if !ok {
		reason := m.auth.errPages.fileError(key)
		if reason == "" {
			reason = "页面暂不可用"
		}
		httpx.Error(w, http.StatusBadRequest, "%s", reason)
		return
	}
	html := webpage.ForPreview(webpage.RenderCustom(page, previewRequest(r), sampleProblem(previewCode(r.URL.Query().Get("code")))))
	httpx.OK(w, ErrorStylePreview{ID: key, HTML: string(html)})
}
