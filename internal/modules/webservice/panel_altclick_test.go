package webservice

import (
	"strings"
	"testing"
)

// cardByLabel 按 aria-label 找卡片：多地址链接直接打开时 title 带有提示文字，不能用 cardOf 按 title 查找。
func cardByLabel(t *testing.T, body, name string) string {
	t.Helper()
	for _, c := range cardRe.FindAllString(body, -1) {
		if strings.Contains(c, `aria-label="`+name+`"`) {
			return c
		}
	}
	t.Fatalf("没有找到卡片 %s", name)
	return ""
}

// 多地址点击方式只接受空（弹出选择）与 direct。
func TestPanelAltClickValidate(t *testing.T) {
	for _, v := range []string{"", "direct"} {
		p := validPanel()
		p.AltClick = v
		if err := p.validate(); err != nil {
			t.Errorf("点击方式 %q 应通过: %v", v, err)
		}
	}
	for _, v := range []string{"Direct", "chooser", " direct", "menu"} {
		p := validPanel()
		p.AltClick = v
		if err := p.validate(); err == nil {
			t.Errorf("点击方式 %q 应拒绝", v)
		}
	}
}

// 点击直接打开：脚本数据带 direct，可直接打开的多地址链接不再标明弹出列表、提示可右键或长按；
// 主地址暂不可用的多地址链接仍是按钮，点击弹出选择。默认（弹出选择）时脚本数据为 false。
func TestPanelAltClickDirect(t *testing.T) {
	links := []PanelLink{
		{Name: "NAS", URL: "https://nas.example.com", Alts: []PanelAlt{{Name: "IPv6", URL: "https://[2001:db8::1]:5001"}}},
		{Name: "STUN 服务", URL: "http://{STUN_none_ADDR}/", Alts: []PanelAlt{{Name: "frp", URL: "https://frp.example.com"}}},
		{Name: "相册", URL: "https://photo.example.com"},
	}
	p := Panel{Key: "p1", Name: "家里", Enable: true, Links: links}
	h, _ := panelHandler(t, []Panel{p})
	body := panelGet(h, "/", "203.0.113.9:1").Body.String()
	if !strings.Contains(body, `"direct":false`) || !strings.Contains(cardOf(t, body, "NAS"), `aria-haspopup="dialog"`) {
		t.Fatal("默认点击弹出地址选择")
	}

	p.AltClick = PanelAltClickDirect
	h, _ = panelHandler(t, []Panel{p})
	body = panelGet(h, "/", "203.0.113.9:1").Body.String()
	if !strings.Contains(body, `"direct":true`) {
		t.Error("脚本数据应带 direct")
	}
	nas := cardByLabel(t, body, "NAS")
	if strings.Contains(nas, `aria-haspopup`) || !strings.Contains(nas, `title="NAS - 右键或长按可选择其他访问地址"`) || !strings.Contains(nas, `class="alts" hidden`) {
		t.Errorf("直接打开的多地址链接: %s", nas)
	}
	if c := cardByLabel(t, body, "STUN 服务"); !strings.Contains(c, `<button class="hit" type="button" title="STUN 服务" aria-label="STUN 服务" aria-haspopup="dialog">`) {
		t.Errorf("主地址不可用时仍点击弹出选择: %s", c)
	}
	if c := cardOf(t, body, "相册"); strings.Contains(c, "右键") {
		t.Errorf("单地址链接不加提示: %s", c)
	}
}
