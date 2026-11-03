package webservice

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"slices"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

func moveTo(listener string) *SiteMoveTarget { return &SiteMoveTarget{Listener: listener} }

func strPtr(s string) *string { return &s }

// siteKeys 返回监听器中站点 key 的顺序。
func (m *Module) siteKeys(t *testing.T, lk string) []string {
	t.Helper()
	for _, l := range m.cfg.Listeners {
		if l.Key == lk {
			var out []string
			for _, s := range l.Sites {
				out = append(out, s.Key)
			}
			return out
		}
	}
	t.Fatalf("监听器 %s 不存在", lk)
	return nil
}

func TestSiteBatchMoveBasic(t *testing.T) {
	m := batchModule(t)
	code, res, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "a", "l1", "c"), Action: BatchMove, Move: moveTo("l2")})
	if code != 200 || res.Changed != 2 || res.Unchanged != 0 {
		t.Fatalf("移动失败: %d %s", code, body)
	}
	if got := m.siteKeys(t, "l1"); !slices.Equal(got, []string{"b"}) {
		t.Fatalf("源监听应只剩 b: %v", got)
	}
	// 未指定位置与分组时按提交顺序放在末尾，key 不冲突时保持不变，站点内容随之移动
	if got := m.siteKeys(t, "l2"); !slices.Equal(got, []string{"x", "a", "c"}) {
		t.Fatalf("目标监听顺序错误: %v", got)
	}
	if s := m.siteOf(t, "l2", "c"); !s.Compress || s.Text != "c" || !s.Enable {
		t.Fatalf("站点内容应随之移动: %+v", s)
	}
	want := []SiteMoved{{From: SiteRef{"l1", "a"}, To: SiteRef{"l2", "a"}}, {From: SiteRef{"l1", "c"}, To: SiteRef{"l2", "c"}}}
	if !slices.Equal(res.Moved, want) {
		t.Fatalf("移动结果错误: %+v", res.Moved)
	}
	// 默认站点不受影响
	if !m.siteOf(t, "l1", "default").Enable || m.siteOf(t, "l2", "default").Enable {
		t.Fatal("默认站点不应变化")
	}
	// 已保存到磁盘
	var saved fileConfig
	if _, err := m.store.Load(ID, &saved); err != nil || len(saved.Listeners[1].Sites) != 3 {
		t.Fatalf("应已保存: %v %+v", err, saved.Listeners)
	}
}

// 分组为 nil 时保留原分组；已在目标监听中的站点不动，计入 unchanged。
func TestSiteBatchMoveKeepGroupAndSameListener(t *testing.T) {
	m := batchModule(t)
	code, res, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "b", "l2", "x"), Action: BatchMove, Move: moveTo("l2")})
	if code != 200 || res.Changed != 1 || res.Unchanged != 1 || len(res.Moved) != 1 {
		t.Fatalf("结果错误: %d %s", code, body)
	}
	if s := m.siteOf(t, "l2", "b"); s.Group != "组" || s.Enable {
		t.Fatalf("应保留原分组与停用状态: %+v", s)
	}
	// 全部已在目标监听时不保存也不报错
	code, res, _ = m.batch(t, SiteBatchRequest{Sites: refs("l2", "x"), Action: BatchMove, Move: moveTo("l2")})
	if code != 200 || res.Changed != 0 || res.Unchanged != 1 || res.Moved != nil {
		t.Fatalf("无变化时结果错误: %d %+v", code, res)
	}
}

// 目标监听已有相同 key 时换用随机新 key，不覆盖目标中的站点。
func TestSiteBatchMoveKeyConflict(t *testing.T) {
	m := batchModule(t)
	m.cfg.Listeners[1].Sites[0].Key = "a"
	code, res, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchMove, Move: moveTo("l2")})
	if code != 200 || len(res.Moved) != 1 {
		t.Fatalf("移动失败: %d %s", code, body)
	}
	nk := res.Moved[0].To.Site
	if nk == "a" || nk == "" || nk == "default" || res.Moved[0].To.Listener != "l2" {
		t.Fatalf("冲突时应换用新 key: %+v", res.Moved[0])
	}
	if m.siteOf(t, "l2", "a").Text != "x" || m.siteOf(t, "l2", nk).Text != "a" {
		t.Fatal("目标原有站点不应被覆盖")
	}
}

// 移入站点的域名与目标监听已有站点重复时整体拒绝，任何监听都不改动。
func TestSiteBatchMoveDomainConflict(t *testing.T) {
	m := batchModule(t)
	m.cfg.Listeners[1].Sites[0].Domains = []string{"x.old.com", "keep.org"}
	before := m.siteKeys(t, "l1")
	code, _, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchMove, Move: moveTo("l2")})
	if code != http.StatusBadRequest {
		t.Fatalf("域名冲突应 400: %d %s", code, body)
	}
	if got := m.siteKeys(t, "l1"); !slices.Equal(got, before) || len(m.siteKeys(t, "l2")) != 1 {
		t.Fatalf("失败后不应修改: %v %v", got, m.siteKeys(t, "l2"))
	}
	// 两个来自不同监听、域名相同的站点一起移入同一监听也拒绝
	m = batchModule(t)
	m.cfg.Listeners = append(m.cfg.Listeners, Listener{Key: "l3", Port: 18083, Sites: []Site{{Key: "y", Enable: true, Domains: []string{"keep.org"}, Type: TypeText, Text: "y"}}})
	if code, _, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "a", "l3", "y"), Action: BatchMove, Move: moveTo("l2")}); code != http.StatusBadRequest {
		t.Fatalf("移入的站点之间域名重复应 400: %d %s", code, body)
	}
}

func TestSiteBatchMovePosition(t *testing.T) {
	m := batchModule(t)
	m.cfg.Listeners[1].Sites = []Site{
		{Key: "x", Enable: true, Group: "G", Domains: []string{"x.old.com"}, Type: TypeText, Text: "x"},
		{Key: "y", Enable: true, Group: "G", Domains: []string{"y.old.com"}, Type: TypeText, Text: "y"},
		{Key: "z", Enable: true, Domains: []string{"z.old.com"}, Type: TypeText, Text: "z"},
	}
	// 插到 y 之前并加入 y 的分组
	if code, _, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchMove, Move: &SiteMoveTarget{Listener: "l2", Before: "y"}}); code != 200 {
		t.Fatalf("移动失败: %d %s", code, body)
	}
	if got := m.siteKeys(t, "l2"); !slices.Equal(got, []string{"x", "a", "y", "z"}) || m.siteOf(t, "l2", "a").Group != "G" {
		t.Fatalf("插入位置或分组错误: %v %q", got, m.siteOf(t, "l2", "a").Group)
	}
	// 插到 z 之后，b 原有分组被清空（z 未分组）
	if code, _, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "b"), Action: BatchMove, Move: &SiteMoveTarget{Listener: "l2", After: "z"}}); code != 200 {
		t.Fatalf("移动失败: %d %s", code, body)
	}
	if got := m.siteKeys(t, "l2"); !slices.Equal(got, []string{"x", "a", "y", "z", "b"}) || m.siteOf(t, "l2", "b").Group != "" {
		t.Fatalf("插入位置或分组错误: %v %q", got, m.siteOf(t, "l2", "b").Group)
	}
	// 放入已有分组时排在该分组末尾
	if code, _, body := m.batch(t, SiteBatchRequest{Sites: refs("l2", "b"), Action: BatchMove, Move: &SiteMoveTarget{Listener: "l1", Group: strPtr("新组")}}); code != 200 {
		t.Fatalf("移动失败: %d %s", code, body)
	}
	if code, _, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "b"), Action: BatchMove, Move: &SiteMoveTarget{Listener: "l2", Group: strPtr(" G ")}}); code != 200 {
		t.Fatalf("移动失败: %d %s", code, body)
	}
	if got := m.siteKeys(t, "l2"); !slices.Equal(got, []string{"x", "a", "y", "b", "z"}) || m.siteOf(t, "l2", "b").Group != "G" {
		t.Fatalf("应排在分组 G 末尾: %v %q", got, m.siteOf(t, "l2", "b").Group)
	}
	// 目标中没有的分组放在未分组站点之前
	if code, _, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "c"), Action: BatchMove, Move: &SiteMoveTarget{Listener: "l2", Group: strPtr("H")}}); code != 200 {
		t.Fatalf("移动失败: %d %s", code, body)
	}
	if got := m.siteKeys(t, "l2"); !slices.Equal(got, []string{"x", "a", "y", "b", "c", "z"}) {
		t.Fatalf("新分组应放在未分组之前: %v", got)
	}
	// 移到未分组：排在未分组末尾
	if code, _, body := m.batch(t, SiteBatchRequest{Sites: refs("l2", "c"), Action: BatchMove, Move: &SiteMoveTarget{Listener: "l1", Group: strPtr("")}}); code != 200 {
		t.Fatalf("移动失败: %d %s", code, body)
	}
	if s := m.siteOf(t, "l1", "c"); s.Group != "" {
		t.Fatalf("应移出分组: %q", s.Group)
	}
}

func TestSiteBatchMoveRejected(t *testing.T) {
	cases := []struct {
		name string
		req  SiteBatchRequest
		code int
	}{
		{"缺少目标", SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchMove}, 400},
		{"目标为空", SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchMove, Move: moveTo("")}, 400},
		{"目标不存在", SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchMove, Move: moveTo("nope")}, 404},
		{"默认站点", SiteBatchRequest{Sites: refs("l1", "a", "l1", "default"), Action: BatchMove, Move: moveTo("l2")}, 400},
		{"同时指定前后", SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchMove, Move: &SiteMoveTarget{Listener: "l2", Before: "x", After: "x"}}, 400},
		{"相邻站点不存在", SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchMove, Move: &SiteMoveTarget{Listener: "l2", Before: "nope"}}, 400},
		{"分组过长", SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchMove, Move: &SiteMoveTarget{Listener: "l2", Group: strPtr(string(make([]rune, 100)))}}, 400},
		{"站点不存在", SiteBatchRequest{Sites: refs("l1", "nope"), Action: BatchMove, Move: moveTo("l2")}, 404},
	}
	for _, c := range cases {
		m := batchModule(t)
		code, _, body := m.batch(t, c.req)
		if code != c.code {
			t.Errorf("%s: 期望 %d，实际 %d %s", c.name, c.code, code, body)
		}
		if got := m.siteKeys(t, "l1"); !slices.Equal(got, []string{"a", "b", "c"}) || len(m.siteKeys(t, "l2")) != 1 {
			t.Errorf("%s: 拒绝后不应修改: %v %v", c.name, got, m.siteKeys(t, "l2"))
		}
	}
}

// 目标监听校验失败（引用的 IP 列表不存在）时整体回滚，源监听的站点切片也不被改写。
func TestSiteBatchMoveValidateRollback(t *testing.T) {
	m := batchModule(t)
	m.cfg.Listeners[0].Sites[0].IPSets = []string{"missing"}
	orig := m.cfg.Listeners[0].Sites
	code, _, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchMove, Move: moveTo("l2")})
	if code != http.StatusBadRequest {
		t.Fatalf("引用不存在的 IP 列表应 400: %d %s", code, body)
	}
	if len(orig) != 3 || orig[0].Key != "a" || len(m.siteKeys(t, "l2")) != 1 || len(m.siteKeys(t, "l1")) != 3 {
		t.Fatalf("失败后配置应保持原样: %v %v", m.siteKeys(t, "l1"), m.siteKeys(t, "l2"))
	}
}

// 移动后回调收到旧标识到新标识的映射，回调执行时模块锁已释放；分享链接的已兑换次数随之迁移。
func TestSiteBatchMoveHooksAndShareUses(t *testing.T) {
	m := batchModule(t)
	m.cfg.Listeners[1].Sites[0].Key = "a"
	var got map[string]string
	m.OnSitesMoved(func(renamed map[string]string) {
		if !m.mu.TryLock() {
			t.Error("回调执行时模块锁应已释放")
			return
		}
		m.mu.Unlock()
		got = renamed
	})
	m.auth.shareUses["l1/a/|x"] = 9
	m.auth.shareUses["l1/a|once"] = 1
	m.auth.shareUses["l1/b|once"] = 2
	code, res, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "a", "l1", "b"), Action: BatchMove, Move: moveTo("l2")})
	if code != 200 {
		t.Fatalf("移动失败: %d %s", code, body)
	}
	nk := res.Moved[0].To.Site
	want := map[string]string{"l1/a": "l2/" + nk, "l1/b": "l2/b"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("回调参数错误: %v，期望 %v", got, want)
	}
	uses := m.auth.shareUses
	if uses["l2/"+nk+"|once"] != 1 || uses["l2/b|once"] != 2 || uses["l1/a|once"] != 0 || uses["l1/a/|x"] != 9 {
		t.Fatalf("分享链接计数迁移错误: %v", uses)
	}
	// 失败的移动不触发回调
	got = nil
	if code, _, _ := m.batch(t, SiteBatchRequest{Sites: refs("l1", "default"), Action: BatchMove, Move: moveTo("l2")}); code != 400 || got != nil {
		t.Fatalf("失败时不应回调: %d %v", code, got)
	}
}

// 在运行中的监听之间移动站点：源监听不再匹配该域名，目标监听立即提供站点；
// 会话作用域随之变化，已兑换的分享 Cookie 需重新兑换，限次分享链接不会因移动而重新可用。
func TestSiteBatchMoveLive(t *testing.T) {
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
	share := []ShareLink{{Key: "all", Token: "aaaaaaaaaaaaaaaaaaaa"}, {Key: "once", Token: "bbbbbbbbbbbbbbbbbbbb", MaxUses: 1}}
	l1 := Listener{Key: "l1", Enable: true, ListenIP: "127.0.0.1", Network: "tcp4", Port: p1, Sites: []Site{
		{Key: "a", Enable: true, Domains: []string{"a.test"}, Type: TypeText, Text: "site-a", BasicAuth: true, Users: []string{"u:p"}, ShareLinks: share},
	}}
	l2 := Listener{Key: "l2", Enable: true, ListenIP: "127.0.0.1", Network: "tcp4", Port: p2, Sites: []Site{
		{Key: "a", Enable: true, Domains: []string{"other.test"}, Type: TypeText, Text: "other"},
	}}
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

	resp, _ := get(t, c, a1, "a.test", "/?_gs=aaaaaaaaaaaaaaaaaaaa", nil)
	if resp.StatusCode != 302 || len(resp.Cookies()) == 0 {
		t.Fatalf("兑换分享链接失败: %d", resp.StatusCode)
	}
	cookie := resp.Cookies()[0]
	if resp, _ := get(t, c, a1, "a.test", "/?_gs=bbbbbbbbbbbbbbbbbbbb", nil); resp.StatusCode != 302 {
		t.Fatalf("首次兑换限次链接应成功: %d", resp.StatusCode)
	}

	code, res, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchMove, Move: moveTo("l2")})
	if code != 200 || len(res.Moved) != 1 {
		t.Fatalf("移动失败: %d %s", code, body)
	}
	if resp, _ := get(t, c, a1, "a.test", "/", nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("源监听不应再提供该站点: %d", resp.StatusCode)
	}
	if resp, body := get(t, c, a2, "a.test", "/", map[string]string{"Authorization": "Basic dTpw"}); resp.StatusCode != 200 || body != "site-a" {
		t.Fatalf("目标监听应提供移入的站点: %d %q", resp.StatusCode, body)
	}
	if _, body := get(t, c, a2, "other.test", "/", nil); body != "other" {
		t.Fatalf("目标监听原有站点应不受影响: %q", body)
	}
	req, _ := http.NewRequest("GET", "http://"+a2+"/", nil)
	req.Host = "a.test"
	req.AddCookie(cookie)
	r, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("移动后旧会话应失效: %d", r.StatusCode)
	}
	if resp, _ := get(t, c, a2, "a.test", "/?_gs=bbbbbbbbbbbbbbbbbbbb", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("限次链接不应因移动而重新可用: %d", resp.StatusCode)
	}
	if resp, _ := get(t, c, a2, "a.test", "/?_gs=aaaaaaaaaaaaaaaaaaaa", nil); resp.StatusCode != 302 {
		t.Fatalf("不限次链接在新位置应可兑换: %d", resp.StatusCode)
	}
}

// 目标监听已停用时照常移动配置：源监听立即不再提供该站点，目标监听保持停止，不会因此被启动；
// 源监听移空后仍在运行，未匹配的请求交给默认站点。
func TestSiteBatchMoveToDisabledListener(t *testing.T) {
	st, _ := config.Open(t.TempDir())
	m := New(nil, nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	p1 := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	l1 := Listener{Key: "l1", Enable: true, ListenIP: "127.0.0.1", Network: "tcp4", Port: p1,
		Sites:   []Site{{Key: "a", Enable: true, Domains: []string{"a.test"}, Type: TypeText, Text: "site-a"}},
		Default: Site{Enable: true, Type: TypeText, Text: "fallback"}}
	l2 := Listener{Key: "l2", Enable: false, ListenIP: "127.0.0.1", Network: "tcp4", Port: 1}
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
	addr := fmt.Sprintf("127.0.0.1:%d", p1)
	if _, body := get(t, noRedirect(), addr, "a.test", "/", nil); body != "site-a" {
		t.Fatalf("移动前应提供站点: %q", body)
	}
	if code, _, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchMove, Move: moveTo("l2")}); code != 200 {
		t.Fatalf("移动失败: %d %s", code, body)
	}
	if _, body := get(t, noRedirect(), addr, "a.test", "/", nil); body != "fallback" {
		t.Fatalf("源监听移空后应交给默认站点: %q", body)
	}
	m.mu.Lock()
	_, running := m.runners["l2"]
	m.mu.Unlock()
	if running {
		t.Fatal("停用的目标监听不应被启动")
	}
	if m.siteOf(t, "l2", "a").Text != "site-a" {
		t.Fatal("配置应已移到目标监听")
	}
}

// 并发移动同一批站点：模块锁保证只有一次真正移动，另一次看到站点已在目标监听（或已不在源监听），配置不会重复或丢失。
func TestSiteBatchMoveConcurrent(t *testing.T) {
	m := batchModule(t)
	done := make(chan int, 2)
	for range 2 {
		go func() {
			code, _, _ := m.batch(t, SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchMove, Move: moveTo("l2")})
			done <- code
		}()
	}
	codes := []int{<-done, <-done}
	slices.Sort(codes)
	if codes[0] != 200 || (codes[1] != 200 && codes[1] != http.StatusNotFound) {
		t.Fatalf("并发结果异常: %v", codes)
	}
	n := 0
	for _, l := range m.cfg.Listeners {
		for _, s := range l.Sites {
			if s.Text == "a" {
				n++
			}
		}
	}
	if n != 1 || !slices.Equal(m.siteKeys(t, "l2"), []string{"x", "a"}) {
		t.Fatalf("站点应恰好存在一份: %d %v", n, m.siteKeys(t, "l2"))
	}
}
