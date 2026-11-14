package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/auth"
)

// resetWait 是等待运行中的实例应用重置请求的最长时间，需大于服务端的轮询间隔（2 秒）。
const resetWait = 6 * time.Second

// knownConfigDirs 是各安装方式的配置目录：systemd、OpenWrt 与 Docker 为 /etc/gately，飞牛 fnOS 安装包为 /var/apps/gately/var/config。
// 未指定 -c 且默认目录中没有账号文件时依次尝试，省去用户查找配置目录。
var knownConfigDirs = []string{"/etc/gately", "/var/apps/gately/var/config"}

// runReset 实现 gately reset-password 与 gately reset-totp：在服务器上重置管理员密码或关闭两步验证，
// 用于忘记密码、丢失验证器等无法登录管理界面的情况。能读写配置目录即视为拥有服务器权限。
func runReset(cmd string, args []string, in io.Reader, out io.Writer) int {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(out)
	dir := fs.String("c", defaultConfigDir(), "Gately 的配置目录，需与服务启动参数 -c 一致")
	yes := fs.Bool("y", false, "跳过确认，用于脚本等非交互环境")
	password := new(string)
	if cmd == "reset-password" {
		password = fs.String("p", "", fmt.Sprintf("新密码，至少 10 位且不能是常见弱口令；不指定时重置为默认密码 %s", auth.DefaultPassword))
	}
	fs.Usage = func() {
		if cmd == "reset-password" {
			say(out, "用法: gately reset-password [-c 配置目录] [-p 新密码] [-y]")
			say(out, "重置管理员密码（默认重置为 %s），所有已登录会话随之失效；若已关闭密码登录会同时重新开启。", auth.DefaultPassword)
			fmt.Fprintln(out)
		} else {
			say(out, "用法: gately reset-totp [-c 配置目录] [-y]")
			say(out, "关闭管理员的两步验证（TOTP），用于丢失验证器的情况，所有已登录会话随之失效。")
			fmt.Fprintln(out)
		}
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		say(out, "多余的参数: %s", strings.Join(fs.Args(), " "))
		return 2
	}

	if !flagSet(fs, "c") {
		*dir = probeConfigDir(*dir)
	}
	acc, err := readAccount(*dir)
	if err != nil {
		say(out, "%v", err)
		return 1
	}
	say(out, "配置目录: %s", *dir)

	var req auth.ResetRequest
	if cmd == "reset-password" {
		req, err = auth.NewPasswordReset(*password)
		if err != nil {
			say(out, "%v", err)
			return 2
		}
		if *password == "" {
			say(out, "将把管理员 %s 的密码重置为默认密码 %s。", acc.Username, auth.DefaultPassword)
		} else {
			say(out, "将把管理员 %s 的密码重置为指定的新密码。", acc.Username)
		}
	} else {
		if acc.TOTPSecret == "" {
			say(out, "管理员 %s 未启用两步验证，无需重置。", acc.Username)
			return 0
		}
		req = auth.ResetRequest{DisableTOTP: true, CreatedAt: time.Now()}
		say(out, "将关闭管理员 %s 的两步验证（TOTP），之后可在管理界面重新启用。", acc.Username)
	}
	say(out, "所有已登录的会话将失效。")

	// 二次确认：必须完整输入 yes，防止误操作
	if !*yes {
		fmt.Fprintf(out, "确认继续请输入 yes：")
		line, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && strings.TrimSpace(line) == "" {
			fmt.Fprintln(out)
			say(out, "未读取到确认输入，已取消；非交互环境（如 docker exec 未加 -it）请加 -y 参数。")
			return 1
		}
		if strings.TrimSpace(line) != "yes" {
			say(out, "已取消。")
			return 1
		}
	}

	if err := auth.WriteResetRequest(*dir, req); err != nil {
		say(out, "写入重置请求失败: %v（需要配置目录的写权限，可用 root 或 sudo 运行）", err)
		return 1
	}
	// 请求文件与账号文件保持同一属主，服务以其他用户运行时也能读取并删除
	matchOwner(filepath.Join(*dir, auth.ResetFile), filepath.Join(*dir, "account.json"))

	if waitResetApplied(*dir) {
		say(out, "已生效，所有已登录会话已失效。")
	} else {
		say(out, "已提交，运行中的 Gately 暂未处理（可能未在运行），将在下次启动时生效。")
	}
	if cmd == "reset-password" {
		say(out, "请使用用户名 %s 和新密码登录，登录后尽快修改密码。", acc.Username)
		if acc.TOTPSecret != "" {
			say(out, "该账号已启用两步验证，如验证器丢失请再运行 gately reset-totp。")
		}
	}
	return 0
}

// flagSet 返回命令行是否显式指定了某个参数。
func flagSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

// probeConfigDir 在默认目录没有账号文件时，返回第一个存在账号文件的常见配置目录；都没有时返回 dir。
func probeConfigDir(dir string) string {
	for _, d := range append([]string{dir}, knownConfigDirs...) {
		if _, err := os.Stat(filepath.Join(d, "account.json")); err == nil {
			return d
		}
	}
	return dir
}

// say 输出一行提示。格式串不含换行，以便逐行翻译的输出能按原文模板匹配译文。
func say(out io.Writer, format string, a ...any) {
	fmt.Fprintln(out, fmt.Sprintf(format, a...))
}

// readAccount 读取配置目录中的管理员账号，用于确认目录正确并展示用户名。
func readAccount(dir string) (auth.Account, error) {
	var acc auth.Account
	data, err := os.ReadFile(filepath.Join(dir, "account.json"))
	if errors.Is(err, os.ErrNotExist) {
		return acc, fmt.Errorf("未在 %s 找到管理员账号文件 account.json：请用 -c 指定 Gately 的配置目录；从未启动过的全新安装首次启动后默认账号为 admin / %s", dir, auth.DefaultPassword)
	}
	if err != nil {
		return acc, fmt.Errorf("读取管理员账号失败: %v（需要配置目录的读写权限，可用 root 或 sudo 运行）", err)
	}
	if err := json.Unmarshal(data, &acc); err != nil {
		return acc, fmt.Errorf("解析管理员账号失败: %v", err)
	}
	if acc.Username == "" {
		acc.Username = "admin"
	}
	return acc, nil
}

// waitResetApplied 等待运行中的实例取走重置请求，返回是否已生效。
func waitResetApplied(dir string) bool {
	deadline := time.Now().Add(resetWait)
	for time.Now().Before(deadline) {
		if !auth.ResetPending(dir) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return !auth.ResetPending(dir)
}
