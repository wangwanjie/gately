package logbuf

import (
	"path/filepath"
	"strings"
	"testing"
)

// 最低级别以下的日志直接丢弃：不进缓冲、不写文件、不回显，序号也不为它们递增。新建的缓冲记录全部级别。
func TestMinLevel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.log")
	b := New(100, nil)
	var out syncBuffer
	echo := NewEcho(&out, nil)
	b.SetEcho(echo)
	if err := b.Persist(path, 0); err != nil {
		t.Fatal(err)
	}
	l := b.Logger("s")
	write := func() {
		l.Debugf("调试 %d", 1)
		l.Infof("信息 %d", 2)
		l.Warnf("警告 %d", 3)
		l.Errorf("错误 %d", 4)
		b.Add(LevelDebug, "", "直接写入的调试")
		l.Log(LevelDebug, "托管进程的调试输出")
		l.Log("notice", "未知级别")
	}
	levels := func() string {
		var s []string
		for _, e := range b.Since(0) {
			s = append(s, e.Level)
		}
		return strings.Join(s, ",")
	}
	write()
	if got := levels(); got != "debug,info,warn,error,debug,debug,notice" {
		t.Fatalf("默认应记录全部级别，得到 %s", got)
	}
	cases := map[string]string{
		LevelInfo:  "info,warn,error,notice",
		LevelWarn:  "warn,error,notice",
		LevelError: "error,notice",
		LevelDebug: "debug,info,warn,error,debug,debug,notice",
		"bogus":    "debug,info,warn,error,debug,debug,notice",
	}
	for level, want := range cases {
		b.Clear()
		b.SetMinLevel(level)
		if b.Enabled(LevelDebug) != strings.HasPrefix(want, "debug") || !b.Enabled(LevelError) || l.Enabled(LevelInfo) != strings.Contains(want, "info") {
			t.Errorf("最低级别 %s 时 Enabled 的结果不对", level)
		}
		first := b.nextSeq
		write()
		if got := levels(); got != want {
			t.Errorf("最低级别 %s：得到 %s，应为 %s", level, got, want)
		}
		if n := int(b.nextSeq - first); n != strings.Count(want, ",")+1 {
			t.Errorf("最低级别 %s：序号递增了 %d，应只为记录下来的日志递增", level, n)
		}
	}
	// 文件与回显里同样没有被丢弃的日志
	b.Clear()
	b.SetMinLevel(LevelInfo)
	echo.Flush()
	out.Reset()
	write()
	b.ClosePersist()
	echo.Flush()
	file := readTail(path, 100)
	if len(file) != 4 {
		t.Fatalf("文件里应有 4 条，得到 %d", len(file))
	}
	for _, e := range file {
		if e.Level == LevelDebug {
			t.Fatalf("调试日志不应写入文件: %+v", e)
		}
	}
	if s := out.String(); strings.Contains(s, "调试") || strings.Count(s, "\n") != 4 {
		t.Fatalf("回显内容不对:\n%s", s)
	}
}

// 调试级别关闭时 Debugf 在格式化之前返回，自身不分配内存；Infof 等同理。
func TestDisabledLevelDoesNotAllocate(t *testing.T) {
	b := New(100, nil)
	b.SetMinLevel(LevelInfo)
	l := b.Logger("s")
	// 参数事先装进 []any：调用点把值转成接口本身可能分配，那发生在进入 Debugf 之前
	args := []any{"10.0.0.1:1234", "10.0.0.2:80", 42}
	if n := testing.AllocsPerRun(1000, func() { l.Debugf("%s -> %s 已连接 %d", args...) }); n != 0 {
		t.Errorf("调试级别关闭时 Debugf 分配了 %v 次内存", n)
	}
	b.SetMinLevel(LevelError)
	if n := testing.AllocsPerRun(1000, func() {
		l.Infof("%s -> %s 已连接 %d", args...)
		l.Warnf("%s -> %s 已连接 %d", args...)
	}); n != 0 {
		t.Errorf("级别关闭时 Infof / Warnf 分配了 %v 次内存", n)
	}
	if len(b.Since(0)) != 0 {
		t.Error("关闭的级别不应写入日志")
	}
}
