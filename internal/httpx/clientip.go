package httpx

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// 管理端口常放在本机反向代理（nginx、haproxy、Gately Web 服务）之后。此时 TCP 对端是代理自己，
// 真实客户端地址只能从代理写入的请求头中取得。只有对端是可信代理时才读取这些请求头，
// 否则任何人都能伪造来源地址，绕过"仅内网访问"与登录限流。

// proxyTrust 是当前生效的代理信任配置。
type proxyTrust struct {
	enable bool
	extra  []netip.Prefix
}

var trust atomic.Pointer[proxyTrust]

func init() {
	trust.Store(&proxyTrust{enable: true})
}

// SetProxyTrust 设置是否信任本机代理传来的客户端地址，extra 为额外信任的代理地址段。
func SetProxyTrust(enable bool, extra []netip.Prefix) {
	trust.Store(&proxyTrust{enable: enable, extra: extra})
}

// ParsePrefixes 把 IP 或 CIDR 列表解析为地址段，单个 IP 视为 /32 或 /128。
func ParsePrefixes(list []string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, s := range list {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if p, err := netip.ParsePrefix(s); err == nil {
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(strings.Trim(s, "[]"))
		if err != nil {
			return nil, err
		}
		a = a.Unmap()
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}

// PeerIP 返回 TCP 对端地址，不读取任何请求头。
func PeerIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return strings.Trim(host, "[]")
}

// ClientIP 返回请求的客户端地址。对端为可信代理时，从 X-Forwarded-For（自右向左跳过可信代理）与 X-Real-IP 中取得真实地址；
// 否则返回对端地址。
//
// 两个请求头只有一个时直接使用。两个都有且不一致时，说明其中一个是客户端伪造后被代理原样转发的：
// 只追加 X-Forwarded-For 的代理（haproxy option forwardfor、Caddy 等）会原样转发客户端自带的 X-Real-IP，
// 只写 X-Real-IP 的代理（只配置了 proxy_set_header X-Real-IP 的 nginx）会原样转发客户端自带的 X-Forwarded-For。
// 单看一次请求无法分辨是哪一种，因此：
//   - 一个是内网地址、另一个是公网地址时取公网地址，伪造内网地址无法绕过“仅内网访问”；
//   - 其余情况以 X-Forwarded-For 为准，它的最右一项由最近的代理追加，客户端无法改写。
func ClientIP(r *http.Request) string {
	peer := PeerIP(r)
	t := trust.Load()
	if !t.enable || !trustedProxy(peer, t) {
		return peer
	}
	realIP := ""
	if v := strings.TrimSpace(r.Header.Get("X-Real-IP")); v != "" {
		if a, err := netip.ParseAddr(strings.Trim(v, "[]")); err == nil {
			realIP = a.Unmap().String()
		}
	}
	fwd, ok := forwardedClient(r, t)
	if !ok {
		// X-Forwarded-For 格式异常时不再采信它，有合法的 X-Real-IP 就用，否则保守地返回对端地址
		if realIP != "" {
			return realIP
		}
		return peer
	}
	switch {
	case fwd == "" && realIP == "":
		return peer
	case fwd == "":
		return realIP
	case realIP == "" || realIP == fwd:
		return fwd
	case isInternal(fwd) && !isInternal(realIP):
		// X-Forwarded-For 给出内网地址而 X-Real-IP 是公网地址：按伪造内网地址处理，取公网地址
		return realIP
	}
	return fwd
}

// forwardedClient 从 X-Forwarded-For 自右向左跳过可信代理，返回第一个不可信的地址；
// 没有该请求头或全部是可信代理时返回空串，格式异常时 ok 为 false。
func forwardedClient(r *http.Request, t *proxyTrust) (ip string, ok bool) {
	// X-Forwarded-For 最右侧由最近的代理追加；继续向左只跳过同样可信的代理，第一个不可信的地址即客户端
	var hops []string
	for _, h := range r.Header.Values("X-Forwarded-For") {
		for _, part := range strings.Split(h, ",") {
			if part = strings.TrimSpace(part); part != "" {
				hops = append(hops, part)
			}
		}
	}
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.Trim(hops[i], "[]"))
		if err != nil {
			return "", false
		}
		s := a.Unmap().String()
		if !trustedProxy(s, t) {
			return s, true
		}
	}
	return "", true
}

// isInternal 判断地址是否为内网地址（回环、私有、链路本地、CGNAT 与 IPv6 ULA），与“仅内网访问”的判定一致。
func isInternal(ip string) bool {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	a = a.Unmap()
	if a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() {
		return true
	}
	return cgnat.Contains(a)
}

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// trustedProxy 判断地址是否为可信代理：回环地址、本机网卡地址或额外配置的地址段。
func trustedProxy(ip string, t *proxyTrust) bool {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	a = a.Unmap()
	if a.IsLoopback() {
		return true
	}
	for _, p := range t.extra {
		if p.Contains(a) {
			return true
		}
	}
	return localAddrs().has(a)
}

// 本机地址缓存：网卡地址会变化（DHCP、IPv6 前缀轮换），定期刷新。
type addrSet struct {
	at    time.Time
	addrs map[netip.Addr]bool
}

func (s *addrSet) has(a netip.Addr) bool {
	return s != nil && s.addrs[a]
}

var (
	localMu    sync.Mutex
	localCache *addrSet
	// interfaceAddrs 允许测试替换本机地址来源
	interfaceAddrs = net.InterfaceAddrs
)

const localAddrTTL = 30 * time.Second

func localAddrs() *addrSet {
	localMu.Lock()
	defer localMu.Unlock()
	if localCache != nil && time.Since(localCache.at) < localAddrTTL {
		return localCache
	}
	set := &addrSet{at: time.Now(), addrs: map[netip.Addr]bool{}}
	if addrs, err := interfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				if ip, ok := netip.AddrFromSlice(ipn.IP); ok {
					set.addrs[ip.Unmap()] = true
				}
			}
		}
	}
	localCache = set
	return set
}

// viaKey 是请求上下文中“经由的代理名称”的键。
type viaKey struct{}

// WithVia 标记请求经由指定的代理转发（例如飞牛统一网关）。这类代理给出的客户端地址无法核实，
// 审计日志据此显示来源，避免把转发来的地址当作已验证的地址。
func WithVia(r *http.Request, via string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), viaKey{}, via))
}

// ViaOf 返回 WithVia 标记的代理名称，未标记时为空。
func ViaOf(r *http.Request) string {
	v, _ := r.Context().Value(viaKey{}).(string)
	return v
}
