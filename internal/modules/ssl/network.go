package ssl

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/miekg/dns"
)

// 本文件实现签发证书时限定 IPv4 / IPv6 出站（ACMEConfig.Network）。
// 主机有 IPv6 地址但 IPv6 不通时，系统会优先尝试 IPv6：HTTP 请求还能在超时后回退到 IPv4，
// lego 检查 TXT 生效时对权威服务器发出的 UDP 查询却只会用第一个地址，一直超时导致签发失败。

// networkName 返回网络类型的显示名称。
func networkName(network string) string {
	if network == NetworkIPv6 {
		return "IPv6"
	}
	return "IPv4"
}

// forceNetwork 返回只通过 network（tcp4/tcp6）建立连接的 HTTP 客户端，保留 c 的超时与 TLS 等设置；c 不会被修改。
func forceNetwork(c *http.Client, network string) *http.Client {
	out := &http.Client{}
	if c != nil {
		*out = *c
	}
	tr, ok := out.Transport.(*http.Transport)
	if !ok || tr == nil {
		tr = http.DefaultTransport.(*http.Transport)
	}
	tr = tr.Clone()
	// 经代理（如 HTTPS_PROXY）访问时，到目标的连接由代理建立，本机只连代理；
	// 代理地址按自身情况选择地址族，不受 network 限制，否则代理只有另一种地址时会连不上
	var proxies sync.Map
	if proxyFn := tr.Proxy; proxyFn != nil {
		tr.Proxy = func(req *http.Request) (*url.URL, error) {
			u, err := proxyFn(req)
			if u != nil {
				proxies.Store(proxyAddr(u), true)
			}
			return u, err
		}
	}
	d := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	tr.DialContext = func(ctx context.Context, _, addr string) (net.Conn, error) {
		if _, ok := proxies.Load(addr); ok {
			return d.DialContext(ctx, "tcp", addr)
		}
		return d.DialContext(ctx, network, addr)
	}
	out.Transport = tr
	return out
}

// proxyAddr 返回 Transport 连接代理 u 时使用的 host:port（未写端口时按协议取默认端口）。
func proxyAddr(u *url.URL) string {
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

// lookupNetIP 按地址族解析主机名，测试中替换。
var lookupNetIP = net.DefaultResolver.LookupNetIP

// filterNameservers 保留与 network 同一地址族的 DNS 服务器（host:port）。
// 主机名形式的服务器先解析为该地址族的 IP：lego 的 CNAME 跟随与区域查询用 "udp" 发出，
// 主机名可能被解析到不通的另一地址族，直到超时；解析不到该地址族地址的服务器丢弃。
func filterNameservers(servers []string, network string) []string {
	family := "ip4"
	if network == NetworkIPv6 {
		family = "ip6"
	}
	var out []string
	for _, s := range servers {
		host, port, err := net.SplitHostPort(s)
		if err != nil {
			host, port = s, "53"
		}
		ip, err := netip.ParseAddr(host)
		if err != nil {
			ctx, cancel := context.WithTimeout(context.Background(), dnsQueryTimeout)
			ips, err := lookupNetIP(ctx, family, host)
			cancel()
			if err != nil {
				continue
			}
			for _, a := range ips {
				if a.Unmap().Is4() == (network == NetworkIPv4) {
					out = append(out, net.JoinHostPort(a.Unmap().String(), port))
				}
			}
			continue
		}
		if ip.Unmap().Is4() == (network == NetworkIPv4) {
			out = append(out, s)
		}
	}
	return out
}

// netPreCheck 在限定网络下检查 TXT 是否生效，规则与 lego 默认检查一致：
// 先查递归 DNS（跟随 CNAME），再要求全部权威服务器都已返回验证值（关闭权威检查时直接通过）。
type netPreCheck struct {
	network string
	// nameservers 为已按地址族过滤的递归 DNS（host:port）
	nameservers []string
	// authoritative 为是否查询权威服务器
	authoritative bool
	// noCNAME 为 true 时不跟随递归 DNS 返回的 CNAME，直接检查 fqdn 本身
	noCNAME bool
}

// authoritativePort 是权威服务器的 DNS 端口，测试中替换。
var authoritativePort = "53"

// dnsQueryTimeout 是单次 DNS 查询超时。
var dnsQueryTimeout = 10 * time.Second

// exchange 用限定网络向 server 发出一次查询，响应被截断时改用 TCP 重试。
func (p netPreCheck) exchange(m *dns.Msg, server string) (*dns.Msg, error) {
	// 未限定网络时由系统选择地址族
	udp, tcp := "udp", "tcp"
	switch p.network {
	case NetworkIPv4:
		udp, tcp = "udp4", "tcp4"
	case NetworkIPv6:
		udp, tcp = "udp6", "tcp6"
	}
	r, _, err := (&dns.Client{Net: udp, Timeout: dnsQueryTimeout}).Exchange(m, server)
	if err == nil && r != nil && r.Truncated {
		r, _, err = (&dns.Client{Net: tcp, Timeout: dnsQueryTimeout}).Exchange(m, server)
	}
	if err != nil {
		return nil, fmt.Errorf("查询 %s 失败: %w", server, err)
	}
	return r, nil
}

// query 依次向 servers 查询，返回第一个成功的响应。
func (p netPreCheck) query(name string, qtype uint16, servers []string, recursive bool) (*dns.Msg, error) {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), qtype)
	m.SetEdns0(4096, false)
	m.RecursionDesired = recursive
	var errs []error
	for _, s := range servers {
		r, err := p.exchange(m, s)
		if err == nil {
			return r, nil
		}
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		return nil, errors.New("没有可用的 DNS 服务器")
	}
	return nil, errors.Join(errs...)
}

// nsAddrs 通过递归 DNS 解析权威服务器在当前地址族下的地址。
// 查询失败时返回错误，由调用方稍后重试；确实没有该地址族的地址时返回空列表。
func (p netPreCheck) nsAddrs(host string) ([]string, error) {
	// 未限定网络时 IPv4 与 IPv6 地址都用，IPv4 在前
	qtypes := []uint16{dns.TypeA, dns.TypeAAAA}
	switch p.network {
	case NetworkIPv4:
		qtypes = []uint16{dns.TypeA}
	case NetworkIPv6:
		qtypes = []uint16{dns.TypeAAAA}
	}
	var out []string
	for _, qtype := range qtypes {
		r, err := p.query(host, qtype, p.nameservers, true)
		if err == nil && r.Rcode != dns.RcodeSuccess {
			err = fmt.Errorf("解析 %s 返回 %s", host, dns.RcodeToString[r.Rcode])
		}
		if err != nil {
			// 未限定网络时已有 IPv4 地址就不因 IPv6 查询失败而中断
			if p.network == "" && len(out) > 0 {
				continue
			}
			return nil, err
		}
		for _, rr := range r.Answer {
			switch v := rr.(type) {
			case *dns.A:
				out = append(out, net.JoinHostPort(v.A.String(), authoritativePort))
			case *dns.AAAA:
				out = append(out, net.JoinHostPort(v.AAAA.String(), authoritativePort))
			}
		}
	}
	return out, nil
}

// hasTXT 判断响应中是否有值为 value 的 TXT 记录。
func hasTXT(r *dns.Msg, value string) bool {
	for _, rr := range r.Answer {
		if txt, ok := rr.(*dns.TXT); ok && strings.Join(txt.Txt, "") == value {
			return true
		}
	}
	return false
}

// check 实现 dns01.WrapPreCheckFunc，替换 lego 默认的生效检查。
func (p netPreCheck) check(_, fqdn, value string, _ dns01.PreCheckFunc) (bool, error) {
	if len(p.nameservers) == 0 {
		return false, fmt.Errorf("检查 TXT 生效所用的 DNS 服务器中没有 %s 地址，请在「检查用 DNS」中填写 %s 的 DNS 服务器", networkName(p.network), networkName(p.network))
	}
	r, err := p.query(fqdn, dns.TypeTXT, p.nameservers, true)
	if err != nil {
		return false, fmt.Errorf("递归 DNS: %w", err)
	}
	// 与 lego 一致：验证记录是 CNAME 时检查 CNAME 指向的记录（决定不跟随 CNAME 时除外）
	if !p.noCNAME {
		for _, rr := range r.Answer {
			if cn, ok := rr.(*dns.CNAME); ok && strings.EqualFold(cn.Hdr.Name, fqdn) {
				fqdn = cn.Target
			}
		}
	}
	if !p.authoritative {
		return true, nil
	}
	zone, err := dns01.FindZoneByFqdnCustom(fqdn, p.nameservers)
	if err != nil {
		return false, fmt.Errorf("查询 %s 的权威区域失败: %w", fqdn, err)
	}
	r, err = p.query(zone, dns.TypeNS, p.nameservers, true)
	if err != nil {
		return false, fmt.Errorf("查询 %s 的权威服务器失败: %w", zone, err)
	}
	var hosts []string
	for _, rr := range r.Answer {
		if ns, ok := rr.(*dns.NS); ok {
			hosts = append(hosts, strings.ToLower(ns.Ns))
		}
	}
	if len(hosts) == 0 {
		return false, fmt.Errorf("没有查到 %s 的权威服务器", zone)
	}
	checked := 0
	for _, host := range hosts {
		addrs, err := p.nsAddrs(host)
		if err != nil {
			return false, fmt.Errorf("权威服务器 %s: %w", host, err)
		}
		// 没有当前地址族地址的权威服务器无法访问，跳过
		if len(addrs) == 0 {
			continue
		}
		checked++
		r, err := p.query(fqdn, dns.TypeTXT, addrs, false)
		if err != nil {
			return false, fmt.Errorf("权威服务器 %s: %w", host, err)
		}
		if r.Rcode != dns.RcodeSuccess {
			return false, fmt.Errorf("权威服务器 %s 对 %s 返回 %s", host, fqdn, dns.RcodeToString[r.Rcode])
		}
		if !hasTXT(r, value) {
			return false, nil
		}
	}
	if checked == 0 {
		return false, fmt.Errorf("%s 的权威服务器都没有 %s 地址，可勾选「只查询上面的 DNS，不查询权威服务器」或改用其他网络", zone, networkName(p.network))
	}
	return true, nil
}
