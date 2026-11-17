package gen

import (
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	reIPv4   = regexp.MustCompile(`\b(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\b`)
	reDomain = regexp.MustCompile(`\b((?:[a-z0-9-]+\.)+[a-z]{2,})\b`)
	reserved = []string{"192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24", "127.0.0.0/8", "0.0.0.0/8", "255.255.255.255/32", "2001:db8::/32"}
)

// 允许出现的域名后缀：文档保留域名与程序自身相关的说明地址。
var allowedDomain = []string{"example.com", "example.net", "example.org", "duckdns.org", "ts.net", "localhost", "duckduckgo.com", "wikipedia.org"}

// 只检查这些常见顶级域：事件名（guard.banned）等形似域名的内部标识不在检查范围。
var checkedTLD = regexp.MustCompile(`\.(com|net|org|cn|io|dev|me|xyz|cc|tv|info|app|top|cloud)$`)

func isReservedIP(s string) bool {
	a, err := netip.ParseAddr(s)
	if err != nil {
		return true
	}
	for _, p := range reserved {
		if netip.MustParsePrefix(p).Contains(a) {
			return true
		}
	}
	return false
}

// FindUnreservedAddresses 扫描生成的 JSON 配置（跳过证书与密钥 PEM），返回不在文档保留范围内的 IPv4 地址与域名。
func FindUnreservedAddresses(dir string) []string {
	var bad []string
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	more, _ := filepath.Glob(filepath.Join(dir, "*", "*.json"))
	for _, f := range append(files, more...) {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		text := rePEM.ReplaceAllString(string(b), "")
		text = reHash.ReplaceAllString(text, "")
		for _, m := range reIPv4.FindAllString(text, -1) {
			if !isReservedIP(m) {
				bad = append(bad, filepath.Base(f)+": "+m)
			}
		}
		for _, m := range reDomain.FindAllString(strings.ToLower(text), -1) {
			if !checkedTLD.MatchString(m) {
				continue
			}
			ok := false
			for _, s := range allowedDomain {
				if m == s || strings.HasSuffix(m, "."+s) {
					ok = true
				}
			}
			// 文件名、版本号等形似域名的字符串
			if !ok && !strings.HasSuffix(m, ".json") && !strings.HasSuffix(m, ".log") && !strings.HasSuffix(m, ".html") && !strings.HasSuffix(m, ".xml") && !strings.HasSuffix(m, ".js") && !strings.HasSuffix(m, ".css") && !strings.HasSuffix(m, ".php") && !strings.HasSuffix(m, ".crt") && !strings.HasSuffix(m, ".key") {
				bad = append(bad, filepath.Base(f)+": "+m)
			}
		}
	}
	return bad
}

var (
	rePEM  = regexp.MustCompile(`-----BEGIN [A-Z ]+-----[^"]*?-----END [A-Z ]+-----\\n`)
	reHash = regexp.MustCompile(`\$2[aby]\$[^"]+`)
)
