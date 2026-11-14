package webservice

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/webpage"
)

// 背景图片：只接受 http(s) 地址与白名单格式的内联图片，限制大小，遮罩浓度在 0-90。
func TestPanelBackgroundValidate(t *testing.T) {
	ok := []string{"", pngIcon, "https://img.example.com/bg.jpg?x=1", "http://nas.lan/bg.png",
		"data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciLz4="}
	for _, b := range ok {
		if _, err := validatePanelBackground(b); err != nil {
			t.Errorf("%.40s 应通过: %v", b, err)
		}
	}
	bad := []string{
		"javascript:alert(1)", "ftp://x.example.com/a.png", "//x.example.com/a.png", "/local/path.png", "bg.png",
		"data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==", "data:image/png;base64,@@@", "data:image/png,rawdata",
		"https://u:p@x.example.com/a.png", "https://x.example.com/a b.png", "https:///a.png",
		"data:image/png;base64," + strings.Repeat("A", maxPanelBackground),
		"https://x.example.com/" + strings.Repeat("a", maxPanelURLLen),
	}
	for _, b := range bad {
		if _, err := validatePanelBackground(b); err == nil {
			t.Errorf("%.40s 应拒绝", b)
		}
	}
	// 内联背景计入面板的图片总量
	big := "data:image/png;base64," + strings.Repeat("A", 1000<<10)
	p := validPanel()
	p.Background = big
	p.Links = nil
	for i := range 4 {
		p.Links = append(p.Links, PanelLink{Name: "l" + string(rune('a'+i)), URL: "https://x.example.com", Icon: "data:image/png;base64," + strings.Repeat("A", 90<<10)})
	}
	if err := p.validate(); err != nil {
		t.Fatalf("总量未超限应通过: %v", err)
	}
	for i := range 40 {
		p.Links = append(p.Links, PanelLink{Name: "m" + string(rune('A'+i%26)) + string(rune('a'+i/26)), URL: "https://x.example.com", Icon: "data:image/png;base64," + strings.Repeat("A", 90<<10)})
	}
	if err := p.validate(); err == nil || !strings.Contains(err.Error(), "合计") {
		t.Fatalf("图标与背景合计超限应拒绝: %v", err)
	}
	for _, b := range []int{-1, maxPanelBgBlur + 1} {
		p := validPanel()
		p.BgBlur = b
		if err := p.validate(); err == nil {
			t.Errorf("模糊 %d 应拒绝", b)
		}
	}
	for _, d := range []int{-1, maxPanelBgDim + 1} {
		p := validPanel()
		p.BgDim = d
		if err := p.validate(); err == nil {
			t.Errorf("遮罩 %d 应拒绝", d)
		}
	}
	for _, d := range []int{0, 45, maxPanelBgDim} {
		p := validPanel()
		p.BgDim = d
		if err := p.validate(); err != nil {
			t.Errorf("遮罩 %d 应通过: %v", d, err)
		}
	}
}

var bgSrcRe = regexp.MustCompile(`<div class="bg has"[^>]*><img src="([^"]+)"`)

// 页面与背景图片接口：内联图片走专用路径（带指纹版本、可缓存、sandbox），图片地址直接引用；没有背景时没有 img。
func TestPanelBackgroundServing(t *testing.T) {
	svg := "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciPjxzY3JpcHQ+YWxlcnQoMSk8L3NjcmlwdD48L3N2Zz4="
	mk := func(bg string, dim int) Panel {
		return Panel{Key: "p1", Name: "家里", Enable: true, Background: bg, BgDim: dim, BgBlur: dim / 7, Links: []PanelLink{{Name: "NAS", URL: "https://nas.example.com"}}}
	}

	t.Run("内联图片", func(t *testing.T) {
		h, _ := panelHandler(t, []Panel{mk(pngIcon, 40)})
		w := panelGet(h, "/", "203.0.113.9:1")
		m := bgSrcRe.FindStringSubmatch(w.Body.String())
		if m == nil || !strings.HasPrefix(m[1], panelBgPath+"?v=") || strings.Contains(w.Body.String(), "base64") {
			t.Fatalf("页面应引用背景路径而不内联图片数据: %v", m)
		}
		if !strings.Contains(w.Body.String(), ":root{--bgdim:0.40;--bgblur:5px}") {
			t.Error("缺少遮罩与模糊变量")
		}
		img := panelGet(h, m[1], "203.0.113.9:1")
		if img.Code != 200 || img.Header().Get("Content-Type") != "image/png" || len(img.Body.Bytes()) == 0 || img.Body.Bytes()[1] != 'P' {
			t.Fatalf("背景图片: %d %s %d", img.Code, img.Header().Get("Content-Type"), img.Body.Len())
		}
		hd := img.Header()
		if hd.Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(hd.Get("Content-Security-Policy"), "sandbox") || !strings.HasPrefix(hd.Get("Cache-Control"), "private, max-age=") {
			t.Fatalf("安全与缓存头: %v", hd)
		}
		// 条件请求
		r := httptest.NewRequest("GET", "http://home.nav.test"+panelBgPath, nil)
		r.Host = "home.nav.test"
		r.Header.Set("If-None-Match", hd.Get("ETag"))
		nw := httptest.NewRecorder()
		h.ServeHTTP(nw, r)
		if nw.Code != http.StatusNotModified || nw.Body.Len() != 0 {
			t.Fatalf("应 304: %d", nw.Code)
		}
		// 方法限制
		r = httptest.NewRequest("POST", "http://home.nav.test"+panelBgPath, nil)
		r.Host = "home.nav.test"
		pw := httptest.NewRecorder()
		h.ServeHTTP(pw, r)
		if pw.Code != http.StatusMethodNotAllowed {
			t.Fatalf("POST 应拒绝: %d", pw.Code)
		}
		// 换图后指纹变化
		h2, _ := panelHandler(t, []Panel{mk(svg, 40)})
		m2 := bgSrcRe.FindStringSubmatch(panelGet(h2, "/", "203.0.113.9:1").Body.String())
		if m2 == nil || m2[1] == m[1] {
			t.Fatalf("换图后版本应变化: %v %v", m, m2)
		}
	})
	t.Run("SVG 只能以 sandbox 输出", func(t *testing.T) {
		h, _ := panelHandler(t, []Panel{mk(svg, 0)})
		img := panelGet(h, panelBgPath, "203.0.113.9:1")
		if img.Code != 200 || img.Header().Get("Content-Type") != "image/svg+xml" || img.Header().Get("Content-Security-Policy") != "default-src 'none'; sandbox" {
			t.Fatalf("SVG: %d %v", img.Code, img.Header())
		}
	})
	t.Run("图片地址", func(t *testing.T) {
		h, _ := panelHandler(t, []Panel{mk("https://img.example.com/a.jpg?x=1&y=2", 20)})
		w := panelGet(h, "/", "203.0.113.9:1")
		m := bgSrcRe.FindStringSubmatch(w.Body.String())
		if m == nil || m[1] != "https://img.example.com/a.jpg?x=1&amp;y=2" || !strings.Contains(w.Body.String(), `referrerpolicy="no-referrer"`) {
			t.Fatalf("应直接引用图片地址: %v", m)
		}
		// 没有内联图片时背景路径不存在
		if img := panelGet(h, panelBgPath, "203.0.113.9:1"); img.Code != 404 {
			t.Fatalf("没有内联背景应 404: %d", img.Code)
		}
	})
	t.Run("没有背景", func(t *testing.T) {
		h, _ := panelHandler(t, []Panel{mk("", 0)})
		body := panelGet(h, "/", "203.0.113.9:1").Body.String()
		if strings.Contains(body, `class="bg has"`) || !strings.Contains(body, ":root{--bgdim:0.00;--bgblur:0px}") {
			t.Fatal("没有背景时不应输出背景图片与遮罩")
		}
		if img := panelGet(h, panelBgPath, "203.0.113.9:1"); img.Code != 404 {
			t.Fatalf("没有背景应 404: %d", img.Code)
		}
	})
	t.Run("面板停用", func(t *testing.T) {
		p := mk(pngIcon, 0)
		p.Enable = false
		h, _ := panelHandler(t, []Panel{p})
		if img := panelGet(h, panelBgPath, "203.0.113.9:1"); img.Code != 404 {
			t.Fatalf("停用面板的背景应 404: %d", img.Code)
		}
	})
	t.Run("预览内联", func(t *testing.T) {
		cp := &compiledPanel{Panel: mk(pngIcon, 30)}
		r := httptest.NewRequest("GET", "http://x/", nil)
		html, _ := webpage.RenderPanel(r, cp.view(redirectRequest{r: r}, false, false, true, true))
		if !strings.Contains(string(html), `<img src="`+pngIcon+`"`) {
			t.Fatal("预览应直接内联图片")
		}
	})
}

// 站点的认证对背景图片同样生效：整体需要登录的面板，未登录取不到图片。
func TestPanelBackgroundHonorsAuth(t *testing.T) {
	l := Listener{Sites: []Site{{Enable: true, Key: "must", Domains: []string{"must.test"}, Type: TypePanel, Panel: "p1", WebAuth: true, Users: []string{"alice:pw"}}}}
	addr, r := startRunner(t, l, nil, nil)
	r.auth.panels.set([]Panel{{Key: "p1", Name: "家里", Enable: true, Background: pngIcon, Links: []PanelLink{{Name: "NAS", URL: "https://nas.example.com"}}}})
	resp, body := get(t, noRedirect(), addr, "must.test", panelBgPath, map[string]string{"Accept": "image/*", "User-Agent": "curl"})
	if resp.StatusCode == 200 || strings.Contains(resp.Header.Get("Content-Type"), "image/") || len(body) > 0 && body[0] == 0x89 {
		t.Fatalf("未登录不应取到背景图片: %d", resp.StatusCode)
	}
	cookie := loginCookie(t, addr, "must.test", "alice", "pw")
	resp, _ = get(t, noRedirect(), addr, "must.test", panelBgPath, map[string]string{"Cookie": cookie})
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("登录后应取到背景图片: %d", resp.StatusCode)
	}
}
