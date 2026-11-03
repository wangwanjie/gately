//go:build !no_tailscale

package registry

import (
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/tailscale"
)

func init() {
	register(part{id: tailscale.ID, order: order(tailscale.ID), create: func(*wiring) module.Module { return tailscale.New() }})
}
