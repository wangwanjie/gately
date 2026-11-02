package webservice

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/iplist"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

func TestIPListSubscription(t *testing.T) {
	var body atomic.Value
	body.Store("# 本机\n127.0.0.0/8\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, body.Load().(string))
	}))
	defer srv.Close()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer backend.Close()

	dir := t.TempDir()
	st, _ := config.Open(dir)
	m := New(nil, nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	const key = "test-local"
	t.Cleanup(func() { iplist.Default.Store(key, nil) })
	l := IPList{Key: key, Name: "local", Enable: true, URL: srv.URL}
	if err := l.Validate(); err != nil || l.RefreshHours != 24 {
		t.Fatalf("应补全默认刷新间隔: %v %+v", err, l)
	}
	m.cfg.IPLists = []IPList{l}
	m.refreshDueIPLists(context.Background())
	set, ok := iplist.Default.Get(key)
	if !ok || !set.Contains(netip.MustParseAddr("127.0.0.1")) || m.cfg.IPLists[0].Count != 1 || m.cfg.IPLists[0].UpdatedAt.IsZero() {
		t.Fatalf("下载后应载入列表并记录状态: %+v", m.cfg.IPLists[0])
	}
	if _, err := os.Stat(m.ipListPath(key)); err != nil {
		t.Fatalf("应缓存列表内容: %v", err)
	}

	// 白名单引用该列表时本机可访问，黑名单引用时被拒绝；未知列表在保存时报错
	listener := Listener{Sites: []Site{
		{Enable: true, Domains: []string{"white.test"}, Targets: []string{backend.URL}, IPMode: IPModeWhitelist, IPSets: []string{key}},
		{Enable: true, Domains: []string{"black.test"}, Targets: []string{backend.URL}, IPMode: IPModeBlacklist, IPSets: []string{key}},
		{Enable: true, Domains: []string{"rule.test"}, Targets: []string{backend.URL},
			Rules: []PathRule{{Prefix: "/admin", IPMode: IPModeBlacklist, IPSets: []string{key}}}},
	}}
	if err := m.checkIPSetsLocked(&listener); err != nil {
		t.Fatal(err)
	}
	addr, _ := startRunner(t, listener, nil, nil)
	for _, c := range []struct {
		host, path string
		want       int
	}{{"white.test", "/", 200}, {"black.test", "/", 403}, {"rule.test", "/", 200}, {"rule.test", "/admin/x", 403}} {
		if resp, _ := get(t, http.DefaultClient, addr, c.host, c.path, nil); resp.StatusCode != c.want {
			t.Errorf("%s%s 应返回 %d，实际 %d", c.host, c.path, c.want, resp.StatusCode)
		}
	}
	bad := Listener{Sites: []Site{{Domains: []string{"x.test"}, IPSets: []string{"missing"}}}}
	if err := m.checkIPSetsLocked(&bad); err == nil {
		t.Fatal("引用不存在的列表应报错")
	}
	m.cfg.Listeners = []Listener{listener}
	if users := m.ipListUsersLocked(key); len(users) != 3 {
		t.Fatalf("应列出引用该列表的站点: %v", users)
	}

	// 下载到无效内容时报错并保留原列表
	body.Store("<html>502 Bad Gateway</html>")
	m.ipListStateLocked(key).refreshing = true
	if err := m.refreshIPList(context.Background(), m.cfg.IPLists[0]); err == nil {
		t.Fatal("无效内容应报错")
	}
	if set, ok := iplist.Default.Get(key); !ok || !set.Contains(netip.MustParseAddr("127.0.0.1")) {
		t.Fatal("刷新失败时应保留原列表")
	}
	if st := m.ipListStateLocked(key); st.lastError == "" || st.nextRetry.IsZero() {
		t.Fatalf("刷新失败应记录错误并退避: %+v", st)
	}

	// 重启后从缓存载入，无需联网
	iplist.Default.Store(key, nil)
	st2, _ := config.Open(dir)
	m2 := New(nil, nil)
	if err := m2.Init(module.Env{Store: st2, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	if set, ok := iplist.Default.Get(key); !ok || set.Len() != 1 {
		t.Fatal("重启后应从缓存载入列表")
	}

	// 载入时发布列表目录，供 frp 等模块校验引用；其它模块登记的引用方计入列表的使用者
	if name, ok := iplist.Default.Name(key); !ok || name != "local" {
		t.Fatalf("应发布列表目录: %q %v", name, ok)
	}
	iplist.Default.SetUsers("frp", map[string][]string{key: {"frps / 家里"}})
	t.Cleanup(func() { iplist.Default.SetUsers("frp", nil) })
	if users := m2.ipListUsersLocked(key); len(users) != 1 || users[0] != "frps / 家里" {
		t.Fatalf("应包含其它模块的引用方: %v", users)
	}
}

func TestIPListProxy(t *testing.T) {
	var proxied atomic.Int32
	// 正向代理：只代理 http:// 目标，返回固定列表，确认请求确实经过了它
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !r.URL.IsAbs() {
			http.Error(w, "not proxy", http.StatusBadRequest)
			return
		}
		proxied.Add(1)
		_, _ = io.WriteString(w, "10.0.0.0/8\n")
	}))
	defer proxy.Close()

	for _, bad := range []string{"127.0.0.1:7890", "ftp://x"} {
		l := IPList{Name: "x", URL: "http://example.invalid/list", Proxy: bad}
		if l.Validate() == nil {
			t.Errorf("代理 %q 应被拒绝", bad)
		}
	}
	l := IPList{Key: "p", Name: "p", Enable: true, URL: "http://lists.example.invalid/cn.txt", Proxy: " " + proxy.URL + " "}
	if err := l.Validate(); err != nil || l.Proxy != proxy.URL {
		t.Fatalf("代理应通过校验并去掉空白: %q %v", l.Proxy, err)
	}
	st, _ := config.Open(t.TempDir())
	m := New(nil, nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	set, n, err := m.downloadIPList(context.Background(), l)
	if err != nil || n != 1 || !set.Contains(netip.MustParseAddr("10.1.2.3")) || proxied.Load() != 1 {
		t.Fatalf("应经代理下载列表: n=%d err=%v 代理次数=%d", n, err, proxied.Load())
	}
}

func TestGeoIPDownloadProxy(t *testing.T) {
	m := New(nil, nil)
	st, _ := config.Open(t.TempDir())
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	var proxied atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied.Add(1)
		_, _ = io.WriteString(w, "not a database")
	}))
	defer proxy.Close()
	do := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/geoip/download", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		m.handleGeoIPDownload(w, req)
		return w
	}
	if w := do(`{"url":"http://geo.example.invalid/x.mmdb","proxy":"1.2.3.4:80"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("无效代理应返回 400: %d %s", w.Code, w.Body.String())
	}
	// 经代理下载：代理返回的不是有效数据库，报错但确认请求经过了代理
	if w := do(`{"url":"http://geo.example.invalid/x.mmdb","proxy":"` + proxy.URL + `"}`); w.Code != http.StatusBadGateway || proxied.Load() != 1 {
		t.Fatalf("应经代理下载: %d %s 代理次数=%d", w.Code, w.Body.String(), proxied.Load())
	}
}
