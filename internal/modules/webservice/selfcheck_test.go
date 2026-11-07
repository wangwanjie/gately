package webservice

import (
	"context"
	"crypto/tls"
	"errors"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/module"
)

// nameCerts 按名称表模拟证书模块：键为精确域名或 *.根域名。
type nameCerts map[string]bool

func (f nameCerts) GetCertificate(h *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if f[h.ServerName] {
		return &tls.Certificate{}, nil
	}
	if i := strings.IndexByte(h.ServerName, '.'); i > 0 && f["*."+h.ServerName[i+1:]] {
		return &tls.Certificate{}, nil
	}
	return nil, errors.New("no cert")
}

func TestSelfCheckCertCoverage(t *testing.T) {
	m := New(nameCerts{"a.example.com": true, "*.example.org": true}, nil)
	m.cfg.Settings.Enable = true
	m.cfg.Listeners = []Listener{
		{Enable: true, TLS: true, Port: 443, Name: "https", Sites: []Site{
			{Enable: true, Name: "ok", Domains: []string{"a.example.com", "x.example.org", "*.example.org", "app-*.example.org", "*"}},
			{Enable: true, Name: "bad", Domains: []string{"dashboard.x-ui.example.com", "*.example.com", "app-*.example.com", "a.example.com"}},
			{Enable: false, Name: "off", Domains: []string{"nocert.example.net"}},
		}},
		{Enable: true, TLS: false, Port: 80, Sites: []Site{{Enable: true, Domains: []string{"plain.example.net"}}}},
		{Enable: false, TLS: true, Port: 8443, Sites: []Site{{Enable: true, Domains: []string{"stopped.example.net"}}}},
	}
	got := m.SelfCheck(context.Background(), module.CheckEnv{})
	if len(got) != 1 || got[0].ID != "web.certMissing" || got[0].Params["site"] != "https / bad" || got[0].Params["domains"] != "dashboard.x-ui.example.com, *.example.com, app-*.example.com" {
		t.Fatalf("应只报告缺少证书的站点与域名: %+v", got)
	}
	m.cfg.Listeners[0].Sites[1].Enable = false
	got = m.SelfCheck(context.Background(), module.CheckEnv{})
	if len(got) != 1 || got[0].ID != "web.certOk" || got[0].Params["count"] != "1" {
		t.Fatalf("全部覆盖时应报告正常: %+v", got)
	}
}
