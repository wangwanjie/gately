//go:build !no_docker

package registry

import (
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/docker"
)

func init() {
	// Docker 管理升级 Gately 自身时，在辅助容器中运行这个子命令重建 Gately 容器
	subcommands[docker.SelfUpgradeCommand] = docker.RunSelfUpgrade
	var dock *docker.Module
	register(part{
		id:    docker.ID,
		order: order(docker.ID),
		prepare: func(w *wiring) {
			dock = docker.New()
			// 证书部署后重启容器、入侵防护读取容器日志都复用 Docker 模块的客户端
			w.restart = dock.RestartContainers
			w.containers = dock.ContainerNames
			w.followDocker = dock.FollowLogs
		},
		create: func(*wiring) module.Module { return dock },
	})
}
