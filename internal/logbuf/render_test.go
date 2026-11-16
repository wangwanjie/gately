package logbuf

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

type namedInt int

func (n namedInt) String() string { return "named" }

type namedStr string

func (s namedStr) Error() string { return "err:" + string(s) }

// nilErr 的 Error 方法在 nil 接收者上会 panic，fmt 对此有专门的输出（<nil>）
type nilErr struct{ msg string }

func (e *nilErr) Error() string { return e.msg }

// render 的结果必须与 fmt.Sprintf 逐字节相同，记录的参数长度加上格式串的固定片段恰好拼成整条消息。
func TestRenderMatchesSprintf(t *testing.T) {
	addr := &net.TCPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 443}
	var np *nilErr
	cases := []struct {
		format string
		args   []any
		// lens 为 false 表示这种写法不记录参数位置
		lens bool
	}{
		{"拒绝 %s 访问 %s（访问控制）", []any{"1.2.3.4", "example.com"}, true},
		{"%s -> %s 已连接", []any{addr, net.Addr(addr)}, true},
		{"端口 %d 已被占用", []any{8080}, true},
		{"%d/%d/%d/%d/%d/%d", []any{int64(-5), int32(7), uint16(65535), uint32(1), uint64(1 << 63), uint(9)}, true},
		{"%d %d %v %s", []any{namedInt(3), time.Second, namedInt(4), namedInt(5)}, true},
		{"丢包率 %.0f%%，延迟 %5.1fms", []any{12.4, 3.14159}, true},
		// 访问日志的格式：参数超过 6 个，结果仍与 fmt 相同，只是不记录位置
		{"%s %s %s%s %d %dms %q %q", []any{"1.1.1.1", "GET", "h", "/p?a=1", 200, int64(12), "-", "Mozilla/5.0 \"x\"\n"}, false},
		{"%s %s%s %d %dms %q", []any{"1.1.1.1", "h", "/p?a=1", 200, int64(12), "Mozilla/5.0 \"x\"\n"}, true},
		{"失败: %v", []any{errors.New("用户名或密码错误")}, true},
		{"失败: %v / %s / %v", []any{nil, nil, error(np)}, true},
		{"%v|%+v|%#v|%T|%x|%X|%08.3f|%-6s|%6s|%t|%c|%U", []any{namedStr("a"), struct{ A int }{1}, []string{"x"}, 1.5, 255, "hi", 3.14159, "ab", "cd", true, 'x', 0x4e2d}, false},
		{"没有动词", nil, true},
		{"100%% 完成", nil, true},
		{"", nil, true},
		{"%s", []any{""}, true},
		{"%s%s", []any{"a", "b"}, true},
		// 参数个数与动词不符、显式序号、星号宽度、不完整或未知的动词：整句交给 fmt
		{"%s 和 %s", []any{"只有一个"}, false},
		{"%s", []any{"a", "多一个"}, false},
		{"没有动词却有参数", []any{1}, false},
		{"%[2]s %[1]s", []any{"a", "b"}, false},
		{"%*d", []any{5, 42}, false},
		{"结尾的百分号 %", nil, false},
		{"%!", []any{1}, false},
		{"%z %s", []any{1, "x"}, true},
		{"%5%", nil, false},
		{"长参数 %s", []any{strings.Repeat("x", 70000)}, false},
		{"%d %d %d %d %d %d %d", []any{1, 2, 3, 4, 5, 6, 7}, false},
	}
	for _, c := range cases {
		want := fmt.Sprintf(c.format, c.args...)
		got, lens := render(c.format, c.args)
		if got != want {
			t.Errorf("render(%q) = %q，fmt.Sprintf 为 %q", c.format, got, want)
			continue
		}
		n := lens.Slice()
		if !c.lens || len(c.args) == 0 {
			if len(n) != 0 && !c.lens {
				t.Errorf("render(%q) 不应记录参数位置: %v", c.format, n)
			}
			continue
		}
		if len(n) != len(c.args) {
			t.Errorf("render(%q) 记录了 %d 个参数长度，应为 %d", c.format, len(n), len(c.args))
			continue
		}
		// 用固定片段与长度重新拼出消息
		p := parsePlan(c.format)
		var sb strings.Builder
		rest := got
		for i, l := range n {
			if !strings.HasPrefix(rest, p.lits[i]) {
				t.Errorf("render(%q): 第 %d 个固定片段对不上", c.format, i)
				break
			}
			rest = rest[len(p.lits[i]):]
			sb.WriteString(p.lits[i] + rest[:l])
			if want := fmt.Sprintf(p.specs[i], c.args[i]); rest[:l] != want {
				t.Errorf("render(%q): 第 %d 个参数切出 %q，应为 %q", c.format, i, rest[:l], want)
			}
			rest = rest[l:]
		}
		if sb.String()+rest != got || rest != p.lits[len(n)] {
			t.Errorf("render(%q): 按长度拼不回原消息", c.format)
		}
	}
}

// Logger 写入的日志带着格式串与参数长度；直接写入整句的日志没有。
func TestLoggerRecordsFormat(t *testing.T) {
	b := New(10, nil)
	b.Logger("r").With(Fields{FieldIP: "1.2.3.4"}).Warnf("拒绝 %s 访问 %s（访问控制）", "1.2.3.4", "a.example.com")
	b.Add(LevelInfo, "", "直接写入")
	b.Logger("r").Log(LevelInfo, "托管进程的输出")
	got := b.Since(0)
	if got[0].Message != "拒绝 1.2.3.4 访问 a.example.com（访问控制）" || got[0].Format != "拒绝 %s 访问 %s（访问控制）" {
		t.Fatalf("格式串没有记录: %+v", got[0])
	}
	if n := got[0].Args.Slice(); len(n) != 2 || n[0] != 7 || n[1] != 13 {
		t.Fatalf("参数长度不对: %v", n)
	}
	if got[0].Fields[FieldIP] != "1.2.3.4" || got[0].Source != "r" || got[0].Level != LevelWarn {
		t.Fatalf("其余字段不对: %+v", got[0])
	}
	for _, e := range got[1:] {
		if e.Format != "" || len(e.Args.Slice()) != 0 {
			t.Fatalf("整句写入的日志不应有格式串: %+v", e)
		}
	}
}

// 运行时拼出来的格式串不会让解析缓存无限增长。
func TestPlanCacheBounded(t *testing.T) {
	before := planCount.Load()
	for i := 0; i < 2*maxPlans; i++ {
		render(fmt.Sprintf("动态格式串 %d: %%s", i), []any{"x"})
	}
	if n := planCount.Load(); n > maxPlans || n < before {
		t.Fatalf("解析缓存条目数 %d 超过上限 %d", n, maxPlans)
	}
	// 缓存满了以后仍能正确格式化
	if got, _ := render("缓存已满 %d %s", []any{1, "x"}); got != "缓存已满 1 x" {
		t.Fatalf("得到 %q", got)
	}
}
