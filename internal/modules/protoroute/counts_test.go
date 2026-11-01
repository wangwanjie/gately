package protoroute

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/iplist"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// hit 模拟一条识别结果分到的连接，与 runner.handle 的计数方式相同。
func hit(t *testing.T, r *runner, res sniffResult) {
	t.Helper()
	tg := r.current().router.match(res)
	if tg == nil {
		t.Fatalf("%v 没有命中任何目标", res)
	}
	tg.conns.Add(1)
}

// routeCounts 按路由名称输出快照中的命中数，便于在重排后比较。
func routeCounts(r *runner) map[string]int64 {
	snap := r.snapshot()
	out := map[string]int64{"default": snap.DefaultConns}
	for i, n := range snap.RouteConns {
		out[r.current().rule.Routes[i].Name] = n
	}
	return out
}

func validRule(t *testing.T, rule Rule) Rule {
	t.Helper()
	if err := rule.Validate(); err != nil {
		t.Fatal(err)
	}
	return rule
}

// 热更新后，匹配条件不变的路由继续累计，与位置、名称、目标是否改动无关；新增与改了匹配条件的路由从 0 开始
func TestUpdateKeepsRouteCounts(t *testing.T) {
	base := Rule{
		Protocols: []string{"tcp4"}, ListenAddress: "127.0.0.1", ListenPorts: "443",
		TargetAddresses: []string{"127.0.0.9"},
		Routes: []Route{
			{Name: "web", Protocol: "tls", Domains: []string{"a.example.com", "b.example.com"}, TargetAddresses: []string{"127.0.0.1"}},
			{Name: "h2", Protocol: "tls", Domains: []string{"c.example.com"}, ALPN: []string{"h2"}, TargetAddresses: []string{"127.0.0.1"}},
			{Name: "files", Protocol: "http", Domains: []string{"files.example.com"}, TargetAddresses: []string{"127.0.0.1"}},
			{Name: "ssh", Protocol: "ssh", TargetAddresses: []string{"127.0.0.1"}},
			{Name: "gone", Protocol: "rdp", TargetAddresses: []string{"127.0.0.1"}},
		},
	}
	r := testRunner(validRule(t, base))
	hit(t, r, sniffResult{Proto: ProtoTLS, Host: "a.example.com"})
	hit(t, r, sniffResult{Proto: ProtoTLS, Host: "b.example.com"})
	hit(t, r, sniffResult{Proto: ProtoTLS, Host: "c.example.com", ALPN: []string{"h2"}})
	hit(t, r, sniffResult{Proto: ProtoHTTP, Host: "files.example.com"})
	hit(t, r, sniffResult{Proto: ProtoHTTP, Host: "files.example.com"})
	hit(t, r, sniffResult{Proto: ProtoHTTP, Host: "files.example.com"})
	hit(t, r, sniffResult{Proto: ProtoSSH})
	hit(t, r, sniffResult{Proto: ProtoRDP})
	hit(t, r, sniffResult{})
	hit(t, r, sniffResult{})
	want := map[string]int64{"web": 2, "h2": 1, "files": 3, "ssh": 1, "gone": 1, "default": 2}
	if got := routeCounts(r); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("热更新前 %v，期望 %v", got, want)
	}

	// 只改备注与来源 IP 名单：全部计数保留
	next := base
	next.Remark = "changed"
	next.Access = iplist.Access{IPMode: iplist.ModeBlacklist, IPList: []string{"10.0.0.0/8"}}
	r.update(validRule(t, next), r.settings, r.current().log)
	if got := routeCounts(r); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("改备注与名单后 %v，期望 %v", got, want)
	}

	// 重排、改名、改目标、域名换个顺序写、删一条、加一条、改一条的域名与另一条的 ALPN、改默认目标地址
	next.Routes = []Route{
		{Name: "ssh-renamed", Protocol: "ssh", TargetAddresses: []string{"127.0.0.2"}, TargetPort: 2222},
		{Name: "files", Protocol: "http", Domains: []string{"files.example.com", "more.example.com"}, TargetAddresses: []string{"127.0.0.1"}},
		{Name: "web", Protocol: "tls", Domains: []string{"B.example.com", "a.example.com"}, TargetAddresses: []string{"127.0.0.3"}, ProxyProtocol: "v2"},
		{Name: "h2", Protocol: "tls", Domains: []string{"c.example.com"}, ALPN: []string{"h2", "http/1.1"}, TargetAddresses: []string{"127.0.0.1"}},
		{Name: "new", Protocol: "socks", TargetAddresses: []string{"127.0.0.1"}},
	}
	next.TargetAddresses = []string{"127.0.0.8"}
	r.update(validRule(t, next), r.settings, r.current().log)
	want = map[string]int64{"ssh-renamed": 1, "files": 0, "web": 2, "h2": 0, "new": 0, "default": 2}
	if got := routeCounts(r); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("增删重排后 %v，期望 %v", got, want)
	}

	// 沿用的计数继续累计
	hit(t, r, sniffResult{Proto: ProtoTLS, Host: "a.example.com"})
	hit(t, r, sniffResult{Proto: ProtoSOCKS})
	hit(t, r, sniffResult{})
	want = map[string]int64{"ssh-renamed": 1, "files": 0, "web": 3, "h2": 0, "new": 1, "default": 3}
	if got := routeCounts(r); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("继续累计后 %v，期望 %v", got, want)
	}

	// 去掉默认目标后计数丢弃，再加回从 0 开始；删掉又加回的路由同样从 0 开始
	noDefault := next
	noDefault.TargetAddresses = nil
	noDefault.Routes = next.Routes[1:]
	r.update(validRule(t, noDefault), r.settings, r.current().log)
	if s := r.snapshot(); s.DefaultConns != 0 {
		t.Fatalf("去掉默认目标后默认计数 %d", s.DefaultConns)
	}
	r.update(validRule(t, next), r.settings, r.current().log)
	want = map[string]int64{"ssh-renamed": 0, "files": 0, "web": 3, "h2": 0, "new": 1, "default": 0}
	if got := routeCounts(r); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("加回默认目标与路由后 %v，期望 %v", got, want)
	}
}

// 未经校验的配置中匹配条件相同的路由按出现先后一一对应
func TestInheritCountsDuplicateRoutes(t *testing.T) {
	rule := Rule{Routes: []Route{
		{Name: "a", Protocol: "ssh", TargetAddresses: []string{"x"}},
		{Name: "b", Protocol: "ssh", TargetAddresses: []string{"x"}},
	}}
	prev := newRouter(rule)
	prev.targets[0].conns.Add(5)
	prev.targets[1].conns.Add(7)
	rule.Routes = append(rule.Routes[:1:1], Route{Name: "c", Protocol: "SSH"}, rule.Routes[1], Route{Name: "d", Protocol: "ssh"})
	next := newRouter(rule)
	next.inheritCounts(prev)
	got := []int64{}
	for _, tg := range next.targets {
		got = append(got, tg.conns.Load())
	}
	if fmt.Sprint(got) != "[5 7 0 0]" {
		t.Fatalf("重复路由计数 %v，期望 [5 7 0 0]", got)
	}
}

// 热更新前已取到旧路由表的连接在更新后才计入，计数不会丢失
func TestUpdateCountsInFlightConns(t *testing.T) {
	rule := validRule(t, Rule{Protocols: []string{"tcp4"}, ListenPorts: "80",
		Routes: []Route{{Name: "web", Protocol: "http", TargetAddresses: []string{"127.0.0.1"}}}})
	r := testRunner(rule)
	old := r.current()
	rule.Remark = "x"
	r.update(rule, r.settings, old.log)
	old.router.match(sniffResult{Proto: ProtoHTTP}).conns.Add(1)
	if s := r.snapshot(); s.RouteConns[0] != 1 {
		t.Fatalf("更新前开始的连接未计入，计数 %v", s.RouteConns)
	}
}

// metricValue 返回指定标签的路由命中指标，不存在时返回 -1。
func metricValue(samples []module.Sample, rule, route string) float64 {
	for _, s := range samples {
		if s.Name == "gately_protoroute_route_connections_total" && s.Labels["rule"] == rule && s.Labels["route"] == route {
			return s.Value
		}
	}
	return -1
}

// hostReaches 发送带指定 Host 的 HTTP 请求行，返回后端回复的标签。
func hostReaches(t *testing.T, port int, host string) string {
	t.Helper()
	conn, err := net.Dial("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	_, _ = fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: %s\r\n\r\n", host)
	line := make([]byte, 64)
	n, err := conn.Read(line)
	if err != nil && err != io.EOF {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(string(line[:n]), "\n", 2)[0])
}

// 端到端：经模块热更新后，真实连接的命中数延续，Prometheus 指标与卡片快照一致
func TestModuleUpdateKeepsCountsAndMetrics(t *testing.T) {
	a, b, def := tagEcho(t, "a"), tagEcho(t, "b"), tagEcho(t, "def")
	listenPort := freePort(t)
	rule := validRule(t, Rule{
		Key: "k1", Name: "edge", Enable: true,
		Protocols: []string{"tcp4"}, ListenAddress: "127.0.0.1", ListenPorts: strconv.Itoa(listenPort),
		TargetAddresses: []string{"127.0.0.1"}, TargetPorts: strconv.Itoa(def),
		Routes: []Route{
			{Name: "a", Protocol: "http", Domains: []string{"a.example.com"}, TargetAddresses: []string{"127.0.0.1"}, TargetPort: a},
			{Name: "b", Protocol: "http", Domains: []string{"b.example.com"}, TargetAddresses: []string{"127.0.0.1"}, TargetPort: b},
		},
	})
	m := New()
	m.ctx = context.Background()
	m.logs = logbuf.New(100, log.New(io.Discard, "", 0))
	m.cfg.Settings = defaultSettings()
	m.cfg.Rules = []Rule{rule}
	m.startRuleLocked(rule)
	r := m.runners["k1"]
	if r == nil {
		t.Fatalf("规则未启动: %v", m.errors)
	}
	defer m.stopRuleLocked("k1")

	for host, want := range map[string]string{"a.example.com": "a", "b.example.com": "b", "x.example.com": "def"} {
		if got := hostReaches(t, listenPort, host); got != want {
			t.Fatalf("%s 到达 %q，期望 %q", host, got, want)
		}
	}
	hostReaches(t, listenPort, "a.example.com")

	// 重排路由、加一条路由、改名单，走模块的热更新路径
	rule.Routes = []Route{rule.Routes[1], {Name: "c", Protocol: "http", Domains: []string{"c.example.com"}, TargetAddresses: []string{"127.0.0.1"}, TargetPort: a}, rule.Routes[0]}
	rule.Access = iplist.Access{IPMode: iplist.ModeWhitelist, IPList: []string{"127.0.0.0/8"}}
	rule.Remark = "edited"
	rule = validRule(t, rule)
	m.cfg.Rules = []Rule{rule}
	m.restartLocked("k1")
	if m.runners["k1"] != r {
		t.Fatal("应热更新而不是重启")
	}
	hostReaches(t, listenPort, "b.example.com")
	hostReaches(t, listenPort, "c.example.com")

	snap := r.snapshot()
	if fmt.Sprint(snap.RouteConns) != "[2 1 2]" || snap.DefaultConns != 1 || snap.TotalConns != 6 {
		t.Fatalf("热更新后路由计数 %v 默认 %d 总数 %d，期望 [2 1 2] 默认 1 总数 6", snap.RouteConns, snap.DefaultConns, snap.TotalConns)
	}
	samples := m.Metrics()
	for i, route := range rule.Routes {
		if v := metricValue(samples, "edge", route.Name); v != float64(snap.RouteConns[i]) {
			t.Fatalf("路由 %s 指标 %v，卡片 %d", route.Name, v, snap.RouteConns[i])
		}
	}
	if v := metricValue(samples, "edge", "default"); v != float64(snap.DefaultConns) {
		t.Fatalf("默认目标指标 %v，卡片 %d", v, snap.DefaultConns)
	}
}
