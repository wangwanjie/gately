package httpx

import (
	"net"
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	oldAddrs := interfaceAddrs
	defer func() {
		interfaceAddrs = oldAddrs
		localCache = nil
		SetProxyTrust(true, nil)
	}()
	// 本机网卡地址：局域网 IPv4 192.168.1.8
	interfaceAddrs = func() ([]net.Addr, error) {
		return []net.Addr{&net.IPNet{IP: net.ParseIP("192.168.1.8"), Mask: net.CIDRMask(24, 32)}}, nil
	}
	localCache = nil
	extra, err := ParsePrefixes([]string{"10.9.0.0/16", "172.17.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	SetProxyTrust(true, extra)

	cases := []struct {
		name, peer, realIP, xff, want string
	}{
		{"外部直连不读请求头", "203.0.113.5:1234", "1.2.3.4", "1.2.3.4", "203.0.113.5"},
		{"本机网卡上的代理用 X-Real-IP", "192.168.1.8:5555", "2001:db8::1", "", "2001:db8::1"},
		{"回环代理只有 X-Real-IP", "127.0.0.1:5555", "198.51.100.7", "", "198.51.100.7"},
		{"两个请求头一致", "127.0.0.1:5555", "198.51.100.7", "1.1.1.1, 198.51.100.7", "198.51.100.7"},
		// 只追加 XFF 的代理（haproxy、Caddy）原样转发客户端伪造的 X-Real-IP
		{"伪造 X-Real-IP 为内网地址", "127.0.0.1:5555", "192.168.1.50", "203.0.113.9", "203.0.113.9"},
		{"伪造 X-Real-IP 为公网地址绕过限流", "127.0.0.1:5555", "1.1.1.1", "203.0.113.9", "203.0.113.9"},
		{"伪造 X-Real-IP 为回环地址", "127.0.0.1:5555", "127.0.0.1", "203.0.113.9", "203.0.113.9"},
		// 只写 X-Real-IP 的 nginx 原样转发客户端伪造的 XFF
		{"伪造 XFF 为内网地址", "127.0.0.1:5555", "203.0.113.9", "10.0.0.5", "203.0.113.9"},
		{"伪造 XFF 为 CGNAT 地址", "127.0.0.1:5555", "203.0.113.9", "100.100.1.1", "203.0.113.9"},
		{"XFF 全是可信代理时用 X-Real-IP", "127.0.0.1:5555", "198.51.100.7", "127.0.0.1", "198.51.100.7"},
		{"XFF 格式异常时用 X-Real-IP", "127.0.0.1:5555", "198.51.100.7", "garbage", "198.51.100.7"},
		{"内网客户端两头一致", "127.0.0.1:5555", "192.168.1.20", "192.168.1.20", "192.168.1.20"},
		{"IPv4 映射地址视为同一地址", "127.0.0.1:5555", "::ffff:203.0.113.9", "203.0.113.9", "203.0.113.9"},
		{"XFF 自右向左跳过可信代理", "127.0.0.1:5555", "", "6.6.6.6, 198.51.100.7, 127.0.0.1", "198.51.100.7"},
		{"额外信任的地址段", "10.9.3.4:80", "", "198.51.100.8", "198.51.100.8"},
		{"额外信任的单个 IP", "172.17.0.1:80", "198.51.100.9", "", "198.51.100.9"},
		{"可信代理但没有请求头", "192.168.1.8:5555", "", "", "192.168.1.8"},
		{"X-Real-IP 非法时回退 XFF", "127.0.0.1:1", "not-an-ip", "198.51.100.10", "198.51.100.10"},
		{"XFF 格式异常时返回对端", "127.0.0.1:1", "", "garbage", "127.0.0.1"},
		{"局域网其他机器不是可信代理", "192.168.1.20:1", "8.8.8.8", "", "192.168.1.20"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.peer
		if c.realIP != "" {
			r.Header.Set("X-Real-IP", c.realIP)
		}
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := ClientIP(r); got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}

	// 关闭后一律使用对端地址
	SetProxyTrust(false, nil)
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:1"
	r.Header.Set("X-Real-IP", "198.51.100.7")
	if got := ClientIP(r); got != "127.0.0.1" {
		t.Fatalf("关闭信任后应返回对端地址，实际 %s", got)
	}
	if _, err := ParsePrefixes([]string{"nope"}); err == nil {
		t.Fatal("非法地址应报错")
	}
}
