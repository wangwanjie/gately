package mountfs

import (
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/afero"
	xwebdav "golang.org/x/net/webdav"
)

// 共享目录的文件系统，WebDAV 与 FTP 共用：所有访问经 os.Root 进行。
// 共享目录内指向目录外的符号链接（例如经其他途径写入、或被共享的家目录中原有的链接）无法被跟随，
// 读、写、列目录、改名、复制都不会越出共享目录；指向共享目录内的符号链接照常可用。
// 每次操作都重新打开共享目录，目录被删除重建后不会继续使用旧的目录句柄。

// errPathEscapes 是 os.Root 拒绝越出根目录的路径时的错误文字（os 包未导出该错误）。
const errPathEscapes = "path escapes from parent"

// rootRel 把客户端路径（以 / 分隔，可带或不带开头的 /）转换为 os.Root 使用的相对路径，共享目录本身为 "."。
// 路径先按 / 规范化，".." 无法越过共享目录；含 NUL 或本系统路径分隔符（Windows 的 \）的名称视为不存在。
func rootRel(name string) (string, bool) {
	if strings.Contains(name, "\x00") || (filepath.Separator != '/' && strings.ContainsRune(name, filepath.Separator)) {
		return "", false
	}
	rel := strings.TrimPrefix(path.Clean("/"+name), "/")
	if rel == "" {
		return ".", true
	}
	return filepath.FromSlash(rel), true
}

// mapRootErr 把越出共享目录的错误改为权限错误：客户端看到的是拒绝访问，WebDAV 列目录时会跳过这一项。
func mapRootErr(op, name string, err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), errPathEscapes) {
		return &fs.PathError{Op: op, Path: name, Err: fs.ErrPermission}
	}
	return err
}

// withRoot 打开共享目录 dir 并对相对路径 name 执行 fn。
func withRoot(dir, op, name string, fn func(r *os.Root, rel string) error) error {
	rel, ok := rootRel(name)
	if !ok {
		return &fs.PathError{Op: op, Path: name, Err: fs.ErrNotExist}
	}
	r, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer r.Close()
	return mapRootErr(op, name, fn(r, rel))
}

// rootDAV 是以 os.Root 限定在共享目录内的 webdav.FileSystem，取代 webdav.Dir。
type rootDAV struct{ dir string }

// RootDAV 返回限定在 dir 内的 webdav.FileSystem。
func RootDAV(dir string) xwebdav.FileSystem { return rootDAV{dir: dir} }

func (d rootDAV) Mkdir(_ context.Context, name string, perm os.FileMode) error {
	return withRoot(d.dir, "mkdir", name, func(r *os.Root, rel string) error { return r.Mkdir(rel, perm) })
}

func (d rootDAV) OpenFile(_ context.Context, name string, flag int, perm os.FileMode) (xwebdav.File, error) {
	var f *os.File
	err := withRoot(d.dir, "open", name, func(r *os.Root, rel string) error {
		var err error
		// 从 Root 打开的文件在 Root 关闭后仍然有效
		f, err = r.OpenFile(rel, flag, perm)
		return err
	})
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (d rootDAV) RemoveAll(_ context.Context, name string) error {
	return withRoot(d.dir, "remove", name, func(r *os.Root, rel string) error {
		// 与 webdav.Dir 一致：不允许删除共享目录本身
		if rel == "." {
			return os.ErrInvalid
		}
		return r.RemoveAll(rel)
	})
}

func (d rootDAV) Rename(_ context.Context, oldName, newName string) error {
	oldRel, ok1 := rootRel(oldName)
	newRel, ok2 := rootRel(newName)
	if !ok1 || !ok2 {
		return &fs.PathError{Op: "rename", Path: oldName, Err: fs.ErrNotExist}
	}
	// 与 webdav.Dir 一致：共享目录本身不能被改名，也不能被覆盖
	if oldRel == "." || newRel == "." {
		return os.ErrInvalid
	}
	return withRoot(d.dir, "rename", oldName, func(r *os.Root, _ string) error { return r.Rename(oldRel, newRel) })
}

func (d rootDAV) Stat(_ context.Context, name string) (os.FileInfo, error) {
	var fi os.FileInfo
	err := withRoot(d.dir, "stat", name, func(r *os.Root, rel string) error {
		var err error
		fi, err = r.Stat(rel)
		return err
	})
	return fi, err
}

// rootAfero 是以 os.Root 限定在共享目录内的 afero.Fs，取代 afero.BasePathFs。
type rootAfero struct{ dir string }

// RootAfero 返回限定在 dir 内的 afero.Fs。修改属主（FTP 的 SITE CHOWN）一律拒绝：
// 文件服务以 Gately 进程身份读写，允许改属主等于让客户端把文件交给任意系统用户。
func RootAfero(dir string) afero.Fs { return rootAfero{dir: dir} }

func (a rootAfero) Name() string { return "rootfs" }

func (a rootAfero) Create(name string) (afero.File, error) {
	return a.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
}

func (a rootAfero) Mkdir(name string, perm os.FileMode) error {
	return withRoot(a.dir, "mkdir", name, func(r *os.Root, rel string) error { return r.Mkdir(rel, perm) })
}

func (a rootAfero) MkdirAll(name string, perm os.FileMode) error {
	return withRoot(a.dir, "mkdir", name, func(r *os.Root, rel string) error { return r.MkdirAll(rel, perm) })
}

func (a rootAfero) Open(name string) (afero.File, error) {
	return a.OpenFile(name, os.O_RDONLY, 0)
}

func (a rootAfero) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	var f *os.File
	err := withRoot(a.dir, "open", name, func(r *os.Root, rel string) error {
		var err error
		f, err = r.OpenFile(rel, flag, perm)
		return err
	})
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (a rootAfero) Remove(name string) error {
	return withRoot(a.dir, "remove", name, func(r *os.Root, rel string) error {
		if rel == "." {
			return os.ErrInvalid
		}
		return r.Remove(rel)
	})
}

func (a rootAfero) RemoveAll(name string) error {
	return withRoot(a.dir, "remove", name, func(r *os.Root, rel string) error {
		if rel == "." {
			return os.ErrInvalid
		}
		return r.RemoveAll(rel)
	})
}

func (a rootAfero) Rename(oldName, newName string) error {
	oldRel, ok1 := rootRel(oldName)
	newRel, ok2 := rootRel(newName)
	if !ok1 || !ok2 {
		return &fs.PathError{Op: "rename", Path: oldName, Err: fs.ErrNotExist}
	}
	if oldRel == "." || newRel == "." {
		return os.ErrInvalid
	}
	return withRoot(a.dir, "rename", oldName, func(r *os.Root, _ string) error { return r.Rename(oldRel, newRel) })
}

func (a rootAfero) Stat(name string) (os.FileInfo, error) {
	var fi os.FileInfo
	err := withRoot(a.dir, "stat", name, func(r *os.Root, rel string) error {
		var err error
		fi, err = r.Stat(rel)
		return err
	})
	return fi, err
}

func (a rootAfero) Chmod(name string, mode os.FileMode) error {
	return withRoot(a.dir, "chmod", name, func(r *os.Root, rel string) error { return r.Chmod(rel, mode) })
}

// Chown 一律拒绝，见 RootAfero。
func (a rootAfero) Chown(name string, _, _ int) error {
	return &fs.PathError{Op: "chown", Path: name, Err: fs.ErrPermission}
}

func (a rootAfero) Chtimes(name string, atime, mtime time.Time) error {
	return withRoot(a.dir, "chtimes", name, func(r *os.Root, rel string) error { return r.Chtimes(rel, atime, mtime) })
}

// 编译期检查接口实现。
var (
	_ xwebdav.FileSystem = rootDAV{}
	_ afero.Fs           = rootAfero{}
)
