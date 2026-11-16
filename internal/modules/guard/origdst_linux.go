package guard

import (
	"encoding/binary"
	"net"
	"syscall"
)

// soOriginalDst 为 netfilter 的 SO_ORIGINAL_DST（IPv4）与 IP6T_SO_ORIGINAL_DST（IPv6），两者取值相同
const soOriginalDst = 80

// origDstPort 读取被 nat 重定向的连接原本的目的端口，读不到时返回 0。
// getsockopt 返回 sockaddr_in / sockaddr_in6，借用标准库中大小足够的结构体接收（与常见透明代理的做法相同）。
func origDstPort(c net.Conn) int {
	tc, ok := c.(*net.TCPConn)
	if !ok {
		return 0
	}
	raw, err := tc.SyscallConn()
	if err != nil {
		return 0
	}
	v4 := false
	if la, ok := c.LocalAddr().(*net.TCPAddr); ok && la.IP.To4() != nil {
		v4 = true
	}
	port := 0
	_ = raw.Control(func(fd uintptr) {
		if v4 {
			// sockaddr_in：族(2) 端口(2，网络字节序) 地址(4)，落在 IPv6Mreq.Multiaddr 的前 8 字节
			if m, err := syscall.GetsockoptIPv6Mreq(int(fd), syscall.IPPROTO_IP, soOriginalDst); err == nil {
				port = int(binary.BigEndian.Uint16(m.Multiaddr[2:4]))
			}
			return
		}
		// sockaddr_in6 落在 IPv6MTUInfo.Addr 中，端口按内存中的字节（网络字节序）还原
		if info, err := syscall.GetsockoptIPv6MTUInfo(int(fd), syscall.IPPROTO_IPV6, soOriginalDst); err == nil {
			var b [2]byte
			binary.NativeEndian.PutUint16(b[:], info.Addr.Port)
			port = int(binary.BigEndian.Uint16(b[:]))
		}
	})
	return port
}
