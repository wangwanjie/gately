//go:build !no_guard

package registry

import (
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/guard"
)

func init() {
	register(part{id: guard.ID, order: order(guard.ID), create: func(w *wiring) module.Module {
		// Docker 模块没有编译进来时 FollowDocker 为 nil，入侵防护不提供容器日志来源
		return guard.New(guard.Deps{FollowDocker: w.followDocker})
	}})
}
