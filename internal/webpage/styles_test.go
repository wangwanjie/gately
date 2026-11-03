package webpage

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	xhtml "golang.org/x/net/html"
)

// 全部内置风格（含默认样式）在各类状态码下都能生成完整页面：含状态码、标题与说明，没有脚本与外部资源，
// 访客控制的域名与路径经过转义。
func TestRenderErrorAllStyles(t *testing.T) {
	ids := append([]string{""}, StyleIDs()...)
	if len(ids) != 11 {
		t.Fatalf("应有默认样式与 10 种内置风格，实际 %v", ids)
	}
	for _, id := range ids {
		for _, code := range []int{400, 403, 404, 429, 500, 502, 503, 504, 599} {
			r := httptest.NewRequest("GET", "http://x/", nil)
			r.Header.Set("Accept-Language", "zh-CN")
			r.Host = `evil.example"><script>alert(1)</script>`
			r.URL.Path = `/a"><img src=x onerror=alert(1)>`
			p := Problem{Code: code, Title: "标题<u>", Detail: "说明文字", Hint: "提示", Home: true, Action: "/next", ActionText: "继续"}
			out := string(RenderError(r, p, id))
			name := id + "/" + strconv.Itoa(code)
			for _, bad := range []string{"<script", "<img", "<u>", "onclick", `src="http`, `href="http`, "url(http", "@import"} {
				if strings.Contains(out, bad) {
					t.Fatalf("%s: 页面不应含 %q", name, bad)
				}
			}
			for _, want := range []string{strconv.Itoa(code), "标题&lt;u&gt;", "说明文字", "提示", `href="/next"`, `href="/"`, "<!doctype html>", `name="viewport"`} {
				if !strings.Contains(out, want) {
					t.Fatalf("%s: 页面应含 %q", name, want)
				}
			}
			// 5xx 默认显示重新加载
			if code >= 500 && !strings.Contains(out, `href=""`) {
				t.Fatalf("%s: 5xx 应有重新加载按钮", name)
			}
		}
	}
	// 未知风格回落到默认样式
	r := httptest.NewRequest("GET", "http://x/", nil)
	if a, b := RenderError(r, Problem{Code: 404, Title: "t"}, "nope"), RenderError(r, Problem{Code: 404, Title: "t"}, ""); string(a[:200]) != string(b[:200]) {
		t.Fatal("未知风格应使用默认样式")
	}
	if HasStyle("") || HasStyle("custom") || !HasStyle("terminal") {
		t.Fatal("HasStyle 判断不对")
	}
}

// 内置风格的文案按访客语言翻译。
func TestRenderErrorTranslated(t *testing.T) {
	r := httptest.NewRequest("GET", "http://x/", nil)
	r.Header.Set("Accept-Language", "en-US,en;q=0.9")
	for _, id := range StyleIDs() {
		out := string(RenderError(r, Problem{Code: 502, Title: "无法连接到后端服务", Home: true}, id))
		if !strings.Contains(out, `lang="en"`) || !strings.Contains(out, "Reload") || strings.Contains(out, "重新加载") {
			t.Fatalf("%s: 应输出英文页面", id)
		}
	}
}

// WriteErrorHTML 附加 CSP 等响应头，HEAD 请求不写响应体。
func TestWriteErrorHTML(t *testing.T) {
	for _, method := range []string{"GET", "HEAD"} {
		w := httptest.NewRecorder()
		WriteErrorHTML(w, httptest.NewRequest(method, "/", nil), 502, []byte("<h1>x</h1>"))
		h := w.Result().Header
		if w.Code != 502 || h.Get("Content-Security-Policy") != ErrorPageCSP || h.Get("Content-Type") != "text/html; charset=utf-8" ||
			h.Get("Cache-Control") != "no-store" || h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Content-Length") != "10" {
			t.Fatalf("%s 响应头不对: %d %v", method, w.Code, h)
		}
		if (method == "HEAD") != (w.Body.Len() == 0) {
			t.Fatalf("%s 响应体 %q", method, w.Body.String())
		}
	}
	if !strings.HasPrefix(ErrorPageCSP, "sandbox;") || !strings.Contains(ErrorPageCSP, "default-src 'none'") {
		t.Fatal("CSP 必须含 sandbox 与 default-src 'none'")
	}
}

// 默认错误页的“重新加载”不再依赖内联脚本，在 CSP 下同样可用。
func TestDefaultErrorPageNoInlineScript(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()
	Error(w, r, Problem{Code: http.StatusBadGateway, Title: "t"})
	if strings.Contains(w.Body.String(), "onclick") || !strings.Contains(w.Body.String(), `href=""`) {
		t.Fatalf("默认错误页不应含内联脚本:\n%s", w.Body.String())
	}
}

// 预览页面在 <head> 最前面带上禁止外部资源的 CSP 与 no-referrer，内置风格、清理后的自定义页面都适用，
// <head> 带属性或文本里出现 "<head>" 字样时同样插在正确位置。
func TestForPreview(t *testing.T) {
	r := httptest.NewRequest("GET", "http://x/", nil)
	var pages [][]byte
	for _, src := range []string{
		`<p>x</p><img src="https://tracker.example/p.gif">`,
		`<html><head lang="en"><title>t</title></head><body><img src="https://tracker.example/p.gif"></body></html>`,
		`<head class=x><style>/*<head>*/</style></head><p>x</p>`,
	} {
		page, _, err := SanitizeCustom([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		pages = append(pages, RenderCustom(page, r, Problem{Code: 502}))
	}
	pages = append(pages, RenderError(r, Problem{Code: 502}, ""), RenderError(r, Problem{Code: 502}, "neon"))
	for _, html := range pages {
		out := string(ForPreview(html))
		doc, err := xhtml.Parse(strings.NewReader(out))
		if err != nil {
			t.Fatal(err)
		}
		var head *xhtml.Node
		var find func(n *xhtml.Node)
		find = func(n *xhtml.Node) {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == xhtml.ElementNode && c.Data == "head" && head == nil {
					head = c
				}
				find(c)
			}
		}
		find(doc)
		first := head.FirstChild
		if first == nil || attr(first, "http-equiv") != "Content-Security-Policy" || attr(first, "content") != previewCSP ||
			first.NextSibling == nil || attr(first.NextSibling, "name") != "referrer" {
			t.Fatalf("预览应在 head 最前面带 CSP 与 referrer:\n%s", out[:min(len(out), 500)])
		}
	}
}
