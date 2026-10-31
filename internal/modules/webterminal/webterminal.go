// Package webterminal 实现 Web 终端：通过 WebSocket 在浏览器中使用 SSH、Telnet 或本机 Shell，
// 会话由服务端持久保存，浏览器断开后可按设置保留并重新附着。
//
// SSH 主机密钥采用"首次信任"：第一次连接时记录指纹，之后指纹变化会拒绝连接，防止中间人攻击。
// 本机 Shell 等同于服务器的完全控制权，默认关闭，需要在模块设置中显式开启。
package webterminal

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// ID 是模块标识。
const ID = "webterminal"

// 连接类型
const (
	TypeSSH    = "ssh"
	TypeTelnet = "telnet"
	TypeLocal  = "local"
)

// LocalConfig 是本机 Shell 参数。
type LocalConfig struct {
	Shell   string `json:"shell"`
	WorkDir string `json:"workDir"`
	// Env 为附加环境变量
	Env map[string]string `json:"env"`
}

// Connection 是一个终端连接配置。
type Connection struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Group 为列表中的分组名称，只影响管理界面的展示，留空为未分组
	Group  string       `json:"group"`
	Type   string       `json:"type"`
	SSH    SSHConfig    `json:"ssh"`
	Telnet TelnetConfig `json:"telnet"`
	Local  LocalConfig  `json:"local"`
	Remark string       `json:"remark"`
	// Enable 为复用通用接口保留，终端连接始终可用
	Enable bool `json:"enable"`
	// QuickAccessDirs 为 SFTP 快速访问目录
	QuickAccessDirs []string `json:"quickAccessDirs"`
}

// Settings 是模块级配置。
type Settings struct {
	// AllowLocalShell 允许打开本机 Shell
	AllowLocalShell bool `json:"allowLocalShell"`
	MaxSessions     int  `json:"maxSessions"`
	// IdleTimeout 为会话无输入输出自动结束的时间（分钟），0 不限
	IdleTimeout int `json:"idleTimeout"`
	// BufferSize 为每个会话的输出回放缓冲（KB）
	BufferSize int `json:"bufferSize"`
	// HeartbeatInterval 为 WebSocket 心跳间隔（秒）
	HeartbeatInterval int `json:"heartbeatInterval"`
	// SessionKeepAlive 为浏览器断开后会话保留时间（分钟），0 表示断开即结束
	SessionKeepAlive int `json:"sessionKeepAlive"`
}

// 设置默认值与上限
const (
	defaultMaxSessions = 20
	defaultBufferKB    = 256
	maxBufferKB        = 16384
	defaultHeartbeat   = 30
	maxHeartbeat       = 600
	maxKeepAliveMin    = 7 * 24 * 60
)

// normalize 补全默认值并把数值限制在合理范围内。
func (s *Settings) normalize() {
	if s.MaxSessions <= 0 {
		s.MaxSessions = defaultMaxSessions
	}
	if s.MaxSessions > 1000 {
		s.MaxSessions = 1000
	}
	if s.IdleTimeout < 0 {
		s.IdleTimeout = 0
	}
	if s.BufferSize <= 0 {
		s.BufferSize = defaultBufferKB
	}
	if s.BufferSize > maxBufferKB {
		s.BufferSize = maxBufferKB
	}
	if s.HeartbeatInterval <= 0 {
		s.HeartbeatInterval = defaultHeartbeat
	}
	if s.HeartbeatInterval > maxHeartbeat {
		s.HeartbeatInterval = maxHeartbeat
	}
	if s.SessionKeepAlive < 0 {
		s.SessionKeepAlive = 0
	}
	if s.SessionKeepAlive > maxKeepAliveMin {
		s.SessionKeepAlive = maxKeepAliveMin
	}
}

// Shortcut 是全局快捷命令。
type Shortcut struct {
	Name      string `json:"name"`
	Command   string `json:"command"`
	SendEnter bool   `json:"sendEnter"`
}

type fileConfig struct {
	Settings    Settings     `json:"settings"`
	Connections []Connection `json:"connections"`
	Shortcuts   []Shortcut   `json:"shortcuts"`
	// SplitLayout 为前端自定义的分屏布局，原样存取
	SplitLayout json.RawMessage `json:"splitLayout"`
	Security    Security        `json:"security"`
	// Credentials 为 SSH 认证预设
	Credentials []Credential `json:"credentials"`
}

// cleanDirs 去除空白与重复的目录。
func cleanDirs(dirs []string) []string {
	out := make([]string, 0, len(dirs))
	seen := map[string]bool{}
	for _, d := range dirs {
		d = strings.TrimSpace(d)
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	return out
}

// Validate 校验连接配置并补全默认值。
func (c *Connection) Validate() error {
	if err := crud.NormalizeGroup(&c.Group); err != nil {
		return err
	}
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return errors.New("名称不能为空")
	}
	c.QuickAccessDirs = cleanDirs(c.QuickAccessDirs)
	switch c.Type {
	case "", TypeSSH:
		c.Type = TypeSSH
		if err := c.validateSSH(); err != nil {
			return err
		}
	case TypeTelnet:
		t := &c.Telnet
		t.Host = strings.TrimSpace(t.Host)
		if t.Host == "" {
			return errors.New("请填写主机")
		}
		if t.Port == 0 {
			t.Port = 23
		}
		if t.Port < 1 || t.Port > 65535 {
			return errors.New("端口无效")
		}
		t.Timeout = clampTimeout(t.Timeout)
		if err := t.Proxy.validate(); err != nil {
			return err
		}
	case TypeLocal:
		env := map[string]string{}
		for k, v := range c.Local.Env {
			k = strings.TrimSpace(k)
			if k == "" || strings.Contains(k, "=") {
				continue
			}
			env[k] = v
		}
		c.Local.Env = env
	default:
		return fmt.Errorf("未知的连接类型 %q", c.Type)
	}
	c.Enable = true
	return nil
}

// clampTimeout 返回有效的连接超时（秒），默认 15。
func clampTimeout(v int) int {
	if v <= 0 {
		return 15
	}
	return min(v, 120)
}

func (c *Connection) validateSSH() error {
	s := &c.SSH
	s.Host = strings.TrimSpace(s.Host)
	s.Username = strings.TrimSpace(s.Username)
	if s.Host == "" {
		return errors.New("请填写主机")
	}
	// 使用认证预设时用户名可留空，连接时取预设中的默认用户名
	if s.Username == "" && s.AuthType != AuthCredential {
		return errors.New("请填写主机与用户名")
	}
	if s.Port == 0 {
		s.Port = 22
	}
	if s.Port < 1 || s.Port > 65535 {
		return errors.New("端口无效")
	}
	s.Timeout = clampTimeout(s.Timeout)
	s.KeyFile = strings.TrimSpace(s.KeyFile)
	if s.AuthType != AuthCredential {
		s.CredentialKey = ""
	}
	switch s.AuthType {
	case "", "password":
		s.AuthType = "password"
	case AuthCredential:
		if s.CredentialKey == "" {
			return errors.New("请选择认证预设")
		}
		// 凭据由预设统一保存，连接内不再保留单独的密码或私钥
		s.Password, s.PrivateKey, s.KeyFile, s.Passphrase = "", "", "", ""
	case "key":
		if s.PrivateKey == "" && s.KeyFile == "" {
			return errors.New("请填写私钥或私钥文件路径")
		}
		if s.PrivateKey != "" && s.PrivateKey != crud.SecretMask {
			if _, err := parseKey(s.PrivateKey, s.Passphrase); err != nil {
				return fmt.Errorf("私钥无效: %w", err)
			}
		}
	default:
		return fmt.Errorf("未知的认证方式 %q", s.AuthType)
	}
	return s.Proxy.validate()
}

// mergeSecrets 把被脱敏的密文字段恢复为旧值；主机或端口变化时重置已记录的主机密钥。
func mergeSecrets(old, updated *Connection) {
	crud.KeepSecret(old.SSH.Password, &updated.SSH.Password)
	crud.KeepSecret(old.SSH.PrivateKey, &updated.SSH.PrivateKey)
	crud.KeepSecret(old.SSH.Passphrase, &updated.SSH.Passphrase)
	crud.KeepSecret(old.SSH.Proxy.Password, &updated.SSH.Proxy.Password)
	crud.KeepSecret(old.Telnet.Password, &updated.Telnet.Password)
	crud.KeepSecret(old.Telnet.Proxy.Password, &updated.Telnet.Proxy.Password)
	if old.SSH.Host != updated.SSH.Host || old.SSH.Port != updated.SSH.Port {
		updated.SSH.HostKey = ""
	} else if updated.SSH.HostKey == "" {
		updated.SSH.HostKey = old.SSH.HostKey
	}
	// 快速访问目录由专用接口维护，编辑连接时未携带则沿用旧值
	if updated.QuickAccessDirs == nil {
		updated.QuickAccessDirs = old.QuickAccessDirs
	}
}

// Module 是 Web 终端模块。
type Module struct {
	mu    sync.Mutex
	store *config.Store
	logs  *logbuf.Buffer
	cfg   fileConfig

	hub   *hub
	pool  *sshPool
	twofa *twoFAState
}

// New 创建模块。
func New() *Module {
	m := &Module{pool: newSSHPool(), twofa: newTwoFAState()}
	m.hub = newHub(m.settingsSnapshot)
	return m
}

// ID 实现 module.Module。
func (m *Module) ID() string { return ID }

// Title 实现 module.Module。
func (m *Module) Title() string { return "Web 终端" }

// Init 实现 module.Module。旧配置缺失的字段保持默认值。
func (m *Module) Init(env module.Env) error {
	m.store, m.logs = env.Store, env.Logs
	m.cfg.Settings = Settings{MaxSessions: defaultMaxSessions, BufferSize: defaultBufferKB, HeartbeatInterval: defaultHeartbeat}
	_, err := m.store.Load(ID, &m.cfg)
	m.cfg.Settings.normalize()
	if m.cfg.Shortcuts == nil {
		m.cfg.Shortcuts = []Shortcut{}
	}
	if m.cfg.Credentials == nil {
		m.cfg.Credentials = []Credential{}
	}
	return err
}

// Start 实现 module.Module。
func (m *Module) Start(context.Context) error {
	m.hub.ensureJanitor()
	return nil
}

// Stop 实现 module.Module：结束全部会话并关闭连接池。
func (m *Module) Stop() error {
	m.hub.closeAll("服务已停止")
	m.pool.closeAll()
	return nil
}

func (m *Module) saveLocked() error { return m.store.Save(ID, m.cfg) }

// maskProxy 返回代理配置的脱敏副本。
func maskProxy(p Proxy) Proxy {
	p.Password = crud.Mask(p.Password)
	return p
}

func (m *Module) viewLocked(c *Connection) any {
	v := *c
	v.SSH.Password = crud.Mask(c.SSH.Password)
	v.SSH.PrivateKey = crud.Mask(c.SSH.PrivateKey)
	v.SSH.Passphrase = crud.Mask(c.SSH.Passphrase)
	v.SSH.Proxy = maskProxy(c.SSH.Proxy)
	v.Telnet.Password = crud.Mask(c.Telnet.Password)
	v.Telnet.Proxy = maskProxy(c.Telnet.Proxy)
	if v.QuickAccessDirs == nil {
		v.QuickAccessDirs = []string{}
	}
	if v.Local.Env == nil {
		v.Local.Env = map[string]string{}
	}
	return v
}

// connectionSecrets 返回连接中被脱敏字段的明文，键为字段的 JSON 路径，例如 ssh.proxy.password。
// 私钥在界面上没有显示按钮，不经此接口返回。
func connectionSecrets(c *Connection) map[string]string {
	return map[string]string{
		"ssh.password":          c.SSH.Password,
		"ssh.passphrase":        c.SSH.Passphrase,
		"ssh.proxy.password":    c.SSH.Proxy.Password,
		"telnet.password":       c.Telnet.Password,
		"telnet.proxy.password": c.Telnet.Proxy.Password,
	}
}

// Routes 实现 module.Module。
func (m *Module) Routes(r module.Router) {
	conns := &crud.Collection[Connection]{
		Mu:       &m.mu,
		Items:    &m.cfg.Connections,
		Save:     m.saveLocked,
		Key:      func(c *Connection) *string { return &c.Key },
		Enabled:  func(c *Connection) *bool { return &c.Enable },
		Group:    func(c *Connection) *string { return &c.Group },
		Validate: func(c *Connection) error { return c.Validate() },
		Merge:    mergeSecrets,
		View:     m.viewLocked,
		Secrets:  connectionSecrets,
		// 明文密码可直接登录目标主机，与打开终端一样需要先接受协议并通过二次验证
		SecretsGuard: m.requireAccess,
		// 连接配置变更或删除后，丢弃该连接在池中的 SSH 连接
		Changed: func(key string) { m.pool.drop(key) },
		Removed: func(key string) { m.pool.drop(key) },
	}
	conns.Register(r, "/connections")
	r.HandleFunc("DELETE /connections/{key}/hostkey", m.handleResetHostKey)
	r.HandleFunc("POST /connections/{key}/test", m.handleTestSaved)
	r.HandleFunc("POST /connections/test", m.handleTestDraft)
	r.HandleFunc("PUT /connections/{key}/quickaccess", m.handleQuickAccess)
	m.registerCredentials(r)
	m.registerSessionRoutes(r)
	m.registerSecurity(r)
	m.registerSFTP(r)

	r.HandleFunc("GET /settings", func(w http.ResponseWriter, req *http.Request) {
		httpx.OK(w, map[string]any{"settings": m.settingsSnapshot(), "activeSessions": m.hub.count()})
	})
	r.HandleFunc("PUT /settings", m.handlePutSettings)
	r.HandleFunc("GET /shells", func(w http.ResponseWriter, req *http.Request) {
		httpx.OK(w, listShells())
	})
	r.HandleFunc("GET /globalshortcuts", func(w http.ResponseWriter, req *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		httpx.OK(w, m.cfg.Shortcuts)
	})
	r.HandleFunc("PUT /globalshortcuts", m.handlePutShortcuts)
	r.HandleFunc("GET /splitlayout", func(w http.ResponseWriter, req *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		layout := m.cfg.SplitLayout
		if len(layout) == 0 {
			layout = json.RawMessage("null")
		}
		httpx.OK(w, layout)
	})
	r.HandleFunc("PUT /splitlayout", func(w http.ResponseWriter, req *http.Request) {
		var layout json.RawMessage
		if err := httpx.ReadJSON(req, &layout); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		m.setSplitLayout(w, layout)
	})
	r.HandleFunc("DELETE /splitlayout", func(w http.ResponseWriter, req *http.Request) {
		m.setSplitLayout(w, nil)
	})
}

func (m *Module) setSplitLayout(w http.ResponseWriter, layout json.RawMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	old := m.cfg.SplitLayout
	m.cfg.SplitLayout = layout
	if err := m.saveLocked(); err != nil {
		m.cfg.SplitLayout = old
		httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
		return
	}
	httpx.OK(w, map[string]bool{"ok": true})
}

func (m *Module) handlePutSettings(w http.ResponseWriter, req *http.Request) {
	var s Settings
	if err := httpx.ReadJSON(req, &s); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	s.normalize()
	// 新开启本机 Shell 属于高风险操作，需通过协议与二次验证门禁
	if s.AllowLocalShell && !m.settingsSnapshot().AllowLocalShell {
		if !m.requireAccess(w, req) {
			return
		}
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
	m.logs.Add(logbuf.LevelInfo, "", fmt.Sprintf("设置已修改：本机 Shell=%v", s.AllowLocalShell))
	httpx.OK(w, s)
}

func (m *Module) handlePutShortcuts(w http.ResponseWriter, req *http.Request) {
	var list []Shortcut
	if err := httpx.ReadJSON(req, &list); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if len(list) > 500 {
		httpx.Error(w, http.StatusBadRequest, "快捷命令最多 500 条")
		return
	}
	out := make([]Shortcut, 0, len(list))
	for _, sc := range list {
		sc.Name = strings.TrimSpace(sc.Name)
		if sc.Name == "" || sc.Command == "" {
			httpx.Error(w, http.StatusBadRequest, "快捷命令的名称与命令不能为空")
			return
		}
		out = append(out, sc)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	old := m.cfg.Shortcuts
	m.cfg.Shortcuts = out
	if err := m.saveLocked(); err != nil {
		m.cfg.Shortcuts = old
		httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
		return
	}
	httpx.OK(w, out)
}

func (m *Module) handleQuickAccess(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Dirs []string `json:"dirs"`
	}
	if err := httpx.ReadJSON(req, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	dirs := cleanDirs(body.Dirs)
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.cfg.Connections {
		if m.cfg.Connections[i].Key != req.PathValue("key") {
			continue
		}
		old := m.cfg.Connections[i].QuickAccessDirs
		m.cfg.Connections[i].QuickAccessDirs = dirs
		if err := m.saveLocked(); err != nil {
			m.cfg.Connections[i].QuickAccessDirs = old
			httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
			return
		}
		httpx.OK(w, map[string]any{"ok": true, "dirs": dirs})
		return
	}
	httpx.Error(w, http.StatusNotFound, "连接不存在")
}

func (m *Module) handleResetHostKey(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.cfg.Connections {
		if m.cfg.Connections[i].Key == r.PathValue("key") {
			m.cfg.Connections[i].SSH.HostKey = ""
			if err := m.saveLocked(); err != nil {
				httpx.Error(w, http.StatusInternalServerError, "%v", err)
				return
			}
			m.pool.drop(m.cfg.Connections[i].Key)
			httpx.OK(w, map[string]bool{"ok": true})
			return
		}
	}
	httpx.Error(w, http.StatusNotFound, "连接不存在")
}

// listShells 读取 /etc/shells，仅保留存在的可执行文件；读取失败时退化为 $SHELL 与 /bin/sh。
func listShells() []string {
	out := []string{}
	if runtime.GOOS == "windows" {
		return out
	}
	seen := map[string]bool{}
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		if st, err := os.Stat(p); err != nil || st.IsDir() || st.Mode()&0o111 == 0 {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	if f, err := os.Open("/etc/shells"); err == nil {
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			add(line)
		}
	}
	if len(out) == 0 {
		add(os.Getenv("SHELL"))
		add("/bin/sh")
	}
	return out
}
