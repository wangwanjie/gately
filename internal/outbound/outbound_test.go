package outbound

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"
)

// withRestricted 在测试期间设置全局开关，结束后恢复。
func withRestricted(t *testing.T, on bool) {
	t.Helper()
	old := Restricted()
	SetRestricted(on)
	t.Cleanup(func() { SetRestricted(old) })
}

func TestIsBlocked(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "::1", "10.0.0.1", "172.16.5.4", "192.168.1.1", "169.254.169.254", "fe80::1",
		"fd00::1", "fc00::1", "0.0.0.0", "::", "100.64.0.1", "224.0.0.1", "ff02::1", "198.18.0.1",
		"::ffff:127.0.0.1", "::ffff:10.1.2.3", "64:ff9b::a9fe:a9fe",
		// 6to4 内嵌内网地址、本地 NAT64、Teredo、站点本地、IPv4 兼容、丢弃与文档前缀
		"2002:7f00:1::1", "2002:a9fe:a9fe::", "64:ff9b:1::a00:1", "2001:0:4136:e378::1", "fec0::1", "::7f00:1",
		"100::1", "2001:db8::1",
	}
	for _, s := range blocked {
		if !IsBlocked(netip.MustParseAddr(s)) {
			t.Errorf("%s 应被拒绝", s)
		}
	}
	allowed := []string{"8.8.8.8", "1.1.1.1", "203.0.113.1", "2606:4700:4700::1111", "::ffff:8.8.8.8", "64:ff9b::808:808", "2002:808:808::1"}
	for _, s := range allowed {
		if IsBlocked(netip.MustParseAddr(s)) {
			t.Errorf("%s 不应被拒绝", s)
		}
	}
}

func okServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") }))
	t.Cleanup(srv.Close)
	return srv
}

// 开关开启时回环地址被拒绝，包括用域名（解析后为回环）访问；关闭时放行。
func TestLoopbackBlockedWhenRestricted(t *testing.T) {
	srv := okServer(t)
	client := NewClient(5 * time.Second)

	withRestricted(t, false)
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("开关关闭时应放行: %v", err)
	}
	resp.Body.Close()

	SetRestricted(true)
	// 已缓存的 Transport 可能持有空闲连接，关闭后再验证“新连接”被拒绝
	client.CloseIdleConnections()
	if _, err := client.Get(srv.URL); !errors.Is(err, ErrBlocked) {
		t.Fatalf("开关开启时回环地址应被拒绝，实际: %v", err)
	}
	// 域名解析到回环地址同样被拒绝（校验的是解析后的真实 IP）
	u, _ := url.Parse(srv.URL)
	if _, err := client.Get("http://localhost:" + u.Port()); !errors.Is(err, ErrBlocked) {
		t.Fatalf("解析到回环的域名应被拒绝，实际: %v", err)
	}
	// 元数据地址
	if _, err := client.Get("http://169.254.169.254/latest/meta-data/"); !errors.Is(err, ErrBlocked) {
		t.Fatalf("元数据地址应被拒绝，实际: %v", err)
	}
}

// 重定向到内网地址在拨号时被拒绝：第一跳建立连接后开启限制，随后的跳转必须被拒。
func TestRedirectToInternalBlocked(t *testing.T) {
	target := okServer(t)
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 第一跳已经放行；此后开关开启，模拟“公网地址重定向到内网”
		SetRestricted(true)
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer first.Close()

	withRestricted(t, false)
	client := NewClient(5 * time.Second)
	_, err := client.Get(first.URL)
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("重定向到内网应被拒绝，实际: %v", err)
	}

	// 开关关闭时重定向正常跟随
	SetRestricted(false)
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redir.Close()
	resp, err := client.Get(redir.URL)
	if err != nil {
		t.Fatalf("开关关闭时重定向应放行: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "ok" {
		t.Fatalf("响应异常: %q", body)
	}
}

// 经代理访问：代理自身（回环）不受限，目标为内网地址时被拒绝，目标为公网地址时放行。
func TestProxyTargetChecked(t *testing.T) {
	var hits int
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = io.WriteString(w, "via proxy")
	}))
	defer proxy.Close()
	pu, _ := url.Parse(proxy.URL)
	client := &http.Client{Transport: ProxyTransport(pu), Timeout: 5 * time.Second}

	withRestricted(t, true)
	resp, err := client.Get("http://203.0.113.1/")
	if err != nil {
		t.Fatalf("公网目标经回环代理应放行: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "via proxy") {
		t.Fatalf("响应异常: %q", body)
	}
	before := hits
	for _, target := range []string{"http://169.254.169.254/", "http://10.1.2.3/", "http://localhost/"} {
		if _, err := client.Get(target); !errors.Is(err, ErrBlocked) {
			t.Fatalf("%s 经代理访问也应被拒绝，实际: %v", target, err)
		}
	}
	if hits != before {
		t.Fatal("被拒绝的请求不应到达代理")
	}
}

func TestWrapKeepsCustomRoundTripper(t *testing.T) {
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("x") })
	c := Wrap(&http.Client{Transport: rt, Timeout: time.Second})
	if _, ok := c.Transport.(roundTripFunc); !ok || c.Timeout != time.Second {
		t.Fatal("自定义 RoundTripper 与超时应保持不变")
	}
	if _, ok := Wrap(nil).Transport.(*http.Transport); !ok {
		t.Fatal("nil 应得到带限制的 Transport")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
