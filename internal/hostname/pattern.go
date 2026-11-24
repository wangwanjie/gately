package hostname

import (
	"net/netip"
	"strings"
)

// NormalizePattern 用于 Web 站点域名，在 Normalize 的写法（精确域名、IP、*、*.example.com）之外，
// 允许任意一段中出现一个 *：
//   - 段内通配 api-*.example.com、audiobook.api-*.example.com：* 只匹配该段内的零个或多个字符；
//   - 中间整段 audiobook.*.example.com：* 匹配恰好一段；
//   - 末尾整段 audiobook.*：* 匹配后面的一段或多段（audiobook.example.com、audiobook.home.lan）。
//
// 整个规则只能有一个 *，重定向变量 {1} 取它匹配到的部分。
// Normalize 保持不变，避免其他只支持 *.example.com 的模块接受无法匹配的配置。
func NormalizePattern(d string) (string, bool) {
	if name, ok := Normalize(d, true); ok {
		return name, true
	}
	d = strings.ToLower(strings.Trim(strings.TrimSpace(d), "."))
	if strings.Count(d, "*") != 1 || len(d) > maxLen {
		return "", false
	}
	labels := strings.Split(d, ".")
	if len(labels) < 2 {
		return "", false
	}
	idx := -1
	for i, l := range labels {
		if strings.Contains(l, "*") {
			idx = i
			continue
		}
		// 非通配段逐段规范化，国际化域名转 punycode
		n, ok := normalizeName(l)
		if !ok {
			return "", false
		}
		labels[i] = n
	}
	// 通配段：整段 * 或段内一个 *（把 * 换成字母后须为合法段，首尾不能是 -）
	if w := labels[idx]; w != "*" && !validLabel(strings.Replace(w, "*", "x", 1)) {
		return "", false
	}
	// 通配只能用于主机名：* 两侧的固定部分是 IP（api-*.1.2.3.4、192.168.1.*）没有意义
	if _, err := netip.ParseAddr(strings.Join(labels[idx+1:], ".")); err == nil {
		return "", false
	}
	if _, err := netip.ParseAddr(strings.Replace(strings.Join(labels, "."), "*", "0", 1)); err == nil {
		return "", false
	}
	out := strings.Join(labels, ".")
	if len(out) > maxLen {
		return "", false
	}
	return out, true
}

// CapturePattern 匹配已规范化的 Web 站点域名，返回 * 捕获的部分。
// *.example.com 与末尾整段的 audiobook.* 匹配一段或多段；中间整段 * 匹配恰好一段；
// 段内的 * 只匹配该段内的零个或多个字符，匹配后的段仍须是合法的段。
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
	before, after, found := strings.Cut(pattern, "*")
	if !found || strings.Contains(after, "*") {
		return "", false
	}
	// 长度检查避免前后固定部分在 host 中重叠（ab*bc 不能匹配 abc）
	if len(host) < len(before)+len(after) || !strings.HasPrefix(host, before) || !strings.HasSuffix(host, after) {
		return "", false
	}
	// 捕获可以为空：段内通配 app-*-dev 能匹配 app--dev；整段通配捕获为空时补回后是空段，下面的段校验会拒绝
	capture := host[len(before) : len(host)-len(after)]
	// 末尾整段通配：捕获的是一段或多段，每段都须合法
	if after == "" && strings.HasSuffix(before, ".") {
		for _, l := range strings.Split(capture, ".") {
			if !validLabel(l) {
				return "", false
			}
		}
		return capture, true
	}
	// 其余情况 * 不跨越 .，补回同段的固定部分后须为合法的段
	if strings.Contains(capture, ".") {
		return "", false
	}
	start := strings.LastIndexByte(before, '.') + 1
	end := strings.IndexByte(after, '.')
	if end < 0 {
		end = len(after)
	}
	if !validLabel(before[start:] + capture + after[:end]) {
		return "", false
	}
	return capture, true
}
