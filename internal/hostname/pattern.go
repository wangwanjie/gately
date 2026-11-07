package hostname

import (
	"net/netip"
	"strings"
)

// NormalizePattern 用于 Web 站点域名，在 Normalize 的写法之外允许首段内的一个 *，
// 如 api-*.example.com 或 app-*-dev.example.com。此时 * 不跨越域名分段。
// Normalize 保持不变，避免其他只支持 *.example.com 的模块接受无法匹配的配置。
func NormalizePattern(d string) (string, bool) {
	if name, ok := Normalize(d, true); ok {
		return name, true
	}
	d = strings.ToLower(strings.Trim(strings.TrimSpace(d), "."))
	label, suffix, found := strings.Cut(d, ".")
	if !found || strings.Count(label, "*") != 1 || !validLabel(strings.Replace(label, "*", "x", 1)) {
		return "", false
	}
	if _, err := netip.ParseAddr(suffix); err == nil {
		return "", false
	}
	suffix, ok := normalizeName(suffix)
	if !ok || len(label)+1+len(suffix) > maxLen {
		return "", false
	}
	return label + "." + suffix, true
}

// CapturePattern 匹配已规范化的 Web 站点域名，返回 * 捕获的部分。
// *.example.com 保留匹配任意级子域名的行为；首段内的 * 只匹配该段内的零个或多个字符。
func CapturePattern(pattern, host string) (string, bool) {
	switch {
	case pattern == host:
		return "", true
	case pattern == "*":
		return host, true
	case strings.HasPrefix(pattern, "*."):
		capture, ok := strings.CutSuffix(host, pattern[1:])
		return capture, ok
	}
	label, suffix, found := strings.Cut(pattern, ".")
	if !found || strings.Count(label, "*") != 1 {
		return "", false
	}
	hostLabel, ok := strings.CutSuffix(host, "."+suffix)
	if !ok || !validLabel(hostLabel) {
		return "", false
	}
	prefix, tail, _ := strings.Cut(label, "*")
	if len(hostLabel) < len(prefix)+len(tail) || !strings.HasPrefix(hostLabel, prefix) || !strings.HasSuffix(hostLabel, tail) {
		return "", false
	}
	return hostLabel[len(prefix) : len(hostLabel)-len(tail)], true
}
