package registry

import "github.com/wangwanjie/gately/internal/module"

// Info 描述一个功能模块。
type Info = module.Info

// Catalog 是 Gately 全部功能模块（含没有编译进本程序的），按显示顺序排列。
// 新增模块时同步这里、对应的 mod_<ID>.go 与仓库根目录的 modules.json，registry 的测试会校验三者一致。
var Catalog = []Info{
	{ID: "portforward", Title: "端口转发"},
	{ID: "protoroute", Title: "协议分流"},
	{ID: "ddns", Title: "动态域名"},
	{ID: "stun", Title: "STUN 穿透"},
	{ID: "tailscale", Title: "Tailscale 组网"},
	{ID: "netbird", Title: "NetBird 组网"},
	{ID: "frp", Title: "frp 内网穿透"},
	{ID: "cloudflared", Title: "Cloudflare 隧道"},
	{ID: "ssl", Title: "SSL 证书"},
	{ID: "waf", Title: "WAF 防火墙"},
	{ID: "webservice", Title: "Web 服务"},
	{ID: "guard", Title: "入侵防护"},
	{ID: "knock", Title: "端口敲门"},
	{ID: "wol", Title: "网络唤醒"},
	{ID: "webterminal", Title: "Web 终端"},
	{ID: "cron", Title: "计划任务"},
	{ID: "docker", Title: "Docker 管理"},
	{ID: "webdav", Title: "WebDAV 服务"},
	{ID: "ftp", Title: "FTP 服务"},
	{ID: "smb", Title: "SMB 服务"},
	{ID: "filebrowser", Title: "File Browser"},
	{ID: "storage", Title: "存储挂载"},
	{ID: "notify", Title: "通知中心"},
	{ID: "perf", Title: "性能监控"},
	{ID: "nettools", Title: "网络诊断"},
	{ID: "backup", Title: "配置备份"},
}

// order 返回模块在 Catalog 中的位置，用作显示顺序。
func order(id string) int {
	for i, c := range Catalog {
		if c.ID == id {
			return i
		}
	}
	panic("registry: unknown module " + id)
}
