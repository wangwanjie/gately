package gen

func (g *generator) portforward() error {
	r := func(key, name, group string, protos []string, listen, target string, tport string, extra obj) obj {
		o := obj{"key": key, "name": name, "enable": true, "group": group, "protocols": protos, "listenAddress": "",
			"listenPorts": listen, "autoFirewall": true, "targetAddresses": []string{target}, "targetPorts": tport,
			"maxConnections": 0, "maxConnsPerSource": 0, "udpSessionTimeout": 0, "proxyProtocol": "", "ipMode": "", "ipList": []string{}, "remark": ""}
		for k, v := range extra {
			o[k] = v
		}
		return o
	}
	pf := func(n int) string { return itoa(g.port(n)) }
	return g.write("portforward", obj{
		"settings": obj{"enable": true, "defaultMaxConnections": 256, "defaultUDPSessionTimeout": 60, "maxUDPSessions": 1024, "dialTimeout": 5, "firstDataTimeout": 60},
		"rules": list(
			r("pf-ssh", "SSH 跳板", "远程管理", strs("tcp"), pf(2222), "192.0.2.10", "22", obj{"ipMode": "whitelist", "ipList": strs("203.0.113.0/24"), "remark": "仅办公室出口可访问"}),
			r("pf-rdp", "Windows 远程桌面", "远程管理", strs("tcp", "udp"), pf(3389), "192.0.2.50", "3389", obj{"enable": false, "remark": "需要时再开"}),
			r("pf-mc", "Minecraft 服务器", "游戏", strs("tcp"), pf(25565), "192.0.2.60", "25565", obj{"proxyProtocol": "", "maxConnsPerSource": 4}),
			r("pf-mcb", "Minecraft 基岩版", "游戏", strs("udp"), pf(19132), "192.0.2.60", "19132", obj{"udpSessionTimeout": 120}),
			r("pf-wg", "WireGuard", "VPN", strs("udp"), pf(51820), "192.0.2.70", "51820", obj{}),
			r("pf-mqtt", "MQTT 代理", "物联网", strs("tcp"), pf(1883), "192.0.2.80", "1883", obj{"ipMode": "whitelist", "ipList": strs("192.0.2.0/24")}),
			r("pf-range", "下载端口段", "其他", strs("tcp"), pf(6881)+"-"+pf(6889), "192.0.2.90", pf(6881)+"-"+pf(6889), obj{}),
		),
	})
}

func (g *generator) protoroute() error {
	return g.write("protoroute", obj{
		"settings": obj{"enable": true, "defaultMaxConnections": 256, "dialTimeout": 5},
		"rules": list(obj{
			"key": "pr-443", "name": "443 端口协议分流", "enable": true, "group": "", "protocols": []string{"tcp"}, "listenAddress": "",
			"listenPorts": itoa(g.port(8444)), "autoFirewall": true, "sniffTimeout": 3,
			"routes": list(
				obj{"name": "SSH", "protocol": "ssh", "targetAddresses": []string{"192.0.2.10"}, "targetPort": 22},
				obj{"name": "博客 HTTPS", "protocol": "tls", "domains": []string{"blog.example.com"}, "targetAddresses": []string{"127.0.0.1"}, "targetPort": g.port(443)},
				obj{"name": "其余 TLS", "protocol": "tls", "alpn": []string{"h2", "http/1.1"}, "targetAddresses": []string{"192.0.2.21"}, "targetPort": 8443},
			),
			"targetAddresses": []string{"192.0.2.99"}, "targetPorts": "8080", "remark": "同一个端口同时提供 SSH 与 HTTPS",
		}),
	})
}

func (g *generator) ddns() error {
	rec := func(key, zone, sub, typ string) obj {
		return obj{"key": key, "enable": true, "zone": zone, "sub": sub, "type": typ, "value": "", "ttl": 600, "line": "", "remark": "", "group": ""}
	}
	src4 := obj{"enable": true, "type": "url", "urls": []string{"https://ipv4.example.com/ip"}, "regex": ""}
	src6 := obj{"enable": true, "type": "url", "urls": []string{"https://ipv6.example.com/ip"}, "regex": ""}
	off := obj{"enable": false, "type": "url", "urls": []string{}}
	task := func(key, name, group, provider, id string, v4, v6 obj, recs ...any) obj {
		return obj{"key": key, "name": name, "enable": true, "group": group, "ipv4": v4, "ipv6": v6, "provider": provider,
			"credentials": obj{"id": id, "secret": "demo-not-a-real-secret"}, "records": recs, "interval": 300, "forceInterval": 24,
			"httpTimeout": 10, "webhook": obj{"enable": false}, "remark": ""}
	}
	return g.write("ddns", obj{
		"settings": obj{"enable": true, "deleteRemote": false},
		"tasks": list(
			task("d-cf", "Cloudflare：example.com", "公网域名", "cloudflare", "demo@example.com", src4, src6,
				rec("r1", "example.com", "@", "A"), rec("r2", "example.com", "blog", "A"), rec("r3", "example.com", "home", "AAAA"), rec("r4", "example.com", "*.lab", "A")),
			task("d-ali", "阿里云：example.net", "公网域名", "alidns", "LTAIdemoAccessKeyId", src4, off,
				rec("r5", "example.net", "www", "A"), rec("r6", "example.net", "blog", "A")),
			task("d-dp", "DNSPod：example.org", "备用域名", "dnspod", "100001,demo0000000000000000000000000000", src4, src6,
				rec("r7", "example.org", "@", "A"), rec("r8", "example.org", "@", "AAAA")),
			task("d-duck", "DuckDNS", "备用域名", "duckdns", "example", src4, off,
				rec("r9", "duckdns.org", "example", "A")),
		),
	})
}

func (g *generator) stun() error {
	rule := func(key, name, proto string, port int, target string, tport int, extra obj) obj {
		o := obj{"key": key, "name": name, "group": "", "enable": true, "protocol": proto, "bind": obj{"mode": "any"}, "localPort": g.port(port),
			"useGlobalServers": true, "stunServers": []string{}, "keepAliveServers": []string{}, "targetAddresses": []string{target}, "targetPort": tport,
			"upnp": true, "autoFirewall": true, "heartbeatInterval": 25, "stunTimeout": 5, "retryInterval": 30, "autoRetry": true,
			"useGlobalWebhook": true, "remark": "", "ipMode": "", "ipList": []string{}}
		for k, v := range extra {
			o[k] = v
		}
		return o
	}
	return g.write("stun", obj{
		"settings": obj{"enable": true, "stunServers": []string{"stun.example.com:3478", "stun2.example.com:3478"},
			"keepAliveServers": []string{"keepalive.example.com:80"}, "addrCheckInterval": 60, "webhook": obj{"enable": false}},
		"rules": list(
			rule("st-web", "家宽 HTTPS 穿透", "tcp", 14433, "192.0.2.21", 443, obj{"remark": "NAT1 下打通 TCP，外网地址写进 DDNS"}),
			rule("st-bt", "下载 UDP 穿透", "udp", 16881, "192.0.2.90", 6881, obj{}),
			rule("st-game", "游戏联机", "udp", 19133, "192.0.2.60", 19132, obj{"enable": false}),
		),
	})
}

func (g *generator) frp() error {
	return g.write("frp", obj{
		"settings": obj{"frpcPath": "", "frpsPath": ""},
		"install":  obj{},
		"clients": list(
			obj{"key": "fc-hk", "name": "香港中转", "group": "", "enable": true, "serverAddr": "hk.example.com", "serverPort": 7000, "token": "demo-token-0000", "user": "",
				"protocol": "quic", "access": obj{"httpScheme": "https", "httpPort": 0, "httpsPort": 443, "subdomainHost": "hk.example.com"}, "remark": "家宽出口经香港对外"},
			obj{"key": "fc-us", "name": "美国备用", "group": "", "enable": false, "serverAddr": "us.example.com", "serverPort": 7000, "token": "demo-token-0001", "user": "", "protocol": "tcp", "access": obj{}, "remark": ""},
		),
		"servers": list(
			obj{"key": "fs-main", "name": "公网 frps", "group": "", "enable": true, "bindPort": g.port(7000), "kcpBindPort": 0, "quicBindPort": g.port(7001), "token": "demo-token-0002",
				"autoFirewall": true, "vhostHttpPort": 0, "vhostHttpsPort": 0, "subdomainHost": "frp.example.com", "allowPorts": "6000-6100", "dashboardPort": 0,
				"ipMode": "whitelist", "ipList": []string{"198.51.100.0/24", "203.0.113.0/24"}, "remark": "只允许自家机房的 frpc 登录"},
		),
		"proxies": list(
			obj{"key": "fp-ssh", "name": "家里 SSH", "group": "", "type": "tcp", "enable": true, "localIp": "192.0.2.10", "localPort": 22, "remotePort": 6022, "client": "fc-hk", "remark": ""},
			obj{"key": "fp-web", "name": "家里网站", "group": "", "type": "https", "enable": true, "localIp": "192.0.2.21", "localPort": 443, "customDomains": []string{"home.example.com"}, "client": "fc-hk", "remark": ""},
			obj{"key": "fp-nas", "name": "NAS 文件服务", "group": "", "type": "tcp", "enable": true, "localIp": "192.0.2.10", "localPort": 5000, "remotePort": 6050, "client": "fc-hk", "remark": ""},
		),
	})
}

func (g *generator) tailscale() error {
	return g.write("tailscale", obj{
		"settings": obj{"tailscaledPath": "", "tailscalePath": "", "installProxy": ""},
		"instances": list(
			obj{"key": "ts-main", "name": "家庭网络", "group": "", "enable": true, "manageDaemon": true, "stateDir": "", "userspace": true, "socks5": "127.0.0.1:1055",
				"authKey": "tskey-auth-demo-not-real", "hostname": "gately-home", "advertiseRoutes": []string{"192.0.2.0/24"}, "advertiseExitNode": true, "acceptRoutes": false,
				"acceptDns": false, "remark": "通告家庭网段并作为出口节点"},
		),
	})
}

func (g *generator) netbird() error {
	return g.write("netbird", obj{
		"settings": obj{"netbirdPath": "", "installProxy": ""},
		"instances": list(
			obj{"key": "nb-main", "name": "异地组网", "group": "", "enable": true, "manageDaemon": true, "stateDir": "", "userspace": true, "localForwarding": false,
				"socks5": "", "setupKey": "demo-setup-key-not-real", "managementUrl": "https://netbird.example.com:443", "hostname": "gately-home",
				"acceptRoutes": true, "serverRoutes": true, "acceptDns": false, "blockInbound": false, "remark": "自建管理端"},
		),
	})
}
