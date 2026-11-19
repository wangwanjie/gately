package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/app"
)

// rollbackListMax 为列出的最近保存条数
const rollbackListMax = 15

// rollbackWait 为等待运行中的实例取走回滚请求的时间，测试中缩短
var rollbackWait = resetWait

// runRollback 实现 gately rollback：管理界面因配置改坏而无法访问时，在服务器上撤销最近的保存或回滚待确认的变更。
// Gately 在运行时交给它处理（恢复后重启），没有运行时直接改写配置文件。
func runRollback(args []string, in io.Reader, out io.Writer) int {
	// 选择序号与确认共用一个缓冲读取器：各建一个时，管道输入会被第一个整块读走
	br := bufio.NewReader(in)
	fs := flag.NewFlagSet("rollback", flag.ContinueOnError)
	fs.SetOutput(out)
	dir := fs.String("c", defaultConfigDir(), "Gately 的配置目录，需与服务启动参数 -c 一致")
	list := fs.Bool("list", false, "只列出最近的配置保存与待确认的变更，不做修改")
	last := fs.Bool("last", false, "撤销最近一次保存")
	pending := fs.Bool("pending", false, "立即回滚待确认的配置变更")
	cfgName := fs.String("config", "", "要回滚的配置名（如 portforward、webservice、gately），与 -version 一起使用")
	version := fs.String("version", "", "历史版本 ID（见 -list），与 -config 一起使用")
	yes := fs.Bool("y", false, "跳过确认，用于脚本等非交互环境")
	fs.Usage = func() {
		say(out, "用法: gately rollback [-c 配置目录] [-list | -last | -pending | -config 名称 -version 版本] [-y]")
		say(out, "管理界面因配置改坏而无法访问时，撤销最近的配置保存或回滚待确认的配置变更；不带参数时列出最近的保存并按序号选择。")
		fmt.Fprintln(out)
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
	if (*cfgName == "") != (*version == "") {
		say(out, "-config 与 -version 需要同时指定")
		return 2
	}
	if !flagSet(fs, "c") {
		*dir = probeConfigDir(*dir)
	}
	// 先确认目录确实是 Gately 的配置目录
	if _, err := readAccount(*dir); err != nil {
		say(out, "%v", err)
		return 1
	}
	store, err := app.OpenConfigStore(*dir)
	if err != nil {
		say(out, "打开配置目录失败: %v", err)
		return 1
	}
	say(out, "配置目录: %s", *dir)
	pend, err := app.ReadPendingConfirm(*dir)
	if err != nil {
		say(out, "读取待确认的配置变更失败: %v", err)
	}
	changes := app.ListConfigChanges(store, rollbackListMax)
	printRollbackState(out, pend, changes)
	if *list {
		return 0
	}

	var req app.RollbackRequest
	var desc string
	switch {
	case *pending:
		if pend == nil {
			say(out, "没有待确认的配置变更。")
			return 1
		}
		req, desc = app.RollbackRequest{Pending: true}, pendingDesc(pend)
	case *last:
		if len(changes) == 0 {
			say(out, "没有可撤销的配置保存。")
			return 1
		}
		req, desc = changeRequest(changes[0])
	case *cfgName != "":
		if _, err := store.ReadVersion(*cfgName, *version); err != nil {
			say(out, "找不到配置 %s 的历史版本 %s: %v", *cfgName, *version, err)
			return 1
		}
		req = app.RollbackRequest{Config: *cfgName, Version: *version}
		desc = fmt.Sprintf("将把配置 %s 恢复为历史版本 %s。", *cfgName, *version)
	default:
		if len(changes) == 0 && pend == nil {
			say(out, "没有可撤销的配置保存。")
			return 0
		}
		var ok bool
		req, desc, ok = chooseRollback(br, out, pend, changes)
		if !ok {
			return 1
		}
	}
	say(out, "%s", desc)
	say(out, "Gately 正在运行时会随之重启。")
	if !*yes {
		fmt.Fprintf(out, "确认继续请输入 yes：")
		line, err := br.ReadString('\n')
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

	req.CreatedAt = time.Now()
	if err := app.WriteRollbackRequest(*dir, req); err != nil {
		say(out, "写入回滚请求失败: %v（需要配置目录的写权限，可用 root 或 sudo 运行）", err)
		return 1
	}
	matchOwner(filepath.Join(*dir, app.RollbackRequestFile), filepath.Join(*dir, "account.json"))
	if waitRollbackTaken(*dir) {
		say(out, "已交给运行中的 Gately 处理，正在重启，稍后即可重新访问管理界面。")
		return 0
	}
	// 没有运行中的实例取走请求：收回后直接改写配置文件
	taken, err := app.TakeRollbackRequest(*dir)
	if err != nil {
		say(out, "收回回滚请求失败: %v", err)
		return 1
	}
	if taken == nil {
		// 刚好在收回前被运行中的实例取走
		say(out, "已交给运行中的 Gately 处理，正在重启，稍后即可重新访问管理界面。")
		return 0
	}
	msg, err := app.ApplyRollbackOffline(store, *taken)
	if err != nil {
		say(out, "回滚失败: %v", err)
		return 1
	}
	say(out, "%s", msg)
	say(out, "Gately 未在运行，已直接改写配置文件，下次启动时生效；改写前的内容保存在配置历史中。")
	return 0
}

// printRollbackState 列出待确认的变更与最近的保存。
func printRollbackState(out io.Writer, pend *app.PendingSummary, changes []app.ConfigChange) {
	if pend != nil {
		say(out, "待确认的配置变更（序号 P）：%s", pendingDesc(pend))
	}
	if len(changes) == 0 {
		say(out, "配置历史中没有记录。")
		return
	}
	say(out, "最近的配置保存（撤销某次保存即把该配置恢复为那次保存之前的内容）：")
	for i, c := range changes {
		say(out, "%3d. %s  %-12s  %s", i+1, c.Time.Local().Format("2006-01-02 15:04:05"), c.Config, c.Version)
	}
}

func pendingDesc(p *app.PendingSummary) string {
	names := strings.Join(p.Configs, "、")
	switch {
	case p.Rollback:
		return fmt.Sprintf("涉及 %s，已决定回滚，等待重启", names)
	case p.WaitRestart:
		return fmt.Sprintf("涉及 %s，等待重启后开始计时", names)
	}
	return fmt.Sprintf("涉及 %s，未确认时将在 %s 自动回滚", names, p.Deadline.Local().Format("15:04:05"))
}

func changeRequest(c app.ConfigChange) (app.RollbackRequest, string) {
	return app.RollbackRequest{Config: c.Config, Version: c.Version},
		fmt.Sprintf("将撤销 %s 对配置 %s 的保存：恢复为那次保存之前的内容（历史版本 %s）。", c.Time.Local().Format("2006-01-02 15:04:05"), c.Config, c.Version)
}

// chooseRollback 交互选择要撤销的保存。
func chooseRollback(br *bufio.Reader, out io.Writer, pend *app.PendingSummary, changes []app.ConfigChange) (app.RollbackRequest, string, bool) {
	fmt.Fprintf(out, "输入要撤销的序号（P 为待确认的变更），直接回车取消：")
	line, err := br.ReadString('\n')
	choice := strings.TrimSpace(line)
	if choice == "" {
		if err != nil {
			fmt.Fprintln(out)
			say(out, "未读取到输入，已取消；非交互环境请使用 -last、-pending 或 -config/-version 参数。")
		} else {
			say(out, "已取消。")
		}
		return app.RollbackRequest{}, "", false
	}
	if strings.EqualFold(choice, "p") {
		if pend == nil {
			say(out, "没有待确认的配置变更。")
			return app.RollbackRequest{}, "", false
		}
		return app.RollbackRequest{Pending: true}, "将回滚待确认的配置变更：" + pendingDesc(pend) + "。", true
	}
	n, err := strconv.Atoi(choice)
	if err != nil || n < 1 || n > len(changes) {
		say(out, "无效的序号: %s", choice)
		return app.RollbackRequest{}, "", false
	}
	req, desc := changeRequest(changes[n-1])
	return req, desc, true
}

// waitRollbackTaken 等待运行中的实例取走回滚请求。
func waitRollbackTaken(dir string) bool {
	deadline := time.Now().Add(rollbackWait)
	for time.Now().Before(deadline) {
		if !app.RollbackRequestPending(dir) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return !app.RollbackRequestPending(dir)
}
