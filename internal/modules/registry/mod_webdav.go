//go:build !no_webdav

package registry

import (
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/ssl"
	"github.com/wangwanjie/gately/internal/modules/webdav"
)

func init() {
	register(part{id: webdav.ID, order: order(webdav.ID), requires: []string{ssl.ID}, create: func(w *wiring) module.Module { return webdav.New(w.certs) }})
}
