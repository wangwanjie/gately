package protoroute

import "testing"

func TestSNIRouterMatch(t *testing.T) {
	rule := Rule{
		Protocols:       []string{"tcp"},
		ListenPorts:     "443",
		TargetAddresses: []string{"127.0.0.9"},
		Routes: []Route{
			{Name: "exact", Domains: []string{"VPN.example.com."}, TargetAddresses: []string{"127.0.0.1"}},
			{Name: "wild", Domains: []string{"*.example.com"}, TargetAddresses: []string{"127.0.0.2"}},
			{Name: "deep", Domains: []string{"*.home.example.com"}, TargetAddresses: []string{"127.0.0.3"}},
			{Name: "any", Domains: []string{"*"}, TargetAddresses: []string{"127.0.0.4"}},
		},
	}
	if err := rule.Validate(); err != nil {
		t.Fatal(err)
	}
	rt := newRouter(rule)
	cases := map[string]string{
		"vpn.example.com":      "exact",
		"a.example.com":        "wild",
		"a.b.example.com":      "wild",
		"nas.home.example.com": "deep",
		"example.com":          "any",
		"other.org":            "any",
		"":                     "",
	}
	for host, want := range cases {
		got := rt.match(sniffResult{Proto: ProtoTLS, Host: host})
		if got == nil || got.name != want {
			t.Fatalf("%q 命中 %v，期望 %q", host, got, want)
		}
	}
	// 没有 SNI 时走默认目标
	if rt.match(sniffResult{Proto: ProtoTLS}) != rt.fallback {
		t.Fatal("无 SNI 应走默认目标")
	}
	// TLS 路由只匹配 TLS 连接，携带相同域名的 HTTP 连接走默认目标
	if rt.match(sniffResult{Proto: ProtoHTTP, Host: "vpn.example.com"}) != rt.fallback {
		t.Fatal("HTTP 连接不应命中 TLS 路由")
	}
}

func TestProtocolRouterMatch(t *testing.T) {
	rule := Rule{
		Protocols: []string{"tcp"}, ListenPorts: "443",
		TargetAddresses: []string{"127.0.0.9"},
		Routes: []Route{
			{Name: "web", Protocol: "tls", Domains: []string{"*.example.com"}, TargetAddresses: []string{"x"}},
			{Name: "web-h2", Protocol: "tls", Domains: []string{"*.example.com"}, ALPN: []string{"h2"}, TargetAddresses: []string{"x"}},
			{Name: "acme", Protocol: "TLS", ALPN: []string{"acme-tls/1"}, TargetAddresses: []string{"x"}},
			{Name: "pg-tls", Protocol: "tls", Domains: []string{"db.example.com"}, ALPN: []string{"postgresql"}, TargetAddresses: []string{"x"}},
			{Name: "tls-all", Protocol: "tls", TargetAddresses: []string{"x"}},
			{Name: "http-files", Protocol: "http", Domains: []string{"files.example.com"}, TargetAddresses: []string{"x"}},
			{Name: "http-all", Protocol: "http", TargetAddresses: []string{"x"}},
			{Name: "ssh", Protocol: "ssh", TargetAddresses: []string{"x"}},
			{Name: "vnc", Protocol: "timeout", TargetAddresses: []string{"x"}},
		},
	}
	if err := rule.Validate(); err != nil {
		t.Fatal(err)
	}
	rt := newRouter(rule)
	cases := []struct {
		res  sniffResult
		want string
	}{
		{sniffResult{Proto: ProtoTLS, Host: "a.example.com"}, "web"},
		{sniffResult{Proto: ProtoTLS, Host: "a.example.com", ALPN: []string{"h2", "http/1.1"}}, "web-h2"},
		{sniffResult{Proto: ProtoTLS, Host: "a.example.com", ALPN: []string{"http/1.1"}}, "web"},
		// 精确域名只有限定 ALPN 的路由，ALPN 不匹配时继续找通配
		{sniffResult{Proto: ProtoTLS, Host: "db.example.com", ALPN: []string{"postgresql"}}, "pg-tls"},
		{sniffResult{Proto: ProtoTLS, Host: "db.example.com", ALPN: []string{"h2"}}, "web-h2"},
		// 未填域名的 ALPN 路由：TLS-ALPN-01 验证不论域名都交给它，但具体域名的 ALPN 路由优先
		{sniffResult{Proto: ProtoTLS, Host: "other.org", ALPN: []string{"acme-tls/1"}}, "acme"},
		{sniffResult{Proto: ProtoTLS, Host: "other.org"}, "tls-all"},
		{sniffResult{Proto: ProtoTLS}, "tls-all"},
		{sniffResult{Proto: ProtoHTTP, Host: "files.example.com"}, "http-files"},
		{sniffResult{Proto: ProtoHTTP, Host: "a.example.com"}, "http-all"},
		{sniffResult{Proto: ProtoHTTP}, "http-all"},
		{sniffResult{Proto: ProtoSSH}, "ssh"},
		{sniffResult{Proto: ProtoTimeout}, "vnc"},
		// 没有对应路由的协议与无法识别的数据走默认目标
		{sniffResult{Proto: ProtoRDP}, ""},
		{sniffResult{}, ""},
	}
	for _, c := range cases {
		got := rt.match(c.res)
		if got == nil || got.name != c.want {
			t.Fatalf("%+v 命中 %v，期望 %q", c.res, got, c.want)
		}
	}

	// 未经校验的旧配置：协议为空的路由按 TLS 处理
	old := newRouter(Rule{Routes: []Route{{Name: "legacy", Domains: []string{"a.example.com"}, TargetAddresses: []string{"x"}}}})
	if got := old.match(sniffResult{Proto: ProtoTLS, Host: "a.example.com"}); got == nil || got.name != "legacy" {
		t.Fatalf("旧配置路由: %v", got)
	}
	if old.match(sniffResult{Proto: ProtoTimeout}) != nil {
		t.Fatal("没有默认目标时应返回 nil")
	}
}
