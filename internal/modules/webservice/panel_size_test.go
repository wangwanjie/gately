package webservice

import (
	"regexp"
	"strings"
	"testing"
)

// App 图标样式不显示名称：html 带 an 类名（访客切换到图标样式时同样生效），名称仍在页面中供搜索与宽块使用。
func TestPanelHideAppName(t *testing.T) {
	p := Panel{Key: "p1", Name: "家里", Enable: true, CardStyle: PanelStyleApp, HideAppName: true, Links: []PanelLink{
		{Name: "NAS", URL: "https://nas.example.com"},
		{Name: "宽块", URL: "https://w.example.com", Size: PanelSizeWide},
	}}
	h, _ := panelHandler(t, []Panel{p})
	body := panelGet(h, "/", "203.0.113.9:1").Body.String()
	if !regexp.MustCompile(`<html[^>]* class="app an"`).MatchString(body) {
		t.Errorf("html 类名应带 an: %.200s", body)
	}
	if !strings.Contains(cardOf(t, body, "NAS"), `class="nm">NAS`) || !strings.Contains(cardOf(t, body, "宽块"), `card wd link`) {
		t.Error("名称仍输出，由样式按尺寸隐藏")
	}
	// 图标样式的基础规则悬停时把卡片重置为占一列，1×2、2×2 的跨列规则必须同时覆盖悬停状态，否则悬停时块缩小、来回跳动
	for _, want := range []string{
		"html.app .grid .card.xl,html.app .grid .card.xl.link:hover{grid-column:span 2;grid-row:span 2;",
		"html.app .grid .card.wd,html.app .grid .card.wd.link:hover{grid-column:span 2;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("缺少悬停时的跨列规则 %s", want)
		}
	}
	p.HideAppName = false
	h, _ = panelHandler(t, []Panel{p})
	if b := panelGet(h, "/", "203.0.113.9:1").Body.String(); regexp.MustCompile(`<html[^>]* class="[^"]*\ban\b`).MatchString(b) {
		t.Error("默认显示名称")
	}
}

// 每行只有一个链接时，1×2、2×2 的块改为占一列，不撑出多余的列；其他数量不输出该规则。
func TestPanelSingleColumnSpans(t *testing.T) {
	p := Panel{Key: "p1", Name: "家里", Enable: true, CardStyle: PanelStyleApp, Columns: 1, MobileColumns: 1, Links: []PanelLink{{Name: "NAS", URL: "https://nas.example.com", Size: PanelSizeBig}}}
	h, _ := panelHandler(t, []Panel{p})
	body := panelGet(h, "/", "203.0.113.9:1").Body.String()
	for _, want := range []string{
		"@media (min-width:641px){html.app.lc .grid .card.wd,html.app.lc .grid .card.xl,html.app.lc .grid .card.wd.link:hover,html.app.lc .grid .card.xl.link:hover{grid-column:span 1}}",
		"html.lm.app .grid .card.wd,html.lm.app .grid .card.xl,html.lm.app .grid .card.wd.link:hover,html.lm.app .grid .card.xl.link:hover{grid-column:span 1}}",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("缺少 %s", want)
		}
	}
	p.Columns, p.MobileColumns = 2, 2
	h, _ = panelHandler(t, []Panel{p})
	if b := panelGet(h, "/", "203.0.113.9:1").Body.String(); strings.Contains(b, "grid-column:span 1}}") {
		t.Error("每行多于一个时不应输出占一列的规则")
	}
}
