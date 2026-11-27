package sysstat

import "os"

// defaultRouteIfaces 读取当前网络命名空间的路由表，返回承载默认路由的网卡。容器里读到的是容器自己的路由表。
func defaultRouteIfaces() map[string]bool {
	v4, _ := os.ReadFile("/proc/net/route")
	v6, _ := os.ReadFile("/proc/net/ipv6_route")
	return parseDefaultRoutes(string(v4), string(v6))
}
