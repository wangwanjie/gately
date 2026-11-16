package webservice

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// leafCerts 按名称表模拟证书模块，返回带到期时间的证书：键为精确域名或 *.根域名。
type leafCerts map[string]*tls.Certificate

func (f leafCerts) GetCertificate(h *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if c, ok := f[h.ServerName]; ok {
		return c, nil
	}
	if i := strings.IndexByte(h.ServerName, '.'); i > 0 {
		if c, ok := f["*."+h.ServerName[i+1:]]; ok {
			return c, nil
		}
	}
	return nil, errors.New("no cert")
}

// 监听列表接口为 HTTPS 站点的每个域名给出匹配到的证书与到期时间，缺证书的域名没有证书名称。
func TestListenerViewSiteCerts(t *testing.T) {
	a := selfSigned(t, "a.example.com")
	m := New(leafCerts{"a.example.com": a}, nil)
	l := Listener{Key: "l1", Enable: true, TLS: true, Port: 443, Sites: []Site{
		{Key: "s1", Enable: true, Domains: []string{"a.example.com", "b.example.com", "*"}},
		{Key: "s2", Enable: true, Domains: []string{"*"}},
	}}
	v := m.viewLocked(&l).(ListenerView)
	got := v.SiteCerts["s1"]
	if len(got) != 2 {
		t.Fatalf("s1 应有两个可检查的域名: %+v", v.SiteCerts)
	}
	if got[0].Domain != "a.example.com" || got[0].Name != "a.example.com" || time.Until(got[0].NotAfter) <= 0 {
		t.Fatalf("a.example.com 应匹配到证书: %+v", got[0])
	}
	if got[1].Domain != "b.example.com" || got[1].Name != "" || !got[1].NotAfter.IsZero() {
		t.Fatalf("b.example.com 应缺证书: %+v", got[1])
	}
	if _, ok := v.SiteCerts["s2"]; ok {
		t.Fatal("只有兜底域名的站点不应出现")
	}
	b, _ := json.Marshal(got[1])
	if string(b) != `{"domain":"b.example.com"}` {
		t.Fatalf("缺证书的 JSON = %s", b)
	}
	// 非 HTTPS 监听不输出
	l.TLS = false
	if v := m.viewLocked(&l).(ListenerView); v.SiteCerts != nil {
		t.Fatalf("HTTP 监听不应有证书信息: %+v", v.SiteCerts)
	}
}
