package fwopen

import (
	"net"
	"sort"
	"strings"
)

// ForListen 计算一个监听需要放行的端口：network 为 tcp / tcp4 / tcp6 / udp / udp4 / udp6，
// host 为监听地址（空或 0.0.0.0、:: 表示所有地址），ports 为实际监听成功的端口。
// 连续端口合并为区间，绑定具体地址时只放行发往该地址的连接；
// 只监听回环地址（本机访问）时返回 nil，调用方无需放行。
func ForListen(network, host string, ports []int) []Port {
	network = strings.ToLower(network)
	proto := strings.TrimRight(network, "46")
	if proto != "tcp" && proto != "udp" {
		return nil
	}
	family := strings.TrimPrefix(network, proto)
	host = strings.Trim(strings.TrimSpace(host), "[]")
	// 去掉 IPv6 区域标识（fe80::1%eth0），防火墙按地址匹配
	if i := strings.IndexByte(host, '%'); i >= 0 {
		host = host[:i]
	}
	addr := ""
	if host != "" {
		ip := net.ParseIP(host)
		if ip == nil {
			// 不是 IP（例如主机名）时无法确定监听地址，不放行，避免放行范围超出监听
			return nil
		}
		if ip.IsLoopback() {
			return nil
		}
		// 监听 0.0.0.0 或 :: 时不限定地址：与 Go 的 net.Listen 一致，tcp / udp 为双栈，tcp4 / tcp6 按后缀区分
		if !ip.IsUnspecified() {
			addr = ip.String()
		}
	}
	var out []Port
	for _, r := range collapse(ports) {
		out = append(out, Port{Proto: proto, Port: r[0], End: r[1], Family: family, Addr: addr})
	}
	return out
}

// LoopbackNote 是只监听本机回环地址、无需放行时展示的说明。
const LoopbackNote = "只监听本机回环地址，无需放行防火墙"

// IsLoopback 判断监听地址是否为本机回环地址（127.0.0.0/8、::1），这类监听从外部无法访问，无需放行。
func IsLoopback(host string) bool {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if i := strings.IndexByte(host, '%'); i >= 0 {
		host = host[:i]
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// collapse 去重排序并把连续端口合并为 [起, 止] 区间，止与起相同时为 0。
func collapse(ports []int) [][2]int {
	list := make([]int, 0, len(ports))
	seen := map[int]bool{}
	for _, p := range ports {
		if p <= 0 || p > 65535 || seen[p] {
			continue
		}
		seen[p] = true
		list = append(list, p)
	}
	sort.Ints(list)
	var out [][2]int
	for i := 0; i < len(list); {
		j := i
		for j+1 < len(list) && list[j+1] == list[j]+1 {
			j++
		}
		end := 0
		if j > i {
			end = list[j]
		}
		out = append(out, [2]int{list[i], end})
		i = j + 1
	}
	return out
}
