// Package stun 实现 STUN 内网穿透模块：在 NAT1（全锥形）网络下获取公网映射端口，
// 并通过保活与内置转发把外部访问引到内网服务。
package stun

import (
	"context"
	"net/http"
	"reflect"
	"sync"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/iplist"
	"github.com/wangwanjie/gately/internal/ipsource"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// ID 是模块标识。
const ID = "stun"

// Module 是 STUN 穿透模块。
type Module struct {
	mu      sync.Mutex
	store   *config.Store
	logs    *logbuf.Buffer
	cfg     fileConfig
	ctx     context.Context
	runners map[string]*runner
}

// New 创建模块实例。
func New() *Module {
	return &Module{runners: map[string]*runner{}}
}

// ID 实现 module.Module。
func (m *Module) ID() string { return ID }

// Title 实现 module.Module。
func (m *Module) Title() string { return "STUN 穿透" }

// Init 实现 module.Module。
func (m *Module) Init(env module.Env) error {
	m.store = env.Store
	m.logs = env.Logs
	m.cfg = fileConfig{Settings: defaultSettings()}
	if _, err := m.store.Load(ID, &m.cfg); err != nil {
		return err
	}
	m.cfg.Settings.normalize()
	// 模块启动前先登记一次，Web 服务的 IP 列表页也能看到引用
	m.publishIPSetUsersLocked()
	return nil
}

// Start 实现 module.Module。
func (m *Module) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ctx = ctx
	if !m.cfg.Settings.Enable {
		m.logs.Add(logbuf.LevelInfo, "", "STUN 穿透模块已禁用")
		return nil
	}
	for _, r := range m.cfg.Rules {
		if r.Enable {
			m.startLocked(r)
		}
	}
	return nil
}

// Stop 实现 module.Module。
func (m *Module) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key := range m.runners {
		m.stopLocked(key)
	}
	return nil
}

func (m *Module) startLocked(rule Rule) {
	if m.ctx == nil {
		return
	}
	m.stopLocked(rule.Key)
	r := newRunner(rule, m.cfg.Settings, m.logs.Logger(rule.Name))
	r.start(m.ctx)
	m.runners[rule.Key] = r
}

func (m *Module) stopLocked(key string) {
	if r, ok := m.runners[key]; ok {
		r.stop()
		delete(m.runners, key)
	}
}

// applyLocked 让一条规则与配置一致：只修改了来源 IP 访问控制（或没有变化）时原地替换名单，
// 不重新打洞，公网端口与已建立的转发连接保持不变；其它修改重启规则。
func (m *Module) applyLocked(key string) {
	if run, ok := m.runners[key]; ok && m.cfg.Settings.Enable {
		for _, r := range m.cfg.Rules {
			if r.Key == key && r.Enable && onlyAccessChanged(run.rule, r) {
				run.setAccess(r.Access)
				return
			}
		}
	}
	m.restartLocked(key)
}

// restartLocked 按当前配置重启一条规则（规则不存在或已禁用时只停止）。
func (m *Module) restartLocked(key string) {
	m.stopLocked(key)
	if !m.cfg.Settings.Enable {
		return
	}
	for _, r := range m.cfg.Rules {
		if r.Key == key && r.Enable {
			m.startLocked(r)
		}
	}
}

// onlyAccessChanged 判断两条规则是否只有来源 IP 访问控制不同。
func onlyAccessChanged(a, b Rule) bool {
	a.Access, b.Access = iplist.Access{}, iplist.Access{}
	return reflect.DeepEqual(a, b)
}

// publishIPSetUsersLocked 向 IP 列表注册表登记各规则对订阅列表的引用，Web 服务据此展示引用方并阻止删除仍被引用的列表。
// 调用方持有 mu（或处于 Init）。
func (m *Module) publishIPSetUsersLocked() {
	iplist.PublishUsers(ID, m.cfg.Rules, func(r *Rule) (*iplist.Access, string) {
		return &r.Access, ID + " / " + r.Name
	})
}

func (m *Module) saveLocked() error {
	return m.store.Save(ID, m.cfg)
}

// RuleView 是带运行状态的规则。
type RuleView struct {
	Rule
	State RuleState `json:"state"`
}

func (m *Module) viewLocked(r *Rule) any {
	v := RuleView{Rule: *r, State: RuleState{Status: StatusStopped}}
	if run, ok := m.runners[r.Key]; ok {
		v.State = run.snapshot()
	}
	return v
}

// Routes 实现 module.Module。
func (m *Module) Routes(r module.Router) {
	rules := &crud.Collection[Rule]{
		Mu:      &m.mu,
		Items:   &m.cfg.Rules,
		Save:    m.saveLocked,
		Key:     func(r *Rule) *string { return &r.Key },
		Enabled: func(r *Rule) *bool { return &r.Enable },
		Group:   func(r *Rule) *string { return &r.Group },
		Validate: func(r *Rule) error {
			return r.Validate()
		},
		Check: func(r *Rule) error { return r.Access.CheckSets() },
		View:  m.viewLocked,
		Changed: func(key string) {
			m.publishIPSetUsersLocked()
			m.applyLocked(key)
		},
		Removed: func(key string) {
			m.publishIPSetUsersLocked()
			m.stopLocked(key)
		},
	}
	rules.Register(r, "/rules")
	r.HandleFunc("POST /rules/{key}/restart", func(w http.ResponseWriter, req *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.restartLocked(req.PathValue("key"))
		httpx.OK(w, map[string]bool{"ok": true})
	})
	r.HandleFunc("GET /netinterfaces", func(w http.ResponseWriter, req *http.Request) {
		list, err := ipsource.ListInterfaces()
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "%v", err)
			return
		}
		httpx.OK(w, list)
	})
	r.HandleFunc("GET /settings", func(w http.ResponseWriter, req *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		httpx.OK(w, m.cfg.Settings)
	})
	r.HandleFunc("PUT /settings", m.handlePutSettings)
}

func (m *Module) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var s Settings
	if err := httpx.ReadJSON(r, &s); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if err := s.Webhook.Validate(); err != nil {
		httpx.Error(w, http.StatusBadRequest, "全局 Webhook: %v", err)
		return
	}
	s.normalize()
	m.mu.Lock()
	defer m.mu.Unlock()
	old := m.cfg.Settings
	m.cfg.Settings = s
	if err := m.saveLocked(); err != nil {
		m.cfg.Settings = old
		httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
		return
	}
	// 服务器列表等全局参数会影响所有规则，整体重启
	for key := range m.runners {
		m.stopLocked(key)
	}
	if s.Enable {
		for _, rule := range m.cfg.Rules {
			if rule.Enable {
				m.startLocked(rule)
			}
		}
	}
	httpx.OK(w, s)
}
