package webservice

import (
	"bufio"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/logbuf"
)

// TestRedirectWithoutHost 验证不带 Host 的请求（HTTP/1.0、PRI *）访问用 {host} 跳转的站点时返回 400，
// 不记「重定向目标暂不可用」警告，也不会跳到缺少主机的地址。
func TestRedirectWithoutHost(t *testing.T) {
	for _, to := range []string{"https://{host}", "https://{hostAndPort}/", "https://{1}.t.test/", "https://{domainPart1}.t.test/"} {
		r := httptest.NewRequest("GET", "http://x/", nil)
		r.Host = ""
		s := Site{RedirectTo: to, KeepPath: true}
		if got, err := redirectTarget(&s, r); !errors.Is(err, errBadHost) {
			t.Fatalf("%s 遇到空 Host 应按非法请求拒绝，得到 %q %v", to, got, err)
		}
	}
	// 不引用域名变量的跳转不受 Host 影响
	r := httptest.NewRequest("GET", "http://x/a", nil)
	r.Host = ""
	if got, err := redirectTarget(&Site{RedirectTo: "https://fixed.test", KeepPath: true}, r); err != nil || got != "https://fixed.test/a" {
		t.Fatalf("固定目标应正常跳转，得到 %q %v", got, err)
	}

	logs := logbuf.New(200, log.New(io.Discard, "", 0))
	l := Listener{Default: Site{Enable: true, Type: TypeRedirect, RedirectTo: "https://{host}", RedirectCode: 301, KeepPath: true}}
	addr, _ := startRunnerLogs(t, l, nil, nil, logs)
	for _, raw := range []string{"GET / HTTP/1.0\r\n\r\n", "GET / HTTP/1.0\r\nHost: a.test\r\n\r\n"} {
		c, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = c.Write([]byte(raw))
		resp, err := http.ReadResponse(bufio.NewReader(c), nil)
		c.Close()
		if err != nil {
			t.Fatal(err)
		}
		hasHost := strings.Contains(raw, "Host:")
		if !hasHost && resp.StatusCode != http.StatusBadRequest {
			t.Errorf("不带 Host 应返回 400，实际 %d", resp.StatusCode)
		}
		if hasHost && (resp.StatusCode != http.StatusMovedPermanently || resp.Header.Get("Location") != "https://a.test/") {
			t.Errorf("带 Host 应正常跳转，实际 %d %q", resp.StatusCode, resp.Header.Get("Location"))
		}
	}
	for _, e := range logs.Since(0) {
		if e.Level == logbuf.LevelWarn {
			t.Errorf("不应记警告: %s", e.Message)
		}
	}
}
