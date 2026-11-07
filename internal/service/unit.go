package service

import (
	"fmt"
	"strings"
)

// systemdQuote 转义参数供 systemd 单元的 ExecStart 使用：ExecStart 会展开 % 说明符与 $变量，
// 分别写成 %% 与 $$；含空白、引号或反斜杠时再加双引号。控制字符由 validateSpec 提前拒绝。
func systemdQuote(s string) string {
	s = strings.NewReplacer("%", "%%", "$", "$$").Replace(s)
	if s == "" || strings.ContainsAny(s, " \t\"'\\") {
		r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
		return `"` + r.Replace(s) + `"`
	}
	return s
}

// validateSpec 拒绝含换行等控制字符的路径与参数：它们会写进单元文件、plist 或服务命令行，
// 换行可以在 systemd 单元里另起一行注入任意指令。
func validateSpec(sp Spec) error {
	for _, v := range append([]string{sp.Exe}, sp.args()...) {
		for _, c := range v {
			if c < 0x20 || c == 0x7f {
				return fmt.Errorf("路径或参数含控制字符: %q", v)
			}
		}
	}
	return nil
}

// systemdUnit 生成 systemd 单元文件内容，与 deploy/systemd/gately.service 保持一致。
func systemdUnit(sp Spec) string {
	exec := []string{systemdQuote(sp.Exe)}
	for _, a := range sp.args() {
		exec = append(exec, systemdQuote(a))
	}
	var b strings.Builder
	b.WriteString("[Unit]\n")
	b.WriteString("Description=Gately network gateway toolkit\n")
	b.WriteString("Documentation=https://gately.cn\n")
	b.WriteString("After=network-online.target\n")
	b.WriteString("Wants=network-online.target\n\n")
	b.WriteString("[Service]\n")
	b.WriteString("Type=simple\n")
	b.WriteString("ExecStart=" + strings.Join(exec, " ") + "\n")
	// 管理界面“重启”会以相同参数重新执行自身；失败时退出，由 systemd 拉起
	b.WriteString("Restart=on-failure\n")
	b.WriteString("RestartSec=3\n")
	// 监听 80/443 等低位端口
	b.WriteString("AmbientCapabilities=CAP_NET_BIND_SERVICE CAP_NET_RAW CAP_NET_ADMIN\n")
	b.WriteString("LimitNOFILE=1048576\n\n")
	b.WriteString("[Install]\n")
	b.WriteString("WantedBy=multi-user.target\n")
	return b.String()
}

// xmlEscape 转义 plist 中的文本。
func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

// launchdPlist 生成 launchd 的 plist 内容。
// KeepAlive 仅在异常退出时拉起：管理界面的“重启”在重新执行失败时以非零码退出，会被重新拉起，
// 而 stop（SIGTERM）正常退出后不会被重新拉起。
func launchdPlist(sp Spec, logPath string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString(`<plist version="1.0">` + "\n<dict>\n")
	b.WriteString("\t<key>Label</key>\n\t<string>" + xmlEscape(Label) + "</string>\n")
	b.WriteString("\t<key>ProgramArguments</key>\n\t<array>\n")
	b.WriteString("\t\t<string>" + xmlEscape(sp.Exe) + "</string>\n")
	for _, a := range sp.args() {
		b.WriteString("\t\t<string>" + xmlEscape(a) + "</string>\n")
	}
	b.WriteString("\t</array>\n")
	b.WriteString("\t<key>RunAtLoad</key>\n\t<true/>\n")
	b.WriteString("\t<key>KeepAlive</key>\n\t<dict>\n\t\t<key>SuccessfulExit</key>\n\t\t<false/>\n\t</dict>\n")
	b.WriteString("\t<key>ThrottleInterval</key>\n\t<integer>3</integer>\n")
	b.WriteString("\t<key>StandardOutPath</key>\n\t<string>" + xmlEscape(logPath) + "</string>\n")
	b.WriteString("\t<key>StandardErrorPath</key>\n\t<string>" + xmlEscape(logPath) + "</string>\n")
	b.WriteString("</dict>\n</plist>\n")
	return b.String()
}
