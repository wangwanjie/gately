package fileusers

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	"github.com/wangwanjie/gately/internal/fsperm"
	"github.com/wangwanjie/gately/internal/mountfs"
)

// 文件服务的用户目录不能与 Gately 的配置目录重叠（相同、位于其中或包含它）：配置目录里有会话签名密钥，
// 能登录文件服务的用户读到它就能签出管理员会话。保存时用 CheckDirs 拒绝；升级前已保存的配置不经过保存时的校验，
// 各服务在启动与登录时用 Usable 把这样的目录换成一个不存在的路径，其余目录与用户照常使用。

// CheckDirs 校验一个用户的目录（单目录 root 或多个目录 mounts，结构与各文件服务的用户一致），
// 有目录与 Gately 的配置目录重叠时返回错误。
func CheckDirs(root string, mounts []mountfs.Mount) error {
	if len(mounts) == 0 {
		return fsperm.CheckExposable(root)
	}
	for _, mt := range mounts {
		if err := fsperm.CheckExposable(mt.Path); err != nil {
			return err
		}
	}
	return nil
}

// checkUsersDirs 对全部共享用户执行 CheckDirs。
func checkUsersDirs(users []User) error {
	for _, u := range users {
		if err := CheckDirs(u.Root, u.Mounts); err != nil {
			return fmt.Errorf("共享用户 %s: %w", u.Name, err)
		}
	}
	return nil
}

// BlockedDir 是替换被拒绝目录的占位路径：它位于空设备之下，永远不存在也无法创建，
// 各文件服务对它与对不存在的目录一样处理——用户仍可登录，但这个目录里没有任何内容。
var BlockedDir = blockedDir()

func blockedDir() string {
	if runtime.GOOS == "windows" {
		return `\\.\NUL\gately-blocked-dir`
	}
	return filepath.Join(os.DevNull, "gately-blocked-dir")
}

// Usable 返回可以实际提供服务的用户：与 Gately 配置目录重叠的目录换成 BlockedDir，blocked 为被替换的原目录，
// 供调用方写日志提示用户修改。没有需要替换的目录时原样返回。
func Usable(u User) (out User, blocked []string) {
	if len(u.Mounts) == 0 {
		if fsperm.Protected(u.Root) != "" {
			blocked = append(blocked, u.Root)
			u.Root = BlockedDir
		}
		return u, blocked
	}
	u.Mounts = slices.Clone(u.Mounts)
	for i := range u.Mounts {
		if fsperm.Protected(u.Mounts[i].Path) != "" {
			blocked = append(blocked, u.Mounts[i].Path)
			u.Mounts[i].Path = BlockedDir
		}
	}
	return u, blocked
}

// BlockedText 返回写入运行日志的提示：用户 name 的目录 dir 与配置目录重叠，已停止提供。
// 三个文件服务与共享用户共用同一句话，便于按模板翻译。
func BlockedText(name, dir string) string {
	return fmt.Sprintf("用户 %s 的目录 %s 与 Gately 的配置目录重叠，已停止提供，请改为其他目录", name, dir)
}

// Blocked 返回已保存的共享用户中与配置目录重叠的目录的提示，供启动时写入日志；没有时返回空。
func (r *Registry) Blocked() []string {
	r.mu.RLock()
	users := clone(r.users)
	r.mu.RUnlock()
	var out []string
	for _, u := range users {
		_, blocked := Usable(u)
		for _, dir := range blocked {
			out = append(out, fmt.Sprintf("共享用户 %s 的目录 %s 与 Gately 的配置目录重叠，已停止提供，请改为其他目录", u.Name, dir))
		}
	}
	return out
}
