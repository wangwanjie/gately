package webservice

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// 反向代理的刷新策略（见 proxyFlushInterval）：SSE、长度未知的流式响应、gRPC（h2c）与 WebSocket 必须实时送达，
// 带 Content-Length 但慢慢吐出的后端最多延迟一个刷新间隔。
//
// 这些用例都采用“一问一答”：后端写出一段后阻塞，等客户端确认收到才写下一段。代理若把数据攒在缓冲里，
// 客户端永远等不到，用例会超时失败，因此与机器快慢无关。实时类用例另外限制总耗时：
// 逐段延迟一个刷新间隔时 rounds 段至少要 rounds × 100 毫秒，限制在其一半以内即可区分“立即刷新”与“定时刷新”。

const flushRounds = 12

// flushRealtimeBound 为实时类用例 flushRounds 段的总耗时上限：按 100 毫秒刷新间隔逐段延迟时至少 1.2 秒
const flushRealtimeBound = flushRounds * 50 * time.Millisecond

// stepBackend 返回按步骤写出的后端：每写出一段就刷新并等待 next 中的信号。
// header 设置响应头（可设置 Content-Length），chunk 返回第 i 段内容。
func stepBackend(t *testing.T, next chan struct{}, header func(http.Header), chunk func(i int) string, rounds int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header(w.Header())
		w.WriteHeader(200)
		rc := http.NewResponseController(w)
		for i := range rounds {
			_, _ = io.WriteString(w, chunk(i))
			_ = rc.Flush()
			select {
			case <-next:
			case <-r.Context().Done():
				return
			case <-time.After(10 * time.Second):
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// readStep 在 timeout 内从 br 读到 n 个字节，超时返回错误。
func readStep(br *bufio.Reader, n int, timeout time.Duration) (string, error) {
	type res struct {
		s   string
		err error
	}
	ch := make(chan res, 1)
	go func() {
		buf := make([]byte, n)
		_, err := io.ReadFull(br, buf)
		ch <- res{string(buf), err}
	}()
	select {
	case r := <-ch:
		return r.s, r.err
	case <-time.After(timeout):
		return "", fmt.Errorf("%v 内未收到数据", timeout)
	}
}

func TestProxyFlushStreaming(t *testing.T) {
	type tc struct {
		name     string
		header   func(http.Header)
		compress bool
		// realtime 为 true 时每段必须立即送达（总耗时远小于 rounds 个刷新间隔）
		realtime bool
	}
	event := func(i int) string {
		return fmt.Sprintf("data: %02d\n\n", i)
	}
	cases := []tc{
		{name: "SSE", realtime: true, header: func(h http.Header) {
			h.Set("Content-Type", "text/event-stream")
		}},
		{name: "SSE 开启压缩", realtime: true, compress: true, header: func(h http.Header) {
			h.Set("Content-Type", "text/event-stream")
		}},
		{name: "带长度的 SSE", realtime: true, header: func(h http.Header) {
			h.Set("Content-Type", "text/event-stream; charset=utf-8")
			h.Set("Content-Length", strconv.Itoa(flushRounds*len(event(0))))
		}},
		{name: "长度未知的流", realtime: true, header: func(h http.Header) {
			h.Set("Content-Type", "application/x-ndjson")
		}},
		// 定长响应慢慢吐出：允许延迟一个刷新间隔，但必须在超时前送达，不能攒到缓冲写满或响应结束
		{name: "定长慢速响应", header: func(h http.Header) {
			h.Set("Content-Type", "text/plain")
			h.Set("Content-Length", strconv.Itoa(flushRounds*len(event(0))))
		}},
		{name: "定长慢速响应开启压缩", compress: true, header: func(h http.Header) {
			h.Set("Content-Type", "text/plain")
			h.Set("Content-Length", strconv.Itoa(flushRounds*len(event(0))))
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			next := make(chan struct{})
			backend := stepBackend(t, next, c.header, event, flushRounds)
			addr, _ := startRunner(t, Listener{Sites: []Site{{Enable: true, Domains: []string{"s.test"}, Targets: []string{backend.URL}, Compress: c.compress}}}, nil, nil)
			tr := &http.Transport{}
			defer tr.CloseIdleConnections()
			req, _ := http.NewRequest("GET", "http://"+addr+"/", nil)
			req.Host = "s.test"
			req.Header.Set("Accept-Encoding", "identity")
			// 响应头也必须及时送达（定长响应在一个刷新间隔内），否则 Do 会一直阻塞到后端写完
			got := make(chan *http.Response, 1)
			go func() {
				resp, err := tr.RoundTrip(req)
				if err != nil {
					t.Error(err)
					close(got)
					return
				}
				got <- resp
			}()
			var resp *http.Response
			select {
			case resp = <-got:
			case <-time.After(3 * time.Second):
				t.Fatal("响应头被缓冲，3 秒内未送达")
			}
			if resp == nil {
				return
			}
			defer resp.Body.Close()
			br := bufio.NewReader(resp.Body)
			start := time.Now()
			for i := range flushRounds {
				s, err := readStep(br, len(event(i)), 3*time.Second)
				if err != nil {
					t.Fatalf("第 %d 段被缓冲: %v", i, err)
				}
				if s != event(i) {
					t.Fatalf("第 %d 段内容错误: %q", i, s)
				}
				if i < flushRounds-1 {
					next <- struct{}{}
				}
			}
			el := time.Since(start)
			if c.realtime && el > flushRealtimeBound {
				t.Fatalf("%d 段用时 %v，流式响应应立即刷新", flushRounds, el)
			}
			close(next)
		})
	}
}

// gRPC（明文 HTTP/2 后端、HTTPS 前端）的双向流逐条实时送达。
func TestProxyFlushGRPC(t *testing.T) {
	url, _ := h2cBackend(t, grpcLikeHandler(t))
	l := Listener{TLS: true, MinTLS: "1.2", Sites: []Site{{Enable: true, Domains: []string{"secure.test"}, Targets: []string{url}, UpstreamH2C: true}}}
	addr, _ := startRunner(t, l, fakeCerts{selfSigned(t, "secure.test")}, nil)
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, ServerName: "secure.test"}, ForceAttemptHTTP2: true}
	defer tr.CloseIdleConnections()
	pr, pw := io.Pipe()
	req, _ := http.NewRequest("POST", "https://"+addr+"/svc/M", pr)
	req.Host = "secure.test"
	req.Header.Set("Content-Type", "application/grpc")
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	br := bufio.NewReader(resp.Body)
	start := time.Now()
	for i := range flushRounds {
		msg := fmt.Sprintf("m%02d\n", i)
		if _, err := io.WriteString(pw, msg); err != nil {
			t.Fatal(err)
		}
		if s, err := readStep(br, len("echo:"+msg), 3*time.Second); err != nil || s != "echo:"+msg {
			t.Fatalf("第 %d 条回显: %q %v", i, s, err)
		}
	}
	if el := time.Since(start); el > flushRealtimeBound {
		t.Fatalf("%d 条往返用时 %v，gRPC 流应立即刷新", flushRounds, el)
	}
	_ = pw.Close()
	_, _ = io.Copy(io.Discard, br)
	if resp.Trailer.Get("Grpc-Status") != "0" {
		t.Fatalf("尾部丢失: %v", resp.Trailer)
	}
}

// WebSocket 升级后双向直接拷贝，逐条实时送达。
func TestProxyFlushWebSocket(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, brw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = brw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
		_ = brw.Flush()
		for {
			line, err := brw.ReadString('\n')
			if err != nil {
				return
			}
			_, _ = brw.WriteString("echo:" + line)
			_ = brw.Flush()
		}
	}))
	defer srv.Close()
	addr, _ := startRunner(t, Listener{Sites: []Site{{Enable: true, Domains: []string{"ws.test"}, Targets: []string{srv.URL}}}}, nil, nil)
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, _ = c.Write([]byte("GET /ws HTTP/1.1\r\nHost: ws.test\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"))
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, nil)
	if err != nil || resp.StatusCode != 101 {
		t.Fatalf("升级失败: %v %v", resp, err)
	}
	start := time.Now()
	for i := range flushRounds {
		msg := fmt.Sprintf("w%02d\n", i)
		_, _ = c.Write([]byte(msg))
		if s, err := readStep(br, len("echo:"+msg), 3*time.Second); err != nil || s != "echo:"+msg {
			t.Fatalf("第 %d 条回显: %q %v", i, s, err)
		}
	}
	if el := time.Since(start); el > flushRealtimeBound {
		t.Fatalf("%d 条往返用时 %v，WebSocket 应实时转发", flushRounds, el)
	}
}

// 定长的小响应在响应结束时一次写出，响应头与响应体完整到达（刷新间隔不影响已结束的响应）。
func TestProxyFlushSmallFixedLength(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "5")
		_, _ = io.WriteString(w, "hello")
	}))
	defer backend.Close()
	addr, _ := startRunner(t, Listener{Sites: []Site{{Enable: true, Domains: []string{"s.test"}, Targets: []string{backend.URL}}}}, nil, nil)
	start := time.Now()
	resp, body := get(t, noRedirect(), addr, "s.test", "/", nil)
	if resp.StatusCode != 200 || body != "hello" || resp.ContentLength != 5 {
		t.Fatalf("响应错误: %d %q %d", resp.StatusCode, body, resp.ContentLength)
	}
	if el := time.Since(start); el >= 100*time.Millisecond {
		t.Fatalf("已结束的定长响应不应等刷新间隔: %v", el)
	}
}
