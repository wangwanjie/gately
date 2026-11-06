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
		"FATA failed to start daemon":                 logbuf.LevelError,
		"PANI runtime error":                          logbuf.LevelError,
		"TRAC client/internal/engine.go:1: x":         logbuf.LevelDebug,
		"FATAL boom":                                  logbuf.LevelError,
		"DEBUG verbose":                               logbuf.LevelDebug,
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

func TestMaskURLPaths(t *testing.T) {
	cases := map[string]string{
		"To authenticate, visit: https://login.tailscale.com/a/1a2b3c4d5e6f": "To authenticate, visit: https://login.tailscale.com/********",
		"https://hs.example.com:8080/register/nodekey:abc?x=1 now":           "https://hs.example.com:8080/******** now",
		"see https://example.com/ or http://example.com":                     "see https://example.com/ or http://example.com",
		"no url here": "no url here",
	}
	for in, want := range cases {
		if got := MaskURLPaths(in); got != want {
			t.Errorf("MaskURLPaths(%q) = %q，期望 %q", in, got, want)
		}
	}
}

// TestRunLogLine 验证 LogLine 只改写写入日志的内容，OnLine 收到原始输出。
func TestRunLogLine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("依赖 sh")
	}
	buf, log := newLog()
	var raw string
	spec := Spec{Path: "sh", Args: []string{"-c", "echo https://login.example.com/a/secret"}, LogLine: MaskURLPaths, OnLine: func(l string) { raw = l }}
	if err := Run(context.Background(), spec, log); err != nil {
		t.Fatal(err)
	}
	if hasLog(buf, logbuf.LevelInfo, "secret") || !hasLog(buf, logbuf.LevelInfo, "https://login.example.com/********") {
		t.Error("日志应隐藏链接路径")
	}
	if !strings.Contains(raw, "/a/secret") {
		t.Errorf("OnLine 应收到原始输出: %q", raw)
	}
}

// findManaged 在 Running 的结果中按 PID 查找
func findManaged(pid int) (Managed, bool) {
	for _, m := range Running() {
		if m.PID == pid {
			return m, true
		}
	}
	return Managed{}, false
}

// 运行中的托管进程会被 Running 列出，只带归属与程序名；停止或无法启动的不列出
func TestRunningRegistry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("依赖 sleep 命令")
	}
	_, log := newLog()
	s := New(log)
	s.Start(Spec{Path: "sleep", Args: []string{"60"}, Env: []string{"GATELY_TEST_SECRET=hunter2"}, Module: "frp", Name: "家里"})
	waitFor(t, func() bool { return s.State().Running })
	pid := s.State().PID
	m, ok := findManaged(pid)
	if !ok {
		t.Fatalf("运行中的进程应被列出: %+v", Running())
	}
	if m.Module != "frp" || m.Name != "家里" || m.Program != "sleep" || m.Since.IsZero() || m.Restarts != 0 {
		t.Fatalf("登记信息不符: %+v", m)
	}

	// 没有指定名称时用日志来源
	_, log2 := newLog()
	s2 := New(log2)
	s2.Start(Spec{Path: "sleep", Args: []string{"60"}})
	waitFor(t, func() bool { return s2.State().Running })
	if m2, ok := findManaged(s2.State().PID); !ok || m2.Name != "t" || m2.Module != "" {
		t.Fatalf("未指定名称时应使用日志来源: %+v %v", m2, ok)
	}
	s2.Stop()

	s.Stop()
	if _, ok := findManaged(pid); ok {
		t.Fatal("停止后不应再列出")
	}
	registry.mu.Lock()
	_, left := registry.sups[s]
	registry.mu.Unlock()
	if left {
		t.Fatal("停止后应注销托管器")
	}

	// 无法启动的不列出
	s3 := New(log)
	s3.Start(Spec{Path: "/nonexistent/gately-test-bin", Module: "frp"})
	waitFor(t, func() bool { return s3.State().Error != "" })
	for _, m := range Running() {
		if m.Program == "gately-test-bin" {
			t.Fatalf("无法启动的进程不应列出: %+v", m)
		}
	}
	s3.Stop()
}

// 等待重启期间没有进程，不列出；重新启动后列出新的 PID 与重启次数
func TestRunningDuringRestart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("依赖 sh 命令")
	}
	_, log := newLog()
	s := New(log)
	s.Start(Spec{Path: "sh", Args: []string{"-c", "exit 1"}, Module: "cloudflared", Name: "x"})
	defer s.Stop()
	waitFor(t, func() bool { return s.State().Restarts >= 1 && !s.State().Running })
	for _, m := range Running() {
		if m.Module == "cloudflared" && m.Name == "x" && !s.State().Running {
			t.Fatalf("等待重启时不应列出: %+v", m)
		}
	}
}
