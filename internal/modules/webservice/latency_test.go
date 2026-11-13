package webservice

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLatIndexBoundaries(t *testing.T) {
	cases := map[float64]int{0: 0, 1: 0, 1.2: 1, 5: 6, 100: 20, 300: 26, 300.1: 27, 60000: len(latBounds) - 1, 60000.5: len(latBounds), 1e9: len(latBounds)}
	for ms, want := range cases {
		if got := latIndex(ms); got != want {
			t.Errorf("latIndex(%v) = %d，期望 %d", ms, got, want)
		}
	}
	if latBuckets != LatencyBucketCount || len(LatencyBounds()) != len(latBounds) {
		t.Fatal("导出的桶数量应与内部一致")
	}
	// 桶边界严格递增，Prometheus 导出的边界都必须是细分桶的边界，否则导出计数不精确
	for i := 1; i < len(latBounds); i++ {
		if latBounds[i] <= latBounds[i-1] {
			t.Fatalf("桶边界必须递增: %v", latBounds)
		}
	}
	for _, le := range latExportBounds {
		if i := latIndex(le); i >= len(latBounds) || latBounds[i] != le {
			t.Fatalf("导出边界 %v 不是细分桶的边界", le)
		}
	}
}

// hist 按给定的 (耗时毫秒, 次数) 构造分桶。
func hist(pairs ...[2]float64) ([]uint64, []float64) {
	n := make([]uint64, latBuckets)
	sum := make([]float64, latBuckets)
	for _, p := range pairs {
		i := latIndex(p[0])
		n[i] += uint64(p[1])
		sum[i] += p[0] * p[1]
	}
	return n, sum
}

func TestLatQuantile(t *testing.T) {
	n, sum := hist()
	if got := LatencyQuantile(n, sum, 0.95); got != 0 {
		t.Fatalf("没有样本应为 0: %v", got)
	}
	// 实测场景：5/7 的请求约 2 毫秒、2/7 约 302 毫秒（经代理略超过 300），P95 应接近 302，不能被估到 400 以上
	n, sum = hist([2]float64{2, 500}, [2]float64{302, 200})
	if got := LatencyQuantile(n, sum, 0.95); got < 301 || got > 303 {
		t.Fatalf("P95 应约为 302 毫秒: %v", got)
	}
	if got := LatencyQuantile(n, sum, 0.5); got != 2 {
		t.Fatalf("P50 应为 2 毫秒: %v", got)
	}
	// 90 个快请求 + 10 个慢请求：P95 在慢的一档，P50 在快的一档
	n, sum = hist([2]float64{3, 90}, [2]float64{800, 10})
	if p50, p95 := LatencyQuantile(n, sum, 0.5), LatencyQuantile(n, sum, 0.95); p50 != 3 || p95 != 800 {
		t.Fatalf("分位数不对: p50=%v p95=%v", p50, p95)
	}
	// 同一桶内耗时不同：取该桶平均值，落在桶的边界之内
	n, sum = hist([2]float64{310, 50}, [2]float64{345, 50})
	if got := LatencyQuantile(n, sum, 0.95); got <= 300 || got > 350 {
		t.Fatalf("应取桶内平均值: %v", got)
	}
	// 超过全部边界的 +Inf 桶同样取平均值
	n, sum = hist([2]float64{90000, 5})
	if got := LatencyQuantile(n, sum, 0.99); got != 90000 {
		t.Fatalf("+Inf 桶应取平均值: %v", got)
	}
	// q 越界按边界处理
	if LatencyQuantile(n, sum, -1) != 90000 || LatencyQuantile(n, sum, 7) != 90000 {
		t.Fatal("越界 q 应被夹到 0~1")
	}
}

func TestObserveLatencyMinutesAndReset(t *testing.T) {
	m := newSiteMetrics()
	if m.lat != nil {
		t.Fatal("没有请求的站点不应分配延迟统计")
	}
	if v := m.view(5, time.Now()); v.Series[4].P95 != 0 {
		t.Fatal("没有延迟统计时分位数应为 0")
	}
	now := time.Date(2026, 11, 2, 10, 5, 30, 0, time.UTC)
	for i := 0; i < 90; i++ {
		m.observeLatency(3*time.Millisecond, now)
	}
	for i := 0; i < 10; i++ {
		m.observeLatency(800*time.Millisecond, now)
	}
	m.record(parseAddr("1.1.1.1"), 200, 10, -1, now)
	v := m.view(2, now)
	last := v.Series[len(v.Series)-1]
	if last.P50 != 3 || last.P95 != 800 {
		t.Fatalf("当前分钟的分位数不对: %+v", last)
	}
	if v.Series[0].P95 != 0 {
		t.Fatalf("没有数据的分钟分位数应为 0: %+v", v.Series[0])
	}
	// 下一分钟有新请求时，上一分钟的分位数写入分钟环，两分钟各自独立
	next := now.Add(time.Minute)
	m.observeLatency(50*time.Millisecond, next)
	v = m.view(2, next)
	if v.Series[0].P95 != 800 || v.Series[1].P95 != 50 {
		t.Fatalf("分钟环应保留上一分钟: %+v", v.Series)
	}
	n, sum := m.latencyTotals()
	var total uint64
	var ms float64
	for i := range n {
		total += n[i]
		ms += sum[i]
	}
	if total != 101 || ms != 8320 {
		t.Fatalf("累计值不对: total=%d sum=%v", total, ms)
	}
	// 24 小时后同一槽位被新的一分钟复用：只含新数据
	later := now.Add(24 * time.Hour)
	m.observeLatency(2*time.Millisecond, later)
	v = m.view(1, later)
	if p := v.Series[0]; p.P95 != 2 {
		t.Fatalf("复用的分钟应只含新数据: %+v", p)
	}
}

func TestCountsForLatency(t *testing.T) {
	at := time.Now()
	cases := []struct {
		name string
		rec  statusRecorder
		want bool
	}{
		{"正常响应", statusRecorder{status: 200, firstByte: at}, true},
		{"未写出响应", statusRecorder{status: 200}, false},
		{"协议升级", statusRecorder{status: http.StatusSwitchingProtocols, firstByte: at}, false},
		{"断开连接", statusRecorder{status: statusClosed, firstByte: at}, false},
		{"5xx 也计入", statusRecorder{status: 502, firstByte: at}, true},
	}
	for _, c := range cases {
		if got := c.rec.countsForLatency(); got != c.want {
			t.Errorf("%s: %v，期望 %v", c.name, got, c.want)
		}
	}
	killed := &activeReq{}
	killed.killed.Store(true)
	if (&statusRecorder{status: 200, firstByte: at, req: killed}).countsForLatency() {
		t.Error("被管理员断开的请求不应计入")
	}
}

// 首字节延迟只算到响应头发出，不含响应体传输时间；站点热更新后仍沿用累计值。
func TestLatencyMeasuresFirstByteOnly(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow-header" {
			time.Sleep(150 * time.Millisecond)
		}
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		if r.URL.Path == "/slow-body" {
			time.Sleep(400 * time.Millisecond)
		}
		_, _ = w.Write([]byte("body"))
	}))
	defer backend.Close()
	m, addr := newConnsModule(t, []Site{{Enable: true, Key: "s", Domains: []string{"s.test"}, Targets: []string{backend.URL}}}, Site{})
	h := func() *siteHandler {
		m.mu.Lock()
		defer m.mu.Unlock()
		return m.findSiteLocked("l1", "s")
	}()
	if h == nil {
		t.Fatal("站点未运行")
	}
	start := time.Now()
	get(t, http.DefaultClient, addr, "s.test", "/slow-body", nil)
	if time.Since(start) < 350*time.Millisecond {
		t.Fatal("测试前提不成立：响应体应当很慢")
	}
	cnt, sums := h.metrics.latencyTotals()
	var n uint64
	var sum float64
	for i := range cnt {
		n += cnt[i]
		sum += sums[i]
	}
	if n != 1 || sum > 200 {
		t.Fatalf("响应体慢不应计入首字节延迟: n=%d sum=%vms", n, sum)
	}
	get(t, http.DefaultClient, addr, "s.test", "/slow-header", nil)
	cnt, sums = h.metrics.latencyTotals()
	n, sum = 0, 0
	for i := range cnt {
		n += cnt[i]
		sum += sums[i]
	}
	if n != 2 || sum < 150 {
		t.Fatalf("响应头慢应计入: n=%d sum=%vms", n, sum)
	}
	// 分钟序列里带上分位数
	v := h.metrics.view(1, time.Now())
	if v.Series[0].P95 < 100 {
		t.Fatalf("分钟序列应含 P95: %+v", v.Series[0])
	}
}

// 导出 Prometheus 直方图：累计桶单调、+Inf 等于 _count，同一指标族只有一组 HELP / TYPE。
func TestMetricsLatencyHistogram(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer backend.Close()
	m, addr := newConnsModule(t, []Site{{Enable: true, Key: "s", Domains: []string{"s.test"}, Targets: []string{backend.URL}}}, Site{})
	for i := 0; i < 5; i++ {
		get(t, http.DefaultClient, addr, "s.test", "/", nil)
	}
	// 延迟在响应发给客户端之后才记录，最后一个请求的计数可能稍晚写入，等计数到齐再检查
	var body string
	for deadline := time.Now().Add(3 * time.Second); ; {
		rec := httptest.NewRecorder()
		m.handleMetrics(rec, httptest.NewRequest("GET", "/metrics", nil))
		body = rec.Body.String()
		if histogramCount(body) == "5" || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if strings.Count(body, "# TYPE gately_site_request_duration_seconds histogram") != 1 {
		t.Fatalf("直方图应有且仅有一组 TYPE:\n%s", body)
	}
	if strings.Contains(body, "# TYPE gately_site_request_duration_seconds_bucket") {
		t.Fatal("_bucket 不应单独声明 TYPE")
	}
	var prev float64 = -1
	var inf, count float64
	buckets := 0
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "gately_site_request_duration_seconds") {
			continue
		}
		val := line[strings.LastIndex(line, " ")+1:]
		f := parseFloat(t, val)
		switch {
		case strings.Contains(line, "_bucket{"):
			buckets++
			if f < prev {
				t.Fatalf("累计桶必须单调不减: %s", line)
			}
			prev = f
			if strings.Contains(line, `le="+Inf"`) {
				inf = f
			}
		case strings.Contains(line, "_count{"):
			count = f
		}
	}
	if buckets != len(latExportBounds)+1 || inf != 5 || count != 5 {
		t.Fatalf("桶数=%d +Inf=%v count=%v\n%s", buckets, inf, count, body)
	}
	if !strings.Contains(body, `le="0.005"`) || !strings.Contains(body, `le="60"`) || strings.Contains(body, `le="0.0015"`) {
		t.Fatalf("le 应以秒为单位:\n%s", body)
	}
}

// 并发请求与读取分位数、导出指标同时进行（配合 -race）。
func TestLatencyConcurrent(t *testing.T) {
	m := newSiteMetrics()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 2000; i++ {
				m.observeLatency(time.Duration(i%50)*time.Millisecond, time.Now())
			}
		}()
	}
	for g := 0; g < 2; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				m.view(60, time.Now())
				m.latencyTotals()
			}
		}()
	}
	wg.Wait()
	cnt, _ := m.latencyTotals()
	var n uint64
	for _, c := range cnt {
		n += c
	}
	if n != 16000 {
		t.Fatalf("累计应为 16000: %d", n)
	}
}

// PerfSites 返回运行中站点的快照：累计响应、5xx、延迟、后端连接数与稳定标识。
func TestPerfSites(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/boom" {
			w.WriteHeader(500)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()
	m, addr := newConnsModule(t, []Site{{Enable: true, Key: "s", Name: "主站", Domains: []string{"s.test"}, Targets: []string{backend.URL}}}, Site{})
	for _, p := range []string{"/", "/", "/boom"} {
		get(t, http.DefaultClient, addr, "s.test", p, nil)
	}
	// 客户端收到响应时服务端可能还没登记完最后一个请求（响应写出后才计入完成数），等到计数稳定再检查
	var got []PerfSite
	deadline := time.Now().Add(2 * time.Second)
	for {
		got = m.PerfSites()
		if len(got) != 1 || got[0].Done == 3 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(got) != 1 {
		t.Fatalf("应有 1 个站点: %+v", got)
	}
	s := got[0]
	if s.Key != "l1/s" || s.Site != "s" || !strings.HasSuffix(s.Name, "主站") {
		t.Fatalf("标识或名称不对: %+v", s)
	}
	if s.Requests != 3 || s.Done != 3 || s.S5xx != 1 || s.LatCount != 3 {
		t.Fatalf("累计值不对: %+v", s)
	}
	if len(s.Upstreams) != 1 || !s.Upstreams[0].Healthy || strings.Contains(s.Upstreams[0].URL, "@") {
		t.Fatalf("后端信息不对: %+v", s.Upstreams)
	}
}

func parseAddr(s string) netip.Addr { return netip.MustParseAddr(s) }

func parseFloat(t *testing.T, s string) float64 {
	t.Helper()
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("无法解析数值 %q", s)
	}
	return f
}

// histogramCount 返回请求延迟直方图 _count 行的值，没有时为空串。
func histogramCount(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "gately_site_request_duration_seconds_count{") {
			return line[strings.LastIndex(line, " ")+1:]
		}
	}
	return ""
}
