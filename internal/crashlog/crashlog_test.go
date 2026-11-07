package crashlog

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// 子进程模式：GATELY_CRASHLOG_CHILD 指定崩溃方式，GATELY_CRASHLOG_DIR 为配置目录。
// 子进程开启崩溃记录后真实崩溃，父进程再检查下次启动能否整理出崩溃信息。
func TestMain(m *testing.M) {
	if mode := os.Getenv("GATELY_CRASHLOG_CHILD"); mode != "" {
		runChild(mode, os.Getenv("GATELY_CRASHLOG_DIR"))
		return
	}
	os.Exit(m.Run())
}

//go:noinline
func crashInGoroutine() {
	go func() {
		var m map[string]int
		// 向 nil map 写入：运行时 panic，且发生在非主协程，未被恢复时整个进程退出
		m["198.51.100.7"] = 1
	}()
	// 主协程不能先于崩溃输出结束进程
	time.Sleep(10 * time.Second)
}

func runChild(mode, dir string) {
	rec, _, err := Start(dir, "gately test child")
	if err != nil {
		os.Exit(10)
	}
	switch mode {
	case "panic":
		crashInGoroutine()
	case "fatal":
		// sync 包的致命错误不能被 recover，走 runtime.fatal 路径
		var mu sync.Mutex
		mu.Unlock()
	case "exitfatal":
		rec.Fatal("run failed: listen tcp :16680: bind: address already in use")
		os.Exit(1)
	case "killed":
		// 不调用 Close 直接被结束，模拟 kill -9 / OOM
		p, _ := os.FindProcess(os.Getpid())
		_ = p.Kill()
		time.Sleep(10 * time.Second)
	case "clean":
		rec.Close()
	}
	os.Exit(0)
}

// runCrashChild 在子进程中开启崩溃记录并按 mode 退出，返回子进程的输出。
func runCrashChild(t *testing.T, dir, mode string) string {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "GATELY_CRASHLOG_CHILD="+mode, "GATELY_CRASHLOG_DIR="+dir, "GOTRACEBACK=")
	out, _ := cmd.CombinedOutput()
	return string(out)
}

// 子进程里真实发生的 panic 与 fatal error：下次启动判定为崩溃，crash.log 带有版本首行、panic 信息与崩溃位置的堆栈，
// 运行标记文件随后删除，再次启动不会重复报告。
func TestRealCrashIsRecorded(t *testing.T) {
	cases := []struct {
		mode, reason, stack string
	}{
		{"panic", "panic: assignment to entry in nil map", "crashlog.crashInGoroutine.func1"},
		// 致命错误的 "fatal error: ..." 一行 Go 只写到 stderr，原因改为出错的函数
		{"fatal", "fatal error in internal/sync.(*Mutex).unlockSlow", "crashlog.runChild"},
	}
	for _, c := range cases {
		dir := t.TempDir()
		out := runCrashChild(t, dir, c.mode)
		if !strings.Contains(out, "goroutine ") {
			t.Fatalf("%s: 子进程应在 stderr 输出崩溃信息，实际: %s", c.mode, out)
		}
		rec, prev, err := Start(dir, "gately next")
		if err != nil {
			t.Fatal(err)
		}
		rec.Close()
		if prev.Kind != Crashed || prev.Reason != c.reason {
			t.Fatalf("%s: 应判定为崩溃并给出首行原因，实际 %+v", c.mode, prev)
		}
		if time.Since(prev.Time) > time.Minute {
			t.Fatalf("%s: 崩溃时间应为标记文件最后写入的时间: %v", c.mode, prev.Time)
		}
		log, _ := os.ReadFile(filepath.Join(dir, LogName))
		for _, want := range []string{"===== ", " crash | gately test child =====", c.reason, "goroutine ", c.stack, "crashlog_test.go:"} {
			if !strings.Contains(string(log), want) {
				t.Fatalf("%s: crash.log 缺少 %q:\n%s", c.mode, want, log)
			}
		}
		if _, err := os.Stat(filepath.Join(dir, runName)); !os.IsNotExist(err) {
			t.Fatalf("%s: Close 后应删除运行标记文件: %v", c.mode, err)
		}
		rec, prev, _ = Start(dir, "gately again")
		rec.Close()
		if prev.Kind != None {
			t.Fatalf("%s: 正常退出后再次启动不应报告异常: %+v", c.mode, prev)
		}
	}
}

// 记录了致命错误后退出（例如管理端口被占用）按崩溃报告，原因为记录的错误。
func TestFatalExitIsRecorded(t *testing.T) {
	dir := t.TempDir()
	runCrashChild(t, dir, "exitfatal")
	rec, prev, err := Start(dir, "gately next")
	if err != nil {
		t.Fatal(err)
	}
	rec.Close()
	if prev.Kind != Crashed || prev.Reason != "fatal: run failed: listen tcp :16680: bind: address already in use" {
		t.Fatalf("应报告记录的致命错误: %+v", prev)
	}
}

// 进程被 SIGKILL 结束时没有任何输出：判定为被强制结束，crash.log 写明没有崩溃信息及可能原因；
// 正常调用 Close 退出则不报告。
func TestKilledAndCleanExit(t *testing.T) {
	dir := t.TempDir()
	runCrashChild(t, dir, "killed")
	rec, prev, err := Start(dir, "gately next")
	if err != nil {
		t.Fatal(err)
	}
	rec.Close()
	if prev.Kind != Killed || prev.Reason != "" {
		t.Fatalf("被强制结束应判定为 killed: %+v", prev)
	}
	log, _ := os.ReadFile(filepath.Join(dir, LogName))
	if !strings.Contains(string(log), " killed | gately test child =====") || !strings.Contains(string(log), "OOM killer") {
		t.Fatalf("crash.log 应记录被强制结束: %s", log)
	}

	clean := t.TempDir()
	runCrashChild(t, clean, "clean")
	rec, prev, _ = Start(clean, "gately next")
	rec.Close()
	if prev.Kind != None {
		t.Fatalf("正常退出不应报告异常: %+v", prev)
	}
	if _, err := os.Stat(filepath.Join(clean, LogName)); !os.IsNotExist(err) {
		t.Fatalf("没有异常时不应创建 crash.log: %v", err)
	}
}

// 心跳刷新运行标记文件的修改时间，被强制结束时据此估计结束时间。
func TestHeartbeatTouchesRunFile(t *testing.T) {
	old := heartbeat
	heartbeat = 20 * time.Millisecond
	defer func() { heartbeat = old }()
	dir := t.TempDir()
	rec, _, err := Start(dir, "gately")
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Close()
	path := filepath.Join(dir, runName)
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if time.Since(st.ModTime()) < time.Minute {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("心跳没有刷新运行标记文件的修改时间")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// crash.log 超过上限时从最早的整条记录开始丢弃，保留最新的记录且以记录标题开头。
func TestLogIsTrimmedByEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, LogName)
	big := strings.Repeat("x", 300<<10)
	for i := 0; i < 5; i++ {
		entry := entryPrefix + "entry " + string(rune('A'+i)) + " =====\n" + big + "\n\n"
		if err := appendLog(path, []byte(entry)); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(path)
	if len(data) > maxLogBytes {
		t.Fatalf("crash.log 不应超过上限: %d", len(data))
	}
	s := string(data)
	if !strings.HasPrefix(s, entryPrefix) || !strings.Contains(s, "entry E") || strings.Contains(s, "entry A") {
		t.Fatalf("应只保留最新的完整记录: 开头 %q", s[:40])
	}
}

// 同一配置目录已有实例在运行时，第二个实例拿不到实例锁：不整理、不删除前者的运行标记，也不误报意外结束；
// 前者退出（或被强制结束）释放锁后才能正常开启。
func TestSecondInstanceKeepsRunningMarker(t *testing.T) {
	dir := t.TempDir()
	first, _, err := Start(dir, "gately first")
	if err != nil {
		t.Fatal(err)
	}
	run := filepath.Join(dir, runName)
	second, prev, err := Start(dir, "gately second")
	if !errors.Is(err, ErrLocked) || second != nil || prev.Kind != None {
		t.Fatalf("第二个实例应得到 ErrLocked 且不报告上次运行: %v %v %+v", err, second, prev)
	}
	if data, _ := os.ReadFile(run); !strings.HasPrefix(string(data), "gately first") {
		t.Fatalf("第一个实例的运行标记被改动: %q", data)
	}
	if _, err := os.Stat(filepath.Join(dir, LogName)); !os.IsNotExist(err) {
		t.Fatalf("不应把运行中实例的标记写进 crash.log: %v", err)
	}
	// 第一个实例被强制结束：锁释放、标记保留，下一个实例报告意外结束
	first.Abandon()
	third, prev, err := Start(dir, "gately third")
	if err != nil || prev.Kind != Killed {
		t.Fatalf("前一个实例结束后应能开启并报告意外结束: %v %+v", err, prev)
	}
	third.Close()
	fourth, _, err := Start(dir, "gately fourth")
	if err != nil {
		t.Fatalf("正常退出后应能再次开启: %v", err)
	}
	fourth.Close()
}

// 裁剪位置恰好落在某条记录的标题行开头时，这条记录应完整保留。
func TestTrimHeadKeepsEntryAtCutBoundary(t *testing.T) {
	a := entryPrefix + "A =====\n" + strings.Repeat("a", 50) + "\n\n"
	b := entryPrefix + "B =====\n" + strings.Repeat("b", 50) + "\n\n"
	data := []byte(a + b)
	got := string(trimHead(data, len(b)))
	if got != b {
		t.Fatalf("应完整保留记录 B，实际 %q", got)
	}
	if got := string(trimHead(data, len(data))); got != a+b {
		t.Fatalf("未超限时不应裁剪: %q", got)
	}
}

// 单条记录就超过上限时保留开头（标题与崩溃原因）和结尾，中间按整行截断。
func TestTrimHeadTruncatesHugeEntryInTheMiddle(t *testing.T) {
	path := filepath.Join(t.TempDir(), LogName)
	if err := appendLog(path, []byte(entryPrefix+"old =====\nold\n\n")); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString(entryPrefix + "2026-11-07T00:00:00Z crash | gately v0.6.5.1 =====\nfatal error in main.handler\n\n")
	for i := 0; b.Len() < 3*maxLogBytes; i++ {
		b.WriteString("goroutine " + strconv.Itoa(i) + " [running]:\nmain.worker(0xc000123400)\n\t/src/main.go:42 +0x1d\n\n")
	}
	b.WriteString("last line of the dump\n\n")
	if err := appendLog(path, []byte(b.String())); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	s := string(data)
	if len(data) > maxLogBytes {
		t.Fatalf("crash.log 不应超过上限: %d", len(data))
	}
	if !strings.HasPrefix(s, entryPrefix+"2026-11-07T00:00:00Z crash | gately v0.6.5.1 =====\nfatal error in main.handler\n") {
		t.Fatalf("应保留最新记录的开头: %q", s[:120])
	}
	if strings.Contains(s, "old") || !strings.Contains(s, "bytes truncated) ...") || !strings.HasSuffix(s, "last line of the dump\n\n") {
		t.Fatalf("应丢弃旧记录、保留结尾并标明截断: 结尾 %q", s[len(s)-80:])
	}
	// 截断点对齐整行：截断说明前后都是完整的行
	i := strings.Index(s, "\n... (")
	if !strings.HasSuffix(s[:i], "\n") && !strings.HasSuffix(s[:i], "+0x1d") && !strings.HasSuffix(s[:i], "]:") && !strings.HasSuffix(s[:i], ")") {
		t.Fatalf("截断点应在行尾: %q", s[i-40:i+10])
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Fatalf("crash.log 权限应为 0600: %v", st.Mode())
	}
	if m, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*.tmp")); len(m) != 0 {
		t.Fatalf("不应残留临时文件: %v", m)
	}
}
