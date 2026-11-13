// Package openports 登记各模块临时打开、不一定随时处于监听状态的端口（如 FTP 被动模式的数据端口）。
// 入侵防护的端口扫描检测按 /proc/net/tcp 中正在监听的端口判断开放端口，几秒内开了又关的端口
// 往往读不到；模块在这里登记后，扫描检测把它们一并视为开放端口，访客连接时不会被当成扫描。
package openports

import (
	"net"
	"slices"
	"sort"
	"strings"
	"sync"
)

// Range 是一段端口范围，Lo 与 Hi 均包含。
type Range struct {
	Lo, Hi int
}

// Entry 是一个模块登记的端口。
type Entry struct {
	// Ranges 为固定的端口范围
	Ranges []Range
	// Dynamic 表示模块还会使用由系统随机分配、无法事先列出的端口
	Dynamic bool
}

var (
	mu      sync.Mutex
	entries = map[string]Entry{}
	// version 在登记内容变化时递增，便于使用方判断是否需要重新计算
	version uint64
	// wake 在登记内容变化时收到一个信号（容量为 1，多次变化合并为一次），入侵防护据此立即刷新开放端口集合
	wake = make(chan struct{}, 1)
)

// Notify 返回登记内容变化的通知通道。多次变化合并为一个信号，使用方收到后重新读取 All。
func Notify() <-chan struct{} {
	return wake
}

// signal 发出变化通知，通道已有未处理的信号时直接返回，不阻塞调用方。
func signal() {
	select {
	case wake <- struct{}{}:
	default:
	}
}

// Set 设置 owner 登记的端口，覆盖之前的登记；Ranges 为空且 Dynamic 为 false 时等同 Clear。
func Set(owner string, e Entry) {
	mu.Lock()
	defer mu.Unlock()
	if len(e.Ranges) == 0 && !e.Dynamic {
		if _, ok := entries[owner]; ok {
			delete(entries, owner)
			version++
			signal()
		}
		return
	}
	next := Entry{Ranges: append([]Range(nil), e.Ranges...), Dynamic: e.Dynamic}
	// 内容没有变化时不通知，避免模块周期性重复登记引发无谓的防火墙刷新
	if old, ok := entries[owner]; ok && old.Dynamic == next.Dynamic && slices.Equal(old.Ranges, next.Ranges) {
		return
	}
	entries[owner] = next
	version++
	signal()
}

// SetPorts 按端口列表登记 owner 的端口：去重排序，连续端口合并为区间；列表为空时等同 Clear。
// 超出 1–65535 的端口忽略。
func SetPorts(owner string, ports []int) {
	list := make([]int, 0, len(ports))
	for _, p := range ports {
		if p > 0 && p <= 65535 {
			list = append(list, p)
		}
	}
	sort.Ints(list)
	list = slices.Compact(list)
	var ranges []Range
	for _, p := range list {
		if n := len(ranges); n > 0 && ranges[n-1].Hi+1 == p {
			ranges[n-1].Hi = p
			continue
		}
		ranges = append(ranges, Range{Lo: p, Hi: p})
	}
	Set(owner, Entry{Ranges: ranges})
}

// Clear 删除 owner 的登记。
func Clear(owner string) {
	Set(owner, Entry{})
}

// All 返回全部登记的端口范围，以及使用随机端口的模块（按名称排序）。
func All() (ranges []Range, dynamic []string) {
	mu.Lock()
	defer mu.Unlock()
	for owner, e := range entries {
		ranges = append(ranges, e.Ranges...)
		if e.Dynamic {
			dynamic = append(dynamic, owner)
		}
	}
	sort.Strings(dynamic)
	return ranges, dynamic
}

// holdOwner 是 Hold 登记端口时使用的 owner 名称
const holdOwner = "listeners"

var (
	holdMu sync.Mutex
	// holds 记录每个端口被多少个监听占用（同一端口可能同时有 IPv4 与 IPv6 两个监听）
	holds = map[int]int{}
)

// Hold 登记一个刚刚监听成功的 TCP 地址，返回撤销登记的函数（重复调用只生效一次）。
// 监听成功的端口在 /proc/net/tcp 中要等入侵防护下一次轮询才能读到；登记后立即视为开放端口，
// 并通知入侵防护马上刷新，避免新监听的端口在这个空档里被扫描检测当成未开放端口。
// 只监听回环地址、不是 TCP 地址或端口为 0 时返回空操作的函数。
func Hold(addr net.Addr) func() {
	tcp, ok := addr.(*net.TCPAddr)
	if !ok || tcp.Port <= 0 || (tcp.IP != nil && tcp.IP.IsLoopback()) {
		return func() {}
	}
	port := tcp.Port
	holdMu.Lock()
	holds[port]++
	publishHoldsLocked()
	holdMu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			holdMu.Lock()
			defer holdMu.Unlock()
			if holds[port] <= 1 {
				delete(holds, port)
			} else {
				holds[port]--
			}
			publishHoldsLocked()
		})
	}
}

// publishHoldsLocked 把当前占用的端口登记为 holdOwner 的端口。调用方持有 holdMu。
func publishHoldsLocked() {
	ports := make([]int, 0, len(holds))
	for p := range holds {
		ports = append(ports, p)
	}
	SetPorts(holdOwner, ports)
}

// HoldHostPort 登记 host:port 形式的 TCP 监听地址（如托管程序的 SOCKS5 监听），语义同 Hold。
// host 为空表示监听所有地址；localhost 与回环地址不登记；无法解析的地址返回空操作的函数。
func HoldHostPort(hostport string) func() {
	host, portText, err := net.SplitHostPort(hostport)
	if err != nil {
		return func() {}
	}
	port, err := net.LookupPort("tcp", portText)
	if err != nil {
		return func() {}
	}
	host = strings.Trim(host, "[]")
	switch {
	case host == "":
		return Hold(&net.TCPAddr{IP: net.IPv6unspecified, Port: port})
	case strings.EqualFold(host, "localhost"):
		return func() {}
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return func() {}
	}
	return Hold(&net.TCPAddr{IP: ip, Port: port})
}
