package ftp

import (
	"io"
	"log"
	"net"
	"net/textproto"
	"strconv"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/fileusers"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// TestFTPDisabledUsers 验证停用的本地用户与共享用户即使密码正确也无法登录，停用的本地用户不回落到同名共享用户；
// 停用一个已登录的共享用户时服务重启，其会话被断开，之后无法再登录。
func TestFTPDisabledUsers(t *testing.T) {
	dir := t.TempDir()
	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	st, _ := config.Open(t.TempDir())
	m := New(nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	if err := fileusers.Default.Save([]fileusers.User{
		{Name: "sam", Password: "ps", Root: dir},
		{Name: "dup", Password: "shared", Root: dir},
	}); err != nil {
		t.Fatal(err)
	}
	m.settings = Settings{Enable: true, ListenIP: "127.0.0.1", Port: port, PassiveStart: pasvStart, PassiveEnd: pasvStart + pasvSpan, SharedUsers: true,
		Users: []User{
			{Name: "on", Password: "p1", Root: dir},
			{Name: "off", Password: "p2", Root: dir, Disabled: true},
			{Name: "dup", Password: "local", Root: dir, Disabled: true},
		}}
	if err := m.settings.Validate(); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.restartLocked()
	m.mu.Unlock()
	if m.err != "" {
		t.Fatal(m.err)
	}
	defer m.Stop()

	// dial 登录并返回 PASS 的响应码与连接，调用方负责关闭连接
	dial := func(user, pass string) (int, *textproto.Conn) {
		t.Helper()
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 3*time.Second)
		if err != nil {
			t.Fatal(err)
		}
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
		return code, tp
	}
	login := func(user, pass string) int {
		t.Helper()
		code, tp := dial(user, pass)
		tp.Close()
		waitClientsGone(t, m)
		return code
	}
	if c := login("on", "p1"); c != 230 {
		t.Fatalf("启用的用户应能登录: %d", c)
	}
	if c := login("off", "p2"); c != 530 {
		t.Fatalf("停用的本地用户应被拒绝: %d", c)
	}
	if c := login("dup", "shared"); c != 530 {
		t.Fatalf("停用本地用户的同名共享用户应被拒绝: %d", c)
	}
	if c := login("dup", "local"); c != 530 {
		t.Fatalf("停用的本地用户应被拒绝: %d", c)
	}

	// sam 已登录，随后被停用：服务重启断开其会话，再登录被拒绝
	code, tp := dial("sam", "ps")
	if code != 230 {
		t.Fatalf("启用的共享用户应能登录: %d", code)
	}
	defer tp.Close()
	if err := fileusers.Default.Save([]fileusers.User{{Name: "sam", Password: "ps", Root: dir, Disabled: true}}); err != nil {
		t.Fatal(err)
	}
	_ = tp.PrintfLine("NOOP")
	if c, _, err := tp.ReadResponse(200); err == nil {
		t.Fatalf("停用后已登录的会话应被断开: %d", c)
	}
	if c := login("sam", "ps"); c != 530 {
		t.Fatalf("停用的共享用户应被拒绝: %d", c)
	}
}
