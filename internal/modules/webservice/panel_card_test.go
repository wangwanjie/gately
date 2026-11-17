package webservice

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// 卡片尺寸：只接受空（大卡片）与 small；标题、描述的隐藏只影响显示，不影响搜索数据。
func TestPanelLinkSizeValidate(t *testing.T) {
	for _, sz := range []string{"", "small"} {
		p := validPanel()
		p.Links[0].Size = sz
		if err := p.validate(); err != nil {
			t.Errorf("尺寸 %q 应通过: %v", sz, err)
		}
	}
	for _, sz := range []string{"large", "Small", "huge", " small"} {
		p := validPanel()
		p.Links[0].Size = sz
		if err := p.validate(); err == nil {
			t.Errorf("尺寸 %q 应拒绝", sz)
		}
	}
}

var cardRe = regexp.MustCompile(`(?s)<div class="card[^"]*"[^>]*data-s="[^"]*"[^>]*>.*?\n</div>`)

func cardOf(t *testing.T, body, name string) string {
	t.Helper()
	for _, c := range cardRe.FindAllString(body, -1) {
		if strings.Contains(c, `title="`+name+`"`) {
			return c
		}
	}
	t.Fatalf("没有找到卡片 %s", name)
	return ""
}

func TestPanelCardRendering(t *testing.T) {
	p := Panel{Key: "p1", Name: "家里", Enable: true, Links: []PanelLink{
		{Name: "大卡", URL: "https://a.example.com", Description: "描述A"},
		{Name: "无标题", URL: "https://b.example.com", Description: "描述B", HideName: true},
		{Name: "无描述", URL: "https://c.example.com", Description: "描述C", HideDesc: true},
		{Name: "无描述无地址", URL: "https://d.example.com", HideDesc: true},
		{Name: "全隐藏", URL: "https://f.example.com", Description: "描述F", HideName: true, HideDesc: true},
		{Name: "全隐藏有入口", URL: "https://g.example.com", HideName: true, HideDesc: true, Alts: []PanelAlt{{Name: "IPv6", URL: "https://[2001:db8::1]"}}},
		{Name: "小卡", URL: "https://e.example.com", Description: "描述E", Size: PanelSizeSmall, Alts: []PanelAlt{{Name: "IPv6", URL: "https://[2001:db8::1]"}}},
	}}
	h, _ := panelHandler(t, []Panel{p})
	body := panelGet(h, "/", "203.0.113.9:1").Body.String()
	big := cardOf(t, body, "大卡")
	if strings.Contains(big, `card sm`) || !strings.Contains(big, `class="nm">大卡`) || !strings.Contains(big, "描述A") {
		t.Errorf("大卡片应有标题与描述: %s", big)
	}
	if c := cardOf(t, body, "无标题"); strings.Contains(c, `class="nm"`) || !strings.Contains(c, "描述B") {
		t.Errorf("隐藏标题只隐藏标题: %s", c)
	}
	if c := cardOf(t, body, "无描述"); !strings.Contains(c, `class="nm">无描述`) || strings.Contains(c, "描述C") {
		t.Errorf("隐藏描述只隐藏描述: %s", c)
	}
	if c := cardOf(t, body, "无描述无地址"); strings.Contains(c, `class="ds`) {
		t.Errorf("没有描述时地址也应隐藏: %s", c)
	}
	// 标题和描述都不显示时没有可放文字的内容，按小卡片显示；其他访问方式不显示在卡片上，有没有都一样
	if c := cardOf(t, body, "全隐藏"); !strings.Contains(c, `card sm link`) {
		t.Errorf("标题与描述都隐藏时应按小卡片显示: %s", c)
	}
	if c := cardOf(t, body, "全隐藏有入口"); !strings.Contains(c, `card sm link multi`) || !strings.Contains(c, `<span class="alts" hidden><a class="alt"`) {
		t.Errorf("有其他访问方式时也按小卡片显示，访问方式留给选择列表: %s", c)
	}
	small := cardOf(t, body, "小卡")
	if !strings.Contains(small, `card sm link`) || strings.Contains(small, `class="nm"`) || strings.Contains(small, "描述E") || !strings.Contains(small, `class="alts" hidden`) || !strings.Contains(small, `aria-label="小卡"`) {
		t.Errorf("小卡片只显示图标，名称在提示中: %s", small)
	}
	// 隐藏的标题与描述仍可被页面搜索命中
	if m := regexp.MustCompile(`data-s="([^"]*)"`).FindAllStringSubmatch(big+cardOf(t, body, "无标题"), -1); len(m) != 2 || !strings.Contains(m[1][1], "无标题") {
		t.Errorf("搜索数据应含名称: %v", m)
	}
	// 常用链接按分组与名称标识，地址变化不影响
	k1 := regexp.MustCompile(`data-k="([0-9a-f]+)"`).FindStringSubmatch(big)[1]
	p.Links[0].URL = "https://changed.example.com"
	h2, _ := panelHandler(t, []Panel{p})
	if k2 := regexp.MustCompile(`data-k="([0-9a-f]+)"`).FindStringSubmatch(cardOf(t, panelGet(h2, "/", "203.0.113.9:1").Body.String(), "大卡"))[1]; k1 != k2 {
		t.Errorf("链接标识应只取决于分组与名称: %s %s", k1, k2)
	}
}

// 语言：Cookie 选择优先于 Accept-Language，无效值被忽略；页面提供语言菜单数据。
func TestPanelLanguageCookie(t *testing.T) {
	p := Panel{Key: "p1", Name: "家里", Enable: true, Links: []PanelLink{{Name: "NAS", URL: "https://nas.example.com"}}}
	h, _ := panelHandler(t, []Panel{p})
	get := func(cookie, accept string) string {
		r := httptest.NewRequest("GET", "http://home.nav.test/", nil)
		r.Host = "home.nav.test"
		r.RemoteAddr = "203.0.113.9:1"
		r.Header.Set("Accept-Language", accept)
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: panelLangCookie, Value: cookie})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Body.String()
	}
	if b := get("", "zh-CN"); !strings.Contains(b, `<html lang="zh-CN"`) || !strings.Contains(b, `placeholder="搜索"`) {
		t.Error("默认按 Accept-Language")
	}
	b := get("en", "zh-CN")
	if !strings.Contains(b, `<html lang="en"`) || !strings.Contains(b, `placeholder="Search"`) {
		t.Errorf("Cookie 选择英语: %.200s", b)
	}
	for _, want := range []string{`id="lang-btn"`, `"langs":[{"c":"zh-CN","n":"简体中文"}`, `"n":"Português (Brasil)"`, `"lang":"en"`} {
		if !strings.Contains(b, want) {
			t.Errorf("缺少 %s", want)
		}
	}
	if strings.Contains(b, "gately/issues") || strings.Contains(b, `class="note"`) {
		t.Error("语言菜单不应再有 AI 翻译提示")
	}
	for _, bad := range []string{"xx", "../../etc", `en"><script>`, "EN", "zh_CN"} {
		if b := get(bad, "ja"); !strings.Contains(b, `<html lang="ja"`) {
			t.Errorf("无效语言 %q 应被忽略", bad)
		}
	}
	if b := get("", "zh-CN"); !strings.Contains(b, `id="set-btn"`) {
		t.Error("应有页面设置按钮")
	}
}
