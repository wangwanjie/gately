// Package service 把 Gately 注册为系统服务，实现开机自启与异常退出后自动拉起：
// Linux 使用 systemd，macOS 使用 launchd，Windows 使用服务控制管理器（SCM）。
// 入口是命令 gately service install|uninstall|start|stop|restart|status。
package service

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	// Name 是服务名：systemd 单元名、Windows 服务名
	Name = "gately"
	// Label 是 macOS launchd 的标识
	Label = "cn.gately.gately"
)

// Spec 描述要注册的服务。
type Spec struct {
	// Exe 是 Gately 可执行文件的绝对路径，服务直接引用它，在线升级替换该文件即可
	Exe string
	// ConfigDir 是配置目录的绝对路径
	ConfigDir string
	// Listen 非空时作为 -listen 参数传给服务
	Listen string
	// User 表示 macOS 上注册为当前用户的登录项（LaunchAgent），默认注册为开机启动的系统服务（LaunchDaemon）
	User bool
	// NoStart 表示只注册，不立即启动
	NoStart bool
}

// args 返回服务启动时传给 Gately 的参数（不含可执行文件）。
func (s Spec) args() []string {
	args := []string{"-c", s.ConfigDir}
	if s.Listen != "" {
		args = append(args, "-listen", s.Listen)
	}
	return args
}

// Run 实现 gately service 子命令，返回进程退出码。
func Run(args []string, out io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "-help" || args[0] == "--help" {
		usage(out)
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	action := args[0]
	switch action {
	case "install", "uninstall", "start", "stop", "restart", "status":
	default:
		say(out, "未知的服务操作: %s", action)
		usage(out)
		return 2
	}

	fs := flag.NewFlagSet("service "+action, flag.ContinueOnError)
	fs.SetOutput(out)
	sp := Spec{}
	dir := fs.String("c", "", "配置目录，不指定时使用系统默认位置")
	fs.StringVar(&sp.Listen, "listen", "", "管理界面监听地址，覆盖配置文件，例如 :16680")
	if action == "install" {
		fs.BoolVar(&sp.NoStart, "no-start", false, "只注册，不立即启动")
	}
	if supportsUserMode {
		fs.BoolVar(&sp.User, "user", false, "注册为当前用户的登录项，登录后启动，无需管理员权限")
	}
	if err := fs.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		say(out, "多余的参数: %s", strings.Join(fs.Args(), " "))
		return 2
	}

	if action == "install" {
		exe, err := os.Executable()
		if err != nil {
			say(out, "无法确定可执行文件路径: %v", err)
			return 1
		}
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		sp.Exe = exe
		sp.ConfigDir = *dir
		if sp.ConfigDir == "" {
			sp.ConfigDir = DefaultConfigDir(sp.User)
		}
		abs, err := filepath.Abs(sp.ConfigDir)
		if err != nil {
			say(out, "配置目录无效: %v", err)
			return 1
		}
		sp.ConfigDir = abs
	}

	if err := platformRun(action, sp, out); err != nil {
		say(out, "操作失败: %v", err)
		return 1
	}
	return 0
}

func usage(out io.Writer) {
	say(out, "用法: gately service <install|uninstall|start|stop|restart|status> [参数]")
	say(out, "把 Gately 注册为系统服务，实现开机自启与异常退出后自动拉起。")
	say(out, "  install    注册并启动服务，可用 -c 指定配置目录、-listen 指定管理界面监听地址")
	say(out, "  uninstall  停止并移除服务，配置目录保持不变")
	say(out, "  start / stop / restart / status  启动、停止、重启服务与查看状态")
	say(out, "安装与卸载需要管理员权限（Linux、macOS 使用 sudo，Windows 使用管理员身份运行终端）。")
}

func say(out io.Writer, format string, a ...any) {
	fmt.Fprintln(out, fmt.Sprintf(format, a...))
}
