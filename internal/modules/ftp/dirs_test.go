package ftp

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/wangwanjie/gately/internal/fileusers"
	"github.com/wangwanjie/gately/internal/fsperm/fspermtest"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/mountfs"
)

// idClient 在 fakeClient 之上补上会话编号：共享用户登录时服务端按编号登记会话。
type idClient struct {
	fakeClient
}

func (c *idClient) ID() uint32 { return 1 }

// startFTP 以 l 中保存的设置启动 FTP 模块（模拟升级前已保存的配置），shared 为共享用户。
func startFTP(t *testing.T, l fspermtest.Layout, users []User, shared []fileusers.User) (*Module, *logbuf.Buffer) {
	t.Helper()
	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	if err := l.Store.Save(ID, Settings{Enable: true, ListenIP: "127.0.0.1", Port: port, SharedUsers: len(shared) > 0, Users: users}); err != nil {
		t.Fatal(err)
	}
	if len(shared) > 0 {
		if err := l.Store.Save(fileusers.ID, map[string]any{"users": shared}); err != nil {
			t.Fatal(err)
		}
	}
	logs := logbuf.New(50, log.New(io.Discard, "", 0))
	m := New(nil)
	if err := m.Init(module.Env{Store: l.Store, Logs: logs}); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop() })
	if m.err != "" {
		t.Fatalf("服务应正常启动: %s", m.err)
	}
	return m, logs
}

func loginFS(t *testing.T, m *Module, user string) afero.Fs {
	t.Helper()
	drv, err := m.drv.AuthUser(&idClient{}, user, "pw")
	if err != nil {
		t.Fatalf("%s 登录失败: %v", user, err)
	}
	return drv.(afero.Fs)
}

func logCount(logs *logbuf.Buffer, sub string) int {
	n := 0
	for _, e := range logs.Query(0, 0, 0, nil) {
		if strings.Contains(e.Message, sub) {
			n++
		}
	}
	return n
}

// 用户目录就是配置目录或位于其中：保存时拒绝；升级前已保存的照常启动，但这个目录不再提供任何内容。
func TestUserDirsInsideConfigDir(t *testing.T) {
	l := fspermtest.New(t)
	m, logs := startFTP(t, l, []User{
		{Name: "leak", Password: "pw", Root: l.Conf},
		{Name: "multi", Password: "pw", Mounts: []mountfs.Mount{{Name: "ok", Path: l.Media}, {Name: "cfg", Path: filepath.Join(l.Conf, "history")}}},
	}, []fileusers.User{{Name: "shared", Password: "pw", Root: l.Conf}})
	for _, c := range [][2]string{{"leak", "/account.json"}, {"shared", "/account.json"}} {
		if b, err := afero.ReadFile(loginFS(t, m, c[0]), c[1]); err == nil || strings.Contains(string(b), fspermtest.Secret) {
			t.Fatalf("%s 读取 %s 不应成功: %q", c[0], c[1], b)
		}
	}
	if err := afero.WriteFile(loginFS(t, m, "leak"), "/x.json", []byte("{}"), 0o644); err == nil {
		t.Fatal("向被停止提供的目录写入不应成功")
	}
	if b, err := afero.ReadFile(loginFS(t, m, "multi"), "/ok/hello.txt"); err != nil || string(b) != "hello" {
		t.Fatalf("同一用户的其他目录应照常可用: %q %v", b, err)
	}
	if n := logCount(logs, "配置目录重叠"); n != 2 {
		t.Fatalf("应为两个被停止提供的本地用户目录各写一条警告，实际 %d", n)
	}
	mux := http.NewServeMux()
	m.Routes(secretsRouter{mux})
	put := func(u User) (int, string) {
		b, _ := json.Marshal(Settings{ListenIP: "127.0.0.1", Port: 1, Users: []User{u}})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("PUT", "/settings", strings.NewReader(string(b)))
		req.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	for name, u := range map[string]User{
		"配置目录":    {Name: "x", Password: "p", Root: l.Conf},
		"配置目录内":   {Name: "x", Password: "p", Root: filepath.Join(l.Conf, "history")},
		"多目录中的一个": {Name: "x", Password: "p", Mounts: []mountfs.Mount{{Name: "a", Path: l.Media}, {Name: "b", Path: l.Conf}}},
	} {
		if code, body := put(u); code != 400 || !strings.Contains(body, "配置目录") {
			t.Errorf("%s 应被拒绝: %d %s", name, code, body)
		}
	}
	if code, body := put(User{Name: "x", Password: "p", Root: l.Share}); code != 200 {
		t.Fatalf("包含配置目录的目录应能保存: %d %s", code, body)
	}
}

// 用户目录包含配置目录（整个数据盘）：照常提供，配置目录这棵子树不可见、不可达，改名进出、删除上级目录都被拒绝。
func TestUserDirsContainingConfigDir(t *testing.T) {
	l := fspermtest.New(t)
	m, logs := startFTP(t, l, []User{
		{Name: "all", Password: "pw", Root: l.Share},
		{Name: "multi", Password: "pw", Mounts: []mountfs.Mount{{Name: "vol", Path: l.Share}}},
	}, []fileusers.User{{Name: "shared", Password: "pw", Root: l.Share}})
	if logCount(logs, "配置目录已自动隐藏") != 2 {
		t.Fatal("应说明两个本地用户目录中的配置目录已自动隐藏")
	}
	for user, prefix := range map[string]string{"all": "", "multi": "/vol", "shared": ""} {
		fs := loginFS(t, m, user)
		for p, want := range l.VisibleFiles() {
			if b, err := afero.ReadFile(fs, prefix+"/"+p); err != nil || string(b) != want {
				t.Errorf("%s 读取 %s 应成功: %q %v", user, p, b, err)
			}
		}
		for _, p := range l.HiddenFiles() {
			if b, err := afero.ReadFile(fs, prefix+"/"+p); err == nil || strings.Contains(string(b), fspermtest.Secret) {
				t.Errorf("%s 读取 %s 不应成功", user, p)
			}
			if _, err := fs.Stat(prefix + "/" + p); err == nil {
				t.Errorf("%s 查看 %s 的信息不应成功", user, p)
			}
		}
		for _, p := range l.HiddenDirs() {
			if _, err := afero.ReadDir(fs, prefix+"/"+p); err == nil {
				t.Errorf("%s 列出 %s 不应成功", user, p)
			}
		}
		for dir, names := range l.Listings() {
			// FTP 的 LIST 打开目录后按 Readdir 列出
			f, err := fs.Open(prefix + "/" + dir)
			if err != nil {
				t.Fatalf("%s 打开 /%s 失败: %v", user, dir, err)
			}
			infos, err := f.Readdir(-1)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]bool{}
			for _, fi := range infos {
				got[fi.Name()] = true
			}
			for name, want := range names {
				if got[name] != want {
					t.Errorf("%s /%s 的列表中 %s 出现=%v，期望 %v", user, dir, name, got[name], want)
				}
			}
		}
		for _, r := range l.Renames() {
			if err := fs.Rename(prefix+"/"+r[0], prefix+"/"+r[1]); err == nil {
				t.Errorf("%s 改名 %s → %s 应被拒绝", user, r[0], r[1])
			}
		}
		for _, p := range l.Removals() {
			if err := fs.RemoveAll(prefix + "/" + p); err == nil {
				t.Errorf("%s 删除 %s 应被拒绝", user, p)
			}
			if err := fs.Remove(prefix + "/" + p); err == nil {
				t.Errorf("%s 删除 %s 应被拒绝", user, p)
			}
		}
		for _, p := range []string{"1000", "1000/docker", "1000/docker/gately"} {
			if err := fs.Chmod(prefix+"/"+p, 0); err == nil {
				t.Errorf("%s 修改 %s 的权限应被拒绝", user, p)
			}
		}
		for _, p := range l.Creates() {
			if err := afero.WriteFile(fs, prefix+"/"+p, []byte("x"), 0o644); err == nil {
				t.Errorf("%s 写入 %s 应被拒绝", user, p)
			}
			if err := fs.MkdirAll(prefix+"/"+p+"-dir/sub", 0o755); err == nil {
				t.Errorf("%s 新建 %s 应被拒绝", user, p)
			}
		}
		if err := afero.WriteFile(fs, prefix+"/media/new.txt", []byte("n"), 0o644); err != nil {
			t.Fatalf("%s 写普通文件应成功: %v", user, err)
		}
		if err := fs.Rename(prefix+"/media/new.txt", prefix+"/media/new2.txt"); err != nil {
			t.Fatalf("%s 普通文件改名应成功: %v", user, err)
		}
		if err := fs.Remove(prefix + "/media/new2.txt"); err != nil {
			t.Fatalf("%s 删除普通文件应成功: %v", user, err)
		}
	}
	l.Check(t)
}
