package sysstat

import (
	"os"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// clockTicks 是 Windows CPU 时间（FILETIME）的单位：每秒 1000 万个 100 纳秒。
// 主机与进程的 CPU 计数都直接保存为 100 纳秒数，计算使用率时按此换算成秒。
const clockTicks = 10_000_000

// x/sys/windows 没有封装的几个 kernel32 函数，按需加载。
var (
	modKernel32              = windows.NewLazySystemDLL("kernel32.dll")
	procGetSystemTimes       = modKernel32.NewProc("GetSystemTimes")
	procGlobalMemoryStatusEx = modKernel32.NewProc("GlobalMemoryStatusEx")
	procGetTickCount64       = modKernel32.NewProc("GetTickCount64")
	// K32GetProcessMemoryInfo 自 Windows 7 起由 kernel32 导出，不必再加载 psapi.dll
	procGetProcessMemoryInfo = modKernel32.NewProc("K32GetProcessMemoryInfo")
)

// memoryStatusEx 对应 MEMORYSTATUSEX。
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// processMemoryCounters 对应 PROCESS_MEMORY_COUNTERS。
type processMemoryCounters struct {
	Cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

func filetimeValue(ft windows.Filetime) uint64 {
	return uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime)
}

// readRaw 读取整机 CPU 时间、Gately 进程 CPU 时间与网卡流量计数。
func readRaw() rawSample {
	s := rawSample{at: time.Now(), ifBytes: map[string][2]uint64{}}
	var idle, kernel, user windows.Filetime
	if r, _, _ := procGetSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user))); r != 0 {
		// 内核时间已包含空闲时间，总时间 = 内核 + 用户
		s.cpuTotal = filetimeValue(kernel) + filetimeValue(user)
		s.cpuIdle = filetimeValue(idle)
		s.ok = true
	}
	if t, ok := processTimes(windows.CurrentProcess()); ok {
		s.procTicks = t
	}
	readIfBytes(s.ifBytes)
	return s
}

// processTimes 返回进程累计的内核 + 用户 CPU 时间（100 纳秒）。
func processTimes(h windows.Handle) (uint64, bool) {
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return 0, false
	}
	return filetimeValue(kernel) + filetimeValue(user), true
}

// processRSS 返回进程的工作集大小，即任务管理器“内存”列使用的常驻内存口径。
func processRSS(h windows.Handle) uint64 {
	var pmc processMemoryCounters
	pmc.Cb = uint32(unsafe.Sizeof(pmc))
	if r, _, _ := procGetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&pmc)), uintptr(pmc.Cb)); r == 0 {
		return 0
	}
	return uint64(pmc.WorkingSetSize)
}

// readIfBytes 读取各网卡的累计收发字节，按网卡别名（与 net.Interfaces 返回的名称相同）索引。
// 同一别名出现多行时（筛选驱动等附加层），取计数更大的一行。
func readIfBytes(out map[string][2]uint64) {
	var table *windows.MibIfTable2
	if err := windows.GetIfTable2Ex(windows.MibIfTableNormal, &table); err != nil || table == nil {
		return
	}
	defer windows.FreeMibTable(unsafe.Pointer(table))
	rows := unsafe.Slice(&table.Table[0], table.NumEntries)
	for i := range rows {
		name := windows.UTF16ToString(rows[i].Alias[:])
		if name == "" {
			continue
		}
		v := [2]uint64{rows[i].InOctets, rows[i].OutOctets}
		if old, ok := out[name]; ok && old[0]+old[1] >= v[0]+v[1] {
			continue
		}
		out[name] = v
	}
}

// fillPlatform 读取系统版本、CPU 型号、开机时间与内存。Windows 没有负载均值，Load 留空。
func fillPlatform(s *Snapshot) {
	s.Platform, s.Kernel = windowsVersion()
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE); err == nil {
		if v, _, err := k.GetStringValue("ProcessorNameString"); err == nil {
			s.CPU.Model = strings.TrimSpace(v)
		}
		k.Close()
	}
	if r, _, _ := procGetTickCount64.Call(); r != 0 {
		s.BootTime = time.Now().Add(-time.Duration(r) * time.Millisecond).Unix()
	}
	s.Memory = memoryStatus()
}

// windowsVersion 返回产品名称（如“Windows 11 Pro 25H2”）与内核版本（如“10.0.26200.7462”）。
// 版本号取自 RtlGetVersion，不受程序清单兼容性设置影响；注册表中 Windows 11 的 ProductName 仍写作
// “Windows 10 …”，按内部版本号 22000 及以上改正。
func windowsVersion() (platform, kernel string) {
	v := windows.RtlGetVersion()
	kernel = strconv.Itoa(int(v.MajorVersion)) + "." + strconv.Itoa(int(v.MinorVersion)) + "." + strconv.Itoa(int(v.BuildNumber))
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return "Windows", kernel
	}
	defer k.Close()
	if ubr, _, err := k.GetIntegerValue("UBR"); err == nil {
		kernel += "." + strconv.FormatUint(ubr, 10)
	}
	platform, _, _ = k.GetStringValue("ProductName")
	if platform == "" {
		platform = "Windows"
	}
	if v.BuildNumber >= 22000 && strings.HasPrefix(platform, "Windows 10") {
		platform = "Windows 11" + strings.TrimPrefix(platform, "Windows 10")
	}
	if dv, _, err := k.GetStringValue("DisplayVersion"); err == nil && dv != "" {
		platform += " " + dv
	}
	return platform, kernel
}

// memoryStatus 读取物理内存与页面文件。页面文件大小 = 提交上限 - 物理内存；
// 页面文件已用量按“已提交 - 已用物理内存”近似，限制在 0 到页面文件大小之间。
func memoryStatus() *Memory {
	var ms memoryStatusEx
	ms.Length = uint32(unsafe.Sizeof(ms))
	if r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&ms))); r == 0 || ms.TotalPhys == 0 {
		return nil
	}
	avail := ms.AvailPhys
	if avail > ms.TotalPhys {
		avail = ms.TotalPhys
	}
	m := &Memory{Total: ms.TotalPhys, Available: avail, Used: ms.TotalPhys - avail}
	if ms.TotalPageFile > ms.TotalPhys {
		m.SwapTotal = ms.TotalPageFile - ms.TotalPhys
	}
	if ms.TotalPageFile >= ms.AvailPageFile {
		committed := ms.TotalPageFile - ms.AvailPageFile
		if committed > m.Used {
			m.SwapUsed = min(committed-m.Used, m.SwapTotal)
		}
	}
	return m
}

// fillProcessOS 读取 Gately 进程的工作集与线程数。Windows 没有文件描述符，FDs 留空。
func fillProcessOS(p *Process) {
	p.RSS = processRSS(windows.CurrentProcess())
	if ents, err := processEntries(); err == nil {
		if e, ok := ents[uint32(os.Getpid())]; ok {
			p.Threads = int(e.Threads)
		}
	}
}

// processEntries 用进程快照列出所有进程的父进程与线程数，按 PID 索引。
func processEntries() (map[uint32]windows.ProcessEntry32, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)
	out := map[uint32]windows.ProcessEntry32{}
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		out[e.ProcessID] = e
	}
	return out, nil
}

// readChildren 读取托管进程的 CPU 时间、工作集与线程数。与 Linux 一样只接受父进程是 Gately 自身的 PID，
// 进程退出后 PID 被其他进程复用时不会把别人的占用算进来。
func readChildren(pids []int) map[int]childRaw {
	if len(pids) == 0 {
		return nil
	}
	ents, err := processEntries()
	if err != nil {
		return nil
	}
	self := uint32(os.Getpid())
	out := make(map[int]childRaw, len(pids))
	for _, pid := range pids {
		if pid <= 0 {
			continue
		}
		e, ok := ents[uint32(pid)]
		if !ok || e.ParentProcessID != self {
			continue
		}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
		if err != nil {
			continue
		}
		ticks, ok := processTimes(h)
		rss := processRSS(h)
		windows.CloseHandle(h)
		if !ok {
			continue
		}
		out[pid] = childRaw{ticks: ticks, rss: rss, threads: int(e.Threads)}
	}
	return out
}
