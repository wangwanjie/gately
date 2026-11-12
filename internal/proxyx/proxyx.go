// Package proxyx 统一处理出站代理设置：通知渠道、Webhook、订阅下载等访问外部服务时，
// 可以按配置走指定代理、复用全局「更新代理」或强制直连。
//
// 代理设置是一个字符串，取值为：
//
//	""        未设置：HTTP 请求遵循 HTTPS_PROXY 等环境变量，TCP 连接（如 SMTP）直连
//	"direct"  直连：不经任何代理，也不读取环境变量
//	"update"  使用全局设置中的更新代理；更新代理为空时按未设置处理
//	代理地址   http://、https://、socks5:// 或 socks5h:// 开头，可带 用户名:密码@
//
// 少数场景（如通知渠道）还会把 "" 解释为“跟随上一级设置”，由调用方先行合并后再交给本包。
package proxyx

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	xproxy "golang.org/x/net/proxy"

	"github.com/wangwanjie/gately/internal/outbound"
)

// 特殊取值
const (
	// Direct 表示直连，不经任何代理
	Direct = "direct"
	// Update 表示使用全局设置中的更新代理
	Update = "update"
)

// updateFn 返回全局设置中的更新代理，由应用启动时注入
var updateFn atomic.Pointer[func() string]

// SetUpdateProxy 注入读取全局更新代理的函数，每次解析代理设置时调用，修改设置后立即生效。
func SetUpdateProxy(fn func() string) {
	if fn == nil {
		updateFn.Store(nil)
		return
	}
	updateFn.Store(&fn)
}

// UpdateProxy 返回当前的全局更新代理，未注入时为空。
func UpdateProxy() string {
	if fn := updateFn.Load(); fn != nil {
		return strings.TrimSpace((*fn)())
	}
	return ""
}

// errInvalid 为代理地址格式错误的提示（完整字面量，便于翻译）
var errInvalid = errors.New("代理地址需为 http://、https:// 或 socks5:// 开头的地址，例如 http://127.0.0.1:7890")

// ValidateURL 校验代理地址，支持 http、https、socks5 与 socks5h。
func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" && u.Scheme != "socks5h") {
		return errInvalid
	}
	// 只允许 协议://[用户名:密码@]主机[:端口]，带路径、查询串的多半是填错了地址
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return errInvalid
	}
	if p := u.Port(); p != "" {
		if n, err := strconv.Atoi(p); err != nil || n < 1 || n > 65535 {
			return errInvalid
		}
	}
	return nil
}

// Validate 校验代理设置：空值、direct、update 或代理地址。
func Validate(setting string) error {
	switch setting = Normalize(setting); setting {
	case "", Direct, Update:
		return nil
	}
	return ValidateURL(setting)
}

// Normalize 去掉首尾空白，并把大小写不同的 direct、update 统一为小写。
func Normalize(setting string) string {
	setting = strings.TrimSpace(setting)
	switch strings.ToLower(setting) {
	case Direct:
		return Direct
	case Update:
		return Update
	}
	return setting
}

// Resolve 解析代理设置：返回要使用的代理地址；u 为 nil 时 direct 表示强制直连，否则表示未指定（沿用默认行为）。
func Resolve(setting string) (u *url.URL, direct bool) {
	setting = Normalize(setting)
	switch setting {
	case "":
		return nil, false
	case Direct:
		return nil, true
	case Update:
		setting = UpdateProxy()
		if setting == "" {
			return nil, false
		}
	}
	u, err := url.Parse(setting)
	if err != nil || ValidateURL(setting) != nil {
		// 保存时已校验，这里只防御异常配置（例如更新代理被手工改坏）：按未设置处理，不因此中断发送
		return nil, false
	}
	return u, false
}

// Describe 返回代理设置的展示文本（隐藏密码），用于日志与错误信息。
func Describe(setting string) string {
	u, direct := Resolve(setting)
	switch {
	case direct:
		return Direct
	case u == nil:
		return ""
	}
	return u.Redacted()
}

// ---- HTTP 客户端 ----

// 按“代理地址 + 是否跳过证书校验”缓存 Transport，复用连接；配置中的代理通常只有几个，超过上限时整体清空重建
const maxTransports = 32

var (
	transportMu sync.Mutex
	transports  = map[string]*http.Transport{}
)

// transportFor 返回对应代理的 Transport。
func transportFor(u *url.URL, direct, insecure bool) *http.Transport {
	key := "env"
	switch {
	case direct:
		key = Direct
	case u != nil:
		key = u.String()
	}
	if insecure {
		key += "|insecure"
	}
	transportMu.Lock()
	defer transportMu.Unlock()
	if tr, ok := transports[key]; ok {
		return tr
	}
	if len(transports) >= maxTransports {
		for k, tr := range transports {
			tr.CloseIdleConnections()
			delete(transports, k)
		}
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	switch {
	case direct:
		tr.Proxy = nil
	case u != nil:
		tr.Proxy = http.ProxyURL(u)
	default:
		tr.Proxy = http.ProxyFromEnvironment
	}
	if insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	// 全局开关开启时拒绝访问内网与链路本地地址；开关在每次建立连接时读取，缓存的 Transport 也随之生效
	outbound.Guard(tr)
	transports[key] = tr
	return tr
}

// Client 返回按代理设置访问外部服务的 HTTP 客户端；timeout 为 0 表示不限（由请求的 context 控制）。
// insecure 为 true 时不校验服务端证书。
func Client(setting string, timeout time.Duration, insecure bool) *http.Client {
	u, direct := Resolve(setting)
	return &http.Client{Transport: transportFor(u, direct, insecure), Timeout: timeout}
}

// ---- TCP 连接 ----

// Dial 按代理设置建立到 addr 的 TCP 连接：http、https 代理使用 CONNECT 隧道，socks5 代理由代理解析域名。
// 未设置代理（含 update 而更新代理为空）时直连，不读取环境变量，与此前 SMTP 等直连行为保持一致。
func Dial(ctx context.Context, setting, addr string, timeout time.Duration) (net.Conn, error) {
	d := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	u, _ := Resolve(setting)
	if u == nil {
		// 直连时在拨号层校验解析后的真实 IP（开关关闭时无影响）
		guarded := outbound.NewDialer(timeout)
		return guarded.DialContext(ctx, "tcp", addr)
	}
	// 经代理时连接由代理建立，本机拨号层看不到真实目标，开关开启时先检查目标主机
	if host, _, err := net.SplitHostPort(addr); err == nil {
		if err := outbound.CheckHost(ctx, host); err != nil {
			return nil, err
		}
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	switch u.Scheme {
	case "socks5", "socks5h":
		var auth *xproxy.Auth
		if u.User != nil {
			pass, _ := u.User.Password()
			auth = &xproxy.Auth{User: u.User.Username(), Password: pass}
		}
		sd, err := xproxy.SOCKS5("tcp", hostPort(u), auth, d)
		if err != nil {
			return nil, err
		}
		conn, err := sd.(xproxy.ContextDialer).DialContext(ctx, "tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("经代理 %s 连接失败: %w", u.Redacted(), err)
		}
		return conn, nil
	}
	conn, err := dialConnect(ctx, d, u, addr)
	if err != nil {
		return nil, fmt.Errorf("经代理 %s 连接失败: %w", u.Redacted(), err)
	}
	return conn, nil
}

// hostPort 返回代理的 host:port，未写端口时按协议取默认端口。
func hostPort(u *url.URL) string {
	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "https":
			port = "443"
		case "socks5", "socks5h":
			port = "1080"
		default:
			port = "80"
		}
	}
	return net.JoinHostPort(u.Hostname(), port)
}

// maxConnectResponse 限制 CONNECT 响应头的大小，防止异常代理返回超长内容占用内存
const maxConnectResponse = 16 << 10

// dialConnect 通过 HTTP(S) 代理的 CONNECT 方法建立隧道。
func dialConnect(ctx context.Context, d *net.Dialer, u *url.URL, addr string) (net.Conn, error) {
	// 目标地址会写进请求行与 Host 头，含空白或控制字符时拒绝，避免拼出额外的请求头
	if addr == "" || strings.ContainsFunc(addr, func(r rune) bool { return r <= ' ' || r == 0x7f }) {
		return nil, errors.New("目标地址无效")
	}
	conn, err := d.DialContext(ctx, "tcp", hostPort(u))
	if err != nil {
		return nil, err
	}
	// 握手期间 context 取消或超时时关闭连接，打断阻塞的读写
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	fail := func(err error) (net.Conn, error) {
		stop()
		_ = conn.Close()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	if u.Scheme == "https" {
		tc := tls.Client(conn, &tls.Config{ServerName: u.Hostname()})
		if err := tc.HandshakeContext(ctx); err != nil {
			return fail(err)
		}
		conn = tc
	}
	req := "CONNECT " + addr + " HTTP/1.1\r\nHost: " + addr + "\r\n"
	if u.User != nil {
		pass, _ := u.User.Password()
		req += "Proxy-Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(u.User.Username()+":"+pass)) + "\r\n"
	}
	req += "\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		return fail(err)
	}
	br := bufio.NewReader(&limitedReader{r: conn, n: maxConnectResponse})
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		return fail(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fail(fmt.Errorf("代理拒绝了 CONNECT 请求: %s", resp.Status))
	}
	if !stop() {
		// context 已触发关闭
		return nil, ctx.Err()
	}
	if br.Buffered() > 0 {
		// 代理在响应头之后紧接着转发了目标的数据（如 SMTP 欢迎语），先交给读取方
		buf, _ := br.Peek(br.Buffered())
		return &prefixConn{Conn: conn, prefix: append([]byte(nil), buf...)}, nil
	}
	return conn, nil
}

// limitedReader 只限制 CONNECT 响应阶段的读取量；隧道建立后不再经过它（剩余数据由 prefixConn 转交）。
type limitedReader struct {
	r io.Reader
	n int
}

func (l *limitedReader) Read(p []byte) (int, error) {
	if l.n <= 0 {
		return 0, errors.New("代理响应过长")
	}
	if len(p) > l.n {
		p = p[:l.n]
	}
	n, err := l.r.Read(p)
	l.n -= n
	return n, err
}

// prefixConn 先返回已读入缓冲的数据，再从连接读取。
type prefixConn struct {
	net.Conn
	prefix []byte
}

func (c *prefixConn) Read(p []byte) (int, error) {
	if len(c.prefix) > 0 {
		n := copy(p, c.prefix)
		c.prefix = c.prefix[n:]
		return n, nil
	}
	return c.Conn.Read(p)
}
