package frp

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// 本文件解析已有的 frpc / frps 配置文件，转换为 Gately 的设置与代理列表。
// 支持三种写法：
//   - 旧版 INI（frpc.ini / frps.ini，含 [common] 段，每个代理一个 [名称] 段）；
//   - 把旧版 INI 内容放进 .toml 文件的写法（语法与 INI 相同，值可能带引号）；
//   - frp 0.52+ 的 TOML（顶层键 + [[proxies]]）。
// 不依赖 TOML 库：frp 配置只用到字符串、数字、布尔、字符串数组与内联表，按行解析即可覆盖。

// 导入的角色。
const (
	RoleClient = "client"
	RoleServer = "server"
)

// ImportResult 是解析结果，Warnings 列出被忽略或需要用户留意的内容。
type ImportResult struct {
	Role     string          `json:"role"`
	Client   *ClientSettings `json:"client,omitempty"`
	Server   *ServerSettings `json:"server,omitempty"`
	Proxies  []Proxy         `json:"proxies"`
	Warnings []string        `json:"warnings"`
}

// section 是配置文件中的一个段。顶层键属于名称为空的段。
type section struct {
	name string
	// array 表示 TOML 的 [[name]] 数组表
	array bool
	kv    map[string]any
}

var sectionRe = regexp.MustCompile(`^\[\[?\s*([^\[\]]*?)\s*\]\]?$`)

// parseConfig 把配置文本解析为段列表。legacy 为 true 时按旧版 INI 处理，注释只能独占一行。
func parseConfig(text string) (secs []*section, legacy bool, err error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for _, l := range lines {
		if strings.EqualFold(strings.TrimSpace(l), "[common]") {
			legacy = true
			break
		}
	}
	cur := &section{kv: map[string]any{}}
	secs = append(secs, cur)
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !legacy {
			line = stripComment(line)
		}
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && sectionRe.MatchString(line) {
			m := sectionRe.FindStringSubmatch(line)
			cur = &section{name: m[1], array: strings.HasPrefix(line, "[["), kv: map[string]any{}}
			secs = append(secs, cur)
			continue
		}
		key, raw, ok := strings.Cut(line, "=")
		if !ok {
			return nil, legacy, fmt.Errorf("第 %d 行无法解析: %s", i+1, line)
		}
		key = unquote(strings.TrimSpace(key))
		raw = strings.TrimSpace(raw)
		// 多行数组：括号未闭合时继续读取后续行
		for !legacy && strings.HasPrefix(raw, "[") && depth(raw) > 0 && i+1 < len(lines) {
			i++
			raw += " " + stripComment(strings.TrimSpace(lines[i]))
		}
		cur.kv[key] = parseValue(raw)
	}
	return secs, legacy, nil
}

// stripComment 去掉 TOML 行尾的 # 注释，引号内的 # 保留。
func stripComment(line string) string {
	var quote rune
	for i, r := range line {
		switch {
		case quote != 0:
			if r == quote && (quote == '\'' || i == 0 || line[i-1] != '\\') {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == '#':
			return strings.TrimSpace(line[:i])
		}
	}
	return line
}

// depth 返回括号嵌套深度（引号内的括号不计）。
func depth(s string) int {
	var quote rune
	n := 0
	for i, r := range s {
		switch {
		case quote != 0:
			if r == quote && (quote == '\'' || i == 0 || s[i-1] != '\\') {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == '[' || r == '{':
			n++
		case r == ']' || r == '}':
			n--
		}
	}
	return n
}

// splitTop 按逗号拆分，忽略引号与括号内的逗号。
func splitTop(s string) []string {
	var out []string
	var quote rune
	level, start := 0, 0
	for i, r := range s {
		switch {
		case quote != 0:
			if r == quote && (quote == '\'' || i == 0 || s[i-1] != '\\') {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == '[' || r == '{':
			level++
		case r == ']' || r == '}':
			level--
		case r == ',' && level == 0:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

// unquote 去掉首尾成对的引号并处理双引号内的转义。
func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		if u, err := strconv.Unquote(s); err == nil {
			return u
		}
		return s[1 : len(s)-1]
	}
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}
	return s
}

// parseValue 把值解析为 string、[]any（数组）或 map[string]string（内联表）。
func parseValue(raw string) any {
	raw = strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]"):
		var out []any
		for _, part := range splitTop(raw[1 : len(raw)-1]) {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, parseValue(part))
			}
		}
		return out
	case strings.HasPrefix(raw, "{") && strings.HasSuffix(raw, "}"):
		out := map[string]string{}
		for _, part := range splitTop(raw[1 : len(raw)-1]) {
			if k, v, ok := strings.Cut(part, "="); ok {
				out[unquote(strings.TrimSpace(k))] = unquote(strings.TrimSpace(v))
			}
		}
		return out
	}
	return unquote(raw)
}

// pick 依次按候选键取字符串值，返回命中的键名以便标记为已处理。
func pick(kv map[string]any, names ...string) string {
	for _, n := range names {
		if v, ok := kv[n]; ok {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

// pickInt 取整数值，缺失或格式错误返回 0。
func pickInt(kv map[string]any, names ...string) int {
	n, _ := strconv.Atoi(pick(kv, names...))
	return n
}

// pickList 取列表值：数组原样返回，逗号分隔的字符串拆分后返回。
func pickList(kv map[string]any, names ...string) []string {
	for _, n := range names {
		switch v := kv[n].(type) {
		case []any:
			var out []string
			for _, it := range v {
				if s, ok := it.(string); ok && strings.TrimSpace(s) != "" {
					out = append(out, strings.TrimSpace(s))
				}
			}
			return out
		case string:
			var out []string
			for _, s := range strings.Split(v, ",") {
				if s = strings.TrimSpace(s); s != "" {
					out = append(out, s)
				}
			}
			return out
		}
	}
	return nil
}

// flatten 把 TOML 的 [table] 展开为带前缀的键（如 [webServer] 下的 port 变成 webServer.port）。
func flatten(secs []*section) map[string]any {
	out := map[string]any{}
	for _, s := range secs {
		if s.array {
			continue
		}
		for k, v := range s.kv {
			if s.name != "" {
				k = s.name + "." + k
			}
			out[k] = v
		}
	}
	return out
}

// handled 记录已被映射的键，其余键会作为"未支持"提示给用户。
type handled map[string]bool

func (h handled) add(names ...string) {
	for _, n := range names {
		h[n] = true
	}
}

func (h handled) leftover(kv map[string]any) []string {
	var out []string
	for k := range kv {
		if !h[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// ParseImport 解析 frpc 或 frps 配置文本。
func ParseImport(role, content string) (*ImportResult, error) {
	if role != RoleClient && role != RoleServer {
		return nil, fmt.Errorf("未知角色 %q", role)
	}
	if strings.TrimSpace(content) == "" {
		return nil, errors.New("配置内容为空")
	}
	secs, legacy, err := parseConfig(content)
	if err != nil {
		return nil, err
	}
	var top map[string]any
	if legacy {
		for _, s := range secs {
			if strings.EqualFold(s.name, "common") {
				top = s.kv
			}
		}
	} else {
		top = flatten(secs)
	}
	res := &ImportResult{Role: role, Proxies: []Proxy{}, Warnings: []string{}}
	isServerFile := pick(top, "bindPort", "bind_port") != "" && pick(top, "serverAddr", "server_addr") == ""
	if role == RoleClient {
		if isServerFile {
			return nil, errors.New("这是 frps 服务端配置，请在「服务端 frps」页导入")
		}
		res.Client = importClient(top, legacy, res)
		if err := res.Client.Validate(); err != nil {
			return nil, fmt.Errorf("客户端配置无效: %w", err)
		}
		importProxies(secs, legacy, res)
		return res, nil
	}
	if !isServerFile && pick(top, "serverAddr", "server_addr") != "" {
		return nil, errors.New("这是 frpc 客户端配置，请在「客户端 frpc」页导入")
	}
	res.Server = importServer(top, legacy, res)
	if err := res.Server.Validate(); err != nil {
		return nil, fmt.Errorf("服务端配置无效: %w", err)
	}
	return res, nil
}

func importClient(top map[string]any, legacy bool, res *ImportResult) *ClientSettings {
	h := handled{}
	h.add("authentication_method", "auth.method", "loginFailExit", "login_fail_exit")
	c := &ClientSettings{
		ServerAddr: pick(top, "serverAddr", "server_addr"),
		ServerPort: pickInt(top, "serverPort", "server_port"),
		User:       pick(top, "user"),
		Token:      pick(top, "auth.token", "token", "privilege_token"),
		Protocol:   pick(top, "transport.protocol", "protocol"),
	}
	h.add("serverAddr", "server_addr", "serverPort", "server_port", "user", "auth.token", "token", "privilege_token", "transport.protocol", "protocol")
	if c.ServerAddr == "" {
		res.Warnings = append(res.Warnings, "未找到服务端地址（serverAddr / server_addr），请导入后手动填写")
	}
	if c.ServerPort == 0 {
		c.ServerPort = 7000
	}
	if c.Protocol == "tcp" {
		c.Protocol = ""
	}
	if !protocols[c.Protocol] {
		res.Warnings = append(res.Warnings, fmt.Sprintf("不支持的传输协议 %q，已改用 tcp", c.Protocol))
		c.Protocol = ""
	}
	if m := pick(top, "auth.method", "authentication_method"); m != "" && m != "token" {
		res.Warnings = append(res.Warnings, fmt.Sprintf("认证方式 %s 暂不支持，仅导入令牌", m))
	}
	// Gately 生成的配置沿用 frp 默认的 TLS，无法关闭
	h.add("tls_enable", "transport.tls.enable")
	if pick(top, "tls_enable", "transport.tls.enable") == "false" {
		res.Warnings = append(res.Warnings, "原配置关闭了 TLS，Gately 使用 frp 默认的 TLS，需确认 frps 端兼容")
	}
	res.Warnings = append(res.Warnings, leftoverWarning("客户端字段暂不支持，已忽略", h.leftover(top)))
	res.Warnings = compact(res.Warnings)
	return c
}

func importServer(top map[string]any, legacy bool, res *ImportResult) *ServerSettings {
	h := handled{}
	h.add("authentication_method", "auth.method", "tls_enable", "tls_only", "transport.tls.force")
	s := &ServerSettings{
		BindAddr:       pick(top, "bindAddr", "bind_addr"),
		BindPort:       pickInt(top, "bindPort", "bind_port"),
		ProxyBindAddr:  pick(top, "proxyBindAddr", "proxy_bind_addr"),
		Token:          pick(top, "auth.token", "token", "privilege_token"),
		VhostHTTPPort:  pickInt(top, "vhostHTTPPort", "vhost_http_port"),
		VhostHTTPSPort: pickInt(top, "vhostHTTPSPort", "vhost_https_port"),
		SubdomainHost:  pick(top, "subDomainHost", "subdomain_host"),
		DashboardPort:  pickInt(top, "webServer.port", "dashboard_port"),
		DashboardAddr:  pick(top, "webServer.addr", "dashboard_addr"),
		DashboardUser:  pick(top, "webServer.user", "dashboard_user"),
		DashboardPwd:   pick(top, "webServer.password", "dashboard_pwd"),
	}
	h.add("bindAddr", "bind_addr", "proxyBindAddr", "proxy_bind_addr", "bindPort", "bind_port", "auth.token", "token", "privilege_token", "vhostHTTPPort", "vhost_http_port", "vhostHTTPSPort", "vhost_https_port",
		"subDomainHost", "subdomain_host", "webServer.port", "dashboard_port", "webServer.addr", "dashboard_addr",
		"webServer.user", "dashboard_user", "webServer.password", "dashboard_pwd", "allowPorts", "allow_ports")
	if s.BindPort == 0 {
		s.BindPort = 7000
	}
	if s.ProxyBindAddr == "" {
		// 原配置未写 proxyBindAddr 时 frps 沿用 bindAddr，导入后保持相同的监听范围
		s.ProxyBindAddr = s.BindAddr
	}
	// Gately 中留空即监听所有地址，0.0.0.0 统一为空，与生成的配置可互相导回
	if s.BindAddr == "0.0.0.0" {
		s.BindAddr = ""
	}
	if s.ProxyBindAddr == "0.0.0.0" {
		s.ProxyBindAddr = ""
	}
	if m := pick(top, "auth.method", "authentication_method"); m != "" && m != "token" {
		res.Warnings = append(res.Warnings, fmt.Sprintf("认证方式 %s 暂不支持，仅导入令牌", m))
	}
	s.AllowPorts = importAllowPorts(top)
	if s.DashboardPort != 0 {
		// 旧版 frps 的控制台默认监听所有网卡，新版默认仅本机；按原配置的语义保留
		if s.DashboardAddr == "" && legacy {
			s.DashboardAddr = "0.0.0.0"
		}
		if s.DashboardAddr == "0.0.0.0" || s.DashboardAddr == "::" {
			res.Warnings = append(res.Warnings, "控制台监听所有网卡，建议改为 127.0.0.1 并通过反向代理访问")
		}
		if s.DashboardUser == "" || s.DashboardPwd == "" {
			s.DashboardUser, s.DashboardPwd = "admin", "admin"
			res.Warnings = append(res.Warnings, "原配置未设置控制台账号密码，frp 默认为 admin / admin，请导入后立即修改")
		}
	}
	res.Warnings = append(res.Warnings, leftoverWarning("服务端字段暂不支持，已忽略", h.leftover(top)))
	res.Warnings = compact(res.Warnings)
	return s
}

// importAllowPorts 把 allowPorts（TOML 内联表数组）或 allow_ports（字符串）转换为 "6000-6100,7000" 形式。
func importAllowPorts(top map[string]any) string {
	if s, ok := top["allow_ports"].(string); ok {
		return strings.ReplaceAll(strings.TrimSpace(s), " ", "")
	}
	arr, _ := top["allowPorts"].([]any)
	var parts []string
	for _, it := range arr {
		switch v := it.(type) {
		case map[string]string:
			if v["single"] != "" {
				parts = append(parts, v["single"])
			} else if v["start"] != "" && v["end"] == v["start"] {
				parts = append(parts, v["start"])
			} else if v["start"] != "" && v["end"] != "" {
				parts = append(parts, v["start"]+"-"+v["end"])
			}
		case string:
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, ",")
}

// proxyKeys 是代理段中已映射到 Proxy 字段的键。
var proxyKeys = []string{
	"name", "type", "localIP", "local_ip", "localPort", "local_port", "remotePort", "remote_port",
	"customDomains", "custom_domains", "subdomain", "secretKey", "sk",
}

func importProxies(secs []*section, legacy bool, res *ImportResult) {
	h := handled{}
	h.add(proxyKeys...)
	h.add("role")
	unsupported := map[string]int{}
	seen := map[string]bool{}
	var skipped []string
	for _, s := range secs {
		var kv map[string]any
		name := ""
		switch {
		case legacy && s.name != "" && !strings.EqualFold(s.name, "common"):
			kv, name = s.kv, s.name
		case !legacy && s.array && s.name == "proxies":
			kv, name = s.kv, pick(s.kv, "name")
		case !legacy && s.array && s.name != "":
			skipped = append(skipped, s.name)
			continue
		default:
			continue
		}
		if strings.HasPrefix(name, "range:") {
			skipped = append(skipped, name)
			continue
		}
		if pick(kv, "role") == "visitor" {
			skipped = append(skipped, name+"（visitor）")
			continue
		}
		p := Proxy{
			Name:          name,
			Type:          pick(kv, "type"),
			Enable:        true,
			LocalIP:       pick(kv, "localIP", "local_ip"),
			LocalPort:     pickInt(kv, "localPort", "local_port"),
			RemotePort:    pickInt(kv, "remotePort", "remote_port"),
			CustomDomains: pickList(kv, "customDomains", "custom_domains"),
			Subdomain:     pick(kv, "subdomain"),
			SecretKey:     pick(kv, "secretKey", "sk"),
		}
		if p.Type == "" && legacy {
			// 旧版 frp 省略 type 时默认为 tcp
			p.Type = TypeTCP
		}
		// 只去掉写错的自定义域名，代理的其他域名照常导入
		if domains, bad := splitCustomDomains(p.CustomDomains); len(bad) > 0 {
			p.CustomDomains = domains
			res.Warnings = append(res.Warnings, fmt.Sprintf("代理 %s 的自定义域名 %s 格式不正确，已忽略", name, strings.Join(bad, "、")))
		}
		if err := p.Validate(); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("已跳过代理 %s：%v", name, err))
			continue
		}
		if seen[p.Name] {
			res.Warnings = append(res.Warnings, fmt.Sprintf("代理名称 %s 重复，已跳过后面的同名代理", p.Name))
			continue
		}
		seen[p.Name] = true
		for _, k := range h.leftover(kv) {
			unsupported[k]++
		}
		res.Proxies = append(res.Proxies, p)
	}
	if len(skipped) > 0 {
		res.Warnings = append(res.Warnings, "以下条目暂不支持，已跳过："+strings.Join(skipped, "、"))
	}
	if len(unsupported) > 0 {
		keys := make([]string, 0, len(unsupported))
		for k := range unsupported {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			keys[i] = fmt.Sprintf("%s（%d 个代理）", k, unsupported[k])
		}
		res.Warnings = append(res.Warnings, "代理字段暂不支持，已忽略："+strings.Join(keys, "、"))
	}
	res.Warnings = compact(res.Warnings)
}

// leftoverWarning 把未映射的键汇总为一条提示，没有时返回空串。
func leftoverWarning(prefix string, keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	return prefix + "：" + strings.Join(keys, "、")
}

// compact 去掉空提示。
func compact(in []string) []string {
	out := in[:0]
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
