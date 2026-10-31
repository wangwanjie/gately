package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/i18n"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/resolver"
	"github.com/wangwanjie/gately/internal/thirdauth"
	"github.com/wangwanjie/gately/internal/webpage"
)

// Settings 是全局设置（settings.json）。
type Settings struct {
	// Listen 为管理界面 HTTP 监听地址，例如 :16680
	Listen string `json:"listen"`
	// HTTPSListen 非空时额外以 HTTPS 提供管理界面，证书由证书模块按域名提供
	HTTPSListen string `json:"httpsListen"`
	// ForceHTTPS 把 HTTP 访问重定向到 HTTPS（仅当证书模块有该域名的证书时）
	ForceHTTPS bool `json:"forceHTTPS"`
	// SafePath 为安全入口：设置后需先访问该路径才能打开登录页，其他路径返回 404
	SafePath string `json:"safePath"`
	// LANOnly 只允许内网地址（含 Tailscale 等 CGNAT 地址）访问管理界面
	LANOnly bool `json:"lanOnly"`
	// GatewaySSO 为 true 时，经飞牛 fnOS 统一网关访问的飞牛管理员自动以 Gately 管理员身份登录（跳过两步验证）
	GatewaySSO bool `json:"gatewaySSO"`
	// DisableProxyHeaders 为 true 时不再信任本机反向代理传来的 X-Real-IP / X-Forwarded-For，一律以 TCP 对端为客户端地址。
	// 默认信任：管理界面放在 nginx、haproxy 或 Gately Web 服务之后时，仅内网访问、登录限流与审计日志才能看到真实来源。
	DisableProxyHeaders bool `json:"disableProxyHeaders"`
	// TrustedProxies 为本机之外额外信任的代理地址（IP 或 CIDR），例如 Docker 网桥网关 172.17.0.1
	TrustedProxies []string `json:"trustedProxies"`
	// MaxLoginFailures / LockMinutes 为登录限流参数
	MaxLoginFailures int `json:"maxLoginFailures"`
	LockMinutes      int `json:"lockMinutes"`
	// LegacyAPIToken 是旧版的单个全局 API 令牌，仅用于启动时迁移为"遗留令牌"，迁移后清空。
	// 现在的令牌由 apitoken 包管理，保存在 tokens.json。
	LegacyAPIToken string `json:"apiToken,omitempty"`
	// Language 为服务端语言（日志、通知等无请求上下文的输出）；为空时按系统环境（GATELY_LANG / LANG）自动选择，
	// 环境变量未指定语言时跟随 UILanguage
	Language string `json:"language"`
	// UILanguage 为最近一次登录管理界面时界面使用的语言，由服务端在登录成功时记录，客户端提交的值被忽略
	UILanguage string `json:"uiLanguage,omitempty"`
	// TimeZone 为 IANA 时区名，例如 Asia/Shanghai；为空使用系统时区
	TimeZone string `json:"timeZone"`
	// DNSServers 为自定义 DNS 服务器，影响所有出站域名解析（DDNS、Webhook、ACME 等）；
	// 支持 IP、IP:端口、doh@主机 与 https:// DoH 地址
	DNSServers []string `json:"dnsServers"`
	// UpdateProxy 为检查更新与在线升级访问 GitHub 时使用的代理，支持 http、https 与 socks5；
	// 为空时直连（仍遵循 HTTPS_PROXY 等环境变量）
	UpdateProxy string `json:"updateProxy"`
	// LogLines 为每个模块保留的日志条数
	LogLines int `json:"logLines"`
	// DisableLogPersist 为 true 时运行日志只保留在内存，重启后丢失；默认写入配置目录下的 logs/
	DisableLogPersist bool `json:"disableLogPersist"`
	// LogFileMB 为每个日志文件的大小上限（MB），超过后轮转并保留一份历史，0 表示默认 2 MB
	LogFileMB int `json:"logFileMB"`
	// GCPercent / MaxProcs 为运行时调优参数，0 表示默认
	GCPercent int `json:"gcPercent"`
	MaxProcs  int `json:"maxProcs"`
	// ThirdLogin 为管理界面的第三方登录（GitHub / Google / QQ / OIDC）
	ThirdLogin thirdauth.LoginConfig `json:"thirdLogin"`
	// DisabledModules 为关闭的功能模块 ID：只加载配置，不启动服务、不提供管理接口，界面中隐藏；修改后重启生效。
	// 采用关闭列表，升级后新增的模块默认开启
	DisabledModules []string `json:"disabledModules"`
}

// 默认值
const (
	defaultLogLines = 2000
	maxLogLines     = 20000
	maxLogFileMB    = 100
)

func (s *Settings) normalize() {
	if s.Listen == "" {
		s.Listen = DefaultListen
	}
	if s.LogLines <= 0 {
		s.LogLines = defaultLogLines
	}
	s.SafePath = strings.TrimSpace(s.SafePath)
	if s.SafePath != "" && !strings.HasPrefix(s.SafePath, "/") {
		s.SafePath = "/" + s.SafePath
	}
	s.SafePath = strings.TrimRight(s.SafePath, "/")
	var proxies []string
	for _, p := range s.TrustedProxies {
		if p = strings.TrimSpace(p); p != "" {
			proxies = append(proxies, p)
		}
	}
	s.TrustedProxies = proxies
	var dns []string
	for _, d := range s.DNSServers {
		if d = strings.TrimSpace(d); d != "" {
			dns = append(dns, d)
		}
	}
	s.DNSServers = dns
	s.UpdateProxy = strings.TrimSpace(s.UpdateProxy)
	s.ThirdLogin.Normalize()
	s.Language = i18n.Normalize(s.Language)
	s.UILanguage = i18n.Normalize(s.UILanguage)
	// 去空、去重并排序，便于比较是否变化
	var off []string
	for _, id := range s.DisabledModules {
		if id = strings.TrimSpace(id); id != "" && !slices.Contains(off, id) {
			off = append(off, id)
		}
	}
	slices.Sort(off)
	s.DisabledModules = off
}

// Validate 校验设置。
func (s *Settings) Validate() error {
	s.normalize()
	for _, addr := range []string{s.Listen, s.HTTPSListen} {
		if addr == "" {
			continue
		}
		if _, port, err := net.SplitHostPort(addr); err != nil || port == "" {
			return fmt.Errorf("监听地址 %q 格式错误，应为 :端口 或 IP:端口", addr)
		}
	}
	if s.HTTPSListen != "" && s.HTTPSListen == s.Listen {
		return errors.New("HTTPS 监听地址不能与 HTTP 相同")
	}
	if _, err := httpx.ParsePrefixes(s.TrustedProxies); err != nil {
		return fmt.Errorf("可信代理地址无效: %w", err)
	}
	if s.ForceHTTPS && s.HTTPSListen == "" {
		return errors.New("强制 HTTPS 需要先设置 HTTPS 监听地址")
	}
	if s.SafePath != "" {
		if s.SafePath == "/api" || strings.HasPrefix(s.SafePath, "/api/") || strings.HasPrefix(s.SafePath, "/assets") || strings.ContainsAny(s.SafePath, " ?#") {
			return errors.New("安全入口不能以 /api、/assets 开头，且不能包含空格、? 或 #")
		}
		if len(s.SafePath) < 4 {
			return errors.New("安全入口至少 3 个字符")
		}
	}
	if s.MaxLoginFailures < 0 || s.MaxLoginFailures > 100 {
		return errors.New("连续登录失败上限应在 1~100 之间")
	}
	if s.LockMinutes < 0 || s.LockMinutes > 7*24*60 {
		return errors.New("锁定时长应在 1 分钟到 7 天之间")
	}
	if s.TimeZone != "" {
		if _, err := time.LoadLocation(s.TimeZone); err != nil {
			return fmt.Errorf("时区 %q 无效", s.TimeZone)
		}
	}
	if err := resolver.Validate(s.DNSServers); err != nil {
		return err
	}
	if s.UpdateProxy != "" {
		if err := validateProxyURL(s.UpdateProxy); err != nil {
			return err
		}
	}
	if s.LogLines > maxLogLines {
		return fmt.Errorf("日志条数不能超过 %d", maxLogLines)
	}
	if s.LogFileMB < 0 || s.LogFileMB > maxLogFileMB {
		return fmt.Errorf("日志文件上限应在 1~%d MB 之间", maxLogFileMB)
	}
	if s.GCPercent < 0 || s.GCPercent > 1000 || s.MaxProcs < 0 || s.MaxProcs > 1024 {
		return errors.New("运行时参数超出范围")
	}
	return nil
}

// restartFields 返回需要重启才能生效的字段是否变化。
func restartRequired(old, cur Settings) bool {
	return old.Listen != cur.Listen || old.HTTPSListen != cur.HTTPSListen || !slices.Equal(old.DisabledModules, cur.DisabledModules)
}

// applyRuntime 应用无需重启即可生效的设置。
func (a *App) applyRuntime() {
	s := a.settingsSnapshot()
	i18n.SetServerLang(s.Language)
	i18n.SetUILang(s.UILanguage)
	a.auth.SetLimits(s.MaxLoginFailures, time.Duration(s.LockMinutes)*time.Minute)
	// 已通过 Validate，解析不会失败
	extra, _ := httpx.ParsePrefixes(s.TrustedProxies)
	httpx.SetProxyTrust(!s.DisableProxyHeaders, extra)
	// 时区变化时才替换 time.Local：它是全局变量，其他协程随时在读，未改时不重复写
	if s.TimeZone != "" && time.Local.String() != s.TimeZone {
		if loc, err := time.LoadLocation(s.TimeZone); err == nil {
			time.Local = loc
		}
	}
	if err := resolver.Install(s.DNSServers); err != nil {
		a.sysLogs.Add(logbuf.LevelWarn, "settings", "部分 DNS 服务器无效: "+err.Error())
	}
	a.sysLogs.Resize(s.LogLines)
	for _, e := range a.modules {
		e.logs.Resize(s.LogLines)
	}
	a.applyLogPersist(s)
	if s.GCPercent > 0 {
		debug.SetGCPercent(s.GCPercent)
	} else {
		debug.SetGCPercent(100)
	}
	if s.MaxProcs > 0 {
		runtime.GOMAXPROCS(s.MaxProcs)
	}
}

func (a *App) settingsSnapshot() Settings {
	a.settingsMu.RLock()
	defer a.settingsMu.RUnlock()
	return a.settings
}

// settingsView 是返回给前端的设置。
type settingsView struct {
	Settings
	// SessionHours 为登录会话有效期，保存在账号中
	SessionHours int `json:"sessionHours"`
	// RestartRequired 表示有修改需要重启后生效
	RestartRequired bool `json:"restartRequired"`
	// GatewayAvailable 表示以统一网关方式运行（飞牛 fpk 包），界面据此显示飞牛登录态自动登录开关；只读
	GatewayAvailable bool `json:"gatewayAvailable"`
	// Modules 为全部功能模块及其依赖关系，设置页据此渲染开关；只读
	Modules []moduleOption `json:"modules"`
}

func (a *App) viewSettings() settingsView {
	s := a.settingsSnapshot()
	s.LegacyAPIToken = ""
	s.ThirdLogin = s.ThirdLogin.Masked()
	return settingsView{Settings: s, SessionHours: a.auth.TokenTTLHours(), RestartRequired: a.restartPending.Load(), GatewayAvailable: a.opts.GatewaySocket != "", Modules: a.moduleOptions()}
}

func (a *App) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	httpx.OK(w, a.viewSettings())
}

// handleSettingsSecrets 返回管理界面第三方登录的 Client Secret 明文，按登录方式标识索引（{"github": "密钥"}）。
func (a *App) handleSettingsSecrets(w http.ResponseWriter, r *http.Request) {
	httpx.OK(w, a.settingsSnapshot().ThirdLogin.Secrets())
}

func (a *App) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var req settingsView
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	next := req.Settings
	if err := next.Validate(); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if err := a.validateDisabledModules(next); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if req.SessionHours != 0 {
		if err := a.auth.SetTokenTTL(req.SessionHours); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
	}
	a.settingsMu.Lock()
	old := a.settings
	// 第三方登录的 Client Secret 在界面上是脱敏的，需要用已保存的值还原后再校验
	if err := next.ThirdLogin.Validate(old.ThirdLogin); err != nil {
		a.settingsMu.Unlock()
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	// 令牌不再通过全局设置管理，忽略客户端提交的旧字段
	next.LegacyAPIToken = old.LegacyAPIToken
	// 界面语言只由登录记录，不随表单提交改变
	next.UILanguage = old.UILanguage
	// 命令行 -listen 覆盖时（如 OpenWrt 的 LuCI 中设置了端口）：没改监听地址就保留配置文件中的原地址；
	// 在界面上改了则写入配置文件，运行中仍以覆盖值为准，去掉覆盖后生效
	if a.opts.Listen != "" && next.Listen == old.Listen {
		next.Listen = a.fileListen
	}
	if err := a.store.Save(settingsName, next); err != nil {
		a.settingsMu.Unlock()
		httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
		return
	}
	if a.opts.Listen != "" {
		a.fileListen = next.Listen
		next.Listen = old.Listen
	}
	a.settings = next
	a.settingsMu.Unlock()
	if restartRequired(old, next) {
		a.restartPending.Store(true)
	}
	a.applyRuntime()
	a.sysLogs.Add(logbuf.LevelInfo, "settings", "全局设置已更新")
	httpx.OK(w, a.viewSettings())
}

// ---- 访问控制中间件 ----

// entryCookie 记录已通过安全入口的浏览器。
const entryCookie = "gately_entry"

func (a *App) entryValue(path string) string {
	m := hmac.New(sha256.New, []byte(a.auth.EntrySecret()))
	m.Write([]byte("entry|" + path))
	return hex.EncodeToString(m.Sum(nil))[:32]
}

// isPrivate 判断地址是否属于内网（含回环、链路本地、CGNAT 与 IPv6 ULA）。
func isPrivate(ip string) bool {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	a = a.Unmap()
	if a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() {
		return true
	}
	return netip.MustParsePrefix("100.64.0.0/10").Contains(a)
}

// accessControl 依次执行：内网限制、强制 HTTPS、安全入口。
// 经统一网关访问时全部跳过：访问控制由飞牛负责（登录态、两步验证，本应用仅管理员可见），入口地址固定，协议由网关决定。
// 网关转发的客户端地址来自请求头、可被伪造，也无法据此判断是否内网，因此“仅内网访问”只约束直接访问 TCP 端口。
func (a *App) accessControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, viaGateway := gatewayPrefix(r); viaGateway {
			next.ServeHTTP(w, r)
			return
		}
		s := a.settingsSnapshot()
		// 被封禁的地址（入侵防护、Web 服务认证失败等）不能访问管理后台；内网与回环地址除外，
		// 避免在 Web 站点上输错几次密码就把管理员自己锁在外面
		if ip, err := netip.ParseAddr(httpx.ClientIP(r)); err == nil && !isPrivate(ip.String()) && banlist.Default.Banned(ip) {
			http.Error(w, "403 Forbidden", http.StatusForbidden)
			return
		}
		if s.LANOnly && !isPrivate(httpx.ClientIP(r)) {
			http.Error(w, "403 Forbidden", http.StatusForbidden)
			return
		}
		if s.ForceHTTPS && r.TLS == nil && a.httpsReady.Load() && a.hasCert(r.Host) {
			_, port, _ := net.SplitHostPort(s.HTTPSListen)
			target := "https://" + hostOnly(r.Host)
			if port != "443" {
				target += ":" + port
			}
			http.Redirect(w, r, target+r.URL.RequestURI(), http.StatusFound)
			return
		}
		if s.SafePath != "" {
			if r.URL.Path == s.SafePath || r.URL.Path == s.SafePath+"/" {
				http.SetCookie(w, &http.Cookie{Name: entryCookie, Value: a.entryValue(s.SafePath), Path: "/", HttpOnly: true,
					SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil, MaxAge: 365 * 24 * 3600})
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}
			c, err := r.Cookie(entryCookie)
			passed := err == nil && hmac.Equal([]byte(c.Value), []byte(a.entryValue(s.SafePath)))
			if !passed && !a.tokenPeek(r) {
				// 已登录的会话（例如修改安全入口之前登录）仍可访问
				if _, ok := a.auth.Verify(auth.TokenFromRequest(r)); !ok {
					// 浏览器访问时给出提示页（仍返回 404），接口等其他请求保持纯文本 404
					webpage.Error(w, r, webpage.Problem{
						Code:   http.StatusNotFound,
						Lock:   true,
						Title:  "请通过安全入口访问",
						Detail: "当前地址未通过安全入口验证，无法打开管理后台。",
						Hint:   "请使用完整的安全入口地址（例如 http://域名:端口/入口路径）访问；忘记入口时，可在配置目录的 gately.json 中清空 safePath 后重启 Gately。",
					})
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// hasCert 判断证书模块是否有该域名的证书。
func (a *App) hasCert(host string) bool {
	if a.opts.Certs == nil {
		return false
	}
	return a.opts.Certs.HasCertificate(hostOnly(host))
}

func hostOnly(h string) string {
	if host, _, err := net.SplitHostPort(h); err == nil {
		return host
	}
	return strings.Trim(h, "[]")
}

// rememberUILanguage 记录登录时管理界面使用的语言（前端请求的 Accept-Language 即界面语言）。
// 服务端语言为「跟随系统」且环境变量未指定语言时，日志与通知按该语言输出。
func (a *App) rememberUILanguage(r *http.Request) {
	lang := i18n.FromAcceptLanguage(r.Header.Get("Accept-Language"))
	if lang == "" {
		return
	}
	if err := a.setUILanguage(lang); err != nil {
		a.sysLogs.Add(logbuf.LevelWarn, "settings", "保存管理界面语言失败: "+err.Error())
	}
}

// setUILanguage 保存管理界面语言并立即生效；语言没变时不写配置文件。
func (a *App) setUILanguage(lang string) error {
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	if a.settings.UILanguage == lang {
		return nil
	}
	next := a.settings
	next.UILanguage = lang
	// 命令行 -listen 覆盖时写回配置文件中的原地址
	if a.opts.Listen != "" {
		next.Listen = a.fileListen
	}
	if err := a.store.Save(settingsName, next); err != nil {
		return err
	}
	a.settings.UILanguage = lang
	i18n.SetUILang(lang)
	return nil
}

// handleUILanguage 在管理员登录状态下切换界面语言时调用，记录新的界面语言。
// 位于 /api/account 下：API 令牌与只读账号不能访问，只有管理员的界面语言影响服务端输出。
func (a *App) handleUILanguage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Language string `json:"language"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	lang := i18n.Normalize(req.Language)
	if lang == "" {
		httpx.Error(w, http.StatusBadRequest, "不支持的语言 %q", req.Language)
		return
	}
	if err := a.setUILanguage(lang); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
		return
	}
	httpx.OK(w, map[string]string{"uiLanguage": lang})
}
