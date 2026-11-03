//go:build !no_protoroute

package registry

import (
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/protoroute"
)

func init() {
	register(part{id: protoroute.ID, order: order(protoroute.ID), create: func(*wiring) module.Module { return protoroute.New() }})
}
