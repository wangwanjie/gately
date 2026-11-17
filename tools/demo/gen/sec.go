package gen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

func (g *generator) guard() error {
	if err := g.write("guard", obj{
		"settings": obj{
			"firewall": true, "banSeconds": 3600, "increment": true, "maxBanSeconds": 604800, "ignorePrivate": true,
			"ignoreList": []string{"203.0.113.17"},
			"trap":       obj{"enable": true, "ports": "2323,3306,6379,27017", "banSeconds": 86400},
			"scan":       obj{"enable": true, "threshold": 8, "excludePorts": "", "banSeconds": 86400},
		},
		"jails": list(
			obj{"key": "j-ssh", "name": "SSH 暴力破解", "group": "", "enable": true, "source": "file", "target": "/var/log/auth.log", "preset": "sshd",
				"filter": `Failed password for .* from <HOST> port \d+`, "maxRetry": 5, "findTime": 600, "banTime": 3600, "remark": ""},
			obj{"key": "j-nginx", "name": "常见漏洞路径探测", "group": "", "enable": true, "source": "file", "target": "/var/log/nginx/access.log", "preset": "nginx-botsearch",
				"filter": `^<HOST> \S+ \S+ \[[^\]]+\] "(?:GET|POST|HEAD) [^"]*(?:wp-login\.php|/\.env|/\.git/)[^"]*" (?:400|401|403|404) `, "maxRetry": 3, "findTime": 600, "banTime": 86400, "remark": ""},
		),
	}); err != nil {
		return err
	}
	type ban struct {
		Key     string    `json:"key"`
		Until   time.Time `json:"until,omitzero"`
		Created time.Time `json:"created"`
		Reason  string    `json:"reason"`
		Source  string    `json:"source"`
		Count   int       `json:"count"`
	}
	h := time.Hour
	bans := []ban{
		{"198.51.100.23", g.now.Add(50 * time.Minute), g.now.Add(-10 * time.Minute), "触发扫描检测：8 秒内访问 12 个端口", "scan", 1},
		{"198.51.100.77", g.now.Add(23 * h), g.now.Add(-1 * h), "访问诱捕端口 3306", "trap", 2},
		{"203.0.113.201", g.now.Add(2 * h), g.now.Add(-30 * time.Minute), "SSH 暴力破解：10 分钟内失败 5 次", "jail:j-ssh", 1},
		{"203.0.113.202", g.now.Add(70 * h), g.now.Add(-2 * h), "漏洞路径探测：/.env", "jail:j-nginx", 3},
		{"198.51.100.0/26", time.Time{}, g.now.Add(-72 * h), "长期扫描的网段，手动封禁", "manual", 1},
		{"2001:db8:bad::/64", g.now.Add(40 * h), g.now.Add(-5 * h), "WAF 多次拦截：SQL 注入", "webservice", 1},
		{"192.0.2.250", g.now.Add(15 * time.Minute), g.now.Add(-45 * time.Minute), "认证连续失败", "webservice", 1},
		{"203.0.113.88", g.now.Add(5 * h), g.now.Add(-20 * time.Minute), "访问诱捕端口 6379", "trap", 1},
	}
	dir := filepath.Join(g.opt.Dir, "guard")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(obj{"bans": bans, "offenses": obj{}})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "bans.json"), b, 0o600)
}

func (g *generator) knock() error {
	return g.write("knock", obj{
		"settings": obj{"port": g.port(62201), "autoFirewall": true, "allowLan": true, "allowList": []string{}, "maxAge": 120},
		"rules": list(
			obj{"key": "k-ssh", "name": "SSH 敲门", "enable": true, "ports": []string{"tcp/22"}, "firewall": true, "encKey": "ZGVtby1ub3QtcmVhbC1rZXk=", "hmacKey": "ZGVtby1ub3QtcmVhbC1obWFj",
				"hmacType": "sha256", "timeout": 30, "maxTimeout": 3600, "requireAddr": false, "remark": "fwknop 兼容，授权 30 秒"},
		),
	})
}

func (g *generator) notify() error {
	hook := func(u string) obj {
		return obj{"enable": true, "url": u, "method": "POST", "headers": []string{"Content-Type: application/json"}, "body": `{"text":"{{.Title}}"}`, "timeout": 10}
	}
	return g.write("notify", obj{
		"settings": obj{"proxy": "", "serverName": "gately-demo"},
		"channels": list(
			obj{"key": "n-tg", "name": "Telegram 个人", "group": "", "enable": true, "kind": "telegram", "secret": "123456:demo-not-a-real-bot-token", "target": "100000001", "webhook": obj{"enable": false}, "remark": ""},
			obj{"key": "n-bark", "name": "Bark（iPhone）", "group": "", "enable": true, "kind": "bark", "url": "https://bark.example.com", "secret": "demoBarkDeviceKey", "webhook": obj{"enable": false}, "remark": ""},
			obj{"key": "n-mail", "name": "邮件", "group": "", "enable": true, "kind": "smtp", "host": "smtp.example.com", "port": 465, "security": "tls", "user": "alerts@example.com",
				"secret": "demo-not-a-real-password", "from": "Gately <alerts@example.com>", "target": "admin@example.com", "webhook": obj{"enable": false}, "remark": ""},
			obj{"key": "n-hook", "name": "团队 Webhook", "group": "", "enable": false, "kind": "webhook", "webhook": hook("https://hooks.example.com/gately"), "remark": ""},
		),
		"rules": list(
			obj{"key": "nr-sec", "name": "安全事件", "group": "", "enable": true, "events": []string{"guard.banned", "auth.failed", "auth.banned", "waf.blocked"}, "minLevel": "warn", "channels": []string{"n-tg", "n-bark"}, "silence": 300},
			obj{"key": "nr-ops", "name": "运维事件", "group": "", "enable": true, "events": []string{"ddns.failed", "ssl.expiring", "upstream.down", "upstream.up", "stun.changed"}, "minLevel": "info", "channels": []string{"n-mail", "n-tg"}, "silence": 600},
		),
	})
}

func (g *generator) cron() error {
	return g.write("cron", obj{"tasks": list(
		obj{"key": "cr-clean", "name": "清理临时文件", "group": "维护", "enable": true, "spec": "0 3 * * *", "type": "command", "command": "find /tmp -mtime +7 -delete", "http": obj{"enable": false}, "remark": "每天 03:00"},
		obj{"key": "cr-ping", "name": "心跳上报", "group": "监控", "enable": true, "spec": "@every 5m", "type": "http", "command": "",
			"http": obj{"enable": true, "url": "https://hc.example.com/ping/demo", "method": "GET", "timeout": 10}, "remark": ""},
		obj{"key": "cr-backup", "name": "每周备份通知", "group": "维护", "enable": false, "spec": "30 4 * * 0", "type": "command", "command": "echo backup", "http": obj{"enable": false}, "remark": ""},
	)})
}

func (g *generator) perf() error {
	return g.write("perf", obj{"rules": list(
		obj{"key": "pf1", "name": "CPU 持续偏高", "group": "", "enable": true, "metric": "host.cpu", "op": ">", "threshold": 85, "duration": 5, "level": "warn", "remark": ""},
		obj{"key": "pf2", "name": "磁盘空间不足", "group": "", "enable": true, "metric": "host.disk", "op": ">", "threshold": 90, "duration": 10, "level": "error", "remark": ""},
		obj{"key": "pf3", "name": "站点 5xx 过多", "group": "", "enable": true, "metric": "site.err5xx", "op": ">", "threshold": 5, "duration": 3, "minRequests": 20, "sites": []string{}, "level": "warn", "remark": ""},
	)})
}

func (g *generator) wol() error {
	return g.write("wol", obj{
		"bemfa": obj{},
		"devices": list(
			obj{"key": "w-nas", "name": "NAS", "group": "", "enable": true, "macs": []string{"02:00:5e:00:53:01"}, "broadcastIps": []string{"192.0.2.255"}, "port": 9, "repeat": 3,
				"checkAddr": "192.0.2.10:22", "shutdownMode": "http", "shutdownHttp": obj{"method": "POST", "url": "http://192.0.2.10:8080/api/shutdown", "headers": []string{}, "body": ""}, "remark": "每天夜里休眠"},
			obj{"key": "w-pc", "name": "台式机", "group": "", "enable": true, "macs": []string{"02:00:5e:00:53:02"}, "broadcastIps": []string{"192.0.2.255"}, "port": 9, "repeat": 3,
				"checkAddr": "192.0.2.50:3389", "shutdownMode": "", "remark": ""},
		),
	})
}

func (g *generator) misc() error { return nil }
