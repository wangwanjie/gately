package logbuf

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseKeywords(t *testing.T) {
	cases := []struct {
		in             string
		terms, exclude string
	}{
		{"", "[]", "[]"},
		{"  Login  失败 ", "[login 失败]", "[]"},
		{`"connection refused" -debug`, "[connection refused]", "[debug]"},
		{`-"a b" c`, "[c]", "[a b]"},
		{"- x", "[x]", "[]"},
		{"a-b　c", "[a-b c]", "[]"},
		// 以下边界情况前端高亮（LogViewer.vue 的 highlightTerms）逐字符照搬，修改时两边同步
		{`foo"bar baz" x`, "[foobar baz x]", "[]"},
		{`"abc d`, "[abc d]", "[]"},
		{"-", "[]", "[]"},
	}
	for _, c := range cases {
		terms, ex := ParseKeywords(c.in)
		if got := fmt.Sprint(terms); got != c.terms {
			t.Errorf("ParseKeywords(%q) terms = %s，期望 %s", c.in, got, c.terms)
		}
		if got := fmt.Sprint(ex); got != c.exclude {
			t.Errorf("ParseKeywords(%q) excludes = %s，期望 %s", c.in, got, c.exclude)
		}
	}
}

func TestParseStatus(t *testing.T) {
	ok := map[string]string{
		"404":      "[{404 404}]",
		"4xx, 502": "[{400 499} {502 502}]",
		"500-599":  "[{500 599}]",
		"2XX，3xx":  "[{200 299} {300 399}]",
		"":         "[]",
	}
	for in, want := range ok {
		got, valid := ParseStatus(in)
		if !valid || fmt.Sprint(got) != want {
			t.Errorf("ParseStatus(%q) = %v %v，期望 %s", in, got, valid, want)
		}
	}
	for _, in := range []string{"abc", "6xx", "99", "500-400", "1000", "4x"} {
		if _, valid := ParseStatus(in); valid {
			t.Errorf("ParseStatus(%q) 应当无效", in)
		}
	}
}

func TestIPMatcher(t *testing.T) {
	m := ParseIP("10.0.0.0/8, 2001:db8::1, 192.168.")
	cases := map[string]bool{
		"10.1.2.3":          true,
		"10.1.2.3:8080":     true,
		"[2001:db8::1]:443": true,
		"::ffff:10.0.0.1":   true,
		"192.168.1.20":      true,
		"172.16.0.1":        false,
		"2001:db8::2":       false,
	}
	for in, want := range cases {
		if got := m.match(in); got != want {
			t.Errorf("match(%q) = %v，期望 %v", in, got, want)
		}
	}
	if ParseIP(" , ") != nil {
		t.Error("没有有效项时应返回 nil")
	}
	// IPv4 映射网段要转换成 IPv4 网段，日志中的地址都已 Unmap
	mapped := ParseIP("::ffff:1.2.3.0/120")
	for in, want := range map[string]bool{"1.2.3.9": true, "::ffff:1.2.3.9": true, "1.2.4.1": false} {
		if got := mapped.match(in); got != want {
			t.Errorf("映射网段 match(%q) = %v，期望 %v", in, got, want)
		}
	}
}

func TestAddrTokens(t *testing.T) {
	got := addrTokens("来自 1.2.3.4:5678 的登录失败，封禁 [2001:db8::1]:22 与 10.0.0.0/24。face")
	joined := strings.Join(got, " ")
	for _, s := range []string{"1.2.3.4:5678", "[2001:db8::1]:22", "10.0.0.0"} {
		if !strings.Contains(joined, s) {
			t.Errorf("addrTokens 结果 %q 缺少 %s", joined, s)
		}
	}
	m := ParseIP("2001:db8::1")
	e := Entry{Message: "封禁 [2001:db8::1]:22"}
	if !m.matchEntry(&e) {
		t.Error("应能从消息中提取带端口的 IPv6 地址")
	}
}

func TestFilterMatch(t *testing.T) {
	now := time.Now()
	access := Entry{Time: now, Level: LevelInfo, Source: "HTTPS :443", Message: "1.2.3.4 GET a.example.com/x 404 3ms",
		Fields: Fields{FieldIP: "1.2.3.4", FieldHost: "a.example.com", FieldSite: "博客", FieldStatus: "404"}}
	login := Entry{Time: now.Add(-time.Hour), Level: LevelWarn, Source: "auth", Message: "来自 5.6.7.8 的登录失败: 密码错误"}

	cases := []struct {
		name   string
		f      Filter
		access bool
		login  bool
	}{
		{"空条件", Filter{}, true, true},
		{"最低级别", Filter{MinLevel: LevelWarn}, false, true},
		{"来源不区分大小写", Filter{Source: "AUTH"}, false, true},
		{"关键词全部包含", Filter{Terms: []string{"get", "404"}}, true, false},
		{"关键词匹配字段值", Filter{Terms: []string{"博客"}}, true, false},
		{"排除词", Filter{Excludes: []string{"登录"}}, true, false},
		{"IP 字段", Filter{IP: ParseIP("1.2.3.4")}, true, false},
		{"IP 从消息提取", Filter{IP: ParseIP("5.6.0.0/16")}, false, true},
		{"站点名称", Filter{Host: "博客"}, true, false},
		{"域名片段", Filter{Host: "example"}, true, false},
		{"无字段时域名在消息中查找", Filter{Host: "5.6.7"}, false, true},
		{"状态码只匹配带字段的日志", Filter{Status: []StatusRange{{400, 499}}}, true, false},
		{"状态码不命中", Filter{Status: []StatusRange{{500, 599}}}, false, false},
		{"开始时间", Filter{From: now.Add(-time.Minute)}, true, false},
		{"结束时间", Filter{To: now.Add(-time.Minute)}, false, true},
	}
	for _, c := range cases {
		if got := c.f.Match(&access); got != c.access {
			t.Errorf("%s: 访问日志匹配 = %v，期望 %v", c.name, got, c.access)
		}
		if got := c.f.Match(&login); got != c.login {
			t.Errorf("%s: 登录日志匹配 = %v，期望 %v", c.name, got, c.login)
		}
	}
	var nilFilter *Filter
	if !nilFilter.Match(&login) {
		t.Error("nil 过滤条件应匹配全部")
	}
}

func TestFilterTranslate(t *testing.T) {
	tr := func(s string) string {
		return strings.NewReplacer("登录失败", "login failed", "入侵防护", "Intrusion").Replace(s)
	}
	e := Entry{Level: LevelWarn, Source: "入侵防护", Message: "来自 5.6.7.8 的登录失败"}
	f := Filter{Terms: []string{"login failed"}, Translate: tr}
	if !f.Match(&e) {
		t.Error("关键词应能匹配译文")
	}
	f = Filter{Terms: []string{"登录失败"}, Translate: tr}
	if !f.Match(&e) {
		t.Error("关键词仍应能匹配原文")
	}
	f = Filter{Source: "intrusion", Translate: tr}
	if !f.Match(&e) {
		t.Error("来源应能匹配译文")
	}
}

func TestQueryFilter(t *testing.T) {
	b := New(100, nil)
	for i := 1; i <= 30; i++ {
		l := b.Logger("r")
		if i%3 == 0 {
			l.With(Fields{FieldIP: "10.0.0.1"}).Warnf("第 %d 条", i)
		} else {
			l.Infof("第 %d 条", i)
		}
	}
	f := &Filter{MinLevel: LevelWarn}
	if got := fmt.Sprint(seqs(b.Query(0, 0, 0, f))); got != "[3 6 9 12 15 18 21 24 27 30]" {
		t.Errorf("按级别过滤 = %s", got)
	}
	// limit 取最新的若干条，before 向前翻页，since 增量拉取
	if got := fmt.Sprint(seqs(b.Query(0, 0, 3, f))); got != "[24 27 30]" {
		t.Errorf("limit = %s", got)
	}
	if got := fmt.Sprint(seqs(b.Query(0, 24, 3, f))); got != "[15 18 21]" {
		t.Errorf("before = %s", got)
	}
	if got := fmt.Sprint(seqs(b.Query(25, 0, 0, f))); got != "[27 30]" {
		t.Errorf("since = %s", got)
	}
	if got := fmt.Sprint(seqs(b.Query(0, 0, 2, &Filter{IP: ParseIP("10.0.0.1")}))); got != "[27 30]" {
		t.Errorf("按 IP 过滤 = %s", got)
	}
	if got := b.Query(0, 0, 0, nil); len(got) != 30 {
		t.Errorf("无过滤条件应返回全部，得到 %d 条", len(got))
	}
}

// TestQueryFile 验证内存缓冲凑不够时会继续查找日志文件（含 .1 历史文件），且不与缓冲重复。
func TestQueryFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.log")
	b := New(5, nil)
	// 文件上限很小，写入过程中会轮转出 .1
	if err := b.Persist(path, 1000); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 20; i++ {
		lv := LevelInfo
		if i%2 == 0 {
			lv = LevelError
		}
		b.Add(lv, "", fmt.Sprintf("第 %d 条日志", i))
		if i%4 == 0 {
			// 分批写盘，触发轮转
			b.sink.flush()
		}
	}
	b.sink.flush()
	defer b.ClosePersist()

	// 缓冲只有 16..20，更早的要从文件找
	f := &Filter{MinLevel: LevelError}
	got := seqs(b.Query(0, 0, 4, f))
	if fmt.Sprint(got) != "[14 16 18 20]" {
		t.Errorf("跨缓冲与文件查询 = %v", got)
	}
	// 加载更早：before 落在文件范围内
	got = seqs(b.Query(0, 14, 2, f))
	if fmt.Sprint(got) != "[10 12]" {
		t.Errorf("从文件加载更早的日志 = %v", got)
	}
	// 关键词预过滤：只在文件中出现的那条
	got = seqs(b.Query(0, 0, 10, &Filter{Terms: []string{"第 13 条"}}))
	if fmt.Sprint(got) != "[13]" {
		t.Errorf("文件中按关键词查找 = %v", got)
	}
	// 不带过滤条件时，加载更早的日志也能翻到文件里，结果按序号递增且不重复
	all := seqs(b.Query(0, 0, 100, nil))
	for i := 1; i < len(all); i++ {
		if all[i] <= all[i-1] {
			t.Fatalf("结果应按序号递增且不重复: %v", all)
		}
	}
	if fmt.Sprint(all) != "[9 10 11 12 13 14 15 16 17 18 19 20]" {
		t.Errorf("应包含文件中的历史日志: %v", all)
	}
	// since 落在缓冲范围内时只查缓冲
	if got := fmt.Sprint(seqs(b.Query(18, 0, 10, nil))); got != "[19 20]" {
		t.Errorf("增量拉取 = %s", got)
	}
	// limit 为 0 时不查文件
	if got := b.Query(0, 0, 0, nil); len(got) != 5 {
		t.Errorf("limit 为 0 时只返回缓冲，得到 %d 条", len(got))
	}
}

func TestScanBackward(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.log")
	var sb strings.Builder
	n := 5000
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, `{"seq":%d,"message":"%s"}`+"\n", i+1, strings.Repeat("x", i%50))
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	size := int64(sb.Len())
	ctx := context.Background()
	var got []uint64
	done, err := scanBackward(ctx, file, size, func(line []byte) bool {
		s, ok := lineSeq(line)
		if !ok {
			t.Fatalf("无法解析行 %q", line)
		}
		got = append(got, s)
		return true
	})
	if err != nil || !done || len(got) != n || got[0] != uint64(n) || got[n-1] != 1 {
		t.Fatalf("倒序读取结果不对: done=%v 共 %d 条", done, len(got))
	}
	for i := 1; i < n; i++ {
		if got[i] != got[i-1]-1 {
			t.Fatalf("第 %d 行顺序错误: %d 之后是 %d", i, got[i-1], got[i])
		}
	}
	// 提前停止
	count := 0
	if done, err := scanBackward(ctx, file, size, func([]byte) bool { count++; return count < 3 }); done || err != nil || count != 3 {
		t.Errorf("提前停止失败，count=%d", count)
	}
	// 文件比记下的大小短（读不满）时返回错误，调用方不会跳过这一段继续读更早的文件
	if done, err := scanBackward(ctx, file, size+10, func([]byte) bool { return true }); done || !errors.Is(err, io.EOF) {
		t.Errorf("读取不完整时应返回错误: done=%v err=%v", done, err)
	}
	// 已取消的 ctx 立即停止
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if done, err := scanBackward(canceled, file, size, func([]byte) bool { return true }); done || !errors.Is(err, context.Canceled) {
		t.Errorf("取消后应停止: done=%v err=%v", done, err)
	}
}

// TestQueryPending 验证刚被挤出内存缓冲、尚未批量写盘的日志也能立即查到。
func TestQueryPending(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.log")
	b := New(10, nil)
	if err := b.Persist(path, 0); err != nil {
		t.Fatal(err)
	}
	defer b.ClosePersist()
	for i := 1; i <= 30; i++ {
		b.Add(LevelInfo, "", fmt.Sprintf("第 %d 条", i))
	}
	// 不等待延迟写盘，前 20 条此时还在 pending 中
	got := seqs(b.Query(0, 0, 100, nil))
	if len(got) != 30 || got[0] != 1 || got[29] != 30 {
		t.Fatalf("应返回全部 30 条，得到 %v", got)
	}
	// 带过滤条件时同样覆盖
	if got := fmt.Sprint(seqs(b.Query(0, 0, 100, &Filter{Terms: []string{"第 5 条"}}))); got != "[5]" {
		t.Errorf("按关键词查找 pending 中的日志 = %s", got)
	}
}

// TestSearchStopsOnError 验证当前文件读取出错时不再继续读 .1，避免结果中间缺一段。
func TestSearchStopsOnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "e.log")
	b := New(2, nil)
	if err := b.Persist(path, 0); err != nil {
		t.Fatal(err)
	}
	defer b.ClosePersist()
	for i := 1; i <= 6; i++ {
		b.Add(LevelInfo, "", fmt.Sprintf("第 %d 条", i))
	}
	b.sink.flush()
	// 手动轮转：前 6 条进入 .1；再写 4 条到当前文件
	if err := os.Rename(path, path+".1"); err != nil {
		t.Fatal(err)
	}
	b.sink.size = 0
	for i := 7; i <= 10; i++ {
		b.Add(LevelInfo, "", fmt.Sprintf("第 %d 条", i))
	}
	b.sink.flush()
	if got := fmt.Sprint(seqs(b.Query(0, 0, 100, nil))); got != "[1 2 3 4 5 6 7 8 9 10]" {
		t.Fatalf("正常情况应跨两个文件，得到 %s", got)
	}
	// 当前文件比快照记下的大小短：模拟读取出错，此时不应继续读 .1 的 1..6
	files := b.sink.snapshot()
	defer func() {
		for _, sf := range files {
			_ = sf.file.Close()
		}
	}()
	if len(files) != 2 {
		t.Fatalf("应打开两个文件，实际 %d", len(files))
	}
	if got := searchFiles(context.Background(), files, 0, 100, 100, nil); len(got) != 10 {
		t.Fatalf("快照正常时应读到 10 条，得到 %d", len(got))
	}
	files[0].size++
	if got := searchFiles(context.Background(), files, 0, 100, 100, nil); len(got) != 0 {
		t.Errorf("当前文件读取出错后不应继续读 .1，得到 %v", seqs(got))
	}
	// 已取消的查询不读文件，只返回缓冲内的结果
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := fmt.Sprint(seqs(b.QueryContext(ctx, 0, 0, 100, nil))); got != "[9 10]" {
		t.Errorf("取消后应只返回缓冲内容，得到 %s", got)
	}
}

// TestIPMatcherExclude 验证 IP 条件的排除项：前缀 ! 或 - 的地址、网段与片段命中即排除，可以与包含项组合。
func TestIPMatcherExclude(t *testing.T) {
	cases := []struct {
		expr string
		want map[string]bool
	}{
		// 只有排除项：其余地址都匹配
		{"!1.2.3.4, -10.0.0.0/8", map[string]bool{"1.2.3.4": false, "1.2.3.4:80": false, "10.9.9.9": false, "::ffff:10.0.0.1": false, "1.2.3.5": true, "2001:db8::1": true}},
		// 包含与排除组合：先排除再要求命中包含项
		{"10.0.0.0/8 !10.0.0.5", map[string]bool{"10.0.0.4": true, "10.0.0.5": false, "11.0.0.1": false}},
		// IPv6 网段与地址片段
		{"!2001:db8::/32 !192.168.", map[string]bool{"2001:db8::9": false, "[2001:db8::9]:443": false, "2001:db9::1": true, "192.168.1.1": false, "172.16.0.1": true}},
		// 映射网段同样转换
		{"!::ffff:1.2.3.0/120", map[string]bool{"1.2.3.9": false, "1.2.4.1": true}},
	}
	for _, c := range cases {
		m := ParseIP(c.expr)
		if m == nil {
			t.Fatalf("%q 不应解析为空", c.expr)
		}
		for in, want := range c.want {
			if got := m.match(in); got != want {
				t.Errorf("%q match(%q) = %v，期望 %v", c.expr, in, got, want)
			}
		}
	}
	// 单独的 ! 或 - 不构成条件
	if ParseIP("! -") != nil {
		t.Error("只有 ! 与 - 时应返回 nil")
	}

	// 日志没有 ip 字段时，消息中出现任意一个被排除的地址即排除；只有排除项时不含地址的日志照常显示
	m := ParseIP("!5.6.7.8")
	for _, c := range []struct {
		e    Entry
		want bool
	}{
		{Entry{Message: "来自 5.6.7.8 的登录失败"}, false},
		{Entry{Message: "转发 1.1.1.1:80 -> 5.6.7.8:22"}, false},
		{Entry{Message: "转发 1.1.1.1:80 -> 9.9.9.9:22"}, true},
		{Entry{Message: "模块已启动"}, true},
		// 有 ip 字段时只看字段，消息里出现的其他地址（例如后端地址）不影响
		{Entry{Message: "1.1.1.1 GET a/ 502 后端 5.6.7.8 不可用", Fields: Fields{FieldIP: "1.1.1.1"}}, true},
		{Entry{Message: "5.6.7.8 GET a/ 200", Fields: Fields{FieldIP: "5.6.7.8"}}, false},
	} {
		if got := m.matchEntry(&c.e); got != c.want {
			t.Errorf("matchEntry(%q) = %v，期望 %v", c.e.Message, got, c.want)
		}
	}
	// 有包含项时，不含地址的日志不匹配（与原有行为一致）
	if ParseIP("10.0.0.0/8 !10.0.0.5").matchEntry(&Entry{Message: "模块已启动"}) {
		t.Error("有包含项时不含地址的日志不应匹配")
	}
}

// TestFilterHide 验证按字段隐藏：被封禁来源被拒绝的访问日志、管理员自己的请求，其余日志不受影响。
func TestFilterHide(t *testing.T) {
	banned := Entry{Level: LevelInfo, Message: "1.2.3.4 GET a/ 403", Fields: Fields{FieldIP: "1.2.3.4", FieldStatus: "403", FieldDeny: DenyBan}}
	ipDenied := Entry{Level: LevelInfo, Message: "1.2.3.5 GET a/ 403", Fields: Fields{FieldIP: "1.2.3.5", FieldStatus: "403", FieldDeny: "ip"}}
	admin := Entry{Level: LevelInfo, Message: "1.2.3.6 GET a/api/webservice/logs 200", Fields: Fields{FieldIP: "1.2.3.6", FieldStatus: "200", FieldAdmin: "admin"}}
	normal := Entry{Level: LevelInfo, Message: "1.2.3.7 GET a/ 200", Fields: Fields{FieldIP: "1.2.3.7", FieldStatus: "200"}}
	plain := Entry{Level: LevelWarn, Message: "模块已启动"}

	hideBan := FieldCond{Key: FieldDeny, Value: DenyBan}
	hideAdmin := FieldCond{Key: FieldAdmin}
	cases := []struct {
		name string
		f    Filter
		want [5]bool // banned、ipDenied、admin、normal、plain
	}{
		{"隐藏被封禁来源", Filter{Hide: []FieldCond{hideBan}}, [5]bool{false, true, true, true, true}},
		{"隐藏后台请求", Filter{Hide: []FieldCond{hideAdmin}}, [5]bool{true, true, false, true, true}},
		{"同时隐藏", Filter{Hide: []FieldCond{hideBan, hideAdmin}}, [5]bool{false, true, false, true, true}},
		{"与其他条件为且", Filter{Hide: []FieldCond{hideBan}, Status: []StatusRange{{400, 499}}}, [5]bool{false, true, false, false, false}},
	}
	for _, c := range cases {
		if c.f.Empty() {
			t.Fatalf("%s: 带隐藏条件时不应视为空条件", c.name)
		}
		for i, e := range []Entry{banned, ipDenied, admin, normal, plain} {
			if got := c.f.Match(&e); got != c.want[i] {
				t.Errorf("%s: 第 %d 条（%s）匹配 = %v，期望 %v", c.name, i, e.Message, got, c.want[i])
			}
		}
	}
}

// TestQueryHideFile 验证隐藏条件对日志文件中的历史同样生效，且隐藏的日志不占 limit 名额。
func TestQueryHideFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web.log")
	b := New(4, nil)
	if err := b.Persist(path, 1<<20); err != nil {
		t.Fatal(err)
	}
	defer b.ClosePersist()
	l := b.Logger("HTTP :80")
	for i := 0; i < 20; i++ {
		l.With(Fields{FieldIP: "1.2.3.4", FieldStatus: "403", FieldDeny: DenyBan}).Infof("1.2.3.4 GET a/ 403 #%d", i)
		if i%5 == 0 {
			l.With(Fields{FieldIP: "5.6.7.8", FieldStatus: "200"}).Infof("5.6.7.8 GET a/ 200 #%d", i)
		}
	}
	got := b.Query(0, 0, 3, &Filter{Hide: []FieldCond{{Key: FieldDeny, Value: DenyBan}}})
	if len(got) != 3 {
		t.Fatalf("应从文件中凑够 3 条未隐藏的日志，得到 %d 条: %+v", len(got), got)
	}
	for _, e := range got {
		if e.Fields[FieldDeny] != "" {
			t.Errorf("不应返回被隐藏的日志: %+v", e)
		}
	}
	if got[2].Message != "5.6.7.8 GET a/ 200 #15" || got[0].Message != "5.6.7.8 GET a/ 200 #5" {
		t.Errorf("结果应为最新的 3 条且按时间顺序: %+v", got)
	}
}
