// Package webdav 实现 WebDAV 文件服务：多用户、每个用户映射到一个或多个目录，可设为只读。
package webdav

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
	"time"

	xwebdav "golang.org/x/net/webdav"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/fileusers"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/mountfs"
)

// ID 是模块标识。
const ID = "webdav"

// User 是一个 WebDAV 账号。
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
	// TLS 时证书由证书模块按 SNI 提供
	TLS   bool   `json:"tls"`
	Users []User `json:"users"`
	// SharedUsers 为 true 时文件服务共享用户（见 fileusers 包）也可登录，与本地用户同名时只按本地用户校验
	SharedUsers bool `json:"sharedUsers"`
}

// CertSource 提供 HTTPS 证书。
type CertSource interface {
	GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error)
}

// Validate 校验配置。
func (s *Settings) Validate() error {
	if s.Port == 0 {
		s.Port = 18800
	}
	if s.Port < 1 || s.Port > 65535 {
		return errors.New("端口无效")
	}
	s.ListenIP = strings.Trim(strings.TrimSpace(s.ListenIP), "[]")
	if s.ListenIP != "" && net.ParseIP(s.ListenIP) == nil {
		return errors.New("监听地址无效")
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
		return errors.New("启用 WebDAV 时至少添加一个用户")
	}
	return nil
}

type userHandler struct {
	user    User
	handler *xwebdav.Handler
}

// Module 是 WebDAV 模块。
type Module struct {
	mu       sync.Mutex
	store    *config.Store
	logs     *logbuf.Buffer
	certs    CertSource
	settings Settings
	srv      *http.Server
	// ln 为交给 srv 的监听，停止时同步关闭，见 stopLocked 中的说明
	ln       net.Listener
	err      string
	handlers map[string]*userHandler
	// throttle 按来源地址限制密码校验失败的次数，防止暴力猜测密码
	throttle fileusers.Throttle
	// shared 为共享用户的处理器，按用户名缓存；共享用户被修改后清空，下次请求按新的目录配置重建
	shared map[string]*userHandler
}

// New 创建模块。
func New(certs CertSource) *Module { return &Module{certs: certs} }

// ID 实现 module.Module。
func (m *Module) ID() string { return ID }

// Title 实现 module.Module。
func (m *Module) Title() string { return "WebDAV 服务" }

// Init 实现 module.Module。
func (m *Module) Init(env module.Env) error {
	m.store, m.logs = env.Store, env.Logs
	m.settings.Port = 18800
	if _, err := m.store.Load(ID, &m.settings); err != nil {
		return err
	}
	if err := fileusers.Default.Init(env.Store); err != nil {
		return err
	}
	// 共享用户修改后丢弃缓存的处理器，已登录的客户端下次请求即按新的密码与目录校验
	// 服务不重启，运行中且开启共享用户时记一条日志说明当前用户数
	fileusers.Default.Subscribe(func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.shared = map[string]*userHandler{}
		if m.settings.Enable && m.settings.SharedUsers && m.srv != nil {
			shared := m.sharedCountLocked()
			m.logs.Add(logbuf.LevelInfo, "", fmt.Sprintf("共享用户已更新（%d 个用户，其中 %d 个共享用户）", len(m.settings.Users)+shared, shared))
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
			return errors.New("WebDAV 已启用且只使用共享用户，至少保留一个共享用户")
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
	if m.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = m.srv.Shutdown(ctx)
		// Serve 在后台 goroutine 中启动：刚启动就停止时它可能还没登记监听，Shutdown 关不到，
		// 监听要等那个 goroutine 退出时才关闭，紧接着重启时在同一端口监听会报端口已被占用。这里同步关闭
		if m.ln != nil {
			_ = m.ln.Close()
		}
		m.srv, m.ln = nil, nil
	}
}

func (m *Module) restartLocked() {
	m.stopLocked()
	m.err = ""
	if !m.settings.Enable {
		return
	}
	m.handlers = map[string]*userHandler{}
	m.shared = map[string]*userHandler{}
	for _, u := range m.settings.Users {
		m.handlers[u.Name] = newUserHandler(u)
	}
	addr := net.JoinHostPort(m.settings.ListenIP, strconv.Itoa(m.settings.Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		m.err = err.Error()
		m.logs.Add(logbuf.LevelError, "", "监听失败: "+err.Error())
		return
	}
	// 被封禁地址的连接在 Accept 时直接关闭
	ln = banlist.Default.Listener(ln)
	// ErrorLog 把 TLS 握手失败等错误写入模块运行日志
	m.srv = &http.Server{Handler: http.HandlerFunc(m.serve), ReadHeaderTimeout: 30 * time.Second, ErrorLog: m.logs.Logger("").ServerErrorLog()}
	if m.settings.TLS {
		if m.certs == nil {
			ln.Close()
			m.err = "证书模块不可用"
			return
		}
		m.srv.TLSConfig = &tls.Config{GetCertificate: m.certs.GetCertificate, MinVersion: tls.VersionTLS12}
		ln = tls.NewListener(ln, m.srv.TLSConfig)
	}
	srv := m.srv
	m.ln = ln
	go func() { _ = srv.Serve(ln) }()
	m.logListenLocked(addr)
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
		m.logs.Add(logbuf.LevelInfo, "", fmt.Sprintf("WebDAV 监听 %s（%d 个用户，其中 %d 个共享用户）", addr, local+shared, shared))
		return
	}
	m.logs.Add(logbuf.LevelInfo, "", fmt.Sprintf("WebDAV 监听 %s（%d 个用户）", addr, local))
}

// newUserHandler 按用户的目录配置创建 WebDAV 处理器：Mounts 非空时为多目录，否则为单个目录。
func newUserHandler(u User) *userHandler {
	var fs xwebdav.FileSystem
	if len(u.Mounts) > 0 {
		fs = mountfs.WebDAV(u.Mounts)
	} else {
		fs = xwebdav.Dir(u.Root)
		if u.ReadOnly {
			fs = mountfs.ReadOnlyDAV{FileSystem: fs}
		}
	}
	return &userHandler{user: u, handler: &xwebdav.Handler{FileSystem: fs, LockSystem: xwebdav.NewMemLS()}}
}

// lookupLocked 按用户名查找处理器：先找本地用户，没有同名本地用户且开启了共享用户时再找共享用户。调用方需持有 m.mu。
func (m *Module) lookupLocked(name string) (uh *userHandler, shared bool) {
	if uh := m.handlers[name]; uh != nil {
		return uh, false
	}
	if !m.settings.SharedUsers || m.shared == nil {
		return nil, false
	}
	if uh := m.shared[name]; uh != nil {
		return uh, true
	}
	u, ok := fileusers.Default.Lookup(name)
	if !ok {
		return nil, false
	}
	uh = newUserHandler(User(u))
	m.shared[name] = uh
	return uh, true
}

func (m *Module) serve(w http.ResponseWriter, r *http.Request) {
	name, pass, ok := r.BasicAuth()
	ip := httpx.ClientIP(r)
	// 失败次数（含进行中的校验）已达上限的来源一律拒绝，包括凭据正确的请求，避免被拒绝的来源借响应判断密码是否猜中
	if ok && m.throttle.Blocked(ip) {
		http.Error(w, "429 Too Many Requests", http.StatusTooManyRequests)
		return
	}
	m.mu.Lock()
	uh, shared := m.lookupLocked(name)
	m.mu.Unlock()
	var authed bool
	if ok && uh != nil && fileusers.KnownGood(uh.user.Password, pass) {
		// 近期验证通过的凭据不占限流名额：客户端并发的多个请求（rclone 等）不会因进行中的校验被误拒
		authed = true
	} else if ok {
		// 其余带凭据的请求先占位：失败次数加进行中的校验达到上限时直接拒绝，不再校验密码
		if !m.throttle.Reserve(ip) {
			http.Error(w, "429 Too Many Requests", http.StatusTooManyRequests)
			return
		}
		authed = uh != nil && fileusers.CheckPassword(uh.user.Password, pass)
		m.throttle.Done(ip, authed)
	}
	if !authed {
		if ok && shared {
			m.logs.Add(logbuf.LevelWarn, "", fmt.Sprintf("%s 认证失败（共享用户 %q）", r.RemoteAddr, fileusers.ClipName(name)))
		} else if ok {
			m.logs.Add(logbuf.LevelWarn, "", fmt.Sprintf("%s 认证失败（用户 %q）", r.RemoteAddr, fileusers.ClipName(name)))
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="Gately WebDAV"`)
		http.Error(w, "401 Unauthorized", http.StatusUnauthorized)
		return
	}
	uh.handler.ServeHTTP(w, r)
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
