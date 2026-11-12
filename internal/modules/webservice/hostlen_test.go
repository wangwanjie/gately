package webservice

import (
	"net/http"
	"strings"
	"testing"
)

// 域名最长 253 个字符：过长的 Host 请求头直接返回 400，不进入站点路由与访问日志，
// 否则默认站点会把接近 1 MB 的 Host 原样写进每一条访问日志与错误提示。
func TestOversizedHostRejected(t *testing.T) {
	logs := testLogs()
	l := Listener{Default: Site{Enable: true, Type: TypeText, Text: "ok", AccessLog: true}}
	addr, _ := startRunnerWAFLogs(t, l, nil, logs)

	q, _ := http.NewRequest("GET", "http://"+addr+"/", nil)
	q.Host = strings.Repeat("a", 200<<10) + ".test"
	if _, body := sendDirect(t, addr, q); strings.Contains(body, "ok") {
		t.Fatal("过长的 Host 不应被站点处理")
	}
	for _, e := range logs.Query(0, 0, 100, nil) {
		if len(e.Message) > 4096 {
			t.Fatalf("日志条目过大: %d 字节", len(e.Message))
		}
	}

	q, _ = http.NewRequest("GET", "http://"+addr+"/", nil)
	q.Host = "normal.test"
	if _, body := sendDirect(t, addr, q); !strings.Contains(body, "ok") {
		t.Fatalf("正常 Host 应由默认站点处理: %q", body)
	}
}
