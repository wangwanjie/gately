package perf

import (
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/proc"
	"github.com/wangwanjie/gately/internal/sysstat"
)

// ProcSample 是托管进程的一次采样，取不到的值为 null。
type ProcSample struct {
	Time int64    `json:"t"`
	CPU  *float64 `json:"cpu"`
	// RSS 为常驻内存（MB）
	RSS *float64 `json:"rss"`
}

// ProcessView 是一个托管进程的当前资源占用与最近约 60 秒的采样。
type ProcessView struct {
	proc.Managed
	// CPU 为按整机核数归一化的百分比，RSS 为常驻内存（字节）
	CPU     *float64     `json:"cpu"`
	RSS     uint64       `json:"rss"`
	Threads int          `json:"threads"`
	Samples []ProcSample `json:"samples"`
}

// procTracker 按「模块 + 实例名 + 程序」跟踪托管进程，同一实例重启后（PID 变化）沿用之前的采样，
// 曲线不会因为重启断开；最近一次采样中已不存在的进程直接移除。只在内存中保存，每个进程最多 shortSlots 个点。并发安全。
type procTracker struct {
	mu    sync.Mutex
	items map[string]*procTrack
	order []string
	// gone 暂存刚从采样中消失的实例：进程退出到被重新拉起之间可能恰好错过一次采样，
	// 若立即丢弃，重启后的曲线会从头开始；在 goneGrace 内重新出现就沿用原来的采样
	gone map[string]goneTrack
}

type goneTrack struct {
	track *procTrack
	until time.Time
}

// goneGrace 是消失的实例保留采样的时间：进程退出后自动重启的等待时间最长 60 秒（1、2、4……秒递增），
// 与曲线保留的时长一致，连续崩溃重启时曲线也不会中断
const goneGrace = time.Duration(shortSlots) * SampleInterval

type procTrack struct {
	last    sysstat.ManagedProcess
	samples []ProcSample
}

func procKey(m proc.Managed) string {
	return m.Module + "\x00" + m.Name + "\x00" + m.Program
}

// add 登记一次采样。
func (t *procTracker) add(now time.Time, list []sysstat.ManagedProcess) {
	t.mu.Lock()
	defer t.mu.Unlock()
	next := make(map[string]*procTrack, len(list))
	order := make([]string, 0, len(list))
	for _, c := range list {
		k := procKey(c.Managed)
		if _, dup := next[k]; dup {
			// 同一实例理论上只有一个进程；万一重复，只保留第一个，避免曲线来回跳
			continue
		}
		tr := t.items[k]
		if tr == nil {
			if g, ok := t.gone[k]; ok && !now.After(g.until) {
				tr = g.track
			}
			delete(t.gone, k)
		}
		if tr == nil {
			tr = &procTrack{}
		}
		tr.last = c
		s := ProcSample{Time: now.Unix()}
		if c.CPU != nil {
			v := *c.CPU
			s.CPU = &v
		}
		if c.RSS > 0 {
			v := float64(c.RSS) / (1 << 20)
			s.RSS = &v
		}
		tr.samples = append(tr.samples, s)
		if len(tr.samples) > shortSlots {
			tr.samples = append([]ProcSample(nil), tr.samples[len(tr.samples)-shortSlots:]...)
		}
		next[k] = tr
		order = append(order, k)
	}
	// 本次没有出现的实例暂存起来，超时的清除
	for k, tr := range t.items {
		if _, still := next[k]; !still {
			if t.gone == nil {
				t.gone = make(map[string]goneTrack)
			}
			t.gone[k] = goneTrack{track: tr, until: now.Add(goneGrace)}
		}
	}
	for k, g := range t.gone {
		if now.After(g.until) {
			delete(t.gone, k)
		}
	}
	t.items, t.order = next, order
}

// views 返回各托管进程的视图，顺序与采样结果一致（按模块、名称、程序排序）。
func (t *procTracker) views() []ProcessView {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]ProcessView, 0, len(t.order))
	for _, k := range t.order {
		tr := t.items[k]
		v := ProcessView{Managed: tr.last.Managed, RSS: tr.last.RSS, Threads: tr.last.Threads, Samples: append([]ProcSample(nil), tr.samples...)}
		if tr.last.CPU != nil {
			c := *tr.last.CPU
			v.CPU = &c
		}
		out = append(out, v)
	}
	return out
}
