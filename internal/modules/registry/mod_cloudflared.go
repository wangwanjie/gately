//go:build !no_cloudflared

package registry

import (
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/cloudflared"
)

func init() {
	register(part{id: cloudflared.ID, order: order(cloudflared.ID), create: func(*wiring) module.Module { return cloudflared.New() }})
}
