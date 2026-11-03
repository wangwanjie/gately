//go:build !no_backup

package registry

import (
	"github.com/wangwanjie/gately/internal/module"
	backupmod "github.com/wangwanjie/gately/internal/modules/backup"
	"github.com/wangwanjie/gately/internal/modules/storage"
)

func init() {
	register(part{id: backupmod.ID, order: order(backupmod.ID), requires: []string{storage.ID}, create: func(w *wiring) module.Module {
		// 备份写到存储挂载模块管理的存储上；depcheck_backup.go 保证不带 no_backup 时存储挂载模块一定编译
		return backupmod.New(w.files.(backupmod.Storage))
	}})
}
