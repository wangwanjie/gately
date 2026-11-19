package app

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/hops"
)

// 经端口转发访问管理界面：路径识别出转发规则，保存前的检查只对影响这条规则访问的修改报风险
func TestAccessPathThroughPortForward(t *testing.T) {
	a, srv, c := loginApp(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	for _, e := range a.modules {
		if err := e.mod.Start(ctx); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = e.mod.Stop() })
	}
	_, adminPort, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	port := freeTCPPort(t)
	rule := map[string]any{
		"name": "entry", "enable": true, "protocols": []string{"tcp4"},
		"listenAddress": "127.0.0.1", "listenPorts": strconv.Itoa(port),
		"targetAddresses": []string{"127.0.0.1"}, "targetPorts": adminPort, "remark": "",
	}
	code, body := doJSON(t, c, "POST", srv.URL+"/api/portforward/rules", rule)
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	var created map[string]any
	_ = json.Unmarshal(body, &created)
	key := created["key"].(string)
	entry := "http://127.0.0.1:" + strconv.Itoa(port)
	// 登录 Cookie 按主机名保存，经转发端口访问时同样带上
	u, _ := url.Parse(entry)
	c.Jar.SetCookies(u, c.Jar.Cookies(mustURL(srv.URL)))
	waitUp(t, entry)

	code, body = doJSON(t, c, "GET", entry+"/api/system/access-path", nil)
	var p accessPath
	if code != 200 || json.Unmarshal(body, &p) != nil {
		t.Fatalf("%d %s", code, body)
	}
	// 测试中的访客就在本机，最外一跳为本机（浏览器或 frp 等程序）
	if len(p.Hops) != 3 || p.Hops[0].Kind != hopLocal || p.Hops[1].Kind != "portforward" || p.Hops[1].Rule != key ||
		p.Hops[1].Name != "entry" || p.Hops[2].Kind != hopAdmin {
		t.Fatalf("应识别出 本机 → 端口转发 → 管理界面: %s", body)
	}
	// 直接访问时没有转发
	_, body = doJSON(t, c, "GET", srv.URL+"/api/system/access-path", nil)
	if strings.Contains(string(body), "portforward") {
		t.Fatalf("直接访问不应经过端口转发: %s", body)
	}

	check := func(method, path string, b any) []pathRisk {
		t.Helper()
		raw, _ := json.Marshal(b)
		code, body := doJSON(t, c, "POST", entry+"/api/system/access-path/check", map[string]any{"method": method, "path": path, "body": json.RawMessage(raw)})
		var res accessCheckResult
		if code != 200 || json.Unmarshal(body, &res) != nil {
			t.Fatalf("%d %s", code, body)
		}
		return res.Risks
	}
	cur := map[string]any{}
	_ = json.Unmarshal(body, &cur)
	edit := func(k string, v any) map[string]any {
		m := map[string]any{}
		for kk, vv := range created {
			m[kk] = vv
		}
		m[k] = v
		return m
	}
	if r := check("PUT", "/api/portforward/rules/"+key, edit("remark", "只改备注")); len(r) != 0 {
		t.Fatalf("只改备注不应报风险: %+v", r)
	}
	if r := check("PUT", "/api/portforward/rules/"+key, edit("targetPorts", "1")); len(r) != 1 || r[0].Kind != riskEdit || r[0].Hop != 1 || r[0].Fields[0] != "targetPorts" {
		t.Fatalf("改目标端口应报修改风险: %+v", r)
	}
	// 改用本机不可用的监听地址：除了修改风险，还说明新地址不可用
	if r := check("PUT", "/api/portforward/rules/"+key, edit("listenAddress", "203.0.113.5")); len(r) != 2 || r[1].Kind != riskListenUnavailable || r[1].Addr != "203.0.113.5" || r[1].Error == "" {
		t.Fatalf("应报告监听地址不可用: %+v", r)
	}
	if r := check("PUT", "/api/portforward/rules/"+key, edit("enable", false)); len(r) != 1 || r[0].Kind != riskDisable {
		t.Fatalf("停用应报风险: %+v", r)
	}
	if r := check("DELETE", "/api/portforward/rules/"+key, nil); len(r) != 1 || r[0].Kind != riskDelete {
		t.Fatalf("删除应报风险: %+v", r)
	}
	if r := check("PUT", "/api/portforward/rules/"+key+"/enable", map[string]bool{"enable": false}); len(r) != 1 || r[0].Kind != riskDisable {
		t.Fatalf("单独停用应报风险: %+v", r)
	}
	if r := check("POST", "/api/portforward/rules-delete", map[string][]string{"keys": {"other", key}}); len(r) != 1 || r[0].Kind != riskDelete {
		t.Fatalf("批量删除包含该规则应报风险: %+v", r)
	}
	if r := check("PUT", "/api/portforward/rules/other", edit("targetPorts", "1")); len(r) != 0 {
		t.Fatalf("其他规则不应报风险: %+v", r)
	}
	if r := check("PUT", "/api/portforward/settings", map[string]any{"enable": false}); len(r) != 1 || r[0].Kind != riskModuleOff {
		t.Fatalf("关闭端口转发应报风险: %+v", r)
	}
	s := a.settingsSnapshot()
	s.DisabledModules = []string{"portforward"}
	if r := check("PUT", "/api/system/settings", s); len(r) != 1 || r[0].Kind != riskModuleOff || r[0].Module != "portforward" {
		t.Fatalf("关闭端口转发模块应报风险: %+v", r)
	}
	s = a.settingsSnapshot()
	s.Listen = "127.0.0.1:1"
	if r := check("PUT", "/api/system/settings", s); len(r) != 1 || r[0].Kind != riskAdminListen {
		t.Fatalf("修改正在使用的管理界面监听应报风险: %+v", r)
	}
	s = a.settingsSnapshot()
	s.LogLines = 3000
	if r := check("PUT", "/api/system/settings", s); len(r) != 0 {
		t.Fatalf("无关设置不应报风险: %+v", r)
	}
	// 命令行 -listen 覆盖时（OpenWrt LuCI 等）：设置接口返回生效中的覆盖地址，与配置文件中的地址不同，
	// 原样保存不应误报；界面上改监听地址只写入配置文件、运行中仍用覆盖值，也不影响当前访问
	oldListen, oldFile := a.opts.Listen, a.fileListen
	t.Cleanup(func() {
		a.opts.Listen, a.fileListen = oldListen, oldFile
	})
	a.opts.Listen, a.fileListen = a.settingsSnapshot().Listen, ":16601"
	s = a.settingsSnapshot()
	if r := check("PUT", "/api/system/settings", s); len(r) != 0 {
		t.Fatalf("-listen 覆盖时原样保存不应报风险: %+v", r)
	}
	s.Listen = "127.0.0.1:1"
	if r := check("PUT", "/api/system/settings", s); len(r) != 0 {
		t.Fatalf("-listen 覆盖时改监听地址不影响当前访问: %+v", r)
	}
}

func mustURL(s string) *url.URL {
	u, _ := url.Parse(s)
	return u
}

// waitUp 等待转发端口开始监听。
func waitUp(t *testing.T, base string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if resp, err := http.Get(base + "/api/demo"); err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("转发端口未就绪")
}

// 比较时空值等价、掩码视为未改、嵌套对象的名称不参与比较
func TestLooseEqual(t *testing.T) {
	for _, tc := range []struct {
		n, c any
		eq   bool
	}{
		{nil, []any{}, true},
		{"********", "secret", true},
		{"a", "b", false},
		{[]any{map[string]any{"name": "x", "port": 1.0}}, []any{map[string]any{"name": "y", "port": 1.0}}, true},
		{[]any{map[string]any{"port": 2.0}}, []any{map[string]any{"port": 1.0}}, false},
	} {
		if got := looseEqual(tc.n, tc.c); got != tc.eq {
			t.Errorf("looseEqual(%v, %v) = %v", tc.n, tc.c, got)
		}
	}
}

// Web 服务这一跳：访客地址等于 TCP 对端时不重复显示；经本机程序（如 frp）转来、Web 服务按可信代理识别出外网访客时补上访客
func TestTraceWebserviceHop(t *testing.T) {
	a, _, _ := loginApp(t)
	trace := func(h hops.Hop) accessPath {
		r := httptest.NewRequest("GET", "/api/system/access-path", nil)
		r.RemoteAddr = "127.0.0.1:40000"
		r.Header.Set(hops.Header, hops.Encode(h))
		return a.traceAccessPath(r)
	}
	p := trace(hops.Hop{Module: "webservice", Rule: "l", Site: "s", Peer: "127.0.0.1:41000", Client: "127.0.0.1:41000"})
	if len(p.Hops) != 3 || p.Hops[0].Kind != hopLocal || p.Hops[1].Kind != "webservice" {
		t.Fatalf("访客地址等于对端时不应另列: %+v", p.Hops)
	}
	p = trace(hops.Hop{Module: "webservice", Rule: "l", Site: "s", Peer: "127.0.0.1:41000", Client: "203.0.113.9:5000"})
	if len(p.Hops) != 4 || p.Hops[0].Kind != hopClient || p.Hops[0].Addr != "203.0.113.9:5000" || p.Hops[1].Kind != hopLocal {
		t.Fatalf("经本机程序转来的外网访客应列在最外: %+v", p.Hops)
	}
	// 本实例两层站点：内层这一跳的 Prev 带着外层，由外向内显示为 访客 → 外层 → 内层 → 管理界面
	p = trace(hops.Hop{Module: "webservice", Rule: "in", Site: "si", Peer: "127.0.0.1:42000",
		Prev: []hops.Hop{{Module: "webservice", Rule: "out", Site: "so", Peer: "198.51.100.8:6000"}}})
	if len(p.Hops) != 4 || p.Hops[0].Kind != hopClient || p.Hops[0].Addr != "198.51.100.8:6000" ||
		p.Hops[1].Rule != "out" || p.Hops[2].Rule != "in" || p.Hops[3].Kind != hopAdmin {
		t.Fatalf("两层站点应逐层显示并从外层的对端继续追溯: %+v", p.Hops)
	}
	// 伪造的路径头无效，按直接访问处理
	r := httptest.NewRequest("GET", "/api/system/access-path", nil)
	r.RemoteAddr = "198.51.100.7:1234"
	r.Header.Set(hops.Header, "e30.forged")
	if p := a.traceAccessPath(r); len(p.Hops) != 2 || p.Hops[0].Addr != "198.51.100.7:1234" {
		t.Fatalf("伪造的路径头应被忽略: %+v", p.Hops)
	}
}
