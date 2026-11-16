package httpx

import (
	"net"
	"net/netip"
	"strconv"
	"sync/atomic"
)

// 管理后台的监听地址：后台常经本机 Web 服务站点反向代理访问（站点后端指向后台端口），
// 这类站点必须把后台登录 Cookie 转给后端，其余后端一律不能收到它。管理后台在监听成功后登记实际地址，
// Web 服务据此判断某个后端是不是本实例的后台。

// adminListen 为登记的后台监听地址，未登记时为 nil。
var adminListen atomic.Pointer[[]netip.AddrPort]

// SetAdminListen 登记管理后台实际监听的 TCP 地址（net.Listener.Addr().String() 的形式，例如 [::]:16680、127.0.0.1:16680），
// 无法解析的地址忽略；传入空列表取消登记。
func SetAdminListen(addrs []string) {
	var out []netip.AddrPort
	for _, a := range addrs {
		host, port, err := net.SplitHostPort(a)
		if err != nil {
			continue
		}
		p, err := strconv.ParseUint(port, 10, 16)
		if err != nil || p == 0 {
			continue
		}
		ip := netip.IPv6Unspecified()
		if host != "" {
			parsed, err := netip.ParseAddr(host)
			if err != nil {
				continue
			}
			ip = parsed.Unmap().WithZone("")
		}
		out = append(out, netip.AddrPortFrom(ip, uint16(p)))
	}
	if len(out) == 0 {
		adminListen.Store(nil)
		return
	}
	adminListen.Store(&out)
}

// IsAdminAddr 判断 ip:port 是否落在本实例管理后台的监听上：端口与某个登记的地址相同，
// 且该地址为全部地址（0.0.0.0、::）时 ip 是本机地址（回环或本机网卡地址），为具体地址时 ip 与之相同。
// 没有登记任何地址时返回 false。
func IsAdminAddr(ip netip.Addr, port uint16) bool {
	list := adminListen.Load()
	if list == nil || !ip.IsValid() {
		return false
	}
	ip = ip.Unmap().WithZone("")
	for _, a := range *list {
		if a.Port() != port {
			continue
		}
		if a.Addr().IsUnspecified() {
			if ip.IsLoopback() || localAddrs().has(ip) {
				return true
			}
			continue
		}
		if a.Addr() == ip {
			return true
		}
	}
	return false
}
