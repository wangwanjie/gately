package ftp

import (
	"fmt"

	"github.com/wangwanjie/gately/internal/fileusers"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// checkDirs 拒绝与 Gately 配置目录重叠（相同、位于其中或包含它）的用户目录，保存与导入时在 Validate 之后调用。
// 不放进 Validate：升级前保存的配置里可能已有这样的目录，那时只停止提供该目录（见 usable），服务与其余用户照常使用。
func (s *Settings) checkDirs() error {
	for _, u := range s.Users {
		if err := fileusers.CheckDirs(u.Root, u.Mounts); err != nil {
			return fmt.Errorf("用户 %s: %w", u.Name, err)
		}
	}
	return nil
}

// usable 返回实际提供服务时使用的用户：与配置目录重叠的目录换成不存在的路径（见 fileusers.Usable）。
// logs 不为 nil 时为每个被替换的目录写一条警告。
func usable(u User, logs *logbuf.Buffer) User {
	out, blocked := fileusers.Usable(fileusers.User(u))
	if logs != nil {
		for _, dir := range blocked {
			logs.Add(logbuf.LevelWarn, "", fileusers.BlockedText(u.Name, dir))
		}
	}
	return User(out)
}
