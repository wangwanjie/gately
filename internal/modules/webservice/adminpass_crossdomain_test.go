package webservice

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// 跨父域的后台登录免认证（后台入口握手）已移除：单点登录域之外的站点即使带着之前握手写下的仅主机凭据也不放行，
// 原握手地址不再有特殊处理，按站点原有的认证方式响应，不会跳转到其他域名；单点登录域内的站点照常免认证。
func TestAdminPassCrossDomainRemoved(t *testing.T) {
	env := newAdminEnv(t)
	addr, _ := startAdminPass(t, "example.test", "http://127.0.0.1:1")
	admin := env.login(t, "1.1.1.1")
	resp, _ := get(t, noRedirect(), addr, "admin.example.test", "/", hdr("Cookie", admin, "X-Forwarded-Proto", "https"))
	c := bridgeCookie(resp)
	if c == nil {
		t.Fatal("单点登录域内应照常签发桥接凭据")
	}
	bridge := adminBridgeCookie + "=" + c.Value

	// 域外站点：有效凭据（模拟升级前握手写在站点自己域名上的仅主机 Cookie）也要求认证
	for _, path := range []string{"/", authPath + "/admin-start?next=/x", authPath + "/admin-login?ticket=0123456789abcdef0123456789abcdef"} {
		h := hdr("Cookie", bridge)
		for k, v := range browserHdr {
			h[k] = v
		}
		resp, _ := get(t, noRedirect(), addr, "outside.test", path, h)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("域外站点 %s 不应认凭据: %d", path, resp.StatusCode)
		}
		if loc := resp.Header.Get("Location"); loc != "" {
			t.Fatalf("域外站点 %s 不应跳转: %q", path, loc)
		}
	}

	// 网页认证站点上的原握手地址：未带凭据时与其他路径一样跳到站点自己的登录页
	resp, _ = get(t, noRedirect(), addr, "web.example.test", authPath+"/admin-start?next=/x", browserHdr)
	if loc := resp.Header.Get("Location"); resp.StatusCode == http.StatusFound && !strings.HasPrefix(loc, authPath+"/") {
		t.Fatalf("原握手地址不应跳到站点以外: %d %q", resp.StatusCode, loc)
	}
	for _, ck := range resp.Cookies() {
		if ck.Name == "gately_admin_hs" {
			t.Fatal("不应再写握手 Cookie")
		}
	}

	// 后台所在站点上的原签发票据地址：带后台登录态也不签发票据、不跳转
	h := hdr("Cookie", admin, "X-Forwarded-Proto", "https")
	for k, v := range browserHdr {
		h[k] = v
	}
	resp, body := get(t, noRedirect(), addr, "admin.example.test", authPath+"/admin-ticket?state=x&s=1", h)
	if loc := resp.Header.Get("Location"); strings.Contains(loc, "ticket=") || strings.Contains(body, "admin-ticket") {
		t.Fatalf("不应再签发握手票据: %d %q", resp.StatusCode, loc)
	}

	// 单点登录域内的站点照常免认证
	if resp, body := get(t, noRedirect(), addr, "basic.example.test", "/", hdr("Cookie", bridge)); resp.StatusCode != http.StatusOK || body != "basic-ok" {
		t.Fatalf("单点登录域内应照常免认证: %d %q", resp.StatusCode, body)
	}
}

// 升级前保存的 adminPassEntry：配置文件里的旧字段照常加载并在下次保存时去掉；
// 升级前打开的页面或脚本仍带着它保存全局设置时接受并忽略，不因未知字段拒绝。
func TestAdminPassEntryIgnored(t *testing.T) {
	dir := t.TempDir()
	st, err := config.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.WriteRaw(ID, []byte(`{"settings":{"enable":true,"adminPassEntry":"https://admin.example.test"},"listeners":[]}`)); err != nil {
		t.Fatal(err)
	}
	m := New(nil, nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatalf("带旧字段的配置应能加载: %v", err)
	}
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/settings", strings.NewReader(`{"enable":true,"adminPassEntry":"https://admin.example.test"}`))
	r.Header.Set("Content-Type", "application/json")
	m.handlePutSettings(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("带旧字段的保存应被接受: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "adminPassEntry") {
		t.Fatalf("响应不应再含旧字段: %s", rec.Body.String())
	}
	matches, _ := filepath.Glob(filepath.Join(dir, ID+"*"))
	if len(matches) == 0 {
		t.Fatal("应写出配置文件")
	}
	for _, p := range matches {
		data, _ := os.ReadFile(p)
		if strings.Contains(string(data), "adminPassEntry") {
			t.Fatalf("保存后的配置不应再含旧字段: %s", p)
		}
	}
}
