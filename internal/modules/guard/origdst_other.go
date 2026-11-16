//go:build !linux

package guard

import "net"

// origDstPort 只在 Linux 上可用（nat 重定向只在 Linux 防火墙规则中使用），其他平台返回 0。
func origDstPort(net.Conn) int {
	return 0
}
