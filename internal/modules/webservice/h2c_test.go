package webservice

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/http2"
)

// h2cBackend 启动只接受明文 HTTP/2 的后端（与 grpc-go 服务端一致，HTTP/1.1 请求会被拒绝）。
// conns 统计后端接受过的 TCP 连接数。
func h2cBackend(t *testing.T, h http.Handler) (url string, conns *atomic.Int64) {
	t.Helper()
	var protos http.Protocols
	protos.SetUnencryptedHTTP2(true)
	conns = &atomic.Int64{}
	srv := httptest.NewUnstartedServer(h)
	srv.Config.Protocols = &protos
	srv.Config.ConnState = func(_ net.Conn, st http.ConnState) {
		if st == http.StateNew {
			conns.Add(1)
		}
	}
	srv.Start()
	t.Cleanup(srv.Close)
	return srv.URL, conns
}

// grpcLikeHandler 模拟 gRPC 双向流：读到一行回一行并立即刷新，结束时写出尾部 Grpc-Status。
func grpcLikeHandler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(200)
			return
		}
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("Trailer", "Grpc-Status")
		w.Header().Set("X-Backend-Proto", fmt.Sprint(r.ProtoMajor))
		w.Header().Set("X-Backend-TE", r.Header.Get("Te"))
		w.WriteHeader(200)
		rc := http.NewResponseController(w)
		// 立即发出响应头，客户端无需等第一条消息就能开始双向收发
		_ = rc.Flush()
		br := bufio.NewReader(r.Body)
		for {
			line, err := br.ReadString('\n')
			if line != "" {
				_, _ = io.WriteString(w, "echo:"+line)
				_ = rc.Flush()
			}
			if err != nil {
				break
			}
		}
		w.Header().Set("Grpc-Status", "0")
	})
}

func TestH2CBackendGRPCStreaming(t *testing.T) {
	url, _ := h2cBackend(t, grpcLikeHandler(t))
	l := Listener{TLS: true, MinTLS: "1.2", Sites: []Site{
		{Enable: true, Domains: []string{"secure.test"}, Targets: []string{url}, UpstreamH2C: true, ForwardedHeaders: true},
	}}
	addr, _ := startRunner(t, l, fakeCerts{selfSigned(t, "secure.test")}, nil)
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, ServerName: "secure.test"}, ForceAttemptHTTP2: true}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr}

	pr, pw := io.Pipe()
	req, _ := http.NewRequest("POST", "https://"+addr+"/svc/Method", pr)
	req.Host = "secure.test"
	req.Header.Set("Content-Type", "application/grpc")
	req.Header.Set("Te", "trailers")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("状态码 %d", resp.StatusCode)
	}
	if resp.ProtoMajor != 2 || resp.Header.Get("X-Backend-Proto") != "2" {
		t.Fatalf("前后端都应为 HTTP/2: 前端 %d 后端 %s", resp.ProtoMajor, resp.Header.Get("X-Backend-Proto"))
	}
	if resp.Header.Get("X-Backend-Te") != "trailers" {
		t.Fatalf("gRPC 依赖的 TE: trailers 应转给后端: %q", resp.Header.Get("X-Backend-Te"))
	}
	// 双向流：发一行必须立刻收到回显（不能被缓冲到请求结束）
	br := bufio.NewReader(resp.Body)
	for i := 0; i < 3; i++ {
		msg := fmt.Sprintf("m%d\n", i)
		if _, err := io.WriteString(pw, msg); err != nil {
			t.Fatal(err)
		}
		done := make(chan string, 1)
		go func() {
			s, _ := br.ReadString('\n')
			done <- s
		}()
		select {
		case got := <-done:
			if got != "echo:"+msg {
				t.Fatalf("第 %d 条回显错误: %q", i, got)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("第 %d 条回显超时，双向流被缓冲", i)
		}
	}
	_ = pw.Close()
	if rest, _ := io.ReadAll(br); len(rest) != 0 {
		t.Fatalf("多余数据: %q", rest)
	}
	if resp.Trailer.Get("Grpc-Status") != "0" {
		t.Fatalf("尾部 Grpc-Status 丢失: %v", resp.Trailer)
	}
}

// 未开启时 http:// 后端走 HTTP/1.1，只讲 h2c 的后端会拒绝；开启后正常
func TestH2CBackendOptIn(t *testing.T) {
	url, _ := h2cBackend(t, grpcLikeHandler(t))
	l := Listener{Sites: []Site{
		{Enable: true, Domains: []string{"off.test"}, Targets: []string{url}},
		{Enable: true, Domains: []string{"on.test"}, Targets: []string{url}, UpstreamH2C: true},
	}}
	addr, _ := startRunner(t, l, nil, nil)
	if resp, _ := get(t, http.DefaultClient, addr, "off.test", "/x", nil); resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("未开启 h2c 时应返回 502，实际 %d", resp.StatusCode)
	}
	if resp, _ := get(t, http.DefaultClient, addr, "on.test", "/x", nil); resp.StatusCode != 200 || resp.Header.Get("X-Backend-Proto") != "2" {
		t.Fatalf("开启 h2c 后应成功: %d %s", resp.StatusCode, resp.Header.Get("X-Backend-Proto"))
	}
}

// 并发请求复用后端的同一条 h2c 连接，不会为每个请求新建连接；开启“禁用连接复用”时每个请求独占连接
func TestH2CBackendMultiplexAndNoKeepAlive(t *testing.T) {
	var inflight, peak atomic.Int64
	url, conns := h2cBackend(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := inflight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
		inflight.Add(-1)
		_, _ = w.Write([]byte("ok"))
	}))
	run := func(s Site, n int, parallel bool) {
		s.Enable, s.Domains, s.Targets, s.UpstreamH2C = true, []string{"m.test"}, []string{url}, true
		addr, _ := startRunner(t, Listener{Sites: []Site{s}}, nil, nil)
		// 先单独发一个请求建好连接，之后的并发请求才会复用它
		get(t, http.DefaultClient, addr, "m.test", "/", nil)
		conns.Store(0)
		var wg sync.WaitGroup
		one := func() {
			if resp, body := get(t, http.DefaultClient, addr, "m.test", "/", nil); resp.StatusCode != 200 || body != "ok" {
				t.Errorf("请求失败: %d %q", resp.StatusCode, body)
			}
		}
		for i := 0; i < n; i++ {
			if !parallel {
				one()
				continue
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				one()
			}()
		}
		wg.Wait()
	}
	run(Site{}, 20, true)
	if c := conns.Load(); c != 0 {
		t.Fatalf("20 个并发请求应全部复用已建好的 h2c 连接，实际 %d", c)
	}
	if peak.Load() < 10 {
		t.Fatalf("请求应在同一连接上并发处理，峰值并发仅 %d", peak.Load())
	}
	run(Site{DisableKeepAlives: true}, 5, false)
	if c := conns.Load(); c != 5 {
		t.Fatalf("禁用连接复用时 5 个请求应各用 1 条连接，实际 %d", c)
	}
}

// 访客中断请求时后端的请求上下文随之取消（流式 RPC 不会残留在后端）
func TestH2CBackendCancelPropagates(t *testing.T) {
	cancelled := make(chan struct{})
	url, _ := h2cBackend(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_ = http.NewResponseController(w).Flush()
		<-r.Context().Done()
		close(cancelled)
	}))
	addr, _ := startRunner(t, Listener{Sites: []Site{{Enable: true, Domains: []string{"c.test"}, Targets: []string{url}, UpstreamH2C: true}}}, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://"+addr+"/", nil)
	req.Host = "c.test"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	resp.Body.Close()
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("访客断开后后端请求未被取消")
	}
}

// https:// 后端不受 h2c 开关影响：仍按 ALPN 协商，只讲 HTTP/1.1 的 HTTPS 后端照常可用
func TestH2CIgnoredForHTTPSBackend(t *testing.T) {
	b := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.Proto))
	}))
	defer b.Close()
	addr, _ := startRunner(t, Listener{Sites: []Site{{Enable: true, Domains: []string{"s.test"}, Targets: []string{b.URL}, InsecureSkipVerify: true, UpstreamH2C: true}}}, nil, nil)
	if resp, body := get(t, http.DefaultClient, addr, "s.test", "/", nil); resp.StatusCode != 200 || !strings.HasPrefix(body, "HTTP/") {
		t.Fatalf("HTTPS 后端异常: %d %q", resp.StatusCode, body)
	}
}

// HTTP 健康检查对 h2c 后端也按 h2c 发送；未开启时对只讲 h2c 的后端检查失败
func TestH2CHealthProbe(t *testing.T) {
	url, _ := h2cBackend(t, grpcLikeHandler(t))
	for _, h2c := range []bool{true, false} {
		p := mustPool(t, Site{Targets: []string{url}, UpstreamH2C: h2c, Health: HealthCheck{Enable: true, Mode: "http", Path: "/healthz", Timeout: 2}})
		err := p.probe(context.Background(), p.ups[0])
		if h2c && err != nil {
			t.Fatalf("h2c 健康检查应通过: %v", err)
		}
		if !h2c && err == nil {
			t.Fatal("未开启 h2c 时 HTTP/1.1 检查应失败")
		}
	}
}

var _ = http2.ErrNoCachedConn
