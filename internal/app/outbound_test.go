package app

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/outbound"
	"github.com/wangwanjie/gately/internal/proxyx"
)

// 全局设置“出站请求禁止访问内网”保存后立即生效并持久化；关闭后恢复放行。
func TestRestrictOutboundSetting(t *testing.T) {
	_, srv, c := loginApp(t)
	t.Cleanup(func() { outbound.SetRestricted(false) })
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer local.Close()

	if code, body := putSettings(t, c, srv.URL, func(s *settingsView) { s.RestrictOutbound = true }); code != 200 {
		t.Fatalf("保存失败 %d %s", code, body)
	}
	if !outbound.Restricted() {
		t.Fatal("保存后开关应立即生效")
	}
	// 经 proxyx 构造的客户端（Webhook、通知、IP 列表订阅使用）无需重建即受限制
	hc := proxyx.Client(proxyx.Direct, 3*time.Second, false)
	if _, err := hc.Get(local.URL); !errors.Is(err, outbound.ErrBlocked) {
		t.Fatalf("开启后回环地址应被拒绝，实际: %v", err)
	}
	_, body := doJSON(t, c, "GET", srv.URL+"/api/system/settings", nil)
	if !strings.Contains(string(body), `"restrictOutbound":true`) {
		t.Fatalf("设置未持久化: %s", body)
	}

	if code, body := putSettings(t, c, srv.URL, func(s *settingsView) { s.RestrictOutbound = false }); code != 200 {
		t.Fatalf("保存失败 %d %s", code, body)
	}
	if outbound.Restricted() {
		t.Fatal("关闭后应恢复放行")
	}
	resp, err := hc.Get(local.URL)
	if err != nil {
		t.Fatalf("关闭后应放行: %v", err)
	}
	resp.Body.Close()
}

// 默认保持现有行为：新实例不限制出站地址。
func TestRestrictOutboundDefaultOff(t *testing.T) {
	outbound.SetRestricted(false)
	a := newDocApp(t)
	if a.settingsSnapshot().RestrictOutbound || outbound.Restricted() {
		t.Fatal("出站限制默认应关闭")
	}
}
