// Package protoroute 实现协议分流模块：多个服务共用一个 TCP 端口，按客户端最先发送的数据识别协议
// （TLS 再看 SNI 与 ALPN，HTTP 再看 Host），把整条连接透传给对应的后端。
//
// 文件划分：sniff.go 识别协议，tls.go 解析 TLS ClientHello，router.go 按识别结果选择目标，
// runner.go 管理监听与连接，migrate.go 从旧版端口转发迁移 SNI 分流规则。
// 监听、连接数限制、目标故障转移与 PROXY 协议等四层中转的公共部分在 internal/l4proxy，与端口转发模块共用。
package protoroute

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// ID 是模块标识。
const ID = "protoroute"

// Module 是协议分流模块。
type Module struct {
	mu      sync.Mutex
	store   *config.Store
	logs    *logbuf.Buffer
	cfg     fileConfig
	ctx     context.Context
	runners map[string]*runner
	// errors 记录规则级启动错误（规则 key -> 错误信息）
	errors map[string]string
	// migrated 是本次启动从端口转发迁移来的规则数，Init 时写入日志
	migrated int
}

// New 创建模块实例。
func New() *Module {
	return &Module{runners: map[string]*runner{}, errors: map[string]string{}}
}

// ID 实现 module.Module。
func (m *Module) ID() string { return ID }

// Title 实现 module.Module。
func (m *Module) Title() string { return "协议分流" }

// Migrate 实现 module.Migrator：把旧版端口转发中的 SNI 分流规则搬到本模块。
func (m *Module) Migrate(store *config.Store) (string, error) {
	n, err := migrateFromPortForward(store)
	if err != nil {
		return "", err
	}
	m.migrated = n
	if n == 0 {
		return "", nil
	}
	return portForwardID, nil
}

// Init 实现 module.Module。
func (m *Module) Init(env module.Env) error {
	m.store = env.Store
	m.logs = env.Logs
	m.cfg = fileConfig{Settings: defaultSettings()}
	if _, err := m.store.Load(ID, &m.cfg); err != nil {
		return err
	}
	m.cfg.Settings.normalize()
	if m.migrated > 0 {
		m.logs.Add(logbuf.LevelInfo, "", fmt.Sprintf("已从端口转发迁移 %d 条 SNI 分流规则", m.migrated))
	}
	return nil
}

// Start 实现 module.Module。
func (m *Module) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ctx = ctx
	if !m.cfg.Settings.Enable {
		m.logs.Add(logbuf.LevelInfo, "", "协议分流模块已禁用")
		return nil
	}
	for _, rule := range m.cfg.Rules {
		if rule.Enable {
			m.startRuleLocked(rule)
		}
	}
	return nil
}

// Stop 实现 module.Module。
func (m *Module) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key := range m.runners {
		m.stopRuleLocked(key)
	}
	return nil
}

func (m *Module) startRuleLocked(rule Rule) {
	if m.ctx == nil {
		return
	}
	r := newRunner(rule, m.cfg.Settings, m.logs.Logger(ruleLabel(rule)))
	if err := r.start(m.ctx); err != nil {
		m.errors[rule.Key] = err.Error()
		return
	}
	delete(m.errors, rule.Key)
	if len(r.listenErrors) > 0 {
		m.errors[rule.Key] = fmt.Sprintf("%d 个监听失败", len(r.listenErrors))
	}
	m.runners[rule.Key] = r
}

func (m *Module) stopRuleLocked(key string) {
	if r, ok := m.runners[key]; ok {
		r.stop()
		delete(m.runners, key)
		r.current().log.Infof("已停止")
	}
	delete(m.errors, key)
}

// restartLocked 按当前配置重启一条规则（规则不存在或已禁用时只停止）。
// 监听参数不变时只热替换路由表与默认目标，不断开已有连接：管理界面本身可能正经由这条规则访问，
// 重启会把正在等待响应的保存请求一起断开。
func (m *Module) restartLocked(key string) {
	if r, ok := m.runners[key]; ok && m.cfg.Settings.Enable {
		for _, rule := range m.cfg.Rules {
			if rule.Key == key && rule.Enable && r.canUpdate(rule, m.cfg.Settings) {
				r.update(rule, m.cfg.Settings, m.logs.Logger(ruleLabel(rule)))
				return
			}
		}
	}
	m.stopRuleLocked(key)
	if !m.cfg.Settings.Enable {
		return
	}
	for _, rule := range m.cfg.Rules {
		if rule.Key == key && rule.Enable {
			m.startRuleLocked(rule)
		}
	}
}

func (m *Module) saveLocked() error {
	return m.store.Save(ID, m.cfg)
}

func ruleLabel(r Rule) string {
	if r.Name != "" {
		return r.Name
	}
	return r.ListenPorts
}

// RuleView 是带运行状态的规则，用于 API 输出。
type RuleView struct {
	Rule
	Running bool          `json:"running"`
	Error   string        `json:"error,omitempty"`
	Details []string      `json:"details,omitempty"`
	Stats   StatsSnapshot `json:"stats"`
}

func (m *Module) viewLocked(rule Rule) RuleView {
	v := RuleView{Rule: rule, Error: m.errors[rule.Key], Stats: StatsSnapshot{RouteConns: make([]int64, len(rule.Routes))}}
	if r, ok := m.runners[rule.Key]; ok {
		v.Running = true
		v.Stats = r.snapshot()
		v.Details = r.listenErrors
	}
	return v
}

// Routes 实现 module.Module。
func (m *Module) Routes(r module.Router) {
	rules := &crud.Collection[Rule]{
		Mu:       &m.mu,
		Items:    &m.cfg.Rules,
		Save:     m.saveLocked,
		Key:      func(r *Rule) *string { return &r.Key },
		Enabled:  func(r *Rule) *bool { return &r.Enable },
		Group:    func(r *Rule) *string { return &r.Group },
		Validate: func(r *Rule) error { return r.Validate() },
		View:     func(r *Rule) any { return m.viewLocked(*r) },
		Changed:  m.restartLocked,
		Removed:  m.stopRuleLocked,
	}
	rules.Register(r, "/rules")
	r.HandleFunc("GET /settings", m.handleGetSettings)
	r.HandleFunc("PUT /settings", m.handlePutSettings)
}

func (m *Module) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	httpx.OK(w, m.cfg.Settings)
}

func (m *Module) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var s Settings
	if err := httpx.ReadJSON(r, &s); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
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
	// 全局参数影响所有规则：逐条应用，连接上限变化的规则才重启
	for _, rule := range m.cfg.Rules {
		m.restartLocked(rule.Key)
	}
	httpx.OK(w, s)
}
