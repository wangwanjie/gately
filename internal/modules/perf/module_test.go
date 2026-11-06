package perf

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/webservice"
	"github.com/wangwanjie/gately/internal/sysstat"
)

func TestRuleValidate(t *testing.T) {
	ok := func(r Rule) Rule {
		t.Helper()
		if err := r.Validate(); err != nil {
			t.Fatalf("应校验通过: %v (%+v)", err, r)
		}
		return r
	}
	bad := func(r Rule, want string) {
		t.Helper()
		err := r.Validate()
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("应报错含 %q，实际 %v (%+v)", want, err, r)
		}
	}
	base := Rule{Name: " CPU ", Metric: MetricHostCPU, Op: OpGT, Threshold: 90, Duration: 5}
	r := ok(base)
	if r.Name != "CPU" || r.Level != levelWarn {
		t.Fatalf("应去空白并补默认级别: %+v", r)
	}
	// 主机指标不使用站点范围与最小请求数
	withSites := base
	withSites.Sites, withSites.MinRequests = []string{"l/a"}, 50
	if r := ok(withSites); r.Sites != nil || r.MinRequests != 0 {
		t.Fatalf("主机指标应清空站点范围与最小请求数: %+v", r)
	}
	// 站点范围去重、去空白
	site := Rule{Name: "x", Metric: MetricSiteActive, Op: OpGE, Threshold: 100, Duration: 1, Sites: []string{" l/a ", "l/a", "", "l/b"}}
	if r := ok(site); strings.Join(r.Sites, ",") != "l/a,l/b" {
		t.Fatalf("站点范围应去重: %+v", r.Sites)
	}
	// 5xx 比例与延迟默认最小请求数；QPS 不需要
	e5 := ok(Rule{Name: "x", Metric: MetricSite5xx, Op: OpGT, Threshold: 5, Duration: 1})
	if e5.MinRequests != DefaultMinRequests {
		t.Fatalf("5xx 比例应有默认最小请求数: %+v", e5)
	}
	if q := ok(Rule{Name: "x", Metric: MetricSiteQPS, Op: OpLT, Threshold: 1, Duration: 1, MinRequests: 9}); q.MinRequests != 0 {
		t.Fatalf("QPS 不应使用最小请求数: %+v", q)
	}

	bad(Rule{Metric: MetricHostCPU, Op: OpGT, Threshold: 1, Duration: 1}, "名称")
	bad(Rule{Name: strings.Repeat("长", 65), Metric: MetricHostCPU, Op: OpGT, Threshold: 1, Duration: 1}, "名称")
	bad(Rule{Name: "x", Metric: "host.nope", Op: OpGT, Threshold: 1, Duration: 1}, "指标")
	bad(Rule{Name: "x", Metric: MetricHostCPU, Op: "==", Threshold: 1, Duration: 1}, "比较符")
	bad(Rule{Name: "x", Metric: MetricHostCPU, Op: OpGT, Threshold: 101, Duration: 1}, "阈值")
	bad(Rule{Name: "x", Metric: MetricHostCPU, Op: OpGT, Threshold: -1, Duration: 1}, "阈值")
	bad(Rule{Name: "x", Metric: MetricHostCPU, Op: OpGT, Threshold: math.NaN(), Duration: 1}, "阈值")
	bad(Rule{Name: "x", Metric: MetricHostCPU, Op: OpGT, Threshold: math.Inf(1), Duration: 1}, "阈值")
	bad(Rule{Name: "x", Metric: MetricHostCPU, Op: OpGT, Threshold: 1, Duration: 0}, "持续时长")
	bad(Rule{Name: "x", Metric: MetricHostCPU, Op: OpGT, Threshold: 1, Duration: MaxDurationMinutes + 1}, "持续时长")
	bad(Rule{Name: "x", Metric: MetricHostCPU, Op: OpGT, Threshold: 1, Duration: 1, Level: "info"}, "级别")
	bad(Rule{Name: "x", Metric: MetricSite5xx, Op: OpGT, Threshold: 1, Duration: 1, MinRequests: -3}, "最小请求数")
	bad(Rule{Name: "x", Metric: MetricSiteP95, Op: OpGT, Threshold: 1, Duration: 1, MinRequests: MaxMinRequests + 1}, "最小请求数")
	bad(Rule{Name: "x", Metric: MetricHostCPU, Op: OpGT, Threshold: 1, Duration: 1, Remark: strings.Repeat("a", 201)}, "备注")
	many := make([]string, MaxRuleSites+1)
	for i := range many {
		many[i] = "l/" + strings.Repeat("x", i%5) + string(rune('a'+i%26))
	}
	bad(Rule{Name: "x", Metric: MetricSiteActive, Op: OpGT, Threshold: 1, Duration: 1, Sites: many}, "站点")
	bad(Rule{Name: "x", Metric: MetricSiteActive, Op: OpGT, Threshold: 1, Duration: 1, Sites: []string{strings.Repeat("x", 201)}}, "站点标识")
}

// fakeHost 返回可控的系统快照。
type fakeHost struct {
	mu      sync.Mutex
	cpu     float64
	managed []sysstat.ManagedProcess
}

func (f *fakeHost) snapshot() sysstat.Snapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	cpu, proc := f.cpu, 1.5
	return sysstat.Snapshot{Hostname: "box", CPU: sysstat.CPU{Cores: 4, Usage: &cpu, Load: []float64{0.5}},
		Memory:  &sysstat.Memory{Total: 1000, Used: 250},
		Disk:    &sysstat.Disk{Total: 100, Used: 10},
		Process: sysstat.Process{PID: 1, CPU: &proc, RSS: 64 << 20, FDs: 12, Goroutines: 30},
		Managed: append([]sysstat.ManagedProcess(nil), f.managed...)}
}

type harness struct {
	m      *Module
	mux    *http.ServeMux
	host   *fakeHost
	now    time.Time
	mu     sync.Mutex
	events []events.Event
}

type muxRouter struct{ mux *http.ServeMux }

func (r muxRouter) HandleFunc(p string, h http.HandlerFunc) {
	method, path, _ := strings.Cut(p, " ")
	r.mux.HandleFunc(method+" /api/perf"+path, h)
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	st, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{host: &fakeHost{cpu: 10}, now: t0}
	m := New(nil)
	m.src.Host = h.host.snapshot
	m.src.Sites = func() []webservice.PerfSite { return nil }
	m.now = func() time.Time { return h.now }
	m.publish = func(e events.Event) {
		h.mu.Lock()
		h.events = append(h.events, e)
		h.mu.Unlock()
	}
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	h.m = m
	h.mux = http.NewServeMux()
	m.Routes(muxRouter{h.mux})
	return h
}

func (h *harness) call(t *testing.T, method, path string, body any) (int, json.RawMessage) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	return rec.Code, json.RawMessage(rec.Body.Bytes())
}

// step 推进 n 次采样。
func (h *harness) step(n int) {
	for i := 0; i < n; i++ {
		h.m.sampleOnce()
		h.now = h.now.Add(SampleInterval)
	}
}

func TestModuleRulesAPIAndAlertFlow(t *testing.T) {
	h := newHarness(t)
	code, _ := h.call(t, "POST", "/api/perf/rules", Rule{Name: "CPU", Enable: true, Metric: MetricHostCPU, Op: OpGT, Threshold: 80, Duration: 1})
	if code != 200 {
		t.Fatalf("新增规则失败: %d", code)
	}
	if code, body := h.call(t, "POST", "/api/perf/rules", Rule{Name: "bad", Metric: MetricHostCPU, Op: OpGT, Threshold: 500, Duration: 1}); code != 400 {
		t.Fatalf("越界阈值应被拒绝: %d %s", code, body)
	}
	// 规则持久化到配置文件
	var saved fileConfig
	if ok, err := h.m.store.Load(ID, &saved); !ok || err != nil || len(saved.Rules) != 1 {
		t.Fatalf("规则应保存: %v %v %+v", ok, err, saved)
	}

	h.host.cpu = 95
	h.step(13)
	h.mu.Lock()
	n := len(h.events)
	h.mu.Unlock()
	if n != 1 || h.events[0].Type != events.PerfAlert {
		t.Fatalf("应触发一次告警: %+v", h.events)
	}
	_, body := h.call(t, "GET", "/api/perf/alerts", nil)
	var alerts []AlertState
	_ = json.Unmarshal(body, &alerts)
	if len(alerts) != 1 || alerts[0].State != "firing" || alerts[0].Target != "box" || alerts[0].Subject != "host" {
		t.Fatalf("告警列表不对: %s", body)
	}
	// 总览摘要与 Prometheus 指标反映触发中的告警
	if s := h.m.Summary(); s[1].Value != "1" || s[1].Level != "error" {
		t.Fatalf("总览摘要不对: %+v", s)
	}
	var found bool
	for _, smp := range h.m.Metrics() {
		if smp.Name == "gately_perf_alerts_firing" && smp.Labels["rule"] == "CPU" && smp.Value == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("缺少 gately_perf_alerts_firing: %+v", h.m.Metrics())
	}

	h.host.cpu = 20
	h.step(1)
	if len(h.events) != 2 || h.events[1].Type != events.PerfRecovered {
		t.Fatalf("应发出恢复事件: %+v", h.events)
	}
}

func TestModuleRuleLimit(t *testing.T) {
	h := newHarness(t)
	for i := 0; i < MaxRules; i++ {
		h.m.cfg.Rules = append(h.m.cfg.Rules, Rule{Key: "k" + string(rune('A'+i%26)) + strings.Repeat("x", i/26), Name: "r", Metric: MetricHostCPU, Op: OpGT, Threshold: 1, Duration: 1})
	}
	if code, _ := h.call(t, "POST", "/api/perf/rules", Rule{Name: "多余", Metric: MetricHostCPU, Op: OpGT, Threshold: 1, Duration: 1}); code != 400 {
		t.Fatalf("超过上限应拒绝新增: %d", code)
	}
	// 已满时仍可修改既有规则
	key := h.m.cfg.Rules[0].Key
	if code, body := h.call(t, "PUT", "/api/perf/rules/"+key, Rule{Name: "改", Metric: MetricHostCPU, Op: OpGT, Threshold: 2, Duration: 1}); code != 200 {
		t.Fatalf("已满时修改应允许: %d %s", code, body)
	}
}

func TestModuleOverviewRanges(t *testing.T) {
	h := newHarness(t)
	if code, _ := h.call(t, "GET", "/api/perf/overview?range=7d", nil); code != 400 {
		t.Fatalf("不支持的时间档应返回 400: %d", code)
	}
	_, body := h.call(t, "GET", "/api/perf/overview", nil)
	var o Overview
	_ = json.Unmarshal(body, &o)
	if o.Range != "60s" || o.Latest != nil || len(o.Series) != 0 {
		t.Fatalf("尚未采样时应为空: %s", body)
	}
	h.step(24) // 2 分钟
	_, body = h.call(t, "GET", "/api/perf/overview?range=60s", nil)
	_ = json.Unmarshal(body, &o)
	if len(o.Series) != shortSlots || o.Step != 5 || o.Latest == nil || *o.Latest.ProcRSS != 64 || *o.Latest.Mem != 25 || *o.Latest.Disk != 10 {
		t.Fatalf("60 秒档不对: %s", body)
	}
	_, body = h.call(t, "GET", "/api/perf/overview?range=1h", nil)
	_ = json.Unmarshal(body, &o)
	if o.Step != 60 || len(o.Series) < 2 || len(o.Series) > 3 {
		t.Fatalf("1 小时档不对: %s", body)
	}
	_, body = h.call(t, "GET", "/api/perf/overview?range=24h", nil)
	_ = json.Unmarshal(body, &o)
	if o.Step != 300 || len(o.Series) != 1 {
		t.Fatalf("24 小时档应合并为 5 分钟一点: %s", body)
	}
	// 历史只含聚合数值，不含访客数据
	if strings.Contains(string(body), "ip") {
		t.Fatalf("输出不应含访客字段: %s", body)
	}
}

func TestModuleStartStop(t *testing.T) {
	h := newHarness(t)
	h.m.now = time.Now
	if err := h.m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	// 重复启动无副作用
	if err := h.m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, ok := h.m.eng.hist.latest(); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("启动后应立即采样一次")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := h.m.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := h.m.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestToHostSampleMissingFields(t *testing.T) {
	s := toHostSample(sysstat.Snapshot{})
	if s.CPU != unknown || s.Mem != unknown || s.Disk != unknown || s.ProcCPU != unknown || s.ProcRSSMB != unknown || s.Load1 != unknown {
		t.Fatalf("缺失字段应为 unknown: %+v", s)
	}
}

// 站点移到其他监听后，告警规则中引用的站点标识随之改写并保存，未引用的规则不变。
func TestRenameSites(t *testing.T) {
	h := newHarness(t)
	for _, r := range []Rule{
		{Name: "慢", Enable: true, Metric: MetricSiteP95, Op: OpGT, Threshold: 500, Duration: 1, Sites: []string{"l1/a", "l2/b", "l3/c"}},
		{Name: "全部", Enable: true, Metric: MetricSiteP95, Op: OpGT, Threshold: 500, Duration: 1},
	} {
		if code, body := h.call(t, "POST", "/api/perf/rules", r); code != 200 {
			t.Fatalf("新增规则失败: %d %s", code, body)
		}
	}
	// l1/a 移入 l2 后与已引用的 l2/b 同名时去重
	h.m.renameSites(map[string]string{"l1/a": "l2/b", "l3/c": "l4/c"})
	var saved fileConfig
	if _, err := h.m.store.Load(ID, &saved); err != nil {
		t.Fatal(err)
	}
	if got := saved.Rules[0].Sites; strings.Join(got, ",") != "l2/b,l4/c" {
		t.Fatalf("规则站点应被改写并保存: %v", got)
	}
	if len(saved.Rules[1].Sites) != 0 {
		t.Fatalf("不限站点的规则不应变化: %v", saved.Rules[1].Sites)
	}
}

// 与真实的 Web 服务模块联动：通过批量接口移动站点后，性能监控规则中的站点标识同步更新。
func TestRenameSitesWiredToWebService(t *testing.T) {
	st, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	env := module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}
	web := webservice.New(nil, nil)
	if err := web.Init(env); err != nil {
		t.Fatal(err)
	}
	m := New(web)
	m.src.Host = (&fakeHost{cpu: 10}).snapshot
	if err := m.Init(env); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	web.Routes(prefixRouter{mux, "/api/webservice"})
	m.Routes(muxRouter{mux})
	call := func(method, path string, body any) (int, string) {
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, strings.NewReader(string(b)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	// 新增监听时 key 由接口生成，从响应中取得
	var keys []string
	for _, l := range []webservice.Listener{
		{Port: 18091, Sites: []webservice.Site{{Key: "a", Enable: true, Domains: []string{"a.test"}, Type: webservice.TypeText, Text: "a"}}},
		{Port: 18092},
	} {
		code, body := call("POST", "/api/webservice/listeners", l)
		var out struct{ Key string }
		if code != 200 || json.Unmarshal([]byte(body), &out) != nil || out.Key == "" {
			t.Fatalf("新增监听失败: %d %s", code, body)
		}
		keys = append(keys, out.Key)
	}
	if code, body := call("POST", "/api/perf/rules", Rule{Name: "慢", Enable: true, Metric: MetricSiteP95, Op: OpGT, Threshold: 500, Duration: 1, Sites: []string{keys[0] + "/a"}}); code != 200 {
		t.Fatalf("新增规则失败: %d %s", code, body)
	}
	req := webservice.SiteBatchRequest{Sites: []webservice.SiteRef{{Listener: keys[0], Site: "a"}}, Action: webservice.BatchMove, Move: &webservice.SiteMoveTarget{Listener: keys[1]}}
	if code, body := call("POST", "/api/webservice/sites/batch", req); code != 200 {
		t.Fatalf("移动站点失败: %d %s", code, body)
	}
	m.mu.Lock()
	got := strings.Join(m.cfg.Rules[0].Sites, ",")
	m.mu.Unlock()
	if got != keys[1]+"/a" {
		t.Fatalf("规则站点应同步为 %s/a，实际 %s", keys[1], got)
	}
}

type prefixRouter struct {
	mux    *http.ServeMux
	prefix string
}

func (r prefixRouter) HandleFunc(p string, h http.HandlerFunc) {
	method, path, _ := strings.Cut(p, " ")
	r.mux.HandleFunc(method+" "+r.prefix+path, h)
}
