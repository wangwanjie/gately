package webservice

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/visitors"
)

// TestSoakNoLeak 对完整请求路径（访问日志、访客审计、统计、实时连接登记）做浸泡：
// 大量来源地址、一部分请求中途取消，结束后 goroutine 数与堆占用必须回落。
func TestSoakNoLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("浸泡测试")
	}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			select {
			case <-r.Context().Done():
			case <-time.After(2 * time.Second):
			}
			return
		}
		_, _ = w.Write([]byte("hello"))
	}))
	defer backend.Close()
	auth := newAuthenticator("secret")
	auth.visits = visitors.New(filepath.Join(t.TempDir(), "v.jsonl.gz"), nil)
	s := Site{Key: "s1", Enable: true, Domains: []string{"a.test"}, Type: TypeProxy, Targets: []string{backend.URL}, AccessLog: true}
	if err := s.validate(false); err != nil {
		t.Fatal(err)
	}
	h, err := newSiteHandler(s, testLogs().Logger("t"), auth, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer h.closeIdle()
	front := httptest.NewServer(h)
	defer front.Close()

	client := &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: 64}}
	defer client.CloseIdleConnections()

	run := func(total int) {
		var wg sync.WaitGroup
		per := total / 32
		for w := 0; w < 32; w++ {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()
				for i := 0; i < per; i++ {
					path := fmt.Sprintf("/p/%d/%d", w, i)
					ctx := context.Background()
					cancel := func() {}
					if i%10 == 0 {
						path = "/slow"
						ctx, cancel = context.WithTimeout(ctx, 20*time.Millisecond)
					}
					req, _ := http.NewRequestWithContext(ctx, "GET", front.URL+path, nil)
					req.Host = "a.test"
					req.Header.Set("X-Forwarded-For", fmt.Sprintf("10.%d.%d.%d", w, i>>8&255, i&255))
					resp, err := client.Do(req)
					if err == nil {
						_, _ = io.Copy(io.Discard, resp.Body)
						resp.Body.Close()
					}
					cancel()
				}
			}(w)
		}
		wg.Wait()
	}
	settle := func() (int, uint64) {
		var g int
		var m runtime.MemStats
		for i := 0; i < 8; i++ {
			// 后端空闲连接池按并发峰值保留连接，先关闭再比较，避免把正常的连接池算作泄漏
			client.CloseIdleConnections()
			h.closeIdle()
			runtime.GC()
			time.Sleep(100 * time.Millisecond)
			g = runtime.NumGoroutine()
		}
		runtime.ReadMemStats(&m)
		return g, m.HeapInuse
	}
	run(2000)
	g0, m0 := settle()
	run(10000)
	g1, m1 := settle()
	t.Logf("goroutine %d -> %d, HeapInuse %d KB -> %d KB, active=%d", g0, g1, m0>>10, m1>>10, len(h.conns.active()))
	if g1 > g0+5 {
		t.Fatalf("goroutine 增长: %d -> %d", g0, g1)
	}
	if m1 > m0+32<<20 {
		t.Fatalf("堆增长过多: %d -> %d", m0, m1)
	}
}
