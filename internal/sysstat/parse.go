package sysstat

import (
	"strconv"
	"strings"
)

// 托管进程计数器的解析函数与平台无关，放在这里便于在任意平台上测试。

// parseStat 解析 /proc/<pid>/stat。进程名可能包含空格和括号，从最后一个右括号之后开始取字段：
// ppid、utime、stime、num_threads、rss 分别是第 4、14、15、20、24 个字段，rss 的单位是页。
func parseStat(str string, page uint64) (r childRaw, ppid int, ok bool) {
	i := strings.LastIndexByte(str, ')')
	if i < 0 {
		return r, 0, false
	}
	f := strings.Fields(str[i+1:])
	if len(f) < 22 {
		return r, 0, false
	}
	ppid, err := strconv.Atoi(f[1])
	if err != nil {
		return r, 0, false
	}
	ut, err1 := strconv.ParseUint(f[11], 10, 64)
	st, err2 := strconv.ParseUint(f[12], 10, 64)
	threads, err3 := strconv.Atoi(f[17])
	rss, err4 := strconv.ParseInt(f[21], 10, 64)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return r, 0, false
	}
	r.ticks = ut + st
	r.threads = threads
	if rss > 0 {
		r.rss = uint64(rss) * page
	}
	return r, ppid, true
}

// parsePS 解析 ps -o pid=,ppid=,rss=,time= 的输出：rss 单位为 KB，time 为累计 CPU 时间（[dd-][hh:]mm:ss[.cc]）。
func parsePS(out string, self int) map[int]childRaw {
	res := map[int]childRaw{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		rss, err3 := strconv.ParseUint(f[2], 10, 64)
		sec, ok := parseCPUTime(f[3])
		if err1 != nil || err2 != nil || err3 != nil || !ok || ppid != self {
			continue
		}
		res[pid] = childRaw{rss: rss * 1024, ticks: uint64(sec * clockTicks)}
	}
	return res
}

// parseCPUTime 把 ps 的累计 CPU 时间换算成秒。
func parseCPUTime(v string) (float64, bool) {
	days := 0.0
	if d, rest, ok := strings.Cut(v, "-"); ok {
		n, err := strconv.Atoi(d)
		if err != nil {
			return 0, false
		}
		days, v = float64(n), rest
	}
	parts := strings.Split(v, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	total := 0.0
	for i, p := range parts {
		n, err := strconv.ParseFloat(p, 64)
		if err != nil || n < 0 {
			return 0, false
		}
		if i < len(parts)-1 {
			total = (total + n) * 60
		} else {
			total += n
		}
	}
	return days*86400 + total, true
}
