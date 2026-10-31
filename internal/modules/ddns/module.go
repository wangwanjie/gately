// Package ddns 实现动态域名解析模块。
package ddns

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/dnsprovider"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/ipsource"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// ID 是模块标识。
const ID = "ddns"

// 启动后首次检查的延迟，给网络留出就绪时间
var startupDelay = 10 * time.Second

// Module 是 DDNS 模块。
type Module struct {
	mu      sync.Mutex
	store   *config.Store
	logs    *logbuf.Buffer
	cfg     fileConfig
	ctx     context.Context
	runners map[string]*runner
	// unsub 取消对 STUN 地址变化事件的订阅
	unsub func()
}

// New 创建模块实例。
func New() *Module {
	return &Module{runners: map[string]*runner{}}
}

// ID 实现 module.Module。
func (m *Module) ID() string { return ID }

// Title 实现 module.Module。
func (m *Module) Title() string { return "动态域名" }

// Init 实现 module.Module。
func (m *Module) Init(env module.Env) error {
	m.store = env.Store
	m.logs = env.Logs
	m.cfg = fileConfig{Settings: Settings{Enable: true}}
	_, err := m.store.Load(ID, &m.cfg)
	return err
}

// Start 实现 module.Module。
func (m *Module) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ctx = ctx
	if m.unsub == nil {
		m.unsub = events.Subscribe(m.onEvent)
	}
	if !m.cfg.Settings.Enable {
		m.logs.Add(logbuf.LevelInfo, "", "动态域名模块已禁用")
		return nil
	}
	for _, t := range m.cfg.Tasks {
		if t.Enable {
			m.startLocked(t, startupDelay)
		}
	}
	return nil
}

// Stop 实现 module.Module。
func (m *Module) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.unsub != nil {
		m.unsub()
		m.unsub = nil
	}
	for key := range m.runners {
		m.stopLocked(key)
	}
	return nil
}

// onEvent 在 STUN 规则公网地址变化时，让引用了该规则变量的任务立即同步，不必等到下个检查周期。
func (m *Module) onEvent(e events.Event) {
	if e.Type != events.STUNChanged {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.runners {
		// 变量既可以写规则名，也可以写规则 key
		inRecords, inWebhook := r.task.stunRefs(e.Source, e.Fields["ruleKey"])
		if inWebhook {
			// 只在 Webhook 中引用时记录值不会变化，标记后本次同步照样调用 Webhook
			r.stunWebhook.Store(true)
		}
		if inRecords || inWebhook {
			r.requestSync(false)
		}
	}
}

func (m *Module) startLocked(t Task, delay time.Duration) {
	if m.ctx == nil {
		return
	}
	var prev *TaskState
	var prevTask *Task
	if old, ok := m.runners[t.Key]; ok {
		// 先停止再取快照，确保拿到进行中那次同步的结果
		old.stop()
		s := old.snapshot()
		prev = &s
		pt := old.task
		prevTask = &pt
	}
	label := t.Name
	if label == "" {
		label = t.Provider
	}
	r := newRunner(t, m.logs.Logger(label), prev, prevTask)
	r.start(m.ctx, delay)
	m.runners[t.Key] = r
}

func (m *Module) stopLocked(key string) {
	if r, ok := m.runners[key]; ok {
		r.stop()
		delete(m.runners, key)
	}
}

// restartLocked 按最新配置重启任务；修改后立即执行一次同步。
func (m *Module) restartLocked(key string) {
	t := m.findLocked(key)
	if t == nil || !t.Enable || !m.cfg.Settings.Enable {
		m.stopLocked(key)
		return
	}
	m.startLocked(*t, 0)
}

func (m *Module) findLocked(key string) *Task {
	for i := range m.cfg.Tasks {
		if m.cfg.Tasks[i].Key == key {
			return &m.cfg.Tasks[i]
		}
	}
	return nil
}

func (m *Module) saveLocked() error {
	return m.store.Save(ID, m.cfg)
}

// TaskView 是带运行状态、凭据脱敏的任务。
type TaskView struct {
	Task
	State TaskState `json:"state"`
}

func (m *Module) viewLocked(t *Task) any {
	v := TaskView{Task: *t}
	v.Credentials.Secret = crud.Mask(t.Credentials.Secret)
	if r, ok := m.runners[t.Key]; ok {
		v.State = r.snapshot()
	} else {
		v.State.Records = map[string]RecordState{}
	}
	return v
}

// Routes 实现 module.Module。
func (m *Module) Routes(r module.Router) {
	tasks := &crud.Collection[Task]{
		Mu:       &m.mu,
		Items:    &m.cfg.Tasks,
		Save:     m.saveLocked,
		Key:      func(t *Task) *string { return &t.Key },
		Enabled:  func(t *Task) *bool { return &t.Enable },
		Group:    func(t *Task) *string { return &t.Group },
		Validate: func(t *Task) error { return t.Validate() },
		Merge: func(old, updated *Task) {
			crud.KeepSecret(old.Credentials.Secret, &updated.Credentials.Secret)
		},
		View: m.viewLocked,
		// 查看被脱敏的 DNS 服务商凭据密钥明文
		Secrets: func(t *Task) map[string]string { return map[string]string{"secret": t.Credentials.Secret} },
		Changed: m.restartLocked,
		Removed: m.stopLocked,
	}
	tasks.Register(r, "/tasks")
	r.HandleFunc("POST /tasks/{key}/sync", m.handleSync)
	r.HandleFunc("DELETE /tasks/{key}/records/{rec}", m.handleDeleteRecord)
	r.HandleFunc("PUT /tasks/{key}/records-layout", m.handleRecordsLayout)
	module.DocOf(r, "PUT /tasks/{key}/records-layout", module.Doc{Summary: "调整任务内解析记录的顺序与分组", Req: &recordsLayoutRequest{}})
	r.HandleFunc("GET /providers", func(w http.ResponseWriter, r *http.Request) {
		httpx.OK(w, dnsprovider.List())
	})
	r.HandleFunc("GET /netinterfaces", func(w http.ResponseWriter, r *http.Request) {
		list, err := ipsource.ListInterfaces()
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "%v", err)
			return
		}
		httpx.OK(w, list)
	})
	r.HandleFunc("POST /iptest", m.handleIPTest)
	r.HandleFunc("GET /defaults", func(w http.ResponseWriter, r *http.Request) {
		httpx.OK(w, map[string]any{"ipv4URLs": ipsource.DefaultIPv4URLs, "ipv6URLs": ipsource.DefaultIPv6URLs})
	})
	r.HandleFunc("GET /settings", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		httpx.OK(w, m.cfg.Settings)
	})
	r.HandleFunc("PUT /settings", m.handlePutSettings)
}

func (m *Module) handleSync(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.runners[r.PathValue("key")]
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "任务未运行")
		return
	}
	run.requestSync(r.URL.Query().Get("force") == "1")
	httpx.OK(w, map[string]bool{"ok": true})
}

// handleDeleteRecord 从任务中移除一条记录，供列表页不进入编辑直接删除。
// 只修改 Gately 的任务配置，DNS 服务商处已有的解析不受影响；任务至少保留一条记录。
func (m *Module) handleDeleteRecord(w http.ResponseWriter, r *http.Request) {
	key, rec := r.PathValue("key"), r.PathValue("rec")
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.findLocked(key)
	if t == nil {
		httpx.Error(w, http.StatusNotFound, "条目不存在")
		return
	}
	// 新建切片，避免校验或保存失败时改动原任务的记录
	records := make([]Record, 0, len(t.Records))
	for _, it := range t.Records {
		if it.Key != rec {
			records = append(records, it)
		}
	}
	if len(records) == len(t.Records) {
		httpx.Error(w, http.StatusNotFound, "记录不存在")
		return
	}
	if len(records) == 0 {
		httpx.Error(w, http.StatusBadRequest, "任务至少保留一条记录，如需全部删除请删除任务")
		return
	}
	old := *t
	updated := *t
	updated.Records = records
	if err := updated.Validate(); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	*t = updated
	if err := m.saveLocked(); err != nil {
		*t = old
		httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
		return
	}
	m.restartLocked(key)
	httpx.OK(w, m.viewLocked(t))
}

type ipTestRequest struct {
	Source ipsource.Source `json:"source"`
	Family int             `json:"family"`
}

func (m *Module) handleIPTest(w http.ResponseWriter, r *http.Request) {
	var req ipTestRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	req.Source.Enable = true
	if err := req.Source.Validate(); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	fam := ipsource.IPv4
	if req.Family == 6 {
		fam = ipsource.IPv6
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	addr, err := ipsource.Get(ctx, req.Source, fam)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	httpx.OK(w, map[string]string{"ip": addr.String()})
}

func (m *Module) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var s Settings
	if err := httpx.ReadJSON(r, &s); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	old := m.cfg.Settings
	m.cfg.Settings = s
	if err := m.saveLocked(); err != nil {
		m.cfg.Settings = old
		httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
		return
	}
	for key := range m.runners {
		m.stopLocked(key)
	}
	if s.Enable {
		for _, t := range m.cfg.Tasks {
			if t.Enable {
				m.startLocked(t, 0)
			}
		}
	}
	httpx.OK(w, s)
}
