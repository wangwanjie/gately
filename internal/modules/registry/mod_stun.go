//go:build !no_stun

package registry

import (
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/stun"
)

func init() {
	register(part{id: stun.ID, order: order(stun.ID), create: func(*wiring) module.Module { return stun.New() }})
}
