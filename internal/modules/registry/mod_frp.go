//go:build !no_frp

package registry

import (
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/frp"
)

func init() {
	register(part{id: frp.ID, order: order(frp.ID), create: func(*wiring) module.Module { return frp.New() }})
}
