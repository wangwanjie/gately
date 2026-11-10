//go:build !no_wol

package registry

import (
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/wol"
)

func init() {
	register(part{id: wol.ID, order: order(wol.ID), create: func(w *wiring) module.Module {
		// 远程关机复用 Web 终端的 SSH 连接；Web 终端未编译时两者都为 nil，关机不可用
		deps := wol.Deps{SSHExec: w.sshExec}
		if w.sshTargets != nil {
			deps.SSHTargets = func() []wol.SSHTarget {
				out := []wol.SSHTarget{}
				for _, t := range w.sshTargets() {
					out = append(out, wol.SSHTarget{Key: t.Key, Name: t.Name, Host: t.Host, Trusted: t.Trusted})
				}
				return out
			}
		}
		return wol.New(deps)
	}})
}
