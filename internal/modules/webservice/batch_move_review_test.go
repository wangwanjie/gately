package webservice

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// 保存失败时整体回滚：内存配置保持原样，分享链接计数不迁移，也不触发移动回调。
func TestSiteBatchMoveSaveFailure(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("需要非 root 的类 Unix 环境才能用只读目录模拟保存失败")
	}
	m := batchModule(t)
	called := false
	m.OnSitesMoved(func(map[string]string) {
		called = true
	})
	m.auth.shareUses["l1/a|once"] = 1
	dir := m.store.Dir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o700)
	})
	code, _, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchMove, Move: moveTo("l2")})
	if code != http.StatusInternalServerError {
		t.Fatalf("保存失败应返回 500: %d %s", code, body)
	}
	if !slices.Equal(m.siteKeys(t, "l1"), []string{"a", "b", "c"}) || !slices.Equal(m.siteKeys(t, "l2"), []string{"x"}) {
		t.Fatalf("保存失败后配置应回滚: %v %v", m.siteKeys(t, "l1"), m.siteKeys(t, "l2"))
	}
	if called {
		t.Fatal("保存失败时不应触发移动回调")
	}
	if m.auth.shareUses["l1/a|once"] != 1 || len(m.auth.shareUses) != 1 {
		t.Fatalf("保存失败时分享链接计数不应迁移: %v", m.auth.shareUses)
	}
}

// 一次移动多个来自不同监听、域名相同的站点：移入后在目标监听内重复，整体拒绝。
func TestSiteBatchMoveDomainConflictAmongMoved(t *testing.T) {
	m := batchModule(t)
	l3 := Listener{Key: "l3", Name: "third", ListenIP: "127.0.0.1", Port: 18083, Sites: []Site{
		{Key: "y", Enable: true, Domains: []string{"keep.org"}, Type: TypeText, Text: "y"},
	}}
	if err := l3.Validate(); err != nil {
		t.Fatal(err)
	}
	m.cfg.Listeners = append(m.cfg.Listeners, l3)
	// l1/a 与 l3/y 都有 keep.org，一起移到 l2 时冲突
	code, _, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "a", "l3", "y"), Action: BatchMove, Move: moveTo("l2")})
	if code != http.StatusBadRequest {
		t.Fatalf("移入的站点之间域名重复应拒绝: %d %s", code, body)
	}
	if !slices.Equal(m.siteKeys(t, "l2"), []string{"x"}) || len(m.siteKeys(t, "l1")) != 3 || len(m.siteKeys(t, "l3")) != 1 {
		t.Fatalf("拒绝后不应修改: %v %v %v", m.siteKeys(t, "l1"), m.siteKeys(t, "l2"), m.siteKeys(t, "l3"))
	}
}

// 来自多个源监听的站点一起移动：各自从源监听删除，按提交顺序放入目标；同名 key 换用新 key 且彼此不重复。
func TestSiteBatchMoveFromSeveralListeners(t *testing.T) {
	m := batchModule(t)
	l3 := Listener{Key: "l3", Name: "third", ListenIP: "127.0.0.1", Port: 18083, Sites: []Site{
		{Key: "a", Enable: true, Domains: []string{"a3.test"}, Type: TypeText, Text: "a3"},
	}}
	if err := l3.Validate(); err != nil {
		t.Fatal(err)
	}
	m.cfg.Listeners = append(m.cfg.Listeners, l3)
	code, res, body := m.batch(t, SiteBatchRequest{Sites: refs("l3", "a", "l1", "a"), Action: BatchMove, Move: moveTo("l2")})
	if code != 200 || len(res.Moved) != 2 {
		t.Fatalf("移动失败: %d %s", code, body)
	}
	got := m.siteKeys(t, "l2")
	if len(got) != 3 || got[0] != "x" || got[1] != "a" || got[2] == "a" || got[2] == "x" {
		t.Fatalf("目标监听的 key 应互不重复且按提交顺序: %v", got)
	}
	if m.siteOf(t, "l2", got[1]).Text != "a3" || m.siteOf(t, "l2", got[2]).Text != "a" {
		t.Fatalf("站点内容与顺序不符: %v", got)
	}
	if len(m.siteKeys(t, "l3")) != 0 || !slices.Equal(m.siteKeys(t, "l1"), []string{"b", "c"}) {
		t.Fatalf("源监听应删除移出的站点: %v %v", m.siteKeys(t, "l1"), m.siteKeys(t, "l3"))
	}
}

// 相邻的两次移动（l1/a→l2，紧接着 l2/a→l3）：回调按移动的先后顺序执行，
// 否则引用方（性能监控告警规则）会先处理后一次映射、再处理前一次，最终停在已不存在的 l2/a 上。
func TestSiteBatchMoveHooksOrdered(t *testing.T) {
	m := batchModule(t)
	l3 := Listener{Key: "l3", Name: "third", ListenIP: "127.0.0.1", Port: 18083}
	if err := l3.Validate(); err != nil {
		t.Fatal(err)
	}
	m.cfg.Listeners = append(m.cfg.Listeners, l3)
	entered := make(chan struct{})
	release := make(chan struct{})
	var mu sync.Mutex
	var order []string
	m.OnSitesMoved(func(renamed map[string]string) {
		if _, first := renamed["l1/a"]; first {
			close(entered)
			<-release
		}
		mu.Lock()
		for k, v := range renamed {
			order = append(order, k+">"+v)
		}
		mu.Unlock()
	})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if code, _, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchMove, Move: moveTo("l2")}); code != 200 {
			t.Errorf("第一次移动失败: %d %s", code, body)
		}
	}()
	<-entered
	second := make(chan int, 1)
	go func() {
		defer wg.Done()
		code, _, _ := m.batch(t, SiteBatchRequest{Sites: refs("l2", "a"), Action: BatchMove, Move: moveTo("l3")})
		second <- code
	}()
	// 第一次移动的回调尚未结束，第二次移动必须等待，不能抢先执行回调
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	early := len(order)
	mu.Unlock()
	close(release)
	wg.Wait()
	if early != 0 {
		t.Fatalf("前一次移动的回调结束前，后一次移动的回调不应执行: %v", order)
	}
	if code := <-second; code != 200 {
		t.Fatalf("第二次移动失败: %d", code)
	}
	if !slices.Equal(order, []string{"l1/a>l2/a", "l2/a>l3/a"}) {
		t.Fatalf("回调顺序错误: %v", order)
	}
}

// 网页认证站点移到其他监听后，旧监听签发的登录会话在新位置失效：
// 即使把 Cookie 改成新位置的名称重放，会话中的作用域仍是旧的“监听/站点”，校验不通过。
func TestSiteBatchMoveWebAuthSessionScoped(t *testing.T) {
	st, _ := config.Open(t.TempDir())
	m := New(nil, nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	port := func() int {
		ln, _ := net.Listen("tcp4", "127.0.0.1:0")
		defer ln.Close()
		return ln.Addr().(*net.TCPAddr).Port
	}
	p1, p2 := port(), port()
	l1 := Listener{Key: "l1", Enable: true, ListenIP: "127.0.0.1", Network: "tcp4", Port: p1, Sites: []Site{
		{Key: "a", Enable: true, Domains: []string{"a.test"}, Type: TypeText, Text: "site-a", WebAuth: true, Users: []string{"u:p"}},
	}}
	l2 := Listener{Key: "l2", Enable: true, ListenIP: "127.0.0.1", Network: "tcp4", Port: p2}
	for _, l := range []*Listener{&l1, &l2} {
		if err := l.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	m.cfg.Settings.Enable = true
	m.cfg.Listeners = []Listener{l1, l2}
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop() })
	a1, a2 := fmt.Sprintf("127.0.0.1:%d", p1), fmt.Sprintf("127.0.0.1:%d", p2)
	c := noRedirect()

	form := url.Values{"username": {"u"}, "password": {"p"}, "next": {"/"}}
	req, _ := http.NewRequest("POST", "http://"+a1+authPath+"/login", strings.NewReader(form.Encode()))
	req.Host = "a.test"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	var sess *http.Cookie
	for _, ck := range resp.Cookies() {
		if strings.HasPrefix(ck.Name, "gately_site_") && ck.Value != "" {
			sess = ck
		}
	}
	if resp.StatusCode != http.StatusFound || sess == nil {
		t.Fatalf("登录失败: %d %+v", resp.StatusCode, resp.Cookies())
	}
	visit := func(addr string, cookies ...*http.Cookie) *http.Response {
		r, _ := http.NewRequest("GET", "http://"+addr+"/", nil)
		r.Host = "a.test"
		for _, ck := range cookies {
			r.AddCookie(ck)
		}
		out, err := c.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		out.Body.Close()
		return out
	}
	if r := visit(a1, sess); r.StatusCode != 200 {
		t.Fatalf("移动前会话应有效: %d", r.StatusCode)
	}

	if code, _, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchMove, Move: moveTo("l2")}); code != 200 {
		t.Fatalf("移动失败: %d %s", code, body)
	}
	renamed := &http.Cookie{Name: cookieName(Site{Key: "a", scope: "l2/a"}), Value: sess.Value}
	if renamed.Name == sess.Name {
		t.Fatal("不同位置的会话 Cookie 名称应不同")
	}
	if r := visit(a2, sess, renamed); r.StatusCode == 200 {
		t.Fatalf("移动后旧会话（含改名重放）不应有效: %d", r.StatusCode)
	}
}
