package ddns

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/resolver"
)

// selfCheckLookupTimeout 是单个域名解析的超时；本机 DNS 偶尔超时或丢包，超时太短会误报“无法解析”。
const selfCheckLookupTimeout = 8 * time.Second

// selfCheckWorkers 是同时解析的域名数量；记录多时逐个解析会耗尽自检的总时限。
const selfCheckWorkers = 8

// lookupFunc 解析域名，network 为 ip4 或 ip6。
type lookupFunc func(ctx context.Context, network, host string) ([]net.IP, error)

// systemLookup 使用本机（或设置中自定义的）DNS 解析，测试可替换。
var systemLookup lookupFunc = func(ctx context.Context, network, host string) ([]net.IP, error) {
	return net.DefaultResolver.LookupIP(ctx, network, host)
}

// publicDNSServers 是复查用的公共 DNS：本机 DNS 超时、缓存了旧结果或返回内网地址时，以公共 DNS 的结果为准。
var publicDNSServers = []resolver.Server{
	{Addr: "223.5.5.5:53"},
	{Addr: "119.29.29.29:53"},
	{Addr: "1.1.1.1:53"},
	{Addr: "8.8.8.8:53"},
}

// publicLookup 同时向各公共 DNS 查询，取第一个解析出地址的结果，测试可替换。
// 单个服务器偶尔返回 SERVFAIL，依次尝试的解析器会把它当作答复而不再问下一个，所以这里分别查询。
var publicLookup lookupFunc = func(ctx context.Context, network, host string) ([]net.IP, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch := make(chan lookupResult, len(publicDNSServers))
	for _, srv := range publicDNSServers {
		go func() {
			addrs, err := resolver.New([]resolver.Server{srv}).LookupIP(ctx, network, host)
			ch <- lookupResult{addrs, err}
		}()
	}
	var firstErr, notFoundErr error
	for range publicDNSServers {
		r := <-ch
		if r.ok() {
			return r.addrs, nil
		}
		if r.err != nil && notFound(r.err) {
			notFoundErr = r.err
		}
		if firstErr == nil {
			firstErr = r.err
		}
	}
	// 有服务器明确答复记录不存在时以它为准，否则返回任一错误
	if notFoundErr != nil {
		return nil, notFoundErr
	}
	return nil, firstErr
}

// lookupResult 是一次解析的结果。
type lookupResult struct {
	addrs []net.IP
	err   error
}

// has 判断解析结果中是否包含期望的地址。
func (r lookupResult) has(want string) bool {
	w, err := netip.ParseAddr(want)
	if err != nil {
		return false
	}
	for _, a := range r.addrs {
		if ip, ok := netip.AddrFromSlice(a); ok && ip.Unmap() == w.Unmap() {
			return true
		}
	}
	return false
}

func (r lookupResult) ok() bool {
	return r.err == nil && len(r.addrs) > 0
}

// SelfCheck 实现 module.SelfChecker：检查使用动态地址变量的记录，其域名当前解析结果是否等于本机出口公网地址。
func (m *Module) SelfCheck(ctx context.Context, env module.CheckEnv) []module.CheckResult {
	type target struct{ fqdn, typ, want string }
	var targets []target
	seen := map[string]bool{}
	m.mu.Lock()
	for _, t := range m.cfg.Tasks {
		if !t.Enable {
			continue
		}
		for _, rec := range t.Records {
			if !rec.Enable {
				continue
			}
			fqdn := rec.Zone
			if rec.Sub != "" && rec.Sub != "@" {
				fqdn = rec.Sub + "." + rec.Zone
			}
			var tg target
			switch {
			case rec.Type == "A" && strings.Contains(rec.Value, "{ipv4Addr}") && env.PublicIPv4 != "":
				tg = target{fqdn, "ip4", env.PublicIPv4}
			case rec.Type == "AAAA" && strings.Contains(rec.Value, "{ipv6Addr}") && env.PublicIPv6 != "":
				tg = target{fqdn, "ip6", env.PublicIPv6}
			default:
				continue
			}
			key := strings.ToLower(tg.fqdn) + "|" + tg.typ
			if seen[key] {
				continue
			}
			seen[key] = true
			targets = append(targets, tg)
		}
	}
	m.mu.Unlock()

	// 多个域名并发解析，结果按记录顺序输出
	out := make([]module.CheckResult, len(targets))
	sem := make(chan struct{}, selfCheckWorkers)
	var wg sync.WaitGroup
	for i, tg := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out[i] = checkRecord(ctx, tg.fqdn, tg.typ, tg.want)
		}()
	}
	wg.Wait()
	return out
}

// checkRecord 检查一个域名的解析结果。先用本机 DNS，同时向公共 DNS 复查：
// 本机 DNS 解析到期望地址时直接通过；否则只要公共 DNS 解析到期望地址也算通过，
// 避免本机 DNS 偶发超时、缓存了旧记录或做了内网解析时误报。
func checkRecord(ctx context.Context, fqdn, typ, want string) module.CheckResult {
	lctx, cancel := context.WithTimeout(ctx, selfCheckLookupTimeout)
	defer cancel()

	// 本机 DNS 已解析到期望地址时直接返回，公共 DNS 的查询随 lctx 取消而结束（结果写入带缓冲的通道，不会阻塞）
	pubCh := make(chan lookupResult, 1)
	lookupPub := publicLookup
	go func() {
		addrs, err := lookupPub(lctx, typ, fqdn)
		pubCh <- lookupResult{addrs, err}
	}()
	addrs, err := systemLookup(lctx, typ, fqdn)
	sys := lookupResult{addrs, err}

	params := map[string]string{"domain": fqdn, "expected": want}
	if sys.ok() && sys.has(want) {
		params["resolved"] = joinIPs(sys.addrs)
		return module.CheckResult{ID: "ddns.match", Status: module.CheckOK, Params: params}
	}
	pub := <-pubCh
	switch {
	case pub.ok() && pub.has(want):
		params["resolved"] = joinIPs(pub.addrs)
		return module.CheckResult{ID: "ddns.match", Status: module.CheckOK, Params: params}
	case sys.ok():
		params["resolved"] = joinIPs(sys.addrs)
		return module.CheckResult{ID: "ddns.mismatch", Status: module.CheckWarn, Params: params}
	case pub.ok():
		params["resolved"] = joinIPs(pub.addrs)
		return module.CheckResult{ID: "ddns.mismatch", Status: module.CheckWarn, Params: params}
	case notFound(sys.err) || notFound(pub.err):
		// 有 DNS 明确答复记录不存在
		return module.CheckResult{ID: "ddns.unresolved", Status: module.CheckWarn, Params: params}
	default:
		// 本机 DNS 与公共 DNS 都没有给出答复（超时、拒绝等），无法判断记录是否存在
		params["error"] = lookupErrText(sys.err)
		return module.CheckResult{ID: "ddns.lookupFailed", Status: module.CheckWarn, Params: params}
	}
}

// notFound 判断解析失败是否因为记录不存在（NXDOMAIN 或没有该类型的记录）。
func notFound(err error) bool {
	if err == nil {
		// 没有错误但也没有地址，同样视为没有记录
		return true
	}
	var de *net.DNSError
	return errors.As(err, &de) && de.IsNotFound
}

// lookupErrText 返回解析错误的说明。
func lookupErrText(err error) string {
	if err == nil {
		return ""
	}
	var de *net.DNSError
	if errors.As(err, &de) {
		if de.IsTimeout {
			return "timeout"
		}
		return de.Err
	}
	return err.Error()
}

func joinIPs(addrs []net.IP) string {
	s := make([]string, len(addrs))
	for i, a := range addrs {
		s[i] = a.String()
	}
	return strings.Join(s, ", ")
}
