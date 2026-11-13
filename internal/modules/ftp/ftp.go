// Package ftp 实现 FTP 文件服务：多用户、每个用户映射到一个或多个目录，可设为只读，支持 FTPS 与被动端口范围。
package ftp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	ftpserver "github.com/fclairamb/ftpserverlib"
	"github.com/spf13/afero"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/fileusers"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/mountfs"
	"github.com/wangwanjie/gately/internal/openports"
)

// ID 是模块标识。
const ID = "ftp"

// TLS 模式，取值与导入配置中的 TLSRequired 对应。
const (
	TLSOptional  = 0 // 明文与 FTPS 均可
	TLSMandatory = 1 // 必须先 AUTH TLS（显式 FTPS）
	TLSImplicit  = 2 // 隐式 FTPS，连接即加密
)

// User 是一个 FTP 账号。
type User struct {
	Name     string `json:"name"`
	Password string `json:"password"`
	// Root 为该用户可访问的目录（绝对路径），ReadOnly 作用于该目录；Mounts 非空时二者不再使用
	Root     string `json:"root"`
	ReadOnly bool   `json:"readOnly"`
	// Mounts 为多个目录：登录后根目录按名称列出各目录，每个目录可单独设为只读
	Mounts []mountfs.Mount `json:"mounts,omitempty"`
}

// Settings 是模块配置。
type Settings struct {
	Enable   bool   `json:"enable"`
	ListenIP string `json:"listenIp"`
	Port     int    `json:"port"`
	// PassiveStart/PassiveEnd 为被动模式数据端口范围，均为 0 时由系统随机分配
	PassiveStart int `json:"passiveStart"`
	PassiveEnd   int `json:"passiveEnd"`
	// PublicHost 为被动模式对外通告的 IP（位于 NAT 之后时填写公网或转发后的 IP），留空自动取连接地址
	PublicHost string `json:"publicHost"`
	// TLSMode 取 TLSOptional / TLSMandatory / TLSImplicit，非 0 时证书由证书模块按 SNI 提供
	TLSMode int `json:"tlsMode"`
	// IdleTimeout 控制连接空闲多少秒后断开，ConnectionTimeout 为建立数据连接的超时秒数
	IdleTimeout       int    `json:"idleTimeout"`
	ConnectionTimeout int    `json:"connectionTimeout"`
	DisableActiveMode bool   `json:"disableActiveMode"`
	Users             []User `json:"users"`
	// SharedUsers 为 true 时文件服务共享用户（见 fileusers 包）也可登录，与本地用户同名时只按本地用户校验
	SharedUsers bool `json:"sharedUsers"`
}

// CertSource 提供 FTPS 证书。
type CertSource interface {
	GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error)
}

// Validate 校验并规范化配置。
func (s *Settings) Validate() error {
	if s.Port == 0 {
		s.Port = 2121
	}
	if s.Port < 1 || s.Port > 65535 {
		return errors.New("端口无效")
	}
	s.ListenIP = strings.Trim(strings.TrimSpace(s.ListenIP), "[]")
	if s.ListenIP != "" && net.ParseIP(s.ListenIP) == nil {
		return errors.New("监听地址无效")
	}
	s.PublicHost = strings.TrimSpace(s.PublicHost)
	if s.PublicHost != "" && net.ParseIP(s.PublicHost) == nil {
		return errors.New("被动模式对外 IP 无效，仅支持 IP 地址")
	}
	if s.PassiveStart != 0 || s.PassiveEnd != 0 {
		if s.PassiveStart < 1 || s.PassiveEnd > 65535 || s.PassiveStart > s.PassiveEnd {
			return errors.New("被动端口范围无效")
		}
	}
	if s.TLSMode < TLSOptional || s.TLSMode > TLSImplicit {
		return errors.New("TLS 模式无效")
	}
	if s.IdleTimeout < 0 || s.ConnectionTimeout < 0 {
		return errors.New("超时时间不能为负数")
	}
	if s.IdleTimeout == 0 {
		s.IdleTimeout = 900
	}
	if s.ConnectionTimeout == 0 {
		s.ConnectionTimeout = 30
	}
	seen := map[string]bool{}
	for i := range s.Users {
		u := &s.Users[i]
		u.Name = strings.TrimSpace(u.Name)
		if u.Name == "" || u.Password == "" {
			return errors.New("用户名和密码不能为空")
		}
		if seen[u.Name] {
			return fmt.Errorf("用户 %s 重复", u.Name)
		}
		seen[u.Name] = true
		if len(u.Mounts) > 0 {
			// 多目录模式下根目录为虚拟目录，单目录字段清空，避免两套配置并存
			u.Root, u.ReadOnly = "", false
			if err := mountfs.Validate(u.Mounts); err != nil {
				return fmt.Errorf("用户 %s: %w", u.Name, err)
			}
			continue
		}
		if !filepath.IsAbs(u.Root) {
			return fmt.Errorf("用户 %s 的目录需为绝对路径", u.Name)
		}
	}
	if s.Enable && len(s.Users) == 0 && (!s.SharedUsers || len(fileusers.Default.List()) == 0) {
		return errors.New("启用 FTP 时至少添加一个用户")
	}
	return nil
}

// driver 实现 ftpserver.MainDriver，每次启动持有一份配置快照。
type driver struct {
	s     Settings
	users map[string]User
	certs CertSource
	logs  *logbuf.Buffer
	// throttle 按来源地址限制密码校验失败的次数，防止暴力猜测密码
	throttle fileusers.Throttle

	// clients 为已连接的客户端。ftpserverlib 的 Stop 只关闭监听，已登录的会话会继续使用旧的目录与权限，
	// 因此停止或重启服务时由 closeClients 逐个断开
	clientsMu sync.Mutex
	clients   map[uint32]ftpserver.ClientContext
}

// closeClients 断开全部已连接的客户端。
func (d *driver) closeClients() {
	d.clientsMu.Lock()
	list := make([]ftpserver.ClientContext, 0, len(d.clients))
	for _, cc := range d.clients {
		list = append(list, cc)
	}
	d.clients = map[uint32]ftpserver.ClientContext{}
	d.clientsMu.Unlock()
	for _, cc := range list {
		_ = cc.Close()
	}
}

func (d *driver) GetSettings() (*ftpserver.Settings, error) {
	st := &ftpserver.Settings{
		ListenAddr:        net.JoinHostPort(d.s.ListenIP, strconv.Itoa(d.s.Port)),
		PublicHost:        d.s.PublicHost,
		Banner:            "Gately FTP",
		IdleTimeout:       d.s.IdleTimeout,
		ConnectionTimeout: d.s.ConnectionTimeout,
		DisableActiveMode: d.s.DisableActiveMode,
		TLSRequired:       ftpserver.TLSRequirement(d.s.TLSMode),
	}
	if d.s.PassiveStart != 0 {
		st.PassiveTransferPortRange = &ftpserver.PortRange{Start: d.s.PassiveStart, End: d.s.PassiveEnd}
	}
	return st, nil
}

// ClientConnected 拒绝被封禁地址的连接。
func (d *driver) ClientConnected(cc ftpserver.ClientContext) (string, error) {
	if banlist.Default.BannedAddr(cc.RemoteAddr()) {
		return "", errors.New("Access denied")
	}
	d.clientsMu.Lock()
	if d.clients == nil {
		d.clients = map[uint32]ftpserver.ClientContext{}
	}
	d.clients[cc.ID()] = cc
	d.clientsMu.Unlock()
	return "Gately FTP", nil
}

func (d *driver) ClientDisconnected(cc ftpserver.ClientContext) {
	d.clientsMu.Lock()
	delete(d.clients, cc.ID())
	d.clientsMu.Unlock()
}

func (d *driver) AuthUser(cc ftpserver.ClientContext, user, pass string) (ftpserver.ClientDriver, error) {
	ip := remoteIP(cc.RemoteAddr())
	if !d.throttle.Reserve(ip) {
		return nil, errors.New("登录失败次数过多，请稍后再试")
	}
	fileUser, err := d.authUser(cc, user, pass)
	d.throttle.Done(ip, err == nil)
	return fileUser, err
}

// remoteIP 返回连接对端的 IP。
func remoteIP(a net.Addr) string {
	if ta, ok := a.(*net.TCPAddr); ok {
		return ta.IP.String()
	}
	if a == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(a.String())
	if err != nil {
		return a.String()
	}
	return host
}

func (d *driver) authUser(cc ftpserver.ClientContext, user, pass string) (ftpserver.ClientDriver, error) {
	u, ok := d.users[user]
	shared := false
	// 没有同名本地用户且开启了共享用户时按共享用户校验；共享用户在登录时读取，修改后无需重启服务
	if !ok && d.s.SharedUsers {
		var su fileusers.User
		if su, ok = fileusers.Default.Lookup(user); ok {
			u, shared = User(su), true
		}
	}
	if !ok || !fileusers.CheckPassword(u.Password, pass) {
		if shared {
			d.logs.Add(logbuf.LevelWarn, "", fmt.Sprintf("%s 认证失败（共享用户 %q）", cc.RemoteAddr(), fileusers.ClipName(user)))
		} else {
			d.logs.Add(logbuf.LevelWarn, "", fmt.Sprintf("%s 认证失败（用户 %q）", cc.RemoteAddr(), fileusers.ClipName(user)))
		}
		return nil, errors.New("用户名或密码错误")
	}
	var fs afero.Fs
	if len(u.Mounts) > 0 {
		fs = mountfs.Afero(u.Mounts)
	} else {
		fs = afero.NewBasePathFs(afero.NewOsFs(), u.Root)
		if u.ReadOnly {
			fs = afero.NewReadOnlyFs(fs)
		}
	}
	if shared {
		d.logs.Add(logbuf.LevelInfo, "", fmt.Sprintf("%s 登录（共享用户 %q）", cc.RemoteAddr(), user))
	} else {
		d.logs.Add(logbuf.LevelInfo, "", fmt.Sprintf("%s 登录（用户 %q）", cc.RemoteAddr(), user))
	}
	return fs, nil
}

func (d *driver) GetTLSConfig() (*tls.Config, error) {
	if d.certs == nil {
		return nil, errors.New("证书模块不可用")
	}
	return &tls.Config{GetCertificate: d.certs.GetCertificate, MinVersion: tls.VersionTLS12}, nil
}

// Module 是 FTP 模块。
type Module struct {
	mu       sync.Mutex
	store    *config.Store
	logs     *logbuf.Buffer
	certs    CertSource
	settings Settings
	srv      *ftpserver.FtpServer
	drv      *driver
	// releaseListen 撤销控制端口在 openports 中的登记
	releaseListen func()
	err           string
}

// New 创建模块。
func New(certs CertSource) *Module { return &Module{certs: certs} }

// ID 实现 module.Module。
func (m *Module) ID() string { return ID }

// Title 实现 module.Module。
func (m *Module) Title() string { return "FTP 服务" }

// Init 实现 module.Module。
func (m *Module) Init(env module.Env) error {
	m.store, m.logs = env.Store, env.Logs
	m.settings.Port = 2121
	if _, err := m.store.Load(ID, &m.settings); err != nil {
		return err
	}
	if err := fileusers.Default.Init(env.Store); err != nil {
		return err
	}
	// 共享用户修改后重启服务，断开已登录的连接，避免被删除或改了密码、目录的共享用户继续使用旧会话
	fileusers.Default.Subscribe(func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.settings.Enable && m.settings.SharedUsers && m.srv != nil {
			m.restartLocked()
		}
	})
	// 共享用户页面展示本服务的启用状态、开关与本地用户
	fileusers.Default.AddService(ID, func() fileusers.Service {
		m.mu.Lock()
		defer m.mu.Unlock()
		sv := fileusers.Service{Enable: m.settings.Enable, SharedUsers: m.settings.SharedUsers}
		for _, u := range m.settings.Users {
			sv.LocalUsers = append(sv.LocalUsers, u.Name)
		}
		return sv
	})
	// 只靠共享用户启用时，不允许把共享用户删光
	fileusers.Default.AddCheck(m.ID(), func(users []fileusers.User) error {
		m.mu.Lock()
		s := m.settings
		m.mu.Unlock()
		if s.Enable && len(s.Users) == 0 && s.SharedUsers && len(users) == 0 {
			return errors.New("FTP 已启用且只使用共享用户，至少保留一个共享用户")
		}
		return nil
	})
	if err := fileusers.Default.LoadErr(); err != nil {
		m.logs.Add(logbuf.LevelWarn, "", "共享用户配置无效，共享用户暂不能登录: "+err.Error())
	}
	return nil
}

// Start 实现 module.Module。
func (m *Module) Start(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restartLocked()
	return nil
}

// Stop 实现 module.Module。
func (m *Module) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
	return nil
}

func (m *Module) stopLocked() {
	openports.Clear(ID)
	if m.releaseListen != nil {
		m.releaseListen()
		m.releaseListen = nil
	}
	if m.srv != nil {
		_ = m.srv.Stop()
		m.srv = nil
	}
	if m.drv != nil {
		m.drv.closeClients()
		m.drv = nil
	}
}

func (m *Module) restartLocked() {
	m.stopLocked()
	m.err = ""
	if !m.settings.Enable {
		return
	}
	if m.settings.TLSMode != TLSOptional && m.certs == nil {
		m.err = "证书模块不可用"
		return
	}
	d := &driver{s: m.settings, users: map[string]User{}, certs: m.certs, logs: m.logs}
	for _, u := range m.settings.Users {
		d.users[u.Name] = u
	}
	srv := ftpserver.NewFtpServer(d)
	if err := srv.Listen(); err != nil {
		m.err = err.Error()
		m.logs.Add(logbuf.LevelError, "", "监听失败: "+err.Error())
		return
	}
	m.srv, m.drv = srv, d
	// 控制端口监听成功后立即视为开放端口，客户端随即连接时不会被扫描检测当成未开放端口
	if addr, err := net.ResolveTCPAddr("tcp", srv.Addr()); err == nil {
		m.releaseListen = openports.Hold(addr)
	}
	// 被动模式的数据端口只在传输时短暂监听，登记给入侵防护，避免 IPv6 严格模式把数据连接当成端口扫描
	if m.settings.PassiveStart != 0 {
		openports.Set(ID, openports.Entry{Ranges: []openports.Range{{Lo: m.settings.PassiveStart, Hi: m.settings.PassiveEnd}}})
	} else {
		openports.Set(ID, openports.Entry{Dynamic: true})
	}
	go func() {
		// Serve 在 Stop 后返回，其他错误写入运行日志
		if err := srv.Serve(); err != nil {
			m.logs.Add(logbuf.LevelWarn, "", "FTP 服务退出: "+err.Error())
		}
	}()
	m.logListenLocked(srv.Addr())
}

// sharedCountLocked 返回开启共享用户时可以登录的共享用户数（与本地用户同名的按本地用户，用户名区分大小写），未开启时为 0。
func (m *Module) sharedCountLocked() int {
	if !m.settings.SharedUsers {
		return 0
	}
	local := make([]string, 0, len(m.settings.Users))
	for _, u := range m.settings.Users {
		local = append(local, u.Name)
	}
	return fileusers.CountShared(local, fileusers.Default.List(), false)
}

// logListenLocked 记录监听日志，开启共享用户时用户数计入共享用户；格式串保持完整以便日志翻译按模板匹配。
func (m *Module) logListenLocked(addr string) {
	local := len(m.settings.Users)
	if m.settings.SharedUsers {
		shared := m.sharedCountLocked()
		m.logs.Add(logbuf.LevelInfo, "", fmt.Sprintf("FTP 监听 %s（%d 个用户，其中 %d 个共享用户）", addr, local+shared, shared))
		return
	}
	m.logs.Add(logbuf.LevelInfo, "", fmt.Sprintf("FTP 监听 %s（%d 个用户）", addr, local))
}

// Routes 实现 module.Module。
func (m *Module) Routes(r module.Router) {
	r.HandleFunc("GET /settings", func(w http.ResponseWriter, req *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		s := m.settings
		s.Users = append([]User{}, s.Users...)
		for i := range s.Users {
			s.Users[i].Password = crud.Mask(s.Users[i].Password)
		}
		httpx.OK(w, map[string]any{"settings": s, "running": m.srv != nil, "error": m.err})
	})
	// 查看本地用户密码明文，结果按用户名索引（{"用户名": "密码"}）；bcrypt 哈希的密码原样返回哈希。
	// 路径含 secrets 片段，按写权限鉴权，只读账号与只读令牌无法调用
	r.HandleFunc("GET /settings/secrets", crud.SecretsHandler(&m.mu, func() map[string]string {
		return userPasswords(m.settings.Users)
	}))
	module.DocOf(r, "GET /settings/secrets", module.Doc{Summary: "查看本地用户密码明文", Resp: map[string]string{}})
	// 文件服务共享用户：WebDAV、FTP、SMB 共用同一份列表
	fileusers.Default.Routes(r, nil)
	r.HandleFunc("PUT /settings", func(w http.ResponseWriter, req *http.Request) {
		var s Settings
		if err := httpx.ReadJSON(req, &s); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		// 用户没有固定 ID，改名后仍按位置保留原密码，见 crud.KeepSecrets
		crud.KeepSecrets(m.settings.Users, s.Users, func(u *User) string { return u.Name }, func(u *User) *string { return &u.Password })
		if err := s.Validate(); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		if err := m.store.Save(ID, s); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "%v", err)
			return
		}
		m.settings = s
		m.restartLocked()
		httpx.OK(w, map[string]any{"running": m.srv != nil, "error": m.err})
	})
}

// userPasswords 返回本地用户的密码明文，按用户名索引。
func userPasswords(users []User) map[string]string {
	out := make(map[string]string, len(users))
	for _, u := range users {
		out[u.Name] = u.Password
	}
	return out
}
