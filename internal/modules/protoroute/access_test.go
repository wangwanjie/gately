package protoroute

import (
	"context"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/iplist"
)

// httpReaches 发送一个 HTTP 请求行，返回后端回复的标签；连接被访问控制断开时返回空串。
func httpReaches(t *testing.T, port int) string {
	t.Helper()
	conn, err := net.Dial("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	_, _ = conn.Write([]byte("GET / HTTP/1.1\r\nHost: a.example.com\r\n\r\n"))
	line := make([]byte, 64)
	n, err := conn.Read(line)
	if err != nil && err != io.EOF {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(string(line[:n]), "\n", 2)[0])
}

func TestAccessControl(t *testing.T) {
	set, _, _ := iplist.Parse(strings.NewReader("127.0.0.0/8\n"))
	iplist.Default.Store("t-pr", set)
	t.Cleanup(func() { iplist.Default.Store("t-pr", nil) })

	backend := tagEcho(t, "web")
	listenPort := freePort(t)
	rule := Rule{
		Protocols:     []string{"tcp4"},
		ListenAddress: "127.0.0.1",
		ListenPorts:   strconv.Itoa(listenPort),
		Routes:        []Route{{Name: "web", Protocol: ProtoHTTP, TargetAddresses: []string{"127.0.0.1"}, TargetPort: backend}},
		Access:        iplist.Access{IPMode: iplist.ModeWhitelist, IPList: []string{"10.0.0.0/8"}},
	}
	if err := rule.Validate(); err != nil {
		t.Fatal(err)
	}
	r := testRunner(rule)
	if err := r.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer r.stop()

	if got := httpReaches(t, listenPort); got != "" {
		t.Fatalf("不在白名单中的来源应被断开，实际到达 %q", got)
	}
	if s := r.snapshot(); s.Denied != 1 || s.TotalConns != 0 || s.Rejected != 0 {
		t.Fatalf("拒绝应只计入 Denied: %+v", s)
	}

	// 热更新为引用订阅列表的白名单
	rule.Access = iplist.Access{IPMode: iplist.ModeWhitelist, IPSets: []string{"t-pr"}}
	if !r.canUpdate(rule, r.settings) {
		t.Fatal("只修改名单时应能热更新")
	}
	r.update(rule, r.settings, r.current().log)
	if got := httpReaches(t, listenPort); got != "web" {
		t.Fatalf("命中订阅白名单应放行，实际 %q", got)
	}

	// 订阅列表刷新后立即生效
	set2, _, _ := iplist.Parse(strings.NewReader("192.0.2.0/24\n"))
	iplist.Default.Store("t-pr", set2)
	if got := httpReaches(t, listenPort); got != "" {
		t.Fatalf("列表刷新后已移出的来源应被断开，实际 %q", got)
	}

	// 黑名单
	rule.Access = iplist.Access{IPMode: iplist.ModeBlacklist, IPList: []string{"192.0.2.0/24"}}
	r.update(rule, r.settings, r.current().log)
	if got := httpReaches(t, listenPort); got != "web" {
		t.Fatalf("不在黑名单中的来源应放行，实际 %q", got)
	}

	// 封禁优先于名单
	if _, _, err := banlist.Default.Ban("127.0.0.1", banlist.Options{Duration: time.Hour, Force: true, Source: banlist.SourceManual}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { banlist.Default.Unban("127.0.0.1") })
	rule.Access = iplist.Access{IPMode: iplist.ModeWhitelist, IPList: []string{"127.0.0.1"}}
	r.update(rule, r.settings, r.current().log)
	if got := httpReaches(t, listenPort); got != "" {
		t.Fatalf("被封禁的地址即使在白名单中也应被断开，实际 %q", got)
	}
}
