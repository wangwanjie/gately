package protoroute

import (
	"strings"
	"testing"
)

func TestSNIRuleValidate(t *testing.T) {
	base := func() Rule {
		return Rule{Protocols: []string{"tcp"}, ListenPorts: "443",
			Routes: []Route{{Domains: []string{"a.example.com"}, TargetAddresses: []string{"127.0.0.1"}, TargetPort: 8443, ProxyProtocol: "v2"}}}
	}
	r := base()
	if err := r.Validate(); err != nil {
		t.Fatalf("合法规则被拒绝: %v", err)
	}
	bad := map[string]func(*Rule){
		"UDP": func(r *Rule) { r.Protocols = []string{"tcp", "udp"} },
		"重复域名": func(r *Rule) {
			r.Routes = append(r.Routes, Route{Domains: []string{"A.example.com"}, TargetAddresses: []string{"x"}})
		},
		"无目标":     func(r *Rule) { r.Routes[0].TargetAddresses = nil },
		"错误通配":    func(r *Rule) { r.Routes[0].Domains = []string{"a.*.com"} },
		"端口越界":    func(r *Rule) { r.Routes[0].TargetPort = 70000 },
		"PROXY版本": func(r *Rule) { r.Routes[0].ProxyProtocol = "v3" },
		"空规则":     func(r *Rule) { r.Routes = nil },
		"连接数为负":   func(r *Rule) { r.MaxConnections = -1 },
	}
	for name, mutate := range bad {
		r := base()
		mutate(&r)
		if r.Validate() == nil {
			t.Fatalf("%s: 应校验失败", name)
		}
	}
	// 只有默认目标也合法，等同于普通转发
	r = base()
	r.Routes = nil
	r.TargetAddresses = []string{"127.0.0.1"}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestProtocolRuleValidate(t *testing.T) {
	route := func(proto string) Route {
		return Route{Protocol: proto, TargetAddresses: []string{"127.0.0.1"}}
	}
	base := func(routes ...Route) Rule {
		return Rule{Protocols: []string{"tcp"}, ListenPorts: "443", Routes: routes}
	}
	ok := map[string]Rule{
		"全部协议各一条": base(route("tls"), route("http"), route("ssh"), route("rdp"), route("socks"), route("openvpn"),
			route("xmpp"), route("mqtt"), route("postgresql"), route("rtsp"), route("sip"), route("minecraft"), route("redis"),
			route("amqp"), route("smb"), route("git"), route("bittorrent"), route("adb"), route("irc"), route("timeout")),
		"Minecraft 按域名": base(
			Route{Protocol: "minecraft", Domains: []string{"a.mc.com"}, TargetAddresses: []string{"x"}},
			Route{Protocol: "minecraft", Domains: []string{"*"}, TargetAddresses: []string{"x"}},
			Route{Protocol: "minecraft", TargetAddresses: []string{"x"}}),
		"识别超时带小数": {Protocols: []string{"tcp"}, ListenPorts: "443", SniffTimeout: 0.3, Routes: []Route{route("ssh")}},
		"同域名不同 ALPN": base(
			Route{Protocol: "tls", Domains: []string{"a.com"}, ALPN: []string{"h2"}, TargetAddresses: []string{"x"}},
			Route{Protocol: "tls", Domains: []string{"a.com"}, ALPN: []string{"http/1.1"}, TargetAddresses: []string{"x"}},
			Route{Protocol: "tls", Domains: []string{"a.com"}, TargetAddresses: []string{"x"}}),
		"TLS 与 HTTP 同域名": base(
			Route{Protocol: "tls", Domains: []string{"a.com"}, TargetAddresses: []string{"x"}},
			Route{Protocol: "http", Domains: []string{"a.com"}, TargetAddresses: []string{"x"}}),
	}
	for name, r := range ok {
		if err := r.Validate(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	bad := map[string]Rule{
		"未知协议":             base(route("ftp")),
		"SSH 填域名":          base(Route{Protocol: "ssh", Domains: []string{"a.com"}, TargetAddresses: []string{"x"}}),
		"RTSP 填域名":         base(Route{Protocol: "rtsp", Domains: []string{"a.com"}, TargetAddresses: []string{"x"}}),
		"Minecraft 填 ALPN": base(Route{Protocol: "minecraft", ALPN: []string{"h2"}, TargetAddresses: []string{"x"}}),
		"Minecraft 同域名": base(Route{Protocol: "minecraft", Domains: []string{"a.com"}, TargetAddresses: []string{"x"}},
			Route{Protocol: "minecraft", Domains: []string{"A.com"}, TargetAddresses: []string{"x"}}),
		"识别超时过短":      {Protocols: []string{"tcp"}, ListenPorts: "443", SniffTimeout: 0.05, Routes: []Route{route("ssh")}},
		"HTTP 填 ALPN": base(Route{Protocol: "http", ALPN: []string{"h2"}, TargetAddresses: []string{"x"}}),
		"SSH 两条":      base(route("ssh"), route("ssh")),
		"等待超时两条":      base(route("timeout"), route("timeout")),
		"两条全部 TLS":    base(route("tls"), route("")),
		"同域名同 ALPN": base(
			Route{Protocol: "tls", Domains: []string{"a.com"}, ALPN: []string{"h2", "x"}, TargetAddresses: []string{"x"}},
			Route{Protocol: "tls", Domains: []string{"A.com"}, ALPN: []string{"h2"}, TargetAddresses: []string{"x"}}),
		"未填域名同 ALPN": base(
			Route{Protocol: "tls", ALPN: []string{"acme-tls/1"}, TargetAddresses: []string{"x"}},
			Route{Protocol: "tls", ALPN: []string{"acme-tls/1"}, TargetAddresses: []string{"x"}}),
		"ALPN 含空格": base(Route{Protocol: "tls", ALPN: []string{"h2 http/1.1"}, TargetAddresses: []string{"x"}}),
		"识别超时为负":   {Protocols: []string{"tcp"}, ListenPorts: "443", SniffTimeout: -1, Routes: []Route{route("ssh")}},
		"识别超时过长":   {Protocols: []string{"tcp"}, ListenPorts: "443", SniffTimeout: 61, Routes: []Route{route("ssh")}},
	}
	for name, r := range bad {
		if err := r.Validate(); err == nil {
			t.Fatalf("%s: 应校验失败", name)
		}
	}

	// 规范化：协议转小写、空协议补成 tls、域名与 ALPN 去重去空白
	r := base(Route{Protocol: " TLS ", Domains: []string{"A.com", "a.com.", " "}, ALPN: []string{" h2", "h2", ""}, TargetAddresses: []string{"x"}},
		Route{Domains: []string{"b.com"}, TargetAddresses: []string{"x"}})
	r.SniffTimeout = 2
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if r.Routes[0].Protocol != "tls" || strings.Join(r.Routes[0].Domains, ",") != "a.com" || strings.Join(r.Routes[0].ALPN, ",") != "h2" || r.Routes[1].Protocol != "tls" {
		t.Fatalf("规范化结果: %+v", r.Routes)
	}
}
