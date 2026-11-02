package webservice

import (
	"net/http"
	"net/netip"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/geoip"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/module"
)

const (
	// metricMinutes 是保留的分钟桶数量（24 小时）
	metricMinutes = 1440
	// topKeysCap 限制排行榜统计的不同键数量，防止被大量来源撑大内存
	topKeysCap = 5000
)

type minuteBucket struct {
	minute                 int64
	req, bytes             int64
	s2xx, s3xx, s4xx, s5xx int64
}

// siteMetrics 记录站点最近 24 小时的分钟级统计与来源排行。
type siteMetrics struct {
	mu        sync.Mutex
	ring      [metricMinutes]minuteBucket
	ips       map[string]int64
	countries map[string]int64
	bytesOut  int64
	status    [6]int64
	// lat 为首字节延迟统计，首次登记延迟时才分配
	lat *latState
	// rate 为秒级速率（自带锁，不受 mu 保护），recent 为近期活动的访客地址
	rate   rateRing
	recent *recentIPs
}

func newSiteMetrics() *siteMetrics {
	return &siteMetrics{ips: map[string]int64{}, countries: map[string]int64{}, recent: newRecentIPs()}
}

// record 登记一次请求；lat 为首字节延迟，小于 0 表示不计入延迟统计。请求计数与延迟在同一次加锁内登记。
func (m *siteMetrics) record(ip netip.Addr, status int, bytes int64, lat time.Duration, now time.Time) {
	min := now.Unix() / 60
	idx := int(min % metricMinutes)
	class := status / 100
	var country string
	if geoip.Default.Info().Loaded && !geoip.IsLocal(ip) {
		country = geoip.Default.Country(ip)
	}
	key := limitKeyAddr(ip)
	m.mu.Lock()
	defer m.mu.Unlock()
	b := &m.ring[idx]
	if b.minute != min {
		*b = minuteBucket{minute: min}
	}
	b.req++
	b.bytes += bytes
	m.bytesOut += bytes
	switch class {
	case 2:
		b.s2xx++
	case 3:
		b.s3xx++
	case 4:
		b.s4xx++
	case 5:
		b.s5xx++
	}
	if class >= 1 && class <= 5 {
		m.status[class]++
	}
	if _, ok := m.ips[key]; ok || len(m.ips) < topKeysCap {
		m.ips[key]++
	}
	if country != "" {
		m.countries[country]++
	}
	if lat >= 0 {
		m.observeLatencyLocked(lat, now)
	}
}

// Point 是一个时间点的统计。
type Point struct {
	Time     int64 `json:"t"`
	Requests int64 `json:"requests"`
	Bytes    int64 `json:"bytes"`
	S4xx     int64 `json:"s4xx"`
	S5xx     int64 `json:"s5xx"`
	// P50、P95 为本分钟首字节延迟的估算分位数（毫秒），分钟内没有可统计的响应时为 0
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
}

// TopItem 是排行榜的一项。
type TopItem struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

// SiteStatsView 是站点统计接口的输出。
type SiteStatsView struct {
	Series       []Point          `json:"series"`
	Status       map[string]int64 `json:"status"`
	BytesOut     int64            `json:"bytesOut"`
	TopIPs       []TopItem        `json:"topIps"`
	TopCountries []TopItem        `json:"topCountries"`
	Upstreams    []UpstreamStatus `json:"upstreams,omitempty"`
	// Blocked 为按原因分的累计拦截次数：ban（已封禁）、ip（IP 名单）、country（国家过滤）、rateLimit（限流）、waf
	Blocked map[string]int64 `json:"blocked"`
}

func top(m map[string]int64, n int) []TopItem {
	out := make([]TopItem, 0, len(m))
	for k, v := range m {
		out = append(out, TopItem{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// view 返回最近 minutes 分钟的序列（每分钟一点，缺失的分钟为 0）与累计排行。
func (m *siteMetrics) view(minutes int, now time.Time) SiteStatsView {
	if minutes < 1 || minutes > metricMinutes {
		minutes = 60
	}
	cur := now.Unix() / 60
	m.mu.Lock()
	defer m.mu.Unlock()
	series := make([]Point, 0, minutes)
	for i := minutes - 1; i >= 0; i-- {
		min := cur - int64(i)
		b := m.ring[int(min%metricMinutes)]
		p := Point{Time: min * 60}
		if b.minute == min {
			p.Requests, p.Bytes, p.S4xx, p.S5xx = b.req, b.bytes, b.s4xx, b.s5xx
		}
		p.P50, p.P95 = m.minuteQuantiles(min)
		series = append(series, p)
	}
	return SiteStatsView{
		Series:       series,
		Status:       map[string]int64{"2xx": m.status[2], "3xx": m.status[3], "4xx": m.status[4], "5xx": m.status[5]},
		BytesOut:     m.bytesOut,
		TopIPs:       top(m.ips, 10),
		TopCountries: top(m.countries, 10),
	}
}

// totals 返回累计请求数、字节与各状态类别，供 Prometheus 导出。
func (m *siteMetrics) totals() (bytes int64, status [6]int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bytesOut, m.status
}

// findSite 在运行中的监听器里查找站点处理器。
func (m *Module) findSiteLocked(listener, site string) *siteHandler {
	r, ok := m.runners[listener]
	if !ok {
		return nil
	}
	t := r.current()
	if site == "default" {
		return t.deflt
	}
	for _, s := range t.sites {
		if s.site.Key == site {
			return s
		}
	}
	return nil
}

func (m *Module) handleStats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	minutes, _ := strconv.Atoi(q.Get("minutes"))
	m.mu.Lock()
	h := m.findSiteLocked(q.Get("listener"), q.Get("site"))
	m.mu.Unlock()
	if h == nil {
		httpx.Error(w, http.StatusNotFound, "站点不存在或所属监听未运行")
		return
	}
	v := h.metrics.view(minutes, time.Now())
	v.Upstreams = h.upstreamStatus()
	v.Blocked = h.stats.blocked()
	httpx.OK(w, v)
}

// Metrics 实现 module.Metricer：导出各站点的请求、流量与后端健康指标。
func (m *Module) Metrics() []module.Sample {
	type row struct {
		labels map[string]string
		h      *siteHandler
	}
	var rows []row
	// acts 为各监听的监听级处理计数
	type actRow struct {
		label string
		run   *listenerRunner
	}
	var acts []actRow
	m.mu.Lock()
	for lk, run := range m.runners {
		acts = append(acts, actRow{run.cfg.Name + "#" + lk, run})
		for _, s := range run.current().all() {
			rows = append(rows, row{map[string]string{"listener": run.cfg.Name + "#" + lk, "site": siteLabel(s.site)}, s})
		}
	}
	m.mu.Unlock()
	sort.Slice(acts, func(i, j int) bool { return acts[i].label < acts[j].label })
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].labels["listener"] != rows[j].labels["listener"] {
			return rows[i].labels["listener"] < rows[j].labels["listener"]
		}
		return rows[i].labels["site"] < rows[j].labels["site"]
	})
	with := func(base map[string]string, k, v string) map[string]string {
		out := make(map[string]string, len(base)+1)
		for bk, bv := range base {
			out[bk] = bv
		}
		out[k] = v
		return out
	}
	var out []module.Sample
	add := func(name, help, typ string, labels map[string]string, v float64) {
		out = append(out, module.Sample{Name: name, Help: help, Type: typ, Labels: labels, Value: v})
	}
	for _, r := range rows {
		add("gately_site_requests_total", "Total requests handled by the site", module.MetricCounter, r.labels, float64(r.h.stats.Requests.Load()))
	}
	for _, r := range rows {
		add("gately_site_errors_total", "Backend errors of the site", module.MetricCounter, r.labels, float64(r.h.stats.Errors.Load()))
	}
	for _, r := range rows {
		add("gately_site_active_requests", "Requests currently in flight for the site", module.MetricGauge, r.labels, float64(r.h.stats.Active.Load()))
	}
	for _, r := range rows {
		bytes, _ := r.h.metrics.totals()
		add("gately_site_response_bytes_total", "Total response bytes of the site", module.MetricCounter, r.labels, float64(bytes))
	}
	for _, r := range rows {
		_, st := r.h.metrics.totals()
		for c := 2; c <= 5; c++ {
			add("gately_site_responses_total", "Responses of the site by status class", module.MetricCounter, with(r.labels, "class", strconv.Itoa(c)+"xx"), float64(st[c]))
		}
	}
	for _, r := range rows {
		bl := r.h.stats.blocked()
		for _, name := range blockNames {
			add("gately_site_blocked_total", "Requests blocked by the site by reason", module.MetricCounter, with(r.labels, "reason", name), float64(bl[name]))
		}
	}
	// 首字节延迟直方图（秒）：le 为累计桶，取常见的 13 个边界（均为内部细分桶的边界，计数精确），不含协议升级与被断开的请求
	for _, r := range rows {
		n, sums := r.h.metrics.latencyTotals()
		var cum, total uint64
		var sumMs float64
		for i := range n {
			total += n[i]
			sumMs += sums[i]
		}
		fam := "gately_site_request_duration_seconds"
		help := "Time to first response byte of the site in seconds (excludes upgraded and aborted requests)"
		next := 0
		for _, le := range latExportBounds {
			for next < len(latBounds) && latBounds[next] <= le {
				cum += n[next]
				next++
			}
			out = append(out, module.Sample{Name: fam + "_bucket", Family: fam, Help: help, Type: module.MetricHistogram,
				Labels: with(r.labels, "le", strconv.FormatFloat(le/1000, 'g', -1, 64)), Value: float64(cum)})
		}
		out = append(out, module.Sample{Name: fam + "_bucket", Family: fam, Help: help, Type: module.MetricHistogram, Labels: with(r.labels, "le", "+Inf"), Value: float64(total)})
		out = append(out, module.Sample{Name: fam + "_sum", Family: fam, Labels: r.labels, Value: sumMs / 1000})
		out = append(out, module.Sample{Name: fam + "_count", Family: fam, Labels: r.labels, Value: float64(total)})
	}
	// 后端标签：站点后端只有 upstream；分路径与按用户分流的后端另加 route（路径前缀，或 @用户列表），
	// 同一地址出现在多个分流中时序列不会重复
	upLabels := func(base map[string]string, u UpstreamStatus) map[string]string {
		l := with(base, "upstream", u.URL)
		switch u.Scope {
		case scopeLocation:
			l["route"] = u.Match
		case scopeUser:
			l["route"] = "@" + u.Match
		}
		return l
	}
	for _, r := range rows {
		for _, u := range r.h.upstreamStatus() {
			v := 0.0
			if u.Healthy {
				v = 1
			}
			add("gately_upstream_up", "Whether the upstream is healthy (1) or down (0)", module.MetricGauge, upLabels(r.labels, u), v)
		}
	}
	for _, r := range rows {
		for _, u := range r.h.upstreamStatus() {
			add("gately_upstream_requests_total", "Requests forwarded to the upstream", module.MetricCounter, upLabels(r.labels, u), float64(u.Requests))
		}
	}
	for _, r := range rows {
		for _, u := range r.h.upstreamStatus() {
			add("gately_upstream_response_bytes_total", "Response body bytes received from the upstream", module.MetricCounter, upLabels(r.labels, u), float64(u.Bytes))
		}
	}
	for _, l := range acts {
		add("gately_listener_requests_total", "Requests handled by the listener itself without a site", module.MetricCounter,
			map[string]string{"listener": l.label, "action": "https_redirect"}, float64(l.run.acts.redirect.total.Load()))
		add("gately_listener_requests_total", "Requests handled by the listener itself without a site", module.MetricCounter,
			map[string]string{"listener": l.label, "action": "not_found"}, float64(l.run.acts.notFound.total.Load()))
	}
	return out
}

// handleMetrics 以 Prometheus 文本格式导出各站点指标。
func (m *Module) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_ = module.WriteSamples(w, m.Metrics())
}
