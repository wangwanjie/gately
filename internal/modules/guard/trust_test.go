package guard

import (
	"context"
	"net"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/trustsrc"
)

// stubPeers 替换读取已建立连接的函数，返回 peers 的当前内容。
func stubPeers(t *testing.T, peers map[int]map[netip.Addr]bool) {
	t.Helper()
	old := establishedPeersFn
	establishedPeersFn = func(map[int]bool) (map[int]map[netip.Addr]bool, error) { return peers, nil }
	t.Cleanup(func() { establishedPeersFn = old })
}

func TestParseEstablished(t *testing.T) {
	// 本机 17000 端口：一条 IPv4 已建立连接（14.19.55.154:31541）、一条 IPv4 映射的 IPv6 连接（203.0.113.7）、
	// 一条 IPv6 连接（2001:db8::1）；监听行、其他端口与其他状态的连接都应跳过
	tcp := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:4268 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 1 1 0000000000000000 100 0 0 10 0
   1: 8E3EF426:4268 9A37130E:7B35 01 00000000:00000000 02:000A7E6B 00000000     0        0 2 1 0000000000000000 20 4 30 10 -1
   2: 8E3EF426:01BB 0B0B0B0B:7B36 01 00000000:00000000 02:000A7E6B 00000000     0        0 3 1 0000000000000000 20 4 30 10 -1
   3: 8E3EF426:4268 0C0C0C0C:7B37 06 00000000:00000000 02:000A7E6B 00000000     0        0 4 1 0000000000000000 20 4 30 10 -1
`
	tcp6 := `  sl  local_address                         remote_address                        st tx_queue rx_queue
   0: 0000000000000000FFFF00008E3EF426:4268 0000000000000000FFFF0000077100CB:7B38 01 00000000:00000000 00:00000000
   1: 00000000000000000000000000000000:4268 B80D0120000000000000000001000000:7B39 01 00000000:00000000 00:00000000
`
	out := map[int]map[netip.Addr]bool{}
	ports := map[int]bool{17000: true}
	parseEstablished(strings.NewReader(tcp), ports, out)
	parseEstablished(strings.NewReader(tcp6), ports, out)
	want := []string{"14.19.55.154", "203.0.113.7", "2001:db8::1"}
	if len(out) != 1 || len(out[17000]) != len(want) {
		t.Fatalf("解析结果 = %v，应只有 17000 端口的 %v", out, want)
	}
	for _, w := range want {
		if !out[17000][netip.MustParseAddr(w)] {
			t.Errorf("缺少 %s: %v", w, out[17000])
		}
	}
}

func TestEffectiveTrusted(t *testing.T) {
	now := time.Now()
	home := netip.MustParseAddr("14.19.55.154")
	gone := netip.MustParseAddr("198.51.100.20")
	fresh := netip.MustParseAddr("2001:db8::7")
	stubPeers(t, map[int]map[netip.Addr]bool{17000: {home: true}})
	got := effectiveTrusted([]trustsrc.Source{
		// 过了登录宽限期，但控制连接还在
		{Addr: home, Port: 17000, Until: now.Add(-time.Hour)},
		// 过了宽限期且已断开
		{Addr: gone, Port: 17000, Until: now.Add(-time.Minute)},
		// 刚登录，还在宽限期内
		{Addr: fresh, Port: 17000, Until: now.Add(time.Minute)},
	}, now)
	if s := strings.Join(prefixStrings(got), ","); s != "14.19.55.154,2001:db8::7" {
		t.Fatalf("有效的可信来源 = %s", s)
	}
	if effectiveTrusted(nil, now) != nil {
		t.Fatal("没有登记时应为空")
	}
}

// 可信来源加入 input 链的放行集合，IPv6 按 /64 放宽
func TestBuildScriptTrusted(t *testing.T) {
	s := Settings{Firewall: true}
	s.Scan.Enable, s.Scan.Threshold = true, 20
	allow := allowFor(s, nil, []netip.Prefix{netip.MustParsePrefix("14.19.55.154/32"), netip.MustParsePrefix("2001:db8:1:2::9/128")})
	script, _, _ := buildScriptWith(true, s, allow, nil, []portRange{{443, 443}}, nil, time.Now())
	for _, want := range []string{"elements = { 14.19.55.154 }", "elements = { 2001:db8:1:2::/64 }", "ip saddr @allow4 accept"} {
		if !strings.Contains(script, want) {
			t.Errorf("脚本缺少 %q:\n%s", want, script)
		}
	}
}

// frpc 登录后立即放行；临时集合里已有的可信来源不写入封禁表；断开且过了宽限期后撤销放行
func TestFirewallTrustedSources(t *testing.T) {
	cleanDefault(t)
	const owner = "test-trust"
	t.Cleanup(func() {
		trustsrc.Clear(owner)
		// 清掉测试留下的通知信号，免得影响其他测试
		select {
		case <-trustsrc.Notify():
		default:
		}
	})
	peers := map[int]map[netip.Addr]bool{}
	stubPeers(t, peers)
	fake := &fakeNFT{output: `{"nftables": [{"metainfo": {"version": "1.0.6"}},
		{"set": {"family": "inet", "name": "scan4", "table": "gately", "type": "ipv4_addr", "elem": ["14.19.55.154", "198.51.100.9"]}},
		{"set": {"family": "inet", "name": "scan6", "table": "gately", "type": "ipv6_addr", "elem": ["2001:db8:1:2::"]}}
	]}`}
	fw := activeFirewall(t, fake)
	ctx := context.Background()
	home := netip.MustParseAddr("14.19.55.154")
	v6 := netip.MustParseAddr("2001:db8:1:2::9")
	trustsrc.Set(owner, []trustsrc.Source{
		{Addr: home, Port: 17000, Until: time.Now().Add(time.Minute)},
		{Addr: v6, Port: 17000, Until: time.Now().Add(time.Minute)},
	})
	fw.refreshTrust(ctx)
	got := fake.last()
	if !strings.Contains(got, "flush set inet gately allow4") || !strings.Contains(got, "14.19.55.154") || !strings.Contains(got, "2001:db8:1:2::/64") {
		t.Fatalf("登录后应把可信来源写入放行集合:\n%s", got)
	}
	// 内容不变时不重写
	n := len(fake.calls)
	fw.refreshTrust(ctx)
	if len(fake.calls) != n {
		t.Fatal("可信来源不变时不应重写放行集合")
	}

	var imported []netip.Addr
	fw.importFn = func(_ string, addrs []netip.Addr, _ portHits) {
		imported = append(imported, addrs...)
	}
	fw.poll(ctx)
	if len(imported) != 1 || imported[0] != netip.MustParseAddr("198.51.100.9") {
		t.Fatalf("只应导入非可信来源，实际 %v", imported)
	}
	all := strings.Join(fake.calls, "\n")
	if !strings.Contains(all, "delete element inet gately scan4 { 14.19.55.154, 198.51.100.9 }") && !strings.Contains(all, "delete element inet gately scan4 { 198.51.100.9, 14.19.55.154 }") {
		t.Fatalf("可信来源也要从临时集合删除:\n%s", all)
	}
	if banlist.Default.Banned(home) {
		t.Fatal("可信来源不应被封禁")
	}

	// 过了宽限期：控制连接还在的继续放行，已断开的撤销
	peers[17000] = map[netip.Addr]bool{home: true}
	trustsrc.Set(owner, []trustsrc.Source{
		{Addr: home, Port: 17000, Until: time.Now().Add(-time.Minute)},
		{Addr: v6, Port: 17000, Until: time.Now().Add(-time.Minute)},
	})
	fw.refreshTrust(ctx)
	got = fake.last()
	if !strings.Contains(got, "14.19.55.154") || strings.Contains(got, "2001:db8:1:2::/64") {
		t.Fatalf("断开且过了宽限期的来源应撤销放行:\n%s", got)
	}
	delete(peers, 17000)
	fw.refreshTrust(ctx)
	if got = fake.last(); strings.Contains(got, "14.19.55.154") {
		t.Fatalf("控制连接断开后应撤销放行:\n%s", got)
	}
}

// iptables 后端：可信来源变化时只重写 input 部分的链，放行规则排在诱饵端口与扫描检测之前
func TestFirewallIPTTrustedSources(t *testing.T) {
	cleanDefault(t)
	const owner = "test-trust-ipt"
	t.Cleanup(func() {
		trustsrc.Clear(owner)
		select {
		case <-trustsrc.Notify():
		default:
		}
	})
	stubPeers(t, nil)
	ipt := &fakeIPT{save: map[string]string{"iptables": goodSave(""), "ip6tables": goodSave("")}}
	fw, _ := iptFirewall(t, ipt)
	old := listeningPortsFn
	listeningPortsFn = func() (map[int]bool, error) { return map[int]bool{22: true}, nil }
	t.Cleanup(func() { listeningPortsFn = old })
	ctx := context.Background()
	s := defaultSettings()
	s.Firewall = true
	s.Trap.Enable, s.Trap.Ports = true, "23"
	s.Scan.Enable = true
	fw.apply(ctx, s)

	ipt.reset()
	trustsrc.Set(owner, []trustsrc.Source{{Addr: netip.MustParseAddr("14.19.55.154"), Port: 17000, Until: time.Now().Add(time.Minute)}})
	fw.refreshTrust(ctx)
	calls := ipt.find("iptables-legacy-restore --noflush", ":GATELY_IN - [0:0]", "-s 14.19.55.154/32 -j RETURN")
	if len(calls) != 1 || strings.Contains(calls[0], ":GATELY_PRE") {
		t.Fatalf("应只重写 input 部分的链并放行可信来源: %v", ipt.calls)
	}
	if i, j := strings.Index(calls[0], "-s 14.19.55.154/32 -j RETURN"), strings.Index(calls[0], "-j GATELY_TRAP"); j >= 0 && i > j {
		t.Fatalf("放行规则应在诱饵端口规则之前:\n%s", calls[0])
	}
}

// 在 Linux 上读取真实的 /proc/net/tcp{,6}：本机建立的连接能按端口找到对端地址
func TestEstablishedPeersReal(t *testing.T) {
	if _, err := os.Stat("/proc/net/tcp"); err != nil {
		t.Skip("没有 /proc/net/tcp")
	}
	for _, network := range []string{"tcp4", "tcp6"} {
		host := "127.0.0.1"
		if network == "tcp6" {
			host = "::1"
		}
		ln, err := net.Listen(network, net.JoinHostPort(host, "0"))
		if err != nil {
			t.Logf("%s 不可用: %v", network, err)
			continue
		}
		accepted := make(chan net.Conn, 1)
		go func() {
			c, err := ln.Accept()
			if err == nil {
				accepted <- c
			}
		}()
		c, err := net.Dial(network, ln.Addr().String())
		if err != nil {
			ln.Close()
			t.Fatal(err)
		}
		s := <-accepted
		port := ln.Addr().(*net.TCPAddr).Port
		peers, err := establishedPeers(map[int]bool{port: true})
		c.Close()
		s.Close()
		ln.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !peers[port][netip.MustParseAddr(host)] {
			t.Errorf("%s: 端口 %d 的已建立连接应能找到对端 %s，实际 %v", network, port, host, peers)
		}
	}
}
