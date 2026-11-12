package webservice

import (
	"net/http"
	"testing"
)

// typicalHeaders 是浏览器常见的一组请求头，用于基准测试来源类请求头的清理。
func typicalHeaders() http.Header {
	h := http.Header{}
	for k, v := range map[string]string{
		"Host": "a.test", "User-Agent": "Mozilla/5.0", "Accept": "text/html", "Accept-Language": "zh-CN",
		"Accept-Encoding": "gzip, br", "Cookie": "a=b", "Referer": "https://a.test/", "Sec-Fetch-Site": "same-origin",
		"Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "document", "Sec-Ch-Ua": "x", "Sec-Ch-Ua-Mobile": "?0",
		"Sec-Ch-Ua-Platform": "macOS", "Upgrade-Insecure-Requests": "1", "Cache-Control": "max-age=0",
	} {
		h.Set(k, v)
	}
	return h
}

// BenchmarkStripForwardedHeaders 衡量每个反代请求都要执行的来源类与源端口头清理。
func BenchmarkStripForwardedHeaders(b *testing.B) {
	base := typicalHeaders()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		h := base.Clone()
		stripForwardedHeaders(h)
		stripClientPort(h)
	}
}

// TestHeaderVariantEqual 覆盖不分配内存的变体比较：大小写不敏感、- 与 _ 等价，长度不同或其他字符不同则不相等。
func TestHeaderVariantEqual(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"X-Real-Ip", "x_real_IP", true}, {"X-Forwarded-For", "X-FORWARDED-FOR", true}, {"X-Real-Ip", "X-Real-Ipx", false},
		{"X-Real-Ip", "X-Real.Ip", false}, {"", "", true}, {"a", "B", false},
	} {
		if got := headerVariantEqual(c.a, c.b); got != c.want {
			t.Errorf("%q vs %q: %v", c.a, c.b, got)
		}
	}
}
