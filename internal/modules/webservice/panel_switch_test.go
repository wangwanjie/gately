package webservice

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// 锁标记：已登录访客默认看到锁标记，面板设置不显示后没有锁标记；未登录时链接本身就不下发。
func TestPanelHideLock(t *testing.T) {
	q := redirectRequest{r: httptest.NewRequest("GET", "http://home.nav.test/", nil)}
	p := authOnlyPanel()
	if l := (&compiledPanel{Panel: p}).view(q, false, false, false, true).Groups[1].Links[0]; !l.Locked {
		t.Fatalf("默认显示锁标记: %+v", l)
	}
	p.HideLock = true
	v := (&compiledPanel{Panel: p}).view(q, false, false, false, true)
	if l := v.Groups[1].Links[0]; l.Locked || l.Name != "内部站" {
		t.Fatalf("不显示锁标记时链接照常显示、没有锁标记: %+v", l)
	}
	if v = (&compiledPanel{Panel: p}).view(q, false, false, false, false); v.Hidden != 1 || len(v.Groups) != 1 {
		t.Fatalf("未登录时仍不下发登录后可见的链接: %+v", v)
	}
}

// 不显示换肤按钮：页面没有换肤按钮，皮肤脚本数据带 fixed（不读取访客保存的选择）；默认显示且 fixed 为 false。
func TestPanelHideSkin(t *testing.T) {
	p := Panel{Key: "p1", Name: "家里", Enable: true, Links: []PanelLink{{Name: "NAS", URL: "https://nas.example.com"}}}
	h, _ := panelHandler(t, []Panel{p})
	body := panelGet(h, "/", "203.0.113.9:1").Body.String()
	if !strings.Contains(body, `id="skin-btn"`) || !strings.Contains(body, `"fixed":false`) {
		t.Fatal("默认显示换肤按钮")
	}
	p.HideSkin = true
	h, _ = panelHandler(t, []Panel{p})
	body = panelGet(h, "/", "203.0.113.9:1").Body.String()
	if strings.Contains(body, `id="skin-btn"`) || !strings.Contains(body, `"fixed":true`) {
		t.Errorf("不显示换肤按钮: %.400s", body)
	}
	// 面板设置的皮肤变量仍由服务端输出
	if !strings.Contains(body, "--brand:") {
		t.Error("应输出面板皮肤的样式变量")
	}
}
