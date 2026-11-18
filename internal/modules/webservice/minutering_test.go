package webservice

import (
	"net/netip"
	"testing"
	"time"
)

// testHourMin 为测试用的 Unix 分钟数，正好是某个整点小时的第 0 分钟（60 的倍数）
const testHourMin = 483_334 * 60

// 分钟环按小时分块：只分配用到的小时，读写同一分钟，没有写入的分钟读出 nil。
func TestMinuteRingChunks(t *testing.T) {
	var r minuteRing[minuteBucket]
	base := int64(testHourMin) // 某个整点小时的第 0 分钟
	r.slot(base + 5).req = 3
	r.slot(base+5).req++
	if got := r.at(base + 5); got == nil || got.req != 4 {
		t.Fatalf("同一分钟应累加: %+v", got)
	}
	if r.at(base+6) != nil && r.at(base+6).req != 0 {
		t.Fatal("未写入的分钟应为零值")
	}
	if r.at(base+65) != nil {
		t.Fatal("没有分配的小时应读出 nil")
	}
	if n := r.allocated(); n != 1 {
		t.Fatalf("只写了一个小时，应只分配 1 块: %d", n)
	}
	r.slot(base + 125).req = 1
	if n := r.allocated(); n != 2 {
		t.Fatalf("写了两个小时，应分配 2 块: %d", n)
	}
}

// 24 小时窗口跨 25 个整点小时：当前小时写入后，窗口最早那一小时的数据仍在。
func TestMinuteRingFullWindow(t *testing.T) {
	var r minuteRing[minuteBucket]
	now := int64(testHourMin) + 30 // 某小时的第 30 分钟
	oldest := now - (metricMinutes - 1)
	r.slot(oldest).req = 7
	for m := oldest + 60; m <= now; m += 60 {
		r.slot(m).req = 1
	}
	r.slot(now).req = 2
	if got := r.at(oldest); got == nil || got.req != 7 {
		t.Fatalf("窗口内最早一分钟的数据不应被当前小时覆盖: %+v", got)
	}
	if got := r.at(now); got == nil || got.req != 2 {
		t.Fatalf("当前分钟: %+v", got)
	}
}

// 超过保留范围的块在写入新小时或读取时释放；块属于更早的小时时清零复用。
func TestMinuteRingExpire(t *testing.T) {
	var r minuteRing[minuteBucket]
	base := int64(testHourMin)
	r.slot(base).req = 1
	r.slot(base + 60).req = 1
	// 26 小时后写入：两块都已过期，只剩新写入的一块
	r.slot(base + 26*60).req = 5
	if n := r.allocated(); n != 1 {
		t.Fatalf("过期的块应释放，只剩 1 块: %d", n)
	}
	if r.at(base) != nil {
		t.Fatal("过期数据不应再读到")
	}
	// 只读的站点在查看统计时释放过期块
	r.prune(base + 60*60)
	if n := r.allocated(); n != 0 {
		t.Fatalf("查看统计时应释放过期块: %d", n)
	}
	// 同一下标的块属于 25 小时前时清零复用，不残留旧数据
	var q minuteRing[minuteBucket]
	q.slot(base + 10).req = 9
	q.lastHour = 0 // 模拟中间没有写入、也没有清理
	q.slot(base+ringHours*60+10).req++
	if got := q.at(base + ringHours*60 + 10); got == nil || got.req != 1 {
		t.Fatalf("复用的块应清零: %+v", got)
	}
}

// 时钟回拨超过一小时以新时间为准；一小时内写到上一小时（延迟统计补写上一分钟）不丢弃当前小时的数据。
func TestMinuteRingClockBack(t *testing.T) {
	var r minuteRing[minuteBucket]
	base := int64(testHourMin)
	r.slot(base + 120).req = 1
	r.slot(base + 119).req = 1
	if r.at(base+120) == nil || r.at(base+119) == nil {
		t.Fatal("写回上一小时不应丢弃当前小时")
	}
	r.slot(base - 600).req = 1
	if r.at(base+120) != nil {
		t.Fatal("回拨超过一小时后，“未来”的块应丢弃")
	}
	if got := r.at(base - 600); got == nil || got.req != 1 {
		t.Fatalf("回拨后的写入: %+v", got)
	}
}

// 站点统计跨小时的曲线与延迟分位数与原先的分钟环一致；只有一小时流量的站点只占一个小时块。
func TestSiteMetricsAcrossHours(t *testing.T) {
	m := newSiteMetrics()
	ip := netip.MustParseAddr("192.0.2.1")
	start := time.Unix((testHourMin+58)*60, 0) // 某小时的第 58 分钟
	for i := 0; i < 4; i++ {
		now := start.Add(time.Duration(i) * time.Minute)
		m.record(ip, 200, 100, 10*time.Millisecond, now)
		m.record(ip, 503, 50, 30*time.Millisecond, now)
	}
	v := m.view(5, start.Add(4*time.Minute))
	if len(v.Series) != 5 {
		t.Fatalf("应有 5 个点: %d", len(v.Series))
	}
	for i, p := range v.Series[:4] {
		if p.Requests != 2 || p.Bytes != 150 || p.S5xx != 1 || p.S4xx != 0 {
			t.Fatalf("第 %d 分钟计数错误: %+v", i, p)
		}
		// 最后一分钟的分位数仍在当前直方图中，其余已写入分钟环
		if p.P50 <= 0 || p.P95 < p.P50 {
			t.Fatalf("第 %d 分钟分位数错误: %+v", i, p)
		}
	}
	if p := v.Series[4]; p.Requests != 0 {
		t.Fatalf("没有流量的分钟应为 0: %+v", p)
	}
	if n := m.ring.allocated(); n != 2 {
		t.Fatalf("跨两个小时应分配 2 块: %d", n)
	}
	// 全天都有流量时块数不超过 ringHours
	for i := 0; i < 30*60; i += 30 {
		m.record(ip, 200, 1, -1, start.Add(time.Duration(i)*time.Minute))
	}
	if n := m.ring.allocated(); n > ringHours {
		t.Fatalf("块数不应超过 %d: %d", ringHours, n)
	}
}
