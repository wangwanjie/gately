package ftp

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	ftpserver "github.com/fclairamb/ftpserverlib"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/fileusers"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/mountfs"
)

func TestSettingsValidate(t *testing.T) {
	s := Settings{Enable: true}
	if err := s.Validate(); err == nil {
		t.Fatal("启用但无用户应报错")
	}
	s = Settings{Users: []User{{Name: "a", Password: "p", Root: "relative"}}}
	if err := s.Validate(); err == nil {
		t.Fatal("相对路径应报错")
	}
	s = Settings{PassiveStart: 3000, PassiveEnd: 2000}
	if err := s.Validate(); err == nil {
		t.Fatal("被动端口范围反向应报错")
	}
	s = Settings{}
	if err := s.Validate(); err != nil || s.Port != 2121 || s.IdleTimeout != 900 {
		t.Fatalf("默认值不正确: %v %+v", err, s)
	}
}

func TestFTPLogin(t *testing.T) {
	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	st, _ := config.Open(t.TempDir())
	m := New(nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(20, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	m.settings = Settings{Enable: true, ListenIP: "127.0.0.1", Port: port, Users: []User{
		{Name: "rw", Password: "p1", Root: t.TempDir()},
	}}
	if err := m.settings.Validate(); err != nil {
		t.Fatal(err)
	}
	m.restartLocked()
	if m.err != "" {
		t.Fatal(m.err)
	}
	defer m.Stop()

	login := func(user, pass string) int {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 3*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer waitClientsGone(t, m)
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		tp := textproto.NewConn(conn)
		if _, _, err := tp.ReadResponse(220); err != nil {
			t.Fatal(err)
		}
		_ = tp.PrintfLine("USER %s", user)
		if _, _, err := tp.ReadResponse(331); err != nil {
			t.Fatal(err)
		}
		_ = tp.PrintfLine("PASS %s", pass)
		code, _, _ := tp.ReadResponse(0)
		return code
	}
	if c := login("rw", "p1"); c != 230 {
		t.Fatalf("正确密码应返回 230，实际 %d", c)
	}
	if c := login("rw", "bad"); c != 530 {
		t.Fatalf("错误密码应返回 530，实际 %d", c)
	}
}

// 同一来源连续猜错密码达到上限后，即使随后给出正确密码也被拒绝，防止暴力猜测。
func TestFTPLoginThrottle(t *testing.T) {
	d := &driver{s: Settings{}, users: map[string]User{"rw": {Name: "rw", Password: "p1", Root: t.TempDir()}}, logs: logbuf.New(20, log.New(io.Discard, "", 0))}
	cc := &fakeClient{}
	for i := 0; i < fileusers.ThrottleMaxFailures; i++ {
		if _, err := d.AuthUser(cc, "rw", "bad"); err == nil {
			t.Fatal("错误密码应失败")
		}
	}
	if _, err := d.AuthUser(cc, "rw", "p1"); err == nil {
		t.Fatal("失败次数达到上限后，正确密码也应被拒绝")
	}
	other := &fakeClient{addr: &net.TCPAddr{IP: net.ParseIP("203.0.113.77"), Port: 4000}}
	if _, err := d.AuthUser(other, "rw", "p1"); err != nil {
		t.Fatalf("其他来源不应受影响: %v", err)
	}
}

// fakeClient 只实现 AuthUser 用到的 RemoteAddr。
type fakeClient struct {
	ftpserver.ClientContext
	addr net.Addr
}

func (f *fakeClient) RemoteAddr() net.Addr {
	if f.addr != nil {
		return f.addr
	}
	return &net.TCPAddr{IP: net.ParseIP("198.51.100.5"), Port: 3000}
}

// waitClientsGone 等服务端处理完已断开的会话：ftpserverlib 在认证失败时自行断开连接，
// 与 Stop/重启时 closeClients 再次关闭同一会话会在库内产生数据竞争（-race 下测试失败）。
func waitClientsGone(t *testing.T, m *Module) {
	t.Helper()
	for i := 0; i < 200; i++ {
		m.mu.Lock()
		d := m.drv
		m.mu.Unlock()
		if d == nil {
			return
		}
		d.clientsMu.Lock()
		n := len(d.clients)
		d.clientsMu.Unlock()
		if n == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("等待 FTP 会话结束超时")
}

// TestFTPMounts 验证多目录用户：根目录列出挂载名称，只读挂载拒绝上传。
func TestFTPMounts(t *testing.T) {
	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	rw, ro := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(ro, "readme.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, _ := config.Open(t.TempDir())
	m := New(nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(20, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	m.settings = Settings{Enable: true, ListenIP: "127.0.0.1", Port: port, Users: []User{
		{Name: "u", Password: "p", Root: "/ignored", Mounts: []mountfs.Mount{{Name: "data", Path: rw}, {Name: "docs", Path: ro, ReadOnly: true}}},
	}}
	if err := m.settings.Validate(); err != nil {
		t.Fatal(err)
	}
	if m.settings.Users[0].Root != "" {
		t.Fatal("多目录用户的单目录字段应被清空")
	}
	m.restartLocked()
	if m.err != "" {
		t.Fatal(m.err)
	}
	defer m.Stop()

	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	tp := textproto.NewConn(conn)
	cmd := func(expect int, format string, args ...any) string {
		t.Helper()
		_ = tp.PrintfLine(format, args...)
		_, msg, err := tp.ReadResponse(expect)
		if err != nil {
			t.Fatalf("%s: %v", fmt.Sprintf(format, args...), err)
		}
		return msg
	}
	if _, _, err := tp.ReadResponse(220); err != nil {
		t.Fatal(err)
	}
	cmd(331, "USER u")
	cmd(230, "PASS p")
	// transfer 通过 EPSV 建立数据连接执行 c，返回读取到的数据
	transfer := func(c string, upload []byte) (string, int) {
		t.Helper()
		msg := cmd(229, "EPSV")
		var dataPort int
		if _, err := fmt.Sscanf(msg[strings.Index(msg, "(|||"):], "(|||%d|)", &dataPort); err != nil {
			t.Fatalf("解析 EPSV 响应失败: %q", msg)
		}
		dc, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(dataPort), 3*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		_ = tp.PrintfLine("%s", c)
		code, _, _ := tp.ReadResponse(0)
		if code != 150 {
			dc.Close()
			return "", code
		}
		var out []byte
		if upload != nil {
			_, _ = dc.Write(upload)
		} else {
			out, _ = io.ReadAll(dc)
		}
		dc.Close()
		code, _, _ = tp.ReadResponse(0)
		return string(out), code
	}
	list, code := transfer("NLST /", nil)
	if code != 226 || !strings.Contains(list, "data") || !strings.Contains(list, "docs") {
		t.Fatalf("根目录应列出挂载名称，code=%d list=%q", code, list)
	}
	if body, code := transfer("RETR /docs/readme.txt", nil); code != 226 || body != "hello" {
		t.Fatalf("读取只读挂载失败: %d %q", code, body)
	}
	if _, code := transfer("STOR /data/up.txt", []byte("x")); code != 226 {
		t.Fatalf("上传到可写挂载失败: %d", code)
	}
	if _, err := os.Stat(filepath.Join(rw, "up.txt")); err != nil {
		t.Fatal("文件应写入可写挂载目录")
	}
	if _, code := transfer("STOR /docs/up.txt", []byte("x")); code < 400 {
		t.Fatalf("只读挂载应拒绝上传，实际 %d", code)
	}
}

// TestSettingsSecrets 验证查看本地用户密码明文的接口按用户名返回原始密码，而设置接口仍返回占位符。
func TestSettingsSecrets(t *testing.T) {
	st, _ := config.Open(t.TempDir())
	m := New(nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(20, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.settings = Settings{Users: []User{{Name: "alice", Password: "p1", Root: "/a"}, {Name: "bob", Password: "p2", Root: "/b"}}}
	m.mu.Unlock()
	mux := http.NewServeMux()
	m.Routes(secretsRouter{mux})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/settings/secrets", nil))
	var out map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || len(out) != 2 || out["alice"] != "p1" || out["bob"] != "p2" {
		t.Fatalf("应按用户名返回密码明文: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/settings", nil))
	if strings.Contains(rec.Body.String(), "p1") {
		t.Fatalf("设置接口不应返回密码明文: %s", rec.Body.String())
	}
}

// secretsRouter 把模块接口直接注册到 ServeMux，用于测试。
type secretsRouter struct{ *http.ServeMux }

func (r secretsRouter) HandleFunc(pattern string, h http.HandlerFunc) {
	r.ServeMux.HandleFunc(pattern, h)
}
