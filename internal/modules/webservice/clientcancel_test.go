package webservice

import (
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/logbuf"
)

// TestProxyClientCancelNotBackendError 验证 issue #128：访客在后端响应前断开时，反向代理报 context canceled，
// 不记为“后端错误”警告、不计入错误数与被动检查，访问日志按 499 记录；后端真正不可达时仍记后端错误。
func TestProxyClientCancelNotBackendError(t *testing.T) {
	backendDone := make(chan struct{})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 一直等到请求被取消，模拟威联通 chartReq.cgi 这类长时间不返回的请求
		<-r.Context().Done()
		close(backendDone)
	}))
	defer backend.Close()
	// 一个已关闭的端口作为不可达后端
	dead, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadURL := "http://" + dead.Addr().String()
	dead.Close()

	logs := logbuf.New(100, log.New(io.Discard, "", 0))
	addr, run := startRunnerLogs(t, Listener{Sites: []Site{
		{Enable: true, Key: "s1", Domains: []string{"app.test"}, Targets: []string{backend.URL}, AccessLog: true},
		{Enable: true, Key: "s2", Domains: []string{"dead.test"}, Targets: []string{deadURL}, AccessLog: true},
	}}, nil, nil, logs)
	h := run.current().sites[0]

	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(c, "POST /cgi-bin/management/chartReq.cgi HTTP/1.1\r\nHost: app.test\r\nContent-Length: 0\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	activeOne(t, h)
	// 访客中止请求
	c.Close()
	select {
	case <-backendDone:
	case <-time.After(5 * time.Second):
		t.Fatal("访客断开后后端请求应被取消")
	}
	var got []logbuf.Entry
	waitConn(t, "写出访问日志", func() bool {
		got = logs.Query(0, 0, 0, &logbuf.Filter{Host: "app.test", Status: []logbuf.StatusRange{{Min: 100, Max: 999}}})
		return len(got) == 1
	})
	if got[0].Fields[logbuf.FieldStatus] != "499" {
		t.Fatalf("访客断开应按 499 记入访问日志: %+v", got[0])
	}
	if h.stats.Errors.Load() != 0 {
		t.Fatalf("访客断开不应计入错误数: %d", h.stats.Errors.Load())
	}
	for _, e := range logs.Query(0, 0, 0, &logbuf.Filter{MinLevel: logbuf.LevelWarn}) {
		if strings.Contains(e.Message, "后端错误") {
			t.Fatalf("访客断开不应记录后端错误: %q", e.Message)
		}
	}

	// 真正的后端错误仍记为警告并返回 502
	resp, _ := get(t, noRedirect(), addr, "dead.test", "/", nil)
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("后端不可达应返回 502，实际 %d", resp.StatusCode)
	}
	found := false
	for _, e := range logs.Query(0, 0, 0, &logbuf.Filter{Host: "dead.test", MinLevel: logbuf.LevelWarn}) {
		if strings.Contains(e.Message, "后端错误") {
			found = true
		}
	}
	if !found {
		t.Fatal("后端不可达应记录后端错误")
	}
	if run.current().sites[1].stats.Errors.Load() != 1 {
		t.Fatal("后端不可达应计入错误数")
	}
}
