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
	"path/filepath"
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
	"github.com/wangwanjie/gately/internal/outbound"
	"github.com/wangwanjie/gately/internal/resolver"
	"github.com/wangwanjie/gately/internal/thirdauth"
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
	// DenyMode 为未授权访问（来源地址被封禁、仅内网访问拦截、未通过安全入口）时的响应方式，取值见 deny.go；
	// 为空时保持默认：安全入口返回 404 提示页，封禁与仅内网返回 403
	DenyMode string `json:"denyMode"`
	// DenyStatus / DenyPage 为 DenyMode 为 custom 时返回的状态码与 HTML 内容
	DenyStatus int    `json:"denyStatus"`
	DenyPage   string `json:"denyPage"`
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
	// LoginGlobalLimit 为陌生来源（从未成功登录过的地址）10 分钟内允许累计的登录失败总数：用完后陌生地址的登录暂时受限，
	// 常用地址不受影响，用来抵御靠大量不同地址分散尝试、绕过单 IP 限流的爆破。0 表示默认 20，负数表示关闭
	LoginGlobalLimit int `json:"loginGlobalLimit"`
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
	// RestrictOutbound 为 true 时，管理员配置的出站 HTTP 请求（Webhook、通知、IP 列表订阅、DDNS 回调与查询地址、
	// 更新与程序下载、第三方登录等）在拨号层拒绝访问回环、私网、链路本地（含 169.254.169.254）与 ULA 等地址，
	// 同时覆盖重定向与 DNS 重绑定。默认关闭以保持内网用法；反向代理上游、WOL、存储挂载等面向内网的功能不受影响
	RestrictOutbound bool `json:"restrictOutbound"`
	// DisableUpdateCheck 为 true 时不再定期检查更新与安全公告，只在点击“检查更新”时访问 GitHub。
	// 默认开启：安全修复版本发布后需要尽快让用户知道
	DisableUpdateCheck bool `json:"disableUpdateCheck"`
	// LogLines 为每个模块保留的日志条数
	LogLines int `json:"logLines"`
	// DisableLogPersist 为 true 时运行日志只保留在内存，重启后丢失；默认写入配置目录下的 logs/
	DisableLogPersist bool `json:"disableLogPersist"`
	// LogFileMB 为每个日志文件的大小上限（MB），超过后轮转并保留一份历史，0 表示默认 2 MB；
	// Web 服务访问日志另按 AccessLogDays、AccessLogMB 归档
	LogFileMB int `json:"logFileMB"`
	// AccessLogDays 为访问日志（Web 服务）归档的保留天数，0 表示默认 7 天
	AccessLogDays int `json:"accessLogDays"`
	// AccessLogMB 为访问日志当前文件与全部压缩归档的总大小上限（MB），0 表示默认：32 MB，
	// 但不超过日志所在分区容量的 5%，闪存很小的路由器上自动调低
	AccessLogMB int `json:"accessLogMB"`
	// LogDir 为运行日志的保存目录（绝对路径），为空时使用配置目录下的 logs/。
	// 修改后新日志写入新目录，原目录中的日志文件保留不动
	LogDir string `json:"logDir"`
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
	// 访问日志保留天数与总大小的取值范围
	maxAccessLogDays = 3650
	minAccessLogMB   = 4
	maxAccessLogMB   = 100 << 10
	// maxLogDirLen 限制日志目录路径长度
	maxLogDirLen = 1024
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
	s.DenyMode = strings.ToLower(strings.TrimSpace(s.DenyMode))
	// 状态码只在自定义页面下使用；其他方式下的无效值（例如手工改坏的配置文件）恢复为 200，不影响保存其他设置
	if s.DenyStatus == 0 || (s.DenyMode != denyModeCustom && !slices.Contains(denyStatuses, s.DenyStatus)) {
		s.DenyStatus = http.StatusOK
	}
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
	if s.LogDir = strings.TrimSpace(s.LogDir); s.LogDir != "" && filepath.IsAbs(s.LogDir) {
		s.LogDir = filepath.Clean(s.LogDir)
	}
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
	if err := validateDeny(s); err != nil {
		return err
	}
	if s.MaxLoginFailures < 0 || s.MaxLoginFailures > 100 {
		return errors.New("连续登录失败上限应在 1~100 之间")
	}
	if s.LockMinutes < 0 || s.LockMinutes > 7*24*60 {
		return errors.New("锁定时长应在 1 分钟到 7 天之间")
	}
	if s.LoginGlobalLimit > 10000 {
		return errors.New("陌生来源的失败预算不能超过 10000")
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
	if s.AccessLogDays < 0 || s.AccessLogDays > maxAccessLogDays {
		return fmt.Errorf("访问日志保留天数应在 1~%d 之间", maxAccessLogDays)
	}
	if s.AccessLogMB != 0 && (s.AccessLogMB < minAccessLogMB || s.AccessLogMB > maxAccessLogMB) {
		return fmt.Errorf("访问日志总大小应在 %d~%d MB 之间", minAccessLogMB, maxAccessLogMB)
	}
	if err := validateLogDir(s.LogDir); err != nil {
		return err
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
	a.applyLoginBudget(s)
	// 已通过 Validate，解析不会失败
	extra, _ := httpx.ParsePrefixes(s.TrustedProxies)
	httpx.SetProxyTrust(!s.DisableProxyHeaders, extra)
	// 出站请求限制随设置立即生效，已缓存的客户端在下次建立连接时读取
	outbound.SetRestricted(s.RestrictOutbound)
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
	// 新填写的日志目录要能创建、能写入，否则保存后日志只能留在内存里，用户却不知道
	if next.LogDir != "" && !next.DisableLogPersist && next.LogDir != a.settingsSnapshot().LogDir {
		if err := a.checkLogDir(next.LogDir); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
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
	// 没有编译进来的模块不在设置页中，保留它们原来的开关
	a.keepAbsentDisabled(old.DisabledModules, &next)
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
	if old.DisableUpdateCheck && !next.DisableUpdateCheck {
		// 重新开启自动检查后按上次检查时间重新计时，不必等到原定的下一次
		a.wakeUpdateCheck()
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
			a.deny(w, r, s, deny403)
			return
		}
		if s.LANOnly && !isPrivate(httpx.ClientIP(r)) {
			a.deny(w, r, s, deny403)
			return
		}
		// 安全入口：isEntry 为访问入口路径本身，passed 为已越过入口（未设置入口时恒为 true）
		isEntry, passed := false, true
		if s.SafePath != "" {
			isEntry = isEntryPath(r, s.SafePath)
			passed = isEntry || a.entryPassed(r, s.SafePath)
		}
		// 隐藏管理端口的响应方式下先拦截未越过入口的请求：未授权的访客不能收到强制 HTTPS 的跳转，那会暴露 HTTPS 端口与服务的存在。
		// 入口路径本身照常跳转到 HTTPS，由 HTTPS 端写入入口 Cookie
		if !passed && s.DenyMode != denyModeDefault {
			a.deny(w, r, s, denyEntryHint)
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
		if isEntry {
			http.SetCookie(w, &http.Cookie{Name: entryCookie, Value: a.entryValue(s.SafePath), Path: "/", HttpOnly: true,
				SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil, MaxAge: 365 * 24 * 3600})
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		if !passed {
			a.deny(w, r, s, denyEntryHint)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isEntryPath 判断请求的是否为安全入口路径本身。
func isEntryPath(r *http.Request, safePath string) bool {
	return r.URL.Path == safePath || r.URL.Path == safePath+"/"
}

// entryPassed 判断请求能否越过安全入口：带有效的入口 Cookie、签过名的主屏幕图标地址、API 令牌、
// 模块免登录接口的有效凭据或已登录的会话。
func (a *App) entryPassed(r *http.Request, safePath string) bool {
	if c, err := r.Cookie(entryCookie); err == nil && hmac.Equal([]byte(c.Value), []byte(a.entryValue(safePath))) {
		return true
	}
	// 主屏幕图标请求不带 Cookie，凭页面中签过名的地址放行（见 webclip.go）
	// 模块免登录接口（如网络唤醒客户端）带有效凭据时放行，客户端无法先访问入口路径拿 Cookie
	if a.iconKeyValid(r, safePath) || a.tokenPeek(r) || a.publicAuth(r) {
		return true
	}
	// 已登录的会话（例如修改安全入口之前登录）仍可访问
	_, ok := a.auth.Verify(auth.TokenFromRequest(r))
	return ok
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
