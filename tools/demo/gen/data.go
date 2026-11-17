package gen

import (
	"fmt"
	"math/rand"
	"net/netip"
	"os"
	"path/filepath"
	"time"

	"github.com/wangwanjie/gately/internal/visitors"
)

// visitors 写入访客审计数据：近两周的访问，来源地址全部取自文档保留网段。
func (g *generator) visitors() error {
	rng := rand.New(rand.NewSource(20260101))
	dir := filepath.Join(g.opt.Dir, "visitors")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	st := visitors.New(filepath.Join(dir, "visitors.jsonl.gz"), nil)
	st.Apply(visitors.Settings{Days: 30, Max: 100000})
	type site struct{ listener, key, name, host string }
	sites := []site{
		{"l-pub", "s-blog", "博客", "blog.example.com"}, {"l-pub", "s-photos", "相册", "photos.example.com"},
		{"l-pub", "s-git", "代码托管", "git.example.com"}, {"l-pub", "s-docs", "文档站", "docs.example.com"},
		{"l-pub", "s-home", "导航面板", "home.example.com"}, {"l-intra", "s-grafana", "监控面板", "grafana.example.com"},
		{"l-intra", "s-nas", "NAS 管理", "nas.example.com"},
	}
	paths := []string{"/", "/about/", "/posts/hello-world/", "/api/v1/status", "/assets/app.js", "/assets/style.css", "/login", "/favicon.ico", "/feed.xml", "/search"}
	probes := []string{"/.env", "/wp-login.php", "/.git/config", "/phpmyadmin/", "/actuator/health", "/admin"}
	uas := []string{
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1",
		"Mozilla/5.0 (X11; Linux x86_64; rv:125.0) Gecko/20100101 Firefox/125.0",
		"curl/8.5.0", "Mozilla/5.0 (compatible; ExampleBot/1.0; +https://example.com/bot)", "python-requests/2.31.0", "Go-http-client/2.0",
	}
	addr := func(i int) netip.Addr {
		switch i % 4 {
		case 0:
			return netip.AddrFrom4([4]byte{198, 51, 100, byte(1 + i%250)})
		case 1:
			return netip.AddrFrom4([4]byte{203, 0, 113, byte(1 + i%250)})
		case 2:
			return netip.AddrFrom4([4]byte{192, 0, 2, byte(2 + i%240)})
		}
		return netip.MustParseAddr(fmt.Sprintf("2001:db8:%x::%x", 1+i%30, 1+i))
	}
	for i := 0; i < 160; i++ {
		a := addr(i)
		scanner := i%9 == 0
		visits := 3 + rng.Intn(60)
		if scanner {
			visits = 8 + rng.Intn(12)
		}
		for v := 0; v < visits; v++ {
			s := sites[rng.Intn(len(sites))]
			t := g.now.Add(-time.Duration(rng.Int63n(int64(14 * 24 * time.Hour))))
			h := visitors.Hit{
				Addr: a, Time: t, Listener: s.listener, Site: s.key, SiteName: s.name, Host: s.host, Method: "GET",
				Path: paths[rng.Intn(len(paths))], Status: 200, Bytes: int64(2000 + rng.Intn(400000)), UA: uas[(i+v/20)%len(uas)],
			}
			switch {
			case scanner:
				h.Path = probes[rng.Intn(len(probes))]
				h.Status = 404
				h.UA = uas[4+i%4]
				if rng.Intn(3) == 0 {
					h.Status = 403
					h.Deny = []string{"waf", "ip", "ban"}[rng.Intn(3)]
				}
			case rng.Intn(12) == 0:
				h.Status = 404
			case rng.Intn(40) == 0:
				h.Status = 502
			case rng.Intn(15) == 0:
				h.Status, h.Method = 304, "GET"
			}
			if s.key == "s-photos" && !scanner {
				h.User = []string{"alice", "bob"}[rng.Intn(2)]
			}
			st.Record(h)
		}
	}
	return st.Save()
}

// logs 写入各模块与系统的运行日志，界面的日志页与总览据此有内容。
func (g *generator) logs() error {
	dir := filepath.Join(g.opt.Dir, "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	type line struct {
		ago   time.Duration
		level string
		src   string
		msg   string
	}
	m := time.Minute
	h := time.Hour
	files := map[string][]line{
		"gately": {
			{6 * h, "info", "", "管理界面监听 :16680"},
			{5*h + 30*m, "info", "auth", "admin 从 203.0.113.17 登录"},
			{3 * h, "warn", "auth", "来自 198.51.100.23 的登录失败: 用户名或密码错误"},
			{80 * m, "info", "settings", "全局设置已更新"},
		},
		"webservice": {
			{6 * h, "info", "公网 HTTPS 入口", fmt.Sprintf("HTTPS 监听 :%d（7 个站点）", g.port(443))},
			{6 * h, "info", "公网 HTTPS 入口", fmt.Sprintf("HTTP/3 监听 :%d（UDP）", g.port(443))},
			{6 * h, "info", "内网管理入口", fmt.Sprintf("HTTPS 监听 192.0.2.1:%d（3 个站点）", g.port(8443))},
			{2 * h, "info", "公网 HTTPS 入口", "站点配置已更新（7 个站点），监听未中断"},
		},
		"ddns": {
			{4 * h, "info", "Cloudflare：example.com", "IPv4 地址: - -> 203.0.113.10"},
			{4 * h, "info", "Cloudflare：example.com", "A blog.example.com 已更新 -> 203.0.113.10"},
			{50 * m, "info", "阿里云：example.net", "A www.example.net 已更新 -> 203.0.113.10"},
		},
		"ssl": {
			{6 * h, "info", "", "申请证书 *.example.com, example.com（CA: https://acme-v02.api.letsencrypt.org/directory）"},
		},
		"guard": {
			{3 * h, "warn", "", "198.51.100.23 已封禁 1 小时：触发扫描检测"},
			{90 * m, "warn", "", "203.0.113.201 已封禁 1 小时：SSH 暴力破解"},
		},
		"portforward": {
			{6 * h, "info", "SSH 跳板", fmt.Sprintf("tcp 0.0.0.0:%d -> 192.0.2.10:22 已启动", g.port(2222))},
			{6 * h, "info", "Minecraft 服务器", fmt.Sprintf("tcp 0.0.0.0:%d -> 192.0.2.60:25565 已启动", g.port(25565))},
		},
	}
	for name, ls := range files {
		var buf []byte
		for i, l := range ls {
			entry := obj{"seq": i + 1, "time": g.now.Add(-l.ago).Format(time.RFC3339Nano), "level": l.level, "message": l.msg}
			if l.src != "" {
				entry["source"] = l.src
			}
			b, err := jsonLine(entry)
			if err != nil {
				return err
			}
			buf = append(buf, b...)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".log"), buf, 0o600); err != nil {
			return err
		}
	}
	return nil
}
