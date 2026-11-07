//go:build linux

package service

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

const supportsUserMode = false

const unitPath = "/etc/systemd/system/" + Name + ".service"

// DefaultConfigDir 返回服务模式下默认的配置目录，与 deploy/systemd 的约定一致。
func DefaultConfigDir(user bool) string {
	return "/etc/gately"
}

func platformRun(action string, sp Spec, out io.Writer) error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return errors.New("未找到 systemd（systemctl），请改用系统自带的服务管理方式，或参考 deploy/systemd/gately.service")
	}
	if action != "status" && os.Geteuid() != 0 {
		return errors.New("需要 root 权限，请使用 sudo 运行")
	}
	switch action {
	case "install":
		if err := os.MkdirAll(sp.ConfigDir, 0o700); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(unitPath, []byte(systemdUnit(sp)), 0o644); err != nil {
			return err
		}
		if err := runCmd(out, "systemctl", "daemon-reload"); err != nil {
			return err
		}
		if sp.NoStart {
			err := runCmd(out, "systemctl", "enable", Name)
			say(out, "已注册开机自启服务 %s", Name)
			return err
		}
		if err := runCmd(out, "systemctl", "enable", "--now", Name); err != nil {
			return err
		}
		say(out, "已安装并启动服务 %s，开机自动运行；配置目录 %s", Name, sp.ConfigDir)
		say(out, "默认账号 admin / gately.cn，忘记密码使用 gately reset-password")
		return nil
	case "uninstall":
		// 服务不存在时 disable 会报错，忽略后继续清理单元文件
		_ = runCmd(out, "systemctl", "disable", "--now", Name)
		if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := runCmd(out, "systemctl", "daemon-reload"); err != nil {
			return err
		}
		say(out, "已移除服务 %s，配置目录保持不变", Name)
		return nil
	case "status":
		// status 在服务未运行时退出码非零，输出本身已说明状态
		_ = runCmd(out, "systemctl", "status", Name, "--no-pager")
		return nil
	default:
		return runCmd(out, "systemctl", action, Name)
	}
}
