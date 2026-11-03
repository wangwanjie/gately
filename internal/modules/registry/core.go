package registry

import (
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/portforward"
	"github.com/wangwanjie/gately/internal/modules/ssl"
	"github.com/wangwanjie/gately/internal/modules/webservice"
)

// 核心模块：端口转发、SSL 证书与 Web 服务始终编译，不提供 build tag。
func init() {
	register(part{id: portforward.ID, order: order(portforward.ID), create: func(*wiring) module.Module { return portforward.New() }})
	register(part{id: ssl.ID, order: order(ssl.ID), create: func(w *wiring) module.Module { return w.certs }})
	// Web 服务按站点引用 WAF 实例，WAF 模块编译进来时它开启期间 WAF 不能关闭
	register(part{id: webservice.ID, order: order(webservice.ID), requires: []string{ssl.ID, "waf"}, create: func(w *wiring) module.Module { return w.web }})
}
