package webpage

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/wangwanjie/gately/internal/i18n"
)

// 自定义错误页：管理员上传、粘贴或指定服务器上的 HTML 文件，按访客请求替换其中的占位符后输出。
//
// 安全上分两层：保存（或文件变化后首次使用）时先解析并清理页面，去掉脚本、事件属性、javascript: 链接、
// 内嵌框架、表单等可执行或会把访客带走的内容；输出时再附加 ErrorPageCSP（sandbox 且不加载外部资源），
// 即使清理有遗漏，页面中的脚本也无法执行。占位符的值一律经 HTML 转义后再替换，不能借此注入标签。

// MaxCustomPage 为自定义错误页的大小上限（清理前的原始内容）。
const MaxCustomPage = 256 << 10

// Placeholders 为自定义页面中可用的占位符，输出时替换为本次错误的信息（已转义）。
var Placeholders = []string{"{code}", "{status}", "{title}", "{detail}", "{host}", "{path}", "{time}"}

// CheckReport 是自定义页面的检查结果，提示文字均为中文原文，由接口按访客语言翻译。
type CheckReport struct {
	// Size 为清理后的页面字节数
	Size int `json:"size"`
	// PageTitle 为页面 <title> 的内容（来自用户文件，不翻译）
	PageTitle string `json:"pageTitle"`
	// Removed 为清理时去掉的内容，Warnings 为不影响保存、但可能导致显示效果与预期不同的问题
	Removed  []string `json:"removed"`
	Warnings []string `json:"warnings"`
	// Placeholders 为页面中用到的占位符
	Placeholders []string `json:"placeholders"`
}

// 清理时整个删除的元素：脚本与可嵌入其他文档、插件或改变链接基准的元素。
// noscript 也删除：页面在 sandbox 中脚本被禁用，浏览器会把其中的内容当作普通标记解析，与清理时的解析结果不一致。
var dropElements = map[string]string{
	"script":   "脚本",
	"noscript": "noscript 内容",
	"iframe":   "内嵌框架",
	"frame":    "内嵌框架",
	"frameset": "内嵌框架",
	"object":   "插件对象",
	"embed":    "插件对象",
	"applet":   "插件对象",
	"portal":   "内嵌框架",
	"base":     "base 标签",
	"math":     "MathML 公式",
	"template": "template 模板",
}

// dangerousSchemes 为链接中不允许的协议（比较前已去掉空白与控制字符并转为小写）。
var dangerousSchemes = []string{"javascript:", "vbscript:", "livescript:", "data:text/html", "data:application/xhtml", "data:text/xml", "data:application/xml"}

// cssDanger 为样式中不允许出现的写法：旧版 IE 的 expression() 与 behavior、旧版 Firefox 的 -moz-binding，
// 以及在样式中引用的脚本协议。
var cssDanger = []string{"expression(", "behavior:", "-moz-binding", "javascript:", "vbscript:"}

// urlAttrs 为会让浏览器加载资源的属性，其中非 data: 的地址在 ErrorPageCSP 下不会加载，检查时提示。
var urlAttrs = map[string]bool{"src": true, "srcset": true, "poster": true, "background": true, "data": true, "xlink:href": true}

// sanitizer 记录一次清理的结果。
type sanitizer struct {
	removed   []string
	warnings  []string
	external  []string
	title     string
	viewport  bool
	elements  int
	textBytes int
}

func (s *sanitizer) remove(what string) {
	if !slices.Contains(s.removed, what) {
		s.removed = append(s.removed, what)
	}
}

func (s *sanitizer) warn(what string) {
	if !slices.Contains(s.warnings, what) {
		s.warnings = append(s.warnings, what)
	}
}

// addExternal 记录页面引用的、不会加载的资源（最多列出 5 个）。
func (s *sanitizer) addExternal(u string) {
	if len(s.external) < 5 && !slices.Contains(s.external, u) {
		s.external = append(s.external, u)
	}
}

// compact 去掉空白与控制字符并转为小写，用于识别 "java\tscript:"、"JaVaScRiPt:" 一类的绕过写法。
func compact(v string) string {
	return strings.Map(func(r rune) rune {
		if r <= ' ' || r == 0x7f {
			return -1
		}
		return r
	}, strings.ToLower(v))
}

// loadsResource 判断资源地址在 ErrorPageCSP 下是否会被拦下：除 data: 内嵌与页内锚点（#id）外，
// 外部地址与站内的相对地址都不会加载（错误页本身就意味着站点后端可能不可用）。
func loadsResource(v string) bool {
	c := compact(v)
	return c != "" && !strings.HasPrefix(c, "data:") && !strings.HasPrefix(c, "#")
}

// cssEscape 匹配 CSS 的反斜杠转义：1~6 位十六进制（可带一个空白结尾）或单个其他字符。
var cssEscape = regexp.MustCompile(`\\([0-9A-Fa-f]{1,6}[ \t\r\n\f]?|[^0-9A-Fa-f\r\n\f])`)

// cssComment 匹配 CSS 注释。
var cssComment = regexp.MustCompile(`(?s)/\*.*?(\*/|$)`)

// normalizeCSS 去掉注释并解码反斜杠转义，用于识别 "\65xpression("、"expr/**/ession(" 一类的绕过写法。
func normalizeCSS(css string) string {
	css = cssComment.ReplaceAllString(css, "")
	return cssEscape.ReplaceAllStringFunc(css, func(m string) string {
		v := strings.TrimRight(m[1:], " \t\r\n\f")
		if n, err := strconv.ParseUint(v, 16, 32); err == nil {
			if n == 0 || n > 0x10ffff {
				return "\uFFFD"
			}
			return string(rune(n))
		}
		return v
	})
}

// visitorPlaceholders 为内容由访客请求决定的占位符。它们只做了 HTML 转义，放进样式里时访客可以借 ; { } 等写入任意样式，
// 所以样式（<style> 与 style 属性）中的这类占位符会被去掉。
var visitorPlaceholders = []string{"{host}", "{path}"}

// stripVisitorPlaceholders 去掉样式文本中由访客决定的占位符，返回去掉后的文本与是否有改动。
func stripVisitorPlaceholders(css string) (string, bool) {
	out := css
	for _, p := range visitorPlaceholders {
		out = strings.ReplaceAll(out, p, "")
	}
	return out, out != css
}

// scanCSS 检查样式文本：含危险写法时返回 false；不会加载的资源引用（url(…) 与 @import）记为提示。
func (s *sanitizer) scanCSS(css string) bool {
	c := compact(css)
	nc := compact(normalizeCSS(css))
	for _, d := range cssDanger {
		if strings.Contains(c, d) || strings.Contains(nc, d) {
			return false
		}
	}
	for _, marker := range []string{"url(", "@import"} {
		for i := 0; ; {
			j := strings.Index(c[i:], marker)
			if j < 0 {
				break
			}
			rest := strings.TrimLeft(c[i+j+len(marker):], `"'`)
			// @import 后可以直接跟引号中的地址，也可以跟 url(…)，后者由 url( 分支处理
			if marker == "@import" && strings.HasPrefix(rest, "url(") {
				i += j + len(marker)
				continue
			}
			if loadsResource(rest) {
				end := strings.IndexAny(rest, `"')`)
				if end < 0 || end > 120 {
					end = min(len(rest), 120)
				}
				s.addExternal(rest[:end])
			}
			i += j + len(marker)
		}
	}
	return true
}

// cleanAttrs 清理元素的属性：删除事件属性、危险协议的链接与危险样式，记录外部资源。
func (s *sanitizer) cleanAttrs(n *xhtml.Node) {
	kept := n.Attr[:0]
	for _, a := range n.Attr {
		key := strings.ToLower(a.Key)
		if a.Namespace != "" {
			key = strings.ToLower(a.Namespace) + ":" + key
		}
		switch {
		case strings.ContainsAny(a.Key, "{}"):
			// 占位符只能放在文本与属性值中：写在属性名的位置（如 <div {path}>）时，替换后的内容会变成新的属性
			s.remove("写在标签或属性名位置的占位符")
			continue
		case strings.HasPrefix(strings.ToLower(a.Key), "on"):
			s.remove("事件处理属性（onclick 等）")
			continue
		case key == "srcdoc" || key == "formaction" || key == "action":
			s.remove("可执行或提交数据的属性")
			continue
		case key == "style" && !s.scanCSS(a.Val):
			s.remove("含有危险写法的样式")
			continue
		case key == "style":
			if v, changed := stripVisitorPlaceholders(a.Val); changed {
				a.Val = v
				s.remove("样式中的 {host}、{path} 占位符")
			}
		}
		c := compact(a.Val)
		bad := false
		for _, scheme := range dangerousSchemes {
			if strings.Contains(c, scheme) {
				bad = true
				break
			}
		}
		if bad {
			s.remove("javascript: 等脚本链接")
			continue
		}
		switch {
		case key == "srcset":
			// srcset 为“地址 描述符”以逗号分隔的列表；data: 地址本身含逗号，按空白切分后跳过描述符（2x、480w）
			for _, f := range strings.Fields(a.Val) {
				f = strings.TrimSuffix(f, ",")
				if !isSrcsetDescriptor(f) && loadsResource(f) {
					s.addExternal(f)
				}
			}
		case urlAttrs[key] || (key == "href" && n.Data != "a" && n.Data != "area"):
			if loadsResource(a.Val) {
				s.addExternal(strings.TrimSpace(a.Val))
			}
		}
		kept = append(kept, a)
	}
	n.Attr = kept
}

// isSrcsetDescriptor 判断 srcset 中的一项是否为宽度或像素密度描述符（如 480w、1.5x）。
func isSrcsetDescriptor(f string) bool {
	if len(f) < 2 || (f[len(f)-1] != 'w' && f[len(f)-1] != 'x') {
		return false
	}
	_, err := strconv.ParseFloat(f[:len(f)-1], 64)
	return err == nil
}

// attr 返回元素的属性值（不区分大小写）。
func attr(n *xhtml.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

// clean 递归清理节点 n 的子节点。
func (s *sanitizer) clean(n *xhtml.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		switch c.Type {
		case xhtml.CommentNode:
			// 注释不显示，也可能藏有条件注释（<!--[if IE]>）一类的旧版浏览器指令，一并去掉
			n.RemoveChild(c)
		case xhtml.TextNode:
			s.textBytes += len(strings.TrimSpace(c.Data))
		case xhtml.ElementNode:
			s.cleanElement(n, c)
		}
		c = next
	}
}

// cleanElement 清理单个元素：按需整个删除、去掉标签保留内容，或清理属性后继续处理子节点。
func (s *sanitizer) cleanElement(parent, c *xhtml.Node) {
	name := strings.ToLower(c.Data)
	if strings.ContainsAny(name, "{}") {
		// 如 <x{path}>：替换后标签名后面会多出访客控制的属性
		s.remove("写在标签或属性名位置的占位符")
		parent.RemoveChild(c)
		return
	}
	if what, drop := dropElements[name]; drop {
		s.remove(what)
		parent.RemoveChild(c)
		return
	}
	switch name {
	case "meta":
		equiv := strings.ToLower(strings.TrimSpace(attr(c, "http-equiv")))
		if equiv != "" {
			// refresh 会把访客跳转走，set-cookie 会写 Cookie，其余 http-equiv 也不应由错误页设置
			s.remove("meta http-equiv（自动跳转等）")
			parent.RemoveChild(c)
			return
		}
		if cs := strings.ToLower(strings.TrimSpace(attr(c, "charset"))); cs != "" && cs != "utf-8" && cs != "utf8" {
			s.warn("页面声明的编码不是 UTF-8，已按 UTF-8 输出")
			parent.RemoveChild(c)
			return
		}
		if strings.EqualFold(attr(c, "name"), "viewport") {
			s.viewport = true
		}
	case "link":
		rel := strings.ToLower(attr(c, "rel"))
		if strings.Contains(rel, "icon") && strings.HasPrefix(compact(attr(c, "href")), "data:image/") {
			break
		}
		if strings.Contains(rel, "stylesheet") {
			if href := attr(c, "href"); href != "" {
				s.addExternal(href)
			}
			s.warn("外部样式表不会加载，请把样式写进 <style> 标签")
		} else {
			s.remove("link 标签（预加载、外部资源引用等）")
		}
		parent.RemoveChild(c)
		return
	case "form":
		// 去掉表单本身、保留其中的内容：ErrorPageCSP 也禁止提交，这里让页面不再出现可提交的表单
		s.remove("表单")
		s.clean(c)
		for gc := c.FirstChild; gc != nil; {
			next := gc.NextSibling
			c.RemoveChild(gc)
			parent.InsertBefore(gc, c)
			gc = next
		}
		parent.RemoveChild(c)
		return
	case "style":
		css := ""
		for t := c.FirstChild; t != nil; t = t.NextSibling {
			if t.Type == xhtml.TextNode {
				css += t.Data
			}
		}
		if !s.scanCSS(css) {
			s.remove("含有危险写法的样式")
			parent.RemoveChild(c)
			return
		}
		for t := c.FirstChild; t != nil; t = t.NextSibling {
			if t.Type != xhtml.TextNode {
				continue
			}
			if v, changed := stripVisitorPlaceholders(t.Data); changed {
				t.Data = v
				s.remove("样式中的 {host}、{path} 占位符")
			}
		}
	case "title":
		if s.title == "" && c.FirstChild != nil {
			s.title = strings.TrimSpace(c.FirstChild.Data)
		}
	}
	if c.DataAtom != atom.Html && c.DataAtom != atom.Head && c.DataAtom != atom.Body {
		s.elements++
	}
	s.cleanAttrs(c)
	s.clean(c)
}

// SanitizeCustom 校验并清理自定义错误页，返回清理后的 HTML 与检查报告；内容无法作为页面使用时返回错误（中文原文）。
func SanitizeCustom(src []byte) (string, CheckReport, error) {
	var rep CheckReport
	src = bytes.TrimPrefix(src, []byte("\xef\xbb\xbf"))
	if len(bytes.TrimSpace(src)) == 0 {
		return "", rep, errors.New("页面内容为空")
	}
	if len(src) > MaxCustomPage {
		return "", rep, fmt.Errorf("页面不能超过 %d KB", MaxCustomPage>>10)
	}
	if bytes.IndexByte(src, 0) >= 0 {
		return "", rep, errors.New("页面含有空字符，可能不是文本文件")
	}
	if !utf8.Valid(src) {
		return "", rep, errors.New("页面不是 UTF-8 编码，请转换为 UTF-8 后重试")
	}
	doc, err := xhtml.Parse(bytes.NewReader(src))
	if err != nil {
		return "", rep, fmt.Errorf("无法解析页面: %v", err)
	}
	s := &sanitizer{}
	hasDoctype := false
	for c := doc.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == xhtml.DoctypeNode {
			hasDoctype = true
		}
	}
	s.clean(doc)
	if s.elements == 0 {
		return "", rep, errors.New("内容不是 HTML 页面")
	}
	if s.elements+s.textBytes == 0 {
		return "", rep, errors.New("页面没有可显示的内容")
	}
	// 没有文档类型声明时浏览器按怪异模式排版，补上标准声明
	if !hasDoctype {
		doc.InsertBefore(&xhtml.Node{Type: xhtml.DoctypeNode, Data: "html"}, doc.FirstChild)
	}
	var out bytes.Buffer
	if err := xhtml.Render(&out, doc); err != nil {
		return "", rep, fmt.Errorf("无法生成页面: %v", err)
	}
	page := out.String()
	if !s.viewport {
		s.warn("页面没有设置 viewport，在手机上可能显示得很小")
	}
	if len(s.external) > 0 {
		s.warn("页面引用的图片、字体、样式表等资源不会加载，请改为 data: 内嵌: " + strings.Join(s.external, ", "))
	}
	for _, p := range Placeholders {
		if strings.Contains(page, p) {
			rep.Placeholders = append(rep.Placeholders, p)
		}
	}
	if len(rep.Placeholders) == 0 {
		s.warn("页面没有使用占位符，所有状态码都会显示相同的内容")
	}
	rep.Size, rep.PageTitle, rep.Removed, rep.Warnings = len(page), clipShown(s.title, 200), s.removed, s.warnings
	return page, rep, nil
}

// hostPattern 为 {host} 允许的写法：域名或 IPv4（字母、数字、点、连字符、下划线）或方括号内的 IPv6，可带端口。
var hostPattern = regexp.MustCompile(`^(\[[0-9A-Fa-f:.]+\]|[A-Za-z0-9._-]+)(:[0-9]{1,5})?$`)

// shownHost 返回可以放进页面的访客域名：Host 请求头由访客填写，页面把 {host} 写进链接时
// （如 href="//{host}/"）不能借此拼出 javascript: 等其他地址，不符合主机名写法时返回空。
func shownHost(h string) string {
	if !hostPattern.MatchString(h) {
		return ""
	}
	return h
}

// shownPath 让路径以单个 / 开头：请求行写成 //evil.example/x 时路径也以 // 开头，
// 页面把 {path} 写进链接（如 href="{path}"）会变成指向其他网站的协议相对地址。
// 浏览器解析地址时把 \ 当作 /、并忽略 TAB 与换行，所以先去掉控制字符，开头的 \ 也按斜杠去掉。
func shownPath(p string) string {
	p = strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f {
			return -1
		}
		return r
	}, p)
	return "/" + strings.TrimLeft(p, "/\\")
}

// RenderCustom 用本次错误的信息替换自定义页面中的占位符，返回要输出的 HTML。page 须为 SanitizeCustom 的结果。
func RenderCustom(page string, r *http.Request, p Problem) []byte {
	lang := i18n.RequestLang(r)
	esc := html.EscapeString
	rep := strings.NewReplacer(
		"{code}", strconv.Itoa(p.Code),
		"{status}", esc(http.StatusText(p.Code)),
		"{title}", esc(i18n.Tr(lang, p.Title)),
		"{detail}", esc(i18n.Tr(lang, p.Detail)),
		"{host}", esc(clipShown(shownHost(r.Host), maxShownHost)),
		"{path}", esc(clipShown(shownPath(r.URL.Path), maxShownPath)),
		"{time}", time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
	)
	return []byte(rep.Replace(page))
}
