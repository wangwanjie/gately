//go:build linux || darwin

package smb

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

	smb2 "github.com/cloudsoda/go-smb2"

	"github.com/wangwanjie/gately/internal/fsperm/fspermtest"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/mountfs"
)

// startSMB 以 l 中保存的设置启动 SMB 模块（模拟升级前已保存的配置）。
func startSMB(t *testing.T, l fspermtest.Layout, users []User) (*Module, int) {
	t.Helper()
	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	if err := l.Store.Save(ID, Settings{Enable: true, ListenIP: "127.0.0.1", Port: port, Users: users}); err != nil {
		t.Fatal(err)
	}
	m := New()
	if err := m.Init(module.Env{Store: l.Store, Logs: logbuf.New(200, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	if m.loadErr != nil {
		t.Fatalf("存量配置不应让整个服务失效: %v", m.loadErr)
	}
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop() })
	if m.err != "" {
		t.Fatalf("存量配置不应让服务启动失败: %s", m.err)
	}
	return m, port
}

func mountShare(t *testing.T, port int, user, share string) *smb2.Share {
	t.Helper()
	sess, err := dial(t, port, user, "pw")
	if err != nil {
		t.Fatal(err)
	}
	fs, err := sess.Mount(share)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fs.Umount() })
	return fs
}

// win 把以 / 分隔的路径换成 SMB 使用的 \。
func win(p string) string { return strings.ReplaceAll(strings.TrimPrefix(p, "/"), "/", `\`) }

// 用户目录就是配置目录或位于其中：保存时拒绝；升级前已保存的照常启动，但这个共享不再提供任何内容。
func TestUserDirsInsideConfigDir(t *testing.T) {
	l := fspermtest.New(t)
	m, port := startSMB(t, l, []User{
		{Name: "leak", Password: "pw", Root: l.Conf},
		{Name: "multi", Password: "pw", Mounts: []mountfs.Mount{{Name: "ok", Path: l.Media}, {Name: "cfg", Path: filepath.Join(l.Conf, "history")}}},
	})
	if !hasLog(m, "用户 leak 的目录 "+l.Conf+" 与 Gately 的配置目录重叠") || !hasLog(m, "用户 multi 的目录 "+filepath.Join(l.Conf, "history")+" 与 Gately 的配置目录重叠") {
		t.Fatal("被停止提供的目录应写入警告日志")
	}
	sess, err := dial(t, port, "leak", "pw")
	if err != nil {
		t.Fatal(err)
	}
	if fs, err := sess.Mount("leak"); err == nil {
		if b, err := fs.ReadFile("account.json"); err == nil {
			t.Fatalf("不应读到配置目录中的文件: %s", b)
		}
		_ = fs.Umount()
	}
	if b, err := mountShare(t, port, "multi", "ok").ReadFile("hello.txt"); err != nil || string(b) != "hello" {
		t.Fatalf("其他共享应照常可用: %q %v", b, err)
	}
	mux := http.NewServeMux()
	m.Routes(muxRouter{mux})
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

// 用户目录包含配置目录（整个数据盘）：照常共享，配置目录这棵子树不可见、不可达，改名进出、删除上级目录都被拒绝。
func TestUserDirsContainingConfigDir(t *testing.T) {
	l := fspermtest.New(t)
	m, port := startSMB(t, l, []User{
		{Name: "all", Password: "pw", Root: l.Share},
		{Name: "multi", Password: "pw", Mounts: []mountfs.Mount{{Name: "vol", Path: l.Share}}},
	})
	if !hasLog(m, "配置目录已自动隐藏") {
		t.Fatal("应说明配置目录已自动隐藏")
	}
	for user, share := range map[string]string{"all": "all", "multi": "vol"} {
		fs := mountShare(t, port, user, share)
		for p, want := range l.VisibleFiles() {
			if b, err := fs.ReadFile(win(p)); err != nil || string(b) != want {
				t.Errorf("%s 读取 %s 应成功: %q %v", user, p, b, err)
			}
		}
		for _, p := range l.HiddenFiles() {
			if b, err := fs.ReadFile(win(p)); err == nil || strings.Contains(string(b), fspermtest.Secret) {
				t.Errorf("%s 读取 %s 不应成功", user, p)
			}
			if _, err := fs.Stat(win(p)); err == nil {
				t.Errorf("%s 查看 %s 的信息不应成功", user, p)
			}
		}
		for _, p := range l.HiddenDirs() {
			if _, err := fs.ReadDir(win(p)); err == nil {
				t.Errorf("%s 列出 %s 不应成功", user, p)
			}
		}
		for dir, names := range l.Listings() {
			infos, err := fs.ReadDir(win(dir))
			if err != nil {
				t.Fatalf("%s 列出 /%s 失败: %v", user, dir, err)
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
			if err := fs.Rename(win(r[0]), win(r[1])); err == nil {
				t.Errorf("%s 改名 %s → %s 应被拒绝", user, r[0], r[1])
			}
		}
		for _, p := range l.Removals() {
			if err := fs.Remove(win(p)); err == nil {
				t.Errorf("%s 删除 %s 应被拒绝", user, p)
			}
		}
		for _, p := range l.Creates() {
			if err := fs.WriteFile(win(p), []byte("x"), 0o644); err == nil {
				t.Errorf("%s 写入 %s 应被拒绝", user, p)
			}
			if err := fs.Mkdir(win(p+"-dir"), 0o755); err == nil {
				t.Errorf("%s 新建 %s 应被拒绝", user, p)
			}
		}
		if err := fs.WriteFile(win("media/new.txt"), []byte("n"), 0o644); err != nil {
			t.Fatalf("%s 写普通文件应成功: %v", user, err)
		}
		if err := fs.Rename(win("media/new.txt"), win("1000/docker/gately-old/new.txt")); err != nil {
			t.Fatalf("%s 普通文件改名应成功: %v", user, err)
		}
		if err := fs.Remove(win("1000/docker/gately-old/new.txt")); err != nil {
			t.Fatalf("%s 删除普通文件应成功: %v", user, err)
		}
	}
	// 客户端递归删除上级目录（逐个删除列出的条目再删目录）：看不到的配置目录不会被删，上级目录因此删不掉
	if err := mountShare(t, port, "all", "all").RemoveAll(win("1000")); err == nil {
		t.Fatal("递归删除包含配置目录的上级目录不应成功")
	}
	l.Check(t)
}
