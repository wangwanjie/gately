package webservice

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// batchModule 返回带两个监听器的模块：l1 有三个站点和一个启用的默认站点，l2 有一个站点。
func batchModule(t *testing.T) *Module {
	t.Helper()
	st, _ := config.Open(t.TempDir())
	m := New(nil, nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	l1 := Listener{Key: "l1", Name: "main", ListenIP: "127.0.0.1", Port: 18081, Sites: []Site{
		{Key: "a", Enable: true, Domains: []string{"a.old.com", "www.a.old.com", "keep.org"}, Type: TypeText, Text: "a"},
		{Key: "b", Enable: false, Group: "组", Domains: []string{"*.old.com", "old.com"}, Type: TypeText, Text: "b"},
		{Key: "c", Enable: true, Domains: []string{"c.other.net"}, Type: TypeText, Text: "c", Compress: true},
	}, Default: Site{Enable: true, Type: TypeText, Text: "d"}}
	l2 := Listener{Key: "l2", Name: "second", ListenIP: "127.0.0.1", Port: 18082, Sites: []Site{
		{Key: "x", Enable: true, Domains: []string{"x.old.com"}, Type: TypeText, Text: "x"},
	}}
	for _, l := range []*Listener{&l1, &l2} {
		if err := l.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	m.cfg.Listeners = []Listener{l1, l2}
	return m
}

func (m *Module) batch(t *testing.T, req SiteBatchRequest) (int, SiteBatchResult, string) {
	t.Helper()
	raw, _ := json.Marshal(req)
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/sites/batch", strings.NewReader(string(raw)))
	r.Header.Set("Content-Type", "application/json")
	m.handleSiteBatch(rec, r)
	var out SiteBatchResult
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out, rec.Body.String()
}

func (m *Module) siteOf(t *testing.T, l, s string) Site {
	t.Helper()
	for _, lv := range m.cfg.Listeners {
		if lv.Key != l {
			continue
		}
		if s == "default" {
			return lv.Default
		}
		for _, st := range lv.Sites {
			if st.Key == s {
				return st
			}
		}
	}
	t.Fatalf("站点 %s/%s 不存在", l, s)
	return Site{}
}

func refs(pairs ...string) []SiteRef {
	var out []SiteRef
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, SiteRef{Listener: pairs[i], Site: pairs[i+1]})
	}
	return out
}

// 批量启停：跨监听器、包含默认站点；已是目标状态的站点计入 Unchanged；全部已符合时不保存。
func TestSiteBatchEnable(t *testing.T) {
	m := batchModule(t)
	code, res, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "a", "l1", "b", "l2", "x", "l1", "default"), Action: BatchDisable})
	if code != 200 || res.Changed != 3 || res.Unchanged != 1 {
		t.Fatalf("应有 3 个变化 1 个不变: %d %s", code, body)
	}
	if m.siteOf(t, "l1", "a").Enable || m.siteOf(t, "l2", "x").Enable || m.siteOf(t, "l1", "default").Enable || !m.siteOf(t, "l1", "c").Enable {
		t.Fatalf("启停结果不对: %+v", m.cfg.Listeners)
	}
	code, res, _ = m.batch(t, SiteBatchRequest{Sites: refs("l1", "a", "l1", "a"), Action: BatchDisable})
	if code != 200 || res.Changed != 0 || res.Unchanged != 1 {
		t.Fatalf("重复引用只算一次且无变化: %d %+v", code, res)
	}
	if code, res, _ = m.batch(t, SiteBatchRequest{Sites: refs("l1", "b"), Action: BatchEnable}); code != 200 || res.Changed != 1 || !m.siteOf(t, "l1", "b").Enable {
		t.Fatalf("启用失败: %d %+v", code, res)
	}
}

// 批量删除：保留分组与剩余站点顺序；默认站点不能删；删除结果跨监听器生效。
func TestSiteBatchDelete(t *testing.T) {
	m := batchModule(t)
	if code, _, _ := m.batch(t, SiteBatchRequest{Sites: refs("l1", "default"), Action: BatchDelete}); code != 400 {
		t.Fatalf("默认站点不能删除，实际 %d", code)
	}
	code, res, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "a", "l1", "c", "l2", "x"), Action: BatchDelete})
	if code != 200 || res.Changed != 3 {
		t.Fatalf("删除失败: %d %s", code, body)
	}
	if got := m.cfg.Listeners[0].Sites; len(got) != 1 || got[0].Key != "b" || got[0].Group != "组" {
		t.Fatalf("应只剩 b: %+v", got)
	}
	if len(m.cfg.Listeners[1].Sites) != 0 {
		t.Fatalf("l2 应无站点: %+v", m.cfg.Listeners[1].Sites)
	}
}

// 批量增删域名：规范化（大小写、空白）、去重；删光域名时拒绝且整体不修改。
func TestSiteBatchDomains(t *testing.T) {
	m := batchModule(t)
	// 同一监听器的两个站点加入同一个新域名会冲突，整体拒绝
	code, _, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "a", "l1", "c"), Action: BatchAddDomains, Domains: []string{"new.example.com"}})
	if code != 400 || !strings.Contains(body, "new.example.com") || slices.Contains(m.siteOf(t, "l1", "a").Domains, "new.example.com") {
		t.Fatalf("同一监听器内重复的新域名应 400: %d %s", code, body)
	}
	// 不同监听器的站点可以使用同一域名
	if code, _, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "c", "l2", "x"), Action: BatchAddDomains, Domains: []string{"shared.example.com"}}); code != 200 {
		t.Fatalf("不同监听器可共用域名: %d %s", code, body)
	}
	m = batchModule(t)
	code, res, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchAddDomains, Domains: []string{" NEW.Example.com ", "keep.org", "new.example.com"}})
	if code != 200 || res.Changed != 1 {
		t.Fatalf("增加失败: %d %s", code, body)
	}
	if code, _, body = m.batch(t, SiteBatchRequest{Sites: refs("l1", "c"), Action: BatchAddDomains, Domains: []string{"c2.other.net"}}); code != 200 {
		t.Fatalf("增加失败: %d %s", code, body)
	}
	if got := m.siteOf(t, "l1", "a").Domains; !slices.Equal(got, []string{"a.old.com", "www.a.old.com", "keep.org", "new.example.com"}) {
		t.Fatalf("a 的域名不对: %v", got)
	}
	// 已有的域名不重复加入，不算变化
	if code, res, _ = m.batch(t, SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchAddDomains, Domains: []string{"keep.org"}}); code != 200 || res.Changed != 0 || res.Unchanged != 1 {
		t.Fatalf("已有域名应不变: %d %+v", code, res)
	}

	code, res, _ = m.batch(t, SiteBatchRequest{Sites: refs("l1", "a", "l1", "c"), Action: BatchRemoveDomains, Domains: []string{"KEEP.org", "c2.other.net", "nothere.com"}})
	if code != 200 || res.Changed != 2 || slices.Contains(m.siteOf(t, "l1", "a").Domains, "keep.org") || slices.Contains(m.siteOf(t, "l1", "c").Domains, "c2.other.net") {
		t.Fatalf("删除域名失败: %d %+v", code, res)
	}

	// c 删光域名：整体拒绝，a 也不能被改
	before := m.siteOf(t, "l1", "a").Domains
	code, _, body = m.batch(t, SiteBatchRequest{Sites: refs("l1", "a", "l1", "c"), Action: BatchRemoveDomains, Domains: []string{"a.old.com", "c.other.net"}})
	if code != 400 || !strings.Contains(body, "至少填写一个域名") {
		t.Fatalf("删光域名应 400: %d %s", code, body)
	}
	if !slices.Equal(m.siteOf(t, "l1", "a").Domains, before) || !slices.Contains(m.siteOf(t, "l1", "c").Domains, "c.other.net") {
		t.Fatalf("失败后不应有任何修改: %+v", m.cfg.Listeners[0].Sites)
	}

	for _, bad := range [][]string{nil, {"bad host"}, {"a.com/path"}, {"user@a.com"}} {
		if code, _, _ := m.batch(t, SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchAddDomains, Domains: bad}); code != 400 {
			t.Fatalf("非法域名 %v 应 400，实际 %d", bad, code)
		}
	}
	if code, _, _ := m.batch(t, SiteBatchRequest{Sites: refs("l1", "default"), Action: BatchAddDomains, Domains: []string{"a.com"}}); code != 400 {
		t.Fatalf("默认站点不能改域名，实际 %d", code)
	}
}

func TestReplaceDomainSuffix(t *testing.T) {
	cases := []struct{ d, from, to, want string }{
		{"old.com", "old.com", "new.com", "new.com"},
		{"a.old.com", "old.com", "new.com", "a.new.com"},
		{"*.old.com", "old.com", "new.com", "*.new.com"},
		{"x.y.old.com", "old.com", "new.com", "x.y.new.com"},
		{"bold.com", "old.com", "new.com", "bold.com"},
		{"old.com.cn", "old.com", "new.com", "old.com.cn"},
		{"*", "old.com", "new.com", "*"},
	}
	for _, c := range cases {
		if got := replaceDomainSuffix(c.d, c.from, c.to); got != c.want {
			t.Errorf("replaceDomainSuffix(%q,%q,%q)=%q want %q", c.d, c.from, c.to, got, c.want)
		}
	}
}

// 批量替换主域名：精确与通配、子域名一起替换，替换后重复的域名合并，不匹配的站点计入 Unchanged。
func TestSiteBatchReplaceDomain(t *testing.T) {
	m := batchModule(t)
	// 替换后 l1 中两个站点都得到 new.com 相关域名，但没有完全相同的新域名，不冲突
	code, res, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "a", "l1", "b", "l1", "c", "l2", "x"), Action: BatchReplaceDomain, From: " OLD.com ", To: "new.com"})
	if code != 200 || res.Changed != 3 || res.Unchanged != 1 {
		t.Fatalf("替换失败: %d %s", code, body)
	}
	if got := m.siteOf(t, "l1", "a").Domains; !slices.Equal(got, []string{"a.new.com", "www.a.new.com", "keep.org"}) {
		t.Fatalf("a: %v", got)
	}
	if got := m.siteOf(t, "l1", "b").Domains; !slices.Equal(got, []string{"*.new.com", "new.com"}) {
		t.Fatalf("b: %v", got)
	}
	// 替换后与同一监听器其他站点的域名相同时拒绝
	m3 := batchModule(t)
	m3.cfg.Listeners[0].Sites[2].Domains = []string{"a.new.com"}
	if code, _, body := m3.batch(t, SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchReplaceDomain, From: "old.com", To: "new.com"}); code != 400 || !strings.Contains(body, "a.new.com") {
		t.Fatalf("替换后与其他站点冲突应 400: %d %s", code, body)
	}
	// 替换后出现重复时合并
	if code, _, _ = m.batch(t, SiteBatchRequest{Sites: refs("l1", "b"), Action: BatchReplaceDomain, From: "*.new.com", To: "new.com"}); code != 400 {
		t.Fatalf("通配符不应作为替换源，实际 %d", code)
	}
	m2 := batchModule(t)
	m2.cfg.Listeners[0].Sites[0].Domains = []string{"a.old.com", "a.new.com"}
	if code, _, body := m2.batch(t, SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchReplaceDomain, From: "old.com", To: "new.com"}); code != 200 || !slices.Equal(m2.siteOf(t, "l1", "a").Domains, []string{"a.new.com"}) {
		t.Fatalf("重复应合并: %d %s %v", code, body, m2.siteOf(t, "l1", "a").Domains)
	}
	for _, c := range []SiteBatchRequest{
		{Sites: refs("l1", "a"), Action: BatchReplaceDomain, From: "", To: "x.com"},
		{Sites: refs("l1", "a"), Action: BatchReplaceDomain, From: "a.com", To: "*.x.com"},
		{Sites: refs("l1", "a"), Action: BatchReplaceDomain, From: "a b", To: "x.com"},
	} {
		if code, _, _ := m.batch(t, c); code != 400 {
			t.Fatalf("参数 %+v 应 400，实际 %d", c, code)
		}
	}
}

// 批量修改选项：只改提供的字段，其余保持；没有变化的站点计入 Unchanged；不认识的字段被忽略。
func TestSiteBatchSet(t *testing.T) {
	m := batchModule(t)
	yes, no, waf := true, false, " w1 "
	code, res, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "a", "l1", "c", "l2", "x"), Action: BatchSet, Options: &SiteBatchOptions{Compress: &yes, AccessLog: &yes, WAF: &waf, CORS: &no}})
	if code != 200 || res.Changed != 3 {
		t.Fatalf("修改选项失败: %d %s", code, body)
	}
	a := m.siteOf(t, "l1", "a")
	if !a.Compress || !a.AccessLog || a.WAF != "w1" || a.CORS || a.DenyClose || a.Text != "a" {
		t.Fatalf("a 的选项不对: %+v", a)
	}
	code, res, _ = m.batch(t, SiteBatchRequest{Sites: refs("l1", "a", "l1", "c"), Action: BatchSet, Options: &SiteBatchOptions{Compress: &yes}})
	if code != 200 || res.Changed != 0 || res.Unchanged != 2 {
		t.Fatalf("已一致时不应有变化: %d %+v", code, res)
	}
	empty := ""
	if code, _, _ = m.batch(t, SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchSet, Options: &SiteBatchOptions{WAF: &empty}}); code != 200 || m.siteOf(t, "l1", "a").WAF != "" {
		t.Fatalf("应能取消 WAF 引用: %d", code)
	}
	if code, _, _ = m.batch(t, SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchSet}); code != 400 {
		t.Fatalf("没有选项应 400，实际 %d", code)
	}
	// 请求体中白名单外的字段（如试图改后端地址、用户、访问控制）会被拒绝，不会写入站点
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/sites/batch", strings.NewReader(`{"sites":[{"listener":"l1","site":"a"}],"action":"set","options":{"cors":true,"targets":["http://evil:1"],"users":["a:b"],"ipMode":"blacklist"}}`))
	m.handleSiteBatch(rec, r)
	if a := m.siteOf(t, "l1", "a"); rec.Code != 400 || a.CORS || len(a.Targets) != 0 || len(a.Users) != 0 || a.IPMode != IPModeOff {
		t.Fatalf("不应写入白名单外的字段: %d %+v", rec.Code, a)
	}
}

// 输入错误：未知监听器与站点、未知动作、空列表、超过上限都被拒绝且不修改配置。
func TestSiteBatchRejected(t *testing.T) {
	m := batchModule(t)
	want := []int{404, 404, 400, 400}
	reqs := []SiteBatchRequest{
		{Sites: refs("zz", "a"), Action: BatchDisable},
		{Sites: refs("l1", "a", "l1", "nope"), Action: BatchDisable},
		{Sites: refs("l1", "a"), Action: "explode"},
		{Action: BatchDisable},
	}
	for i, req := range reqs {
		if code, _, body := m.batch(t, req); code != want[i] {
			t.Fatalf("请求 %d 应 %d，实际 %d %s", i, want[i], code, body)
		}
	}
	if !m.siteOf(t, "l1", "a").Enable {
		t.Fatal("拒绝后不应修改 a")
	}
	var many []SiteRef
	for i := 0; i < 501; i++ {
		many = append(many, SiteRef{Listener: "l1", Site: "a" + string(rune('a'+i%26)) + string(rune('a'+i/26))})
	}
	if code, _, _ := m.batch(t, SiteBatchRequest{Sites: many, Action: BatchDisable}); code != 400 {
		t.Fatalf("超过上限应 400，实际 %d", code)
	}
}

// 引用不存在的 IP 列表等监听器级校验失败时整体回滚（这里用启用无认证用户的基本认证站点触发校验失败）。
func TestSiteBatchValidateRollback(t *testing.T) {
	m := batchModule(t)
	m.cfg.Listeners[0].Default.Enable = false
	// 默认站点在停用时不校验；批量启用一个内容不完整的默认站点应整体失败
	m.cfg.Listeners[0].Default = Site{Key: "default", Type: TypeProxy}
	code, _, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "a", "l1", "default"), Action: BatchDisable})
	if code != 200 {
		t.Fatalf("停用不应触发校验失败: %d %s", code, body)
	}
	code, _, body = m.batch(t, SiteBatchRequest{Sites: refs("l1", "b", "l1", "default"), Action: BatchEnable})
	if code != http.StatusBadRequest {
		t.Fatalf("启用不完整的默认站点应 400: %d %s", code, body)
	}
	if m.siteOf(t, "l1", "b").Enable || m.siteOf(t, "l1", "default").Enable {
		t.Fatalf("失败后不应启用任何站点: %+v", m.cfg.Listeners[0])
	}
}

// knownWAF 是只认识部分实例的 WAF，用于校验批量设置 WAF 引用。
type knownWAF map[string]bool

func (knownWAF) Middleware(_ string, next http.Handler) http.Handler { return next }
func (k knownWAF) HasInstance(key string) bool                       { return k[key] }

// 批量设置引用不存在的 WAF 实例时拒绝，避免选中的站点静默失去防护。
func TestSiteBatchSetUnknownWAF(t *testing.T) {
	m := batchModule(t)
	m.waf = knownWAF{"w1": true}
	bad, good := "typo", "w1"
	if code, _, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchSet, Options: &SiteBatchOptions{WAF: &bad}}); code != 400 {
		t.Fatalf("不存在的 WAF 实例应 400: %d %s", code, body)
	}
	if m.siteOf(t, "l1", "a").WAF != "" {
		t.Fatal("拒绝后不应修改站点")
	}
	if code, _, body := m.batch(t, SiteBatchRequest{Sites: refs("l1", "a"), Action: BatchSet, Options: &SiteBatchOptions{WAF: &good}}); code != 200 {
		t.Fatalf("存在的 WAF 实例应成功: %d %s", code, body)
	}
}

// 超大的站点或域名数组在去重之前就被拒绝，不会长时间占用 CPU。
func TestSiteBatchOversizedInput(t *testing.T) {
	m := batchModule(t)
	many := make([]SiteRef, 100000)
	for i := range many {
		many[i] = SiteRef{Listener: "l1", Site: strconv.Itoa(i)}
	}
	start := time.Now()
	if code, _, _ := m.batch(t, SiteBatchRequest{Sites: many, Action: BatchDisable}); code != 400 {
		t.Fatalf("超大站点数组应 400，实际 %d", code)
	}
	domains := make([]string, 100000)
	for i := range domains {
		domains[i] = "d" + strconv.Itoa(i) + ".example.com"
	}
	if _, err := normalizeBatchDomains(domains); err == nil {
		t.Fatal("超大域名数组应被拒绝")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("拒绝超大输入耗时过长: %v", d)
	}
}
