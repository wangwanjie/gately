package webpage

import (
	"bytes"
	"html/template"
	"net/http"
	"strconv"
	"unicode/utf8"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// 站点错误页的内置风格。默认风格（ID 为空）就是 errorSrc，其余风格各自是完整的页面模板：
// 只用内联样式与纯 CSS 动画，不含脚本与外部资源，在 ErrorPageCSP 下可以完整显示。
// 文案与默认风格一样以中文字面量为键，渲染时按访客语言翻译。

// ErrorPageCSP 是站点错误页（内置风格、自定义页面与拦截后端错误后的页面）的内容安全策略：
// sandbox 让页面运行在独立的不透明源中，即使页面里混进了脚本也不能执行，也读不到站点的 Cookie 与存储；
// 除内联样式与 data: 内嵌的图片、字体、音视频外不加载任何资源，访客的地址不会泄露给第三方。
// 不限制 frame-ancestors：站点本身可能被嵌入在 iframe 中，后端出错时嵌入处也应显示错误页。
const ErrorPageCSP = "sandbox; default-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src data:; media-src data:; base-uri 'none'; form-action 'none'"

// styleTexts 为内置风格共用的文案（中文原文）。
var styleTexts = map[string]string{
	"retry":     "重新加载",
	"home":      "返回首页",
	"status":    "状态码",
	"host":      "主机",
	"time":      "时间",
	"browser":   "浏览器",
	"you":       "你",
	"gateway":   "网关",
	"site":      "站点",
	"ok":        "正常",
	"failed":    "出错",
	"missing":   "未找到",
	"what":      "发生了什么？",
	"todo":      "我该怎么办？",
	"todo5xx":   "请稍后重试；如果问题持续出现，请联系网站管理员。",
	"todo404":   "请检查地址是否正确，或返回首页。",
	"todoOther": "请检查访问地址，或返回首页。",
}

// style 是一个已编译的内置风格。
type style struct {
	id  string
	tpl *template.Template
}

var (
	styleList []*style
	styleByID = map[string]*style{}
)

// sharedPartials 为各风格共用的页面头部与操作按钮。按钮的样式由各风格通过 .acts、.b、.pri、.sec 定义；
// “重新加载”用空 href 重新打开当前地址，不依赖脚本（错误页在 sandbox 下运行，脚本不会执行）。
const sharedPartials = `{{define "head"}}<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="robots" content="noindex,nofollow"><title>{{.PageTitle}}</title><link rel="icon" href="data:,">{{end}}
{{define "actions"}}{{if or .Action .Retry .Home}}<nav class="acts">
{{if .Action}}<a class="b pri" href="{{.Action}}">{{.ActionText}}</a>{{end}}
{{if .Retry}}<a class="b {{if .Action}}sec{{else}}pri{{end}}" href="">{{.T.retry}}</a>{{end}}
{{if .Home}}<a class="b {{if or .Action .Retry}}sec{{else}}pri{{end}}" href="/">{{.T.home}}</a>{{end}}
</nav>{{end}}{{end}}`

// registerStyle 注册一个内置风格，src 需以 {{define "page"}} 定义完整页面；在包初始化时调用，模板错误直接 panic。
func registerStyle(id, src string) {
	t := template.Must(template.New(id).Parse(sharedPartials))
	template.Must(t.Parse(src))
	s := &style{id: id, tpl: t}
	styleList = append(styleList, s)
	styleByID[id] = s
}

// StyleIDs 返回全部内置风格的 ID（不含默认风格），顺序即管理界面中的展示顺序。
func StyleIDs() []string {
	out := make([]string, 0, len(styleList))
	for _, s := range styleList {
		out = append(out, s.id)
	}
	return out
}

// HasStyle 判断 id 是否为内置风格（不含默认风格的空 ID）。
func HasStyle(id string) bool {
	return styleByID[id] != nil
}

// maxShownHost 与 maxShownPath 为错误页中显示的域名与路径的最大字节数：两者都由访客控制，过长时截断。
const (
	maxShownHost = 200
	maxShownPath = 160
)

// clipShown 把访客提供的内容截断到最多 n 字节（不截断多字节字符），并把无效的 UTF-8 替换掉。
func clipShown(s string, n int) string {
	s = toValidUTF8(s)
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

// toValidUTF8 把无效的 UTF-8 字节替换为 U+FFFD。
func toValidUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return string(bytes.ToValidUTF8([]byte(s), []byte("�")))
}

// RenderError 生成错误页的 HTML。styleID 为空或不是内置风格时使用默认风格。
func RenderError(r *http.Request, p Problem, styleID string) []byte {
	lang, data := problemData(r, p)
	data["Host"] = clipShown(r.Host, maxShownHost)
	var buf bytes.Buffer
	s := styleByID[styleID]
	if s == nil {
		mu.RLock()
		t := pages["error"]
		mu.RUnlock()
		data["Lang"] = lang
		data["T"] = Localize(lang, errorTexts)
		_ = t.ExecuteTemplate(&buf, "layout", data)
		return buf.Bytes()
	}
	data["Lang"] = lang
	data["T"] = Localize(lang, styleTexts)
	data["Path"] = clipShown(r.URL.Path, maxShownPath)
	data["Server"] = p.Code >= 500
	data["NotFound"] = p.Code == http.StatusNotFound
	_ = s.tpl.ExecuteTemplate(&buf, "page", data)
	return buf.Bytes()
}

// previewCSP 为管理界面预览时插入页面的内容安全策略：预览放在 srcdoc iframe 中，没有响应头可用，
// 用 meta 标签同样禁止加载任何外部资源，管理员打开预览时浏览器不会访问页面里引用的第三方地址；
// previewReferrer 让预览中点开的链接不带管理后台的地址。
const (
	previewCSP      = "default-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src data:; media-src data:; base-uri 'none'; form-action 'none'"
	previewReferrer = "no-referrer"
)

// ForPreview 把 previewCSP 与 previewReferrer 以 meta 标签插到页面 <head> 的最前面，供管理界面预览使用。
// 在解析树上插入：<head> 带属性或文本中出现 "<head>" 字样时也能放到正确位置。
func ForPreview(page []byte) []byte {
	doc, err := xhtml.Parse(bytes.NewReader(page))
	if err != nil {
		return nil
	}
	var head *xhtml.Node
	var find func(n *xhtml.Node)
	find = func(n *xhtml.Node) {
		for c := n.FirstChild; c != nil && head == nil; c = c.NextSibling {
			if c.Type == xhtml.ElementNode && c.DataAtom == atom.Head {
				head = c
				return
			}
			find(c)
		}
	}
	find(doc)
	if head == nil {
		return nil
	}
	meta := func(attrs ...xhtml.Attribute) *xhtml.Node {
		return &xhtml.Node{Type: xhtml.ElementNode, Data: "meta", DataAtom: atom.Meta, Attr: attrs}
	}
	first := head.FirstChild
	head.InsertBefore(meta(xhtml.Attribute{Key: "http-equiv", Val: "Content-Security-Policy"}, xhtml.Attribute{Key: "content", Val: previewCSP}), first)
	head.InsertBefore(meta(xhtml.Attribute{Key: "name", Val: "referrer"}, xhtml.Attribute{Key: "content", Val: previewReferrer}), first)
	var out bytes.Buffer
	if err := xhtml.Render(&out, doc); err != nil {
		return nil
	}
	return out.Bytes()
}

// WriteErrorHTML 以 status 输出已生成的错误页，附加 ErrorPageCSP；HEAD 请求只写响应头。
func WriteErrorHTML(w http.ResponseWriter, r *http.Request, status int, body []byte) {
	h := w.Header()
	SetErrorHeaders(h, len(body))
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

// SetErrorHeaders 设置错误页的响应头：内容类型、禁止缓存、禁止类型嗅探与 ErrorPageCSP。
// 替换后端的错误响应时同样使用，后端原有的 CSP 会被覆盖，否则可能挡住错误页的内联样式。
func SetErrorHeaders(h http.Header, size int) {
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", ErrorPageCSP)
	h.Del("Content-Security-Policy-Report-Only")
	h.Set("Content-Length", strconv.Itoa(size))
}
