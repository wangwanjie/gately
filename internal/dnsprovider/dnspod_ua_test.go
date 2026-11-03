package dnsprovider

import (
	"strings"
	"testing"
)

// dnsapi.cn 会对含「DDNS Client」的 UA 返回 nginx 403，回归保护
func TestDNSPodUserAgentNotBlocked(t *testing.T) {
	ua := dnspodUserAgent()
	if !strings.HasPrefix(ua, "Gately/") {
		t.Fatalf("UA 应以 Gately/ 开头：%q", ua)
	}
	if strings.Contains(strings.ToLower(ua), "ddns client") {
		t.Fatalf("UA 不能含 DDNS Client：%q", ua)
	}
}
