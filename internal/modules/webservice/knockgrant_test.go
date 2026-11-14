package webservice

import (
	"bufio"
	"encoding/json"
	"html"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/knockgate"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// withKnockRules 发布测试用的敲门规则目录，结束后清空授权与目录。
func withKnockRules(t *testing.T, rules ...knockgate.Rule) {
	t.Helper()
	knockgate.Default.Clear()
	knockgate.Default.SetRules(rules)
	t.Cleanup(func() {
		knockgate.Default.Clear()
		knockgate.Default.SetRules(nil)
		knockgate.Default.SetUsers(ID, nil)
	})
}

func TestKnockValidate(t *testing.T) {
	base := func() Site {
		return Site{Domains: []string{"in.test", "v4.in.test", "v6.in.test"}, Type: TypeText, Text: "x", BasicAuth: true, Users: []string{"a:b"},
			KnockGrant: KnockGrant{Rule: " k "}}
	}
	s := base()
	if err := s.validate(false); err != nil {
		t.Fatal(err)
	}
	if g := s.KnockGrant; g.Rule != "k" || g.Minutes != defaultKnockMinutes || g.V6Prefix != defaultKnockV6Prefix {
		t.Fatalf("默认值: %+v", g)
	}
	s = base()
	s.KnockGrant.ProbeV4, s.KnockGrant.ProbeV6 = " V4.In.Test ", "v6.in.test"
	if err := s.validate(false); err != nil || s.KnockGrant.ProbeV4 != "v4.in.test" {
		t.Fatalf("探测域名应规范化: %v %+v", err, s.KnockGrant)
	}
	bad := map[string]func(*Site){
		"未启用认证":    func(s *Site) { s.BasicAuth = false },
		"准入同一规则":   func(s *Site) { s.Knock = "k" },
		"时长过短":     func(s *Site) { s.KnockGrant.Minutes = 1 },
		"时长过长":     func(s *Site) { s.KnockGrant.Minutes = maxKnockMinutes + 1 },
		"前缀过短":     func(s *Site) { s.KnockGrant.V6Prefix = 32 },
		"前缀过长":     func(s *Site) { s.KnockGrant.V6Prefix = 129 },
		"探测域名不属本站": func(s *Site) { s.KnockGrant.ProbeV4 = "other.test" },
		"探测域名通配":   func(s *Site) { s.KnockGrant.ProbeV4 = "*.in.test" },
		"探测域名带路径":  func(s *Site) { s.KnockGrant.ProbeV6 = "v6.in.test/x" },
		"两个探测域名相同": func(s *Site) { s.KnockGrant.ProbeV4, s.KnockGrant.ProbeV6 = "v4.in.test", "v4.in.test" },
	}
	for name, f := range bad {
		s := base()
		f(&s)
		if err := s.validate(false); err == nil {
			t.Errorf("%s 应校验失败", name)
		}
	}
	// 默认站点不能作为放行入口，但可以要求准入
	d := Site{Type: TypeText, Text: "x", BasicAuth: true, Users: []string{"a:b"}, KnockGrant: KnockGrant{Rule: "k"}}
	if err := d.validate(true); err == nil {
		t.Fatal("默认站点不能作为放行入口")
	}
	d = Site{Type: TypeText, Text: "x", Knock: "k", KnockLAN: true}
	if err := d.validate(true); err != nil || d.Knock != "k" || !d.KnockLAN {
		t.Fatalf("默认站点可以要求准入: %v", err)
	}
	// 关闭放行入口时清空其余设置；不要求准入时局域网免放行无意义
	s = Site{Domains: []string{"a.test"}, Type: TypeText, Text: "x", KnockLAN: true, KnockGrant: KnockGrant{Minutes: 9, ProbeV4: "x"}}
	if err := s.validate(false); err != nil || s.KnockLAN || s.KnockGrant != (KnockGrant{}) {
		t.Fatalf("应清空: %v %+v", err, s)
	}
	// 引用不存在的规则在保存时拒绝
	withKnockRules(t, knockgate.Rule{Key: "k", Name: "web", Enable: true})
	l := &Listener{Sites: []Site{{Name: "x", Knock: "nope"}}}
	if err := checkKnockRefsLocked(l); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("应拒绝不存在的规则: %v", err)
	}
	l = &Listener{Default: Site{Knock: "k"}, Sites: []Site{{KnockGrant: KnockGrant{Rule: "k"}}}}
	if err := checkKnockRefsLocked(l); err != nil {
		t.Fatal(err)
	}
}

func TestKnockGrantPrefix(t *testing.T) {
	g := KnockGrant{V6Prefix: 64}
	cases := map[string]string{
		"203.0.113.5":          "203.0.113.5/32",
		"::ffff:203.0.113.5":   "203.0.113.5/32",
		"2001:db8:1:2:3:4:5:6": "2001:db8:1:2::/64",
		"127.0.0.1":            "",
		"::1":                  "",
		"192.168.1.5":          "",
		"10.1.2.3":             "",
		"fd00::1":              "",
		"fe80::1":              "",
		"0.0.0.0":              "",
		"224.0.0.1":            "",
	}
	for in, want := range cases {
		p, ok := g.prefix(netip.MustParseAddr(in))
		got := ""
		if ok {
			got = p.String()
		}
		if got != want {
			t.Errorf("%s: 得到 %q，应为 %q", in, got, want)
		}
	}
	if p, _ := (KnockGrant{V6Prefix: 128}).prefix(netip.MustParseAddr("2001:db8::1")); p.Bits() != 128 {
		t.Fatalf("128 只放行单个地址: %s", p)
	}
	if !knockLAN(netip.MustParseAddr("192.168.1.5")) || !knockLAN(netip.MustParseAddr("fd00::1")) || knockLAN(netip.MustParseAddr("127.0.0.1")) || knockLAN(netip.MustParseAddr("203.0.113.1")) {
		t.Fatal("局域网判断不对：回环地址不算局域网")
	}
}

// knockEnv 是端到端测试环境：放行入口 in.test（基本认证），准入站点 nas.test、lan.test（局域网免放行）。
type knockEnv struct {
	t    *testing.T
	addr string
	mu   sync.Mutex
	evts []events.Event
}

func newKnockEnv(t *testing.T) *knockEnv {
	t.Helper()
	withKnockRules(t, knockgate.Rule{Key: "k", Name: "nas", Enable: true})
	sites := []Site{
		{Enable: true, Key: "in", Name: "入口", Domains: []string{"in.test", "v4.in.test", "v6.in.test"}, Type: TypeText, Text: "entry",
			BasicAuth: true, Users: []string{"alice:pw", "bob:pw2"},
			KnockGrant: KnockGrant{Rule: "k", Minutes: 60, ProbeV4: "v4.in.test", ProbeV6: "v6.in.test"}},
		{Enable: true, Key: "nas", Domains: []string{"nas.test"}, Type: TypeText, Text: "nas", Knock: "k"},
		{Enable: true, Key: "lan", Domains: []string{"lan.test"}, Type: TypeText, Text: "lan", Knock: "k", KnockLAN: true},
		{Enable: true, Key: "close", Domains: []string{"close.test"}, Type: TypeText, Text: "c", Knock: "k", DenyClose: true},
	}
	addr, _ := startRunnerLogs(t, Listener{Key: "L", AcceptProxyProtocol: true, Sites: sites}, nil, []string{"127.0.0.1"},
		logbuf.New(500, log.New(io.Discard, "", 0)))
	e := &knockEnv{t: t, addr: addr}
	unsub := events.Subscribe(func(ev events.Event) {
		if ev.Type == events.KnockOpened {
			e.mu.Lock()
			e.evts = append(e.evts, ev)
			e.mu.Unlock()
		}
	})
	t.Cleanup(unsub)
	return e
}

// do 以访客地址 src（经 PROXY 头声明）访问；user 非空时带基本认证。
func (e *knockEnv) do(src, method, host, path, user string, hdr map[string]string, form url.Values) (int, string, http.Header) {
	e.t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, _ := http.NewRequest(method, "http://"+host+path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if user != "" {
		name, pass, _ := strings.Cut(user, ":")
		req.SetBasicAuth(name, pass)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	code, text, h := sendViaProxyFull(e.t, e.addr, src, req)
	return code, text, h
}

func (e *knockEnv) status(src, host string) int {
	e.t.Helper()
	code, _, _ := e.do(src, "GET", host, "/", "", nil, nil)
	return code
}

func (e *knockEnv) opened() []events.Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]events.Event{}, e.evts...)
}

// sendViaProxyFull 经 PROXY 头发送请求，返回状态码、正文与响应头；连接被直接断开时状态码为 0。
func sendViaProxyFull(t *testing.T, addr, src string, req *http.Request) (int, string, http.Header) {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	writeProxyHeader(t, c, src)
	req.Header.Set("Connection", "close")
	if err := req.Write(c); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(c), req)
	if err != nil {
		return 0, "", nil
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), resp.Header
}

func TestKnockAdmissionFlow(t *testing.T) {
	e := newKnockEnv(t)
	const (
		visitor = "203.0.113.5"
		other   = "203.0.113.6"
	)
	// 未放行：准入站点按访问控制拒绝；开启断开连接的站点直接断开；放行入口本身照常要求认证
	if code := e.status(visitor, "nas.test"); code != http.StatusForbidden {
		t.Fatalf("未放行应拒绝: %d", code)
	}
	if code := e.status(visitor, "close.test"); code != 0 {
		t.Fatalf("开启断开连接时应直接断开: %d", code)
	}
	if code := e.status(visitor, "in.test"); code != http.StatusUnauthorized {
		t.Fatalf("入口应要求认证: %d", code)
	}
	// 密码错误不放行
	if code, _, _ := e.do(visitor, "GET", "in.test", "/", "alice:bad", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("密码错误: %d", code)
	}
	if code := e.status(visitor, "nas.test"); code != http.StatusForbidden {
		t.Fatal("认证失败不应放行")
	}
	// 通过认证后放行该地址，其他地址不受影响
	if code, body, _ := e.do(visitor, "GET", "in.test", "/", "alice:pw", nil, nil); code != http.StatusOK || body != "entry" {
		t.Fatalf("登录入口: %d %q", code, body)
	}
	if code := e.status(visitor, "nas.test"); code != http.StatusOK {
		t.Fatalf("放行后应可访问: %d", code)
	}
	if code := e.status(visitor, "close.test"); code != http.StatusOK {
		t.Fatalf("放行后应可访问: %d", code)
	}
	if code := e.status(other, "nas.test"); code != http.StatusForbidden {
		t.Fatalf("其他地址不应放行: %d", code)
	}
	gs := knockgate.Default.List()
	if len(gs) != 1 || gs[0].User != "alice" || gs[0].Site != "L/in" || gs[0].SiteName != "入口" || gs[0].Source != knockgate.SourceWeb {
		t.Fatalf("授权记录: %+v", gs)
	}
	if ev := e.opened(); len(ev) != 1 || ev[0].Fields["ip"] != visitor || ev[0].Fields["user"] != "alice" {
		t.Fatalf("应发布一次放行事件: %+v", ev)
	}
	// 一分钟内的后续请求不重复登记、不重复发布事件
	for range 3 {
		_, _, _ = e.do(visitor, "GET", "in.test", "/x", "alice:pw", nil, nil)
	}
	if n := len(e.opened()); n != 1 {
		t.Fatalf("续期不应重复发布事件: %d", n)
	}

	// IPv6 按 /64 放行：同一网段的其他地址（临时地址轮换）也能访问
	if code, _, _ := e.do("2001:db8:1:2::5", "GET", "in.test", "/", "alice:pw", nil, nil); code != http.StatusOK {
		t.Fatal("IPv6 登录")
	}
	if code := e.status("2001:db8:1:2:aaaa::9", "nas.test"); code != http.StatusOK {
		t.Fatalf("同一 /64 应放行: %d", code)
	}
	if code := e.status("2001:db8:1:3::5", "nas.test"); code != http.StatusForbidden {
		t.Fatalf("其他网段不应放行: %d", code)
	}

	// 内网地址登录不放行（经隧道转发时所有访客都是这类地址）；局域网免放行的站点直接可访问，回环地址不算局域网
	if code, _, _ := e.do("192.168.1.5", "GET", "in.test", "/", "alice:pw", nil, nil); code != http.StatusOK {
		t.Fatal("内网登录入口本身应可访问")
	}
	if code := e.status("192.168.1.5", "nas.test"); code != http.StatusForbidden {
		t.Fatalf("内网地址不应被放行: %d", code)
	}
	if code := e.status("192.168.1.5", "lan.test"); code != http.StatusOK {
		t.Fatalf("局域网免放行: %d", code)
	}
	if code := e.status("127.0.0.1", "lan.test"); code != http.StatusForbidden {
		t.Fatalf("回环地址不算局域网: %d", code)
	}
	if code := e.status("203.0.113.77", "lan.test"); code != http.StatusForbidden {
		t.Fatalf("公网地址仍需放行: %d", code)
	}
}

// 放行页面：显示放行地址与另一地址族的探测链接；探测令牌只能使用一次、必须来自指定地址族；撤销后失效。
func TestKnockPageProbeRevoke(t *testing.T) {
	e := newKnockEnv(t)
	const visitor = "198.51.100.20"
	code, body, _ := e.do(visitor, "GET", "in.test", knockPagePath, "", nil, nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("放行页面需要认证: %d", code)
	}
	code, body, _ = e.do(visitor, "GET", "in.test", knockPagePath, "alice:pw", nil, nil)
	if code != http.StatusOK || !strings.Contains(body, visitor) || !strings.Contains(body, "nas") {
		t.Fatalf("放行页面: %d %s", code, body)
	}
	// IPv4 访客只探测 IPv6
	urls := regexp.MustCompile(`data-url="([^"]+)"`).FindAllStringSubmatch(body, -1)
	if len(urls) != 1 {
		t.Fatalf("应只有 IPv6 探测: %v", urls)
	}
	probe, err := url.Parse(html.UnescapeString(urls[0][1]))
	if err != nil || probe.Host != "v6.in.test" || probe.Path != knockProbePath {
		t.Fatalf("探测地址: %v %v", probe, err)
	}
	// 探测域名意外解析到 IPv4：令牌指定的地址族不符，拒绝并说明原因
	code, body, _ = e.do("198.51.100.21", "GET", "v6.in.test", probe.RequestURI(), "", nil, nil)
	if code != http.StatusConflict || !strings.Contains(body, "AAAA") {
		t.Fatalf("地址族不符: %d %s", code, body)
	}
	// 篡改的令牌按不存在处理
	if code, _, _ := e.do("2001:db8:9::1", "GET", "v6.in.test", knockProbePath+"?t=x"+probe.Query().Get("t"), "", nil, nil); code != http.StatusNotFound {
		t.Fatalf("篡改的令牌: %d", code)
	}
	// 正常探测：放行 IPv6 /64，跨域读取只开放给本站点
	code, body, h := e.do("2001:db8:9::1", "GET", "v6.in.test", probe.RequestURI(), "", map[string]string{"Origin": "http://in.test"}, nil)
	if code != http.StatusOK || h.Get("Access-Control-Allow-Origin") != "http://in.test" {
		t.Fatalf("探测: %d %s %v", code, body, h)
	}
	var got struct{ Addr string }
	if err := json.Unmarshal([]byte(body), &got); err != nil || got.Addr != "2001:db8:9::/64" {
		t.Fatalf("探测结果: %s", body)
	}
	if code := e.status("2001:db8:9::abc", "nas.test"); code != http.StatusOK {
		t.Fatalf("探测登记的网段应放行: %d", code)
	}
	// 令牌只能使用一次
	if code, _, _ := e.do("2001:db8:9::2", "GET", "v6.in.test", probe.RequestURI(), "", nil, nil); code != http.StatusGone {
		t.Fatalf("重复使用令牌: %d", code)
	}
	gs := knockgate.Default.List()
	for _, g := range gs {
		if g.User != "alice" {
			t.Fatalf("探测登记的用户: %+v", g)
		}
	}
	// 其他站点的来源不能读取探测结果
	_, body, _ = e.do(visitor, "GET", "in.test", knockPagePath, "alice:pw", nil, nil)
	probe2, _ := url.Parse(html.UnescapeString(regexp.MustCompile(`data-url="([^"]+)"`).FindStringSubmatch(body)[1]))
	_, _, h = e.do("2001:db8:9::3", "GET", "v6.in.test", probe2.RequestURI(), "", map[string]string{"Origin": "http://evil.test"}, nil)
	if h.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("不应向其他来源开放")
	}

	// 跨站提交的撤销请求被拒绝
	if code, _, _ := e.do(visitor, "POST", "in.test", knockPagePath, "alice:pw", map[string]string{"Origin": "http://evil.test"}, url.Values{"op": {"all"}}); code != http.StatusForbidden {
		t.Fatalf("跨站撤销应拒绝: %d", code)
	}
	if code := e.status(visitor, "nas.test"); code != http.StatusOK {
		t.Fatal("跨站撤销不应生效")
	}
	// 其他用户的放行不受影响
	if code, _, _ := e.do("203.0.113.50", "GET", "in.test", "/", "bob:pw2", nil, nil); code != http.StatusOK {
		t.Fatal("bob 登录")
	}
	// 撤销当前网络：只撤销当前地址
	code, body, _ = e.do(visitor, "POST", "in.test", knockPagePath, "alice:pw", map[string]string{"Origin": "http://in.test"}, url.Values{"op": {"here"}})
	if code != http.StatusOK || !strings.Contains(body, "1") {
		t.Fatalf("撤销当前网络: %d %s", code, body)
	}
	if e.status(visitor, "nas.test") != http.StatusForbidden || e.status("2001:db8:9::abc", "nas.test") != http.StatusOK {
		t.Fatal("只撤销当前地址")
	}
	// 撤销全部：该用户在本入口的其他放行一并撤销，其他用户保留
	if code, _, _ := e.do(visitor, "POST", "in.test", knockPagePath, "alice:pw", map[string]string{"Origin": "http://in.test"}, url.Values{"op": {"all"}}); code != http.StatusOK {
		t.Fatal("撤销全部")
	}
	if e.status("2001:db8:9::abc", "nas.test") != http.StatusForbidden || e.status("203.0.113.50", "nas.test") != http.StatusOK {
		t.Fatal("撤销全部只撤销本人的放行")
	}
	// 规则停用：页面说明原因，不再放行
	knockgate.Default.SetRules([]knockgate.Rule{{Key: "k", Name: "nas"}})
	code, body, _ = e.do("203.0.113.99", "GET", "in.test", knockPagePath, "alice:pw", nil, nil)
	if code != http.StatusForbidden || e.status("203.0.113.99", "nas.test") != http.StatusForbidden {
		t.Fatalf("规则停用后不应放行: %d %s", code, body)
	}
	// 内网地址打开放行页面：说明无法放行
	knockgate.Default.SetRules([]knockgate.Rule{{Key: "k", Name: "nas", Enable: true}})
	code, body, _ = e.do("10.0.0.8", "GET", "in.test", knockPagePath, "alice:pw", nil, nil)
	if code != http.StatusForbidden || !strings.Contains(body, "内网") {
		t.Fatalf("内网地址: %d %s", code, body)
	}
}

// 保存配置时撤销已失效的网页登录放行，并登记站点对规则的引用。
func TestKnockSync(t *testing.T) {
	withKnockRules(t, knockgate.Rule{Key: "k", Name: "nas", Enable: true})
	entry := Site{Enable: true, Key: "in", Name: "入口", Domains: []string{"in.test"}, BasicAuth: true, SharedUsers: true, OAuthUsers: []string{"github:octo"},
		Users: []string{"alice:pw"}, KnockGrant: KnockGrant{Rule: "k", Minutes: 60, V6Prefix: 64}}
	m := &Module{auth: newAuthenticator("secret"), logs: logbuf.New(100, log.New(io.Discard, "", 0))}
	m.cfg.Settings.AuthUsers = []AuthUser{{Name: "carol", Password: "hash1"}}
	m.cfg.Listeners = []Listener{{Key: "L", Default: Site{Knock: "k"}, Sites: []Site{entry, {Enable: true, Key: "nas", Name: "NAS", Knock: "k"}}}}
	shared := map[string]AuthUser{"carol": m.cfg.Settings.AuthUsers[0]}
	until := time.Now().Add(time.Hour)
	add := func(ip, user, site string) {
		t.Helper()
		o := knockgate.Owner{User: user, Site: site, Cred: m.auth.knockCred(entry, shared, user)}
		if err := knockgate.Default.AllowWeb(netip.MustParsePrefix(ip), "k", until, o, netip.Addr{}); err != nil {
			t.Fatal(err)
		}
	}
	add("203.0.113.1/32", "alice", "L/in")
	add("203.0.113.2/32", "carol", "L/in")
	add("203.0.113.3/32", "github:octo", "L/in")
	add("203.0.113.4/32", "gately:admin", "L/in")
	add("203.0.113.5/32", "alice", "L/gone")
	_ = knockgate.Default.Allow(netip.MustParseAddr("203.0.113.6"), "k", until)
	m.syncKnockLocked()
	if n := len(knockgate.Default.List()); n != 5 {
		t.Fatalf("只撤销入口站点已不存在的放行: %d %+v", n, knockgate.Default.List())
	}
	users := strings.Join(knockgate.Default.Users("k"), ",")
	for _, want := range []string{"默认站点", "入口", "NAS"} {
		if !strings.Contains(users, want) {
			t.Fatalf("引用方应包含 %s: %s", want, users)
		}
	}
	allowed := func(ip string) bool { return knockgate.Default.Allowed(netip.MustParseAddr(ip), "k") }
	// 修改站点用户密码、共享用户密码、移除第三方身份：对应放行撤销；后台管理员与敲门报文的授权保留
	s := &m.cfg.Listeners[0].Sites[0]
	s.Users = []string{"alice:new"}
	m.syncKnockLocked()
	if allowed("203.0.113.1") || !allowed("203.0.113.2") {
		t.Fatal("修改站点用户密码后应撤销")
	}
	m.cfg.Settings.AuthUsers[0].Password = "hash2"
	m.syncKnockLocked()
	if allowed("203.0.113.2") {
		t.Fatal("修改共享用户密码后应撤销")
	}
	s.OAuthUsers = []string{"github:someone"}
	m.syncKnockLocked()
	if allowed("203.0.113.3") || !allowed("203.0.113.4") || !allowed("203.0.113.6") {
		t.Fatal("移除第三方身份后应撤销，管理员与报文授权保留")
	}
	// 入口关闭放行或停用：全部撤销
	s.KnockGrant = KnockGrant{}
	m.syncKnockLocked()
	if allowed("203.0.113.4") || !allowed("203.0.113.6") {
		t.Fatal("关闭放行入口后应撤销其放行")
	}
}

// 退出登录时撤销该用户为当前地址登记的放行。
func TestKnockLogoutRevokes(t *testing.T) {
	withKnockRules(t, knockgate.Rule{Key: "k", Name: "nas", Enable: true})
	a := newAuthenticator("secret")
	s := Site{KnockGrant: KnockGrant{Rule: "k", V6Prefix: 64}}
	until := time.Now().Add(time.Hour)
	_ = knockgate.Default.AllowWeb(netip.MustParsePrefix("2001:db8:1:2::/64"), "k", until, knockgate.Owner{User: "alice"}, netip.Addr{})
	_ = knockgate.Default.AllowWeb(netip.MustParsePrefix("2001:db8:1:3::/64"), "k", until, knockgate.Owner{User: "alice"}, netip.Addr{})
	_ = knockgate.Default.AllowWeb(netip.MustParsePrefix("2001:db8:1:4::/64"), "k", until, knockgate.Owner{User: "bob"}, netip.Addr{})
	a.revokeKnockOnLogout(s, netip.MustParseAddr("2001:db8:1:2::77"), "alice")
	a.revokeKnockOnLogout(s, netip.MustParseAddr("2001:db8:1:4::1"), "alice")
	if n := len(knockgate.Default.List()); n != 2 {
		t.Fatalf("只撤销本人当前网段: %d", n)
	}
}

// BenchmarkKnockAdmission 衡量准入判断的开销（准入站点每个请求都会调用）。
func BenchmarkKnockAdmission(b *testing.B) {
	knockgate.Default.Clear()
	knockgate.Default.SetRules([]knockgate.Rule{{Key: "k", Enable: true}})
	b.Cleanup(func() { knockgate.Default.Clear(); knockgate.Default.SetRules(nil) })
	until := time.Now().Add(time.Hour)
	for i := range 5000 {
		_ = knockgate.Default.AllowWeb(netip.PrefixFrom(netip.AddrFrom4([4]byte{198, 51, byte(i >> 8), byte(i)}), 32), "k", until, knockgate.Owner{User: "u"}, netip.Addr{})
	}
	h := &siteHandler{site: Site{Knock: "k", KnockLAN: true}}
	a := netip.MustParseAddr("203.0.113.9")
	b.ResetTimer()
	for range b.N {
		h.knockAdmitted(a)
	}
}

// 运营商级 NAT 段（Tailscale、NetBird 等组网使用）与内网、回环一样不能放行：经它们转发时所有访客都显示为同一地址。
func TestKnockGrantableExcludesCGNAT(t *testing.T) {
	for _, ip := range []string{"100.64.0.1", "100.100.1.2", "100.127.255.254", "10.0.0.1", "127.0.0.1", "::1", "fe80::1"} {
		if knockGrantable(netip.MustParseAddr(ip)) {
			t.Fatalf("%s 不应放行", ip)
		}
	}
	for _, ip := range []string{"100.63.255.255", "100.128.0.1", "203.0.113.9", "2001:db8::1"} {
		if !knockGrantable(netip.MustParseAddr(ip)) {
			t.Fatalf("%s 应可放行", ip)
		}
	}
}

// 管理员身份的授权在站点关闭“后台登录免认证”后失效。
func TestKnockHolderAdmin(t *testing.T) {
	a := newAuthenticator("secret")
	s := Site{AdminPass: true}
	adm := knockgate.Owner{User: adminVisitorPrefix + "admin", Cred: a.knockCred(s, nil, adminVisitorPrefix+"admin")}
	if !a.knockHolderValid(s, nil, adm) {
		t.Fatal("开启后台登录免认证时管理员身份的授权应有效")
	}
	s.AdminPass = false
	if a.knockHolderValid(s, nil, adm) {
		t.Fatal("关闭后台登录免认证后管理员身份的授权应失效")
	}
}
