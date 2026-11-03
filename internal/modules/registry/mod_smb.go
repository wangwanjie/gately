//go:build !no_smb

package registry

import (
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/smb"
)

func init() {
	register(part{id: smb.ID, order: order(smb.ID), create: func(*wiring) module.Module { return smb.New() }})
}
