//go:build !no_ftp

package registry

import (
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/ftp"
	"github.com/wangwanjie/gately/internal/modules/ssl"
)

func init() {
	register(part{id: ftp.ID, order: order(ftp.ID), requires: []string{ssl.ID}, create: func(w *wiring) module.Module { return ftp.New(w.certs) }})
}
