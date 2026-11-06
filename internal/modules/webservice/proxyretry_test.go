package webservice

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// deadAddr 返回一个没有监听的本地地址，连接会被立即拒绝。
func deadAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

// retryEchoBackend 回显后端名称、收到的路径、Host 与请求体，并计数。
func retryEchoBackend(t *testing.T, name string, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		body, _ := io.ReadAll(r.Body)
		fmt.Fprintf(w, "%s %s %s %s", name, r.URL.Path, r.Host, body)
	}))
	t.Cleanup(b.Close)
	return b
}

func postBody(t *testing.T, addr, host, path, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest("POST", "http://"+addr+path, strings.NewReader(body))
	req.Host = host
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// 连不上的后端换下一个重试：POST 请求体完整送达，路径按新后端重新拼接（不叠加上一个后端的路径），
// 失败的后端计入被动检查，请求数记在实际处理的后端上。
func TestRetryNextUpstreamOnDialFailure(t *testing.T) {
	var hits atomic.Int32
	live := retryEchoBackend(t, "live", &hits)
	dead := deadAddr(t)
	site := Site{Enable: true, Key: "s1", Domains: []string{"a.test"}, Targets: []string{"http://" + dead + "/x", live.URL + "/y"}}
	addr, run := startRunner(t, Listener{Sites: []Site{site}}, nil, nil)
	// 轮询时连不上的后端每两次被选中一次，6 次后连续失败 3 次被摘除
	for i := 0; i < 6; i++ {
		code, body := postBody(t, addr, "a.test", "/req", "payload")
		if code != 200 || body != "live /y/req a.test payload" {
			t.Fatalf("第 %d 次应改用可用后端: %d %q", i, code, body)
		}
	}
	if hits.Load() != 6 {
		t.Fatalf("可用后端应收到 6 次请求: %d", hits.Load())
	}
	h := siteOf(run)
	deadUp, liveUp := h.pool.ups[0], h.pool.ups[1]
	if !deadUp.down.Load() {
		t.Fatal("连续连接失败的后端应被摘除")
	}
	if liveUp.stats.requests.Load() != 6 || liveUp.conns.Load() != 0 || deadUp.conns.Load() != 0 {
		t.Fatalf("计数不符: live=%d conns=%d/%d", liveUp.stats.requests.Load(), liveUp.conns.Load(), deadUp.conns.Load())
	}
}

// 使用后端地址作 Host 时，重试后的 Host 为新后端的地址。
func TestRetryUsesNewTargetHost(t *testing.T) {
	var hits atomic.Int32
	live := retryEchoBackend(t, "live", &hits)
	site := Site{Enable: true, Domains: []string{"a.test"}, UseTargetHost: true, Targets: []string{"http://" + deadAddr(t), live.URL}}
	addr, _ := startRunner(t, Listener{Sites: []Site{site}}, nil, nil)
	code, body := postBody(t, addr, "a.test", "/p", "")
	if code != 200 || body != "live /p "+strings.TrimPrefix(live.URL, "http://")+" " {
		t.Fatalf("Host 应为新后端地址: %d %q", code, body)
	}
}

// 后端都连不上时最多尝试 3 个后端后返回 502；只有一个后端时不重试。
func TestRetryLimitAndAllDead(t *testing.T) {
	var targets []string
	for i := 0; i < 5; i++ {
		targets = append(targets, "http://"+deadAddr(t))
	}
	addr, run := startRunner(t, Listener{Sites: []Site{{Enable: true, Domains: []string{"a.test"}, Targets: targets}}}, nil, nil)
	code, _ := postBody(t, addr, "a.test", "/", "x")
	if code != http.StatusBadGateway {
		t.Fatalf("全部连不上应返回 502: %d", code)
	}
	var tried int64
	for _, up := range siteOf(run).pool.ups {
		tried += up.stats.requests.Load()
	}
	if tried != maxProxyAttempts {
		t.Fatalf("应尝试 %d 个后端，实际 %d", maxProxyAttempts, tried)
	}

	addr, run = startRunner(t, Listener{Sites: []Site{{Enable: true, Domains: []string{"a.test"}, Targets: targets[:1]}}}, nil, nil)
	if code, _ := postBody(t, addr, "a.test", "/", "x"); code != http.StatusBadGateway || siteOf(run).pool.ups[0].stats.requests.Load() != 1 {
		t.Fatalf("单个后端不应重试: %d", code)
	}
}

// 连接已建立后的失败（后端读了请求后断开）不重试，避免把可能已被处理的请求再发一次。
func TestNoRetryAfterConnected(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			buf := make([]byte, 4096)
			_, _ = c.Read(buf)
			c.Close()
		}
	}()
	var hits atomic.Int32
	live := retryEchoBackend(t, "live", &hits)
	addr, _ := startRunner(t, Listener{Sites: []Site{{Enable: true, Domains: []string{"a.test"}, Targets: []string{"http://" + ln.Addr().String(), live.URL}}}}, nil, nil)
	code, _ := postBody(t, addr, "a.test", "/", "once")
	if code != http.StatusBadGateway || hits.Load() != 0 {
		t.Fatalf("已连接后的失败不应重试: %d hits=%d", code, hits.Load())
	}
}

// 备用后端：主后端都连不上时改用备用后端。
func TestRetryFallsBackToBackup(t *testing.T) {
	var hits atomic.Int32
	backup := retryEchoBackend(t, "backup", &hits)
	addr, _ := startRunner(t, Listener{Sites: []Site{{Enable: true, Domains: []string{"a.test"}, Targets: []string{"http://" + deadAddr(t), backup.URL + " backup"}}}}, nil, nil)
	code, body := postBody(t, addr, "a.test", "/b", "")
	if code != 200 || !strings.HasPrefix(body, "backup /b") {
		t.Fatalf("应改用备用后端: %d %q", code, body)
	}
}
