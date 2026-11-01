package webservice

import (
	"net/netip"
	"sync"
	"sync/atomic"
)

// 拓扑视图与实时速率使用的秒级计数。
//
// 分钟桶（siteMetrics.ring）适合看 24 小时趋势，但突发流量要一分钟后才体现；这里另外为站点、后端与
// 监听级动作各保留最近 rateSlots 秒的请求数与字节数，接口按“最近 rateAvgWindow 秒均值”给出 QPS 与吞吐，
// 并提供近 60 秒的曲线。每次登记只是加锁后对数组下标累加，不分配内存。

const (
	// rateSlots 为秒级环的槽数：保留 60 个完整秒，另加正在累计的当前秒与余量
	rateSlots = 64
	// rateSeriesSeconds 为曲线返回的完整秒数
	rateSeriesSeconds = 60
	// rateAvgWindow 为 QPS 与吞吐取均值的完整秒数
	rateAvgWindow = 10
)

type rateBucket struct {
	sec, req, bytes int64
}

// rateRing 记录最近 rateSlots 秒每秒的请求数与字节数。零值可用。
type rateRing struct {
	mu sync.Mutex
	b  [rateSlots]rateBucket
}

// add 在第 sec 秒（Unix 秒）登记 req 个请求、bytes 个字节。
func (r *rateRing) add(sec, req, bytes int64) {
	if sec <= 0 {
		return
	}
	r.mu.Lock()
	b := &r.b[sec%rateSlots]
	if b.sec != sec {
		*b = rateBucket{sec: sec}
	}
	b.req += req
	b.bytes += bytes
	r.mu.Unlock()
}

// RatePoint 是秒级曲线的一个点。
type RatePoint struct {
	Time     int64 `json:"t"`
	Requests int64 `json:"requests"`
	Bytes    int64 `json:"bytes"`
}

// series 返回 now 之前 n 个完整秒（不含正在累计的当前秒）的曲线，按时间正序，缺失的秒为 0。
func (r *rateRing) series(now int64, n int) []RatePoint {
	if n > rateSlots-1 {
		n = rateSlots - 1
	}
	out := make([]RatePoint, 0, n)
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := n; i >= 1; i-- {
		sec := now - int64(i)
		p := RatePoint{Time: sec}
		if b := r.b[sec%rateSlots]; b.sec == sec {
			p.Requests, p.Bytes = b.req, b.bytes
		}
		out = append(out, p)
	}
	return out
}

// sum 返回 now 之前 n 个完整秒的请求数与字节数之和。
func (r *rateRing) sum(now int64, n int) (req, bytes int64) {
	if n > rateSlots-1 {
		n = rateSlots - 1
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := 1; i <= n; i++ {
		sec := now - int64(i)
		if b := r.b[sec%rateSlots]; b.sec == sec {
			req += b.req
			bytes += b.bytes
		}
	}
	return req, bytes
}

// Rate 是一个节点的速率。QPS、BPS 为最近 rateAvgWindow 个完整秒的均值，QPS60 为最近 60 秒均值。
type Rate struct {
	QPS   float64 `json:"qps"`
	BPS   float64 `json:"bps"`
	QPS60 float64 `json:"qps60"`
}

// rate 返回速率快照。
func (r *rateRing) rate(now int64) Rate {
	req, bytes := r.sum(now, rateAvgWindow)
	req60, _ := r.sum(now, rateSeriesSeconds)
	return Rate{
		QPS:   float64(req) / rateAvgWindow,
		BPS:   float64(bytes) / rateAvgWindow,
		QPS60: float64(req60) / rateSeriesSeconds,
	}
}

// addRate 把 b 累加到 a（汇总监听、整个实例的速率）。
func (a *Rate) addRate(b Rate) {
	a.QPS += b.QPS
	a.BPS += b.BPS
	a.QPS60 += b.QPS60
}

// addSeries 把 b 按位置累加到 a（两者来自同一时刻、长度相同），a 为空时复制 b。
func addSeries(a, b []RatePoint) []RatePoint {
	if a == nil {
		return append([]RatePoint(nil), b...)
	}
	for i := range a {
		if i < len(b) {
			a[i].Requests += b[i].Requests
			a[i].Bytes += b[i].Bytes
		}
	}
	return a
}

const (
	// recentWindow 为“近期活动 IP”的时间窗口（秒）
	recentWindow = 60
	// recentCap 为每个站点（或监听级动作）最多记录的近期 IP 数，防止扫描或攻击时大量来源撑大内存。
	// 满了以后每秒最多清理一次过期项，仍然满时新来源不再记录，并标记列表不完整
	recentCap = 1024
)

// recentIPs 记录最近 recentWindow 秒内出现过的访客地址及最后出现的时间。
type recentIPs struct {
	mu sync.Mutex
	m  map[netip.Addr]int64
	// lastSweep 为最近一次清理过期项的秒，满载时每秒最多清理一次，避免每个新来源都遍历整张表
	lastSweep int64
	// overflow 为最近一次因满载而丢弃新来源的秒，窗口内发生过时列表不完整
	overflow atomic.Int64
}

func newRecentIPs() *recentIPs {
	return &recentIPs{m: map[netip.Addr]int64{}}
}

// touch 记录地址 a 在第 sec 秒出现。r 为 nil 时不记录。
func (r *recentIPs) touch(a netip.Addr, sec int64) {
	if r == nil || !a.IsValid() {
		return
	}
	a = a.Unmap()
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.m[a]; ok || len(r.m) < recentCap {
		r.m[a] = sec
		return
	}
	if r.lastSweep != sec {
		r.lastSweep = sec
		for k, v := range r.m {
			if v <= sec-recentWindow {
				delete(r.m, k)
			}
		}
		if len(r.m) < recentCap {
			r.m[a] = sec
			return
		}
	}
	r.overflow.Store(sec)
}

// each 对窗口内出现过的每个地址调用 fn，并顺带删除过期项。返回列表是否因满载而不完整。
func (r *recentIPs) each(now int64, fn func(a netip.Addr, last int64)) (truncated bool) {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, v := range r.m {
		if v <= now-recentWindow {
			delete(r.m, k)
			continue
		}
		fn(k, v)
	}
	return r.overflow.Load() > now-recentWindow
}
