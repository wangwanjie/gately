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

	"github.com/wangwanjie/gately/internal/fsperm"
)

// 共享目录的文件系统，WebDAV 与 FTP 共用：所有访问经 os.Root 进行。
// 共享目录内指向目录外的符号链接（例如经其他途径写入、或被共享的家目录中原有的链接）无法被跟随，
// 读、写、列目录、改名、复制都不会越出共享目录；指向共享目录内的符号链接照常可用。
// 每次操作都重新打开共享目录，目录被删除重建后不会继续使用旧的目录句柄。
// 共享目录包含 Gately 的配置目录时（例如整个数据盘被共享），配置目录这棵子树经 fsperm.Guard 屏蔽：
// 访问其中的路径按不存在处理，列目录时不出现，改名、删除包含它的上级目录也被拒绝。

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

// rootShare 是一个共享目录及其屏蔽规则（不包含受保护目录时为 nil）。
type rootShare struct {
	dir string
	g   *fsperm.Guard
}

func newRootShare(dir string) rootShare { return rootShare{dir: dir, g: fsperm.NewGuard(dir)} }

// check 按屏蔽规则检查相对路径 rel：位于受保护目录中时按不存在拒绝；mutate 为 true（改名、删除、修改权限）时，
// 与受保护目录重叠（包括作为它的上级目录）也拒绝。
func (s rootShare) check(op, name, rel string, mutate bool) error {
	if s.g == nil {
		return nil
	}
	p := filepath.Join(s.dir, rel)
	if s.g.Hidden(p) {
		return &fs.PathError{Op: op, Path: name, Err: fs.ErrNotExist}
	}
	if mutate && s.g.Touches(p) {
		return &fs.PathError{Op: op, Path: name, Err: fs.ErrPermission}
	}
	return nil
}

// do 打开共享目录并对相对路径 name 执行 fn，执行前按屏蔽规则检查。
func (s rootShare) do(op, name string, mutate bool, fn func(r *os.Root, rel string) error) error {
	rel, ok := rootRel(name)
	if !ok {
		return &fs.PathError{Op: op, Path: name, Err: fs.ErrNotExist}
	}
	if err := s.check(op, name, rel, mutate); err != nil {
		return err
	}
	r, err := os.OpenRoot(s.dir)
	if err != nil {
		return err
	}
	defer r.Close()
	return mapRootErr(op, name, fn(r, rel))
}

// open 打开文件；共享目录有屏蔽规则且打开的是目录时，返回列目录时过滤受保护目录的包装。
func (s rootShare) open(name string, flag int, perm os.FileMode) (*os.File, *guardedDir, error) {
	var f *os.File
	var rel string
	err := s.do("open", name, false, func(r *os.Root, rl string) error {
		var err error
		rel = rl
		// 从 Root 打开的文件在 Root 关闭后仍然有效
		f, err = r.OpenFile(rl, flag, perm)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	if s.g != nil {
		if st, err := f.Stat(); err == nil && st.IsDir() {
			return f, &guardedDir{File: f, dir: filepath.Join(s.dir, rel), g: s.g}, nil
		}
	}
	return f, nil, nil
}

// rename 改名，源与目标都按屏蔽规则检查。
func (s rootShare) rename(oldName, newName string) error {
	oldRel, ok1 := rootRel(oldName)
	newRel, ok2 := rootRel(newName)
	if !ok1 || !ok2 {
		return &fs.PathError{Op: "rename", Path: oldName, Err: fs.ErrNotExist}
	}
	// 与 webdav.Dir 一致：共享目录本身不能被改名，也不能被覆盖
	if oldRel == "." || newRel == "." {
		return os.ErrInvalid
	}
	if err := s.check("rename", oldName, oldRel, true); err != nil {
		return err
	}
	if err := s.check("rename", newName, newRel, true); err != nil {
		return err
	}
	return s.do("rename", oldName, false, func(r *os.Root, _ string) error { return r.Rename(oldRel, newRel) })
}

// guardedDir 是共享目录中某个目录的句柄，列目录时去掉受保护目录及指向它的符号链接。
type guardedDir struct {
	*os.File
	dir string
	g   *fsperm.Guard
}

// Readdir 返回过滤后的条目。按页读取（n > 0）时一页全被过滤掉也继续读下一页，不会在目录未读完时返回空页。
func (d *guardedDir) Readdir(n int) ([]os.FileInfo, error) {
	for {
		infos, err := d.File.Readdir(n)
		infos = d.g.FilterInfos(d.dir, infos)
		if len(infos) > 0 || err != nil || n <= 0 {
			return infos, err
		}
	}
}

// ReadDir 同 Readdir，返回 fs.DirEntry。
func (d *guardedDir) ReadDir(n int) ([]os.DirEntry, error) {
	for {
		entries, err := d.File.ReadDir(n)
		entries = d.g.FilterEntries(d.dir, entries)
		if len(entries) > 0 || err != nil || n <= 0 {
			return entries, err
		}
	}
}

// Readdirnames 同 Readdir，只返回名称。
func (d *guardedDir) Readdirnames(n int) ([]string, error) {
	infos, err := d.Readdir(n)
	names := make([]string, 0, len(infos))
	for _, fi := range infos {
		names = append(names, fi.Name())
	}
	return names, err
}

// rootDAV 是以 os.Root 限定在共享目录内的 webdav.FileSystem，取代 webdav.Dir。
type rootDAV struct{ rootShare }

// RootDAV 返回限定在 dir 内的 webdav.FileSystem。
func RootDAV(dir string) xwebdav.FileSystem { return rootDAV{newRootShare(dir)} }

func (d rootDAV) Mkdir(_ context.Context, name string, perm os.FileMode) error {
	return d.do("mkdir", name, false, func(r *os.Root, rel string) error { return r.Mkdir(rel, perm) })
}

func (d rootDAV) OpenFile(_ context.Context, name string, flag int, perm os.FileMode) (xwebdav.File, error) {
	f, gd, err := d.open(name, flag, perm)
	if err != nil {
		return nil, err
	}
	if gd != nil {
		return gd, nil
	}
	return f, nil
}

func (d rootDAV) RemoveAll(_ context.Context, name string) error {
	return d.do("remove", name, true, func(r *os.Root, rel string) error {
		// 与 webdav.Dir 一致：不允许删除共享目录本身
		if rel == "." {
			return os.ErrInvalid
		}
		return r.RemoveAll(rel)
	})
}

func (d rootDAV) Rename(_ context.Context, oldName, newName string) error {
	return d.rename(oldName, newName)
}

func (d rootDAV) Stat(_ context.Context, name string) (os.FileInfo, error) {
	var fi os.FileInfo
	err := d.do("stat", name, false, func(r *os.Root, rel string) error {
		var err error
		fi, err = r.Stat(rel)
		return err
	})
	return fi, err
}

// rootAfero 是以 os.Root 限定在共享目录内的 afero.Fs，取代 afero.BasePathFs。
type rootAfero struct{ rootShare }

// RootAfero 返回限定在 dir 内的 afero.Fs。修改属主（FTP 的 SITE CHOWN）一律拒绝：
// 文件服务以 Gately 进程身份读写，允许改属主等于让客户端把文件交给任意系统用户。
func RootAfero(dir string) afero.Fs { return rootAfero{newRootShare(dir)} }

func (a rootAfero) Name() string { return "rootfs" }

func (a rootAfero) Create(name string) (afero.File, error) {
	return a.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
}

func (a rootAfero) Mkdir(name string, perm os.FileMode) error {
	return a.do("mkdir", name, false, func(r *os.Root, rel string) error { return r.Mkdir(rel, perm) })
}

func (a rootAfero) MkdirAll(name string, perm os.FileMode) error {
	return a.do("mkdir", name, false, func(r *os.Root, rel string) error { return r.MkdirAll(rel, perm) })
}

func (a rootAfero) Open(name string) (afero.File, error) {
	return a.OpenFile(name, os.O_RDONLY, 0)
}

func (a rootAfero) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	f, gd, err := a.open(name, flag, perm)
	if err != nil {
		return nil, err
	}
	if gd != nil {
		return gd, nil
	}
	return f, nil
}

func (a rootAfero) Remove(name string) error {
	return a.do("remove", name, true, func(r *os.Root, rel string) error {
		if rel == "." {
			return os.ErrInvalid
		}
		return r.Remove(rel)
	})
}

func (a rootAfero) RemoveAll(name string) error {
	return a.do("remove", name, true, func(r *os.Root, rel string) error {
		if rel == "." {
			return os.ErrInvalid
		}
		return r.RemoveAll(rel)
	})
}

func (a rootAfero) Rename(oldName, newName string) error {
	return a.rename(oldName, newName)
}

func (a rootAfero) Stat(name string) (os.FileInfo, error) {
	var fi os.FileInfo
	err := a.do("stat", name, false, func(r *os.Root, rel string) error {
		var err error
		fi, err = r.Stat(rel)
		return err
	})
	return fi, err
}

// Chmod 修改权限：受保护目录的上级目录同样不能修改，否则改成不可进入就能让 Gately 读不到自己的配置。
func (a rootAfero) Chmod(name string, mode os.FileMode) error {
	return a.do("chmod", name, true, func(r *os.Root, rel string) error { return r.Chmod(rel, mode) })
}

// Chown 一律拒绝，见 RootAfero。
func (a rootAfero) Chown(name string, _, _ int) error {
	return &fs.PathError{Op: "chown", Path: name, Err: fs.ErrPermission}
}

func (a rootAfero) Chtimes(name string, atime, mtime time.Time) error {
	return a.do("chtimes", name, false, func(r *os.Root, rel string) error { return r.Chtimes(rel, atime, mtime) })
}

// 编译期检查接口实现。
var (
	_ xwebdav.FileSystem = rootDAV{}
	_ afero.Fs           = rootAfero{}
	_ xwebdav.File       = (*guardedDir)(nil)
	_ afero.File         = (*guardedDir)(nil)
)
