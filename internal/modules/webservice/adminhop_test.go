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

// 本实例两层站点（外层站点 → 本机内层监听器 → 管理后台）：外层发往内层时也附带路径，内层把外层这一跳串在 Prev 中，
// 管理界面才能看到外层监听器；访客自己带来的（伪造的）路径头不会被串进去
func TestAdminHopHeaderChained(t *testing.T) {
	got := make(chan string, 4)
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get(hops.Header)
	}))
	t.Cleanup(admin.Close)
	_, adminPort, _ := net.SplitHostPort(admin.Listener.Addr().String())
	withAdminListen(t, "127.0.0.1:"+adminPort)

	inner, _ := startRunner(t, Listener{Key: "in", Sites: []Site{
		{Enable: true, Key: "si", Domains: []string{"admin.test"}, Targets: []string{admin.URL}},
	}}, nil, nil)
	outer, _ := startRunner(t, Listener{Key: "out", Sites: []Site{
		{Enable: true, Key: "so", Domains: []string{"admin.test"}, Targets: []string{"http://" + inner}},
	}}, nil, nil)

	forged := hops.Encode(hops.Hop{Module: "webservice", Rule: "evil"})
	// 访客经外层访问：外层收到的伪造头被丢弃，内层看到的是外层签发的头
	get(t, noRedirect(), outer, "admin.test", "/api/session", hdr("Cookie", auth.CookieName+"=x", hops.Header, "forged"))
	h, ok := hops.Decode(<-got)
	chain := h.Chain()
	if !ok || len(chain) != 2 || chain[0].Rule != "in" || chain[0].Site != "si" || chain[0].Listen != hops.Normalize(inner) ||
		chain[1].Rule != "out" || chain[1].Site != "so" || chain[1].Listen != hops.Normalize(outer) || chain[1].Peer == "" {
		t.Fatalf("内层应把外层这一跳串在 Prev 中: %+v %v", h, ok)
	}
	// 访客带着签名有效的请求头（模拟签名外泄）访问外层：它没有写明发给外层的端口（ToPort），外层不采信，
	// 去掉后重新签发，访客无法把任意跳塞进 Prev
	get(t, noRedirect(), outer, "admin.test", "/api/session", hdr("Cookie", auth.CookieName+"=x", hops.Header, forged))
	h, _ = hops.Decode(<-got)
	for _, x := range h.Chain() {
		if x.Rule == "evil" {
			t.Fatalf("访客带来的路径头不应被串入: %+v", h)
		}
	}
}
