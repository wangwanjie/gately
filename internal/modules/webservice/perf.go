package webservice

import "sort"

// 供性能监控模块（perf）采样的只读视图：每个运行中的站点一条记录，只含累计计数与瞬时值，
// 不带访客信息。性能监控按固定间隔取两次快照的差值来计算窗口内的请求速率、5xx 比例与延迟分位数。

// PerfUpstream 是一个后端节点的瞬时状态。
type PerfUpstream struct {
	// URL 已去掉用户名、密码与查询串，可安全出现在通知里
	URL     string
	Scope   string
	Match   string
	Healthy bool
	Conns   int64
}

// PerfSite 是一个运行中站点的性能快照。
type PerfSite struct {
	// Key 为 “监听 key/站点 key” 的稳定标识（默认站点的站点 key 为 default），Name 为面向人的名称
	Key      string
	Listener string
	Site     string
	Name     string
	// Active 为进行中的请求数
	Active int64
	// Requests 为到达的请求累计数（含进行中），用于计算每秒请求数
	Requests uint64
	// Done 为已完成响应的累计数（2xx~5xx），S5xx 为其中 5xx 的累计数
	Done, S5xx uint64
	// Lat、LatSum 为累计的首字节延迟分桶次数与耗时总和（毫秒），LatCount 为次数总和
	Lat       [LatencyBucketCount]uint64
	LatSum    [LatencyBucketCount]float64
	LatCount  uint64
	Upstreams []PerfUpstream
}

// PerfSites 返回全部运行中站点（含默认站点）的性能快照，按 Key 排序。
func (m *Module) PerfSites() []PerfSite {
	type item struct {
		key, listener, site, name string
		h                         *siteHandler
	}
	var items []item
	m.mu.Lock()
	for lk, run := range m.runners {
		for _, s := range run.current().all() {
			sk := s.site.Key
			if s.deflt {
				sk = "default"
			}
			items = append(items, item{key: lk + "/" + sk, listener: run.cfg.Name, site: sk, name: run.cfg.Name + " / " + siteLabel(s.site), h: s})
		}
	}
	m.mu.Unlock()
	sort.Slice(items, func(i, j int) bool { return items[i].key < items[j].key })
	out := make([]PerfSite, 0, len(items))
	for _, it := range items {
		_, st := it.h.metrics.totals()
		lat, latSum := it.h.metrics.latencyTotals()
		ps := PerfSite{Key: it.key, Listener: it.listener, Site: it.site, Name: it.name, Active: it.h.stats.Active.Load(), Requests: uint64(it.h.stats.Requests.Load())}
		for c := 2; c <= 5; c++ {
			ps.Done += uint64(st[c])
		}
		ps.S5xx = uint64(st[5])
		ps.Lat, ps.LatSum = lat, latSum
		for _, c := range lat {
			ps.LatCount += c
		}
		for _, u := range it.h.upstreamStatus() {
			ps.Upstreams = append(ps.Upstreams, PerfUpstream{URL: u.URL, Scope: u.Scope, Match: u.Match, Healthy: u.Healthy, Conns: u.Conns})
		}
		out = append(out, ps)
	}
	return out
}
