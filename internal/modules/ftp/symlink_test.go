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
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/mountfs"
)

// ftpSession 登录 FTP 服务，返回发送命令与经 EPSV 传输的函数。
func ftpSession(t *testing.T, port int, user, pass string) (cmd func(format string, args ...any) (int, string), transfer func(c string, upload []byte) (string, int)) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	tp := textproto.NewConn(conn)
	if _, _, err := tp.ReadResponse(220); err != nil {
		t.Fatal(err)
	}
	cmd = func(format string, args ...any) (int, string) {
		t.Helper()
		_ = tp.PrintfLine(format, args...)
		code, msg, _ := tp.ReadResponse(0)
		return code, msg
	}
	if code, _ := cmd("USER %s", user); code != 331 {
		t.Fatalf("USER: %d", code)
	}
	if code, _ := cmd("PASS %s", pass); code != 230 {
		t.Fatalf("PASS: %d", code)
	}
	transfer = func(c string, upload []byte) (string, int) {
		t.Helper()
		code, msg := cmd("EPSV")
		if code != 229 {
			t.Fatalf("EPSV: %d %s", code, msg)
		}
		var dataPort int
		if _, err := fmt.Sscanf(msg[strings.Index(msg, "(|||"):], "(|||%d|)", &dataPort); err != nil {
			t.Fatalf("解析 EPSV 响应失败: %q", msg)
		}
		dc, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(dataPort), 3*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		_ = tp.PrintfLine("%s", c)
		code, _, _ = tp.ReadResponse(0)
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
	return cmd, transfer
}

// TestSymlinkEscape：共享目录内指向目录外的符号链接，经 FTP 读、写、列目录、改名都不能越出，
// SITE CHOWN 被拒绝；指向共享目录内的链接照常可用。单目录与多目录两种用户都检查。
func TestSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	root, out := filepath.Join(base, "root"), filepath.Join(base, "out")
	_ = os.MkdirAll(root, 0o755)
	_ = os.MkdirAll(out, 0o755)
	_ = os.WriteFile(filepath.Join(out, "secret.txt"), []byte("top-secret"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "inside.txt"), []byte("inside"), 0o644)
	if err := os.Symlink(filepath.Join(out, "secret.txt"), filepath.Join(root, "link-file")); err != nil {
		t.Skip(err)
	}
	_ = os.Symlink(out, filepath.Join(root, "link-dir"))
	_ = os.Symlink("inside.txt", filepath.Join(root, "link-in"))

	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	st, _ := config.Open(t.TempDir())
	m := New(nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(20, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	m.settings = Settings{Enable: true, ListenIP: "127.0.0.1", Port: port, PassiveStart: pasvStart, PassiveEnd: pasvStart + pasvSpan, Users: []User{
		{Name: "single", Password: "p", Root: root},
		{Name: "multi", Password: "p", Mounts: []mountfs.Mount{{Name: "m", Path: root}}},
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

	for _, c := range []struct{ user, prefix string }{{"single", ""}, {"multi", "/m"}} {
		cmd, transfer := ftpSession(t, port, c.user, "p")
		p := func(s string) string { return c.prefix + s }
		if body, code := transfer("RETR "+p("/link-file"), nil); code < 400 || strings.Contains(body, "top-secret") {
			t.Fatalf("%s: 不应能读取目录外的文件: %d %q", c.user, code, body)
		}
		if body, code := transfer("RETR "+p("/link-dir/secret.txt"), nil); code < 400 || strings.Contains(body, "top-secret") {
			t.Fatalf("%s: 不应能经目录链接读取目录外的文件: %d", c.user, code)
		}
		if _, code := transfer("STOR "+p("/link-dir/new.txt"), []byte("x")); code < 400 {
			t.Fatalf("%s: 不应能在目录外建文件: %d", c.user, code)
		}
		if _, code := transfer("STOR "+p("/link-file"), []byte("overwrite")); code < 400 {
			t.Fatalf("%s: 不应能覆盖目录外的文件: %d", c.user, code)
		}
		if list, code := transfer("NLST "+p("/link-dir"), nil); code < 400 && strings.Contains(list, "secret.txt") {
			t.Fatalf("%s: 不应能列出目录外的目录: %q", c.user, list)
		}
		if code, _ := cmd("MKD %s", p("/link-dir/newdir")); code < 400 {
			t.Fatalf("%s: 不应能在目录外建目录: %d", c.user, code)
		}
		if code, _ := cmd("RNFR %s", p("/link-dir/secret.txt")); code < 400 {
			if code2, _ := cmd("RNTO %s", p("/stolen.txt")); code2 < 400 {
				t.Fatalf("%s: 不应能把目录外的文件移入", c.user)
			}
		}
		if code, _ := cmd("RNFR %s", p("/inside.txt")); code < 400 {
			if code2, _ := cmd("RNTO %s", p("/link-dir/moved.txt")); code2 < 400 {
				t.Fatalf("%s: 不应能把文件移到目录外", c.user)
			}
		}
		if code, _ := cmd("DELE %s", p("/link-dir/secret.txt")); code < 400 {
			t.Fatalf("%s: 不应能删除目录外的文件", c.user)
		}
		if code, msg := cmd("SITE CHOWN 0:0 %s", p("/inside.txt")); code < 400 {
			t.Fatalf("%s: SITE CHOWN 应被拒绝: %d %s", c.user, code, msg)
		}
		if body, code := transfer("RETR "+p("/link-in"), nil); code != 226 || body != "inside" {
			t.Fatalf("%s: 目录内的链接应可读: %d %q", c.user, code, body)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(out, "secret.txt")); string(b) != "top-secret" {
		t.Fatalf("目录外的文件被改动: %q", b)
	}
	if entries, _ := os.ReadDir(out); len(entries) != 1 {
		t.Fatalf("目录外出现了新内容: %v", entries)
	}
}
