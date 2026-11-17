package webservice

import (
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/webpage"
)

// 链接样式与布局的校验：0 为默认，其余必须在范围内；样式只接受空与 app。
func TestPanelLayoutValidate(t *testing.T) {
	ok := []func(p *Panel){
		func(p *Panel) {},
		func(p *Panel) { p.CardStyle = PanelStyleApp },
		func(p *Panel) { p.IconSize, p.Columns, p.MobileColumns, p.Gap, p.MaxWidth = 24, 1, 1, 4, 640 },
		func(p *Panel) { p.IconSize, p.Columns, p.MobileColumns, p.Gap, p.MaxWidth = 128, 12, 8, 48, 3840 },
	}
	for i, f := range ok {
		p := validPanel()
		f(&p)
		if err := p.validate(); err != nil {
			t.Errorf("第 %d 组应通过: %v", i, err)
		}
	}
	bad := map[string]func(p *Panel){
		"样式大写":   func(p *Panel) { p.CardStyle = "App" },
		"未知样式":   func(p *Panel) { p.CardStyle = "list" },
		"图标过小":   func(p *Panel) { p.IconSize = 23 },
		"图标过大":   func(p *Panel) { p.IconSize = 129 },
		"图标为负":   func(p *Panel) { p.IconSize = -1 },
		"每行过多":   func(p *Panel) { p.Columns = 13 },
		"每行为负":   func(p *Panel) { p.Columns = -2 },
		"手机每行过多": func(p *Panel) { p.MobileColumns = 9 },
		"间距过小":   func(p *Panel) { p.Gap = 3 },
		"间距过大":   func(p *Panel) { p.Gap = 49 },
		"宽度过小":   func(p *Panel) { p.MaxWidth = 639 },
		"宽度过大":   func(p *Panel) { p.MaxWidth = 3841 },
	}
	for name, f := range bad {
		p := validPanel()
		f(&p)
		if err := p.validate(); err == nil {
			t.Errorf("%s 应拒绝", name)
		}
	}
}

// 默认布局：页面不带布局类名与变量，保持原来的卡片样式；其他访问方式的菜单也会输出（卡片样式下由样式隐藏）。
func TestPanelLayoutDefault(t *testing.T) {
	p := Panel{Key: "p1", Name: "家里", Enable: true, Links: []PanelLink{{Name: "NAS", URL: "https://nas.example.com"}}}
	h, _ := panelHandler(t, []Panel{p})
	body := panelGet(h, "/", "203.0.113.9:1").Body.String()
	if regexp.MustCompile(`<html[^>]* class=`).MatchString(body) {
		t.Errorf("默认布局不应给 html 加类名: %.300s", body)
	}
	for _, bad := range []string{"--is:", "--cols:", "--maxw:", "html.lm .grid{grid-template-columns:repeat("} {
		if strings.Contains(body, bad) {
			t.Errorf("默认布局不应输出 %s", bad)
		}
	}
	if !strings.Contains(body, `"layout":{"style":"card","icon":0}`) {
		t.Error("页面脚本数据应带默认布局")
	}
}

// App 图标样式与布局数值：html 带类名，变量与手机每行数量按设置输出；有其他访问方式的大卡片带菜单，小卡片不带。
func TestPanelLayoutApp(t *testing.T) {
	p := Panel{Key: "p1", Name: "家里", Enable: true, CardStyle: PanelStyleApp, IconSize: 72, Columns: 6, MobileColumns: 4, Gap: 20, MaxWidth: 1600,
		Background: "https://img.example.com/bg.jpg",
		Links: []PanelLink{
			{Name: "NAS", URL: "https://nas.example.com", Description: "群晖", Alts: []PanelAlt{{Name: "IPv6", URL: "https://[2001:db8::1]:5001"}, {Name: "STUN", URL: "http://{STUN_none_ADDR}/"}}},
			{Name: "小", URL: "https://s.example.com", Size: PanelSizeSmall, Alts: []PanelAlt{{Name: "frp", URL: "https://frp.example.com"}}},
		}}
	if err := p.validate(); err != nil {
		t.Fatal(err)
	}
	h, _ := panelHandler(t, []Panel{p})
	body := panelGet(h, "/", "203.0.113.9:1").Body.String()
	if !regexp.MustCompile(`<html[^>]* class="hasbg app lc lm"`).MatchString(body) {
		t.Errorf("html 类名: %.300s", body)
	}
	for _, want := range []string{
		":root{--is:72px;--cols:6;--gap:20px;--maxw:1600px}",
		"html.lm .grid{grid-template-columns:repeat(8,minmax(0,1fr))}",
		"html.lm.app .grid{grid-template-columns:repeat(4,minmax(0,1fr))}",
		`"layout":{"style":"app","icon":72}`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("缺少 %s", want)
		}
	}
	nas := cardOf(t, body, "NAS")
	if !strings.Contains(nas, `<details class="am"><summary title="其他访问方式"`) || !strings.Contains(nas, `<a href="https://[2001:db8::1]:5001" target="_blank" rel="noopener noreferrer"`) ||
		!strings.Contains(nas, `<span class="off" title="暂不可用">STUN</span>`) {
		t.Errorf("访问方式菜单: %s", nas)
	}
	// 描述仍在页面中（卡片样式与搜索要用），由样式隐藏
	if !strings.Contains(nas, "群晖") {
		t.Error("描述应保留在页面中")
	}
	if strings.Contains(cardOf(t, body, "小"), `class="am"`) {
		t.Error("小卡片不显示其他访问方式")
	}
}

// 布局写入样式前再收紧一次：即使配置文件被改成越界的值，页面中也只有范围内的整数。
func TestPanelLayoutClampedInPage(t *testing.T) {
	p := Panel{Key: "p1", Name: "家里", Enable: true, CardStyle: "app;}body{display:none", IconSize: 100000, Columns: 99, MobileColumns: -3, Gap: 1, MaxWidth: 1 << 30,
		Links: []PanelLink{{Name: "NAS", URL: "https://nas.example.com"}}}
	c := &compiledPanel{Panel: p}
	v := c.view(redirectRequest{}, false, false, false, true)
	data, _ := json.Marshal(v.Layout)
	if !strings.Contains(string(data), `"IconSize":100000`) {
		t.Fatalf("view 原样传递: %s", data)
	}
	// 直接渲染，绕过载入时的校验
	page, _ := webpage.RenderPanel(httptest.NewRequest("GET", "/", nil), v)
	body := string(page)
	if strings.Contains(body, "body{display:none") || strings.Contains(body, "100000") || strings.Contains(body, "1073741824") {
		t.Errorf("越界的布局应被收紧: %.600s", body)
	}
	for _, want := range []string{"--is:128px", "--cols:12", "--gap:4px", "--maxw:3840px"} {
		if !strings.Contains(body, want) {
			t.Errorf("缺少收紧后的 %s", want)
		}
	}
	if regexp.MustCompile(`<html[^>]* class="[^"]*\bapp\b`).MatchString(body) || strings.Contains(body, " lm") {
		t.Error("无效样式不应启用 App 图标，负的手机每行数量视为默认")
	}
}

// 预览按设置的样式生成，没有脚本时其他访问方式菜单也能点开（details 元素）。
func TestPanelLayoutPreview(t *testing.T) {
	_, do := newCopyModule(t)
	p := validPanel()
	p.CardStyle, p.IconSize = PanelStyleApp, 80
	p.Links = append(p.Links, PanelLink{Name: "多入口", URL: "https://a.example.com", Alts: []PanelAlt{{Name: "IPv6", URL: "https://[2001:db8::2]"}}})
	code, out := do("POST", "/api/webservice/panels/preview", PanelPreviewRequest{Panel: p})
	if code != 200 {
		t.Fatalf("预览: %d %v", code, out)
	}
	html := out["html"].(string)
	if strings.Contains(html, "<script") || !strings.Contains(html, `class="app"`) || !strings.Contains(html, "--is:80px") || !strings.Contains(html, `<details class="am">`) {
		t.Fatalf("预览页面: %.800s", html)
	}
	p.IconSize = 500
	if code, _ = do("POST", "/api/webservice/panels/preview", PanelPreviewRequest{Panel: p}); code != 400 {
		t.Fatalf("越界的图标大小应拒绝预览: %d", code)
	}
}
