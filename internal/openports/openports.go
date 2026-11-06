// Package openports 登记各模块临时打开、不一定随时处于监听状态的端口（如 FTP 被动模式的数据端口）。
// 入侵防护的端口扫描检测按 /proc/net/tcp 中正在监听的端口判断开放端口，几秒内开了又关的端口
// 往往读不到；模块在这里登记后，扫描检测把它们一并视为开放端口，访客连接时不会被当成扫描。
package openports

import (
	"sort"
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
)

// Set 设置 owner 登记的端口，覆盖之前的登记；Ranges 为空且 Dynamic 为 false 时等同 Clear。
func Set(owner string, e Entry) {
	mu.Lock()
	defer mu.Unlock()
	if len(e.Ranges) == 0 && !e.Dynamic {
		if _, ok := entries[owner]; ok {
			delete(entries, owner)
			version++
		}
		return
	}
	entries[owner] = Entry{Ranges: append([]Range(nil), e.Ranges...), Dynamic: e.Dynamic}
	version++
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
