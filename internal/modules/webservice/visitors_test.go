package webservice

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/visitors"
)

// startVisitRunner 启动一个上报访客审计的监听器，返回地址与访客存储。
func startVisitRunner(t *testing.T, l Listener) (string, *visitors.Store) {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	l.Port = ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	l.ListenIP, l.Network = "127.0.0.1", "tcp4"
	if err := l.Validate(); err != nil {
		t.Fatal(err)
	}
	auth := newAuthenticator("secret")
	store := visitors.New("", t.Logf)
	auth.visits = store
	r, err := newListenerRunner(l, logbuf.New(200, log.New(io.Discard, "", 0)).Logger("t"), auth, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.start(nil, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.stop)
	return fmt.Sprintf("127.0.0.1:%d", l.Port), store
}

func TestVisitorsRecordedFromRequests(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer backend.Close()
	l := Listener{Key: "lk", Name: "HTTP", Sites: []Site{
		{Key: "app", Name: "应用", Enable: true, Domains: []string{"app.test"}, Targets: []string{backend.URL}},
		{Key: "auth", Enable: true, Domains: []string{"auth.test"}, Targets: []string{backend.URL}, BasicAuth: true, Users: []string{"alice:secret"}},
		{Key: "deny", Enable: true, Domains: []string{"deny.test"}, Targets: []string{backend.URL}, IPMode: IPModeBlacklist, IPList: []string{"127.0.0.0/8"}},
	}}
	addr, store := startVisitRunner(t, l)
	c := http.DefaultClient
	get(t, c, addr, "app.test", "/hello?token=SECRET", map[string]string{"User-Agent": "TestAgent/1.0"})
	req, _ := http.NewRequest("GET", "http://"+addr+"/private", nil)
	req.Host = "auth.test"
	req.SetBasicAuth("alice", "secret")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	// 密码错误的基本认证不能记成身份
	req, _ = http.NewRequest("GET", "http://"+addr+"/private", nil)
	req.Host = "auth.test"
	req.SetBasicAuth("mallory", "wrong")
	if resp, err = c.Do(req); err == nil {
		resp.Body.Close()
	}
	if resp, _ := get(t, c, addr, "deny.test", "/x", nil); resp.StatusCode != 403 {
		t.Fatalf("黑名单应返回 403: %d", resp.StatusCode)
	}
	// 没有默认站点时，未知域名在监听层返回 404，同样记为未匹配站点
	if resp, _ := get(t, c, addr, "unknown.test", "/.env", nil); resp.StatusCode != 404 {
		t.Fatalf("未知域名应返回 404: %d", resp.StatusCode)
	}

	v, ok := store.Get(netip.MustParseAddr("127.0.0.1"))
	if !ok {
		t.Fatal("应记录访客")
	}
	if v.Requests != 5 || v.Denied != 1 || v.Noise != 2 {
		t.Fatalf("计数不对: req=%d denied=%d noise=%d", v.Requests, v.Denied, v.Noise)
	}
	if len(v.Identities) != 1 || v.Identities[0].Name != "alice" || v.Identities[0].Kind != "user" {
		t.Fatalf("身份 = %+v", v.Identities)
	}
	if len(v.Deny) != 1 || v.Deny[0].Name != "ip" {
		t.Fatalf("拦截原因 = %+v", v.Deny)
	}
	sites := map[string]visitors.SiteStat{}
	for _, st := range v.Sites {
		key := st.Key
		if st.Unmatched {
			key = "-"
		}
		sites[key] = st
		if st.Listener != "lk" {
			t.Fatalf("监听 key = %q", st.Listener)
		}
	}
	if sites["app"].Name != "应用" || sites["auth"].Req != 2 || sites["deny"].Denied != 1 || sites["-"].Req != 1 {
		t.Fatalf("站点计数不对: %+v", v.Sites)
	}
	for _, p := range v.Paths {
		if strings.Contains(p.Path, "SECRET") || strings.Contains(p.Path, "?") {
			t.Fatalf("不应记录查询串: %q", p.Path)
		}
	}
	if v.LastReq.Path != "/.env" || v.LastReq.Status != 404 || v.LastReq.Host != "unknown.test" {
		t.Fatalf("最近请求 = %+v", v.LastReq)
	}
	found := false
	for _, a := range v.Agents {
		found = found || a.UA == "TestAgent/1.0"
	}
	if !found {
		t.Fatalf("User-Agent = %+v", v.Agents)
	}
}

func TestVisitorsDefaultSiteAndAdmin(t *testing.T) {
	httpx.SetSessionCheck(func(r *http.Request) string {
		if c, err := r.Cookie("gately_token"); err == nil && c.Value == "admin-ok" {
			return "root"
		}
		return ""
	})
	t.Cleanup(func() { httpx.SetSessionCheck(nil) })
	l := Listener{Key: "lk", Sites: []Site{
		{Key: "app", Enable: true, Domains: []string{"app.test"}, Type: TypeText, Text: "hi", StatusCode: 200},
		{Key: "deny", Enable: true, Domains: []string{"deny.test"}, Type: TypeText, Text: "hi", StatusCode: 200, IPMode: IPModeBlacklist, IPList: []string{"127.0.0.1"}},
	}, Default: Site{Enable: true, Type: TypeClose}}
	addr, store := startVisitRunner(t, l)
	cookie := map[string]string{"Cookie": "gately_token=admin-ok"}
	get(t, http.DefaultClient, addr, "app.test", "/", cookie)
	// 被拦截的请求即使带后台登录态也不记为管理员
	get(t, http.DefaultClient, addr, "deny.test", "/", cookie)
	// 默认站点为断开连接：请求失败，但访客照样记录为未匹配站点、444
	req, _ := http.NewRequest("GET", "http://"+addr+"/wp-login.php", nil)
	req.Host = "scan.test"
	if resp, err := http.DefaultClient.Do(req); err == nil {
		resp.Body.Close()
		t.Fatal("断开连接的默认站点不应返回响应")
	}
	v, ok := store.Get(netip.MustParseAddr("127.0.0.1"))
	if !ok {
		t.Fatal("应记录访客")
	}
	// 连接被断开时 Go 客户端会重试一次幂等请求，未匹配站点的请求至少 1 次
	if v.Requests < 3 || v.Closed < 1 || uint64(v.Closed) != v.Requests-2 {
		t.Fatalf("计数不对: %+v", v)
	}
	if len(v.Identities) != 1 || v.Identities[0].Kind != "admin" || v.Identities[0].Name != "root" || v.Identities[0].Req != 1 {
		t.Fatalf("管理员身份 = %+v", v.Identities)
	}
	unmatched := false
	for _, st := range v.Sites {
		if st.Unmatched {
			unmatched = true
			if st.Key != "" || st.Name != "" {
				t.Fatalf("未匹配站点不应带站点信息: %+v", st)
			}
		}
	}
	if !unmatched {
		t.Fatal("默认站点处理的请求应记为未匹配站点")
	}

	// 停用后不再记录
	store.Apply(visitors.Settings{Disable: true, Days: 30, Max: 1000})
	get(t, http.DefaultClient, addr, "app.test", "/", nil)
	if v2, _ := store.Get(netip.MustParseAddr("127.0.0.1")); v2.Requests != v.Requests {
		t.Fatalf("停用后不应记录: %d", v2.Requests)
	}
}

// newVisitorsModule 初始化一个只用于接口测试的模块。
func newVisitorsModule(t *testing.T, dir string) (*Module, *http.ServeMux) {
	t.Helper()
	st, err := config.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := New(nil, nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	m.Routes(secretsRouter{mux})
	return m, mux
}

func call(t *testing.T, mux *http.ServeMux, method, path, body string, out any) int {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if out != nil && rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("%s %s: %v %s", method, path, err, rec.Body.String())
		}
	}
	return rec.Code
}

func TestVisitorsAPI(t *testing.T) {
	dir := t.TempDir()
	m, mux := newVisitorsModule(t, dir)
	m.mu.Lock()
	m.cfg.Listeners = []Listener{{Key: "lk", Name: "HTTPS", Port: 443}}
	m.mu.Unlock()
	m.visits.Record(visitors.Hit{Addr: netip.MustParseAddr("203.0.113.1"), Listener: "lk", Site: "s", SiteName: "博客", Path: "/a", Status: 200, User: "alice"})
	m.visits.Record(visitors.Hit{Addr: netip.MustParseAddr("192.168.1.2"), Listener: "lk", Unmatched: true, Path: "/.env", Status: 404})

	var list struct {
		Items []struct {
			IP      string `json:"ip"`
			Country string `json:"country"`
		} `json:"items"`
		Total     int               `json:"total"`
		Listeners map[string]string `json:"listeners"`
		Info      visitors.Info     `json:"info"`
		Summary   visitors.Summary  `json:"summary"`
	}
	if code := call(t, mux, "GET", "/visitors?days=7&sort=requests", "", &list); code != 200 || list.Total != 2 || list.Listeners["lk"] != "HTTPS" ||
		list.Info.Count != 2 || list.Info.Days != visitors.DefaultDays || list.Summary.Authed != 1 {
		t.Fatalf("列表: %d %+v", code, list)
	}
	for _, it := range list.Items {
		if it.IP == "192.168.1.2" && it.Country != "LAN" {
			t.Fatalf("内网地址应标为 LAN: %+v", it)
		}
	}
	if call(t, mux, "GET", "/visitors?q=alice&filter=authed", "", &list); list.Total != 1 || list.Items[0].IP != "203.0.113.1" {
		t.Fatalf("搜索: %+v", list)
	}

	var detail struct {
		IP         string               `json:"ip"`
		Identities []visitors.IdentStat `json:"identities"`
		Daily      []visitors.DayPoint  `json:"daily"`
		Listeners  map[string]string    `json:"listeners"`
		Sites      []visitors.SiteStat  `json:"sites"`
	}
	if code := call(t, mux, "GET", "/visitors/detail?ip=203.0.113.1", "", &detail); code != 200 || detail.IP != "203.0.113.1" ||
		len(detail.Identities) != 1 || len(detail.Daily) != visitors.DefaultDays || detail.Listeners["lk"] != "HTTPS" {
		t.Fatalf("详情: %d %+v", code, detail)
	}
	if code := call(t, mux, "GET", "/visitors/detail?ip=nope", "", nil); code != 400 {
		t.Fatalf("无效地址应 400: %d", code)
	}
	if code := call(t, mux, "GET", "/visitors/detail?ip=8.8.8.8", "", nil); code != 404 {
		t.Fatalf("不存在应 404: %d", code)
	}

	// 设置：校验范围、保存到配置文件
	if code := call(t, mux, "PUT", "/visitors/settings", `{"days":0,"max":50}`, nil); code != 400 {
		t.Fatalf("上限过小应 400: %d", code)
	}
	if code := call(t, mux, "PUT", "/visitors/settings", `{"days":999}`, nil); code != 400 {
		t.Fatalf("天数过大应 400: %d", code)
	}
	var info visitors.Info
	if code := call(t, mux, "PUT", "/visitors/settings", `{"days":7,"max":500}`, &info); code != 200 || info.Days != 7 || info.Max != 500 {
		t.Fatalf("保存设置: %d %+v", code, info)
	}
	raw, _ := os.ReadFile(dir + "/webservice.json")
	if !strings.Contains(string(raw), `"visitors"`) {
		t.Fatal("设置应写入配置文件")
	}
	// 修改设置时立即写盘
	if _, err := os.Stat(m.visitorsPath()); err != nil {
		t.Fatalf("应写出数据文件: %v", err)
	}

	// 删除：参数校验与按地址删除
	if code := call(t, mux, "POST", "/visitors/delete", `{}`, nil); code != 400 {
		t.Fatalf("空列表应 400: %d", code)
	}
	if code := call(t, mux, "POST", "/visitors/delete", `{"ips":["bad"]}`, nil); code != 400 {
		t.Fatalf("无效地址应 400: %d", code)
	}
	var del map[string]int
	if code := call(t, mux, "POST", "/visitors/delete", `{"ips":["::ffff:203.0.113.1"]}`, &del); code != 200 || del["deleted"] != 1 {
		t.Fatalf("删除: %d %v", code, del)
	}

	// 模块停止时写盘，重新初始化后读回数据与设置
	m.visits.Record(visitors.Hit{Addr: netip.MustParseAddr("198.51.100.9"), Path: "/", Status: 200})
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
	m2, mux2 := newVisitorsModule(t, dir)
	if m2.visits.Len() != 2 {
		t.Fatalf("重启后访客数 = %d", m2.visits.Len())
	}
	if code := call(t, mux2, "GET", "/visitors/settings", "", &info); code != 200 || info.Days != 7 || info.Max != 500 || info.FileBytes == 0 {
		t.Fatalf("重启后设置: %+v", info)
	}
	if code := call(t, mux2, "POST", "/visitors/delete", `{"all":true}`, &del); code != 200 || del["deleted"] != 2 || m2.visits.Len() != 0 {
		t.Fatalf("清空: %v", del)
	}
}

func TestVisitorsInvalidSettingsFallBack(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/webservice.json", []byte(`{"settings":{"enable":true},"visitors":{"days":-5,"max":3,"disable":true}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	m, _ := newVisitorsModule(t, dir)
	set := m.visits.Settings()
	if set.Days != visitors.DefaultDays || set.Max != visitors.DefaultMax || !set.Disable {
		t.Fatalf("无效设置应改用默认值并保留停用: %+v", set)
	}
}
