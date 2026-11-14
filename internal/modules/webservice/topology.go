package webservice

import (
	"net/http"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/httpx"
)

// Web 服务拓扑视图：互联网 → Gately → 监听 → 站点 / 监听级动作 → 后端。
//
// 接口只读取内存中的计数快照，前端每 2 秒轮询一次。节点编号：
//   - 监听 l:<监听 key>
//   - 站点 s:<监听 key>/<站点 key>，默认站点的站点 key 为 default
//   - 后端 u:<站点编号去掉 s: 前缀>/<序号>，序号为站点全部后端组（站点、分路径、按用户分流）展开后的位置
//   - 监听级动作 a:<监听 key>/redirect（明文跳转 HTTPS）、a:<监听 key>/notfound（未匹配站点返回 404）
//
// 速率口径：qps、bps 为最近 10 个完整秒的均值，qps60 为最近 60 秒均值；累计数为监听（站点）启动以来的值。
// 活动 IP 为最近 60 秒内在某个站点或监听级动作上出现过的访客地址，加上仍有进行中请求的地址。

const (
	// topoMaxIPs 为一次返回的活动 IP 上限（按进行中请求数、最后出现时间排序），超出时 ipTruncated 为 true
	topoMaxIPs = 500
	// topoMaxDomains 为每个站点返回的域名上限
	topoMaxDomains = 50
)

// topoMaxAggIPs 为汇总活动 IP 时最多处理的地址数（测试可调小）。每个节点各自最多记 1024 个近期地址，
// 大量不同来源（如逐个换用一个 IPv6 /64 内的地址）时节点数 × 1024 的汇总、排序会让每次轮询耗时上百毫秒。
// 超出后只再收录有进行中请求的地址与正在筛选的地址，并标记 ipTruncated
var topoMaxAggIPs = 5000

// TopologyView 是拓扑接口的输出。
type TopologyView struct {
	// Time 为快照时刻（Unix 秒），Window 为 qps / bps 取均值的秒数
	Time   int64 `json:"time"`
	Window int   `json:"window"`
	// Enabled 为 Web 服务模块是否启用
	Enabled bool `json:"enabled"`
	// Requests、Active、Rate 为整个实例（全部监听）的汇总
	Requests  int64          `json:"requests"`
	Active    int64          `json:"active"`
	Rate      Rate           `json:"rate"`
	Listeners []TopoListener `json:"listeners"`
	// IPs 为活动 IP，IPTotal 为去重后的总数；IPTruncated 表示列表超出上限或某个站点的近期地址表已满而不完整
	IPs         []TopoIP `json:"ips"`
	IPTotal     int      `json:"ipTotal"`
	IPTruncated bool     `json:"ipTruncated"`
	// Focus 为请求的 focus 参数，Series 为该节点（或 internet / gately 表示整个实例）近 60 秒每秒的请求数与字节数
	Focus  string      `json:"focus,omitempty"`
	Series []RatePoint `json:"series,omitempty"`
	// SeriesStep 为曲线每个点覆盖的秒数，实时快照为 1（省略），回放快照为采样间隔
	SeriesStep int `json:"seriesStep,omitempty"`
	// Replay 只在回放快照中出现，给出这一帧在录制数据中的位置
	Replay *ReplayMeta `json:"replay,omitempty"`
}

// TopoListener 是一个监听。
type TopoListener struct {
	ID       string `json:"id"`
	Key      string `json:"key"`
	Name     string `json:"name"`
	Enable   bool   `json:"enable"`
	Running  bool   `json:"running"`
	Error    string `json:"error,omitempty"`
	Network  string `json:"network"`
	ListenIP string `json:"listenIp"`
	Port     int    `json:"port"`
	TLS      bool   `json:"tls"`
	HTTP3    bool   `json:"http3"`
	// PlainHTTP 为 HTTPS 端口收到明文请求时的处理方式（见 Listener.PlainHTTP）
	PlainHTTP string `json:"plainHttp"`
	// HTTPPort、HTTPPortMode 为额外的明文 HTTP 端口及其处理方式（见 Listener.HTTPPort）
	HTTPPort      int    `json:"httpPort,omitempty"`
	HTTPPortMode  string `json:"httpPortMode,omitempty"`
	ProxyProtocol bool   `json:"proxyProtocol"`
	// Requests、Active、Rate 为该监听全部站点与监听级动作的汇总
	Requests int64        `json:"requests"`
	Active   int64        `json:"active"`
	Rate     Rate         `json:"rate"`
	Sites    []TopoSite   `json:"sites"`
	Actions  []TopoAction `json:"actions"`
}

// TopoAction 是在监听层处理、不经过站点的一类请求。
type TopoAction struct {
	ID string `json:"id"`
	// Kind 为 redirect（明文请求跳转到 HTTPS）或 notFound（未匹配任何站点且未启用默认站点，返回 404）
	Kind     string `json:"kind"`
	Requests int64  `json:"requests"`
	Rate     Rate   `json:"rate"`
}

// TopoSite 是一个站点（含默认站点）。
type TopoSite struct {
	ID      string   `json:"id"`
	Key     string   `json:"key"`
	Name    string   `json:"name"`
	Group   string   `json:"group,omitempty"`
	Type    string   `json:"type"`
	Domains []string `json:"domains"`
	Default bool     `json:"default,omitempty"`
	Enable  bool     `json:"enable"`
	// Running 表示站点正在所属监听上生效（监听运行中且站点已启用）
	Running bool `json:"running"`
	// Target 为非代理站点的去向：跳转地址、静态目录或固定内容的状态码
	Target string `json:"target,omitempty"`
	// Guards 为站点启用的防护：auth、waf、ip、country、rateLimit、denyClose
	Guards []string `json:"guards"`
	// AccessLog 表示站点开启了访问日志，拓扑页据此提示按站点查看日志是否会有内容
	AccessLog bool             `json:"accessLog"`
	Requests  int64            `json:"requests"`
	Active    int64            `json:"active"`
	Errors    int64            `json:"errors"`
	Closed    int64            `json:"closed"`
	Rate      Rate             `json:"rate"`
	Status    map[string]int64 `json:"status"`
	Blocked   map[string]int64 `json:"blocked"`
	// Upstreams 为反向代理站点的后端（站点后端、分路径后端、按用户分流后端依次展开）
	Upstreams []TopoUpstream `json:"upstreams"`
}

// TopoUpstream 是一个后端。
type TopoUpstream struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Scope  string `json:"scope,omitempty"`
	Match  string `json:"match,omitempty"`
	Weight int    `json:"weight"`
	Backup bool   `json:"backup,omitempty"`
	// Healthy 在站点未运行时省略（状态未知）
	Healthy  *bool `json:"healthy,omitempty"`
	Conns    int64 `json:"conns"`
	Requests int64 `json:"requests"`
	Bytes    int64 `json:"bytes"`
	Rate     Rate  `json:"rate"`
}

// TopoIP 是一个活动的访客地址。
type TopoIP struct {
	IP string `json:"ip"`
	// Nodes 为该地址近期到达过的站点、监听级动作与后端（含进行中请求所在的后端）的节点编号
	Nodes []string `json:"nodes"`
	// Active 为该地址当前进行中的请求总数，ActiveAt 为按站点与后端节点分的进行中请求数，Last 为最后出现的时间（Unix 秒）
	Active   int            `json:"active"`
	ActiveAt map[string]int `json:"activeAt,omitempty"`
	Last     int64          `json:"last"`
}

// topoSiteRef 记录一个运行中站点对应的处理器，在释放模块锁后再读取它的计数。
type topoSiteRef struct {
	li, si int
	h      *siteHandler
	// ups 为与 TopoSite.Upstreams 一一对应的后端
	ups []topoUp
}

type topoUp struct {
	u            *upstream
	scope, match string
}

func siteGuards(s Site) []string {
	g := []string{}
	if s.Type == TypeClose {
		return g
	}
	if s.BasicAuth || s.WebAuth || s.ForwardAuth.URL != "" {
		g = append(g, "auth")
	}
	if s.WAF != "" {
		g = append(g, "waf")
	}
	if s.IPMode != IPModeOff || len(s.IPDenyList) > 0 || len(s.IPDenySets) > 0 {
		g = append(g, "ip")
	}
	if s.CountryMode != CountryOff && len(s.Countries) > 0 {
		g = append(g, "country")
	}
	if s.RateLimit.Enable {
		g = append(g, "rateLimit")
	}
	if s.DenyClose {
		g = append(g, "denyClose")
	}
	return g
}

func siteTarget(s Site) string {
	switch s.Type {
	case TypeRedirect:
		return clip(s.RedirectTo, 256)
	case TypeStatic:
		return clip(s.Root, 256)
	case TypeText:
		return strconv.Itoa(s.StatusCode)
	}
	return ""
}

// configUpstreams 列出未运行站点在配置中的后端（无运行状态）。
func configUpstreams(s Site) []TopoUpstream {
	// 非反向代理站点只有按用户分流的后端（#79），断开连接站点没有后端
	if s.Type == TypeClose || (s.Type != TypeProxy && len(s.UserRoutes) == 0) {
		return []TopoUpstream{}
	}
	out := []TopoUpstream{}
	add := func(targets []string, scope, match string) {
		for _, t := range targets {
			spec, err := parseUpstream(t)
			if err != nil {
				continue
			}
			u := &upstream{url: spec.URL}
			out = append(out, TopoUpstream{URL: u.displayURL(), Scope: scope, Match: match, Weight: spec.Weight, Backup: spec.Backup})
		}
	}
	if s.Type == TypeProxy {
		add(s.Targets, "", "")
	}
	for _, l := range s.Locations {
		if len(l.Targets) > 0 {
			add(l.Targets, scopeLocation, l.Prefix)
		}
	}
	for _, u := range s.UserRoutes {
		add(u.Targets, scopeUser, strings.Join(u.Users, ","))
	}
	return out
}

// newTopoSite 生成站点节点的配置部分。
func newTopoSite(lk string, s Site, deflt bool) TopoSite {
	key := s.Key
	if deflt {
		key = "default"
	}
	domains := s.Domains
	if len(domains) > topoMaxDomains {
		domains = domains[:topoMaxDomains]
	}
	return TopoSite{
		ID: "s:" + lk + "/" + key, Key: key, Name: s.Name, Group: s.Group, Type: s.Type,
		Domains: append([]string{}, domains...), Default: deflt, Enable: s.Enable,
		Target: siteTarget(s), Guards: siteGuards(s), AccessLog: s.AccessLog,
		Status: map[string]int64{}, Blocked: map[string]int64{}, Upstreams: []TopoUpstream{},
	}
}

func (m *Module) handleTopology(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	focus := clip(q.Get("focus"), 512)
	var wantIP netip.Addr
	if s := strings.TrimSpace(q.Get("ip")); s != "" {
		a, err := netip.ParseAddr(s)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "IP 地址无效")
			return
		}
		wantIP = a.Unmap()
	}
	httpx.OK(w, m.topology(time.Now().Unix(), focus, wantIP))
}

// topology 生成拓扑快照。focus 为需要返回曲线的节点编号；wantIP 有效时保证该地址出现在活动 IP 列表中（只要它是活动的）。
func (m *Module) topology(now int64, focus string, wantIP netip.Addr) TopologyView {
	v := TopologyView{Time: now, Window: rateAvgWindow, Listeners: []TopoListener{}, IPs: []TopoIP{}, Focus: focus}
	var refs []topoSiteRef
	runners := map[int]*listenerRunner{}

	// 持有模块锁时只复制配置与处理器指针，计数在释放锁后读取（各计数自带锁或为原子量）
	m.mu.Lock()
	v.Enabled = m.cfg.Settings.Enable
	for li, l := range m.cfg.Listeners {
		tl := TopoListener{
			ID: "l:" + l.Key, Key: l.Key, Name: l.Name, Enable: l.Enable, Error: m.errors[l.Key],
			Network: l.Network, ListenIP: l.ListenIP, Port: l.Port, TLS: l.TLS, HTTP3: l.HTTP3,
			PlainHTTP: l.PlainHTTP, HTTPPort: l.HTTPPort, HTTPPortMode: l.HTTPPortMode, ProxyProtocol: l.AcceptProxyProtocol,
			Sites: []TopoSite{}, Actions: []TopoAction{},
		}
		run := m.runners[l.Key]
		var handlers map[string]*siteHandler
		var deflt *siteHandler
		if run != nil {
			tl.Running = true
			runners[li] = run
			t := run.current()
			handlers = make(map[string]*siteHandler, len(t.sites))
			for _, h := range t.sites {
				handlers[h.site.Key] = h
			}
			deflt = t.deflt
		}
		addSite := func(s Site, h *siteHandler, isDefault bool) {
			ts := newTopoSite(l.Key, s, isDefault)
			if h == nil {
				ts.Upstreams = configUpstreams(s)
				for i := range ts.Upstreams {
					ts.Upstreams[i].ID = "u:" + strings.TrimPrefix(ts.ID, "s:") + "/" + strconv.Itoa(i)
				}
				tl.Sites = append(tl.Sites, ts)
				return
			}
			ts.Running = true
			ref := topoSiteRef{li: li, si: len(tl.Sites), h: h}
			for _, p := range h.pools() {
				for _, u := range p.ups {
					ref.ups = append(ref.ups, topoUp{u: u, scope: p.scope, match: p.match})
				}
			}
			tl.Sites = append(tl.Sites, ts)
			refs = append(refs, ref)
		}
		for _, s := range l.Sites {
			addSite(s, handlers[s.Key], false)
		}
		if l.Default.Enable {
			addSite(l.Default, deflt, true)
		}
		if run != nil {
			if l.TLS && (l.PlainHTTP == PlainHTTPRedirect || (l.HTTPPort > 0 && l.HTTPPortMode == PlainHTTPRedirect)) {
				tl.Actions = append(tl.Actions, TopoAction{ID: "a:" + l.Key + "/redirect", Kind: "redirect"})
			}
			if deflt == nil {
				tl.Actions = append(tl.Actions, TopoAction{ID: "a:" + l.Key + "/notfound", Kind: "notFound"})
			}
		}
		v.Listeners = append(v.Listeners, tl)
	}
	m.mu.Unlock()

	// 曲线：后端节点只取自身；站点与监听级动作节点取自身，并计入所属监听与整个实例（internet / gately）的汇总。
	// 请求从站点到后端是同一批请求，汇总时不能再加后端，否则会重复计数
	var focusListener string
	if strings.HasPrefix(focus, "l:") {
		focusListener = strings.TrimPrefix(focus, "l:")
	}
	addFocus := func(id, lk string, ring *rateRing) {
		if focus == "" {
			return
		}
		if focus == id || focus == "internet" || focus == "gately" || (focusListener != "" && focusListener == lk) {
			v.Series = addSeries(v.Series, ring.series(now, rateSeriesSeconds))
		}
	}

	// 活动 IP：地址 → 节点集合
	type ipAgg struct {
		nodes    map[string]bool
		active   int
		activeAt map[string]int
		last     int64
	}
	ips := map[netip.Addr]*ipAgg{}
	// agg 返回地址的汇总项；达到 topoMaxAggIPs 后，除非 force（有进行中请求）或是正在筛选的地址，否则返回 nil 不再收录
	agg := func(a netip.Addr, force bool) *ipAgg {
		x := ips[a]
		if x == nil {
			if len(ips) >= topoMaxAggIPs && !force && a != wantIP {
				v.IPTruncated = true
				return nil
			}
			x = &ipAgg{nodes: map[string]bool{}}
			ips[a] = x
		}
		return x
	}

	for _, ref := range refs {
		ts := &v.Listeners[ref.li].Sites[ref.si]
		h := ref.h
		ts.Requests = h.stats.Requests.Load()
		ts.Active = h.stats.Active.Load()
		ts.Errors = h.stats.Errors.Load()
		ts.Closed = h.conns.closed.Load()
		ts.Rate = h.metrics.rate.rate(now)
		ts.Blocked = h.stats.blocked()
		_, st := h.metrics.totals()
		for c := 2; c <= 5; c++ {
			ts.Status[strconv.Itoa(c)+"xx"] = st[c]
		}
		addFocus(ts.ID, v.Listeners[ref.li].Key, &h.metrics.rate)
		upIDs := map[*upstream]string{}
		for i, x := range ref.ups {
			u := x.u
			id := "u:" + strings.TrimPrefix(ts.ID, "s:") + "/" + strconv.Itoa(i)
			upIDs[u] = id
			healthy := !u.down.Load()
			ts.Upstreams = append(ts.Upstreams, TopoUpstream{ID: id, URL: u.displayURL(), Scope: x.scope, Match: x.match,
				Weight: u.weight, Backup: u.backup, Healthy: &healthy, Conns: u.conns.Load(),
				Requests: u.stats.requests.Load(), Bytes: u.stats.bytes.Load(), Rate: u.stats.rate.rate(now)})
			if focus == id {
				v.Series = addSeries(v.Series, u.stats.rate.series(now, rateSeriesSeconds))
			}
			if u.stats.recent.each(now, func(a netip.Addr, last int64) {
				if x := agg(a, false); x != nil {
					x.nodes[id] = true
				}
			}) {
				v.IPTruncated = true
			}
		}
		if h.metrics.recent.each(now, func(a netip.Addr, last int64) {
			x := agg(a, false)
			if x == nil {
				return
			}
			x.nodes[ts.ID] = true
			if last > x.last {
				x.last = last
			}
		}) {
			v.IPTruncated = true
		}
		for _, a := range h.conns.active() {
			// 经 unix socket 等进来的请求没有有效的客户端地址
			if !a.client.Addr().IsValid() {
				continue
			}
			x := agg(a.client.Addr(), true)
			if x.activeAt == nil {
				x.activeAt = map[string]int{}
			}
			x.nodes[ts.ID] = true
			x.active++
			x.activeAt[ts.ID]++
			if up := a.upstream.Load(); up != nil {
				if id, ok := upIDs[up]; ok {
					x.nodes[id] = true
					x.activeAt[id]++
				}
			}
			if s := a.start.Unix(); s > x.last {
				x.last = s
			}
		}
	}

	// 监听级动作与汇总
	for li := range v.Listeners {
		tl := &v.Listeners[li]
		run := runners[li]
		for ai := range tl.Actions {
			ta := &tl.Actions[ai]
			act := &run.acts.notFound
			if ta.Kind == "redirect" {
				act = &run.acts.redirect
			}
			ta.Requests = act.total.Load()
			ta.Rate = act.rate.rate(now)
			addFocus(ta.ID, tl.Key, &act.rate)
			if act.recent.each(now, func(a netip.Addr, last int64) {
				x := agg(a, false)
				if x == nil {
					return
				}
				x.nodes[ta.ID] = true
				if last > x.last {
					x.last = last
				}
			}) {
				v.IPTruncated = true
			}
			tl.Requests += ta.Requests
			tl.Rate.addRate(ta.Rate)
		}
		for _, ts := range tl.Sites {
			tl.Requests += ts.Requests
			tl.Active += ts.Active
			tl.Rate.addRate(ts.Rate)
		}
		v.Requests += tl.Requests
		v.Active += tl.Active
		v.Rate.addRate(tl.Rate)
	}

	// 活动 IP 排序：进行中请求多的在前，其次最近出现的在前，最后按地址
	list := make([]TopoIP, 0, len(ips))
	for a, x := range ips {
		t := TopoIP{IP: a.String(), Active: x.active, ActiveAt: x.activeAt, Last: x.last, Nodes: make([]string, 0, len(x.nodes))}
		for n := range x.nodes {
			t.Nodes = append(t.Nodes, n)
		}
		sort.Strings(t.Nodes)
		list = append(list, t)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Active != list[j].Active {
			return list[i].Active > list[j].Active
		}
		if list[i].Last != list[j].Last {
			return list[i].Last > list[j].Last
		}
		return list[i].IP < list[j].IP
	})
	v.IPTotal = len(list)
	if len(list) > topoMaxIPs {
		v.IPTruncated = true
		keep := list[:topoMaxIPs]
		// 正在筛选的地址排在上限之外时也保留，筛选不会因为列表截断而失效
		if wantIP.IsValid() {
			ws := wantIP.String()
			found := false
			for _, t := range keep {
				if t.IP == ws {
					found = true
					break
				}
			}
			if !found {
				for _, t := range list[topoMaxIPs:] {
					if t.IP == ws {
						keep = append(keep, t)
						break
					}
				}
			}
		}
		list = keep
	}
	v.IPs = list
	return v
}
