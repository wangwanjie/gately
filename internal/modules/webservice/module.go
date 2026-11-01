// Package webservice 实现 Web 服务模块：按域名把请求分发到反向代理、重定向、静态文件或固定文本，
// 支持 HTTPS（证书来自证书模块）、PROXY 协议、基本认证、网页认证与 IP 访问控制。
package webservice

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
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
	"github.com/wangwanjie/gately/internal/geoip"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
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
}

// New 创建模块。certs 为证书来源，可为 nil（此时不能启用 HTTPS）；waf 可为 nil（站点的 WAF 设置不生效）。
func New(certs CertSource, waf WAFProvider) *Module {
	return &Module{certs: certs, waf: waf, runners: map[string]*listenerRunner{}, errors: map[string]string{}, realIP: &realIPSource{}, ipListStates: map[string]*ipListState{}}
}

// ID 实现 module.Module。
func (m *Module) ID() string { return ID }

// Title 实现 module.Module。
func (m *Module) Title() string { return "Web 服务" }

// Init 实现 module.Module。
func (m *Module) Init(env module.Env) error {
	m.store = env.Store
	m.logs = env.Logs
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
		m.realIP.set(from, headers)
	}
	m.auth.shared.set(m.cfg.Settings.AuthUsers, m.cfg.Settings.SSODomain)
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

// Start 实现 module.Module。
func (m *Module) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ctx = ctx
	go m.ipListRefresher(ctx)
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
	return nil
}

func (m *Module) startLocked(l Listener) {
	if m.ctx == nil {
		return
	}
	label := l.Name
	if label == "" {
		label = ":" + strconv.Itoa(l.Port)
	}
	r, err := newListenerRunner(l, m.logs.Logger(label), m.auth, m.waf)
	if err == nil {
		r.realIP = m.realIP
		err = r.start(m.certs, m.cfg.Settings.TrustedProxies)
	}
	if err != nil {
		m.errors[l.Key] = err.Error()
		m.logs.Logger(label).Errorf("启动失败: %v", err)
		return
	}
	delete(m.errors, l.Key)
	m.runners[l.Key] = r
}

func (m *Module) stopLocked(key string) {
	if r, ok := m.runners[key]; ok {
		r.stop()
		delete(m.runners, key)
	}
	delete(m.errors, key)
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
					delete(m.errors, key)
				}
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
			return m.checkIPSetsLocked(l)
		},
		View:    m.viewLocked,
		Changed: m.restartLocked,
		Removed: m.stopLocked,
	}
	listeners.Register(r, "/listeners")
	m.registerIPLists(r)
	m.registerUsers(r)
	r.HandleFunc("POST /import/nginx", m.handleImportNginx)
	r.HandleFunc("GET /settings", func(w http.ResponseWriter, req *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		s := m.cfg.Settings
		s.AuthSecret = ""
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
	module.DocOf(r, "GET /topology", module.Doc{Summary: "拓扑视图快照：监听、站点、后端与监听级动作的实时速率、计数与活动 IP（?focus=节点编号&ip=地址）", Resp: &TopologyView{}})
	r.HandleFunc("GET /connections", m.handleConns)
	r.HandleFunc("POST /connections/close", m.handleCloseConn)
	module.DocOf(r, "GET /connections", module.Doc{Summary: "站点进行中的请求与最近结束的长请求（?listener=&site=&q=）", Resp: &ConnsView{}})
	module.DocOf(r, "POST /connections/close", module.Doc{Summary: "断开站点的一个进行中请求，可同时封禁其来源地址", Req: &closeConnRequest{}, Resp: &closeConnResult{}})
	r.HandleFunc("GET /bans", m.handleListBans)
	r.HandleFunc("POST /bans", m.handleAddBan)
	r.HandleFunc("DELETE /bans", m.handleRemoveBan)
	r.HandleFunc("GET /geoip", m.handleGeoIPInfo)
	r.HandleFunc("POST /geoip/upload", m.handleGeoIPUpload)
	r.HandleFunc("POST /geoip/download", m.handleGeoIPDownload)
	r.HandleFunc("DELETE /geoip", m.handleGeoIPDelete)
	module.DocOf(r, "GET /bans", module.Doc{Summary: "当前被封禁的 IP", Resp: []BanInfo{}})
	module.DocOf(r, "POST /bans", module.Doc{Summary: "手动封禁 IP", Req: &banRequest{}})
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
	s.RealIPFrom, s.RealIPHeaders = from, headers
	m.mu.Lock()
	defer m.mu.Unlock()
	old := m.cfg.Settings
	s.AuthSecret = old.AuthSecret
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
	// 真实 IP 在每个请求进入时判断，替换设置即可生效，不需要重新监听
	m.realIP.set(s.RealIPFrom, s.RealIPHeaders)
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
