// Package hostname 规范化与校验按 Host 头或 TLS SNI 匹配的域名。
//
// 浏览器发出的 Host 与 SNI 总是小写、不带方括号的 IPv6、国际化域名为 punycode，
// 这里把配置中的写法统一成同样的形式，保存时就能发现永远匹配不上的写法（带端口、路径、@ 等）。
package hostname

import (
	"net/netip"
	"strings"

	"golang.org/x/net/idna"
)

// maxLen 为域名的最大长度（不含末尾的点）
const maxLen = 253

// Normalize 规范化一条域名：去掉空白与首尾的点，转为小写；IPv6 去掉方括号并转为标准写法；
// 国际化域名转为 punycode。wildcard 为 true 时允许单独的 * 与 *.example.com。
// 空字符串原样返回 ok；不是合法主机名、IP 或通配写法时 ok 为 false。
func Normalize(d string, wildcard bool) (string, bool) {
	d = strings.ToLower(strings.Trim(strings.TrimSpace(d), "."))
	if d == "" {
		return "", true
	}
	if d == "*" {
		return d, wildcard
	}
	if rest, found := strings.CutPrefix(d, "*."); found {
		if !wildcard {
			return "", false
		}
		// 通配只能用于主机名，*.1.2.3.4 这类写法没有意义
		if _, err := netip.ParseAddr(rest); err == nil {
			return "", false
		}
		name, ok := normalizeName(rest)
		if !ok {
			return "", false
		}
		return "*." + name, true
	}
	// 方括号只用于包住 IPv6
	bracketed := strings.HasPrefix(d, "[") && strings.HasSuffix(d, "]")
	if bracketed {
		d = d[1 : len(d)-1]
	}
	if ip, err := netip.ParseAddr(d); err == nil {
		// 带区域的链路本地地址（fe80::1%eth0）不会出现在 Host 中
		if ip.Zone() != "" || (bracketed && !ip.Is6()) {
			return "", false
		}
		return ip.String(), true
	}
	if bracketed {
		return "", false
	}
	// 含冒号却不是 IP 的值，通常是写成了 example.com:8080
	if strings.ContainsAny(d, ":[]") {
		return "", false
	}
	return normalizeName(d)
}

// normalizeName 校验并规范化主机名：每段 1～63 个字符，只含字母、数字、- 与 _，首尾不能是 -。
func normalizeName(d string) (string, bool) {
	if !isASCII(d) {
		a, err := idna.ToASCII(d)
		if err != nil {
			return "", false
		}
		d = a
	}
	if len(d) > maxLen {
		return "", false
	}
	for _, label := range strings.Split(d, ".") {
		if !validLabel(label) {
			return "", false
		}
	}
	return d, true
}

func validLabel(l string) bool {
	if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
		return false
	}
	for i := 0; i < len(l); i++ {
		c := l[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}
