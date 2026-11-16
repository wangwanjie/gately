package httpx

import (
	"net"
	"net/netip"
	"testing"
)

// 后台地址判定：监听全部地址时只认本机地址的同一端口，监听具体地址时只认该地址；未登记时一律不是后台。
func TestIsAdminAddr(t *testing.T) {
	oldAddrs := interfaceAddrs
	defer func() {
		interfaceAddrs = oldAddrs
		localCache = nil
		SetAdminListen(nil)
	}()
	interfaceAddrs = func() ([]net.Addr, error) {
		return []net.Addr{&net.IPNet{IP: net.ParseIP("192.168.1.8"), Mask: net.CIDRMask(24, 32)}}, nil
	}
	localCache = nil
	ip := netip.MustParseAddr

	if IsAdminAddr(ip("127.0.0.1"), 16680) {
		t.Fatal("未登记时不应判定为后台")
	}
	SetAdminListen([]string{"[::]:16680", "127.0.0.1:8443", "bad", "1.2.3.4:0", ":x"})
	for _, tc := range []struct {
		addr string
		port uint16
		want bool
	}{
		{"127.0.0.1", 16680, true},
		{"::1", 16680, true},
		{"::ffff:127.0.0.1", 16680, true},
		{"192.168.1.8", 16680, true},
		{"192.168.1.9", 16680, false}, // 局域网其他主机
		{"127.0.0.1", 16681, false},   // 本机其他端口
		{"127.0.0.1", 8443, true},
		{"::1", 8443, false}, // 只监听 127.0.0.1 的端口
		{"192.168.1.8", 8443, false},
	} {
		if got := IsAdminAddr(ip(tc.addr), tc.port); got != tc.want {
			t.Errorf("%s:%d 应为 %v", tc.addr, tc.port, tc.want)
		}
	}
	SetAdminListen(nil)
	if IsAdminAddr(ip("127.0.0.1"), 16680) {
		t.Fatal("取消登记后不应判定为后台")
	}
}
