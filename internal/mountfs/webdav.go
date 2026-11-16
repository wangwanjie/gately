package mountfs

import (
	"context"
	"io/fs"
	"os"
	"time"

	xwebdav "golang.org/x/net/webdav"
)

// ReadOnlyDAV 包装 webdav.FileSystem，拒绝所有写操作。
type ReadOnlyDAV struct{ xwebdav.FileSystem }

func (ReadOnlyDAV) Mkdir(context.Context, string, os.FileMode) error { return os.ErrPermission }
func (ReadOnlyDAV) RemoveAll(context.Context, string) error          { return os.ErrPermission }
func (ReadOnlyDAV) Rename(context.Context, string, string) error     { return os.ErrPermission }
func (r ReadOnlyDAV) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (xwebdav.File, error) {
	if isWrite(flag) {
		return nil, os.ErrPermission
	}
	return r.FileSystem.OpenFile(ctx, name, flag, perm)
}

// davFS 是供 WebDAV 使用的组合文件系统。
type davFS struct {
	mounts []Mount
	fss    map[string]xwebdav.FileSystem
}

// WebDAV 返回由 mounts 组成的 webdav.FileSystem，挂载列表需已通过 Validate。
func WebDAV(mounts []Mount) xwebdav.FileSystem {
	d := &davFS{mounts: mounts, fss: map[string]xwebdav.FileSystem{}}
	for _, mt := range mounts {
		// 经 os.Root 访问，指向挂载目录之外的符号链接无法被跟随
		var f xwebdav.FileSystem = RootDAV(mt.Path)
		if mt.ReadOnly {
			f = ReadOnlyDAV{f}
		}
		d.fss[mt.Name] = f
	}
	return d
}

func (d *davFS) resolve(op, name string) (xwebdav.FileSystem, string, error) {
	mount, rest := split(name)
	if mount == "" {
		return nil, "/", nil
	}
	f, ok := d.fss[mount]
	if !ok {
		return nil, "", errOp(op, name, fs.ErrNotExist)
	}
	return f, rest, nil
}

// resolveEntry 用于修改类操作：根目录与挂载目录本身都不允许修改。
func (d *davFS) resolveEntry(op, name string) (xwebdav.FileSystem, string, error) {
	// 根目录下只有挂载目录，在根目录新建或修改条目一律视为无权限
	if _, rest := split(name); rest == "/" {
		return nil, "", errOp(op, name, fs.ErrPermission)
	}
	f, rest, err := d.resolve(op, name)
	if err != nil {
		return nil, "", err
	}
	return f, rest, nil
}

func (d *davFS) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	f, rest, err := d.resolveEntry("mkdir", name)
	if err != nil {
		return err
	}
	return f.Mkdir(ctx, rest, perm)
}

func (d *davFS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (xwebdav.File, error) {
	if _, rest := split(name); rest == "/" && isWrite(flag) {
		return nil, errOp("open", name, fs.ErrPermission)
	}
	f, rest, err := d.resolve("open", name)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return newRoot(d.mounts), nil
	}
	file, err := f.OpenFile(ctx, rest, flag, perm)
	if err != nil || rest != "/" {
		return file, err
	}
	// 挂载目录本身的名称以挂载名称为准，PROPFIND 的 displayname 才与根目录列表一致
	mount, _ := split(name)
	return namedFile{file, mount}, nil
}

func (d *davFS) RemoveAll(ctx context.Context, name string) error {
	f, rest, err := d.resolveEntry("remove", name)
	if err != nil {
		return err
	}
	return f.RemoveAll(ctx, rest)
}

// Rename 只允许在同一挂载内移动，跨挂载移动返回权限错误。
func (d *davFS) Rename(ctx context.Context, oldName, newName string) error {
	from, fromRest, err := d.resolveEntry("rename", oldName)
	if err != nil {
		return err
	}
	to, toRest, err := d.resolveEntry("rename", newName)
	if err != nil {
		return err
	}
	if from != to {
		return errOp("rename", newName, fs.ErrPermission)
	}
	return from.Rename(ctx, fromRest, toRest)
}

func mountOf(name string) string {
	m, _ := split(name)
	return m
}

func (d *davFS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	f, rest, err := d.resolve("stat", name)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return dirInfo{name: "/", mod: time.Now()}, nil
	}
	fi, err := f.Stat(ctx, rest)
	if err != nil || rest != "/" {
		return fi, err
	}
	return namedInfo{fi, mountOf(name)}, nil
}

// namedFile 让挂载目录句柄的 Stat 返回挂载名称。
type namedFile struct {
	xwebdav.File
	name string
}

func (n namedFile) Stat() (fs.FileInfo, error) {
	fi, err := n.File.Stat()
	if err != nil {
		return nil, err
	}
	return namedInfo{fi, n.name}, nil
}
