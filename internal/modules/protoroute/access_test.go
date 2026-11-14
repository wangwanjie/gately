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

// startAccessRule 在 listenAddr 上启动一条只有 HTTP 路由的分流规则，返回 runner、规则与端口。
func startAccessRule(t *testing.T, acc iplist.Access, listenAddr string, protocols []string) (*runner, Rule, int) {
	t.Helper()
	backend := tagEcho(t, "web")
	listenPort := freePort(t)
	rule := Rule{
		Protocols:     protocols,
		ListenAddress: listenAddr,
		ListenPorts:   strconv.Itoa(listenPort),
		Routes:        []Route{{Name: "web", Protocol: ProtoHTTP, TargetAddresses: []string{"127.0.0.1"}, TargetPort: backend}},
		Access:        acc,
	}
	if err := rule.Validate(); err != nil {
		t.Fatal(err)
	}
	r := testRunner(rule)
	if err := r.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.stop)
	return r, rule, listenPort
}

// httpReachesFrom 与 httpReaches 相同，但从 host 发起连接。
func httpReachesFrom(t *testing.T, host string, port int) string {
	t.Helper()
	conn, err := net.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	_, _ = conn.Write([]byte("GET / HTTP/1.1\r\nHost: a.example.com\r\n\r\n"))
	line := make([]byte, 64)
	n, _ := conn.Read(line)
	return strings.TrimSpace(strings.SplitN(string(line[:n]), "\n", 2)[0])
}

// 订阅列表未载入或已删除时按空列表处理：白名单拒绝，黑名单放行
func TestAccessMissingSubscription(t *testing.T) {
	set, _, _ := iplist.Parse(strings.NewReader("127.0.0.0/8\n"))
	iplist.Default.Store("t-pr-gone", set)
	iplist.Default.Store("t-pr-gone", nil)
	for _, key := range []string{"t-pr-never", "t-pr-gone"} {
		r, _, port := startAccessRule(t, iplist.Access{IPMode: iplist.ModeWhitelist, IPSets: []string{key}}, "127.0.0.1", []string{"tcp4"})
		if got := httpReaches(t, port); got != "" {
			t.Fatalf("%s: 白名单引用的列表不可用时应拒绝，实际到达 %q", key, got)
		}
		if s := r.snapshot(); s.Denied != 1 {
			t.Fatalf("%s: 拒绝次数应为 1，实际 %d", key, s.Denied)
		}
		r2, _, port2 := startAccessRule(t, iplist.Access{IPMode: iplist.ModeBlacklist, IPSets: []string{key}}, "127.0.0.1", []string{"tcp4"})
		if got := httpReaches(t, port2); got != "web" {
			t.Fatalf("%s: 黑名单引用的列表不可用时应放行，实际 %q", key, got)
		}
		if s := r2.snapshot(); s.Denied != 0 || s.TotalConns != 1 {
			t.Fatalf("%s: 放行时统计错误: %+v", key, s)
		}
	}
}

// 双栈监听下 IPv4 来源以映射地址到达，应按 IPv4 判断；纯 IPv6 来源按 IPv6 网段判断
func TestAccessIPv6Sources(t *testing.T) {
	if ln, err := net.Listen("tcp6", "[::1]:0"); err != nil {
		t.Skip("本机不支持 IPv6 回环")
	} else {
		ln.Close()
	}
	r, rule, port := startAccessRule(t, iplist.Access{IPMode: iplist.ModeWhitelist, IPList: []string{"127.0.0.1"}}, "", []string{"tcp"})
	if got := httpReachesFrom(t, "127.0.0.1", port); got != "web" {
		t.Fatalf("IPv4 映射地址应按 IPv4 命中白名单，实际 %q", got)
	}
	if got := httpReachesFrom(t, "::1", port); got != "" {
		t.Fatalf("不在白名单中的 IPv6 来源应被拒绝，实际 %q", got)
	}
	rule.Access = iplist.Access{IPMode: iplist.ModeBlacklist, IPList: []string{"::1/128"}}
	r.update(rule, r.settings, r.current().log)
	if got := httpReachesFrom(t, "::1", port); got != "" {
		t.Fatalf("命中 IPv6 黑名单的来源应被拒绝，实际 %q", got)
	}
	if got := httpReachesFrom(t, "127.0.0.1", port); got != "web" {
		t.Fatalf("IPv4 来源不应命中 IPv6 黑名单，实际 %q", got)
	}
}

func TestAccessValidate(t *testing.T) {
	base := func(a iplist.Access) Rule {
		return Rule{Protocols: []string{"tcp"}, ListenPorts: "1", TargetAddresses: []string{"127.0.0.1"}, Access: a}
	}
	for name, a := range map[string]iplist.Access{
		"模式非法":  {IPMode: "deny", IPList: []string{"10.0.0.1"}},
		"网段非法":  {IPMode: iplist.ModeBlacklist, IPList: []string{"10.0.0.0/40"}},
		"白名单为空": {IPMode: iplist.ModeWhitelist},
	} {
		r := base(a)
		if err := r.Validate(); err == nil {
			t.Errorf("%s 应校验失败", name)
		}
	}
	ok := base(iplist.Access{IPMode: iplist.ModeBlacklist, IPSets: []string{"t-pr-missing"}})
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := ok.Access.CheckSets(); err == nil {
		t.Error("引用不存在的订阅列表应报错")
	}
}

// 黑名单命中拒绝，计数逐次累加
func TestAccessBlacklistHit(t *testing.T) {
	r, _, port := startAccessRule(t, iplist.Access{IPMode: iplist.ModeBlacklist, IPList: []string{"127.0.0.0/8"}}, "127.0.0.1", []string{"tcp4"})
	for i := 1; i <= 3; i++ {
		if got := httpReaches(t, port); got != "" {
			t.Fatalf("命中黑名单的来源应被拒绝，实际 %q", got)
		}
		if s := r.snapshot(); s.Denied != int64(i) || s.TotalConns != 0 {
			t.Fatalf("第 %d 次后统计错误: %+v", i, s)
		}
	}
}

// 优先拒绝名单（#95）：白名单命中也先断开，热更新生效。
func TestAccessDenyFirst(t *testing.T) {
	r, rule, port := startAccessRule(t, iplist.Access{IPMode: iplist.ModeWhitelist, IPList: []string{"127.0.0.0/8"}}, "127.0.0.1", []string{"tcp4"})
	if got := httpReaches(t, port); got != "web" {
		t.Fatalf("白名单内的来源应放行，实际 %q", got)
	}
	rule.Access = iplist.Access{IPMode: iplist.ModeWhitelist, IPList: []string{"127.0.0.0/8"}, IPDenyList: []string{"127.0.0.1"}}
	r.update(rule, r.settings, r.current().log)
	if got := httpReaches(t, port); got != "" {
		t.Fatalf("命中优先拒绝名单的来源应被断开，实际 %q", got)
	}
	if s := r.snapshot(); s.Denied < 1 {
		t.Fatalf("拒绝应计入 Denied: %+v", s)
	}
}
