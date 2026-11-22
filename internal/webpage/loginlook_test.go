package webpage

import (
	"strings"
	"testing"
)

func TestSafeStyleText(t *testing.T) {
	got := SafeStyleText("a{content:\"</style><script>\"}\x00")
	if strings.Contains(got, "<") || strings.Contains(got, "\x00") || !strings.Contains(got, `\3c /style>`) {
		t.Fatalf("got %q", got)
	}
}

func TestBuildLook(t *testing.T) {
	lk := BuildLook(LookOptions{Skin: "split", Color: "#0a0b0c", Background: `/x"y<z.png`, BgDim: 200, Logo: "/l.png", HideBrand: true, Host: `<i>h`, Lang: "zh-CN"})
	if lk.BodyClass != "skin-split has-bg" {
		t.Fatalf("class: %s", lk.BodyClass)
	}
	css := string(lk.Style)
	for _, want := range []string{"--brand:#0a0b0c", "--brand-2:#", `url("/x\"y\3c z.png")`, "rgba(0,0,0,0.90)", ".foot .brand,.foot .sep{display:none}"} {
		if !strings.Contains(css, want) {
			t.Errorf("样式缺少 %q", want)
		}
	}
	d := string(lk.Decor)
	if !strings.Contains(d, "&lt;i&gt;h") || !strings.Contains(d, `<img src="/l.png"`) || !strings.Contains(d, "受保护的站点") {
		t.Fatalf("分屏装饰: %s", d)
	}
	if def := BuildLook(LookOptions{}); def.BodyClass != "skin-default" || def.Decor != "" {
		t.Fatalf("默认外观: %+v", def)
	}
	for _, id := range LoginSkinIDs() {
		if s := loginSkinByID[id]; strings.Contains(s.css, "<") {
			t.Errorf("样式 %s 含有 <", id)
		}
	}
}
