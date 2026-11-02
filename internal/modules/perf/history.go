package perf

import (
	"sync"
	"time"
)

const (
	// SampleInterval 为后台采样与告警评估的间隔
	SampleInterval = 5 * time.Second
	// shortSlots 为秒级环的槽数：60 秒 / 采样间隔
	shortSlots = int(60 * time.Second / SampleInterval)
	// historyMinutes 为分钟级历史保留的分钟数（24 小时）
	historyMinutes = 1440
	// bucketMinutes 为 24 小时档合并的分钟数，把 1440 个点降到 288 个，减小传输量
	bucketMinutes = 5
)

// unknown 表示该指标这一刻无法取得（例如首次采样前的 CPU 使用率、不支持的平台）。
const unknown = -1.0

// hostSample 是一次主机与进程采样，取不到的字段为 unknown。
type hostSample struct {
	At time.Time
	// Name 为主机名，用作告警对象的名称
	Name                       string
	CPU, Mem, Disk, Load1      float64
	ProcCPU, ProcRSSMB, ProcFD float64
	Goroutines                 float64
}

// 采样字段数量与下标，聚合时按下标循环。
const (
	fCPU = iota
	fMem
	fDisk
	fLoad
	fProcCPU
	fProcRSS
	fProcFD
	fGoroutines
	fieldCount
)

func (s hostSample) fields() [fieldCount]float64 {
	return [fieldCount]float64{s.CPU, s.Mem, s.Disk, s.Load1, s.ProcCPU, s.ProcRSSMB, s.ProcFD, s.Goroutines}
}

// minuteAgg 累计一分钟内的采样，得到平均值与 CPU 峰值。
type minuteAgg struct {
	minute int64
	sum    [fieldCount]float64
	n      [fieldCount]int
	// cpuMax、procCPUMax 为分钟内的峰值，短暂尖峰不会被平均值抹平
	cpuMax, procCPUMax float64
}

func (a *minuteAgg) add(s hostSample) {
	for i, v := range s.fields() {
		if v == unknown {
			continue
		}
		a.sum[i] += v
		a.n[i]++
	}
	if s.CPU > a.cpuMax {
		a.cpuMax = s.CPU
	}
	if s.ProcCPU > a.procCPUMax {
		a.procCPUMax = s.ProcCPU
	}
}

// point 返回聚合后的点，没有任何有效采样的字段为 unknown。
func (a *minuteAgg) point() hostPoint {
	p := hostPoint{T: a.minute * 60}
	var out [fieldCount]float64
	for i := range out {
		out[i] = unknown
		if a.n[i] > 0 {
			out[i] = a.sum[i] / float64(a.n[i])
		}
	}
	p.CPU, p.Mem, p.Disk, p.Load1 = out[fCPU], out[fMem], out[fDisk], out[fLoad]
	p.ProcCPU, p.ProcRSSMB, p.ProcFD, p.Goroutines = out[fProcCPU], out[fProcRSS], out[fProcFD], out[fGoroutines]
	p.CPUMax, p.ProcCPUMax = unknown, unknown
	if a.n[fCPU] > 0 {
		p.CPUMax = a.cpuMax
	}
	if a.n[fProcCPU] > 0 {
		p.ProcCPUMax = a.procCPUMax
	}
	return p
}

// hostPoint 是历史中的一个点，unknown 表示该时刻没有数据。
type hostPoint struct {
	T                          int64
	CPU, Mem, Disk, Load1      float64
	CPUMax                     float64
	ProcCPU, ProcRSSMB, ProcFD float64
	ProcCPUMax                 float64
	Goroutines                 float64
}

// hostHistory 保存最近 60 秒（每个采样一点）与最近 24 小时（每分钟一点）的主机与进程历史。并发安全。
type hostHistory struct {
	mu     sync.Mutex
	short  [shortSlots]hostSample
	shortN int
	ring   [historyMinutes]hostPoint
	// have 标记 ring 中对应槽是否写过，minute 校验槽内数据属于哪一分钟
	have [historyMinutes]bool
	cur  minuteAgg
	curN bool
	last hostSample
}

// add 登记一次采样。
func (h *hostHistory) add(s hostSample) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.last = s
	h.short[h.shortN%shortSlots] = s
	h.shortN++
	min := s.At.Unix() / 60
	if h.curN && h.cur.minute != min {
		h.flushLocked()
	}
	if !h.curN {
		h.cur = minuteAgg{minute: min}
		h.curN = true
	}
	h.cur.add(s)
}

// flushLocked 把当前分钟的聚合写入 ring。
func (h *hostHistory) flushLocked() {
	idx := int(h.cur.minute % historyMinutes)
	h.ring[idx] = h.cur.point()
	h.have[idx] = true
	h.curN = false
}

// latest 返回最近一次采样。
func (h *hostHistory) latest() (hostSample, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.last, h.shortN > 0
}

// recent 返回最近 60 秒的采样，按时间正序。
func (h *hostHistory) recent() []hostSample {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := h.shortN
	if n > shortSlots {
		n = shortSlots
	}
	out := make([]hostSample, 0, n)
	for i := h.shortN - n; i < h.shortN; i++ {
		out = append(out, h.short[i%shortSlots])
	}
	return out
}

// minutes 返回截至 now 的最近 n 分钟（含正在累计的当前分钟）的点，按时间正序，没有数据的分钟不返回。
func (h *hostHistory) minutes(now time.Time, n int) []hostPoint {
	if n < 1 {
		return nil
	}
	if n > historyMinutes {
		n = historyMinutes
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	cur := now.Unix() / 60
	out := make([]hostPoint, 0, n)
	for i := n - 1; i >= 0; i-- {
		min := cur - int64(i)
		if h.curN && h.cur.minute == min {
			out = append(out, h.cur.point())
			continue
		}
		idx := int(min % historyMinutes)
		if h.have[idx] && h.ring[idx].T == min*60 {
			out = append(out, h.ring[idx])
		}
	}
	return out
}

// merge 把按时间正序的分钟点每 size 个合并为一个点：平均值取均值，峰值取最大值，时间取这一段的起点。
func merge(points []hostPoint, size int) []hostPoint {
	if size <= 1 || len(points) == 0 {
		return points
	}
	var out []hostPoint
	for i := 0; i < len(points); i += size {
		end := i + size
		if end > len(points) {
			end = len(points)
		}
		out = append(out, mergeGroup(points[i:end]))
	}
	return out
}

func mergeGroup(g []hostPoint) hostPoint {
	out := hostPoint{T: g[0].T}
	avg := func(get func(hostPoint) float64) float64 {
		var sum float64
		var n int
		for _, p := range g {
			if v := get(p); v != unknown {
				sum += v
				n++
			}
		}
		if n == 0 {
			return unknown
		}
		return sum / float64(n)
	}
	max := func(get func(hostPoint) float64) float64 {
		best := unknown
		for _, p := range g {
			if v := get(p); v != unknown && v > best {
				best = v
			}
		}
		return best
	}
	out.CPU = avg(func(p hostPoint) float64 { return p.CPU })
	out.Mem = avg(func(p hostPoint) float64 { return p.Mem })
	out.Disk = avg(func(p hostPoint) float64 { return p.Disk })
	out.Load1 = avg(func(p hostPoint) float64 { return p.Load1 })
	out.ProcCPU = avg(func(p hostPoint) float64 { return p.ProcCPU })
	out.ProcRSSMB = avg(func(p hostPoint) float64 { return p.ProcRSSMB })
	out.ProcFD = avg(func(p hostPoint) float64 { return p.ProcFD })
	out.Goroutines = avg(func(p hostPoint) float64 { return p.Goroutines })
	out.CPUMax = max(func(p hostPoint) float64 { return p.CPUMax })
	out.ProcCPUMax = max(func(p hostPoint) float64 { return p.ProcCPUMax })
	return out
}
