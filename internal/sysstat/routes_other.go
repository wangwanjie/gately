//go:build !linux

package sysstat

// defaultRouteIfaces 在非 Linux 平台上不读取路由表，虚拟网卡按名称规则隐藏。
func defaultRouteIfaces() map[string]bool {
	return nil
}
