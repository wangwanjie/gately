package webservice

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/logbuf"
)

func testLogs() *logbuf.Buffer {
	return logbuf.New(50, log.New(io.Discard, "", 0))
}

// TestRateRing 验证秒级环：曲线按时间正序、不含正在累计的当前秒、缺失的秒为 0，
// 超过槽数的旧数据被覆盖，均值按完整秒计算。
func TestRateRing(t *testing.T) {
	var r rateRing
	const now = int64(1_000_000)
	r.add(now, 5, 500)    // 当前秒，不计入
	r.add(now-1, 3, 300)  // 最近一个完整秒
	r.add(now-1, 1, 100)  // 同一秒累加
	r.add(now-10, 6, 600) // 10 秒窗口内最早的一秒
	r.add(now-11, 7, 700) // 只在 60 秒窗口内
	r.add(now-60, 2, 0)   // 60 秒窗口内最早的一秒
	r.add(now-61, 100, 0) // 窗口外
	r.add(0, 100, 100)    // 无效时间忽略
	s := r.series(now, rateSeriesSeconds)
	if len(s) != 60 || s[0].Time != now-60 || s[59].Time != now-1 {
		t.Fatalf("曲线范围错误: len=%d first=%d last=%d", len(s), s[0].Time, s[len(s)-1].Time)
	}
	if s[59].Requests != 4 || s[59].Bytes != 400 || s[0].Requests != 2 || s[30].Requests != 0 {
		t.Fatalf("曲线数值错误: %+v %+v %+v", s[59], s[0], s[30])
	}
	rt := r.rate(now)
	if rt.QPS != 1 || rt.BPS != 100 {
		t.Fatalf("10 秒均值错误: %+v", rt)
	}
	if want := float64(4+6+7+2) / 60; rt.QPS60 != want {
		t.Fatalf("60 秒均值错误: %v != %v", rt.QPS60, want)
	}
	// 同一槽位在 rateSlots 秒后复用，旧数据不能混入
	r.add(now-1+rateSlots, 1, 0)
	if s := r.series(now+rateSlots, 1); s[0].Requests != 1 {
		t.Fatalf("槽位复用后应只含新数据: %+v", s)
	}
	if got := len(r.series(now, 1000)); got != rateSlots-1 {
		t.Fatalf("曲线长度应限制在 %d: %d", rateSlots-1, got)
	}
}

// TestRecentIPs 验证近期地址表：窗口过期、IPv4 映射地址归一、满载时每秒最多清理一次、仍满时标记不完整、nil 安全。
func TestRecentIPs(t *testing.T) {
	r := newRecentIPs()
	const now = int64(2_000_000)
	r.touch(netip.MustParseAddr("::ffff:192.0.2.1"), now-70)
	r.touch(netip.MustParseAddr("192.0.2.2"), now-5)
	r.touch(netip.Addr{}, now)
	got := map[string]int64{}
	if r.each(now, func(a netip.Addr, last int64) { got[a.String()] = last }) {
		t.Fatal("未满载时不应标记不完整")
	}
	if len(got) != 1 || got["192.0.2.2"] != now-5 {
		t.Fatalf("窗口内地址错误: %v", got)
	}
	if len(r.m) != 1 {
		t.Fatal("遍历时应删除过期项")
	}

	// 填满：一半在窗口外即将过期
	r = newRecentIPs()
	for i := 0; i < recentCap; i++ {
		sec := now - 1
		if i%2 == 0 {
			sec = now - recentWindow - 1
		}
		r.touch(netip.AddrFrom4([4]byte{10, 0, byte(i >> 8), byte(i)}), sec)
	}
	extra := netip.MustParseAddr("198.51.100.1")
	r.touch(extra, now)
	if _, ok := r.m[extra]; !ok || len(r.m) != recentCap/2+1 {
		t.Fatalf("满载时应清理过期项后记录新地址: len=%d", len(r.m))
	}
	// 再次填满，且全部在窗口内
	for i := 0; len(r.m) < recentCap; i++ {
		r.touch(netip.AddrFrom4([4]byte{172, 16, byte(i >> 8), byte(i)}), now)
	}
	r.touch(netip.MustParseAddr("198.51.100.2"), now)
	if len(r.m) != recentCap || r.overflow.Load() != now {
		t.Fatalf("仍满时应丢弃新地址并记录溢出: len=%d overflow=%d", len(r.m), r.overflow.Load())
	}
	// 同一秒内不再清理：把一项改为过期，同秒的新地址仍被丢弃
	r.m[extra] = now - recentWindow - 5
	r.touch(netip.MustParseAddr("198.51.100.3"), now)
	if _, ok := r.m[netip.MustParseAddr("198.51.100.3")]; ok {
		t.Fatal("同一秒内不应重复清理")
	}
	// 下一秒清理后可以记录
	r.touch(netip.MustParseAddr("198.51.100.3"), now+1)
	if _, ok := r.m[netip.MustParseAddr("198.51.100.3")]; !ok {
		t.Fatal("下一秒应清理过期项并记录新地址")
	}
	if !r.each(now+1, func(netip.Addr, int64) {}) {
		t.Fatal("窗口内发生过溢出时应标记不完整")
	}
	if r.each(now+recentWindow+1, func(netip.Addr, int64) {}) {
		t.Fatal("溢出移出窗口后不应再标记不完整")
	}

	var nilR *recentIPs
	nilR.touch(extra, now)
	if nilR.each(now, func(netip.Addr, int64) { t.Fatal("nil 不应有数据") }) {
		t.Fatal("nil 不应标记不完整")
	}
}

// TestBlockedReasons 验证按原因的拦截计数：IP 名单、限流、封禁分别计数，认证失败与后端返回的 403 不算拦截，
// 统计接口与 Prometheus 指标给出相同的值。
func TestBlockedReasons(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/forbidden" {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer backend.Close()
	m, addr := newConnsModule(t, []Site{
		{Enable: true, Key: "wl", Domains: []string{"wl.test"}, Targets: []string{backend.URL}, IPMode: IPModeWhitelist, IPList: []string{"198.51.100.0/24"}},
		{Enable: true, Key: "rl", Domains: []string{"rl.test"}, Targets: []string{backend.URL}, RateLimit: RateLimit{Enable: true, Rate: 0.001, Burst: 1}},
		{Enable: true, Key: "auth", Domains: []string{"auth.test"}, Targets: []string{backend.URL}, BasicAuth: true, Users: []string{"u:p"}},
	}, Site{})
	m.realIP.set([]string{"127.0.0.0/8"}, nil)
	t.Cleanup(func() { m.auth.unban("203.0.113.9") })
	send := func(host, path, ip string) int {
		resp, _ := get(t, http.DefaultClient, addr, host, path, map[string]string{"X-Forwarded-For": ip})
		return resp.StatusCode
	}
	if c := send("wl.test", "/", "203.0.113.1"); c != 403 {
		t.Fatalf("名单外应被拒绝: %d", c)
	}
	if c := send("wl.test", "/forbidden", "198.51.100.1"); c != 403 {
		t.Fatalf("后端 403 应原样返回: %d", c)
	}
	if c := send("rl.test", "/", "203.0.113.2"); c != 200 {
		t.Fatalf("首个请求应放行: %d", c)
	}
	if c := send("rl.test", "/", "203.0.113.2"); c != 429 {
		t.Fatalf("超出限流应返回 429: %d", c)
	}
	if c := send("auth.test", "/", "203.0.113.3"); c != 401 {
		t.Fatalf("未认证应返回 401: %d", c)
	}
	m.auth.ban("203.0.113.9", time.Minute, "test", true)
	if c := send("auth.test", "/", "203.0.113.9"); c != 403 {
		t.Fatalf("封禁地址应被拒绝: %d", c)
	}
	m.mu.Lock()
	wl, rl, au := m.findSiteLocked("l1", "wl"), m.findSiteLocked("l1", "rl"), m.findSiteLocked("l1", "auth")
	m.mu.Unlock()
	want := map[*siteHandler]map[string]int64{
		wl: {"ip": 1}, rl: {"rateLimit": 1}, au: {"ban": 1},
	}
	for h, w := range want {
		got := h.stats.blocked()
		for _, name := range blockNames {
			if got[name] != w[name] {
				t.Fatalf("%s 的 %s 拦截数 %d != %d", h.site.Key, name, got[name], w[name])
			}
		}
	}
	code, out := callJSON(t, m.handleStats, "GET", "/stats?listener=l1&site=rl", nil, "")
	if code != 200 || out["blocked"].(map[string]any)["rateLimit"].(float64) != 1 {
		t.Fatalf("统计接口应返回拦截原因: %d %v", code, out)
	}
	rec := httptest.NewRecorder()
	m.handleMetrics(rec, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(rec.Body.String(), `gately_site_blocked_total{listener="#l1",reason="ip",site="wl.test"} 1`) {
		t.Fatalf("Prometheus 应导出拦截原因:\n%s", rec.Body.String())
	}
}

// TestBlockedWAF 验证 WAF 拦截计入 waf、访问日志记 deny=waf，正常请求不计。
func TestBlockedWAF(t *testing.T) {
	fw := blockingWAF{}
	l := Listener{Sites: []Site{{Enable: true, Key: "w", Domains: []string{"w.test"}, Type: TypeText, Text: "ok", StatusCode: 200, WAF: "k", AccessLog: true}}}
	logs := testLogs()
	addr, run := startRunnerWAFLogs(t, l, fw, logs)
	if resp, _ := get(t, http.DefaultClient, addr, "w.test", "/", nil); resp.StatusCode != 200 {
		t.Fatalf("正常请求应放行: %d", resp.StatusCode)
	}
	if resp, _ := get(t, http.DefaultClient, addr, "w.test", "/attack", nil); resp.StatusCode != 403 {
		t.Fatalf("WAF 应拦截: %d", resp.StatusCode)
	}
	if n := siteOf(run).stats.Blocked[blockWAF].Load(); n != 1 {
		t.Fatalf("WAF 拦截数应为 1: %d", n)
	}
	es := logs.Query(0, 0, 0, &logbuf.Filter{Status: []logbuf.StatusRange{{Min: 100, Max: 599}}})
	if len(es) != 2 || es[0].Fields[logbuf.FieldDeny] != "" || es[1].Fields[logbuf.FieldDeny] != "waf" {
		t.Fatalf("访问日志应只给 WAF 拦截的请求记 deny=waf: %+v", es)
	}
}

// blockingWAF 拦截路径为 /attack 的请求。
type blockingWAF struct{}

func (blockingWAF) Middleware(_ string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/attack" {
			http.Error(w, "blocked", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func startRunnerWAF(t *testing.T, l Listener, waf WAFProvider) (string, *listenerRunner) {
	t.Helper()
	return startRunnerWAFLogs(t, l, waf, testLogs())
}

// startRunnerWAFLogs 与 startRunnerWAF 相同，日志写入 logs，便于检查访问日志。
func startRunnerWAFLogs(t *testing.T, l Listener, waf WAFProvider, logs *logbuf.Buffer) (string, *listenerRunner) {
	t.Helper()
	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	l.Port = ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	l.ListenIP, l.Network = "127.0.0.1", "tcp4"
	if err := l.Validate(); err != nil {
		t.Fatal(err)
	}
	r, err := newListenerRunner(l, logs.Logger("t"), newAuthenticator("s"), waf)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.start(nil, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.stop)
	return fmt.Sprintf("127.0.0.1:%d", l.Port), r
}

// TestUpstreamStatsAndScopes 验证分路径与按用户分流的后端出现在后端状态中（地址中的账号密码被去掉），
// 请求与响应字节计入实际转发的后端，修改配置热更新后同一后端的计数沿用；WebSocket 升级不受统计包装影响。
func TestUpstreamStatsAndScopes(t *testing.T) {
	mk := func(body string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, body)
		}))
	}
	main, api, vip := mk("main-body"), mk("api"), mk("vip!")
	defer main.Close()
	defer api.Close()
	defer vip.Close()
	withCred := strings.Replace(main.URL, "http://", "http://user:secret@", 1)
	site := Site{Enable: true, Key: "s", Domains: []string{"s.test"}, Targets: []string{withCred},
		BasicAuth: true, Users: []string{"alice:pw", "bob:pw"},
		Locations:  []Location{{Prefix: "/api", Targets: []string{api.URL}}, {Prefix: "/h", RequestHeaders: []string{"X-A: 1"}}},
		UserRoutes: []UserRoute{{Users: []string{"bob"}, Targets: []string{vip.URL}}}}
	addr, run := startRunner(t, Listener{Sites: []Site{site}}, nil, nil)
	h := siteOf(run)
	st := h.upstreamStatus()
	if len(st) != 3 {
		t.Fatalf("应有站点、分路径、按用户分流三个后端: %+v", st)
	}
	if strings.Contains(st[0].URL, "secret") || strings.Contains(st[0].URL, "user@") || st[0].Scope != "" {
		t.Fatalf("站点后端地址应去掉账号密码: %+v", st[0])
	}
	if st[1].Scope != scopeLocation || st[1].Match != "/api" || st[2].Scope != scopeUser || st[2].Match != "bob" {
		t.Fatalf("分流信息错误: %+v", st)
	}
	call := func(user, path string) {
		req, _ := http.NewRequest("GET", "http://"+addr+path, nil)
		req.Host = "s.test"
		req.SetBasicAuth(user, "pw")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
	}
	call("alice", "/")
	call("alice", "/h/x")
	call("alice", "/api/x")
	call("bob", "/")
	st = h.upstreamStatus()
	if st[0].Requests != 2 || st[0].Bytes != int64(2*len("main-body")) || st[1].Requests != 1 || st[1].Bytes != 3 || st[2].Requests != 1 || st[2].Bytes != 4 {
		t.Fatalf("后端计数错误: %+v", st)
	}
	if r := h.pool.ups[0].stats.rate.rate(time.Now().Unix() + 1); r.QPS60 == 0 {
		t.Fatalf("后端秒级速率应有数据: %+v", r)
	}
	// 热更新（改备注）后沿用计数
	l2 := run.cfg
	l2.Sites = []Site{site}
	l2.Sites[0].Remark = "changed"
	if err := l2.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := run.reload(l2, nil); err != nil {
		t.Fatal(err)
	}
	if st2 := siteOf(run).upstreamStatus(); st2[0].Requests != 2 || st2[1].Requests != 1 || st2[2].Requests != 1 {
		t.Fatalf("热更新后应沿用后端计数: %+v", st2)
	}
}

// TestUpstreamStatsWebSocket 验证经反向代理的 WebSocket 升级仍可用，请求计入后端，升级后的数据计入站点秒级流量。
func TestUpstreamStatsWebSocket(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, brw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = brw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
		_ = brw.Flush()
		line, _ := brw.ReadString('\n')
		_, _ = brw.WriteString("echo:" + line)
		_ = brw.Flush()
	}))
	defer backend.Close()
	addr, run := startRunner(t, Listener{Sites: []Site{{Enable: true, Domains: []string{"ws.test"}, Targets: []string{backend.URL}}}}, nil, nil)
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, _ = c.Write([]byte("GET /ws HTTP/1.1\r\nHost: ws.test\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"))
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, nil)
	if err != nil || resp.StatusCode != 101 {
		t.Fatalf("升级失败: %v %v", resp, err)
	}
	_, _ = c.Write([]byte("hi\n"))
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	if line, _ := br.ReadString('\n'); line != "echo:hi\n" {
		t.Fatalf("升级后数据转发失败: %q", line)
	}
	h := siteOf(run)
	if st := h.upstreamStatus(); st[0].Requests != 1 {
		t.Fatalf("升级请求应计入后端: %+v", st)
	}
	_, bytes := h.metrics.rate.sum(time.Now().Unix()+1, rateSeriesSeconds)
	if bytes < int64(len("echo:hi\n")) {
		t.Fatalf("升级后发给访客的数据应计入站点流量: %d", bytes)
	}
}

// TestListenerActs 验证监听级处理计数：HTTPS 端口收到的明文请求跳转、未匹配站点且无默认站点时的 404。
func TestListenerActs(t *testing.T) {
	certs := fakeCerts{selfSigned(t, "secure.test")}
	addr, run := startRunner(t, Listener{TLS: true, PlainHTTP: PlainHTTPRedirect,
		Sites: []Site{{Enable: true, Domains: []string{"secure.test"}, Type: TypeText, Text: "ok", StatusCode: 200}}}, certs, nil)
	if resp, _ := get(t, noRedirect(), addr, "secure.test", "/", nil); resp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("明文请求应跳转: %d", resp.StatusCode)
	}
	tc := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, ServerName: "secure.test"}}}
	req, _ := http.NewRequest("GET", "https://"+addr+"/", nil)
	req.Host = "unknown.test"
	resp, err := tc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("未知域名应返回 404: %d", resp.StatusCode)
	}
	if run.acts.redirect.total.Load() != 1 || run.acts.notFound.total.Load() != 1 {
		t.Fatalf("监听级计数错误: redirect=%d notFound=%d", run.acts.redirect.total.Load(), run.acts.notFound.total.Load())
	}
	n := 0
	run.acts.redirect.recent.each(time.Now().Unix(), func(netip.Addr, int64) { n++ })
	run.acts.notFound.recent.each(time.Now().Unix(), func(netip.Addr, int64) { n++ })
	if n != 2 {
		t.Fatalf("两类监听级处理应各有 1 个近期地址: %d", n)
	}
}

// topoOf 调用拓扑接口并解析。
func topoOf(t *testing.T, m *Module, query string) TopologyView {
	t.Helper()
	req := httptest.NewRequest("GET", "/topology"+query, nil)
	rec := httptest.NewRecorder()
	m.handleTopology(rec, req)
	if rec.Code != 200 {
		t.Fatalf("拓扑接口失败: %d %s", rec.Code, rec.Body.String())
	}
	var out TopologyView
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestTopologyAPI 验证拓扑快照：监听、站点（含未启用站点与默认站点）、后端与监听级动作的结构与编号，
// 汇总值、活动 IP 及其经过的节点（进行中请求精确到后端），focus 曲线，地址参数校验与列表上限。
func TestTopologyAPI(t *testing.T) {
	release := make(chan struct{})
	var once sync.Once
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/hold" {
			select {
			case <-release:
			case <-r.Context().Done():
			}
			return
		}
		_, _ = io.WriteString(w, "hello")
	}))
	defer backend.Close()
	// 先于 backend.Close 执行，放行挂起的请求
	defer once.Do(func() { close(release) })
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer other.Close()
	m, addr := newConnsModule(t, []Site{
		{Enable: true, Key: "a", Name: "A", Group: "g1", Domains: []string{"a.test"}, Targets: []string{other.URL, backend.URL + " backup"}, LBPolicy: LBRoundRobin, WAF: "k", RateLimit: RateLimit{Enable: true, Rate: 100, Burst: 100}},
		{Enable: false, Key: "off", Domains: []string{"off.test"}, Targets: []string{"http://u:pw@10.0.0.9:80"}},
		{Enable: true, Key: "r", Domains: []string{"*.r.test"}, Type: TypeRedirect, RedirectTo: "https://example.com", RedirectCode: 302},
	}, Site{Enable: true, Type: TypeClose})
	m.realIP.set([]string{"127.0.0.0/8"}, nil)

	// 备用后端只在主后端不可用时使用；把主后端标为不可用，请求都走 backend
	m.mu.Lock()
	ha := m.findSiteLocked("l1", "a")
	m.mu.Unlock()
	ha.pool.ups[0].down.Store(true)
	ha.pool.ups[0].retryAt.Store(time.Now().Add(time.Hour).UnixNano())

	send := func(host, path, ip string) {
		// 默认站点直接断开连接，GET 会被客户端自动重试，改用 POST
		method := "GET"
		if host == "unknown.test" {
			method = "POST"
		}
		req, _ := http.NewRequest(method, "http://"+addr+path, nil)
		req.Host = host
		req.Header.Set("X-Forwarded-For", ip)
		resp, err := noRedirect().Do(req)
		if err == nil {
			_, _ = io.ReadAll(resp.Body)
			resp.Body.Close()
		}
	}
	send("a.test", "/", "198.51.100.7")
	send("x.r.test", "/", "198.51.100.8")
	send("unknown.test", "/", "198.51.100.9") // 默认站点断开
	go send("a.test", "/hold", "203.0.113.50")
	waitConn(t, "进行中的请求", func() bool { return ha.stats.Active.Load() == 1 })

	v := topoOf(t, m, "?focus=s:l1/a&ip=203.0.113.50")
	if !v.Enabled || v.Window != rateAvgWindow || len(v.Listeners) != 1 {
		t.Fatalf("快照概要错误: %+v", v)
	}
	l := v.Listeners[0]
	if l.ID != "l:l1" || !l.Running || len(l.Sites) != 4 {
		t.Fatalf("监听节点错误: %+v", l)
	}
	if len(l.Actions) != 0 {
		t.Fatalf("有默认站点、未开启 HTTPS 跳转时不应有监听级动作: %+v", l.Actions)
	}
	a, off, r, d := l.Sites[0], l.Sites[1], l.Sites[2], l.Sites[3]
	if a.ID != "s:l1/a" || !a.Running || a.Group != "g1" || a.Requests != 2 || a.Active != 1 || len(a.Upstreams) != 2 {
		t.Fatalf("站点 a 错误: %+v", a)
	}
	if strings.Join(a.Guards, ",") != "waf,rateLimit" {
		t.Fatalf("站点防护标记错误: %v", a.Guards)
	}
	if a.Upstreams[0].ID != "u:l1/a/0" || a.Upstreams[0].Healthy == nil || *a.Upstreams[0].Healthy || !a.Upstreams[1].Backup || a.Upstreams[1].Requests != 2 {
		t.Fatalf("站点 a 的后端错误: %+v", a.Upstreams)
	}
	if off.Running || off.Enable || len(off.Upstreams) != 1 || off.Upstreams[0].Healthy != nil || strings.Contains(off.Upstreams[0].URL, "pw") || off.Upstreams[0].ID != "u:l1/off/0" {
		t.Fatalf("未启用站点应列出配置中的后端、无运行状态、去掉密码: %+v", off)
	}
	if r.Type != TypeRedirect || r.Target != "https://example.com" || r.Requests != 1 || r.Status["3xx"] != 1 {
		t.Fatalf("跳转站点错误: %+v", r)
	}
	if !d.Default || d.ID != "s:l1/default" || d.Type != TypeClose || d.Requests != 1 || len(d.Guards) != 0 {
		t.Fatalf("默认站点错误: %+v", d)
	}
	if l.Requests != 4 || v.Requests != 4 || v.Active != 1 {
		t.Fatalf("汇总错误: listener=%d total=%d active=%d", l.Requests, v.Requests, v.Active)
	}
	if len(v.Series) != rateSeriesSeconds {
		t.Fatalf("focus 曲线长度错误: %d", len(v.Series))
	}
	ipNodes := map[string]string{}
	for _, ip := range v.IPs {
		ipNodes[ip.IP] = strings.Join(ip.Nodes, ",")
	}
	if v.IPTotal != 4 || ipNodes["198.51.100.7"] != "s:l1/a,u:l1/a/1" || ipNodes["198.51.100.8"] != "s:l1/r" || ipNodes["198.51.100.9"] != "s:l1/default" {
		t.Fatalf("活动 IP 错误: %+v", v.IPs)
	}
	if v.IPs[0].IP != "203.0.113.50" || v.IPs[0].Active != 1 || ipNodes["203.0.113.50"] != "s:l1/a,u:l1/a/1" ||
		v.IPs[0].ActiveAt["s:l1/a"] != 1 || v.IPs[0].ActiveAt["u:l1/a/1"] != 1 || len(v.IPs[0].ActiveAt) != 2 {
		t.Fatalf("进行中的地址应排在最前并精确到后端: %+v", v.IPs[0])
	}
	once.Do(func() { close(release) })

	// 汇总节点的曲线：internet 为全部站点之和，不含后端（避免重复计数）
	// 取 2 秒后的快照，让全部请求所在的秒都已完整
	all := m.topology(time.Now().Unix()+2, "internet", netip.Addr{})
	var sum int64
	for _, p := range all.Series {
		sum += p.Requests
	}
	if sum != 4 {
		t.Fatalf("实例曲线应为全部站点请求数之和（不含后端）: %d", sum)
	}
	if up := m.topology(time.Now().Unix()+2, "u:l1/a/1", netip.Addr{}); len(up.Series) != rateSeriesSeconds {
		t.Fatalf("后端曲线长度错误: %d", len(up.Series))
	}

	if code, _ := callJSON(t, m.handleTopology, "GET", "/topology?ip=bad", nil, ""); code != 400 {
		t.Fatalf("无效地址应返回 400: %d", code)
	}

	// 列表上限：超出时截断，但正在筛选的地址保留
	now := time.Now().Unix()
	for i := 0; i < topoMaxIPs+20; i++ {
		ha.metrics.recent.touch(netip.AddrFrom4([4]byte{100, 64, byte(i >> 8), byte(i)}), now)
	}
	// 挑一个最后出现得早、排在上限之外的地址
	k := topoMaxIPs + 19
	last := netip.AddrFrom4([4]byte{100, 64, byte(k >> 8), byte(k)})
	ha.metrics.recent.touch(last, now-30)
	big := topoOf(t, m, "?ip="+last.String())
	if !big.IPTruncated || big.IPTotal < topoMaxIPs+20 || len(big.IPs) != topoMaxIPs+1 || big.IPs[len(big.IPs)-1].IP != last.String() {
		t.Fatalf("截断或保留筛选地址错误: truncated=%v total=%d len=%d", big.IPTruncated, big.IPTotal, len(big.IPs))
	}

	// 汇总上限：大量不同来源时只汇总前 topoMaxAggIPs 个地址，正在筛选的地址仍保留
	oldAgg := topoMaxAggIPs
	topoMaxAggIPs = 100
	t.Cleanup(func() { topoMaxAggIPs = oldAgg })
	capped := m.topology(now, "", last)
	if !capped.IPTruncated || capped.IPTotal > 101 {
		t.Fatalf("汇总应在上限处停止: truncated=%v total=%d", capped.IPTruncated, capped.IPTotal)
	}
	found := false
	for _, ip := range capped.IPs {
		found = found || ip.IP == last.String()
	}
	if !found {
		t.Fatal("超出汇总上限时正在筛选的地址也应保留")
	}
}

// 同一组里同一后端地址写了两次时，热更新后两者各自沿用自己的统计，不合并成一份。
func TestInheritDuplicateUpstreams(t *testing.T) {
	site := Site{Enable: true, Key: "s", Domains: []string{"a.test"}, Targets: []string{"http://10.0.0.1:80", "http://10.0.0.1:80 backup"}}
	if err := site.validate(false); err != nil {
		t.Fatal(err)
	}
	mk := func() *siteHandler {
		h, err := newSiteHandler(site, testLogs().Logger("t"), newAuthenticator("x"), nil)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	old, cur := mk(), mk()
	old.pool.ups[0].stats.requests.Store(5)
	old.pool.ups[1].stats.requests.Store(7)
	cur.inherit(old)
	if cur.pool.ups[0].stats.requests.Load() != 5 || cur.pool.ups[1].stats.requests.Load() != 7 || cur.pool.ups[0].stats == cur.pool.ups[1].stats {
		t.Fatalf("重复地址的统计应按顺序一一沿用: %d %d", cur.pool.ups[0].stats.requests.Load(), cur.pool.ups[1].stats.requests.Load())
	}
}

// 后端展示地址（含监控指标标签）去掉用户名、密码与查询串。
func TestUpstreamDisplayURL(t *testing.T) {
	for in, want := range map[string]string{
		"http://10.0.0.1:80":                   "http://10.0.0.1:80",
		"http://u:pw@10.0.0.1:80/api":          "http://10.0.0.1:80/api",
		"https://h.example/p?token=secret&x=1": "https://h.example/p",
		"http://u@h.example:8080/?k=v":         "http://h.example:8080/",
	} {
		spec, err := parseUpstream(in)
		if err != nil {
			t.Fatal(err)
		}
		u := &upstream{url: spec.URL}
		if got := u.displayURL(); got != want {
			t.Errorf("%s → %s，应为 %s", in, got, want)
		}
	}
}

// TestTopologyListenerActions 验证开启 HTTPS 跳转、没有默认站点的监听列出两类监听级动作，并计入监听汇总与活动 IP。
func TestTopologyListenerActions(t *testing.T) {
	m, addr := newConnsModule(t, []Site{{Enable: true, Key: "a", Domains: []string{"a.test"}, Type: TypeText, Text: "ok", StatusCode: 200}}, Site{})
	if resp, _ := get(t, http.DefaultClient, addr, "nope.test", "/", nil); resp.StatusCode != 404 {
		t.Fatalf("未知域名应返回 404: %d", resp.StatusCode)
	}
	v := topoOf(t, m, "?focus=l:l1")
	l := v.Listeners[0]
	if len(l.Actions) != 1 || l.Actions[0].ID != "a:l1/notfound" || l.Actions[0].Kind != "notFound" || l.Actions[0].Requests != 1 || l.Requests != 1 {
		t.Fatalf("监听级动作错误: %+v", l)
	}
	if len(v.IPs) != 1 || strings.Join(v.IPs[0].Nodes, ",") != "a:l1/notfound" {
		t.Fatalf("监听级动作的活动 IP 错误: %+v", v.IPs)
	}
	if len(v.Series) != rateSeriesSeconds {
		t.Fatalf("监听曲线长度错误: %d", len(v.Series))
	}
	// 停用的监听仍列出，未运行
	m.mu.Lock()
	m.cfg.Listeners[0].Enable = false
	m.restartLocked("l1")
	m.mu.Unlock()
	v = topoOf(t, m, "")
	if l := v.Listeners[0]; l.Running || len(l.Sites) != 1 || l.Sites[0].Running || len(l.Actions) != 0 {
		t.Fatalf("停用的监听应列出且未运行: %+v", l)
	}
}

// TestMetricsUpstreamRoutes 验证 Prometheus 中分路径与按用户分流的后端带 route 标签，同一地址不产生重复序列，
// 并导出后端请求数与监听级处理计数。
func TestMetricsUpstreamRoutes(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer backend.Close()
	m, _ := newConnsModule(t, []Site{{Enable: true, Key: "s", Domains: []string{"s.test"}, Targets: []string{backend.URL},
		BasicAuth: true, Users: []string{"bob:pw"},
		Locations:  []Location{{Prefix: "/api", Targets: []string{backend.URL}}},
		UserRoutes: []UserRoute{{Users: []string{"bob"}, Targets: []string{backend.URL}}}}}, Site{})
	rec := httptest.NewRecorder()
	m.handleMetrics(rec, httptest.NewRequest("GET", "/metrics", nil))
	body := rec.Body.String()
	seen := map[string]bool{}
	for _, line := range strings.Split(body, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		series := line[:strings.LastIndex(line, " ")]
		if seen[series] {
			t.Fatalf("重复的序列: %s", series)
		}
		seen[series] = true
	}
	for _, want := range []string{
		`gately_upstream_up{listener="#l1",site="s.test",upstream="` + backend.URL + `"} 1`,
		`gately_upstream_up{listener="#l1",route="/api",site="s.test",upstream="` + backend.URL + `"} 1`,
		`gately_upstream_up{listener="#l1",route="@bob",site="s.test",upstream="` + backend.URL + `"} 1`,
		`gately_upstream_requests_total{listener="#l1",site="s.test",upstream="` + backend.URL + `"} 0`,
		`gately_listener_requests_total{action="not_found",listener="#l1"} 0`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("缺少指标 %s:\n%s", want, body)
		}
	}
}

// BenchmarkSiteServeHTTPTopology 与 BenchmarkSiteServeHTTP 相同但每次换一个来源地址，衡量近期地址表在大量来源下的开销。
func BenchmarkSiteServeHTTPManyIPs(b *testing.B) {
	h, err := newSiteHandler(Site{Enable: true, Key: "s", Domains: []string{"a.test"}, Type: TypeText, Text: "ok", StatusCode: 200},
		testLogs().Logger("b"), newAuthenticator("x"), nil)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	var n uint32
	var mu sync.Mutex
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			n++
			i := n
			mu.Unlock()
			r := httptest.NewRequest("GET", "http://a.test/", nil)
			r.RemoteAddr = fmt.Sprintf("10.%d.%d.%d:1234", byte(i>>16), byte(i>>8), byte(i))
			h.ServeHTTP(httptest.NewRecorder(), r)
		}
	})
}

// TestTopologyConcurrent 在持续请求、反复热更新站点的同时读取拓扑快照与指标，配合 -race 检查数据竞争。
func TestTopologyConcurrent(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "x")
	}))
	defer backend.Close()
	sites := []Site{
		{Enable: true, Key: "a", Domains: []string{"a.test"}, Targets: []string{backend.URL}, Locations: []Location{{Prefix: "/api", Targets: []string{backend.URL}}}},
		{Enable: true, Key: "b", Domains: []string{"b.test"}, Type: TypeText, Text: "ok", StatusCode: 200},
	}
	m, addr := newConnsModule(t, sites, Site{})
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			hosts := []string{"a.test", "b.test", "nope.test"}
			paths := []string{"/", "/api/x"}
			for n := 0; ; n++ {
				select {
				case <-stop:
					return
				default:
				}
				req, _ := http.NewRequest("GET", "http://"+addr+paths[n%2], nil)
				req.Host = hosts[(n+i)%3]
				if resp, err := http.DefaultClient.Do(req); err == nil {
					_, _ = io.ReadAll(resp.Body)
					resp.Body.Close()
				}
			}
		}(i)
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for n := 0; time.Now().Before(deadline); n++ {
		_ = m.topology(time.Now().Unix(), "gately", netip.Addr{})
		_ = m.Metrics()
		m.mu.Lock()
		m.cfg.Listeners[0].Sites[0].Remark = fmt.Sprint(n)
		m.restartLocked("l1")
		m.mu.Unlock()
	}
	close(stop)
	wg.Wait()
	v := m.topology(time.Now().Unix()+1, "", netip.Addr{})
	if v.Requests == 0 || v.Listeners[0].Sites[0].Upstreams[0].Requests == 0 {
		t.Fatalf("热更新后计数应沿用且大于 0: %+v", v.Listeners[0].Sites[0])
	}
}
