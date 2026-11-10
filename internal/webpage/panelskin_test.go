package webpage

import (
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// 皮肤数据包含管理界面的全部色系，每个色系至少有一套调色板，基础色都是 #rrggbb；10 种语言的文案齐全。
func TestPanelSkinData(t *testing.T) {
	loadSkins()
	if len(skins.Families) < 10 || !ValidSkin(skins.Default) || !ValidSkin("") || ValidSkin("nope") {
		t.Fatalf("色系数据异常: %d %q", len(skins.Families), skins.Default)
	}
	hex := regexp.MustCompile(`^#[0-9a-f]{6}$`)
	for _, f := range skins.Families {
		if f.Light == nil && f.Dark == nil {
			t.Fatalf("%s 没有调色板", f.ID)
		}
		for _, p := range []*SkinPalette{f.Light, f.Dark} {
			if p == nil {
				continue
			}
			for _, c := range []string{p.Primary, p.Page, p.Card, p.Sidebar, p.Border, p.Text, p.TextSub, p.Logo[0], p.Logo[1], p.Logo[2]} {
				if !hex.MatchString(c) {
					t.Fatalf("%s 颜色格式错误: %q", f.ID, c)
				}
			}
		}
	}
	for _, lang := range []string{"zh-CN", "zh-TW", "en", "ja", "ko", "de", "fr", "es", "pt-BR", "ru"} {
		tx, ok := skins.Texts[lang]
		if !ok || len(tx.Names) != len(skins.Families) || tx.UI["switchTheme"] == "" {
			t.Fatalf("%s 文案不全", lang)
		}
	}
	if skinLangTexts("xx").UI["title"] != skins.Texts["en"].UI["title"] {
		t.Fatal("未知语言应使用英文")
	}
}

// 服务端按面板设置的皮肤输出初始变量：默认色系与管理界面一致；只有一种模式的色系两套变量相同。
func TestPanelSkinCSS(t *testing.T) {
	css := skinCSS("")
	if !strings.Contains(css, ":root{--brand:#0e7c66;") || !strings.Contains(css, "--bg:#f4f6f8;") || !strings.Contains(css, "--r:14px;") {
		t.Fatalf("默认色系变量错误: %s", css)
	}
	if !strings.Contains(css, "prefers-color-scheme:dark") || !strings.Contains(css, ":root[data-theme=dark]{--brand:#27a589;") {
		t.Fatalf("暗色变量错误: %s", css)
	}
	for _, f := range skins.Families {
		if f.Light != nil && f.Dark != nil {
			continue
		}
		css := skinCSS(f.ID)
		p := f.Light
		if p == nil {
			p = f.Dark
		}
		if strings.Count(css, "--bg:"+p.Page+";") != 4 {
			t.Fatalf("%s 只有一种模式，四处变量都应使用它: %s", f.ID, css)
		}
	}
	if mixHex("#000000", "#ffffff", 0.5) != "#808080" || rgba("#0e7c66", 0.1) != "rgba(14,124,102,0.1)" {
		t.Fatal("颜色计算错误")
	}
}

// 页面输出皮肤数据、换肤按钮与脚本；数据在 JSON 块中经过转义，不能提前结束脚本块；预览不输出脚本与按钮。
func TestRenderPanelSkin(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept-Language", "en")
	page, csp := RenderPanel(r, PanelView{Title: "</script><script>alert(1)</script>", Skin: "nord", Theme: "dark"})
	html := string(page)
	if !strings.Contains(csp, "script-src 'nonce-") {
		t.Fatalf("CSP: %s", csp)
	}
	if !strings.Contains(html, `id="skin-btn"`) || !strings.Contains(html, `data-theme="dark"`) || !strings.Contains(html, "Switch theme") {
		t.Fatal("缺少换肤按钮或模式")
	}
	m := regexp.MustCompile(`(?s)<script type="application/json" id="skin-data">(.*?)</script>`).FindStringSubmatch(html)
	if m == nil {
		t.Fatal("缺少皮肤数据")
	}
	var d struct {
		Skin, Mode string
		Families   []SkinFamily
		Texts      skinTexts
	}
	if err := json.Unmarshal([]byte(m[1]), &d); err != nil {
		t.Fatalf("皮肤数据不是合法 JSON: %v", err)
	}
	if d.Skin != "nord" || d.Mode != "dark" || len(d.Families) != len(skins.Families) || d.Texts.Names["nord"] == "" {
		t.Fatalf("皮肤数据错误: %+v", d)
	}
	if strings.Count(html, "<script") != 3 || strings.Contains(html, "<script>alert") {
		t.Fatal("标题中的脚本标签应被转义")
	}
	nord, _ := findSkin("nord")
	if !strings.Contains(html, "--bg:"+nord.Light.Page+";") {
		t.Fatal("页面应输出面板设置的皮肤")
	}
	// 无效皮肤退回默认色系
	page, _ = RenderPanel(r, PanelView{Title: "x", Skin: "nope"})
	if !strings.Contains(string(page), "--brand:#0e7c66;") {
		t.Fatal("无效皮肤应使用默认色系")
	}
	page, _ = RenderPanel(r, PanelView{Title: "x", Skin: "nord", Preview: true})
	if html := string(page); strings.Contains(html, `id="skin-data"`) || strings.Contains(html, `id="skin-btn"`) || strings.Contains(html, "<script") || !strings.Contains(html, "--bg:"+nord.Light.Page+";") {
		t.Fatal("预览应只输出皮肤样式，不含脚本与按钮")
	}
}
