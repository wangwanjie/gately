// Package tailscale 托管 tailscaled 并通过 tailscale 命令完成登录与状态展示，
// 把本机加入 Tailscale 或 Headscale 组网，可发布子网路由与出口节点。
// 支持多个实例：每个实例是一个独立的 tailscaled（状态目录、控制套接字与进程互不相同），可分别加入不同的 tailnet。
package tailscale

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/auth"
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

// instRuntime 是一个实例的运行时信息。
type instRuntime struct {
	daemon *proc.Supervisor
	// sig 是最近一次按配置启动时的运行参数签名，实例停用或模块未启动时为空；签名不变时不重启
	sig string
	// conflict 是最近一次启动前检查到的冲突，非空时实例未启动
	conflict string
	// inst 是最近一次应用的实例配置，删除实例时据此退出登录与清理状态目录
	inst Instance
	// upCancel 取消正在进行的 tailscale up
	upCancel context.CancelFunc
	upDone   chan struct{}
	// errMu 保护 err：启动协程需要写入，而停止流程在持有模块锁时等待该协程退出，共用模块锁会死锁
	errMu sync.Mutex
	err   string
	// cache 缓存最近一次状态，避免总览页与状态页频繁轮询时反复执行命令；由模块锁保护
	cache   StatusView
	cacheAt time.Time
}

func (rt *instRuntime) putErr(msg string) {
	rt.errMu.Lock()
	rt.err = msg
	rt.errMu.Unlock()
}

func (rt *instRuntime) getErr() string {
	rt.errMu.Lock()
	defer rt.errMu.Unlock()
	return rt.err
}

// Module 是 Tailscale 模块。
type Module struct {
	mu      sync.Mutex
	store   *config.Store
	logs    *logbuf.Buffer
	cfg     fileConfig
	runs    map[string]*instRuntime
	started bool
	// versions 缓存各二进制版本
	versions map[string]string
	// httpClient 返回下载程序使用的 HTTP 客户端，由框架按“更新代理”设置注入
	httpClient func() *http.Client
	install    *binstall.Installer
}

// New 创建模块。
func New() *Module {
	return &Module{
		runs:       map[string]*instRuntime{},
		versions:   map[string]string{},
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
func (m *Module) Title() string { return "Tailscale 组网" }

// Init 实现 module.Module。
func (m *Module) Init(env module.Env) error {
	m.store, m.logs = env.Store, env.Logs
	m.install = m.newInstaller()
	cfg, migrated, err := loadConfig(m.store)
	if err != nil {
		return err
	}
	m.cfg = cfg
	if migrated {
		// 立即保存迁移结果，避免每次启动都生成新的实例 key；
		// 保存失败（如配置目录只读）不影响其他模块启动，内存中照常使用迁移后的配置，下次启动会重新迁移
		if err := m.saveLocked(); err != nil {
			m.logs.Logger("tailscale").Warnf("保存迁移后的 Tailscale 配置失败: %v", err)
		} else {
			m.logs.Logger("tailscale").Infof("已把旧版单个 Tailscale 配置迁移为实例")
		}
	}
	return nil
}

// loadConfig 读取 tailscale.json：新格式含 instances 字段；旧版单实例配置（顶层即为全部字段）迁移为一个实例，
// migrated 表示发生了迁移、需要保存。文件不存在时返回空配置。
func loadConfig(store *config.Store) (cfg fileConfig, migrated bool, err error) {
	var raw map[string]json.RawMessage
	found, err := store.Load(ID, &raw)
	if err != nil || !found {
		return fileConfig{}, false, err
	}
	if _, ok := raw["instances"]; ok {
		_, err = store.Load(ID, &cfg)
		return cfg, false, err
	}
	old := v1Settings{Options: DefaultOptions()}
	if _, err := store.Load(ID, &old); err != nil {
		return fileConfig{}, false, err
	}
	return migrateLegacy(old), true, nil
}

func (m *Module) saveLocked() error {
	if m.cfg.Instances == nil {
		// 保存为 []，下次加载时据此识别为新格式
		m.cfg.Instances = []Instance{}
	}
	return m.store.Save(ID, m.cfg)
}

// Start 实现 module.Module。
func (m *Module) Start(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = true
	m.syncLocked(func(string) bool { return true })
	return nil
}

// Stop 实现 module.Module。
func (m *Module) Stop() error {
	m.mu.Lock()
	m.started = false
	for key, rt := range m.runs {
		stopRuntime(rt)
		delete(m.runs, key)
	}
	m.mu.Unlock()
	return nil
}

// stopRuntime 取消进行中的 up 并停止托管的 tailscaled。调用方持有模块锁。
func stopRuntime(rt *instRuntime) {
	if rt.upCancel != nil {
		rt.upCancel()
		<-rt.upDone
		rt.upCancel, rt.upDone = nil, nil
	}
	rt.daemon.Stop()
}

func (m *Module) findInstance(key string) *Instance {
	for i := range m.cfg.Instances {
		if m.cfg.Instances[i].Key == key {
			return &m.cfg.Instances[i]
		}
	}
	return nil
}

// daemonSource 与 cliSource 返回实例的日志来源，例如 tailscaled:家里、tailscale:家里。
func daemonSource(name string) string { return "tailscaled:" + name }

func cliSource(name string) string { return "tailscale:" + name }

// signature 返回实例应运行的参数签名：分组与备注只影响展示，下载代理只影响在线安装，都不参与签名；
// 名称是日志来源，改名后重启以使用新的来源。停用或模块未启动时为空。
func (m *Module) signature(in *Instance) string {
	if !m.started || !in.Enable {
		return ""
	}
	v := *in
	v.Group, v.Remark = "", ""
	b, _ := json.Marshal(struct {
		Instance
		TailscaledPath string
		TailscalePath  string
	}{v, m.cfg.Settings.TailscaledPath, m.cfg.Settings.TailscalePath})
	return string(b)
}

// syncLocked 让全部实例与配置一致：签名或冲突状态变化的实例重启，force 返回 true 的实例无条件重启，
// 已删除实例的进程停止。实例增删改时用它只重启受影响的实例。调用方持有 mu。
func (m *Module) syncLocked(force func(key string) bool) {
	keep := map[string]bool{}
	dir := m.store.Dir()
	for i := range m.cfg.Instances {
		in := &m.cfg.Instances[i]
		keep[in.Key] = true
		sig := m.signature(in)
		conflict := ""
		if sig != "" {
			if err := checkConflicts(m.cfg.Instances, in, dir, true); err != nil {
				conflict = err.Error()
			}
		}
		rt := m.runs[in.Key]
		if rt != nil && !force(in.Key) && rt.sig == sig && rt.conflict == conflict {
			// 运行参数未变，只同步分组、备注等展示字段
			rt.inst = *in
			continue
		}
		m.runInstanceLocked(in, sig, conflict)
	}
	for key, rt := range m.runs {
		if !keep[key] {
			stopRuntime(rt)
			delete(m.runs, key)
		}
	}
}

// applyLocked 在实例新增、修改、启停后同步运行状态：运行参数变化的实例重启，该实例上次启动失败时也重启
// （保存一次即重试）；只改分组或备注时不重启，避免组网中断。其他实例只在冲突状态变化时重启。调用方持有 mu。
func (m *Module) applyLocked(key string) {
	m.syncLocked(func(k string) bool {
		if k != key {
			return false
		}
		rt := m.runs[k]
		return rt != nil && rt.sig != "" && rt.getErr() != ""
	})
}

// restartLocked 重启指定实例：重新启动托管的 tailscaled 并再次执行 tailscale up。调用方持有 mu。
func (m *Module) restartLocked(key string) {
	m.syncLocked(func(k string) bool { return k == key })
}

// runInstanceLocked 按签名重建实例的运行状态：sig 为空表示应停止，conflict 非空时不启动并记录原因。调用方持有 mu。
func (m *Module) runInstanceLocked(in *Instance, sig, conflict string) {
	rt := m.runs[in.Key]
	if rt != nil {
		stopRuntime(rt)
	}
	// 名称可能被修改，重建托管器以使用新的日志来源
	rt = &instRuntime{daemon: proc.New(m.logs.Logger(daemonSource(in.Name))), sig: sig, conflict: conflict, inst: *in}
	m.runs[in.Key] = rt
	if sig == "" {
		return
	}
	log := m.logs.Logger(cliSource(in.Name))
	fail := func(msg string) {
		rt.putErr(msg)
		log.Errorf("%s", msg)
	}
	if conflict != "" {
		fail(conflict)
		return
	}
	dir := m.store.Dir()
	s := m.cfg.Settings
	cli, err := proc.Resolve(s.TailscalePath, dir, "tailscale")
	if err != nil {
		fail(err.Error())
		return
	}
	if in.ManageDaemon {
		daemon, err := proc.Resolve(s.TailscaledPath, dir, "tailscaled")
		if err != nil {
			fail(err.Error())
			return
		}
		// 默认状态目录在配置目录内，收紧为 0700；自定义状态目录可能是已有的其他目录，只在不存在时创建，不改动已有目录的权限
		mkdir := os.MkdirAll
		if in.StateDir == "" {
			mkdir = fsperm.MkdirAll
		}
		if err := mkdir(in.stateDir(dir), 0o700); err != nil {
			fail("创建状态目录失败: " + err.Error())
			return
		}
		rt.daemon.Start(proc.Spec{Path: daemon, Args: in.DaemonArgs(dir), Env: in.DaemonEnvList()})
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	rt.upCancel, rt.upDone = cancel, done
	inst := *in
	go func() {
		defer close(done)
		bringUp(ctx, rt, log, cli, inst, dir)
	}()
}

// bringUp 等待 tailscaled 就绪后执行 tailscale up。未提供认证密钥时 up 会一直等待，
// 登录链接写入日志，并可通过状态接口获取。不使用模块锁。
func bringUp(ctx context.Context, rt *instRuntime, log *logbuf.Logger, cli string, in Instance, dir string) {
	fail := func(msg string) {
		rt.putErr(msg)
		log.Errorf("%s", msg)
	}
	global := in.CLIArgs(dir)
	deadline := time.Now().Add(readyTimeout)
	for {
		if _, err := runStatus(cli, global); err == nil {
			break
		}
		if ctx.Err() != nil {
			return
		}
		if time.Now().After(deadline) {
			fail("等待 tailscaled 就绪超时，请检查日志")
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
	args := append(append([]string{}, global...), in.UpArgs()...)
	// 登录链接可把本机登记到任意 tailnet，日志里只保留主机部分，完整链接通过状态接口提供给有写权限的身份
	if err := proc.Run(ctx, proc.Spec{Path: cli, Args: args, LogLine: proc.MaskURLPaths}, log); err != nil && ctx.Err() == nil {
		fail("tailscale up 失败: " + err.Error())
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

// StatusView 是一个实例的连接状态。
type StatusView struct {
	// Key、Name 为实例的 key 与名称
	Key     string     `json:"key"`
	Name    string     `json:"name"`
	Enable  bool       `json:"enable"`
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

// status 返回实例的状态，带短期缓存；实例不存在时 ok 为 false。
func (m *Module) status(key string) (v StatusView, ok bool) {
	m.mu.Lock()
	in := m.findInstance(key)
	if in == nil {
		m.mu.Unlock()
		return StatusView{}, false
	}
	if rt := m.runs[key]; rt != nil && time.Since(rt.cacheAt) < statusCacheTTL {
		v := rt.cache
		m.mu.Unlock()
		return v, true
	}
	m.mu.Unlock()
	v = m.queryStatus(key)
	m.mu.Lock()
	// 查询期间实例可能被重启（运行时被替换），缓存写到当前的运行时上
	if rt := m.runs[key]; rt != nil {
		rt.cache, rt.cacheAt = v, time.Now()
	}
	m.mu.Unlock()
	return v, true
}

// statusAll 并行查询全部实例的状态，顺序同实例列表。
func (m *Module) statusAll() []StatusView {
	m.mu.Lock()
	keys := make([]string, len(m.cfg.Instances))
	for i := range m.cfg.Instances {
		keys[i] = m.cfg.Instances[i].Key
	}
	m.mu.Unlock()
	out := make([]StatusView, len(keys))
	var wg sync.WaitGroup
	for i, k := range keys {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i], _ = m.status(k)
		}()
	}
	wg.Wait()
	// 查询期间被删除的实例 key 为空，去掉
	kept := out[:0]
	for _, v := range out {
		if v.Key != "" {
			kept = append(kept, v)
		}
	}
	return kept
}

func (m *Module) queryStatus(key string) StatusView {
	m.mu.Lock()
	in := m.findInstance(key)
	if in == nil {
		m.mu.Unlock()
		return StatusView{}
	}
	inst := *in
	s := m.cfg.Settings
	v := StatusView{Key: inst.Key, Name: inst.Name, Enable: inst.Enable, Peers: []PeerView{}}
	if rt := m.runs[key]; rt != nil {
		v.Daemon, v.Error = rt.daemon.State(), rt.getErr()
	}
	dir := m.store.Dir()
	m.mu.Unlock()

	cli, err := proc.Resolve(s.TailscalePath, dir, "tailscale")
	if err != nil {
		if v.Error == "" && inst.Enable {
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
	// 停用的实例不查询，避免对不存在的套接字反复执行命令
	if !inst.Enable {
		return v
	}
	st, err := runStatus(cli, inst.CLIArgs(dir))
	if err != nil {
		if v.Error == "" {
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

// InstanceView 是界面展示的实例，认证密钥已脱敏。
type InstanceView struct {
	Instance
	// State 为托管的 tailscaled 进程状态
	State proc.State `json:"state"`
	// Error 为启动流程的错误（含与其他实例冲突而未启动的原因）
	Error string `json:"error"`
	// DefaultStateDir 为 stateDir 留空时使用的状态目录
	DefaultStateDir string `json:"defaultStateDir"`
}

func (m *Module) instanceView(in *Instance) InstanceView {
	v := InstanceView{Instance: *in, DefaultStateDir: in.defaultStateDir(m.store.Dir())}
	v.AuthKey = crud.Mask(v.AuthKey)
	if v.AdvertiseRoutes == nil {
		v.AdvertiseRoutes = []string{}
	}
	if rt := m.runs[in.Key]; rt != nil {
		v.State = rt.daemon.State()
		v.Error = rt.getErr()
		if v.Error == "" {
			v.Error = v.State.Error
		}
	}
	return v
}

// SettingsView 是 GET /settings 的返回内容。
type SettingsView struct {
	Settings Settings `json:"settings"`
	// CLI 为实际使用的 tailscale 命令路径，找不到时为空
	CLI string `json:"cli"`
}

// Routes 实现 module.Module。
func (m *Module) Routes(r module.Router) {
	instances := &crud.Collection[Instance]{
		Mu:       &m.mu,
		Items:    &m.cfg.Instances,
		Save:     m.saveLocked,
		Key:      func(in *Instance) *string { return &in.Key },
		Enabled:  func(in *Instance) *bool { return &in.Enable },
		Group:    func(in *Instance) *string { return &in.Group },
		Validate: func(in *Instance) error { return in.Validate() },
		Check: func(in *Instance) error {
			return checkConflicts(m.cfg.Instances, in, m.store.Dir(), false)
		},
		CheckEnable: func(in *Instance) error {
			return checkConflicts(m.cfg.Instances, in, m.store.Dir(), false)
		},
		Merge: func(old, updated *Instance) {
			crud.KeepSecret(old.AuthKey, &updated.AuthKey)
			// 是否沿用旧版默认状态目录由服务端维护
			updated.LegacyStateDir = old.LegacyStateDir
		},
		Copy: func(src, dst *Instance) {
			crud.KeepSecret(src.AuthKey, &dst.AuthKey)
			// 副本使用自己的默认状态目录，不与模板共用节点身份
			dst.LegacyStateDir = false
		},
		View:    func(in *Instance) any { return m.instanceView(in) },
		Secrets: func(in *Instance) map[string]string { return map[string]string{"authKey": in.AuthKey} },
		Changed: m.applyLocked,
		Removed: m.removeLocked,
	}
	instances.Register(r, "/instances")

	// withInstance 包装按实例操作的接口：实例不存在时返回 404
	withInstance := func(fn func(w http.ResponseWriter, req *http.Request, key string)) http.HandlerFunc {
		return func(w http.ResponseWriter, req *http.Request) {
			key := req.PathValue("key")
			m.mu.Lock()
			exists := m.findInstance(key) != nil
			m.mu.Unlock()
			if !exists {
				httpx.Error(w, http.StatusNotFound, "实例不存在")
				return
			}
			fn(w, req, key)
		}
	}
	// 登录链接可把本机登记到任意 tailnet，只提供给有写权限的身份
	hideAuth := func(req *http.Request, v *StatusView) {
		if !auth.Allowed(req.Context(), ID+":write") {
			v.AuthURL = ""
		}
	}
	r.HandleFunc("GET /status", func(w http.ResponseWriter, req *http.Request) {
		list := m.statusAll()
		for i := range list {
			hideAuth(req, &list[i])
		}
		httpx.OK(w, list)
	})
	module.DocOf(r, "GET /status", module.Doc{Summary: "查看全部实例的连接状态", Resp: []StatusView{}})
	r.HandleFunc("GET /instances/{key}/status", withInstance(func(w http.ResponseWriter, req *http.Request, key string) {
		v, ok := m.status(key)
		if !ok {
			httpx.Error(w, http.StatusNotFound, "实例不存在")
			return
		}
		hideAuth(req, &v)
		httpx.OK(w, v)
	}))
	module.DocOf(r, "GET /instances/{key}/status", module.Doc{Summary: "查看实例的连接状态", Resp: &StatusView{}})
	r.HandleFunc("POST /instances/{key}/restart", withInstance(func(w http.ResponseWriter, req *http.Request, key string) {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.restartLocked(key)
		httpx.OK(w, map[string]bool{"ok": true})
	}))
	module.DocOf(r, "POST /instances/{key}/restart", module.Doc{Summary: "重启实例"})
	r.HandleFunc("POST /instances/{key}/logout", withInstance(func(w http.ResponseWriter, req *http.Request, key string) {
		m.mu.Lock()
		in := m.findInstance(key)
		if in == nil {
			m.mu.Unlock()
			httpx.Error(w, http.StatusNotFound, "实例不存在")
			return
		}
		inst, s, dir := *in, m.cfg.Settings, m.store.Dir()
		m.mu.Unlock()
		cli, err := proc.Resolve(s.TailscalePath, dir, "tailscale")
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		out, err := proc.Output(15*time.Second, cli, append(inst.CLIArgs(dir), "logout"), nil)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "退出登录失败: %s", strings.TrimSpace(string(out)))
			return
		}
		m.logs.Logger(cliSource(inst.Name)).Infof("已退出 tailnet 登录")
		httpx.OK(w, map[string]bool{"ok": true})
	}))
	module.DocOf(r, "POST /instances/{key}/logout", module.Doc{Summary: "退出实例的 tailnet 登录"})

	r.HandleFunc("GET /settings", func(w http.ResponseWriter, req *http.Request) {
		m.mu.Lock()
		s, dir := m.cfg.Settings, m.store.Dir()
		m.mu.Unlock()
		v := SettingsView{Settings: s}
		if cli, err := proc.Resolve(s.TailscalePath, dir, "tailscale"); err == nil {
			v.CLI = cli
		}
		httpx.OK(w, v)
	})
	r.HandleFunc("PUT /settings", func(w http.ResponseWriter, req *http.Request) {
		var s Settings
		if err := httpx.ReadJSON(req, &s); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		s.Validate()
		m.mu.Lock()
		defer m.mu.Unlock()
		old := m.cfg.Settings
		// 下载代理只由安装接口修改
		s.InstallProxy = old.InstallProxy
		m.cfg.Settings = s
		if err := m.saveLocked(); err != nil {
			m.cfg.Settings = old
			httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
			return
		}
		// 程序路径属于运行参数签名，变化后已启用的实例按签名重启
		m.syncLocked(func(string) bool { return false })
		httpx.OK(w, map[string]bool{"ok": true})
	})
	module.DocOf(r, "GET /settings", module.Doc{Summary: "读取程序路径", Resp: &SettingsView{}})
	module.DocOf(r, "PUT /settings", module.Doc{Summary: "保存程序路径", Req: &Settings{}})
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

// logoutLocked 在 Gately 托管的 tailscaled 运行中时退出 tailnet 登录（尽力而为），本机会从 tailnet 中移除。
// 调用方持有 mu。
func (m *Module) logoutLocked(rt *instRuntime) {
	in := rt.inst
	if !in.ManageDaemon || !rt.daemon.State().Running {
		return
	}
	dir := m.store.Dir()
	cli, err := proc.Resolve(m.cfg.Settings.TailscalePath, dir, "tailscale")
	if err != nil {
		return
	}
	if out, err := proc.Output(15*time.Second, cli, append(in.CLIArgs(dir), "logout"), nil); err != nil {
		m.logs.Logger(cliSource(in.Name)).Warnf("退出 tailnet 登录失败: %s", strings.TrimSpace(string(out)))
	}
}

// removeStateLocked 删除实例在配置目录内的默认状态目录（登录状态与节点密钥）。自定义状态目录可能是其他程序
// 在用的已有目录，迁移自旧版的默认目录 <配置目录>/tailscale 还包含其他实例的目录，这两种只提示不删除。
func (m *Module) removeStateLocked(in Instance) error {
	log := m.logs.Logger("tailscale")
	dir := m.store.Dir()
	switch {
	case in.StateDir != "":
		log.Warnf("实例 %s 的自定义状态目录 %s 未删除，如不再需要请手动删除", in.Name, in.StateDir)
	case in.LegacyStateDir:
		log.Warnf("实例 %s 的状态目录 %s 未删除，如不再需要请手动删除", in.Name, in.defaultStateDir(dir))
	default:
		if p := in.defaultStateDir(dir); p != "" {
			if err := os.RemoveAll(p); err != nil {
				return fmt.Errorf("删除状态目录失败: %v", err)
			}
		}
	}
	return nil
}

// removeLocked 在实例删除后停止进程。使用配置目录内默认状态目录的托管实例会先退出登录、再删除该目录
// （这个目录只属于该实例，删除后不会再被使用）；自定义状态目录保留登录状态，便于新建实例时沿用节点身份。调用方持有 mu。
func (m *Module) removeLocked(key string) {
	rt := m.runs[key]
	if rt == nil {
		return
	}
	in := rt.inst
	if in.StateDir == "" && !in.LegacyStateDir {
		m.logoutLocked(rt)
	}
	stopRuntime(rt)
	delete(m.runs, key)
	if err := m.removeStateLocked(in); err != nil {
		m.logs.Logger("tailscale").Warnf("%v", err)
	}
	// 删除可能解除了与其他实例的冲突
	m.syncLocked(func(string) bool { return false })
}

// resetLocked 恢复默认配置：Gately 托管的 tailscaled 正在运行时先退出 tailnet 登录（尽力而为），
// 再停止全部实例、清空实例与程序设置并删除默认状态目录中的登录状态。自定义的状态目录与系统已有的 tailscaled
// 不会被删除或登出；安装的程序不受影响。调用方持有 mu。
func (m *Module) resetLocked() error {
	old := m.cfg
	dir := m.store.Dir()
	for _, in := range old.Instances {
		if rt := m.runs[in.Key]; rt != nil && in.StateDir == "" {
			m.logoutLocked(rt)
		}
	}
	m.cfg = fileConfig{}
	if err := m.saveLocked(); err != nil {
		m.cfg = old
		return fmt.Errorf("保存失败: %v", err)
	}
	// 实例已全部删除，sync 会停止全部进程
	m.syncLocked(func(string) bool { return false })
	m.versions = map[string]string{}
	for _, in := range old.Instances {
		if in.LegacyStateDir && in.StateDir == "" {
			// 旧版默认目录就是 <配置目录>/tailscale，恢复默认时整个删除
			continue
		}
		if err := m.removeStateLocked(in); err != nil {
			return err
		}
	}
	// <配置目录>/tailscale 只存放默认状态目录，全部删除；自定义状态目录恰好位于其中时保留，只提示
	base := filepath.Join(dir, "tailscale")
	for _, in := range old.Instances {
		if in.StateDir != "" && (in.StateDir == base || strings.HasPrefix(in.StateDir, base+string(filepath.Separator))) {
			m.logs.Logger("tailscale").Warnf("自定义状态目录 %s 位于 %s 中，%s 未删除", in.StateDir, base, base)
			m.logs.Logger("tailscale").Infof("已恢复默认配置")
			return nil
		}
	}
	if err := os.RemoveAll(base); err != nil {
		return fmt.Errorf("删除状态目录失败: %v", err)
	}
	m.logs.Logger("tailscale").Infof("已恢复默认配置")
	return nil
}

// Summary 实现 module.Summarizer。
func (m *Module) Summary() []module.SummaryItem {
	m.mu.Lock()
	var keys []string
	for _, in := range m.cfg.Instances {
		if in.Enable {
			keys = append(keys, in.Key)
		}
	}
	m.mu.Unlock()
	if len(keys) == 0 {
		return []module.SummaryItem{{Label: "状态", Value: "未启用", Level: "info"}}
	}
	if len(keys) == 1 {
		return m.instanceSummary(keys[0])
	}
	// 多个实例时汇总已连接的数量，有异常时提示
	connected, failed := 0, 0
	for _, k := range keys {
		items := m.instanceSummary(k)
		switch {
		case len(items) > 0 && items[0].Level == "ok":
			connected++
		case len(items) > 0 && items[0].Level == "error":
			failed++
		}
	}
	level := "warn"
	switch {
	case failed > 0:
		level = "error"
	case connected == len(keys):
		level = "ok"
	}
	return []module.SummaryItem{{Label: "状态", Value: fmt.Sprintf("%d / %d 已连接", connected, len(keys)), Level: level}}
}

// instanceSummary 返回一个已启用实例的摘要，第一项为状态。
func (m *Module) instanceSummary(key string) []module.SummaryItem {
	m.mu.Lock()
	in := m.findInstance(key)
	if in == nil {
		m.mu.Unlock()
		return nil
	}
	managed := in.ManageDaemon
	var err string
	running := false
	if rt := m.runs[key]; rt != nil {
		err, running = rt.getErr(), rt.daemon.State().Running
	}
	m.mu.Unlock()
	switch {
	case err != "":
		return []module.SummaryItem{{Label: "状态", Value: "异常", Level: "error"}}
	case managed && !running:
		return []module.SummaryItem{{Label: "守护进程", Value: "未运行", Level: "warn"}}
	}
	// 后端状态需要执行命令，status 内部带短期缓存，总览页轮询不会反复执行
	st, _ := m.status(key)
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
