package sysstat

import (
	"encoding/binary"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/proc"

	"golang.org/x/sys/unix"
)

// macOS 上无法在不依赖 cgo 的情况下读取 CPU 与网卡累计计数，只提供静态信息，主要便于本地开发预览。
const clockTicks = 100

func readRaw() rawSample {
	return rawSample{at: time.Now()}
}

func fillPlatform(s *Snapshot) {
	s.Kernel, _ = unix.Sysctl("kern.osrelease")
	if v, err := unix.Sysctl("kern.osproductversion"); err == nil {
		s.Platform = "macOS " + v
	}
	s.CPU.Model, _ = unix.Sysctl("machdep.cpu.brand_string")
	if tv, err := unix.SysctlTimeval("kern.boottime"); err == nil {
		s.BootTime = tv.Sec
	}
	// vm.loadavg 对应 struct loadavg { fixpt_t ldavg[3]; long fscale; }
	if raw, err := unix.SysctlRaw("vm.loadavg"); err == nil && len(raw) >= 24 {
		scale := float64(binary.LittleEndian.Uint64(raw[16:24]))
		if scale > 0 {
			for i := 0; i < 3; i++ {
				s.CPU.Load = append(s.CPU.Load, float64(binary.LittleEndian.Uint32(raw[i*4:]))/scale)
			}
		}
	}
	total, err := unix.SysctlUint64("hw.memsize")
	if err != nil || total == 0 {
		return
	}
	m := &Memory{Total: total}
	// 可用内存按空闲页 + 可清除页 + 文件缓存页近似，与活动监视器的“已使用内存”口径接近
	page, _ := unix.SysctlUint32("hw.pagesize")
	var pages uint64
	for _, k := range []string{"vm.page_free_count", "vm.page_purgeable_count", "vm.page_pageable_external_count"} {
		n, _ := unix.SysctlUint32(k)
		pages += uint64(n)
	}
	avail := pages * uint64(page)
	if avail > total {
		avail = total
	}
	m.Available = avail
	m.Used = total - avail
	s.Memory = m
}

func fillProcessOS(p *Process) {}

// readChildren 在 macOS 上用 ps 读取托管进程的常驻内存与累计 CPU 时间（不依赖 cgo 读不到这些计数），
// 只在有托管进程时调用；线程数取不到。与 Linux 一样只接受父进程是 Gately 自身的 PID。
func readChildren(pids []int) map[int]childRaw {
	if len(pids) == 0 {
		return nil
	}
	list := make([]string, 0, len(pids))
	for _, pid := range pids {
		if pid > 0 {
			list = append(list, strconv.Itoa(pid))
		}
	}
	if len(list) == 0 {
		return nil
	}
	// PID 只来自托管器登记，均为数字，以单个参数传给 ps，不经过 shell
	out, err := proc.Output(2*time.Second, "/bin/ps", []string{"-o", "pid=,ppid=,rss=,time=", "-p", strings.Join(list, ",")}, []string{"LC_ALL=C"})
	if err != nil && len(out) == 0 {
		return nil
	}
	return parsePS(string(out), os.Getpid())
}
