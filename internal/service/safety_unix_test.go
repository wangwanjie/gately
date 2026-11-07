//go:build linux || darwin

package service

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 普通用户运行测试时临时目录属于该用户，正是“可被 root 以外的用户修改”的情形
func TestCheckInstallSafetyWarnsUserWritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("以 root 运行时临时目录属于 root，不构成用户可写")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "gately")
	if err := os.WriteFile(exe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	checkInstallSafety(Spec{Exe: exe, ConfigDir: filepath.Join(dir, "conf")}, &out)
	if !strings.Contains(out.String(), exe) || !strings.Contains(out.String(), "/usr/local/bin") {
		t.Errorf("用户可写的程序没有警告:\n%s", out.String())
	}
	// 每个路径只警告一次：程序与配置目录共用的上级目录不重复
	if n := strings.Count(out.String(), dir+" "); n != 1 {
		t.Errorf("上级目录 %s 警告了 %d 次:\n%s", dir, n, out.String())
	}
	// 注册为当前用户登录项时以当前用户运行，不存在提权
	out.Reset()
	checkInstallSafety(Spec{Exe: exe, ConfigDir: dir, User: true}, &out)
	if out.Len() != 0 {
		t.Errorf("-user 模式不应警告:\n%s", out.String())
	}
}

func TestUnsafeForRootSystemPaths(t *testing.T) {
	// 根目录属于 root 且其他用户不可写；不存在的路径不判断
	if unsafeForRoot("/") {
		t.Error("/ 被判为用户可写")
	}
	if unsafeForRoot("/nonexistent-gately-path") {
		t.Error("不存在的路径被判为用户可写")
	}
	// /tmp 其他用户可写但带粘滞位，root 拥有的文件不能被他人替换
	if st, err := os.Stat("/tmp"); err == nil && st.Mode()&os.ModeSticky != 0 {
		if real, err := filepath.EvalSymlinks("/tmp"); err == nil && unsafeForRoot(real) {
			t.Error("带粘滞位的 /tmp 被判为用户可写")
		}
	}
}
