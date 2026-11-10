package webservice

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	proxyproto "github.com/pires/go-proxyproto"

	"github.com/wangwanjie/gately/internal/geoip"
	"github.com/wangwanjie/gately/internal/httpcompress"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/iplist"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/webpage"
	"github.com/wangwanjie/gately/internal/webprobe"
)

// 访问被拦截的原因，按原因分别计数。
const (
	// blockBan 为来源地址已被封禁（认证失败自动封禁、入侵防护、手动封禁）
	blockBan = iota
	// blockIP 为 IP 名单（含路径规则的名单与引用的 IP 列表）拒绝
	blockIP
	// blockCountry 为国家过滤拒绝
	blockCountry
	// blockRateLimit 为限流拒绝（429）
	blockRateLimit
	// blockWAF 为 WAF 拦截
	blockWAF
	blockKinds
)

// blockNames 为各拦截原因在接口与 Prometheus 指标中的名称。
var blockNames = [blockKinds]string{"ban", "ip", "country", "rateLimit", "waf"}

// SiteStats 是站点的请求统计。
type SiteStats struct {
	Requests atomic.Int64
	Active   atomic.Int64
	Errors   atomic.Int64
	// Blocked 为按原因分的拦截次数
	Blocked [blockKinds]atomic.Int64
}

// blocked 返回按原因分的拦截次数。
func (s *SiteStats) blocked() map[string]int64 {
	out := make(map[string]int64, blockKinds)
	for i := range s.Blocked {
		out[blockNames[i]] = s.Blocked[i].Load()
	}
	return out
}

// WAFProvider 提供按实例检查请求的中间件（由 WAF 模块实现）。
type WAFProvider interface {
	Middleware(key string, next http.Handler) http.Handler
}

// siteHandler 是一个站点编译后的处理器。
type siteHandler struct {
	site    Site
	ipList  []netip.Prefix
	stats   *SiteStats
	log     *logbuf.Logger
	auth    *authenticator
	handler http.Handler
	// pipeline 为 WAF（可选）→ 跨域 → 认证 → 站点处理器
	pipeline http.Handler
	pool     *pool
	// locations 为按路径前缀分流的子后端（最长前缀在前）
	locations []*locationHandler
	// userRoutes 为按认证用户分流的后端，顺序与配置一致
	userRoutes []*userRouteHandler
	// fwd 为外部转发认证，未启用时为 nil
	fwd *forwardAuther
	// idleClosers 关闭各反向代理的空闲后端连接，站点被替换时调用
	idleClosers []func()
	limiter     *limiter
	cache       *respCache
	rewrites    []compiledRewrite
	metrics     *siteMetrics
	// rules 为预编译的路径规则（最长前缀在前），countries 为国家过滤集合
	rules     []compiledRule
	countries map[string]bool
	// listener 为所属监听的 key，deflt 表示这是监听的默认站点（处理未匹配任何站点域名的请求），供路径探测上报
	listener string
	deflt    bool
	// conns 记录进行中的请求与最近结束的长请求，供实时连接列表与断开使用
	conns *connTracker
	// errPage 为站点的错误页设置（样式与拦截后端错误），nil 表示默认样式且不拦截
	errPage *errorPager
}

type ctxKeyClientAddr struct{}

// clientAddr 返回请求的客户端地址（PROXY 协议处理后的真实地址）。
func clientAddr(r *http.Request) netip.AddrPort {
	ap, _ := netip.ParseAddrPort(r.RemoteAddr)
	return netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port())
}

// reqLog 返回附带本次请求结构化字段（客户端 IP、域名、站点、方法、路径）的写入器，日志页可按这些字段过滤。
func (h *siteHandler) reqLog(r *http.Request) *logbuf.Logger {
	// 地址无效（例如经 unix socket 进入）时 String() 会得到 "invalid IP"，留空让 With 忽略该字段
	ip := ""
	if a := clientAddr(r).Addr(); a.IsValid() {
		ip = a.String()
	}
	return h.log.With(logbuf.Fields{
		logbuf.FieldIP:     ip,
		logbuf.FieldHost:   hostOnly(r.Host),
		logbuf.FieldSite:   h.site.Name,
		logbuf.FieldMethod: r.Method,
		logbuf.FieldPath:   r.URL.Path,
	})
}

func newSiteHandler(s Site, log *logbuf.Logger, auth *authenticator, waf WAFProvider) (*siteHandler, error) {
	h := &siteHandler{site: s, stats: &SiteStats{}, log: log, auth: auth, metrics: newSiteMetrics(), conns: newConnTracker()}
	// 断开连接的站点在 ServeHTTP 中直接断开，其余设置都不生效（保存时也不校验），不再编译重写、名单等规则；
	// 保留一个只会断开的处理器，以防其他调用路径意外写出响应
	if s.Type == TypeClose {
		h.handler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic(http.ErrAbortHandler)
		})
		h.pipeline = h.handler
		return h, nil
	}
	if auth != nil {
		h.errPage = newErrorPager(s, auth.errPages)
	}
	var err error
	if h.rewrites, err = compileRewrites(s.Rewrites); err != nil {
		return nil, err
	}
	if s.RateLimit.Enable {
		h.limiter = newLimiter(s.RateLimit)
	}
	if h.ipList, err = parsePrefixes(s.IPList); err != nil {
		return nil, err
	}
	if h.rules, err = compileRules(s.Rules); err != nil {
		return nil, err
	}
	h.countries = map[string]bool{}
	for _, c := range s.Countries {
		h.countries[strings.ToUpper(c)] = true
	}
	if s.ForwardAuth.URL != "" {
		h.fwd = newForwardAuther(s.ForwardAuth)
	}
	switch s.Type {
	case TypeProxy:
		if h.pool, err = newPool(s, log); err != nil {
			return nil, err
		}
		h.handler = h.newProxy(s, h.pool)
		if len(s.Locations) > 0 {
			if err := h.buildLocations(); err != nil {
				return nil, err
			}
			h.handler = h.dispatchLocations(h.locations, h.handler)
		}
		if len(s.UserRoutes) > 0 {
			if err := h.buildUserRoutes(); err != nil {
				return nil, err
			}
			h.handler = h.dispatchUserRoutes(h.handler)
		}
		if s.Cache.Enable {
			h.cache = newRespCache(s.Cache)
			h.handler = h.cache.wrap(h.handler)
		}
	case TypeRedirect:
		h.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 目标中的 {host}、{path}、{1}、{STUN_规则名_PORT} 等变量按本次请求替换，见 redirectvars.go
			to, err := redirectTarget(&s, r)
			if errors.Is(err, errBadHost) {
				writeErrorPage(h.errPage, w, r, http.StatusBadRequest, "请求无效", "请求的域名不合法")
				return
			}
			if err != nil {
				// 引用的 STUN 规则暂无公网地址，或变量展开后缺少主机，跳到半成品地址没有意义。
				// 原因（含规则名）只写日志，不展示给访客
				h.reqLog(r).Warnf("重定向目标暂不可用: %v", err)
				writeErrorPage(h.errPage, w, r, http.StatusServiceUnavailable, "重定向目标暂不可用", "跳转地址暂时无法确定，请稍后重试。")
				return
			}
			applyHeaders(w.Header(), s.ResponseHeaders)
			http.Redirect(w, r, to, s.RedirectCode)
		})
	case TypeStatic:
		h.handler = h.newStatic(s)
	case TypeText:
		h.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			applyHeaders(w.Header(), s.ResponseHeaders)
			w.Header().Set("Content-Type", s.ContentType)
			w.WriteHeader(s.StatusCode)
			_, _ = w.Write([]byte(s.Text))
		})
	}
	if s.Compress && s.Type != TypeRedirect {
		h.handler = httpcompress.Handler(h.handler)
	}
	h.pipeline = http.HandlerFunc(h.serveInner)
	// WAF 放在认证之前，登录页同样受保护
	if s.WAF != "" && waf != nil {
		h.pipeline = waf.Middleware(s.WAF, h.pipeline)
	}
	return h, nil
}

type ctxKeyUpstream struct{}

// upstreamBody 统计后端响应体的字节数。
type upstreamBody struct {
	io.ReadCloser
	st *upstreamStats
}

func (b *upstreamBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.st.bytes.Add(int64(n))
		b.st.rate.add(time.Now().Unix(), 0, int64(n))
	}
	return n, err
}

// pools 返回站点的全部后端组：站点后端、自带后端的分路径规则、按用户分流规则，顺序固定，同一组只出现一次。
func (h *siteHandler) pools() []*pool {
	var out []*pool
	if h.pool != nil {
		out = append(out, h.pool)
	}
	for _, l := range h.locations {
		if l.pool != nil {
			out = append(out, l.pool)
		}
	}
	for _, u := range h.userRoutes {
		out = append(out, u.pool)
	}
	return out
}

// upstreamStatus 返回站点全部后端组的状态（含分路径与按用户分流的后端），非代理站点为空。
func (h *siteHandler) upstreamStatus() []UpstreamStatus {
	var out []UpstreamStatus
	for _, p := range h.pools() {
		out = append(out, p.status()...)
	}
	return out
}

// startBackground 启动站点的后台任务（主动健康检查）。
func (h *siteHandler) startBackground(ctx context.Context) {
	if h.pool != nil {
		h.pool.startChecks(ctx)
	}
	for _, l := range h.locations {
		if l.pool != nil {
			l.pool.startChecks(ctx)
		}
	}
	for _, u := range h.userRoutes {
		u.pool.startChecks(ctx)
	}
}

// closeIdle 关闭站点各反向代理的空闲后端连接。
func (h *siteHandler) closeIdle() {
	for _, f := range h.idleClosers {
		f()
	}
}

// inherit 在热更新站点时沿用旧处理器的统计数据，避免修改配置后计数与分钟曲线清零。
func (h *siteHandler) inherit(old *siteHandler) {
	h.stats = old.stats
	h.metrics = old.metrics
	// 进行中的请求仍由旧处理器服务，沿用登记表才能继续列出并断开它们
	h.conns = old.conns
	// 同一分流下地址相同的后端沿用转发统计，修改其他设置后累计数与速率不清零。
	// 同一组里同一地址写了多次（如一条普通、一条备用）时按出现顺序一一对应，不合并成一份统计
	key := func(p *pool, u *upstream, seen map[string]int) string {
		k := p.scope + "\x00" + p.match + "\x00" + u.url.String()
		seen[k]++
		return k + "\x00" + strconv.Itoa(seen[k])
	}
	prev := map[string]*upstreamStats{}
	seen := map[string]int{}
	for _, p := range old.pools() {
		for _, u := range p.ups {
			prev[key(p, u, seen)] = u.stats
		}
	}
	seen = map[string]int{}
	for _, p := range h.pools() {
		for _, u := range p.ups {
			if st := prev[key(p, u, seen)]; st != nil {
				u.stats = st
			}
		}
	}
}

// proxyBufferPool 复用反向代理的拷贝缓冲区，避免每个请求分配 32 KB 带来的 GC 压力。
type proxyBufferPool struct{ p sync.Pool }

func (b *proxyBufferPool) Get() []byte {
	if v, ok := b.p.Get().(*[]byte); ok {
		return *v
	}
	return make([]byte, 32*1024)
}

func (b *proxyBufferPool) Put(buf []byte) {
	if cap(buf) < 32*1024 {
		return
	}
	buf = buf[:32*1024]
	b.p.Put(&buf)
}

var sharedBufferPool = &proxyBufferPool{}

// sharedSessionCache 缓存到 HTTPS 后端的 TLS 会话，新连接可以复用会话，省去完整握手。
var sharedSessionCache = tls.NewLRUClientSessionCache(512)

// newTransport 为一个后端创建连接池。每个后端独立一个 Transport：
// 开启“使用访问域名作为 SNI”时请求 URL 的主机名会换成访问域名，独立的连接池保证连接不会被串用到别的后端。
func newTransport(s Site, up *upstream) *http.Transport {
	dialer := &net.Dialer{Timeout: s.Timeouts.connect(), KeepAlive: 30 * time.Second}
	// 实际拨号地址始终是后端配置的地址，与请求 URL 中的主机名无关
	target := up.url.Host
	if up.url.Port() == "" {
		port := "80"
		if up.url.Scheme == "https" {
			port = "443"
		}
		target = net.JoinHostPort(up.url.Hostname(), port)
	}
	return &http.Transport{
		Proxy:             nil,
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: s.InsecureSkipVerify, ClientSessionCache: sharedSessionCache},
		ForceAttemptHTTP2: !s.SendProxyProtocol,
		// 本机后端的并发可能很高，空闲连接上限放宽到与 nginx keepalive 常用值相当，减少突发时的重复建连
		MaxIdleConnsPerHost: 256,
		IdleConnTimeout:     90 * time.Second,
		// TLS 握手单独计时（拨号超时不含握手），避免卡在握手的后端无限占用连接
		TLSHandshakeTimeout: s.Timeouts.tlsHandshake(),
		// 0 表示不限；只限制请求发完到收到响应头之间的等待
		ResponseHeaderTimeout: s.Timeouts.responseHeader(),
		// 发送 PROXY 头时每个连接只对应一个客户端，必须禁用连接复用
		DisableKeepAlives: s.DisableKeepAlives || s.SendProxyProtocol,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			c, err := dialer.DialContext(ctx, network, target)
			if err != nil || !s.SendProxyProtocol {
				return c, err
			}
			src, _ := ctx.Value(ctxKeyClientAddr{}).(netip.AddrPort)
			if !src.IsValid() {
				return c, nil
			}
			header := proxyproto.HeaderProxyFromAddrs(2, net.TCPAddrFromAddrPort(src), c.LocalAddr())
			if _, err := header.WriteTo(c); err != nil {
				c.Close()
				return nil, err
			}
			return c, nil
		},
	}
}

// upstreamRoundTripper 按请求选中的后端使用对应的连接池；与后端建连失败时换下一个后端重试（见 proxyAttempt）。
type upstreamRoundTripper struct {
	trs  map[*upstream]*http.Transport
	pool *pool
	// target 把请求改写为发往指定后端（URL、Host 与 SNI），重试换后端时使用
	target func(pr *httputil.ProxyRequest, up *upstream)
	log    func(r *http.Request) *logbuf.Logger
}

func (rt *upstreamRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	att := attemptFrom(req.Context())
	if att == nil {
		return nil, errors.New("缺少后端选择")
	}
	out := req
	var body *retryBody
	if req.Body != nil && req.Body != http.NoBody && len(rt.pool.ups) > 1 {
		// 失败时 Transport 会关闭请求体，重试需要它保持可读；读过的请求体不能重发，此时不再重试。
		// 访客请求体最终由 net/http 在请求结束时关闭
		body = &retryBody{ReadCloser: req.Body}
		out = shallowRequest(req)
		out.Body = body
	}
	for {
		up := att.current()
		resp, err := rt.trs[up].RoundTrip(out)
		if err == nil || !dialFailed(err) || req.Context().Err() != nil || (body != nil && body.read.Load()) {
			return resp, err
		}
		next := att.next()
		if next == nil {
			return resp, err
		}
		rt.pool.fail(up, err)
		rt.log(att.in).Warnf("%s %s%s 连接后端 %s 失败（%v），改用 %s", req.Method, req.Host, req.URL.Path, up.url.Host, err, next.url.Host)
		att.use(next)
		retry := shallowRequest(out)
		pr := &httputil.ProxyRequest{In: att.in, Out: retry}
		// 从访客请求的地址重新拼接，避免叠加上一个后端的路径前缀
		u := *att.in.URL
		retry.URL = &u
		rt.target(pr, next)
		out = retry
	}
}

// newProxy 创建反向代理处理器，s 为生效的站点配置（分路径规则会传入合并后的配置），p 为其后端集合。
func (h *siteHandler) newProxy(s Site, p *pool) http.Handler {
	// target 把发往后端的请求指向 up：URL 改为后端地址，按设置保留访客的 Host，按设置用访问域名作 SNI
	target := func(pr *httputil.ProxyRequest, up *upstream) {
		pr.SetURL(up.url)
		if !s.UseTargetHost {
			// SetURL 会改写 Host，这里恢复客户端请求的 Host
			pr.Out.Host = pr.In.Host
		}
		if s.UpstreamSNIFromHost && up.url.Scheme == "https" {
			// 把 URL 主机名换成访问域名，TLS 握手的 SNI 与证书校验都使用它；拨号地址仍是后端地址（见 newTransport）
			if host := hostOnly(pr.In.Host); host != "" && net.ParseIP(host) == nil {
				port := up.url.Port()
				if port == "" {
					port = "443"
				}
				pr.Out.URL.Host = net.JoinHostPort(host, port)
			}
		}
	}
	rt := &upstreamRoundTripper{trs: map[*upstream]*http.Transport{}, pool: p, target: target, log: h.reqLog}
	for _, up := range p.ups {
		tr := newTransport(s, up)
		rt.trs[up] = tr
		h.idleClosers = append(h.idleClosers, tr.CloseIdleConnections)
	}
	rp := &httputil.ReverseProxy{
		Transport:  rt,
		BufferPool: sharedBufferPool,
		Rewrite: func(pr *httputil.ProxyRequest) {
			target(pr, attemptFrom(pr.In.Context()).current())
			// 客户端自带的 X-Real-IP 不可信，只在开启转发头时由 Gately 写入
			pr.Out.Header.Del("X-Real-Ip")
			if s.ForwardedHeaders {
				pr.SetXForwarded()
				// SetXForwarded 只看本连接是否为 TLS；访客经可信反向代理以 HTTPS 访问时，向后端如实写 https
				if pr.In.TLS == nil && requestIsHTTPS(pr.In) {
					pr.Out.Header.Set("X-Forwarded-Proto", "https")
				}
				pr.Out.Header.Set("X-Real-Ip", clientAddr(pr.In).Addr().String())
				if port := localPort(pr.In); port != "" {
					pr.Out.Header.Set("X-Forwarded-Port", port)
				}
			}
			// 站点会话 Cookie 只用于 Gately 自身认证，不转发给后端；后端是负责认证的另一台 Gately 时按站点设置原样转发
			if !s.PassAuthCookies {
				stripCookie(pr.Out, cookieName(s), oauthNonceCookie, ssoCookie)
			}
			applyHeaders(pr.Out.Header, s.RequestHeaders)
		},
		ModifyResponse: func(resp *http.Response) error {
			applyHeaders(resp.Header, s.ResponseHeaders)
			if att := attemptFrom(resp.Request.Context()); att != nil {
				up := att.current()
				p.ok(up)
				// 统计后端返回的响应体字节数。101 协议升级时 ReverseProxy 要求响应体是可读写的连接，包装会破坏升级，不统计
				if resp.StatusCode != http.StatusSwitchingProtocols && resp.Body != nil && resp.Body != http.NoBody {
					resp.Body = &upstreamBody{ReadCloser: resp.Body, st: up.stats}
				}
			}
			// 拦截后端返回的错误状态码，换成站点设置的错误页（只替换浏览器打开页面的请求）
			if h.errPage.intercepts(resp) {
				h.errPage.replace(resp)
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			// 管理员在实时连接中断开的请求不算后端错误，也不写出响应（写出会被丢弃）
			if errors.Is(context.Cause(r.Context()), errConnClosed) {
				return
			}
			// 访客发送请求体时空闲超时：不是后端的错，不计入被动检查，返回 408（与 nginx 的 client_body_timeout 一致）
			if clientDeadlinesFrom(r.Context()).readTimedOut() {
				h.reqLog(r).Warnf("%s %s%s 读取请求体超时: %v", r.Method, r.Host, r.URL.Path, err)
				writeErrorPage(h.errPage, w, r, http.StatusRequestTimeout, "请求超时", "请求没有在规定时间内完成，请重试。")
				return
			}
			h.stats.Errors.Add(1)
			// 客户端中途断开不是后端的错
			if att := attemptFrom(r.Context()); att != nil && r.Context().Err() == nil {
				p.fail(att.current(), err)
			}
			h.reqLog(r).Warnf("%s %s%s 后端错误: %v", r.Method, r.Host, r.URL.Path, err)
			// 连接、TLS 握手或等待响应头超时返回 504（与 nginx 一致），其余错误返回 502
			if isUpstreamTimeout(err) {
				writeErrorPage(h.errPage, w, r, http.StatusGatewayTimeout, "后端服务响应超时", "站点的后端服务没有在规定时间内响应，请稍后重试。")
				return
			}
			writeErrorPage(h.errPage, w, r, http.StatusBadGateway, "无法连接到后端服务", "站点的后端服务没有响应，可能未启动或网络不可达。请稍后重试。")
		},
		// 流式响应（SSE 等）及时刷新
		FlushInterval: -1,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client := clientAddr(r)
		active, _ := r.Context().Value(ctxKeyActiveReq{}).(*activeReq)
		att := &proxyAttempt{pool: p, client: client.Addr(), active: active}
		att.use(p.pick(client.Addr()))
		defer att.release()
		ctx := context.WithValue(r.Context(), ctxKeyClientAddr{}, client)
		ctx = context.WithValue(ctx, ctxKeyUpstream{}, att)
		if h.errPage != nil && h.errPage.codes != nil {
			ctx = context.WithValue(ctx, ctxKeyVisitorReq{}, r)
		}
		in := r.WithContext(ctx)
		att.in = in
		rp.ServeHTTP(w, in)
	})
}

// isUpstreamTimeout 判断代理错误是否为连接后端时的超时：拨号、TLS 握手与等待响应头超时都实现 net.Error 的 Timeout。
// 访客断开导致的 context.Canceled 不算超时。
func isUpstreamTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// localPort 返回请求到达的本地端口。经 PROXY 协议转入时为原始目的端口。
func localPort(r *http.Request) string {
	if a, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		if _, port, err := net.SplitHostPort(a.String()); err == nil {
			return port
		}
	}
	return ""
}

// stripCookie 从请求中删除指定名称的 Cookie。
func stripCookie(r *http.Request, names ...string) {
	cookies := r.Cookies()
	if len(cookies) == 0 {
		return
	}
	var kept []string
	for _, c := range cookies {
		drop := false
		for _, n := range names {
			if c.Name == n {
				drop = true
				break
			}
		}
		if !drop {
			kept = append(kept, c.Name+"="+c.Value)
		}
	}
	if len(kept) == len(cookies) {
		return
	}
	r.Header.Del("Cookie")
	if len(kept) > 0 {
		r.Header.Set("Cookie", strings.Join(kept, "; "))
	}
}

// applyHeaders 按 "Key: Value" 设置头，值为空时删除。
func applyHeaders(h http.Header, list []string) {
	for _, line := range list {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if v == "" {
			h.Del(k)
		} else {
			h.Set(k, v)
		}
	}
}

// allowedIP 按访问控制模式判断客户端是否允许访问。
// 命中的路径规则设置了 IP 模式时，以规则的名单为准。
// 引用的订阅 IP 列表尚未下载成功时视为空列表：白名单因此拒绝访问，黑名单不额外拦截。
func (h *siteHandler) allowedIP(a netip.Addr, rule *compiledRule) bool {
	mode, list, sets := h.site.IPMode, h.ipList, h.site.IPSets
	if rule != nil && rule.ipMode != IPModeOff {
		mode, list, sets = rule.ipMode, rule.ipList, rule.ipSets
	}
	return iplist.Default.Allowed(mode, a, list, sets)
}

// statusRecorder 记录响应状态码，用于访问日志。
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
	// denied 表示请求被访问控制拒绝并断开连接（记为 444）
	denied bool
	// deny 为请求被拦截的原因（blockNames 中的名称），写入访问日志的 deny 字段，日志页据此隐藏被封禁来源的请求
	deny string
	// req 为该请求在实时连接列表中的登记项，可为 nil
	req *activeReq
	// wroteHeader 表示已写出响应头
	wroteHeader bool
	// rate 为站点的秒级速率，写出时登记字节数（大文件下载的吞吐随写出实时体现）
	rate *rateRing
	// firstByte 为首次写出响应头（或响应体）的时间，零值表示尚未写出，用于首字节延迟统计
	firstByte time.Time
	// dl 为访客写出空闲超时，每次写出前设置截止时间，写完清除；nil 表示不限制
	dl *clientDeadlines
}

// countsForLatency 判断这次请求是否计入首字节延迟：必须写出过响应，且不是协议升级、断开连接或被管理员断开的请求。
func (s *statusRecorder) countsForLatency() bool {
	if s.firstByte.IsZero() || s.status == http.StatusSwitchingProtocols || s.status == statusClosed {
		return false
	}
	return s.req == nil || !s.req.killed.Load()
}

// markFirstByte 记录首次写出响应的时间。
func (s *statusRecorder) markFirstByte() {
	if s.firstByte.IsZero() {
		s.firstByte = time.Now()
	}
}

func (s *statusRecorder) Write(p []byte) (int, error) {
	// 被管理员断开后不再写出任何内容，让正在拷贝响应体的处理器（静态文件、反向代理）尽快结束
	if s.req != nil && s.req.killed.Load() {
		return 0, errConnClosed
	}
	s.wroteHeader = true
	s.markFirstByte()
	armed := s.dl != nil && s.dl.armWrite()
	n, err := s.ResponseWriter.Write(p)
	if armed {
		s.dl.clearWrite()
	}
	s.bytes += int64(n)
	if s.req != nil {
		s.req.down.Add(int64(n))
	}
	if s.rate != nil && n > 0 {
		s.rate.add(time.Now().Unix(), 0, int64(n))
	}
	return n, err
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.req != nil && s.req.killed.Load() {
		return
	}
	s.status = code
	// 1xx 是中间响应，之后还会写出最终的响应头
	if code >= 200 || code == http.StatusSwitchingProtocols {
		s.wroteHeader = true
		s.markFirstByte()
	}
	s.ResponseWriter.WriteHeader(code)
}

// orDash 把空的请求头记为 "-"，与 nginx 访问日志一致。
func orDash(v string) string {
	if v == "" {
		return "-"
	}
	return v
}

// statusClosed 是断开连接时记入访问日志与统计的状态码，沿用 nginx 的 444。
const statusClosed = 444

// 访问日志中 URI、Referer、User-Agent 的最大字节数。请求头最长可达 1 MB，%q 转义后还会变长，
// 不截断时一条日志就可能有数 MB，同时写入内存日志、落盘文件与标准输出。
const (
	maxLogURI    = 2048
	maxLogHeader = 512
)

// clip 把 s 截断到最多 n 字节（不截断多字节字符），截断时末尾加 "…"。
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

// Unwrap 让 http.ResponseController 能访问底层连接（WebSocket 升级等需要）。
func (s *statusRecorder) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}

func (h *siteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.stats.Requests.Add(1)
	h.stats.Active.Add(1)
	defer h.stats.Active.Add(-1)
	start := time.Now()
	client := clientAddr(r)
	// 秒级速率按到达计数，近期活动 IP 在到达时记录（WebSocket 等长请求进行中也能看到）
	h.metrics.rate.add(start.Unix(), 1, 0)
	h.metrics.recent.touch(client.Addr(), start.Unix())
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK, rate: &h.metrics.rate}
	// 重写规则会原地改写 r.URL，访问日志与路径探测要用访客实际请求的路径
	origPath, origURI := r.URL.Path, r.URL.RequestURI()
	// 带本实例管理后台登录态的请求（管理员经这个站点访问后台，例如日志页的轮询）在访问日志中标记用户名，
	// 日志页可以隐藏它们，访客审计据此标记管理员。在处理前取得：之后请求头可能被改写；只在记录访问日志或访客审计时校验
	admin := ""
	if h.site.AccessLog || h.auth.recordsVisits() {
		admin = httpx.SessionUser(r)
	}

	defer func() {
		// 首字节延迟：协议升级（长连接）、被管理员断开与未写出响应（断开连接）的请求不计入（-1）
		lat := time.Duration(-1)
		if rec.countsForLatency() {
			lat = rec.firstByte.Sub(start)
		}
		h.metrics.record(client.Addr(), rec.status, rec.bytes, lat, time.Now())
		h.recordVisit(r, client.Addr(), rec, origPath, adminTag(admin, rec.deny))
		if h.site.AccessLog {
			uri := origURI
			// 认证路径的查询串含票据、授权码等敏感信息，不写入日志
			if strings.HasPrefix(origPath, authPath) {
				uri = origPath
			}
			// 末尾按 nginx combined 格式附上 Referer 与 User-Agent，便于区分同一出口 IP 后面的不同设备；
			// %q 会转义换行等控制字符，防止伪造日志行；三者都由访客控制，先截断再写入
			h.reqLog(r).With(logbuf.Fields{logbuf.FieldStatus: strconv.Itoa(rec.status), logbuf.FieldDeny: rec.deny, logbuf.FieldAdmin: adminTag(admin, rec.deny)}).
				Infof("%s %s %s%s %d %dms %q %q", client.Addr(), r.Method, r.Host, clip(uri, maxLogURI), rec.status, time.Since(start).Milliseconds(),
					orDash(clip(r.Referer(), maxLogHeader)), orDash(clip(r.UserAgent(), maxLogHeader)))
		}
		// 交给入侵防护的路径探测规则计数（444 断开连接同样经过这里）。普通站点上访问控制拒绝后断开的请求按 403 上报，
		// 与未开启“拒绝时断开连接”时一致：它们是被名单或国家过滤挡下的访客，不是在探测不存在的路径。
		// 默认站点例外，仍按 444 上报：访问未知域名的名单外访客与「断开连接」类型默认站点处理的请求是同一类，
		// 默认站点改用“反向代理 + 白名单 + 拒绝时断开连接”放行自家 IP 后，未知域名扫描仍能被统计
		// 管理员手动断开的请求不是访客行为，不计入路径探测
		if rec.req != nil && rec.req.killed.Load() {
			return
		}
		status := rec.status
		if rec.denied && !h.deflt {
			status = http.StatusForbidden
		}
		webprobe.Report(webprobe.Hit{Addr: client.Addr(), Listener: h.listener, Site: h.site.Key, Unknown: h.deflt,
			Method: r.Method, Host: r.Host, Path: origPath, Status: status})
	}()

	// 断开连接的站点不经过访问控制、WAF、跨域、认证与重写，直接断开，与 nginx server 级 return 444 在 rewrite 阶段生效一致。
	// 只改记录的状态码、不写出响应；net/http 遇到 ErrAbortHandler 不写响应：HTTP/1.x 直接断开连接，HTTP/2、HTTP/3 重置这个请求流。
	// 访问日志与统计在上面的 defer 中照常记录为 444
	if h.site.Type == TypeClose {
		rec.status = statusClosed
		panic(http.ErrAbortHandler)
	}

	// 登记到实时连接列表：断开时取消请求上下文，后续写出被丢弃，处理结束后在这里中断连接（HTTP/2、HTTP/3 重置请求流）。
	// 注销放在最后注册的 defer 中，先于访问日志执行，历史记录与计数及时回落
	r, ar := h.conns.begin(w, r, client)
	defer h.conns.end(ar)
	rec.req = ar
	// 访客空闲超时：读取请求体与写出响应时一次卡住超过设置的秒数即放弃，防止极慢的访客长期占住连接
	dl := newClientDeadlines(w, h.site.ClientTimeouts)
	// 处理函数返回后，仍在读取请求体的转发协程不能再设置截止时间（见 clientDeadlines）
	defer dl.finish()
	rec.dl = dl
	r = r.WithContext(context.WithValue(r.Context(), ctxKeyClientDeadlines{}, dl))
	if r.Body != nil && r.Body != http.NoBody {
		r.Body = &idleBody{ReadCloser: &trackedBody{ReadCloser: r.Body, req: ar}, d: dl}
	}
	defer func() {
		// 已接管的连接由 close 直接关闭，无需再中断
		if !ar.killed.Load() || ar.upgraded.Load() {
			return
		}
		// 处理过程中的其他 panic 原样向上传递
		if p := recover(); p != nil {
			panic(p)
		}
		// 还没写出响应头时，访问日志与统计按 444 记录（与断开连接站点一致）
		if !rec.wroteHeader {
			rec.status = statusClosed
		}
		panic(http.ErrAbortHandler)
	}()

	release, ok := h.accessCheck(rec, r)
	if !ok {
		return
	}
	defer release()
	// WAF（Coraza 规则组或雷池）拦截时在请求上下文中置位标记，认证、后端等返回的 403 不算 WAF 拦截
	var wafBlocked *atomic.Bool
	if h.site.WAF != "" {
		r, wafBlocked = httpx.WithWAFFlag(r)
	}
	h.pipeline.ServeHTTP(rec, r)
	if wafBlocked != nil && wafBlocked.Load() && !ar.killed.Load() {
		h.block(rec, blockWAF)
	}
}

// accessCheck 执行站点的访问控制：封禁、IP 名单、国家过滤与限流。不通过时已写出 403 或 429（开启拒绝时断开连接则直接断开）并返回 false；
// 通过时返回的 release 需在请求结束后调用，以释放限流的并发计数。签发者域名上的 OIDC 协议路径同样经过这里。
func (h *siteHandler) accessCheck(w http.ResponseWriter, r *http.Request) (release func(), ok bool) {
	client := clientAddr(r)
	noop := func() {}
	if h.auth.banned(client.Addr()) {
		h.block(w, blockBan)
		h.deny(w)
		return noop, false
	}
	if !h.allowedIP(client.Addr(), h.matchRule(r.URL.Path)) {
		h.block(w, blockIP)
		h.reqLog(r).Warnf("拒绝 %s 访问 %s（访问控制）", client.Addr(), r.Host)
		h.deny(w)
		return noop, false
	}
	if !h.countryAllowed(client.Addr()) {
		h.block(w, blockCountry)
		h.reqLog(r).Warnf("拒绝 %s 访问 %s（国家过滤：%s）", client.Addr(), r.Host, geoip.Default.Country(client.Addr()))
		h.deny(w)
		return noop, false
	}
	if h.limiter != nil {
		rel, retry, ok := h.limiter.acquire(limitKeyAddr(client.Addr()))
		if !ok {
			h.block(w, blockRateLimit)
			w.Header().Set("Retry-After", retryAfterSeconds(retry))
			http.Error(w, "429 Too Many Requests", http.StatusTooManyRequests)
			return noop, false
		}
		return rel, true
	}
	return noop, true
}

// block 按原因计入拦截次数，并把原因记到 statusRecorder 上供访问日志使用；
// 签发者域名上的 OIDC 协议路径直接传入原始 ResponseWriter，只计数。
func (h *siteHandler) block(w http.ResponseWriter, kind int) {
	h.stats.Blocked[kind].Add(1)
	if rec, ok := w.(*statusRecorder); ok {
		rec.deny = blockNames[kind]
	}
}

// deny 拒绝一次访问控制未通过的请求：站点开启了“拒绝时断开连接”时不写响应直接断开，
// 访问日志与统计记为 444（与断开连接站点相同，HTTP/2、HTTP/3 下重置这个请求流）；否则返回 403。
func (h *siteHandler) deny(w http.ResponseWriter) {
	if h.site.DenyClose {
		// 经 ServeHTTP 进来时 w 是 statusRecorder，改记录的状态码供 defer 中的访问日志使用；
		// 签发者域名上的 OIDC 协议路径直接传入原始 ResponseWriter，无需记录
		if rec, ok := w.(*statusRecorder); ok {
			rec.status = statusClosed
			rec.denied = true
		}
		panic(http.ErrAbortHandler)
	}
	http.Error(w, "403 Forbidden", http.StatusForbidden)
}

// adminTag 返回访问日志中的后台用户字段：被拦截（封禁、名单、WAF 等）的请求不标记，
// 带后台登录态也不能让被拦截的请求在“隐藏后台自身请求”时消失。
func adminTag(admin, deny string) string {
	if deny != "" {
		return ""
	}
	return admin
}

// serveInner 处理跨域、认证并交给站点处理器。
func (h *siteHandler) serveInner(rec http.ResponseWriter, r *http.Request) {
	if h.site.CORS {
		if origin := r.Header.Get("Origin"); origin != "" && h.corsAllowed(origin) {
			rec.Header().Set("Access-Control-Allow-Origin", origin)
			rec.Header().Set("Access-Control-Allow-Credentials", "true")
			rec.Header().Set("Vary", "Origin")
			if r.Method == http.MethodOptions {
				rec.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				if req := r.Header.Get("Access-Control-Request-Headers"); req != "" {
					rec.Header().Set("Access-Control-Allow-Headers", req)
				}
				rec.WriteHeader(http.StatusNoContent)
				return
			}
		}
	}
	// 登录页等认证保留路径始终交给认证器处理，其余路径按规则决定是否需要认证
	if h.site.WebAuth && strings.HasPrefix(r.URL.Path, authPath) {
		h.auth.check(rec, r, h.site)
		return
	}
	if h.fwd != nil {
		// 无论该路径是否需要认证，都不允许访客自带身份头
		h.fwd.stripIdentity(r)
	}
	h.stripIdentityHeaders(r)
	// 跳转类重写不返回任何内容，先于认证执行，与 nginx 中 location = / { return 302 ...; } 不经认证的行为一致
	if len(h.rewrites) > 0 && h.redirectBeforeAuth(rec, r) {
		return
	}
	// id 为认证识别出的访客身份，用于按用户分流与传递身份信息
	var id visitor
	if h.needAuth(h.matchRule(r.URL.Path)) {
		handled, allowed := h.tryShare(rec, r)
		if handled {
			return
		}
		if !allowed {
			if h.fwd != nil {
				if !h.fwd.check(rec, r, h) {
					return
				}
				id = h.fwd.forwardIdentity(r)
			} else {
				var ok bool
				if id, ok = h.auth.checkUser(rec, r, h.site); !ok {
					return
				}
			}
		}
	} else if h.usesIdentity() && h.fwd == nil {
		// 无需认证的路径上，已网页登录的访客仍按身份分流，同一应用的页面不会落到不同后端
		id = h.auth.sessionIdentity(r, h.site)
	}
	// 认证识别出的身份记到实时连接登记项上，请求结束时随访客审计一并记录
	if len(id.names) > 0 {
		if a, ok := r.Context().Value(ctxKeyActiveReq{}).(*activeReq); ok {
			name := id.names[0]
			a.user.Store(&name)
		}
	}
	if h.usesIdentity() {
		r = h.withIdentity(r, id)
	}
	if len(h.rewrites) > 0 && h.applyRewrite(rec, r) {
		return
	}
	h.handler.ServeHTTP(rec, r)
}

// corsAllowed 判断是否向该来源开放带凭据的跨域访问。
// 站点开启认证时只允许站点自身域名作为来源，否则任意网页都能借用户的登录态读取内容。
func (h *siteHandler) corsAllowed(origin string) bool {
	if !h.site.BasicAuth && !h.site.WebAuth {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	host := hostOnly(u.Host)
	for _, d := range h.site.Domains {
		if d != "*" && matchDomain(d, host) > 0 {
			return true
		}
	}
	return false
}

// snapshot 返回统计快照。
func (s *SiteStats) snapshot() map[string]int64 {
	return map[string]int64{"requests": s.Requests.Load(), "active": s.Active.Load(), "errors": s.Errors.Load()}
}

func (h *siteHandler) String() string {
	return fmt.Sprintf("%s %v", h.site.Type, h.site.Domains)
}

// writeErrorPage 浏览器访问时输出样式化的错误页（404 与 5xx 按站点设置的样式），其他客户端输出纯文本。
// e 为站点的错误页设置，nil 表示默认样式。
func writeErrorPage(e *errorPager, w http.ResponseWriter, r *http.Request, code int, title, detail string) {
	e.write(w, r, webpage.Problem{Code: code, Title: title, Detail: detail})
}

// writeNotFound 输出 404：浏览器看到错误页，其他客户端与 http.NotFound 一致。
func writeNotFound(e *errorPager, w http.ResponseWriter, r *http.Request, title, detail string) {
	if !webpage.WantsHTML(r) {
		http.NotFound(w, r)
		return
	}
	e.write(w, r, webpage.Problem{Code: http.StatusNotFound, Title: title, Detail: detail, Home: r.URL.Path != "/"})
}
