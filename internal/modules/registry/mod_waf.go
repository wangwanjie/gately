//go:build !no_waf

package registry

import (
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/waf"
)

func init() {
	var firewall *waf.Module
	register(part{
		id:    waf.ID,
		order: order(waf.ID),
		prepare: func(w *wiring) {
			firewall = waf.New()
			w.waf = firewall
		},
		create: func(*wiring) module.Module { return firewall },
	})
}
