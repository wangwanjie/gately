//go:build !no_easytier

package registry

import (
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/easytier"
)

func init() {
	register(part{id: easytier.ID, order: order(easytier.ID), create: func(*wiring) module.Module { return easytier.New() }})
}
