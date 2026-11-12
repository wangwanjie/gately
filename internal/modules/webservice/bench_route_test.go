package webservice

import (
	"fmt"
	"testing"
)

// benchRouteRunner 构造含 n 个站点的监听器：每个站点一个精确域名，另有 n/10 个通配域名站点。
func benchRouteRunner(n int) *listenerRunner {
	t := &siteTable{exact: map[string]*siteHandler{}}
	for i := 0; i < n; i++ {
		d := fmt.Sprintf("app%d.example.com", i)
		if i%10 == 0 {
			d = fmt.Sprintf("*.grp%d.example.com", i)
		}
		h := &siteHandler{site: Site{Key: fmt.Sprintf("s%d", i), Domains: []string{d}}}
		t.sites = append(t.sites, h)
		t.indexDomains(h)
	}
	t.wildReady = true
	t.deflt = &siteHandler{deflt: true}
	l := &listenerRunner{}
	l.table.Store(t)
	return l
}

// BenchmarkRouteExact 精确域名命中索引。
func BenchmarkRouteExact(b *testing.B) {
	l := benchRouteRunner(500)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if l.route("app77.example.com") == nil {
			b.Fatal("应命中站点")
		}
	}
}

// BenchmarkRouteWildcard 访问通配域名站点：精确索引未命中，需要逐个比对。
func BenchmarkRouteWildcard(b *testing.B) {
	l := benchRouteRunner(500)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if l.route("x.grp40.example.com") == nil {
			b.Fatal("应命中站点")
		}
	}
}

// BenchmarkRouteMiss 扫描器用随机 Host 访问：精确未命中、通配也不匹配，最后落到默认站点。
func BenchmarkRouteMiss(b *testing.B) {
	l := benchRouteRunner(500)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if l.route("scanner.invalid") == nil {
			b.Fatal("应落到默认站点")
		}
	}
}

// TestRouteIndexedMatchesLinear 验证按通配列表选站点与逐个比对全部域名的结果一致（含重复域名、多种通配与全部匹配）。
func TestRouteIndexedMatchesLinear(t *testing.T) {
	l := benchRouteRunner(200)
	tbl := l.current()
	extra := []string{"*", "*.example.com", "*.a.grp10.example.com", "api-*.example.com", "app5.example.com"}
	for i, d := range extra {
		h := &siteHandler{site: Site{Key: fmt.Sprintf("x%d", i), Domains: []string{d}}}
		tbl.sites = append(tbl.sites, h)
		tbl.indexDomains(h)
	}
	legacy := &siteTable{sites: tbl.sites, deflt: tbl.deflt, exact: tbl.exact}
	ll := &listenerRunner{}
	ll.table.Store(legacy)
	hosts := []string{"app5.example.com", "app6.example.com", "x.grp10.example.com", "z.a.grp10.example.com", "api-v1.example.com",
		"foo.example.com", "scanner.invalid", "grp10.example.com", "deep.x.grp20.example.com", "1.2.3.4"}
	for _, h := range hosts {
		got, want := l.route(h), ll.route(h)
		if got != want {
			t.Errorf("%s: 索引路由 %v 与逐个比对 %v 不一致", h, got.site.Key, want.site.Key)
		}
	}
}
