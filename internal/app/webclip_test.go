package app

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/portforward"
)

// 开启安全入口后，主屏幕图标请求不带 Cookie，凭页面里签过名的地址也能取到图标；没有签名或签名错误仍返回 404。
func TestHomeIconsBehindSafePath(t *testing.T) {
	web := fstest.MapFS{
		"index.html": {Data: []byte(`<!doctype html><html><head>` +
			`<link rel="icon" href="./favicon.svg" />` +
			`<link rel="manifest" href="./manifest.webmanifest" crossorigin="use-credentials" />` +
			`<link rel="apple-touch-icon" href="./apple-touch-icon.png" sizes="180x180" />` +
			`<script type="module" src="./assets/a.js"></script></head><body></body></html>`)},
		"assets/a.js":          {Data: []byte("console.log(1)")},
		"favicon.svg":          {Data: []byte("<svg/>")},
		"apple-touch-icon.png": {Data: []byte("PNG-180")},
		"icon-192.png":         {Data: []byte("PNG-192")},
		"manifest.webmanifest": {Data: []byte(`{"name":"Gately","icons":[{"src":"icon-192.png","sizes":"192x192"}]}`)},
	}
	a, err := New(Options{ConfigDir: t.TempDir(), Web: web, Modules: []module.Module{portforward.New()}})
	if err != nil {
		t.Fatal(err)
	}
	a.logger = log.New(io.Discard, "", 0)
	a.settingsMu.Lock()
	a.settings.SafePath = "/my-entry"
	a.settingsMu.Unlock()
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)

	get := func(c *http.Client, url string) (int, string, string) {
		t.Helper()
		resp, err := c.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, resp.Header.Get("Content-Type"), string(b)
	}
	noRedirect := func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	bare := &http.Client{CheckRedirect: noRedirect}

	// 未通过入口：图标与首页都返回 404
	for _, p := range []string{"/apple-touch-icon.png", "/apple-touch-icon.png?k=0000", "/"} {
		if code, _, _ := get(bare, srv.URL+p); code != 404 {
			t.Fatalf("%s 未通过安全入口应返回 404，实际 %d", p, code)
		}
	}

	// 通过入口后的首页：图标与 manifest 地址带签名，其他资源不带
	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar}
	code, _, index := get(browser, srv.URL+"/my-entry")
	if code != 200 {
		t.Fatalf("经安全入口打开首页失败 %d", code)
	}
	m := regexp.MustCompile(`href="/apple-touch-icon\.png\?k=([0-9a-f]+)"`).FindStringSubmatch(index)
	if m == nil || !strings.Contains(index, `href="/manifest.webmanifest?k=`+m[1]+`"`) || !strings.Contains(index, `href="/favicon.svg?k=`+m[1]+`"`) {
		t.Fatalf("首页图标地址未签名: %s", index)
	}
	if strings.Contains(index, "a.js?k=") {
		t.Fatalf("非图标资源不应签名: %s", index)
	}
	key := m[1]

	// 不带 Cookie 的请求凭签名取到图标与 manifest，manifest 中的图标同样带签名
	if code, _, body := get(bare, srv.URL+"/apple-touch-icon.png?k="+key); code != 200 || body != "PNG-180" {
		t.Fatalf("带签名应能取到 apple-touch-icon，实际 %d %q", code, body)
	}
	code, ct, body := get(bare, srv.URL+"/manifest.webmanifest?k="+key)
	if code != 200 || ct != "application/manifest+json" {
		t.Fatalf("带签名应能取到 manifest，实际 %d %s", code, ct)
	}
	var mf struct {
		Name  string `json:"name"`
		Icons []struct {
			Src string `json:"src"`
		} `json:"icons"`
	}
	if err := json.Unmarshal([]byte(body), &mf); err != nil || mf.Name != "Gately" || len(mf.Icons) != 1 || mf.Icons[0].Src != "icon-192.png?k="+key {
		t.Fatalf("manifest 图标未签名: %s", body)
	}
	if code, _, _ := get(bare, srv.URL+"/icon-192.png?k="+key); code != 200 {
		t.Fatalf("manifest 中的图标应能取到，实际 %d", code)
	}

	// 签名只对图标文件有效，不能用来打开首页或其他资源
	for _, p := range []string{"/?k=" + key, "/assets/a.js?k=" + key, "/api/system/info?k=" + key} {
		if code, _, _ := get(bare, srv.URL+p); code != 404 {
			t.Fatalf("%s 签名不应放行非图标文件，实际 %d", p, code)
		}
	}

	// 更换安全入口后旧签名失效
	a.settingsMu.Lock()
	a.settings.SafePath = "/other-entry"
	a.settingsMu.Unlock()
	if code, _, _ := get(bare, srv.URL+"/apple-touch-icon.png?k="+key); code != 404 {
		t.Fatalf("更换安全入口后旧签名应失效，实际 %d", code)
	}
}
