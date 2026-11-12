//go:build linux || darwin

package smb

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	smb2 "github.com/cloudsoda/go-smb2"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/fileusers"
	"github.com/wangwanjie/gately/internal/gosamba/parent"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/mountfs"
)

// startTestModule 在随机端口启动 SMB 模块，返回模块与端口。
func startTestModule(t *testing.T, s Settings) (*Module, int) {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	st, _ := config.Open(t.TempDir())
	m := New()
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(200, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	s.Enable, s.ListenIP, s.Port = true, "127.0.0.1", port
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	m.settings = s
	m.mu.Lock()
	m.restartLocked()
	m.mu.Unlock()
	if m.err != "" {
		t.Fatal(m.err)
	}
	t.Cleanup(func() { _ = m.Stop() })
	return m, port
}

// dial 用 go-smb2 客户端登录。
func dial(t *testing.T, port int, user, pass string) (*smb2.Session, error) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	d := &smb2.Dialer{Initiator: &smb2.NTLMInitiator{User: user, Password: pass}}
	sess, err := d.DialConn(context.Background(), conn, "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		conn.Close()
		return nil, err
	}
	t.Cleanup(func() { _ = sess.Logoff(); conn.Close() })
	return sess, nil
}

// hasLog 等待运行日志出现包含 sub 的记录。服务端在回复客户端之后才写日志，需要稍等。
func hasLog(m *Module, sub string) bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, e := range m.logs.Since(0) {
			if strings.Contains(e.Message, sub) {
				return true
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func TestServeFiles(t *testing.T) {
	media, docs, single := t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(docs, "readme.txt"), []byte("docs"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, port := startTestModule(t, Settings{Users: []User{
		{Name: "alice", Password: "p1", Mounts: []mountfs.Mount{
			{Name: "media", Path: media},
			{Name: "docs", Path: docs, ReadOnly: true},
		}},
		{Name: "bob", Password: "p2", Root: single},
	}})

	if _, err := dial(t, port, "alice", "wrong"); err == nil {
		t.Fatal("错误密码应登录失败")
	}
	if !hasLog(m, `认证失败（用户 "alice"）`) {
		t.Fatal("认证失败应写入运行日志")
	}

	sess, err := dial(t, port, "alice", "p1")
	if err != nil {
		t.Fatal(err)
	}
	if !hasLog(m, `登录（用户 "alice"）`) {
		t.Fatal("登录成功应写入运行日志")
	}
	names, err := sess.ListSharenames()
	if err != nil {
		t.Fatal(err)
	}
	// 共享列表只包含当前用户自己的共享
	slices.Sort(names)
	if !slices.Contains(names, "docs") || !slices.Contains(names, "media") || slices.Contains(names, "bob") {
		t.Fatalf("共享列表不正确: %v", names)
	}

	// 读写共享：写入、读回、改名、建目录、删除
	fs, err := sess.Mount("media")
	if err != nil {
		t.Fatal(err)
	}
	data := bytes.Repeat([]byte("0123456789"), 300_000) // 约 3MB，跨越多个读写请求
	if err := fs.WriteFile("中文 文件.bin", data, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(media, "中文 文件.bin")); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("写入内容不一致: %v", err)
	}
	if err := fs.Mkdir("sub", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fs.Rename("中文 文件.bin", `sub\moved.bin`); err != nil {
		t.Fatal(err)
	}
	if got, err := fs.ReadFile(`sub\moved.bin`); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("读回内容不一致: %v", err)
	}
	if err := fs.Remove(`sub\moved.bin`); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(media, "sub", "moved.bin")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("文件应已删除: %v", err)
	}
	// 共享之外的路径不可访问
	if _, err := fs.ReadFile(`..\..\etc\passwd`); err == nil {
		t.Fatal("不应能访问共享之外的文件")
	}

	// 只读共享：可读不可写
	ro, err := sess.Mount("docs")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := ro.ReadFile("readme.txt"); err != nil || string(got) != "docs" {
		t.Fatalf("只读共享读取失败: %q %v", got, err)
	}
	if err := ro.WriteFile("new.txt", []byte("x"), 0o644); err == nil {
		t.Fatal("只读共享不应能写入")
	}
	if err := ro.Remove("readme.txt"); err == nil {
		t.Fatal("只读共享不应能删除")
	}

	// 其他用户的共享不可连接
	if _, err := sess.Mount("bob"); err == nil {
		t.Fatal("不应能连接其他用户的共享")
	}

	// 单目录用户的共享名为用户名，用户名不区分大小写
	bobSess, err := dial(t, port, "BOB", "p2")
	if err != nil {
		t.Fatal(err)
	}
	bfs, err := bobSess.Mount("bob")
	if err != nil {
		t.Fatal(err)
	}
	if err := bfs.WriteFile("hello.txt", []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(single, "hello.txt")); string(got) != "hi" {
		t.Fatalf("单目录写入不正确: %q", got)
	}
	if _, err := bobSess.Mount("media"); err == nil {
		t.Fatal("bob 不应能连接 alice 的共享")
	}
}

// 不同用户的同名共享指向各自的目录。
func TestSameShareNamePerUser(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(a, "who"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(b, "who"), []byte("b"), 0o644)
	_, port := startTestModule(t, Settings{Users: []User{
		{Name: "ua", Password: "p", Mounts: []mountfs.Mount{{Name: "home", Path: a}}},
		{Name: "ub", Password: "p", Mounts: []mountfs.Mount{{Name: "home", Path: b}}},
	}})
	for user, want := range map[string]string{"ua": "a", "ub": "b"} {
		sess, err := dial(t, port, user, "p")
		if err != nil {
			t.Fatal(err)
		}
		fs, err := sess.Mount("HOME")
		if err != nil {
			t.Fatal(err)
		}
		if got, err := fs.ReadFile("who"); err != nil || string(got) != want {
			t.Fatalf("%s 读到 %q %v，期望 %q", user, got, err, want)
		}
	}
}

func TestRequireEncryption(t *testing.T) {
	dir := t.TempDir()
	_, port := startTestModule(t, Settings{RequireEncryption: true, Users: []User{{Name: "u", Password: "p", Root: dir}}})
	sess, err := dial(t, port, "u", "p")
	if err != nil {
		t.Fatal(err)
	}
	fs, err := sess.Mount("u")
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFile("x", []byte("enc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "x")); string(got) != "enc" {
		t.Fatalf("加密会话写入不正确: %q", got)
	}
}

func TestRestartAndBanlist(t *testing.T) {
	dir := t.TempDir()
	m, port := startTestModule(t, Settings{Users: []User{{Name: "u", Password: "old", Root: dir}}})
	if _, err := dial(t, port, "u", "old"); err != nil {
		t.Fatal(err)
	}

	// 改密码后重启，旧密码失效、新密码可用，端口能重新监听
	m.mu.Lock()
	m.settings.Users[0].Password = "new"
	m.restartLocked()
	m.mu.Unlock()
	if m.err != "" {
		t.Fatal(m.err)
	}
	if _, err := dial(t, port, "u", "old"); err == nil {
		t.Fatal("旧密码应失效")
	}
	if _, err := dial(t, port, "u", "new"); err != nil {
		t.Fatal(err)
	}

	// 被封禁的地址连接后立即断开
	if _, _, err := banlist.Default.Ban("127.0.0.1", banlist.Options{Duration: time.Minute, Force: true}); err != nil {
		t.Fatal(err)
	}
	defer banlist.Default.Unban(banlist.KeyFor(netip.MustParseAddr("127.0.0.1")))
	if _, err := dial(t, port, "u", "new"); err == nil {
		t.Fatal("被封禁的地址不应能登录")
	}

	// 停用后端口释放
	m.mu.Lock()
	m.settings.Enable = false
	m.restartLocked()
	m.mu.Unlock()
	ln, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatalf("停用后端口应释放: %v", err)
	}
	ln.Close()
}

func TestListenConflict(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	st, _ := config.Open(t.TempDir())
	m := New()
	_ = m.Init(module.Env{Store: st, Logs: logbuf.New(20, log.New(io.Discard, "", 0))})
	m.settings = Settings{Enable: true, ListenIP: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port, Users: []User{{Name: "u", Password: "p", Root: t.TempDir()}}}
	m.mu.Lock()
	m.restartLocked()
	m.mu.Unlock()
	if m.err == "" || m.srv != nil {
		t.Fatal("端口被占用时应报告错误")
	}
}

func TestLogHandlerFilters(t *testing.T) {
	logs := logbuf.New(20, log.New(io.Discard, "", 0))
	lg := slog.New(&logHandler{logs: logs}).With("remote", "1.2.3.4:5")
	lg.Info("connection opened")
	lg.Warn("negotiate failed", "err", "read negotiate frame: EOF")
	lg.Warn("session-setup error", "err", `user "a": ntlm: authentication failed`)
	lg.Warn("delete-on-close failed", "path", "/x/t", "err", "remove /x/t: directory not empty")
	lg.Warn("auth failed", "err", `unknown user "x\"y"`)
	lg.Warn("auth failed", "err", `too many failed logins (user "z")`)
	lg.Warn("session-setup error", "err", `too many failed logins (user "z")`)
	lg.Info("session authenticated", "smb_user", "alice")
	lg.Warn("tree-connect: unknown share", "share_name", "media")
	var got []string
	for _, e := range logs.Since(0) {
		got = append(got, e.Level+" "+e.Message)
	}
	want := []string{
		`warn 1.2.3.4:5 认证失败（用户 "x\"y"）`,
		`warn 1.2.3.4:5 登录失败次数过多，已拒绝（用户 "z"）`,
		`info 1.2.3.4:5 登录（用户 "alice"）`,
		`warn 1.2.3.4:5 tree-connect: unknown share share_name=media`,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("日志转换不正确:\n%q\n期望\n%q", got, want)
	}
}

// TestAuthThrottle 验证同一来源认证失败 10 次后被拒绝（即使密码正确）并写入日志，
// 重启服务后清空计数。
func TestAuthThrottle(t *testing.T) {
	root := t.TempDir()
	m, port := startTestModule(t, Settings{Users: []User{{Name: "alice", Password: "p1", Root: root}}})
	for i := 0; i < fileusers.ThrottleMaxFailures; i++ {
		if _, err := dial(t, port, "alice", "wrong"); err == nil {
			t.Fatal("错误密码应登录失败")
		}
	}
	if _, err := dial(t, port, "alice", "p1"); err == nil {
		t.Fatal("失败次数过多的来源应被拒绝")
	}
	if !hasLog(m, `登录失败次数过多，已拒绝（用户 "alice"）`) {
		t.Fatal("拒绝应写入运行日志")
	}

	m.mu.Lock()
	m.restartLocked()
	m.mu.Unlock()
	if _, err := dial(t, port, "alice", "p1"); err != nil {
		t.Fatalf("重启后限流应清空: %v", err)
	}
}

func TestMDNSNames(t *testing.T) {
	cases := []struct{ in, instance, host string }{
		{"nas", "nas (Gately)", "nas"},
		{"VanJay-fnOS.local", "VanJay-fnOS (Gately)", "VanJay-fnOS"},
		{"", "gately (Gately)", "gately"},
		{strings.Repeat("a", 70), strings.Repeat("a", 54) + " (Gately)", strings.Repeat("a", 63)},
		{strings.Repeat("中", 30), strings.Repeat("中", 18) + " (Gately)", strings.Repeat("中", 21)},
	}
	for _, c := range cases {
		instance, host := mdnsNames(c.in)
		if instance != c.instance || host != c.host || len(instance) > 63 || len(host) > 63 {
			t.Errorf("mdnsNames(%q) = %q, %q", c.in, instance, host)
		}
	}
}

// TestCloseReleasesDurableHandles 验证服务关闭（保存配置时的重启）会关闭为断线续用保留的句柄，
// 不必等到 GC 才释放文件描述符与锁。
func TestCloseReleasesDurableHandles(t *testing.T) {
	m, _ := startTestModule(t, Settings{Users: []User{{Name: "u", Password: "p", Root: t.TempDir()}}})
	f, err := os.CreateTemp(t.TempDir(), "durable-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	var cg, crg [16]byte
	crg[0] = 1
	m.mu.Lock()
	srv := m.srv
	srv.durable.Register(cg, crg, &parent.Open{File: f}, time.Minute, "u", "u")
	srv.durable.Detach(cg, crg)
	m.restartLocked()
	m.mu.Unlock()
	if err := f.Close(); err == nil {
		t.Fatal("重启后旧服务保留的句柄应已关闭")
	}
}

// TestStatusAfterListenerFailure 验证监听不可恢复地失败后状态不再显示为运行中，并带上错误。
func TestStatusAfterListenerFailure(t *testing.T) {
	m, _ := startTestModule(t, Settings{Users: []User{{Name: "u", Password: "p", Root: t.TempDir()}}})
	m.mu.Lock()
	running, msg := m.statusLocked()
	m.mu.Unlock()
	if !running || msg != "" {
		t.Fatalf("刚启动应为运行中: %v %q", running, msg)
	}
	// 绕过服务直接关闭底层监听，模拟 Accept 永久失败
	m.mu.Lock()
	_ = m.srv.ln.Close()
	m.mu.Unlock()
	deadline := time.Now().Add(2 * time.Second)
	for {
		m.mu.Lock()
		running, msg = m.statusLocked()
		m.mu.Unlock()
		if !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("监听失败后状态仍为运行中")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if msg == "" {
		t.Fatal("监听失败后应返回错误信息")
	}
	select {
	case <-m.srv.done:
	case <-time.After(2 * time.Second):
		t.Fatal("监听失败后服务循环应退出")
	}
}

// TestInitInvalidConfig 验证读取到无效配置时不启动服务，并报告校验错误；保存有效配置后恢复正常。
func TestInitInvalidConfig(t *testing.T) {
	st, _ := config.Open(t.TempDir())
	if err := st.Save(ID, Settings{Enable: true, Port: 70000, Users: []User{{Name: "u", Password: "p", Root: t.TempDir()}}}); err != nil {
		t.Fatal(err)
	}
	m := New()
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(20, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	_ = m.Start(context.Background())
	defer m.Stop()
	m.mu.Lock()
	running, msg := m.statusLocked()
	srv := m.srv
	m.mu.Unlock()
	if srv != nil || running || msg != "端口无效" {
		t.Fatalf("无效配置不应启动服务: srv=%v running=%v err=%q", srv != nil, running, msg)
	}
	if !hasLog(m, "端口无效") {
		t.Fatal("应记录配置校验错误")
	}

	// 有效配置加载时按保存规则规范化（去掉用户名首尾空白、补默认端口）
	st2, _ := config.Open(t.TempDir())
	if err := st2.Save(ID, Settings{Users: []User{{Name: " u ", Password: "p", Root: "/tmp"}}}); err != nil {
		t.Fatal(err)
	}
	m2 := New()
	if err := m2.Init(module.Env{Store: st2, Logs: logbuf.New(20, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	if m2.loadErr != nil || m2.settings.Users[0].Name != "u" || m2.settings.Port != DefaultPort {
		t.Fatalf("加载时应规范化配置: %+v %v", m2.settings, m2.loadErr)
	}
}
