//go:build !no_ddns

package registry

import (
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/ddns"
)

func init() {
	register(part{id: ddns.ID, order: order(ddns.ID), create: func(*wiring) module.Module { return ddns.New() }})
}
