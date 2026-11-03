package proxyx

import (
	"context"
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
)

// resetTransports 清空 Transport 缓存，避免与其他测试互相影响。
func resetTransports(t *testing.T) {
	t.Helper()
	transportMu.Lock()
	transports = map[string]*http.Transport{}
	transportMu.Unlock()
}

func TestTransportCacheBoundedAndConcurrent(t *testing.T) {
	resetTransports(t)
	t.Cleanup(func() { resetTransports(t) })
	// 并发为大量不同代理创建客户端：缓存不超过上限，-race 下不应有数据竞争
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for j := 0; j < maxTransports*2; j++ {
				setting := fmt.Sprintf("http://u:p@10.%d.%d.1:3128", g, j)
				if c := Client(setting, 0, j%2 == 0); c.Transport == nil {
					t.Errorf("%s 没有 Transport", setting)
				}
				_ = Client("", 0, false)
				_ = Client("direct", 0, false)
			}
		}(i)
	}
	wg.Wait()
	transportMu.Lock()
	n := len(transports)
	transportMu.Unlock()
	if n > maxTransports {
		t.Fatalf("Transport 缓存应不超过 %d 个，实际 %d", maxTransports, n)
	}
	// 清空重建后同一设置仍然复用同一个 Transport，直连与环境变量各自独立
	a := Client("http://10.9.9.9:1", 0, false).Transport
	if a != Client("http://10.9.9.9:1", 0, false).Transport {
		t.Fatalf("同一代理应复用 Transport")
	}
	direct := Client("direct", 0, false).Transport.(*http.Transport)
	env := Client("", 0, false).Transport.(*http.Transport)
	if direct == env || direct.Proxy != nil || env.Proxy == nil {
		t.Fatalf("direct 应不设代理函数，留空应读取环境变量")
	}
}

func TestClientFollowsUpdateProxyChange(t *testing.T) {
	t.Cleanup(func() { SetUpdateProxy(nil) })
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer target.Close()
	var hitsA, hitsB atomic.Int32
	proxyA := forwardProxy(t, &hitsA, "")
	proxyB := forwardProxy(t, &hitsB, "")
	var mu sync.Mutex
	cur := proxyA.URL
	SetUpdateProxy(func() string {
		mu.Lock()
		defer mu.Unlock()
		return cur
	})
	get := func() {
		t.Helper()
		resp, err := Client("update", 5*time.Second, false).Get(target.URL)
		if err != nil {
			t.Fatalf("请求失败: %v", err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	get()
	if hitsA.Load() != 1 || hitsB.Load() != 0 {
		t.Fatalf("应经代理 A：A=%d B=%d", hitsA.Load(), hitsB.Load())
	}
	// 修改更新代理后，下一次请求立即改走新代理，不会沿用缓存中旧代理的 Transport
	mu.Lock()
	cur = proxyB.URL
	mu.Unlock()
	get()
	if hitsA.Load() != 1 || hitsB.Load() != 1 {
		t.Fatalf("更新代理修改后应经代理 B：A=%d B=%d", hitsA.Load(), hitsB.Load())
	}
	// 清空更新代理后按未设置处理（回环地址不受环境变量影响，直接访问）
	mu.Lock()
	cur = ""
	mu.Unlock()
	get()
	if hitsA.Load() != 1 || hitsB.Load() != 1 {
		t.Fatalf("更新代理清空后不应再经代理：A=%d B=%d", hitsA.Load(), hitsB.Load())
	}
}

func TestDialErrorsHidePassword(t *testing.T) {
	// 找一个没有监听的端口：代理连不上时错误信息带代理地址但隐藏密码
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	for _, scheme := range []string{"http", "socks5"} {
		_, err := Dial(context.Background(), scheme+"://alice:topsecret@"+addr, "127.0.0.1:25", 2*time.Second)
		if err == nil {
			t.Fatalf("%s 代理不可达时应报错", scheme)
		}
		if strings.Contains(err.Error(), "topsecret") {
			t.Fatalf("%s 代理的错误信息不应包含密码: %v", scheme, err)
		}
	}
	// HTTP 客户端经不可达代理时，错误信息同样不包含密码
	_, err = Client("http://alice:topsecret@"+addr, 2*time.Second, false).Get("http://example.invalid/")
	if err == nil || strings.Contains(err.Error(), "topsecret") {
		t.Fatalf("HTTP 客户端错误信息不应包含密码: %v", err)
	}
}

func TestValidateRejectsControlChars(t *testing.T) {
	// 代理地址中的换行等控制字符不能混进 CONNECT 请求或认证头
	for _, s := range []string{"http://u:p\r\nX-Evil: 1@127.0.0.1:1", "http://127.0.0.1\r\n:1", "http://u\n@127.0.0.1:1", "socks5://127.0.0.1:1080#x"} {
		if err := Validate(s); err == nil {
			t.Errorf("Validate(%q) 应失败", s)
		}
		if u, direct := Resolve(s); u != nil || direct {
			t.Errorf("Resolve(%q) 应按未设置处理", s)
		}
	}
}

func TestDialConnectRequestLine(t *testing.T) {
	// 记录代理收到的原始 CONNECT 请求，确认请求行、Host 与认证头的格式
	got := make(chan string, 1)
	proxy := connectProxy(t, "", func(c net.Conn) {
		defer c.Close()
		buf := make([]byte, 1024)
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		var sb strings.Builder
		for !strings.Contains(sb.String(), "\r\n\r\n") {
			n, err := c.Read(buf)
			sb.Write(buf[:n])
			if err != nil {
				break
			}
		}
		got <- sb.String()
		_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\n\r\n")
	})
	conn, err := Dial(context.Background(), "http://bob:pw@"+proxy, "smtp.example.com:465", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	req := <-got
	want := "CONNECT smtp.example.com:465 HTTP/1.1\r\nHost: smtp.example.com:465\r\nProxy-Authorization: Basic Ym9iOnB3\r\n\r\n"
	if req != want {
		t.Fatalf("CONNECT 请求不符:\n%q\n期望\n%q", req, want)
	}
}
