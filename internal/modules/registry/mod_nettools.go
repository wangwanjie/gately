//go:build !no_nettools

package registry

import (
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/nettools"
)

func init() {
	register(part{id: nettools.ID, order: order(nettools.ID), create: func(*wiring) module.Module { return nettools.New() }})
}
