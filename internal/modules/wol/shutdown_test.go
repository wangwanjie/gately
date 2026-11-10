package wol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"golang.org/x/crypto/ssh"
)

type testRouter struct{ mux *http.ServeMux }

func (r testRouter) HandleFunc(p string, h http.HandlerFunc) {
	method, path, _ := strings.Cut(p, " ")
	r.mux.HandleFunc(method+" /api/wol"+path, h)
}

// newTestModule 创建带接口的模块，exec 为模拟的 SSH 执行函数（可为 nil，表示未编译 Web 终端）。
func newTestModule(t *testing.T, exec func(ctx context.Context, conn, cmd string) (string, error)) (*Module, *httptest.Server, *logbuf.Buffer) {
	t.Helper()
	st, _ := config.Open(t.TempDir())
	deps := Deps{SSHExec: exec}
	if exec != nil {
		deps.SSHTargets = func() []SSHTarget { return []SSHTarget{{Key: "c1", Name: "pc", Host: "root@10.0.0.2", Trusted: true}} }
	}
	m := New(deps)
	logs := logbuf.New(200, log.New(io.Discard, "", 0))
	if err := m.Init(module.Env{Store: st, Logs: logs}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	m.Routes(testRouter{mux})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop() })
	return m, srv, logs
}

// call 发送 JSON 请求，返回状态码与响应体。
func call(t *testing.T, srv *httptest.Server, method, path string, body any) (int, string) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	}
	req, _ := http.NewRequest(method, srv.URL+"/api/wol"+path, rd)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func addDevice(t *testing.T, srv *httptest.Server, d map[string]any) string {
	t.Helper()
	code, body := call(t, srv, "POST", "/devices", d)
	if code != 200 {
		t.Fatalf("新增设备失败: %d %s", code, body)
	}
	var out struct {
		Key string `json:"key"`
	}
	_ = json.Unmarshal([]byte(body), &out)
	if out.Key == "" {
		t.Fatalf("没有返回 key: %s", body)
	}
	return out.Key
}

func TestShutdownValidate(t *testing.T) {
	base := func() Device { return Device{Name: "pc", MACs: []string{"aa:bb:cc:dd:ee:ff"}} }
	d := base()
	d.ShutdownConn, d.ShutdownCommand = " c1 ", " shutdown -h now "
	if err := d.Validate(); err != nil || d.ShutdownConn != "c1" || d.ShutdownCommand != "shutdown -h now" {
		t.Fatalf("规范化错误: %+v %v", d, err)
	}
	d = base()
	d.ShutdownConn = "c1"
	if err := d.Validate(); err == nil {
		t.Fatal("选择了连接却没填命令应报错")
	}
	d = base()
	d.ShutdownCommand = "poweroff"
	if err := d.Validate(); err != nil || d.ShutdownCommand != "" {
		t.Fatalf("未选连接时应清空命令: %+v %v", d, err)
	}
	for _, cmd := range []string{"shutdown\nrm -rf /", "a\rb", "a\x00b", strings.Repeat("x", 1025)} {
		d = base()
		d.ShutdownConn, d.ShutdownCommand = "c1", cmd
		if err := d.Validate(); err == nil {
			t.Fatalf("命令 %q 应被拒绝", cmd)
		}
	}
}

func TestShutdownAPI(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	result := func(ctx context.Context, conn, cmd string) (string, error) { return "bye", nil }
	exec := func(ctx context.Context, conn, cmd string) (string, error) {
		mu.Lock()
		calls = append(calls, conn+"|"+cmd)
		f := result
		mu.Unlock()
		return f(ctx, conn, cmd)
	}
	_, srv, logs := newTestModule(t, exec)
	key := addDevice(t, srv, map[string]any{"name": "pc", "macs": []string{"aa:bb:cc:dd:ee:ff"}, "shutdownConn": "c1", "shutdownCommand": "shutdown /s /t 0"})
	plain := addDevice(t, srv, map[string]any{"name": "nas", "macs": []string{"aa:bb:cc:dd:ee:01"}})

	code, body := call(t, srv, "POST", "/devices/"+key+"/shutdown", nil)
	if code != 200 || !strings.Contains(body, "bye") || len(calls) != 1 || calls[0] != "c1|shutdown /s /t 0" {
		t.Fatalf("关机失败: %d %s %v", code, body, calls)
	}
	// 关机命令使连接在返回退出码前断开：视为已开始关机
	mu.Lock()
	result = func(context.Context, string, string) (string, error) {
		return "", fmt.Errorf("远程命令执行失败: %w", &ssh.ExitMissingError{})
	}
	mu.Unlock()
	if code, body := call(t, srv, "POST", "/devices/"+key+"/shutdown", nil); code != 200 {
		t.Fatalf("断开应视为成功: %d %s", code, body)
	}
	mu.Lock()
	result = func(context.Context, string, string) (string, error) {
		return "denied", errors.New("远程命令执行失败: exit 1")
	}
	mu.Unlock()
	if code, body := call(t, srv, "POST", "/devices/"+key+"/shutdown", nil); code != http.StatusBadGateway || !strings.Contains(body, "exit 1") {
		t.Fatalf("命令失败应返回 502: %d %s", code, body)
	}
	if !strings.Contains(fmt.Sprint(logs.Since(0)), "denied") {
		t.Fatal("失败时日志应带上命令输出")
	}
	if code, _ := call(t, srv, "POST", "/devices/"+plain+"/shutdown", nil); code != http.StatusBadRequest {
		t.Fatalf("未配置关机应返回 400: %d", code)
	}
	if code, _ := call(t, srv, "POST", "/devices/nope/shutdown", nil); code != http.StatusNotFound {
		t.Fatalf("不存在的设备应返回 404: %d", code)
	}
	code, body = call(t, srv, "GET", "/ssh-targets", nil)
	if code != 200 || !strings.Contains(body, "root@10.0.0.2") {
		t.Fatalf("SSH 连接列表错误: %d %s", code, body)
	}
}

// 同一设备的关机命令同时只执行一次，正在执行时再次请求直接拒绝，不会堆积 SSH 连接。
func TestShutdownBusy(t *testing.T) {
	release := make(chan struct{})
	var running atomic.Int32
	exec := func(ctx context.Context, conn, cmd string) (string, error) {
		running.Add(1)
		<-release
		return "", nil
	}
	_, srv, _ := newTestModule(t, exec)
	key := addDevice(t, srv, map[string]any{"name": "pc", "macs": []string{"aa:bb:cc:dd:ee:ff"}, "shutdownConn": "c1", "shutdownCommand": "poweroff"})
	done := make(chan int)
	go func() {
		code, _ := call(t, srv, "POST", "/devices/"+key+"/shutdown", nil)
		done <- code
	}()
	for running.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if code, body := call(t, srv, "POST", "/devices/"+key+"/shutdown", nil); code != http.StatusBadRequest || !strings.Contains(body, "正在执行") {
		t.Fatalf("执行中再次关机应拒绝: %d %s", code, body)
	}
	close(release)
	if code := <-done; code != 200 {
		t.Fatalf("第一次关机应成功: %d", code)
	}
	if running.Load() != 1 {
		t.Fatalf("只应执行一次: %d", running.Load())
	}
}

// 关机命令超时后返回，不会一直占着设备。
func TestShutdownTimeout(t *testing.T) {
	old := shutdownTimeout
	shutdownTimeout = 100 * time.Millisecond
	defer func() { shutdownTimeout = old }()
	exec := func(ctx context.Context, conn, cmd string) (string, error) {
		<-ctx.Done()
		return "", fmt.Errorf("远程命令执行超时: %w", ctx.Err())
	}
	_, srv, _ := newTestModule(t, exec)
	key := addDevice(t, srv, map[string]any{"name": "pc", "macs": []string{"aa:bb:cc:dd:ee:ff"}, "shutdownConn": "c1", "shutdownCommand": "poweroff"})
	if code, body := call(t, srv, "POST", "/devices/"+key+"/shutdown", nil); code != http.StatusBadGateway || !strings.Contains(body, "超时") {
		t.Fatalf("应超时: %d %s", code, body)
	}
}

// 未编译 Web 终端时关机给出明确提示，SSH 连接列表为空。
func TestShutdownWithoutTerminal(t *testing.T) {
	_, srv, _ := newTestModule(t, nil)
	key := addDevice(t, srv, map[string]any{"name": "pc", "macs": []string{"aa:bb:cc:dd:ee:ff"}, "shutdownConn": "c1", "shutdownCommand": "poweroff"})
	if code, body := call(t, srv, "POST", "/devices/"+key+"/shutdown", nil); code != http.StatusBadGateway || !strings.Contains(body, "Web 终端") {
		t.Fatalf("应提示缺少 Web 终端: %d %s", code, body)
	}
	if code, body := call(t, srv, "GET", "/ssh-targets", nil); code != 200 || strings.TrimSpace(body) != "[]" {
		t.Fatalf("应返回空列表: %d %s", code, body)
	}
}
