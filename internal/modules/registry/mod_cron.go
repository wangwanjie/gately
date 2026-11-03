//go:build !no_cron

package registry

import (
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/cron"
)

func init() {
	register(part{id: cron.ID, order: order(cron.ID), create: func(*wiring) module.Module { return cron.New() }})
}
