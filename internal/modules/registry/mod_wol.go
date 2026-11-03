//go:build !no_wol

package registry

import (
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/wol"
)

func init() {
	register(part{id: wol.ID, order: order(wol.ID), create: func(*wiring) module.Module { return wol.New() }})
}
