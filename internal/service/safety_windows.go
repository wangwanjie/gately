//go:build windows

package service

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

// checkInstallSafety 提示把程序放到只有管理员可写的位置：服务以 LocalSystem 运行该程序，
// 程序若在“下载”等普通用户可写的目录，被替换后即可借机获得系统权限。
func checkInstallSafety(sp Spec, out io.Writer) {
	exe := strings.ToLower(filepath.Clean(sp.Exe))
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "SystemRoot"} {
		base := os.Getenv(env)
		if base == "" {
			continue
		}
		if strings.HasPrefix(exe, strings.ToLower(filepath.Clean(base))+`\`) {
			return
		}
	}
	say(out, "警告: 程序不在 Program Files 中，服务以 SYSTEM 身份运行它；请确认 %s 所在目录只有管理员可写，或把程序移到 C:\\Program Files\\Gately 后重新安装", sp.Exe)
}
