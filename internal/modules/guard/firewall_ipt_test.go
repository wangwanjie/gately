package guard

import (
	"context"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// fakeIPT 模拟 iptables-legacy / ipset 命令：记录调用，按地址族与表返回 iptables-save 的输出。
type fakeIPT struct {
	mu    sync.Mutex
	calls []string
	// save 为各地址族 iptables-save -c -t mangle 的输出，键为命令名中的 iptables / ip6tables；
	// nat 表的输出键为 "iptables nat" / "ip6tables nat"，没有时视为 nat 表为空
	save map[string]string
	// ipsetSave 为 ipset save <集合> 的输出
	ipsetSave map[string]string
	fail      func(bin string, args []string) error
}

func (f *fakeIPT) run(_ context.Context, stdin, bin string, args ...string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, bin+" "+strings.Join(args, " ")+"\n"+stdin)
	if f.fail != nil {
		if err := f.fail(bin, args); err != nil {
			return "", err
		}
	}
	switch {
	case strings.HasSuffix(bin, "-save"):
		key := strings.TrimSuffix(bin, "-legacy-save")
		if slices.Contains(args, "nat") {
			if out, ok := f.save[key+" nat"]; ok {
				return out, nil
			}
			return "*nat\n:PREROUTING ACCEPT [0:0]\nCOMMIT\n", nil
		}
		return f.save[key], nil
	case bin == "ipset" && len(args) == 2 && args[0] == "save":
		return f.ipsetSave[args[1]], nil
	}
	return "", nil
}

// find 返回包含全部片段的调用。
func (f *fakeIPT) find(parts ...string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		ok := true
		for _, p := range parts {
			if !strings.Contains(c, p) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, c)
		}
	}
	return out
}

func (f *fakeIPT) reset() {
	f.mu.Lock()
	f.calls = nil
	f.mu.Unlock()
}

func newFakeIPTBackend(fake *fakeIPT) *iptBackend {
	return &iptBackend{
		run:   fake.run,
		ipset: "ipset",
		fams: []iptFamily{
			{bin: "iptables-legacy", save: "iptables-legacy-save", restore: "iptables-legacy-restore"},
			{v6: true, bin: "ip6tables-legacy", save: "ip6tables-legacy-save", restore: "ip6tables-legacy-restore"},
		},
		version: "iptables v1.8.10 (legacy), ipset v7.21",
	}
}

// goodSave 返回规则已正确写入时 mangle 表 iptables-save 的输出：自有链齐全，跳转在第一位。
func goodSave(extraPre string) string {
	var b strings.Builder
	b.WriteString("*mangle\n:PREROUTING ACCEPT [0:0]\n:INPUT ACCEPT [0:0]\n")
	for _, c := range iptChains {
		b.WriteString(":" + c + " - [0:0]\n")
	}
	b.WriteString(extraPre)
	b.WriteString("[1:60] -A PREROUTING -j GATELY_PRE\n")
	b.WriteString("[7:420] -A GATELY_PRE -m set --match-set gately_ban4 src -j DROP\n")
	b.WriteString("[3:180] -A GATELY_DENY -s 198.51.100.0/24 -j DROP\n")
	b.WriteString("COMMIT\n")
	return b.String()
}

// goodNat 返回重定向规则已写入时 nat 表 iptables-save 的输出：自有链齐全，跳转在 Docker 之后。
func goodNat() string {
	var b strings.Builder
	b.WriteString("*nat\n:PREROUTING ACCEPT [0:0]\n:DOCKER - [0:0]\n")
	for _, c := range iptNatChains {
		b.WriteString(":" + c + " - [0:0]\n")
	}
	b.WriteString("[9:540] -A PREROUTING -m addrtype --dst-type LOCAL -j DOCKER\n[4:240] -A PREROUTING -j GATELY_NAT\nCOMMIT\n")
	return b.String()
}

// legacySave 返回旧版本写入的 mangle 表：INPUT 跳到 GATELY_IN，诱饵端口与扫描检测在内核中直接封禁。
func legacySave() string {
	return "*mangle\n:PREROUTING ACCEPT [0:0]\n:INPUT ACCEPT [0:0]\n:GATELY_PRE - [0:0]\n:GATELY_DENY - [0:0]\n:GATELY_IN - [0:0]\n:GATELY_TRAP - [0:0]\n:GATELY_SCAN - [0:0]\n:GATELY_SCANHIT - [0:0]\n" +
		"[1:60] -A PREROUTING -j GATELY_PRE\n[0:0] -A INPUT -j GATELY_IN\n[2:120] -A GATELY_PRE -m set --match-set gately_trap4 src -j DROP\n" +
		"[4:240] -A GATELY_TRAP -j SET --add-set gately_trap4 src --exist --timeout 3600\nCOMMIT\n"
}

func TestMultiportChunks(t *testing.T) {
	var list []portRange
	for p := 1; p <= 16; p++ {
		list = append(list, portRange{p * 10, p * 10})
	}
	list = append(list, portRange{1000, 2000})
	got := multiportChunks(list)
	want := []string{"10,20,30,40,50,60,70,80,90,100,110,120,130,140,150", "160,1000:2000"}
	if !slices.Equal(got, want) {
		t.Fatalf("分组不符:\n%v\n%v", got, want)
	}
	// 范围占两个位置：14 个单端口之后放不下一个范围
	list = nil
	for p := 1; p <= 14; p++ {
		list = append(list, portRange{p, p})
	}
	list = append(list, portRange{100, 200})
	if got := multiportChunks(list); len(got) != 2 || got[1] != "100:200" {
		t.Fatalf("范围应放到下一组: %v", got)
	}
	if got := multiportChunks(nil); len(got) != 0 {
		t.Fatalf("空列表不应生成分组: %v", got)
	}
}

func TestIPTRules(t *testing.T) {
	s := defaultSettings()
	s.Trap.Enable, s.Trap.BanSeconds = true, 600
	s.Scan.Enable, s.Scan.Threshold = true, 2000
	allow := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("fd00::/8")}
	deny := []netip.Prefix{netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("2001:db8::/32")}
	open := []portRange{{22, 22}, {8000, 8100}}
	trap := []portRange{{23, 23}}
	rp := redirPorts{trap: 40001, scan: 40002}

	// mangle 表只丢弃封禁集合与网段封禁
	r4 := iptRules(false, deny)
	pre := strings.Join(r4[iptPre], "\n")
	for _, want := range []string{"-A GATELY_PRE -i lo -j RETURN", "--ctdir REPLY -j RETURN", "--match-set gately_ban4 src -j DROP", "-A GATELY_PRE -j GATELY_DENY"} {
		if !strings.Contains(pre, want) {
			t.Errorf("GATELY_PRE 缺少 %q:\n%s", want, pre)
		}
	}
	if strings.Contains(pre, "trap") || strings.Contains(pre, "scan") {
		t.Errorf("内核不应凭 SYN 丢弃诱饵或扫描来源:\n%s", pre)
	}
	if !slices.Equal(r4[iptDeny], []string{"-A GATELY_DENY -s 198.51.100.0/24 -j DROP"}) {
		t.Errorf("IPv4 网段封禁不符: %v", r4[iptDeny])
	}
	if !slices.Equal(iptRules(true, deny)[iptDeny], []string{"-A GATELY_DENY -s 2001:db8::/32 -j DROP"}) {
		t.Errorf("IPv6 网段封禁不符")
	}

	// nat 表把诱饵端口与可疑来源的新连接重定向给 Gately 确认握手
	n4 := iptNatRules(s, false, allow, open, trap, rp)
	nat := strings.Join(n4[iptNat], "\n")
	for _, want := range []string{"-A GATELY_NAT -i lo -j RETURN", "! -p tcp -j RETURN", "-p tcp ! --syn -j RETURN", "-m addrtype ! --dst-type LOCAL -j RETURN",
		"-s 10.0.0.0/8 -j RETURN", "-p tcp -m multiport --dports 23 -j REDIRECT --to-ports 40001", "-p tcp -j GATELY_SCAN"} {
		if !strings.Contains(nat, want) {
			t.Errorf("GATELY_NAT 缺少 %q:\n%s", want, nat)
		}
	}
	if strings.Contains(nat, "fd00::") {
		t.Error("IPv4 链不应包含 IPv6 白名单")
	}
	if strings.Index(nat, "-s 10.0.0.0/8 -j RETURN") > strings.Index(nat, "REDIRECT") {
		t.Errorf("白名单放行应在重定向之前:\n%s", nat)
	}
	scan := strings.Join(n4[iptScan], "\n")
	for _, want := range []string{"--dports 22,8000:8100 -j RETURN", "-m set --match-set gately_sus4 src -j REDIRECT --to-ports 40002", "--hitcount 255 -j GATELY_SUS"} {
		if !strings.Contains(scan, want) {
			t.Errorf("GATELY_SCAN 缺少 %q（阈值超过上限时按 255）:\n%s", want, scan)
		}
	}
	sus := strings.Join(n4[iptSus], "\n")
	if !strings.Contains(sus, "-j SET --add-set gately_sus4 src --exist --timeout 600") || !strings.Contains(sus, "-p tcp -j REDIRECT --to-ports 40002") {
		t.Errorf("GATELY_SUS 应加入可疑来源集合并重定向:\n%s", sus)
	}
	if strings.Contains(nat+scan+sus, "DROP") {
		t.Error("nat 表不应丢弃任何连接")
	}

	// IPv6：按 /64 计数；严格模式第一次即成为可疑来源
	n6 := iptNatRules(s, true, allow, open, trap, rp)
	if !strings.Contains(strings.Join(n6[iptScan], "\n"), "--mask "+v6Mask) || !strings.Contains(strings.Join(n6[iptNat], "\n"), "-s fd00::/8 -j RETURN") {
		t.Errorf("IPv6 计数应按 /64: %v", n6)
	}
	s.Scan.IPv6Strict = true
	n6 = iptNatRules(s, true, allow, open, trap, rp)
	last := n6[iptScan][len(n6[iptScan])-1]
	if last != "-A GATELY_SCAN -j GATELY_SUS" || strings.Contains(strings.Join(n6[iptScan], "\n"), "recent") {
		t.Errorf("严格模式应直接成为可疑来源: %v", n6[iptScan])
	}

	// 诱饵端口与扫描检测都关闭、或没有确认握手的监听端口时没有 nat 规则
	s.Trap.Enable, s.Scan.Enable = false, false
	if r := iptNatRules(s, false, allow, open, trap, rp); len(r) != 0 {
		t.Errorf("功能关闭时不应生成 nat 规则: %v", r)
	}
	s.Trap.Enable = true
	if r := iptNatRules(s, false, allow, open, trap, redirPorts{}); len(r) != 0 {
		t.Errorf("没有监听端口时不应生成 nat 规则: %v", r)
	}
	// 声明的链会被清空：只重写 GATELY_DENY 时只声明它
	text := renderRestore("mangle", []string{iptDeny}, r4)
	if strings.Contains(text, ":GATELY_PRE") || !strings.HasPrefix(text, "*mangle\n:GATELY_DENY - [0:0]\n") || !strings.HasSuffix(text, "COMMIT\n") {
		t.Errorf("restore 内容不符:\n%s", text)
	}
	if text := renderRestore("nat", iptNatChains, n4); !strings.HasPrefix(text, "*nat\n:GATELY_NAT - [0:0]\n:GATELY_SCAN - [0:0]\n:GATELY_SUS - [0:0]\n") {
		t.Errorf("nat 表 restore 内容不符:\n%s", text)
	}
}

func TestIPTCountersAndJumps(t *testing.T) {
	chains, lines := parseSave(goodSave(""))
	if !chains[iptDeny] || !chains["PREROUTING"] {
		t.Fatalf("应识别声明的链: %v", chains)
	}
	_, padded := parseSave("[      5:       292] -A GATELY_DENY -s 192.168.18.8/32 -j DROP\n")
	if len(padded) != 1 || padded[0].packets != 5 || padded[0].chain != iptDeny {
		t.Fatalf("应解析补了空格的计数: %+v", padded)
	}
	d, h := iptCounters(lines)
	if d["ban"] != 7 || d["deny"] != 3 || len(h) != 0 {
		t.Fatalf("计数不符: %v %v", d, h)
	}

	fake := &fakeIPT{save: map[string]string{"iptables": goodSave(""), "ip6tables": goodSave("")}}
	b := newFakeIPTBackend(fake)
	if fixed, err := b.ensureJumps(context.Background()); err != nil || len(fixed) != 0 || len(fake.find(" -I ")) != 0 {
		t.Fatalf("跳转已在第一位时不应改动: %v %v %v", fixed, err, fake.calls)
	}

	// 其他程序把规则插到了 PREROUTING 最前面：删除原跳转后插回第一位
	fake = &fakeIPT{save: map[string]string{"iptables": goodSave("[0:0] -A PREROUTING -j OTHER\n"), "ip6tables": goodSave("")}}
	b = newFakeIPTBackend(fake)
	fixed, err := b.ensureJumps(context.Background())
	if err != nil || !slices.Equal(fixed, []string{"PREROUTING"}) {
		t.Fatalf("应挪回 PREROUTING 的跳转: %v %v", fixed, err)
	}
	if len(fake.find("iptables-legacy -w -t mangle -D PREROUTING -j GATELY_PRE")) != 1 || len(fake.find("iptables-legacy -w -t mangle -I PREROUTING 1 -j GATELY_PRE")) != 1 {
		t.Fatalf("应先删除再插入第一位: %v", fake.calls)
	}
	if len(fake.find("ip6tables-legacy -w")) != 0 {
		t.Fatal("IPv6 的跳转已在第一位，不应改动")
	}

	// 跳转缺失时直接插入，不计为挪动
	fake = &fakeIPT{save: map[string]string{"iptables": "*mangle\n:PREROUTING ACCEPT [0:0]\nCOMMIT\n", "ip6tables": goodSave("")}}
	b = newFakeIPTBackend(fake)
	fixed, _ = b.ensureJumps(context.Background())
	if len(fixed) != 0 || len(fake.find("-I PREROUTING 1 -j GATELY_PRE")) != 1 || len(fake.find(" -D ")) != 0 {
		t.Fatalf("缺失的跳转应直接插入: %v %v", fixed, fake.calls)
	}

	// nat 表：跳转只要求存在，缺失时追加到末尾（排在 Docker 之后），重复时删掉后追加一条
	fake = &fakeIPT{}
	b = newFakeIPTBackend(fake)
	err = b.fixNatJump(context.Background(), map[bool]string{false: goodNat(), true: "*nat\n:PREROUTING ACCEPT [0:0]\n-A PREROUTING -j GATELY_NAT\n-A PREROUTING -j GATELY_NAT\nCOMMIT\n"})
	if err != nil || len(fake.find("iptables-legacy -w -t nat")) != 0 {
		t.Fatalf("IPv4 的 nat 跳转已存在，不应改动: %v %v", err, fake.calls)
	}
	if len(fake.find("ip6tables-legacy -w -t nat -D PREROUTING -j GATELY_NAT")) != 2 || len(fake.find("ip6tables-legacy -w -t nat -A PREROUTING -j GATELY_NAT")) != 1 {
		t.Fatalf("重复的 nat 跳转应删掉后追加一条: %v", fake.calls)
	}
}

// iptFirewall 返回使用模拟 iptables 后端的防火墙
func iptFirewall(t *testing.T, fake *fakeIPT) (*firewall, *logbuf.Buffer) {
	t.Helper()
	buf := logbuf.New(200, nil)
	fw := newFirewall(buf.Logger("fw"))
	fw.ipt = newFakeIPTBackend(fake)
	t.Cleanup(func() { stopHandshakeForTest(fw) })
	return fw, buf
}

// 内核没有 nf_tables 时自动改用 iptables + ipset，写入当前封禁
func TestFirewallSwitchesToIPTables(t *testing.T) {
	cleanDefault(t)
	banlist.Default.Ban("203.0.113.5", banlist.Options{Duration: time.Hour})
	banlist.Default.Ban("2001:db8:1:2::/64", banlist.Options{Duration: time.Hour})
	banlist.Default.Ban("198.51.100.0/24", banlist.Options{Duration: time.Hour})
	nft := &fakeNFT{fail: func(_ string, args []string) error {
		if len(args) > 0 && args[0] == "--version" {
			return nil
		}
		return errorString(dsmError)
	}}
	fw, buf := inactiveFirewall(t, nft)
	ipt := &fakeIPT{save: map[string]string{"iptables": goodSave(""), "ip6tables": goodSave("")}}
	old := findIPTFn
	findIPTFn = func() *iptBackend { return newFakeIPTBackend(ipt) }
	t.Cleanup(func() { findIPTFn = old })

	s := defaultSettings()
	s.Firewall = true
	fw.apply(context.Background(), s)
	st := fw.snapshot()
	if !st.Active || st.Backend != BackendIPT || st.Reason != "" || !strings.Contains(st.Version, "legacy") {
		t.Fatalf("应改用 iptables 并生效: %+v", st)
	}
	restore := ipt.find("ipset restore -exist", "create gately_ban4 hash:ip family inet timeout 0")
	if len(restore) != 1 {
		t.Fatalf("应创建集合并写入封禁: %v", ipt.calls)
	}
	for _, want := range []string{"create gately_ban6 hash:ip family inet6 netmask 64 timeout 0", "add gately_ban4 203.0.113.5 timeout", "add gately_ban6 2001:db8:1:2:: timeout"} {
		if !strings.Contains(restore[0], want) {
			t.Errorf("ipset 内容缺少 %q:\n%s", want, restore[0])
		}
	}
	if len(ipt.find("iptables-legacy-restore --noflush", "-A GATELY_DENY -s 198.51.100.0/24 -j DROP")) != 1 {
		t.Errorf("网段封禁应写入 GATELY_DENY: %v", ipt.calls)
	}
	// 先清空自有链再销毁集合，避免集合仍被规则引用
	first := slices.IndexFunc(ipt.calls, func(c string) bool { return strings.Contains(c, "ipset destroy") })
	flushAt := slices.IndexFunc(ipt.calls, func(c string) bool { return strings.Contains(c, "-restore --noflush") })
	if flushAt < 0 || first < flushAt {
		t.Errorf("应先清空链再销毁集合: %v", ipt.calls)
	}
	warns := logsAt(buf, "warn")
	if len(warns) != 0 {
		t.Errorf("改用 iptables 后不应再提示不支持: %v", warns)
	}
	if infos := strings.Join(logsAt(buf, "info"), "\n"); !strings.Contains(infos, "改用 iptables") || !strings.Contains(infos, "已写入 iptables") {
		t.Errorf("应记录改用 iptables 与写入成功: %s", infos)
	}
	// 之后的保存设置直接使用 iptables，不再调用 nft
	nftCalls := nft.countCalls("")
	fw.apply(context.Background(), s)
	if nft.countCalls("") != nftCalls || !fw.snapshot().Active {
		t.Fatal("切换后保存设置不应再调用 nft")
	}
}

// 没有 nft 命令时直接使用 iptables；两者都没有时为找不到命令
func TestFirewallIPTablesWhenNFTMissing(t *testing.T) {
	cleanDefault(t)
	ipt := &fakeIPT{save: map[string]string{"iptables": goodSave(""), "ip6tables": goodSave("")}}
	fw, _ := iptFirewall(t, ipt)
	s := defaultSettings()
	s.Firewall = true
	fw.apply(context.Background(), s)
	if st := fw.snapshot(); !st.Active || st.Backend != BackendIPT {
		t.Fatalf("应使用 iptables: %+v", st)
	}
	// 写入失败按错误处理并定时重试
	ipt.fail = func(bin string, _ []string) error {
		if bin == "ipset" {
			return errorString("ipset v7.21: Kernel error received: Operation not supported")
		}
		return nil
	}
	fw.apply(context.Background(), s)
	if st := fw.snapshot(); st.Active || st.Reason != ReasonError {
		t.Fatalf("写入失败应为错误: %+v", st)
	}
	ipt.fail = func(string, []string) error {
		return errorString("iptables v1.8.10 (legacy): can't initialize iptables table `mangle': Permission denied (you must be root)")
	}
	fw.apply(context.Background(), s)
	if st := fw.snapshot(); st.Reason != ReasonPermission {
		t.Fatalf("没有权限时应为权限不足: %+v", st)
	}
}

// 封禁变化增量写入集合与 GATELY_DENY；停止时删除跳转、链与集合
func TestFirewallIPTFlushAndStop(t *testing.T) {
	cleanDefault(t)
	ipt := &fakeIPT{save: map[string]string{"iptables": goodSave(""), "ip6tables": goodSave("")}}
	fw, _ := iptFirewall(t, ipt)
	unsub := banlist.Default.Subscribe(fw.change)
	defer unsub()
	ctx := context.Background()
	s := defaultSettings()
	s.Firewall = true
	fw.apply(ctx, s)

	ipt.reset()
	banlist.Default.Ban("1.2.3.4", banlist.Options{Duration: time.Hour})
	fw.flush(ctx)
	if len(ipt.find("ipset restore -exist", "add gately_ban4 1.2.3.4 timeout")) != 1 || len(ipt.find("-restore")) != 0 {
		t.Fatalf("单个来源只应写入集合: %v", ipt.calls)
	}
	// 延长时长：先删后加
	ipt.reset()
	banlist.Default.Ban("1.2.3.4", banlist.Options{Duration: 3 * time.Hour})
	fw.flush(ctx)
	if len(ipt.find("del gately_ban4 1.2.3.4\nadd gately_ban4 1.2.3.4 timeout")) != 1 {
		t.Fatalf("延长封禁应先删后加: %v", ipt.calls)
	}
	// 永久封禁与网段：从集合移到 GATELY_DENY
	ipt.reset()
	banlist.Default.Ban("1.2.3.4", banlist.Options{Permanent: true})
	banlist.Default.Ban("2001:db8::/48", banlist.Options{Duration: time.Hour})
	fw.flush(ctx)
	if len(ipt.find("del gately_ban4 1.2.3.4")) != 1 {
		t.Errorf("永久封禁应从集合删除: %v", ipt.calls)
	}
	if len(ipt.find("iptables-legacy-restore --noflush", ":GATELY_DENY - [0:0]", "-s 1.2.3.4/32 -j DROP")) != 1 || len(ipt.find("ip6tables-legacy-restore --noflush", "-s 2001:db8::/48 -j DROP")) != 1 {
		t.Errorf("网段与永久封禁应写入 GATELY_DENY: %v", ipt.calls)
	}
	// 内容不变时不重写
	ipt.reset()
	fw.mu.Lock()
	fw.dirty = true
	fw.mu.Unlock()
	fw.flush(ctx)
	if len(ipt.find("-restore")) != 0 {
		t.Errorf("GATELY_DENY 内容不变时不应重写: %v", ipt.calls)
	}

	ipt.reset()
	ipt.mu.Lock()
	ipt.save["iptables nat"] = goodNat()
	ipt.mu.Unlock()
	fw.stop()
	for _, want := range []string{"-t mangle -D PREROUTING -j GATELY_PRE", "-t mangle -X GATELY_DENY", "-t nat -D PREROUTING -j GATELY_NAT", "-t nat -X GATELY_SUS", "ipset destroy gately_ban6", "ipset destroy gately_sus4"} {
		if len(ipt.find(want)) == 0 {
			t.Errorf("停止时应执行 %q: %v", want, ipt.calls)
		}
	}
	if fw.active() {
		t.Error("停止后不应生效")
	}
}

// 开启诱饵端口与扫描检测：nat 表写入指向确认握手端口的重定向，跳转追加在 PREROUTING 末尾；
// 轮询不再从集合导入来源；mangle 或 nat 的自有链被外部清除时完整重写
func TestFirewallIPTPollAndRedirect(t *testing.T) {
	cleanDefault(t)
	ipt := &fakeIPT{save: map[string]string{"iptables": goodSave(""), "ip6tables": goodSave("")}}
	fw, buf := iptFirewall(t, ipt)
	imported := 0
	fw.importFn = func(string, []netip.Addr, portHits) { imported++ }
	unsub := banlist.Default.Subscribe(fw.change)
	defer unsub()
	old := listeningPortsFn
	listeningPortsFn = func() (map[int]bool, error) { return map[int]bool{22: true}, nil }
	t.Cleanup(func() { listeningPortsFn = old })
	ctx := context.Background()
	s := defaultSettings()
	s.Firewall = true
	s.Trap.Enable, s.Trap.Ports = true, "23"
	s.Scan.Enable = true
	fw.apply(ctx, s)
	defer fw.stop()
	st := fw.snapshot()
	if !st.Active || !st.Redirect || !fw.kernelTrap() {
		t.Fatalf("应生效且重定向可用: %+v", st)
	}
	fw.mu.Lock()
	ports := fw.hs.ports
	fw.mu.Unlock()
	natRestore := ipt.find("iptables-legacy-restore --noflush", "*nat", ":GATELY_NAT - [0:0]")
	if len(natRestore) != 1 || !strings.Contains(natRestore[0], "--dports 23 -j REDIRECT --to-ports "+strconv.Itoa(ports.trap)) ||
		!strings.Contains(natRestore[0], "-j REDIRECT --to-ports "+strconv.Itoa(ports.scan)) {
		t.Fatalf("nat 表应写入指向确认握手端口 %+v 的重定向: %v", ports, ipt.calls)
	}
	if len(ipt.find("iptables-legacy -w -t nat -A PREROUTING -j GATELY_NAT")) != 1 {
		t.Fatalf("nat 跳转应追加到 PREROUTING 末尾: %v", ipt.calls)
	}
	if len(ipt.find("create gately_sus4 hash:ip family inet timeout 600 maxelem 65536")) != 1 || len(ipt.find("create gately_sus6 hash:ip family inet6 netmask 64 timeout 600 maxelem 65536")) != 1 {
		t.Fatalf("应创建有容量上限的可疑来源集合: %v", ipt.calls)
	}

	ipt.mu.Lock()
	ipt.save["iptables nat"], ipt.save["ip6tables nat"] = goodNat(), goodNat()
	ipt.mu.Unlock()
	ipt.reset()
	fw.poll(ctx)
	if imported != 0 || len(ipt.find("ipset save")) != 0 {
		t.Fatalf("轮询不应再从集合导入来源: %d %v", imported, ipt.calls)
	}
	if st := fw.snapshot(); st.Dropped["ban"] != 14 {
		t.Fatalf("计数应为 IPv4 与 IPv6 之和: %+v", st)
	}

	// nat 表的自有链被清除：重新写入
	ipt.mu.Lock()
	delete(ipt.save, "iptables nat")
	ipt.mu.Unlock()
	ipt.reset()
	fw.poll(ctx)
	if len(ipt.find("ipset restore -exist", "create gately_ban4")) != 1 || len(ipt.find("*nat", ":GATELY_SUS - [0:0]")) == 0 {
		t.Fatalf("nat 链被清除后应重新写入: %v", ipt.calls)
	}
	// mangle 的自有链被清除：重新写入
	ipt.mu.Lock()
	ipt.save["iptables nat"] = goodNat()
	ipt.save["iptables"] = "*mangle\n:PREROUTING ACCEPT [0:0]\nCOMMIT\n"
	ipt.mu.Unlock()
	ipt.reset()
	fw.poll(ctx)
	if len(ipt.find("ipset restore -exist", "create gately_ban4")) != 1 {
		t.Fatalf("链被清除后应重新写入: %v", ipt.calls)
	}
	if !strings.Contains(strings.Join(logsAt(buf, "warn"), "\n"), "已被外部清除") {
		t.Error("应记录规则被外部清除")
	}
}

// 内核缺少 nat 表的 REDIRECT 目标（如精简内核）时只写 mangle 部分：封禁照常生效，诱饵端口交回 Gately 直接监听
func TestIPTRedirectFallback(t *testing.T) {
	cleanDefault(t)
	ipt := &fakeIPT{save: map[string]string{"iptables": goodSave(""), "ip6tables": goodSave("")}}
	fw, buf := iptFirewall(t, ipt)
	// nat 表的 restore 失败
	b := fw.ipt
	orig := b.run
	b.run = func(ctx context.Context, stdin, bin string, args ...string) (string, error) {
		if strings.HasPrefix(stdin, "*nat") {
			return "", errorString("iptables-restore v1.8.10 (legacy): Couldn't load target `REDIRECT':No such file or directory")
		}
		return orig(ctx, stdin, bin, args...)
	}
	s := defaultSettings()
	s.Firewall = true
	s.Trap.Enable, s.Trap.Ports = true, "23"
	fw.apply(context.Background(), s)
	defer fw.stop()
	st := fw.snapshot()
	if !st.Active || st.Redirect || fw.kernelTrap() {
		t.Fatalf("缺少 REDIRECT 时封禁应生效、重定向不可用: %+v", st)
	}
	if !strings.Contains(strings.Join(logsAt(buf, "warn"), "\n"), "REDIRECT") {
		t.Errorf("应提示缺少重定向: %v", logsAt(buf, "warn"))
	}
}

// 升级：旧版本在 mangle INPUT 中的跳转、链与诱饵 / 扫描 / 端口集合在重建时清理
func TestIPTLegacyCleanup(t *testing.T) {
	cleanDefault(t)
	ipt := &fakeIPT{save: map[string]string{"iptables": legacySave(), "ip6tables": goodSave("")}}
	fw, _ := iptFirewall(t, ipt)
	s := defaultSettings()
	s.Firewall = true
	fw.apply(context.Background(), s)
	if !fw.snapshot().Active {
		t.Fatal("应生效")
	}
	for _, want := range []string{"iptables-legacy -w -t mangle -D INPUT -j GATELY_IN", "iptables-legacy -w -t mangle -F GATELY_SCANHIT", "iptables-legacy -w -t mangle -X GATELY_IN",
		"iptables-legacy -w -t mangle -X GATELY_TRAP", "ipset destroy gately_trap4", "ipset destroy gately_scan6", "ipset destroy gately_port4"} {
		if len(ipt.find(want)) == 0 {
			t.Errorf("应清理旧版本的 %q: %v", want, ipt.calls)
		}
	}
	if len(ipt.find("ip6tables-legacy -w -t mangle -X GATELY_IN")) != 0 {
		t.Error("IPv6 没有旧链，不应删除")
	}
}

// 端口变化与白名单变化只重写 input 部分的链
func TestFirewallIPTRefreshPorts(t *testing.T) {
	cleanDefault(t)
	ipt := &fakeIPT{save: map[string]string{"iptables": goodSave(""), "ip6tables": goodSave("")}}
	fw, _ := iptFirewall(t, ipt)
	listening := map[int]bool{22: true}
	old := listeningPortsFn
	listeningPortsFn = func() (map[int]bool, error) { return listening, nil }
	t.Cleanup(func() { listeningPortsFn = old })
	ctx := context.Background()
	s := defaultSettings()
	s.Firewall = true
	s.Trap.Enable, s.Trap.Ports = true, "23,2323"
	s.Scan.Enable = true
	fw.apply(ctx, s)

	// 2323 开始监听：从诱饵端口中排除，并视为开放端口
	listening[2323] = true
	ipt.reset()
	fw.mu.Lock()
	fw.refreshPortsLocked(ctx)
	fw.mu.Unlock()
	calls := ipt.find("iptables-legacy-restore --noflush", "*nat", ":GATELY_NAT - [0:0]")
	if len(calls) != 1 || len(ipt.find("*mangle")) != 0 || !strings.Contains(calls[0], "--dports 23 -j REDIRECT") || strings.Contains(calls[0], "--dports 23,2323") || !strings.Contains(calls[0], "--dports 22,2323 -j RETURN") {
		t.Fatalf("应只重写 nat 表的链: %v", ipt.calls)
	}
	ipt.reset()
	fw.setLocal(ctx, []netip.Prefix{netip.MustParsePrefix("2001:db8:5::/64")})
	if len(ipt.find("ip6tables-legacy-restore --noflush", "*nat", "-s 2001:db8:5::/64 -j RETURN", "--dports 23 -j REDIRECT")) != 1 {
		t.Fatalf("白名单变化应重写 nat 链并保留当前端口: %v", ipt.calls)
	}
	fw.stop()
}

// 集合被其他规则引用、无法销毁时清空后沿用，不让整次写入失败
func TestIPTRebuildSetInUse(t *testing.T) {
	cleanDefault(t)
	ipt := &fakeIPT{save: map[string]string{"iptables": goodSave(""), "ip6tables": goodSave("")}}
	ipt.fail = func(bin string, args []string) error {
		if bin == "ipset" && len(args) == 2 && args[0] == "destroy" && args[1] == "gately_ban6" {
			return errorString("ipset v7.24: Set cannot be destroyed: it is in use by a kernel component")
		}
		return nil
	}
	fw, _ := iptFirewall(t, ipt)
	s := defaultSettings()
	s.Firewall = true
	fw.apply(context.Background(), s)
	if st := fw.snapshot(); !st.Active {
		t.Fatalf("集合被占用时应清空后继续: %+v", st)
	}
	if len(ipt.find("ipset restore -exist", "create gately_ban6 hash:ip family inet6 netmask 64 timeout 0", "flush gately_ban6")) != 1 {
		t.Fatalf("应创建并清空被占用的集合: %v", ipt.calls)
	}
}

// 剩余时间超过 ipset 上限（约 24.8 天）的封禁按上限写入，临近到期时由对账按剩余时间重新写入；集合容量为 iptMaxElem
func TestIPTLongBanCapped(t *testing.T) {
	cleanDefault(t)
	banlist.Default.Ban("203.0.113.9", banlist.Options{Duration: 30 * 24 * time.Hour})
	banlist.Default.Ban("203.0.113.10", banlist.Options{Duration: time.Hour})
	ipt := &fakeIPT{save: map[string]string{"iptables": goodSave(""), "ip6tables": goodSave("")}}
	fw, _ := iptFirewall(t, ipt)
	s := defaultSettings()
	s.Firewall = true
	fw.apply(context.Background(), s)
	if st := fw.snapshot(); !st.Active {
		t.Fatalf("应生效: %+v", st)
	}
	restore := ipt.find("ipset restore -exist", "create gately_ban4")
	if len(restore) != 1 {
		t.Fatalf("应写入集合: %v", ipt.calls)
	}
	for _, want := range []string{"add gately_ban4 203.0.113.9 timeout 2147483\n", "maxelem 65536"} {
		if !strings.Contains(restore[0], want) {
			t.Fatalf("ipset 内容缺少 %q:\n%s", want, restore[0])
		}
	}
	if strings.Contains(restore[0], "203.0.113.10 timeout 2147483") {
		t.Fatalf("短期封禁不应按上限写入:\n%s", restore[0])
	}
	fw.mu.Lock()
	exp, ok := fw.capped[banlist.KeyFor(netip.MustParseAddr("203.0.113.9"))]
	fw.mu.Unlock()
	if !ok || time.Until(exp) > iptMaxTimeout*time.Second || time.Until(exp) < iptMaxTimeout*time.Second-time.Minute {
		t.Fatalf("应记录按上限写入的到期时间: %v %v", ok, exp)
	}

	// 未临近到期时轮询不触发重写
	ipt.reset()
	fw.mu.Lock()
	fw.pollIPTLocked(context.Background())
	fw.mu.Lock()
	dirty := fw.dirty
	fw.mu.Unlock()
	fw.flush(context.Background())
	if dirty || len(ipt.find("ipset restore -exist", "203.0.113.9")) != 0 {
		t.Fatalf("未临近到期不应重写: dirty=%v %v", dirty, ipt.calls)
	}

	// 内核中的元素临近到期：轮询标记后对账先删后加，延续到真正的解封时间
	fw.mu.Lock()
	for k := range fw.capped {
		fw.capped[k] = time.Now().Add(time.Hour)
	}
	fw.mu.Unlock()
	ipt.reset()
	fw.mu.Lock()
	fw.pollIPTLocked(context.Background())
	fw.flush(context.Background())
	got := ipt.find("ipset restore -exist", "add gately_ban4 203.0.113.9 timeout 2147483")
	if len(got) != 1 || !strings.Contains(got[0], "del gately_ban4 203.0.113.9\n") {
		t.Fatalf("临近到期应重新写入: %v", ipt.calls)
	}
	if strings.Contains(got[0], "203.0.113.10") {
		t.Fatalf("其他封禁不应重写:\n%s", got[0])
	}
	fw.mu.Lock()
	exp = fw.capped[banlist.KeyFor(netip.MustParseAddr("203.0.113.9"))]
	fw.mu.Unlock()
	if time.Until(exp) < iptMaxTimeout*time.Second-time.Minute {
		t.Fatalf("重新写入后应更新到期时间: %v", exp)
	}
}

// 写入中途失败（IPv4 已写入、IPv6 出错）时尽力删除已写入的部分，不留下未生效却在拦截的规则
func TestIPTRebuildFailureCleansUp(t *testing.T) {
	cleanDefault(t)
	banlist.Default.Ban("198.51.100.0/24", banlist.Options{Duration: time.Hour})
	ipt := &fakeIPT{save: map[string]string{"iptables": goodSave(""), "ip6tables": goodSave("")}}
	// 第一次 IPv6 restore 是清空自有链，第二次是写入规则：只让第二次失败。
	// fail 在 fakeIPT 持锁时调用，不能再调用 find
	v6Restores := 0
	ipt.fail = func(bin string, _ []string) error {
		if bin == "ip6tables-legacy-restore" {
			v6Restores++
			if v6Restores == 2 {
				return errorString("ip6tables-restore v1.8.10 (legacy): line 3 failed")
			}
		}
		return nil
	}
	fw, _ := iptFirewall(t, ipt)
	s := defaultSettings()
	s.Firewall = true
	fw.apply(context.Background(), s)
	if st := fw.snapshot(); st.Active || st.Reason != ReasonError {
		t.Fatalf("写入失败应为错误: %+v", st)
	}
	for _, want := range [][]string{{"iptables-legacy -w -t mangle -D PREROUTING -j GATELY_PRE"}, {"iptables-legacy -w -t mangle -X GATELY_DENY"}, {"ipset destroy gately_ban4"}} {
		if len(ipt.find(want...)) == 0 {
			t.Errorf("失败后应清理 %v: %v", want, ipt.calls)
		}
	}
}

// 增量写入失败（例如与群晖防火墙同时修改规则）后由下一次轮询完整重写；支持时 iptables-restore 带 -w 等待锁
func TestIPTIncrementalFailureResyncs(t *testing.T) {
	cleanDefault(t)
	ipt := &fakeIPT{save: map[string]string{"iptables": goodSave(""), "ip6tables": goodSave("")}}
	fw, buf := iptFirewall(t, ipt)
	for i := range fw.ipt.fams {
		fw.ipt.fams[i].wait = true
	}
	s := defaultSettings()
	s.Firewall = true
	fw.apply(context.Background(), s)
	if !fw.snapshot().Active {
		t.Fatal("应生效")
	}
	if len(ipt.find("iptables-legacy-restore -w --noflush")) == 0 || len(ipt.find("iptables-legacy-restore --noflush")) != 0 {
		t.Fatalf("支持时 iptables-restore 应带 -w: %v", ipt.calls)
	}
	locked := true
	ipt.fail = func(bin string, _ []string) error {
		if locked && (bin == "ipset" || strings.HasSuffix(bin, "-restore")) {
			return errorString("Another app is currently holding the xtables lock")
		}
		return nil
	}
	banlist.Default.Ban("203.0.113.20", banlist.Options{Duration: time.Hour})
	banlist.Default.Ban("198.51.100.0/24", banlist.Options{Duration: time.Hour})
	fw.change(banlist.Change{})
	fw.flush(context.Background())
	fw.mu.Lock()
	resync := fw.resync
	fw.mu.Unlock()
	if !resync {
		t.Fatal("增量写入失败后应标记完整重写")
	}
	locked = false
	ipt.reset()
	fw.mu.Lock()
	fw.pollIPTLocked(context.Background())
	if !fw.snapshot().Active {
		t.Fatal("重写后应生效")
	}
	if len(ipt.find("ipset restore -exist", "add gately_ban4 203.0.113.20 timeout")) != 1 || len(ipt.find("-restore", "-A GATELY_DENY -s 198.51.100.0/24 -j DROP")) == 0 {
		t.Fatalf("轮询应完整重写并补上失败的变化: %v", ipt.calls)
	}
	fw.mu.Lock()
	resync = fw.resync
	fw.mu.Unlock()
	if resync {
		t.Fatal("重写成功后应清除标记")
	}
	if !strings.Contains(strings.Join(logsAt(buf, "info"), "\n"), "补上之前未写入的变化") {
		t.Errorf("应记录重新写入: %v", logsAt(buf, "info"))
	}
}

// 旧版 iptables-restore 不认识 -w，不带该参数
func TestIPTRestoreArgs(t *testing.T) {
	if got := (iptFamily{}).restoreArgs(); !slices.Equal(got, []string{"--noflush"}) {
		t.Fatalf("不支持 -w 时: %v", got)
	}
	if got := (iptFamily{wait: true}).restoreArgs(); !slices.Equal(got, []string{"-w", "--noflush"}) {
		t.Fatalf("支持 -w 时: %v", got)
	}
}

// 内核禁用 IPv6 时 ip6tables 读不了 mangle 表，只处理 IPv4，IPv4 照常写入
func TestIPTDropsUnusableV6(t *testing.T) {
	cleanDefault(t)
	banlist.Default.Ban("203.0.113.30", banlist.Options{Duration: time.Hour})
	ipt := &fakeIPT{save: map[string]string{"iptables": goodSave("")}}
	ipt.fail = func(bin string, _ []string) error {
		if strings.HasPrefix(bin, "ip6tables") {
			return errorString("ip6tables v1.8.10 (legacy): can't initialize ip6tables table `mangle': Address family not supported by protocol")
		}
		return nil
	}
	b := newFakeIPTBackend(ipt)
	b.dropUnusableV6(context.Background())
	if len(b.fams) != 1 || b.fams[0].v6 || b.hasV6() {
		t.Fatalf("应只保留 IPv4: %+v", b.fams)
	}
	fw, _ := iptFirewall(t, ipt)
	fw.ipt = b
	s := defaultSettings()
	s.Firewall = true
	fw.apply(context.Background(), s)
	if !fw.snapshot().Active || len(ipt.find("ipset restore -exist", "add gately_ban4 203.0.113.30")) != 1 {
		t.Fatalf("IPv4 应照常生效: %+v %v", fw.snapshot(), ipt.calls)
	}
	if len(ipt.find("ip6tables-legacy-restore")) != 0 || len(ipt.find("create gately_ban6")) != 0 {
		t.Fatalf("不应再写入 IPv6: %v", ipt.calls)
	}
}

// 网段封禁变化、端口变化只重写部分链，重写会把这些链的计数清零：重写前并入基数，累计计数不倒退
func TestIPTPartialRewriteKeepsCounts(t *testing.T) {
	cleanDefault(t)
	ipt := &fakeIPT{save: map[string]string{"iptables": goodSave(""), "ip6tables": "*mangle\n" + strings.Join(func() []string {
		var out []string
		for _, c := range iptChains {
			out = append(out, ":"+c+" - [0:0]")
		}
		return out
	}(), "\n") + "\n[0:0] -A PREROUTING -j GATELY_PRE\nCOMMIT\n"}}
	fw, _ := iptFirewall(t, ipt)
	s := defaultSettings()
	s.Firewall = true
	fw.apply(context.Background(), s)
	if !fw.snapshot().Active {
		t.Fatal("应生效")
	}
	// GATELY_DENY 计数为 3，新增网段封禁后重写 GATELY_DENY
	banlist.Default.Ban("192.0.2.0/24", banlist.Options{Duration: time.Hour})
	fw.change(banlist.Change{})
	fw.flush(context.Background())
	if len(ipt.find("-restore", "-A GATELY_DENY -s 192.0.2.0/24 -j DROP")) == 0 {
		t.Fatalf("应重写 GATELY_DENY: %v", ipt.calls)
	}
	// 重写后内核中 GATELY_DENY 的计数归零，其他链不变
	ipt.mu.Lock()
	ipt.save["iptables"] = strings.Replace(goodSave(""), "[3:180] -A GATELY_DENY", "[0:0] -A GATELY_DENY", 1)
	ipt.mu.Unlock()
	fw.mu.Lock()
	fw.pollIPTLocked(context.Background())
	st := fw.snapshot()
	if st.Dropped["deny"] != 3 || st.Dropped["ban"] != 7 {
		t.Fatalf("网段封禁的计数应保留且其他计数不重复累加: dropped=%v hits=%v", st.Dropped, st.Hits)
	}
}
