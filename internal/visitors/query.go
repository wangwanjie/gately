package visitors

import (
	"cmp"
	"errors"
	"net/netip"
	"slices"
	"strings"
	"time"
)

var (
	errDays = errors.New("访客审计的保留天数需在 1~180 之间")
	errMax  = errors.New("访客审计的最多访客数需在 100~100000 之间")
)

// 列表筛选条件。
const (
	FilterDenied = "denied"
	FilterAuthed = "authed"
	FilterBanned = "banned"
	FilterNoise  = "noise"
	FilterAdmin  = "admin"
)

// 列表排序方式。
const (
	SortLast     = "last"
	SortFirst    = "first"
	SortRequests = "requests"
	SortDenied   = "denied"
)

// MaxLimit 是一次查询最多返回的访客数。
const MaxLimit = 200

// Query 是访客列表的查询条件。
type Query struct {
	// Days 为统计范围：最近 Days 天（含今天），0 表示全部保留期
	Days int
	// Q 为关键词，匹配地址、国家代码、身份、站点、域名、路径与 User-Agent（不区分大小写）
	Q string
	// Filter 为 denied、authed、banned、noise、admin 之一，空表示不筛选
	Filter string
	// Site 为“监听key/站点key”，"-" 表示未匹配站点，空表示不筛选
	Site   string
	Sort   string
	Offset int
	Limit  int
	// Banned 与 Country 由调用方提供，分别判断地址是否在封禁中、查询国家代码，可为 nil
	Banned  func(netip.Addr) bool
	Country func(netip.Addr) string
}

// SiteRef 是列表中的站点摘要。
type SiteRef struct {
	Listener  string `json:"listener"`
	Key       string `json:"key"`
	Name      string `json:"name"`
	Unmatched bool   `json:"unmatched,omitempty"`
	Req       uint64 `json:"req,omitempty"`
}

// Item 是列表中的一个访客。Requests、Denied 为统计范围内的次数，TotalRequests 为保留期内的总数。
type Item struct {
	IP            string      `json:"ip"`
	Country       string      `json:"country,omitempty"`
	Banned        bool        `json:"banned"`
	Noise         bool        `json:"noise"`
	First         int64       `json:"first"`
	Last          int64       `json:"last"`
	Requests      uint64      `json:"requests"`
	Denied        uint64      `json:"denied"`
	TotalRequests uint64      `json:"totalRequests"`
	Sites         []SiteRef   `json:"sites"`
	SiteCount     int         `json:"siteCount"`
	Identities    []IdentStat `json:"identities"`
	Agent         string      `json:"agent,omitempty"`
	LastReq       LastReq     `json:"lastReq"`
}

// Summary 是统计范围内（筛选后）的汇总。New 为首次出现在统计范围内的访客数。
type Summary struct {
	Visitors int    `json:"visitors"`
	Requests uint64 `json:"requests"`
	Denied   uint64 `json:"denied"`
	Authed   int    `json:"authed"`
	Banned   int    `json:"banned"`
	Noise    int    `json:"noise"`
	New      int    `json:"new"`
}

// DayPoint 是统计范围内某一天的汇总，Day 为本地日期 YYYY-MM-DD。
type DayPoint struct {
	Day      string `json:"day"`
	Visitors int    `json:"visitors"`
	Requests uint64 `json:"requests"`
	Denied   uint64 `json:"denied"`
}

// Result 是查询结果。Sites 为全部访客访问过的站点（供筛选下拉）。
type Result struct {
	Items   []Item     `json:"items"`
	Total   int        `json:"total"`
	Summary Summary    `json:"summary"`
	Daily   []DayPoint `json:"daily"`
	Sites   []SiteRef  `json:"sites"`
}

// containsFold 不区分大小写（ASCII）判断 s 是否包含 sub，sub 已转为小写。
func containsFold(s, sub string) bool {
	if sub == "" {
		return true
	}
	n := len(sub)
	for i := 0; i+n <= len(s); i++ {
		if strings.EqualFold(s[i:i+n], sub) {
			return true
		}
	}
	return false
}

// matches 判断访客是否匹配关键词。
func (v *Visitor) matches(q, country string) bool {
	if containsFold(v.IP, q) || (country != "" && containsFold(country, q)) {
		return true
	}
	if containsFold(v.LastReq.Host, q) || containsFold(v.LastReq.Path, q) {
		return true
	}
	for _, id := range v.Identities {
		if containsFold(id.Name, q) {
			return true
		}
	}
	for _, st := range v.Sites {
		if containsFold(st.Name, q) {
			return true
		}
	}
	for _, p := range v.Paths {
		if containsFold(p.Path, q) {
			return true
		}
	}
	for _, a := range v.Agents {
		if containsFold(a.UA, q) {
			return true
		}
	}
	return false
}

func (v *Visitor) hasKind(kind string) bool {
	for _, id := range v.Identities {
		if id.Kind == kind {
			return true
		}
	}
	return false
}

func (v *Visitor) hasSite(site string) bool {
	for _, st := range v.Sites {
		if site == "-" && st.Unmatched {
			return true
		}
		if !st.Unmatched && st.Listener+"/"+st.Key == site {
			return true
		}
	}
	return false
}

// rangeCounts 返回从 fromDay 起的请求数与拦截数。
func (v *Visitor) rangeCounts(fromDay int32) (req, denied uint64) {
	for i := len(v.Days) - 1; i >= 0 && v.Days[i].D >= fromDay; i-- {
		req += uint64(v.Days[i].Req)
		denied += uint64(v.Days[i].Denied)
	}
	return
}

// formatDay 把日期编号格式化为 YYYY-MM-DD。
func formatDay(d int32) string {
	return time.Unix(int64(d)*86400, 0).UTC().Format(time.DateOnly)
}

// queryChunk 为查询时每次持锁复制的访客数：分批持锁，查询再大也不会长时间挡住请求的记录。
const queryChunk = 512

// prefilter 判断访客是否满足只依赖记录本身的筛选条件（站点、拦截、身份、扫描类），在持锁时调用，只读字段不分配内存。
func (v *Visitor) prefilter(q *Query, denied uint64) bool {
	if q.Site != "" && !v.hasSite(q.Site) {
		return false
	}
	switch q.Filter {
	case FilterDenied:
		return denied > 0
	case FilterAuthed:
		return v.hasKind("user")
	case FilterAdmin:
		return v.hasKind("admin")
	case FilterNoise:
		return v.noise()
	}
	return true
}

// siteID 是汇总站点列表时的站点标识；未匹配站点的请求合为一项。
type siteID struct {
	listener, key string
	unmatched     bool
}

// Query 按条件查询访客列表。
//
// 持锁时只做两件事：取全部记录的指针，再分批复制符合基本条件的记录；封禁与国家回调、关键词匹配、
// 汇总与排序都在锁外进行，查询不会阻塞请求的记录。查询串行执行，并发的查询依次等待，
// 避免多个客户端同时轮询时复制出多份全部记录。
func (s *Store) Query(q Query) Result {
	s.queryMu.Lock()
	defer s.queryMu.Unlock()

	type ref struct {
		addr netip.Addr
		v    *Visitor
	}
	s.mu.Lock()
	now := s.now()
	keepDays := s.set.Days
	refs := make([]ref, 0, len(s.m))
	for a, v := range s.m {
		refs = append(refs, ref{a, v})
	}
	s.mu.Unlock()

	today := dayOf(now)
	span := q.Days
	if span <= 0 || span > keepDays {
		span = keepDays
	}
	fromDay := today - int32(span) + 1
	// 统计范围从起始那天的本地 0 点算起，“今天”的新访客即今天 0 点之后首次出现的访客
	fromSec := time.Date(now.Year(), now.Month(), now.Day()-span+1, 0, 0, 0, 0, now.Location()).Unix()
	kw := strings.ToLower(strings.TrimSpace(q.Q))

	type hit struct {
		addr    netip.Addr
		v       Visitor
		req     uint64
		denied  uint64
		banned  bool
		country string
	}
	res := Result{Items: []Item{}, Daily: []DayPoint{}, Sites: []SiteRef{}}
	siteSeen := map[siteID]int{}
	var cands []hit
	for i := 0; i < len(refs); i += queryChunk {
		s.mu.Lock()
		for _, r := range refs[i:min(i+queryChunk, len(refs))] {
			v := r.v
			// 取指针之后被删除或淘汰（之后可能又以新记录出现）的访客跳过
			if s.m[r.addr] != v {
				continue
			}
			for _, st := range v.Sites {
				id := siteID{unmatched: true}
				if !st.Unmatched {
					id = siteID{listener: st.Listener, key: st.Key}
				}
				if j, ok := siteSeen[id]; ok {
					res.Sites[j].Req += uint64(st.Req)
					continue
				}
				siteSeen[id] = len(res.Sites)
				res.Sites = append(res.Sites, SiteRef{Listener: st.Listener, Key: st.Key, Name: st.Name, Unmatched: st.Unmatched, Req: uint64(st.Req)})
			}
			req, denied := v.rangeCounts(fromDay)
			if req == 0 || v.Last < fromSec {
				continue
			}
			if !v.prefilter(&q, denied) {
				continue
			}
			cands = append(cands, hit{addr: r.addr, v: v.clone(), req: req, denied: denied})
		}
		s.mu.Unlock()
	}
	refs = nil

	daily := make([]DayPoint, span)
	for i := range daily {
		daily[i].Day = formatDay(fromDay + int32(i))
	}
	hits := make([]*hit, 0, len(cands))
	for i := range cands {
		h := &cands[i]
		v := &h.v
		h.banned = q.Banned != nil && q.Banned(h.addr)
		if q.Filter == FilterBanned && !h.banned {
			continue
		}
		if q.Country != nil {
			h.country = q.Country(h.addr)
		}
		if kw != "" && !v.matches(kw, h.country) {
			continue
		}
		hits = append(hits, h)
		sm := &res.Summary
		sm.Visitors++
		sm.Requests += h.req
		sm.Denied += h.denied
		if v.hasKind("user") {
			sm.Authed++
		}
		if h.banned {
			sm.Banned++
		}
		if v.noise() {
			sm.Noise++
		}
		if v.First >= fromSec {
			sm.New++
		}
		for j := len(v.Days) - 1; j >= 0 && v.Days[j].D >= fromDay; j-- {
			d := v.Days[j]
			if idx := int(d.D - fromDay); idx < span {
				daily[idx].Visitors++
				daily[idx].Requests += uint64(d.Req)
				daily[idx].Denied += uint64(d.Denied)
			}
		}
	}
	res.Daily = daily
	slices.SortFunc(res.Sites, func(x, y SiteRef) int { return strings.Compare(x.Name, y.Name) })

	slices.SortFunc(hits, func(x, y *hit) int {
		var c int
		switch q.Sort {
		case SortFirst:
			c = cmp.Compare(y.v.First, x.v.First)
		case SortRequests:
			c = cmp.Compare(y.req, x.req)
		case SortDenied:
			c = cmp.Compare(y.denied, x.denied)
		}
		if c == 0 {
			c = cmp.Compare(y.v.Last, x.v.Last)
		}
		if c == 0 {
			c = strings.Compare(x.v.IP, y.v.IP)
		}
		return c
	})
	res.Total = len(hits)
	limit := q.Limit
	if limit <= 0 || limit > MaxLimit {
		limit = 50
	}
	off := max(q.Offset, 0)
	for i := off; i < len(hits) && i < off+limit; i++ {
		h := hits[i]
		res.Items = append(res.Items, h.v.item(h.req, h.denied, h.banned, h.country))
	}
	return res
}

// item 生成列表项：站点按请求数取前 3 个，User-Agent 取请求最多的一个。
func (v *Visitor) item(req, denied uint64, banned bool, country string) Item {
	it := Item{IP: v.IP, Country: country, Banned: banned, Noise: v.noise(), First: v.First, Last: v.Last,
		Requests: req, Denied: denied, TotalRequests: v.Requests, SiteCount: len(v.Sites),
		Identities: slices.Clone(v.Identities), LastReq: v.LastReq}
	sites := slices.Clone(v.Sites)
	slices.SortStableFunc(sites, func(x, y SiteStat) int { return cmp.Compare(y.Req, x.Req) })
	for i := 0; i < len(sites) && i < 3; i++ {
		st := sites[i]
		it.Sites = append(it.Sites, SiteRef{Listener: st.Listener, Key: st.Key, Name: st.Name, Unmatched: st.Unmatched, Req: uint64(st.Req)})
	}
	if it.Sites == nil {
		it.Sites = []SiteRef{}
	}
	if it.Identities == nil {
		it.Identities = []IdentStat{}
	}
	var best *AgentStat
	for i := range v.Agents {
		if best == nil || v.Agents[i].Req > best.Req {
			best = &v.Agents[i]
		}
	}
	if best != nil {
		it.Agent = best.UA
	}
	return it
}

// Detail 是单个访客的完整记录，Days 补齐为保留期内的每一天（YYYY-MM-DD）。
type Detail struct {
	Visitor
	Country string     `json:"country,omitempty"`
	Banned  bool       `json:"banned"`
	Noise   bool       `json:"noiseOnly"`
	Daily   []DayPoint `json:"daily"`
}

// Detail 返回单个访客的完整记录；列表按请求数从多到少排序。
func (s *Store) Detail(a netip.Addr, banned func(netip.Addr) bool, country func(netip.Addr) string) (Detail, bool) {
	s.mu.Lock()
	v, ok := s.m[a.Unmap()]
	if !ok {
		s.mu.Unlock()
		return Detail{}, false
	}
	c := v.clone()
	noise := v.noise()
	span := s.set.Days
	today := dayOf(s.now())
	s.mu.Unlock()

	d := Detail{Visitor: c, Noise: noise}
	if banned != nil {
		d.Banned = banned(a.Unmap())
	}
	if country != nil {
		d.Country = country(a.Unmap())
	}
	from := today - int32(span) + 1
	d.Daily = make([]DayPoint, span)
	for i := range d.Daily {
		d.Daily[i].Day = formatDay(from + int32(i))
	}
	for _, x := range c.Days {
		if idx := int(x.D - from); idx >= 0 && idx < span {
			d.Daily[idx].Visitors = 1
			d.Daily[idx].Requests = uint64(x.Req)
			d.Daily[idx].Denied = uint64(x.Denied)
		}
	}
	slices.SortStableFunc(d.Sites, func(x, y SiteStat) int { return cmp.Compare(y.Req, x.Req) })
	slices.SortStableFunc(d.Paths, func(x, y PathStat) int { return cmp.Compare(y.Req, x.Req) })
	slices.SortStableFunc(d.Identities, func(x, y IdentStat) int { return cmp.Compare(y.Req, x.Req) })
	slices.SortStableFunc(d.Agents, func(x, y AgentStat) int { return cmp.Compare(y.Req, x.Req) })
	slices.SortStableFunc(d.Deny, func(x, y Count) int { return cmp.Compare(y.N, x.N) })
	// 空列表输出 []，界面不必判断 null
	if d.Deny == nil {
		d.Deny = []Count{}
	}
	if d.Sites == nil {
		d.Sites = []SiteStat{}
	}
	if d.Paths == nil {
		d.Paths = []PathStat{}
	}
	if d.Identities == nil {
		d.Identities = []IdentStat{}
	}
	if d.Agents == nil {
		d.Agents = []AgentStat{}
	}
	// 按天计数已展开到 Daily
	d.Days = nil
	return d, true
}
