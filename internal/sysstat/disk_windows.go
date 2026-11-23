package sysstat

import "golang.org/x/sys/windows"

// diskUsage 返回 dir 所在卷的容量。空闲取当前用户可用的字节数（受磁盘配额限制时更准确），
// 已用按“总量 - 整卷空闲”计算，与资源管理器的口径一致。
func diskUsage(dir string) *Disk {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return nil
	}
	var avail, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &avail, &total, &totalFree); err != nil || total == 0 {
		return nil
	}
	used := uint64(0)
	if totalFree <= total {
		used = total - totalFree
	}
	return &Disk{Path: dir, Total: total, Used: used, Free: avail}
}
