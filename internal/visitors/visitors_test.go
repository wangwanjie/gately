package visitors

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// base 为测试的起始时间（本地时间中午，避免跨天）
var base = time.Date(2026, 11, 10, 12, 0, 0, 0, time.Local)

func newTestStore(t *testing.T, path string) (*Store, *time.Time) {
	t.Helper()
	now := base
	s := New(path, t.Logf)
	s.now = func() time.Time { return now }
	return s, &now
}

func ip(s string) netip.Addr {
	return netip.MustParseAddr(s)
}

func TestRecordAggregates(t *testing.T) {
	s, _ := newTestStore(t, "")
	a := ip("203.0.113.5")
	s.Record(Hit{Addr: a, Time: base, Listener: "l1", Site: "s1", SiteName: "博客", Host: "blog.example.com", Method: "GET", Path: "/", Status: 200, Bytes: 100, UA: "UA-1"})
	s.Record(Hit{Addr: a, Time: base.Add(time.Minute), Listener: "l1", Site: "s1", SiteName: "博客", Host: "blog.example.com", Method: "GET", Path: "/admin", Status: 403, Deny: "ip", UA: "UA-1"})
	s.Record(Hit{Addr: a, Time: base.Add(2 * time.Minute), Listener: "l1", Site: "s2", SiteName: "相册", Host: "pic.example.com", Method: "POST", Path: "/upload", Status: 201, User: "alice", UA: "UA-2"})
	s.Record(Hit{Addr: a, Time: base.Add(25 * time.Hour), Listener: "l1", Unmatched: true, Host: "x.example.com", Method: "GET", Path: "/.env", Status: 444, Admin: ""})
	// IPv4 映射的 IPv6 地址与 IPv4 是同一个访客
	s.Record(Hit{Addr: ip("::ffff:203.0.113.5"), Time: base.Add(26 * time.Hour), Listener: "l1", Site: "s1", SiteName: "博客", Method: "GET", Path: "/", Status: 200, Admin: "root"})

	if s.Len() != 1 {
		t.Fatalf("访客数 = %d", s.Len())
	}
	v, ok := s.Get(a)
	if !ok {
		t.Fatal("找不到访客")
	}
	if v.Requests != 5 || v.Denied != 1 || v.Noise != 2 || v.Bytes != 100 || v.Closed != 1 {
		t.Fatalf("计数不对: %+v", v)
	}
	if v.Status != [5]uint32{0, 3, 0, 2, 0} {
		t.Fatalf("状态码分布 = %v", v.Status)
	}
	if v.First != base.Unix() || v.Last != base.Add(26*time.Hour).Unix() {
		t.Fatalf("首次/最近时间不对: %d %d", v.First, v.Last)
	}
	if len(v.Days) != 2 || v.Days[0].Req != 3 || v.Days[0].Denied != 1 || v.Days[1].Req != 2 || v.Days[1].D != v.Days[0].D+1 {
		t.Fatalf("按天计数不对: %+v", v.Days)
	}
	if len(v.Sites) != 3 {
		t.Fatalf("站点 = %+v", v.Sites)
	}
	if len(v.Identities) != 2 || v.Identities[0].Name != "alice" || v.Identities[0].Kind != "user" || v.Identities[1].Kind != "admin" {
		t.Fatalf("身份 = %+v", v.Identities)
	}
	if len(v.Agents) != 2 || len(v.Deny) != 1 || v.Deny[0] != (Count{"ip", 1}) {
		t.Fatalf("UA/拦截原因不对: %+v %+v", v.Agents, v.Deny)
	}
	if v.LastReq.Path != "/" || v.LastReq.SiteName != "博客" || v.LastReq.Status != 200 {
		t.Fatalf("最近请求 = %+v", v.LastReq)
	}
}

func TestRecordIgnoresInvalidAndDisabled(t *testing.T) {
	s, _ := newTestStore(t, "")
	s.Record(Hit{Time: base, Path: "/"})
	var nilStore *Store
	nilStore.Record(Hit{Addr: ip("1.1.1.1")})
	if nilStore.Active() || nilStore.Flush() != nil {
		t.Fatal("nil 存储应安全返回")
	}
	s.Apply(Settings{Disable: true, Days: 30, Max: 100})
	s.Record(Hit{Addr: ip("1.1.1.1"), Time: base})
	if s.Len() != 0 || s.Active() {
		t.Fatal("停用后不应记录")
	}
	s.Apply(Settings{Days: 30, Max: 100})
	s.Record(Hit{Addr: ip("1.1.1.1"), Time: base})
	if s.Len() != 1 {
		t.Fatal("重新启用后应记录")
	}
}

func TestListsCappedAndStringsClipped(t *testing.T) {
	s, _ := newTestStore(t, "")
	a := ip("198.51.100.7")
	long := "/" + strings.Repeat("a", 5000)
	for i := 0; i < 50; i++ {
		s.Record(Hit{Addr: a, Time: base, Listener: "l", Site: fmt.Sprint("s", i), SiteName: strings.Repeat("名", 200),
			Path: fmt.Sprintf("/p%d", i), User: fmt.Sprint("u", i), UA: fmt.Sprint("ua", i), Deny: fmt.Sprint("d", i), Host: strings.Repeat("h", 1000)})
	}
	// 控制字符被去掉，超长路径截断后仍能合并
	s.Record(Hit{Addr: a, Time: base, Path: long + "x"})
	s.Record(Hit{Addr: a, Time: base, Path: long + "y", UA: "evil\r\nInjected: 1\x00"})
	v, _ := s.Get(a)
	if len(v.Sites) != maxSites || len(v.Paths) != maxPaths || len(v.Identities) != maxIdents || len(v.Agents) != maxAgents || len(v.Deny) != maxDeny {
		t.Fatalf("列表未限制条数: %d %d %d %d %d", len(v.Sites), len(v.Paths), len(v.Identities), len(v.Agents), len(v.Deny))
	}
	for _, st := range v.Sites {
		if len(st.Name) > maxNameLen {
			t.Fatalf("站点名未截断: %d", len(st.Name))
		}
	}
	found := false
	for _, p := range v.Paths {
		if len(p.Path) > maxPathLen {
			t.Fatalf("路径未截断: %d", len(p.Path))
		}
		if strings.HasPrefix(p.Path, "/aaa") {
			found = true
			if p.Req != 2 {
				t.Fatalf("截断后相同的路径应合并: %d", p.Req)
			}
		}
	}
	if !found {
		t.Fatal("最近的长路径应保留")
	}
	for _, ag := range v.Agents {
		if strings.ContainsAny(ag.UA, "\r\n\x00") {
			t.Fatalf("UA 含控制字符: %q", ag.UA)
		}
	}
	if len(v.LastReq.Host) > maxHostLen {
		t.Fatal("域名未截断")
	}
}

func TestEvictPrefersNoise(t *testing.T) {
	s, now := newTestStore(t, "")
	s.Apply(Settings{Days: 30, Max: 100})
	// 先来的正常访客
	for i := 0; i < 50; i++ {
		s.Record(Hit{Addr: ip(fmt.Sprintf("10.0.0.%d", i)), Time: *now, Site: "s", Path: "/"})
	}
	// 之后来的扫描器：只访问未匹配站点或被拦截
	for i := 0; i < 50; i++ {
		*now = now.Add(time.Second)
		h := Hit{Addr: ip(fmt.Sprintf("10.1.0.%d", i)), Time: *now, Unmatched: true, Path: "/.env", Status: 404}
		if i%2 == 0 {
			h = Hit{Addr: h.Addr, Time: *now, Site: "s", Deny: "ban", Status: 403}
		}
		s.Record(h)
	}
	// 继续来新的正常访客，挤掉的应是扫描器
	for i := 0; i < 20; i++ {
		*now = now.Add(time.Second)
		s.Record(Hit{Addr: ip(fmt.Sprintf("10.2.0.%d", i)), Time: *now, Site: "s", Path: "/"})
	}
	if s.Len() > 100 {
		t.Fatalf("超过上限: %d", s.Len())
	}
	for i := 0; i < 50; i++ {
		if _, ok := s.Get(ip(fmt.Sprintf("10.0.0.%d", i))); !ok {
			t.Fatalf("正常访客 10.0.0.%d 不应被淘汰", i)
		}
	}
	// 上限调小时立即淘汰，正常访客中最久未出现的先走
	s.Apply(Settings{Days: 30, Max: 60})
	if s.Len() != 60 {
		t.Fatalf("调小上限后 = %d", s.Len())
	}
	if _, ok := s.Get(ip("10.2.0.19")); !ok {
		t.Fatal("最近的访客应保留")
	}
}

func TestIdentityProtectsFromNoiseEviction(t *testing.T) {
	v := &Visitor{Requests: 3, Noise: 3}
	if !v.noise() {
		t.Fatal("全部为拦截请求应视为扫描类")
	}
	v.Identities = []IdentStat{{Name: "bob", Kind: "user"}}
	if v.noise() {
		t.Fatal("认证过的访客不应视为扫描类")
	}
}

func TestPruneByDays(t *testing.T) {
	s, now := newTestStore(t, "")
	old := ip("192.0.2.1")
	fresh := ip("192.0.2.2")
	s.Record(Hit{Addr: old, Time: base.AddDate(0, 0, -20)})
	s.Record(Hit{Addr: fresh, Time: base.AddDate(0, 0, -20)})
	s.Record(Hit{Addr: fresh, Time: base})
	s.Apply(Settings{Days: 7, Max: 100})
	if _, ok := s.Get(old); ok {
		t.Fatal("超过保留天数的访客应删除")
	}
	v, ok := s.Get(fresh)
	if !ok || len(v.Days) != 1 || v.Days[0].D != dayOf(*now) {
		t.Fatalf("超出保留天数的按天计数应删除: %+v", v.Days)
	}
}

func TestSettingsNormalize(t *testing.T) {
	var s Settings
	if err := s.Normalize(); err != nil || s.Days != DefaultDays || s.Max != DefaultMax {
		t.Fatalf("默认值: %+v %v", s, err)
	}
	for _, bad := range []Settings{{Days: -1}, {Days: 181}, {Max: 99}, {Max: 100001}} {
		if bad.Normalize() == nil {
			t.Fatalf("%+v 应报错", bad)
		}
	}
}

func seed(s *Store, now time.Time) {
	// a：今天正常访问 + 认证；b：3 天前被拦截；c：10 天前访问未匹配站点；d：今天管理员
	s.Record(Hit{Addr: ip("203.0.113.1"), Time: now, Listener: "l1", Site: "web", SiteName: "Web", Host: "a.example.com", Path: "/home", Status: 200, User: "alice", UA: "Mozilla/5.0 Firefox"})
	s.Record(Hit{Addr: ip("203.0.113.1"), Time: now.Add(-time.Hour), Listener: "l1", Site: "web", SiteName: "Web", Path: "/", Status: 200})
	s.Record(Hit{Addr: ip("203.0.113.2"), Time: now.AddDate(0, 0, -3), Listener: "l1", Site: "web", SiteName: "Web", Path: "/wp-login.php", Status: 403, Deny: "waf"})
	s.Record(Hit{Addr: ip("2001:db8::3"), Time: now.AddDate(0, 0, -10), Listener: "l2", Unmatched: true, Host: "scan", Path: "/.git/config", Status: 404})
	s.Record(Hit{Addr: ip("192.168.1.9"), Time: now.Add(-2 * time.Minute), Listener: "l2", Site: "nas", SiteName: "NAS", Path: "/api", Status: 200, Admin: "root"})
}

func ips(r Result) []string {
	var out []string
	for _, it := range r.Items {
		out = append(out, it.IP)
	}
	return out
}

func TestQuery(t *testing.T) {
	s, now := newTestStore(t, "")
	seed(s, *now)
	banned := func(a netip.Addr) bool { return a == ip("203.0.113.2") }
	country := func(a netip.Addr) string {
		if a.Is4() && a.As4()[0] == 203 {
			return "JP"
		}
		return ""
	}
	q := func(mod func(*Query)) Result {
		qq := Query{Banned: banned, Country: country}
		mod(&qq)
		return s.Query(qq)
	}
	all := q(func(*Query) {})
	if got := strings.Join(ips(all), ","); got != "203.0.113.1,192.168.1.9,203.0.113.2,2001:db8::3" {
		t.Fatalf("默认按最近访问排序: %s", got)
	}
	if all.Total != 4 || all.Summary.Visitors != 4 || all.Summary.Requests != 5 || all.Summary.Denied != 1 ||
		all.Summary.Authed != 1 || all.Summary.Banned != 1 || all.Summary.Noise != 2 {
		t.Fatalf("汇总不对: %+v", all.Summary)
	}
	if len(all.Daily) != DefaultDays || all.Daily[DefaultDays-1].Requests != 3 || all.Daily[DefaultDays-1].Visitors != 2 {
		t.Fatalf("每日汇总不对: %+v", all.Daily[DefaultDays-1])
	}
	if len(all.Sites) != 3 {
		t.Fatalf("站点列表: %+v", all.Sites)
	}
	it := all.Items[0]
	if it.Country != "JP" || it.Agent != "Mozilla/5.0 Firefox" || it.Requests != 2 || len(it.Identities) != 1 || it.SiteCount != 1 {
		t.Fatalf("列表项不对: %+v", it)
	}
	if got := ips(q(func(x *Query) { x.Days = 1 })); strings.Join(got, ",") != "203.0.113.1,192.168.1.9" {
		t.Fatalf("今天: %v", got)
	}
	if got := ips(q(func(x *Query) { x.Days = 7 })); len(got) != 3 {
		t.Fatalf("7 天: %v", got)
	}
	checks := map[string]string{
		FilterDenied: "203.0.113.2", FilterAuthed: "203.0.113.1", FilterBanned: "203.0.113.2",
		FilterAdmin: "192.168.1.9", FilterNoise: "203.0.113.2,2001:db8::3",
	}
	for f, want := range checks {
		if got := strings.Join(ips(q(func(x *Query) { x.Filter = f })), ","); got != want {
			t.Fatalf("筛选 %s: %s", f, got)
		}
	}
	for kw, want := range map[string]string{"ALICE": "203.0.113.1", "wp-login": "203.0.113.2", "firefox": "203.0.113.1", "jp": "203.0.113.1,203.0.113.2", "nas": "192.168.1.9", "db8": "2001:db8::3"} {
		if got := strings.Join(ips(q(func(x *Query) { x.Q = kw })), ","); got != want {
			t.Fatalf("关键词 %s: %s", kw, got)
		}
	}
	if got := strings.Join(ips(q(func(x *Query) { x.Site = "-" })), ","); got != "2001:db8::3" {
		t.Fatalf("未匹配站点: %s", got)
	}
	if got := strings.Join(ips(q(func(x *Query) { x.Site = "l2/nas" })), ","); got != "192.168.1.9" {
		t.Fatalf("站点筛选: %s", got)
	}
	if got := ips(q(func(x *Query) { x.Sort = SortRequests })); got[0] != "203.0.113.1" {
		t.Fatalf("按请求数: %v", got)
	}
	if got := ips(q(func(x *Query) { x.Sort = SortDenied })); got[0] != "203.0.113.2" {
		t.Fatalf("按拦截数: %v", got)
	}
	if got := ips(q(func(x *Query) { x.Sort = SortFirst })); got[len(got)-1] != "2001:db8::3" {
		t.Fatalf("按首次访问: %v", got)
	}
	page := q(func(x *Query) { x.Offset = 1; x.Limit = 2 })
	if page.Total != 4 || strings.Join(ips(page), ",") != "192.168.1.9,203.0.113.2" {
		t.Fatalf("分页: %v", ips(page))
	}
	if r := q(func(x *Query) { x.Offset = 99 }); len(r.Items) != 0 || r.Items == nil {
		t.Fatal("超出范围的分页应返回空列表")
	}
}

func TestDetail(t *testing.T) {
	s, _ := newTestStore(t, "")
	seed(s, base)
	if _, ok := s.Detail(ip("8.8.8.8"), nil, nil); ok {
		t.Fatal("不存在的访客")
	}
	d, ok := s.Detail(ip("203.0.113.1"), func(netip.Addr) bool { return true }, func(netip.Addr) string { return "JP" })
	if !ok || !d.Banned || d.Country != "JP" || len(d.Daily) != DefaultDays || d.Daily[DefaultDays-1].Requests != 2 || d.Days != nil {
		t.Fatalf("详情不对: %+v", d)
	}
	if d.Paths[0].Req < d.Paths[len(d.Paths)-1].Req {
		t.Fatal("路径应按请求数从多到少")
	}
	if d.Deny == nil || d.Agents == nil {
		t.Fatal("空列表应为 []")
	}
	// 详情是副本，修改不影响存储
	d.Paths[0].Path = "changed"
	v, _ := s.Get(ip("203.0.113.1"))
	for _, p := range v.Paths {
		if p.Path == "changed" {
			t.Fatal("详情应为副本")
		}
	}
}

func TestDeleteAndClear(t *testing.T) {
	s, _ := newTestStore(t, "")
	seed(s, base)
	if n := s.Delete([]netip.Addr{ip("203.0.113.1"), ip("::ffff:203.0.113.2"), ip("8.8.8.8")}); n != 2 {
		t.Fatalf("删除 = %d", n)
	}
	if n := s.Clear(); n != 2 || s.Len() != 0 {
		t.Fatalf("清空 = %d", n)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "visitors", "visitors.jsonl.gz")
	s, _ := newTestStore(t, path)
	seed(s, base)
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("文件权限 = %v", st.Mode().Perm())
	}
	if in := s.Info(); in.FileBytes != st.Size() || in.SavedAt == 0 || in.Count != 4 || in.Error != "" {
		t.Fatalf("存储状态 = %+v", in)
	}
	// 没有变化时 Flush 不重写文件
	mt := st.ModTime()
	time.Sleep(10 * time.Millisecond)
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	if st2, _ := os.Stat(path); !st2.ModTime().Equal(mt) {
		t.Fatal("没有变化时不应写盘")
	}

	s2, _ := newTestStore(t, path)
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}
	a, _ := s.Get(ip("203.0.113.1"))
	b, ok := s2.Get(ip("203.0.113.1"))
	if !ok || fmt.Sprintf("%+v", a) != fmt.Sprintf("%+v", b) {
		t.Fatalf("读回不一致:\n%+v\n%+v", a, b)
	}
	if s2.Len() != 4 {
		t.Fatalf("读回 %d 个", s2.Len())
	}
	// 读回后按当前设置清理：保留 7 天时 10 天前的访客被删除
	s3, _ := newTestStore(t, path)
	s3.Apply(Settings{Days: 7, Max: 100})
	if err := s3.Load(); err != nil || s3.Len() != 3 {
		t.Fatalf("读回时应按保留天数清理: %d %v", s3.Len(), err)
	}
}

func writeGz(t *testing.T, path string, content string) {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write([]byte(content))
	_ = zw.Close()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadSanitizes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v.gz")
	now := base.Unix()
	var lines []string
	lines = append(lines, "not json", `{"ip":"bad"}`, `{"ip":"fe80::1%eth0","last":1}`)
	// 手工编辑的记录：列表超长、字符串超长、计数矛盾、按天计数乱序重复
	paths := make([]string, 0, 30)
	for i := 0; i < 30; i++ {
		paths = append(paths, fmt.Sprintf(`{"path":"/%s%d","req":1}`, strings.Repeat("x", 500), i))
	}
	d := dayOf(base)
	lines = append(lines, fmt.Sprintf(`{"ip":"::ffff:1.2.3.4","first":%d,"last":1,"requests":1,"denied":9,"noise":9,"paths":[%s],"identities":[{"name":"x","kind":"root"}],"days":[{"d":%d,"req":1},{"d":%d,"req":2},{"d":%d,"req":3}],"lastReq":{"host":"%s"}}`,
		now, strings.Join(paths, ","), d, d-1, d, strings.Repeat("h", 999)))
	writeGz(t, path, strings.Join(lines, "\n")+"\n")
	s, _ := newTestStore(t, path)
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	if s.Len() != 1 {
		t.Fatalf("应只读回 1 条有效记录: %d", s.Len())
	}
	v, _ := s.Get(ip("1.2.3.4"))
	if v.IP != "1.2.3.4" || v.Last != now || v.Denied != 1 || v.Noise != 1 || len(v.Paths) != maxPaths ||
		len(v.Paths[0].Path) > maxPathLen || v.Identities[0].Kind != "user" || len(v.LastReq.Host) > maxHostLen {
		t.Fatalf("规整不对: %+v", v)
	}
	if len(v.Days) != 2 || v.Days[0].D != d-1 {
		t.Fatalf("按天计数应排序去重: %+v", v.Days)
	}
}

func TestLoadRejectsSymlinkAndHandlesTruncation(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.gz")
	writeGz(t, target, `{"ip":"1.1.1.1","last":1}`+"\n")
	link := filepath.Join(dir, "link.gz")
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	s, _ := newTestStore(t, link)
	if err := s.Load(); err == nil {
		t.Fatal("符号链接应拒绝")
	}

	// 不存在的文件不报错
	s2, _ := newTestStore(t, filepath.Join(dir, "none.gz"))
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}

	// 截断的文件：已读出的记录照常使用
	full := filepath.Join(dir, "full.gz")
	s3, _ := newTestStore(t, full)
	for i := 0; i < 200; i++ {
		s3.Record(Hit{Addr: ip(fmt.Sprintf("10.9.%d.%d", i/250, i%250)), Time: base, Path: strings.Repeat("p", 100) + fmt.Sprint(i)})
	}
	if err := s3.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(full)
	if err := os.WriteFile(full, data[:len(data)*2/3], 0o600); err != nil {
		t.Fatal(err)
	}
	s4, _ := newTestStore(t, full)
	if err := s4.Load(); err == nil {
		t.Fatal("截断的文件应报告读取不完整")
	}
	if s4.Len() == 0 || s4.Len() >= 200 {
		t.Fatalf("截断时应读回部分记录: %d", s4.Len())
	}
}

func TestSaveReplacesPlantedTempSymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "v.gz")
	victim := filepath.Join(dir, "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, path+".tmp"); err != nil {
		t.Skip(err)
	}
	s, _ := newTestStore(t, path)
	s.Record(Hit{Addr: ip("1.1.1.1"), Time: base})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep" {
		t.Fatal("不能经预置的临时文件符号链接写入其他文件")
	}
}

func TestDelayedSave(t *testing.T) {
	old := saveDelay
	saveDelay = 20 * time.Millisecond
	defer func() { saveDelay = old }()
	path := filepath.Join(t.TempDir(), "v.gz")
	s, _ := newTestStore(t, path)
	s.Record(Hit{Addr: ip("1.1.1.1"), Time: base})
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("有变化后应延迟写盘")
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.mu.Lock()
	pending := s.timer != nil || s.dirty
	s.mu.Unlock()
	if pending {
		t.Fatal("写盘后不应再有待写的变化")
	}
}

func TestSaveFailureKeepsDirty(t *testing.T) {
	dir := t.TempDir()
	// 目标目录被同名文件占用，写盘失败
	blocker := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	s, _ := newTestStore(t, filepath.Join(blocker, "v.gz"))
	s.Record(Hit{Addr: ip("1.1.1.1"), Time: base})
	if err := s.Flush(); err == nil {
		t.Fatal("应写盘失败")
	}
	if in := s.Info(); in.Error == "" {
		t.Fatal("应报告写盘失败原因")
	}
	s.mu.Lock()
	dirty := s.dirty
	s.mu.Unlock()
	if !dirty {
		t.Fatal("写盘失败后应保留脏标记以便重试")
	}
}

func TestConcurrentRecordQuerySave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v.gz")
	s := New(path, t.Logf)
	s.Apply(Settings{Days: 30, Max: 500})
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				s.Record(Hit{Addr: ip(fmt.Sprintf("10.%d.%d.%d", g, i/250, i%250)), Site: "s", Path: fmt.Sprint("/", i%7), Status: 200, User: "u"})
			}
		}(g)
	}
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			s.Query(Query{Q: "10.1", Sort: SortRequests})
			s.Detail(ip("10.1.0.1"), nil, nil)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			_ = s.Save()
		}
	}()
	wg.Wait()
	if s.Len() > 500 {
		t.Fatalf("超过上限: %d", s.Len())
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
}

func TestDayOfLocal(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*3600)
	a := time.Date(2026, 11, 10, 0, 30, 0, 0, loc)
	b := time.Date(2026, 11, 9, 23, 30, 0, 0, loc)
	if dayOf(a) != dayOf(b)+1 {
		t.Fatal("应按本地时间分天")
	}
	if formatDay(dayOf(a)) != "2026-11-10" {
		t.Fatalf("日期格式 = %s", formatDay(dayOf(a)))
	}
	if dayOf(time.Unix(-1, 0).UTC()) != -1 {
		t.Fatal("1970 年前向下取整")
	}
}

func BenchmarkRecord(b *testing.B) {
	s := New("", nil)
	h := Hit{Addr: ip("203.0.113.9"), Listener: "l", Site: "s", SiteName: "站点", Host: "a.example.com", Method: "GET", Path: "/api/x", Status: 200, Bytes: 1000, UA: "Mozilla/5.0"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		h.Time = time.Now()
		s.Record(h)
	}
}
