package webservice

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/hostname"
	"github.com/wangwanjie/gately/internal/iplist"
	"github.com/wangwanjie/gately/internal/thirdauth"
	"github.com/wangwanjie/gately/internal/visitors"
	"golang.org/x/net/http/httpguts"
)

// 站点类型
const (
	TypeProxy    = "proxy"
	TypeRedirect = "redirect"
	TypeStatic   = "static"
	TypeText     = "text"
	// TypePanel 输出面板库中的一个导航面板，把多个服务的链接汇总在一页
	TypePanel = "panel"
	// TypeClose 不返回任何响应，直接断开连接（相当于 nginx 的 return 444），常用于默认站点拒绝未知域名的扫描
	TypeClose = "close"
)

// hostPlaceholder 在重定向地址中代表访客请求的域名。
const hostPlaceholder = "{host}"

// 访问控制模式
const (
	IPModeOff       = ""
	IPModeWhitelist = "whitelist"
	IPModeBlacklist = "blacklist"
)

// Site 是监听器下按域名匹配的一个站点。
type Site struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Enable bool   `json:"enable"`
	// Group 为列表中的分组名称，只影响管理界面的展示，留空为未分组；默认站点不分组
	Group string `json:"group"`
	// Domains 支持精确域名、*.example.com（任意级子域名）、api-*.example.com（首段内通配）与 *（匹配所有）
	Domains []string `json:"domains"`
	Type    string   `json:"type"`

	// 反向代理
	Targets            []string `json:"targets"`
	InsecureSkipVerify bool     `json:"insecureSkipVerify"`
	// UseTargetHost 为 true 时 Host 头使用目标地址，否则保留客户端请求的 Host
	UseTargetHost     bool `json:"useTargetHost"`
	DisableKeepAlives bool `json:"disableKeepAlives"`
	// SendProxyProtocol 向后端发送 PROXY v2 头传递真实客户端 IP（需要后端支持）
	SendProxyProtocol bool `json:"sendProxyProtocol"`
	// ForwardedHeaders 添加 X-Forwarded-For/Proto/Host/Port 与 X-Real-IP
	ForwardedHeaders bool `json:"forwardedHeaders"`
	// UpstreamSNIFromHost 为 true 时连接 HTTPS 后端使用访客请求的域名作为 SNI 并按它校验证书
	// （相当于 nginx 的 proxy_ssl_server_name on + proxy_ssl_name $host），后端按 SNI 分流时（如 frp 的 vhost_https）需要开启
	UpstreamSNIFromHost bool `json:"upstreamSniFromHost"`
	// PassAuthCookies 为 true 时把 Gately 的登录 Cookie（单点登录、站点会话、第三方登录随机数）原样转发给后端，
	// 用于后端是另一台 Gately 并由它负责认证的场景（如公网 Gately 只做 WAF、内网 Gately 负责登录）；
	// 默认去掉这些 Cookie，避免把登录凭据交给后端程序。本站点启用认证时不能开启，否则会把本机签发的凭据转给后端
	PassAuthCookies bool `json:"passAuthCookies"`
	// Locations 为按路径前缀分到不同后端的规则，最长前缀优先，未命中时使用站点后端
	Locations []Location `json:"locations"`
	// UserRoutes 为按认证用户分到不同后端的规则，需启用认证，见 UserRoute
	UserRoutes []UserRoute `json:"userRoutes"`
	// IdentityHeaders 把认证用户的用户名、邮箱、显示名、分组写入指定请求头传给后端，见 IdentityHeader
	IdentityHeaders []IdentityHeader `json:"identityHeaders"`
	// UserHeader 为旧版的“传递用户名”请求头，运行时视为一行用户名身份请求头，保存时并入 IdentityHeaders 并清空
	UserHeader string `json:"userHeader,omitempty"`

	// 重定向：RedirectTo 为目标地址前缀，可用 {host} 代表访客请求的域名；KeepPath 时附加原路径与查询串
	RedirectTo   string `json:"redirectTo"`
	RedirectCode int    `json:"redirectCode"`
	KeepPath     bool   `json:"keepPath"`

	// 静态文件
	Root    string `json:"root"`
	ListDir bool   `json:"listDir"`
	// CleanURLs 时找不到文件会再尝试 路径.html 与 路径/index.html
	CleanURLs bool `json:"cleanUrls"`
	// NotFoundPage 为文件不存在时返回的页面，相对 Root 的路径，例如 404.html
	NotFoundPage string `json:"notFoundPage"`

	// 导航面板：Panel 为引用的面板 key
	Panel string `json:"panel"`

	// 固定文本
	Text        string `json:"text"`
	ContentType string `json:"contentType"`
	StatusCode  int    `json:"statusCode"`

	// 认证：Users 每项为 用户名:密码（密码可为 bcrypt 哈希）
	BasicAuth bool     `json:"basicAuth"`
	WebAuth   bool     `json:"webAuth"`
	Users     []string `json:"users"`
	// SharedUsers 为 true 时网页认证与基本认证也接受全局设置中的共享用户（支持 TOTP 与跨站单点登录）
	SharedUsers bool `json:"sharedUsers"`
	// OAuthUsers 为允许通过第三方登录访问的身份，格式 提供方key:标识（用户名、邮箱、稳定 ID 均可，行尾可用 " #" 加备注），提供方key:* 表示该提供方任意用户
	OAuthUsers []string `json:"oauthUsers"`

	IPMode string   `json:"ipMode"`
	IPList []string `json:"ipList"`
	// IPSets 为引用的订阅 IP 列表 key，与 IPList 一起参与黑白名单判断
	IPSets []string `json:"ipSets"`
	// IPDenyList 与 IPDenySets 是优先拒绝名单：无论访问控制模式与路径规则如何，命中的来源一律拒绝，
	// 在白名单、黑名单之前判断，因此同时在白名单中的地址也会被拒绝（先黑后白）
	IPDenyList []string `json:"ipDenyList"`
	IPDenySets []string `json:"ipDenySets"`

	// CountryMode 为 ""（不限）、allow（仅允许列出的国家）、deny（拒绝列出的国家），需先在设置中导入 GeoIP 数据库
	CountryMode string `json:"countryMode"`
	// Countries 为两位 ISO 国家代码，如 CN、US
	Countries []string `json:"countries"`
	// DenyClose 为 true 时，被封禁、IP 访问控制（含路径规则）或国家过滤拒绝的请求不返回 403，
	// 而是像断开连接站点一样不写响应直接断开（访问日志记为 444），用于“自家 IP 放行、其余断开”
	DenyClose bool `json:"denyClose"`
	// ForwardAuth 把认证交给外部服务（如 Authelia），与基本认证、网页认证不能同时使用
	ForwardAuth ForwardAuth `json:"forwardAuth"`
	// AuthOptional 为 true 时，认证只作用于路径规则中标记为"必须认证"的路径，其余路径公开
	AuthOptional bool `json:"authOptional"`
	// TrustAuthUsers 为 true 时，带有本站点 Gately 认证有效凭据（网页认证与第三方登录的会话、共享用户单点登录、
	// 基本认证）的请求不经过 WAF，也不交给入侵防护的路径探测规则计数（issue #81，见 trustauth.go）。
	// 需启用基本认证或网页认证；转发认证由外部服务判断身份，不适用
	TrustAuthUsers bool `json:"trustAuthUsers"`
	// TrustBackendAuth 为 true 时（仅反向代理站点、本站点不启用认证），后端是另一台负责认证的 Gately：
	// 后端对受信任的已登录用户返回带签名的认证标记（全局设置的认证标记密钥须一致），本站点校验后让该凭据的请求
	// 不经过 WAF、不计入路径探测，并在返回访客前去掉标记头（issue #81，见 authmarker.go）
	TrustBackendAuth bool `json:"trustBackendAuth"`
	// Rules 为按路径前缀覆盖认证与 IP 访问控制的规则
	Rules []PathRule `json:"rules"`
	// ShareLinks 为临时分享链接，持令牌访客可绕过认证
	ShareLinks []ShareLink `json:"shareLinks"`

	// 反向代理增强：Targets 每项可附 weight=N、backup；LBPolicy 为 ""（轮询）、random、ip_hash、least_conn
	LBPolicy string      `json:"lbPolicy"`
	Health   HealthCheck `json:"health"`
	// Timeouts 为连接后端的超时，分路径与按用户分流的后端同样适用
	Timeouts BackendTimeouts `json:"timeouts"`
	// ClientTimeouts 为对访客的读写空闲超时，所有站点类型（断开连接除外）都适用
	ClientTimeouts ClientTimeouts `json:"clientTimeouts"`
	Compress       bool           `json:"compress"`
	RateLimit      RateLimit      `json:"rateLimit"`
	Cache          CacheConfig    `json:"cache"`
	Rewrites       []Rewrite      `json:"rewrites"`

	// RequestHeaders / ResponseHeaders 每项为 "Key: Value"，Value 为空表示删除该头
	RequestHeaders  []string `json:"requestHeaders"`
	ResponseHeaders []string `json:"responseHeaders"`
	CORS            bool     `json:"cors"`
	AccessLog       bool     `json:"accessLog"`
	// WAF 为引用的 WAF 实例 key，留空不检查；实例不存在或停用时放行
	WAF string `json:"waf"`
	// ErrorPage 为 404 与 5xx 错误页的样式，以及（反向代理站点）拦截后端错误的设置，见 errorpages.go。
	// 默认站点的设置同时用于没有任何站点匹配时的 404（默认站点未启用时同样生效）
	ErrorPage ErrorPageOptions `json:"errorPage"`
	Remark    string           `json:"remark"`

	// scope 为 监听器key/站点key，运行时用于隔离会话 Cookie，不持久化
	scope string
	// authBase 为登录页等认证保留路径的前缀，空表示 authPath；OIDC 身份提供方的登录页使用 oidcPath
	authBase string
	// loginClient 为登录后将前往的应用名称，显示在登录页副标题中（仅 OIDC 登录页）
	loginClient string
}

// authPrefix 返回站点认证保留路径的前缀。
func (s Site) authPrefix() string {
	if s.authBase != "" {
		return s.authBase
	}
	return authPath
}

// 开启 HTTPS 的监听器在同一端口收到明文 HTTP 请求时的处理方式。
const (
	// PlainHTTPRedirect 把明文请求跳转到同主机同端口的 https 地址
	PlainHTTPRedirect = "redirect"
	// PlainHTTPServe 直接按 HTTP 提供站点
	PlainHTTPServe = "serve"
)

// Listener 是一个监听端口及其站点。
type Listener struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Enable bool   `json:"enable"`
	// Network 为 tcp（双栈）、tcp4、tcp6
	Network  string `json:"network"`
	ListenIP string `json:"listenIp"`
	Port     int    `json:"port"`
	TLS      bool   `json:"tls"`
	// MinTLS 为 1.0 / 1.1 / 1.2 / 1.3
	MinTLS string `json:"minTls"`
	// HTTP3 在同一端口号的 UDP 上同时提供 HTTP/3（QUIC），需要开启 HTTPS
	HTTP3 bool `json:"http3"`
	// PlainHTTP 决定开启 HTTPS 后同一端口收到明文 HTTP 请求时的处理方式：
	// 空为不接受（与普通 HTTPS 端口相同），redirect 跳转到同端口的 https 地址，serve 直接按 HTTP 提供站点
	PlainHTTP string `json:"plainHttp"`
	// HTTPPort 为额外的明文 HTTP 端口，0 为不开启。它与主端口共用全部站点与设置，常用于 HTTPS 监听 443 时再开 80，
	// 不必为 HTTP 另建一个监听并重复填写站点
	HTTPPort int `json:"httpPort"`
	// HTTPPortMode 为额外 HTTP 端口收到请求时的处理方式：redirect 跳转到主端口的 https 地址，serve 直接按 HTTP 提供站点。
	// 开启 HTTPS 时默认 redirect；主端口本身是 HTTP 时只能 serve
	HTTPPortMode string `json:"httpPortMode"`
	// AcceptProxyProtocol 接收上游（如 haproxy、frp）发送的 PROXY 协议头，仅信任全局设置中的来源
	AcceptProxyProtocol bool `json:"acceptProxyProtocol"`
	// AccessScheme 与 AccessPort 为浏览器访问站点时使用的协议与端口，仅用于管理界面生成站点链接，不影响监听与转发。
	// 经协议分流、路由器端口映射、frp 或 CDN 访问时，对外的协议和端口可能与监听不同。
	// AccessScheme 为空时按是否开启 HTTPS 推算，AccessPort 为 0 时使用监听端口
	AccessScheme string `json:"accessScheme"`
	AccessPort   int    `json:"accessPort"`
	// AutoFirewall 为 true 时在本机防火墙（nftables / iptables / ip6tables 的 input 链）放行监听端口
	// （开启 HTTP/3 时还有同号 UDP 端口）；只放行监听成功的端口，绑定具体地址时只放行发往该地址的连接，
	// 只监听回环地址时不放行。监听停用、删除或启动失败时撤销。只对 Gately 所在主机的防火墙有效
	AutoFirewall bool   `json:"autoFirewall"`
	Sites        []Site `json:"sites"`
	// Default 在没有站点匹配时使用；未启用时返回 404
	Default Site   `json:"default"`
	Remark  string `json:"remark"`
}

// Settings 是模块级配置。
type Settings struct {
	Enable bool `json:"enable"`
	// TrustedProxies 为可信的 PROXY 协议来源（IP 或 CIDR）
	TrustedProxies []string `json:"trustedProxies"`
	// RealIPFrom 为可信代理（IP 或 CIDR，如 CDN / WAF 回源地址段）。原始对端（PROXY 协议处理后的地址）在其中时，
	// 按 RealIPHeaders 从请求头取得访客真实地址；为空表示不启用，一律以原始对端为客户端地址
	RealIPFrom []string `json:"realIPFrom"`
	// RealIPHeaders 为携带真实地址的请求头，按顺序查找，例如 CF-Connecting-IP、X-Forwarded-For、X-Real-IP
	RealIPHeaders []string `json:"realIPHeaders"`
	// RealPortHeader 为可信代理写入访客源端口的请求头（如 X-Real-Port），为空表示代理不提供端口。
	// 从请求头取得访客地址时，源端口只能来自这个头；取不到时端口未知，不向后端传递端口头
	RealPortHeader string `json:"realPortHeader"`
	// AuthSecret 用于签名网页认证 Cookie，首次启动自动生成
	AuthSecret string `json:"authSecret"`
	// OAuthProviders 为自建的第三方登录配置
	OAuthProviders []OAuthProvider `json:"oauthProviders"`
	// OAuthCallbackBase 为接收第三方登录回调的站点地址（需由 Gately 的某个监听提供），例如 https://auth.example.com
	OAuthCallbackBase string `json:"oauthCallbackBase"`
	// AutoBanSeconds 为认证连续失败后自动封禁来源 IP 的秒数，0 表示不自动封禁
	AutoBanSeconds int `json:"autoBanSeconds"`
	// AuthUsers 为可在多个站点共用的认证用户，站点开启“使用共享用户”后生效
	AuthUsers []AuthUser `json:"authUsers"`
	// SSODomain 为共享用户登录态的 Cookie 域（如 example.com），设置后该域及其子域名下的站点登录一次即可通用
	SSODomain string `json:"ssoDomain"`
	// OIDC 为内置的 OIDC 身份提供方，把共享用户提供给支持 OpenID Connect 的应用登录
	OIDC OIDCSettings `json:"oidc"`
	// SelfServicePassword 为 true 时共享用户可在开启“使用共享用户”的站点与 OIDC 签发者域名上自助修改密码（认证路径下的 /password）
	SelfServicePassword bool `json:"selfServicePassword"`
	// AuthMarkerKey 为认证标记密钥（至少 16 个字符），双层 Gately（外层只做 WAF、内层负责认证）两端填写相同的值：
	// 内层给受信任的已登录用户签发认证标记，外层开启信任后端 Gately 的认证标记的站点据此放行。留空不启用
	AuthMarkerKey string `json:"authMarkerKey"`
}

type fileConfig struct {
	Settings  Settings   `json:"settings"`
	Listeners []Listener `json:"listeners"`
	// IPLists 为订阅的 IP 列表，供站点访问控制引用
	IPLists []IPList `json:"ipLists"`
	// ErrorPages 为自定义错误页面库，供站点的错误页设置引用
	ErrorPages []ErrorPage `json:"errorPages"`
	// Visitors 为访客审计设置
	Visitors visitors.Settings `json:"visitors"`
	// Replay 为拓扑回放设置
	Replay ReplaySettings `json:"replay"`
	// Panels 为导航面板库，供导航面板类型的站点引用
	Panels []Panel `json:"panels"`
}

func cleanList(in []string) []string {
	var out []string
	for _, v := range in {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// uniqueList 去除空白项与重复项，保持原有顺序。
func uniqueList(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, v := range in {
		if v = strings.TrimSpace(v); v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// parsePrefixes 解析 IP 或 CIDR 列表。
func parsePrefixes(list []string) ([]netip.Prefix, error) {
	return iplist.ParsePrefixes(list)
}

// assignSiteKeys 为没有 key 或 key 重复的站点分配新 key。站点的统计、连接与拓扑都按 key 关联，
// 插在列表中间的新站点（例如复制的站点）不能抢走后面已有站点的 key。
func (l *Listener) assignSiteKeys() {
	crud.AssignKeys(len(l.Sites), func(i int) *string { return &l.Sites[i].Key }, "s")
}

// dropShareLinks 清空全部站点（含默认站点）的分享链接。复制监听时使用：分享链接的令牌可绕过认证，
// 不应随副本扩散到新的端口上。
func (l *Listener) dropShareLinks() {
	for i := range l.Sites {
		l.Sites[i].ShareLinks = nil
	}
	l.Default.ShareLinks = nil
}

// Validate 校验监听器及其站点。
func (l *Listener) Validate() error {
	l.Name = strings.TrimSpace(l.Name)
	switch l.Network {
	case "":
		l.Network = "tcp"
	case "tcp", "tcp4", "tcp6":
	default:
		return fmt.Errorf("不支持的网络类型 %q", l.Network)
	}
	l.ListenIP = strings.Trim(strings.TrimSpace(l.ListenIP), "[]")
	if l.ListenIP != "" && net.ParseIP(l.ListenIP) == nil {
		return fmt.Errorf("监听地址 %q 无效", l.ListenIP)
	}
	if l.Port < 1 || l.Port > 65535 {
		return errors.New("监听端口无效")
	}
	if l.HTTP3 && !l.TLS {
		return errors.New("HTTP/3 需要同时开启 HTTPS")
	}
	switch l.PlainHTTP {
	case "", PlainHTTPRedirect, PlainHTTPServe:
	default:
		return fmt.Errorf("不支持的明文 HTTP 处理方式 %q", l.PlainHTTP)
	}
	// 未开启 HTTPS 时端口本来就是明文 HTTP，这个选项没有意义
	if !l.TLS {
		l.PlainHTTP = ""
	}
	if err := l.normalizeHTTPPort(); err != nil {
		return err
	}
	switch l.MinTLS {
	case "", "1.0", "1.1", "1.2", "1.3":
	default:
		return fmt.Errorf("不支持的 TLS 版本 %q", l.MinTLS)
	}
	l.AccessScheme = strings.ToLower(strings.TrimSpace(l.AccessScheme))
	switch l.AccessScheme {
	case "", "http", "https":
	default:
		return fmt.Errorf("不支持的访问协议 %q", l.AccessScheme)
	}
	if l.AccessPort < 0 || l.AccessPort > 65535 {
		return errors.New("访问端口无效")
	}
	for i := range l.Sites {
		s := &l.Sites[i]
		if err := s.validate(false); err != nil {
			label := s.Name
			if label == "" && len(s.Domains) > 0 {
				label = s.Domains[0]
			}
			return fmt.Errorf("站点 %s: %w", label, err)
		}
	}
	l.assignSiteKeys()
	if l.Default.Enable {
		if err := l.Default.validate(true); err != nil {
			return fmt.Errorf("默认站点: %w", err)
		}
	} else if err := l.Default.ErrorPage.validate(); err != nil {
		// 默认站点停用时其错误页设置仍用于未匹配任何站点的 404，单独校验
		return fmt.Errorf("默认站点: %w", err)
	}
	l.Default.Key = "default"
	// 默认站点固定排在最后，不参与分组（停用时不经过 validate，这里单独清空）
	l.Default.Group = ""
	return nil
}

// normalizeHTTPPort 校验额外 HTTP 端口并补全处理方式。
func (l *Listener) normalizeHTTPPort() error {
	if l.HTTPPort == 0 {
		l.HTTPPortMode = ""
		return nil
	}
	if l.HTTPPort < 0 || l.HTTPPort > 65535 {
		return errors.New("HTTP 端口无效")
	}
	if l.HTTPPort == l.Port {
		return errors.New("HTTP 端口不能与监听端口相同")
	}
	switch l.HTTPPortMode {
	case "":
		l.HTTPPortMode = PlainHTTPRedirect
	case PlainHTTPRedirect, PlainHTTPServe:
	default:
		return fmt.Errorf("不支持的 HTTP 端口处理方式 %q", l.HTTPPortMode)
	}
	// 主端口是 HTTP 时没有可跳转的 HTTPS 地址，额外端口只是同一组站点的另一个入口
	if !l.TLS {
		l.HTTPPortMode = PlainHTTPServe
	}
	return nil
}

// httpsPort 返回从额外 HTTP 端口跳转时使用的 HTTPS 端口：填写了访问端口（且访问协议不是 http）时用它，
// 例如经路由器把外网 443 映射到本机 8443；否则用监听端口。
func (l *Listener) httpsPort() int {
	if l.AccessPort > 0 && l.AccessScheme != "http" {
		return l.AccessPort
	}
	return l.Port
}

// splitDomains 把域名分成规范化后可用的域名与无法使用的原始写法，导入外部配置时用它跳过个别错误域名，
// 避免一个域名写错导致整个监听被跳过。
func splitDomains(in []string) (ok, bad []string) {
	for _, raw := range in {
		d, valid := hostname.NormalizePattern(raw)
		switch {
		case !valid:
			bad = append(bad, strings.TrimSpace(raw))
		case d != "" && !slices.Contains(ok, d):
			ok = append(ok, d)
		}
	}
	return ok, bad
}

func (s *Site) validate(isDefault bool) error {
	s.Name = strings.TrimSpace(s.Name)
	if isDefault {
		s.Group = ""
	}
	if err := crud.NormalizeGroup(&s.Group); err != nil {
		return err
	}
	var domains []string
	for _, raw := range s.Domains {
		// 规范化为请求 Host 的形式：小写、IPv6 不带方括号、国际化域名转 punycode；
		// 带端口、路径、@ 等永远匹配不上的写法直接报错
		d, ok := hostname.NormalizePattern(raw)
		if !ok {
			return fmt.Errorf("域名 %q 格式错误", strings.TrimSpace(raw))
		}
		if d == "" || slices.Contains(domains, d) {
			continue
		}
		domains = append(domains, d)
	}
	s.Domains = domains
	// 断开连接与固定文本站点不产生错误页，界面也不显示错误页设置；从其他类型切换过来时清掉，
	// 以免看不见的自定义页面引用挡住页面删除。默认站点的设置还用于未匹配站点的 404，保留
	if !isDefault && (s.Type == TypeClose || s.Type == TypeText) {
		s.ErrorPage = ErrorPageOptions{}
	}
	if !isDefault && len(s.Domains) == 0 {
		return errors.New("至少填写一个域名")
	}
	switch s.Type {
	case "", TypeProxy:
		s.Type = TypeProxy
		s.Targets = cleanList(s.Targets)
		if len(s.Targets) == 0 {
			return errors.New("请填写后端地址")
		}
		for _, t := range s.Targets {
			if _, err := parseUpstream(t); err != nil {
				return err
			}
		}
	case TypeRedirect:
		s.RedirectTo = strings.TrimSpace(s.RedirectTo)
		// 变量换成示例值后再检查，例如 https://{1}.example.com:{STUN_web_PORT}{path}{args}
		sample, err := expandVars(s.RedirectTo, sampleVar)
		if err != nil {
			return err
		}
		u, err := url.Parse(sample)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return errors.New("重定向地址需为完整 URL")
		}
		switch s.RedirectCode {
		case 0:
			s.RedirectCode = 302
		case 301, 302, 307, 308:
		default:
			return errors.New("重定向状态码只能是 301/302/307/308")
		}
	case TypeStatic:
		if !filepath.IsAbs(s.Root) {
			return errors.New("静态文件目录需为绝对路径")
		}
		s.NotFoundPage = strings.TrimSpace(s.NotFoundPage)
		if strings.Contains(s.NotFoundPage, "..") {
			return errors.New("404 页面需为站点目录内的相对路径")
		}
	case TypePanel:
		// 引用的面板是否存在由 checkPanelRefsLocked 检查
		s.Panel = strings.TrimSpace(s.Panel)
		if s.Panel == "" {
			return errors.New("请选择导航面板")
		}
	case TypeText:
		if s.StatusCode == 0 {
			s.StatusCode = 200
		}
		if s.StatusCode < 100 || s.StatusCode > 599 {
			return errors.New("状态码无效")
		}
		if s.ContentType == "" {
			s.ContentType = "text/plain; charset=utf-8"
		}
	case TypeClose:
		// 断开连接的站点不经过认证、访问控制、重写与请求头等处理，其余设置都不生效，不再校验，
		// 以免从其他类型切换过来时被已不显示的设置拦住
		return nil
	default:
		return fmt.Errorf("未知的站点类型 %q", s.Type)
	}
	s.Users = cleanList(s.Users)
	s.OAuthUsers = cleanList(s.OAuthUsers)
	if s.BasicAuth && len(s.Users) == 0 && !s.SharedUsers {
		return errors.New("启用基本认证时至少添加一个用户")
	}
	if s.WebAuth && len(s.Users) == 0 && len(s.OAuthUsers) == 0 && !s.SharedUsers {
		return errors.New("启用网页认证时至少添加一个用户或第三方登录身份")
	}
	if err := thirdauth.ValidateEntries(s.OAuthUsers); err != nil {
		return err
	}
	if err := s.ForwardAuth.validate(); err != nil {
		return err
	}
	if s.ForwardAuth.URL != "" && (s.BasicAuth || s.WebAuth) {
		return errors.New("转发认证不能与基本认证、网页认证同时启用")
	}
	if err := s.validateLocations(); err != nil {
		return err
	}
	if err := s.validateUserRoutes(); err != nil {
		return err
	}
	for _, u := range s.Users {
		if name, pass, ok := strings.Cut(u, ":"); !ok || name == "" || pass == "" {
			return fmt.Errorf("用户格式应为 用户名:密码，实际 %q", u)
		}
	}
	switch s.IPMode {
	case IPModeOff, IPModeWhitelist, IPModeBlacklist:
	default:
		return fmt.Errorf("未知的访问控制模式 %q", s.IPMode)
	}
	if _, err := parsePrefixes(s.IPList); err != nil {
		return err
	}
	s.IPSets = uniqueList(s.IPSets)
	s.IPDenyList = cleanList(s.IPDenyList)
	if _, err := parsePrefixes(s.IPDenyList); err != nil {
		return err
	}
	s.IPDenySets = uniqueList(s.IPDenySets)
	if err := validateHeaderLines(append(append([]string{}, s.RequestHeaders...), s.ResponseHeaders...)); err != nil {
		return err
	}
	if err := s.ClientTimeouts.validate(); err != nil {
		return err
	}
	if err := s.validateProxyExtras(); err != nil {
		return err
	}
	if err := s.ErrorPage.validate(); err != nil {
		return err
	}
	return s.validateAccess()
}

// validateHeaderLines 校验 "Key: Value" 形式的请求头 / 响应头。
// 名称或值含非法字符时 Go 的 Transport 会拒绝整个请求（站点每次都返回 502），所以保存时就要拦下。
func validateHeaderLines(list []string) error {
	for _, h := range list {
		k, v, ok := strings.Cut(h, ":")
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if !ok || k == "" {
			return fmt.Errorf("请求头格式应为 Key: Value，实际 %q", h)
		}
		if !httpguts.ValidHeaderFieldName(k) {
			return fmt.Errorf("请求头名称 %q 含有非法字符", k)
		}
		if !httpguts.ValidHeaderFieldValue(v) {
			return fmt.Errorf("请求头 %s 的值含有非法字符", k)
		}
	}
	return nil
}

// matchDomain 判断 host 是否匹配规则。返回匹配优先级：精确 3、通配 2、全部 1，不匹配 0。
func matchDomain(rule, host string) int {
	switch {
	case rule == host:
		return 3
	case rule == "*":
		return 1
	}
	if _, ok := hostname.CapturePattern(rule, host); ok {
		return 2
	}
	return 0
}

// portsOverlap 判断两个监听的地址能否同时绑定同一端口号：tcp4 与 tcp6 互不冲突（tcp6 只监听 IPv6），
// 其余情况下监听地址相同、或任一方监听全部地址（留空、0.0.0.0、::）即冲突。
func portsOverlap(a, b *Listener) bool {
	if (a.Network == "tcp4" && b.Network == "tcp6") || (a.Network == "tcp6" && b.Network == "tcp4") {
		return false
	}
	wild := func(ip string) bool { return ip == "" || ip == "0.0.0.0" || ip == "::" }
	return a.ListenIP == b.ListenIP || wild(a.ListenIP) || wild(b.ListenIP)
}

// checkHTTPPortConflict 检查监听 l 的额外 HTTP 端口与其他已启用监听的端口（主端口或额外 HTTP 端口）是否冲突，
// 以及其他监听的额外 HTTP 端口是否占用了 l 的主端口。冲突时谁先启动谁占用端口，结果取决于启动顺序，因此保存时拒绝。
// 主端口之间的重复沿用原有行为，不在这里检查。l 未启用时不检查。
func checkHTTPPortConflict(l *Listener, others []Listener) error {
	if !l.Enable {
		return nil
	}
	for i := range others {
		o := &others[i]
		if o.Key == l.Key || !o.Enable || !portsOverlap(l, o) {
			continue
		}
		name := o.Name
		if name == "" {
			name = ":" + strconv.Itoa(o.Port)
		}
		if l.HTTPPort > 0 && (l.HTTPPort == o.Port || l.HTTPPort == o.HTTPPort) {
			return fmt.Errorf("HTTP 端口 %d 已被监听「%s」使用", l.HTTPPort, name)
		}
		if o.HTTPPort > 0 && o.HTTPPort == l.Port {
			return fmt.Errorf("端口 %d 已被监听「%s」用作 HTTP 端口", l.Port, name)
		}
	}
	return nil
}
