package mountfs

import (
	"io/fs"
	"os"
	"time"

	"github.com/spf13/afero"
)

// aferoFS 是供 FTP 使用的组合文件系统。
type aferoFS struct {
	mounts []Mount
	fss    map[string]afero.Fs
}

// Afero 返回由 mounts 组成的 afero.Fs，挂载列表需已通过 Validate。
func Afero(mounts []Mount) afero.Fs {
	a := &aferoFS{mounts: mounts, fss: map[string]afero.Fs{}}
	for _, mt := range mounts {
		// 经 os.Root 访问，指向挂载目录之外的符号链接无法被跟随
		var f afero.Fs = RootAfero(mt.Path)
		if mt.ReadOnly {
			f = afero.NewReadOnlyFs(f)
		}
		a.fss[mt.Name] = f
	}
	return a
}

// resolve 返回路径所在的挂载文件系统与挂载内路径；根目录返回 nil。
func (a *aferoFS) resolve(op, name string) (afero.Fs, string, error) {
	mount, rest := split(name)
	if mount == "" {
		return nil, "/", nil
	}
	f, ok := a.fss[mount]
	if !ok {
		return nil, "", errOp(op, name, fs.ErrNotExist)
	}
	return f, rest, nil
}

// resolveEntry 用于修改类操作：根目录与挂载目录本身都不允许修改。
func (a *aferoFS) resolveEntry(op, name string) (afero.Fs, string, error) {
	// 根目录下只有挂载目录，在根目录新建或修改条目一律视为无权限
	if _, rest := split(name); rest == "/" {
		return nil, "", errOp(op, name, fs.ErrPermission)
	}
	f, rest, err := a.resolve(op, name)
	if err != nil {
		return nil, "", err
	}
	return f, rest, nil
}

func (a *aferoFS) Name() string { return "mountfs" }

func (a *aferoFS) Create(name string) (afero.File, error) {
	return a.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
}

func (a *aferoFS) Mkdir(name string, perm os.FileMode) error {
	f, rest, err := a.resolveEntry("mkdir", name)
	if err != nil {
		return err
	}
	return f.Mkdir(rest, perm)
}

func (a *aferoFS) MkdirAll(name string, perm os.FileMode) error {
	f, rest, err := a.resolve("mkdir", name)
	if err != nil {
		return err
	}
	// 根目录与挂载目录已存在，与 os.MkdirAll 一样视为成功
	if f == nil || rest == "/" {
		return nil
	}
	return f.MkdirAll(rest, perm)
}

func (a *aferoFS) Open(name string) (afero.File, error) {
	return a.OpenFile(name, os.O_RDONLY, 0)
}

func (a *aferoFS) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	if _, rest := split(name); rest == "/" && isWrite(flag) {
		return nil, errOp("open", name, fs.ErrPermission)
	}
	f, rest, err := a.resolve("open", name)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return newRoot(a.mounts), nil
	}
	return f.OpenFile(rest, flag, perm)
}

func (a *aferoFS) Remove(name string) error {
	f, rest, err := a.resolveEntry("remove", name)
	if err != nil {
		return err
	}
	return f.Remove(rest)
}

func (a *aferoFS) RemoveAll(name string) error {
	f, rest, err := a.resolveEntry("remove", name)
	if err != nil {
		return err
	}
	return f.RemoveAll(rest)
}

// Rename 只允许在同一挂载内移动，跨挂载移动返回权限错误，客户端会改为复制后删除或提示失败。
func (a *aferoFS) Rename(oldname, newname string) error {
	from, fromRest, err := a.resolveEntry("rename", oldname)
	if err != nil {
		return err
	}
	to, toRest, err := a.resolveEntry("rename", newname)
	if err != nil {
		return err
	}
	if from != to {
		return errOp("rename", newname, fs.ErrPermission)
	}
	return from.Rename(fromRest, toRest)
}

func (a *aferoFS) Stat(name string) (os.FileInfo, error) {
	f, rest, err := a.resolve("stat", name)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return dirInfo{name: "/", mod: time.Now()}, nil
	}
	fi, err := f.Stat(rest)
	if err != nil {
		return nil, err
	}
	if rest == "/" {
		mount, _ := split(name)
		return namedInfo{fi, mount}, nil
	}
	return fi, nil
}

func (a *aferoFS) Chmod(name string, mode os.FileMode) error {
	f, rest, err := a.resolveEntry("chmod", name)
	if err != nil {
		return err
	}
	return f.Chmod(rest, mode)
}

func (a *aferoFS) Chown(name string, uid, gid int) error {
	f, rest, err := a.resolveEntry("chown", name)
	if err != nil {
		return err
	}
	return f.Chown(rest, uid, gid)
}

func (a *aferoFS) Chtimes(name string, atime, mtime time.Time) error {
	f, rest, err := a.resolveEntry("chtimes", name)
	if err != nil {
		return err
	}
	return f.Chtimes(rest, atime, mtime)
}
