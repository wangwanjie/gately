package webservice

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wangwanjie/gately/internal/outbound"
)

// testPNG 生成边长为 n 的 PNG 图片，用像素颜色区分不同的图标。
func testPNG(t *testing.T, n int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	for x := 0; x < n; x++ {
		for y := 0; y < n; y++ {
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

var testICO = append([]byte{0, 0, 1, 0, 1, 0}, bytes.Repeat([]byte{7}, 40)...)

// faviconSite 是测试网站：routes 为路径到 (Content-Type, 内容) 的映射，其余路径返回 404；hits 统计各路径的访问次数。
type faviconSite struct {
	srv  *httptest.Server
	hits map[string]*atomic.Int32
}

func newFaviconSite(t *testing.T, routes map[string][2]string) *faviconSite {
	t.Helper()
	s := &faviconSite{hits: map[string]*atomic.Int32{}}
	for p := range routes {
		s.hits[p] = &atomic.Int32{}
	}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rt, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		s.hits[r.URL.Path].Add(1)
		if strings.HasPrefix(rt[0], "redirect:") {
			http.Redirect(w, r, strings.TrimPrefix(rt[0], "redirect:"), http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", rt[0])
		_, _ = w.Write([]byte(rt[1]))
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func decodeDataURL(t *testing.T, d string) (string, []byte) {
	t.Helper()
	mt, b64, ok := strings.Cut(strings.TrimPrefix(d, "data:"), ";base64,")
	if !ok {
		t.Fatalf("不是 data URL: %.60s", d)
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatal(err)
	}
	return mt, raw
}

// 网页声明了多个图标时按清晰度挑选：apple-touch-icon（180）优先于 32 像素的 favicon 与 manifest 中 128 像素的图标；
// 单色的 manifest 图标被跳过；相对地址按 <base> 解析。
func TestFetchFaviconPicksBest(t *testing.T) {
	small := string(testPNG(t, 32, color.RGBA{255, 0, 0, 255}))
	apple := string(testPNG(t, 180, color.RGBA{0, 255, 0, 255}))
	mf := string(testPNG(t, 128, color.RGBA{0, 0, 255, 255}))
	site := newFaviconSite(t, map[string][2]string{
		"/": {"text/html; charset=utf-8", `<!doctype html><html><head><base href="/static/">
<link rel="icon" href="fav-32.png" sizes="32x32" type="image/png">
<link rel="apple-touch-icon" href="apple.png">
<link rel="manifest" href="/site.webmanifest">
<link rel="stylesheet" href="x.css"></head><body><link rel="icon" href="/body-icon.svg"></body></html>`},
		"/static/fav-32.png": {"image/png", small},
		"/static/apple.png":  {"image/png", apple},
		"/site.webmanifest":  {"application/manifest+json", `{"icons":[{"src":"/mono.svg","sizes":"any","purpose":"monochrome"},{"src":"/m128.png","sizes":"128x128","type":"image/png"}]}`},
		"/m128.png":          {"image/png", mf},
		"/mono.svg":          {"image/svg+xml", `<svg xmlns="http://www.w3.org/2000/svg"/>`},
		"/body-icon.svg":     {"image/svg+xml", `<svg xmlns="http://www.w3.org/2000/svg"/>`},
	})
	res, err := fetchFavicon(context.Background(), faviconClient(), site.srv.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	mt, raw := decodeDataURL(t, res.Icon)
	if mt != "image/png" || string(raw) != apple || res.Source != site.srv.URL+"/static/apple.png" || res.From != site.srv.URL+"/" {
		t.Fatalf("应选 apple-touch-icon: %s %s", mt, res.Source)
	}
	if site.hits["/mono.svg"].Load() != 0 || site.hits["/body-icon.svg"].Load() != 0 {
		t.Error("单色图标与正文中的声明不应使用")
	}
}

// 矢量图优先；候选图标不是图片（如被重定向到登录页）时换下一个。
func TestFetchFaviconSVGAndFallback(t *testing.T) {
	site := newFaviconSite(t, map[string][2]string{
		"/":         {"text/html", `<link rel="icon" href="/a.svg"><link rel="icon" href="/b.png" sizes="64x64">`},
		"/a.svg":    {"text/html", `<html>login</html>`},
		"/b.png":    {"image/png", string(testPNG(t, 64, color.White))},
		"/c/":       {"text/html", `<link rel="icon" type="image/svg+xml" href="/c/i.svg">`},
		"/c/i.svg":  {"text/plain", "\xef\xbb\xbf<?xml version=\"1.0\"?>\n<!-- logo -->\n<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 1 1\"/>"},
		"/redir":    {"redirect:/c/", ""},
		"/logo.png": {"application/octet-stream", string(testPNG(t, 16, color.Black))},
	})
	c := faviconClient()
	res, err := fetchFavicon(context.Background(), c, site.srv.URL+"/")
	if err != nil || !strings.HasPrefix(res.Icon, "data:image/png;base64,") || site.hits["/a.svg"].Load() != 1 {
		t.Fatalf("SVG 地址不是图片时应换用 PNG: %v %+v", err, res)
	}
	// 跳转后相对地址按最终网页解析；内容按格式识别为 SVG，不看响应头
	res, err = fetchFavicon(context.Background(), c, site.srv.URL+"/redir")
	if err != nil || !strings.HasPrefix(res.Icon, "data:image/svg+xml;base64,") {
		t.Fatalf("跳转后的 SVG: %v %+v", err, res)
	}
	// 地址本身就是图片
	res, err = fetchFavicon(context.Background(), c, site.srv.URL+"/logo.png")
	if err != nil || !strings.HasPrefix(res.Icon, "data:image/png;base64,") || res.Source != site.srv.URL+"/logo.png" {
		t.Fatalf("图片地址: %v %+v", err, res)
	}
}

// 网页没有声明图标时用 /favicon.ico；都没有时报错，错误中不带网页内容。
func TestFetchFaviconICOFallbackAndErrors(t *testing.T) {
	site := newFaviconSite(t, map[string][2]string{
		"/app/":        {"text/html", `<html><head><title>secret-title</title></head></html>`},
		"/favicon.ico": {"image/x-icon", string(testICO)},
	})
	res, err := fetchFavicon(context.Background(), faviconClient(), site.srv.URL+"/app/")
	if err != nil {
		t.Fatal(err)
	}
	if mt, raw := decodeDataURL(t, res.Icon); mt != "image/x-icon" || !bytes.Equal(raw, testICO) {
		t.Fatalf("应使用 /favicon.ico: %s", mt)
	}
	none := newFaviconSite(t, map[string][2]string{"/": {"text/html", `<title>secret-title</title>`}})
	_, err = fetchFavicon(context.Background(), faviconClient(), none.srv.URL+"/")
	if err == nil || strings.Contains(err.Error(), "secret-title") {
		t.Fatalf("没有图标应报错且不泄露网页内容: %v", err)
	}
	// 非图片内容（脚本、HTML）不能冒充图标
	for _, body := range []string{`<script>alert(1)</script>`, `<html><svg></svg></html>`, `GIF`} {
		if _, ok := faviconImage([]byte(body), "image/png"); ok {
			t.Errorf("%q 不应识别为图片", body)
		}
	}
	if _, ok := faviconImage([]byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), "image/svg+xml"); !ok {
		t.Error("应识别 SVG")
	}
}

// 读取量与跳转次数有上限：超过 1 MB 的图片、跳转过多都失败。
func TestFetchFaviconLimits(t *testing.T) {
	big := append(testPNG(t, 8, color.White), bytes.Repeat([]byte{0}, faviconImageMax)...)
	site := newFaviconSite(t, map[string][2]string{
		"/":        {"text/html", `<link rel="icon" href="/big.png">`},
		"/big.png": {"image/png", string(big)},
		"/r1":      {"redirect:/r2", ""},
		"/r2":      {"redirect:/r3", ""},
		"/r3":      {"redirect:/r4", ""},
		"/r4":      {"redirect:/r5", ""},
		"/r5":      {"redirect:/r6", ""},
		"/r6":      {"redirect:/", ""},
	})
	if _, err := fetchFavicon(context.Background(), faviconClient(), site.srv.URL+"/"); err == nil {
		t.Error("超过上限的图片应失败")
	}
	if _, err := fetchFavicon(context.Background(), faviconClient(), site.srv.URL+"/r1"); err == nil || !strings.Contains(err.Error(), "跳转次数过多") {
		t.Errorf("跳转过多应失败: %v", err)
	}
}

// 地址整理：去重、跳过含变量与非网页的地址，全部被跳过时提示原因；带密码的地址拒绝；最多 10 个。
func TestFaviconTargets(t *testing.T) {
	got, err := faviconTargets([]string{" https://a.example.com ", "", "https://a.example.com", "http://{host}:8080", "smb://nas/share", "http://192.168.1.2:5000"})
	if err != nil || strings.Join(got, ",") != "https://a.example.com,http://192.168.1.2:5000" {
		t.Fatalf("整理结果: %v %v", got, err)
	}
	if _, err := faviconTargets([]string{"http://{STUN_x_ADDR}/", "ssh://nas"}); err == nil || !strings.Contains(err.Error(), "含变量") {
		t.Errorf("全部被跳过: %v", err)
	}
	if _, err := faviconTargets(nil); err == nil || !strings.Contains(err.Error(), "请先填写") {
		t.Errorf("没有地址: %v", err)
	}
	if _, err := faviconTargets([]string{"https://u:p@a.example.com"}); err == nil {
		t.Error("带密码的地址应拒绝")
	}
	var many []string
	for i := 0; i < 15; i++ {
		many = append(many, "https://h"+string(rune('a'+i))+".example.com")
	}
	if got, _ := faviconTargets(many); len(got) != faviconMaxURLs {
		t.Errorf("最多 %d 个地址: %d", faviconMaxURLs, len(got))
	}
}

// 接口：按顺序尝试，前一个地址失败时用后一个；全部失败返回 502。
func TestPanelFaviconAPI(t *testing.T) {
	site := newFaviconSite(t, map[string][2]string{
		"/":            {"text/html", `<link rel="icon" href="/i.png">`},
		"/i.png":       {"image/png", string(testPNG(t, 48, color.White))},
		"/favicon.ico": {"text/html", "nope"},
	})
	_, do := newCopyModule(t)
	code, out := do("POST", "/api/webservice/panels/favicon", PanelFaviconRequest{URLs: []string{site.srv.URL + "/missing", site.srv.URL + "/"}})
	if code != 200 || !strings.HasPrefix(out["icon"].(string), "data:image/png;base64,") || out["from"] != site.srv.URL+"/" {
		t.Fatalf("获取图标: %d %v", code, out)
	}
	if code, _ = do("POST", "/api/webservice/panels/favicon", PanelFaviconRequest{URLs: []string{site.srv.URL + "/missing"}}); code != http.StatusBadGateway {
		t.Fatalf("全部失败应返回 502: %d", code)
	}
	if code, _ = do("POST", "/api/webservice/panels/favicon", PanelFaviconRequest{URLs: []string{"http://{host}/"}}); code != http.StatusBadRequest {
		t.Fatalf("含变量应返回 400: %d", code)
	}
}

func TestIconScore(t *testing.T) {
	cases := []struct {
		sizes, typ, path string
		def, want        int
	}{
		{"", "", "/a.png", 32, 32},
		{"16x16 48x48", "", "/a.ico", 32, 48},
		{"any", "", "/a.png", 32, 400},
		{"", "image/svg+xml", "/a", 32, 400},
		{"", "", "/A.SVG", 32, 400},
		{"512x512", "", "/a.png", 192, 240},
		{"4096x4096", "", "/a.png", 192, 156},
		{"bogus", "", "/a.png", 180, 180},
	}
	for _, c := range cases {
		if got := iconScore(c.sizes, c.typ, c.path, c.def); got != c.want {
			t.Errorf("iconScore(%q,%q,%q)=%d，应为 %d", c.sizes, c.typ, c.path, got, c.want)
		}
	}
}

// 开启设置中的“出站请求限制”后，获取图标同样不能访问回环与内网地址。
func TestFetchFaviconRespectsOutboundRestriction(t *testing.T) {
	site := newFaviconSite(t, map[string][2]string{"/favicon.ico": {"image/x-icon", string(testICO)}})
	outbound.SetRestricted(true)
	t.Cleanup(func() { outbound.SetRestricted(false) })
	if _, err := fetchFavicon(context.Background(), faviconClient(), site.srv.URL+"/"); err == nil {
		t.Fatal("限制出站访问内网时应失败")
	}
	if site.hits["/favicon.ico"].Load() != 0 {
		t.Fatal("不应访问到内网地址")
	}
}
