package webservice

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"

	proxyproto "github.com/pires/go-proxyproto"
)

// httpPortListener 返回开启 HTTPS 并带额外 HTTP 端口的监听配置，端口在 startRunner 中分配。
func httpPortListener(t *testing.T, mode string) Listener {
	l := plainHTTPListener("")
	l.HTTPPort = freePort(t)
	l.HTTPPortMode = mode
	return l
}

// rawGet 经明文连接发送一个请求并返回完整响应文本，可指定任意 Host 与附加请求头。
func rawGet(t *testing.T, addr, method, host, extra string) string {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	fmt.Fprintf(c, "%s /a/b?x=1 HTTP/1.1\r\nHost: %s\r\n%sContent-Length: 0\r\nConnection: close\r\n\r\n", method, host, extra)
	b, _ := io.ReadAll(c)
	return string(b)
}

func TestHTTPPortValidate(t *testing.T) {
	cases := []struct {
		name     string
		l        Listener
		wantErr  bool
		wantMode string
	}{
		{"HTTPS 默认跳转", Listener{Port: 443, TLS: true, HTTPPort: 80}, false, PlainHTTPRedirect},
		{"HTTPS 直接提供", Listener{Port: 443, TLS: true, HTTPPort: 80, HTTPPortMode: PlainHTTPServe}, false, PlainHTTPServe},
		{"HTTP 主端口只能直接提供", Listener{Port: 8080, HTTPPort: 80, HTTPPortMode: PlainHTTPRedirect}, false, PlainHTTPServe},
		{"未开启时清空处理方式", Listener{Port: 443, TLS: true, HTTPPortMode: PlainHTTPServe}, false, ""},
		{"与主端口相同", Listener{Port: 443, TLS: true, HTTPPort: 443}, true, ""},
		{"端口越界", Listener{Port: 443, TLS: true, HTTPPort: 70000}, true, ""},
		{"负数端口", Listener{Port: 443, TLS: true, HTTPPort: -1}, true, ""},
		{"未知处理方式", Listener{Port: 443, TLS: true, HTTPPort: 80, HTTPPortMode: "proxy"}, true, ""},
	}
	for _, tc := range cases {
		l := tc.l
		err := l.Validate()
		if (err != nil) != tc.wantErr {
			t.Fatalf("%s: err = %v", tc.name, err)
		}
		if err == nil && l.HTTPPortMode != tc.wantMode {
			t.Fatalf("%s: mode = %q, want %q", tc.name, l.HTTPPortMode, tc.wantMode)
		}
	}
}

// 跳转模式：额外 HTTP 端口的请求跳转到主端口的 https 地址，端口换成 HTTPS 端口；主端口照常提供 HTTPS，
// 主端口本身没有开启同端口明文 HTTP，仍然拒绝明文请求。
func TestHTTPPortRedirect(t *testing.T) {
	l := httpPortListener(t, PlainHTTPRedirect)
	addr, r := startRunner(t, l, fakeCerts{selfSigned(t, "secure.test")}, nil)
	httpAddr := "127.0.0.1:" + strconv.Itoa(l.HTTPPort)
	mainPort := r.cfg.Port

	out := rawGet(t, httpAddr, "GET", "secure.test", "")
	want := fmt.Sprintf("Location: https://secure.test:%d/a/b?x=1", mainPort)
	if !strings.HasPrefix(out, "HTTP/1.1 301") || !strings.Contains(out, want) {
		t.Fatalf("GET 跳转异常: %q", out)
	}
	// 访客 Host 带的是额外端口，跳转时换成 HTTPS 端口，不能沿用 80
	out = rawGet(t, httpAddr, "POST", "secure.test:80", "")
	if !strings.HasPrefix(out, "HTTP/1.1 308") || !strings.Contains(out, want) {
		t.Fatalf("POST 应使用 308 并换成 HTTPS 端口: %q", out)
	}
	resp, body := doReq(t, tlsClient(), "GET", "https://"+addr+"/")
	if body != "mixed ok" || resp.TLS == nil {
		t.Fatalf("HTTPS 主端口异常: %q", body)
	}
	if out := rawGet(t, addr, "GET", "secure.test", ""); strings.Contains(out, "mixed ok") {
		t.Fatalf("主端口未开启明文 HTTP，不应提供站点: %q", out)
	}
	if n := r.acts.redirect.total.Load(); n != 2 {
		t.Fatalf("跳转应计入监听级动作: %d", n)
	}
}

// 访问端口为 443 时跳转地址省略端口；IPv6 地址作为主机名时加方括号。
func TestHTTPPortRedirectAccessPort(t *testing.T) {
	l := httpPortListener(t, PlainHTTPRedirect)
	l.AccessPort = 443
	_, _ = startRunner(t, l, fakeCerts{selfSigned(t, "secure.test")}, nil)
	httpAddr := "127.0.0.1:" + strconv.Itoa(l.HTTPPort)
	if out := rawGet(t, httpAddr, "GET", "secure.test", ""); !strings.Contains(out, "Location: https://secure.test/a/b?x=1\r\n") {
		t.Fatalf("443 应省略端口: %q", out)
	}
	if out := rawGet(t, httpAddr, "GET", "[2001:db8::1]:80", ""); !strings.Contains(out, "Location: https://[2001:db8::1]/a/b?x=1\r\n") {
		t.Fatalf("IPv6 主机名跳转异常: %q", out)
	}
	if out := rawGet(t, httpAddr, "GET", "", ""); !strings.HasPrefix(out, "HTTP/1.1 400") {
		t.Fatalf("缺少 Host 应返回 400: %q", out)
	}
}

func TestHTTPPortRedirectHelper(t *testing.T) {
	cases := []struct{ host, want string }{
		{"a.test", "https://a.test:8443/p"},
		{"a.test:80", "https://a.test:8443/p"},
		{"A.Test.", "https://a.test:8443/p"},
		{"[::1]:80", "https://[::1]:8443/p"},
	}
	for _, tc := range cases {
		req, _ := http.NewRequest("GET", "http://x/p", nil)
		req.Host = tc.host
		w := &headerRecorder{h: http.Header{}}
		redirectToHTTPSPort(w, req, 8443)
		if got := w.h.Get("Location"); got != tc.want || w.code != http.StatusMovedPermanently {
			t.Fatalf("%s: %d %q", tc.host, w.code, got)
		}
	}
}

// headerRecorder 记录状态码与响应头。
type headerRecorder struct {
	h    http.Header
	code int
}

func (w *headerRecorder) Header() http.Header         { return w.h }
func (w *headerRecorder) Write(b []byte) (int, error) { return len(b), nil }
func (w *headerRecorder) WriteHeader(code int)        { w.code = code }

// 直接提供模式：额外端口以明文 HTTP 提供同一组站点。
func TestHTTPPortServe(t *testing.T) {
	l := httpPortListener(t, PlainHTTPServe)
	addr, r := startRunner(t, l, fakeCerts{selfSigned(t, "secure.test")}, nil)
	resp, body := doReq(t, tlsClient(), "GET", "http://127.0.0.1:"+strconv.Itoa(l.HTTPPort)+"/")
	if body != "mixed ok" || resp.TLS != nil || resp.StatusCode != 200 {
		t.Fatalf("额外端口应直接提供站点: %d %q", resp.StatusCode, body)
	}
	if resp, body := doReq(t, tlsClient(), "GET", "https://"+addr+"/"); body != "mixed ok" || resp.TLS == nil {
		t.Fatalf("HTTPS 主端口异常: %q", body)
	}
	if n := r.acts.redirect.total.Load(); n != 0 {
		t.Fatalf("直接提供时不应计跳转: %d", n)
	}
}

// 主端口是 HTTP 时额外端口也是同一组站点的 HTTP 入口。
func TestHTTPPortPlainListener(t *testing.T) {
	l := Listener{HTTPPort: freePort(t), Sites: []Site{{Enable: true, Domains: []string{"secure.test"}, Type: TypeText, Text: "plain ok"}}}
	addr, _ := startRunner(t, l, nil, nil)
	for _, a := range []string{addr, "127.0.0.1:" + strconv.Itoa(l.HTTPPort)} {
		if _, body := doReq(t, http.DefaultClient, "GET", "http://"+a+"/"); body != "plain ok" {
			t.Fatalf("%s: %q", a, body)
		}
	}
}

// 额外端口同样解析可信来源的 PROXY 头：经可信代理并声明 X-Forwarded-Proto: https 的请求不再跳转，否则照常跳转。
func TestHTTPPortProxyProtocol(t *testing.T) {
	l := httpPortListener(t, PlainHTTPRedirect)
	l.AcceptProxyProtocol = true
	_, _ = startRunner(t, l, fakeCerts{selfSigned(t, "secure.test")}, []string{"127.0.0.1"})
	send := func(extra string) string {
		c, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(l.HTTPPort))
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		h := proxyproto.HeaderProxyFromAddrs(2, &net.TCPAddr{IP: net.ParseIP("203.0.113.9"), Port: 1}, c.RemoteAddr())
		_, _ = h.WriteTo(c)
		fmt.Fprintf(c, "GET / HTTP/1.1\r\nHost: secure.test\r\n%sConnection: close\r\n\r\n", extra)
		b, _ := io.ReadAll(c)
		return string(b)
	}
	if out := send("X-Forwarded-Proto: https\r\n"); !strings.Contains(out, "mixed ok") {
		t.Fatalf("可信代理转来的 HTTPS 请求不应跳转: %q", out)
	}
	if out := send(""); !strings.HasPrefix(out, "HTTP/1.1 301") {
		t.Fatalf("经 PROXY 协议的明文请求应跳转: %q", out)
	}
}

// 停止监听后额外端口立即释放，可以马上重新监听。
func TestHTTPPortStopReleases(t *testing.T) {
	l := httpPortListener(t, PlainHTTPServe)
	_, r := startRunner(t, l, fakeCerts{selfSigned(t, "secure.test")}, nil)
	r.stop()
	ln, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(l.HTTPPort))
	if err != nil {
		t.Fatalf("停止后额外端口仍被占用: %v", err)
	}
	ln.Close()
}

// 额外端口被占用时主端口照常运行，监听状态显示原因，自动放行只放行主端口；
// 之后只改站点热更新时错误仍保留。
func TestHTTPPortBusyKeepsMain(t *testing.T) {
	busy, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	busyPort := busy.Addr().(*net.TCPAddr).Port
	mainPort := freePort(t)
	l := Listener{Key: "hp", Enable: true, Network: "tcp4", Port: mainPort, HTTPPort: busyPort, AutoFirewall: true,
		Sites: []Site{{Enable: true, Domains: []string{"a.test"}, Type: TypeText, Text: "ok"}}}
	m := newFirewallModule(t, l)
	m.mu.Lock()
	_, running := m.runners["hp"]
	errMsg := m.errors["hp"]
	m.mu.Unlock()
	if !running || !strings.Contains(errMsg, strconv.Itoa(busyPort)) {
		t.Fatalf("主端口应继续运行并显示额外端口的错误: running=%v err=%q", running, errMsg)
	}
	if s := firewallOf(t, m, "hp"); !strings.Contains(s, strconv.Itoa(mainPort)) || strings.Contains(s, strconv.Itoa(busyPort)) {
		t.Fatalf("只应放行主端口: %q", s)
	}
	m.mu.Lock()
	m.cfg.Listeners[0].Sites[0].Text = "changed"
	m.restartLocked("hp")
	errMsg = m.errors["hp"]
	m.mu.Unlock()
	if !strings.Contains(errMsg, strconv.Itoa(busyPort)) {
		t.Fatalf("热更新不应清掉额外端口的错误: %q", errMsg)
	}
}

// 额外端口正常时自动放行包含它。
func TestHTTPPortFirewall(t *testing.T) {
	mainPort, httpPort := freePort(t), freePort(t)
	l := Listener{Key: "hp", Enable: true, Network: "tcp4", Port: mainPort, HTTPPort: httpPort, AutoFirewall: true,
		Sites: []Site{{Enable: true, Domains: []string{"a.test"}, Type: TypeText, Text: "ok"}}}
	m := newFirewallModule(t, l)
	if s := firewallOf(t, m, "hp"); !strings.Contains(s, strconv.Itoa(mainPort)) || !strings.Contains(s, strconv.Itoa(httpPort)) {
		t.Fatalf("应同时放行两个端口: %q", s)
	}
}

// 修改额外端口或其处理方式要重启监听；只改站点或访问地址热更新，跳转端口随站点表更新。
func TestHTTPPortSameSocket(t *testing.T) {
	base := Listener{Port: 443, TLS: true, HTTPPort: 80, HTTPPortMode: PlainHTTPRedirect}
	sites := base
	sites.Sites = []Site{{Key: "x"}}
	if !sameSocket(base, sites) {
		t.Fatal("只改站点不应重启")
	}
	for name, mod := range map[string]func(*Listener){
		"端口":   func(l *Listener) { l.HTTPPort = 8080 },
		"处理方式": func(l *Listener) { l.HTTPPortMode = PlainHTTPServe },
	} {
		b := base
		mod(&b)
		if sameSocket(base, b) {
			t.Fatalf("改%s应重启监听", name)
		}
	}
	access := base
	access.AccessPort = 8443
	if !sameSocket(base, access) {
		t.Fatal("改访问端口不应重启监听")
	}
}

// 修改访问端口热更新后，额外 HTTP 端口跳转到新的端口，已建立的监听不中断。
func TestHTTPPortRedirectFollowsAccessPort(t *testing.T) {
	l := httpPortListener(t, PlainHTTPRedirect)
	_, r := startRunner(t, l, fakeCerts{selfSigned(t, "secure.test")}, nil)
	httpAddr := "127.0.0.1:" + strconv.Itoa(l.HTTPPort)
	if out := rawGet(t, httpAddr, "GET", "secure.test", ""); !strings.Contains(out, fmt.Sprintf("https://secure.test:%d/", r.cfg.Port)) {
		t.Fatalf("初始应跳转到监听端口: %q", out)
	}
	cfg := r.cfg
	cfg.AccessPort = 443
	if !sameSocket(r.cfg, cfg) {
		t.Fatal("改访问端口应走热更新")
	}
	if err := r.reload(cfg, nil); err != nil {
		t.Fatal(err)
	}
	if out := rawGet(t, httpAddr, "GET", "secure.test", ""); !strings.Contains(out, "Location: https://secure.test/a/b?x=1\r\n") {
		t.Fatalf("热更新后应跳转到访问端口: %q", out)
	}
}

// 拓扑：额外端口为跳转模式时监听下显示跳转动作，并带上端口信息。
func TestHTTPPortTopology(t *testing.T) {
	l := Listener{Key: "hp", Enable: true, Network: "tcp4", ListenIP: "127.0.0.1", Port: freePort(t), HTTPPort: freePort(t), TLS: true,
		Sites: []Site{{Enable: true, Domains: []string{"a.test"}, Type: TypeText, Text: "ok"}}}
	m := newFirewallModule(t, l)
	m.certs = fakeCerts{selfSigned(t, "a.test")}
	m.mu.Lock()
	m.restartLocked("hp")
	m.mu.Unlock()
	v := m.topology(time.Now().Unix(), "", netip.Addr{})
	if len(v.Listeners) != 1 {
		t.Fatalf("listeners = %d", len(v.Listeners))
	}
	tl := v.Listeners[0]
	if tl.HTTPPort != l.HTTPPort || tl.HTTPPortMode != PlainHTTPRedirect {
		t.Fatalf("拓扑缺少额外端口: %+v", tl)
	}
	found := false
	for _, a := range tl.Actions {
		if a.Kind == "redirect" {
			found = true
		}
	}
	if !found {
		t.Fatalf("应显示跳转动作: %+v", tl.Actions)
	}
}
