package webservice

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	proxyproto "github.com/pires/go-proxyproto"
	"github.com/quic-go/quic-go/http3"

	"github.com/wangwanjie/gately/internal/acmehttp"
	"github.com/wangwanjie/gately/internal/logbuf"
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
	cfg  Listener
	auth *authenticator
	log  *logbuf.Logger
	srv  *http.Server
	// ln 为交给 srv 的监听（含各层包装），停止时由 stop 同步关闭，见 stop 中的说明
	ln net.Listener
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
}

// siteTable 是一个监听器在某一时刻的全部站点。
type siteTable struct {
	sites []*siteHandler
	deflt *siteHandler
	// exact 为精确域名到站点的索引，站点很多时避免逐个比较
	exact map[string]*siteHandler
	// cancel 停止这一批站点的后台任务（健康检查）
	cancel context.CancelFunc
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
	for _, s := range t.sites {
		for _, d := range s.site.Domains {
			score := matchDomain(d, host)
			if score == 0 {
				continue
			}
			if score > bestScore || (score == bestScore && len(d) > bestLen) {
				best, bestScore, bestLen = s, score, len(d)
			}
		}
	}
	if best != nil {
		return best
	}
	return t.deflt
}

func (l *listenerRunner) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	// HTTPS 端口收到的明文请求按设置跳转到同端口的 https 地址（ACME 验证已在上面应答）；
	// 可信反向代理已终止 TLS 并以 X-Forwarded-Proto: https 转来的请求不再跳转，否则会循环跳转
	if l.cfg.PlainHTTP == PlainHTTPRedirect && !requestIsHTTPS(r) {
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
		writeNotFound(w, r, "站点不存在", "这个域名没有配置对应的站点，请检查访问地址是否正确。")
		// 未知域名且没有默认站点时，同样交给路径探测规则计数
		webprobe.Report(webprobe.Hit{Addr: clientAddr(r).Addr(), Listener: l.cfg.Key, Unknown: true,
			Method: r.Method, Host: r.Host, Path: r.URL.Path, Status: http.StatusNotFound})
		return
	}
	s.ServeHTTP(w, r)
}

func newListenerRunner(cfg Listener, log *logbuf.Logger, auth *authenticator, waf WAFProvider) (*listenerRunner, error) {
	l := &listenerRunner{cfg: cfg, log: log, auth: auth}
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
	t := &siteTable{exact: map[string]*siteHandler{}}
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
		t.sites = append(t.sites, h)
		for _, d := range h.site.Domains {
			// 同一域名出现在多个站点时以先出现的为准，与逐个比较时的结果一致
			if !strings.Contains(d, "*") {
				if _, dup := t.exact[d]; !dup {
					t.exact[d] = h
				}
			}
		}
	}
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
		a.PlainHTTP == b.PlainHTTP && a.MinTLS == b.MinTLS && a.HTTP3 == b.HTTP3 && a.AcceptProxyProtocol == b.AcceptProxyProtocol
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
func (l *listenerRunner) start(certs CertSource, trusted []string) error {
	addr := net.JoinHostPort(l.cfg.ListenIP, strconv.Itoa(l.cfg.Port))
	ln, err := net.Listen(l.cfg.Network, addr)
	if err != nil {
		return err
	}
	if l.cfg.AcceptProxyProtocol {
		if len(trusted) == 0 {
			ln.Close()
			return errors.New("已开启接收 PROXY 协议，但未配置可信来源")
		}
		// 只有可信来源发来的 PROXY 头才会被采用，其他来源的头被忽略
		policy, err := proxyproto.ConnLaxWhiteListPolicy(trusted)
		if err != nil {
			ln.Close()
			return err
		}
		ln = &proxyproto.Listener{Listener: ln, ConnPolicy: policy, ReadHeaderTimeout: 5 * time.Second}
	}
	baseCtx, cancel := context.WithCancel(context.Background())
	l.cancel = cancel
	l.baseCtx = baseCtx
	l.srv = &http.Server{
		Handler:     l,
		BaseContext: func(net.Listener) context.Context { return baseCtx },
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
		// net/http 内部错误（TLS 握手失败、缺少证书等）写入本监听器的运行日志
		ErrorLog: l.log.ServerErrorLog(),
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
	return nil
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
