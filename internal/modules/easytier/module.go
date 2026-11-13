// Package easytier 托管 EasyTier 的 easytier-core 进程，并通过 easytier-cli 读取节点状态，
// 把本机加入 EasyTier 组网（P2P 打洞优先，必要时经公共节点或自建节点中继）。
// 支持多个实例：每个实例是一个独立的 easytier-core 进程，可分别加入不同的组网。
package easytier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/binstall"
	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/openports"
	"github.com/wangwanjie/gately/internal/outbound"
	"github.com/wangwanjie/gately/internal/proc"
)

// ID 是模块标识。
const ID = "easytier"

// 发布包中的两个程序名。
const (
	coreName = "easytier-core"
	cliName  = "easytier-cli"
)

// statusTimeout 是单次 easytier-cli 调用的超时。
const statusTimeout = 6 * time.Second

// statusCacheTTL 是状态缓存的有效期，总览页与状态页轮询时不会反复执行命令。
const statusCacheTTL = 2 * time.Second

// instRuntime 是一个实例的运行时信息。
type instRuntime struct {
	daemon *proc.Supervisor
	// sig 是最近一次按配置启动时的运行参数签名，实例停用或模块未启动时为空；签名不变时不重启
	sig string
	// conflict 是最近一次启动前检查到的冲突，非空时实例未启动
	conflict string
	// inst 是最近一次应用的实例配置
	inst Instance
	// rpcAddr 是本次启动给 easytier-core 使用的管理端口地址（127.0.0.1:端口）
	rpcAddr string
	// errMu 保护 err，启动流程与状态查询都会读写
	errMu sync.Mutex
	err   string
	// cache 缓存最近一次状态；由模块锁保护
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

// Module 是 EasyTier 模块。
type Module struct {
	mu      sync.Mutex
	store   *config.Store
	logs    *logbuf.Buffer
	cfg     fileConfig
	runs    map[string]*instRuntime
	started bool
	// versions 缓存各程序路径的版本
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
		httpClient: func() *http.Client { return outbound.DefaultClient() },
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
func (m *Module) Title() string { return "EasyTier 组网" }

func (m *Module) moduleLog() *logbuf.Logger {
	return m.logs.Logger("easytier")
}

// Init 实现 module.Module。
func (m *Module) Init(env module.Env) error {
	m.store, m.logs = env.Store, env.Logs
	m.install = m.newInstaller()
	_, err := m.store.Load(ID, &m.cfg)
	return err
}

func (m *Module) saveLocked() error {
	if m.cfg.Instances == nil {
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
		rt.daemon.Stop()
		delete(m.runs, key)
		openports.Clear(openOwner(key))
	}
	m.mu.Unlock()
	return nil
}

func (m *Module) findInstance(key string) *Instance {
	for i := range m.cfg.Instances {
		if m.cfg.Instances[i].Key == key {
			return &m.cfg.Instances[i]
		}
	}
	return nil
}

// openOwner 是实例在 openports 中的登记名。
func openOwner(key string) string { return "easytier:" + key }

// logSource 返回实例的日志来源，例如 easytier:家里。
func logSource(name string) string { return "easytier:" + name }

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
		CorePath string
	}{v, m.cfg.Settings.CorePath})
	return string(b)
}

// syncLocked 让全部实例与配置一致：签名或冲突状态变化的实例重启，force 返回 true 的实例无条件重启，
// 已删除实例的进程停止。调用方持有 mu。
func (m *Module) syncLocked(force func(key string) bool) {
	keep := map[string]bool{}
	for i := range m.cfg.Instances {
		in := &m.cfg.Instances[i]
		keep[in.Key] = true
		sig := m.signature(in)
		conflict := ""
		if sig != "" {
			if err := checkConflicts(m.cfg.Instances, in, true); err != nil {
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
			rt.daemon.Stop()
			delete(m.runs, key)
			openports.Clear(openOwner(key))
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

// freePort 返回 127.0.0.1 上一个当前空闲的端口，供管理端口使用。
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// secretLineRe 匹配 easytier-core 启动时打印的配置中含密钥的行，例如 network_secret = "xxx"。
var secretLineRe = regexp.MustCompile(`(?i)^(\s*(?:network_secret|local_private_key|local_public_key|credential\w*|\w*secret\w*|\w*password\w*|\w*token\w*)\s*=\s*).+$`)

// tunErrRe 匹配 easytier-core 创建 TUN 网卡失败时的日志，例如 tun device error err=rust tun error Operation not permitted (os error 1)。
var tunErrRe = regexp.MustCompile(`tun device error err=(.+)$`)

// maskLine 在写入日志前隐藏启动配置里的密钥：日志对只读身份可见，网络密钥可让他人加入组网。
func maskLine(line string) string {
	return secretLineRe.ReplaceAllString(line, "${1}"+`"`+crud.SecretMask+`"`)
}

// resolveCore 与 resolveCLI 查找两个程序；easytier-cli 优先与 easytier-core 放在同一目录。
func (m *Module) resolveCore() (string, error) {
	return proc.Resolve(m.cfg.Settings.CorePath, m.store.Dir(), coreName)
}

func (m *Module) resolveCLI(core string) (string, error) {
	if core != "" {
		cli := strings.TrimSuffix(core, binstall.ExeName(coreName)) + binstall.ExeName(cliName)
		if cli != core {
			if p, err := proc.Resolve(cli, m.store.Dir(), cliName); err == nil {
				return p, nil
			}
		}
	}
	return proc.Resolve("", m.store.Dir(), cliName)
}

// runInstanceLocked 按签名重建实例的运行状态：sig 为空表示应停止，conflict 非空时不启动并记录原因。调用方持有 mu。
func (m *Module) runInstanceLocked(in *Instance, sig, conflict string) {
	if rt := m.runs[in.Key]; rt != nil {
		rt.daemon.Stop()
	}
	// 名称可能被修改，重建托管器以使用新的日志来源
	rt := &instRuntime{daemon: proc.New(m.logs.Logger(logSource(in.Name))), sig: sig, conflict: conflict, inst: *in}
	m.runs[in.Key] = rt
	// 进程停止或重建时先撤销开放端口登记，启动成功后按新参数重新登记
	openports.Clear(openOwner(in.Key))
	if sig == "" {
		return
	}
	log := m.logs.Logger(logSource(in.Name))
	fail := func(msg string) {
		rt.putErr(msg)
		log.Errorf("%s", msg)
	}
	if conflict != "" {
		fail(conflict)
		return
	}
	core, err := m.resolveCore()
	if err != nil {
		fail(err.Error())
		return
	}
	port, err := freePort()
	if err != nil {
		fail("无法分配管理端口: " + err.Error())
		return
	}
	rt.rpcAddr = "127.0.0.1:" + strconv.Itoa(port)
	openports.SetPorts(openOwner(in.Key), in.tcpOpenPorts())
	rt.daemon.Start(proc.Spec{
		Path: core, Args: in.Args(rt.rpcAddr), Env: in.EnvList(), LogLine: maskLine,
		Module: ID, Name: in.Name,
		// 进程每次（重新）启动时清除上一次运行留下的错误：例如补装 TUN 驱动后被守护重启，旧的提示不应一直显示
		OnStart: func() { rt.putErr("") },
		// 创建 TUN 网卡失败时进程不会退出，只是没有虚拟 IP、其他节点也无法互通，把原因显示在实例上
		OnLine: func(line string) {
			if m := tunErrRe.FindStringSubmatch(line); m != nil {
				rt.putErr(fmt.Sprintf("无法创建 TUN 网卡: %s。需要 root 权限与 TUN 设备（Docker 需要 NET_ADMIN 与 /dev/net/tun），或开启“用户态网络”", m[1]))
			}
		},
	})
}

// rawPeer 对应 easytier-cli -o json peer 中的一项。
type rawPeer struct {
	IPv4     string `json:"ipv4"`
	Hostname string `json:"hostname"`
	Cost     string `json:"cost"`
	LatMs    string `json:"lat_ms"`
	Loss     string `json:"loss_rate"`
	Rx       string `json:"rx_bytes"`
	Tx       string `json:"tx_bytes"`
	Tunnel   string `json:"tunnel_proto"`
	NAT      string `json:"nat_type"`
	ID       string `json:"id"`
	Version  string `json:"version"`
}

// rawNode 对应 easytier-cli -o json node 中需要的字段。输出里的 config 字段含网络密钥，这里刻意不声明。
type rawNode struct {
	Hostname  string   `json:"hostname"`
	IPv4      string   `json:"ipv4_addr"`
	Listeners []string `json:"listeners"`
	Version   string   `json:"version"`
	StunInfo  struct {
		PublicIP []string `json:"public_ip"`
	} `json:"stun_info"`
}

// runJSON 执行 easytier-cli -p 管理端口 -o json 子命令并解析输出。
func runJSON(cli, rpcAddr string, out any, sub ...string) error {
	args := append([]string{"-p", rpcAddr, "-o", "json"}, sub...)
	data, err := proc.Output(statusTimeout, cli, args, nil)
	text := string(data)
	// 输出中可能夹带日志行，从第一个 { 或 [ 开始解析
	if i := strings.IndexAny(text, "{["); i >= 0 {
		if jerr := json.Unmarshal([]byte(text[i:]), out); jerr == nil {
			return nil
		}
	}
	msg := lastLine(text)
	if msg == "" && err != nil {
		msg = err.Error()
	}
	return errors.New(msg)
}

// lastLine 返回文本中最后一个非空行，作为命令失败的原因；easytier-cli 的错误信息是多行的，最后一行最具体。
func lastLine(text string) string {
	ls := strings.Split(strings.TrimSpace(text), "\n")
	for i := len(ls) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(ls[i]); s != "" {
			return s
		}
	}
	return ""
}

// stripPrefixLen 把 10.126.126.2/24 这样的接口地址转为纯 IP，无法解析时原样返回。
func stripPrefixLen(s string) string {
	if p, err := netip.ParsePrefix(s); err == nil {
		return p.Addr().String()
	}
	return s
}

// PeerView 是界面展示的节点。
type PeerView struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
	// Cost 为连接方式：p2p 为直连，relay(N) 为经 N 跳中继
	Cost string `json:"cost"`
	// Direct 表示直连（p2p）
	Direct bool `json:"direct"`
	// LatencyMs 为延迟毫秒数，0 表示未知
	LatencyMs float64 `json:"latencyMs,omitempty"`
	Loss      string  `json:"loss,omitempty"`
	Rx        string  `json:"rx,omitempty"`
	Tx        string  `json:"tx,omitempty"`
	Tunnel    string  `json:"tunnel,omitempty"`
	NAT       string  `json:"nat,omitempty"`
	Version   string  `json:"version,omitempty"`
}

func peerView(p rawPeer) PeerView {
	v := PeerView{Name: p.Hostname, IP: stripPrefixLen(p.IPv4), Cost: p.Cost, Direct: p.Cost == "p2p",
		Loss: dash(p.Loss), Rx: dash(p.Rx), Tx: dash(p.Tx), Tunnel: dash(p.Tunnel), NAT: dash(p.NAT), Version: p.Version}
	if v.Name == "" {
		v.Name = v.IP
	}
	if f, err := strconv.ParseFloat(strings.TrimSpace(p.LatMs), 64); err == nil && f > 0 {
		v.LatencyMs = f
	}
	return v
}

// dash 把 easytier-cli 用来表示“无数据”的 - 转为空串。
func dash(s string) string {
	if s == "-" {
		return ""
	}
	return s
}

// SelfView 是本机在组网中的信息。
type SelfView struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
	// Listeners 为本机实际监听的地址（不含进程内部的 ring 通道）
	Listeners []string `json:"listeners"`
	// PublicIPs 为经 STUN 探测到的公网地址
	PublicIPs []string `json:"publicIps"`
	NAT       string   `json:"nat,omitempty"`
}

// StatusView 是一个实例的运行状态。
type StatusView struct {
	// Key、Name 为实例的 key 与名称
	Key     string     `json:"key"`
	Name    string     `json:"name"`
	Enable  bool       `json:"enable"`
	Daemon  proc.State `json:"daemon"`
	CLI     string     `json:"cli"`
	Core    string     `json:"core"`
	Version string     `json:"version"`
	// Error 为启动流程或状态查询的错误
	Error string `json:"error"`
	// Status 为 stopped（未启用或未运行）、starting（进程已启动、管理端口尚未就绪）或 running
	Status string    `json:"status"`
	Self   *SelfView `json:"self,omitempty"`
	// PeersDirect 为直连的节点数
	PeersDirect int        `json:"peersDirect"`
	Peers       []PeerView `json:"peers"`
}

// status 返回实例的状态，带短期缓存；实例不存在时 ok 为 false。
func (m *Module) status(key string) (v StatusView, ok bool) {
	m.mu.Lock()
	if m.findInstance(key) == nil {
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
	v := StatusView{Key: inst.Key, Name: inst.Name, Enable: inst.Enable, Peers: []PeerView{}, Status: "stopped"}
	rpc := ""
	if rt := m.runs[key]; rt != nil {
		v.Daemon, v.Error = rt.daemon.State(), rt.getErr()
		rpc = rt.rpcAddr
	}
	m.mu.Unlock()

	m.mu.Lock()
	core, cerr := m.resolveCore()
	m.mu.Unlock()
	if cerr != nil {
		if v.Error == "" && inst.Enable {
			v.Error = cerr.Error()
		}
		return v
	}
	v.Core = core
	m.mu.Lock()
	ver, ok := m.versions[core]
	m.mu.Unlock()
	if !ok {
		ver = versionOf(core)
		m.mu.Lock()
		m.versions[core] = ver
		m.mu.Unlock()
	}
	v.Version = ver
	if v.Daemon.Error != "" && v.Error == "" {
		v.Error = v.Daemon.Error
	}
	if v.Error == "" && inst.Enable {
		v.Error = exitedMsg(v.Daemon)
	}
	// 停用的实例、未运行的进程或启动失败的实例不查询
	if !inst.Enable || rpc == "" || !v.Daemon.Running {
		return v
	}
	m.mu.Lock()
	cli, err := m.resolveCLI(core)
	m.mu.Unlock()
	if err != nil {
		if v.Error == "" {
			v.Error = err.Error()
		}
		return v
	}
	v.CLI = cli
	v.Status = "starting"
	var peers []rawPeer
	if err := runJSON(cli, rpc, &peers, "peer"); err != nil {
		// 进程刚启动时管理端口还没就绪，属于正常的启动中状态，不报错
		return v
	}
	v.Status = "running"
	selfNAT := ""
	for _, p := range peers {
		if p.Cost == "Local" {
			selfNAT = dash(p.NAT)
			continue
		}
		pv := peerView(p)
		if pv.Direct {
			v.PeersDirect++
		}
		v.Peers = append(v.Peers, pv)
	}
	sort.SliceStable(v.Peers, func(i, j int) bool {
		if v.Peers[i].Direct != v.Peers[j].Direct {
			return v.Peers[i].Direct
		}
		return v.Peers[i].Name < v.Peers[j].Name
	})
	var node rawNode
	if err := runJSON(cli, rpc, &node, "node"); err == nil {
		self := &SelfView{Name: node.Hostname, IP: stripPrefixLen(node.IPv4), Listeners: []string{}, PublicIPs: node.StunInfo.PublicIP, NAT: selfNAT}
		if self.PublicIPs == nil {
			self.PublicIPs = []string{}
		}
		for _, l := range node.Listeners {
			if !strings.HasPrefix(l, "ring://") {
				self.Listeners = append(self.Listeners, l)
			}
		}
		v.Self = self
	}
	return v
}

// versionRe 取 easytier-core --version 的版本号。
var versionRe = regexp.MustCompile(`\d+\.\d+\.\d+\S*`)

func versionOf(core string) string {
	return versionRe.FindString(proc.Version(core, "--version"))
}

// exitedMsg 返回进程退出后的提示：参数错误等原因会让 easytier-core 启动即退出，由守护自动重试，界面需要显示出来。
func exitedMsg(st proc.State) string {
	if st.Running || st.LastExit == "" {
		return ""
	}
	return fmt.Sprintf("进程已退出（%s），将自动重试，原因请查看日志", st.LastExit)
}

// InstanceView 是界面展示的实例，网络密钥已脱敏。
type InstanceView struct {
	Instance
	// State 为托管的进程状态
	State proc.State `json:"state"`
	// Error 为启动流程的错误（含与其他实例冲突而未启动的原因）
	Error string `json:"error"`
}

func (m *Module) instanceView(in *Instance) InstanceView {
	v := InstanceView{Instance: *in}
	v.NetworkSecret = crud.Mask(v.NetworkSecret)
	if rt := m.runs[in.Key]; rt != nil {
		v.State = rt.daemon.State()
		v.Error = rt.getErr()
		if v.Error == "" {
			v.Error = v.State.Error
		}
		if v.Error == "" && in.Enable {
			v.Error = exitedMsg(v.State)
		}
	}
	return v
}

// SettingsView 是 GET /settings 的返回内容。
type SettingsView struct {
	Settings Settings `json:"settings"`
	// Core 为实际使用的 easytier-core 程序路径，找不到时为空
	Core string `json:"core"`
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
			return checkConflicts(m.cfg.Instances, in, false)
		},
		CheckEnable: func(in *Instance) error {
			return checkConflicts(m.cfg.Instances, in, false)
		},
		Merge: func(old, updated *Instance) {
			crud.KeepSecret(old.NetworkSecret, &updated.NetworkSecret)
		},
		Copy: func(src, dst *Instance) {
			crud.KeepSecret(src.NetworkSecret, &dst.NetworkSecret)
		},
		View:    func(in *Instance) any { return m.instanceView(in) },
		Secrets: func(in *Instance) map[string]string { return map[string]string{"networkSecret": in.NetworkSecret} },
		Changed: m.applyLocked,
		Removed: m.removeLocked,
	}
	instances.Register(r, "/instances")

	r.HandleFunc("GET /status", func(w http.ResponseWriter, req *http.Request) {
		httpx.OK(w, m.statusAll())
	})
	module.DocOf(r, "GET /status", module.Doc{Summary: "查看全部实例的运行状态", Resp: []StatusView{}})
	r.HandleFunc("GET /instances/{key}/status", func(w http.ResponseWriter, req *http.Request) {
		v, ok := m.status(req.PathValue("key"))
		if !ok {
			httpx.Error(w, http.StatusNotFound, "实例不存在")
			return
		}
		httpx.OK(w, v)
	})
	module.DocOf(r, "GET /instances/{key}/status", module.Doc{Summary: "查看实例的运行状态", Resp: &StatusView{}})
	r.HandleFunc("POST /instances/{key}/restart", func(w http.ResponseWriter, req *http.Request) {
		key := req.PathValue("key")
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.findInstance(key) == nil {
			httpx.Error(w, http.StatusNotFound, "实例不存在")
			return
		}
		m.syncLocked(func(k string) bool { return k == key })
		httpx.OK(w, map[string]bool{"ok": true})
	})
	module.DocOf(r, "POST /instances/{key}/restart", module.Doc{Summary: "重启实例"})

	r.HandleFunc("GET /settings", func(w http.ResponseWriter, req *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		v := SettingsView{Settings: m.cfg.Settings}
		if core, err := m.resolveCore(); err == nil {
			v.Core = core
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

// removeLocked 在实例删除后停止进程；删除可能解除了与其他实例的冲突。调用方持有 mu。
func (m *Module) removeLocked(key string) {
	if rt := m.runs[key]; rt != nil {
		rt.daemon.Stop()
		delete(m.runs, key)
	}
	m.syncLocked(func(string) bool { return false })
}

// resetLocked 恢复默认配置：停止全部实例，清空实例与程序设置。EasyTier 不在 Gately 的配置目录里保存其他状态，
// 安装的程序不受影响。调用方持有 mu。
func (m *Module) resetLocked() error {
	old := m.cfg
	m.cfg = fileConfig{}
	if err := m.saveLocked(); err != nil {
		m.cfg = old
		return fmt.Errorf("保存失败: %v", err)
	}
	// 实例已全部删除，sync 会停止全部进程
	m.syncLocked(func(string) bool { return false })
	m.versions = map[string]string{}
	m.moduleLog().Infof("已恢复默认配置")
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
	// 多个实例时汇总运行中的数量，有异常时提示
	running, failed := 0, 0
	for _, k := range keys {
		items := m.instanceSummary(k)
		switch {
		case len(items) > 0 && items[0].Level == "ok":
			running++
		case len(items) > 0 && items[0].Level == "error":
			failed++
		}
	}
	level := "warn"
	switch {
	case failed > 0:
		level = "error"
	case running == len(keys):
		level = "ok"
	}
	return []module.SummaryItem{{Label: "状态", Value: fmt.Sprintf("%d / %d 运行中", running, len(keys)), Level: level}}
}

// instanceSummary 返回一个已启用实例的摘要，第一项为状态。
func (m *Module) instanceSummary(key string) []module.SummaryItem {
	m.mu.Lock()
	in := m.findInstance(key)
	if in == nil {
		m.mu.Unlock()
		return nil
	}
	var err string
	running := false
	if rt := m.runs[key]; rt != nil {
		err, running = rt.getErr(), rt.daemon.State().Running
	}
	m.mu.Unlock()
	switch {
	case err != "":
		return []module.SummaryItem{{Label: "状态", Value: "异常", Level: "error"}}
	case !running:
		return []module.SummaryItem{{Label: "进程", Value: "未运行", Level: "warn"}}
	}
	// 状态需要执行命令，status 内部带短期缓存，总览页轮询不会反复执行
	st, _ := m.status(key)
	if st.Status == "running" {
		return []module.SummaryItem{
			{Label: "状态", Value: "运行中", Level: "ok"},
			{Label: "节点", Value: strconv.Itoa(st.PeersDirect) + "/" + strconv.Itoa(len(st.Peers))},
		}
	}
	return []module.SummaryItem{{Label: "状态", Value: "启动中", Level: "warn"}}
}
