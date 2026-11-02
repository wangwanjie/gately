package perf

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/webservice"
	"github.com/wangwanjie/gately/internal/sysstat"
)

// ID 是模块标识。
const ID = "perf"

type fileConfig struct {
	Rules []Rule `json:"rules"`
}

// Sources 是采样的数据来源，测试中替换为假数据。
type Sources struct {
	// Host 采集主机与进程信息
	Host func() sysstat.Snapshot
	// Sites 返回各运行中站点的性能快照，可为 nil
	Sites func() []webservice.PerfSite
}

// Module 是性能监控模块。
type Module struct {
	mu    sync.Mutex
	store *config.Store
	logs  *logbuf.Buffer
	log   *logbuf.Logger
	cfg   fileConfig
	eng   *engine
	src   Sources
	// publish 允许测试替换事件发布
	publish func(events.Event)
	now     func() time.Time
	cancel  context.CancelFunc
	done    chan struct{}
}

// New 创建模块，web 为 Web 服务模块，用于取得各站点的性能快照；可为 nil（此时只监控主机与进程）。
func New(web *webservice.Module) *Module {
	m := &Module{eng: newEngine(), publish: events.Publish, now: time.Now}
	if web != nil {
		m.src.Sites = web.PerfSites
	}
	return m
}

// ID 实现 module.Module。
func (m *Module) ID() string { return ID }

// Title 实现 module.Module。
func (m *Module) Title() string { return "性能监控" }

// Init 实现 module.Module。
func (m *Module) Init(env module.Env) error {
	m.store, m.logs = env.Store, env.Logs
	m.log = m.logs.Logger("性能监控")
	if m.src.Host == nil {
		c := sysstat.New(env.Store.Dir())
		m.src.Host = c.Snapshot
	}
	_, err := m.store.Load(ID, &m.cfg)
	return err
}

// Start 实现 module.Module：启动后台采样与告警评估。
func (m *Module) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		return nil
	}
	ctx, m.cancel = context.WithCancel(ctx)
	m.done = make(chan struct{})
	go m.loop(ctx, m.done)
	return nil
}

// Stop 实现 module.Module。
func (m *Module) Stop() error {
	m.mu.Lock()
	cancel, done := m.cancel, m.done
	m.cancel, m.done = nil, nil
	m.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
	return nil
}

func (m *Module) loop(ctx context.Context, done chan struct{}) {
	defer close(done)
	t := time.NewTicker(SampleInterval)
	defer t.Stop()
	for {
		m.sampleOnce()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// sampleOnce 执行一次采样与评估并发布产生的事件。
func (m *Module) sampleOnce() {
	var host hostSample
	host = toHostSample(m.src.Host())
	var sites []webservice.PerfSite
	if m.src.Sites != nil {
		sites = m.src.Sites()
	}
	m.mu.Lock()
	rules := append([]Rule(nil), m.cfg.Rules...)
	m.mu.Unlock()
	for _, e := range m.eng.tick(m.now(), host, sites, rules) {
		if m.log != nil {
			m.log.Warnf("%s", e.Title+"："+e.Message)
		}
		m.publish(e)
	}
}

// toHostSample 把系统快照转换为采样值，取不到的字段为 unknown。
func toHostSample(s sysstat.Snapshot) hostSample {
	h := hostSample{Name: s.Hostname, CPU: unknown, Mem: unknown, Disk: unknown, Load1: unknown, ProcCPU: unknown, ProcRSSMB: unknown, ProcFD: unknown}
	if s.CPU.Usage != nil {
		h.CPU = *s.CPU.Usage
	}
	if len(s.CPU.Load) > 0 {
		h.Load1 = s.CPU.Load[0]
	}
	if s.Memory != nil && s.Memory.Total > 0 {
		h.Mem = float64(s.Memory.Used) / float64(s.Memory.Total) * 100
	}
	if s.Disk != nil && s.Disk.Total > 0 {
		h.Disk = float64(s.Disk.Used) / float64(s.Disk.Total) * 100
	}
	if s.Process.CPU != nil {
		h.ProcCPU = *s.Process.CPU
	}
	if s.Process.RSS > 0 {
		h.ProcRSSMB = float64(s.Process.RSS) / (1 << 20)
	}
	if s.Process.FDs > 0 {
		h.ProcFD = float64(s.Process.FDs)
	}
	h.Goroutines = float64(s.Process.Goroutines)
	return h
}

func (m *Module) saveLocked() error { return m.store.Save(ID, m.cfg) }

// Routes 实现 module.Module。
func (m *Module) Routes(r module.Router) {
	rules := &crud.Collection[Rule]{
		Mu:       &m.mu,
		Items:    &m.cfg.Rules,
		Save:     m.saveLocked,
		Key:      func(x *Rule) *string { return &x.Key },
		Enabled:  func(x *Rule) *bool { return &x.Enable },
		Group:    func(x *Rule) *string { return &x.Group },
		Validate: func(x *Rule) error { return x.Validate() },
		Max:      MaxRules,
	}
	rules.Register(r, "/rules")
	r.HandleFunc("GET /metrics", m.handleMetricDefs)
	r.HandleFunc("GET /overview", m.handleOverview)
	r.HandleFunc("GET /sites", m.handleSites)
	r.HandleFunc("GET /alerts", m.handleAlerts)
	module.DocOf(r, "GET /metrics", module.Doc{Summary: "可用于告警的指标", Resp: []MetricDef{}})
	module.DocOf(r, "GET /overview", module.Doc{Summary: "主机与进程历史（?range=60s|1h|24h）", Resp: &Overview{}})
	module.DocOf(r, "GET /sites", module.Doc{Summary: "各站点最近约 60 秒的指标", Resp: []SiteView{}})
	module.DocOf(r, "GET /alerts", module.Doc{Summary: "进行中的告警（计时中与已触发）", Resp: []AlertState{}})
}

func (m *Module) handleMetricDefs(w http.ResponseWriter, _ *http.Request) {
	httpx.OK(w, Metrics)
}

func (m *Module) handleSites(w http.ResponseWriter, _ *http.Request) {
	httpx.OK(w, m.eng.siteViews())
}

func (m *Module) handleAlerts(w http.ResponseWriter, _ *http.Request) {
	httpx.OK(w, m.eng.alerts())
}

// Point 是主机与进程历史中的一个点，没有数据的字段为 null。
type Point struct {
	Time       int64    `json:"t"`
	CPU        *float64 `json:"cpu"`
	CPUMax     *float64 `json:"cpuMax"`
	Mem        *float64 `json:"mem"`
	Disk       *float64 `json:"disk"`
	Load1      *float64 `json:"load1"`
	ProcCPU    *float64 `json:"procCpu"`
	ProcCPUMax *float64 `json:"procCpuMax"`
	// ProcRSS 为常驻内存（MB）
	ProcRSS    *float64 `json:"procRss"`
	ProcFDs    *float64 `json:"procFds"`
	Goroutines *float64 `json:"goroutines"`
}

// Overview 是主机与进程历史接口的输出。
type Overview struct {
	// Range 为 60s、1h 或 24h；Step 为相邻两点的间隔秒数（5 秒、60 秒或 300 秒）
	Range  string  `json:"range"`
	Step   int     `json:"step"`
	Series []Point `json:"series"`
	// Latest 为最近一次采样，尚未采样时为 null
	Latest *Point `json:"latest"`
}

func opt(v float64) *float64 {
	if v == unknown {
		return nil
	}
	return &v
}

func pointOf(p hostPoint) Point {
	return Point{Time: p.T, CPU: opt(p.CPU), CPUMax: opt(p.CPUMax), Mem: opt(p.Mem), Disk: opt(p.Disk), Load1: opt(p.Load1),
		ProcCPU: opt(p.ProcCPU), ProcCPUMax: opt(p.ProcCPUMax), ProcRSS: opt(p.ProcRSSMB), ProcFDs: opt(p.ProcFD), Goroutines: opt(p.Goroutines)}
}

func pointOfSample(s hostSample) Point {
	return pointOf(hostPoint{T: s.At.Unix(), CPU: s.CPU, CPUMax: s.CPU, Mem: s.Mem, Disk: s.Disk, Load1: s.Load1,
		ProcCPU: s.ProcCPU, ProcCPUMax: s.ProcCPU, ProcRSSMB: s.ProcRSSMB, ProcFD: s.ProcFD, Goroutines: s.Goroutines})
}

// overview 返回指定时间档的历史。
func (m *Module) overview(rng string, now time.Time) (Overview, error) {
	out := Overview{Range: rng, Series: []Point{}}
	switch rng {
	case "", "60s":
		out.Range, out.Step = "60s", int(SampleInterval/time.Second)
		for _, s := range m.eng.hist.recent() {
			out.Series = append(out.Series, pointOfSample(s))
		}
	case "1h":
		out.Step = 60
		for _, p := range m.eng.hist.minutes(now, 60) {
			out.Series = append(out.Series, pointOf(p))
		}
	case "24h":
		out.Step = bucketMinutes * 60
		for _, p := range merge(m.eng.hist.minutes(now, historyMinutes), bucketMinutes) {
			out.Series = append(out.Series, pointOf(p))
		}
	default:
		return out, errors.New("range 只支持 60s、1h、24h")
	}
	if s, ok := m.eng.hist.latest(); ok {
		p := pointOfSample(s)
		out.Latest = &p
	}
	return out, nil
}

func (m *Module) handleOverview(w http.ResponseWriter, r *http.Request) {
	o, err := m.overview(r.URL.Query().Get("range"), m.now())
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	httpx.OK(w, o)
}
