package httpx

import (
	"net"
	"net/netip"
	"strconv"
	"sync"
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
		if ap, ok := parseListenAddr(a); ok {
			out = append(out, ap)
		}
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
	if list == nil {
		return false
	}
	return matchListen(*list, ip, port)
}

// parseListenAddr 解析 net.Listener.Addr().String() 形式的地址，主机为空时视为全部地址。
func parseListenAddr(a string) (netip.AddrPort, bool) {
	host, port, err := net.SplitHostPort(a)
	if err != nil {
		return netip.AddrPort{}, false
	}
	p, err := strconv.ParseUint(port, 10, 16)
	if err != nil || p == 0 {
		return netip.AddrPort{}, false
	}
	ip := netip.IPv6Unspecified()
	if host != "" {
		parsed, err := netip.ParseAddr(host)
		if err != nil {
			return netip.AddrPort{}, false
		}
		ip = parsed.Unmap().WithZone("")
	}
	return netip.AddrPortFrom(ip, uint16(p)), true
}

// matchListen 判断 ip:port 是否落在 list 中的某个监听上，规则见 IsAdminAddr。
func matchListen(list []netip.AddrPort, ip netip.Addr, port uint16) bool {
	if !ip.IsValid() {
		return false
	}
	ip = ip.Unmap().WithZone("")
	for _, a := range list {
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

// Web 服务的监听地址：站点后端指向本实例另一个 Web 服务监听器时（443 站点 → 本机 444 监听器 → 管理后台），
// 请求仍在本进程内，后台登录 Cookie 要原样交给下一跳，由它按自己的后端再判断是否剥离。

var (
	webListenMu  sync.Mutex
	webListenSeq uint64
	webListen    = map[uint64]netip.AddrPort{}
	// webListenList 为 webListen 的快照，供每个请求无锁读取
	webListenList atomic.Pointer[[]netip.AddrPort]
)

// HoldWebListen 登记一个 Web 服务监听器的实际地址，返回撤销函数（可重复调用）；无法解析的地址不登记。
func HoldWebListen(addr net.Addr) (release func()) {
	if addr == nil {
		return func() {}
	}
	ap, ok := parseListenAddr(addr.String())
	if !ok {
		return func() {}
	}
	webListenMu.Lock()
	webListenSeq++
	id := webListenSeq
	webListen[id] = ap
	snapshotWebListenLocked()
	webListenMu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			webListenMu.Lock()
			delete(webListen, id)
			snapshotWebListenLocked()
			webListenMu.Unlock()
		})
	}
}

func snapshotWebListenLocked() {
	if len(webListen) == 0 {
		webListenList.Store(nil)
		return
	}
	list := make([]netip.AddrPort, 0, len(webListen))
	for _, ap := range webListen {
		list = append(list, ap)
	}
	webListenList.Store(&list)
}

// IsWebListenAddr 判断 ip:port 是否落在本实例某个 Web 服务监听器上，规则同 IsAdminAddr。
func IsWebListenAddr(ip netip.Addr, port uint16) bool {
	list := webListenList.Load()
	if list == nil {
		return false
	}
	return matchListen(*list, ip, port)
}
