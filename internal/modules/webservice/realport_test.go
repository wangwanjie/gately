package webservice

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// TestParsePort 只接受 1–65535 的纯数字端口。
func TestParsePort(t *testing.T) {
	for in, want := range map[string]uint16{
		"1": 1, "443": 443, " 65535 ": 65535, "0080": 80,
		"": 0, "0": 0, "65536": 0, "99999": 0, "123456": 0, "-1": 0, "+80": 0, "8 0": 0, "80a": 0, "0x50": 0, "１２": 0,
	} {
		if got := parsePort(in); got != want {
			t.Errorf("parsePort(%q) = %d，应为 %d", in, got, want)
		}
	}
}

// TestNormalizeRealPort 未启用真实 IP 时清空端口头，名称非法时报错。
func TestNormalizeRealPort(t *testing.T) {
	if h, err := normalizeRealPort(nil, "X-Real-Port"); err != nil || h != "" {
		t.Fatalf("未配置可信代理时应清空: %q %v", h, err)
	}
	if h, err := normalizeRealPort([]string{"198.51.100.0/24"}, " X-Real-Port "); err != nil || h != "X-Real-Port" {
		t.Fatalf("应去掉首尾空白: %q %v", h, err)
	}
	if h, err := normalizeRealPort([]string{"198.51.100.0/24"}, ""); err != nil || h != "" {
		t.Fatalf("留空表示代理不提供端口: %q %v", h, err)
	}
	for _, bad := range []string{"Bad Header", "X-中文", "X:Port", "X-Port\r\nX-Evil: 1"} {
		if _, err := normalizeRealPort([]string{"198.51.100.0/24"}, bad); err == nil {
			t.Errorf("%q 应校验失败", bad)
		}
	}
}

// TestRealIPPortApply 从可信代理的请求头取得访客地址时，端口只来自配置的端口头（取最右一项），取不到记为 0；
// 不可信对端的端口头一律不采信，地址与端口保持原样。
func TestRealIPPortApply(t *testing.T) {
	src := &realIPSource{}
	src.set([]string{"198.51.100.0/24"}, []string{"X-Forwarded-For"}, "X-Real-Port")
	apply := func(peer string, hdr map[string][]string) string {
		r := httptest.NewRequest("GET", "http://app.test/", nil)
		r.RemoteAddr = peer
		for k, v := range hdr {
			r.Header[k] = v
		}
		return src.apply(r).RemoteAddr
	}
	cases := []struct {
		name, peer string
		hdr        map[string][]string
		want       string
	}{
		{"代理提供端口", "198.51.100.7:1234", map[string][]string{"X-Forwarded-For": {"203.0.113.9"}, "X-Real-Port": {"50123"}}, "203.0.113.9:50123"},
		{"代理未提供端口", "198.51.100.7:1234", map[string][]string{"X-Forwarded-For": {"203.0.113.9"}}, "203.0.113.9:0"},
		{"多项取最右", "198.51.100.7:1234", map[string][]string{"X-Forwarded-For": {"203.0.113.9"}, "X-Real-Port": {"1111, 2222"}}, "203.0.113.9:2222"},
		{"多行取最后一行", "198.51.100.7:1234", map[string][]string{"X-Forwarded-For": {"203.0.113.9"}, "X-Real-Port": {"1111", "3333"}}, "203.0.113.9:3333"},
		{"无效端口", "198.51.100.7:1234", map[string][]string{"X-Forwarded-For": {"203.0.113.9"}, "X-Real-Port": {"70000"}}, "203.0.113.9:0"},
		{"最右项无效不回退左侧", "198.51.100.7:1234", map[string][]string{"X-Forwarded-For": {"203.0.113.9"}, "X-Real-Port": {"1111, abc"}}, "203.0.113.9:0"},
		{"IPv6 访客", "198.51.100.7:1234", map[string][]string{"X-Forwarded-For": {"2001:db8::9"}, "X-Real-Port": {"443"}}, "[2001:db8::9]:443"},
		// 可信代理没有给出访客地址时仍以对端为客户端，对端端口就是真实端口，端口头不采信
		{"无地址头", "198.51.100.7:1234", map[string][]string{"X-Real-Port": {"9"}}, "198.51.100.7:1234"},
		{"不可信对端", "203.0.113.5:4444", map[string][]string{"X-Forwarded-For": {"1.2.3.4"}, "X-Real-Port": {"9"}}, "203.0.113.5:4444"},
	}
	for _, c := range cases {
		if got := apply(c.peer, c.hdr); got != c.want {
			t.Errorf("%s: RemoteAddr = %s，应为 %s", c.name, got, c.want)
		}
	}
	// 没有配置端口头时，访客自带的同名头不被采信
	src.set([]string{"198.51.100.0/24"}, []string{"X-Forwarded-For"}, "")
	if got := apply("198.51.100.7:1234", map[string][]string{"X-Forwarded-For": {"203.0.113.9"}, "X-Real-Port": {"50123"}}); got != "203.0.113.9:0" {
		t.Errorf("未配置端口头时端口应未知: %s", got)
	}
	// 手工编辑配置填入非法端口头名时按未配置处理
	src.set([]string{"198.51.100.0/24"}, []string{"X-Forwarded-For"}, "Bad Header")
	if p := src.p.Load(); p == nil || p.portHeader != "" {
		t.Errorf("非法端口头名应被忽略: %+v", p)
	}
}

// TestPutSettingsRealPortHeader 保存全局设置时校验真实端口请求头，未配置可信代理时清空，配置后热生效。
func TestPutSettingsRealPortHeader(t *testing.T) {
	st, _ := config.Open(t.TempDir())
	m := New(nil, nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	put := func(body string) int {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/settings", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		m.handlePutSettings(rec, r)
		return rec.Code
	}
	if code := put(`{"enable":true,"realIPFrom":["198.51.100.0/24"],"realPortHeader":"Bad Header"}`); code != http.StatusBadRequest {
		t.Fatalf("非法端口头名应拒绝保存: %d", code)
	}
	if code := put(`{"enable":true,"realPortHeader":"X-Real-Port"}`); code != 200 || m.cfg.Settings.RealPortHeader != "" {
		t.Fatalf("未配置可信代理时应清空端口头: %d %q", code, m.cfg.Settings.RealPortHeader)
	}
	if code := put(`{"enable":true,"realIPFrom":["198.51.100.0/24"],"realPortHeader":" X-Real-Port "}`); code != 200 || m.cfg.Settings.RealPortHeader != "X-Real-Port" {
		t.Fatalf("应保存端口头: %d %q", code, m.cfg.Settings.RealPortHeader)
	}
	if p := m.realIP.p.Load(); p == nil || p.portHeader != "X-Real-Port" {
		t.Fatalf("保存后应立即生效: %+v", p)
	}
}
