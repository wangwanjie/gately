//go:build !no_netbird

package registry

import (
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/netbird"
)

func init() {
	register(part{id: netbird.ID, order: order(netbird.ID), create: func(*wiring) module.Module { return netbird.New() }})
}
