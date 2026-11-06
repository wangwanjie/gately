package app

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/crashlog"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// sysLogCount 统计系统日志中包含 sub 的指定级别日志条数（含落盘后重启恢复的日志）。
func sysLogCount(a *App, level, sub string) int {
	n := 0
	for _, e := range a.sysLogs.Since(0) {
		if e.Level == level && strings.Contains(e.Message, sub) {
			n++
		}
	}
	return n
}

func sysLogHas(a *App, level, sub string) bool {
	return sysLogCount(a, level, sub) > 0
}

// 开启崩溃记录后：上次运行留下的崩溃信息在启动时写进系统日志并整理进 crash.log，诊断包带上脱敏后的 crash.log
// （堆栈函数名保留、访客 IP 去掉）；正常退出不留标记，下次启动不报告；没有退出就再次启动（等同被强制结束）报告意外结束。
func TestCrashLogLifecycle(t *testing.T) {
	dir := t.TempDir()
	// 模拟上次运行崩溃时运行时写下的内容
	prevRun := "gately v0.6.5 (5302aa1) pid 42 started 2026-11-06T14:40:00Z\n" +
		"panic: runtime error: invalid memory address or nil pointer dereference from 203.0.113.9\n\n" +
		"goroutine 812 [running]:\n" +
		"golang.org/x/net/http2.(*serverConn).serve(0xc000123400)\n" +
		"\t/home/runner/go/pkg/mod/golang.org/x/net@v0.47.0/http2/server.go:952 +0x1d8\n"
	if err := os.WriteFile(filepath.Join(dir, "crash-run.log"), []byte(prevRun), 0o600); err != nil {
		t.Fatal(err)
	}
	listen := "127.0.0.1:" + strconv.Itoa(freeTCPPort(t))
	a, err := New(Options{ConfigDir: dir, Listen: listen, CrashLog: true})
	if err != nil {
		t.Fatal(err)
	}
	if !sysLogHas(a, logbuf.LevelError, "panic: runtime error: invalid memory address") {
		t.Fatalf("启动时应在系统日志报告上次崩溃: %+v", a.sysLogs.Since(0))
	}
	b, err := a.buildDiagnostics(true, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var crash string
	for _, f := range b.Files {
		if f.Name == "logs/"+crashlog.LogName {
			crash = string(f.Data)
		}
	}
	for _, want := range []string{"crash | gately v0.6.5 (5302aa1) pid 42", "golang.org/x/net/http2.(*serverConn).serve(0xc000123400)", "server.go:952", "<ipv4:public>"} {
		if !strings.Contains(crash, want) {
			t.Fatalf("诊断包的 crash.log 缺少 %q:\n%s", want, crash)
		}
	}
	if strings.Contains(crash, "203.0.113.9") {
		t.Fatalf("crash.log 应脱敏访客 IP:\n%s", crash)
	}
	if b, _ := a.buildDiagnostics(false, time.Now()); len(b.Files) > 0 {
		for _, f := range b.Files {
			if strings.HasPrefix(f.Name, "logs/") {
				t.Fatalf("不含日志时不应导出 %s", f.Name)
			}
		}
	}
	run := filepath.Join(dir, "crash-run.log")
	if data, err := os.ReadFile(run); err != nil || !strings.HasPrefix(string(data), "gately ") {
		t.Fatalf("运行期间应有本次的运行标记: %q %v", data, err)
	}

	// 正常退出：删除运行标记，下次启动不报告异常
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()
	time.Sleep(200 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(run); !os.IsNotExist(err) {
		t.Fatalf("正常退出后应删除运行标记: %v", err)
	}
	a2, err := New(Options{ConfigDir: dir, Listen: listen, CrashLog: true})
	if err != nil {
		t.Fatal(err)
	}
	// 系统日志落盘，重启后恢复的是第一次启动时那一条，不应新增
	if sysLogCount(a2, logbuf.LevelError, "崩溃退出") != 1 || sysLogHas(a2, logbuf.LevelWarn, "意外结束") {
		t.Fatal("正常退出后再次启动不应报告异常")
	}

	// a2 没有退出就再次启动，等同进程被强制结束
	a3, err := New(Options{ConfigDir: dir, Listen: listen, CrashLog: true})
	if err != nil {
		t.Fatal(err)
	}
	defer a3.crash.Close()
	if !sysLogHas(a3, logbuf.LevelWarn, "上次运行意外结束") {
		t.Fatalf("被强制结束后启动应报告意外结束: %+v", a3.sysLogs.Since(0))
	}
	data, _ := os.ReadFile(filepath.Join(dir, crashlog.LogName))
	if strings.Count(string(data), "===== ") != 2 || !strings.Contains(string(data), " killed | gately ") {
		t.Fatalf("crash.log 应依次记录崩溃与被强制结束:\n%s", data)
	}
}

// 管理端口被占用等导致 Run 返回错误时，错误写进崩溃记录，下次启动按崩溃报告这条错误。
func TestCrashLogRecordsRunError(t *testing.T) {
	dir := t.TempDir()
	a, err := New(Options{ConfigDir: dir, Listen: "127.0.0.1:bad-port", CrashLog: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Run(context.Background()); err == nil {
		t.Fatal("监听非法地址应失败")
	}
	a2, err := New(Options{ConfigDir: dir, Listen: "127.0.0.1:" + strconv.Itoa(freeTCPPort(t)), CrashLog: true})
	if err != nil {
		t.Fatal(err)
	}
	defer a2.crash.Close()
	if !sysLogHas(a2, logbuf.LevelError, "fatal: run failed: listen tcp") {
		t.Fatalf("应报告上次运行的致命错误: %+v", a2.sysLogs.Since(0))
	}
}
