//go:build !no_filebrowser

package registry

import (
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/filebrowser"
)

func init() {
	register(part{id: filebrowser.ID, order: order(filebrowser.ID), create: func(*wiring) module.Module { return filebrowser.New() }})
}
