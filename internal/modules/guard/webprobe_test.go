package guard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/webprobe"
)

func TestCompilePaths(t *testing.T) {
	res, err := compilePaths("*/.env\n/admin/*\n~^/wp-.*\\.php$")
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]bool{
		"/.env":           true,
		"/api/.env":       true,
		"/.ENV":           true, // 通配不区分大小写
		"/.env.bak":       false,
		"//.env":          true, // 规范化后为 /.env
		"/a/../.env":      true,
		"/admin/x":        true,
		"/admin":          false,
		"/wp-login.php":   true,
		"/index.html":     false,
		"/x/wp-login.php": false, // 正则按原样匹配，不自动加前缀
	}
	for p, want := range cases {
		if got := matchPath(res, p); got != want {
			t.Errorf("%s 匹配结果 %v，期望 %v", p, got, want)
		}
	}
	// 多段通配与超长路径：超长路径只取开头与结尾匹配，前缀与后缀规则照常生效
	multi, err := compilePaths("/a*b*c\n/.git*\n*.php")
	if err != nil {
		t.Fatal(err)
	}
	long := "/" + strings.Repeat("x", 100000)
	for p, want := range map[string]bool{
		"/abc": true, "/aXbYc": true, "/acb": false, "/a/b/c": true, "/ab": false,
		"/.git/config": true,
		long + ".php":  true,
		"/.git" + long: true,
		long:           false,
	} {
		if got := matchPath(multi, p); got != want {
			t.Errorf("%.20s…%s（长 %d）匹配结果 %v，期望 %v", p, p[max(0, len(p)-8):], len(p), got, want)
		}
	}
	for _, bad := range []string{"env", "~", "~(", "/a\n.git"} {
		if _, err := compilePaths(bad); err == nil {
			t.Errorf("%q 应校验失败", bad)
		}
	}
}

func TestWebJailValidate(t *testing.T) {
	j := Jail{Name: "probe", Source: SourceWeb, Target: " l1 \n\nl2 ", Filter: "*/.env\n", Ignore: ""}
	if err := j.Validate(); err != nil {
		t.Fatal(err)
	}
	if j.Target != "l1\nl2" || j.Filter != "*/.env" || j.MaxRetry != defaultMaxRetry || j.FindTime != defaultFindTime {
		t.Fatalf("未规范化: %+v", j)
	}
	// 路径规则都可留空，表示全部路径
	if err := (&Jail{Name: "all", Source: SourceWeb}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (&Jail{Name: "bad", Source: SourceWeb, Filter: "env"}).Validate(); err == nil {
		t.Fatal("路径规则不以 / 或 * 开头应校验失败")
	}
	if err := (&Jail{Name: "all", Source: SourceWeb, AnyStatus: true}).Validate(); err == nil {
		t.Fatal("任意状态码且不限路径时应校验失败")
	}
	if err := (&Jail{Name: "host", Source: SourceWeb, AnyStatus: true, UnknownHost: true}).Validate(); err != nil {
		t.Fatalf("只统计未知域名时可以不填路径: %v", err)
	}
	if err := (&Jail{Name: "bad", Source: SourceWeb, Target: "a b"}).Validate(); err == nil {
		t.Fatal("监听 key 含空格应校验失败")
	}
}

func TestWebPresets(t *testing.T) {
	get := func(id string) *webMatcher {
		for _, p := range Presets {
			if p.Preset == id {
				w, err := newWebMatcher(p)
				if err != nil {
					t.Fatal(err)
				}
				return w
			}
		}
		t.Fatalf("缺少预设 %s", id)
		return nil
	}
	decoy := get("web-decoy")
	for _, p := range []string{"/.env", "/backend/.env.local", "/.git/config", "/wp-login.php", "/phpMyAdmin/index.php", "/vendor/phpunit/phpunit/src/Util/PHP/eval-stdin.php"} {
		// 诱饵路径任何状态码都计数
		if !decoy.match(webprobe.Hit{Path: p, Status: 200}) {
			t.Errorf("诱饵路径应命中 %s", p)
		}
	}
	for _, p := range []string{"/", "/index.html", "/.well-known/acme-challenge/x", "/environment"} {
		if decoy.match(webprobe.Hit{Path: p, Status: 404}) {
			t.Errorf("不应命中 %s", p)
		}
	}
	unknown := get("web-unknown-host")
	if !unknown.match(webprobe.Hit{Path: "/", Status: 444, Unknown: true}) || unknown.match(webprobe.Hit{Path: "/x", Status: 404}) {
		t.Fatal("未知域名规则只统计未匹配站点的请求")
	}
	generic := get("web-404")
	if generic.match(webprobe.Hit{Path: "/favicon.ico", Status: 404}) || generic.match(webprobe.Hit{Path: "/app.js.map", Status: 404}) {
		t.Fatal("常见资源缺失不应计数")
	}
	if !generic.match(webprobe.Hit{Path: "/admin.php", Status: 404}) || generic.match(webprobe.Hit{Path: "/admin.php", Status: 200}) {
		t.Fatal("默认只统计 404 与 444")
	}
}

func TestWebJailBans(t *testing.T) {
	m := newTestModule(t)
	add := func(j Jail) {
		t.Helper()
		if err := j.Validate(); err != nil {
			t.Fatal(err)
		}
		m.mu.Lock()
		m.cfg.Jails = append(m.cfg.Jails, j)
		m.startJailLocked(j)
		m.mu.Unlock()
	}
	add(Jail{Key: "w1", Name: "probe", Enable: true, Source: SourceWeb, Target: "l1", Filter: "/x/*", MaxRetry: 3, FindTime: 60})
	hit := func(ip, listener, path string, status int) {
		webprobe.Report(webprobe.Hit{Addr: netip.MustParseAddr(ip), Listener: listener, Method: "GET", Host: "a.example.com", Path: path, Status: status})
	}
	a := netip.MustParseAddr("203.0.113.9")
	// 其他监听、其他路径、200 状态都不计数
	for i := 0; i < 5; i++ {
		hit("203.0.113.9", "l2", "/x/1", 404)
		hit("203.0.113.9", "l1", "/y", 404)
		hit("203.0.113.9", "l1", "/x/1", 200)
	}
	if banlist.Default.Banned(a) {
		t.Fatal("不符合条件的请求不应计数")
	}
	hit("203.0.113.9", "l1", "/x/1", 404)
	hit("203.0.113.9", "l1", "/x/2", 444)
	if banlist.Default.Banned(a) {
		t.Fatal("未达到次数不应封禁")
	}
	hit("::ffff:203.0.113.9", "l1", "/x/3", 404)
	e, ok := banlist.Default.Get("203.0.113.9")
	if !ok || e.Source != banlist.SourceJailPrefix+"w1" || !strings.Contains(e.Reason, "/x/3") {
		t.Fatalf("应由路径探测规则封禁: %+v %v", e, ok)
	}
	// 白名单（内网）地址不计数
	for i := 0; i < 5; i++ {
		hit("192.168.1.5", "l1", "/x/1", 404)
	}
	if banlist.Default.Banned(netip.MustParseAddr("192.168.1.5")) {
		t.Fatal("白名单地址不应被封禁")
	}

	// 停用后不再计数
	m.mu.Lock()
	m.cfg.Jails[0].Enable = false
	m.startJailLocked(m.cfg.Jails[0])
	m.mu.Unlock()
	for i := 0; i < 5; i++ {
		hit("198.51.100.3", "l1", "/x/1", 404)
	}
	if banlist.Default.Banned(netip.MustParseAddr("198.51.100.3")) {
		t.Fatal("停用的规则不应计数")
	}
	// 删除后快照为空；模块停止后取消注册
	m.mu.Lock()
	m.jailRemovedLocked("w1")
	m.mu.Unlock()
	if p := m.webJails.Load(); p == nil || len(*p) != 0 {
		t.Fatal("删除规则后快照应为空")
	}
	add(Jail{Key: "w2", Name: "decoy", Enable: true, Source: SourceWeb, Filter: "*/.env", MaxRetry: 1, FindTime: 60, AnyStatus: true})
	hit("198.51.100.4", "any", "//.env", 200)
	if !banlist.Default.Banned(netip.MustParseAddr("198.51.100.4")) {
		t.Fatal("诱饵路径一次即应封禁")
	}
	_ = m.Stop()
	hit("198.51.100.5", "any", "/.env", 200)
	if banlist.Default.Banned(netip.MustParseAddr("198.51.100.5")) {
		t.Fatal("模块停止后不应再计数")
	}
}

func TestTLSJailValidate(t *testing.T) {
	j := Jail{Name: "tls", Source: SourceTLS, Target: " l1 \n", Filter: "reset\neof\nreset", Ignore: "x", AnyStatus: true}
	if err := j.Validate(); err != nil {
		t.Fatal(err)
	}
	// 类别去重并按固定顺序保存，不使用的字段被清空
	if j.Filter != "eof\nreset" || j.Target != "l1" || j.Ignore != "" || j.AnyStatus || j.MaxRetry != defaultMaxRetry {
		t.Fatalf("未规范化: %+v", j)
	}
	if err := (&Jail{Name: "none", Source: SourceTLS}).Validate(); err == nil {
		t.Fatal("没有选择类型应校验失败，否则会把所有握手失败都算进去")
	}
	if err := (&Jail{Name: "bad", Source: SourceTLS, Filter: "nope"}).Validate(); err == nil {
		t.Fatal("未知类型应校验失败")
	}
	if err := (&Jail{Name: "bad", Source: SourceTLS, Filter: "eof", Target: "a b"}).Validate(); err == nil {
		t.Fatal("监听 key 含空格应校验失败")
	}
	if err := (&Jail{Name: "bad", Source: SourceTLS, Filter: "eof", MaxRetry: 2000}).Validate(); err == nil {
		t.Fatal("次数超限应校验失败")
	}
}

func TestTLSPresets(t *testing.T) {
	for _, id := range []string{"tls-probe", "tls-abort"} {
		found := false
		for _, p := range Presets {
			if p.Preset != id {
				continue
			}
			found = true
			j := p
			if err := j.Validate(); err != nil {
				t.Errorf("预设 %s 无法通过校验: %v", id, err)
			}
			tm, err := newTLSMatcher(j)
			if err != nil {
				t.Fatal(err)
			}
			hit := func(reason string) bool {
				return tm.match(webprobe.TLSFail{Kind: webprobe.ClassifyTLS(reason), Reason: reason})
			}
			if id == "tls-probe" {
				// 明文打 HTTPS、SSLv2、无法协商会计数；中途断开与客户端不信任证书不计数
				if !hit("client sent an HTTP request to an HTTPS server") || !hit("tls: no cipher suite supported by both client and server") {
					t.Error("异常握手预设应命中协议错误与协商失败")
				}
				if hit("EOF") || hit("remote error: tls: bad certificate") {
					t.Error("异常握手预设不应命中断开与证书警报")
				}
			} else {
				if !hit("EOF") || !hit("read tcp 1.1.1.1:443->2.2.2.2:5: read: connection reset by peer") {
					t.Error("握手中断预设应命中 EOF 与重置")
				}
				if hit("client sent an HTTP request to an HTTPS server") {
					t.Error("握手中断预设不应命中协议错误")
				}
			}
		}
		if !found {
			t.Errorf("缺少预设 %s", id)
		}
	}
}

func TestTLSJailBans(t *testing.T) {
	m := newTestModule(t)
	add := func(j Jail) {
		t.Helper()
		if err := j.Validate(); err != nil {
			t.Fatal(err)
		}
		m.mu.Lock()
		m.cfg.Jails = append(m.cfg.Jails, j)
		m.startJailLocked(j)
		m.mu.Unlock()
	}
	add(Jail{Key: "t1", Name: "tls", Enable: true, Source: SourceTLS, Target: "l1", Filter: "eof\nreset", MaxRetry: 3, FindTime: 60})
	fail := func(ip, listener, kind string) {
		webprobe.ReportTLS(webprobe.TLSFail{Addr: netip.MustParseAddr(ip), Listener: listener, Kind: kind, Reason: "reason " + kind})
	}
	a := netip.MustParseAddr("203.0.113.20")
	// 其他监听、未选择的类别都不计数
	for i := 0; i < 6; i++ {
		fail("203.0.113.20", "l2", webprobe.TLSEOF)
		fail("203.0.113.20", "l1", webprobe.TLSProtocol)
		fail("203.0.113.20", "l1", webprobe.TLSAlert)
	}
	if banlist.Default.Banned(a) {
		t.Fatal("不符合条件的握手失败不应计数")
	}
	fail("203.0.113.20", "l1", webprobe.TLSEOF)
	fail("::ffff:203.0.113.20", "l1", webprobe.TLSReset)
	if banlist.Default.Banned(a) {
		t.Fatal("未达到次数不应封禁")
	}
	fail("203.0.113.20", "l1", webprobe.TLSEOF)
	e, ok := banlist.Default.Get("203.0.113.20")
	if !ok || e.Source != banlist.SourceJailPrefix+"t1" || !strings.Contains(e.Reason, "TLS 握手失败") {
		t.Fatalf("应由 TLS 规则封禁: %+v %v", e, ok)
	}
	// 其他地址互不影响；白名单地址不计数
	for i := 0; i < 5; i++ {
		fail("192.168.1.9", "l1", webprobe.TLSEOF)
	}
	if banlist.Default.Banned(netip.MustParseAddr("192.168.1.9")) {
		t.Fatal("白名单地址不应被封禁")
	}
	// 控制字符被转义，不会伪造日志行与通知内容
	add(Jail{Key: "t2", Name: "tls2", Enable: true, Source: SourceTLS, Filter: "nocert", MaxRetry: 1, FindTime: 60})
	webprobe.ReportTLS(webprobe.TLSFail{Addr: netip.MustParseAddr("198.51.100.8"), Kind: webprobe.TLSNoCert, Reason: "x\nfake"})
	e, ok = banlist.Default.Get("198.51.100.8")
	if !ok || strings.Contains(e.Reason, "\n") {
		t.Fatalf("原因应转义换行: %+v %v", e, ok)
	}

	// 停用后不再计数；删除后快照为空
	m.mu.Lock()
	m.cfg.Jails[0].Enable = false
	m.startJailLocked(m.cfg.Jails[0])
	m.mu.Unlock()
	for i := 0; i < 5; i++ {
		fail("198.51.100.3", "l1", webprobe.TLSEOF)
	}
	if banlist.Default.Banned(netip.MustParseAddr("198.51.100.3")) {
		t.Fatal("停用的规则不应计数")
	}
	m.mu.Lock()
	m.jailRemovedLocked("t1")
	m.jailRemovedLocked("t2")
	m.mu.Unlock()
	if p := m.tlsJails.Load(); p == nil || len(*p) != 0 {
		t.Fatal("删除规则后快照应为空")
	}
	// 模块停止后取消注册
	_ = m.Stop()
	if webprobe.TLSEnabled() {
		t.Fatal("模块停止后应取消注册")
	}
}

// TestTLSTestEndpoint 验证规则测试接口按类别判断握手失败原因。
func TestTLSTestEndpoint(t *testing.T) {
	m := newTestModule(t)
	body, _ := json.Marshal(testRequest{Source: SourceTLS, Filter: "eof\nprotocol", Lines: "EOF\nclient sent an HTTP request to an HTTPS server\nremote error: tls: bad certificate"})
	rec := httptest.NewRecorder()
	m.handleTest(rec, jsonReq(body))
	var resp []TestResult
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("%v: %s", err, rec.Body.String())
	}
	if len(resp) != 3 || !resp[0].Matched || !resp[1].Matched || resp[2].Matched || resp[2].Kind != webprobe.TLSAlert {
		t.Fatalf("测试结果不符: %+v", resp)
	}
	// 未知类型报 400
	bad, _ := json.Marshal(testRequest{Source: SourceTLS, Filter: "nope", Lines: "EOF"})
	rec = httptest.NewRecorder()
	m.handleTest(rec, jsonReq(bad))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("未知类型应返回 400，实际 %d", rec.Code)
	}
}

func jsonReq(body []byte) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/jails/test", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	return r
}
