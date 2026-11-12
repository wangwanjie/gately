package webservice

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/i18n"
)

// nginx 导入的上限
const maxNginxConfig = 4 << 20

// NginxImportRequest 是导入 nginx 配置的请求。
type NginxImportRequest struct {
	// Config 为 nginx 配置文本，推荐使用 nginx -T 的完整输出（包含 include 的文件）
	Config string `json:"config"`
	// Apply 为 false 时只预览转换结果，不修改配置
	Apply bool `json:"apply"`
	// Enable 为 true 时导入的站点与新建的监听立即启用，否则导入为停用状态，确认后再手动启用
	Enable bool `json:"enable"`
}

// NginxImportReport 是转换结果。
type NginxImportReport struct {
	Listeners []NginxListenerPlan `json:"listeners"`
	// Warnings 为与具体站点无关的提示，例如 stream 转发、无法解析的 upstream
	Warnings []string `json:"warnings"`
	Sites    int      `json:"sites"`
	Skipped  int      `json:"skipped"`
	Applied  bool     `json:"applied"`
	// plans 为应用时使用的完整计划（含待导入的站点配置）
	plans []*NginxListenerPlan
}

// NginxListenerPlan 是一个监听端口的导入计划。
type NginxListenerPlan struct {
	Port     int    `json:"port"`
	TLS      bool   `json:"tls"`
	ListenIP string `json:"listenIp"`
	HTTP3    bool   `json:"http3"`
	// ProxyProtocol 为 nginx 监听上的 proxy_protocol，对应“接收 PROXY 协议”
	ProxyProtocol bool `json:"proxyProtocol"`
	// Existing 为合并进去的已有监听名称，为空表示新建监听
	Existing    string          `json:"existing,omitempty"`
	ExistingKey string          `json:"existingKey,omitempty"`
	Sites       []NginxSitePlan `json:"sites"`
	// plainRedirect 为 HTTPS 监听上的 error_page 497：同端口收到明文 HTTP 时跳转到 HTTPS
	plainRedirect bool
	listener      *Listener        // 新建监听时的配置
	sites         []nginxSiteDraft // 待导入的站点
}

// NginxSitePlan 是一个站点的转换结果。
type NginxSitePlan struct {
	Name    string   `json:"name"`
	Domains []string `json:"domains"`
	Type    string   `json:"type,omitempty"`
	// Summary 为后端、根目录或跳转目标的简述
	Summary string `json:"summary,omitempty"`
	Source  string `json:"source"`
	// Default 表示作为监听的默认站点（default_server 或 server_name _）
	Default bool `json:"default,omitempty"`
	// Skipped 非空表示该站点不会导入，内容为原因
	Skipped  string   `json:"skipped,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	// Ignored 为没有对应功能、未转换的 nginx 指令
	Ignored []string `json:"ignored,omitempty"`
	// CertMissing 为 HTTPS 站点中尚无匹配证书的域名，CertFiles 为 nginx 使用的证书文件
	CertMissing []string `json:"certMissing,omitempty"`
	CertFiles   []string `json:"certFiles,omitempty"`
}

type nginxSiteDraft struct {
	plan *NginxSitePlan
	site Site
	// plainRedirect 为该 server 块配置了 error_page 497
	plainRedirect bool
}

// nginxListen 是一条 listen 指令解析后的结果。
type nginxListen struct {
	ip            string
	port          int
	tls           bool
	quic          bool
	def           bool
	proxyProtocol bool
}

// nginxUpstream 是 upstream 块。
type nginxUpstream struct {
	servers []string
	policy  string
	warns   []string
}

// 转换时静默忽略的指令：Gately 有等价的默认行为，或属于性能、日志等不影响功能的调优
var nginxIgnoredSilently = map[string]bool{
	"listen": true, "server_name": true, "location": true, "ssl_certificate": true, "ssl_certificate_key": true,
	"ssl_protocols": true, "ssl_ciphers": true, "ssl_prefer_server_ciphers": true, "ssl_session_cache": true,
	"ssl_session_timeout": true, "ssl_session_tickets": true, "ssl_stapling": true, "ssl_stapling_verify": true,
	"ssl_dhparam": true, "ssl_ecdh_curve": true, "ssl_trusted_certificate": true, "ssl_early_data": true, "ssl_buffer_size": true,
	"ssl_conf_command": true, "ssl": true, "http2": true, "http3": true, "quic_retry": true, "include": true,
	"access_log": true, "error_log": true, "log_not_found": true, "server_tokens": true, "charset": true, "index": true,
	"client_max_body_size": true, "client_body_buffer_size": true, "client_body_timeout": true, "client_header_timeout": true,
	"keepalive_timeout": true, "send_timeout": true, "sendfile": true, "tcp_nopush": true, "tcp_nodelay": true,
	"proxy_http_version": true, "proxy_buffering": true, "proxy_request_buffering": true, "proxy_buffers": true,
	"proxy_buffer_size": true, "proxy_busy_buffers_size": true, "proxy_read_timeout": true, "proxy_send_timeout": true,
	"proxy_connect_timeout": true, "proxy_redirect": true, "proxy_cache_bypass": true, "proxy_max_temp_file_size": true,
	"proxy_ssl_server_name": true, "proxy_ssl_name": true, "proxy_ssl_verify": true, "proxy_ssl_protocols": true,
	"proxy_set_header": true, "proxy_pass": true, "proxy_hide_header": true, "proxy_ignore_headers": true,
	"proxy_intercept_errors": true, "proxy_next_upstream": true, "proxy_pass_request_headers": true,
	"add_header": true, "gzip": true, "gzip_types": true, "gzip_vary": true, "gzip_proxied": true, "gzip_comp_level": true,
	"gzip_min_length": true, "gzip_disable": true, "gzip_http_version": true, "gzip_buffers": true, "gzip_static": true,
	"root": true, "alias": true, "autoindex": true, "autoindex_exact_size": true, "autoindex_localtime": true,
	"autoindex_format": true, "try_files": true, "error_page": true, "return": true, "rewrite": true, "default_type": true,
	"allow": true, "deny": true, "auth_basic": true, "auth_basic_user_file": true, "resolver": true, "resolver_timeout": true,
	"large_client_header_buffers": true, "expires": true, "etag": true, "if_modified_since": true, "open_file_cache": true,
	"chunked_transfer_encoding": true, "underscores_in_headers": true, "ignore_invalid_headers": true, "absolute_redirect": true,
	"port_in_redirect": true, "server_name_in_redirect": true, "types": true, "types_hash_max_size": true, "real_ip_header": true,
	"set_real_ip_from": true, "real_ip_recursive": true, "limit_req": true, "limit_conn": true,
}

// nginxIgnoredPrefixes 是静默忽略的指令前缀：缓存、缓冲、超时与 FastCGI 等调优参数
var nginxIgnoredPrefixes = []string{"fastcgi_", "uwsgi_", "scgi_", "client_", "proxy_cache", "proxy_no_cache", "proxy_temp",
	"proxy_store", "ssl_", "open_file_cache", "http2_", "quic_", "lingering_", "keepalive_", "gzip_", "brotli", "output_buffers",
	"postpone_output", "aio", "directio", "sendfile", "tcp_", "reset_timedout_connection", "auth_request_set"}

// ignoredSilently 判断指令是否属于无需提示的调优参数。
func ignoredSilently(name string) bool {
	if nginxIgnoredSilently[name] {
		return true
	}
	for _, p := range nginxIgnoredPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

var reNginxVar = regexp.MustCompile(`\$\{?[A-Za-z_][A-Za-z0-9_]*\}?`)

// importNginx 把 nginx 配置转换为导入计划；existing 为当前的监听配置，用于合并到相同端口并跳过已存在的域名。
func importNginx(text string, existing []Listener, certs CertSource) (*NginxImportReport, error) {
	root, warns, err := parseNginx(text)
	if err != nil {
		return nil, fmt.Errorf("解析 nginx 配置失败: %w", err)
	}
	rep := &NginxImportReport{Listeners: []NginxListenerPlan{}, Warnings: append([]string{}, warns...)}
	upstreams := map[string]*nginxUpstream{}
	var servers []*ngDirective
	httpGzip := false
	// mapDefaults 为 http 级 map 指令的默认值，用于近似解析跳转地址中的变量
	mapDefaults := map[string]string{}
	// 递归收集 upstream 与 server；stream 块内的是 TCP/UDP 转发，提示改用端口转发
	var walk func(list []*ngDirective, ctx string)
	walk = func(list []*ngDirective, ctx string) {
		for _, d := range list {
			switch {
			case d.Name == "stream" && d.isBlock():
				rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s：stream 块是 TCP/UDP 转发，请在端口转发中配置（SNI 分流对应 ssl_preread）", d.where()))
			case d.Name == "upstream" && d.isBlock():
				upstreams[d.arg(0)] = parseNginxUpstream(d)
			case d.Name == "server" && d.isBlock():
				if ctx == "stream" || isStreamServer(d) {
					rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s：TCP/UDP 转发的 server 块请在端口转发中配置", d.where()))
					continue
				}
				servers = append(servers, d)
			case d.Name == "gzip" && d.arg(0) == "on" && ctx == "http":
				httpGzip = true
			case d.Name == "map" && d.isBlock() && len(d.Args) == 2:
				for _, x := range d.Block {
					if x.Name == "default" {
						mapDefaults[strings.TrimPrefix(d.Args[1], "$")] = x.arg(0)
					}
				}
			case d.isBlock() && (d.Name == "http" || d.Name == "events"):
				walk(d.Block, d.Name)
			}
		}
	}
	walk(root, "")
	if len(servers) == 0 {
		return nil, errors.New("没有找到 server 块。请粘贴站点配置，或在 nginx 所在主机执行 nginx -T 后粘贴完整输出")
	}

	plans := map[string]*NginxListenerPlan{}
	var order []string
	for _, srv := range servers {
		listens, lw := parseNginxListens(srv)
		for _, l := range listens {
			key := fmt.Sprintf("%s|%d|%v", l.ip, l.port, l.tls)
			p := plans[key]
			if p == nil {
				p = &NginxListenerPlan{Port: l.port, TLS: l.tls, ListenIP: l.ip}
				plans[key] = p
				order = append(order, key)
			}
			p.HTTP3 = p.HTTP3 || l.quic
			p.ProxyProtocol = p.ProxyProtocol || l.proxyProtocol
			draft := convertNginxServer(srv, l, upstreams, httpGzip, mapDefaults)
			draft.plan.Warnings = append(slices.Clone(lw), draft.plan.Warnings...)
			if l.tls && hasNginxPlainRedirect(srv) {
				p.plainRedirect = true
				draft.plainRedirect = true
			}
			p.sites = append(p.sites, draft)
		}
	}

	// 合并到已有监听：端口与 HTTPS 一致，监听地址相同或任一方为全部地址
	usedDomains := map[string]bool{}
	for _, key := range order {
		p := plans[key]
		var target *Listener
		for i := range existing {
			l := &existing[i]
			if l.Port == p.Port && l.TLS == p.TLS && (l.ListenIP == p.ListenIP || l.ListenIP == "" || p.ListenIP == "") {
				target = l
				break
			}
		}
		taken := map[string]bool{}
		hasDefault := false
		if target != nil {
			p.Existing, p.ExistingKey = target.Name, target.Key
			if p.Existing == "" {
				p.Existing = ":" + strconv.Itoa(target.Port)
			}
			for _, s := range target.Sites {
				for _, d := range s.Domains {
					taken[strings.ToLower(d)] = true
				}
			}
			hasDefault = target.Default.Enable || siteConfigured(&target.Default)
		} else {
			p.listener = &Listener{Name: fmt.Sprintf("nginx :%d", p.Port), Port: p.Port, TLS: p.TLS, ListenIP: p.ListenIP,
				HTTP3: p.HTTP3 && p.TLS, AcceptProxyProtocol: p.ProxyProtocol, Remark: "从 nginx 配置导入",
				// 与界面新建监听一致：默认站点为停用的空反向代理
				Default: Site{Type: TypeProxy, ForwardedHeaders: true}}
			if p.TLS {
				p.listener.MinTLS = "1.2"
			}
			if p.plainRedirect {
				p.listener.PlainHTTP = PlainHTTPRedirect
			}
		}
		for i := range p.sites {
			d := &p.sites[i]
			pl := d.plan
			// 合并到已有监听时不改动其端口设置，提示用户自行开启
			if d.plainRedirect && target != nil && target.PlainHTTP == "" {
				pl.Warnings = append(pl.Warnings, "error_page 497 未导入：已有监听未开启「同端口 HTTP」，如需跳转请在监听设置中选择「跳转到 HTTPS」")
			}
			switch {
			case pl.Skipped != "":
			case pl.Default && hasDefault:
				pl.Skipped = "该监听已有默认站点"
			case !pl.Default:
				var dup []string
				for _, dom := range pl.Domains {
					k := strings.ToLower(dom) + "|" + key
					if taken[strings.ToLower(dom)] || usedDomains[k] {
						dup = append(dup, dom)
					}
				}
				if len(dup) == len(pl.Domains) {
					pl.Skipped = fmt.Sprintf("域名已存在：%s", strings.Join(dup, ", "))
				} else if len(dup) > 0 {
					pl.Warnings = append(pl.Warnings, fmt.Sprintf("已存在的域名不再导入：%s", strings.Join(dup, ", ")))
					d.site.Domains = slices.DeleteFunc(d.site.Domains, func(s string) bool { return slices.Contains(dup, s) })
					pl.Domains = d.site.Domains
				}
			}
			if pl.Skipped == "" {
				if pl.Default {
					hasDefault = true
				}
				for _, dom := range pl.Domains {
					usedDomains[strings.ToLower(dom)+"|"+key] = true
				}
				// 先单独校验，无法通过的站点跳过并给出原因，不影响其他站点
				probe := Listener{Port: p.Port, TLS: p.TLS, Sites: []Site{d.site}}
				if pl.Default {
					probe = Listener{Port: p.Port, TLS: p.TLS, Default: d.site}
				}
				if err := probe.Validate(); err != nil {
					pl.Skipped = fmt.Sprintf("转换结果无效：%v", err)
				}
			}
			if pl.Skipped == "" && p.TLS && certs != nil {
				for _, dom := range pl.Domains {
					if name, ok := certCheckName(dom); ok {
						if _, err := certs.GetCertificate(&tls.ClientHelloInfo{ServerName: name}); err != nil {
							pl.CertMissing = append(pl.CertMissing, dom)
						}
					}
				}
			}
			if pl.Skipped != "" {
				rep.Skipped++
			} else {
				rep.Sites++
			}
			p.Sites = append(p.Sites, *pl)
		}
		rep.Listeners = append(rep.Listeners, *p)
		rep.plans = append(rep.plans, p)
	}
	return rep, nil
}

// siteConfigured 判断站点是否已填写了实际内容（停用的默认站点也算已配置，避免覆盖）。
func siteConfigured(s *Site) bool {
	return len(s.Targets) > 0 || s.RedirectTo != "" || s.Root != "" || s.Text != ""
}

// isStreamServer 判断 server 块是否为 stream 上下文的 TCP/UDP 转发（nginx -T 中 include 的文件无法得知所在上下文）。
func isStreamServer(d *ngDirective) bool {
	hasProxy := false
	for _, c := range d.Block {
		switch c.Name {
		case "location", "server_name", "root", "return", "rewrite":
			return false
		case "proxy_pass":
			hasProxy = !strings.Contains(c.arg(0), "://")
		case "listen":
			if slices.Contains(c.Args, "udp") {
				return true
			}
		}
	}
	return hasProxy
}

// parseNginxUpstream 解析 upstream 块中的后端与负载均衡策略。
func parseNginxUpstream(d *ngDirective) *nginxUpstream {
	u := &nginxUpstream{}
	for _, c := range d.Block {
		switch c.Name {
		case "server":
			addr := c.arg(0)
			if strings.HasPrefix(addr, "unix:") {
				u.warns = append(u.warns, fmt.Sprintf("upstream %s 中的 Unix 套接字后端 %s 不受支持", d.arg(0), addr))
				continue
			}
			spec := addr
			// 标记为 down 的后端不导入；max_fails 等健康检查参数由 Gately 的被动健康检查代替
			if slices.Contains(c.Args[1:], "down") {
				continue
			}
			for _, a := range c.Args[1:] {
				if strings.HasPrefix(a, "weight=") || a == "backup" {
					spec += " " + a
				}
			}
			u.servers = append(u.servers, spec)
		case "ip_hash", "hash":
			u.policy = "ip_hash"
		case "least_conn":
			u.policy = "least_conn"
		case "random":
			u.policy = "random"
		}
	}
	return u
}

// hasNginxPlainRedirect 判断 server 块是否用 error_page 497 把同端口的明文 HTTP 请求跳转到 HTTPS，
// 例如 error_page 497 https://$host:$server_port$request_uri; 对应监听的「同端口 HTTP：跳转到 HTTPS」。
func hasNginxPlainRedirect(srv *ngDirective) bool {
	for _, d := range srv.Block {
		if d.Name == "error_page" && len(d.Args) >= 2 && slices.Contains(d.Args[:len(d.Args)-1], "497") {
			return true
		}
	}
	return false
}

// parseNginxListens 解析 server 块的 listen 指令；没有 listen 时与 nginx 一样视为 80 端口。
func parseNginxListens(srv *ngDirective) ([]nginxListen, []string) {
	var out []nginxListen
	var warns []string
	sslOn := false
	for _, c := range srv.Block {
		if c.Name == "ssl" && c.arg(0) == "on" {
			sslOn = true
		}
	}
	seen := map[string]int{}
	for _, c := range srv.Block {
		if c.Name != "listen" {
			continue
		}
		addr := c.arg(0)
		if strings.HasPrefix(addr, "unix:") {
			warns = append(warns, fmt.Sprintf("Unix 套接字监听 %s 不受支持", addr))
			continue
		}
		l := nginxListen{tls: sslOn}
		host, port := "", addr
		if strings.HasPrefix(addr, "[") {
			if i := strings.Index(addr, "]"); i > 0 {
				host, port = addr[1:i], strings.TrimPrefix(addr[i+1:], ":")
			}
		} else if h, p, ok := strings.Cut(addr, ":"); ok {
			host, port = h, p
		} else if !isDigits(addr) {
			host, port = addr, "80"
		}
		if port == "" {
			port = "80"
		}
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			warns = append(warns, fmt.Sprintf("无法识别的监听 %s", addr))
			continue
		}
		l.port = n
		if host != "" && host != "*" && host != "0.0.0.0" && host != "::" {
			if ip := net.ParseIP(host); ip != nil {
				l.ip = ip.String()
			} else {
				warns = append(warns, fmt.Sprintf("监听地址 %s 不是 IP，已改为监听全部地址", host))
			}
		}
		for _, a := range c.Args[1:] {
			switch a {
			case "ssl":
				l.tls = true
			case "quic":
				l.quic = true
			case "default_server", "default":
				l.def = true
			case "proxy_protocol":
				l.proxyProtocol = true
			}
		}
		// listen 443 quic 与 listen 443 ssl 是同一个站点，QUIC 合并为 HTTP/3
		key := fmt.Sprintf("%s|%d", l.ip, l.port)
		if i, ok := seen[key]; ok {
			out[i].tls = out[i].tls || l.tls || l.quic
			out[i].quic = out[i].quic || l.quic
			out[i].def = out[i].def || l.def
			out[i].proxyProtocol = out[i].proxyProtocol || l.proxyProtocol
			continue
		}
		if l.quic {
			l.tls = true
		}
		seen[key] = len(out)
		out = append(out, l)
	}
	// IPv4 与 IPv6 的全部地址监听同一端口时合并为一个双栈监听
	if len(out) == 0 {
		out = append(out, nginxListen{port: 80, tls: sslOn})
	}
	return out, warns
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// convertNginxServer 把一个 server 块按指定监听转换为站点。
func convertNginxServer(srv *ngDirective, l nginxListen, upstreams map[string]*nginxUpstream, httpGzip bool, mapDefaults map[string]string) nginxSiteDraft {
	pl := &NginxSitePlan{Source: srv.where()}
	site := Site{Enable: true, AccessLog: true, Compress: httpGzip}
	conv := &nginxConverter{plan: pl, site: &site, upstreams: upstreams, tls: l.tls, mapDefaults: mapDefaults,
		vars: map[string]string{}, authVars: map[string]bool{}}

	// 域名
	var names []string
	for _, c := range srv.Block {
		if c.Name == "server_name" {
			names = append(names, c.Args...)
		}
	}
	for _, n := range names {
		n = strings.ToLower(n)
		switch {
		case n == "_" || n == "" || n == `""` || n == "localhost":
		case strings.HasPrefix(n, "~"):
			pl.Warnings = append(pl.Warnings, fmt.Sprintf("正则域名 %s 不受支持", n))
		case strings.HasPrefix(n, "."):
			// .example.com 在 nginx 中同时匹配 example.com 与其子域名
			site.Domains = append(site.Domains, n[1:], "*"+n)
		case strings.HasSuffix(n, ".*"):
			pl.Warnings = append(pl.Warnings, fmt.Sprintf("后缀通配域名 %s 不受支持", n))
		default:
			site.Domains = append(site.Domains, n)
		}
	}
	// 规范化域名，跳过 Host 中不可能出现的写法（如 $hostname 变量），其余域名照常导入
	var bad []string
	site.Domains, bad = splitDomains(site.Domains)
	if len(bad) > 0 {
		pl.Warnings = append(pl.Warnings, fmt.Sprintf("域名 %s 格式不正确，已忽略", strings.Join(bad, "、")))
	}
	if len(site.Domains) > 0 {
		site.Name = site.Domains[0]
	}
	pl.Domains = site.Domains
	pl.Name = site.Name
	if l.def || len(site.Domains) == 0 {
		pl.Default = true
		if pl.Name == "" {
			pl.Name = "默认站点"
		}
		if !l.def && len(names) > 0 && !slices.Contains(names, "_") {
			pl.Warnings = append(pl.Warnings, "没有可用的域名，作为默认站点导入")
		}
	}

	conv.convert(srv)
	if pl.Skipped == "" && site.Type == "" {
		pl.Skipped = "没有可转换的反向代理、静态目录、跳转或固定内容"
	}
	pl.Type = site.Type
	pl.Summary = conv.summary()
	for _, c := range srv.Block {
		if c.Name == "ssl_certificate" {
			pl.CertFiles = append(pl.CertFiles, c.arg(0))
		}
	}
	pl.Ignored = conv.ignoredList()
	return nginxSiteDraft{plan: pl, site: site}
}

// nginxConverter 保存转换一个站点时的上下文。
type nginxConverter struct {
	plan      *NginxSitePlan
	site      *Site
	upstreams map[string]*nginxUpstream
	tls       bool
	ignored   []string
	// vars 为 set 指令定义的变量，mapDefaults 为 http 级 map 的默认值
	vars        map[string]string
	mapDefaults map[string]string
	// authRequest 为 auth_request 的子请求路径，authVars 为 auth_request_set 定义的变量，
	// loginURL 为 error_page 401 =302 的跳转地址，copyHeaders 为把认证结果传给后端的请求头
	authRequest string
	authVars    map[string]bool
	loginURL    string
	copyHeaders []string
	// authPrefixes 为只在部分路径上启用 auth_request 的 location；为空且 authRequest 非空表示整站认证
	authPrefixes []string
	authSiteWide bool
}

func (c *nginxConverter) warn(format string, args ...any) {
	// 同一条提示（如多个 location 中相同的请求头）只记录一次
	if msg := fmt.Sprintf(format, args...); !slices.Contains(c.plan.Warnings, msg) {
		c.plan.Warnings = append(c.plan.Warnings, msg)
	}
}

func (c *nginxConverter) ignore(name string) {
	if !slices.Contains(c.ignored, name) {
		c.ignored = append(c.ignored, name)
	}
}

func (c *nginxConverter) ignoredList() []string {
	slices.Sort(c.ignored)
	return c.ignored
}

// summary 返回站点的简述。
func (c *nginxConverter) summary() string {
	s := c.site
	switch s.Type {
	case TypeProxy:
		return strings.Join(s.Targets, ", ")
	case TypeStatic:
		return s.Root
	case TypeRedirect:
		if s.KeepPath {
			return fmt.Sprintf("%d → %s（保留路径）", s.RedirectCode, s.RedirectTo)
		}
		return fmt.Sprintf("%d → %s", s.RedirectCode, s.RedirectTo)
	case TypeText:
		return fmt.Sprintf("%d %s", s.StatusCode, truncate(s.Text, 40))
	case TypeClose:
		return "444 断开连接"
	}
	return ""
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// locationMatch 解析 location 的匹配方式与路径。
func locationMatch(d *ngDirective) (mod, path string) {
	if len(d.Args) >= 2 {
		return d.Args[0], d.Args[1]
	}
	a := d.arg(0)
	switch {
	case strings.HasPrefix(a, "^~"):
		return "^~", strings.TrimPrefix(a, "^~")
	case strings.HasPrefix(a, "="):
		return "=", strings.TrimPrefix(a, "=")
	case strings.HasPrefix(a, "~*"):
		return "~*", strings.TrimPrefix(a, "~*")
	case strings.HasPrefix(a, "~"):
		return "~", strings.TrimPrefix(a, "~")
	}
	return "", a
}

// convert 依次处理 server 级指令、根 location 与其他 location。
func (c *nginxConverter) convert(srv *ngDirective) {
	var rootLoc *ngDirective
	var others []*ngDirective
	for _, d := range srv.Block {
		if d.Name != "location" || !d.isBlock() {
			continue
		}
		mod, p := locationMatch(d)
		if p == "/" && (mod == "" || mod == "^~") {
			rootLoc = d
			continue
		}
		others = append(others, d)
	}
	// server 级的 return 444 先于 location 生效，整个站点直接断开连接
	if hasReturn444(srv.Block) {
		c.site.Type = TypeClose
		c.collectCommon(srv.Block)
		if rootLoc != nil {
			others = append([]*ngDirective{rootLoc}, others...)
		}
		for _, o := range others {
			_, p := locationMatch(o)
			c.warn("站点直接断开连接，location %s 未导入", p)
		}
		return
	}
	// server 级的 return / rewrite 跳转先于 location 生效
	if c.applyRedirect(srv.Block) {
		c.collectCommon(srv.Block)
		for _, o := range others {
			_, p := locationMatch(o)
			c.warn("站点整体跳转，location %s 未导入", p)
		}
		return
	}
	c.collectCommon(srv.Block)
	main := srv.Block
	if rootLoc != nil {
		main = rootLoc.Block
		c.collectCommon(rootLoc.Block)
	}
	switch {
	case c.applyRedirect(main):
	case c.applyText(main):
	case c.applyProxy(main):
	case c.applyStatic(main, srv.Block):
	default:
		// 没有根路径的处理：取第一个反向代理的 location 作为站点后端
		for _, o := range others {
			mod, p := locationMatch(o)
			if (mod == "" || mod == "^~") && c.applyProxy(o.Block) {
				c.warn("没有 location /，以 location %s 的后端作为站点后端，其他路径也会转发到这里", p)
				break
			}
		}
	}
	if c.site.Type == TypeProxy {
		// location / 中的设置优先，没有时继承 server 级
		c.applyProxyTimeouts(main, srv.Block)
	}
	c.applyClientTimeouts(main, srv.Block)
	for _, o := range others {
		if isInternalLocation(o) {
			continue
		}
		// 断开连接的站点不经过任何路径规则，其他 location 不导入
		if c.site.Type == TypeClose {
			_, p := locationMatch(o)
			c.warn("站点直接断开连接，location %s 未导入", p)
			continue
		}
		if _, p := locationMatch(o); c.authRequest != "" && p == c.authRequest {
			continue
		}
		c.convertLocation(o)
	}
	c.applyForwardAuth(others)
}

// collectAuth 记录 auth_request 相关的指令，返回是否为启用的 auth_request。
func (c *nginxConverter) collectAuth(d *ngDirective) bool {
	switch d.Name {
	case "auth_request":
		if d.arg(0) != "off" {
			c.authRequest = d.arg(0)
			return true
		}
	case "auth_request_set":
		c.authVars[strings.TrimPrefix(d.arg(0), "$")] = true
	case "set":
		c.vars[strings.TrimPrefix(d.arg(0), "$")] = d.arg(1)
	case "error_page":
		// error_page 401 =302 https://auth.example.com/?rd=$target_url 是 Authelia 等服务的登录跳转
		if len(d.Args) >= 3 && d.Args[0] == "401" && strings.HasPrefix(d.Args[len(d.Args)-2], "=30") {
			c.loginURL = d.Args[len(d.Args)-1]
		}
	}
	return false
}

// isInternalLocation 判断 location 是否只供内部跳转使用（internal），访客无法直接访问。
func isInternalLocation(d *ngDirective) bool {
	return slices.ContainsFunc(d.Block, func(x *ngDirective) bool { return x.Name == "internal" })
}

// resolveVars 用 set 与 map 默认值替换变量，最多替换几轮以支持变量嵌套。usedMap 表示用到了 map 的默认值。
func (c *nginxConverter) resolveVars(v string) (out string, usedMap bool) {
	for range 4 {
		next := reNginxVar.ReplaceAllStringFunc(v, func(m string) string {
			name := strings.Trim(strings.TrimPrefix(m, "$"), "{}")
			if val, ok := c.vars[name]; ok {
				return val
			}
			if val, ok := c.mapDefaults[name]; ok {
				usedMap = true
				return val
			}
			return m
		})
		if next == v {
			break
		}
		v = next
	}
	return v, usedMap
}

// applyForwardAuth 把 auth_request 转换为转发认证：认证地址取子请求 location 的 proxy_pass，
// 登录跳转取 error_page 401 =302 的地址，其中访客原始地址与请求方法换成 {url}、{method}。
func (c *nginxConverter) applyForwardAuth(locs []*ngDirective) {
	if c.authRequest == "" {
		return
	}
	s := c.site
	var authURL string
	for _, o := range locs {
		if _, p := locationMatch(o); p != c.authRequest {
			continue
		}
		for _, x := range o.Block {
			if x.Name == "proxy_pass" {
				authURL, _ = c.resolveVars(x.arg(0))
			}
		}
	}
	if authURL == "" || reNginxVar.MatchString(authURL) {
		c.warn("auth_request %s 无法转换为转发认证（找不到认证地址），请在站点中手动配置转发认证", c.authRequest)
		return
	}
	if s.BasicAuth || s.WebAuth {
		c.warn("站点同时使用基本认证与 auth_request，已改用转发认证")
		s.BasicAuth, s.WebAuth, s.SharedUsers = false, false, false
	}
	s.ForwardAuth = ForwardAuth{URL: authURL, CopyHeaders: c.copyHeaders}
	if !c.authSiteWide {
		// 只有部分路径需要认证：其余路径公开，认证路径加“必须认证”的路径规则
		s.AuthOptional = true
		for _, p := range c.authPrefixes {
			i := slices.IndexFunc(s.Rules, func(r PathRule) bool { return r.Prefix == p })
			if i < 0 {
				s.Rules = append(s.Rules, PathRule{Prefix: p, Auth: RuleAuthOn})
			} else {
				s.Rules[i].Auth = RuleAuthOn
			}
		}
	}
	if c.loginURL != "" {
		login, usedMap := c.resolveVars(c.loginURL)
		login = strings.NewReplacer("$scheme://$http_host$request_uri", "{url}", "$scheme://$host$request_uri", "{url}",
			"$request_method", "{method}").Replace(login)
		if v := reNginxVar.FindString(login); v != "" {
			c.warn("登录跳转地址 %s 中的变量 %s 无法转换，未设置登录地址", c.loginURL, v)
		} else {
			s.ForwardAuth.LoginURL = login
			if usedMap {
				c.warn("登录跳转地址按 map 的默认值转换为 %s，请确认", login)
			}
		}
	}
	c.warn("已把 auth_request 转换为转发认证（%s），请确认", authURL)
}

// collectCommon 处理 server 与 location / 中响应头、压缩、认证、访问控制等与站点类型无关的指令。
func (c *nginxConverter) collectCommon(list []*ngDirective) {
	s := c.site
	var allows, denies []string
	denyAll := false
	for _, d := range list {
		switch d.Name {
		case "add_header":
			if h, ok := c.headerLine(d, "add_header"); ok {
				s.ResponseHeaders = append(s.ResponseHeaders, h)
			}
		case "gzip":
			s.Compress = d.arg(0) == "on"
		case "access_log":
			if d.arg(0) == "off" {
				s.AccessLog = false
			}
		case "auth_basic":
			if d.arg(0) != "off" {
				// 不读取 htpasswd 文件；先改用共享用户认证，在补齐用户前任何人都无法通过，避免站点因导入而失去保护
				s.BasicAuth, s.SharedUsers = true, true
				c.warn("已开启基本认证并使用共享用户，nginx 的用户文件不会被读取，请在全局设置中添加共享用户，或在站点中填写用户（用户名:密码，密码可为 bcrypt 哈希）")
			}
		case "auth_request", "auth_request_set", "set", "error_page":
			if c.collectAuth(d) && d.Name == "auth_request" {
				c.authSiteWide = true
			}
		case "allow":
			if d.arg(0) != "all" {
				allows = append(allows, d.arg(0))
			}
		case "deny":
			if d.arg(0) == "all" {
				denyAll = true
			} else {
				denies = append(denies, d.arg(0))
			}
		case "if", "map", "sub_filter", "limit_except", "geo":
			c.ignore(d.Name)
		default:
			if d.isBlock() || ignoredSilently(d.Name) {
				continue
			}
			c.ignore(d.Name)
		}
	}
	switch {
	case denyAll && len(allows) > 0:
		s.IPMode, s.IPList = IPModeWhitelist, append(s.IPList, allows...)
	case !denyAll && len(denies) > 0:
		s.IPMode, s.IPList = IPModeBlacklist, append(s.IPList, denies...)
	case denyAll:
		c.warn("deny all 拒绝了全部访问，已改为导入白名单为空的站点，请确认")
		s.IPMode = IPModeWhitelist
	}
	s.ResponseHeaders = uniqueList(s.ResponseHeaders)
}

// headerLine 把 add_header / proxy_set_header 转为 "Key: Value"；值含 nginx 变量时无法转换。
func (c *nginxConverter) headerLine(d *ngDirective, kind string) (string, bool) {
	k, v := d.arg(0), d.arg(1)
	if k == "" {
		return "", false
	}
	// proxy_set_header Range $http_range 只是把客户端的同名请求头原样传给后端，Gately 默认就会转发
	if kind == "proxy_set_header" && strings.EqualFold(v, "$http_"+strings.ReplaceAll(k, "-", "_")) {
		return "", false
	}
	// proxy_set_header Remote-User $user（$user 来自 auth_request_set）对应转发认证的“复制认证响应头”
	if kind == "proxy_set_header" && c.authVars[strings.TrimPrefix(v, "$")] {
		if !slices.Contains(c.copyHeaders, k) {
			c.copyHeaders = append(c.copyHeaders, k)
		}
		return "", false
	}
	if reNginxVar.MatchString(v) {
		c.warn("%s %s 使用了 nginx 变量（%s），未导入", kind, k, v)
		return "", false
	}
	return k + ": " + v, true
}

// applyRedirect 识别 return 3xx 与 rewrite ... permanent/redirect 形式的整站跳转。
func (c *nginxConverter) applyRedirect(list []*ngDirective) bool {
	for _, d := range list {
		var code int
		var to string
		switch d.Name {
		case "return":
			n, err := strconv.Atoi(d.arg(0))
			if err != nil || n < 300 || n > 399 || len(d.Args) < 2 {
				continue
			}
			code, to = n, d.arg(1)
		case "rewrite":
			// 只转换匹配整个路径、跳到其他地址的 rewrite，例如 rewrite ^ https://$host$request_uri permanent;
			pat, flag := d.arg(0), d.arg(2)
			to = d.arg(1)
			if !strings.Contains(to, "://") || (flag != "permanent" && flag != "redirect") {
				continue
			}
			switch pat {
			case "^":
			case "^(.*)$", "^(.*)":
				// $1 为完整路径，nginx 会自动附加查询串，与保留路径等价
				to = strings.Replace(to, "$1", "$request_uri", 1)
			case "^/(.*)$", "^/(.*)":
				to = strings.Replace(to, "/$1", "$request_uri", 1)
			default:
				continue
			}
			code = 302
			if flag == "permanent" {
				code = 301
			}
		default:
			continue
		}
		target, keep, ok := c.redirectTarget(to)
		if !ok {
			return false
		}
		s := c.site
		s.Type, s.RedirectTo, s.RedirectCode, s.KeepPath = TypeRedirect, target, code, keep
		return true
	}
	return false
}

// redirectTarget 把 nginx 跳转地址转换为 Gately 的跳转目标：$host 换为 {host}，结尾的 $request_uri 换为“保留路径”。
func (c *nginxConverter) redirectTarget(to string) (string, bool, bool) {
	keep := false
	for _, suffix := range []string{"$request_uri", "$uri$is_args$args", "$uri?$args", "$uri"} {
		if strings.HasSuffix(to, suffix) {
			to, keep = strings.TrimSuffix(to, suffix), true
			if suffix == "$uri" {
				c.warn("跳转目标使用 $uri，导入后会同时保留查询串")
			}
			break
		}
	}
	scheme := "http"
	if c.tls {
		scheme = "https"
	}
	r := strings.NewReplacer("$scheme", scheme, "$http_host", hostPlaceholder, "$host", hostPlaceholder, "${host}", hostPlaceholder)
	to = r.Replace(to)
	if strings.Contains(to, "$server_name") {
		if len(c.site.Domains) == 0 {
			c.warn("跳转目标使用 $server_name，但站点没有域名")
			return "", false, false
		}
		to = strings.ReplaceAll(to, "$server_name", c.site.Domains[0])
	}
	if v := reNginxVar.FindString(to); v != "" {
		c.warn("跳转目标 %s 中的变量 %s 无法转换", to, v)
		return "", false, false
	}
	if to == "" {
		c.warn("跳转目标为空")
		return "", false, false
	}
	return to, keep, true
}

// hasReturn444 判断指令列表中是否有 return 444。
func hasReturn444(list []*ngDirective) bool {
	for _, d := range list {
		if d.Name == "return" && d.arg(0) == "444" {
			return true
		}
	}
	return false
}

// applyText 识别 return 200 "内容" 这类固定内容、return 403/404 拒绝访问，以及 return 444 直接断开连接。
func (c *nginxConverter) applyText(list []*ngDirective) bool {
	contentType := ""
	for _, d := range list {
		if d.Name == "default_type" {
			contentType = d.arg(0)
		}
	}
	for _, d := range list {
		if d.Name != "return" {
			continue
		}
		code, err := strconv.Atoi(d.arg(0))
		if err != nil || code >= 300 && code < 400 {
			continue
		}
		if code == 444 {
			c.site.Type = TypeClose
			return true
		}
		text := d.arg(1)
		if reNginxVar.MatchString(text) {
			c.warn("固定内容中的 nginx 变量不会被替换")
		}
		s := c.site
		s.Type, s.StatusCode, s.Text, s.ContentType = TypeText, code, text, contentType
		if s.ContentType == "" {
			s.ContentType = "text/plain; charset=utf-8"
		}
		return true
	}
	return false
}

// proxyTargets 把 proxy_pass 地址转换为后端列表；uri 为 proxy_pass 中主机之后的路径部分。
func (c *nginxConverter) proxyTargets(pass string) (targets []string, uri, policy string, ok bool) {
	if reNginxVar.MatchString(pass) {
		c.warn("proxy_pass %s 使用了变量，无法转换", pass)
		return nil, "", "", false
	}
	u, err := url.Parse(pass)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		c.warn("无法识别的 proxy_pass %s", pass)
		return nil, "", "", false
	}
	uri = u.Path
	if u.RawQuery != "" {
		uri += "?" + u.RawQuery
	}
	if up, found := c.upstreams[u.Host]; found {
		for _, w := range up.warns {
			c.warn("%s", w)
		}
		if len(up.servers) == 0 {
			c.warn("upstream %s 没有可用的后端", u.Host)
			return nil, "", "", false
		}
		for _, s := range up.servers {
			targets = append(targets, u.Scheme+"://"+s)
		}
		return targets, uri, up.policy, true
	}
	return []string{u.Scheme + "://" + u.Host}, uri, "", true
}

// applyProxy 识别 proxy_pass 反向代理。
func (c *nginxConverter) applyProxy(list []*ngDirective) bool {
	var pass *ngDirective
	for _, d := range list {
		if d.Name == "proxy_pass" {
			pass = d
		}
	}
	if pass == nil {
		return false
	}
	targets, uri, policy, ok := c.proxyTargets(pass.arg(0))
	if !ok {
		return false
	}
	s := c.site
	if uri != "" && uri != "/" {
		// location / 下 proxy_pass 带路径：nginx 把 / 替换为该路径，Gately 把后端路径拼在请求路径前面，效果相同
		for i := range targets {
			base, params, _ := strings.Cut(targets[i], " ")
			targets[i] = strings.TrimSpace(base + strings.TrimRight(uri, "/") + "/ " + params)
		}
	}
	s.Type, s.Targets, s.LBPolicy = TypeProxy, targets, policy
	c.applyProxyOptions(list, &s.RequestHeaders, true)
	return true
}

// applyProxyOptions 处理 proxy_set_header 与上游 TLS 选项。站点级调用时 site 为 true，会设置站点开关。
func (c *nginxConverter) applyProxyOptions(list []*ngDirective, headers *[]string, site bool) {
	s := c.site
	verify, sniFromHost := false, false
	for _, d := range list {
		switch d.Name {
		case "proxy_set_header":
			k := strings.ToLower(d.arg(0))
			v := d.arg(1)
			switch k {
			case "x-forwarded-for", "x-real-ip", "x-forwarded-proto", "x-forwarded-host", "x-forwarded-port", "forwarded":
				if site {
					s.ForwardedHeaders = true
				}
			case "upgrade", "connection":
				// Gately 自动处理 WebSocket 升级
			case "host":
				switch v {
				case "$host", "$http_host", "$host:$server_port":
				case "$proxy_host", "$upstream_addr":
					if site {
						s.UseTargetHost = true
					}
				default:
					if h, ok := c.headerLine(d, "proxy_set_header"); ok {
						*headers = append(*headers, h)
					}
				}
			default:
				if h, ok := c.headerLine(d, "proxy_set_header"); ok {
					*headers = append(*headers, h)
				}
			}
		case "proxy_ssl_verify":
			verify = d.arg(0) == "on"
		case "proxy_ssl_name":
			sniFromHost = d.arg(0) == "$host" || d.arg(0) == "$http_host"
		case "proxy_protocol":
			if site && d.arg(0) == "on" {
				s.SendProxyProtocol = true
			}
		}
	}
	if !site {
		return
	}
	https := false
	for _, t := range s.Targets {
		https = https || strings.HasPrefix(t, "https://")
	}
	if https {
		// nginx 默认不校验后端证书（proxy_ssl_verify off），保持一致
		s.InsecureSkipVerify = !verify
		s.UpstreamSNIFromHost = sniFromHost
	}
}

// applyProxyTimeouts 把 proxy_connect_timeout 与 proxy_read_timeout 转换为站点的后端超时。
// lists 按优先级排列（location / 在前，server 在后），取第一个出现的值；都没有时保持 Gately 的默认值。
// nginx 的 proxy_read_timeout 是两次读取之间的空闲上限，等待响应头同样受它限制，对应 Gately 的等待响应头超时；
// Gately 不限制响应体的读取间隔，长连接不会因此断开。
func (c *nginxConverter) applyProxyTimeouts(lists ...[]*ngDirective) {
	c.site.Timeouts.Connect = c.nginxTimeout("proxy_connect_timeout", maxConnectTimeout, lists)
	c.site.Timeouts.ResponseHeader = c.nginxTimeout("proxy_read_timeout", maxResponseHeaderTimeout, lists)
}

// applyClientTimeouts 把 client_body_timeout 与 send_timeout 转换为站点的访客读写空闲超时，两者在 nginx 中
// 同样是两次读取（写出）之间的空闲上限，含义一致。lists 的优先级与 applyProxyTimeouts 相同。
func (c *nginxConverter) applyClientTimeouts(lists ...[]*ngDirective) {
	c.site.ClientTimeouts.ReadIdle = c.nginxTimeout("client_body_timeout", maxClientIdle, lists)
	c.site.ClientTimeouts.WriteIdle = c.nginxTimeout("send_timeout", maxClientIdle, lists)
}

// nginxTimeout 按优先级在 lists 中查找指令 name 并转换为秒；没有设置或无法识别时返回 0（使用 Gately 的默认值），
// 超过上限时按上限并提示。
func (c *nginxConverter) nginxTimeout(name string, max int, lists [][]*ngDirective) int {
	var d *ngDirective
	for _, list := range lists {
		for _, x := range list {
			if x.Name == name {
				d = x
				break
			}
		}
		if d != nil {
			break
		}
	}
	if d == nil {
		return 0
	}
	n, ok := nginxSeconds(d.arg(0))
	if !ok {
		c.warn("%s %s 无法识别，使用 Gately 的默认值", name, d.arg(0))
		return 0
	}
	if n > max {
		c.warn("%s %s 超过 Gately 的上限，按 %d 秒导入", name, d.arg(0), max)
		return max
	}
	return n
}

// warnLocationTimeouts 在 location 设置了与站点不同的后端超时时提示：Gately 的后端超时按站点设置，对所有路径生效。
func (c *nginxConverter) warnLocationTimeouts(p string, list []*ngDirective) {
	for _, d := range list {
		var cur int
		switch d.Name {
		case "proxy_connect_timeout":
			cur = c.site.Timeouts.Connect
		case "proxy_read_timeout":
			cur = c.site.Timeouts.ResponseHeader
		default:
			continue
		}
		if n, ok := nginxSeconds(d.arg(0)); !ok || n != cur {
			c.warn("location %s 的 %s 未导入，站点的后端超时对所有路径生效", p, d.Name)
		}
	}
}

// nginxSeconds 解析 nginx 的时间值（如 60、30s、5m、1h、1m30s、500ms），不足 1 秒的部分向上取整，结果至少为 1 秒。
// 支持的单位与 nginx 相同：ms、s、m、h、d、w、M（30 天）、y（365 天），不带单位为秒。
func nginxSeconds(v string) (int, bool) {
	units := map[string]int64{"ms": 1, "s": 1000, "m": 60_000, "h": 3_600_000, "d": 86_400_000, "w": 604_800_000, "M": 2_592_000_000, "y": 31_536_000_000}
	if v == "" {
		return 0, false
	}
	var total int64
	for v != "" {
		i := 0
		for i < len(v) && v[i] >= '0' && v[i] <= '9' {
			i++
		}
		// 每段都要以数字开头，且限制位数防止溢出
		if i == 0 || i > 12 {
			return 0, false
		}
		n, _ := strconv.ParseInt(v[:i], 10, 64)
		v = v[i:]
		j := 0
		for j < len(v) && (v[j] < '0' || v[j] > '9') {
			j++
		}
		unit := v[:j]
		v = v[j:]
		if unit == "" {
			// 不带单位的数字只能是最后一段
			if v != "" {
				return 0, false
			}
			unit = "s"
		}
		mul, ok := units[unit]
		if !ok {
			return 0, false
		}
		// 超过 10 年按无法识别处理；先比较再相乘，避免 999999999999y 这类值乘积溢出成负数绕过上限
		const limit = int64(31_536_000_000 * 10)
		if n > (limit-total)/mul {
			return 0, false
		}
		total += n * mul
	}
	if total == 0 {
		// 0 在 nginx 中没有实际意义，交给调用方按无法识别处理
		return 0, false
	}
	return int((total + 999) / 1000), true
}

// applyStatic 识别 root / alias 静态目录。serverList 用于读取 server 级的 root。
func (c *nginxConverter) applyStatic(list, serverList []*ngDirective) bool {
	root := ""
	for _, src := range [][]*ngDirective{serverList, list} {
		for _, d := range src {
			if d.Name == "root" || d.Name == "alias" {
				root = d.arg(0)
			}
		}
	}
	if root == "" {
		return false
	}
	if reNginxVar.MatchString(root) {
		c.warn("静态目录 %s 使用了变量，无法转换", root)
		return false
	}
	s := c.site
	s.Type, s.Root = TypeStatic, strings.TrimRight(root, "/")
	if s.Root == "" {
		s.Root = "/"
	}
	for _, src := range [][]*ngDirective{serverList, list} {
		for _, d := range src {
			switch d.Name {
			case "autoindex":
				s.ListDir = d.arg(0) == "on"
			case "try_files":
				args := strings.Join(d.Args, " ")
				switch {
				case strings.Contains(args, "$uri.html"):
					s.CleanURLs = true
				case strings.HasSuffix(args, "/index.html"):
					c.warn("try_files 回退到 /index.html（单页应用）：Gately 会以 404 状态返回 index.html，请确认前端是否正常")
					s.NotFoundPage = "index.html"
				}
			case "error_page":
				if len(d.Args) >= 2 && slices.Contains(d.Args[:len(d.Args)-1], "404") {
					s.NotFoundPage = strings.TrimPrefix(d.Args[len(d.Args)-1], "/")
				}
			}
		}
	}
	return true
}

// convertLocation 转换根路径以外的 location：反向代理转为分路径后端，allow/deny 转为路径访问规则。
func (c *nginxConverter) convertLocation(d *ngDirective) {
	mod, p := locationMatch(d)
	if mod == "~" || mod == "~*" {
		c.warn("正则 location %s 未导入", p)
		return
	}
	if p == "" || !strings.HasPrefix(p, "/") {
		c.warn("location %s 未导入", strings.Join(d.Args, " "))
		return
	}
	s := c.site
	prefix := strings.TrimRight(p, "/")
	if prefix == "" {
		prefix = "/"
	}
	handled := false
	// 只在该路径上启用的 auth_request：导入为“仅对指定路径认证”
	authHere := false
	for _, x := range d.Block {
		if x.Name == "auth_request" || x.Name == "auth_request_set" || x.Name == "set" || x.Name == "error_page" {
			authHere = c.collectAuth(x) || authHere
		}
	}
	if authHere {
		c.authPrefixes = append(c.authPrefixes, prefix)
		handled = true
	}
	// 访问控制
	var allows, denies []string
	denyAll := false
	for _, x := range d.Block {
		switch x.Name {
		case "allow":
			if x.arg(0) != "all" {
				allows = append(allows, x.arg(0))
			}
		case "deny":
			if x.arg(0) == "all" {
				denyAll = true
			} else {
				denies = append(denies, x.arg(0))
			}
		}
	}
	if denyAll || len(denies) > 0 {
		if mod == "=" {
			c.warn("精确匹配 location = %s 的访问控制按前缀导入", p)
		}
		rule := PathRule{Prefix: prefix}
		if denyAll {
			rule.IPMode, rule.IPList = IPModeWhitelist, allows
		} else {
			rule.IPMode, rule.IPList = IPModeBlacklist, denies
		}
		s.Rules = append(s.Rules, rule)
		handled = true
	}
	// 跳转或固定内容
	for _, x := range d.Block {
		if x.Name == "return" {
			code, _ := strconv.Atoi(x.arg(0))
			switch {
			case code >= 300 && code < 400 && len(x.Args) >= 2:
				if to, keep, ok := c.redirectTarget(x.arg(1)); ok {
					pattern := "^" + regexp.QuoteMeta(prefix)
					if mod == "=" {
						pattern += "$"
					} else if prefix != "/" {
						pattern += "(/.*)?$"
					}
					rw := Rewrite{Pattern: pattern, To: to, Redirect: true, Code: code}
					if keep {
						c.warn("location %s 的跳转保留路径（$request_uri）未导入，跳转到固定地址", p)
					}
					s.Rewrites = append(s.Rewrites, rw)
					handled = true
				}
			case code == 403 || code == 404 || code == 444:
				if !denyAll {
					s.Rules = append(s.Rules, PathRule{Prefix: prefix, IPMode: IPModeWhitelist})
					c.warn("location %s 返回 %d，已转换为禁止所有 IP 访问该路径", p, code)
				}
				handled = true
			}
		}
	}
	// 分路径后端
	var pass *ngDirective
	for _, x := range d.Block {
		if x.Name == "proxy_pass" {
			pass = x
		}
	}
	if pass != nil {
		if s.Type != TypeProxy {
			c.warn("站点不是反向代理，location %s 的 proxy_pass 未导入", p)
			return
		}
		targets, uri, _, ok := c.proxyTargets(pass.arg(0))
		if !ok {
			return
		}
		loc := Location{Prefix: p}
		if mod == "=" {
			c.warn("精确匹配 location = %s 的后端按前缀导入", p)
		}
		if uri != "" && strings.TrimRight(uri, "/") != strings.TrimRight(p, "/") {
			// nginx 把匹配的前缀替换为 proxy_pass 的路径：去掉前缀，路径部分作为后端地址的路径
			if strings.Contains(uri, "?") {
				c.warn("location %s 的 proxy_pass 路径 %s 带查询串，无法转换，请确认后端路径", p, uri)
			} else {
				loc.StripPrefix = true
				switch {
				case mod == "=" && uri != "/":
					// 精确匹配时 nginx 把整个路径换成 proxy_pass 的路径；Gately 按前缀去掉后再拼接，后端收到的路径会多出结尾的 /
					c.warn("精确匹配 location = %s 的 proxy_pass 路径 %s 按去掉前缀导入，后端收到的路径为 %s/，请确认后端能处理", p, uri, strings.TrimRight(uri, "/"))
				case !strings.HasSuffix(uri, "/") && strings.HasSuffix(p, "/"):
					// nginx 直接拼接：location /api/ + proxy_pass http://b/v1 时 /api/x 发往 /v1x；Gately 拼接时补上 /，发往 /v1/x
					c.warn("location %s 的 proxy_pass 路径 %s 不以 / 结尾，nginx 会直接拼接（如 %sx），导入后按 %s/x 转发，请确认后端路径", p, uri, uri, uri)
				}
				if uri != "/" {
					for i := range targets {
						base, params, _ := strings.Cut(targets[i], " ")
						targets[i] = strings.TrimSpace(strings.TrimRight(base, "/") + strings.TrimRight(uri, "/") + "/ " + params)
					}
					loc.Targets = targets
				}
			}
		}
		if !slices.Equal(targets, s.Targets) {
			loc.Targets = targets
		}
		c.applyProxyOptions(d.Block, &loc.RequestHeaders, false)
		c.warnLocationTimeouts(p, d.Block)
		if len(loc.Targets) > 0 || len(loc.RequestHeaders) > 0 || loc.StripPrefix {
			s.Locations = append(s.Locations, loc)
		}
		handled = true
	}
	for _, x := range d.Block {
		switch x.Name {
		case "root", "alias", "try_files", "autoindex":
			if pass == nil {
				c.warn("location %s 的静态目录未导入，Gately 的站点只能有一种类型", p)
				return
			}
		}
	}
	if !handled {
		c.warn("location %s 没有可转换的内容，未导入", p)
	}
}

// handleImportNginx 预览或导入 nginx 配置。
func (m *Module) handleImportNginx(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxNginxConfig+4096)
	var req NginxImportRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if strings.TrimSpace(req.Config) == "" {
		httpx.Error(w, http.StatusBadRequest, "请粘贴 nginx 配置")
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rep, err := importNginx(req.Config, m.cfg.Listeners, m.certs)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if req.Apply {
		if rep.Sites == 0 {
			httpx.Error(w, http.StatusBadRequest, "没有可导入的站点")
			return
		}
		if err := m.applyNginxLocked(rep, req.Enable); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "%v", err)
			return
		}
		rep.Applied = true
	}
	// 站点的简述与跳过原因由程序生成，一并翻译；站点名称、已有监听名称保持原样
	httpx.OK(w, i18n.Localize(r, rep, "summary", "skipped"))
}

// applyNginxLocked 按计划写入配置：合并到已有监听或新建监听，保存后重启受影响的监听。
func (m *Module) applyNginxLocked(rep *NginxImportReport, enable bool) error {
	old := slices.Clone(m.cfg.Listeners)
	var changed []string
	for _, p := range rep.plans {
		var l *Listener
		if p.ExistingKey != "" {
			for i := range m.cfg.Listeners {
				if m.cfg.Listeners[i].Key == p.ExistingKey {
					l = &m.cfg.Listeners[i]
				}
			}
		}
		added := false
		if l == nil {
			nl := *p.listener
			nl.Enable = enable
			m.cfg.Listeners = append(m.cfg.Listeners, nl)
			l = &m.cfg.Listeners[len(m.cfg.Listeners)-1]
		}
		for _, d := range p.sites {
			if d.plan.Skipped != "" {
				continue
			}
			site := d.site
			site.Enable = enable
			site.Remark = fmt.Sprintf("从 nginx 导入（%s）", d.plan.Source)
			if d.plan.Default {
				l.Default = site
			} else {
				l.Sites = append(l.Sites, site)
			}
			added = true
		}
		if !added {
			if p.ExistingKey == "" {
				m.cfg.Listeners = m.cfg.Listeners[:len(m.cfg.Listeners)-1]
			}
			continue
		}
		if l.Key == "" {
			l.Key = crud.NewKey()
		}
		if err := l.Validate(); err != nil {
			m.cfg.Listeners = old
			return fmt.Errorf("监听 :%d 校验失败: %w", p.Port, err)
		}
		changed = append(changed, l.Key)
	}
	if err := m.saveLocked(); err != nil {
		m.cfg.Listeners = old
		return err
	}
	for _, k := range changed {
		m.restartLocked(k)
	}
	return nil
}
