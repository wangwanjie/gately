package proc

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/logbuf"
)

func newLog() (*logbuf.Buffer, *logbuf.Logger) {
	b := logbuf.New(100, nil)
	return b, b.Logger("t")
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("等待条件超时")
}

func hasLog(b *logbuf.Buffer, level, sub string) bool {
	for _, e := range b.Since(0) {
		if e.Level == level && strings.Contains(e.Message, sub) {
			return true
		}
	}
	return false
}

func TestClean(t *testing.T) {
	cases := map[string]string{
		"2026-01-02T03:04:05Z INF connected": "INF connected",
		"2026/01/02 03:04:05 hello":          "hello",
		"2026-01-02 03:04:05.123 [I] start":  "[I] start",
		"\x1b[31mred\x1b[0m\r":               "red",
	}
	for in, want := range cases {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q，期望 %q", in, got, want)
		}
	}
}

func TestClassify(t *testing.T) {
	cases := map[string]string{
		"[E] login failed":    logbuf.LevelError,
		"ERR connection lost": logbuf.LevelError,
		"[W] retry":           logbuf.LevelWarn,
		"WRN slow":            logbuf.LevelWarn,
		"ERRO client/iface/netstack/tun.go:65: error": logbuf.LevelError,
		"WARN client/internal/engine.go:965: failed":  logbuf.LevelWarn,
		"DEBU client/internal/peer/conn.go:1: x":      logbuf.LevelDebug,
		"[I] ok":                                      logbuf.LevelInfo,
		"plain line":                                  logbuf.LevelInfo,
	}
	for in, want := range cases {
		if got := classify(in); got != want {
			t.Errorf("classify(%q) = %s，期望 %s", in, got, want)
		}
	}
}

func TestSupervisorLogsAndRestarts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("依赖 sh")
	}
	buf, log := newLog()
	var lines atomic.Int32
	s := New(log)
	s.Start(Spec{Path: "sh", Args: []string{"-c", "echo '[E] boom'; exit 3"}, OnLine: func(string) { lines.Add(1) }})
	defer s.Stop()
	// 进程立即退出，托管器应记录退出原因并自动重启
	waitFor(t, func() bool { return s.State().Restarts >= 1 })
	if !hasLog(buf, logbuf.LevelError, "boom") {
		t.Errorf("未捕获输出日志: %+v", buf.Since(0))
	}
	if lines.Load() == 0 {
		t.Error("OnLine 未被调用")
	}
	if !strings.Contains(s.State().LastExit, "3") {
		t.Errorf("退出原因应包含退出码: %q", s.State().LastExit)
	}
}

func TestSupervisorStop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("依赖 sh")
	}
	_, log := newLog()
	s := New(log)
	s.Start(Spec{Path: "sleep", Args: []string{"60"}})
	waitFor(t, func() bool { return s.State().Running })
	start := time.Now()
	s.Stop()
	if time.Since(start) > 4*time.Second {
		t.Error("停止耗时过长，终止信号未生效")
	}
	if s.State().Running {
		t.Error("Stop 后仍显示运行中")
	}
}

func TestSupervisorMissingBinary(t *testing.T) {
	_, log := newLog()
	s := New(log)
	s.Start(Spec{Path: "/nonexistent/gately-test-bin"})
	defer s.Stop()
	waitFor(t, func() bool { return s.State().Error != "" })
	if s.State().Restarts != 0 {
		t.Error("二进制缺失不应进入重启循环")
	}
}

func TestRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("依赖 sh")
	}
	buf, log := newLog()
	if err := Run(context.Background(), Spec{Path: "sh", Args: []string{"-c", "echo done"}}, log); err != nil {
		t.Fatal(err)
	}
	if !hasLog(buf, logbuf.LevelInfo, "done") {
		t.Error("未捕获输出")
	}
	if err := Run(context.Background(), Spec{Path: "sh", Args: []string{"-c", "exit 2"}}, log); err == nil {
		t.Error("非零退出应返回错误")
	}
}

func TestResolve(t *testing.T) {
	if _, err := Resolve("", t.TempDir(), "gately-no-such-binary"); err == nil || !strings.Contains(err.Error(), "gately-no-such-binary") {
		t.Errorf("应提示缺失的二进制名: %v", err)
	}
	if _, err := Resolve("/nonexistent/x", t.TempDir(), "x"); err == nil {
		t.Error("指定路径不存在应报错")
	}
	if p, err := Resolve("", t.TempDir(), "sh"); err != nil || p == "" {
		t.Errorf("应在 PATH 中找到 sh: %v", err)
	}
	// 配置目录 bin 下在线安装的程序优先于 PATH 中的同名程序
	if runtime.GOOS != "windows" {
		dir := t.TempDir()
		local := filepath.Join(dir, "bin", "sh")
		if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(local, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		if p, err := Resolve("", dir, "sh"); err != nil || p != local {
			t.Errorf("应优先使用配置目录 bin 下的程序: %q %v", p, err)
		}
		// 相对的配置目录与相对的配置路径都返回绝对路径，避免子进程换工作目录后找不到程序
		t.Chdir(dir)
		if p, err := Resolve("", "", "sh"); err != nil || p != local {
			t.Errorf("相对配置目录应解析为绝对路径: %q %v", p, err)
		}
		if p, err := Resolve(filepath.Join("bin", "sh"), "", "sh"); err != nil || p != local {
			t.Errorf("相对配置路径应解析为绝对路径: %q %v", p, err)
		}
	}
}
