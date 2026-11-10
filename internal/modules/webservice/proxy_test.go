package webservice

import (
	"compress/gzip"
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/andybalholm/brotli"

	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/logbuf"
)

func namedBackend(t *testing.T, name string) *httptest.Server {
	t.Helper()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(200)
			return
		}
		_, _ = w.Write([]byte(name))
	}))
	t.Cleanup(b.Close)
	return b
}

func mustPool(t *testing.T, s Site) *pool {
	t.Helper()
	s.Type = TypeProxy
	p, err := newPool(s, newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseUpstream(t *testing.T) {
	u, err := parseUpstream("http://10.0.0.1:80 weight=3 backup")
	if err != nil || u.Weight != 3 || !u.Backup || u.URL.Host != "10.0.0.1:80" {
		t.Fatalf("解析结果不符: %+v %v", u, err)
	}
	for _, bad := range []string{"", "ftp://x", "http://a weight=0", "http://a foo=1", "http://a weight=x"} {
		if _, err := parseUpstream(bad); err == nil {
			t.Errorf("%q 应解析失败", bad)
		}
	}
}

func TestPoolPolicies(t *testing.T) {
	ip := netip.MustParseAddr("1.2.3.4")
	// 加权轮询：权重 3:1 时 4 次请求中 a 占 3 次
	p := mustPool(t, Site{Targets: []string{"http://a:1 weight=3", "http://b:1"}})
	count := map[string]int{}
	for i := 0; i < 8; i++ {
		count[p.pick(ip).url.Host]++
	}
	if count["a:1"] != 6 || count["b:1"] != 2 {
		t.Fatalf("加权轮询分布不符: %v", count)
	}
	// ip_hash：同一 IP 始终落到同一后端
	p = mustPool(t, Site{LBPolicy: LBIPHash, Targets: []string{"http://a:1", "http://b:1", "http://c:1"}})
	first := p.pick(ip).url.Host
	for i := 0; i < 10; i++ {
		if p.pick(ip).url.Host != first {
			t.Fatal("ip_hash 应保持粘性")
		}
	}
	// least_conn：选择连接数最少的
	p = mustPool(t, Site{LBPolicy: LBLeastConn, Targets: []string{"http://a:1", "http://b:1"}})
	p.ups[0].conns.Store(5)
	if p.pick(ip).url.Host != "b:1" {
		t.Fatal("least_conn 应选连接少的后端")
	}
	// 备用后端：主后端全部不可用时才启用
	p = mustPool(t, Site{Targets: []string{"http://a:1", "http://spare:1 backup"}})
	if p.pick(ip).url.Host != "a:1" {
		t.Fatal("主后端可用时不应使用备用")
	}
	p.ups[0].down.Store(true)
	if p.pick(ip).url.Host != "spare:1" {
		t.Fatal("主后端不可用时应使用备用")
	}
	p.ups[1].down.Store(true)
	if p.pick(ip).url.Host != "a:1" {
		t.Fatal("全部不可用时应回退尝试主后端")
	}
}

func TestPassiveFailureAndRecovery(t *testing.T) {
	p := mustPool(t, Site{Targets: []string{"http://a:1", "http://b:1"}})
	a := p.ups[0]
	for i := 0; i < passiveFailLimit; i++ {
		p.fail(a, io.EOF)
	}
	if !a.down.Load() {
		t.Fatal("连续失败后应摘除")
	}
	for i := 0; i < 6; i++ {
		if p.pick(netip.MustParseAddr("1.1.1.1")) == a {
			t.Fatal("冷却期内不应选择被摘除的后端")
		}
	}
	// 冷却结束后允许试探，成功则恢复
	a.retryAt.Store(time.Now().Add(-time.Second).UnixNano())
	if !a.usable(time.Now()) {
		t.Fatal("冷却结束后应允许试探")
	}
	p.ok(a)
	if a.down.Load() {
		t.Fatal("试探成功后应恢复")
	}
	// 单后端不摘除
	single := mustPool(t, Site{Targets: []string{"http://a:1"}})
	for i := 0; i < 10; i++ {
		single.fail(single.ups[0], io.EOF)
	}
	if single.ups[0].down.Load() {
		t.Fatal("单后端不应被摘除")
	}
}

func TestActiveHealthCheck(t *testing.T) {
	var healthy atomic.Bool
	healthy.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" && !healthy.Load() {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	p := mustPool(t, Site{Targets: []string{srv.URL, "http://127.0.0.1:1"}, Health: HealthCheck{Enable: true, Mode: "http", Path: "/healthz", Interval: 1, Timeout: 1, Fails: 2, Passes: 1}})
	got := make(chan events.Event, 8)
	defer events.Subscribe(func(e events.Event) {
		if e.Type == events.UpstreamDown || e.Type == events.UpstreamUp {
			got <- e
		}
	})()
	// 缩短检查间隔以加快测试
	p.hc.Interval = 1
	ctx, cancel := contextWithCancel(t)
	defer cancel()
	p.startChecks(ctx)
	waitFor(t, 6*time.Second, func() bool { return p.ups[1].down.Load() }, "不可达后端应被标记为不可用")
	if p.ups[0].down.Load() {
		t.Fatal("健康的后端不应被摘除")
	}
	healthy.Store(false)
	waitFor(t, 6*time.Second, func() bool { return p.ups[0].down.Load() }, "返回 503 的后端应被摘除")
	healthy.Store(true)
	waitFor(t, 6*time.Second, func() bool { return !p.ups[0].down.Load() }, "恢复后应重新启用")
	var down, up int
	timeout := time.After(time.Second)
loop:
	for {
		select {
		case e := <-got:
			if e.Type == events.UpstreamDown {
				down++
			} else {
				up++
			}
		case <-timeout:
			break loop
		}
	}
	if down < 2 || up < 1 {
		t.Fatalf("状态变化应发布事件，down=%d up=%d", down, up)
	}
}

func TestLoadBalanceEndToEnd(t *testing.T) {
	a, b := namedBackend(t, "A"), namedBackend(t, "B")
	l := Listener{Sites: []Site{{Enable: true, Domains: []string{"lb.test"}, Targets: []string{a.URL, b.URL}}}}
	addr, _ := startRunner(t, l, nil, nil)
	seen := map[string]int{}
	for i := 0; i < 6; i++ {
		_, body := get(t, http.DefaultClient, addr, "lb.test", "/", nil)
		seen[body]++
	}
	if seen["A"] != 3 || seen["B"] != 3 {
		t.Fatalf("轮询应均分: %v", seen)
	}
	// 一个后端宕机后，被动检查会把请求全部切到另一台
	a.Close()
	for i := 0; i < 12; i++ {
		get(t, http.DefaultClient, addr, "lb.test", "/", nil)
	}
	seen = map[string]int{}
	for i := 0; i < 6; i++ {
		resp, body := get(t, http.DefaultClient, addr, "lb.test", "/", nil)
		if resp.StatusCode == 200 {
			seen[body]++
		}
	}
	if seen["B"] != 6 {
		t.Fatalf("宕机后应只访问 B: %v", seen)
	}
}

func TestCompression(t *testing.T) {
	big := strings.Repeat("hello gately ", 200)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/img":
			w.Header().Set("Content-Type", "image/png")
		case "/small":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("tiny"))
			return
		default:
			w.Header().Set("Content-Type", "text/plain")
		}
		_, _ = w.Write([]byte(big))
	}))
	defer backend.Close()
	l := Listener{Sites: []Site{{Enable: true, Domains: []string{"z.test"}, Targets: []string{backend.URL}, Compress: true}}}
	addr, _ := startRunner(t, l, nil, nil)
	// 禁用自动解压，才能观察原始编码
	c := &http.Client{Transport: &http.Transport{DisableCompression: true}}

	read := func(path, enc string) (*http.Response, []byte) {
		req, _ := http.NewRequest("GET", "http://"+addr+path, nil)
		req.Host = "z.test"
		if enc != "" {
			req.Header.Set("Accept-Encoding", enc)
		}
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, b
	}
	resp, body := read("/", "gzip")
	if resp.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("应使用 gzip: %v", resp.Header)
	}
	zr, err := gzip.NewReader(strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := io.ReadAll(zr)
	if string(plain) != big || len(body) >= len(big) {
		t.Fatalf("gzip 内容不符，压缩后 %d 原始 %d", len(body), len(big))
	}
	resp, body = read("/", "br, gzip")
	if resp.Header.Get("Content-Encoding") != "br" {
		t.Fatalf("应优先 br: %v", resp.Header)
	}
	plain, _ = io.ReadAll(brotli.NewReader(strings.NewReader(string(body))))
	if string(plain) != big {
		t.Fatal("br 解压内容不符")
	}
	if resp, _ = read("/img", "gzip"); resp.Header.Get("Content-Encoding") != "" {
		t.Fatal("图片不应压缩")
	}
	if resp, _ = read("/small", "gzip"); resp.Header.Get("Content-Encoding") != "" {
		t.Fatal("过小的响应不应压缩")
	}
	if resp, body = read("/", ""); resp.Header.Get("Content-Encoding") != "" || string(body) != big {
		t.Fatal("客户端不支持压缩时应原样返回")
	}
}

func TestRateLimit(t *testing.T) {
	b := okBackend(t)
	l := Listener{Sites: []Site{{Enable: true, Domains: []string{"r.test"}, Targets: []string{b.URL}, RateLimit: RateLimit{Enable: true, Rate: 0.5, Burst: 2}}}}
	addr, _ := startRunner(t, l, nil, nil)
	var codes []int
	for i := 0; i < 4; i++ {
		resp, _ := get(t, http.DefaultClient, addr, "r.test", "/", nil)
		codes = append(codes, resp.StatusCode)
		if resp.StatusCode == 429 && resp.Header.Get("Retry-After") == "" {
			t.Fatal("429 应携带 Retry-After")
		}
	}
	if codes[0] != 200 || codes[1] != 200 || codes[2] != 429 || codes[3] != 429 {
		t.Fatalf("突发 2 之后应被限流: %v", codes)
	}
}

func TestLimiterConcurrencyAndRefill(t *testing.T) {
	now := time.Now()
	l := newLimiter(RateLimit{Rate: 1, Burst: 1, MaxConns: 1})
	l.now = func() time.Time { return now }
	rel, _, ok := l.acquire("k")
	if !ok {
		t.Fatal("首个请求应通过")
	}
	if _, _, ok := l.acquire("k"); ok {
		t.Fatal("令牌耗尽应拒绝")
	}
	now = now.Add(2 * time.Second)
	if _, _, ok := l.acquire("k"); ok {
		t.Fatal("并发上限为 1，前一个请求未结束应拒绝")
	}
	rel()
	if _, _, ok := l.acquire("k"); !ok {
		t.Fatal("释放并补充令牌后应通过")
	}
	if _, _, ok := l.acquire("other"); !ok {
		t.Fatal("不同 IP 互不影响")
	}
}

func TestCache(t *testing.T) {
	var hits atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/private":
			w.Header().Set("Cache-Control", "private")
		case "/maxage":
			w.Header().Set("Cache-Control", "max-age=60")
		case "/cookie":
			http.SetCookie(w, &http.Cookie{Name: "a", Value: "b"})
		}
		_, _ = w.Write([]byte("body"))
	}))
	defer backend.Close()
	l := Listener{Sites: []Site{{Enable: true, Domains: []string{"c.test"}, Targets: []string{backend.URL}, Cache: CacheConfig{Enable: true, TTL: 30}}}}
	addr, _ := startRunner(t, l, nil, nil)
	c := http.DefaultClient
	x := func(path string, hdr map[string]string) string {
		resp, _ := get(t, c, addr, "c.test", path, hdr)
		return resp.Header.Get("X-Cache")
	}
	if x("/a", nil) != "MISS" || x("/a", nil) != "HIT" {
		t.Fatal("第二次请求应命中缓存（使用站点默认 TTL）")
	}
	if hits.Load() != 1 {
		t.Fatalf("后端应只被访问一次: %d", hits.Load())
	}
	if x("/maxage", nil) != "MISS" || x("/maxage", nil) != "HIT" {
		t.Fatal("max-age 响应应被缓存")
	}
	for _, p := range []string{"/private", "/cookie"} {
		x(p, nil)
		if x(p, nil) == "HIT" {
			t.Fatalf("%s 不应被缓存", p)
		}
	}
	x("/a", nil)
	if x("/a", map[string]string{"Cookie": "s=1"}) == "HIT" || x("/a", map[string]string{"Authorization": "Bearer x"}) == "HIT" {
		t.Fatal("带 Cookie 或 Authorization 的请求不应命中缓存")
	}
	if x("/a", map[string]string{"Cache-Control": "no-cache"}) == "HIT" {
		t.Fatal("no-cache 请求应绕过缓存")
	}
}

func TestRespCacheEviction(t *testing.T) {
	c := newRespCache(CacheConfig{MaxMB: 1})
	c.max = 100
	exp := time.Now().Add(time.Hour)
	for _, k := range []string{"a", "b", "c"} {
		c.put(&cacheEntry{key: k, body: make([]byte, 40), expires: exp})
	}
	if c.get("a") != nil || c.get("b") == nil || c.get("c") == nil {
		t.Fatal("超出容量应淘汰最久未使用的条目")
	}
	c.now = func() time.Time { return exp.Add(time.Second) }
	if c.get("b") != nil {
		t.Fatal("过期条目应失效")
	}
}

func TestRewrites(t *testing.T) {
	b := okBackend(t)
	l := Listener{Sites: []Site{{Enable: true, Domains: []string{"w.test"}, Targets: []string{b.URL}, Rewrites: []Rewrite{
		{Pattern: `^/old/(.*)$`, To: "/new/$1", Redirect: true, Code: 301},
		{Pattern: `^/api/v1/(.*)$`, To: "/v2/$1"},
	}}}}
	addr, _ := startRunner(t, l, nil, nil)
	c := noRedirect()
	resp, _ := get(t, c, addr, "w.test", "/old/page?x=1", nil)
	if resp.StatusCode != 301 || resp.Header.Get("Location") != "/new/page?x=1" {
		t.Fatalf("重定向不符: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	if _, body := get(t, c, addr, "w.test", "/api/v1/users", nil); body != "ok:/v2/users" {
		t.Fatalf("重写后后端应收到 /v2/users: %s", body)
	}
	if _, body := get(t, c, addr, "w.test", "/other", nil); body != "ok:/other" {
		t.Fatalf("未命中的路径应保持不变: %s", body)
	}
}

func TestStatsAndMetrics(t *testing.T) {
	b := okBackend(t)
	l := Listener{Sites: []Site{{Enable: true, Domains: []string{"m.test"}, Targets: []string{b.URL}}}}
	addr, r := startRunner(t, l, nil, nil)
	for i := 0; i < 3; i++ {
		get(t, http.DefaultClient, addr, "m.test", "/", nil)
	}
	get(t, http.DefaultClient, addr, "unknown.test", "/", nil)
	h := r.current().sites[0]
	v := h.metrics.view(5, time.Now())
	var total int64
	for _, p := range v.Series {
		total += p.Requests
	}
	if len(v.Series) != 5 || total != 3 || v.Status["2xx"] != 3 || v.BytesOut == 0 {
		t.Fatalf("统计不符: total=%d %+v", total, v.Status)
	}
	if len(v.TopIPs) != 1 || v.TopIPs[0].Key != "127.0.0.1" || v.TopIPs[0].Count != 3 {
		t.Fatalf("来源排行不符: %+v", v.TopIPs)
	}
	// 其他分钟的数据不应混入窗口
	old := h.metrics.view(1, time.Now().Add(-10*time.Minute))
	if old.Series[0].Requests != 0 {
		t.Fatal("窗口之外的分钟应为 0")
	}
}

func TestValidateProxyExtras(t *testing.T) {
	base := func() Site { return Site{Domains: []string{"a.com"}, Targets: []string{"http://127.0.0.1:1"}} }
	bad := map[string]func(*Site){
		"策略":     func(s *Site) { s.LBPolicy = "x" },
		"检查路径":   func(s *Site) { s.Health = HealthCheck{Enable: true, Path: "healthz"} },
		"检查超范围":  func(s *Site) { s.Health = HealthCheck{Enable: true, Interval: 99999} },
		"限流空":    func(s *Site) { s.RateLimit = RateLimit{Enable: true} },
		"缓存容量":   func(s *Site) { s.Cache = CacheConfig{Enable: true, MaxMB: 99999} },
		"重写正则":   func(s *Site) { s.Rewrites = []Rewrite{{Pattern: "(", To: "/x"}} },
		"重写目标":   func(s *Site) { s.Rewrites = []Rewrite{{Pattern: "^/a", To: "x"}} },
		"重定向状态码": func(s *Site) { s.Rewrites = []Rewrite{{Pattern: "^/a", To: "/b", Redirect: true, Code: 200}} },
		"后端权重":   func(s *Site) { s.Targets = []string{"http://a weight=500"} },
	}
	for name, mut := range bad {
		s := base()
		mut(&s)
		if err := s.validate(false); err == nil {
			t.Errorf("%s: 应校验失败", name)
		}
	}
	s := base()
	s.Health = HealthCheck{Enable: true}
	s.RateLimit = RateLimit{Enable: true, Rate: 5}
	s.Cache = CacheConfig{Enable: true}
	if err := s.validate(false); err != nil {
		t.Fatal(err)
	}
	if s.Health.Interval != 10 || s.Health.Path != "/" || s.RateLimit.Burst != 6 || s.Cache.MaxMB != 64 {
		t.Fatalf("默认值不符: %+v %+v %+v", s.Health, s.RateLimit, s.Cache)
	}
}

func newTestLogger() *logbuf.Logger {
	return logbuf.New(100, log.New(io.Discard, "", 0)).Logger("t")
}

func contextWithCancel(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithCancel(context.Background())
}

func waitFor(t *testing.T, d time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal(msg)
}

// 重写规则跳转中的 {host}：IPv6 字面地址补回方括号，含域名以外字符的 Host 拒绝代入。
func TestRewriteHost(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"w.test", "w.test", true},
		{"W.Test:8443", "w.test", true},
		{"[2001:db8::1]:8443", "[2001:db8::1]", true},
		{"[2001:db8::1]", "[2001:db8::1]", true},
		{"", "", false},
		{"a&b.test", "", false},
		{"a%2fb.test", "", false},
	}
	for _, c := range cases {
		got, ok := rewriteHost(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("rewriteHost(%q) = %q, %v，期望 %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestRewriteRedirectHostIPv6(t *testing.T) {
	b := okBackend(t)
	l := Listener{Sites: []Site{{Enable: true, Domains: []string{"*"}, Targets: []string{b.URL}, Rewrites: []Rewrite{
		{Pattern: `^/(.*)$`, To: "https://{host}:8443/$1", Redirect: true, Code: 302},
	}}}}
	addr, _ := startRunner(t, l, nil, nil)
	c := noRedirect()
	resp, _ := get(t, c, addr, "[2001:db8::1]:80", "/x", nil)
	if resp.StatusCode != 302 || resp.Header.Get("Location") != "https://[2001:db8::1]:8443/x" {
		t.Fatalf("IPv6 跳转不符: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	resp, _ = get(t, c, addr, "a&b.test", "/x", nil)
	if resp.StatusCode != 400 {
		t.Fatalf("非法 Host 应返回 400: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
}
