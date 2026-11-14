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

// inSet 判断假后端的放行集合中是否有该地址与端口。
func (b *fakeBackend) inSet(addr string, access spa.Access) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.set[fwKey{netip.MustParseAddr(addr), access}]
}

// 网页登录授权同步到系统防火墙：规则开启拦截时为记录的实际地址放行规则端口；撤销后删除；
// 与敲门报文放行同一地址时，撤销网页授权后按报文授权的到期时间写回。
func TestWebGrantFirewall(t *testing.T) {
	env := newEnv(t, sshRule("ssh"), webRule("web"))
	ssh22 := spa.Access{Proto: "tcp", Port: 22}
	wg := spa.Access{Proto: "udp", Port: 51820}
	waitFor(t, "防火墙生效", func() bool { return env.m.fw.active() })
	until := env.clock.now().Add(time.Hour)
	o := knockgate.Owner{User: "alice", Site: "L/s1"}
	// IPv6 按 /64 登记，防火墙只写入实际登录的地址
	if err := knockgate.Default.AllowWeb(netip.MustParsePrefix("2001:db8:5:6::/64"), "r-ssh", until, o, netip.MustParseAddr("2001:db8:5:6::9")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "写入网页登录放行", func() bool { return env.be.inSet("2001:db8:5:6::9", ssh22) && env.be.inSet("2001:db8:5:6::9", wg) })
	if env.be.inSet("2001:db8:5:6::", ssh22) {
		t.Fatal("不应写入网段地址")
	}
	// 同一网段换了临时地址：追加写入
	_ = knockgate.Default.AllowWeb(netip.MustParsePrefix("2001:db8:5:6::/64"), "r-ssh", until, o, netip.MustParseAddr("2001:db8:5:6::a"))
	waitFor(t, "追加新地址", func() bool { return env.be.inSet("2001:db8:5:6::a", ssh22) })
	// 不在网段内的地址不记录
	_ = knockgate.Default.AllowWeb(netip.MustParsePrefix("2001:db8:5:6::/64"), "r-ssh", until, o, netip.MustParseAddr("2001:db8:9::1"))
	// 不拦截防火墙的规则不写防火墙
	_ = knockgate.Default.AllowWeb(netip.MustParsePrefix("198.51.100.8/32"), "r-web", until, o, netip.MustParseAddr("198.51.100.8"))
	time.Sleep(100 * time.Millisecond)
	if env.be.inSet("2001:db8:9::1", ssh22) || env.be.inSet("198.51.100.8", ssh22) {
		t.Fatal("不应写入网段外地址或不拦截规则的地址")
	}
	// 状态接口：网段授权显示实际地址上的放行端口
	srv := env.serve(t)
	resp, err := http.Get(srv.URL + "/api/knock/status")
	if err != nil {
		t.Fatal(err)
	}
	var st Status
	_ = json.NewDecoder(resp.Body).Decode(&st)
	resp.Body.Close()
	for _, g := range st.Grants {
		if g.Addr == "2001:db8:5:6::/64" && strings.Join(g.Ports, ",") != "tcp/22,udp/51820" {
			t.Fatalf("网段授权的防火墙端口: %+v", g)
		}
	}
	// 撤销网段授权：两个实际地址的放行都删除
	knockgate.Default.RevokePrefix(netip.MustParsePrefix("2001:db8:5:6::/64"), "")
	waitFor(t, "撤销后删除放行", func() bool {
		return !env.be.inSet("2001:db8:5:6::9", ssh22) && !env.be.inSet("2001:db8:5:6::a", ssh22) && !env.be.inSet("2001:db8:5:6::9", wg)
	})

	// 同一地址先敲门（30 秒）再网页登录（1 小时）：撤销网页授权后放行按报文授权的到期时间保留
	if err := env.m.handle(env.packet(t, env.rule("r-ssh"), spa.Message{}), src, transportUDP, "UDP"); err != nil {
		t.Fatal(err)
	}
	spaUntil := env.clock.now().Add(30 * time.Second)
	// 再加一条拦截同样端口的规则，模拟同一地址经网页登录取得的另一条授权
	env.m.mu.Lock()
	x := sshRule("x")
	_ = x.Validate()
	env.m.cfg.Rules = append(env.m.cfg.Rules, x)
	env.m.ruleChangedLocked("r-x")
	env.m.mu.Unlock()
	if err := knockgate.Default.AllowWeb(netip.PrefixFrom(src, 32), "r-x", until, o, src); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "网页授权写入", func() bool {
		env.m.stateMu.Lock()
		defer env.m.stateMu.Unlock()
		return env.m.fwGrants[fwGrantKey{addr: src, access: ssh22, rule: "r-x", web: true}].Equal(until)
	})
	n := len(env.be.addedList())
	knockgate.Default.RevokeFunc(func(g knockgate.Grant) bool { return g.Rule == "r-x" })
	waitFor(t, "按报文授权的到期时间写回", func() bool {
		for _, g := range env.be.addedList()[n:] {
			if g.Addr == src && g.Access == ssh22 && g.Until.Equal(spaUntil) {
				return true
			}
		}
		return false
	})
	if !env.be.inSet(src.String(), ssh22) {
		t.Fatal("报文授权仍有效，放行应保留")
	}
	// 关闭规则的防火墙拦截：网页授权的放行随之撤销
	_ = knockgate.Default.AllowWeb(netip.PrefixFrom(netip.MustParseAddr("203.0.113.77"), 32), "r-ssh", until, o, netip.MustParseAddr("203.0.113.77"))
	waitFor(t, "写入", func() bool { return env.be.inSet("203.0.113.77", ssh22) })
	env.m.mu.Lock()
	for i := range env.m.cfg.Rules {
		if env.m.cfg.Rules[i].Key == "r-ssh" {
			env.m.cfg.Rules[i].Firewall = false
		}
	}
	env.m.applyLocked()
	env.m.mu.Unlock()
	waitFor(t, "关闭拦截后不再保留网页授权的放行记录", func() bool {
		env.m.stateMu.Lock()
		defer env.m.stateMu.Unlock()
		for k := range env.m.fwGrants {
			if k.web && k.rule == "r-ssh" {
				return false
			}
		}
		return true
	})
}

// 修改仍启用的规则（例如改名称或端口）只撤销敲门报文取得的授权，网页登录取得的授权保留并按新端口写回防火墙；
// 停用规则时全部撤销。
func TestRuleEditKeepsWebGrants(t *testing.T) {
	env := newEnv(t, sshRule("ssh"))
	waitFor(t, "防火墙生效", func() bool { return env.m.fw.active() })
	until := env.clock.now().Add(time.Hour)
	web := netip.MustParseAddr("203.0.113.50")
	if err := knockgate.Default.AllowWeb(netip.PrefixFrom(web, 32), "r-ssh", until, knockgate.Owner{User: "alice", Site: "L/s1"}, web); err != nil {
		t.Fatal(err)
	}
	src := netip.MustParseAddr("198.51.100.9")
	if err := env.m.handle(env.packet(t, env.rule("r-ssh"), spa.Message{}), src, transportUDP, "UDP"); err != nil {
		t.Fatal(err)
	}
	ssh22 := spa.Access{Proto: "tcp", Port: 22}
	ssh2222 := spa.Access{Proto: "tcp", Port: 2222}
	waitFor(t, "写入网页登录放行", func() bool { return env.be.inSet(web.String(), ssh22) })
	// 改名称并把端口换成 2222
	env.m.mu.Lock()
	for i := range env.m.cfg.Rules {
		if env.m.cfg.Rules[i].Key == "r-ssh" {
			env.m.cfg.Rules[i].Name = "renamed"
			env.m.cfg.Rules[i].Ports = []string{"tcp/2222"}
			_ = env.m.cfg.Rules[i].Validate()
		}
	}
	env.m.ruleEditedLocked("r-ssh")
	env.m.mu.Unlock()
	if !knockgate.Default.Allowed(web, "r-ssh") {
		t.Fatal("改规则后网页登录授权应保留")
	}
	if knockgate.Default.Allowed(src, "r-ssh") {
		t.Fatal("改规则后敲门报文授权应撤销（密钥可能已变）")
	}
	waitFor(t, "按新端口写回防火墙", func() bool {
		env.m.stateMu.Lock()
		defer env.m.stateMu.Unlock()
		_, ok := env.m.fwGrants[fwGrantKey{addr: web, access: ssh2222, rule: "r-ssh", web: true}]
		return ok
	})
	// 停用规则：全部撤销
	env.m.mu.Lock()
	for i := range env.m.cfg.Rules {
		if env.m.cfg.Rules[i].Key == "r-ssh" {
			env.m.cfg.Rules[i].Enable = false
		}
	}
	env.m.ruleEditedLocked("r-ssh")
	env.m.mu.Unlock()
	if knockgate.Default.Allowed(web, "r-ssh") {
		t.Fatal("停用规则后网页登录授权应撤销")
	}
}

// 按单个地址撤销（管理端）不能删掉同一地址所在 IPv6 网段的网页登录授权写入的防火墙放行。
func TestRevokeAddrKeepsPrefixWebGrant(t *testing.T) {
	env := newEnv(t, sshRule("ssh"))
	waitFor(t, "防火墙生效", func() bool { return env.m.fw.active() })
	until := env.clock.now().Add(time.Hour)
	a := netip.MustParseAddr("2001:db8:8:9::5")
	if err := knockgate.Default.AllowWeb(netip.MustParsePrefix("2001:db8:8:9::/64"), "r-ssh", until, knockgate.Owner{User: "alice", Site: "L/s1"}, a); err != nil {
		t.Fatal(err)
	}
	ssh22 := spa.Access{Proto: "tcp", Port: 22}
	waitFor(t, "写入", func() bool { return env.be.inSet(a.String(), ssh22) })
	env.m.revoke(a, "")
	time.Sleep(100 * time.Millisecond)
	if !knockgate.Default.Allowed(a, "r-ssh") || !env.be.inSet(a.String(), ssh22) {
		t.Fatal("网段授权仍有效，防火墙放行应保留")
	}
}
