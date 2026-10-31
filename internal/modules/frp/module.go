// Package frp 托管 frpc（内网穿透客户端）与 frps（服务端）：
// 在界面中维护多个 frpc / frps 实例与代理列表，生成配置文件并守护外部 frp 进程。
package frp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wangwanjie/gately/internal/binstall"
	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/fsperm"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/proc"
)

// ID 是模块标识。
const ID = "frp"

// 两种角色的可执行文件名，也是生成的配置文件名前缀与日志来源前缀。
const (
	binClient = "frpc"
	binServer = "frps"
)

// binInfo 缓存一种可执行文件已探测的版本，避免界面轮询时反复执行。
type binInfo struct {
	version     string
	versionPath string
}

// instRuntime 是一个 frpc / frps 实例的运行时信息。
type instRuntime struct {
	sup *proc.Supervisor
	err string
	// content 是最近一次按配置应用的配置文件内容，实例停用时为空；内容不变时不重启进程
	content string
}

// Module 是 frp 模块。
type Module struct {
	mu      sync.Mutex
	store   *config.Store
	logs    *logbuf.Buffer
	cfg     fileConfig
	clients map[string]*instRuntime
	servers map[string]*instRuntime
	bins    map[string]*binInfo
	started bool
	// httpClient 返回下载程序使用的 HTTP 客户端，由框架按“更新代理”设置注入
	httpClient func() *http.Client
	install    *binstall.Installer
}

// New 创建模块。
func New() *Module {
	return &Module{
		clients:    map[string]*instRuntime{},
		servers:    map[string]*instRuntime{},
		bins:       map[string]*binInfo{binClient: {}, binServer: {}},
		httpClient: func() *http.Client { return http.DefaultClient },
	}
}

// SetHTTPClient 实现 module.HTTPClientAware。
func (m *Module) SetHTTPClient(fn func() *http.Client) {
	if fn != nil {
		m.httpClient = fn
	}
}

// ID 实现 module.Module。
func (m *Module) ID() string { return ID }

// Title 实现 module.Module。
func (m *Module) Title() string { return "frp 内网穿透" }

// Init 实现 module.Module。
func (m *Module) Init(env module.Env) error {
	m.store, m.logs = env.Store, env.Logs
	m.install = m.newInstaller()
	if _, err := m.store.Load(ID, &m.cfg); err != nil {
		return err
	}
	if m.cfg.migrateLegacy() {
		// 立即保存迁移结果，避免每次启动都生成新的实例 key；
		// 保存失败（如配置目录只读）不影响其他模块启动，内存中照常使用迁移后的配置，下次启动会重新迁移
		if err := m.saveLocked(); err != nil {
			m.logs.Logger("frp").Warnf("保存迁移后的 frp 配置失败: %v", err)
		} else {
			m.logs.Logger("frp").Infof("已把旧版单个 frpc / frps 配置迁移为实例")
		}
	}
	// 清理不属于任何 frpc 实例的代理（例如恢复了中间状态的配置历史），它们不会写入任何配置
	if n := m.pruneOrphanProxiesLocked(); n > 0 {
		m.logs.Logger("frp").Warnf("已忽略 %d 个所属 frpc 实例不存在的代理", n)
	}
	m.cfg.fillDefaults()
	return nil
}

// pruneOrphanProxiesLocked 删除所属 frpc 实例不存在的代理，返回删除的数量。调用方持有 mu（或处于 Init）。
func (m *Module) pruneOrphanProxiesLocked() int {
	kept := make([]Proxy, 0, len(m.cfg.Proxies))
	for _, p := range m.cfg.Proxies {
		if m.findClient(p.Client) != nil {
			kept = append(kept, p)
		}
	}
	n := len(m.cfg.Proxies) - len(kept)
	if n > 0 {
		m.cfg.Proxies = kept
	}
	return n
}

// saveClientsLocked 保存 frpc 实例列表，并在同一次保存中删除已删除实例的代理；
// 保存失败时恢复代理列表（实例列表由 crud 回滚），避免出现所属实例不存在的代理。调用方持有 mu。
func (m *Module) saveClientsLocked() error {
	old := m.cfg.Proxies
	m.pruneOrphanProxiesLocked()
	if err := m.saveLocked(); err != nil {
		m.cfg.Proxies = old
		return err
	}
	return nil
}

// migrateLegacy 把旧版单实例配置（client / server 字段）迁移为实例列表，返回是否有改动。
// 已配置过的 frpc 迁移为名为 frpc 的实例，原有代理全部归属该实例；frps 只在启用或改过默认设置时迁移。
func (c *fileConfig) migrateLegacy() bool {
	changed := false
	if lc := c.LegacyClient; lc != nil {
		if c.Settings.FrpcPath == "" {
			c.Settings.FrpcPath = strings.TrimSpace(lc.BinPath)
		}
		if len(c.Clients) == 0 && (lc.Enable || lc.ServerAddr != "" || len(c.Proxies) > 0) {
			cl := Client{Key: crud.NewKey(), Name: binClient, Enable: lc.Enable, ClientSettings: lc.ClientSettings}
			c.Clients = append(c.Clients, cl)
			for i := range c.Proxies {
				if c.Proxies[i].Client == "" {
					c.Proxies[i].Client = cl.Key
				}
			}
		}
		c.LegacyClient = nil
		changed = true
	}
	if ls := c.LegacyServer; ls != nil {
		if c.Settings.FrpsPath == "" {
			c.Settings.FrpsPath = strings.TrimSpace(ls.BinPath)
		}
		def := ServerSettings{BindPort: 7000, DashboardAddr: "127.0.0.1"}
		cur := ls.ServerSettings
		if cur.BindPort == 0 {
			cur.BindPort = def.BindPort
		}
		if cur.DashboardAddr == "" {
			cur.DashboardAddr = def.DashboardAddr
		}
		if len(c.Servers) == 0 && (ls.Enable || cur != def) {
			c.Servers = append(c.Servers, Server{Key: crud.NewKey(), Name: binServer, Enable: ls.Enable, ServerSettings: cur})
		}
		c.LegacyServer = nil
		changed = true
	}
	return changed
}

// fillDefaults 补齐未设置的默认值。
func (c *fileConfig) fillDefaults() {
	for i := range c.Clients {
		if c.Clients[i].ServerPort == 0 {
			c.Clients[i].ServerPort = 7000
		}
	}
	for i := range c.Servers {
		s := &c.Servers[i]
		if s.BindPort == 0 {
			s.BindPort = 7000
		}
		if s.DashboardAddr == "" {
			s.DashboardAddr = "127.0.0.1"
		}
	}
}

// Start 实现 module.Module。
func (m *Module) Start(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = true
	m.syncLocked(true)
	m.cleanupConfigFilesLocked()
	return nil
}

// Stop 实现 module.Module。
func (m *Module) Stop() error {
	m.mu.Lock()
	m.started = false
	runs := make([]*instRuntime, 0, len(m.clients)+len(m.servers))
	for _, rt := range m.clients {
		runs = append(runs, rt)
	}
	for _, rt := range m.servers {
		runs = append(runs, rt)
	}
	m.clients, m.servers = map[string]*instRuntime{}, map[string]*instRuntime{}
	m.mu.Unlock()
	for _, rt := range runs {
		rt.sup.Stop()
	}
	return nil
}

func (m *Module) saveLocked() error { return m.store.Save(ID, m.cfg) }

func (m *Module) findClient(key string) *Client {
	for i := range m.cfg.Clients {
		if m.cfg.Clients[i].Key == key {
			return &m.cfg.Clients[i]
		}
	}
	return nil
}

func (m *Module) findServer(key string) *Server {
	for i := range m.cfg.Servers {
		if m.cfg.Servers[i].Key == key {
			return &m.cfg.Servers[i]
		}
	}
	return nil
}

// clientProxies 返回归属 frpc 实例 key 的代理。
func (m *Module) clientProxies(key string) []Proxy {
	var out []Proxy
	for _, p := range m.cfg.Proxies {
		if p.Client == key {
			out = append(out, p)
		}
	}
	return out
}

// logSource 返回实例的日志来源，例如 frpc:家里。
func logSource(bin, name string) string { return bin + ":" + name }

// configFile 返回实例生成的配置文件路径：<配置目录>/frp/<frpc|frps>-<key>.toml。
func (m *Module) configFile(bin, key string) string {
	return filepath.Join(m.store.Dir(), "frp", bin+"-"+key+".toml")
}

// writeConfig 把配置写入 path，文件含令牌，权限 0600。
func (m *Module) writeConfig(path, content string) error {
	if err := fsperm.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := fsperm.WriteFile(tmp, []byte(content), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// binPath 返回角色配置的可执行文件路径。
func (m *Module) binPath(bin string) string {
	if bin == binClient {
		return m.cfg.Settings.FrpcPath
	}
	return m.cfg.Settings.FrpsPath
}

// runInstance 按期望的配置内容启停一个实例；content 为空表示应停止。force 为 false 且内容未变时保持现状。
// 调用方持有 mu。
func (m *Module) runInstance(runs map[string]*instRuntime, bin, key, name, content string, force bool) {
	rt := runs[key]
	if rt == nil {
		rt = &instRuntime{sup: proc.New(m.logs.Logger(logSource(bin, name)))}
		runs[key] = rt
	}
	if !force && rt.content == content {
		return
	}
	rt.sup.Stop()
	rt.err, rt.content = "", content
	if content == "" {
		// 停用的实例不保留含令牌的配置文件
		m.removeConfigFile(bin, key)
		return
	}
	log := m.logs.Logger(logSource(bin, name))
	exe, err := proc.Resolve(m.binPath(bin), m.store.Dir(), bin)
	if err != nil {
		rt.err = err.Error()
		log.Errorf("%v", err)
		return
	}
	path := m.configFile(bin, key)
	if err := m.writeConfig(path, content); err != nil {
		rt.err = "写入配置文件失败: " + err.Error()
		log.Errorf("%s", rt.err)
		return
	}
	// 名称可能被修改，重建托管器以使用新的日志来源
	rt.sup = proc.New(log)
	rt.sup.Start(proc.Spec{Path: exe, Args: []string{"-c", path}})
}

// clientContent 返回 frpc 实例应运行的配置内容，停用或模块未启动时为空。
func (m *Module) clientContent(c *Client) string {
	if !m.started || !c.Enable {
		return ""
	}
	return ClientTOML(c.ClientSettings, m.clientProxies(c.Key))
}

// serverContent 返回 frps 实例应运行的配置内容，停用或模块未启动时为空。
func (m *Module) serverContent(s *Server) string {
	if !m.started || !s.Enable {
		return ""
	}
	return ServerTOML(s.ServerSettings)
}

// applyClientLocked 重启指定 frpc 实例；实例已删除时停止并清理。调用方持有 mu。
func (m *Module) applyClientLocked(key string) {
	c := m.findClient(key)
	if c == nil {
		m.dropLocked(m.clients, binClient, key)
		return
	}
	m.warnDuplicates(c)
	m.runInstance(m.clients, binClient, key, c.Name, m.clientContent(c), true)
}

// applyServerLocked 重启指定 frps 实例；实例已删除时停止并清理。调用方持有 mu。
func (m *Module) applyServerLocked(key string) {
	s := m.findServer(key)
	if s == nil {
		m.dropLocked(m.servers, binServer, key)
		return
	}
	m.runInstance(m.servers, binServer, key, s.Name, m.serverContent(s), true)
}

// syncLocked 让全部实例与配置一致：force 为 true 时全部重启，否则只重启配置内容有变化的实例，
// 并清理已删除实例的进程与配置文件。代理增删改时用它只重启受影响的 frpc。调用方持有 mu。
func (m *Module) syncLocked(force bool) {
	keep := map[string]bool{}
	for i := range m.cfg.Clients {
		c := &m.cfg.Clients[i]
		keep[c.Key] = true
		content := m.clientContent(c)
		if rt := m.clients[c.Key]; force || rt == nil || rt.content != content {
			m.warnDuplicates(c)
		}
		m.runInstance(m.clients, binClient, c.Key, c.Name, content, force)
	}
	for key := range m.clients {
		if !keep[key] {
			m.dropLocked(m.clients, binClient, key)
		}
	}
	keep = map[string]bool{}
	for i := range m.cfg.Servers {
		s := &m.cfg.Servers[i]
		keep[s.Key] = true
		m.runInstance(m.servers, binServer, s.Key, s.Name, m.serverContent(s), force)
	}
	for key := range m.servers {
		if !keep[key] {
			m.dropLocked(m.servers, binServer, key)
		}
	}
}

// warnDuplicates 提示同一 frpc 实例中重名的已启用代理：frpc 不允许重名，后面的同名代理不写入配置。
func (m *Module) warnDuplicates(c *Client) {
	if !m.started || !c.Enable {
		return
	}
	seen := map[string]bool{}
	for _, p := range m.cfg.Proxies {
		if p.Client != c.Key || !p.Enable {
			continue
		}
		if seen[p.Name] {
			m.logs.Logger(logSource(binClient, c.Name)).Warnf("代理名称 %q 重复，已忽略后面的同名代理", p.Name)
		}
		seen[p.Name] = true
	}
}

// dropLocked 停止并移除实例的运行时，删除生成的配置文件；调用方持有 mu。
func (m *Module) dropLocked(runs map[string]*instRuntime, bin, key string) {
	if rt := runs[key]; rt != nil {
		rt.sup.Stop()
		delete(runs, key)
	}
	m.removeConfigFile(bin, key)
}

// removeConfigFile 删除实例生成的配置文件，文件不存在时忽略。
func (m *Module) removeConfigFile(bin, key string) {
	if key == "" || strings.ContainsAny(key, `/\.`) {
		return
	}
	if err := os.Remove(m.configFile(bin, key)); err != nil && !os.IsNotExist(err) {
		m.logs.Logger("frp").Warnf("删除配置文件失败: %v", err)
	}
}

// cleanupConfigFilesLocked 删除 frp/ 目录下不属于当前任何实例的配置文件，包括旧版的 frpc.toml、frps.toml
// 以及恢复配置历史后遗留的旧实例文件，这些文件含明文令牌。调用方持有 mu。
func (m *Module) cleanupConfigFilesLocked() {
	dir := filepath.Join(m.store.Dir(), "frp")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	keep := map[string]bool{}
	for i := range m.cfg.Clients {
		keep[filepath.Base(m.configFile(binClient, m.cfg.Clients[i].Key))] = true
	}
	for i := range m.cfg.Servers {
		keep[filepath.Base(m.configFile(binServer, m.cfg.Servers[i].Key))] = true
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || keep[name] || !(strings.HasSuffix(name, ".toml") || strings.HasSuffix(name, ".toml.tmp")) {
			continue
		}
		if !strings.HasPrefix(name, binClient) && !strings.HasPrefix(name, binServer) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
			m.logs.Logger("frp").Warnf("删除残留配置文件失败: %v", err)
		}
	}
}

// removeClientLocked 在 frpc 实例删除后停止进程；归属它的代理已由 saveClientsLocked 在同一次保存中删除。
// 调用方持有 mu。
func (m *Module) removeClientLocked(key string) {
	m.dropLocked(m.clients, binClient, key)
}

// InstanceState 是实例的运行状态。
type InstanceState struct {
	State proc.State `json:"state"`
	Error string     `json:"error"`
}

// ClientView 是界面展示的 frpc 实例，令牌已脱敏。
type ClientView struct {
	Client
	InstanceState
}

// ServerView 是界面展示的 frps 实例，令牌与控制台密码已脱敏。
type ServerView struct {
	Server
	InstanceState
}

func stateOf(runs map[string]*instRuntime, key string) InstanceState {
	var v InstanceState
	if rt := runs[key]; rt != nil {
		v.State = rt.sup.State()
		v.Error = rt.err
		if v.Error == "" {
			v.Error = v.State.Error
		}
	}
	return v
}

func (m *Module) clientView(c *Client) ClientView {
	v := ClientView{Client: *c, InstanceState: stateOf(m.clients, c.Key)}
	v.Token = crud.Mask(v.Token)
	return v
}

func (m *Module) serverView(s *Server) ServerView {
	v := ServerView{Server: *s, InstanceState: stateOf(m.servers, s.Key)}
	v.Token, v.DashboardPwd = crud.Mask(v.Token), crud.Mask(v.DashboardPwd)
	return v
}

// BinaryView 是一种可执行文件的查找结果。
type BinaryView struct {
	// Binary 为实际使用的可执行文件路径
	Binary  string `json:"binary"`
	Version string `json:"version"`
	Error   string `json:"error"`
}

// SettingsView 是 GET /settings 的返回内容。
type SettingsView struct {
	Settings Settings   `json:"settings"`
	Frpc     BinaryView `json:"frpc"`
	Frps     BinaryView `json:"frps"`
}

// binaryView 查找可执行文件并探测版本；调用方持有 mu。
func (m *Module) binaryView(bin string) BinaryView {
	var v BinaryView
	exe, err := proc.Resolve(m.binPath(bin), m.store.Dir(), bin)
	if err != nil {
		v.Error = err.Error()
		return v
	}
	v.Binary = exe
	info := m.bins[bin]
	if info.versionPath != exe {
		info.versionPath, info.version = exe, proc.Version(exe, "--version")
	}
	v.Version = info.version
	return v
}

// Routes 实现 module.Module。
func (m *Module) Routes(r module.Router) {
	clients := &crud.Collection[Client]{
		Mu:       &m.mu,
		Items:    &m.cfg.Clients,
		Save:     m.saveClientsLocked,
		Key:      func(c *Client) *string { return &c.Key },
		Enabled:  func(c *Client) *bool { return &c.Enable },
		Group:    func(c *Client) *string { return &c.Group },
		Validate: func(c *Client) error { return c.Validate() },
		Merge:    func(old, updated *Client) { crud.KeepSecret(old.Token, &updated.Token) },
		View:     func(c *Client) any { return m.clientView(c) },
		Secrets:  func(c *Client) map[string]string { return map[string]string{"token": c.Token} },
		Changed:  m.applyClientLocked,
		Removed:  m.removeClientLocked,
	}
	clients.Register(r, "/clients")
	servers := &crud.Collection[Server]{
		Mu:       &m.mu,
		Items:    &m.cfg.Servers,
		Save:     m.saveLocked,
		Key:      func(s *Server) *string { return &s.Key },
		Enabled:  func(s *Server) *bool { return &s.Enable },
		Group:    func(s *Server) *string { return &s.Group },
		Validate: func(s *Server) error { return s.Validate() },
		Merge: func(old, updated *Server) {
			crud.KeepSecret(old.Token, &updated.Token)
			crud.KeepSecret(old.DashboardPwd, &updated.DashboardPwd)
		},
		View: func(s *Server) any { return m.serverView(s) },
		Secrets: func(s *Server) map[string]string {
			return map[string]string{"token": s.Token, "dashboardPwd": s.DashboardPwd}
		},
		Changed: m.applyServerLocked,
		Removed: func(key string) { m.dropLocked(m.servers, binServer, key) },
	}
	servers.Register(r, "/servers")
	proxies := &crud.Collection[Proxy]{
		Mu:       &m.mu,
		Items:    &m.cfg.Proxies,
		Save:     m.saveLocked,
		Key:      func(p *Proxy) *string { return &p.Key },
		Enabled:  func(p *Proxy) *bool { return &p.Enable },
		Group:    func(p *Proxy) *string { return &p.Group },
		Validate: func(p *Proxy) error { return p.Validate() },
		Check: func(p *Proxy) error {
			if m.findClient(p.Client) == nil {
				return fmt.Errorf("代理所属的 frpc 实例不存在")
			}
			return nil
		},
		Merge:   func(old, updated *Proxy) { crud.KeepSecret(old.SecretKey, &updated.SecretKey) },
		View:    func(p *Proxy) any { v := *p; v.SecretKey = crud.Mask(v.SecretKey); return v },
		Secrets: func(p *Proxy) map[string]string { return map[string]string{"secretKey": p.SecretKey} },
		// 代理可能换了所属实例，按内容比对只重启受影响的 frpc
		Changed: func(string) { m.syncLocked(false) },
		Removed: func(string) { m.syncLocked(false) },
	}
	proxies.Register(r, "/proxies")
	m.registerImport(r)
	m.install.Register(r)

	restart := func(exists func(string) bool, apply func(string)) http.HandlerFunc {
		return func(w http.ResponseWriter, req *http.Request) {
			m.mu.Lock()
			defer m.mu.Unlock()
			key := req.PathValue("key")
			if !exists(key) {
				httpx.Error(w, http.StatusNotFound, "实例不存在")
				return
			}
			apply(key)
			httpx.OK(w, map[string]bool{"ok": true})
		}
	}
	r.HandleFunc("POST /clients/{key}/restart", restart(func(k string) bool { return m.findClient(k) != nil }, m.applyClientLocked))
	r.HandleFunc("POST /servers/{key}/restart", restart(func(k string) bool { return m.findServer(k) != nil }, m.applyServerLocked))
	module.DocOf(r, "POST /clients/{key}/restart", module.Doc{Summary: "重启 frpc 实例"})
	module.DocOf(r, "POST /servers/{key}/restart", module.Doc{Summary: "重启 frps 实例"})

	r.HandleFunc("GET /settings", func(w http.ResponseWriter, req *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		httpx.OK(w, SettingsView{Settings: m.cfg.Settings, Frpc: m.binaryView(binClient), Frps: m.binaryView(binServer)})
	})
	r.HandleFunc("PUT /settings", func(w http.ResponseWriter, req *http.Request) {
		var s Settings
		if err := httpx.ReadJSON(req, &s); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		s.FrpcPath, s.FrpsPath = strings.TrimSpace(s.FrpcPath), strings.TrimSpace(s.FrpsPath)
		m.mu.Lock()
		defer m.mu.Unlock()
		old := m.cfg.Settings
		m.cfg.Settings = s
		if err := m.saveLocked(); err != nil {
			m.cfg.Settings = old
			httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
			return
		}
		// 程序路径变化后重启使用该程序的实例
		if s.FrpcPath != old.FrpcPath {
			for i := range m.cfg.Clients {
				m.applyClientLocked(m.cfg.Clients[i].Key)
			}
		}
		if s.FrpsPath != old.FrpsPath {
			for i := range m.cfg.Servers {
				m.applyServerLocked(m.cfg.Servers[i].Key)
			}
		}
		httpx.OK(w, map[string]bool{"ok": true})
	})
	module.DocOf(r, "GET /settings", module.Doc{Summary: "读取程序路径与版本", Resp: &SettingsView{}})
	module.DocOf(r, "PUT /settings", module.Doc{Summary: "保存程序路径", Req: &Settings{}})

	r.HandleFunc("POST /reset", func(w http.ResponseWriter, req *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if err := m.resetLocked(); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "%v", err)
			return
		}
		httpx.OK(w, map[string]bool{"ok": true})
	})
}

// Summary 实现 module.Summarizer。
func (m *Module) Summary() []module.SummaryItem {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []module.SummaryItem
	var clientKeys []string
	for _, c := range m.cfg.Clients {
		if c.Enable {
			clientKeys = append(clientKeys, c.Key)
		}
	}
	if len(clientKeys) > 0 {
		out = append(out, rolesSummary("客户端", m.clients, clientKeys))
		enabled, total := 0, 0
		on := map[string]bool{}
		for _, k := range clientKeys {
			on[k] = true
		}
		for _, p := range m.cfg.Proxies {
			if on[p.Client] {
				total++
				if p.Enable {
					enabled++
				}
			}
		}
		out = append(out, module.SummaryItem{Label: "代理", Value: fmt.Sprintf("%d / %d", enabled, total)})
	}
	var serverKeys []string
	for _, s := range m.cfg.Servers {
		if s.Enable {
			serverKeys = append(serverKeys, s.Key)
		}
	}
	if len(serverKeys) > 0 {
		out = append(out, rolesSummary("服务端", m.servers, serverKeys))
	}
	if len(out) == 0 {
		out = append(out, module.SummaryItem{Label: "状态", Value: "未启用", Level: "info"})
	}
	return out
}

// rolesSummary 汇总已启用实例的状态：只有一个实例时显示其状态，多个实例时显示“运行数 / 启用数”。
func rolesSummary(label string, runs map[string]*instRuntime, keys []string) module.SummaryItem {
	running, failed := 0, 0
	for _, k := range keys {
		rt := runs[k]
		if rt == nil {
			continue
		}
		st := rt.sup.State()
		switch {
		case st.Running:
			running++
		case rt.err != "" || st.Error != "":
			failed++
		}
	}
	level := "warn"
	value := "未运行"
	switch {
	case failed > 0:
		level, value = "error", "启动失败"
	case running == len(keys):
		level, value = "ok", "运行中"
	}
	if len(keys) > 1 {
		value = fmt.Sprintf("%d / %d", running, len(keys))
	}
	return module.SummaryItem{Label: label, Value: value, Level: level}
}

// resetLocked 恢复默认配置：停止全部 frpc 与 frps，清空实例、代理、程序路径与安装设置，删除生成的配置文件。
// 旧配置留在配置历史中，可以恢复；安装的程序不受影响。调用方持有 mu。
func (m *Module) resetLocked() error {
	old := m.cfg
	m.cfg = fileConfig{}
	if err := m.saveLocked(); err != nil {
		m.cfg = old
		return fmt.Errorf("保存失败: %v", err)
	}
	// 实例已全部删除，sync 会停止进程
	m.syncLocked(false)
	if err := os.RemoveAll(filepath.Join(m.store.Dir(), "frp")); err != nil {
		return fmt.Errorf("删除生成的配置文件失败: %v", err)
	}
	m.logs.Logger("frp").Infof("已恢复默认配置")
	return nil
}
