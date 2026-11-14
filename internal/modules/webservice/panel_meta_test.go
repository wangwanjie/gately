package webservice

import (
	"html"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

var (
	titleRe   = regexp.MustCompile(`<title>([^<]*)</title>`)
	faviconRe = regexp.MustCompile(`<link rel="icon" href="([^"]*)">`)
	metaRe    = regexp.MustCompile(`<meta name="description" content="([^"]*)">`)
)

// 标签页标题、标签页图标、页面描述可单独设置，留空时分别沿用页面标题、页面图标，没有描述时不输出 meta。
func TestPanelTabMeta(t *testing.T) {
	page := func(p Panel) string {
		p.Key = "p1"
		p.Enable = true
		if p.Links == nil {
			p.Links = []PanelLink{{Name: "NAS", URL: "https://nas.example.com"}}
		}
		h, _ := panelHandler(t, []Panel{p})
		return panelGet(h, "/", "203.0.113.9:1").Body.String()
	}
	// 默认：标签页标题取页面标题，没有 meta description
	b := page(Panel{Name: "家里", Title: "我的导航"})
	if m := titleRe.FindStringSubmatch(b); m == nil || m[1] != "我的导航" {
		t.Errorf("标签页标题应取页面标题: %v", m)
	}
	if metaRe.MatchString(b) {
		t.Error("没有描述时不应输出 meta description")
	}
	// 单独设置
	b = page(Panel{Name: "家里", Title: "我的导航", TabTitle: "NAS · 主页", Description: `家庭 "NAS" <入口>`})
	if m := titleRe.FindStringSubmatch(b); m == nil || m[1] != "NAS · 主页" {
		t.Errorf("标签页标题: %v", m)
	}
	if !strings.Contains(b, `<h1>我的导航</h1>`) {
		t.Error("页面标题不应受标签页标题影响")
	}
	if m := metaRe.FindStringSubmatch(b); m == nil || m[1] != `家庭 &#34;NAS&#34; &lt;入口&gt;` {
		t.Errorf("描述应被转义: %v", m)
	}
	// 页面标题与标签页标题都为空时退回面板名称（视图层已处理）
	if m := titleRe.FindStringSubmatch(page(Panel{Name: "家里"})); m == nil || m[1] != "家里" {
		t.Errorf("退回面板名称: %v", m)
	}
}

func decodeFavicon(t *testing.T, href string) string {
	t.Helper()
	svg, ok := strings.CutPrefix(href, "data:image/svg+xml,")
	if !ok {
		t.Fatalf("不是 SVG data URL: %.80s", href)
	}
	s, err := url.PathUnescape(svg)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPanelFavicon(t *testing.T) {
	icon := func(p Panel) string {
		p.Key, p.Enable = "p1", true
		p.Links = []PanelLink{{Name: "NAS", URL: "https://nas.example.com"}}
		h, _ := panelHandler(t, []Panel{p})
		m := faviconRe.FindStringSubmatch(panelGet(h, "/", "203.0.113.9:1").Body.String())
		if m == nil {
			t.Fatal("没有找到标签页图标")
		}
		// html/template 会把 href 中的 + 写成 &#43;，浏览器解析后相同
		return html.UnescapeString(m[1])
	}
	// 没有设置：默认图标
	if got := icon(Panel{Name: "家"}); !strings.Contains(got, "%230e7c66") {
		t.Errorf("默认图标: %.80s", got)
	}
	// 沿用页面图标；单独设置的优先
	if got := icon(Panel{Name: "家", Icon: pngIcon}); got != pngIcon {
		t.Errorf("应沿用页面图标: %.60s", got)
	}
	if got := icon(Panel{Name: "家", Icon: pngIcon, Favicon: "https://img.example.com/f.png"}); got != "https://img.example.com/f.png" {
		t.Errorf("单独设置的图标优先: %.60s", got)
	}
	// 文字（emoji）画成色块上的文字，XML 转义
	svg := decodeFavicon(t, icon(Panel{Name: "家", Favicon: "📦"}))
	if !strings.Contains(svg, ">📦</text>") || !strings.Contains(svg, `fill="#`) {
		t.Errorf("文字图标: %s", svg)
	}
	if svg := decodeFavicon(t, icon(Panel{Name: "家", Favicon: "A&B"})); !strings.Contains(svg, ">A&amp;B</text>") || strings.Contains(svg, "A&B<") {
		t.Errorf("文字应被 XML 转义: %s", svg)
	}
	// 内置图标画在底色方块上，内嵌图片地址被转义
	svg = decodeFavicon(t, icon(Panel{Name: "家", Favicon: "builtin:github"}))
	if !strings.Contains(svg, "<image href=\"data:image/svg+xml;base64,") || !strings.Contains(svg, `<rect width="32" height="32" rx="8" fill="#`) {
		t.Errorf("内置图标: %.200s", svg)
	}
	if strings.Count(svg, "<image ") != 1 || strings.Contains(svg, "<script") {
		t.Errorf("SVG 结构异常: %.200s", svg)
	}
}

func TestPanelMetaValidate(t *testing.T) {
	ok := validPanel()
	ok.TabTitle, ok.Favicon, ok.Description = " 标签 ", "📦", " 描述 "
	if err := ok.validate(); err != nil || ok.TabTitle != "标签" || ok.Description != "描述" {
		t.Fatalf("应通过并去掉空白: %v %+v", err, ok)
	}
	for name, mod := range map[string]func(*Panel){
		"标签页标题过长":   func(p *Panel) { p.TabTitle = strings.Repeat("a", maxPanelTitleLen+1) },
		"标签页标题含控制符": func(p *Panel) { p.TabTitle = "a\nb" },
		"描述过长":      func(p *Panel) { p.Description = strings.Repeat("a", 2*maxPanelTextLen+1) },
		"图标无效":      func(p *Panel) { p.Favicon = "javascript:alert(1)" },
		"内置图标不存在":   func(p *Panel) { p.Favicon = "builtin:no-such" },
	} {
		p := validPanel()
		mod(&p)
		if err := p.validate(); err == nil {
			t.Errorf("%s 应拒绝", name)
		}
	}
	// 载入面板库时不存在的内置标签页图标只清空，不丢弃面板
	s := newPanelStore()
	p := validPanel()
	p.Key, p.Favicon = "p1", "builtin:no-such"
	s.set([]Panel{p})
	if cp, ok := s.get("p1"); !ok || cp.Favicon != "" {
		t.Errorf("未知内置标签页图标应降级: %v %+v", ok, cp)
	}
}
