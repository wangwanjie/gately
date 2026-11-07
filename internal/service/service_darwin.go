//go:build darwin

package service

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const supportsUserMode = true

// DefaultConfigDir 返回服务模式下默认的配置目录。
func DefaultConfigDir(user bool) string {
	if user {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, "Library", "Application Support", "Gately")
		}
	}
	return "/usr/local/etc/gately"
}

// loaded 判断 launchd 中是否已加载该任务。
func loaded(target string) bool {
	return exec.Command("launchctl", "print", target).Run() == nil
}

// waitUnloaded 等待任务从 launchd 中卸载，最多等待 timeout。
func waitUnloaded(target string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for loaded(target) && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
	}
}

// launchdPaths 返回 plist 路径、日志路径与 launchctl 域（system 或 gui/<uid>）。
func launchdPaths(user bool) (plist, logPath, domain string, err error) {
	if !user {
		return "/Library/LaunchDaemons/" + Label + ".plist", "/var/log/gately.log", "system", nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", Label+".plist"),
		filepath.Join(home, "Library", "Logs", "Gately.log"),
		fmt.Sprintf("gui/%d", os.Getuid()), nil
}

func platformRun(action string, sp Spec, out io.Writer) error {
	// 卸载、启动、停止时没有 -user 参数无法判断注册方式，按 plist 是否存在自动识别
	user := sp.User
	if action != "install" && !user {
		if p, _, _, err := launchdPaths(false); err == nil {
			if _, err := os.Stat(p); err != nil {
				user = true
			}
		}
	}
	plist, logPath, domain, err := launchdPaths(user)
	if err != nil {
		return err
	}
	if !user && action != "status" && os.Geteuid() != 0 {
		return errors.New("需要 root 权限，请使用 sudo 运行；也可以加 -user 注册为当前用户的登录项")
	}
	target := domain + "/" + Label

	switch action {
	case "install":
		if err := os.MkdirAll(sp.ConfigDir, 0o700); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(plist), 0o755); err != nil {
			return err
		}
		// 已注册时先卸载，使新的 plist 生效；bootout 异步结束进程，等任务真正卸载后再 bootstrap，
		// 否则 bootstrap 常报 "Bootstrap failed: 5: Input/output error"
		if loaded(target) {
			_ = exec.Command("launchctl", "bootout", target).Run()
			waitUnloaded(target, 15*time.Second)
		}
		if err := os.WriteFile(plist, []byte(launchdPlist(sp, logPath)), 0o644); err != nil {
			return err
		}
		if sp.NoStart {
			say(out, "已注册开机自启服务 %s", Label)
			return nil
		}
		if err := runCmd(out, "launchctl", "bootstrap", domain, plist); err != nil {
			return err
		}
		say(out, "已安装并启动服务 %s，开机自动运行；配置目录 %s", Label, sp.ConfigDir)
		say(out, "日志文件: %s", logPath)
		say(out, "默认账号 admin / gately.cn，忘记密码使用 gately reset-password")
		return nil
	case "uninstall":
		_ = exec.Command("launchctl", "bootout", target).Run()
		if err := os.Remove(plist); err != nil && !os.IsNotExist(err) {
			return err
		}
		say(out, "已移除服务 %s，配置目录保持不变", Label)
		return nil
	case "start":
		if _, err := os.Stat(plist); err != nil {
			return errors.New("服务尚未安装，请先运行 gately service install")
		}
		// 任务已加载（运行中，或已加载但进程已退出）时 bootstrap 会报错，改用 kickstart 启动
		if loaded(target) {
			return runCmd(out, "launchctl", "kickstart", target)
		}
		return runCmd(out, "launchctl", "bootstrap", domain, plist)
	case "stop":
		// bootout 会结束进程并卸载任务，plist 保留，开机时仍会自动运行
		return runCmd(out, "launchctl", "bootout", target)
	case "restart":
		if !loaded(target) {
			return errors.New("服务未在运行，请先运行 gately service start")
		}
		return runCmd(out, "launchctl", "kickstart", "-k", target)
	case "status":
		raw, err := exec.Command("launchctl", "print", target).CombinedOutput()
		if err != nil {
			say(out, "服务未运行")
			return nil
		}
		// 输出里 state 会在子结构中重复出现，每个字段只取第一次
		seen := map[string]bool{}
		sc := bufio.NewScanner(bytes.NewReader(raw))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			for _, key := range []string{"state =", "pid =", "last exit code ="} {
				if strings.HasPrefix(line, key) && !seen[key] {
					seen[key] = true
					fmt.Fprintln(out, line)
				}
			}
		}
		return nil
	}
	return nil
}
