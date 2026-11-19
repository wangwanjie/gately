package webservice

import (
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/wangwanjie/gately/internal/hops"
)

// ctxKeyListenerKey 在监听的连接上下文中保存监听 key。
type ctxKeyListenerKey struct{}

// markAdminHop 处理访问路径请求头：访客自带的一律去掉，不能冒充 Gately 的签名；
// 带着后台 Cookie 发往本实例管理后台或本实例另一个 Web 服务监听器时附上这一跳：监听、站点、访客连接的本地地址与 TCP 对端，
// 管理界面再按对端继续追溯前面的端口转发或协议分流（见 hops 包）。
// 请求本身带着本进程签名的请求头时，说明它来自本实例更外层的站点（443 站点 → 本机 444 监听器），
// 把那些跳串在这一跳的 Prev 中一起传下去，否则管理界面只能看到最内层，修改外层监听器时不会提醒。
func markAdminHop(out *http.Request, dest adminDest, up *url.URL, site string, att *proxyAttempt) {
	outer, fromOuter := hops.Decode(out.Header.Get(hops.Header))
	out.Header.Del(hops.Header)
	if dest == destOther {
		return
	}
	ctx := out.Context()
	h := hops.Hop{Module: "webservice", Site: site}
	h.Rule, _ = ctx.Value(ctxKeyListenerKey{}).(string)
	var localPort uint16
	if c, ok := ctx.Value(ctxKeyConn{}).(net.Conn); ok && c != nil {
		h.Peer = hops.RawPeer(c)
		if la := c.LocalAddr(); la != nil {
			h.Listen = hops.Normalize(la.String())
			if ta, ok := la.(*net.TCPAddr); ok {
				localPort = uint16(ta.Port)
			}
		}
	}
	if att != nil && att.clientAP.IsValid() {
		h.Client = att.clientAP.String()
	} else if att != nil && att.client.IsValid() {
		h.Client = att.client.String()
	}
	// 只采信外层发给本监听端口的请求头
	if fromOuter && outer.Module == "webservice" && outer.ToPort != 0 && outer.ToPort == localPort {
		chain := outer.Chain()
		if len(chain) > hops.MaxChain-1 {
			chain = chain[:hops.MaxChain-1]
		}
		h.Prev = chain
	}
	if dest == destWebListen {
		h.ToPort = upstreamPort(up)
	}
	out.Header.Set(hops.Header, hops.Encode(h))
}

// upstreamPort 返回后端地址的端口，未写端口时按协议取默认值，无法解析时为 0。
func upstreamPort(u *url.URL) uint16 {
	if u == nil {
		return 0
	}
	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "https", "wss":
			port = "443"
		default:
			port = "80"
		}
	}
	p, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return 0
	}
	return uint16(p)
}
