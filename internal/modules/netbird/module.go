// Package netbird 托管 NetBird 守护进程并通过 netbird 命令完成登录与状态展示，
// 把本机加入 NetBird 官方云或自建管理服务的组网。
// 支持多个实例：每个实例是一个独立的守护进程（状态目录、控制地址与进程互不相同），可分别加入不同的 NetBird 网络。
package netbird

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
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
const ID = "netbird"

// readyTimeout 是等待守护进程就绪多久后提示超时（之后继续等待）；测试中缩短
var readyTimeout = 30 * time.Second

const (
	statusTimeout = 8 * time.Second
	// loginRetryDelay 是登录链接过期后重新获取的间隔
	loginRetryDelay = 3 * time.Second
	// upRetryMax 是 netbird up 失败后重试的最长间隔
	upRetryMax = 5 * time.Minute
)

// upRetryMin 是 netbird up 失败（例如开机时网络尚未就绪、管理服务暂时不可达）后第一次重试的间隔，之后加倍：
// 托管时关闭了守护进程自身的自动连接，需要由这里重试。测试中缩短
var upRetryMin = 10 * time.Second

// instRuntime 是一个实例的运行时信息。
type instRuntime struct {
	daemon *proc.Supervisor
	// sig 是最近一次按配置启动时的运行参数签名，实例停用或模块未启动时为空；签名不变时不重启
	sig string
	// conflict 是最近一次启动前检查到的冲突，非空时实例未启动
	conflict string
	// inst 是最近一次应用的实例配置，删除实例时据此退出登录与清理状态目录
	inst Instance
	// upCancel 取消正在进行的 netbird up
	upCancel context.CancelFunc
	upDone   chan struct{}
	// err、authURL 与 authCode 由 errMu 保护：启动协程需要写入，而停止流程在持有模块锁时等待该协程退出，
	// 共用模块锁会死锁
	errMu sync.Mutex
	err   string
	// authURL 与 authCode 是 netbird up 输出的登录链接与设备码，登录完成或重新应用配置后清空
	authURL  string
	authCode string
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

func (rt *instRuntime) setAuth(url, code string) {
	rt.errMu.Lock()
	rt.authURL, rt.authCode = url, code
	rt.errMu.Unlock()
}

func (rt *instRuntime) getAuth() (string, string) {
	rt.errMu.Lock()
	defer rt.errMu.Unlock()
	return rt.authURL, rt.authCode
}

// Module 是 NetBird 模块。
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
func (m *Module) Title() string { return "NetBird 组网" }

// moduleLog 返回模块级（与具体实例无关）的日志。
func (m *Module) moduleLog() *logbuf.Logger {
	return m.logs.Logger("netbird")
}

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
			m.moduleLog().Warnf("保存迁移后的 NetBird 配置失败: %v", err)
		} else {
			m.moduleLog().Infof("已把旧版单个 NetBird 配置迁移为实例")
		}
	}
	return nil
}

// loadConfig 读取 netbird.json：新格式含 instances 字段；旧版单实例配置（顶层即为全部字段）迁移为一个实例，
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
	// 旧版保存时总是写出全部字段，以默认参数为底只是为了兼容手写的不完整配置
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

// stopRuntime 取消进行中的 up 并停止托管的守护进程，清空登录链接。调用方持有模块锁。
func stopRuntime(rt *instRuntime) {
	if rt.upCancel != nil {
		rt.upCancel()
		<-rt.upDone
		rt.upCancel, rt.upDone = nil, nil
	}
	rt.daemon.Stop()
	rt.setAuth("", "")
}

func (m *Module) findInstance(key string) *Instance {
	for i := range m.cfg.Instances {
		if m.cfg.Instances[i].Key == key {
			return &m.cfg.Instances[i]
		}
	}
	return nil
}

// daemonSource 与 cliSource 返回实例的日志来源，例如 daemon:家里、netbird:家里。
func daemonSource(name string) string { return "daemon:" + name }

func cliSource(name string) string { return "netbird:" + name }

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
		NetbirdPath string
	}{v, m.cfg.Settings.NetbirdPath})
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

// restartLocked 重启指定实例：重新启动托管的守护进程并再次执行 netbird up。调用方持有 mu。
func (m *Module) restartLocked(key string) {
	m.syncLocked(func(k string) bool { return k == key })
}

// runInstanceLocked 按签名重建实例的运行状态：sig 为空表示应停止，conflict 非空时不启动并记录原因。调用方持有 mu。
func (m *Module) runInstanceLocked(in *Instance, sig, conflict string) {
	if rt := m.runs[in.Key]; rt != nil {
		stopRuntime(rt)
	}
	// 名称可能被修改，重建托管器以使用新的日志来源
	rt := &instRuntime{daemon: proc.New(m.logs.Logger(daemonSource(in.Name))), sig: sig, conflict: conflict, inst: *in}
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
	cli, err := proc.Resolve(m.cfg.Settings.NetbirdPath, dir, "netbird")
	if err != nil {
		fail(err.Error())
		return
	}
	if in.ManageDaemon && in.Userspace {
		m.warnPublicSocks(cli, log)
	}
	inst := *in
	// started 在托管的守护进程每次启动（包括异常退出后被重启）时收到信号，触发一次 netbird up：
	// 托管时关闭了守护进程的自动连接，由 Gately 负责连接
	started := make(chan struct{}, 1)
	var daemonSpec proc.Spec
	if inst.ManageDaemon {
		// 默认状态目录在配置目录内，收紧为 0700；自定义状态目录可能是已有的其他目录，只在不存在时创建，不改动已有目录的权限
		mkdir := os.MkdirAll
		if inst.StateDir == "" {
			mkdir = fsperm.MkdirAll
		}
		if err := mkdir(inst.stateDir(dir), 0o700); err != nil {
			fail("创建状态目录失败: " + err.Error())
			return
		}
		daemonSpec = proc.Spec{Path: cli, Args: inst.DaemonArgs(dir), Env: inst.DaemonEnvList(dir), OnStart: func() {
			select {
			case started <- struct{}{}:
			default:
			}
		}}
	}
	// 内核模式实例启动前检测系统中另外运行的内核模式 NetBird（见 external.go）；检测要执行外部命令，放到协程里，
	// 外部实例列表在持锁时取得
	var targets []externalTarget
	if inst.kernel() {
		targets = m.externalTargetsLocked(&inst)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	rt.upCancel, rt.upDone = cancel, done
	go func() {
		defer close(done)
		if !inst.ManageDaemon {
			bringUp(ctx, rt, log, cli, inst, dir)
			return
		}
		if inst.kernel() {
			if msg := detectExternalKernel(ctx, cli, targets); msg != "" {
				fail(msg)
				return
			}
			if ctx.Err() != nil {
				return
			}
		}
		rt.daemon.Start(daemonSpec)
		restarted := false
		for {
			select {
			case <-ctx.Done():
				return
			case <-started:
			}
			// 需要时重启一次守护进程，清除用户态网络下断开再连接后失效的 SOCKS5 监听；
			// 每次应用配置最多重启一次，避免自动连接没能关闭时反复重启
			if bringUp(ctx, rt, log, cli, inst, dir) && !restarted {
				restarted = true
				log.Infof("已关闭守护进程的自动连接，重启守护进程让 SOCKS5 代理生效")
				rt.daemon.Start(daemonSpec)
			}
		}
	}()
}

// socksAlwaysPublic 判断 NetBird 版本 ver 在用户态网络下是否总把 SOCKS5 代理监听在 0.0.0.0：
// 0.75.0 之前的版本忽略 NB_SOCKS5_LISTENER_ADDRESS，固定监听所有地址且没有认证。无法识别的版本返回 false。
func socksAlwaysPublic(ver string) bool {
	m := nbVersionRe.FindStringSubmatch(ver)
	if m == nil {
		return false
	}
	parts := strings.SplitN(m[1], ".", 3)
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}
	return major == 0 && minor < 75
}

// warnPublicSocks 在托管的守护进程使用用户态网络且 NetBird 版本过旧时，在该实例的日志中提示 SOCKS5 代理对外暴露；
// 调用方持有 mu。
func (m *Module) warnPublicSocks(cli string, log *logbuf.Logger) {
	ver, ok := m.versions[cli]
	if !ok {
		ver = proc.Version(cli, "version")
		m.versions[cli] = ver
	}
	if socksAlwaysPublic(ver) {
		log.Warnf("NetBird %s 在用户态网络下总是在 0.0.0.0 上提供无认证的 SOCKS5 代理（0.75.0 起才默认只监听本机），其他设备可经它访问组网，请升级 NetBird", ver)
	}
}

var (
	// urlRe 匹配 netbird up 输出中的登录链接
	urlRe = regexp.MustCompile(`https?://\S+`)
	// codeRe 匹配设备码登录时需要输入的代码（链接中已包含代码时不会输出）
	codeRe = regexp.MustCompile(`enter the code (\S+)`)
)

// parseAuthLine 从 netbird up 的一行输出中提取登录链接与设备码。
func parseAuthLine(line string) (url, code string) {
	url = urlRe.FindString(line)
	if c := codeRe.FindStringSubmatch(line); c != nil {
		code = strings.TrimRight(c[1], ".")
	}
	return url, code
}

// maskAuthLine 隐藏 netbird up 输出中的登录链接与设备码后再写入日志：日志对只读身份可见，
// 拿到链接与设备码即可把本机登记到他人的 NetBird 网络；完整内容只通过状态接口提供给有写权限的身份。
func maskAuthLine(line string) string {
	line = proc.MaskURLPaths(line)
	return codeRe.ReplaceAllString(line, "enter the code "+crud.SecretMask)
}

// bringUp 等待实例的守护进程就绪后执行 netbird up。未提供设置密钥时 up 会等待登录完成，
// 登录链接写入日志（已打码），并可通过状态接口获取；链接过期后自动重新获取。不使用模块锁。
// 返回 true 表示托管的守护进程需要重启：它按旧配置自动连接过，在用户态网络下断开再连接后 SOCKS5 代理失效。
func bringUp(ctx context.Context, rt *instRuntime, log *logbuf.Logger, cli string, in Instance, dir string) (restartDaemon bool) {
	fail := func(msg string) {
		rt.putErr(msg)
		log.Errorf("%s", msg)
	}
	global := in.CLIArgs(dir)
	deadline := time.Now().Add(readyTimeout)
	// timedOut 表示已经提示过等待超时，守护进程随后就绪时要清除该提示
	timedOut := false
	var st *rawStatus
	for {
		var err error
		if st, err = runStatus(cli, global); err == nil {
			break
		}
		if ctx.Err() != nil {
			return false
		}
		// 超时后提示错误但继续等待：守护进程可能在慢速设备上启动较慢，或由外部服务稍后启动
		if !timedOut && time.Now().After(deadline) {
			fail("等待 NetBird 守护进程就绪超时，请检查日志")
			timedOut = true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(time.Second):
		}
	}
	// 守护进程已就绪，超时提示不再成立；否则等待登录期间（up 阻塞到登录完成）页面会一直显示异常
	if timedOut {
		rt.putErr("")
	}
	// 已连接（守护进程按上次保存的配置自动连接）时 netbird up 只会提示 Already connected，
	// 既不应用也不保存新的配置，所以先断开。托管时 up 会保存“关闭自动连接”，之后守护进程启动后保持空闲，
	// 由这里连接；只有首次（例如从开启自动连接的旧配置升级）需要断开。用户态网络下同一进程内断开再连接时
	// 旧的 SOCKS5 监听不会释放，新连接无法监听，代理失效，因此 up 完成后要重启一次守护进程
	reconnect := st.DaemonStatus == "Connected" || st.DaemonStatus == "Connecting"
	if reconnect {
		if out, err := proc.Output(15*time.Second, cli, append(append([]string{}, global...), "down"), nil); err != nil {
			log.Warnf("断开连接失败: %s", strings.TrimSpace(string(out)))
		}
	}
	args := append(append([]string{}, global...), in.UpArgs()...)
	backoff := upRetryMin
	for {
		gotURL := false
		// lastLine 是 netbird up 的最后一行输出，失败时作为原因展示
		lastLine := ""
		spec := proc.Spec{Path: cli, Args: args, Env: in.UpEnv(), LogLine: maskAuthLine, OnLine: func(line string) {
			if line = strings.TrimSpace(line); line != "" {
				lastLine = line
			}
			if url, code := parseAuthLine(line); url != "" {
				gotURL = true
				rt.setAuth(url, code)
			}
		}}
		err := proc.Run(ctx, spec, log)
		rt.setAuth("", "")
		if ctx.Err() != nil {
			return false
		}
		if err == nil {
			rt.putErr("")
			log.Infof("netbird up 已完成")
			return reconnect && in.ManageDaemon && in.Userspace
		}
		delay := loginRetryDelay
		if gotURL {
			// 输出过登录链接说明在等待登录，失败多半是链接过期，重新获取
			log.Infof("登录链接已过期，重新获取")
		} else {
			reason := lastLine
			if reason == "" {
				reason = err.Error()
			}
			fail(fmt.Sprintf("netbird up 失败: %s，%s 后重试", reason, backoff))
			delay, backoff = backoff, min(backoff*2, upRetryMax)
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(delay):
		}
	}
}

// rawPeer 对应 netbird status --json 中的节点。
type rawPeer struct {
	FQDN             string        `json:"fqdn"`
	IP               string        `json:"netbirdIp"`
	IPv6             string        `json:"netbirdIpv6"`
	Status           string        `json:"status"`
	LastStatusUpdate time.Time     `json:"lastStatusUpdate"`
	ConnType         string        `json:"connectionType"`
	RelayAddress     string        `json:"relayAddress"`
	TransferReceived int64         `json:"transferReceived"`
	TransferSent     int64         `json:"transferSent"`
	Latency          time.Duration `json:"latency"`
	Networks         []string      `json:"networks"`
}

type rawConn struct {
	URL       string `json:"url"`
	Connected bool   `json:"connected"`
	Error     string `json:"error"`
}

type rawStatus struct {
	Peers struct {
		Total     int       `json:"total"`
		Connected int       `json:"connected"`
		Details   []rawPeer `json:"details"`
	} `json:"peers"`
	DaemonVersion string  `json:"daemonVersion"`
	DaemonStatus  string  `json:"daemonStatus"`
	Management    rawConn `json:"management"`
	Signal        rawConn `json:"signal"`
	Relays        struct {
		Total     int `json:"total"`
		Available int `json:"available"`
	} `json:"relays"`
	IP              string   `json:"netbirdIp"`
	IPv6            string   `json:"netbirdIpv6"`
	FQDN            string   `json:"fqdn"`
	KernelInterface bool     `json:"usesKernelInterface"`
	WgPort          int      `json:"wireguardPort"`
	Networks        []string `json:"networks"`
}

func runStatus(cli string, global []string) (*rawStatus, error) {
	return runStatusTimeout(cli, global, statusTimeout)
}

// runStatusTimeout 执行 netbird status --json 并解析结果，超时后放弃。
func runStatusTimeout(cli string, global []string, timeout time.Duration) (*rawStatus, error) {
	out, err := proc.Output(timeout, cli, append(append([]string{}, global...), "status", "--json"), nil)
	var st rawStatus
	// 输出中可能夹带日志行，从第一个 { 开始解析
	text := string(out)
	if i := strings.IndexByte(text, '{'); i >= 0 {
		if jerr := json.Unmarshal([]byte(text[i:]), &st); jerr == nil && st.DaemonStatus != "" {
			return &st, nil
		}
	}
	msg := strings.TrimSpace(text)
	if msg == "" && err != nil {
		msg = err.Error()
	}
	return nil, errors.New(msg)
}

// stripPrefixLen 把 100.92.1.2/16 这样的接口地址转为纯 IP，无法解析时原样返回。
func stripPrefixLen(s string) string {
	if p, err := netip.ParsePrefix(s); err == nil {
		return p.Addr().String()
	}
	return s
}

// ips 返回非空的 IPv4 与 IPv6 地址。
func ips(v4, v6 string) []string {
	out := []string{}
	for _, a := range []string{v4, v6} {
		if a != "" {
			out = append(out, stripPrefixLen(a))
		}
	}
	return out
}

// shortName 返回 FQDN 的第一段作为节点名称。
func shortName(fqdn string) string {
	name, _, _ := strings.Cut(strings.TrimSuffix(fqdn, "."), ".")
	return name
}

// PeerView 是界面展示的节点。
type PeerView struct {
	Name     string   `json:"name"`
	FQDN     string   `json:"fqdn"`
	IPs      []string `json:"ips"`
	Online   bool     `json:"online"`
	Status   string   `json:"status"`
	ConnType string   `json:"connType,omitempty"`
	Relay    string   `json:"relay,omitempty"`
	// LatencyMs 为延迟毫秒数，0 表示未知
	LatencyMs float64   `json:"latencyMs,omitempty"`
	Routes    []string  `json:"routes,omitempty"`
	RxBytes   int64     `json:"rxBytes"`
	TxBytes   int64     `json:"txBytes"`
	LastSeen  time.Time `json:"lastSeen,omitempty"`
}

func peerView(p rawPeer) PeerView {
	fqdn := strings.TrimSuffix(p.FQDN, ".")
	v := PeerView{
		Name: shortName(fqdn), FQDN: fqdn, IPs: ips(p.IP, p.IPv6), Online: p.Status == "Connected", Status: p.Status,
		Routes: p.Networks, RxBytes: p.TransferReceived, TxBytes: p.TransferSent,
	}
	if v.Name == "" {
		v.Name = stripPrefixLen(p.IP)
	}
	if p.ConnType != "-" {
		v.ConnType = p.ConnType
	}
	if p.ConnType == "Relayed" {
		v.Relay = p.RelayAddress
	}
	if p.Latency > 0 {
		v.LatencyMs = float64(p.Latency.Microseconds()) / 1000
	}
	if p.LastStatusUpdate.Year() > 1 {
		v.LastSeen = p.LastStatusUpdate
	}
	return v
}

// ConnView 是管理服务或信令服务的连接状态。
type ConnView struct {
	URL       string `json:"url"`
	Connected bool   `json:"connected"`
	Error     string `json:"error,omitempty"`
}

// SelfView 是本机在组网中的信息。
type SelfView struct {
	Name   string   `json:"name"`
	FQDN   string   `json:"fqdn"`
	IPs    []string `json:"ips"`
	WgPort int      `json:"wgPort,omitempty"`
	// Kernel 表示使用内核 WireGuard，否则为用户态实现
	Kernel bool `json:"kernel"`
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
	Error string `json:"error"`
	// DaemonStatus 为 Idle、Connecting、Connected、NeedsLogin、LoginFailed 或 SessionExpired
	DaemonStatus string `json:"daemonStatus"`
	AuthURL      string `json:"authUrl,omitempty"`
	// AuthCode 为设备码登录时需要在登录页输入的代码，链接已包含代码时为空
	AuthCode       string     `json:"authCode,omitempty"`
	Management     *ConnView  `json:"management,omitempty"`
	Signal         *ConnView  `json:"signal,omitempty"`
	RelaysTotal    int        `json:"relaysTotal"`
	RelaysUp       int        `json:"relaysUp"`
	Self           *SelfView  `json:"self,omitempty"`
	PeersConnected int        `json:"peersConnected"`
	Peers          []PeerView `json:"peers"`
}

// statusCacheTTL 是状态缓存的有效期。
const statusCacheTTL = 2 * time.Second

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
	s := m.cfg.Settings
	v := StatusView{Key: inst.Key, Name: inst.Name, Enable: inst.Enable, Peers: []PeerView{}}
	if rt := m.runs[key]; rt != nil {
		v.Daemon, v.Error = rt.daemon.State(), rt.getErr()
		v.AuthURL, v.AuthCode = rt.getAuth()
	}
	dir := m.store.Dir()
	m.mu.Unlock()

	cli, err := proc.Resolve(s.NetbirdPath, dir, "netbird")
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
	// 停用的实例不查询，避免对不存在的控制地址反复执行命令
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
	v.DaemonStatus = st.DaemonStatus
	if st.DaemonStatus == "Connected" {
		// 登录完成后 up 可能还在等待返回，不再展示登录链接
		v.AuthURL, v.AuthCode = "", ""
	}
	if st.Management.URL != "" {
		v.Management = &ConnView{URL: st.Management.URL, Connected: st.Management.Connected, Error: st.Management.Error}
	}
	if st.Signal.URL != "" {
		v.Signal = &ConnView{URL: st.Signal.URL, Connected: st.Signal.Connected, Error: st.Signal.Error}
	}
	v.RelaysTotal, v.RelaysUp = st.Relays.Total, st.Relays.Available
	if st.IP != "" {
		fqdn := strings.TrimSuffix(st.FQDN, ".")
		v.Self = &SelfView{Name: shortName(fqdn), FQDN: fqdn, IPs: ips(st.IP, st.IPv6), WgPort: st.WgPort, Kernel: st.KernelInterface}
	}
	for _, p := range st.Peers.Details {
		v.Peers = append(v.Peers, peerView(p))
	}
	v.PeersConnected = st.Peers.Connected
	sort.Slice(v.Peers, func(i, j int) bool {
		if v.Peers[i].Online != v.Peers[j].Online {
			return v.Peers[i].Online
		}
		return v.Peers[i].Name < v.Peers[j].Name
	})
	return v
}

// InstanceView 是界面展示的实例，设置密钥已脱敏。
type InstanceView struct {
	Instance
	// State 为托管的守护进程状态
	State proc.State `json:"state"`
	// Error 为启动流程的错误（含与其他实例冲突而未启动的原因）
	Error string `json:"error"`
	// DefaultStateDir 为 stateDir 留空时使用的状态目录，新增实例（尚无 key）时为空
	DefaultStateDir string `json:"defaultStateDir"`
	// DefaultManagementURL 为 managementUrl 留空时使用的 NetBird 官方云地址
	DefaultManagementURL string `json:"defaultManagementUrl"`
}

func (m *Module) instanceView(in *Instance) InstanceView {
	v := InstanceView{Instance: *in, DefaultStateDir: in.defaultStateDir(m.store.Dir()), DefaultManagementURL: DefaultManagementURL}
	v.SetupKey = crud.Mask(v.SetupKey)
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
	// CLI 为实际使用的 netbird 程序路径，找不到时为空
	CLI string `json:"cli"`
	// DefaultManagementURL 为实例 managementUrl 留空时使用的 NetBird 官方云地址
	DefaultManagementURL string `json:"defaultManagementUrl"`
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
			if in.Key == "" {
				// 新增与复制时 key 尚未分配（请求体中的 key 已被清空）：legacyStateDir 由服务端维护，
				// 请求体不能借它让新实例使用旧版的 <配置目录>/netbird
				in.LegacyStateDir = false
			}
			if err := checkConflicts(m.cfg.Instances, in, m.store.Dir(), false); err != nil {
				return err
			}
			return m.foreignIfaceErr(in)
		},
		CheckEnable: func(in *Instance) error {
			if err := checkConflicts(m.cfg.Instances, in, m.store.Dir(), false); err != nil {
				return err
			}
			return m.foreignIfaceErr(in)
		},
		Merge: func(old, updated *Instance) {
			crud.KeepSecret(old.SetupKey, &updated.SetupKey)
			// 是否沿用旧版默认状态目录由服务端维护，请求体中的值不采信
			updated.LegacyStateDir = old.LegacyStateDir
		},
		Copy: func(src, dst *Instance) {
			crud.KeepSecret(src.SetupKey, &dst.SetupKey)
			// 副本使用自己的默认状态目录，不与模板共用节点身份
			dst.LegacyStateDir = false
		},
		View:    func(in *Instance) any { return m.instanceView(in) },
		Secrets: func(in *Instance) map[string]string { return map[string]string{"setupKey": in.SetupKey} },
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
	// 登录链接与设备码可把本机登记到任意 NetBird 账号，只提供给有写权限的身份
	hideAuth := func(req *http.Request, v *StatusView) {
		if !auth.Allowed(req.Context(), ID+":write") {
			v.AuthURL, v.AuthCode = "", ""
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
		cli, err := proc.Resolve(s.NetbirdPath, dir, "netbird")
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		out, err := proc.Output(20*time.Second, cli, append(inst.CLIArgs(dir), "logout"), nil)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "退出登录失败: %s", strings.TrimSpace(string(out)))
			return
		}
		m.mu.Lock()
		if rt := m.runs[key]; rt != nil {
			rt.cacheAt = time.Time{}
		}
		m.mu.Unlock()
		m.logs.Logger(cliSource(inst.Name)).Infof("已退出登录，本机已从组网中删除")
		httpx.OK(w, map[string]bool{"ok": true})
	}))
	module.DocOf(r, "POST /instances/{key}/logout", module.Doc{Summary: "退出实例的登录"})

	r.HandleFunc("GET /settings", func(w http.ResponseWriter, req *http.Request) {
		m.mu.Lock()
		s, dir := m.cfg.Settings, m.store.Dir()
		m.mu.Unlock()
		v := SettingsView{Settings: s, DefaultManagementURL: DefaultManagementURL}
		if cli, err := proc.Resolve(s.NetbirdPath, dir, "netbird"); err == nil {
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

// logoutLocked 在 Gately 托管的守护进程运行中时退出登录（尽力而为），本机会从管理服务注销、从组网中删除。
// 调用方持有 mu。
func (m *Module) logoutLocked(rt *instRuntime) {
	in := rt.inst
	if !in.ManageDaemon || !rt.daemon.State().Running {
		return
	}
	dir := m.store.Dir()
	cli, err := proc.Resolve(m.cfg.Settings.NetbirdPath, dir, "netbird")
	if err != nil {
		return
	}
	if out, err := proc.Output(20*time.Second, cli, append(in.CLIArgs(dir), "logout"), nil); err != nil {
		m.logs.Logger(cliSource(in.Name)).Warnf("退出登录失败: %s", strings.TrimSpace(string(out)))
	}
}

// removeStateLocked 删除实例在配置目录内的默认状态目录（登录状态与 WireGuard 密钥）。自定义状态目录可能是其他程序
// 在用的已有目录，迁移自旧版的默认目录 <配置目录>/netbird 还包含其他实例的目录，这两种只提示不删除。
// 默认目录由 defaultStateDir 按 key 拼出，key 含路径分隔符或点时为空，不会删到配置目录以外。
func (m *Module) removeStateLocked(in Instance) error {
	log := m.moduleLog()
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
// （这个目录只属于该实例，删除后不会再被使用）；自定义状态目录与迁移来的旧目录保留登录状态，
// 便于新建实例时沿用节点身份。调用方持有 mu。
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
		m.moduleLog().Warnf("%v", err)
	}
	// 删除可能解除了与其他实例的冲突
	m.syncLocked(func(string) bool { return false })
}

// resetLocked 恢复默认配置：使用默认状态目录的托管守护进程正在运行时先退出登录（尽力而为，会从组网中删除本机），
// 再停止全部实例、清空实例与程序设置并删除默认状态目录中的登录状态。自定义的状态目录与系统已有的 NetBird 服务
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
			// 旧版默认目录就是 <配置目录>/netbird，恢复默认时整个删除
			continue
		}
		if err := m.removeStateLocked(in); err != nil {
			return err
		}
	}
	// <配置目录>/netbird 只存放默认状态目录，全部删除；自定义状态目录恰好位于其中时保留，只提示
	base := filepath.Join(dir, "netbird")
	for _, in := range old.Instances {
		if in.StateDir != "" && (in.StateDir == base || strings.HasPrefix(in.StateDir, base+string(filepath.Separator))) {
			m.moduleLog().Warnf("自定义状态目录 %s 位于 %s 中，%s 未删除", in.StateDir, base, base)
			m.moduleLog().Infof("已恢复默认配置")
			return nil
		}
	}
	if err := os.RemoveAll(base); err != nil {
		return fmt.Errorf("删除状态目录失败: %v", err)
	}
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
	// 连接状态需要执行命令，status 内部带短期缓存，总览页轮询不会反复执行
	st, _ := m.status(key)
	switch st.DaemonStatus {
	case "Connected":
		return []module.SummaryItem{
			{Label: "状态", Value: "已连接", Level: "ok"},
			{Label: "节点", Value: strconv.Itoa(st.PeersConnected) + "/" + strconv.Itoa(len(st.Peers))},
		}
	case "NeedsLogin", "LoginFailed", "SessionExpired":
		return []module.SummaryItem{{Label: "状态", Value: "等待登录", Level: "warn"}}
	case "", "Connecting":
		return []module.SummaryItem{{Label: "状态", Value: "连接中", Level: "warn"}}
	}
	return []module.SummaryItem{{Label: "状态", Value: st.DaemonStatus, Level: "warn"}}
}
