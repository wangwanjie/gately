// Package tailscale 托管 tailscaled 并通过 tailscale 命令完成登录与状态展示，
// 把本机加入 Tailscale 或 Headscale 组网，可发布子网路由与出口节点。
package tailscale

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

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
const ID = "tailscale"

const (
	// readyTimeout 是等待 tailscaled 就绪的最长时间
	readyTimeout  = 30 * time.Second
	statusTimeout = 8 * time.Second
)

// Module 是 Tailscale 模块。
type Module struct {
	mu       sync.Mutex
	store    *config.Store
	logs     *logbuf.Buffer
	settings Settings
	daemon   *proc.Supervisor
	started  bool
	// err 记录最近一次启动流程的错误，由 errMu 保护：启动协程需要写入，
	// 而停止流程在持有 mu 时等待该协程退出，共用 mu 会死锁
	errMu sync.Mutex
	err   string
	// cache 缓存最近一次状态，避免总览页与状态页频繁轮询时反复执行命令
	cache   StatusView
	cacheAt time.Time
	// upCancel 取消正在进行的 tailscale up
	upCancel context.CancelFunc
	upDone   chan struct{}
	// version 缓存各二进制版本
	versions map[string]string
	// httpClient 返回下载程序使用的 HTTP 客户端，由框架按“更新代理”设置注入
	httpClient func() *http.Client
	install    *binstall.Installer
}

// New 创建模块。
func New() *Module {
	return &Module{versions: map[string]string{}, httpClient: func() *http.Client { return http.DefaultClient }}
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
func (m *Module) Title() string { return "Tailscale 组网" }

// Init 实现 module.Module。
func (m *Module) Init(env module.Env) error {
	m.store, m.logs = env.Store, env.Logs
	m.daemon = proc.New(m.logs.Logger("tailscaled"))
	m.settings = DefaultSettings()
	m.install = m.newInstaller()
	_, err := m.store.Load(ID, &m.settings)
	return err
}

// Start 实现 module.Module。
func (m *Module) Start(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = true
	m.applyLocked()
	return nil
}

// Stop 实现 module.Module。
func (m *Module) Stop() error {
	m.mu.Lock()
	m.started = false
	m.stopLocked()
	m.mu.Unlock()
	return nil
}

// stopLocked 取消进行中的 up 并停止托管的 tailscaled。
func (m *Module) stopLocked() {
	if m.upCancel != nil {
		m.upCancel()
		<-m.upDone
		m.upCancel, m.upDone = nil, nil
	}
	m.daemon.Stop()
}

// applyLocked 按当前配置重建运行状态；调用方持有 mu。
func (m *Module) applyLocked() {
	m.stopLocked()
	m.putErr("")
	m.cacheAt = time.Time{}
	if !m.started || !m.settings.Enable {
		return
	}
	s := m.settings
	dir := m.store.Dir()
	cli, err := proc.Resolve(s.TailscalePath, dir, "tailscale")
	if err != nil {
		m.fail(err.Error())
		return
	}
	if s.ManageDaemon {
		daemon, err := proc.Resolve(s.TailscaledPath, dir, "tailscaled")
		if err != nil {
			m.fail(err.Error())
			return
		}
		// 默认状态目录在配置目录内，收紧为 0700；自定义状态目录可能是已有的其他目录，只在不存在时创建，不改动已有目录的权限
		mkdir := os.MkdirAll
		if s.StateDir == "" {
			mkdir = fsperm.MkdirAll
		}
		if err := mkdir(s.stateDir(dir), 0o700); err != nil {
			m.fail("创建状态目录失败: " + err.Error())
			return
		}
		m.daemon.Start(proc.Spec{Path: daemon, Args: s.DaemonArgs(dir), Env: s.DaemonEnvList()})
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	m.upCancel, m.upDone = cancel, done
	go func() {
		defer close(done)
		m.bringUp(ctx, cli, s, dir)
	}()
}

func (m *Module) putErr(msg string) {
	m.errMu.Lock()
	m.err = msg
	m.errMu.Unlock()
}

func (m *Module) getErr() string {
	m.errMu.Lock()
	defer m.errMu.Unlock()
	return m.err
}

func (m *Module) fail(msg string) {
	m.putErr(msg)
	m.logs.Logger("").Errorf("%s", msg)
}

// bringUp 等待 tailscaled 就绪后执行 tailscale up。未提供认证密钥时 up 会一直等待，
// 登录链接写入日志，并可通过状态接口获取。
func (m *Module) bringUp(ctx context.Context, cli string, s Settings, dir string) {
	log := m.logs.Logger("tailscale")
	global := s.CLIArgs(dir)
	deadline := time.Now().Add(readyTimeout)
	for {
		if _, err := runStatus(cli, global); err == nil {
			break
		}
		if ctx.Err() != nil {
			return
		}
		if time.Now().After(deadline) {
			m.fail("等待 tailscaled 就绪超时，请检查日志")
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
	args := append(append([]string{}, global...), s.UpArgs()...)
	if err := proc.Run(ctx, proc.Spec{Path: cli, Args: args}, log); err != nil && ctx.Err() == nil {
		m.fail("tailscale up 失败: " + err.Error())
		return
	}
	if ctx.Err() == nil {
		log.Infof("tailscale up 已完成")
	}
}

// rawPeer 对应 tailscale status --json 中的节点。
type rawPeer struct {
	HostName       string    `json:"HostName"`
	DNSName        string    `json:"DNSName"`
	OS             string    `json:"OS"`
	TailscaleIPs   []string  `json:"TailscaleIPs"`
	Online         bool      `json:"Online"`
	Active         bool      `json:"Active"`
	ExitNode       bool      `json:"ExitNode"`
	ExitNodeOption bool      `json:"ExitNodeOption"`
	Relay          string    `json:"Relay"`
	CurAddr        string    `json:"CurAddr"`
	RxBytes        int64     `json:"RxBytes"`
	TxBytes        int64     `json:"TxBytes"`
	LastSeen       time.Time `json:"LastSeen"`
	PrimaryRoutes  []string  `json:"PrimaryRoutes"`
}

type rawStatus struct {
	Version        string              `json:"Version"`
	BackendState   string              `json:"BackendState"`
	AuthURL        string              `json:"AuthURL"`
	Health         []string            `json:"Health"`
	MagicDNSSuffix string              `json:"MagicDNSSuffix"`
	Self           *rawPeer            `json:"Self"`
	Peer           map[string]*rawPeer `json:"Peer"`
	CurrentTailnet *struct {
		Name string `json:"Name"`
	} `json:"CurrentTailnet"`
}

func runStatus(cli string, global []string) (*rawStatus, error) {
	out, err := proc.Output(statusTimeout, cli, append(append([]string{}, global...), "status", "--json"), nil)
	var st rawStatus
	if jerr := json.Unmarshal(out, &st); jerr != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" && err != nil {
			msg = err.Error()
		}
		return nil, errors.New(msg)
	}
	return &st, nil
}

// PeerView 是界面展示的节点。
type PeerView struct {
	Name           string    `json:"name"`
	DNSName        string    `json:"dnsName"`
	IPs            []string  `json:"ips"`
	OS             string    `json:"os"`
	Online         bool      `json:"online"`
	Active         bool      `json:"active"`
	ExitNode       bool      `json:"exitNode"`
	ExitNodeOption bool      `json:"exitNodeOption"`
	Relay          string    `json:"relay,omitempty"`
	CurAddr        string    `json:"curAddr,omitempty"`
	Routes         []string  `json:"routes,omitempty"`
	RxBytes        int64     `json:"rxBytes"`
	TxBytes        int64     `json:"txBytes"`
	LastSeen       time.Time `json:"lastSeen,omitempty"`
}

func peerView(p *rawPeer) PeerView {
	v := PeerView{
		Name: p.HostName, DNSName: strings.TrimSuffix(p.DNSName, "."), IPs: p.TailscaleIPs, OS: p.OS,
		Online: p.Online, Active: p.Active, ExitNode: p.ExitNode, ExitNodeOption: p.ExitNodeOption,
		Relay: p.Relay, CurAddr: p.CurAddr, Routes: p.PrimaryRoutes, RxBytes: p.RxBytes, TxBytes: p.TxBytes,
	}
	if !p.LastSeen.IsZero() && p.LastSeen.Year() > 1 {
		v.LastSeen = p.LastSeen
	}
	return v
}

// StatusView 是 /status 接口的返回内容。
type StatusView struct {
	Daemon  proc.State `json:"daemon"`
	CLI     string     `json:"cli"`
	Version string     `json:"version"`
	// Error 为启动流程或状态查询的错误
	Error          string     `json:"error"`
	BackendState   string     `json:"backendState"`
	AuthURL        string     `json:"authUrl,omitempty"`
	Tailnet        string     `json:"tailnet,omitempty"`
	MagicDNSSuffix string     `json:"magicDnsSuffix,omitempty"`
	Health         []string   `json:"health,omitempty"`
	Self           *PeerView  `json:"self,omitempty"`
	Peers          []PeerView `json:"peers"`
}

// statusCacheTTL 是状态缓存的有效期。
const statusCacheTTL = 2 * time.Second

func (m *Module) status() StatusView {
	m.mu.Lock()
	if time.Since(m.cacheAt) < statusCacheTTL {
		v := m.cache
		m.mu.Unlock()
		return v
	}
	m.mu.Unlock()
	v := m.queryStatus()
	m.mu.Lock()
	m.cache, m.cacheAt = v, time.Now()
	m.mu.Unlock()
	return v
}

func (m *Module) queryStatus() StatusView {
	m.mu.Lock()
	s := m.settings
	v := StatusView{Daemon: m.daemon.State(), Error: m.getErr(), Peers: []PeerView{}}
	dir := m.store.Dir()
	m.mu.Unlock()

	cli, err := proc.Resolve(s.TailscalePath, dir, "tailscale")
	if err != nil {
		if v.Error == "" {
			v.Error = err.Error()
		}
		return v
	}
	v.CLI = cli
	m.mu.Lock()
	ver, ok := m.versions[cli]
	m.mu.Unlock()
	if !ok {
		ver = proc.Version(cli, "version")
		m.mu.Lock()
		m.versions[cli] = ver
		m.mu.Unlock()
	}
	v.Version = ver
	if v.Daemon.Error != "" && v.Error == "" {
		v.Error = v.Daemon.Error
	}
	st, err := runStatus(cli, s.CLIArgs(dir))
	if err != nil {
		// 未启用且守护进程不可达是常态，不当作错误提示
		if s.Enable && v.Error == "" {
			v.Error = "无法获取状态: " + err.Error()
		}
		return v
	}
	v.BackendState, v.AuthURL, v.Health, v.MagicDNSSuffix = st.BackendState, st.AuthURL, st.Health, st.MagicDNSSuffix
	if st.CurrentTailnet != nil {
		v.Tailnet = st.CurrentTailnet.Name
	}
	if st.Self != nil {
		self := peerView(st.Self)
		v.Self = &self
	}
	for _, p := range st.Peer {
		v.Peers = append(v.Peers, peerView(p))
	}
	sort.Slice(v.Peers, func(i, j int) bool {
		if v.Peers[i].Online != v.Peers[j].Online {
			return v.Peers[i].Online
		}
		return v.Peers[i].Name < v.Peers[j].Name
	})
	return v
}

// Routes 实现 module.Module。
func (m *Module) Routes(r module.Router) {
	r.HandleFunc("GET /settings", func(w http.ResponseWriter, req *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		s := m.settings
		s.AuthKey = crud.Mask(s.AuthKey)
		if s.AdvertiseRoutes == nil {
			s.AdvertiseRoutes = []string{}
		}
		httpx.OK(w, map[string]any{"settings": s, "defaultStateDir": m.settings.stateDir(m.store.Dir())})
	})
	r.HandleFunc("PUT /settings", func(w http.ResponseWriter, req *http.Request) {
		s := DefaultSettings()
		if err := httpx.ReadJSON(req, &s); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		crud.KeepSecret(m.settings.AuthKey, &s.AuthKey)
		// 下载代理只由安装接口修改
		s.InstallProxy = m.settings.InstallProxy
		if err := s.Validate(); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		old := m.settings
		m.settings = s
		if err := m.store.Save(ID, s); err != nil {
			m.settings = old
			httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
			return
		}
		m.applyLocked()
		httpx.OK(w, map[string]bool{"ok": true})
	})
	// 查看被脱敏的认证密钥明文
	r.HandleFunc("GET /secrets", crud.SecretsHandler(&m.mu, func() map[string]string {
		return map[string]string{"authKey": m.settings.AuthKey}
	}))
	module.DocOf(r, "GET /secrets", module.Doc{Summary: "查看密钥明文", Resp: map[string]string{}})
	r.HandleFunc("GET /status", func(w http.ResponseWriter, req *http.Request) {
		httpx.OK(w, m.status())
	})
	r.HandleFunc("POST /restart", func(w http.ResponseWriter, req *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.applyLocked()
		httpx.OK(w, map[string]bool{"ok": true})
	})
	r.HandleFunc("POST /logout", func(w http.ResponseWriter, req *http.Request) {
		m.mu.Lock()
		s := m.settings
		dir := m.store.Dir()
		m.mu.Unlock()
		cli, err := proc.Resolve(s.TailscalePath, dir, "tailscale")
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		out, err := proc.Output(15*time.Second, cli, append(s.CLIArgs(dir), "logout"), nil)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "退出登录失败: %s", strings.TrimSpace(string(out)))
			return
		}
		m.logs.Logger("tailscale").Infof("已退出 tailnet 登录")
		httpx.OK(w, map[string]bool{"ok": true})
	})
	m.install.Register(r)
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

// resetLocked 恢复默认配置：Gately 托管的 tailscaled 正在运行时先退出 tailnet 登录（尽力而为），
// 再停止守护进程、恢复默认设置并删除默认状态目录中的登录状态。自定义的状态目录与系统已有的 tailscaled
// 不会被删除或登出；安装的程序不受影响。调用方持有 mu。
func (m *Module) resetLocked() error {
	old := m.settings
	dir := m.store.Dir()
	log := m.logs.Logger("tailscale")
	if old.ManageDaemon && m.daemon.State().Running {
		if cli, err := proc.Resolve(old.TailscalePath, dir, "tailscale"); err == nil {
			if out, err := proc.Output(15*time.Second, cli, append(old.CLIArgs(dir), "logout"), nil); err != nil {
				log.Warnf("退出 tailnet 登录失败: %s", strings.TrimSpace(string(out)))
			}
		}
	}
	s := DefaultSettings()
	if err := m.store.Save(ID, s); err != nil {
		return fmt.Errorf("保存失败: %v", err)
	}
	m.settings = s
	// 新配置未启用，apply 只会停止守护进程
	m.applyLocked()
	m.versions = map[string]string{}
	if old.StateDir == "" {
		if err := os.RemoveAll(old.stateDir(dir)); err != nil {
			return fmt.Errorf("删除状态目录失败: %v", err)
		}
	} else {
		log.Warnf("自定义状态目录 %s 未删除，如不再需要请手动删除", old.StateDir)
	}
	log.Infof("已恢复默认配置")
	return nil
}

// Summary 实现 module.Summarizer。
func (m *Module) Summary() []module.SummaryItem {
	m.mu.Lock()
	enabled, err, managed := m.settings.Enable, m.getErr(), m.settings.ManageDaemon
	running := m.daemon.State().Running
	m.mu.Unlock()
	switch {
	case !enabled:
		return []module.SummaryItem{{Label: "状态", Value: "未启用", Level: "info"}}
	case err != "":
		return []module.SummaryItem{{Label: "状态", Value: "异常", Level: "error"}}
	case managed && !running:
		return []module.SummaryItem{{Label: "守护进程", Value: "未运行", Level: "warn"}}
	}
	// 后端状态需要执行命令，status 内部带短期缓存，总览页轮询不会反复执行
	st := m.status()
	switch st.BackendState {
	case "Running":
		return []module.SummaryItem{
			{Label: "状态", Value: "已连接", Level: "ok"},
			{Label: "节点", Value: strconv.Itoa(len(st.Peers))},
		}
	case "NeedsLogin", "NeedsMachineAuth":
		return []module.SummaryItem{{Label: "状态", Value: "等待登录", Level: "warn"}}
	case "":
		return []module.SummaryItem{{Label: "状态", Value: "启动中", Level: "warn"}}
	}
	return []module.SummaryItem{{Label: "状态", Value: st.BackendState, Level: "warn"}}
}
