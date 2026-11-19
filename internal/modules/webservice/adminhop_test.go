package webservice

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/hops"
)

// 发往本实例管理后台的请求附带签名的访问路径（监听与站点 key、访客连接的地址），访客伪造的同名请求头一律去掉
func TestAdminHopHeader(t *testing.T) {
	got := make(chan string, 4)
	echo := func() *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got <- r.Header.Get(hops.Header)
		}))
		t.Cleanup(srv.Close)
		return srv
	}
	admin, other := echo(), echo()
	_, adminPort, _ := net.SplitHostPort(admin.Listener.Addr().String())
	withAdminListen(t, "127.0.0.1:"+adminPort)
	l := Listener{Key: "lk", Sites: []Site{
		{Enable: true, Key: "sa", Domains: []string{"admin.test"}, Targets: []string{admin.URL}},
		{Enable: true, Key: "so", Domains: []string{"other.test"}, Targets: []string{other.URL}},
	}}
	addr, _ := startRunner(t, l, nil, nil)
	forged := hops.Encode(hops.Hop{Module: "webservice", Rule: "evil"})

	get(t, noRedirect(), addr, "admin.test", "/api/session", hdr("Cookie", auth.CookieName+"=x", hops.Header, "forged"))
	h, ok := hops.Decode(<-got)
	if !ok || h.Rule != "lk" || h.Site != "sa" || h.Listen != hops.Normalize(addr) || h.Peer == "" {
		t.Fatalf("发往后台的请求应带本监听与站点的路径: %+v %v", h, ok)
	}
	// 没有后台 Cookie（不是管理员在访问）时不附带
	get(t, noRedirect(), addr, "admin.test", "/", hdr(hops.Header, forged))
	if v := <-got; v != "" {
		t.Fatalf("不带后台 Cookie 时不应附带路径，且应去掉访客自带的头: %q", v)
	}
	get(t, noRedirect(), addr, "other.test", "/", hdr("Cookie", auth.CookieName+"=x", hops.Header, forged))
	if v := <-got; v != "" {
		t.Fatalf("发往其他后端时不应附带路径: %q", v)
	}
}
