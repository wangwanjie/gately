package knock

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/knockgate"
	"github.com/wangwanjie/gately/internal/spa"
)

// webRule 返回只经网页登录放行的规则：没有端口、不拦截防火墙。
func webRule(name string) Rule {
	return Rule{Key: "r-" + name, Name: name, Enable: true}
}

// 没有端口的规则：可以保存；不打开 UDP 监听；敲门报文被拒绝并说明原因；网页登录的授权照常生效。
func TestPortlessRule(t *testing.T) {
	r := webRule("web")
	if err := r.Validate(); err != nil {
		t.Fatalf("不拦截防火墙时端口可以留空: %v", err)
	}
	env := newEnv(t, r)
	l, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.LocalAddr().(*net.UDPAddr).Port
	_ = l.Close()
	env.m.mu.Lock()
	env.m.cfg.Settings.Port = port
	env.m.applyLocked()
	listening := env.m.conn != nil
	env.m.mu.Unlock()
	if listening {
		t.Fatal("只有没有端口的规则时不应监听 UDP")
	}
	if p := env.be.lastPlan(); len(p.Ports) != 0 {
		t.Fatalf("不应拦截任何端口: %+v", p.Ports)
	}
	pkt := env.packet(t, env.rule("r-web"), spa.Message{Access: []spa.Access{{Proto: "tcp", Port: 22}}})
	if err := env.m.handle(pkt, src, transportUDP, "UDP"); err == nil || !strings.Contains(err.Error(), "没有填写端口") {
		t.Fatalf("没有端口的规则应拒绝敲门报文: %v", err)
	}
	if knockgate.Default.Allowed(src, "r-web") {
		t.Fatal("报文不应放行")
	}
	// 网页登录（Web 服务模块登记）的授权按规则目录校验，启用的规则可以登记
	until := env.clock.now().Add(time.Hour)
	if err := knockgate.Default.AllowWeb(netip.MustParsePrefix("2001:db8:5:6::/64"), "r-web", until, knockgate.Owner{User: "alice", Site: "s1", SiteName: "导航"}, netip.Addr{}); err != nil {
		t.Fatal(err)
	}
	if !knockgate.Default.Allowed(netip.MustParseAddr("2001:db8:5:6::9"), "r-web") {
		t.Fatal("网段授权应生效")
	}
}

// 状态接口列出授权来源与网页登录的用户、站点；撤销接口接受网段写法。
func TestStatusAndRevokeWebGrants(t *testing.T) {
	env := newEnv(t, sshRule("ssh"), webRule("web"))
	srv := env.serve(t)
	if err := env.m.handle(env.packet(t, env.rule("r-ssh"), spa.Message{}), src, transportUDP, "UDP"); err != nil {
		t.Fatal(err)
	}
	until := env.clock.now().Add(time.Hour)
	if err := knockgate.Default.AllowWeb(netip.MustParsePrefix("2001:db8:5:6::/64"), "r-web", until, knockgate.Owner{User: "alice", Site: "s1", SiteName: "导航"}, netip.Addr{}); err != nil {
		t.Fatal(err)
	}
	if err := knockgate.Default.AllowWeb(netip.MustParsePrefix("198.51.100.7/32"), "r-web", until, knockgate.Owner{User: "alice", Site: "s1", SiteName: "导航"}, netip.Addr{}); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(srv.URL + "/api/knock/status")
	if err != nil {
		t.Fatal(err)
	}
	var st Status
	err = json.NewDecoder(resp.Body).Decode(&st)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]GrantView{}
	for _, g := range st.Grants {
		got[g.Addr] = g
	}
	if g := got[src.String()]; g.Source != knockgate.SourceSPA || len(g.Ports) == 0 || g.User != "" {
		t.Fatalf("报文授权: %+v", g)
	}
	if g := got["2001:db8:5:6::/64"]; g.Source != knockgate.SourceWeb || g.User != "alice" || g.SiteName != "导航" || len(g.Ports) != 0 || g.RuleName != "web" {
		t.Fatalf("网段授权: %+v (全部 %+v)", g, st.Grants)
	}
	if g := got["198.51.100.7"]; g.Source != knockgate.SourceWeb {
		t.Fatalf("单地址网页授权: %+v", g)
	}
	del := func(body string) int {
		req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/knock/grants", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	for _, bad := range []string{`{"addr":"x/64"}`, `{"addr":"nope"}`} {
		if code := del(bad); code != http.StatusBadRequest {
			t.Fatalf("%s 应返回 400: %d", bad, code)
		}
	}
	if code := del(`{"addr":"2001:db8:5:6::/64","rule":"r-web"}`); code != http.StatusOK {
		t.Fatalf("撤销网段: %d", code)
	}
	if knockgate.Default.Allowed(netip.MustParseAddr("2001:db8:5:6::9"), "r-web") {
		t.Fatal("网段授权应已撤销")
	}
	if code := del(fmt.Sprintf(`{"addr":"%s"}`, "198.51.100.7")); code != http.StatusOK {
		t.Fatalf("撤销地址: %d", code)
	}
	if knockgate.Default.Allowed(netip.MustParseAddr("198.51.100.7"), "r-web") || !knockgate.Default.Allowed(src, "r-ssh") {
		t.Fatal("只撤销指定地址")
	}
}
