package ftp

import (
	"fmt"

	"github.com/wangwanjie/gately/internal/fileusers"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// checkDirs 拒绝是 Gately 配置目录或位于其中的用户目录，保存与导入时在 Validate 之后调用；
// 包含配置目录的目录可以使用，配置目录由文件访问层自动隐藏。
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
// logs 不为 nil 时为每个被替换的目录写一条警告，并说明哪些目录中的配置目录被自动隐藏。
func usable(u User, logs *logbuf.Buffer) User {
	out, blocked := fileusers.Usable(fileusers.User(u))
	if logs != nil {
		for _, dir := range blocked {
			logs.Add(logbuf.LevelWarn, "", fileusers.BlockedText(u.Name, dir))
		}
		for _, dir := range fileusers.HiddenDirs(out.Root, out.Mounts) {
			logs.Add(logbuf.LevelInfo, "", fileusers.HiddenText(u.Name, dir))
		}
	}
	return User(out)
}
