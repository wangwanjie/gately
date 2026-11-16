package main

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestMain 让测试程序可以充当 gately 主程序：设置了 GATELY_TEST_SERVE 时直接运行 main（参数取自 GATELY_TEST_ARGS），
// 供下面的测试以子进程方式启动真实的服务进程。
func TestMain(m *testing.M) {
	if os.Getenv("GATELY_TEST_SERVE") == "1" {
		os.Args = append([]string{os.Args[0]}, strings.Fields(os.Getenv("GATELY_TEST_ARGS"))...)
		main()
		return
	}
	os.Exit(m.Run())
}

// safeBuffer 是可并发读写的缓冲，子进程的标准错误输出边写边读。
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// startServe 以子进程方式启动服务，返回进程与它的标准错误输出。
func startServe(t *testing.T, dir, listen string) (*exec.Cmd, *safeBuffer) {
	t.Helper()
	// 关闭自动检查更新（测试不访问外网），日志级别设为 debug 让回显的内容更多
	if err := os.WriteFile(filepath.Join(dir, "gately.json"), []byte(`{"disableUpdateCheck":true,"logLevel":"debug"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "GATELY_TEST_SERVE=1", "GATELY_TEST_ARGS=-c "+dir+" -listen "+listen,
		"GATELY_INITIAL_PASSWORD=", "GATELY_LANG=zh-CN")
	stderr := &safeBuffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	return cmd, stderr
}

// checkEcho 确认日志文件里的每一条日志都完整地出现在标准错误输出里：回显是异步写出的，
// 进程退出前必须全部写完。返回日志文件里的日志条数。
func checkEcho(t *testing.T, dir, stderr string) int {
	t.Helper()
	if !strings.HasSuffix(stderr, "\n") {
		t.Fatalf("标准错误输出的最后一行不完整: %q", stderr[max(0, len(stderr)-200):])
	}
	lines := map[string]bool{}
	for _, ln := range strings.Split(stderr, "\n") {
		lines[ln] = true
	}
	files, _ := filepath.Glob(filepath.Join(dir, "logs", "*.log"))
	n := 0
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range bytes.Split(bytes.TrimSpace(raw), []byte("\n")) {
			if len(row) == 0 {
				continue
			}
			var e struct {
				Time    time.Time `json:"time"`
				Level   string    `json:"level"`
				Source  string    `json:"source"`
				Message string    `json:"message"`
			}
			if err := json.Unmarshal(row, &e); err != nil {
				t.Fatalf("%s 里有无法解析的行: %s", f, row)
			}
			want := e.Time.Local().Format("2006/01/02 15:04:05 ") + "[" + e.Level + "] "
			if e.Source != "" {
				want += "[" + e.Source + "] "
			}
			// 多行内容只核对第一行
			want += strings.SplitN(e.Message, "\n", 2)[0]
			if !lines[want] {
				t.Errorf("%s 里的日志没有出现在标准错误输出中: %q", filepath.Base(f), want)
			}
			n++
		}
	}
	return n
}

// 服务进程收到 SIGTERM 正常退出时，异步回显的日志在退出前全部写到标准错误输出：
// 包括停止各模块过程中的日志，与落盘的日志文件逐条对应。
func TestServeFlushesEchoOnSigterm(t *testing.T) {
	if runtime.GOOS == "windows" || testing.Short() {
		t.Skip("需要启动子进程并发送 SIGTERM")
	}
	dir := t.TempDir()
	cmd, stderr := startServe(t, dir, "127.0.0.1:0")
	deadline := time.Now().Add(30 * time.Second)
	for !strings.Contains(stderr.String(), "管理界面监听") {
		if time.Now().After(deadline) {
			t.Fatalf("服务没有启动:\n%s", stderr.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("收到 SIGTERM 后应正常退出: %v\n%s", err, stderr.String())
	}
	out := stderr.String()
	if n := checkEcho(t, dir, out); n < 2 {
		t.Fatalf("日志文件里只有 %d 条日志，测试没有覆盖到回显", n)
	}
	// 首次启动创建管理员账号的提示经标准库 log 输出，走同一个出口
	if !strings.Contains(out, "已创建管理员账号 admin") {
		t.Errorf("标准日志的输出缺失:\n%s", out)
	}
}

// 启动失败（管理端口被占用）时 log.Fatalf 的错误信息同步写出后才退出，此前异步回显的日志也都在它前面写完。
func TestServeFlushesEchoOnFatal(t *testing.T) {
	if testing.Short() {
		t.Skip("需要启动子进程")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	dir := t.TempDir()
	cmd, stderr := startServe(t, dir, ln.Addr().String())
	err = cmd.Wait()
	var exit *exec.ExitError
	if ee, ok := err.(*exec.ExitError); ok {
		exit = ee
	}
	if exit == nil || exit.ExitCode() != 1 {
		t.Fatalf("端口被占用时应以状态码 1 退出，得到 %v\n%s", err, stderr.String())
	}
	out := stderr.String()
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if last := lines[len(lines)-1]; !strings.Contains(last, "运行失败") || !strings.Contains(last, "address already in use") && !strings.Contains(last, "bind") {
		t.Fatalf("最后一行应为启动失败的原因: %q", last)
	}
	checkEcho(t, dir, out)
}
