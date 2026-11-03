package webservice

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// newCopyModule 创建只注册了路由、未启动监听的 Web 服务模块。
func newCopyModule(t *testing.T) (*Module, func(method, path string, body any) (int, map[string]any)) {
	t.Helper()
	m := New(nil, nil)
	st, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	m.Routes(prefixRouter{mux, "/api/webservice"})
	do := func(method, path string, body any) (int, map[string]any) {
		t.Helper()
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	return m, do
}

func redirectSite(key, domain string) Site {
	return Site{Key: key, Enable: true, Domains: []string{domain}, Type: "redirect", RedirectTo: "https://x.example.com"}
}

// 通过接口复制监听：即使请求体原样带上模板的分享链接，副本的站点与默认站点也不带分享链接；
// 站点 key 原样保留（按监听区分），副本排在模板之后，模板的分享链接不受影响。
func TestCopyListenerDropsShareLinks(t *testing.T) {
	m, do := newCopyModule(t)
	share := []ShareLink{{Key: "k1", Name: "临时", Token: "tok-secret"}}
	tpl := Listener{Key: "l1", Name: "HTTP", Port: 18080, Sites: []Site{redirectSite("s1", "a.example.com"), redirectSite("s2", "b.example.com")}}
	tpl.Sites[0].ShareLinks = share
	tpl.Default.ShareLinks = share
	other := Listener{Key: "l2", Name: "其他", Port: 18081}
	m.mu.Lock()
	m.cfg.Listeners = []Listener{tpl, other}
	m.mu.Unlock()

	draft := tpl
	draft.Key, draft.Name, draft.Port = "", "HTTP 副本", 18082
	draft.Sites = []Site{redirectSite("s1", "a.example.com"), redirectSite("s2", "b.example.com")}
	draft.Sites[0].ShareLinks = share
	draft.Default.ShareLinks = share
	code, out := do("POST", "/api/webservice/listeners/l1/copy", draft)
	if code != 200 {
		t.Fatalf("复制监听失败: %d %v", code, out)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.cfg.Listeners) != 3 || m.cfg.Listeners[2].Key != "l2" {
		t.Fatalf("副本应排在模板之后: %+v", m.cfg.Listeners)
	}
	cp := m.cfg.Listeners[1]
	if cp.Key == "l1" || cp.Key == "" || cp.Port != 18082 {
		t.Fatalf("副本应有新 key 并保存修改: %+v", cp)
	}
	for _, s := range append(cp.Sites, cp.Default) {
		if len(s.ShareLinks) != 0 {
			t.Fatalf("副本不应带分享链接: %+v", s)
		}
	}
	if cp.Sites[0].Key != "s1" || cp.Sites[1].Key != "s2" {
		t.Fatalf("站点 key 应原样保留: %s", siteKeys(&cp))
	}
	if len(m.cfg.Listeners[0].Sites[0].ShareLinks) != 1 || len(m.cfg.Listeners[0].Default.ShareLinks) != 1 {
		t.Fatalf("模板的分享链接不应受影响: %+v", m.cfg.Listeners[0])
	}
}

// 引用不存在的 IP 列表的监听副本被拒绝，不新增。
func TestCopyListenerRejectsUnknownIPList(t *testing.T) {
	m, do := newCopyModule(t)
	m.mu.Lock()
	m.cfg.Listeners = []Listener{{Key: "l1", Port: 18080, Sites: []Site{redirectSite("s1", "a.example.com")}}}
	m.mu.Unlock()
	draft := Listener{Port: 18082, Sites: []Site{redirectSite("s1", "a.example.com")}}
	draft.Sites[0].IPSets = []string{"missing"}
	if code, out := do("POST", "/api/webservice/listeners/l1/copy", draft); code != 400 {
		t.Fatalf("引用不存在的 IP 列表应拒绝: %d %v", code, out)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.cfg.Listeners) != 1 {
		t.Fatalf("被拒绝的复制不应新增: %d", len(m.cfg.Listeners))
	}
}

// 复制订阅 IP 列表：请求体带上的下载记录（更新时间、条目数）被清空，由系统重新下载；模板的记录不变。
func TestCopyIPListResetsDownloadRecord(t *testing.T) {
	m, do := newCopyModule(t)
	at := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	m.mu.Lock()
	m.cfg.IPLists = []IPList{{Key: "i1", Name: "国内", URL: "https://example.invalid/cn.txt", RefreshHours: 24, UpdatedAt: at, Count: 100}}
	m.mu.Unlock()
	draft := IPList{Name: "国内 副本", URL: "https://example.invalid/cn2.txt", RefreshHours: 24, UpdatedAt: at, Count: 100}
	if code, out := do("POST", "/api/webservice/iplists/i1/copy", draft); code != 200 {
		t.Fatalf("复制 IP 列表失败: %d %v", code, out)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.cfg.IPLists) != 2 {
		t.Fatalf("应新增副本: %+v", m.cfg.IPLists)
	}
	cp := m.cfg.IPLists[1]
	if !cp.UpdatedAt.IsZero() || cp.Count != 0 || cp.Key == "i1" {
		t.Fatalf("副本的下载记录应清空: %+v", cp)
	}
	if !m.cfg.IPLists[0].UpdatedAt.Equal(at) || m.cfg.IPLists[0].Count != 100 {
		t.Fatalf("模板的下载记录不应改变: %+v", m.cfg.IPLists[0])
	}
}

// 新增监听与新增 IP 列表并发：监听的校验会读取 IP 列表配置，须与复制、修改一样在持锁状态下校验（go test -race 检查）。
func TestCreateListenerValidatesUnderLock(t *testing.T) {
	_, do := newCopyModule(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 20; i++ {
			do("POST", "/api/webservice/iplists", IPList{Name: "l", URL: "https://example.invalid/x.txt", RefreshHours: 24})
		}
	}()
	for i := 0; i < 20; i++ {
		do("POST", "/api/webservice/listeners", Listener{Port: 19000 + i})
	}
	<-done
}
