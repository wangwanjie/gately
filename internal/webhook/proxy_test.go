package webhook

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/proxyx"
)

// TestSendProxy 验证 Webhook 的代理设置：代理地址经代理、update 复用更新代理、direct 与更新代理为空时不经代理；
// 无效代理在保存时被拒绝。
func TestSendProxy(t *testing.T) {
	t.Cleanup(func() { proxyx.SetUpdateProxy(nil) })
	var got atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		got.Add(1)
		_, _ = w.Write([]byte("ok"))
	}))
	defer target.Close()
	var hits atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		out, _ := http.NewRequest(r.Method, r.URL.String(), r.Body)
		resp, err := http.DefaultTransport.RoundTrip(out)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	defer proxy.Close()

	send := func(p string) {
		t.Helper()
		c := Config{Enable: true, URL: target.URL + "/?ip={ip}", Method: "POST", Body: "{}", Timeout: 5, Proxy: p}
		if err := c.Validate(); err != nil {
			t.Fatalf("%q 校验失败: %v", p, err)
		}
		if _, err := Send(context.Background(), c, map[string]string{"ip": "1.2.3.4"}); err != nil {
			t.Fatalf("%q 发送失败: %v", p, err)
		}
	}
	send(proxy.URL)
	if hits.Load() != 1 {
		t.Fatalf("应经代理发送，代理收到 %d 次", hits.Load())
	}
	send(" DIRECT ")
	if hits.Load() != 1 {
		t.Fatalf("direct 不应经过代理")
	}
	// 更新代理为空时 update 按未设置处理
	send("update")
	if hits.Load() != 1 {
		t.Fatalf("更新代理为空时不应经过代理")
	}
	proxyx.SetUpdateProxy(func() string { return proxy.URL })
	send("Update")
	if hits.Load() != 2 || got.Load() != 4 {
		t.Fatalf("update 应经更新代理：代理 %d 次，目标 %d 次", hits.Load(), got.Load())
	}

	c := Config{Enable: true, URL: target.URL, Proxy: "127.0.0.1:7890"}
	if c.Validate() == nil {
		t.Fatal("缺少协议的代理地址应被拒绝")
	}
	// 未启用时不校验，也不影响保存
	c = Config{Enable: false, Proxy: "bad"}
	if err := c.Validate(); err != nil {
		t.Fatalf("未启用的 Webhook 不应校验: %v", err)
	}
	// 超时由 context 控制：代理无响应时按配置的超时返回
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer slow.Close()
	start := time.Now()
	_, err := Send(context.Background(), Config{Enable: true, URL: target.URL, Timeout: 1, Proxy: slow.URL}, nil)
	if err == nil || time.Since(start) > 3*time.Second {
		t.Fatalf("代理无响应时应按超时返回，耗时 %v，错误 %v", time.Since(start), err)
	}
}
