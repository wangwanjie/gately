package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/webpage"
)

// 未授权访问的响应方式（Settings.DenyMode）。
//
// 默认响应带有明显的 Gately 特征（安全入口提示页、纯文本 403），公网访客一看便知这里是管理后台。
// 其余方式用来隐藏管理端口：普通 404 与空白页不含任何特征；断开连接与不响应在能按 TCP 对端判定时
// （仅内网访问、来源被封禁）直接在接受连接时处理，HTTPS 端口连证书都不会发出。
// TCP 握手由内核完成，端口扫描仍能发现端口开放；要让端口彻底不可见只能靠防火墙或不做端口映射。
const (
	denyModeDefault  = ""
	denyModeNotFound = "notfound"
	denyModeBlank    = "blank"
	denyModeClose    = "close"
	denyModeDrop     = "drop"
	denyModeCustom   = "custom"
)

// denyStatuses 为自定义页面可选的状态码。
var denyStatuses = []int{
	http.StatusOK, http.StatusForbidden, http.StatusNotFound, http.StatusGone,
	http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable,
}

// maxDenyPage 为自定义页面的大小上限：页面内容保存在设置里、每次拦截都整体写出，不宜过大。
const maxDenyPage = 64 << 10

// denyPageCSP 让自定义页面运行在沙箱（独立的不透明源）中：页面与管理后台同源，
// 页面里的脚本如果能执行，就能以访问者（可能是已登录的管理员）的身份调用接口。
// 只允许内联样式与 data: 图片字体，页面不能执行脚本、提交表单或加载外部资源。
const denyPageCSP = "sandbox; default-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src data:; media-src data:; form-action 'none'; base-uri 'none'; frame-ancestors 'none'"

// notFoundPage 为“普通 404”的正文，仿照常见 Web 服务器的默认错误页，不含任何产品信息。
const notFoundPage = "<html>\r\n<head><title>404 Not Found</title></head>\r\n<body>\r\n<center><h1>404 Not Found</h1></center>\r\n</body>\r\n</html>\r\n"

// 不响应时每条连接最多挂起的时长与同时挂起的连接数；超出上限的连接直接断开，避免被用来耗尽资源。
var (
	denyHoldTimeout = 2 * time.Minute
	denyHoldSlots   = make(chan struct{}, 256)
)

// 挂起期间最多读取并丢弃的数据量，读满后断开，防止被当作上传通道占用带宽。
const denyHoldReadLimit = 64 << 10

func validateDeny(s *Settings) error {
	switch s.DenyMode {
	case denyModeDefault, denyModeNotFound, denyModeBlank, denyModeClose, denyModeDrop, denyModeCustom:
	default:
		return fmt.Errorf("未授权访问的响应方式 %q 无效", s.DenyMode)
	}
	if !slices.Contains(denyStatuses, s.DenyStatus) {
		return fmt.Errorf("自定义页面的状态码 %d 无效", s.DenyStatus)
	}
	if len(s.DenyPage) > maxDenyPage {
		return fmt.Errorf("自定义页面不能超过 %d KB", maxDenyPage>>10)
	}
	return nil
}

// denyFallback 为默认方式下各拦截点原有的响应。
type denyFallback int

const (
	deny403       denyFallback = iota // 封禁与仅内网访问：纯文本 403
	denyEntryHint                     // 安全入口：浏览器看到提示页，接口等其他请求为纯文本 404
)

// deny 按设置的方式响应未授权的请求。
func (a *App) deny(w http.ResponseWriter, r *http.Request, s Settings, fallback denyFallback) {
	mode := s.DenyMode
	if (mode == denyModeClose || mode == denyModeDrop) && viaProxy(r) {
		// 经反向代理转发来的请求：中止或挂起只会让代理给访客返回它自己的 502 页面（前置代理是 Gately Web 服务时带有 Gately 标识），
		// 不响应时还会占住代理到这里的连接。改为普通 404，访客看到的仍是一个没有内容的普通站点
		mode = denyModeNotFound
	}
	if mode != denyModeDefault {
		// 管理后台统一加的安全响应头是可识别的特征，隐藏方式下去掉（自定义页面的 CSP 已包含 frame-ancestors）
		h := w.Header()
		h.Del("X-Frame-Options")
		h.Del("Referrer-Policy")
		h.Del("X-Content-Type-Options")
	}
	switch mode {
	case denyModeNotFound:
		writeDenyBody(w, http.StatusNotFound, "text/html", notFoundPage)
	case denyModeBlank:
		writeDenyBody(w, http.StatusOK, "text/html", "")
	case denyModeClose:
		// 不写任何响应直接中止：HTTP/1 关闭连接，HTTP/2 重置该请求的流
		panic(http.ErrAbortHandler)
	case denyModeDrop:
		a.holdRequest(r)
		panic(http.ErrAbortHandler)
	case denyModeCustom:
		w.Header().Set("Content-Security-Policy", denyPageCSP)
		// 手工改坏的配置文件不经过 Validate，状态码不在可选范围时按 200 返回
		status := s.DenyStatus
		if !slices.Contains(denyStatuses, status) {
			status = http.StatusOK
		}
		writeDenyBody(w, status, "text/html; charset=utf-8", s.DenyPage)
	default:
		if fallback == deny403 {
			http.Error(w, "403 Forbidden", http.StatusForbidden)
			return
		}
		// 浏览器访问时给出提示页（仍返回 404），接口等其他请求保持纯文本 404
		webpage.Error(w, r, webpage.Problem{
			Code:   http.StatusNotFound,
			Lock:   true,
			Title:  "请通过安全入口访问",
			Detail: "当前地址未通过安全入口验证，无法打开管理后台。",
			Hint:   "请使用完整的安全入口地址（例如 http://域名:端口/入口路径）访问；忘记入口时，可在配置目录的 gately.json 中清空 safePath 后重启 Gately。",
		})
	}
}

// writeDenyBody 写出拦截响应。带上 Content-Length：短小的正文因此不会被压缩、不会多出 Vary 头，与普通 Web 服务器的默认页一致。
func writeDenyBody(w http.ResponseWriter, status int, contentType, body string) {
	h := w.Header()
	h.Set("Content-Type", contentType)
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

// viaProxy 判断请求是否由可信的反向代理转发（对端可信且带有转发的客户端地址）。
func viaProxy(r *http.Request) bool {
	if r.Header.Get("X-Forwarded-For") == "" && r.Header.Get("X-Real-IP") == "" {
		return false
	}
	return httpx.TrustsPeer(httpx.PeerIP(r))
}

// connHoldKey 是连接上下文中的挂起计数（*atomic.Int32），由 denyConnContext 在每条连接建立时放入。
type connHoldKey struct{}

// denyConnContext 用作管理服务的 http.Server.ConnContext，为每条连接记录正在挂起的请求数。
func denyConnContext(ctx context.Context, _ net.Conn) context.Context {
	return context.WithValue(ctx, connHoldKey{}, new(atomic.Int32))
}

// holdRequest 不作任何响应，等到客户端放弃、挂起超时或服务停止后返回；同时挂起的数量已满时立即返回。
func (a *App) holdRequest(r *http.Request) {
	// 每条连接最多挂起一个请求：HTTP/2 一条连接可并发上百个流，不加限制时一条连接就能占满全部名额，
	// 其余被拒请求随之退化为立即断开
	if n, ok := r.Context().Value(connHoldKey{}).(*atomic.Int32); ok {
		if n.Add(1) > 1 {
			n.Add(-1)
			return
		}
		defer n.Add(-1)
	}
	select {
	case denyHoldSlots <- struct{}{}:
	default:
		return
	}
	defer func() {
		<-denyHoldSlots
	}()
	t := time.NewTimer(denyHoldTimeout)
	defer t.Stop()
	select {
	case <-r.Context().Done():
	case <-a.holdCtx.Done():
	case <-t.C:
	}
}

// denyListener 在接受连接时处理能按 TCP 对端判定的未授权访问（仅内网访问、来源被封禁），
// 用于断开连接与不响应两种方式：连接交给 HTTP 服务之前就被处理，HTTPS 端口不会完成 TLS 握手、不会发出证书。
type denyListener struct {
	net.Listener
	a *App
}

func (l *denyListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if !l.a.denyConn(c) {
			return c, nil
		}
	}
}

// denyConn 判断新连接是否在接受时就拒绝，是则自行处理该连接并返回 true。
func (a *App) denyConn(c net.Conn) bool {
	s := a.settingsSnapshot()
	if s.DenyMode != denyModeClose && s.DenyMode != denyModeDrop {
		return false
	}
	ap, err := netip.ParseAddrPort(c.RemoteAddr().String())
	if err != nil {
		return false
	}
	ip := ap.Addr().Unmap()
	// 内网地址可能是反向代理，真实来源要看请求头；可信代理同理，都留给 HTTP 层判断
	if isPrivate(ip.String()) || httpx.TrustsPeer(ip.String()) {
		return false
	}
	if !s.LANOnly && !banlist.Default.Banned(ip) {
		return false
	}
	if s.DenyMode == denyModeClose {
		resetConn(c)
		return true
	}
	a.holdConn(c)
	return true
}

// resetConn 以 RST 关闭连接，不经过正常的四次挥手。
func resetConn(c net.Conn) {
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.SetLinger(0)
	}
	_ = c.Close()
}

// holdConn 挂起连接：读取并丢弃对方发来的数据但从不回应，直到对方关闭、超时或服务停止；挂起数量已满时直接断开。
func (a *App) holdConn(c net.Conn) {
	select {
	case denyHoldSlots <- struct{}{}:
	default:
		resetConn(c)
		return
	}
	go func() {
		defer func() {
			<-denyHoldSlots
		}()
		_ = c.SetReadDeadline(time.Now().Add(denyHoldTimeout))
		stop := context.AfterFunc(a.holdCtx, func() {
			_ = c.SetReadDeadline(time.Now())
		})
		defer stop()
		_, err := io.CopyN(io.Discard, c, denyHoldReadLimit)
		if err == nil || errors.Is(err, io.EOF) {
			_ = c.Close()
			return
		}
		resetConn(c)
	}()
}
