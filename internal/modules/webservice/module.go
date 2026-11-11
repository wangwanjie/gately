// Package webservice 实现 Web 服务模块：按域名把请求分发到反向代理、重定向、静态文件或固定文本，
// 支持 HTTPS（证书来自证书模块）、PROXY 协议、基本认证、网页认证与 IP 访问控制。
package webservice

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/fwopen"
	"github.com/wangwanjie/gately/internal/geoip"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/visitors"
)

// ID 是模块标识。
const ID = "webservice"

// Module 是 Web 服务模块。
type Module struct {
	mu      sync.Mutex
	store   *config.Store
	logs    *logbuf.Buffer
	cfg     fileConfig
	ctx     context.Context
	certs   CertSource
	waf     WAFProvider
	auth    *authenticator
	runners map[string]*listenerRunner
	errors  map[string]string
	// realIP 为全部监听器共享的可信代理真实 IP 设置
	realIP *realIPSource
	// ipListStates 为订阅 IP 列表的运行状态
	ipListStates map[string]*ipListState
	// sitesMoved 为站点移到其他监听后的回调，参数把旧标识“监听key/站点key”映射到新标识
	sitesMoved []func(map[string]string)
	// moveMu 串行化站点移动及其回调：回调在释放 mu 之后执行，若不串行，相邻两次移动的回调可能乱序，
	// 引用方（性能监控告警规则）先处理后一次的映射，最终停在已不存在的站点标识上。加锁顺序固定为 moveMu → mu
	moveMu sync.Mutex
	// fw 管理各监听的自动放行防火墙，key 为监听 key
	fw *fwopen.Group
	// visits 为访客审计数据，Init 后可用
	visits *visitors.Store
	// replay 为拓扑回放的录制状态
	replay replayRecorder
}

// New 创建模块。certs 为证书来源，可为 nil（此时不能启用 HTTPS）；waf 可为 nil（站点的 WAF 设置不生效）。
func New(certs CertSource, waf WAFProvider) *Module {
	return &Module{certs: certs, waf: waf, runners: map[string]*listenerRunner{}, errors: map[string]string{}, realIP: &realIPSource{}, ipListStates: map[string]*ipListState{}, fw: fwopen.NewGroup("web")}
}

// OnSitesMoved 注册站点移到其他监听后的回调，供按“监听key/站点key”引用站点的模块同步改写引用。
// 回调在 Web 服务模块释放锁之后同步调用，可以在其中调用本模块的只读接口。
func (m *Module) OnSitesMoved(fn func(renamed map[string]string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sitesMoved = append(m.sitesMoved, fn)
}

// ID 实现 module.Module。
func (m *Module) ID() string { return ID }

// Title 实现 module.Module。
func (m *Module) Title() string { return "Web 服务" }

// Init 实现 module.Module。
func (m *Module) Init(env module.Env) error {
	m.store = env.Store
	m.logs = env.Logs
	// 自动放行防火墙按配置目录区分实例，并清理上次运行遗留的放行规则
	fwopen.Setup(m.store.Dir())
	m.cfg = fileConfig{Settings: Settings{Enable: true}}
	if _, err := m.store.Load(ID, &m.cfg); err != nil {
		return err
	}
	if m.cfg.Settings.AuthSecret == "" {
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		m.cfg.Settings.AuthSecret = hex.EncodeToString(b)
		if err := m.saveLocked(); err != nil {
			return err
		}
	}
	// 手工编辑配置启用了 OIDC 但没有签名密钥时补生成，避免身份提供方无法启动
	if o := &m.cfg.Settings.OIDC; o.Enable && o.SigningKey == "" {
		k, err := newOIDCKey()
		if err != nil {
			return err
		}
		o.SigningKey = k
		if err := m.saveLocked(); err != nil {
			return err
		}
	}
	// 老配置中的共享用户没有 ID：加载时补生成并立即保存，保证 OIDC 的 sub 从此保持稳定
	if ensureAuthUserIDs(m.cfg.Settings.AuthUsers) {
		if err := m.saveLocked(); err != nil {
			return err
		}
	}
	m.auth = newAuthenticator(m.cfg.Settings.AuthSecret)
	m.auth.setBanSeconds(m.cfg.Settings.AutoBanSeconds)
	m.applyOAuthLocked()
	// 手工编辑配置填入无效或过宽（如 0.0.0.0/0）的可信代理时不启用真实 IP，避免请求头可被任意伪造
	if from, headers, err := normalizeRealIP(m.cfg.Settings.RealIPFrom, m.cfg.Settings.RealIPHeaders); err != nil {
		m.logs.Logger("").Errorf("真实 IP 设置无效，未启用: %v", err)
	} else {
		// 端口头无效时只是不采信端口，真实 IP 照常启用
		port, perr := normalizeRealPort(from, m.cfg.Settings.RealPortHeader)
		if perr != nil {
			m.logs.Logger("").Errorf("真实端口请求头无效，未启用: %v", perr)
		}
		m.realIP.set(from, headers, port)
	}
	m.auth.shared.set(m.cfg.Settings.AuthUsers, m.cfg.Settings.SSODomain)
	// 手工编辑配置填入过短的认证标记密钥时不启用，避免弱密钥被猜出后伪造标记
	if k := m.cfg.Settings.AuthMarkerKey; k != "" && len(k) < minAuthMarkerKey {
		m.logs.Logger("").Errorf("认证标记密钥少于 %d 个字符，未启用", minAuthMarkerKey)
	} else {
		m.auth.marker.set(k)
	}
	m.auth.selfAuth.Store(m.cfg.Settings.SelfServicePassword)
	m.auth.changeShared = m.changeSharedPassword
	m.auth.onPasswordChanged = func(kind, name, ip string) {
		if kind == kindFile {
			m.logs.Logger("").Infof("文件服务共享用户 %s 从 %s 自助修改了密码", name, ip)
		} else {
			m.logs.Logger("").Infof("共享用户 %s 从 %s 自助修改了密码", name, ip)
		}
	}
	m.auth.onPasswordFailed = func(kind, name, ip string, err error) {
		m.logs.Logger("").Warnf("用户 %s 从 %s 自助修改密码时保存失败: %v", name, ip, err)
	}
	if err := m.auth.oidc.set(m.cfg.Settings.OIDC); err != nil {
		m.logs.Add(logbuf.LevelWarn, "", "OIDC 身份提供方未启动: "+err.Error())
	}
	// 已导入过 GeoIP 数据库时随模块加载，失败只记录日志
	if _, err := os.Stat(m.geoipPath()); err == nil {
		if err := geoip.Default.Load(m.geoipPath()); err != nil {
			m.logs.Add(logbuf.LevelWarn, "", err.Error())
		}
	}
	// 订阅的 IP 列表先用缓存内容，启动后再按刷新间隔下载
	m.loadIPListsLocked()
	// 自定义错误页面库：页面不可用（文件被删、内容无效）时记录日志，引用它的站点改用默认样式
	errLog := m.logs.Logger("")
	m.auth.errPages.warn = errLog.Warnf
	m.auth.errPages.set(m.cfg.ErrorPages)
	// 导航面板库：配置中无效的面板不提供服务，记录日志说明原因
	m.auth.panels.warn = errLog.Warnf
	m.auth.panels.set(m.cfg.Panels)
	m.initVisitors()
	return nil
}

// applyOAuthLocked 把第三方登录设置同步给认证器。
func (m *Module) applyOAuthLocked() {
	env := oauthEnv{providers: map[string]OAuthProvider{}, callbackBase: strings.TrimRight(m.cfg.Settings.OAuthCallbackBase, "/")}
	for _, p := range m.cfg.Settings.OAuthProviders {
		env.providers[p.Key] = p
	}
	m.auth.setOAuth(env)
}

// listenerLogLabel 返回监听器在运行日志中的来源名称：通常为监听器名称，未命名时为 :端口。
// 与其他监听器重名时（例如 IPv4、IPv6 各一个都叫 HTTPS）附上监听地址，否则日志里分不清来自哪个监听器。
func listenerLogLabel(l Listener, all []Listener) string {
	base := func(x Listener) string {
		if x.Name != "" {
			return x.Name
		}
		return ":" + strconv.Itoa(x.Port)
	}
	label := base(l)
	for _, o := range all {
		if o.Key != l.Key && base(o) == label {
			return label + " (" + listenerAddrLabel(l) + ")"
		}
	}
	return label
}

// LogSources 实现 module.LogSourcer：全部监听器（包括已停用的）的日志标签与订阅的 IP 列表名称，
// 与写日志时使用的标签一致，日志页的来源下拉据此列出还没有访问记录的监听器。
func (m *Module) LogSources() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.cfg.Listeners)+len(m.cfg.IPLists))
	for _, l := range m.cfg.Listeners {
		out = append(out, listenerLogLabel(l, m.cfg.Listeners))
	}
	for _, l := range m.cfg.IPLists {
		if l.Name != "" {
			out = append(out, l.Name)
		}
	}
	return out
}

// AccessLogs 实现 module.AccessLogger：站点访问日志按天归档并压缩保存。
func (m *Module) AccessLogs() bool { return true }

// listenerAddrLabel 返回监听地址的简短写法；未填写地址时按网络类型写成 0.0.0.0、[::] 或 *，区分只监听 IPv4 或 IPv6。
func listenerAddrLabel(l Listener) string {
	host := l.ListenIP
	if host == "" {
		host = map[string]string{"tcp4": "0.0.0.0", "tcp6": "::"}[l.Network]
	}
	if host == "" {
		return "*:" + strconv.Itoa(l.Port)
	}
	return net.JoinHostPort(host, strconv.Itoa(l.Port))
}

// Start 实现 module.Module。
func (m *Module) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ctx = ctx
	go m.ipListRefresher(ctx)
	m.syncReplayLocked()
	if !m.cfg.Settings.Enable {
		m.logs.Add(logbuf.LevelInfo, "", "Web 服务模块已禁用")
		return nil
	}
	for _, l := range m.cfg.Listeners {
		if l.Enable {
			m.startLocked(l)
		}
	}
	return nil
}

// Stop 实现 module.Module。
func (m *Module) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key := range m.runners {
		m.stopLocked(key)
	}
	// 同步撤销自动放行，进程退出前关闭端口
	m.fw.ClearAll()
	// 监听已全部停止，不再有新的访问，访客审计数据写盘
	if err := m.visits.Flush(); err != nil {
		m.logs.Logger("").Warnf("访客审计数据写盘失败: %v", err)
	}
	if m.replay.cancel != nil {
		m.replay.cancel()
		m.replay.cancel = nil
	}
	return nil
}

func (m *Module) startLocked(l Listener) {
	if m.ctx == nil {
		return
	}
	label := listenerLogLabel(l, m.cfg.Listeners)
	// 每条日志都带上监听器 key，站点日志据此只看本监听器的记录（同名站点分在 IPv4、IPv6 两个监听器时不会混在一起）
	lg := m.logs.Logger(label).With(logbuf.Fields{logbuf.FieldListener: l.Key})
	r, err := newListenerRunner(l, lg, m.auth, m.waf)
	if err == nil {
		r.realIP = m.realIP
		err = r.start(m.certs, m.cfg.Settings.TrustedProxies)
	}
	if err != nil {
		m.errors[l.Key] = err.Error()
		lg.Errorf("启动失败: %v", err)
		// 监听失败时端口可能被其他程序占用，绝不能为它放行
		m.fw.Want(l.Key, nil, nil)
		return
	}
	m.setRunnerErrorLocked(r)
	m.runners[l.Key] = r
	m.syncFirewallLocked(r)
}

// setRunnerErrorLocked 按运行中的监听更新错误状态：主端口正常而额外 HTTP 端口监听失败时，监听继续运行并显示原因。
func (m *Module) setRunnerErrorLocked(r *listenerRunner) {
	if r.httpErr != "" {
		m.errors[r.key] = r.httpErr
		return
	}
	delete(m.errors, r.key)
}

// syncFirewallLocked 按监听当前的配置登记自动放行：只放行已经监听成功的 TCP 端口，HTTP/3 启动成功时再放行同号 UDP 端口。
func (m *Module) syncFirewallLocked(r *listenerRunner) {
	if !r.cfg.AutoFirewall {
		m.fw.Want(r.cfg.Key, nil, nil)
		return
	}
	tcp := []int{r.cfg.Port}
	// 额外 HTTP 端口只在监听成功时放行
	if r.httpLn != nil {
		tcp = append(tcp, r.cfg.HTTPPort)
	}
	ports := fwopen.ForListen(r.cfg.Network, r.cfg.ListenIP, tcp)
	if r.h3pc != nil {
		udp := map[string]string{"tcp": "udp", "tcp4": "udp4", "tcp6": "udp6"}[r.cfg.Network]
		ports = append(ports, fwopen.ForListen(udp, r.cfg.ListenIP, []int{r.cfg.Port})...)
	}
	m.fw.Want(r.cfg.Key, ports, r.log)
}

func (m *Module) stopLocked(key string) {
	if r, ok := m.runners[key]; ok {
		r.stop()
		delete(m.runners, key)
	}
	delete(m.errors, key)
	m.fw.Want(key, nil, nil)
}

func (m *Module) restartLocked(key string) {
	// 只改了站点、监听端口设置不变时热替换站点表，不断开已有连接
	if r, ok := m.runners[key]; ok && m.cfg.Settings.Enable {
		for _, l := range m.cfg.Listeners {
			if l.Key == key && l.Enable && sameSocket(r.cfg, l) {
				if err := r.reload(l, m.waf); err != nil {
					// 新配置无法编译时保留旧站点继续服务
					m.errors[key] = err.Error()
					r.log.Errorf("更新站点失败，继续使用原配置: %v", err)
				} else {
					m.setRunnerErrorLocked(r)
				}
				// 自动放行开关不影响监听，在热更新时同步
				m.syncFirewallLocked(r)
				return
			}
		}
	}
	m.stopLocked(key)
	if !m.cfg.Settings.Enable {
		return
	}
	for _, l := range m.cfg.Listeners {
		if l.Key == key && l.Enable {
			m.startLocked(l)
		}
	}
}

func (m *Module) saveLocked() error {
	return m.store.Save(ID, m.cfg)
}

// ListenerView 是带运行状态的监听器。
type ListenerView struct {
	Listener
	Running bool                        `json:"running"`
	Error   string                      `json:"error,omitempty"`
	Stats   map[string]map[string]int64 `json:"stats"`
	// Upstreams 为各站点的后端健康状态，键为站点 key
	Upstreams map[string][]UpstreamStatus `json:"upstreams,omitempty"`
	// Firewall 为自动放行防火墙的结果说明，未开启或监听未运行时为空
	Firewall string `json:"firewall,omitempty"`
	// FirewallFailed 表示自动放行有端口放行失败（例如没有权限），界面据此提示
	FirewallFailed bool `json:"firewallFailed,omitempty"`
}

func (m *Module) viewLocked(l *Listener) any {
	v := ListenerView{Listener: *l, Error: m.errors[l.Key], Stats: map[string]map[string]int64{}}
	if r, ok := m.runners[l.Key]; ok {
		v.Running = true
		t := r.current()
		for _, s := range t.sites {
			v.Stats[s.site.Key] = s.stats.snapshot()
			if st := s.upstreamStatus(); st != nil {
				if v.Upstreams == nil {
					v.Upstreams = map[string][]UpstreamStatus{}
				}
				v.Upstreams[s.site.Key] = st
			}
		}
		if t.deflt != nil {
			v.Stats["default"] = t.deflt.stats.snapshot()
		}
		if l.AutoFirewall {
			v.Firewall = m.fw.Status(l.Key)
			v.FirewallFailed = m.fw.Failed(l.Key)
			if v.Firewall == "" && fwopen.IsLoopback(l.ListenIP) {
				v.Firewall = fwopen.LoopbackNote
			}
		}
	}
	return v
}

// Routes 实现 module.Module。
func (m *Module) Routes(r module.Router) {
	listeners := &crud.Collection[Listener]{
		Mu:      &m.mu,
		Items:   &m.cfg.Listeners,
		Save:    m.saveLocked,
		Key:     func(l *Listener) *string { return &l.Key },
		Enabled: func(l *Listener) *bool { return &l.Enable },
		Validate: func(l *Listener) error {
			if err := l.Validate(); err != nil {
				return err
			}
			return m.checkRefsLocked(l)
		},
		// 额外 HTTP 端口不能与其他监听的端口冲突；启用已停用的监听时同样检查
		Check:       func(l *Listener) error { return checkHTTPPortConflict(l, m.cfg.Listeners) },
		CheckEnable: func(l *Listener) error { return checkHTTPPortConflict(l, m.cfg.Listeners) },
		Copy:        func(_, dst *Listener) { dst.dropShareLinks() },
		View:        m.viewLocked,
		Changed:     m.restartLocked,
		Removed:     m.stopLocked,
	}
	listeners.Register(r, "/listeners")
	m.registerIPLists(r)
	m.registerErrorPages(r)
	m.registerPanels(r)
	m.registerUsers(r)
	m.registerBatch(r)
	m.registerCache(r)
	r.HandleFunc("POST /import/nginx", m.handleImportNginx)
	r.HandleFunc("GET /settings", func(w http.ResponseWriter, req *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		s := m.cfg.Settings
		s.AuthSecret = ""
		s.AuthMarkerKey = crud.Mask(s.AuthMarkerKey)
		s.OAuthProviders = append([]OAuthProvider{}, s.OAuthProviders...)
		for i := range s.OAuthProviders {
			s.OAuthProviders[i].ClientSecret = crud.Mask(s.OAuthProviders[i].ClientSecret)
		}
		s.AuthUsers = maskAuthUsers(s.AuthUsers)
		s.OIDC = maskOIDC(s.OIDC)
		httpx.OK(w, s)
	})
	// 查看被脱敏的密钥明文，路径含 secrets 片段，按写权限鉴权，只读账号与只读令牌无法调用。
	// 共享用户密码与 OIDC 客户端密钥只保存 bcrypt 哈希，无法还原明文，不提供查看
	r.HandleFunc("GET /settings/secrets/totp", crud.SecretsHandler(&m.mu, func() map[string]string {
		return totpSecrets(m.cfg.Settings.AuthUsers)
	}))
	r.HandleFunc("GET /settings/secrets/oauth", crud.SecretsHandler(&m.mu, func() map[string]string {
		return oauthSecrets(m.cfg.Settings.OAuthProviders)
	}))
	r.HandleFunc("GET /settings/secrets/marker", crud.SecretsHandler(&m.mu, func() map[string]string {
		return map[string]string{"authMarkerKey": m.cfg.Settings.AuthMarkerKey}
	}))
	module.DocOf(r, "GET /settings/secrets/marker", module.Doc{Summary: "查看认证标记密钥明文（authMarkerKey）", Resp: map[string]string{}})
	module.DocOf(r, "GET /settings/secrets/totp", module.Doc{Summary: "查看共享用户的 TOTP 密钥明文（按用户名索引）", Resp: map[string]string{}})
	module.DocOf(r, "GET /settings/secrets/oauth", module.Doc{Summary: "查看第三方登录的 Client Secret 明文（按登录方式标识索引）", Resp: map[string]string{}})
	r.HandleFunc("POST /totp-secret", m.handleNewTOTP)
	module.DocOf(r, "POST /totp-secret", module.Doc{Summary: "为共享用户生成新的 TOTP 密钥（不保存，保存设置后生效）", Req: &totpRequest{}, Resp: &totpResponse{}})
	r.HandleFunc("PUT /settings", m.handlePutSettings)
	module.DocOf(r, "GET /settings", module.Doc{Summary: "读取 Web 服务全局设置", Resp: &Settings{}})
	module.DocOf(r, "PUT /settings", module.Doc{Summary: "保存 Web 服务全局设置", Req: &Settings{}, Resp: &Settings{}})
	r.HandleFunc("GET /stats", m.handleStats)
	r.HandleFunc("GET /metrics", m.handleMetrics)
	module.DocOf(r, "GET /stats", module.Doc{Summary: "站点分钟级统计（?listener=&site=&minutes=）", Resp: &SiteStatsView{}})
	module.DocOf(r, "GET /metrics", module.Doc{Summary: "Prometheus 文本格式指标"})
	r.HandleFunc("GET /topology", m.handleTopology)
	// 访客审计
	r.HandleFunc("GET /visitors", m.handleVisitors)
	r.HandleFunc("GET /visitors/detail", m.handleVisitorDetail)
	r.HandleFunc("POST /visitors/delete", m.handleDeleteVisitors)
	r.HandleFunc("GET /visitors/settings", m.handleVisitorSettings)
	r.HandleFunc("PUT /visitors/settings", m.handlePutVisitorSettings)
	m.registerReplay(r)
	module.DocOf(r, "GET /topology", module.Doc{Summary: "拓扑视图快照：监听、站点、后端与监听级动作的实时速率、计数与活动 IP（?focus=节点编号&ip=地址）", Resp: &TopologyView{}})
	r.HandleFunc("GET /connections", m.handleConns)
	r.HandleFunc("POST /connections/close", m.handleCloseConn)
	module.DocOf(r, "GET /connections", module.Doc{Summary: "站点进行中的请求与最近结束的长请求（?listener=&site=&q=）", Resp: &ConnsView{}})
	module.DocOf(r, "POST /connections/close", module.Doc{Summary: "断开站点的一个进行中请求，可同时封禁其来源地址（banSeconds 为 -1 表示永久）", Req: &closeConnRequest{}, Resp: &closeConnResult{}})
	r.HandleFunc("GET /bans", m.handleListBans)
	r.HandleFunc("POST /bans", m.handleAddBan)
	r.HandleFunc("DELETE /bans", m.handleRemoveBan)
	r.HandleFunc("GET /geoip", m.handleGeoIPInfo)
	r.HandleFunc("POST /geoip/upload", m.handleGeoIPUpload)
	r.HandleFunc("POST /geoip/download", m.handleGeoIPDownload)
	r.HandleFunc("DELETE /geoip", m.handleGeoIPDelete)
	module.DocOf(r, "GET /bans", module.Doc{Summary: "当前被封禁的 IP", Resp: []BanInfo{}})
	module.DocOf(r, "POST /bans", module.Doc{Summary: "手动封禁 IP（seconds 为 0 表示 24 小时，-1 表示永久）", Req: &banRequest{}})
	module.DocOf(r, "DELETE /bans", module.Doc{Summary: "解除封禁（body 为 {\"key\": 地址}）", Req: &banRequest{}})
	module.DocOf(r, "GET /geoip", module.Doc{Summary: "GeoIP 数据库状态", Resp: &geoip.Info{}})
	module.DocOf(r, "POST /geoip/upload", module.Doc{Summary: "上传 GeoIP 数据库（multipart 字段 file，.mmdb）"})
	module.DocOf(r, "DELETE /geoip", module.Doc{Summary: "移除 GeoIP 数据库", Resp: &geoip.Info{}})
	module.DocOf(r, "POST /geoip/download", module.Doc{Summary: "从地址下载 GeoIP 数据库（.mmdb）", Req: &geoipDownload{}})
}

// totpSecrets 返回共享用户的 TOTP 密钥明文，按用户名索引；未启用二次验证的用户值为空字符串。
func totpSecrets(users []AuthUser) map[string]string {
	out := make(map[string]string, len(users))
	for _, u := range users {
		out[u.Name] = u.TOTPSecret
	}
	return out
}

// oauthSecrets 返回第三方登录的 Client Secret 明文，按登录方式标识（Key）索引。
func oauthSecrets(providers []OAuthProvider) map[string]string {
	out := make(map[string]string, len(providers))
	for _, p := range providers {
		out[p.Key] = p.ClientSecret
	}
	return out
}

// settingsPut 是全局设置的请求体。共享用户、单点登录域、自助改密与 OIDC 改由认证用户页（PUT /users）维护，
// 这几项在请求中省略时保留原值，避免打开较早的全局设置对话框保存时用旧数据覆盖认证用户页的修改。
// 外层同名字段优先于内嵌 Settings 的字段解码。
type settingsPut struct {
	Settings
	AuthUsers           *[]AuthUser   `json:"authUsers"`
	SSODomain           *string       `json:"ssoDomain"`
	SelfServicePassword *bool         `json:"selfServicePassword"`
	OIDC                *OIDCSettings `json:"oidc"`
}

func (m *Module) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var put settingsPut
	if err := httpx.ReadJSON(r, &put); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	s := put.Settings
	if _, err := parsePrefixes(s.TrustedProxies); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	from, headers, err := normalizeRealIP(s.RealIPFrom, s.RealIPHeaders)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	port, err := normalizeRealPort(from, s.RealPortHeader)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "真实端口请求头：%v", err)
		return
	}
	s.RealIPFrom, s.RealIPHeaders, s.RealPortHeader = from, headers, port
	m.mu.Lock()
	defer m.mu.Unlock()
	old := m.cfg.Settings
	s.AuthSecret = old.AuthSecret
	crud.KeepSecret(old.AuthMarkerKey, &s.AuthMarkerKey)
	s.AuthMarkerKey = strings.TrimSpace(s.AuthMarkerKey)
	if s.AuthMarkerKey != "" && len(s.AuthMarkerKey) < minAuthMarkerKey {
		httpx.Error(w, http.StatusBadRequest, "认证标记密钥至少 %d 个字符", minAuthMarkerKey)
		return
	}
	// 保留被脱敏的 Client Secret
	oldSecrets := map[string]string{}
	for _, p := range old.OAuthProviders {
		oldSecrets[p.Key] = p.ClientSecret
	}
	keys := map[string]bool{}
	for i := range s.OAuthProviders {
		p := &s.OAuthProviders[i]
		p.Key = strings.ToLower(strings.TrimSpace(p.Key))
		crud.KeepSecret(oldSecrets[p.Key], &p.ClientSecret)
		if err := p.Validate(); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		if keys[p.Key] {
			httpx.Error(w, http.StatusBadRequest, "第三方登录标识 %s 重复", p.Key)
			return
		}
		keys[p.Key] = true
	}
	s.OAuthCallbackBase = strings.TrimSpace(s.OAuthCallbackBase)
	if s.OAuthCallbackBase != "" {
		if u, err := url.Parse(s.OAuthCallbackBase); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			httpx.Error(w, http.StatusBadRequest, "回调地址需为完整的 http(s) 地址")
			return
		}
	}
	s.TrustedProxies = cleanList(s.TrustedProxies)
	// 省略的认证用户相关字段沿用原值（原值已规范化），提供的字段照常校验
	s.AuthUsers, s.SSODomain, s.SelfServicePassword, s.OIDC = old.AuthUsers, old.SSODomain, old.SelfServicePassword, old.OIDC
	if put.SelfServicePassword != nil {
		s.SelfServicePassword = *put.SelfServicePassword
	}
	if put.AuthUsers != nil {
		users, err := normalizeAuthUsers(*put.AuthUsers, old.AuthUsers)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		s.AuthUsers = users
	}
	if put.SSODomain != nil {
		d, err := validateSSODomain(*put.SSODomain)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		s.SSODomain = d
	}
	if put.OIDC != nil {
		o := *put.OIDC
		if err := normalizeOIDC(&o, old.OIDC); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		s.OIDC = o
	}
	if s.AutoBanSeconds < 0 || time.Duration(s.AutoBanSeconds)*time.Second > maxBanDuration {
		httpx.Error(w, http.StatusBadRequest, "自动封禁时长需在 0 到 30 天之间")
		return
	}
	m.cfg.Settings = s
	m.auth.setBanSeconds(s.AutoBanSeconds)
	if err := m.saveLocked(); err != nil {
		m.cfg.Settings = old
		httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
		return
	}
	m.applyOAuthLocked()
	m.applyUsersLocked(s)
	m.auth.marker.set(s.AuthMarkerKey)
	if s.AuthMarkerKey != old.AuthMarkerKey {
		m.auth.backends.reset()
	}
	// 真实 IP 在每个请求进入时判断，替换设置即可生效，不需要重新监听
	m.realIP.set(s.RealIPFrom, s.RealIPHeaders, s.RealPortHeader)
	// 可信代理在监听层校验 PROXY 协议来源，变化时接收 PROXY 协议的监听器需要重新监听；
	// 其余设置只需热替换站点表，避免断开正经由这些监听器访问的管理界面（包括本次保存请求）
	trustedChanged := !slices.Equal(old.TrustedProxies, s.TrustedProxies)
	for _, l := range m.cfg.Listeners {
		if trustedChanged && l.AcceptProxyProtocol {
			m.stopLocked(l.Key)
		}
		m.restartLocked(l.Key)
	}
	s.AuthSecret = ""
	s.AuthMarkerKey = crud.Mask(s.AuthMarkerKey)
	for i := range s.OAuthProviders {
		s.OAuthProviders[i].ClientSecret = crud.Mask(s.OAuthProviders[i].ClientSecret)
	}
	s.AuthUsers = maskAuthUsers(s.AuthUsers)
	s.OIDC = maskOIDC(s.OIDC)
	httpx.OK(w, s)
}

// changeSharedPassword 保存共享用户自助修改的密码哈希。u 为校验当前密码时的用户：
// 期间管理员改过该用户的密码、改名或删除了该用户时拒绝保存，避免覆盖管理员的修改。
func (m *Module) changeSharedPassword(u AuthUser, hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	old := m.cfg.Settings.AuthUsers
	users := slices.Clone(old)
	i := slices.IndexFunc(users, func(x AuthUser) bool { return x.ID == u.ID && x.Name == u.Name && x.Password == u.Password })
	if i < 0 {
		return errPasswordChanged
	}
	users[i].Password = hash
	m.cfg.Settings.AuthUsers = users
	if err := m.saveLocked(); err != nil {
		m.cfg.Settings.AuthUsers = old
		return fmt.Errorf("保存失败: %v", err)
	}
	m.auth.shared.set(users, m.cfg.Settings.SSODomain)
	return nil
}

type totpRequest struct {
	// Name 为共享用户名，用于验证器 App 中显示的账户名
	Name string `json:"name"`
}

type totpResponse struct {
	Secret string `json:"secret"`
	// URL 为 otpauth 地址，前端据此生成二维码
	URL string `json:"url"`
}

// handleNewTOTP 生成新的 TOTP 密钥。密钥随设置一起保存，未保存前不生效。
func (m *Module) handleNewTOTP(w http.ResponseWriter, r *http.Request) {
	var req totpRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	secret := auth.NewTOTPSecret()
	httpx.OK(w, totpResponse{Secret: secret, URL: auth.TOTPURL(secret, strings.TrimSpace(req.Name))})
}
