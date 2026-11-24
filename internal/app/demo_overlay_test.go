package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"
)

// demoGet 以只读演示访客读取接口，解析 JSON。
func demoGet(t *testing.T, c *http.Client, url string) any {
	t.Helper()
	code, body := doJSON(t, c, "GET", url, nil)
	if code != http.StatusOK {
		t.Fatalf("GET %s 返回 %d: %.300s", url, code, body)
	}
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("GET %s 响应不是 JSON: %v %.300s", url, err, body)
	}
	return v
}

// 演示模式下每个被改写的接口都能返回 JSON，启用的条目都带上运行状态，访客页面不会因为缺字段而空白。
func TestDemoOverlayAllEndpoints(t *testing.T) {
	_, srv := newDemoApp(t, 0)
	c := demoClient()
	if code, body := doJSON(t, c, "POST", srv.URL+"/api/login/demo", nil); code != 200 {
		t.Fatalf("一键进入失败 %d %s", code, body)
	}
	paths := make([]string, 0, len(demoXforms)+len(demoFull))
	for p := range demoXforms {
		paths = append(paths, p)
	}
	for p := range demoFull {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		demoGet(t, c, srv.URL+p)
	}

	// 启用的转发规则显示运行中并带统计
	enabledHaveState := func(path, stateField string) {
		t.Helper()
		items := asMaps(demoGet(t, c, srv.URL+path))
		if len(items) == 0 {
			t.Fatalf("%s 演示数据为空", path)
		}
		n := 0
		for _, it := range items {
			if !isOn(it, "enable") {
				continue
			}
			n++
			if _, ok := it[stateField]; !ok {
				t.Errorf("%s 中启用的条目 %s 缺少 %s", path, str(it, "key"), stateField)
			}
		}
		if n == 0 {
			t.Errorf("%s 没有启用的条目，无法检查运行状态", path)
		}
	}
	enabledHaveState("/api/portforward/rules", "stats")
	enabledHaveState("/api/webservice/listeners", "stats")
	enabledHaveState("/api/ddns/tasks", "state")
	enabledHaveState("/api/stun/rules", "state")
	enabledHaveState("/api/frp/clients", "state")
	enabledHaveState("/api/cron/tasks", "history")

	// 监听器里启用的站点都有请求统计
	for _, l := range asMaps(demoGet(t, c, srv.URL+"/api/webservice/listeners")) {
		if !isOn(l, "enable") {
			continue
		}
		stats, _ := l["stats"].(demoMap)
		for _, s := range asMaps(l["sites"]) {
			if isOn(s, "enable") && stats[str(s, "key")] == nil {
				t.Errorf("监听器 %s 的站点 %s 没有统计", str(l, "key"), str(s, "key"))
			}
		}
	}

	// 防护与敲门状态显示防火墙已生效
	guard, _ := demoGet(t, c, srv.URL+"/api/guard/status").(demoMap)
	if fw, _ := guard["firewall"].(demoMap); !isOn(fw, "active") {
		t.Errorf("入侵防护应显示防火墙生效: %v", guard["firewall"])
	}
	knock, _ := demoGet(t, c, srv.URL+"/api/knock/status").(demoMap)
	if !isOn(knock, "listening") {
		t.Errorf("端口敲门应显示监听中: %v", knock)
	}

	// 整体生成的接口结构完整
	conns, _ := demoGet(t, c, srv.URL+"/api/webservice/connections").(demoMap)
	if list, _ := conns["active"].([]any); len(list) == 0 {
		t.Errorf("实时连接应有内容: %v", conns)
	}
	perf, _ := demoGet(t, c, srv.URL+"/api/perf/overview").(demoMap)
	if list, _ := perf["series"].([]any); len(list) == 0 || perf["latest"] == nil {
		t.Errorf("性能概览应有序列与最新值: %v", perf)
	}
	if list, _ := demoGet(t, c, srv.URL+"/api/notify/history").([]any); len(list) == 0 {
		t.Error("通知历史应有内容")
	}

	// 总览摘要换成模拟值：依赖运行状态的模块都有摘要，且没有异常或警告级别
	status, _ := demoGet(t, c, srv.URL+"/api/status").(demoMap)
	seen := map[string]bool{}
	for _, m := range asMaps(status["modules"]) {
		id := str(m, "id")
		for _, it := range asMaps(m["summary"]) {
			seen[id] = true
			if lv := str(it, "level"); lv == "error" || lv == "warn" {
				t.Errorf("演示总览中模块 %s 的摘要 %v 不应是 %s", id, it, lv)
			}
		}
	}
	for _, id := range []string{"portforward", "ddns", "stun", "guard", "knock", "webservice", "frp"} {
		if !seen[id] {
			t.Errorf("演示总览缺少模块 %s 的摘要", id)
		}
	}

	// 演示数据只使用文档保留地址：改写后的响应里也不能出现其他公网地址
	for _, p := range paths {
		_, body := doJSON(t, c, "GET", srv.URL+p, nil)
		if strings.Contains(string(body), "8.8.8.8") || strings.Contains(string(body), "1.1.1.1") {
			t.Errorf("%s 出现了非保留地址: %.200s", p, body)
		}
	}
}

// 不在演示模式时，改写中间件原样放行，响应不带模拟的运行状态。
func TestDemoOverlayPassThroughOutsideDemo(t *testing.T) {
	a := &App{}
	called := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		_, _ = w.Write([]byte(`[{"key":"r1","enable":true}]`))
	})
	h := a.demoOverlay(next)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/portforward/rules", nil))
	if called != 1 || strings.Contains(rec.Body.String(), "running") {
		t.Fatalf("非演示模式应原样放行: %d %s", called, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/perf/overview", nil))
	if called != 2 {
		t.Fatal("非演示模式下整体生成的接口也应交给模块处理")
	}
}

// parseTarget 解析后端地址后的权重与备用标记。
func TestDemoParseTarget(t *testing.T) {
	cases := []struct {
		in     string
		url    string
		weight int
		backup bool
	}{
		{"http://192.0.2.10:8080", "http://192.0.2.10:8080", 1, false},
		{"http://192.0.2.10:8080 weight=3", "http://192.0.2.10:8080", 3, false},
		{"http://192.0.2.11 backup weight=x", "http://192.0.2.11", 1, true},
		{"", "", 1, false},
	}
	for _, c := range cases {
		u, w, b := parseTarget(c.in)
		if u != c.url || w != c.weight || b != c.backup {
			t.Errorf("parseTarget(%q) = %q %d %v", c.in, u, w, b)
		}
	}
}

// 同一条规则的模拟数值在同一时刻多次读取保持一致，速率落在给定范围内。
func TestDemoValuesDeterministic(t *testing.T) {
	if demoHash("r1") != demoHash("r1") || demoHash("r1") == demoHash("r2") {
		t.Fatal("哈希应确定且区分不同 key")
	}
	for _, k := range []string{"a", "b", "pf-ssh", "l-pub/s-git"} {
		if r := demoRate(k, 0.5, 2); r < 0.5 || r > 2 {
			t.Errorf("demoRate(%q) 超出范围: %v", k, r)
		}
		if w := demoWave(k, 60, 1, 3); w < 1 || w > 3 {
			t.Errorf("demoWave(%q) 超出范围: %v", k, w)
		}
	}
}

// 演示模式下公网地址用文档地址代替：管理员查过或自检写入的真实地址不能被演示账号读到。
func TestDemoPublicIPHidesRealAddress(t *testing.T) {
	a, srv := newDemoApp(t, 0)
	a.publicIP.store(publicIPResult{IPv4: "198.51.100.77", IPv6: "2400:1::77", CheckedAt: time.Now()})
	c := demoClient()
	if code, body := doJSON(t, c, "POST", srv.URL+"/api/login/demo", nil); code != 200 {
		t.Fatalf("一键进入失败 %d %s", code, body)
	}
	v := demoGet(t, c, srv.URL+"/api/system/public-ip")
	m, _ := v.(map[string]any)
	if m["ipv4"] != "203.0.113.10" || m["ipv6"] != "2001:db8::10" {
		t.Fatalf("演示模式返回了 %v", v)
	}
}
