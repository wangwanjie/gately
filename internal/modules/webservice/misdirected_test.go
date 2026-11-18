package webservice

import (
	"crypto/tls"
	"io"
	"net/http"
	"testing"
)

// 浏览器连接合并：Host 与 SNI 不同、本监听没有 Host 的站点而 SNI 是本监听的站点域名时应答 421，让浏览器新建连接重试；
// Host 与 SNI 相同、Host 有站点、HTTP/1.1、SNI 为空或 SNI 不是本机站点域名（反向代理把 SNI 设成后端地址）时照常路由（含交给默认站点）。
func TestMisdirectedRequest(t *testing.T) {
	l := Listener{TLS: true, MinTLS: "1.2",
		Sites: []Site{
			{Enable: true, Domains: []string{"a.test"}, Type: TypeText, Text: "site-a", StatusCode: 200},
			{Enable: true, Domains: []string{"b.test"}, Type: TypeText, Text: "site-b", StatusCode: 200},
		},
		Default: Site{Enable: true, Type: TypeText, Text: "default", StatusCode: 200},
	}
	// 模拟通配证书：任意 SNI 都给同一张证书，与线上 *.thevast.uk 的情形一致
	addr, _ := startRunner(t, l, anyCert{selfSigned(t, "a.test")}, nil)

	do := func(sni, host string, h2 bool) (int, string) {
		t.Helper()
		tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, ServerName: sni}, ForceAttemptHTTP2: h2}
		if !h2 {
			tr.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
		}
		defer tr.CloseIdleConnections()
		req, _ := http.NewRequest("GET", "https://"+addr+"/", nil)
		req.Host = host
		resp, err := tr.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	cases := []struct {
		name, sni, host string
		h2              bool
		code            int
		body            string
	}{
		{"同名", "a.test", "a.test", true, 200, "site-a"},
		{"复用连接访问本机另一站点", "a.test", "b.test", true, 200, "site-b"},
		{"复用连接访问别处的域名", "a.test", "other.test", true, http.StatusMisdirectedRequest, ""},
		{"Host 带端口与尾点", "a.test", "other.test.:443", true, http.StatusMisdirectedRequest, ""},
		{"SNI 未知但与 Host 相同走默认站点", "other.test", "other.test", true, 200, "default"},
		{"HTTP/1.1 不判断", "a.test", "other.test", false, 200, "default"},
		// 另一台 Gately（或 Caddy 等）以 HTTP/2 反代过来：SNI 为后端主机名，Host 为访客域名，交给默认站点
		{"HTTP/2 反向代理 SNI 为后端地址", "home.lan", "public.example", true, 200, "default"},
		{"HTTP/2 反向代理 Host 有站点", "home.lan", "b.test", true, 200, "site-b"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, body := do(c.sni, c.host, c.h2)
			if code != c.code {
				t.Fatalf("状态码 %d，期望 %d（%q）", code, c.code, body)
			}
			if c.body != "" && body != c.body {
				t.Fatalf("响应 %q，期望 %q", body, c.body)
			}
		})
	}
}

// anyCert 对任意 SNI 返回同一张证书。
type anyCert struct{ cert *tls.Certificate }

func (a anyCert) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return a.cert, nil
}
