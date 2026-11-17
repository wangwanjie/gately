package gen

import (
	"golang.org/x/crypto/bcrypt"
)

func (g *generator) waf() error {
	return g.write("waf", obj{"instances": list(
		obj{"key": "waf-main", "name": "站点防护（拦截）", "group": "", "enable": true, "mode": "block", "paranoia": 1,
			"inboundThreshold": 5, "outboundThreshold": 4, "bodyLimitMB": 0, "bodyLimitAction": "reject",
			"ruleSets": list(obj{"enable": true, "remark": "OWASP CRS", "type": "crs", "param": ""}),
			"remark":   "公网站点默认使用"},
		obj{"key": "waf-detect", "name": "仅检测（观察误报）", "group": "", "enable": true, "mode": "detect", "paranoia": 2,
			"inboundThreshold": 5, "outboundThreshold": 4, "bodyLimitMB": 64, "bodyLimitAction": "partial", "webdav": true,
			"ruleSets": list(obj{"enable": true, "remark": "OWASP CRS", "type": "crs", "param": ""}),
			"remark":   "网盘类站点先观察"},
	)})
}

func hash(pw string) string {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.MinCost)
	must(err)
	return string(b)
}

func site(key, name, group string, domains []string, typ string, extra obj) obj {
	s := obj{"key": key, "name": name, "enable": true, "group": group, "domains": domains, "type": typ,
		"targets": []string{}, "compress": true, "accessLog": true, "remark": ""}
	for k, v := range extra {
		s[k] = v
	}
	return s
}

func proxy(key, name, group string, domains []string, target string, extra obj) obj {
	e := obj{"targets": []string{target}, "forwardedHeaders": true}
	for k, v := range extra {
		e[k] = v
	}
	return site(key, name, group, domains, "proxy", e)
}

func (g *generator) webservice() error {
	pub := obj{
		"key": "l-pub", "name": "公网 HTTPS 入口", "enable": true, "network": "tcp", "listenIp": "", "port": g.port(443),
		"tls": true, "minTls": "1.2", "http3": true, "plainHttp": "redirect", "httpPort": g.port(80), "httpPortMode": "redirect",
		"autoFirewall": true, "remark": "对外提供博客、相册、文档与导航面板",
		"sites": list(
			proxy("s-blog", "博客", "对外站点", strs("blog.example.com", "www.example.com"), "http://192.0.2.21:2368", obj{
				"waf": "waf-main", "cache": obj{"enable": true, "ttl": 300}, "remark": "Ghost 博客"}),
			proxy("s-photos", "相册", "对外站点", strs("photos.example.com"), "http://192.0.2.22:2283", obj{
				"waf": "waf-detect", "webAuth": true, "sharedUsers": true, "remark": "登录后访问，使用共享认证用户"}),
			proxy("s-git", "代码托管", "对外站点", strs("git.example.com"), "http://192.0.2.23:3000", obj{
				"waf": "waf-main", "ipMode": "blacklist", "ipDenyList": strs("198.51.100.0/24"), "remark": "拒绝已知扫描网段"}),
			proxy("s-lab", "实验室（泛域名）", "对外站点", strs("*.lab.example.com"), "http://192.0.2.40:8080", obj{
				"waf": "waf-main", "lbPolicy": "least_conn", "targets": []string{"http://192.0.2.40:8080", "http://192.0.2.41:8080 weight=2", "http://192.0.2.42:8080 backup"}}),
			site("s-docs", "文档站", "对外站点", strs("docs.example.com"), "static", obj{"root": "/srv/docs", "cleanUrls": true, "listDir": false, "notFoundPage": "404.html"}),
			site("s-home", "导航面板", "对外站点", strs("home.example.com"), "panel", obj{"panel": "p-home"}),
			site("s-old", "旧地址跳转", "对外站点", strs("old.example.com"), "redirect", obj{"redirectTo": "https://blog.example.com", "redirectCode": 301, "keepPath": true}),
		),
		"default": obj{"key": "default", "name": "默认站点", "enable": true, "type": "close", "domains": []string{"*"}, "remark": "未知域名一律断开连接"},
	}
	intra := obj{
		"key": "l-intra", "name": "内网管理入口", "enable": true, "network": "tcp", "listenIp": "192.0.2.1", "port": g.port(8443),
		"tls": true, "minTls": "1.2", "http3": false, "plainHttp": "redirect", "remark": "只允许内网与 VPN 访问",
		"sites": list(
			proxy("s-nas", "NAS 管理", "内网", strs("nas.example.com"), "https://192.0.2.10:5001", obj{
				"insecureSkipVerify": true, "ipMode": "whitelist", "ipList": strs("192.0.2.0/24", "198.51.100.64/26", "2001:db8::/48")}),
			proxy("s-router", "路由器", "内网", strs("router.example.com"), "http://192.0.2.1:80", obj{
				"ipMode": "whitelist", "ipList": strs("192.0.2.0/24"), "basicAuth": true, "users": strs("admin")}),
			proxy("s-grafana", "监控面板", "内网", strs("grafana.example.com"), "http://192.0.2.31:3000", obj{
				"webAuth": true, "sharedUsers": true, "health": obj{"enable": true, "path": "/api/health", "interval": 30}}),
		),
		"default": obj{"key": "default", "name": "默认站点", "enable": true, "type": "close", "domains": []string{"*"}},
	}
	status := obj{
		"key": "l-status", "name": "状态页（明文 HTTP）", "enable": true, "network": "tcp", "listenIp": "", "port": g.port(8080),
		"tls": false, "remark": "对外提供简单的在线状态",
		"sites": list(
			site("s-status", "在线状态", "", strs("status.example.com"), "text", obj{"text": "OK", "contentType": "text/plain; charset=utf-8", "statusCode": 200}),
		),
		"default": obj{"key": "default", "name": "默认站点", "enable": true, "type": "close", "domains": []string{"*"}},
	}
	v6 := obj{
		"key": "l-v6", "name": "IPv6 入口（已停用）", "enable": false, "network": "tcp6", "listenIp": "", "port": g.port(9443),
		"tls": true, "minTls": "1.2", "remark": "等运营商开放 IPv6 入站后启用",
		"sites":   list(proxy("s-v6", "IPv6 测试", "", strs("v6.example.com"), "http://[2001:db8::21]:8080", nil)),
		"default": obj{"key": "default", "name": "默认站点", "enable": true, "type": "close", "domains": []string{"*"}},
	}
	return g.write("webservice", obj{
		"settings": obj{
			"enable": true, "authSecret": "demo-auth-secret-0000000000000000000000000000000000000000",
			"realIPFrom": []string{"203.0.113.0/24"}, "realIPHeaders": []string{"X-Forwarded-For"},
			"authUsers": list(
				obj{"id": "u1", "name": "alice", "password": hash("demo-password-alice"), "remark": "家人", "email": "alice@example.com", "displayName": "Alice"},
				obj{"id": "u2", "name": "bob", "password": hash("demo-password-bob"), "remark": "朋友", "email": "bob@example.com", "displayName": "Bob"},
			),
			"autoBanSeconds": 600,
		},
		"listeners": list(pub, intra, status, v6),
		"ipLists":   list(obj{"key": "ipl-office", "name": "办公室出口", "enable": true, "type": "static", "list": []string{"203.0.113.17", "203.0.113.18"}, "remark": ""}),
		"panels":    list(g.panel()),
		"visitors":  obj{"days": 30, "max": 100000},
		"replay":    obj{},
	})
}

func (g *generator) panel() obj {
	link := func(name, group, desc, url, icon string) obj {
		return obj{"name": name, "group": group, "description": desc, "url": url, "icon": icon}
	}
	return obj{
		"key": "p-home", "name": "家庭服务导航", "enable": true, "title": "家庭服务", "subtitle": "常用服务一页直达",
		"theme": "", "skin": "", "hideFooter": false, "defaultEngine": 0,
		"engines": list(obj{"name": "DuckDuckGo", "url": "https://duckduckgo.com/?q={q}"}, obj{"name": "Wikipedia", "url": "https://en.wikipedia.org/w/index.php?search={q}"}),
		"links": list(
			link("博客", "对外", "个人博客与笔记", "https://blog.example.com", "📝"),
			link("相册", "对外", "家庭照片，登录后访问", "https://photos.example.com", "🖼️"),
			link("代码托管", "对外", "私有 Git 仓库", "https://git.example.com", "🧰"),
			link("文档", "对外", "项目文档", "https://docs.example.com", "📚"),
			link("NAS", "内网", "存储与备份", "https://nas.example.com", "💾"),
			link("监控", "内网", "Grafana 仪表盘", "https://grafana.example.com", "📈"),
			link("路由器", "内网", "网关与 DHCP", "https://router.example.com", "🛜"),
		),
		"remark": "",
	}
}
