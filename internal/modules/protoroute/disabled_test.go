package protoroute

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/logbuf"
)

// 停用的路由仍校验内容，但不参与冲突检查：同一域名、同一协议可以保留停用的备选路由
func TestDisabledRouteValidate(t *testing.T) {
	base := func(routes ...Route) Rule {
		return Rule{Protocols: []string{"tcp"}, ListenPorts: "443", Routes: routes}
	}
	ok := []Rule{
		// 同一域名一条启用、一条停用
		base(
			Route{Name: "new", Domains: []string{"a.example.com"}, TargetAddresses: []string{"127.0.0.1"}},
			Route{Name: "old", Domains: []string{"a.example.com"}, TargetAddresses: []string{"127.0.0.2"}, Disabled: true},
		),
		// 同一域名同一 ALPN 两条都停用
		base(
			Route{Name: "x", Domains: []string{"a.example.com"}, ALPN: []string{"h2"}, TargetAddresses: []string{"127.0.0.1"}, Disabled: true},
			Route{Name: "y", Domains: []string{"a.example.com"}, ALPN: []string{"h2"}, TargetAddresses: []string{"127.0.0.2"}, Disabled: true},
		),
		// 每种协议一条的限制只针对启用的路由
		base(
			Route{Name: "ssh-a", Protocol: "ssh", TargetAddresses: []string{"127.0.0.1"}, Disabled: true},
			Route{Name: "ssh-b", Protocol: "ssh", TargetAddresses: []string{"127.0.0.2"}},
		),
		// 只有停用的路由、没有默认目标也允许保存，此时规则断开所有连接
		base(Route{Name: "only", Protocol: "ssh", TargetAddresses: []string{"127.0.0.1"}, Disabled: true}),
	}
	for i, rule := range ok {
		if err := rule.Validate(); err != nil {
			t.Fatalf("用例 %d 应通过: %v", i, err)
		}
	}
	bad := map[string]Rule{
		// 两条启用的路由仍然冲突
		"同时出现": base(
			Route{Name: "a", Domains: []string{"a.example.com"}, TargetAddresses: []string{"127.0.0.1"}},
			Route{Name: "b", Domains: []string{"a.example.com"}, TargetAddresses: []string{"127.0.0.2"}, Disabled: true},
			Route{Name: "c", Domains: []string{"A.example.com"}, TargetAddresses: []string{"127.0.0.3"}},
		),
		"同一协议": base(
			Route{Name: "s1", Protocol: "ssh", TargetAddresses: []string{"127.0.0.1"}},
			Route{Name: "s2", Protocol: "ssh", TargetAddresses: []string{"127.0.0.2"}, Disabled: true},
			Route{Name: "s3", Protocol: "ssh", TargetAddresses: []string{"127.0.0.3"}},
		),
		// 停用的路由内容照常校验
		"至少填写一个目标地址": base(Route{Name: "empty", Domains: []string{"a.example.com"}, Disabled: true}),
		"不支持的协议":     base(Route{Name: "p", Protocol: "gopher", TargetAddresses: []string{"127.0.0.1"}, Disabled: true}),
		"格式无效":       base(Route{Name: "d", Domains: []string{"a.example.com:443"}, TargetAddresses: []string{"127.0.0.1"}, Disabled: true}),
		"ALPN":       base(Route{Name: "s", Protocol: "ssh", ALPN: []string{"h2"}, TargetAddresses: []string{"127.0.0.1"}, Disabled: true}),
	}
	for want, rule := range bad {
		err := rule.Validate()
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("期望包含 %q 的错误，得到 %v", want, err)
		}
	}
}

// disabled 字段缺省为启用：旧配置与不带该字段的 API 请求不受影响，启用的路由序列化时不写出该字段
func TestDisabledRouteJSON(t *testing.T) {
	var rt Route
	if err := json.Unmarshal([]byte(`{"name":"a","protocol":"ssh","targetAddresses":["127.0.0.1"]}`), &rt); err != nil {
		t.Fatal(err)
	}
	if rt.Disabled {
		t.Fatal("缺省应为启用")
	}
	b, _ := json.Marshal(rt)
	if strings.Contains(string(b), "disabled") {
		t.Fatalf("启用的路由不应写出 disabled: %s", b)
	}
	rt.Disabled = true
	b, _ = json.Marshal(rt)
	var back Route
	if err := json.Unmarshal(b, &back); err != nil || !back.Disabled {
		t.Fatalf("停用状态应能往返: %s %v", b, err)
	}
}

// 停用的路由不参与匹配：连接落到下一级更宽泛的路由或默认目标，统计仍与路由一一对应
func TestDisabledRouteMatch(t *testing.T) {
	rule := validRule(t, Rule{
		Protocols: []string{"tcp"}, ListenPorts: "443",
		TargetAddresses: []string{"127.0.0.9"},
		Routes: []Route{
			{Name: "exact", Domains: []string{"vpn.example.com"}, TargetAddresses: []string{"x"}, Disabled: true},
			{Name: "wild", Domains: []string{"*.example.com"}, TargetAddresses: []string{"x"}},
			{Name: "h2", Domains: []string{"*.example.com"}, ALPN: []string{"h2"}, TargetAddresses: []string{"x"}, Disabled: true},
			{Name: "http-old", Protocol: "http", Domains: []string{"files.example.com"}, TargetAddresses: []string{"x"}, Disabled: true},
			{Name: "http-new", Protocol: "http", Domains: []string{"files.example.com"}, TargetAddresses: []string{"x"}},
			{Name: "ssh", Protocol: "ssh", TargetAddresses: []string{"x"}, Disabled: true},
			{Name: "vnc", Protocol: "timeout", TargetAddresses: []string{"x"}, Disabled: true},
		},
	})
	rt := newRouter(rule)
	if len(rt.targets) != len(rule.Routes) {
		t.Fatalf("targets %d 条，路由 %d 条，统计会错位", len(rt.targets), len(rule.Routes))
	}
	cases := []struct {
		res  sniffResult
		want string
	}{
		{sniffResult{Proto: ProtoTLS, Host: "vpn.example.com"}, "wild"},
		{sniffResult{Proto: ProtoTLS, Host: "a.example.com", ALPN: []string{"h2"}}, "wild"},
		{sniffResult{Proto: ProtoHTTP, Host: "files.example.com"}, "http-new"},
		{sniffResult{Proto: ProtoSSH}, ""},
		{sniffResult{Proto: ProtoTimeout}, ""},
	}
	for _, c := range cases {
		got := rt.match(c.res)
		if got == nil || got.name != c.want {
			t.Fatalf("%v 命中 %v，期望 %q", c.res, got, c.want)
		}
	}
	if rt.match(sniffResult{Proto: ProtoSSH}) != rt.fallback {
		t.Fatal("停用的 SSH 路由应交给默认目标")
	}
}

// 端到端：经模块热更新切换路由开关，真实连接立即改走其他路由；停用期间计数保留，重新启用后继续累计
func TestModuleToggleRoute(t *testing.T) {
	a, b, def := tagEcho(t, "a"), tagEcho(t, "b"), tagEcho(t, "def")
	listenPort := freePort(t)
	rule := validRule(t, Rule{
		Key: "k1", Name: "edge", Enable: true,
		Protocols: []string{"tcp4"}, ListenAddress: "127.0.0.1", ListenPorts: strconv.Itoa(listenPort),
		TargetAddresses: []string{"127.0.0.1"}, TargetPorts: strconv.Itoa(def),
		Routes: []Route{
			{Name: "a", Protocol: "http", Domains: []string{"a.example.com"}, TargetAddresses: []string{"127.0.0.1"}, TargetPort: a},
			// 同一域名的备选路由，先停用
			{Name: "a-alt", Protocol: "http", Domains: []string{"a.example.com"}, TargetAddresses: []string{"127.0.0.1"}, TargetPort: b, Disabled: true},
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
	// 启动日志注明停用的路由数量
	if logs := fmt.Sprint(m.logs.Since(0)); !strings.Contains(logs, "2 条路由，1 条已停用") {
		t.Fatalf("启动日志未注明停用数量: %s", logs)
	}

	apply := func(disabledA, disabledAlt bool) {
		t.Helper()
		rule.Routes[0].Disabled = disabledA
		rule.Routes[1].Disabled = disabledAlt
		rule = validRule(t, rule)
		m.cfg.Rules = []Rule{rule}
		m.restartLocked("k1")
		if m.runners["k1"] != r {
			t.Fatal("切换路由开关应热更新而不是重启")
		}
	}
	reach := func(want string) {
		t.Helper()
		if got := hostReaches(t, listenPort, "a.example.com"); got != want {
			t.Fatalf("a.example.com 到达 %q，期望 %q", got, want)
		}
	}

	reach("a")
	reach("a")
	// 切到备选路由
	apply(true, false)
	reach("b")
	// 两条都停用：交给默认目标
	apply(true, true)
	reach("def")
	// 切回原路由，原计数继续累计
	apply(false, true)
	reach("a")

	snap := r.snapshot()
	if fmt.Sprint(snap.RouteConns) != "[3 1]" || snap.DefaultConns != 1 {
		t.Fatalf("路由计数 %v 默认 %d，期望 [3 1] 默认 1", snap.RouteConns, snap.DefaultConns)
	}
	samples := m.Metrics()
	if v := metricValue(samples, "edge", "a-alt"); v != 1 {
		t.Fatalf("停用路由的指标应保留，得到 %v", v)
	}
}
