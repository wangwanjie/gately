//go:build !no_storage

package registry

import (
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/storage"
)

func init() {
	var files *storage.Module
	register(part{
		id:    storage.ID,
		order: order(storage.ID),
		prepare: func(w *wiring) {
			files = storage.New()
			w.files = files
		},
		create: func(*wiring) module.Module { return files },
	})
}
