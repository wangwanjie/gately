package visitors

import (
	"cmp"
	"fmt"
	"math/rand/v2"
	"net/netip"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// queryReference 是改为“持锁只取标量、分页后再生成列表项”之前的实现（逐字保留），对照测试以它为准。
//
// 持锁时只做两件事：取全部记录的指针，再分批复制符合基本条件的记录；封禁与国家回调、关键词匹配、
// 汇总与排序都在锁外进行，查询不会阻塞请求的记录。查询串行执行，并发的查询依次等待，
// 避免多个客户端同时轮询时复制出多份全部记录。
func (s *Store) queryReference(q Query) Result {
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

// randomStore 造一批访客：不同站点（含未匹配）、身份（站点用户、管理员）、拦截、路径、User-Agent，时间分布在 40 天内。
func randomStore(t testing.TB, n int, seedN uint64) (*Store, *time.Time) {
	now := base
	s := New("", nil)
	s.now = func() time.Time { return now }
	rng := rand.New(rand.NewPCG(seedN, 7))
	sites := []struct{ l, k, name string }{{"l1", "web", "Web"}, {"l1", "blog", "博客"}, {"l2", "nas", "NAS"}, {"l2", "pic", "Photos"}}
	paths := []string{"/", "/home", "/api/items", "/wp-login.php", "/.env", "/admin/Settings", "/upload", "/static/app.js"}
	uas := []string{"Mozilla/5.0 Firefox/130", "Mozilla/5.0 Chrome/130 Safari", "curl/8.4.0", "sqlmap/1.7", "Go-http-client/1.1"}
	users := []string{"alice", "Bob", "carol"}
	denies := []string{"ip", "waf", "ban", "country", "rateLimit"}
	for i := range n {
		var a netip.Addr
		if i%5 == 0 {
			a = netip.AddrFrom16([16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, byte(i >> 8), byte(i)})
		} else {
			a = netip.AddrFrom4([4]byte{byte(10 + i%200), byte(i >> 16), byte(i >> 8), byte(i)})
		}
		hits := 1 + rng.IntN(6)
		for range hits {
			h := Hit{Addr: a, Time: base.Add(-time.Duration(rng.IntN(40*24)) * time.Hour).Add(-time.Duration(rng.IntN(3600)) * time.Second),
				Method: "GET", Path: paths[rng.IntN(len(paths))], UA: uas[rng.IntN(len(uas))], Status: 200, Host: fmt.Sprintf("h%d.example.com", rng.IntN(5))}
			if rng.IntN(8) == 0 {
				h.Listener, h.Unmatched, h.Status = "l1", true, 404
			} else {
				st := sites[rng.IntN(len(sites))]
				h.Listener, h.Site, h.SiteName = st.l, st.k, st.name
			}
			switch rng.IntN(10) {
			case 0, 1:
				h.Deny, h.Status = denies[rng.IntN(len(denies))], 403
			case 2:
				h.User = users[rng.IntN(len(users))]
			case 3:
				h.Admin = "root"
			}
			s.Record(h)
		}
	}
	return s, &now
}

// 新实现与原实现对同样的数据和条件返回逐条相同的结果（列表项、总数、汇总、按天曲线、站点列表）。
func TestQueryMatchesReference(t *testing.T) {
	s, _ := randomStore(t, 1000, 1)
	banned := func(a netip.Addr) bool { return a.Is4() && a.As4()[3]%7 == 0 }
	country := func(a netip.Addr) string {
		switch {
		case a.Is6():
			return ""
		case a.As4()[0]%3 == 0:
			return "CN"
		case a.As4()[0]%3 == 1:
			return "JP"
		}
		return "US"
	}
	kws := []string{"", "10.", "2001:db8", "cn", "JP", "alice", "BOB", "root", "博客", "photos", "wp-login", "/ADMIN/settings", "firefox", "sqlmap", "h3.example", "nomatch-xyz"}
	filters := []string{"", FilterDenied, FilterAuthed, FilterBanned, FilterNoise, FilterAdmin}
	siteSel := []string{"", "-", "l1/web", "l2/nas", "l9/none"}
	sorts := []string{"", SortLast, SortFirst, SortRequests, SortDenied}
	pages := [][2]int{{0, 0}, {0, 200}, {37, 25}, {100000, 50}, {-5, 500}}
	n := 0
	check := func(q Query) {
		t.Helper()
		got, want := s.Query(q), s.queryReference(q)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("结果不同: %+v\n新: total=%d items=%d %+v\n原: total=%d items=%d %+v", q, got.Total, len(got.Items), got.Summary, want.Total, len(want.Items), want.Summary)
		}
		n++
	}
	days := []int{0, 1, 7, 30}
	// 筛选、站点、排序与分页的全部组合（不带关键词）
	for _, d := range days {
		for _, f := range filters {
			for _, site := range siteSel {
				for _, so := range sorts {
					for _, p := range pages {
						check(Query{Days: d, Filter: f, Site: site, Sort: so, Offset: p[0], Limit: p[1], Banned: banned, Country: country})
					}
				}
			}
		}
	}
	// 关键词与筛选、排序的组合
	for _, d := range days {
		for _, kw := range kws {
			for i, f := range filters {
				check(Query{Days: d, Q: kw, Filter: f, Sort: sorts[i%len(sorts)], Site: siteSel[i%len(siteSel)], Limit: 200, Banned: banned, Country: country})
			}
		}
	}
	// 回调为 nil 时同样一致
	for _, kw := range []string{"", "cn", "alice"} {
		q := Query{Q: kw, Sort: SortDenied}
		if got, want := s.Query(q), s.queryReference(q); !reflect.DeepEqual(got, want) {
			t.Fatalf("无回调时结果不同: %q", kw)
		}
	}
	t.Logf("对照 %d 组查询", n)
}

// 列表查询只为当前页生成列表项，分配远小于克隆全部候选记录（1 万名访客时原实现约 20 MB）。
func TestQueryAllocations(t *testing.T) {
	s, _ := randomStore(t, 10000, 2)
	q := Query{Days: 30, Limit: 50}
	s.Query(q)
	var a0, a1 runtimeAlloc
	a0.read()
	s.Query(q)
	a1.read()
	got := a1.bytes - a0.bytes
	a0.read()
	s.queryReference(q)
	a1.read()
	ref := a1.bytes - a0.bytes
	t.Logf("1 万名访客单次查询分配：新 %d KB，原 %d KB", got>>10, ref>>10)
	if got > ref/4 {
		t.Fatalf("分配应明显减少: 新 %d，原 %d", got, ref)
	}
}

func BenchmarkQuery10k(b *testing.B) {
	s, _ := randomStore(b, 10000, 2)
	for _, c := range []struct {
		name string
		q    Query
	}{{"default", Query{Days: 30, Limit: 50}}, {"keyword", Query{Days: 30, Q: "firefox", Limit: 50}}, {"denied", Query{Filter: FilterDenied, Sort: SortDenied, Limit: 50}}} {
		b.Run("new/"+c.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				s.Query(c.q)
			}
		})
		b.Run("old/"+c.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				s.queryReference(c.q)
			}
		})
	}
}
