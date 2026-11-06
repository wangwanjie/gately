package webservice

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// 后端超时：0 为默认值（建连与 TLS 握手 10 秒、等待响应头不限），超出范围或为负数时保存失败。
func TestBackendTimeoutsValidate(t *testing.T) {
	var zero BackendTimeouts
	if zero.connect() != 10*time.Second || zero.tlsHandshake() != 10*time.Second || zero.responseHeader() != 0 {
		t.Fatalf("默认值不符: %v %v %v", zero.connect(), zero.tlsHandshake(), zero.responseHeader())
	}
	set := BackendTimeouts{Connect: 3, TLSHandshake: 5, ResponseHeader: 120}
	if set.connect() != 3*time.Second || set.tlsHandshake() != 5*time.Second || set.responseHeader() != 120*time.Second {
		t.Fatalf("设置值不符: %+v", set)
	}
	ok := []BackendTimeouts{{}, {Connect: 1, TLSHandshake: 1, ResponseHeader: 1}, {Connect: 300, TLSHandshake: 300, ResponseHeader: 86400}}
	for _, v := range ok {
		s := Site{Domains: []string{"a.test"}, Targets: []string{"http://127.0.0.1:1"}, Timeouts: v}
		if err := s.validate(false); err != nil {
			t.Errorf("%+v 应可保存: %v", v, err)
		}
		if s.Timeouts != v {
			t.Errorf("保存时不应改写超时: %+v -> %+v", v, s.Timeouts)
		}
	}
	bad := []BackendTimeouts{{Connect: -1}, {Connect: 301}, {TLSHandshake: -1}, {TLSHandshake: 301}, {ResponseHeader: -1}, {ResponseHeader: 86401}}
	for _, v := range bad {
		s := Site{Domains: []string{"a.test"}, Targets: []string{"http://127.0.0.1:1"}, Timeouts: v}
		if err := s.validate(false); err == nil {
			t.Errorf("%+v 应保存失败", v)
		}
	}
}

// 连接池使用站点的超时设置。
func TestTransportUsesTimeouts(t *testing.T) {
	up := &upstream{}
	up.url, _ = url.Parse("https://10.0.0.1:8443")
	tr := newTransport(Site{}, up)
	if tr.TLSHandshakeTimeout != 10*time.Second || tr.ResponseHeaderTimeout != 0 {
		t.Fatalf("默认: %v %v", tr.TLSHandshakeTimeout, tr.ResponseHeaderTimeout)
	}
	tr = newTransport(Site{Timeouts: BackendTimeouts{TLSHandshake: 7, ResponseHeader: 45}}, up)
	if tr.TLSHandshakeTimeout != 7*time.Second || tr.ResponseHeaderTimeout != 45*time.Second {
		t.Fatalf("设置: %v %v", tr.TLSHandshakeTimeout, tr.ResponseHeaderTimeout)
	}
}

// proxyThrough 用站点处理器转发一个请求，返回状态码、响应体与耗时。
func proxyThrough(t *testing.T, s Site, accept string) (int, string, time.Duration) {
	t.Helper()
	s.Enable, s.Key, s.Domains, s.Type = true, "s", []string{"a.test"}, TypeProxy
	if err := s.validate(false); err != nil {
		t.Fatal(err)
	}
	h, err := newSiteHandler(s, testLogs().Logger("t"), newAuthenticator("x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://a.test/x", nil)
	req.Header.Set("Accept", accept)
	rec := httptest.NewRecorder()
	start := time.Now()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String(), time.Since(start)
}

// 等待响应头超时：后端迟迟不回响应头时按设置返回 504，未设置时照常等到后端响应。
func TestResponseHeaderTimeout(t *testing.T) {
	release := make(chan struct{})
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-time.After(1500 * time.Millisecond):
		}
		_, _ = w.Write([]byte("late"))
	}))
	t.Cleanup(func() {
		close(release)
		b.Close()
	})
	code, body, took := proxyThrough(t, Site{Targets: []string{b.URL}, Timeouts: BackendTimeouts{ResponseHeader: 1}}, "text/html")
	if code != http.StatusGatewayTimeout || !strings.Contains(body, "后端服务响应超时") || took > 1400*time.Millisecond {
		t.Fatalf("应在 1 秒左右返回 504: %d %v %s", code, took, body)
	}
	code, body, _ = proxyThrough(t, Site{Targets: []string{b.URL}}, "text/html")
	if code != 200 || body != "late" {
		t.Fatalf("未设置时应等到后端响应: %d %s", code, body)
	}
	// 响应头及时到达后，之后的响应体传输不受此超时限制
	slowBody := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		time.Sleep(1500 * time.Millisecond)
		_, _ = w.Write([]byte("tail"))
	}))
	t.Cleanup(slowBody.Close)
	code, body, _ = proxyThrough(t, Site{Targets: []string{slowBody.URL}, Timeouts: BackendTimeouts{ResponseHeader: 1}}, "text/html")
	if code != 200 || body != "tail" {
		t.Fatalf("响应体传输不应受等待响应头超时限制: %d %s", code, body)
	}
}

// TLS 握手超时：后端接受连接却不完成握手时按设置返回 504，而不是一直挂起。
func TestTLSHandshakeTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var conns []net.Conn
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			// 只接受连接，不做 TLS 握手
			conns = append(conns, c)
		}
	}()
	t.Cleanup(func() {
		ln.Close()
		<-done
		for _, c := range conns {
			c.Close()
		}
	})
	code, body, took := proxyThrough(t, Site{Targets: []string{"https://" + ln.Addr().String()}, InsecureSkipVerify: true, Timeouts: BackendTimeouts{TLSHandshake: 1}}, "application/json")
	if code != http.StatusGatewayTimeout || took > 1500*time.Millisecond || strings.Contains(body, "<html") {
		t.Fatalf("应在 1 秒左右返回 504: %d %v %s", code, took, body)
	}
}

// 建连超时：连不上的地址按设置的秒数放弃。测试网络直接拒绝路由时无法验证超时，跳过。
func TestConnectTimeout(t *testing.T) {
	// 192.0.2.0/24 为文档保留地址，通常不会有响应
	code, _, took := proxyThrough(t, Site{Targets: []string{"http://192.0.2.1:81"}, Timeouts: BackendTimeouts{Connect: 1}}, "text/html")
	if code != http.StatusGatewayTimeout {
		t.Skipf("当前网络对保留地址直接返回错误（%d，%v），无法验证建连超时", code, took)
	}
	if took < 900*time.Millisecond || took > 2500*time.Millisecond {
		t.Fatalf("应在约 1 秒后放弃: %v", took)
	}
}

// 连接被拒绝不是超时，仍返回 502。
func TestRefusedStillBadGateway(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	dead := "http://" + ln.Addr().String()
	ln.Close()
	code, body, _ := proxyThrough(t, Site{Targets: []string{dead}}, "text/html")
	if code != http.StatusBadGateway || !strings.Contains(body, "无法连接到后端服务") {
		t.Fatalf("连接被拒绝应为 502: %d %s", code, body)
	}
}
