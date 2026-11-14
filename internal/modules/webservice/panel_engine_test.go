package webservice

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/webpage"
)

// 搜索引擎地址：必须含 {q}，只允许 http(s)，{q} 之前要写完主机，不允许其他变量与密码。
func TestPanelEngineURLValidate(t *testing.T) {
	ok := []string{
		"https://www.google.com/search?q={q}",
		"https://www.baidu.com/s?wd={q}",
		"https://github.com/search?q={q}&type=repositories",
		"https://www.zhihu.com/search?type=content&q={q}",
		"http://nas.lan:8080/s/{q}",
		"https://search.example.com/?{q}",
		"https://x.example.com/#{q}",
		"https://x.example.com/s?a={q}&b={q}",
		"HTTPS://Example.com/s?q={q}",
	}
	for _, u := range ok {
		if err := validatePanelEngineURL(u); err != nil {
			t.Errorf("%s 应通过: %v", u, err)
		}
	}
	bad := map[string]string{
		"https://www.google.com/search?q=":                            "缺少 {q}",
		"javascript:alert({q})":                                       "危险协议",
		"ftp://example.com/s?q={q}":                                   "非 http(s)",
		"//example.com/s?q={q}":                                       "无协议",
		"https://{q}.example.com/":                                    "{q} 在主机中",
		"https://example.com{q}":                                      "{q} 紧跟主机会拼出另一个主机",
		"https://example.com:{q}/":                                    "{q} 在端口中",
		"https://{q}@example.com/":                                    "{q} 在用户信息中",
		"https://u:p@example.com/s?q={q}":                             "含密码",
		"https://example.com/s?q={q}&h={host}":                        "其他变量",
		"https://example.com/s?q={q}}":                                "多余花括号",
		"https://example.com/s ?q={q}":                                "含空白",
		"https:///s?q={q}":                                            "没有主机",
		"https://exa mple.com/s?q={q}":                                "主机含空白",
		"https://example.com/" + strings.Repeat("a", 2100) + "?q={q}": "过长",
	}
	for u, why := range bad {
		if err := validatePanelEngineURL(u); err == nil {
			t.Errorf("%s 应拒绝（%s）", u, why)
		}
	}
}

// 面板中的搜索引擎：数量、名称、重复、默认选项都在保存时校验，并规范化空白。
func TestPanelEnginesValidate(t *testing.T) {
	base := func() Panel {
		p := validPanel()
		p.Engines = []PanelEngine{{Name: " Google ", URL: " https://www.google.com/search?q={q} "}, {Name: "GitHub", URL: "https://github.com/search?q={q}"}}
		return p
	}
	p := base()
	if err := p.validate(); err != nil {
		t.Fatal(err)
	}
	if p.Engines[0].Name != "Google" || p.Engines[0].URL != "https://www.google.com/search?q={q}" {
		t.Fatalf("应去掉首尾空白: %+v", p.Engines[0])
	}
	cases := map[string]func(*Panel){
		"名称为空":     func(p *Panel) { p.Engines[0].Name = " " },
		"名称过长":     func(p *Panel) { p.Engines[0].Name = strings.Repeat("a", maxPanelEngineName+1) },
		"名称含控制字符":  func(p *Panel) { p.Engines[0].Name = "a\nb" },
		"名称重复":     func(p *Panel) { p.Engines[1].Name = "google" },
		"地址缺少 {q}": func(p *Panel) { p.Engines[1].URL = "https://github.com/search" },
		"默认选项越界":   func(p *Panel) { p.DefaultEngine = 3 },
		"默认选项为负":   func(p *Panel) { p.DefaultEngine = -1 },
		"引擎过多": func(p *Panel) {
			p.Engines = nil
			for i := range maxPanelEngines + 1 {
				p.Engines = append(p.Engines, PanelEngine{Name: strings.Repeat("e", 1) + string(rune('a'+i)), URL: "https://e.example.com/?q={q}"})
			}
		},
	}
	for name, mod := range cases {
		p := base()
		mod(&p)
		if err := p.validate(); err == nil {
			t.Errorf("%s 应拒绝", name)
		}
	}
	// 默认选项 0（本站）与最后一个引擎都有效；没有引擎时只能是 0
	for _, d := range []int{0, 1, 2} {
		p := base()
		p.DefaultEngine = d
		if err := p.validate(); err != nil {
			t.Errorf("默认选项 %d 应通过: %v", d, err)
		}
	}
	p = validPanel()
	p.DefaultEngine = 1
	if err := p.validate(); err == nil {
		t.Error("没有引擎时默认选项 1 应拒绝")
	}
}

// 页面：配置了引擎时输出下拉框（名称与地址经 HTML 转义），没有引擎时不输出；预览不含下拉框。
func TestPanelEnginesRendered(t *testing.T) {
	p := Panel{Key: "p1", Name: "家里", Enable: true, DefaultEngine: 2, Links: []PanelLink{{Name: "NAS", URL: "https://nas.example.com"}}}
	p.Engines = []PanelEngine{{Name: "Google", URL: "https://www.google.com/search?q={q}"}, {Name: `<b>"x"</b>`, URL: "https://x.example.com/s?a=1&q={q}"}}
	h, _ := panelHandler(t, []Panel{p})
	body := panelGet(h, "/", "203.0.113.9:1").Body.String()
	// 引擎按钮显示默认引擎的名称（名称经 HTML 转义）；引擎数据放在 JSON 中，< > & 被转义，不会提前结束 script
	for _, want := range []string{`<button id="eng"`, `<span id="eng-name">&lt;b&gt;&#34;x&#34;&lt;/b&gt;</span>`, `<button id="go"`,
		`"u":"https://www.google.com/search?q={q}"`, `"def":2`, `\u003cb\u003e`} {
		if !strings.Contains(body, want) {
			t.Errorf("页面缺少 %s", want)
		}
	}
	if strings.Contains(body, `<b>"x"</b>`) || strings.Contains(body, `"n":"<b>`) {
		t.Error("引擎名称应被转义")
	}
	// 页面脚本在换入关键词后核对协议、新标签页打开
	if !strings.Contains(body, `window.open(u,"_blank","noopener,noreferrer")`) || !strings.Contains(body, "encodeURIComponent(kw)") {
		t.Error("缺少搜索脚本")
	}

	p.Engines = nil
	p.DefaultEngine = 0
	h, _ = panelHandler(t, []Panel{p})
	if body := panelGet(h, "/", "203.0.113.9:1").Body.String(); strings.Contains(body, `<button id="eng"`) {
		t.Error("没有引擎时不应输出下拉框")
	}

	cp := &compiledPanel{Panel: Panel{Name: "x", Engines: []PanelEngine{{Name: "Google", URL: "https://www.google.com/search?q={q}"}}}}
	r := httptest.NewRequest("GET", "http://x/", nil)
	html, _ := webpage.RenderPanel(r, cp.view(redirectRequest{r: r}, false, false, true, true))
	if strings.Contains(string(html), `<button id="eng"`) || strings.Contains(string(html), "<script") {
		t.Error("预览不应含引擎下拉框与脚本")
	}
}
