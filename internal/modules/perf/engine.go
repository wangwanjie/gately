package perf

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/modules/webservice"
)

const (
	// windowSpan 为窗口类指标（每秒请求数、5xx 比例、延迟）的统计窗口
	windowSpan = 60 * time.Second
	// windowMin 为窗口类指标可信所需的最短跨度：刚启动或站点刚出现时数据不足，先不判定
	windowMin = 30 * time.Second
	// windowKeep 为快照保留时长，略长于窗口以便每次取到跨度接近 windowSpan 的基准
	windowKeep = windowSpan + 2*SampleInterval
	// maxTrackedSites 限制同时跟踪窗口数据的站点数，防止配置异常膨胀
	maxTrackedSites = 5000
)

// siteSnap 是一个站点在某次采样时的累计计数。
type siteSnap struct {
	at         time.Time
	requests   uint64
	done, s5xx uint64
	lat        [webservice.LatencyBucketCount]uint64
	latSum     [webservice.LatencyBucketCount]float64
	latCount   uint64
}

// SiteView 是一个站点最近一次采样后的指标，窗口类指标数据不足时为 nil。
type SiteView struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Active int64  `json:"active"`
	// QPS、Err5xx、P95 按最近约 60 秒计算：每秒请求数、5xx 占已完成响应的百分比、首字节延迟 P95（毫秒）
	QPS    *float64 `json:"qps"`
	Err5xx *float64 `json:"err5xx"`
	P95    *float64 `json:"p95"`
	// Requests 为窗口内已完成的响应数，Latencies 为窗口内计入延迟统计的请求数
	Requests  uint64         `json:"requests"`
	Latencies uint64         `json:"latencies"`
	Upstreams []UpstreamView `json:"upstreams,omitempty"`
}

// UpstreamView 是后端节点的瞬时状态。
type UpstreamView struct {
	Key     string `json:"key"`
	URL     string `json:"url"`
	Route   string `json:"route,omitempty"`
	Healthy bool   `json:"healthy"`
	Conns   int64  `json:"conns"`
}

// AlertState 是一个正在进行的告警实例（条件满足中或已触发）。
type AlertState struct {
	Rule    string `json:"rule"`
	Name    string `json:"name"`
	Subject string `json:"subject"`
	// Target 为告警对象的名称（本机、站点名等）
	Target string `json:"target"`
	// State 为 pending（条件满足、计时中）或 firing（已触发）
	State string `json:"state"`
	// Since 为条件开始满足的时间（Unix 秒），FiredAt 为触发时间，pending 时为 0
	Since   int64   `json:"since"`
	FiredAt int64   `json:"firedAt"`
	Value   float64 `json:"value"`
	Level   string  `json:"level"`
}

type alertInst struct {
	rule     string
	sig      string
	subject  string
	target   string
	since    time.Time
	firedAt  time.Time
	firing   bool
	value    float64
	level    string
	ruleName string
}

// engine 保存采样历史并评估告警规则。tick 由单个后台协程调用，查询接口并发读取。
type engine struct {
	mu      sync.Mutex
	hist    *hostHistory
	windows map[string][]siteSnap
	views   []SiteView
	states  map[string]*alertInst
}

func newEngine() *engine {
	return &engine{hist: &hostHistory{}, windows: map[string][]siteSnap{}, states: map[string]*alertInst{}}
}

func snapOf(now time.Time, s webservice.PerfSite) siteSnap {
	return siteSnap{at: now, requests: s.Requests, done: s.Done, s5xx: s.S5xx, lat: s.Lat, latSum: s.LatSum, latCount: s.LatCount}
}

// windowViews 把本次采样并入各站点的窗口，返回各站点的指标。已消失的站点窗口随之清理。
func (e *engine) windowViews(now time.Time, sites []webservice.PerfSite) []SiteView {
	seen := make(map[string]bool, len(sites))
	views := make([]SiteView, 0, len(sites))
	for _, s := range sites {
		seen[s.Key] = true
		cur := snapOf(now, s)
		snaps := e.windows[s.Key]
		// 计数回退说明站点被重建或重启，旧窗口作废
		if n := len(snaps); n > 0 && (cur.requests < snaps[n-1].requests || cur.done < snaps[n-1].done || cur.latCount < snaps[n-1].latCount) {
			snaps = nil
		}
		// 窗口外的快照只保留到 windowKeep
		cut := 0
		for cut < len(snaps) && now.Sub(snaps[cut].at) > windowKeep {
			cut++
		}
		snaps = append(snaps[cut:], cur)
		if _, tracked := e.windows[s.Key]; !tracked && len(e.windows) >= maxTrackedSites {
			delete(seen, s.Key)
			views = append(views, instantView(s))
			continue
		}
		e.windows[s.Key] = snaps

		v := instantView(s)
		base := snaps[0]
		span := now.Sub(base.at)
		if span >= windowMin {
			sec := span.Seconds()
			qps := float64(cur.requests-base.requests) / sec
			v.QPS = &qps
			done := cur.done - base.done
			v.Requests = done
			if done > 0 {
				p := float64(cur.s5xx-base.s5xx) / float64(done) * 100
				v.Err5xx = &p
			}
			var counts [webservice.LatencyBucketCount]uint64
			var sums [webservice.LatencyBucketCount]float64
			for i := range counts {
				counts[i] = cur.lat[i] - base.lat[i]
				sums[i] = cur.latSum[i] - base.latSum[i]
			}
			v.Latencies = cur.latCount - base.latCount
			if v.Latencies > 0 {
				p := webservice.LatencyQuantile(counts[:], sums[:], 0.95)
				v.P95 = &p
			}
		}
		views = append(views, v)
	}
	for k := range e.windows {
		if !seen[k] {
			delete(e.windows, k)
		}
	}
	return views
}

func instantView(s webservice.PerfSite) SiteView {
	v := SiteView{Key: s.Key, Name: s.Name, Active: s.Active}
	for _, u := range s.Upstreams {
		route := ""
		switch u.Scope {
		case "location":
			route = u.Match
		case "user":
			route = "@" + u.Match
		}
		v.Upstreams = append(v.Upstreams, UpstreamView{Key: s.Key + "|" + u.Scope + "|" + u.Match + "|" + u.URL, URL: u.URL, Route: route, Healthy: u.Healthy, Conns: u.Conns})
	}
	return v
}

// subject 是一次评估中的一个告警对象及其当前值。
type subject struct {
	key, name string
	value     float64
	ok        bool
}

// subjectsFor 返回规则指标在本次采样下的全部对象。窗口类指标数据不足的对象 ok 为 false。
func subjectsFor(def MetricDef, r *Rule, host hostSample, views []SiteView) []subject {
	pick := func(key, name string, v float64) []subject {
		return []subject{{key: key, name: name, value: v, ok: v != unknown}}
	}
	// 主机与进程用主机名称呼：各语言通用（通知来源只做整句翻译），多台机器发来的告警也能区分
	hostName := host.Name
	if hostName == "" {
		hostName = "localhost"
	}
	switch def.ID {
	case MetricHostCPU:
		return pick("host", hostName, host.CPU)
	case MetricHostMem:
		return pick("host", hostName, host.Mem)
	case MetricHostDisk:
		return pick("host", hostName, host.Disk)
	case MetricProcCPU:
		return pick("process", "Gately@"+hostName, host.ProcCPU)
	case MetricProcRSS:
		return pick("process", "Gately@"+hostName, host.ProcRSSMB)
	}
	var out []subject
	for _, v := range views {
		if !r.matchesSite(v.Key) {
			continue
		}
		switch def.ID {
		case MetricSiteActive:
			out = append(out, subject{v.Key, v.Name, float64(v.Active), true})
		case MetricSiteQPS:
			s := subject{key: v.Key, name: v.Name}
			if v.QPS != nil {
				s.value, s.ok = *v.QPS, true
			}
			out = append(out, s)
		case MetricSite5xx:
			s := subject{key: v.Key, name: v.Name}
			if v.Err5xx != nil && v.Requests >= uint64(r.MinRequests) {
				s.value, s.ok = *v.Err5xx, true
			}
			out = append(out, s)
		case MetricSiteP95:
			s := subject{key: v.Key, name: v.Name}
			if v.P95 != nil && v.Latencies >= uint64(r.MinRequests) {
				s.value, s.ok = *v.P95, true
			}
			out = append(out, s)
		case MetricUpstreamC:
			for _, u := range v.Upstreams {
				name := v.Name + " → " + u.URL
				if u.Route != "" {
					name += "（" + u.Route + "）"
				}
				out = append(out, subject{u.Key, name, float64(u.Conns), true})
			}
		}
	}
	return out
}

// tick 登记一次采样并评估全部规则，返回需要发布的事件。
func (e *engine) tick(now time.Time, host hostSample, sites []webservice.PerfSite, rules []Rule) []events.Event {
	host.At = now
	e.hist.add(host)

	e.mu.Lock()
	defer e.mu.Unlock()
	e.views = e.windowViews(now, sites)

	// 规则被删除、停用或判定条件被修改后，旧的计时与触发状态作废
	live := map[string]*Rule{}
	for i := range rules {
		if rules[i].Enable {
			live[rules[i].Key] = &rules[i]
		}
	}
	for k, st := range e.states {
		if r := live[st.rule]; r == nil || r.signature() != st.sig {
			delete(e.states, k)
		}
	}

	var out []events.Event
	for _, r := range rules {
		if !r.Enable {
			continue
		}
		def, ok := metricByID(r.Metric)
		if !ok {
			continue
		}
		rule := r
		present := map[string]bool{}
		for _, s := range subjectsFor(def, &rule, host, e.views) {
			stKey := rule.Key + "\x00" + s.key
			present[stKey] = true
			st := e.states[stKey]
			if s.ok && rule.compare(s.value) {
				if st == nil {
					st = &alertInst{rule: rule.Key, sig: rule.signature(), subject: s.key, since: now}
					e.states[stKey] = st
				}
				st.target, st.value, st.level, st.ruleName = s.name, s.value, rule.Level, rule.Name
				if !st.firing && now.Sub(st.since) >= time.Duration(rule.Duration)*time.Minute {
					st.firing, st.firedAt = true, now
					out = append(out, alertEvent(now, &rule, def, st))
				}
				continue
			}
			if st == nil {
				continue
			}
			if st.firing {
				out = append(out, recoverEvent(now, &rule, def, st, s))
			}
			delete(e.states, stKey)
		}
		// 对象已消失（站点被删除或停用）：直接丢弃状态，不发恢复通知
		for k, st := range e.states {
			if st.rule == rule.Key && !present[k] {
				delete(e.states, k)
			}
		}
	}
	return out
}

func thresholdText(r *Rule, def MetricDef) string {
	return r.Op + " " + formatValue(def.Unit, r.Threshold)
}

func alertEvent(now time.Time, r *Rule, def MetricDef, st *alertInst) events.Event {
	return events.Event{
		Type:   events.PerfAlert,
		Source: r.Name + " · " + st.target,
		Level:  r.Level,
		Title:  "性能告警：" + r.Name,
		Message: fmt.Sprintf("%s：%s 当前 %s，触发条件 %s，已持续 %d 分钟",
			st.target, def.Label, formatValue(def.Unit, st.value), thresholdText(r, def), r.Duration),
		Fields: map[string]string{"rule": r.Key, "metric": r.Metric, "target": st.target, "value": formatValue(def.Unit, st.value), "condition": thresholdText(r, def)},
		Time:   now,
	}
}

// recoverEvent 生成恢复事件。s.ok 为 false 表示指标暂时无法判定（窗口内请求不足或取不到数值），此时不写当前值。
func recoverEvent(now time.Time, r *Rule, def MetricDef, st *alertInst, s subject) events.Event {
	cond, dur := thresholdText(r, def), spanText(now.Sub(st.firedAt))
	msg := fmt.Sprintf("%s：%s 已恢复（当前 %s，触发条件 %s），告警持续 %s", st.target, def.Label, formatValue(def.Unit, s.value), cond, dur)
	value := formatValue(def.Unit, s.value)
	if !s.ok {
		msg = fmt.Sprintf("%s：%s 已无法判定（近 60 秒请求不足或暂无数据，触发条件 %s），视为恢复，告警持续 %s", st.target, def.Label, cond, dur)
		value = ""
	}
	return events.Event{
		Type:    events.PerfRecovered,
		Source:  r.Name + " · " + st.target,
		Level:   events.LevelInfo,
		Title:   "性能告警已恢复：" + r.Name,
		Message: msg,
		Fields:  map[string]string{"rule": r.Key, "metric": r.Metric, "target": st.target, "value": value, "condition": cond},
		Time:    now,
	}
}

// spanText 把时长写成“N 秒 / N 分钟 / N 小时 M 分钟”。
func spanText(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d 秒", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟", int(d.Minutes()))
	}
	return fmt.Sprintf("%d 小时 %d 分钟", int(d.Hours()), int(d.Minutes())%60)
}

// siteViews 返回最近一次采样的站点指标副本。
func (e *engine) siteViews() []SiteView {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]SiteView(nil), e.views...)
}

// alerts 返回正在进行的告警实例，已触发的在前，其后按开始时间排序。
func (e *engine) alerts() []AlertState {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]AlertState, 0, len(e.states))
	for _, st := range e.states {
		a := AlertState{Rule: st.rule, Name: st.ruleName, Subject: st.subject, Target: st.target, State: "pending", Since: st.since.Unix(), Value: st.value, Level: st.level}
		if st.firing {
			a.State, a.FiredAt = "firing", st.firedAt.Unix()
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		if (out[i].State == "firing") != (out[j].State == "firing") {
			return out[i].State == "firing"
		}
		if out[i].Since != out[j].Since {
			return out[i].Since < out[j].Since
		}
		return out[i].Rule+out[i].Subject < out[j].Rule+out[j].Subject
	})
	return out
}

// firingCounts 返回各规则当前已触发的告警实例数，供 Prometheus 导出。
func (e *engine) firingCounts() map[string]int {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := map[string]int{}
	for _, st := range e.states {
		if st.firing {
			out[st.ruleName]++
		}
	}
	return out
}
