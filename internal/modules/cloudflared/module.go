// Package cloudflared 托管 cloudflared 隧道：
// 使用 Cloudflare Zero Trust 的隧道令牌运行已配置的隧道，或创建无需账号的临时公网隧道。
package cloudflared

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/wangwanjie/gately/internal/binstall"
	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/proc"
)

// ID 是模块标识。
const ID = "cloudflared"

// 隧道模式。
const (
	// ModeToken 使用在 Cloudflare 控制台创建的隧道令牌，公网主机名与回源服务在控制台配置
	ModeToken = "token"
	// ModeQuick 创建 trycloudflare.com 临时隧道，把本地服务暴露到随机公网地址
	ModeQuick = "quick"
)

// Settings 是模块级配置。
type Settings struct {
	// BinPath 为空时依次在配置目录 bin 与 PATH 下查找 cloudflared
	BinPath string `json:"binPath"`
}

// Tunnel 是一条隧道。
type Tunnel struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Group 为列表中的分组名称，只影响管理界面的展示，留空为未分组
	Group  string `json:"group"`
	Enable bool   `json:"enable"`
	Mode   string `json:"mode"`
	// Token 为 ModeToken 的隧道令牌
	Token string `json:"token"`
	// APIToken 为 ModeToken 可选的 Cloudflare API 令牌（需要 Cloudflare Tunnel 编辑权限，同步 DNS 还需要 DNS 编辑权限），
	// 填写后可在 Gately 中编辑隧道的路由规则
	APIToken string `json:"apiToken,omitempty"`
	// Service 为 ModeQuick 要暴露的本地服务地址，例如 http://localhost:8080
	Service string `json:"service"`
	// Protocol 为边缘连接协议：auto、quic、http2
	Protocol string `json:"protocol"`
	// EdgeIPVersion 为 auto、4、6
	EdgeIPVersion string `json:"edgeIpVersion"`
	Remark        string `json:"remark"`
}

type fileConfig struct {
	Settings Settings `json:"settings"`
	// Install 为在线安装设置
	Install binstall.Config `json:"install"`
	Tunnels []Tunnel        `json:"tunnels"`
}

// Validate 校验并规范化隧道配置。
func (t *Tunnel) Validate() error {
	if err := crud.NormalizeGroup(&t.Group); err != nil {
		return err
	}
	t.Name = strings.TrimSpace(t.Name)
	t.Token = strings.TrimSpace(t.Token)
	t.APIToken = strings.TrimSpace(t.APIToken)
	t.Service = strings.TrimSpace(t.Service)
	if t.Name == "" {
		return errors.New("隧道名称不能为空")
	}
	switch t.Mode {
	case ModeToken:
		if t.Token == "" {
			return errors.New("请填写隧道令牌")
		}
		t.Service = ""
	case ModeQuick:
		u, err := url.Parse(t.Service)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "tcp" && u.Scheme != "ssh" && u.Scheme != "rdp") {
			return errors.New("本地服务地址格式应为 http://localhost:8080")
		}
		t.Token, t.APIToken = "", ""
	default:
		return fmt.Errorf("不支持的模式 %q", t.Mode)
	}
	switch t.Protocol {
	case "", "auto", "quic", "http2":
	default:
		return fmt.Errorf("不支持的协议 %q", t.Protocol)
	}
	switch t.EdgeIPVersion {
	case "", "auto", "4", "6":
	default:
		return fmt.Errorf("不支持的边缘 IP 版本 %q", t.EdgeIPVersion)
	}
	return nil
}

// Args 生成 cloudflared 命令行参数与环境变量。令牌通过环境变量传递，避免出现在进程列表中。
func (t *Tunnel) Args() (args, env []string) {
	args = []string{"tunnel", "--no-autoupdate"}
	if t.Protocol != "" && t.Protocol != "auto" {
		args = append(args, "--protocol", t.Protocol)
	}
	if t.EdgeIPVersion != "" && t.EdgeIPVersion != "auto" {
		args = append(args, "--edge-ip-version", t.EdgeIPVersion)
	}
	if t.Mode == ModeQuick {
		return append(args, "--url", t.Service), nil
	}
	return append(args, "run"), []string{"TUNNEL_TOKEN=" + t.Token}
}

// runtime 是隧道的运行时信息。
type runtime struct {
	sup *proc.Supervisor
	err string

	mu sync.Mutex
	// url 为临时隧道分配的公网地址
	url string
	// conns 为已注册的边缘连接数
	conns int
}

var (
	quickURLRe = regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)
	connRe     = regexp.MustCompile(`Registered tunnel connection`)
)

func (rt *runtime) onLine(line string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if u := quickURLRe.FindString(line); u != "" {
		rt.url = u
	}
	if connRe.MatchString(line) {
		rt.conns++
	}
}

func (rt *runtime) reset() {
	rt.mu.Lock()
	rt.url, rt.conns = "", 0
	rt.mu.Unlock()
}

// Module 是 cloudflared 模块。
type Module struct {
	mu sync.Mutex
	// ingressMu 串行化路由规则的保存（读取远程配置、核对版本、写入），不与 mu 嵌套
	ingressMu sync.Mutex
	store     *config.Store
	logs      *logbuf.Buffer
	cfg       fileConfig
	runs      map[string]*runtime
	started   bool
	// version 缓存已探测的二进制版本
	version     string
	versionPath string
	// httpClient 返回下载程序使用的 HTTP 客户端，由框架按“更新代理”设置注入
	httpClient func() *http.Client
	install    *binstall.Installer
}

// New 创建模块。
func New() *Module {
	return &Module{runs: map[string]*runtime{}, httpClient: func() *http.Client { return http.DefaultClient }}
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
func (m *Module) Title() string { return "Cloudflare 隧道" }

// Init 实现 module.Module。
func (m *Module) Init(env module.Env) error {
	m.store, m.logs = env.Store, env.Logs
	m.install = m.newInstaller()
	_, err := m.store.Load(ID, &m.cfg)
	return err
}

// Start 实现 module.Module。
func (m *Module) Start(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = true
	for i := range m.cfg.Tunnels {
		m.applyLocked(m.cfg.Tunnels[i].Key)
	}
	return nil
}

// Stop 实现 module.Module。
func (m *Module) Stop() error {
	m.mu.Lock()
	m.started = false
	runs := m.runs
	m.runs = map[string]*runtime{}
	m.mu.Unlock()
	for _, rt := range runs {
		rt.sup.Stop()
	}
	return nil
}

func (m *Module) saveLocked() error { return m.store.Save(ID, m.cfg) }

func (m *Module) find(key string) *Tunnel {
	for i := range m.cfg.Tunnels {
		if m.cfg.Tunnels[i].Key == key {
			return &m.cfg.Tunnels[i]
		}
	}
	return nil
}

// applyLocked 按配置启停指定隧道；调用方持有 mu。
func (m *Module) applyLocked(key string) {
	t := m.find(key)
	if t == nil {
		m.dropLocked(key)
		return
	}
	rt := m.runs[key]
	if rt == nil {
		rt = &runtime{sup: proc.New(m.logs.Logger(t.Name))}
		m.runs[key] = rt
	}
	rt.sup.Stop()
	rt.err = ""
	rt.reset()
	if !m.started || !t.Enable {
		return
	}
	bin, err := proc.Resolve(m.cfg.Settings.BinPath, m.store.Dir(), "cloudflared")
	if err != nil {
		rt.err = err.Error()
		m.logs.Logger(t.Name).Errorf("%v", err)
		return
	}
	// 名称可能被修改，重建托管器以使用新的日志来源
	rt.sup = proc.New(m.logs.Logger(t.Name))
	args, env := t.Args()
	rt.sup.Start(proc.Spec{Path: bin, Args: args, Env: env, OnLine: rt.onLine, OnStart: rt.reset})
}

func (m *Module) dropLocked(key string) {
	if rt := m.runs[key]; rt != nil {
		rt.sup.Stop()
		delete(m.runs, key)
	}
}

// TunnelView 是界面展示的隧道。
type TunnelView struct {
	Tunnel
	State proc.State `json:"state"`
	Error string     `json:"error"`
	// URL 为临时隧道的公网地址
	URL string `json:"url,omitempty"`
	// Connections 为已注册的边缘连接数
	Connections int `json:"connections"`
	// TunnelID 为从令牌解析出的隧道 ID
	TunnelID string `json:"tunnelId,omitempty"`
}

func (m *Module) viewLocked(t *Tunnel) TunnelView {
	v := TunnelView{Tunnel: *t}
	v.Token = crud.Mask(v.Token)
	v.APIToken = crud.Mask(v.APIToken)
	if info, err := parseToken(t.Token); err == nil {
		v.TunnelID = info.TunnelID
	}
	if rt := m.runs[t.Key]; rt != nil {
		v.State = rt.sup.State()
		v.Error = rt.err
		if v.Error == "" {
			v.Error = v.State.Error
		}
		rt.mu.Lock()
		v.URL, v.Connections = rt.url, rt.conns
		rt.mu.Unlock()
	}
	return v
}

// Routes 实现 module.Module。
func (m *Module) Routes(r module.Router) {
	tunnels := &crud.Collection[Tunnel]{
		Mu:       &m.mu,
		Items:    &m.cfg.Tunnels,
		Save:     m.saveLocked,
		Key:      func(t *Tunnel) *string { return &t.Key },
		Enabled:  func(t *Tunnel) *bool { return &t.Enable },
		Group:    func(t *Tunnel) *string { return &t.Group },
		Validate: func(t *Tunnel) error { return t.Validate() },
		Merge: func(old, updated *Tunnel) {
			crud.KeepSecret(old.Token, &updated.Token)
			crud.KeepSecret(old.APIToken, &updated.APIToken)
		},
		View: func(t *Tunnel) any { return m.viewLocked(t) },
		// 查看被脱敏的隧道令牌明文
		Secrets: func(t *Tunnel) map[string]string { return map[string]string{"token": t.Token, "apiToken": t.APIToken} },
		Changed: m.applyLocked,
		Removed: m.dropLocked,
	}
	tunnels.Register(r, "/tunnels")
	r.HandleFunc("POST /tunnels/{key}/restart", func(w http.ResponseWriter, req *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		key := req.PathValue("key")
		if m.find(key) == nil {
			httpx.Error(w, http.StatusNotFound, "隧道不存在")
			return
		}
		m.applyLocked(key)
		httpx.OK(w, map[string]bool{"ok": true})
	})
	m.ingressRoutes(r)
	r.HandleFunc("GET /settings", func(w http.ResponseWriter, req *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		out := map[string]any{"settings": m.cfg.Settings, "binary": "", "version": "", "error": ""}
		bin, err := proc.Resolve(m.cfg.Settings.BinPath, m.store.Dir(), "cloudflared")
		if err != nil {
			out["error"] = err.Error()
		} else {
			if m.versionPath != bin {
				m.versionPath, m.version = bin, proc.Version(bin, "--version")
			}
			out["binary"], out["version"] = bin, m.version
		}
		httpx.OK(w, out)
	})
	r.HandleFunc("PUT /settings", func(w http.ResponseWriter, req *http.Request) {
		var s Settings
		if err := httpx.ReadJSON(req, &s); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		s.BinPath = strings.TrimSpace(s.BinPath)
		m.mu.Lock()
		defer m.mu.Unlock()
		old := m.cfg.Settings
		m.cfg.Settings = s
		if err := m.saveLocked(); err != nil {
			m.cfg.Settings = old
			httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
			return
		}
		// 二进制路径变化后重启全部已启用的隧道
		m.restartAllLocked()
		httpx.OK(w, map[string]bool{"ok": true})
	})
	m.install.Register(r)
	r.HandleFunc("POST /reset", func(w http.ResponseWriter, req *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		old := m.cfg
		m.cfg = fileConfig{}
		if err := m.saveLocked(); err != nil {
			m.cfg = old
			httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
			return
		}
		// 停止并移除全部隧道；旧配置留在配置历史中，安装的程序不受影响
		for key := range m.runs {
			m.dropLocked(key)
		}
		m.versionPath = ""
		m.logs.Logger("").Infof("已恢复默认配置")
		httpx.OK(w, map[string]bool{"ok": true})
	})
}

// restartAllLocked 重启全部已启用的隧道；调用方持有 mu。
func (m *Module) restartAllLocked() {
	for i := range m.cfg.Tunnels {
		if m.cfg.Tunnels[i].Enable {
			m.applyLocked(m.cfg.Tunnels[i].Key)
		}
	}
}

// Summary 实现 module.Summarizer。
func (m *Module) Summary() []module.SummaryItem {
	m.mu.Lock()
	defer m.mu.Unlock()
	enabled, running, failed := 0, 0, 0
	for i := range m.cfg.Tunnels {
		t := &m.cfg.Tunnels[i]
		if !t.Enable {
			continue
		}
		enabled++
		if rt := m.runs[t.Key]; rt != nil {
			if rt.sup.State().Running {
				running++
			} else if rt.err != "" || rt.sup.State().Error != "" {
				failed++
			}
		}
	}
	level := "ok"
	if failed > 0 {
		level = "error"
	} else if running < enabled {
		level = "warn"
	}
	return []module.SummaryItem{{Label: "运行中", Value: fmt.Sprintf("%d / %d", running, enabled), Level: level}}
}
