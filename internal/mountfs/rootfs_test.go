package mountfs

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/spf13/afero"
	xwebdav "golang.org/x/net/webdav"
)

// symlinkFixture 建立共享目录 root 与目录外的 out：
//
//	out/secret.txt、out/sub/
//	root/inside.txt、root/indir/a.txt
//	root/link-file -> out/secret.txt，root/link-dir -> out（均为绝对路径）
//	root/link-up -> ../out（相对路径，向上越出）
//	root/link-in -> inside.txt，root/link-indir -> indir（指向共享目录内，应可用）
func symlinkFixture(t *testing.T) (root, out string) {
	t.Helper()
	base := t.TempDir()
	root, out = filepath.Join(base, "root"), filepath.Join(base, "out")
	for _, d := range []string{filepath.Join(out, "sub"), filepath.Join(root, "indir")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(out, "secret.txt"):   "secret",
		filepath.Join(root, "inside.txt"):  "inside",
		filepath.Join(root, "indir/a.txt"): "a",
	}
	for p, c := range files {
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	links := map[string]string{
		"link-file":  filepath.Join(out, "secret.txt"),
		"link-dir":   out,
		"link-up":    "../out",
		"link-in":    "inside.txt",
		"link-indir": "indir",
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Skipf("无法创建符号链接: %v", err)
		}
	}
	return root, out
}

// assertOutsideIntact 确认目录外的内容未被读写或改动。
func assertOutsideIntact(t *testing.T, out string) {
	t.Helper()
	if b, err := os.ReadFile(filepath.Join(out, "secret.txt")); err != nil || string(b) != "secret" {
		t.Fatalf("目录外的文件被改动: %q %v", b, err)
	}
	entries, _ := os.ReadDir(out)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	if len(names) != 2 || names[0] != "secret.txt" || names[1] != "sub" {
		t.Fatalf("目录外出现了新内容: %v", names)
	}
	if sub, _ := os.ReadDir(filepath.Join(out, "sub")); len(sub) != 0 {
		t.Fatalf("目录外的子目录出现了新内容: %v", sub)
	}
}

func TestRootDAVSymlinks(t *testing.T) {
	root, out := symlinkFixture(t)
	ctx := context.Background()
	fsys := RootDAV(root)
	escaping := []string{"/link-file", "/link-dir/secret.txt", "/link-up/secret.txt"}
	for _, name := range escaping {
		if f, err := fsys.OpenFile(ctx, name, os.O_RDONLY, 0); err == nil {
			f.Close()
			t.Fatalf("%s: 不应能读取目录外的文件", name)
		} else if !errors.Is(err, fs.ErrPermission) {
			t.Fatalf("%s: 应返回权限错误，得到 %v", name, err)
		}
		if f, err := fsys.OpenFile(ctx, name, os.O_WRONLY|os.O_TRUNC, 0); err == nil {
			f.Close()
			t.Fatalf("%s: 不应能写入目录外的文件", name)
		}
		if _, err := fsys.Stat(ctx, name); err == nil {
			t.Fatalf("%s: 不应能查看目录外的文件", name)
		}
	}
	for _, dir := range []string{"/link-dir", "/link-up"} {
		if f, err := fsys.OpenFile(ctx, dir, os.O_RDONLY, 0); err == nil {
			f.Close()
			t.Fatalf("%s: 不应能列出目录外的目录", dir)
		}
		if err := fsys.Mkdir(ctx, dir+"/sub/new", 0o755); err == nil {
			t.Fatalf("%s: 不应能在目录外建目录", dir)
		}
		if f, err := fsys.OpenFile(ctx, dir+"/new.txt", os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			f.Close()
			t.Fatalf("%s: 不应能在目录外建文件", dir)
		}
		if err := fsys.Rename(ctx, dir+"/secret.txt", "/stolen.txt"); err == nil {
			t.Fatalf("%s: 不应能把目录外的文件移入", dir)
		}
		if err := fsys.Rename(ctx, "/inside.txt", dir+"/moved.txt"); err == nil {
			t.Fatalf("%s: 不应能把文件移到目录外", dir)
		}
		if err := fsys.RemoveAll(ctx, dir+"/secret.txt"); err == nil {
			t.Fatalf("%s: 不应能删除目录外的文件", dir)
		}
	}
	// 根目录列表照常返回（含链接本身）
	f, err := fsys.OpenFile(ctx, "/", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	infos, err := f.Readdir(-1)
	f.Close()
	if err != nil || len(infos) != 7 {
		t.Fatalf("根目录列表应有 7 项: %d %v", len(infos), err)
	}
	// 指向共享目录内的链接照常可用
	if b := readDAV(t, fsys, "/link-in"); b != "inside" {
		t.Fatalf("目录内的文件链接应可读: %q", b)
	}
	if b := readDAV(t, fsys, "/link-indir/a.txt"); b != "a" {
		t.Fatalf("目录内的目录链接应可读: %q", b)
	}
	// 删除指向目录外目录的链接只删除链接本身
	if err := fsys.RemoveAll(ctx, "/link-dir"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, "link-dir")); !os.IsNotExist(err) {
		t.Fatal("链接应已删除")
	}
	// 改名移动链接本身（不跟随）不越出共享目录
	if err := fsys.Rename(ctx, "/link-file", "/renamed-link"); err != nil {
		t.Fatal(err)
	}
	if _, err := fsys.OpenFile(ctx, "/renamed-link", os.O_RDONLY, 0); err == nil {
		t.Fatal("改名后的链接仍不应能读取目录外的文件")
	}
	// 共享目录本身不能删除或改名
	if err := fsys.RemoveAll(ctx, "/"); err == nil {
		t.Fatal("不应能删除共享目录本身")
	}
	if err := fsys.Rename(ctx, "/", "/x"); err == nil {
		t.Fatal("不应能改名共享目录本身")
	}
	// ".." 无法越过共享目录
	if b := readDAV(t, fsys, "/../../root/inside.txt"); b != "" {
		t.Fatalf("越过共享目录的 .. 应被规范化到共享目录内: %q", b)
	}
	assertOutsideIntact(t, out)
}

func readDAV(t *testing.T, fsys xwebdav.FileSystem, name string) string {
	t.Helper()
	f, err := fsys.OpenFile(context.Background(), name, os.O_RDONLY, 0)
	if err != nil {
		return ""
	}
	defer f.Close()
	b, _ := io.ReadAll(f)
	return string(b)
}

func TestRootAferoSymlinks(t *testing.T) {
	root, out := symlinkFixture(t)
	fsys := RootAfero(root)
	for _, name := range []string{"/link-file", "/link-dir/secret.txt", "/link-up/secret.txt"} {
		if f, err := fsys.Open(name); err == nil {
			f.Close()
			t.Fatalf("%s: 不应能读取目录外的文件", name)
		} else if !errors.Is(err, fs.ErrPermission) {
			t.Fatalf("%s: 应返回权限错误，得到 %v", name, err)
		}
		if f, err := fsys.OpenFile(name, os.O_WRONLY|os.O_TRUNC, 0); err == nil {
			f.Close()
			t.Fatalf("%s: 不应能写入目录外的文件", name)
		}
		if err := fsys.Chmod(name, 0o777); err == nil {
			t.Fatalf("%s: 不应能修改目录外文件的权限", name)
		}
		if err := fsys.Remove(name + "/x"); err == nil {
			t.Fatalf("%s: 不应能删除目录外的文件", name)
		}
	}
	for _, dir := range []string{"/link-dir", "/link-up"} {
		if _, err := fsys.Create(dir + "/new.txt"); err == nil {
			t.Fatalf("%s: 不应能在目录外建文件", dir)
		}
		if err := fsys.MkdirAll(dir+"/sub/x/y", 0o755); err == nil {
			t.Fatalf("%s: 不应能在目录外建目录", dir)
		}
		if err := fsys.Rename(dir+"/secret.txt", "/stolen.txt"); err == nil {
			t.Fatalf("%s: 不应能把目录外的文件移入", dir)
		}
		if err := fsys.Rename("/inside.txt", dir+"/moved.txt"); err == nil {
			t.Fatalf("%s: 不应能把文件移到目录外", dir)
		}
		if err := fsys.RemoveAll(dir + "/sub"); err == nil {
			t.Fatalf("%s: 不应能删除目录外的目录", dir)
		}
	}
	// 修改属主一律拒绝（FTP 的 SITE CHOWN）
	if err := fsys.Chown("/inside.txt", 0, 0); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("修改属主应被拒绝: %v", err)
	}
	if b, _ := afero.ReadFile(fsys, "/link-in"); string(b) != "inside" {
		t.Fatalf("目录内的文件链接应可读: %q", b)
	}
	if names, err := afero.ReadDir(fsys, "/link-indir"); err != nil || len(names) != 1 {
		t.Fatalf("目录内的目录链接应可列出: %v %v", names, err)
	}
	// 正常读写、建目录、改名、删除
	if err := afero.WriteFile(fsys, "/indir/new.txt", []byte("n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := fsys.MkdirAll("/d1/d2", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fsys.Rename("/indir/new.txt", "/d1/d2/new.txt"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "d1/d2/new.txt")); string(b) != "n" {
		t.Fatalf("改名后的内容不正确: %q", b)
	}
	if err := fsys.RemoveAll("/d1"); err != nil {
		t.Fatal(err)
	}
	if err := fsys.RemoveAll("/"); err == nil {
		t.Fatal("不应能删除共享目录本身")
	}
	assertOutsideIntact(t, out)
}

// 多目录组合中各挂载同样不能经符号链接越出。
func TestMountsSymlinks(t *testing.T) {
	root, out := symlinkFixture(t)
	mounts := []Mount{{Name: "m", Path: root}}
	if _, err := Afero(mounts).Open("/m/link-dir/secret.txt"); err == nil {
		t.Fatal("FTP 多目录：不应能读取目录外的文件")
	}
	if err := Afero(mounts).Chown("/m/inside.txt", 0, 0); err == nil {
		t.Fatal("FTP 多目录：修改属主应被拒绝")
	}
	if _, err := WebDAV(mounts).OpenFile(context.Background(), "/m/link-file", os.O_RDONLY, 0); err == nil {
		t.Fatal("WebDAV 多目录：不应能读取目录外的文件")
	}
	if b := readDAV(t, WebDAV(mounts), "/m/link-in"); b != "inside" {
		t.Fatalf("WebDAV 多目录：目录内的链接应可读: %q", b)
	}
	assertOutsideIntact(t, out)
}
