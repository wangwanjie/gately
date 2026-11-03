//go:build !no_notify

package registry

import (
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/notify"
)

func init() {
	register(part{id: notify.ID, order: order(notify.ID), create: func(*wiring) module.Module { return notify.New() }})
}
