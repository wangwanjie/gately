// Package registry 集中创建全部功能模块，供主程序、文档导出与测试共用。
package registry

import (
	"context"

	"github.com/wangwanjie/gately/internal/module"
	backupmod "github.com/wangwanjie/gately/internal/modules/backup"
	"github.com/wangwanjie/gately/internal/modules/cloudflared"
	"github.com/wangwanjie/gately/internal/modules/cron"
	"github.com/wangwanjie/gately/internal/modules/ddns"
	"github.com/wangwanjie/gately/internal/modules/docker"
	"github.com/wangwanjie/gately/internal/modules/filebrowser"
	"github.com/wangwanjie/gately/internal/modules/frp"
	"github.com/wangwanjie/gately/internal/modules/ftp"
	"github.com/wangwanjie/gately/internal/modules/guard"
	"github.com/wangwanjie/gately/internal/modules/nettools"
	"github.com/wangwanjie/gately/internal/modules/notify"
	"github.com/wangwanjie/gately/internal/modules/portforward"
	"github.com/wangwanjie/gately/internal/modules/protoroute"
	"github.com/wangwanjie/gately/internal/modules/smb"
	"github.com/wangwanjie/gately/internal/modules/ssl"
	"github.com/wangwanjie/gately/internal/modules/storage"
	"github.com/wangwanjie/gately/internal/modules/stun"
	"github.com/wangwanjie/gately/internal/modules/tailscale"
	"github.com/wangwanjie/gately/internal/modules/waf"
	"github.com/wangwanjie/gately/internal/modules/webdav"
	"github.com/wangwanjie/gately/internal/modules/webservice"
	"github.com/wangwanjie/gately/internal/modules/webterminal"
	"github.com/wangwanjie/gately/internal/modules/wol"
)

// New 创建全部模块。Web 服务使用证书模块按 SNI 提供 HTTPS 证书，并按站点引用 WAF 实例，
// 因此一并返回证书模块供管理界面 HTTPS 复用。
func New() (mods []module.Module, certs *ssl.Module) {
	certs = ssl.New()
	firewall := waf.New()
	files := storage.New()
	term := webterminal.New()
	dock := docker.New()
	// 证书部署复用 Web 终端的 SSH 连接与 Docker 模块的客户端，经适配函数注入，避免模块之间直接依赖
	certs.SetDeployers(ssl.Deployers{
		Push: func(ctx context.Context, conn, dir string, files []ssl.RemoteFile, command string) (string, error) {
			pf := make([]webterminal.PushFile, 0, len(files))
			for _, f := range files {
				pf = append(pf, webterminal.PushFile{Name: f.Name, Data: f.Data, Mode: f.Mode})
			}
			return term.Push(ctx, conn, dir, pf, command)
		},
		Restart: dock.RestartContainers,
		SSHTargets: func() []ssl.SSHTarget {
			out := []ssl.SSHTarget{}
			for _, t := range term.SSHTargets() {
				out = append(out, ssl.SSHTarget{Key: t.Key, Name: t.Name, Host: t.Host, Trusted: t.Trusted})
			}
			return out
		},
		Containers: dock.ContainerNames,
	})
	mods = []module.Module{
		portforward.New(),
		protoroute.New(),
		ddns.New(),
		stun.New(),
		tailscale.New(),
		frp.New(),
		cloudflared.New(),
		certs,
		firewall,
		webservice.New(certs, firewall),
		// 入侵防护读取容器日志时复用 Docker 模块的客户端
		guard.New(guard.Deps{FollowDocker: dock.FollowLogs}),
		wol.New(),
		term,
		cron.New(),
		dock,
		webdav.New(certs),
		ftp.New(certs),
		smb.New(),
		filebrowser.New(),
		files,
		notify.New(),
		nettools.New(),
		backupmod.New(files),
	}
	return mods, certs
}

// Requires 返回模块间的依赖关系：键为模块 ID，值为它依赖的模块 ID。
// 与 New 中的装配关系一一对应；依赖方开启期间，被依赖的模块不能关闭。
func Requires() map[string][]string {
	return map[string][]string{
		webservice.ID: {ssl.ID, waf.ID},
		webdav.ID:     {ssl.ID},
		ftp.ID:        {ssl.ID},
		backupmod.ID:  {storage.ID},
	}
}
