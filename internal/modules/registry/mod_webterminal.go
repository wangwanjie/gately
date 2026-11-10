//go:build !no_webterminal

package registry

import (
	"context"

	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/ssl"
	"github.com/wangwanjie/gately/internal/modules/webterminal"
)

func init() {
	var term *webterminal.Module
	register(part{
		id:    webterminal.ID,
		order: order(webterminal.ID),
		prepare: func(w *wiring) {
			term = webterminal.New()
			// 证书部署经 Web 终端的 SSH 连接把证书推送到远程主机
			w.sshPush = func(ctx context.Context, conn, dir string, files []ssl.RemoteFile, command string) (string, error) {
				pf := make([]webterminal.PushFile, 0, len(files))
				for _, f := range files {
					pf = append(pf, webterminal.PushFile{Name: f.Name, Data: f.Data, Mode: f.Mode})
				}
				return term.Push(ctx, conn, dir, pf, command)
			}
			w.sshExec = term.Exec
			w.sshTargets = func() []ssl.SSHTarget {
				out := []ssl.SSHTarget{}
				for _, t := range term.SSHTargets() {
					out = append(out, ssl.SSHTarget{Key: t.Key, Name: t.Name, Host: t.Host, Trusted: t.Trusted})
				}
				return out
			}
		},
		create: func(*wiring) module.Module { return term },
	})
}
