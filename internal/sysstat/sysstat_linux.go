package sysstat

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"
)

// clockTicks 是 /proc 中 CPU 时间的单位（USER_HZ），Linux 各架构上固定为 100。
const clockTicks = 100

// readRaw 读取 /proc 中的累计计数器。
func readRaw() rawSample {
	s := rawSample{at: time.Now(), ifBytes: map[string][2]uint64{}}
	if b, err := os.ReadFile("/proc/stat"); err == nil {
		line, _, _ := strings.Cut(string(b), "\n")
		f := strings.Fields(line)
		// cpu user nice system idle iowait irq softirq steal ...
		if len(f) >= 5 && f[0] == "cpu" {
			for i := 1; i < len(f) && i <= 8; i++ {
				v, _ := strconv.ParseUint(f[i], 10, 64)
				s.cpuTotal += v
				if i == 4 || i == 5 {
					s.cpuIdle += v
				}
			}
			s.ok = true
		}
	}
	if b, err := os.ReadFile("/proc/self/stat"); err == nil {
		// 进程名可能包含空格和括号，从最后一个右括号之后开始解析；utime、stime 是第 14、15 个字段
		str := string(b)
		if i := strings.LastIndexByte(str, ')'); i >= 0 {
			f := strings.Fields(str[i+1:])
			if len(f) > 12 {
				ut, _ := strconv.ParseUint(f[11], 10, 64)
				st, _ := strconv.ParseUint(f[12], 10, 64)
				s.procTicks = ut + st
			}
		}
	}
	if f, err := os.Open("/proc/net/dev"); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			name, rest, ok := strings.Cut(sc.Text(), ":")
			if !ok {
				continue
			}
			v := strings.Fields(rest)
			if len(v) < 9 {
				continue
			}
			rx, _ := strconv.ParseUint(v[0], 10, 64)
			tx, _ := strconv.ParseUint(v[8], 10, 64)
			s.ifBytes[strings.TrimSpace(name)] = [2]uint64{rx, tx}
		}
		f.Close()
	}
	return s
}

// fillPlatform 读取 CPU 型号、内存、负载、内核与发行版信息。
func fillPlatform(s *Snapshot) {
	s.Kernel = readTrim("/proc/sys/kernel/osrelease")
	s.Platform = osRelease()
	s.CPU.Model = cpuModel()
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		f := strings.Fields(string(b))
		if len(f) >= 3 {
			for _, x := range f[:3] {
				v, _ := strconv.ParseFloat(x, 64)
				s.CPU.Load = append(s.CPU.Load, v)
			}
		}
	}
	if b, err := os.ReadFile("/proc/uptime"); err == nil {
		f := strings.Fields(string(b))
		if len(f) > 0 {
			if up, err := strconv.ParseFloat(f[0], 64); err == nil {
				s.BootTime = time.Now().Add(-time.Duration(up * float64(time.Second))).Unix()
			}
		}
	}
	s.Memory = meminfo()
}

func readTrim(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// osRelease 返回发行版名称。容器内读取到的是镜像的发行版。
func osRelease() string {
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
			return strings.Trim(v, `"'`)
		}
	}
	return ""
}

// cpuModel 读取 CPU 型号。ARM 设备的 /proc/cpuinfo 通常没有 model name，退而使用 Hardware 或 Model 字段。
func cpuModel() string {
	b, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	var fallback string
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if v == "" {
			continue
		}
		switch k {
		case "model name", "cpu model":
			return v
		case "Hardware", "Model", "uarch":
			if fallback == "" {
				fallback = v
			}
		}
	}
	return fallback
}

func meminfo() *Memory {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return nil
	}
	defer f.Close()
	kv := map[string]uint64{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		fs := strings.Fields(v)
		if len(fs) == 0 {
			continue
		}
		n, _ := strconv.ParseUint(fs[0], 10, 64)
		// /proc/meminfo 的单位是 kB
		kv[k] = n * 1024
	}
	total := kv["MemTotal"]
	if total == 0 {
		return nil
	}
	avail, ok := kv["MemAvailable"]
	if !ok {
		// 老内核没有 MemAvailable，用空闲 + 缓存近似
		avail = kv["MemFree"] + kv["Buffers"] + kv["Cached"]
	}
	if avail > total {
		avail = total
	}
	m := &Memory{Total: total, Available: avail, Used: total - avail, SwapTotal: kv["SwapTotal"]}
	if free := kv["SwapFree"]; free <= m.SwapTotal {
		m.SwapUsed = m.SwapTotal - free
	}
	return m
}

// fillProcessOS 读取进程常驻内存、线程数与打开的文件数。
func fillProcessOS(p *Process) {
	if f, err := os.Open("/proc/self/status"); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			k, v, ok := strings.Cut(sc.Text(), ":")
			if !ok {
				continue
			}
			fs := strings.Fields(v)
			if len(fs) == 0 {
				continue
			}
			switch k {
			case "VmRSS":
				n, _ := strconv.ParseUint(fs[0], 10, 64)
				p.RSS = n * 1024
			case "Threads":
				p.Threads, _ = strconv.Atoi(fs[0])
			}
		}
		f.Close()
	}
	if ents, err := os.ReadDir("/proc/self/fd"); err == nil {
		p.FDs = len(ents)
	}
}

// readChildren 读取托管进程的 CPU 时间、常驻内存与线程数。只接受父进程是 Gately 自身的 PID：
// 托管进程都由 Gately 直接启动，进程退出后 PID 若被系统中其他进程复用，不会把别人的占用算进来。
func readChildren(pids []int) map[int]childRaw {
	out := make(map[int]childRaw, len(pids))
	self := os.Getpid()
	page := uint64(os.Getpagesize())
	for _, pid := range pids {
		if pid <= 0 {
			continue
		}
		b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
		if err != nil {
			continue
		}
		r, ppid, ok := parseStat(string(b), page)
		if !ok || ppid != self {
			continue
		}
		out[pid] = r
	}
	return out
}
