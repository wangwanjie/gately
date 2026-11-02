package perf

import (
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/modules/webservice"
)

var t0 = time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)

func hostAt(cpu float64) hostSample {
	return hostSample{Name: "nas", CPU: cpu, Mem: 40, Disk: 50, Load1: 1, ProcCPU: 2, ProcRSSMB: 100, ProcFD: 30, Goroutines: 50}
}

func cpuRule(duration int) Rule {
	return Rule{Key: "r1", Name: "CPU 过高", Enable: true, Metric: MetricHostCPU, Op: OpGT, Threshold: 80, Duration: duration, Level: levelError}
}

// run 以 SampleInterval 间隔连续喂入 n 次采样，返回期间产生的全部事件。
func run(e *engine, start time.Time, n int, host func(i int) hostSample, sites func(i int) []webservice.PerfSite, rules []Rule) ([]events.Event, time.Time) {
	var out []events.Event
	now := start
	for i := 0; i < n; i++ {
		var s []webservice.PerfSite
		if sites != nil {
			s = sites(i)
		}
		out = append(out, e.tick(now, host(i), s, rules)...)
		now = now.Add(SampleInterval)
	}
	return out, now
}

func TestAlertFiresAfterDurationAndRecovers(t *testing.T) {
	e := newEngine()
	rules := []Rule{cpuRule(1)}
	// 11 次采样（跨 50 秒）条件持续满足但不足 1 分钟：只计时不触发
	evs, now := run(e, t0, 11, func(int) hostSample { return hostAt(95) }, nil, rules)
	if len(evs) != 0 {
		t.Fatalf("未满持续时长不应触发: %+v", evs)
	}
	if a := e.alerts(); len(a) != 1 || a[0].State != "pending" {
		t.Fatalf("应有一条计时中的告警: %+v", a)
	}
	// 第 13 次采样（距首次满足恰好 60 秒）触发一次，第 12 次（55 秒）仍不触发
	evs, now = run(e, now, 2, func(int) hostSample { return hostAt(95) }, nil, rules)
	if len(evs) != 1 || evs[0].Type != events.PerfAlert || evs[0].Level != levelError {
		t.Fatalf("应触发 1 条错误级告警: %+v", evs)
	}
	if !strings.HasPrefix(evs[0].Message, "nas：") || evs[0].Source != "CPU 过高 · nas" || !strings.Contains(evs[0].Message, "95.0%") {
		t.Fatalf("告警文案缺少对象或数值: %s", evs[0].Message)
	}
	// 继续满足条件不重复触发
	evs, now = run(e, now, 20, func(int) hostSample { return hostAt(99) }, nil, rules)
	if len(evs) != 0 {
		t.Fatalf("已触发的告警不应重复发送: %+v", evs)
	}
	if a := e.alerts(); len(a) != 1 || a[0].State != "firing" {
		t.Fatalf("应为触发中: %+v", a)
	}
	// 条件解除：发一条恢复事件，状态清空
	evs, _ = run(e, now, 1, func(int) hostSample { return hostAt(10) }, nil, rules)
	if len(evs) != 1 || evs[0].Type != events.PerfRecovered || evs[0].Level != events.LevelInfo || !strings.Contains(evs[0].Message, "当前 10.0%") {
		t.Fatalf("应发出带当前值的恢复事件: %+v", evs)
	}
	if a := e.alerts(); len(a) != 0 {
		t.Fatalf("恢复后不应有告警: %+v", a)
	}
}

func TestPendingResetsWhenConditionDrops(t *testing.T) {
	e := newEngine()
	rules := []Rule{cpuRule(1)}
	// 满足 40 秒后回落一次，再满足 40 秒：每段都不足 1 分钟，不能累计
	pattern := func(i int) hostSample {
		if i == 8 {
			return hostAt(10)
		}
		return hostAt(95)
	}
	evs, _ := run(e, t0, 17, pattern, nil, rules)
	if len(evs) != 0 {
		t.Fatalf("回落后计时应清零，不应触发: %+v", evs)
	}
}

func TestOperators(t *testing.T) {
	cases := []struct {
		op   string
		thr  float64
		v    float64
		want bool
	}{
		{OpGT, 80, 80, false}, {OpGT, 80, 80.1, true},
		{OpGE, 80, 80, true}, {OpGE, 80, 79.9, false},
		{OpLT, 5, 4.9, true}, {OpLT, 5, 5, false},
		{OpLE, 5, 5, true}, {OpLE, 5, 5.1, false},
	}
	for _, c := range cases {
		r := Rule{Op: c.op, Threshold: c.thr}
		if got := r.compare(c.v); got != c.want {
			t.Errorf("%v %s %v = %v，期望 %v", c.v, c.op, c.thr, got, c.want)
		}
	}
}

func TestUnknownValueIsNotMet(t *testing.T) {
	e := newEngine()
	rules := []Rule{{Key: "r", Name: "低于", Enable: true, Metric: MetricHostCPU, Op: OpLT, Threshold: 50, Duration: 1, Level: levelWarn}}
	// 取不到 CPU 时即使比较符是 “<” 也不能算满足
	evs, _ := run(e, t0, 30, func(int) hostSample { h := hostAt(0); h.CPU = unknown; return h }, nil, rules)
	if len(evs) != 0 || len(e.alerts()) != 0 {
		t.Fatalf("数据缺失不应触发: %+v", evs)
	}
}

func TestDisabledAndEditedRuleDropsState(t *testing.T) {
	e := newEngine()
	rules := []Rule{cpuRule(1)}
	_, now := run(e, t0, 5, func(int) hostSample { return hostAt(95) }, nil, rules)
	if len(e.alerts()) != 1 {
		t.Fatal("应有计时中的告警")
	}
	// 只改名称不改判定条件：保留计时
	renamed := cpuRule(1)
	renamed.Name = "改名"
	_, now = run(e, now, 1, func(int) hostSample { return hostAt(95) }, nil, []Rule{renamed})
	a := e.alerts()
	if len(a) != 1 || a[0].Since != t0.Unix() {
		t.Fatalf("改名不应重置计时: %+v", a)
	}
	// 改阈值：计时重新开始
	changed := cpuRule(1)
	changed.Threshold = 70
	_, now = run(e, now, 1, func(int) hostSample { return hostAt(95) }, nil, []Rule{changed})
	a = e.alerts()
	if len(a) != 1 || a[0].Since == t0.Unix() {
		t.Fatalf("修改阈值应重新计时: %+v", a)
	}
	// 停用：状态清除且不发任何事件
	disabled := changed
	disabled.Enable = false
	evs, _ := run(e, now, 1, func(int) hostSample { return hostAt(95) }, nil, []Rule{disabled})
	if len(evs) != 0 || len(e.alerts()) != 0 {
		t.Fatalf("停用应静默清除状态: %+v", evs)
	}
}

// siteAt 构造一个站点在第 i 次采样时的累计计数：每个采样间隔 reqPer 个请求，其中 errPer 个 5xx，
// 全部落在 latBucket 号延迟桶，每个请求的延迟为该桶的上界。
func siteAt(key string, i int, reqPer, errPer uint64, latBucket int, active int64) webservice.PerfSite {
	n := uint64(i)
	s := webservice.PerfSite{Key: key, Name: "web / " + key, Active: active, Requests: n * reqPer, Done: n * reqPer, S5xx: n * errPer, LatCount: n * reqPer}
	s.Lat[latBucket] = n * reqPer
	s.LatSum[latBucket] = float64(n*reqPer) * webservice.LatencyBounds()[latBucket]
	return s
}

func TestSiteWindowMetrics(t *testing.T) {
	e := newEngine()
	// 每 5 秒 10 个请求（2 QPS），其中 2 个 5xx（20%），每个请求延迟 300 毫秒
	bucket := 26
	if b := webservice.LatencyBounds(); b[bucket] != 300 {
		t.Fatalf("桶边界变了，测试需要同步: %v", b)
	}
	sites := func(i int) []webservice.PerfSite { return []webservice.PerfSite{siteAt("l/a", i, 10, 2, bucket, 7)} }
	run(e, t0, 14, func(int) hostSample { return hostAt(1) }, sites, nil)
	v := e.siteViews()
	if len(v) != 1 || v[0].Active != 7 {
		t.Fatalf("站点视图不对: %+v", v)
	}
	if v[0].QPS == nil || *v[0].QPS < 1.99 || *v[0].QPS > 2.01 {
		t.Fatalf("QPS 应为 2: %v", v[0].QPS)
	}
	if v[0].Err5xx == nil || *v[0].Err5xx < 19.9 || *v[0].Err5xx > 20.1 {
		t.Fatalf("5xx 比例应为 20%%: %v", v[0].Err5xx)
	}
	if v[0].P95 == nil || *v[0].P95 != 300 {
		t.Fatalf("P95 应为 300 毫秒: %v", v[0].P95)
	}
}

func TestSiteWindowNeedsEnoughSpan(t *testing.T) {
	e := newEngine()
	sites := func(i int) []webservice.PerfSite { return []webservice.PerfSite{siteAt("l/a", i, 10, 0, 3, 0)} }
	// 只采 3 次（跨 10 秒）：窗口类指标数据不足
	run(e, t0, 3, func(int) hostSample { return hostAt(1) }, sites, nil)
	v := e.siteViews()
	if v[0].QPS != nil || v[0].Err5xx != nil || v[0].P95 != nil {
		t.Fatalf("窗口不足时应为空: %+v", v[0])
	}
}

func TestSiteCounterResetClearsWindow(t *testing.T) {
	e := newEngine()
	site := func(i int) []webservice.PerfSite { return []webservice.PerfSite{siteAt("l/a", i, 10, 0, 3, 0)} }
	_, now := run(e, t0, 14, func(int) hostSample { return hostAt(1) }, site, nil)
	// 计数回到 0（站点被重建）：不能出现负的差值，窗口重新开始
	e.tick(now, hostAt(1), []webservice.PerfSite{siteAt("l/a", 0, 10, 0, 3, 0)}, nil)
	v := e.siteViews()
	if v[0].QPS != nil {
		t.Fatalf("计数回退后窗口应重置: %+v", v[0])
	}
}

func TestSite5xxRespectsMinRequests(t *testing.T) {
	e := newEngine()
	// 每 5 秒 1 个请求且全是 5xx：窗口内约 12 个请求，低于默认最小请求数 20，不判定
	sites := func(i int) []webservice.PerfSite { return []webservice.PerfSite{siteAt("l/a", i, 1, 1, 3, 0)} }
	rule := Rule{Key: "r", Name: "5xx", Enable: true, Metric: MetricSite5xx, Op: OpGT, Threshold: 5, Duration: 1, MinRequests: 20, Level: levelWarn}
	evs, _ := run(e, t0, 40, func(int) hostSample { return hostAt(1) }, sites, []Rule{rule})
	if len(evs) != 0 {
		t.Fatalf("请求太少不应判定: %+v", evs)
	}
	// 最小请求数降到 5：触发
	e2 := newEngine()
	rule.MinRequests = 5
	evs, _ = run(e2, t0, 40, func(int) hostSample { return hostAt(1) }, sites, []Rule{rule})
	if len(evs) != 1 || evs[0].Type != events.PerfAlert {
		t.Fatalf("应触发 5xx 告警: %+v", evs)
	}
	// 流量停止：窗口内请求不足，按无法判定恢复，文案不写当前值
	stopped := func(int) []webservice.PerfSite { return []webservice.PerfSite{siteAt("l/a", 40, 1, 1, 3, 0)} }
	evs, _ = run(e2, t0.Add(40*SampleInterval), 20, func(int) hostSample { return hostAt(1) }, stopped, []Rule{rule})
	if len(evs) != 1 || evs[0].Type != events.PerfRecovered || !strings.Contains(evs[0].Message, "无法判定") || evs[0].Fields["value"] != "" {
		t.Fatalf("流量停止应按无法判定恢复: %+v", evs)
	}
}

func TestSiteScopeAndPerSiteState(t *testing.T) {
	e := newEngine()
	two := func(i int) []webservice.PerfSite {
		return []webservice.PerfSite{siteAt("l/a", i, 1, 0, 1, 500), siteAt("l/b", i, 1, 0, 1, 500)}
	}
	// 规则只管站点 a：b 的连接数同样超限也不告警
	rule := Rule{Key: "r", Name: "连接数", Enable: true, Metric: MetricSiteActive, Op: OpGT, Threshold: 100, Duration: 1, Sites: []string{"l/a"}, Level: levelWarn}
	evs, _ := run(e, t0, 13, func(int) hostSample { return hostAt(1) }, two, []Rule{rule})
	if len(evs) != 1 || !strings.Contains(evs[0].Message, "l/a") {
		t.Fatalf("只应对站点 a 告警: %+v", evs)
	}
	// 范围留空为全部站点：a、b 各自独立触发
	e2 := newEngine()
	rule.Sites = nil
	evs, _ = run(e2, t0, 13, func(int) hostSample { return hostAt(1) }, two, []Rule{rule})
	if len(evs) != 2 {
		t.Fatalf("两个站点应各告警一次: %+v", evs)
	}
	if evs[0].Source == evs[1].Source {
		t.Fatalf("不同站点的事件来源必须不同，否则通知静默会互相吞掉: %s", evs[0].Source)
	}
}

func TestSiteDisappearDropsStateSilently(t *testing.T) {
	e := newEngine()
	rule := Rule{Key: "r", Name: "连接数", Enable: true, Metric: MetricSiteActive, Op: OpGT, Threshold: 1, Duration: 1, Level: levelWarn}
	site := func(i int) []webservice.PerfSite { return []webservice.PerfSite{siteAt("l/a", i, 1, 0, 1, 5)} }
	_, now := run(e, t0, 13, func(int) hostSample { return hostAt(1) }, site, []Rule{rule})
	if len(e.alerts()) != 1 {
		t.Fatal("应已触发")
	}
	evs := e.tick(now, hostAt(1), nil, []Rule{rule})
	if len(evs) != 0 || len(e.alerts()) != 0 {
		t.Fatalf("站点消失应静默清除: %+v", evs)
	}
}

func TestUpstreamConnsPerNode(t *testing.T) {
	e := newEngine()
	rule := Rule{Key: "r", Name: "后端连接", Enable: true, Metric: MetricUpstreamC, Op: OpGT, Threshold: 10, Duration: 1, Level: levelWarn}
	site := func(i int) []webservice.PerfSite {
		s := siteAt("l/a", i, 1, 0, 1, 0)
		s.Upstreams = []webservice.PerfUpstream{
			{URL: "http://10.0.0.1:80", Healthy: true, Conns: 50},
			{URL: "http://10.0.0.2:80", Healthy: true, Conns: 3},
		}
		return []webservice.PerfSite{s}
	}
	evs, _ := run(e, t0, 13, func(int) hostSample { return hostAt(1) }, site, []Rule{rule})
	if len(evs) != 1 || !strings.Contains(evs[0].Message, "10.0.0.1") || strings.Contains(evs[0].Message, "10.0.0.2") {
		t.Fatalf("只有超限的节点告警: %+v", evs)
	}
}

func TestQPSLessThanDetectsTrafficLoss(t *testing.T) {
	e := newEngine()
	rule := Rule{Key: "r", Name: "无流量", Enable: true, Metric: MetricSiteQPS, Op: OpLT, Threshold: 0.5, Duration: 1, Level: levelWarn}
	// 请求数不再增长：QPS 为 0，窗口满 30 秒后开始计时，再满 1 分钟触发
	site := func(int) []webservice.PerfSite { return []webservice.PerfSite{siteAt("l/a", 10, 1, 0, 1, 0)} }
	evs, _ := run(e, t0, 30, func(int) hostSample { return hostAt(1) }, site, []Rule{rule})
	if len(evs) != 1 {
		t.Fatalf("无流量应触发一次: %+v", evs)
	}
}

func TestHostHistoryRingAndMerge(t *testing.T) {
	h := &hostHistory{}
	// 连续 3 分钟，每 5 秒一次，CPU 在每分钟内取 10 与 90 交替
	now := t0
	for i := 0; i < 36; i++ {
		v := 10.0
		if i%2 == 1 {
			v = 90
		}
		s := hostAt(v)
		s.At = now
		h.add(s)
		now = now.Add(SampleInterval)
	}
	pts := h.minutes(now.Add(-time.Second), 5)
	if len(pts) != 3 {
		t.Fatalf("应有 3 个分钟点，实际 %d", len(pts))
	}
	if pts[0].CPU != 50 || pts[0].CPUMax != 90 {
		t.Fatalf("分钟均值应为 50、峰值 90: %+v", pts[0])
	}
	if got := h.recent(); len(got) != shortSlots {
		t.Fatalf("秒级环应保留 %d 点，实际 %d", shortSlots, len(got))
	}
	m := merge(pts, 3)
	if len(m) != 1 || m[0].CPU != 50 || m[0].CPUMax != 90 || m[0].T != pts[0].T {
		t.Fatalf("合并结果不对: %+v", m)
	}
}

func TestHostHistoryWrapsAfter24h(t *testing.T) {
	h := &hostHistory{}
	now := t0
	// 写入 25 小时，每分钟一个采样，旧的槽被覆盖后不能读出过期数据
	for i := 0; i < 25*60; i++ {
		s := hostAt(float64(i % 100))
		s.At = now
		h.add(s)
		now = now.Add(time.Minute)
	}
	pts := h.minutes(now, historyMinutes)
	if len(pts) < historyMinutes-1 {
		t.Fatalf("应保留约 24 小时，实际 %d", len(pts))
	}
	if pts[0].T < now.Add(-time.Duration(historyMinutes)*time.Minute).Unix() {
		t.Fatalf("不应返回超过 24 小时的数据: %d", pts[0].T)
	}
	for i := 1; i < len(pts); i++ {
		if pts[i].T <= pts[i-1].T {
			t.Fatal("时间必须递增")
		}
	}
}

func TestUnknownFieldsStayNull(t *testing.T) {
	h := &hostHistory{}
	s := hostAt(0)
	s.CPU, s.ProcCPU = unknown, unknown
	s.At = t0
	h.add(s)
	pts := h.minutes(t0, 1)
	if len(pts) != 1 || pts[0].CPU != unknown || pts[0].CPUMax != unknown || pts[0].Mem != 40 {
		t.Fatalf("缺失字段应保持 unknown: %+v", pts)
	}
	if p := pointOf(pts[0]); p.CPU != nil || p.Mem == nil {
		t.Fatalf("输出应把缺失字段置为 null: %+v", p)
	}
}
