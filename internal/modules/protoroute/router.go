package protoroute

import (
	"slices"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/wangwanjie/gately/internal/l4proxy"
)

// target 是一个分流目标（某条路由或默认目标）运行时的状态。
type target struct {
	name   string
	picker *l4proxy.Picker
	// port 为目标端口，0 表示按规则的端口映射（默认目标）或与监听端口相同（路由）
	port          int
	proxyProtocol string
	// match 为路由匹配条件的标识（见 routeMatchKey），热更新时据此沿用旧计数；默认目标为空
	match string
	// conns 为该目标累计分到的连接数。热更新时新旧 target 共享同一个计数器，
	// 仍在使用旧路由表的连接计入后也不会丢失
	conns *atomic.Int64
}

// router 按识别出的协议选择目标。TLS 与 HTTP 路由按域名（SNI 或 Host）匹配，其余协议每种至多一条路由。
type router struct {
	// domains 为 TLS 与 HTTP 各自的域名路由表
	domains map[string]*domainTable
	// others 为不按域名匹配的协议对应的路由，包括等待超时
	others map[string]*target
	// fallback 为默认目标，未命中任何路由或无法识别协议时使用；可为 nil
	fallback *target
	// targets 与规则的 Routes 一一对应，用于统计
	targets []*target
}

// domainTable 是一种协议按域名匹配的路由表：精确域名优先，其次最长的 *.后缀 通配，再次 *，最后是未填域名的路由。
// 每个域名下可以有按 ALPN 区分的多条路由与一条不限 ALPN 的路由。
type domainTable struct {
	exact    map[string]*alpnBucket
	wildcard []wildcardRoute
	// any 为域名 "*"，匹配任何携带域名的连接
	any *alpnBucket
	// all 为未填写域名的路由，匹配该协议的全部连接（包括没有域名的）
	all *alpnBucket
}

type wildcardRoute struct {
	// suffix 为通配去掉 * 后的部分，例如 ".example.com"
	suffix string
	bucket *alpnBucket
}

// alpnBucket 是同一个域名下的路由：byALPN 为限定 ALPN 的路由，generic 为不限 ALPN 的路由。
type alpnBucket struct {
	byALPN  map[string]*target
	generic *target
}

// pick 按客户端声明 ALPN 的先后顺序找限定 ALPN 的路由，都不匹配时使用不限 ALPN 的路由。
func (b *alpnBucket) pick(alpn []string) *target {
	if b == nil {
		return nil
	}
	for _, p := range alpn {
		if t, ok := b.byALPN[p]; ok {
			return t
		}
	}
	return b.generic
}

func (b *alpnBucket) add(t *target, alpn []string) {
	if len(alpn) == 0 {
		b.generic = t
		return
	}
	if b.byALPN == nil {
		b.byALPN = map[string]*target{}
	}
	for _, p := range alpn {
		b.byALPN[p] = t
	}
}

// bucket 返回域名对应的路由桶，不存在时创建。
func (d *domainTable) bucket(domain string) *alpnBucket {
	switch {
	case domain == "":
		if d.all == nil {
			d.all = &alpnBucket{}
		}
		return d.all
	case domain == "*":
		if d.any == nil {
			d.any = &alpnBucket{}
		}
		return d.any
	case strings.HasPrefix(domain, "*."):
		suffix := domain[1:]
		for _, w := range d.wildcard {
			if w.suffix == suffix {
				return w.bucket
			}
		}
		b := &alpnBucket{}
		d.wildcard = append(d.wildcard, wildcardRoute{suffix: suffix, bucket: b})
		return b
	}
	b, ok := d.exact[domain]
	if !ok {
		b = &alpnBucket{}
		d.exact[domain] = b
	}
	return b
}

// match 按域名从具体到宽泛逐级查找；某一级只有限定 ALPN 的路由且 ALPN 不匹配时继续查找下一级。
func (d *domainTable) match(host string, alpn []string) *target {
	if d == nil {
		return nil
	}
	if host != "" {
		if t := d.exact[host].pick(alpn); t != nil {
			return t
		}
		for _, w := range d.wildcard {
			if strings.HasSuffix(host, w.suffix) {
				if t := w.bucket.pick(alpn); t != nil {
					return t
				}
			}
		}
		if t := d.any.pick(alpn); t != nil {
			return t
		}
	}
	return d.all.pick(alpn)
}

// routeProtocol 返回路由的协议，旧配置中为空的按 TLS 处理。
func routeProtocol(r Route) string {
	if r.Protocol == "" {
		return ProtoTLS
	}
	return r.Protocol
}

// routeMatchKey 返回路由匹配条件的标识：协议、域名集合与 ALPN 集合，与书写顺序无关。
// 名称、目标地址、目标端口、PROXY 协议只决定连接去哪里，不影响哪些连接会命中这条路由，因此不参与标识。
func routeMatchKey(r Route) string {
	domains := slices.Clone(r.Domains)
	for i, d := range domains {
		domains[i] = strings.ToLower(d)
	}
	slices.Sort(domains)
	alpn := slices.Clone(r.ALPN)
	slices.Sort(alpn)
	return strings.ToLower(routeProtocol(r)) + "|" + strings.Join(domains, ",") + "|" + strings.Join(alpn, ",")
}

// inheritCounts 让热更新后的路由表沿用旧路由表的命中计数（共享同一个计数器），规则如下：
//   - 路由按匹配条件（协议、域名集合、ALPN 集合，见 routeMatchKey）对应，条件不变的路由继续累计，
//     与它的位置、名称、目标地址、目标端口、PROXY 协议是否改动无关，因此重排路由、改目标都不会清零；
//   - 新增的路由与改了协议、域名或 ALPN 的路由视为新路由，从 0 开始；删除的路由的计数随之丢弃；
//   - 未经校验的配置可能有多条匹配条件相同的路由，按出现的先后一一对应；
//   - 默认目标只要新旧规则都设置了默认目标就继续累计（改默认目标地址不清零），去掉默认目标后计数丢弃。
func (rt *router) inheritCounts(prev *router) {
	if prev == nil {
		return
	}
	old := map[string][]*atomic.Int64{}
	for _, t := range prev.targets {
		old[t.match] = append(old[t.match], t.conns)
	}
	for _, t := range rt.targets {
		if list := old[t.match]; len(list) > 0 {
			t.conns = list[0]
			old[t.match] = list[1:]
		}
	}
	if rt.fallback != nil && prev.fallback != nil {
		rt.fallback.conns = prev.fallback.conns
	}
}

func newRouter(rule Rule) *router {
	rt := &router{domains: map[string]*domainTable{}, others: map[string]*target{}}
	for _, route := range rule.Routes {
		t := &target{
			name:          route.Name,
			picker:        l4proxy.NewPicker(route.TargetAddresses),
			port:          route.TargetPort,
			proxyProtocol: route.ProxyProtocol,
			match:         routeMatchKey(route),
			conns:         new(atomic.Int64),
		}
		// 停用的路由保留在 targets 中，让统计与路由一一对应、重新启用后沿用原计数，但不加入匹配表
		rt.targets = append(rt.targets, t)
		if route.Disabled {
			continue
		}
		proto := routeProtocol(route)
		if !protocolUsesDomains(proto) {
			// 校验保证同一协议只有一条路由；未经校验的配置以第一条为准
			if _, ok := rt.others[proto]; !ok {
				rt.others[proto] = t
			}
			continue
		}
		table := rt.domains[proto]
		if table == nil {
			table = &domainTable{exact: map[string]*alpnBucket{}}
			rt.domains[proto] = table
		}
		domains := route.Domains
		if len(domains) == 0 {
			domains = []string{""}
		}
		for _, d := range domains {
			table.bucket(d).add(t, route.ALPN)
		}
	}
	// 后缀越长越具体，优先匹配
	for _, table := range rt.domains {
		sort.SliceStable(table.wildcard, func(i, j int) bool {
			return len(table.wildcard[i].suffix) > len(table.wildcard[j].suffix)
		})
	}
	if len(rule.TargetAddresses) > 0 {
		rt.fallback = &target{
			name:          "",
			picker:        l4proxy.NewPicker(rule.TargetAddresses),
			proxyProtocol: rule.ProxyProtocol,
			conns:         new(atomic.Int64),
		}
	}
	return rt
}

// match 返回识别结果对应的目标，未命中任何路由时返回默认目标（可能为 nil）。
func (rt *router) match(res sniffResult) *target {
	var t *target
	switch {
	case res.Proto == "":
	case protocolUsesDomains(res.Proto):
		t = rt.domains[res.Proto].match(res.Host, res.ALPN)
	default:
		t = rt.others[res.Proto]
	}
	if t == nil {
		t = rt.fallback
	}
	return t
}
