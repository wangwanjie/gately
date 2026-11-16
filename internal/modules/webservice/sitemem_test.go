package webservice

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"runtime"
	"sync"
	"testing"
	"time"
)

// 没有流量的站点不分配分钟环与连接历史：两者合计约 132 KB，200 个站点时占堆一半以上。
func TestSiteStatsLazyAlloc(t *testing.T) {
	m := newSiteMetrics()
	tr := newConnTracker()
	if m.ring != nil || tr.hist != nil {
		t.Fatal("新建的统计不应预先分配分钟环与连接历史")
	}
	now := time.Now()
	v := m.view(5, now)
	if len(v.Series) != 5 || v.Series[4].Requests != 0 || v.Series[4].Time != now.Unix()/60*60 {
		t.Fatalf("未分配时曲线应为 5 个零点: %+v", v.Series)
	}
	if s := tr.snapshot("", now); len(s.History) != 0 {
		t.Fatalf("未分配时历史应为空: %+v", s.History)
	}

	// 首个请求后分钟环分配，计数正确
	m.record(netip.MustParseAddr("192.0.2.1"), 200, 10, -1, now)
	m.record(netip.MustParseAddr("192.0.2.1"), 503, 5, -1, now)
	if m.ring == nil {
		t.Fatal("登记请求后应分配分钟环")
	}
	v = m.view(5, now)
	if last := v.Series[4]; last.Requests != 2 || last.Bytes != 15 || last.S5xx != 1 {
		t.Fatalf("分钟计数错误: %+v", last)
	}
	// 短请求不进历史，历史仍不分配
	a := fakeReq(t, tr, "192.0.2.1:1000")
	tr.end(a)
	if tr.hist != nil {
		t.Fatal("短请求不应触发连接历史分配")
	}
	// 被断开的请求进入历史，首次写入时分配
	a = fakeReq(t, tr, "192.0.2.2:1000")
	if !tr.closeConn(a.id) {
		t.Fatal("断开失败")
	}
	tr.end(a)
	if s := tr.snapshot("", time.Now()); len(s.History) != 1 || s.History[0].IP != "192.0.2.2" || s.History[0].Result != "closed" {
		t.Fatalf("历史记录错误: %+v", s.History)
	}

	// 200 个空站点的统计合计远小于原先的 200 × 132 KB
	var before, after runtime.MemStats
	keep := make([]any, 0, 400)
	runtime.GC()
	runtime.ReadMemStats(&before)
	for range 200 {
		keep = append(keep, newSiteMetrics(), newConnTracker())
	}
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(keep)
	if d := after.TotalAlloc - before.TotalAlloc; d > 200*8<<10 {
		t.Fatalf("200 个空站点的统计分配了 %d 字节，应在 1.6 MB 以内", d)
	}
}

// 来源排行达到上限后按计数淘汰到一半：高频来源保留，新来源仍能进入排行（原先满了以后新来源一律不计）。
func TestSiteMetricsTopIPsPrune(t *testing.T) {
	m := newSiteMetrics()
	now := time.Now()
	addr := func(i int) netip.Addr {
		return netip.AddrFrom4([4]byte{10, byte(i >> 16), byte(i >> 8), byte(i)})
	}
	// 前 10 个来源各 100 次，其余各 1 次，填满上限
	for i := range topKeysCap {
		n := 1
		if i < 10 {
			n = 100
		}
		for range n {
			m.record(addr(i), 200, 0, -1, now)
		}
	}
	if len(m.ips) != topKeysCap {
		t.Fatalf("应正好填满: %d", len(m.ips))
	}
	// 一个新来源大量访问：触发淘汰后计入，并能进入排行第一
	hot := netip.MustParseAddr("198.51.100.7")
	for range 500 {
		m.record(hot, 200, 0, -1, now)
	}
	if len(m.ips) != topKeysCap/2+1 {
		t.Fatalf("淘汰后应保留一半再加新来源: %d", len(m.ips))
	}
	v := m.view(1, now)
	if v.TopIPs[0].Key != "198.51.100.7" || v.TopIPs[0].Count != 500 {
		t.Fatalf("新的高频来源应排第一: %+v", v.TopIPs[:2])
	}
	for i := range 10 {
		if m.ips[limitKeyAddr(addr(i))] != 100 {
			t.Fatalf("高频来源 %v 应保留原计数", addr(i))
		}
	}
	// 长期运行中不断出现新来源时表的大小始终有上限
	for i := range 3 * topKeysCap {
		m.record(netip.AddrFrom4([4]byte{172, byte(i >> 16), byte(i >> 8), byte(i)}), 200, 0, -1, now)
	}
	if len(m.ips) > topKeysCap {
		t.Fatalf("来源表超过上限: %d", len(m.ips))
	}
	if m.ips[limitKeyAddr(hot)] != 500 {
		t.Fatal("高频来源不应被长尾来源挤掉")
	}
}

// connCounter 统计后端收到的连接：新建与关闭次数。
type connCounter struct {
	mu            sync.Mutex
	opened, close int
}

func (c *connCounter) hook(_ net.Conn, st http.ConnState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch st {
	case http.StateNew:
		c.opened++
	case http.StateClosed:
		c.close++
	}
}

func (c *connCounter) get() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.opened, c.close
}

// 保存配置（热更新）时：未改动的站点直接沿用统计对象与后端连接池，空闲的后端连接继续复用；
// 只有后端连接设置变了的站点关闭旧连接池的空闲连接。
func TestReloadReusesStatsAndIdleConns(t *testing.T) {
	var ca, cb connCounter
	newBackend := func(c *connCounter) *httptest.Server {
		s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, "ok")
		}))
		s.Config.ConnState = c.hook
		s.Start()
		return s
	}
	ba, bb := newBackend(&ca), newBackend(&cb)
	defer ba.Close()
	defer bb.Close()
	l := Listener{Key: "l1", Sites: []Site{
		{Enable: true, Key: "a", Domains: []string{"a.test"}, Targets: []string{ba.URL}},
		{Enable: true, Key: "b", Domains: []string{"b.test"}, Targets: []string{bb.URL}},
	}}
	addr, run := startRunner(t, l, nil, nil)
	get := func(host string) {
		t.Helper()
		req, _ := http.NewRequest("GET", "http://"+addr+"/", nil)
		req.Host = host
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("%s 返回 %d", host, resp.StatusCode)
		}
	}
	get("a.test")
	get("b.test")
	find := func(key string) *siteHandler {
		for _, h := range run.current().sites {
			if h.site.Key == key {
				return h
			}
		}
		t.Fatalf("找不到站点 %s", key)
		return nil
	}
	oldA, oldB := find("a"), find("b")

	// 只改 a 的名称（与后端连接无关）与 b 的后端连接超时
	l2 := run.cfg
	l2.Sites = append([]Site(nil), run.cfg.Sites...)
	l2.Sites[0].Name = "renamed"
	l2.Sites[1].Timeouts.Connect = 5
	if err := run.reload(l2, nil); err != nil {
		t.Fatal(err)
	}
	newA, newB := find("a"), find("b")
	if newA == oldA || newA.site.Name != "renamed" {
		t.Fatal("站点 a 应已热更新")
	}
	if newA.metrics != oldA.metrics || newA.stats != oldA.stats || newA.conns != oldA.conns || newB.metrics != oldB.metrics {
		t.Fatal("热更新应直接沿用统计对象")
	}
	if newA.transports[0].tr != oldA.transports[0].tr {
		t.Fatal("后端连接设置未变的站点应沿用连接池")
	}
	if newB.transports[0].tr == oldB.transports[0].tr {
		t.Fatal("后端连接设置变了的站点应新建连接池")
	}
	if newA.reuse != nil || newB.reuse != nil {
		t.Fatal("构建结束后不应再引用旧连接池索引")
	}
	// b 的旧空闲连接被关闭，a 的保留
	waitConn(t, "b 的旧连接关闭", func() bool {
		_, closed := cb.get()
		return closed == 1
	})
	get("a.test")
	if opened, closed := ca.get(); opened != 1 || closed != 0 {
		t.Fatalf("a 的空闲连接应继续复用: 新建 %d 次，关闭 %d 次", opened, closed)
	}
	if n := newA.metrics.view(1, time.Now()).Series[0].Requests; n != 2 {
		t.Fatalf("热更新前后的请求应累计: %d", n)
	}

	// 停止监听时关闭全部空闲连接
	run.stop()
	waitConn(t, "停止后 a 的连接关闭", func() bool {
		_, closed := ca.get()
		return closed == 1
	})
}

// 同一组里同一后端写了多次时，连接池按出现顺序一一沿用，不会两个后端共用一个连接池。
func TestReloadReusesDuplicateTransports(t *testing.T) {
	target := "http://127.0.0.1:9"
	site := Site{Enable: true, Key: "s", Domains: []string{"a.test"}, Targets: []string{target, target + " backup"}}
	if err := site.validate(false); err != nil {
		t.Fatal(err)
	}
	old, err := buildSiteHandler(site, testLogs().Logger("t"), newAuthenticator("x"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cur, err := buildSiteHandler(site, testLogs().Logger("t"), newAuthenticator("x"), nil, old)
	if err != nil {
		t.Fatal(err)
	}
	if len(cur.transports) != 2 || cur.transports[0].tr != old.transports[0].tr || cur.transports[1].tr != old.transports[1].tr {
		t.Fatal("重复地址的连接池应按顺序一一沿用")
	}
	if cur.transports[0].tr == cur.transports[1].tr {
		t.Fatal("两个后端不应共用一个连接池")
	}
	// 明文 HTTP/2 后端另备的 HTTP/1.1 连接池同样按设置区分
	site.UpstreamH2C = true
	h2, err := buildSiteHandler(site, testLogs().Logger("t"), newAuthenticator("x"), nil, cur)
	if err != nil {
		t.Fatal(err)
	}
	if len(h2.transports) != 4 {
		t.Fatalf("开启明文 HTTP/2 后每个后端应有两个连接池: %d", len(h2.transports))
	}
	reused := 0
	for _, kt := range h2.transports {
		for _, o := range cur.transports {
			if kt.tr == o.tr {
				reused++
			}
		}
	}
	if reused != 2 {
		t.Fatalf("协议升级用的 HTTP/1.1 连接池应沿用原来的两个连接池: %d", reused)
	}
}
