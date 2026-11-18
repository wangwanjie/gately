//go:build !linux

package sysstat

// MemoryLimit 在非 Linux 平台返回 0（未知）。这些平台多为桌面与服务器系统，内存充足。
func MemoryLimit() uint64 {
	return 0
}
