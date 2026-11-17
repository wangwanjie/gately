package webservice

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/i18n"
)

// npmSiteLine 把导入计划中的站点概括为一行，便于整体比较：端口、名称、类型与后端/跳转、认证、IP、缓存、响应头、分路径后端。
func npmSiteLine(port int, pl *NginxSitePlan, s *Site) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d %s", port, pl.Name)
	if pl.Default {
		b.WriteString(" default")
	}
	if pl.Skipped != "" {
		fmt.Fprintf(&b, " skipped(%s)", pl.Skipped)
		return b.String()
	}
	fmt.Fprintf(&b, " %s", s.Type)
	switch s.Type {
	case TypeProxy:
		fmt.Fprintf(&b, " %s", strings.Join(s.Targets, ","))
		if s.InsecureSkipVerify {
			b.WriteString(" insecure")
		}
	case TypeRedirect:
		fmt.Fprintf(&b, " %d %s", s.RedirectCode, s.RedirectTo)
		if s.KeepPath {
			b.WriteString(" keep")
		}
	case TypeText:
		fmt.Fprintf(&b, " %d", s.StatusCode)
	}
	if s.BasicAuth {
		fmt.Fprintf(&b, " basic(shared=%v)", s.SharedUsers)
	}
	if s.IPMode != "" {
		fmt.Fprintf(&b, " %s[%s]", s.IPMode, strings.Join(s.IPList, ","))
	}
	if len(s.Rules) > 0 {
		for _, r := range s.Rules {
			fmt.Fprintf(&b, " rule(%s %s[%s])", r.Prefix, r.IPMode, strings.Join(r.IPList, ","))
		}
	}
	if s.Cache.Enable {
		fmt.Fprintf(&b, " cache(%d)", len(s.Cache.Rules))
	}
	for _, h := range s.ResponseHeaders {
		fmt.Fprintf(&b, " resp(%s)", h)
	}
	for _, h := range s.RequestHeaders {
		fmt.Fprintf(&b, " req(%s)", h)
	}
	for _, l := range s.Locations {
		fmt.Fprintf(&b, " loc(%s>%s strip=%v hdr=%s)", l.Prefix, strings.Join(l.Targets, ","), l.StripPrefix, strings.Join(l.RequestHeaders, "|"))
	}
	if len(pl.Ignored) > 0 {
		fmt.Fprintf(&b, " ignored(%s)", strings.Join(pl.Ignored, ","))
	}
	return b.String()
}

// npmWarnLines 列出报告中的全部提示（报告级与各站点的），lang 非空时按该语言翻译（与接口返回时的翻译方式相同）。
func npmWarnLines(rep *NginxImportReport, lang string) []string {
	tr := func(s string) string {
		if lang == "" {
			return s
		}
		return i18n.Tr(lang, s)
	}
	var out []string
	for _, w := range rep.Warnings {
		out = append(out, "* "+tr(w))
	}
	for _, p := range rep.plans {
		for _, d := range p.sites {
			for _, w := range d.plan.Warnings {
				out = append(out, fmt.Sprintf("%d %s: %s", p.Port, d.plan.Name, tr(w)))
			}
		}
	}
	return out
}

func readNPMFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "npm", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestImportNPM 用 Nginx Proxy Manager 生成的配置做导入测试：tutorial-nginx-T.txt 为教程实测样本，
// npm-2.17-nginx-T.txt 与 npm-2.13-nginx-T.txt 为 NPM 2.17.0 与 2.13.5 容器中 nginx -T 的完整输出（ip_ranges.conf 只保留开头），
// 其余三份是 NPM 2.17.0 生成的站点文件（cat /data/nginx/proxy_host/*.conf，不含 conf.d/include 下的片段）：
// 带访问列表、带自定义 location，以及带 Let's Encrypt 证书、HTTP/2、HSTS 的站点（证书部分按 NPM 模板改为 Let's Encrypt 的写法）。
func TestImportNPM(t *testing.T) {
	const (
		basicWarn   = "已开启基本认证并使用共享用户，nginx 的用户文件不会被读取，请在全局设置中添加共享用户，或在站点中填写用户（用户名:密码，密码可为 bcrypt 哈希）"
		basicWarnEn = "Basic auth is enabled with shared users. The nginx user file is not read; add shared users in global settings or fill in users on the site (username:password, the password may be a bcrypt hash)"
		exploits    = "* Nginx Proxy Manager 的「Block Common Exploits」未导入：需要类似防护时，在站点「安全」页选择 WAF 实例"
		exploitsEn  = "* Nginx Proxy Manager's \"Block Common Exploits\" was not imported; for similar protection, choose a WAF instance on the site's Security tab"
		satisfy     = "satisfy any：192.168.1.0/24 在 nginx 中不用密码即可访问，Gately 不支持按 IP 免认证，导入后这些地址同样需要登录"
		satisfyEn   = "satisfy any: 192.168.1.0/24 can access without a password in nginx. Gately cannot exempt IPs from authentication, so after import these addresses must sign in too"
		timeout     = "location /files/ 的 proxy_read_timeout 未导入，站点的后端超时对所有路径生效"
		timeoutEn   = "proxy_read_timeout in location /files/ was not imported; the site's backend timeouts apply to all paths"
		cache       = " cache(15)"
		hstsSub     = " resp(Strict-Transport-Security: max-age=63072000;includeSubDomains; preload)"
		hstsPlain   = " resp(Strict-Transport-Security: max-age=63072000; preload)"
		toHTTPS     = " redirect 301 https://{host} keep"
		appSite     = " proxy https://192.168.1.20:8443 insecure basic(shared=true) req(Authorization: )" +
			" loc(/api>http://192.168.1.21:9000/v1/ strip=true hdr=Authorization:) loc(/files/>http://192.168.1.22:8080 strip=false hdr=Authorization:)"
		appWarn = "app.example.com: "
	)
	// 证书来源：没有任何证书，HTTPS 站点都会提示缺少证书
	noCerts := nameCerts{}
	certHint := func(dir, mount, rest string) string {
		return fmt.Sprintf("证书文件 %s 在 Nginx Proxy Manager 容器内，对应宿主机上 NPM 挂载到 %s 的目录中的 %s：可在「SSL 证书」页添加证书，来源选「读取文件」（Gately 在 Docker 中时先把该目录挂载进来），也可以直接重新签发", dir, mount, rest)
	}
	certHintEn := func(dir, mount, rest string) string {
		return fmt.Sprintf("The certificate files %s are inside the Nginx Proxy Manager container; on the host they are in %s under the directory NPM mounts at %s. Add the certificate on the SSL certificates page with the source \"Read from file\" (mount that directory first when Gately runs in Docker), or issue a new one", dir, rest, mount)
	}
	custom1 := certHint("/data/custom_ssl/npm-1/", "/data/", "custom_ssl/npm-1/")
	custom1En := certHintEn("/data/custom_ssl/npm-1/", "/data/", "custom_ssl/npm-1/")
	le1 := certHint("/etc/letsencrypt/live/npm-1/", "/etc/letsencrypt/", "live/npm-1/")
	le1En := certHintEn("/etc/letsencrypt/live/npm-1/", "/etc/letsencrypt/", "live/npm-1/")
	le2 := certHint("/etc/letsencrypt/live/npm-2/", "/etc/letsencrypt/", "live/npm-2/")
	le2En := certHintEn("/etc/letsencrypt/live/npm-2/", "/etc/letsencrypt/", "live/npm-2/")

	cases := []struct {
		file   string
		sites  []string
		warns  []string
		warnEn []string
	}{
		{
			file: "tutorial-nginx-T.txt",
			sites: []string{
				"80 nas.example.com" + toHTTPS,
				"80 photos.example.com" + toHTTPS,
				"80 old.example.com redirect 301 https://nas.example.com keep",
				"443 nas.example.com proxy http://192.168.1.10:5000 resp(Strict-Transport-Security: max-age=63072000; preload)",
				"443 photos.example.com proxy http://192.168.1.11:2283",
			},
			warns:  []string{exploits, "443 nas.example.com: " + le1, "443 photos.example.com: " + le1},
			warnEn: []string{exploitsEn, "443 nas.example.com: " + le1En, "443 photos.example.com: " + le1En},
		},
		{
			file: "npm-2.17-nginx-T.txt",
			sites: []string{
				"80 default-host.localhost default text 404",
				"80 nas.example.com" + toHTTPS,
				"80 app.example.com" + appSite,
				"80 lan.example.com proxy http://192.168.1.30:3000 whitelist[192.168.0.0/16]",
				"80 vault.example.com" + toHTTPS,
				"80 family.example.com proxy http://192.168.1.50:8096 basic(shared=true) req(Authorization: )",
				"80 media.example.com proxy http://192.168.1.60:8096 insecure" + cache + " loc(/api/>http://192.168.1.61:9000/v2/ strip=true hdr=) loc(/socket>https://192.168.1.62:8443 strip=false hdr=X-Custom: yes)",
				"80 old.example.com" + toHTTPS,
				"80 go.example.com redirect 302 https://www.example.org",
				"80 gone.example.com text 404",
				"443 nas.example.com proxy http://192.168.1.10:5000" + cache + hstsSub,
				"443 app.example.com" + appSite,
				"443 vault.example.com proxy http://192.168.1.40:8080 basic(shared=true)" + hstsPlain,
				"443 old.example.com redirect 301 https://nas.example.com keep" + hstsPlain,
			},
			warns: []string{
				"* /etc/nginx/nginx.conf:112：stream 块是 TCP/UDP 转发，请在端口转发中配置（SNI 分流对应 ssl_preread）", exploits,
				"80 " + appWarn + basicWarn, "80 " + appWarn + satisfy, "80 " + appWarn + timeout,
				"80 family.example.com: " + basicWarn, "80 family.example.com: " + satisfy,
				"443 nas.example.com: " + custom1,
				"443 " + appWarn + basicWarn, "443 " + appWarn + satisfy, "443 " + appWarn + timeout, "443 " + appWarn + custom1,
				"443 vault.example.com: " + basicWarn, "443 vault.example.com: " + custom1,
				"443 old.example.com: " + custom1,
			},
			warnEn: []string{
				"* /etc/nginx/nginx.conf:112: the stream block is TCP/UDP forwarding; configure it under Port Forwarding (ssl_preread maps to SNI routing)", exploitsEn,
				"80 " + appWarn + basicWarnEn, "80 " + appWarn + satisfyEn, "80 " + appWarn + timeoutEn,
				"80 family.example.com: " + basicWarnEn, "80 family.example.com: " + satisfyEn,
				"443 nas.example.com: " + custom1En,
				"443 " + appWarn + basicWarnEn, "443 " + appWarn + satisfyEn, "443 " + appWarn + timeoutEn, "443 " + appWarn + custom1En,
				"443 vault.example.com: " + basicWarnEn, "443 vault.example.com: " + custom1En,
				"443 old.example.com: " + custom1En,
			},
		},
		{
			// NPM 2.13.5：Host 用 $host、HSTS 用 $hsts_header，map 在每个站点文件里重复定义，以最后一个为准（与 nginx 相同）
			file: "npm-2.13-nginx-T.txt",
			sites: []string{
				"80 nas.example.com" + toHTTPS,
				"80 app.example.com" + appSite,
				"80 lan.example.com proxy http://192.168.1.30:3000 whitelist[192.168.0.0/16]",
				"80 old.example.com" + toHTTPS,
				"80 gone.example.com text 404",
				"443 nas.example.com proxy http://192.168.1.10:5000" + cache + hstsPlain,
				"443 app.example.com" + appSite,
				"443 old.example.com redirect 301 https://nas.example.com keep",
			},
			warns: []string{
				"* /etc/nginx/nginx.conf:88：stream 块是 TCP/UDP 转发，请在端口转发中配置（SNI 分流对应 ssl_preread）", exploits,
				"80 " + appWarn + basicWarn, "80 " + appWarn + satisfy, "80 " + appWarn + timeout,
				"443 nas.example.com: " + custom1,
				"443 " + appWarn + basicWarn, "443 " + appWarn + satisfy, "443 " + appWarn + timeout, "443 " + appWarn + custom1,
				"443 old.example.com: " + custom1,
			},
			warnEn: []string{
				"* /etc/nginx/nginx.conf:88: the stream block is TCP/UDP forwarding; configure it under Port Forwarding (ssl_preread maps to SNI routing)", exploitsEn,
				"80 " + appWarn + basicWarnEn, "80 " + appWarn + satisfyEn, "80 " + appWarn + timeoutEn,
				"443 nas.example.com: " + custom1En,
				"443 " + appWarn + basicWarnEn, "443 " + appWarn + satisfyEn, "443 " + appWarn + timeoutEn, "443 " + appWarn + custom1En,
				"443 old.example.com: " + custom1En,
			},
		},
		{
			// 访问列表：只有 IP（satisfy all）、只有密码、密码加 IP（satisfy any）
			file: "access-list.conf",
			sites: []string{
				"80 lan.example.com proxy http://192.168.1.30:3000 whitelist[192.168.0.0/16]",
				"80 vault.example.com" + toHTTPS,
				"80 family.example.com proxy http://192.168.1.50:8096 basic(shared=true) req(Authorization: )",
				"443 vault.example.com proxy http://192.168.1.40:8080 basic(shared=true)" + hstsPlain,
			},
			warns: []string{
				exploits,
				"80 family.example.com: " + basicWarn, "80 family.example.com: " + satisfy,
				"443 vault.example.com: " + basicWarn, "443 vault.example.com: " + custom1,
			},
			warnEn: []string{
				exploitsEn,
				"80 family.example.com: " + basicWarnEn, "80 family.example.com: " + satisfyEn,
				"443 vault.example.com: " + basicWarnEn, "443 vault.example.com: " + custom1En,
			},
		},
		{
			// 自定义 location：带路径的后端去掉前缀，https 后端不校验证书，location 里的请求头与访问列表
			file: "custom-location.conf",
			sites: []string{
				"80 media.example.com proxy http://192.168.1.60:8096 insecure" + cache + " loc(/api/>http://192.168.1.61:9000/v2/ strip=true hdr=) loc(/socket>https://192.168.1.62:8443 strip=false hdr=X-Custom: yes)",
				"80 app.example.com" + appSite,
				"443 app.example.com" + appSite,
			},
			warns: []string{
				exploits,
				"80 " + appWarn + basicWarn, "80 " + appWarn + satisfy, "80 " + appWarn + timeout,
				"443 " + appWarn + basicWarn, "443 " + appWarn + satisfy, "443 " + appWarn + timeout, "443 " + appWarn + custom1,
			},
			warnEn: []string{
				exploitsEn,
				"80 " + appWarn + basicWarnEn, "80 " + appWarn + satisfyEn, "80 " + appWarn + timeoutEn,
				"443 " + appWarn + basicWarnEn, "443 " + appWarn + satisfyEn, "443 " + appWarn + timeoutEn, "443 " + appWarn + custom1En,
			},
		},
		{
			// Let's Encrypt 证书、强制 HTTPS、HTTP/2、HSTS（含子域名）、缓存静态资源，以及 scheme 为 auto 的重定向主机
			file: "ssl-http2-hsts.conf",
			sites: []string{
				"80 nas.example.com" + toHTTPS,
				"80 old.example.com" + toHTTPS,
				"443 nas.example.com proxy http://192.168.1.10:5000" + cache + hstsSub,
				"443 old.example.com redirect 301 https://nas.example.com keep" + hstsPlain,
			},
			warns:  []string{exploits, "443 nas.example.com: " + le2, "443 old.example.com: " + le2},
			warnEn: []string{exploitsEn, "443 nas.example.com: " + le2En, "443 old.example.com: " + le2En},
		},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			rep, err := importNginx(readNPMFixture(t, tc.file), nil, noCerts)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, p := range rep.plans {
				for i := range p.sites {
					got = append(got, npmSiteLine(p.Port, p.sites[i].plan, &p.sites[i].site))
				}
			}
			if !slices.Equal(got, tc.sites) {
				t.Errorf("站点不符\n得到:\n%s\n期望:\n%s", strings.Join(got, "\n"), strings.Join(tc.sites, "\n"))
			}
			if rep.Skipped != 0 || rep.Sites != len(tc.sites) {
				t.Errorf("统计错误: sites=%d skipped=%d", rep.Sites, rep.Skipped)
			}
			if w := npmWarnLines(rep, ""); !slices.Equal(w, tc.warns) {
				t.Errorf("中文提示不符\n得到:\n%s\n期望:\n%s", strings.Join(w, "\n"), strings.Join(tc.warns, "\n"))
			}
			// 英文界面的提示与中文一一对应，且都有译文
			if w := npmWarnLines(rep, "en"); !slices.Equal(w, tc.warnEn) {
				t.Errorf("英文提示不符\n得到:\n%s\n期望:\n%s", strings.Join(w, "\n"), strings.Join(tc.warnEn, "\n"))
			}
			// 导入后的监听与站点都能通过校验
			for _, p := range rep.plans {
				l := Listener{Port: p.Port, TLS: p.TLS}
				for _, d := range p.sites {
					if d.plan.Default {
						l.Default = d.site
					} else {
						l.Sites = append(l.Sites, d.site)
					}
				}
				if err := l.Validate(); err != nil {
					t.Errorf("监听 :%d 校验失败: %v", p.Port, err)
				}
			}
		})
	}
}

// TestImportNginxSetVars 覆盖 set 变量的代入规则：location 中的 set 覆盖 server 中的，if 中赋值的变量不代入，
// 含变量且带路径的 proxy_pass 给出提示，只有默认值的 map 可以代入。
func TestImportNginxSetVars(t *testing.T) {
	conf := `
map $host $up_scheme { default http; }
map $host $pick { default 10.0.0.9; a.example.com 10.0.0.8; }
server {
    listen 80;
    server_name a.example.com;
    set $backend 10.0.0.1;
    set $port 8080;
    location / { proxy_pass $up_scheme://$backend:$port$request_uri; }
    location /api/ {
        set $backend 10.0.0.2;
        proxy_pass http://$backend:9000;
    }
}
server {
    listen 80;
    server_name b.example.com;
    set $backend 10.0.0.3;
    if ($http_x_beta) { set $backend 10.0.0.4; }
    location / { proxy_pass http://$backend; }
}
server {
    listen 80;
    server_name c.example.com;
    set $backend 10.0.0.5;
    location / { proxy_pass http://$backend/app/; }
}
server {
    listen 80;
    server_name d.example.com;
    location / { proxy_pass http://$pick; }
}
server {
    listen 80;
    server_name e.example.com;
    set $target example.org;
    return 301 https://$target$request_uri;
}`
	rep, err := importNginx(conf, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, pl, s := findSite(rep, "a.example.com")
	if s.Type != TypeProxy || strings.Join(s.Targets, ",") != "http://10.0.0.1:8080" || len(s.Locations) != 1 ||
		strings.Join(s.Locations[0].Targets, ",") != "http://10.0.0.2:9000" || len(pl.Warnings) != 0 {
		t.Fatalf("server 与 location 的 set 代入错误: %+v %v", s, pl.Warnings)
	}
	// if 中赋值的变量随请求变化，仍按无法转换处理
	_, pl, _ = findSite(rep, "b.example.com")
	if pl.Skipped == "" || !slices.Contains(pl.Warnings, "proxy_pass http://$backend 使用了变量，无法转换") {
		t.Fatalf("if 中赋值的变量不应代入: %+v", pl)
	}
	_, pl, s = findSite(rep, "c.example.com")
	if strings.Join(s.Targets, ",") != "http://10.0.0.5/app/" ||
		!slices.Contains(pl.Warnings, "proxy_pass http://$backend/app/ 含变量且带路径 /app/：nginx 会把每个请求的路径都换成它，导入后只替换 location 的前缀，请确认") {
		t.Fatalf("含变量且带路径的 proxy_pass 应提示: %+v %v", s.Targets, pl.Warnings)
	}
	// 有多项的 map 取值随请求变化，不代入默认值
	_, pl, _ = findSite(rep, "d.example.com")
	if pl.Skipped == "" {
		t.Fatalf("多项 map 不应代入默认值: %+v", pl)
	}
	_, _, s = findSite(rep, "e.example.com")
	if s.Type != TypeRedirect || s.RedirectTo != "https://example.org" || !s.KeepPath {
		t.Fatalf("跳转目标中的 set 变量应代入: %+v", s)
	}
}

// TestParseNginxIncludeSuffix 验证 nginx -T 中的文件不在主配置目录下时，相对 include 按路径结尾匹配。
func TestParseNginxIncludeSuffix(t *testing.T) {
	conf := `# configuration file /etc/nginx/nginx.conf:
http { include conf.d/*.conf; }
# configuration file /opt/nginx/conf.d/site.conf:
server { listen 80; server_name a.example.com; location / { include snippets/proxy.conf; } }
# configuration file /opt/nginx/snippets/proxy.conf:
proxy_pass http://127.0.0.1:8080;
`
	rep, err := importNginx(conf, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, s := findSite(rep, "a.example.com")
	if s == nil || s.Type != TypeProxy || s.Targets[0] != "http://127.0.0.1:8080" {
		t.Fatalf("相对 include 应按路径结尾找到文件: %+v", s)
	}
}

// TestImportNPMBuiltinServers 验证 NPM 容器自带的占位页、443 兜底与管理界面不会出现在导入计划中，
// 只粘贴站点文件、没有 conf.d/include 片段时用内置片段补全。
func TestImportNPMBuiltinServers(t *testing.T) {
	rep, err := importNginx(readNPMFixture(t, "npm-2.17-nginx-T.txt"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range rep.Listeners {
		if l.Port == 81 {
			t.Fatalf("不应导入 NPM 管理界面的 81 端口: %+v", l)
		}
		for _, s := range l.Sites {
			if strings.Contains(s.Source, "/etc/nginx/conf.d/") {
				t.Fatalf("不应导入 NPM 自带的站点: %+v", s)
			}
		}
	}
	// 只有 NPM 自带的 server 时提示到 NPM 容器中执行 nginx -T
	only := "# configuration file /etc/nginx/conf.d/default.conf:\nserver { listen 80; set $forward_scheme http; server_name localhost-nginx-proxy-manager; location / { root /var/www/html; } }\n"
	if _, err := importNginx(only, nil, nil); err == nil || !strings.Contains(err.Error(), "Nginx Proxy Manager") {
		t.Fatalf("应提示没有 NPM 站点: %v", err)
	}
}
