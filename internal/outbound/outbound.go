// Package outbound 统一管理员配置的出站 HTTP 请求（Webhook、通知、IP 列表订阅、DDNS 回调与查询地址、
// 更新检查与程序下载、第三方登录等）的拨号层限制。
//
// 全局开关开启后，所有经本包构造的客户端在拨号层校验“解析后的真实 IP”，拒绝回环、私网、链路本地
// （含云厂商元数据地址 169.254.169.254）、唯一本地地址（ULA）、运营商级 NAT 等地址。校验发生在
// 每一次建立连接时，因此同时覆盖重定向与 DNS 重绑定：重定向到内网地址的请求在拨号时被拒绝，
// 域名解析结果在校验与连接之间无法被替换。开关默认关闭，保持内网用法不受影响，修改后立即生效。
//
// 反向代理上游、WOL、存储挂载、Docker、端口转发等本身面向内网的功能不经过本包。
package outbound

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// restricted 为全局开关：true 表示禁止出站请求访问内网与链路本地地址。
var restricted atomic.Bool

// SetRestricted 设置全局开关，立即对新建立的连接生效（已建立的连接保持不变）。
func SetRestricted(on bool) {
	restricted.Store(on)
}

// Restricted 返回全局开关当前是否开启。
func Restricted() bool {
	return restricted.Load()
}

// ErrBlocked 表示目标地址属于被禁止的范围。
var ErrBlocked = errors.New("出站请求被禁止访问内网或链路本地地址")

// 除标准库判断之外还需要拒绝的网段
var extraBlocked = mustPrefixes(
	"0.0.0.0/8",     // “本网络”
	"100.64.0.0/10", // 运营商级 NAT（含 Tailscale 等虚拟网）
	"192.0.0.0/24",  // IETF 协议分配
	"198.18.0.0/15", // 基准测试
	"240.0.0.0/4",   // 保留
)

func mustPrefixes(list ...string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(list))
	for _, s := range list {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}

// nat64 是 NAT64 前缀，末 4 字节内嵌 IPv4 地址，需按内嵌地址判断。
var nat64 = netip.MustParsePrefix("64:ff9b::/96")

// IsBlocked 判断 IP 是否属于被禁止的范围：回环、私网、链路本地、唯一本地地址（ULA）、
// 多播、未指定地址与上面列出的特殊网段；IPv4 映射地址与 NAT64 内嵌地址按其 IPv4 判断。
func IsBlocked(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() {
		return true
	}
	if nat64.Contains(ip) {
		b := ip.As16()
		return IsBlocked(netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}))
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	for _, p := range extraBlocked {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// Control 可直接用作 net.Dialer.Control：此时地址已经解析为 IP，开关开启时拒绝被禁止的目标。
func Control(network, address string, _ syscall.RawConn) error {
	if !restricted.Load() {
		return nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrBlocked, address)
	}
	if IsBlocked(ip) {
		return fmt.Errorf("%w: %s", ErrBlocked, ip)
	}
	return nil
}

// NewDialer 返回带校验的拨号器。
func NewDialer(timeout time.Duration) *net.Dialer {
	return &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second, Control: Control}
}

// CheckHost 在经代理访问时预先检查目标主机：连接由代理建立，本机拨号层看不到真实目标，
// 因此开关开启时在这里解析并检查（尽力而为，无法防御代理侧的 DNS 重绑定）。
func CheckHost(ctx context.Context, host string) error {
	if !restricted.Load() || host == "" {
		return nil
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if IsBlocked(ip) {
			return fmt.Errorf("%w: %s", ErrBlocked, ip)
		}
		return nil
	}
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		// 解析失败时交给代理处理
		return nil
	}
	for _, a := range addrs {
		if IsBlocked(a) {
			return fmt.Errorf("%w: %s (%s)", ErrBlocked, host, a.Unmap())
		}
	}
	return nil
}

// Guard 给 Transport 加上限制并返回它本身：直连时在拨号层校验解析后的 IP；
// 经代理时代理自身的地址不受限（由管理员配置），目标主机在选择代理时预先检查。
func Guard(tr *http.Transport) *http.Transport {
	var proxies sync.Map
	if fn := tr.Proxy; fn != nil {
		tr.Proxy = func(req *http.Request) (*url.URL, error) {
			u, err := fn(req)
			if u != nil {
				if err := CheckHost(req.Context(), req.URL.Hostname()); err != nil {
					return nil, err
				}
				proxies.Store(ProxyAddr(u), true)
			}
			return u, err
		}
	}
	guarded := NewDialer(30 * time.Second)
	plain := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if _, ok := proxies.Load(addr); ok {
			return plain.DialContext(ctx, network, addr)
		}
		return guarded.DialContext(ctx, network, addr)
	}
	return tr
}

// ProxyAddr 返回连接代理 u 时使用的 host:port，未写端口时按协议取默认端口。
func ProxyAddr(u *url.URL) string {
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

// NewTransport 返回克隆自默认 Transport 并带限制的新 Transport，代理遵循环境变量。
func NewTransport() *http.Transport {
	return Guard(http.DefaultTransport.(*http.Transport).Clone())
}

// ProxyTransport 返回经指定代理访问的、带限制的新 Transport。
func ProxyTransport(proxy *url.URL) *http.Transport {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = http.ProxyURL(proxy)
	return Guard(tr)
}

// NewClient 返回带限制的 HTTP 客户端（代理遵循环境变量）；timeout 为 0 表示不限。
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{Transport: NewTransport(), Timeout: timeout}
}

var (
	defaultOnce   sync.Once
	defaultClient *http.Client
)

// DefaultClient 返回进程内共享的带限制客户端，用来替代 http.DefaultClient。
func DefaultClient() *http.Client {
	defaultOnce.Do(func() {
		defaultClient = NewClient(0)
	})
	return defaultClient
}

// Wrap 返回 c 的副本，其 Transport 为带限制的克隆；c 为 nil 时等同 NewClient(0)，c 不会被修改。
func Wrap(c *http.Client) *http.Client {
	out := &http.Client{}
	if c != nil {
		*out = *c
	}
	switch tr := out.Transport.(type) {
	case nil:
		out.Transport = NewTransport()
	case *http.Transport:
		out.Transport = Guard(tr.Clone())
	}
	// 其他自定义 RoundTripper 无法安全改造，保持原样
	return out
}
