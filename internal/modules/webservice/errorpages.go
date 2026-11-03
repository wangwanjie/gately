package webservice

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/wangwanjie/gately/internal/webpage"
)

// 站点错误页：站点可以把 Gately 自己生成的 404 与 5xx 页面（找不到文件、连不上后端等）换成内置风格或自定义页面，
// 反向代理站点还可以把后端返回的指定状态码拦截下来，换成同样的页面。自定义页面集中保存在模块的页面库中，
// 多个站点可以引用同一个页面；页面内容可以上传（保存在配置中），也可以指向服务器上的 HTML 文件（每次使用时检查是否变化）。

// 错误页样式：空为默认样式，custom 为页面库中的自定义页面，其余为 webpage 包中的内置风格 ID。
const (
	ErrorStyleDefault = ""
	ErrorStyleCustom  = "custom"
)

// 自定义页面的来源。
const (
	// ErrorPageInline 为上传或粘贴的页面，清理后的内容保存在配置中
	ErrorPageInline = "inline"
	// ErrorPageFile 为服务器上的 HTML 文件，每次使用时检查是否变化
	ErrorPageFile = "file"
)

// maxErrorPages 为页面库中自定义页面的数量上限，maxInterceptTokens 为拦截状态码最多的条目数。
const (
	maxErrorPages      = 32
	maxInterceptTokens = 50
)

// ErrorPageOptions 是站点的错误页设置。
type ErrorPageOptions struct {
	// Style 为页面样式：空为默认样式，custom 为页面库中的自定义页面，其余为内置风格
	Style string `json:"style"`
	// Page 为 Style 为 custom 时引用的自定义页面 key
	Page string `json:"page"`
	// Intercept 为 true 时（仅反向代理站点）把后端返回的 Codes 中的状态码换成错误页，
	// 只替换浏览器打开页面的请求（Accept 含 text/html），接口与资源请求保持后端原样
	Intercept bool `json:"intercept"`
	// Codes 为要拦截的状态码，逗号分隔，支持范围与 4xx/5xx 简写，例如 "404, 500-599"
	Codes string `json:"codes"`
}

// ErrorPageStyle 为批量设置站点错误页样式时提交的内容，含义同 ErrorPageOptions 的同名字段。
type ErrorPageStyle struct {
	Style string `json:"style"`
	Page  string `json:"page"`
}

// ErrorPage 是页面库中的一个自定义错误页。
type ErrorPage struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Source 为 inline（上传或粘贴）或 file（服务器上的文件）
	Source string `json:"source"`
	// HTML 为 inline 页面清理后的内容；File 为 file 页面的绝对路径（扩展名须为 .html 或 .htm）
	HTML      string    `json:"html,omitempty"`
	File      string    `json:"file,omitempty"`
	Remark    string    `json:"remark"`
	UpdatedAt time.Time `json:"updatedAt,omitzero"`
}

// parseStatusCodes 解析拦截的状态码列表，返回状态码集合与规范化后的写法。
// 支持单个状态码（404）、范围（500-599）与简写（4xx、5xx），只允许 400~599。
func parseStatusCodes(s string) (map[int]bool, string, error) {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '；' || r == '、' || r == ' ' || r == '\t' || r == '\n'
	})
	if len(fields) > maxInterceptTokens {
		return nil, "", fmt.Errorf("拦截的状态码最多填写 %d 项", maxInterceptTokens)
	}
	set := map[int]bool{}
	var norm []string
	for _, f := range fields {
		lo, hi, err := parseCodeToken(f)
		if err != nil {
			return nil, "", err
		}
		for c := lo; c <= hi; c++ {
			set[c] = true
		}
		tok := strconv.Itoa(lo)
		if hi != lo {
			tok += "-" + strconv.Itoa(hi)
		}
		if !slices.Contains(norm, tok) {
			norm = append(norm, tok)
		}
	}
	return set, strings.Join(norm, ", "), nil
}

// parseCodeToken 解析一项状态码写法，返回闭区间。
func parseCodeToken(f string) (int, int, error) {
	bad := fmt.Errorf("状态码 %q 无效，只能填写 400~599 之间的状态码或范围，例如 404, 500-599", f)
	low := strings.ToLower(f)
	if len(low) == 3 && strings.HasSuffix(low, "xx") && (low[0] == '4' || low[0] == '5') {
		base := int(low[0]-'0') * 100
		return base, base + 99, nil
	}
	a, b, isRange := strings.Cut(f, "-")
	lo, err := strconv.Atoi(strings.TrimSpace(a))
	if err != nil {
		return 0, 0, bad
	}
	hi := lo
	if isRange {
		if hi, err = strconv.Atoi(strings.TrimSpace(b)); err != nil {
			return 0, 0, bad
		}
	}
	if lo < 400 || hi > 599 || lo > hi {
		return 0, 0, bad
	}
	return lo, hi, nil
}

// validate 校验并规范化站点的错误页设置；引用的自定义页面是否存在由 checkErrorPageRefsLocked 检查。
func (o *ErrorPageOptions) validate() error {
	o.Style = strings.TrimSpace(o.Style)
	o.Page = strings.TrimSpace(o.Page)
	switch {
	case o.Style == ErrorStyleDefault:
	case o.Style == ErrorStyleCustom:
		if o.Page == "" {
			return errors.New("请选择自定义错误页面")
		}
	case !webpage.HasStyle(o.Style):
		return fmt.Errorf("未知的错误页样式 %q", o.Style)
	}
	if o.Style != ErrorStyleCustom {
		o.Page = ""
	}
	o.Codes = strings.TrimSpace(o.Codes)
	if o.Codes != "" {
		_, norm, err := parseStatusCodes(o.Codes)
		switch {
		case err == nil:
			o.Codes = norm
		case o.Intercept:
			return err
		default:
			// 未开启拦截时界面不显示状态码输入框，写错的内容无处可改，直接丢弃
			o.Codes = ""
		}
	}
	if o.Intercept && o.Codes == "" {
		return errors.New("请填写要拦截的状态码")
	}
	return nil
}

// checkErrorPagePath 校验自定义页面的文件路径，返回解析符号链接后的实际路径。
// 只接受扩展名为 .html / .htm 的绝对路径（符号链接指向的文件同样要求），避免把配置文件、密钥等误当作页面发给访客。
func checkErrorPagePath(p string) (string, error) {
	if p == "" {
		return "", errors.New("请填写页面文件路径")
	}
	if !filepath.IsAbs(p) {
		return "", errors.New("页面文件路径需为绝对路径")
	}
	htmlExt := func(name string) bool {
		ext := strings.ToLower(filepath.Ext(name))
		return ext == ".html" || ext == ".htm"
	}
	if !htmlExt(p) {
		return "", errors.New("页面文件的扩展名需为 .html 或 .htm")
	}
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("页面文件 %s 不存在", p)
		}
		return "", fmt.Errorf("无法访问页面文件: %v", err)
	}
	if !htmlExt(real) {
		return "", errors.New("页面文件是指向非 HTML 文件的符号链接")
	}
	return real, nil
}

// readErrorPageFile 读取并清理服务器上的页面文件，返回清理后的页面、检查报告与文件状态（用于判断之后是否变化）。
func readErrorPageFile(p string) (string, webpage.CheckReport, os.FileInfo, error) {
	var rep webpage.CheckReport
	real, err := checkErrorPagePath(p)
	if err != nil {
		return "", rep, nil, err
	}
	// 打开前先确认是普通文件：命名管道等特殊文件在打开时就会阻塞，读取它会让输出错误页的请求一直挂起。
	// 用 Lstat：real 已解析过符号链接，此时若又变成符号链接，说明文件在校验后被替换
	if pre, err := os.Lstat(real); err != nil {
		return "", rep, nil, fmt.Errorf("无法读取页面文件: %v", err)
	} else if !pre.Mode().IsRegular() {
		return "", rep, nil, errors.New("页面文件不是普通文件")
	}
	f, err := os.Open(real)
	if err != nil {
		return "", rep, nil, fmt.Errorf("无法打开页面文件: %v", err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", rep, nil, fmt.Errorf("无法读取页面文件: %v", err)
	}
	if !st.Mode().IsRegular() {
		return "", rep, nil, errors.New("页面文件不是普通文件")
	}
	// 打开的必须是校验过的那个文件：校验与打开之间路径被换成指向别处的符号链接时，
	// Lstat 得到的是链接本身，与打开的文件不同，拒绝
	if cur, err := os.Lstat(real); err != nil || !os.SameFile(st, cur) {
		return "", rep, nil, errors.New("页面文件在读取时发生了变化，请重试")
	}
	if st.Size() > webpage.MaxCustomPage {
		return "", rep, nil, fmt.Errorf("页面不能超过 %d KB", webpage.MaxCustomPage>>10)
	}
	data, err := io.ReadAll(io.LimitReader(f, webpage.MaxCustomPage+1))
	if err != nil {
		return "", rep, nil, fmt.Errorf("无法读取页面文件: %v", err)
	}
	page, rep, err := webpage.SanitizeCustom(data)
	if err != nil {
		return "", rep, nil, err
	}
	return page, rep, st, nil
}

// validate 校验页面库条目：inline 页面清理后保存，file 页面检查文件可用；返回检查报告。
func (p *ErrorPage) validate() (webpage.CheckReport, error) {
	var rep webpage.CheckReport
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return rep, errors.New("名称不能为空")
	}
	if utf8.RuneCountInString(p.Name) > 64 {
		return rep, errors.New("名称不能超过 64 个字符")
	}
	p.Remark = strings.TrimSpace(p.Remark)
	if utf8.RuneCountInString(p.Remark) > 200 {
		return rep, errors.New("备注不能超过 200 个字符")
	}
	switch p.Source {
	case ErrorPageInline:
		p.File = ""
		page, r, err := webpage.SanitizeCustom([]byte(p.HTML))
		if err != nil {
			return r, err
		}
		p.HTML = page
		return r, nil
	case ErrorPageFile:
		p.HTML = ""
		if p.File = strings.TrimSpace(p.File); p.File != "" {
			p.File = filepath.Clean(p.File)
		}
		_, r, _, err := readErrorPageFile(p.File)
		return r, err
	}
	return rep, fmt.Errorf("未知的页面来源 %q", p.Source)
}

// storedPage 是页面库条目的运行时状态。
type storedPage struct {
	name string
	// html 为 inline 页面清理后的内容；file 为 file 页面的路径
	html string
	file string

	// 以下为 file 页面的缓存：info 为最近一次读取时的文件状态，文件（同一个 inode、修改时间与大小）不变时
	// 直接使用上次的结果，读取失败的结果同样缓存；failed 为失败原因，failedAt 为文件无法访问时的失败时间
	mu       sync.Mutex
	info     os.FileInfo
	cached   string
	failed   string
	failedAt time.Time
}

// fileRecheck 为页面文件无法访问（如被删除）后再次尝试的最短间隔：访客反复触发错误时不必每次都访问文件系统
var fileRecheck = 5 * time.Second

// errorPageStore 保存页面库，供全部监听的站点在输出错误页时按 key 查找。页面库修改后整体替换，站点无需重建。
type errorPageStore struct {
	mu    sync.RWMutex
	pages map[string]*storedPage
	// warn 记录页面不可用的原因（每种失败只记录一次）
	warn func(format string, args ...any)
}

func newErrorPageStore() *errorPageStore {
	return &errorPageStore{pages: map[string]*storedPage{}, warn: func(string, ...any) {}}
}

// set 替换页面库。inline 页面再清理一遍：配置文件可能被手工编辑，不能直接信任其中的内容。
func (s *errorPageStore) set(list []ErrorPage) {
	pages := make(map[string]*storedPage, len(list))
	for _, p := range list {
		sp := &storedPage{name: p.Name, file: p.File}
		if p.Source == ErrorPageInline {
			page, _, err := webpage.SanitizeCustom([]byte(p.HTML))
			if err != nil {
				s.warn("自定义错误页面 %s 不可用，已改用默认样式: %v", p.Name, err)
				sp.failed = err.Error()
			}
			sp.html = page
			sp.file = ""
		}
		pages[p.Key] = sp
	}
	s.mu.Lock()
	s.pages = pages
	s.mu.Unlock()
}

// page 返回可直接输出的页面内容，页面不存在或暂不可用时返回 false（调用方改用默认样式）。
func (s *errorPageStore) page(key string) (string, bool) {
	s.mu.RLock()
	sp := s.pages[key]
	s.mu.RUnlock()
	if sp == nil {
		return "", false
	}
	if sp.file == "" {
		return sp.html, sp.html != ""
	}
	return sp.load(s.warn)
}

// fileError 返回 file 页面最近一次读取失败的原因，供管理界面展示。
func (s *errorPageStore) fileError(key string) string {
	s.mu.RLock()
	sp := s.pages[key]
	s.mu.RUnlock()
	if sp == nil {
		return ""
	}
	if sp.file != "" {
		_, _ = sp.load(s.warn)
	}
	sp.mu.Lock()
	defer sp.mu.Unlock()
	return sp.failed
}

// load 读取 file 页面：文件未变时直接返回上次的结果（成功或失败），否则重新读取并清理。
// 读取与清理一个 256 KB 的页面约需毫秒级，不缓存失败结果时，访客反复请求不存在的页面就能让错误页串行排队。
func (sp *storedPage) load(warn func(string, ...any)) (string, bool) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	st, statErr := os.Stat(sp.file)
	if statErr == nil && sp.info != nil && os.SameFile(st, sp.info) && st.ModTime().Equal(sp.info.ModTime()) && st.Size() == sp.info.Size() {
		return sp.cached, sp.cached != ""
	}
	if statErr != nil && sp.failed != "" && time.Since(sp.failedAt) < fileRecheck {
		return "", false
	}
	page, _, info, err := readErrorPageFile(sp.file)
	if err != nil {
		if sp.failed != err.Error() {
			warn("自定义错误页面 %s 不可用，已改用默认样式: %v", sp.name, err)
		}
		sp.failed, sp.cached, sp.failedAt = err.Error(), "", time.Now()
		// 文件存在但内容无效时按文件状态缓存失败结果，文件变化后再重新读取
		sp.info = nil
		if statErr == nil {
			sp.info = st
		}
		return "", false
	}
	sp.failed, sp.cached, sp.info = "", page, info
	return page, true
}

// errorPager 按站点的错误页设置输出错误页。nil 表示使用默认样式且不拦截。
type errorPager struct {
	style string
	page  string
	store *errorPageStore
	// codes 为要拦截的后端状态码，nil 表示不拦截
	codes map[int]bool
}

// newErrorPager 编译站点的错误页设置；拦截只对反向代理站点生效。设置为默认样式且不拦截时返回 nil。
func newErrorPager(s Site, store *errorPageStore) *errorPager {
	o := s.ErrorPage
	e := &errorPager{style: o.Style, page: o.Page, store: store}
	if o.Intercept && s.Type == TypeProxy {
		e.codes, _, _ = parseStatusCodes(o.Codes)
		if len(e.codes) == 0 {
			e.codes = nil
		}
	}
	if e.style == ErrorStyleDefault && e.codes == nil {
		return nil
	}
	return e
}

// render 生成错误页：自定义页面不可用时退回默认样式。
func (e *errorPager) render(r *http.Request, p webpage.Problem) []byte {
	if e.style == ErrorStyleCustom {
		if page, ok := e.store.page(e.page); ok {
			return webpage.RenderCustom(page, r, p)
		}
		return webpage.RenderError(r, p, ErrorStyleDefault)
	}
	return webpage.RenderError(r, p, e.style)
}

// write 输出 Gately 生成的错误页：404 与 5xx 按站点设置的样式，其余状态码与未设置样式时使用默认错误页。
func (e *errorPager) write(w http.ResponseWriter, r *http.Request, p webpage.Problem) {
	if e == nil || e.style == ErrorStyleDefault || (p.Code != http.StatusNotFound && p.Code < 500) || !webpage.WantsHTML(r) {
		webpage.Error(w, r, p)
		return
	}
	webpage.WriteErrorHTML(w, r, p.Code, e.render(r, p))
}

// drainLimit 与 drainTimeout 为替换后端错误响应时读掉原响应体的上限与时限（时限为变量，便于测试缩短）。
const drainLimit = 64 << 10

var drainTimeout = 2 * time.Second

// ctxKeyVisitorReq 在请求上下文中保存访客的原始请求。拦截后端错误时 resp.Request 是发往后端的请求：
// Host、路径与请求头都可能已按后端地址与站点设置改写，判断是否拦截与生成页面都要用访客的原始请求。
type ctxKeyVisitorReq struct{}

// visitorRequest 返回后端响应对应的访客请求，取不到时退回发往后端的请求。
func visitorRequest(resp *http.Response) *http.Request {
	if resp.Request == nil {
		return nil
	}
	if r, ok := resp.Request.Context().Value(ctxKeyVisitorReq{}).(*http.Request); ok {
		return r
	}
	return resp.Request
}

// intercepts 判断是否要替换后端的这个响应：状态码在拦截列表中，且是浏览器打开页面的请求。
func (e *errorPager) intercepts(resp *http.Response) bool {
	if e == nil || e.codes == nil || !e.codes[resp.StatusCode] {
		return false
	}
	r := visitorRequest(resp)
	return r != nil && webpage.WantsHTML(r)
}

// replace 把后端的错误响应换成错误页，保留状态码与其余响应头（Set-Cookie、Retry-After 等），
// 去掉描述原响应体的头。声明了长度且不超过 64KB 的原响应体读完后关闭，连接还能复用；
// 读取限时 2 秒（后端声明了长度却不发或慢发正文时不让访客一直等），其余情况直接关闭。
func (e *errorPager) replace(resp *http.Response) {
	if resp.Body != nil {
		if resp.ContentLength >= 0 && resp.ContentLength <= drainLimit {
			t := time.AfterFunc(drainTimeout, func() { _ = resp.Body.Close() })
			_, _ = io.CopyN(io.Discard, resp.Body, drainLimit)
			t.Stop()
		}
		_ = resp.Body.Close()
	}
	r := visitorRequest(resp)
	body := e.render(r, statusProblem(resp.StatusCode, r))
	h := resp.Header
	for _, k := range []string{"Content-Encoding", "Content-Range", "Content-Disposition", "Content-Language", "Content-Location", "Etag", "Last-Modified", "Accept-Ranges", "Expires", "Trailer"} {
		h.Del(k)
	}
	resp.Trailer = nil
	resp.TransferEncoding = nil
	webpage.SetErrorHeaders(h, len(body))
	resp.ContentLength = int64(len(body))
	resp.Body = io.NopCloser(strings.NewReader(string(body)))
}

// statusProblem 返回拦截后端状态码时显示的标题与说明（中文原文）。
func statusProblem(code int, r *http.Request) webpage.Problem {
	p := webpage.Problem{Code: code, Home: r == nil || r.URL.Path != "/"}
	switch code {
	case http.StatusBadRequest:
		p.Title, p.Detail = "请求无效", "服务器无法处理这个请求，请检查地址是否正确。"
	case http.StatusUnauthorized:
		p.Title, p.Detail = "需要登录", "访问这个页面需要先登录。"
	case http.StatusForbidden:
		p.Title, p.Detail, p.Lock = "禁止访问", "你没有访问这个页面的权限。", true
	case http.StatusNotFound:
		p.Title, p.Detail = "页面不存在", "你访问的页面不存在，可能已被移动或删除，请检查地址是否正确。"
	case http.StatusMethodNotAllowed:
		p.Title, p.Detail = "不支持的请求方式", "这个页面不支持当前的请求方式。"
	case http.StatusRequestTimeout:
		p.Title, p.Detail = "请求超时", "请求没有在规定时间内完成，请重试。"
	case http.StatusTooManyRequests:
		p.Title, p.Detail = "请求过于频繁", "请求次数太多，请稍后再试。"
	case http.StatusInternalServerError:
		p.Title, p.Detail = "服务器内部错误", "站点的后端服务出现了错误，请稍后重试。"
	case http.StatusNotImplemented:
		p.Title, p.Detail = "功能未实现", "站点暂不支持这个请求。"
	case http.StatusBadGateway:
		p.Title, p.Detail = "网关错误", "站点的后端服务返回了无效的响应，请稍后重试。"
	case http.StatusServiceUnavailable:
		p.Title, p.Detail = "服务暂时不可用", "站点正在维护或负载过高，请稍后重试。"
	case http.StatusGatewayTimeout:
		p.Title, p.Detail = "网关超时", "站点的后端服务响应超时，请稍后重试。"
	default:
		if code >= 500 {
			p.Title, p.Detail = "服务器错误", "站点的后端服务出现了错误，请稍后重试。"
		} else {
			p.Title, p.Detail = "请求未能完成", "请求没有成功，请检查访问地址后重试。"
		}
	}
	return p
}
