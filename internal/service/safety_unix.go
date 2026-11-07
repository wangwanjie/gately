//go:build linux || darwin

package service

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

// checkInstallSafety 在注册以 root 身份运行的系统服务前检查：服务执行的程序、配置目录及它们的各级上级目录
// 若可被 root 以外的用户修改（例如程序还在 ~/Downloads、Intel Mac 上 Homebrew 接管的 /usr/local），
// 该用户就能替换程序或配置借机获得 root 权限，这里逐个给出警告（不阻止安装）。
func checkInstallSafety(sp Spec, out io.Writer) {
	if sp.User {
		return
	}
	seen := map[string]bool{}
	warned := false
	for _, start := range []string{sp.Exe, sp.ConfigDir} {
		for p := start; ; p = filepath.Dir(p) {
			if !seen[p] {
				seen[p] = true
				if unsafeForRoot(p) {
					say(out, "警告: %s 可被 root 以外的用户修改，而服务以 root 身份运行，存在被替换后提权的风险", p)
					warned = true
				}
			}
			if parent := filepath.Dir(p); parent == p {
				break
			}
		}
	}
	if warned {
		say(out, "建议把程序放到 /usr/local/bin 等只有 root 可写的位置、配置目录使用默认位置后重新安装")
	}
}

// unsafeForRoot 判断路径是否可被 root 以外的用户修改：属主不是 root、其他用户可写（带粘滞位的目录除外），
// 或属组可写且属组不是 root（macOS 上的 admin 组成员本就可以 sudo，不算）。路径不存在时不判断。
func unsafeForRoot(p string) bool {
	st, err := os.Lstat(p)
	if err != nil {
		return false
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	if sys.Uid != 0 {
		return true
	}
	mode := st.Mode()
	if mode&0o002 != 0 && !(mode.IsDir() && mode&os.ModeSticky != 0) {
		return true
	}
	if mode&0o020 != 0 && sys.Gid != 0 && !(runtime.GOOS == "darwin" && sys.Gid == 80) {
		return true
	}
	return false
}
