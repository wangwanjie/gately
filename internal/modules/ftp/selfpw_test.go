package ftp

import (
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/fileusers"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// ftpLogin 登录并返回控制连接与 PASS 的响应码。
func ftpLogin(t *testing.T, port int, user, pass string) (*textproto.Conn, int) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	tp := textproto.NewConn(conn)
	if _, _, err := tp.ReadResponse(220); err != nil {
		t.Fatal(err)
	}
	_ = tp.PrintfLine("USER %s", user)
	_, _, _ = tp.ReadResponse(331)
	_ = tp.PrintfLine("PASS %s", pass)
	code, _, _ := tp.ReadResponse(0)
	return tp, code
}

// alive 用 NOOP 检查控制连接是否仍可用。
func alive(tp *textproto.Conn) bool {
	if err := tp.PrintfLine("NOOP"); err != nil {
		return false
	}
	code, _, err := tp.ReadResponse(0)
	return err == nil && code == 200
}

// TestSelfServicePasswordChangeKeepsService：共享用户自助修改密码后 FTP 服务不重启，其他用户的连接与认证失败计数都保留，
// 只有改了密码的用户已登录的连接被断开；新密码立即生效，旧密码失效。
func TestSelfServicePasswordChangeKeepsService(t *testing.T) {
	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	st, _ := config.Open(t.TempDir())
	m := New(nil)
	logs := logbuf.New(50, log.New(io.Discard, "", 0))
	if err := m.Init(module.Env{Store: st, Logs: logs}); err != nil {
		t.Fatal(err)
	}
	bobDir, carolDir := t.TempDir(), t.TempDir()
	req := httptest.NewRequest("PUT", "/fileusers", strings.NewReader(`{"selfService":true,"users":[{"name":"bob","password":"bob-pass-1","root":"`+bobDir+`"},{"name":"carol","password":"carol-pass-1","root":"`+carolDir+`"}]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	fileusers.Default.HandlePut(nil)(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("保存共享用户失败: %d %s", w.Code, w.Body.String())
	}
	m.settings = Settings{Enable: true, ListenIP: "127.0.0.1", Port: port, PassiveStart: pasvStart, PassiveEnd: pasvStart + pasvSpan, SharedUsers: true,
		Users: []User{{Name: "local", Password: "lp", Root: t.TempDir()}}}
	if err := m.settings.Validate(); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.restartLocked()
	srvBefore := m.srv
	m.mu.Unlock()
	if m.err != "" {
		t.Fatal(m.err)
	}
	defer m.Stop()

	bob, code := ftpLogin(t, port, "bob", "bob-pass-1")
	if code != 230 {
		t.Fatalf("bob 登录失败: %d", code)
	}
	carol, code := ftpLogin(t, port, "carol", "carol-pass-1")
	if code != 230 {
		t.Fatalf("carol 登录失败: %d", code)
	}
	for i := 0; i < fileusers.ThrottleMaxFailures-1; i++ {
		if _, code := ftpLogin(t, port, "local", "wrong"); code == 230 {
			t.Fatal("错误密码应登录失败")
		}
	}

	if err := fileusers.Default.ChangePassword("carol", "carol-pass-1", "carol-pass-2"); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	srvAfter := m.srv
	m.mu.Unlock()
	if srvAfter != srvBefore {
		t.Fatal("自助修改密码不应重启 FTP 服务")
	}
	if !alive(bob) {
		t.Fatal("其他用户的连接不应被断开")
	}
	if alive(carol) {
		t.Fatal("改了密码的用户已登录的连接应被断开")
	}
	if _, code := ftpLogin(t, port, "carol", "carol-pass-2"); code != 230 {
		t.Fatalf("新密码应立即生效: %d", code)
	}
	if _, code := ftpLogin(t, port, "carol", "carol-pass-1"); code == 230 {
		t.Fatal("旧密码应失效")
	}
	if _, code := ftpLogin(t, port, "local", "lp"); code == 230 {
		t.Fatal("修改密码不应清空来源的认证失败计数")
	}
	// 恢复共享用户设置，避免影响同一进程中的其他用例
	req = httptest.NewRequest("PUT", "/fileusers", strings.NewReader(`{"selfService":false,"users":[]}`))
	req.Header.Set("Content-Type", "application/json")
	m.settings.SharedUsers = false
	fileusers.Default.HandlePut(nil)(httptest.NewRecorder(), req)
}
