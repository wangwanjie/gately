//go:build unix

package webterminal

import (
	"github.com/wangwanjie/gately/internal/demo"
	"os"
	"os/exec"

	"github.com/creack/pty"
)

// localSession 是基于伪终端的本机 Shell。
type localSession struct {
	cmd *exec.Cmd
	pty *os.File
}

func (l *localSession) Read(p []byte) (int, error)  { return l.pty.Read(p) }
func (l *localSession) Write(p []byte) (int, error) { return l.pty.Write(p) }
func (l *localSession) Resize(c, r int) error {
	return pty.Setsize(l.pty, &pty.Winsize{Cols: uint16(c), Rows: uint16(r)})
}
func (l *localSession) Close() error {
	_ = l.pty.Close()
	if l.cmd.Process != nil {
		_ = l.cmd.Process.Kill()
	}
	_ = l.cmd.Wait()
	return nil
}

func startLocal(cfg LocalConfig, cols, rows int) (session, error) {
	// 演示模式下不执行任何外部命令
	if err := demo.Check(); err != nil {
		return nil, err
	}
	shell := cfg.Shell
	if shell == "" {
		shell = os.Getenv("SHELL")
	}
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell)
	cmd.Dir = cfg.WorkDir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	// 连接配置中的附加环境变量放在最后，可覆盖默认值
	for k, v := range cfg.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, err
	}
	return &localSession{cmd: cmd, pty: f}, nil
}
