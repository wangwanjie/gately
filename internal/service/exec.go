package service

import (
	"io"
	"os/exec"
)

// runCmd 执行外部命令，输出写入 out。
func runCmd(out io.Writer, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}
