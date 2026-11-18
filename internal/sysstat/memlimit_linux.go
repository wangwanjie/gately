package sysstat

import (
	"os"
	"strconv"
	"strings"
)

// MemoryLimit 返回本进程实际可用的内存上限（字节）：物理内存与 cgroup 内存限制中较小的一个，
// 读不到时返回 0。容器里 /proc/meminfo 显示的是宿主机内存，必须再看 cgroup 的限制。
func MemoryLimit() uint64 {
	var total uint64
	if m := meminfo(); m != nil {
		total = m.Total
	}
	// cgroup v2 为 memory.max（不限制时为 max），v1 为 memory.limit_in_bytes（不限制时是一个极大的数）
	for _, p := range []string{"/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory/memory.limit_in_bytes"} {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		v, err := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
		if err != nil || v == 0 {
			continue
		}
		if total == 0 || v < total {
			total = v
		}
		break
	}
	return total
}
