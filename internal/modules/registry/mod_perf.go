//go:build !no_perf

package registry

import (
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/perf"
)

func init() {
	register(part{id: perf.ID, order: order(perf.ID), create: func(w *wiring) module.Module { return perf.New(w.web) }})
}
