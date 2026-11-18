package knock

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/knockgate"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/spa"
)

// teamRule 返回带 alice、bob 两个身份的规则。
func teamRule(name string) Rule {
	r := sshRule(name)
	r.Identities = []Identity{{Name: "alice", Enable: true}, {Name: "bob", Enable: true}}
	return r
}

// identityPacket 用规则中身份 name 的密钥生成报文。
func (e *testEnv) identityPacket(t *testing.T, r Rule, name string, msg spa.Message) []byte {
	t.Helper()
	for _, id := range r.Identities {
		if id.Name != name {
			continue
		}
		k, err := id.spaKey(r.HMACType)
		if err != nil {
			t.Fatal(err)
		}
		if msg.Timestamp == 0 {
			msg.Timestamp = e.clock.now().Unix()
		}
		if msg.Type == 0 {
			msg.Type = spa.AccessMsg
		}
		if msg.Access == nil {
			msg.Access = []spa.Access{{Proto: "tcp", Port: 22}}
		}
		if msg.User == "" {
			msg.User = "u"
		}
		p, err := spa.Encode(msg, k, rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	t.Fatalf("规则中没有身份 %s", name)
	return nil
}

// grantOf 返回 addr 持有规则 rule 的授权，没有时 ok 为 false。
func grantOf(addr netip.Addr, rule string) (knockgate.Grant, bool) {
	for _, g := range knockgate.Default.List() {
		if g.Prefix == netip.PrefixFrom(addr, addr.BitLen()) && g.Rule == rule {
			return g, true
		}
	}
	return knockgate.Grant{}, false
}

func (e *testEnv) fwHas(addr netip.Addr, a spa.Access) bool {
	e.be.mu.Lock()
	defer e.be.mu.Unlock()
	return e.be.set[fwKey{addr, a}]
}

var tcp22 = spa.Access{Proto: "tcp", Port: 22}

// 身份的密钥可以敲门：授权、日志、事件、统计与状态接口都带身份名称；共享密钥仍可用，授权不带身份。
func TestIdentityKnock(t *testing.T) {
	env := newEnv(t, teamRule("ssh"))
	r := env.rule("r-ssh")
	if r.Identities[0].ID == "" || r.Identities[0].ID == r.Identities[1].ID {
		t.Fatalf("应自动分配不同的身份 ID: %+v", r.Identities)
	}
	a, b, c := netip.MustParseAddr("203.0.113.1"), netip.MustParseAddr("203.0.113.2"), netip.MustParseAddr("203.0.113.3")
	if err := env.m.handle(env.identityPacket(t, r, "alice", spa.Message{}), a, transportUDP, "UDP"); err != nil {
		t.Fatal(err)
	}
	if err := env.m.handle(env.identityPacket(t, r, "bob", spa.Message{}), b, transportUDP, "UDP"); err != nil {
		t.Fatal(err)
	}
	if err := env.m.handle(env.packet(t, r, spa.Message{}), c, transportUDP, "UDP"); err != nil {
		t.Fatal(err)
	}
	ga, _ := grantOf(a, "r-ssh")
	gb, _ := grantOf(b, "r-ssh")
	gc, okc := grantOf(c, "r-ssh")
	if ga.User != "alice" || gb.User != "bob" || !okc || gc.User != "" {
		t.Fatalf("授权的身份不对: %q %q %q", ga.User, gb.User, gc.User)
	}
	if !env.hasLog("身份 alice") {
		t.Fatal("日志应写明身份")
	}
	waitFor(t, "成功事件", func() bool { return len(env.eventsOf(events.KnockOpened)) == 3 })
	var withID int
	for _, e := range env.eventsOf(events.KnockOpened) {
		if e.Fields["identity"] != "" {
			withID++
		}
	}
	if withID != 2 {
		t.Fatalf("两个身份的事件应带 identity 字段: %d", withID)
	}
	env.m.stateMu.Lock()
	alice := *env.m.idStats["r-ssh"][r.Identities[0].ID]
	total := *env.m.stats["r-ssh"]
	env.m.stateMu.Unlock()
	if alice.Knocks != 1 || alice.LastAddr != a.String() || total.Knocks != 3 {
		t.Fatalf("统计不对: %+v %+v", alice, total)
	}
	// 状态接口：敲门授权的身份放在 identity 字段，user 只用于网页登录
	srv := env.serve(t)
	var st Status
	getJSON(t, srv.URL+"/api/knock/status", &st)
	found := false
	for _, g := range st.Grants {
		if g.Addr == a.String() {
			found = g.Identity == "alice" && g.User == ""
		}
	}
	if !found {
		t.Fatalf("状态接口应显示身份: %+v", st.Grants)
	}
	// 身份使用错误的端口仍按规则拒绝，并计入该身份的拒绝次数
	bad := env.identityPacket(t, r, "alice", spa.Message{Access: []spa.Access{{Proto: "tcp", Port: 23}}})
	if err := env.m.handle(bad, a, transportTCP, "TCP"); err == nil {
		t.Fatal("不允许的端口应被拒绝")
	}
	env.m.stateMu.Lock()
	rej := env.m.idStats["r-ssh"][r.Identities[0].ID].Rejected
	env.m.stateMu.Unlock()
	if rej != 1 || !env.hasLog("身份 alice，经 TCP") {
		t.Fatalf("拒绝应计入身份并写明身份: %d", rej)
	}
	waitFor(t, "失败事件带身份", func() bool {
		evs := env.eventsOf(events.KnockFailed)
		return len(evs) == 1 && evs[0].Fields["identity"] == "alice"
	})
}

// 停用共享密钥后只接受身份的密钥；停用的身份不能敲门。
func TestSharedKeyOffAndDisabledIdentity(t *testing.T) {
	r := teamRule("ssh")
	r.SharedKeyOff = true
	r.Identities[1].Enable = false
	env := newEnv(t, r)
	r = env.rule("r-ssh")
	if err := env.m.handle(env.packet(t, r, spa.Message{}), src, transportUDP, "UDP"); err == nil || !strings.Contains(err.Error(), "认证失败") {
		t.Fatalf("停用的共享密钥应认证失败: %v", err)
	}
	if err := env.m.handle(env.identityPacket(t, r, "bob", spa.Message{}), src, transportUDP, "UDP"); err == nil {
		t.Fatal("停用的身份不应能敲门")
	}
	if err := env.m.handle(env.identityPacket(t, r, "alice", spa.Message{}), src, transportUDP, "UDP"); err != nil {
		t.Fatal(err)
	}
	if g, ok := grantOf(src, "r-ssh"); !ok || g.User != "alice" {
		t.Fatalf("应放行 alice: %+v", g)
	}
	if n := len(env.m.snap.Load().keys); n != 1 {
		t.Fatalf("只应尝试 1 个密钥: %d", n)
	}
}

// 单地址放行：同一身份从新地址敲门后，此前的地址（含防火墙放行）立即撤销；其他身份与共享密钥互不影响。
func TestSingleAddr(t *testing.T) {
	r := teamRule("ssh")
	r.SingleAddr = true
	env := newEnv(t, r)
	r = env.rule("r-ssh")
	a1, a2 := netip.MustParseAddr("203.0.113.1"), netip.MustParseAddr("2001:db8::1")
	b, s1, s2 := netip.MustParseAddr("203.0.113.2"), netip.MustParseAddr("203.0.113.5"), netip.MustParseAddr("203.0.113.6")
	knock := func(name string, from netip.Addr) {
		t.Helper()
		var pkt []byte
		if name == "" {
			pkt = env.packet(t, r, spa.Message{})
		} else {
			pkt = env.identityPacket(t, r, name, spa.Message{})
		}
		if err := env.m.handle(pkt, from, transportUDP, "UDP"); err != nil {
			t.Fatal(err)
		}
	}
	knock("alice", a1)
	knock("bob", b)
	knock("", s1)
	waitFor(t, "写入放行", func() bool { return env.fwHas(a1, tcp22) && env.fwHas(b, tcp22) && env.fwHas(s1, tcp22) })
	knock("alice", a2)
	if knockgate.Default.Allowed(a1, "r-ssh") || !knockgate.Default.Allowed(a2, "r-ssh") {
		t.Fatal("alice 的旧地址应被撤销、新地址放行")
	}
	if !knockgate.Default.Allowed(b, "r-ssh") || !knockgate.Default.Allowed(s1, "r-ssh") {
		t.Fatal("bob 与共享密钥的放行不应受影响")
	}
	waitFor(t, "删除旧地址的防火墙放行", func() bool { return !env.fwHas(a1, tcp22) && env.fwHas(a2, tcp22) })
	if !env.hasLog("已撤销身份 alice 此前放行的 203.0.113.1") {
		t.Fatal("应记录撤销的地址")
	}
	// 共享密钥也算一个身份
	knock("", s2)
	if knockgate.Default.Allowed(s1, "r-ssh") || !knockgate.Default.Allowed(s2, "r-ssh") || !knockgate.Default.Allowed(b, "r-ssh") {
		t.Fatal("共享密钥的旧地址应被撤销，bob 不受影响")
	}
	// 同一地址再次敲门只是续期，不撤销自己
	env.clock.set(env.clock.now().Add(time.Second))
	knock("alice", a2)
	if !knockgate.Default.Allowed(a2, "r-ssh") {
		t.Fatal("同一地址续期后仍应放行")
	}
}

// 未开启单地址放行时同一身份可以同时放行多个地址；同一地址被另一身份敲门后授权改记为后者。
func TestMultiAddrAndOwnerChange(t *testing.T) {
	env := newEnv(t, teamRule("ssh"))
	r := env.rule("r-ssh")
	a1, a2 := netip.MustParseAddr("203.0.113.1"), netip.MustParseAddr("203.0.113.2")
	for _, a := range []netip.Addr{a1, a2} {
		if err := env.m.handle(env.identityPacket(t, r, "alice", spa.Message{}), a, transportUDP, "UDP"); err != nil {
			t.Fatal(err)
		}
	}
	if !knockgate.Default.Allowed(a1, "r-ssh") || !knockgate.Default.Allowed(a2, "r-ssh") {
		t.Fatal("两个地址都应放行")
	}
	if err := env.m.handle(env.identityPacket(t, r, "bob", spa.Message{}), a2, transportUDP, "UDP"); err != nil {
		t.Fatal(err)
	}
	if g, _ := grantOf(a2, "r-ssh"); g.User != "bob" {
		t.Fatalf("同一地址应改记为最近敲门的身份: %q", g.User)
	}
}

type apiResp struct {
	Error string `json:"error"`
}

func doJSON(t *testing.T, method, url string, body any) (int, apiResp) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, rd)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out apiResp
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &out)
	if out.Error == "" && resp.StatusCode >= 400 {
		out.Error = string(raw)
	}
	return resp.StatusCode, out
}

func getJSON(t *testing.T, url string, v any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatal(err)
	}
}

// 接口：身份密钥脱敏输出、提交占位符时保留原密钥、可单独查看明文；只改身份时其他身份的放行（含防火墙）保留，
// 停用或删除的身份立即失效；修改端口等放行范围时全部敲门授权撤销。
func TestIdentityEditViaAPI(t *testing.T) {
	env := newEnv(t, teamRule("ssh"))
	srv := env.serve(t)
	r := env.rule("r-ssh")
	a, b, s := netip.MustParseAddr("203.0.113.1"), netip.MustParseAddr("203.0.113.2"), netip.MustParseAddr("203.0.113.3")
	for _, k := range []struct {
		name string
		from netip.Addr
	}{{"alice", a}, {"bob", b}, {"", s}} {
		pkt := env.packet(t, r, spa.Message{})
		if k.name != "" {
			pkt = env.identityPacket(t, r, k.name, spa.Message{})
		}
		if err := env.m.handle(pkt, k.from, transportUDP, "UDP"); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, "写入放行", func() bool { return env.fwHas(a, tcp22) && env.fwHas(b, tcp22) })

	// 列表中的身份密钥脱敏
	var list []RuleView
	getJSON(t, srv.URL+"/api/knock/rules", &list)
	view := list[0]
	if view.Identities[0].EncKey != crud.SecretMask || view.Identities[0].HMACKey != crud.SecretMask {
		t.Fatalf("身份密钥应脱敏: %+v", view.Identities[0])
	}
	if view.IdentityStats[r.Identities[0].ID].Knocks != 1 {
		t.Fatalf("应输出身份统计: %+v", view.IdentityStats)
	}
	// 密钥明文：identities.<ID>.encKey
	var sec map[string]string
	getJSON(t, srv.URL+"/api/knock/rules/r-ssh/secrets", &sec)
	if sec["identities."+r.Identities[1].ID+".hmacKey"] != r.Identities[1].HMACKey {
		t.Fatalf("应能查看身份密钥明文: %v", sec)
	}

	// 停用 bob、新增 carol（占位符原样提交），名称改动：alice 与共享密钥的放行保留，bob 撤销
	upd := view.Rule
	upd.Name = "ssh-team"
	upd.Identities[1].Enable = false
	upd.Identities = append(upd.Identities, Identity{Name: "carol", Enable: true})
	if code, resp := doJSON(t, http.MethodPut, srv.URL+"/api/knock/rules/r-ssh", upd); code != http.StatusOK {
		t.Fatalf("保存失败: %d %s", code, resp.Error)
	}
	saved := env.rule("r-ssh")
	if saved.Identities[0].HMACKey != r.Identities[0].HMACKey || saved.Identities[1].EncKey != r.Identities[1].EncKey {
		t.Fatal("提交占位符时应保留原密钥")
	}
	if saved.Identities[2].ID == "" || saved.Identities[2].HMACKey == "" || saved.Identities[2].HMACKey == crud.SecretMask {
		t.Fatalf("新身份应分配 ID 并生成密钥: %+v", saved.Identities[2])
	}
	if !knockgate.Default.Allowed(a, "r-ssh") || !knockgate.Default.Allowed(s, "r-ssh") {
		t.Fatal("未变化的身份与共享密钥的放行应保留")
	}
	if knockgate.Default.Allowed(b, "r-ssh") {
		t.Fatal("停用的身份的放行应撤销")
	}
	waitFor(t, "防火墙按保留的授权重建", func() bool { return env.fwHas(a, tcp22) && env.fwHas(s, tcp22) && !env.fwHas(b, tcp22) })

	// 更换 alice 的密钥：alice 的放行撤销
	// 复制身份列表：直接改 saved 会改到模块配置中的同一底层数组
	upd = saved
	upd.Identities = append([]Identity{}, saved.Identities...)
	upd.Identities[0].HMACKey = ""
	upd.Identities[1].EncKey, upd.Identities[1].HMACKey = crud.SecretMask, crud.SecretMask
	upd.Identities[2].EncKey, upd.Identities[2].HMACKey = crud.SecretMask, crud.SecretMask
	if code, resp := doJSON(t, http.MethodPut, srv.URL+"/api/knock/rules/r-ssh", upd); code != http.StatusOK {
		t.Fatalf("保存失败: %d %s", code, resp.Error)
	}
	if knockgate.Default.Allowed(a, "r-ssh") || !knockgate.Default.Allowed(s, "r-ssh") {
		t.Fatal("更换密钥的身份的放行应撤销，共享密钥保留")
	}
	// 停用共享密钥：共享密钥的放行撤销
	upd = env.rule("r-ssh")
	upd.SharedKeyOff = true
	if code, resp := doJSON(t, http.MethodPut, srv.URL+"/api/knock/rules/r-ssh", upd); code != http.StatusOK {
		t.Fatalf("保存失败: %d %s", code, resp.Error)
	}
	if knockgate.Default.Allowed(s, "r-ssh") {
		t.Fatal("停用共享密钥后其放行应撤销")
	}
	// 修改端口：全部敲门授权撤销
	r = env.rule("r-ssh")
	if err := env.m.handle(env.identityPacket(t, r, "carol", spa.Message{}), a, transportUDP, "UDP"); err != nil {
		t.Fatal(err)
	}
	upd = env.rule("r-ssh")
	upd.Ports = []string{"tcp/22"}
	if code, resp := doJSON(t, http.MethodPut, srv.URL+"/api/knock/rules/r-ssh", upd); code != http.StatusOK {
		t.Fatalf("保存失败: %d %s", code, resp.Error)
	}
	if knockgate.Default.Allowed(a, "r-ssh") {
		t.Fatal("修改端口后应撤销全部敲门授权")
	}
	// 删除身份后统计一并清除
	upd = env.rule("r-ssh")
	upd.Identities = append([]Identity{}, upd.Identities[2:]...)
	if code, resp := doJSON(t, http.MethodPut, srv.URL+"/api/knock/rules/r-ssh", upd); code != http.StatusOK {
		t.Fatalf("保存失败: %d %s", code, resp.Error)
	}
	env.m.stateMu.Lock()
	_, kept := env.m.idStats["r-ssh"][r.Identities[0].ID]
	env.m.stateMu.Unlock()
	if kept {
		t.Fatal("删除的身份的统计应清除")
	}
}

// 复制规则时身份的密钥重新生成；密钥与其他规则重复时拒绝保存。
func TestIdentityCopyAndDuplicateKeys(t *testing.T) {
	env := newEnv(t, teamRule("ssh"))
	srv := env.serve(t)
	var list []RuleView
	getJSON(t, srv.URL+"/api/knock/rules", &list)
	cp := list[0].Rule
	cp.Key, cp.Name = "", "ssh copy"
	code, resp := doJSON(t, http.MethodPost, srv.URL+"/api/knock/rules/r-ssh/copy", cp)
	if code != http.StatusOK {
		t.Fatalf("复制失败: %d %s", code, resp.Error)
	}
	orig := env.rule("r-ssh")
	var dup Rule
	env.m.mu.Lock()
	dup = env.m.cfg.Rules[1]
	env.m.mu.Unlock()
	for i := range dup.Identities {
		if dup.Identities[i].HMACKey == orig.Identities[i].HMACKey || dup.Identities[i].EncKey == orig.Identities[i].EncKey {
			t.Fatal("复制的身份应生成新密钥")
		}
	}
	// 新规则的身份使用已有规则中某个身份的密钥
	x := sshRule("x")
	x.Key = ""
	x.Identities = []Identity{{Name: "dave", Enable: true, EncKey: orig.Identities[0].EncKey, HMACKey: orig.Identities[0].HMACKey}}
	code, resp = doJSON(t, http.MethodPost, srv.URL+"/api/knock/rules", x)
	if code != http.StatusBadRequest || !strings.Contains(resp.Error, "dave") {
		t.Fatalf("重复的密钥应被拒绝: %d %s", code, resp.Error)
	}
}

// 规则校验：身份名称、重复、数量、共享密钥停用与没有端口时的限制。
func TestValidateIdentities(t *testing.T) {
	base := func() Rule {
		r := sshRule("v")
		r.Identities = []Identity{{Name: " alice ", Enable: true}}
		return r
	}
	r := base()
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if r.Identities[0].Name != "alice" || r.Identities[0].ID == "" || r.Identities[0].EncKey == "" {
		t.Fatalf("应规范化名称、分配 ID 并生成密钥: %+v", r.Identities[0])
	}
	cases := []struct {
		name string
		edit func(*Rule)
		want string
	}{
		{"空名称", func(r *Rule) { r.Identities = append(r.Identities, Identity{Name: " ", Enable: true}) }, "请填写名称"},
		{"重复名称", func(r *Rule) { r.Identities = append(r.Identities, Identity{Name: "ALICE", Enable: true}) }, "重复"},
		{"控制字符", func(r *Rule) { r.Identities[0].Name = "a\nb" }, "控制字符"},
		{"过长", func(r *Rule) { r.Identities[0].Name = strings.Repeat("名", maxNameLen+1) }, "不能超过"},
		{"无效密钥", func(r *Rule) { r.Identities[0].HMACKey = "!!" }, "alice"},
		{"与共享密钥相同", func(r *Rule) {
			r.HMACKey = genKey(64)
			r.Identities[0].HMACKey = r.HMACKey
		}, "与共享密钥相同"},
		{"身份之间相同", func(r *Rule) {
			k := genKey(64)
			r.Identities[0].HMACKey = k
			r.Identities = append(r.Identities, Identity{Name: "bob", Enable: true, HMACKey: k})
		}, "与身份「alice」相同"},
		{"停用共享密钥且没有启用的身份", func(r *Rule) {
			r.SharedKeyOff = true
			r.Identities[0].Enable = false
		}, "至少添加一个启用的身份"},
		{"没有端口", func(r *Rule) {
			r.Ports = nil
			r.Firewall = false
		}, "不能添加身份"},
		{"数量超限", func(r *Rule) {
			for i := range maxIdentities {
				r.Identities = append(r.Identities, Identity{Name: fmt.Sprintf("u%d", i)})
			}
		}, "最多"},
	}
	for _, c := range cases {
		r := base()
		c.edit(&r)
		if err := r.Validate(); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	// 停用共享密钥、身份全部停用但规则没有身份以外的用途时不允许；有启用身份时允许
	ok := base()
	ok.SharedKeyOff = true
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
}

// 全部启用规则接受敲门的密钥总数有上限。
func TestTotalKeysLimit(t *testing.T) {
	env := newEnv(t)
	env.m.mu.Lock()
	defer env.m.mu.Unlock()
	for i := range maxTotalKeys / (maxIdentities + 1) {
		r := sshRule(fmt.Sprintf("r%d", i))
		for j := range maxIdentities {
			r.Identities = append(r.Identities, Identity{Name: fmt.Sprintf("u%d", j), Enable: true})
		}
		if err := r.Validate(); err != nil {
			t.Fatal(err)
		}
		env.m.cfg.Rules = append(env.m.cfg.Rules, r)
	}
	x := sshRule("x")
	for j := range maxIdentities {
		x.Identities = append(x.Identities, Identity{Name: fmt.Sprintf("u%d", j), Enable: true})
	}
	if err := x.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := env.m.checkRuleLocked(&x); err == nil || !strings.Contains(err.Error(), "合计不能超过") {
		t.Fatalf("应限制密钥总数: %v", err)
	}
	x.Enable = false
	if err := env.m.checkRuleLocked(&x); err != nil {
		t.Fatalf("停用的规则不计入: %v", err)
	}
}

// 配置文件中身份没有密钥或 ID 时启动生成并保存，重启后不变。
func TestInitSavesIdentityKeys(t *testing.T) {
	store, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := Rule{Key: "k1", Name: "ssh", Enable: true, Ports: []string{"tcp/22"}, EncKey: genKey(32), HMACKey: genKey(64),
		Identities: []Identity{{Name: "alice", Enable: true}}}
	if err := store.Save(ID, fileConfig{Settings: defaultSettings(), Rules: []Rule{r}}); err != nil {
		t.Fatal(err)
	}
	load := func() Identity {
		t.Helper()
		m := New()
		if err := m.Init(module.Env{Store: store, Logs: logbuf.New(50, nil)}); err != nil {
			t.Fatal(err)
		}
		return m.cfg.Rules[0].Identities[0]
	}
	first := load()
	if first.ID == "" || first.EncKey == "" || first.HMACKey == "" {
		t.Fatalf("应生成 ID 与密钥: %+v", first)
	}
	if second := load(); second != first {
		t.Fatalf("重启后身份应不变: %+v / %+v", first, second)
	}
}

// BenchmarkForgedPacketIdentities 测量规则带大量身份时伪造报文的处理成本（每个密钥一次 HMAC）。
func BenchmarkForgedPacketIdentities(b *testing.B) {
	for _, n := range []int{1, 16, 64} {
		b.Run(fmt.Sprintf("keys=%d", n), func(b *testing.B) {
			r := sshRule("r")
			r.SharedKeyOff = true
			for i := range n {
				r.Identities = append(r.Identities, Identity{Name: fmt.Sprintf("u%d", i), Enable: true})
			}
			env := newEnv(b, r)
			env.m.stateMu.Lock()
			env.m.bucket = tokenBucket{rate: 1e12, burst: 1e12}
			env.m.stateMu.Unlock()
			pkt := forgedPacket(200)
			src := netip.MustParseAddr("198.51.100.9")
			b.ResetTimer()
			for range b.N {
				env.clock.set(env.clock.now().Add(time.Second))
				_ = env.m.handle(pkt, src, transportUDP, "UDP")
			}
		})
	}
}

// 单地址放行下同一身份从多个地址并发敲门：最终恰好保留一个地址。
func TestSingleAddrConcurrent(t *testing.T) {
	r := teamRule("ssh")
	r.SingleAddr = true
	env := newEnv(t, r)
	r = env.rule("r-ssh")
	const n = 16
	pkts := make([][]byte, n)
	for i := range pkts {
		pkts[i] = env.identityPacket(t, r, "alice", spa.Message{})
	}
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = env.m.handle(pkts[i], netip.AddrFrom4([4]byte{203, 0, 113, byte(10 + i)}), transportTCP, "TCP")
		}()
	}
	wg.Wait()
	held := 0
	for _, g := range knockgate.Default.List() {
		if g.Rule == "r-ssh" && g.User == "alice" {
			held++
		}
	}
	if held != 1 {
		t.Fatalf("应只保留一个地址: %d", held)
	}
}
