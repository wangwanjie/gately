package ftp

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/fileusers"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// TestFTPSharedUsers 验证共享用户登录后看到自己的目录，本地同名用户优先，修改共享用户后立即按新设置校验。
func TestFTPSharedUsers(t *testing.T) {
	alice := t.TempDir()
	_ = os.WriteFile(filepath.Join(alice, "alice.txt"), []byte("a"), 0o644)
	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	st, _ := config.Open(t.TempDir())
	m := New(nil)
	logs := logbuf.New(50, log.New(io.Discard, "", 0))
	if err := m.Init(module.Env{Store: st, Logs: logs}); err != nil {
		t.Fatal(err)
	}
	if err := fileusers.Default.Save([]fileusers.User{
		{Name: "alice", Password: "pa", Root: alice},
		{Name: "rw", Password: "shared", Root: alice},
	}); err != nil {
		t.Fatal(err)
	}
	m.settings = Settings{Enable: true, ListenIP: "127.0.0.1", Port: port, PassiveStart: pasvStart, PassiveEnd: pasvStart + pasvSpan, SharedUsers: true,
		Users: []User{{Name: "rw", Password: "p1", Root: t.TempDir()}}}
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

	// login 返回 PASS 的响应码，登录成功时用 NLST 列出根目录
	login := func(user, pass string) (int, string) {
		t.Helper()
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
		if code != 230 {
			return code, ""
		}
		_ = tp.PrintfLine("EPSV")
		_, msg, err := tp.ReadResponse(229)
		if err != nil {
			t.Fatal(err)
		}
		var dataPort int
		if _, err := fmt.Sscanf(msg[strings.Index(msg, "(|||"):], "(|||%d|)", &dataPort); err != nil {
			t.Fatalf("解析 EPSV 响应失败: %q", msg)
		}
		dc, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(dataPort), 3*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer dc.Close()
		_ = tp.PrintfLine("NLST /")
		if _, _, err := tp.ReadResponse(150); err != nil {
			t.Fatal(err)
		}
		out, _ := io.ReadAll(dc)
		return code, string(out)
	}
	if c, list := login("alice", "pa"); c != 230 || !strings.Contains(list, "alice.txt") {
		t.Fatalf("alice 应登录到自己的目录: %d %q", c, list)
	}
	if c, _ := login("alice", "bad"); c != 530 {
		t.Fatalf("错误密码应 530: %d", c)
	}
	if c, _ := login("rw", "shared"); c != 530 {
		t.Fatalf("与本地用户同名时共享用户密码不应通过: %d", c)
	}
	if c, _ := login("rw", "p1"); c != 230 {
		t.Fatalf("本地用户应正常: %d", c)
	}
	// 修改共享用户后服务重启，新密码立即生效
	if err := fileusers.Default.Save([]fileusers.User{{Name: "alice", Password: "pa2", Root: alice}}); err != nil {
		t.Fatal(err)
	}
	if c, _ := login("alice", "pa"); c != 530 {
		t.Fatalf("旧密码应失效: %d", c)
	}
	if c, _ := login("alice", "pa2"); c != 230 {
		t.Fatalf("新密码应可登录: %d", c)
	}
	// 关闭共享用户后无法登录
	m.mu.Lock()
	m.settings.SharedUsers = false
	m.restartLocked()
	m.mu.Unlock()
	if c, _ := login("alice", "pa2"); c != 530 {
		t.Fatalf("关闭开关后应拒绝: %d", c)
	}
}

// TestFTPSharedUsersDisconnect 验证修改共享用户后已登录的会话被断开，不能继续使用旧的目录与权限；
// 只靠共享用户启用时不允许删光共享用户。
func TestFTPSharedUsersDisconnect(t *testing.T) {
	root := t.TempDir()
	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	st, _ := config.Open(t.TempDir())
	m := New(nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	if err := fileusers.Default.Save([]fileusers.User{{Name: "alice", Password: "pa", Root: root}}); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.settings = Settings{Enable: true, ListenIP: "127.0.0.1", Port: port, PassiveStart: pasvStart, PassiveEnd: pasvStart + pasvSpan, SharedUsers: true}
	if err := m.settings.Validate(); err != nil {
		m.mu.Unlock()
		t.Fatal(err)
	}
	m.restartLocked()
	m.mu.Unlock()
	if m.err != "" {
		t.Fatal(m.err)
	}
	defer m.Stop()

	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	tp := textproto.NewConn(conn)
	if _, _, err := tp.ReadResponse(220); err != nil {
		t.Fatal(err)
	}
	_ = tp.PrintfLine("USER alice")
	if _, _, err := tp.ReadResponse(331); err != nil {
		t.Fatal(err)
	}
	_ = tp.PrintfLine("PASS pa")
	if _, _, err := tp.ReadResponse(230); err != nil {
		t.Fatal(err)
	}

	if err := fileusers.Default.Save(nil); err == nil {
		t.Fatal("只靠共享用户启用 FTP 时不应允许删光共享用户")
	}
	// 改为只读后，已登录的会话应被断开，需要重新登录才按新设置访问
	if err := fileusers.Default.Save([]fileusers.User{{Name: "alice", Password: "pa", Root: root, ReadOnly: true}}); err != nil {
		t.Fatal(err)
	}
	_ = tp.PrintfLine("NOOP")
	if code, msg, err := tp.ReadResponse(200); err == nil {
		t.Fatalf("修改共享用户后旧会话应被断开，仍收到 %d %s", code, msg)
	}
}

// TestFTPListenLogCountsSharedUsers 验证监听日志的用户数计入共享用户（issue #26），与本地用户同名的共享用户不重复计数，
// 修改共享用户重启后按新列表计数，关闭共享用户后只计本地用户。
func TestFTPListenLogCountsSharedUsers(t *testing.T) {
	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	st, _ := config.Open(t.TempDir())
	m := New(nil)
	logs := logbuf.New(50, log.New(io.Discard, "", 0))
	if err := m.Init(module.Env{Store: st, Logs: logs}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := fileusers.Default.Save([]fileusers.User{
		{Name: "alice", Password: "pa", Root: root},
		{Name: "bob", Password: "pb", Root: root},
		{Name: "rw", Password: "shared", Root: root},
	}); err != nil {
		t.Fatal(err)
	}
	m.settings = Settings{Enable: true, ListenIP: "127.0.0.1", Port: port, PassiveStart: pasvStart, PassiveEnd: pasvStart + pasvSpan, SharedUsers: true,
		Users: []User{{Name: "rw", Password: "p1", Root: root}}}
	m.mu.Lock()
	m.restartLocked()
	m.mu.Unlock()
	if m.err != "" {
		t.Fatal(m.err)
	}
	defer m.Stop()
	addr := "127.0.0.1:" + strconv.Itoa(port)
	// lastListen 返回最近一条监听日志
	lastListen := func() string {
		t.Helper()
		all := logs.Since(0)
		for i := len(all) - 1; i >= 0; i-- {
			if strings.HasPrefix(all[i].Message, "FTP 监听") {
				return all[i].Message
			}
		}
		t.Fatal("没有监听日志")
		return ""
	}
	if got, want := lastListen(), "FTP 监听 "+addr+"（3 个用户，其中 2 个共享用户）"; got != want {
		t.Fatalf("监听日志 %q，期望 %q", got, want)
	}
	// 修改共享用户后服务重启，按新列表计数
	if err := fileusers.Default.Save([]fileusers.User{{Name: "alice", Password: "pa", Root: root}}); err != nil {
		t.Fatal(err)
	}
	if got, want := lastListen(), "FTP 监听 "+addr+"（2 个用户，其中 1 个共享用户）"; got != want {
		t.Fatalf("修改共享用户后监听日志 %q，期望 %q", got, want)
	}
	m.mu.Lock()
	m.settings.SharedUsers = false
	m.restartLocked()
	m.mu.Unlock()
	if got, want := lastListen(), "FTP 监听 "+addr+"（1 个用户）"; got != want {
		t.Fatalf("关闭共享用户后监听日志 %q，期望 %q", got, want)
	}
}
