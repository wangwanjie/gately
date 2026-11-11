package webservice

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// portEchoBackend 返回后端收到的源端口头：X-Real-Port|X-Forwarded-For-Port|X_Real_Port 变体数。
func portEchoBackend(t *testing.T) string {
	t.Helper()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		variants := 0
		for k := range r.Header {
			if strings.EqualFold(k, "X_Real_Port") || strings.EqualFold(k, "X_Forwarded_For_Port") {
				variants++
			}
		}
		_, _ = io.WriteString(w, r.Header.Get("X-Real-Ip")+"|"+strings.Join(r.Header.Values("X-Real-Port"), ",")+"|"+
			strings.Join(r.Header.Values("X-Forwarded-For-Port"), ",")+"|"+strconv.Itoa(variants))
	}))
	t.Cleanup(b.Close)
	return b.URL
}

// sendDirect 直接连接并发送请求，返回本端源端口与响应正文。
func sendDirect(t *testing.T, addr string, req *http.Request) (int, string) {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	req.Header.Set("Connection", "close")
	if err := req.Write(c); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(c), req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return c.LocalAddr().(*net.TCPAddr).Port, string(b)
}

// TestClientPortHeadersEndToEnd 端到端（#83）：开启转发头时后端收到访客源端口（直连为 TCP 源端口，经 PROXY 协议为

// PROXY 头中的源端口，经可信代理为端口头的值）；访客伪造的源端口头及其下划线变体一律删除；端口未知或未开启转发头时不带。
func TestClientPortHeadersEndToEnd(t *testing.T) {
	backend := portEchoBackend(t)
	sites := []Site{
		{Enable: true, Key: "p", Domains: []string{"app.test"}, Type: TypeProxy, Targets: []string{backend}, ForwardedHeaders: true},
		{Enable: true, Key: "n", Domains: []string{"plain.test"}, Type: TypeProxy, Targets: []string{backend}},
	}
	addr, r := startRunner(t, Listener{AcceptProxyProtocol: true, Sites: sites}, nil, []string{"127.0.0.1"})
	r.realIP = &realIPSource{}
	r.realIP.set([]string{"198.51.100.0/24"}, []string{"X-Forwarded-For"}, "X-Real-Port")

	req := func(host string, hdr map[string][]string) *http.Request {
		q, _ := http.NewRequest("GET", "http://"+host+"/", nil)
		for k, v := range hdr {
			q.Header[k] = v
		}
		return q
	}
	forged := map[string][]string{"X-Real-Port": {"1"}, "X-Forwarded-For-Port": {"2"}, "X_Real_Port": {"3"}, "X_forwarded_for_port": {"4"}}

	// 1. 直连（不带 PROXY 头的回环连接）：后端收到本端 TCP 源端口，伪造的头与变体被删除
	port, body := sendDirect(t, addr, req("app.test", forged))
	if want := "127.0.0.1|" + strconv.Itoa(port) + "|" + strconv.Itoa(port) + "|0"; body != want {
		t.Fatalf("直连应得到 TCP 源端口: %q，应为 %q", body, want)
	}
	// 2. 经 PROXY 协议转来：取 PROXY 头中的源端口（writeProxyHeader 固定为 4321）
	if _, body := sendViaProxyBody(t, addr, "203.0.113.5", req("app.test", forged)); body != "203.0.113.5|4321|4321|0" {
		t.Fatalf("应得到 PROXY 头中的源端口: %q", body)
	}
	// 3. 经可信 CDN（PROXY 源地址在可信代理中），CDN 写入 X-Real-Port：取它的值
	if _, body := sendViaProxyBody(t, addr, "198.51.100.7", req("app.test", map[string][]string{"X-Forwarded-For": {"203.0.113.9"}, "X-Real-Port": {"50123"}})); body != "203.0.113.9|50123|50123|0" {
		t.Fatalf("应采信可信代理给出的端口: %q", body)
	}
	// 4. 经可信 CDN 但没有端口头：端口未知，不带端口头，也不能把 CDN 连接的端口 4321 当成访客端口
	if _, body := sendViaProxyBody(t, addr, "198.51.100.7", req("app.test", map[string][]string{"X-Forwarded-For": {"203.0.113.9"}, "X-Forwarded-For-Port": {"2"}})); body != "203.0.113.9|||0" {
		t.Fatalf("端口未知时不应带端口头: %q", body)
	}
	// 5. 未开启转发头：不写端口头，访客自带的也删除
	if _, body := sendViaProxyBody(t, addr, "203.0.113.5", req("plain.test", forged)); body != "|||0" {
		t.Fatalf("未开启转发头时不应带端口头: %q", body)
	}
}

// TestClientPortForwardAuth 转发认证的子请求带上访客源端口，访客伪造的值被替换。
func TestClientPortForwardAuth(t *testing.T) {
	got := make(chan string, 1)
	authz := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("X-Real-Port") + "|" + r.Header.Get("X-Forwarded-For-Port") + "|" + r.Header.Get("X_Real_Port")
		w.WriteHeader(http.StatusOK)
	}))
	defer authz.Close()
	l := Listener{AcceptProxyProtocol: true, Sites: []Site{{
		Enable: true, Domains: []string{"fa.test"}, Type: TypeText, Text: "ok",
		ForwardAuth: ForwardAuth{URL: authz.URL},
	}}}
	addr, _ := startRunner(t, l, nil, []string{"127.0.0.1"})
	q, _ := http.NewRequest("GET", "http://fa.test/", nil)
	q.Header["X-Real-Port"] = []string{"1"}
	q.Header["X_Real_Port"] = []string{"3"}
	if code, _ := sendViaProxyBody(t, addr, "203.0.113.5", q); code != http.StatusOK {
		t.Fatalf("转发认证应通过: %d", code)
	}
	if v := <-got; v != "4321|4321|" {
		t.Fatalf("认证服务应收到真实源端口: %q", v)
	}
}

// TestIdentityHeaderRejectsPortHeaders 源端口头由 Gately 写入，不能用作身份信息头。
func TestIdentityHeaderRejectsPortHeaders(t *testing.T) {
	for _, h := range []string{"X-Real-Port", "x-forwarded-for-port"} {
		if !forbiddenUserHeaders[http.CanonicalHeaderKey(h)] {
			t.Errorf("%s 应在禁止列表中", h)
		}
	}
}
