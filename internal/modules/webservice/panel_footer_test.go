package webservice

import (
	"regexp"
	"strings"
	"testing"
)

var footerRe = regexp.MustCompile(`(?s)<footer class="foot">(.*?)</footer>`)

// 页脚：默认显示 Gately 标志与名称；可改为自定义文字（可带链接）或隐藏。
func TestPanelFooter(t *testing.T) {
	footer := func(p Panel) (string, bool) {
		p.Key, p.Enable = "p1", true
		p.Links = []PanelLink{{Name: "NAS", URL: "https://nas.example.com"}}
		h, _ := panelHandler(t, []Panel{p})
		m := footerRe.FindStringSubmatch(panelGet(h, "/", "203.0.113.9:1").Body.String())
		if m == nil {
			return "", false
		}
		return m[1], true
	}
	if f, ok := footer(Panel{Name: "家"}); !ok || !strings.Contains(f, "Gately") || !strings.Contains(f, "<svg") {
		t.Errorf("默认页脚: %q", f)
	}
	if f, ok := footer(Panel{Name: "家", Footer: "© 2026 我的家"}); !ok || f != "<span>© 2026 我的家</span>" {
		t.Errorf("自定义文字: %q", f)
	}
	f, _ := footer(Panel{Name: "家", Footer: `<b>"我的"</b>`, FooterURL: "https://example.com/a?x=1&y=2"})
	if !strings.Contains(f, `href="https://example.com/a?x=1&amp;y=2"`) || !strings.Contains(f, `rel="noopener noreferrer"`) || strings.Contains(f, "<b>") || !strings.Contains(f, "&lt;b&gt;") {
		t.Errorf("带链接且文字被转义: %q", f)
	}
	if _, ok := footer(Panel{Name: "家", HideFooter: true}); ok {
		t.Error("隐藏页脚时不应输出 footer")
	}
	if _, ok := footer(Panel{Name: "家", HideFooter: true, Footer: "x"}); ok {
		t.Error("隐藏优先于自定义文字")
	}
}

func TestPanelFooterValidate(t *testing.T) {
	ok := validPanel()
	ok.Footer, ok.FooterURL = " © 我 ", " https://example.com/about "
	if err := ok.validate(); err != nil || ok.Footer != "© 我" || ok.FooterURL != "https://example.com/about" {
		t.Fatalf("应通过并去掉空白: %v %+v", err, ok)
	}
	for name, mod := range map[string]func(*Panel){
		"文字过长":     func(p *Panel) { p.Footer = strings.Repeat("a", maxPanelTextLen+1) },
		"文字含控制字符":  func(p *Panel) { p.Footer = "a\nb" },
		"链接 js 协议": func(p *Panel) { p.FooterURL = "javascript:alert(1)" },
		"链接 ftp":   func(p *Panel) { p.FooterURL = "ftp://example.com" },
		"链接含密码":    func(p *Panel) { p.FooterURL = "https://u:p@example.com" },
		"链接无主机":    func(p *Panel) { p.FooterURL = "https:///x" },
		"链接含空白":    func(p *Panel) { p.FooterURL = "https://exa mple.com" },
	} {
		p := validPanel()
		mod(&p)
		if err := p.validate(); err == nil {
			t.Errorf("%s 应拒绝", name)
		}
	}
}
