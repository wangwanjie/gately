package webservice

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// TestProxyRedirectKeepsHTTPS 验证访客以 HTTPS 访问时，后端按 HTTP 拼出的同站绝对跳转改回 https，其他跳转原样保留。
func TestProxyRedirectKeepsHTTPS(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", r.URL.Query().Get("to"))
		w.WriteHeader(http.StatusFound)
	}))
	defer backend.Close()
	site := Site{Enable: true, Domains: []string{"secure.test"}, Targets: []string{backend.URL}}
	tlsAddr, _ := startRunner(t, Listener{TLS: true, Sites: []Site{site}}, fakeCerts{selfSigned(t, "secure.test")}, nil)
	plainAddr, _ := startRunner(t, Listener{Sites: []Site{site}}, nil, nil)
	c := &http.Client{
		Transport:     &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, ServerName: "secure.test"}},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer c.CloseIdleConnections()
	cases := []struct {
		name, base, host, to, want string
	}{
		{"同站 http 改为 https", "https://" + tlsAddr, "secure.test", "http://secure.test/login?code=1", "https://secure.test/login?code=1"},
		{"去掉 HTTP 默认端口", "https://" + tlsAddr, "secure.test", "http://secure.test:80/a", "https://secure.test/a"},
		{"主机名大小写不同", "https://" + tlsAddr, "secure.test", "http://Secure.Test/a", "https://Secure.Test/a"},
		{"访客带端口", "https://" + tlsAddr, "secure.test:8443", "http://secure.test:8443/a", "https://secure.test:8443/a"},
		{"其他站点不改", "https://" + tlsAddr, "secure.test", "http://other.test/a", "http://other.test/a"},
		{"其他端口不改", "https://" + tlsAddr, "secure.test", "http://secure.test:8080/a", "http://secure.test:8080/a"},
		{"已是 https 不改", "https://" + tlsAddr, "secure.test", "https://secure.test/a", "https://secure.test/a"},
		{"相对地址不改", "https://" + tlsAddr, "secure.test", "/login", "/login"},
		{"HTTP 访问不改", "http://" + plainAddr, "secure.test", "http://secure.test/login", "http://secure.test/login"},
	}
	for _, cs := range cases {
		req, _ := http.NewRequest("GET", cs.base+"/?to="+url.QueryEscape(cs.to), nil)
		req.Host = cs.host
		resp, err := c.Do(req)
		if err != nil {
			t.Fatalf("%s: %v", cs.name, err)
		}
		resp.Body.Close()
		if got := resp.Header.Get("Location"); resp.StatusCode != http.StatusFound || got != cs.want {
			t.Errorf("%s: 状态 %d，Location %q，期望 %q", cs.name, resp.StatusCode, got, cs.want)
		}
	}
}
