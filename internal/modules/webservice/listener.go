package webservice

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	proxyproto "github.com/pires/go-proxyproto"
	"github.com/quic-go/quic-go/http3"

	"github.com/wangwanjie/gately/internal/acmehttp"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/openports"
	"github.com/wangwanjie/gately/internal/visitors"
	"github.com/wangwanjie/gately/internal/webprobe"
)

// CertSource 提供按 SNI 选择证书的能力（由证书模块实现）。
type CertSource interface {
	GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error)
}

var tlsVersions = map[string]uint16{
	"1.0": tls.VersionTLS10, "1.1": tls.VersionTLS11, "1.2": tls.VersionTLS12, "1.3": tls.VersionTLS13,
}

// listenerRunner 运行一个监听器。
type listenerRunner struct {
	// cfg 为当前配置，热更新站点时整体替换（持有模块锁），处理请求的 goroutine 不能读取它；
	// 请求路径用到的设置放在下面创建后不再变化的字段中
	cfg Listener
	// key、plainHTTP、httpPortMode 在监听存续期间不变（相关设置变化时会重启监听，见 sameSocket）
	key, plainHTTP string
	// httpPortMode 为额外 HTTP 端口的处理方式；跳转所用的 HTTPS 端口随访问地址变化，放在站点表中热更新
	httpPortMode string
	// httpLn 为额外 HTTP 端口的监听（未开启或监听失败时为 nil），httpErr 为它监听失败的原因
	httpLn  net.Listener
	httpErr string
	auth    *authenticator
	log     *logbuf.Logger
	srv     *http.Server
	// ln 为交给 srv 的监听（含各层包装），停止时由 stop 同步关闭，见 stop 中的说明
	ln net.Listener
	// releases 撤销监听端口在 openports 中的登记（监听成功后立即视为开放端口，见 openports.Hold）
	releases []func()
	// h3 为 HTTP/3（QUIC）服务，仅在监听器开启 HTTP3 时存在；h3pc 为它的 UDP 监听
	h3   *http3.Server
	h3pc net.PacketConn
	// altSvc 为通告 HTTP/3 的响应头值，未开启时为空
	altSvc string
	// cancel 结束所有请求上下文，用于关闭已升级（WebSocket）的连接
	cancel context.CancelFunc
	// baseCtx 为全部请求与站点后台任务的父上下文，监听停止时取消
	baseCtx context.Context
	// table 为当前生效的站点表。修改站点时整体替换，监听端口与已建立的连接不受影响
	table   atomic.Pointer[siteTable]
	startAt time.Time
	err     string
	// realIP 为模块共享的可信代理真实 IP 设置，可为 nil（不启用）
	realIP *realIPSource
	// acts 为在监听层处理、不属于任何站点的请求的统计
	acts listenerActs
}

// listenerAct 是一种监听级处理的计数、秒级速率与近期访客地址。
type listenerAct struct {
	total  atomic.Int64
	rate   rateRing
	recent *recentIPs
}

// listenerActs 统计在监听层直接处理的请求：HTTPS 端口收到明文请求时跳转到 HTTPS，
// 以及没有站点匹配又没有启用默认站点时返回的 404。它们不经过任何站点，站点统计里看不到。
type listenerActs struct {
	redirect listenerAct
	notFound listenerAct
}

// hit 登记一次监听级处理。
func (act *listenerAct) hit(r *http.Request) {
	now := time.Now().Unix()
	act.total.Add(1)
	act.rate.add(now, 1, 0)
	act.recent.touch(clientAddr(r).Addr(), now)
}

// siteTable 是一个监听器在某一时刻的全部站点。
type siteTable struct {
	sites []*siteHandler
	deflt *siteHandler
	// exact 为精确域名到站点的索引，站点很多时避免逐个比较
	exact map[string]*siteHandler
	// wild 为含通配符的域名（按站点顺序），wildReady 为 true 时表示 exact 已包含全部精确域名，
	// 精确索引未命中后只需比对 wild；站点表由 buildSiteTable 构建时置位，其它构造方式回退到逐个比对全部域名
	wild      []wildDomain
	wildReady bool
	// cancel 停止这一批站点的后台任务（健康检查）
	cancel context.CancelFunc
	// missPage 为没有任何站点匹配时输出 404 所用的错误页设置，取自默认站点（默认站点停用时同样生效）
	missPage *errorPager
	// httpsPort 为额外 HTTP 端口跳转时使用的 HTTPS 端口（访问端口或监听端口），修改访问地址时随站点表热更新
	httpsPort int
}

// wildDomain 是一个含通配符的站点域名。
type wildDomain struct {
	h      *siteHandler
	domain string
}

// indexDomains 把站点的域名登记到精确索引与通配列表。同一域名出现在多个站点时以先出现的为准，与逐个比较时的结果一致。
func (t *siteTable) indexDomains(h *siteHandler) {
	for _, d := range h.site.Domains {
		if !strings.Contains(d, "*") {
			if _, dup := t.exact[d]; !dup {
				t.exact[d] = h
			}
		} else {
			t.wild = append(t.wild, wildDomain{h: h, domain: d})
		}
	}
}

// current 返回当前生效的站点表，监听器尚未构建时返回空表。
func (l *listenerRunner) current() *siteTable {
	if t := l.table.Load(); t != nil {
		return t
	}
	return &siteTable{}
}

// all 返回全部站点（含默认站点）。
func (t *siteTable) all() []*siteHandler {
	out := append([]*siteHandler{}, t.sites...)
	if t.deflt != nil {
		out = append(out, t.deflt)
	}
	return out
}

// route 按 Host 选择站点：精确匹配优先，其次最长通配，最后 *。
func (l *listenerRunner) route(host string) *siteHandler {
	host = hostOnly(host)
	t := l.current()
	// 精确匹配的优先级最高，命中索引即可直接返回
	if s, ok := t.exact[host]; ok {
		return s
	}
	var best *siteHandler
	bestScore, bestLen := 0, 0
	consider := func(s *siteHandler, d string) {
		score := matchDomain(d, host)
		if score == 0 {
			return
		}
		if score > bestScore || (score == bestScore && len(d) > bestLen) {
			best, bestScore, bestLen = s, score, len(d)
		}
	}
	if t.wildReady {
		// 精确域名都在索引里且已未命中，只有通配域名可能匹配；站点很多时不必逐个比对全部域名
		for _, w := range t.wild {
			consider(w.h, w.domain)
		}
	} else {
		for _, s := range t.sites {
			for _, d := range s.site.Domains {
				consider(s, d)
			}
		}
	}
	if best != nil {
		return best
	}
	return t.deflt
}

// redirectable 判断明文请求能否跳转到 HTTPS：域名没有对应站点（也没有默认站点），或默认站点是断开连接时不跳转，
// 交给后面的未知域名处理或默认站点断开，与 HTTPS 上的行为一致。否则扫描器用任意 Host 访问 HTTP 端口
// 都会得到一个跳转，既绕过了默认站点的拒绝与路径探测计数，也对外确认了 HTTPS 监听的存在。
func (l *listenerRunner) redirectable(r *http.Request) bool {
	s := l.route(r.Host)
	if s == nil {
		return false
	}
	return !(s.deflt && s.site.Type == TypeClose)
}

// maxHostLen 是 Host 请求头的最大长度：域名最长 253 个字符，加上端口与 IPv6 方括号留有余量。
const maxHostLen = 300

func (l *listenerRunner) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 过长的 Host 不可能是有效域名，却会被写进访问日志、错误页与跳转地址，请求头上限可达 1 MB
	if len(r.Host) > maxHostLen {
		http.Error(w, "400 Bad Request", http.StatusBadRequest)
		return
	}
	// 先确定访客真实地址（对端为可信代理时采信其请求头），后续所有处理都使用改写后的客户端地址
	r = l.realIP.apply(r)
	// 通过 TCP 访问时告知浏览器可改用 HTTP/3；HTTP/3 请求本身无需再通告
	if l.altSvc != "" && r.ProtoMajor < 3 && r.TLS != nil {
		w.Header().Set("Alt-Svc", l.altSvc)
	}
	// ACME HTTP-01 验证请求优先应答（仅在签发进行中登记过 token 时命中）
	if r.TLS == nil && acmehttp.Serve(w, r) {
		return
	}
	// HTTPS 端口收到的明文请求按设置跳转到同端口的 https 地址，额外 HTTP 端口的请求跳转到主端口（ACME 验证已在上面应答）；
	// 可信反向代理已终止 TLS 并以 X-Forwarded-Proto: https 转来的请求不再跳转，否则会循环跳转
	if fromHTTPPort(r) {
		if l.httpPortMode == PlainHTTPRedirect && !requestIsHTTPS(r) && l.redirectable(r) {
			l.acts.redirect.hit(r)
			redirectToHTTPSPort(w, r, l.current().httpsPort)
			return
		}
	} else if l.plainHTTP == PlainHTTPRedirect && !requestIsHTTPS(r) && l.redirectable(r) {
		l.acts.redirect.hit(r)
		redirectToHTTPS(w, r)
		return
	}
	// 第三方登录回调只在配置的认证域名上处理，与站点路由无关
	if r.URL.Path == oauthCallback && l.auth != nil {
		env := l.auth.oauthEnv()
		if u, err := url.Parse(env.callbackBase); err == nil && env.callbackBase != "" && hostOnly(u.Host) == hostOnly(r.Host) {
			l.auth.handleOAuthCallback(w, r, env)
			return
		}
	}
	// 内置 OIDC 身份提供方只在签发者域名的协议路径上处理，其余请求照常按站点路由
	if l.auth != nil {
		if e := l.auth.oidcTarget(r); e != nil {
			// 签发者域名配有站点时，先执行该站点的访问控制（IP 名单、国家过滤、限流），不能借协议路径绕过
			if s := l.route(r.Host); s != nil {
				release, ok := s.accessCheck(w, r)
				if !ok {
					return
				}
				defer release()
			}
			l.auth.serveOIDC(w, r, e, l.log)
			return
		}
	}
	s := l.route(r.Host)
	if s == nil {
		l.acts.notFound.hit(r)
		writeNotFound(l.current().missPage, w, r, "站点不存在", "这个域名没有配置对应的站点，请检查访问地址是否正确。")
		// 未知域名且没有默认站点时，同样交给路径探测规则计数
		webprobe.Report(webprobe.Hit{Addr: clientAddr(r).Addr(), Listener: l.key, Unknown: true,
			Method: r.Method, Host: r.Host, Path: r.URL.Path, Status: http.StatusNotFound})
		if l.auth != nil {
			l.auth.visits.Record(visitors.Hit{Addr: clientAddr(r).Addr(), Listener: l.key, Unmatched: true, Host: hostOnly(r.Host),
				Method: r.Method, Path: r.URL.Path, Status: http.StatusNotFound, UA: r.UserAgent()})
		}
		return
	}
	s.ServeHTTP(w, r)
}

func newListenerRunner(cfg Listener, log *logbuf.Logger, auth *authenticator, waf WAFProvider) (*listenerRunner, error) {
	l := &listenerRunner{cfg: cfg, key: cfg.Key, plainHTTP: cfg.PlainHTTP, httpPortMode: cfg.HTTPPortMode, log: log, auth: auth}
	l.acts.redirect.recent = newRecentIPs()
	l.acts.notFound.recent = newRecentIPs()
	t, err := buildSiteTable(cfg, log, auth, waf, nil)
	if err != nil {
		return nil, err
	}
	l.table.Store(t)
	return l, nil
}

// buildSiteTable 编译监听器的全部站点。prev 为正在运行的旧站点表，同 key 的站点沿用其统计数据，可为 nil。
func buildSiteTable(cfg Listener, log *logbuf.Logger, auth *authenticator, waf WAFProvider, prev *siteTable) (*siteTable, error) {
	old := map[string]*siteHandler{}
	if prev != nil {
		for _, h := range prev.all() {
			old[h.site.Key] = h
		}
	}
	t := &siteTable{exact: map[string]*siteHandler{}, httpsPort: cfg.httpsPort()}
	if auth != nil {
		t.missPage = newErrorPager(cfg.Default, auth.errPages)
	}
	build := func(s Site) (*siteHandler, error) {
		s.scope = cfg.Key + "/" + s.Key
		h, err := newSiteHandler(s, log, auth, waf)
		if err != nil {
			return nil, err
		}
		if o := old[s.Key]; o != nil {
			h.inherit(o)
		}
		h.listener = cfg.Key
		return h, nil
	}
	for _, s := range cfg.Sites {
		if !s.Enable {
			continue
		}
		h, err := build(s)
		if err != nil {
			return nil, err
		}
		// 默认站点处理未匹配任何站点域名的请求，来源域名不受控，不参与后台登录免认证的跳转握手（只有这里的站点设置）
		h.site.adminHS = h.site.AdminPass
		t.sites = append(t.sites, h)
		t.indexDomains(h)
	}
	t.wildReady = true
	if cfg.Default.Enable {
		h, err := build(cfg.Default)
		if err != nil {
			return nil, err
		}
		h.deflt = true
		t.deflt = h
	}
	return t, nil
}

// reportTLSFail 把一次 TLS 握手失败交给入侵防护。没有规则时只有一次原子读取的开销。
func (l *listenerRunner) reportTLSFail(addr, reason string) {
	if !webprobe.TLSEnabled() {
		return
	}
	ap, err := netip.ParseAddrPort(addr)
	if err != nil {
		return
	}
	// 对端是可信代理（CDN / WAF 回源节点）时，握手阶段拿不到访客的真实地址，计数只会落到代理身上。
	// 别人可以把自己的 CDN 指向本机并制造握手失败，借此封掉代理、让经它访问的站点全部中断，所以不上报
	if l.realIP.trustsPeer(ap.Addr().Unmap()) {
		return
	}
	kind := webprobe.ClassifyTLS(reason)
	if _, ok := parseNoCert(reason); ok {
		// 站点明确填写了该域名却没有证书是配置问题，不能算访客的错
		kind = webprobe.TLSOther
		if l.isUnservedCertError(reason) {
			kind = webprobe.TLSNoCert
		}
	}
	webprobe.ReportTLS(webprobe.TLSFail{Addr: ap.Addr().Unmap(), Listener: l.key, Kind: kind, Reason: reason})
}

// noCertFormat 是证书模块缺少证书的错误格式，见 internal/modules/ssl/module.go 的 GetCertificate。
// 直接复用它的格式串，不会引入新的待翻译片段。
const noCertFormat = "没有匹配 %q 的证书"

// parseNoCert 解析缺少证书的错误句并返回请求的域名；按同一格式拼回比对，确保整句完全一致（不接受前后多出内容）。
func parseNoCert(reason string) (name string, ok bool) {
	if _, err := fmt.Sscanf(reason, noCertFormat, &name); err != nil || fmt.Sprintf(noCertFormat, name) != reason {
		return "", false
	}
	return name, true
}

// isUnservedCertError 判断握手失败原因是否为访客请求了本监听器不提供的域名而缺少证书。
// 证书模块的错误格式为「没有匹配 %q 的证书」（见 internal/modules/ssl/module.go 的 GetCertificate）。
// 不带 SNI、用 IP 或别人的域名握手多为公网扫描，与配置无关；站点中填写了该域名却没有证书时仍是配置问题，返回 false。
func (l *listenerRunner) isUnservedCertError(reason string) bool {
	name, ok := parseNoCert(reason)
	if !ok {
		return false
	}
	return !l.current().servesSNI(name)
}

// servesSNI 判断 TLS 握手中的 SNI 是否属于某个已启用站点明确填写的域名（精确、*.example.com 或 api-*.example.com 通配）。
// 默认站点与填写 * 的站点接收任意域名，但不能为任意域名准备证书，因此不算。
func (t *siteTable) servesSNI(name string) bool {
	name = strings.TrimSuffix(strings.ToLower(name), ".")
	if name == "" {
		// 不带 SNI（按 IP 访问）：只有站点域名中填写了 IP 地址时，缺少 IP 证书才是配置问题
		for _, s := range t.sites {
			for _, d := range s.site.Domains {
				if _, err := netip.ParseAddr(strings.Trim(d, "[]")); err == nil {
					return true
				}
			}
		}
		return false
	}
	if _, ok := t.exact[name]; ok {
		return true
	}
	for _, s := range t.sites {
		for _, d := range s.site.Domains {
			if d != "*" && matchDomain(d, name) > 0 {
				return true
			}
		}
	}
	return false
}

// startTable 启动站点表的后台任务。
func startTable(t *siteTable, parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	t.cancel = cancel
	for _, s := range t.all() {
		s.startBackground(ctx)
	}
}

// retire 停止旧站点表的后台任务，并关闭其空闲的后端连接；正在进行的请求不受影响。
func (t *siteTable) retire() {
	if t.cancel != nil {
		t.cancel()
	}
	for _, s := range t.all() {
		s.closeIdle()
	}
}

// sameSocket 判断两份监听配置的端口相关设置是否相同，相同时修改站点无需重启监听。
func sameSocket(a, b Listener) bool {
	return a.Network == b.Network && a.ListenIP == b.ListenIP && a.Port == b.Port && a.TLS == b.TLS &&
		a.PlainHTTP == b.PlainHTTP && a.MinTLS == b.MinTLS && a.HTTP3 == b.HTTP3 && a.AcceptProxyProtocol == b.AcceptProxyProtocol &&
		a.HTTPPort == b.HTTPPort && a.HTTPPortMode == b.HTTPPortMode
}

// reload 在不关闭监听的情况下替换站点表：新请求立即使用新配置，已建立的连接（含 WebSocket）保持不断。
func (l *listenerRunner) reload(cfg Listener, waf WAFProvider) error {
	prev := l.current()
	t, err := buildSiteTable(cfg, l.log, l.auth, waf, prev)
	if err != nil {
		return err
	}
	startTable(t, l.baseCtx)
	l.cfg = cfg
	l.table.Store(t)
	prev.retire()
	l.log.Infof("站点配置已更新（%d 个站点），监听未中断", len(t.sites))
	return nil
}

// start 开始监听。trusted 为可信的 PROXY 协议来源。
func (l *listenerRunner) start(certs CertSource, trusted []string) (err error) {
	addr := net.JoinHostPort(l.cfg.ListenIP, strconv.Itoa(l.cfg.Port))
	ln, err := net.Listen(l.cfg.Network, addr)
	if err != nil {
		return err
	}
	if ln, err = l.wrapProxyProtocol(ln, trusted); err != nil {
		return err
	}
	// 监听成功后立即登记为开放端口，启动失败时撤销；停止时由 stop 撤销
	l.releases = append(l.releases, openports.Hold(ln.Addr()))
	defer func() {
		if err != nil {
			l.releaseHolds()
		}
	}()
	baseCtx, cancel := context.WithCancel(context.Background())
	l.cancel = cancel
	l.baseCtx = baseCtx
	// 额外 HTTP 端口的连接在上下文中做标记，处理请求时据此决定是否跳转到 HTTPS（见 fromHTTPPort）；
	// 标记上下文仍派生自 baseCtx，监听停止时同样被取消
	httpPortCtx := context.WithValue(baseCtx, ctxKeyHTTPPort{}, true)
	var httpLn net.Listener
	l.srv = &http.Server{
		Handler: l,
		BaseContext: func(ln net.Listener) context.Context {
			if httpLn != nil && ln == httpLn {
				return httpPortCtx
			}
			return baseCtx
		},
		// 记下连接本身，处理请求时据此判断它是否经可信 PROXY 来源进入（见 viaTrustedProxy）
		ConnContext:       func(ctx context.Context, c net.Conn) context.Context { return context.WithValue(ctx, ctxKeyConn{}, c) },
		ReadHeaderTimeout: 30 * time.Second,
		IdleTimeout:       120 * time.Second,
		// 默认的 HTTP/2 接收窗口为 1 MB，高延迟线路上会限制单连接的上传速度，这里放大到与 nginx 默认缓冲相当的水平。
		// 窗口只是上限，只有后端读得比客户端发得慢时才会真正占用内存
		HTTP2: &http.HTTP2Config{
			MaxReceiveBufferPerConnection: 8 << 20,
			MaxReceiveBufferPerStream:     4 << 20,
		},
		// net/http 内部错误（TLS 握手失败、缺少证书等）写入本监听器的运行日志；
		// 扫描器用不属于任何站点的域名握手导致的缺少证书只记调试级别，见 isUnservedCertError
		// 每次握手失败另交给入侵防护按原因类别计数封禁
		ErrorLog: l.log.ServerErrorLogHook(l.isUnservedCertError, l.reportTLSFail),
	}
	if l.cfg.TLS {
		if certs == nil {
			ln.Close()
			return errors.New("证书模块不可用")
		}
		min := tlsVersions[l.cfg.MinTLS]
		if min == 0 {
			min = tls.VersionTLS12
		}
		l.srv.TLSConfig = &tls.Config{GetCertificate: certs.GetCertificate, MinVersion: min, NextProtos: []string{"h2", "http/1.1"}}
		if l.cfg.PlainHTTP != "" {
			// 同一端口同时接受 HTTPS 与明文 HTTP，按连接的第一个字节区分
			ln = newSniffListener(ln, l.srv.TLSConfig)
		} else {
			ln = tls.NewListener(ln, l.srv.TLSConfig)
		}
		if l.cfg.HTTP3 {
			if err := l.startHTTP3(addr); err != nil {
				// HTTP/3 失败不影响 HTTP/1.1 与 HTTP/2 服务
				l.log.Errorf("HTTP/3 监听 %s 失败: %v", addr, err)
			}
		}
	}
	// 额外 HTTP 端口与主端口共用 srv 与站点表；它监听失败不影响主端口，原因记在 httpErr 中显示在监听状态里
	if l.cfg.HTTPPort > 0 {
		httpAddr := net.JoinHostPort(l.cfg.ListenIP, strconv.Itoa(l.cfg.HTTPPort))
		hl, err := net.Listen(l.cfg.Network, httpAddr)
		if err == nil {
			hl, err = l.wrapProxyProtocol(hl, trusted)
		}
		if err != nil {
			l.httpErr = fmt.Sprintf("HTTP 端口 %d 监听失败: %v", l.cfg.HTTPPort, err)
			l.log.Errorf("%s", l.httpErr)
		} else {
			httpLn = hl
			l.httpLn = hl
			l.releases = append(l.releases, openports.Hold(hl.Addr()))
		}
	}
	l.startAt = time.Now()
	startTable(l.current(), baseCtx)
	l.ln = ln
	srv := l.srv
	go func() {
		// 使用本次启动的 srv：goroutine 可能在下一次 start 替换 l.srv 之后才运行
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			l.log.Errorf("监听 %s 退出: %v", addr, err)
		}
	}()
	if httpLn != nil {
		go func() {
			if err := srv.Serve(httpLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
				l.log.Errorf("HTTP 端口 %d 退出: %v", l.cfg.HTTPPort, err)
			}
		}()
	}
	scheme := "http"
	if l.cfg.TLS {
		scheme = "https"
	}
	l.log.Infof("%s 监听 %s（%d 个站点）", scheme, addr, len(l.current().sites))
	switch l.cfg.PlainHTTP {
	case PlainHTTPRedirect:
		l.log.Infof("同一端口收到的明文 HTTP 请求跳转到 HTTPS")
	case PlainHTTPServe:
		l.log.Infof("同一端口同时以明文 HTTP 提供站点")
	}
	if httpLn != nil {
		if l.httpPortMode == PlainHTTPRedirect {
			l.log.Infof("HTTP 端口 %d 的请求跳转到 HTTPS（端口 %d）", l.cfg.HTTPPort, l.cfg.httpsPort())
		} else {
			l.log.Infof("HTTP 端口 %d 以明文 HTTP 提供同一组站点", l.cfg.HTTPPort)
		}
	}
	return nil
}

// wrapProxyProtocol 按设置为监听加上 PROXY 协议解析；出错时关闭 ln。
func (l *listenerRunner) wrapProxyProtocol(ln net.Listener, trusted []string) (net.Listener, error) {
	if !l.cfg.AcceptProxyProtocol {
		return ln, nil
	}
	if len(trusted) == 0 {
		ln.Close()
		return nil, errors.New("已开启接收 PROXY 协议，但未配置可信来源")
	}
	// 只有可信来源发来的 PROXY 头才会被采用，其他来源的头被忽略
	policy, err := proxyproto.ConnLaxWhiteListPolicy(trusted)
	if err != nil {
		ln.Close()
		return nil, err
	}
	return &proxyproto.Listener{Listener: ln, ConnPolicy: policy, ReadHeaderTimeout: 5 * time.Second}, nil
}

// ctxKeyHTTPPort 标记请求来自额外 HTTP 端口。
type ctxKeyHTTPPort struct{}

// fromHTTPPort 判断请求是否经额外 HTTP 端口进入。
func fromHTTPPort(r *http.Request) bool {
	v, _ := r.Context().Value(ctxKeyHTTPPort{}).(bool)
	return v
}

// redirectToHTTPSPort 把额外 HTTP 端口收到的请求跳转到主端口的 https 地址：保留访客请求的主机名，端口换成 HTTPS 端口
// （443 时省略）。GET/HEAD 用 301，其余方法用 308 保留请求方法与请求体。
func redirectToHTTPSPort(w http.ResponseWriter, r *http.Request, port int) {
	host := hostOnly(r.Host)
	if host == "" {
		http.Error(w, "缺少 Host 请求头，无法跳转到 HTTPS", http.StatusBadRequest)
		return
	}
	if port != 443 {
		host = net.JoinHostPort(host, strconv.Itoa(port))
	} else if strings.Contains(host, ":") {
		// IPv6 地址作为主机名时要加方括号
		host = "[" + host + "]"
	}
	code := http.StatusMovedPermanently
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		code = http.StatusPermanentRedirect
	}
	http.Redirect(w, r, "https://"+host+r.URL.RequestURI(), code)
}

// redirectToHTTPS 把明文请求跳转到同主机同端口的 https 地址。Host 原样保留（含客户端访问时的端口），
// 经过端口映射访问时跳转后的端口与客户端看到的一致。GET/HEAD 用 301，其余方法用 308 保留请求方法与请求体。
func redirectToHTTPS(w http.ResponseWriter, r *http.Request) {
	if r.Host == "" {
		http.Error(w, "缺少 Host 请求头，无法跳转到 HTTPS", http.StatusBadRequest)
		return
	}
	code := http.StatusMovedPermanently
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		code = http.StatusPermanentRedirect
	}
	http.Redirect(w, r, "https://"+r.Host+r.URL.RequestURI(), code)
}

// startHTTP3 在与 TCP 相同的地址与端口号上启动 UDP（QUIC）监听。
func (l *listenerRunner) startHTTP3(addr string) error {
	network := map[string]string{"tcp": "udp", "tcp4": "udp4", "tcp6": "udp6"}[l.cfg.Network]
	pc, err := net.ListenPacket(network, addr)
	if err != nil {
		return err
	}
	l.h3pc = pc
	l.h3 = &http3.Server{
		Handler:     l,
		TLSConfig:   http3.ConfigureTLSConfig(l.srv.TLSConfig),
		IdleTimeout: 120 * time.Second,
	}
	l.altSvc = fmt.Sprintf(`h3=":%d"; ma=86400`, l.cfg.Port)
	go func() {
		if err := l.h3.Serve(pc); err != nil && !errors.Is(err, http.ErrServerClosed) {
			l.log.Errorf("HTTP/3 监听 %s 退出: %v", addr, err)
		}
	}()
	l.log.Infof("HTTP/3 监听 %s（UDP）", addr)
	return nil
}

func (l *listenerRunner) stop() {
	if l.srv == nil {
		return
	}
	defer l.releaseHolds()
	if l.h3 != nil {
		_ = l.h3.Close()
		_ = l.h3pc.Close()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := l.srv.Shutdown(ctx); err != nil {
		_ = l.srv.Close()
	}
	// Serve 在后台 goroutine 中启动：刚启动就停止时它可能还没登记监听，Shutdown 关不到，
	// 监听要等那个 goroutine 退出时才关闭，紧接着在同一端口重新监听会报端口已被占用。这里同步关闭
	if l.ln != nil {
		_ = l.ln.Close()
		l.ln = nil
	}
	if l.httpLn != nil {
		_ = l.httpLn.Close()
		l.httpLn = nil
	}
	// Shutdown 不管理已劫持的连接；取消请求上下文后反向代理会关闭升级后的连接
	l.cancel()
	l.current().retire()
}

// ctxKeyConn 在请求上下文中保存接入的连接（可能经过 TLS 与协议嗅探的包装）。
type ctxKeyConn struct{}

// viaTrustedProxy 判断请求所在的连接是否带着可信来源发来的 PROXY 头：只有可信来源的头会被采用，
// 采用后 ProxyHeader 才非空。调用时连接的 PROXY 头已在读取请求前处理完，不会阻塞；
// 不能放在 ConnContext 里判断，那里还没读头，慢客户端会卡住 accept 循环。
func viaTrustedProxy(r *http.Request) bool {
	c, _ := r.Context().Value(ctxKeyConn{}).(net.Conn)
	for c != nil {
		switch v := c.(type) {
		case *proxyproto.Conn:
			return v.ProxyHeader() != nil
		case *tls.Conn:
			c = v.NetConn()
		case *prefixConn:
			c = v.Conn
		default:
			return false
		}
	}
	return false
}

// releaseHolds 撤销本监听登记的开放端口。
func (l *listenerRunner) releaseHolds() {
	for _, release := range l.releases {
		release()
	}
	l.releases = nil
}
