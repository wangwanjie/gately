package webservice

import (
	"net"
	"net/http"

	"github.com/wangwanjie/gately/internal/hops"
)

// ctxKeyListenerKey 在监听的连接上下文中保存监听 key。
type ctxKeyListenerKey struct{}

// markAdminHop 处理访问路径请求头：访客自带的一律去掉，不能冒充 Gately 的签名；
// 带着后台 Cookie 发往本实例管理后台时（admin 为 true）附上这一跳：监听、站点、访客连接的本地地址与 TCP 对端，
// 管理界面再按对端继续追溯前面的端口转发或协议分流（见 hops 包）。
func markAdminHop(out *http.Request, admin bool, site string, att *proxyAttempt) {
	out.Header.Del(hops.Header)
	if !admin {
		return
	}
	ctx := out.Context()
	h := hops.Hop{Module: "webservice", Site: site}
	h.Rule, _ = ctx.Value(ctxKeyListenerKey{}).(string)
	if c, ok := ctx.Value(ctxKeyConn{}).(net.Conn); ok && c != nil {
		h.Peer = hops.RawPeer(c)
		if la := c.LocalAddr(); la != nil {
			h.Listen = hops.Normalize(la.String())
		}
	}
	if att != nil && att.clientAP.IsValid() {
		h.Client = att.clientAP.String()
	} else if att != nil && att.client.IsValid() {
		h.Client = att.client.String()
	}
	out.Header.Set(hops.Header, hops.Encode(h))
}
