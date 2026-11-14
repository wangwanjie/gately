package frp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/iplist"
)

// newAccessModule 创建只含 frps 实例的模块并发布访问控制快照。
func newAccessModule(servers ...Server) *Module {
	m := New()
	m.cfg.Servers = servers
	m.publishAccessLocked()
	return m
}

func TestCheckLoginModes(t *testing.T) {
	set, _, err := iplist.Parse(strings.NewReader("198.51.100.0/24\n"))
	if err != nil {
		t.Fatal(err)
	}
	iplist.Default.Store("t-sub", set)
	t.Cleanup(func() { iplist.Default.Store("t-sub", nil) })

	m := newAccessModule(
		Server{Key: "off", Name: "off"},
		Server{Key: "wl", Name: "wl", ServerSettings: ServerSettings{IPMode: iplist.ModeWhitelist, IPList: []string{"203.0.113.5"}, IPSets: []string{"t-sub"}}},
		Server{Key: "bl", Name: "bl", ServerSettings: ServerSettings{IPMode: iplist.ModeBlacklist, IPSets: []string{"t-sub"}}},
	)
	cases := []struct {
		key, addr string
		allow     bool
	}{
		{"off", "192.0.2.1:5000", true},
		{"wl", "203.0.113.5:5000", true},
		{"wl", "198.51.100.77:5000", true},
		{"wl", "[::ffff:198.51.100.77]:5000", true},
		{"wl", "192.0.2.1:5000", false},
		{"wl", "", false},
		{"bl", "198.51.100.77:5000", false},
		{"bl", "192.0.2.1:5000", true},
		{"bl", "", true},
		{"missing", "192.0.2.1:5000", false},
	}
	for _, c := range cases {
		if got := m.checkLogin(c.key, c.addr) == ""; got != c.allow {
			t.Errorf("%s %q: 放行=%v，期望 %v", c.key, c.addr, got, c.allow)
		}
	}

	// 订阅列表刷新后立即生效，不需要重启实例
	set2, _, _ := iplist.Parse(strings.NewReader("192.0.2.0/24\n"))
	iplist.Default.Store("t-sub", set2)
	if m.checkLogin("wl", "192.0.2.1:5000") != "" {
		t.Error("列表更新后应放行新网段")
	}
	if m.checkLogin("wl", "198.51.100.77:5000") == "" {
		t.Error("列表更新后应拒绝已移出的网段")
	}
}

func TestCheckLoginBanWins(t *testing.T) {
	m := newAccessModule(
		Server{Key: "wl", Name: "wl", ServerSettings: ServerSettings{IPMode: iplist.ModeWhitelist, IPList: []string{"203.0.113.0/24"}}},
		Server{Key: "off", Name: "off"},
	)
	if _, _, err := banlist.Default.Ban("203.0.113.9", banlist.Options{Duration: time.Hour, Force: true, Source: banlist.SourceManual}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { banlist.Default.Unban("203.0.113.9") })
	// 封禁优先于白名单：网段在白名单中，段内被封禁的单个地址仍被拒绝
	if r := m.checkLogin("wl", "203.0.113.9:1234"); !strings.Contains(r, "banned") {
		t.Errorf("被封禁的地址应被拒绝，实际 %q", r)
	}
	if m.checkLogin("wl", "203.0.113.10:1234") != "" {
		t.Error("同网段未封禁的地址应放行")
	}
	// 未启用访问控制的实例同样遵守封禁表
	if m.checkLogin("off", "203.0.113.9:1234") == "" {
		t.Error("未启用访问控制时被封禁的地址也应被拒绝")
	}
}

func TestLoginGateHTTP(t *testing.T) {
	m := newAccessModule(Server{Key: "wl", Name: "wl", ServerSettings: ServerSettings{IPMode: iplist.ModeWhitelist, IPList: []string{"203.0.113.5"}}})
	g, err := startLoginGate(m)
	if err != nil {
		t.Fatal(err)
	}
	defer g.close()

	post := func(path, op, addr string) (int, loginResponse) {
		body, _ := json.Marshal(map[string]any{"version": "0.1.0", "op": op, "content": map[string]any{"user": "u", "client_address": addr}})
		resp, err := http.Post("http://"+g.ln.Addr().String()+path+"?version=0.1.0&op="+op, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out loginResponse
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	path := "/frps/" + g.token + "/wl"
	if code, r := post(path, "Login", "203.0.113.5:40000"); code != 200 || r.Reject || !r.Unchange {
		t.Errorf("白名单内应放行: %d %+v", code, r)
	}
	if code, r := post(path, "Login", "192.0.2.1:40000"); code != 200 || !r.Reject || r.RejectReason == "" {
		t.Errorf("白名单外应拒绝: %d %+v", code, r)
	}
	if n := m.rejectedCount("wl"); n != 1 {
		t.Errorf("拒绝次数 = %d，期望 1", n)
	}
	if code, _ := post("/frps/wrong/wl", "Login", "203.0.113.5:40000"); code != http.StatusNotFound {
		t.Errorf("令牌错误应返回 404，实际 %d", code)
	}

	// 写入 frps 配置的插件段指向该端点
	toml := g.pluginTOML("wl")
	for _, want := range []string{"[[httpPlugins]]", `addr = "` + g.ln.Addr().String() + `"`, `path = "` + path + `"`, `ops = ["Login", "NewProxy"]`} {
		if !strings.Contains(toml, want) {
			t.Errorf("插件配置缺少 %q:\n%s", want, toml)
		}
	}
}

func TestServerContentPlugin(t *testing.T) {
	m := newAccessModule()
	m.started = true
	s := &Server{Key: "k", Name: "frps", Enable: true, ServerSettings: ServerSettings{BindPort: 7000, IPMode: iplist.ModeWhitelist, IPList: []string{"10.0.0.0/8"}}}
	// 登录检查端点不可用时，设置了访问控制的实例不应在无检查的情况下启动
	if c, err := m.serverContent(s); err == nil || c != "" {
		t.Errorf("端点不可用时应返回错误，实际 %q %v", c, err)
	}
	s.IPMode = iplist.ModeOff
	if c, err := m.serverContent(s); err != nil || strings.Contains(c, "httpPlugins") {
		t.Errorf("未设置访问控制时可不带插件启动: %q %v", c, err)
	}
	g, err := startLoginGate(m)
	if err != nil {
		t.Fatal(err)
	}
	defer g.close()
	m.gate = g
	if c, err := m.serverContent(s); err != nil || !strings.Contains(c, "[[httpPlugins]]") {
		t.Errorf("端点可用时应追加登录插件: %q %v", c, err)
	}
}

func TestValidateAccess(t *testing.T) {
	s := ServerSettings{IPMode: iplist.ModeWhitelist}
	if err := s.Validate(); err == nil {
		t.Error("白名单为空应报错")
	}
	s = ServerSettings{IPMode: "allow"}
	if err := s.Validate(); err == nil {
		t.Error("未知模式应报错")
	}
	s = ServerSettings{IPMode: iplist.ModeWhitelist, IPList: []string{"1.2.3"}}
	if err := s.Validate(); err == nil {
		t.Error("无效 IP 应报错")
	}
	s = ServerSettings{IPMode: iplist.ModeBlacklist, IPList: []string{" 1.2.3.4 ", ""}, IPSets: []string{"a", "a", " "}}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(s.IPList) != 1 || s.IPList[0] != "1.2.3.4" || len(s.IPSets) != 1 {
		t.Errorf("规范化结果错误: %+v %+v", s.IPList, s.IPSets)
	}
}

func TestCheckIPSetsAndUsers(t *testing.T) {
	iplist.Default.SetCatalog([]iplist.Info{{Key: "gd", Name: "广东电信"}})
	t.Cleanup(func() {
		iplist.Default.SetCatalog(nil)
		iplist.Default.SetUsers(ID, nil)
	})
	s := &Server{Name: "x", ServerSettings: ServerSettings{IPMode: iplist.ModeWhitelist, IPSets: []string{"gd", "nope"}}}
	if err := checkIPSets(s); err == nil {
		t.Error("引用不存在的列表应报错")
	}
	s.IPSets = []string{"gd"}
	if err := checkIPSets(s); err != nil {
		t.Error(err)
	}

	// 启用访问控制的实例登记为列表的引用方，关闭时不登记
	newAccessModule(
		Server{Key: "a", Name: "家里", ServerSettings: ServerSettings{IPMode: iplist.ModeWhitelist, IPSets: []string{"gd"}}},
		Server{Key: "b", Name: "停用名单", ServerSettings: ServerSettings{IPSets: []string{"gd"}}},
	)
	if got := iplist.Default.Users("gd"); len(got) != 1 || got[0] != "frps / 家里" {
		t.Errorf("引用方 = %v", got)
	}
}

func TestImportKeepsAccess(t *testing.T) {
	m, srv, _ := newTestModule(t, t.TempDir())
	m.cfg.Servers = []Server{{Key: "s1", Name: "frps", ServerSettings: ServerSettings{BindPort: 7000, DashboardAddr: "127.0.0.1", IPMode: iplist.ModeWhitelist, IPList: []string{"10.0.0.0/8"}}}}
	req := ImportRequest{Role: RoleServer, Target: "s1", Apply: true, Content: "bindPort = 7100\n"}
	if code := doJSON(t, "POST", srv.URL+"/api/frp/import", req, nil); code != http.StatusOK {
		t.Fatalf("导入失败: %d", code)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.findServer("s1")
	if s.BindPort != 7100 || s.IPMode != iplist.ModeWhitelist || len(s.IPList) != 1 {
		t.Errorf("导入后应沿用访问控制: %+v", s.ServerSettings)
	}
}

func TestParseClientAddr(t *testing.T) {
	for in, want := range map[string]string{"1.2.3.4:5": "1.2.3.4", "[2001:db8::1]:7000": "2001:db8::1", "1.2.3.4": "1.2.3.4"} {
		a, ok := parseClientAddr(in)
		if !ok || a != netip.MustParseAddr(want) {
			t.Errorf("%q -> %v %v", in, a, ok)
		}
	}
	if _, ok := parseClientAddr("bad"); ok {
		t.Error("无效地址应解析失败")
	}
}

// 优先拒绝名单（#95）：先于白名单、黑名单判断，与访问控制模式无关，订阅列表实时生效，并登记引用方。
func TestCheckLoginDenyFirst(t *testing.T) {
	set, _, _ := iplist.Parse(strings.NewReader("198.51.100.0/24\n"))
	iplist.Default.Store("t-deny", set)
	iplist.Default.SetCatalog([]iplist.Info{{Key: "t-deny", Name: "情报"}})
	t.Cleanup(func() {
		iplist.Default.Store("t-deny", nil)
		iplist.Default.SetCatalog(nil)
		iplist.Default.SetUsers(ID, nil)
	})
	m := newAccessModule(
		Server{Key: "wl", Name: "wl", ServerSettings: ServerSettings{IPMode: iplist.ModeWhitelist, IPList: []string{"203.0.113.0/24", "198.51.100.0/24"}, IPDenyList: []string{"203.0.113.9"}, IPDenySets: []string{"t-deny"}}},
		Server{Key: "off", Name: "off", ServerSettings: ServerSettings{IPDenyList: []string{"10.0.0.0/8"}}},
	)
	cases := []struct {
		key, addr string
		allow     bool
	}{
		{"wl", "203.0.113.5:1", true},
		{"wl", "203.0.113.9:1", false},
		{"wl", "198.51.100.7:1", false},
		{"off", "10.1.1.1:1", false},
		{"off", "8.8.8.8:1", true},
	}
	for _, c := range cases {
		if got := m.checkLogin(c.key, c.addr) == ""; got != c.allow {
			t.Errorf("%s %s: 放行=%v，期望 %v", c.key, c.addr, got, c.allow)
		}
	}
	if r := m.checkLogin("wl", "203.0.113.9:1"); !strings.Contains(r, "deny list") {
		t.Errorf("拒绝原因应说明命中拒绝名单: %q", r)
	}
	// 模式关闭的实例也登记订阅列表引用，并校验引用存在
	m.cfg.Servers[1].IPDenySets = []string{"t-deny"}
	m.publishAccessLocked()
	if users := iplist.Default.Users("t-deny"); len(users) != 2 {
		t.Errorf("引用方 = %v", users)
	}
	if err := checkIPSets(&m.cfg.Servers[1]); err != nil {
		t.Error(err)
	}
	m.cfg.Servers[1].IPDenySets = []string{"t-gone"}
	if checkIPSets(&m.cfg.Servers[1]) == nil {
		t.Error("引用不存在的订阅列表应报错")
	}
	// 订阅列表清空后放行
	iplist.Default.Store("t-deny", nil)
	if m.checkLogin("wl", "198.51.100.7:1") != "" {
		t.Error("列表清空后应放行")
	}
}
