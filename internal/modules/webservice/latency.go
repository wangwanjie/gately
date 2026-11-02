package webservice

import (
	"sort"
	"time"
)

// 请求延迟统计。
//
// 延迟口径为“首字节延迟”：从请求到达到响应头发出的时间，代表后端或站点本身的响应速度。
// 不统计响应体传输耗时，大文件下载与 WebSocket 等长连接因此不会拉高分位数；
// 协议升级（101）、被管理员断开与被访问控制直接断开（没有写出响应）的请求不计入。
//
// 每个请求落进固定的细分桶（latBounds），桶里同时累计次数与耗时总和，不保存样本。分位数取所在桶的平均耗时：
// 同一桶内的请求耗时接近（相邻边界相差约 10%~25%），估算误差不超过桶宽，常见的“请求都差不多慢”场景几乎精确。
//
// 每个站点只保留两份细分直方图：累计值（Prometheus 导出与性能监控按窗口取差值）与当前分钟；
// 24 小时分钟环只存每分钟算好的 P50 / P95（每站点约 23 KB），站点收到第一个计入延迟的请求时才分配。

// latBounds 为各延迟桶的上界（毫秒），最后一个桶（+Inf）不在其中。
// 包含 Prometheus 导出用的全部边界（latExportBounds），导出的累计计数因此是精确值。
var latBounds = [...]float64{
	1, 1.5, 2, 2.5, 3, 4, 5, 6, 8, 10,
	12, 15, 20, 25, 30, 40, 50, 60, 75, 90,
	100, 120, 150, 175, 200, 250, 300, 350, 400, 450,
	500, 600, 750, 900, 1000, 1250, 1500, 1750, 2000, 2500,
	3000, 4000, 5000, 6000, 7500, 10000, 15000, 20000, 30000, 60000,
}

// latExportBounds 为 Prometheus 直方图导出的桶上界（毫秒），取常见的 5 毫秒到 60 秒。
var latExportBounds = []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000}

// latBuckets 为延迟桶数量：每个上界一个桶，另加一个 +Inf 桶。
const latBuckets = len(latBounds) + 1

// LatencyBucketCount 为延迟桶数量，供其他包分配与校验。
const LatencyBucketCount = latBuckets

// LatencyBounds 返回延迟桶的上界（毫秒），供测试使用。
func LatencyBounds() []float64 {
	return append([]float64(nil), latBounds[:]...)
}

// latIndex 返回延迟（毫秒）所在的桶下标：第一个上界不小于 ms 的桶，超过全部上界时为 +Inf 桶。
func latIndex(ms float64) int {
	return sort.SearchFloat64s(latBounds[:], ms)
}

// latHist 是一组细分桶的次数与耗时总和（毫秒）。
type latHist struct {
	n   [latBuckets]uint64
	sum [latBuckets]float64
}

func (h *latHist) add(idx int, ms float64) {
	h.n[idx]++
	h.sum[idx] += ms
}

// latQuantile 估算 q 分位（0~1）的延迟毫秒数：按次数找到分位所在的桶，取该桶的平均耗时。没有样本时返回 0。
func latQuantile(n []uint64, sum []float64, q float64) float64 {
	var total uint64
	for _, c := range n {
		total += c
	}
	if total == 0 {
		return 0
	}
	if q < 0 {
		q = 0
	}
	if q > 1 {
		q = 1
	}
	rank := q * float64(total)
	var seen float64
	last := 0
	for i, c := range n {
		if c == 0 {
			continue
		}
		last = i
		seen += float64(c)
		if seen >= rank {
			return sum[i] / float64(c)
		}
	}
	return sum[last] / float64(n[last])
}

// LatencyQuantile 是 latQuantile 的导出版本，n 与 sum 的长度必须等于 LatencyBucketCount。
func LatencyQuantile(n []uint64, sum []float64, q float64) float64 {
	return latQuantile(n, sum, q)
}

// latMinute 是 24 小时分钟环中的一分钟：该分钟的 P50、P95（毫秒），minute 为 Unix 分钟。
type latMinute struct {
	minute   int64
	p50, p95 float32
}

// latState 是站点的延迟统计，首次登记延迟时才分配。由 siteMetrics.mu 保护。
type latState struct {
	// cum 为累计值
	cum latHist
	// cur 为当前分钟（curMin）的直方图，进入新的一分钟时把分位数写入 ring 并清零
	cur    latHist
	curMin int64
	ring   [metricMinutes]latMinute
}

// flush 把当前分钟的分位数写入分钟环。
func (l *latState) flush() {
	var total uint64
	for _, c := range l.cur.n {
		total += c
	}
	if total == 0 {
		return
	}
	l.ring[int(l.curMin%metricMinutes)] = latMinute{
		minute: l.curMin,
		p50:    float32(latQuantile(l.cur.n[:], l.cur.sum[:], 0.5)),
		p95:    float32(latQuantile(l.cur.n[:], l.cur.sum[:], 0.95)),
	}
}

// observeLatency 在站点统计里登记一次首字节延迟（单独登记，主要供测试使用；处理请求时由 record 一并登记）。
func (m *siteMetrics) observeLatency(d time.Duration, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeLatencyLocked(d, now)
}

// observeLatencyLocked 登记一次首字节延迟，调用方持有 m.mu。
func (m *siteMetrics) observeLatencyLocked(d time.Duration, now time.Time) {
	ms := float64(d) / float64(time.Millisecond)
	if ms < 0 {
		ms = 0
	}
	idx := latIndex(ms)
	min := now.Unix() / 60
	l := m.lat
	if l == nil {
		l = &latState{curMin: min}
		m.lat = l
	}
	if l.curMin != min {
		l.flush()
		l.cur = latHist{}
		l.curMin = min
	}
	l.cur.add(idx, ms)
	l.cum.add(idx, ms)
}

// latencyTotals 返回累计的延迟分桶次数与耗时总和（毫秒），供 Prometheus 导出与性能采样。
func (m *siteMetrics) latencyTotals() (n [latBuckets]uint64, sum [latBuckets]float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lat == nil {
		return
	}
	return m.lat.cum.n, m.lat.cum.sum
}

// minuteQuantiles 返回第 min 分钟的 P50 与 P95（毫秒），该分钟没有延迟样本时均为 0。调用方持有 m.mu。
func (m *siteMetrics) minuteQuantiles(min int64) (p50, p95 float64) {
	l := m.lat
	if l == nil {
		return 0, 0
	}
	if l.curMin == min {
		return latQuantile(l.cur.n[:], l.cur.sum[:], 0.5), latQuantile(l.cur.n[:], l.cur.sum[:], 0.95)
	}
	b := l.ring[int(min%metricMinutes)]
	if b.minute != min {
		return 0, 0
	}
	return float64(b.p50), float64(b.p95)
}
