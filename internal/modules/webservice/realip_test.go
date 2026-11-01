package webservice

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	proxyproto "github.com/pires/go-proxyproto"
)

func TestNormalizeRealIP(t *testing.T) {
	for _, bad := range [][]string{{"0.0.0.0/0"}, {"::/0"}, {"198.51.100.0/24", "0.0.0.0/0"}, {"not-an-ip"}} {
		if _, _, err := normalizeRealIP(bad, nil); err == nil {
			t.Fatalf("%v 应被拒绝", bad)
		}
	}
	if _, _, err := normalizeRealIP([]string{"198.51.100.0/24"}, []string{"Bad Header"}); err == nil {
		t.Fatal("非法头名应被拒绝")
	}
	from, headers, err := normalizeRealIP([]string{" 198.51.100.0/24 ", ""}, nil)
	if err != nil || len(from) != 1 || len(headers) != 1 || headers[0] != "X-Forwarded-For" {
		t.Fatalf("未指定头名时应默认 X-Forwarded-For: %v %v %v", from, headers, err)
	}
	_, headers, _ = normalizeRealIP([]string{"198.51.100.1"}, []string{"cf-connecting-ip", "CF-Connecting-IP", "x-real-ip"})
	if strings.Join(headers, ",") != "cf-connecting-ip,x-real-ip" {
		t.Fatalf("头名应不区分大小写去重: %v", headers)
	}
	from, headers, _ = normalizeRealIP(nil, []string{"X-Real-IP"})
	if from != nil || headers != nil {
		t.Fatal("未配置可信代理时应清空头名")
	}
}

func TestRealIPResolve(t *testing.T) {
	p := &realIPPolicy{from: []netip.Prefix{netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("10.0.0.0/8")},
		headers: []string{"Cf-Connecting-Ip", "X-Forwarded-For"}}
	for _, tc := range []struct {
		name string
		hdr  map[string][]string
		want string
	}{
		{"CF 头优先", map[string][]string{"Cf-Connecting-Ip": {"203.0.113.9"}, "X-Forwarded-For": {"203.0.113.1"}}, "203.0.113.9"},
		{"没有 CF 头时读 XFF", map[string][]string{"X-Forwarded-For": {"203.0.113.1"}}, "203.0.113.1"},
		{"XFF 左侧伪造被忽略", map[string][]string{"X-Forwarded-For": {"1.1.1.1, 203.0.113.9"}}, "203.0.113.9"},
		{"XFF 跳过可信的多级代理", map[string][]string{"X-Forwarded-For": {"1.1.1.1, 203.0.113.9, 10.1.2.3", "198.51.100.7"}}, "203.0.113.9"},
		{"全是可信代理时取最左", map[string][]string{"X-Forwarded-For": {"10.0.0.5, 198.51.100.7"}}, "10.0.0.5"},
		{"带端口与 IPv6", map[string][]string{"X-Forwarded-For": {"[2001:db8::1]:443"}}, "2001:db8::1"},
		{"CF 头无效时改看 XFF", map[string][]string{"Cf-Connecting-Ip": {"garbage"}, "X-Forwarded-For": {"203.0.113.1"}}, "203.0.113.1"},
		{"XFF 右侧无法解析时不采信", map[string][]string{"X-Forwarded-For": {"203.0.113.1, garbage"}}, ""},
		{"没有头", nil, ""},
	} {
		h := http.Header{}
		for k, vs := range tc.hdr {
			for _, v := range vs {
				h.Add(k, v)
			}
		}
		a, ok := p.resolve(h)
		got := ""
		if ok {
			got = a.String()
		}
		if got != tc.want {
			t.Fatalf("%s: 得到 %q，应为 %q", tc.name, got, tc.want)
		}
	}
}

// echoBackend 返回后端收到的 X-Real-Ip，即 Gately 认定的客户端地址。
func echoBackend(t *testing.T) string {
	t.Helper()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.Header.Get("X-Real-Ip")+"|"+r.Header.Get("X-Forwarded-For"))
	}))
	t.Cleanup(b.Close)
	return b.URL
}

// sendRaw 直接（不带 PROXY 头）发送请求，读回响应正文。
func sendRaw(t *testing.T, addr string, req *http.Request) (int, string) {
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
	return resp.StatusCode, string(b)
}

// sendViaProxyBody 与 sendViaProxy 相同，额外读回响应正文。
func sendViaProxyBody(t *testing.T, addr, src string, req *http.Request) (int, string) {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	writeProxyHeader(t, c, src)
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
	return resp.StatusCode, string(b)
}

// 端到端：只在原始对端（PROXY 处理后的地址）是可信代理时采信真实 IP 头，改写后的地址同时用于转发给后端与 IP 名单。
func TestRealIPEndToEnd(t *testing.T) {
	backend := echoBackend(t)
	sites := []Site{
		{Enable: true, Key: "p", Domains: []string{"app.test"}, Type: TypeProxy, Targets: []string{backend}, ForwardedHeaders: true},
		{Enable: true, Key: "b", Domains: []string{"deny.test"}, Type: TypeText, Text: "ok", IPMode: IPModeBlacklist, IPList: []string{"203.0.113.66"}},
	}
	// 127.0.0.1 作为可信 PROXY 来源（模拟协议分流经回环转来），可信代理只有 CDN 段 198.51.100.0/24
	addr, r := startRunner(t, Listener{AcceptProxyProtocol: true, Sites: sites}, nil, []string{"127.0.0.1"})
	r.realIP = &realIPSource{}
	r.realIP.set([]string{"198.51.100.0/24"}, []string{"Cf-Connecting-Ip", "X-Forwarded-For"})

	req := func(host string, hdr map[string]string) *http.Request {
		q, _ := http.NewRequest("GET", "http://"+host+"/", nil)
		for k, v := range hdr {
			q.Header.Set(k, v)
		}
		return q
	}
	ipOf := func(body string) string { return strings.SplitN(body, "|", 2)[0] }

	// 1. 经 CDN 访问：取 CF-Connecting-IP，后端收到的 X-Forwarded-For 也只有访客地址
	if _, body := sendViaProxyBody(t, addr, "198.51.100.7", req("app.test", map[string]string{"CF-Connecting-IP": "203.0.113.9"})); body != "203.0.113.9|203.0.113.9" {
		t.Fatalf("经可信代理应得到访客地址: %q", body)
	}
	// 2. 直接访问（PROXY 头中是访客自己的地址）：伪造的 CF-Connecting-IP 被忽略
	if _, body := sendViaProxyBody(t, addr, "203.0.113.5", req("app.test", map[string]string{"CF-Connecting-IP": "1.2.3.4"})); ipOf(body) != "203.0.113.5" {
		t.Fatalf("直接访问时伪造的头不应被采信: %q", body)
	}
	// 3. 回环陷阱：TCP 对端是回环，但没有 PROXY 头时原始对端就是 127.0.0.1，不在可信代理中，头不被采信
	if _, body := sendRaw(t, addr, req("app.test", map[string]string{"CF-Connecting-IP": "1.2.3.4"})); ipOf(body) != "127.0.0.1" {
		t.Fatalf("回环直连不应采信请求头: %q", body)
	}
	// 4. 经 CDN 时 XFF 左侧伪造，取右侧由 CDN 追加的值
	if _, body := sendViaProxyBody(t, addr, "198.51.100.7", req("app.test", map[string]string{"X-Forwarded-For": "1.2.3.4, 203.0.113.9"})); ipOf(body) != "203.0.113.9" {
		t.Fatalf("应取 XFF 右侧的访客地址: %q", body)
	}
	// IP 黑名单按真实地址生效：经 CDN 来的被封访客被拒，同一 CDN 的其他访客正常
	if code, _ := sendViaProxyBody(t, addr, "198.51.100.7", req("deny.test", map[string]string{"CF-Connecting-IP": "203.0.113.66"})); code != http.StatusForbidden {
		t.Fatalf("黑名单应按真实地址拦截: %d", code)
	}
	if code, _ := sendViaProxyBody(t, addr, "198.51.100.7", req("deny.test", map[string]string{"CF-Connecting-IP": "203.0.113.67"})); code != http.StatusOK {
		t.Fatalf("其他访客应放行: %d", code)
	}
	// 未启用时保持原行为
	r.realIP.set(nil, nil)
	if _, body := sendViaProxyBody(t, addr, "198.51.100.7", req("app.test", map[string]string{"CF-Connecting-IP": "203.0.113.9"})); ipOf(body) != "198.51.100.7" {
		t.Fatalf("未启用时应使用对端地址: %q", body)
	}
}

// 可信代理以明文转发并标明 X-Forwarded-Proto: https 时按 HTTPS 处理，即使客户端地址已改写为公网访客。
func TestRealIPForwardedProto(t *testing.T) {
	src := &realIPSource{}
	src.set([]string{"198.51.100.0/24"}, []string{"X-Forwarded-For"})
	r := httptest.NewRequest("GET", "http://app.test/", nil)
	r.RemoteAddr = "198.51.100.7:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.9")
	r.Header.Set("X-Forwarded-Proto", "https")
	r = src.apply(r)
	if r.RemoteAddr != "203.0.113.9:1234" || !requestIsHTTPS(r) {
		t.Fatalf("可信代理的 X-Forwarded-Proto 应被采信: %s %v", r.RemoteAddr, requestIsHTTPS(r))
	}
}

// writeProxyHeader 写入声明访客地址为 src 的 PROXY v2 头。
func writeProxyHeader(t *testing.T, c net.Conn, src string) {
	t.Helper()
	h := proxyproto.HeaderProxyFromAddrs(2, &net.TCPAddr{IP: net.ParseIP(src), Port: 4321}, c.RemoteAddr())
	if _, err := h.WriteTo(c); err != nil {
		t.Fatal(err)
	}
}
