// Package netbird 托管 NetBird 守护进程并通过 netbird 命令完成登录与状态展示，
// 把本机加入 NetBird 官方云或自建管理服务的组网。
package netbird

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"regexp"
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

// Module 是 NetBird 模块。
type Module struct {
	mu       sync.Mutex
	store    *config.Store
	logs     *logbuf.Logger
	logBuf   *logbuf.Buffer
	settings Settings
	daemon   *proc.Supervisor
	started  bool
	// err 与 auth 由 errMu 保护：启动协程需要写入，而停止流程在持有 mu 时等待该协程退出，共用 mu 会死锁
	errMu sync.Mutex
	err   string
	// authURL 与 authCode 是 netbird up 输出的登录链接与设备码，登录完成或重新应用配置后清空
	authURL  string
	authCode string
	// cache 缓存最近一次状态，避免总览页与状态页频繁轮询时反复执行命令
	cache   StatusView
	cacheAt time.Time
	// upCancel 取消正在进行的 netbird up
	upCancel context.CancelFunc
	upDone   chan struct{}
	// versions 缓存各程序路径的版本
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
func (m *Module) Title() string { return "NetBird 组网" }

// Init 实现 module.Module。
func (m *Module) Init(env module.Env) error {
	m.store, m.logBuf = env.Store, env.Logs
	m.logs = m.logBuf.Logger("netbird")
	m.daemon = proc.New(m.logBuf.Logger("daemon"))
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

// stopLocked 取消进行中的 up 并停止托管的守护进程。
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
	m.setAuth("", "")
	m.cacheAt = time.Time{}
	if !m.started || !m.settings.Enable {
		return
	}
	s := m.settings
	dir := m.store.Dir()
	cli, err := proc.Resolve(s.NetbirdPath, dir, "netbird")
	if err != nil {
		m.fail(err.Error())
		return
	}
	if s.ManageDaemon && s.Userspace {
		m.warnPublicSocks(cli)
	}
	// started 在托管的守护进程每次启动（包括异常退出后被重启）时收到信号，触发一次 netbird up：
	// 托管时关闭了守护进程的自动连接，由 Gately 负责连接
	started := make(chan struct{}, 1)
	var daemonSpec proc.Spec
	if s.ManageDaemon {
		if err := fsperm.MkdirAll(s.stateDir(dir), 0o700); err != nil {
			m.fail("创建状态目录失败: " + err.Error())
			return
		}
		daemonSpec = proc.Spec{Path: cli, Args: s.DaemonArgs(dir), Env: s.DaemonEnvList(dir), OnStart: func() {
			select {
			case started <- struct{}{}:
			default:
			}
		}}
		m.daemon.Start(daemonSpec)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	m.upCancel, m.upDone = cancel, done
	go func() {
		defer close(done)
		if !s.ManageDaemon {
			m.bringUp(ctx, cli, s, dir)
			return
		}
		restarted := false
		for {
			select {
			case <-ctx.Done():
				return
			case <-started:
			}
			// 需要时重启一次守护进程，清除用户态网络下断开再连接后失效的 SOCKS5 监听；
			// 每次应用配置最多重启一次，避免自动连接没能关闭时反复重启
			if m.bringUp(ctx, cli, s, dir) && !restarted {
				restarted = true
				m.logs.Infof("已关闭守护进程的自动连接，重启守护进程让 SOCKS5 代理生效")
				m.daemon.Start(daemonSpec)
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

// warnPublicSocks 在托管的守护进程使用用户态网络且 NetBird 版本过旧时提示 SOCKS5 代理对外暴露；调用方持有 mu。
func (m *Module) warnPublicSocks(cli string) {
	ver, ok := m.versions[cli]
	if !ok {
		ver = proc.Version(cli, "version")
		m.versions[cli] = ver
	}
	if socksAlwaysPublic(ver) {
		m.logs.Warnf("NetBird %s 在用户态网络下总是在 0.0.0.0 上提供无认证的 SOCKS5 代理（0.75.0 起才默认只监听本机），其他设备可经它访问组网，请升级 NetBird", ver)
	}
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

func (m *Module) setAuth(url, code string) {
	m.errMu.Lock()
	m.authURL, m.authCode = url, code
	m.errMu.Unlock()
}

func (m *Module) getAuth() (string, string) {
	m.errMu.Lock()
	defer m.errMu.Unlock()
	return m.authURL, m.authCode
}

func (m *Module) fail(msg string) {
	m.putErr(msg)
	m.logs.Errorf("%s", msg)
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

// bringUp 等待守护进程就绪后执行 netbird up。未提供设置密钥时 up 会等待登录完成，
// 登录链接写入日志，并可通过状态接口获取；链接过期后自动重新获取。
// 返回 true 表示托管的守护进程需要重启：它按旧配置自动连接过，在用户态网络下断开再连接后 SOCKS5 代理失效。
func (m *Module) bringUp(ctx context.Context, cli string, s Settings, dir string) (restartDaemon bool) {
	global := s.CLIArgs(dir)
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
			m.fail("等待 NetBird 守护进程就绪超时，请检查日志")
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
		m.putErr("")
	}
	// 已连接（守护进程按上次保存的配置自动连接）时 netbird up 只会提示 Already connected，
	// 既不应用也不保存新的配置，所以先断开。托管时 up 会保存“关闭自动连接”，之后守护进程启动后保持空闲，
	// 由这里连接；只有首次（例如从开启自动连接的旧配置升级）需要断开。用户态网络下同一进程内断开再连接时
	// 旧的 SOCKS5 监听不会释放，新连接无法监听，代理失效，因此 up 完成后要重启一次守护进程
	reconnect := st.DaemonStatus == "Connected" || st.DaemonStatus == "Connecting"
	if reconnect {
		if out, err := proc.Output(15*time.Second, cli, append(append([]string{}, global...), "down"), nil); err != nil {
			m.logs.Warnf("断开连接失败: %s", strings.TrimSpace(string(out)))
		}
	}
	args := append(append([]string{}, global...), s.UpArgs()...)
	backoff := upRetryMin
	for {
		gotURL := false
		// lastLine 是 netbird up 的最后一行输出，失败时作为原因展示
		lastLine := ""
		spec := proc.Spec{Path: cli, Args: args, Env: s.UpEnv(), OnLine: func(line string) {
			if line = strings.TrimSpace(line); line != "" {
				lastLine = line
			}
			if url, code := parseAuthLine(line); url != "" {
				gotURL = true
				m.setAuth(url, code)
			}
		}}
		err := proc.Run(ctx, spec, m.logs)
		m.setAuth("", "")
		if ctx.Err() != nil {
			return false
		}
		if err == nil {
			m.putErr("")
			m.logs.Infof("netbird up 已完成")
			return reconnect && s.ManageDaemon && s.Userspace
		}
		delay := loginRetryDelay
		if gotURL {
			// 输出过登录链接说明在等待登录，失败多半是链接过期，重新获取
			m.logs.Infof("登录链接已过期，重新获取")
		} else {
			reason := lastLine
			if reason == "" {
				reason = err.Error()
			}
			m.fail(fmt.Sprintf("netbird up 失败: %s，%s 后重试", reason, backoff))
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
	out, err := proc.Output(statusTimeout, cli, append(append([]string{}, global...), "status", "--json"), nil)
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

// StatusView 是 /status 接口的返回内容。
type StatusView struct {
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
	v.AuthURL, v.AuthCode = m.getAuth()

	cli, err := proc.Resolve(s.NetbirdPath, dir, "netbird")
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

// Routes 实现 module.Module。
func (m *Module) Routes(r module.Router) {
	r.HandleFunc("GET /settings", func(w http.ResponseWriter, req *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		s := m.settings
		s.SetupKey = crud.Mask(s.SetupKey)
		httpx.OK(w, map[string]any{"settings": s, "defaultStateDir": m.settings.stateDir(m.store.Dir()), "defaultManagementUrl": DefaultManagementURL})
	})
	r.HandleFunc("PUT /settings", func(w http.ResponseWriter, req *http.Request) {
		s := DefaultSettings()
		if err := httpx.ReadJSON(req, &s); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		crud.KeepSecret(m.settings.SetupKey, &s.SetupKey)
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
	// 查看被脱敏的设置密钥明文
	r.HandleFunc("GET /secrets", crud.SecretsHandler(&m.mu, func() map[string]string {
		return map[string]string{"setupKey": m.settings.SetupKey}
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
		cli, err := proc.Resolve(s.NetbirdPath, dir, "netbird")
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		out, err := proc.Output(20*time.Second, cli, append(s.CLIArgs(dir), "logout"), nil)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "退出登录失败: %s", strings.TrimSpace(string(out)))
			return
		}
		m.mu.Lock()
		m.cacheAt = time.Time{}
		m.mu.Unlock()
		m.logs.Infof("已退出登录，本机已从组网中删除")
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

// resetLocked 恢复默认配置：Gately 托管的守护进程正在运行时先退出登录（尽力而为，会从组网中删除本机），
// 再停止守护进程、恢复默认设置并删除默认状态目录中的登录状态。自定义的状态目录与系统已有的 NetBird 服务
// 不会被删除或登出；安装的程序不受影响。调用方持有 mu。
func (m *Module) resetLocked() error {
	old := m.settings
	dir := m.store.Dir()
	if old.ManageDaemon && m.daemon.State().Running {
		if cli, err := proc.Resolve(old.NetbirdPath, dir, "netbird"); err == nil {
			if out, err := proc.Output(20*time.Second, cli, append(old.CLIArgs(dir), "logout"), nil); err != nil {
				m.logs.Warnf("退出登录失败: %s", strings.TrimSpace(string(out)))
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
		m.logs.Warnf("自定义状态目录 %s 未删除，如不再需要请手动删除", old.StateDir)
	}
	m.logs.Infof("已恢复默认配置")
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
	// 连接状态需要执行命令，status 内部带短期缓存，总览页轮询不会反复执行
	st := m.status()
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
