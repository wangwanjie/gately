//go:build !no_knock

package registry

import (
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/knock"
)

func init() {
	register(part{id: knock.ID, order: order(knock.ID), create: func(*wiring) module.Module { return knock.New() }})
}
