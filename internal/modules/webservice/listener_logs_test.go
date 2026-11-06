package webservice

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// TestListenerLogsSeparated 验证同一端口分开监听 IPv4、IPv6 且监听器与站点同名时，
// 日志带各自的监听器 key，按 listener 过滤只看到本监听器的访客，来源名称也附上地址以便区分。
func TestListenerLogsSeparated(t *testing.T) {
	if ln, err := net.Listen("tcp6", "[::1]:0"); err != nil {
		t.Skip("本机没有 IPv6 回环地址")
	} else {
		ln.Close()
	}
	st, _ := config.Open(t.TempDir())
	logs := logbuf.New(200, log.New(io.Discard, "", 0))
	m := New(nil, nil)
	if err := m.Init(module.Env{Store: st, Logs: logs}); err != nil {
		t.Fatal(err)
	}
	free := func(network, addr string) int {
		ln, err := net.Listen(network, addr)
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		return ln.Addr().(*net.TCPAddr).Port
	}
	site := Site{Enable: true, Name: "博客", Domains: []string{"blog.test"}, Type: TypeText, Text: "ok", StatusCode: 200, AccessLog: true}
	v4 := Listener{Key: "v4key", Name: "HTTPS", Enable: true, ListenIP: "127.0.0.1", Network: "tcp4", Port: free("tcp4", "127.0.0.1:0"), Sites: []Site{site}}
	v6 := Listener{Key: "v6key", Name: "HTTPS", Enable: true, ListenIP: "::1", Network: "tcp6", Port: free("tcp6", "[::1]:0"), Sites: []Site{site}}
	for _, l := range []*Listener{&v4, &v6} {
		if err := l.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	m.cfg.Listeners = []Listener{v4, v6}
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	c := &http.Client{Timeout: 3 * time.Second}
	for _, addr := range []string{fmt.Sprintf("127.0.0.1:%d", v4.Port), fmt.Sprintf("[::1]:%d", v6.Port)} {
		if resp, body := get(t, c, addr, "blog.test", "/", nil); resp.StatusCode != 200 || body != "ok" {
			t.Fatalf("%s 请求异常: %d %q", addr, resp.StatusCode, body)
		}
	}
	access := func(f *logbuf.Filter) []logbuf.Entry {
		var out []logbuf.Entry
		for _, e := range logs.Since(0) {
			if e.Fields[logbuf.FieldStatus] != "" && f.Match(&e) {
				out = append(out, e)
			}
		}
		return out
	}
	if n := len(access(&logbuf.Filter{Site: "博客"})); n != 2 {
		t.Fatalf("两个监听器的同名站点共 2 条访问日志，实际 %d", n)
	}
	for key, ip := range map[string]string{"v4key": "127.0.0.1", "v6key": "::1"} {
		es := access(&logbuf.Filter{Site: "博客", Listener: key})
		if len(es) != 1 || es[0].Fields[logbuf.FieldIP] != ip {
			t.Fatalf("监听器 %s 应只有来自 %s 的 1 条访问日志: %+v", key, ip, es)
		}
	}
	// 来源名称附上监听地址，日志页按来源也能区分
	var sources []string
	for _, e := range logs.Since(0) {
		if strings.Contains(e.Message, "监听") && e.Fields[logbuf.FieldListener] != "" {
			sources = append(sources, e.Source)
		}
	}
	want := []string{fmt.Sprintf("HTTPS (127.0.0.1:%d)", v4.Port), fmt.Sprintf("HTTPS ([::1]:%d)", v6.Port)}
	if fmt.Sprint(sources) != fmt.Sprint(want) {
		t.Fatalf("启动日志的来源应为 %v，实际 %v", want, sources)
	}
}

// TestListenerLogLabel 验证来源名称只在重名时附上地址，未填写地址时按网络类型区分 IPv4、IPv6。
func TestListenerLogLabel(t *testing.T) {
	all := []Listener{
		{Key: "a", Name: "HTTPS", Network: "tcp4", Port: 443},
		{Key: "b", Name: "HTTPS", Network: "tcp6", Port: 443},
		{Key: "c", Name: "HTTPS", Network: "tcp", Port: 8443},
		{Key: "d", Name: "管理", Network: "tcp", ListenIP: "10.0.0.1", Port: 80},
		{Key: "e", Network: "tcp", Port: 80},
		{Key: "f", Network: "tcp", ListenIP: "10.0.0.1", Port: 80},
	}
	want := map[string]string{
		"a": "HTTPS (0.0.0.0:443)",
		"b": "HTTPS ([::]:443)",
		"c": "HTTPS (*:8443)",
		"d": "管理",
		"e": ":80 (*:80)",
		"f": ":80 (10.0.0.1:80)",
	}
	for _, l := range all {
		if got := listenerLogLabel(l, all); got != want[l.Key] {
			t.Errorf("%s: %q，期望 %q", l.Key, got, want[l.Key])
		}
	}
}
