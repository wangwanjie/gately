// Package portforward 实现 TCP/UDP 端口转发模块。
package portforward

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/fwopen"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/iplist"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// ID 是模块标识。
const ID = "portforward"

// Module 是端口转发模块。
type Module struct {
	mu      sync.Mutex
	store   *config.Store
	logs    *logbuf.Buffer
	cfg     fileConfig
	ctx     context.Context
	runners map[string]*runner
	// errors 记录规则级启动错误（规则 key -> 错误信息）
	errors map[string]string
	// fw 管理各规则的自动放行防火墙，key 为规则 key
	fw *fwopen.Group
}

// New 创建模块实例。
func New() *Module {
	return &Module{runners: map[string]*runner{}, errors: map[string]string{}, fw: fwopen.NewGroup("pf")}
}

// ID 实现 module.Module。
func (m *Module) ID() string { return ID }

// Title 实现 module.Module。
func (m *Module) Title() string { return "端口转发" }

// Init 实现 module.Module。
func (m *Module) Init(env module.Env) error {
	m.store = env.Store
	m.logs = env.Logs
	// 自动放行防火墙按配置目录区分实例，并清理上次运行遗留的放行规则
	fwopen.Setup(m.store.Dir())
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
		m.logs.Add(logbuf.LevelInfo, "", "端口转发模块已禁用")
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
	// 同步撤销自动放行，进程退出前关闭端口
	m.fw.ClearAll()
	return nil
}

func (m *Module) startRuleLocked(rule Rule) {
	if m.ctx == nil {
		return
	}
	// 分流规则已由「协议分流」模块在启动前迁走；仍残留时（例如迁移失败）不能按普通转发启动
	if rule.Mode != "" {
		m.errors[rule.Key] = "SNI 与协议分流规则已移到「协议分流」模块"
		return
	}
	r := newRunner(rule, m.cfg.Settings, m.logs.Logger(ruleLabel(rule)))
	if err := r.start(m.ctx); err != nil {
		m.errors[rule.Key] = err.Error()
		m.fw.Want(rule.Key, nil, nil)
		return
	}
	delete(m.errors, rule.Key)
	if len(r.listenErrors) > 0 {
		m.errors[rule.Key] = fmt.Sprintf("%d 个监听失败", len(r.listenErrors))
	}
	m.runners[rule.Key] = r
	m.syncFirewallLocked(rule, r)
}

// syncFirewallLocked 按规则当前的开关登记自动放行，只放行 r 中监听成功的端口。
func (m *Module) syncFirewallLocked(rule Rule, r *runner) {
	if !rule.AutoFirewall {
		m.fw.Want(rule.Key, nil, nil)
		return
	}
	m.fw.Want(rule.Key, r.firewallPorts(), r.current().log)
}

func (m *Module) stopRuleLocked(key string) {
	if r, ok := m.runners[key]; ok {
		r.stop()
		delete(m.runners, key)
		r.current().log.Infof("已停止")
	}
	delete(m.errors, key)
	m.fw.Want(key, nil, nil)
}

// restartLocked 按当前配置重启一条规则（规则不存在或已禁用时只停止）。
// 监听参数不变时只热替换目标与路由表，不断开已有连接：管理界面本身可能正经由这条规则访问，
// 重启会把正在等待响应的保存请求一起断开。
func (m *Module) restartLocked(key string) {
	if r, ok := m.runners[key]; ok && m.cfg.Settings.Enable {
		for _, rule := range m.cfg.Rules {
			if rule.Key == key && rule.Enable && r.canUpdate(rule, m.cfg.Settings) {
				r.update(rule, m.cfg.Settings, m.logs.Logger(ruleLabel(rule)))
				// 自动放行开关不影响监听，在热更新时同步
				m.syncFirewallLocked(rule, r)
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

// publishIPSetUsersLocked 向 IP 列表注册表登记各规则对订阅列表的引用，Web 服务据此展示引用方并阻止删除仍被引用的列表。
// 调用方持有 mu（或处于 Init）。
func (m *Module) publishIPSetUsersLocked() {
	iplist.PublishUsers(ID, m.cfg.Rules, func(r *Rule) (*iplist.Access, string) {
		return &r.Access, ID + " / " + ruleLabel(*r)
	})
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
	// Firewall 为自动放行防火墙的结果说明，未开启或规则未运行时为空
	Firewall string `json:"firewall,omitempty"`
	// FirewallFailed 表示自动放行有端口放行失败（例如没有权限），界面据此提示
	FirewallFailed bool `json:"firewallFailed,omitempty"`
}

func (m *Module) viewLocked(rule Rule) RuleView {
	v := RuleView{Rule: rule, Error: m.errors[rule.Key]}
	if r, ok := m.runners[rule.Key]; ok {
		v.Running = true
		v.Stats = r.snapshot()
		v.Details = r.listenErrors
		if rule.AutoFirewall {
			v.Firewall = m.fw.Status(rule.Key)
			v.FirewallFailed = m.fw.Failed(rule.Key)
			if v.Firewall == "" && fwopen.IsLoopback(rule.ListenAddress) {
				v.Firewall = fwopen.LoopbackNote
			}
		}
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
		Check:    func(r *Rule) error { return r.Access.CheckSets() },
		View:     func(r *Rule) any { return m.viewLocked(*r) },
		Changed: func(key string) {
			m.publishIPSetUsersLocked()
			m.restartLocked(key)
		},
		Removed: func(key string) {
			m.publishIPSetUsersLocked()
			m.stopRuleLocked(key)
		},
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
	// 全局参数影响所有规则：逐条应用，连接上限等监听参数变化的规则才重启
	for _, rule := range m.cfg.Rules {
		m.restartLocked(rule.Key)
	}
	httpx.OK(w, s)
}

// ImportRules 追加导入规则（由 Lucky 导入器调用），返回导入数量。
func (m *Module) ImportRules(rules []Rule) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	old := m.cfg.Rules
	added := 0
	for _, rule := range rules {
		if err := rule.Validate(); err != nil {
			continue
		}
		rule.Key = crud.NewKey()
		m.cfg.Rules = append(m.cfg.Rules, rule)
		added++
	}
	if err := m.saveLocked(); err != nil {
		m.cfg.Rules = old
		return 0, err
	}
	m.publishIPSetUsersLocked()
	for _, rule := range m.cfg.Rules[len(old):] {
		m.restartLocked(rule.Key)
	}
	return added, nil
}
