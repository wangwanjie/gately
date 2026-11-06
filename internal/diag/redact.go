// Package diag 生成可安全提交到 issue 的诊断包：对配置与日志做脱敏，只保留定位问题所需的结构信息。
//
// 脱敏策略以“默认移除”为原则：
//   - 配置文件只保留字段名、数字、布尔值和白名单内的枚举型字符串，其余字符串一律替换为 <redacted>；
//   - 地址类字段只保留端口与 IP 类别（私网、公网等），主机名、域名和具体 IP 均不保留；
//   - 日志与错误信息中的 IP、域名、邮箱、URL、密钥样式的长串、MAC 地址、用户目录均会被替换。
package diag

import (
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Redacted 是被移除内容的占位符。
const Redacted = "<redacted>"

// Redactor 对文本做脱敏。零值不可用，请使用 NewRedactor。
type Redactor struct {
	// literals 是需要原样替换的本机敏感字面量（配置目录、用户目录、主机名），按长度从长到短排列
	literals []literal
}

type literal struct {
	from, to string
}

// NewRedactor 创建脱敏器。configDir 与主机名、用户目录出现在文本中时会被替换为占位符。
func NewRedactor(configDir string) *Redactor {
	r := &Redactor{}
	add := func(from, to string) {
		from = strings.TrimSpace(from)
		// 过短的字面量（如 "/"、"a"）会误伤正常文本
		if len(from) < 3 {
			return
		}
		r.literals = append(r.literals, literal{from, to})
	}
	if configDir != "" {
		if abs, err := filepath.Abs(configDir); err == nil {
			add(abs, "<config-dir>")
		}
		add(configDir, "<config-dir>")
	}
	if home, err := os.UserHomeDir(); err == nil {
		add(home, "<home>")
	}
	if host, err := os.Hostname(); err == nil {
		add(host, "<hostname>")
	}
	// 长的先替换，避免短字面量破坏长字面量
	for i := 1; i < len(r.literals); i++ {
		for j := i; j > 0 && len(r.literals[j].from) > len(r.literals[j-1].from); j-- {
			r.literals[j], r.literals[j-1] = r.literals[j-1], r.literals[j]
		}
	}
	return r
}

var (
	reURL      = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]{1,15})://[^\s"'<>]+`)
	reEmail    = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	reMAC      = regexp.MustCompile(`(?i)\b(?:[0-9a-f]{2}[:-]){5}[0-9a-f]{2}\b`)
	reKV       = regexp.MustCompile(`(?i)\b(password|passwd|pwd|passphrase|secret|token|apikey|api_key|access_key|authorization|cookie|credential)s?\b(["']?\s*[:=]\s*)("[^"]*"|'[^']*'|\S+)`)
	reBearer   = regexp.MustCompile(`(?i)\bbearer\s+\S+`)
	reIPv4     = regexp.MustCompile(`\b\d{1,3}(?:\.\d{1,3}){3}\b`)
	reIPv6     = regexp.MustCompile(`[0-9A-Fa-f:.]*:[0-9A-Fa-f:.]*:[0-9A-Fa-f:.]*(?:%[A-Za-z0-9]+)?`)
	reDomain   = regexp.MustCompile(`\b(?:[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,24}\b`)
	reLongTok  = regexp.MustCompile(`\b[A-Za-z0-9_-]{32,}\b`)
	reUserPath = regexp.MustCompile(`(?i)(/Users/|/home/|[A-Za-z]:\\Users\\)[^/\\\s]+`)
)

// fileExts 是常见文件扩展名，形如 main.go 的片段不应被当作域名。
var fileExts = map[string]bool{
	"go": true, "json": true, "yml": true, "yaml": true, "log": true, "conf": true, "txt": true, "zip": true,
	"pem": true, "crt": true, "key": true, "sh": true, "db": true, "sock": true, "toml": true, "ini": true,
	"html": true, "js": true, "css": true, "md": true, "enc": true, "tmp": true, "bak": true, "lock": true,
}

// publicDomains 是不会暴露用户信息的公共服务域名（含其子域），保留它们有助于判断是网络不通还是服务异常。
var publicDomains = []string{
	"github.com", "githubusercontent.com", "docker.io", "docker.com", "ghcr.io", "gcr.io",
	"letsencrypt.org", "zerossl.com", "google.com", "cloudflare.com", "aliyuncs.com", "dnspod.cn",
	"tencentcloudapi.com", "myhuaweicloud.com", "godaddy.com", "porkbun.com", "duckdns.org", "dynv6.com",
}

func isPublicDomain(d string) bool {
	d = strings.ToLower(d)
	for _, p := range publicDomains {
		if d == p || strings.HasSuffix(d, "."+p) {
			return true
		}
	}
	return false
}

// IPClass 返回 IP 的类别，用于在不暴露具体地址的前提下说明网络环境。
func IPClass(ip netip.Addr) string {
	ip = ip.Unmap()
	switch {
	case ip.IsUnspecified():
		return "unspecified"
	case ip.IsLoopback():
		return "loopback"
	case ip.IsPrivate():
		return "private"
	case ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast():
		return "linklocal"
	case ip.IsMulticast():
		return "multicast"
	case ip.Is4() && ip.As4()[0] == 100 && ip.As4()[1]&0xC0 == 64:
		// 100.64.0.0/10：运营商级 NAT 与 Tailscale
		return "cgnat"
	}
	return "public"
}

func ipPlaceholder(ip netip.Addr) string {
	family := "ipv6"
	if ip.Unmap().Is4() {
		family = "ipv4"
	}
	return "<" + family + ":" + IPClass(ip) + ">"
}

// Text 对自由文本（日志、错误信息）脱敏。
func (r *Redactor) Text(s string) string {
	if s == "" {
		return s
	}
	for _, l := range r.literals {
		s = strings.ReplaceAll(s, l.from, l.to)
	}
	s = reUserPath.ReplaceAllString(s, "${1}<user>")
	s = reURL.ReplaceAllStringFunc(s, func(m string) string {
		scheme := m[:strings.Index(m, "://")]
		switch strings.ToLower(scheme) {
		case "unix", "file":
			// 本地套接字与文件路径没有网络隐私，保留以便判断 Docker 连接问题
			return m
		}
		return strings.ToLower(scheme) + "://<url>"
	})
	s = reEmail.ReplaceAllString(s, "<email>")
	s = reBearer.ReplaceAllString(s, "Bearer "+Redacted)
	s = reKV.ReplaceAllString(s, "${1}${2}"+Redacted)
	s = reMAC.ReplaceAllString(s, "<mac>")
	s = reIPv6.ReplaceAllStringFunc(s, func(m string) string {
		cand := m
		if i := strings.IndexByte(cand, '%'); i >= 0 {
			cand = cand[:i]
		}
		ip, err := netip.ParseAddr(cand)
		if err != nil || !ip.Is6() {
			return m
		}
		return ipPlaceholder(ip)
	})
	s = reIPv4.ReplaceAllStringFunc(s, func(m string) string {
		ip, err := netip.ParseAddr(m)
		if err != nil {
			return m
		}
		return ipPlaceholder(ip)
	})
	s = reDomain.ReplaceAllStringFunc(s, func(m string) string {
		dot := strings.LastIndexByte(m, '.')
		if fileExts[strings.ToLower(m[dot+1:])] || isPublicDomain(m) {
			return m
		}
		return "<domain>"
	})
	s = reLongTok.ReplaceAllString(s, "<secret>")
	return s
}

// reStackFile 匹配 Go 堆栈中的源码位置行，例如 "\t/build/internal/app/app.go:123 +0x1a"；
// 收到 SIGQUIT 等信号或 GOTRACEBACK=crash 时行尾还带 "fp=0x… sp=0x… pc=0x…"。
var reStackFile = regexp.MustCompile(`^\t\S+\.(?:go|s):\d+(?: \+0x[0-9a-f]+)?(?: fp=0x[0-9a-f]+ sp=0x[0-9a-f]+ pc=0x[0-9a-f]+)?$`)

// reStackGoroutine 匹配堆栈中的协程标题行，例如 "goroutine 12 [running]:"。
var reStackGoroutine = regexp.MustCompile(`^goroutine \d+ (?:gp=\S+ m=\S+ (?:mp=\S+ )?)?\[[^\]]*\]:$`)

// Stack 对含 Go 崩溃堆栈的文本脱敏。堆栈中的函数名与源码路径（如 golang.org/x/net/http2.(*serverConn).serve）
// 形似域名，按自由文本处理会被替换成 <domain> 而无法定位，因此堆栈行只替换配置目录与用户目录；
// panic 信息等其余行可能带有地址、域名等访客数据，按 Text 完整脱敏。
func (r *Redactor) Stack(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		frame := reStackFile.MatchString(line) || reStackGoroutine.MatchString(line) ||
			// 函数行紧跟着源码位置行；"created by" 行同样后接源码位置
			(i+1 < len(lines) && reStackFile.MatchString(lines[i+1]) && !strings.HasPrefix(line, "\t"))
		if frame {
			lines[i] = r.paths(line)
			continue
		}
		lines[i] = r.Text(line)
	}
	return strings.Join(lines, "\n")
}

// paths 只替换本机敏感字面量与用户目录。
func (r *Redactor) paths(s string) string {
	for _, l := range r.literals {
		s = strings.ReplaceAll(s, l.from, l.to)
	}
	return reUserPath.ReplaceAllString(s, "${1}<user>")
}

var reNumList = regexp.MustCompile(`^[0-9,\-: ]+$`)

// Addr 对地址类字段脱敏：保留协议前缀与端口，主机替换为 IP 类别或 <host>，丢弃路径、查询串与账号信息。
func (r *Redactor) Addr(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || reNumList.MatchString(s) {
		return s
	}
	scheme := ""
	if i := strings.Index(s, "://"); i > 0 && i <= 10 {
		scheme, s = s[:i+3], s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndexByte(s, '@'); i >= 0 {
		s = s[i+1:]
	}
	host, port := s, ""
	if h, p, err := net.SplitHostPort(s); err == nil {
		host, port = h, ":"+p
	}
	host = strings.Trim(host, "[]")
	if i := strings.IndexByte(host, '%'); i >= 0 {
		host = host[:i]
	}
	switch {
	case host == "":
		return scheme + port
	default:
		if ip, err := netip.ParseAddr(host); err == nil {
			return scheme + ipPlaceholder(ip) + port
		}
		return scheme + "<host>" + port
	}
}
