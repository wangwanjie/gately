package portforward

import (
	"context"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/iplist"
)

// tcpEchoes 判断经监听端口能否完成一次回显：被访问控制拒绝的连接会被直接关闭。
func tcpEchoes(t *testing.T, port int) bool {
	t.Helper()
	return tcpEchoesFrom(t, "127.0.0.1", port)
}

// tcpEchoesFrom 从 host 发起连接（目标地址决定本机使用的来源地址），判断能否完成一次回显。
func tcpEchoesFrom(t *testing.T, host string, port int) bool {
	t.Helper()
	conn, err := net.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write([]byte("ping")); err != nil {
		return false
	}
	buf := make([]byte, 4)
	_, err = io.ReadFull(conn, buf)
	return err == nil && string(buf) == "ping"
}

// udpEchoes 判断经监听端口发送的 UDP 数据包能否收到回显。
func udpEchoes(t *testing.T, port int) bool {
	t.Helper()
	return udpEchoesFrom(t, "127.0.0.1", port)
}

// udpEchoesFrom 从 host 发送 UDP 数据包，判断能否收到回显。
func udpEchoesFrom(t *testing.T, host string, port int) bool {
	t.Helper()
	conn, err := net.Dial("udp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 16)
	n, err := conn.Read(buf)
	return err == nil && string(buf[:n]) == "ping"
}

// startAccessRule 启动一条 TCP+UDP 规则，目标为本机的回显服务。
func startAccessRule(t *testing.T, acc iplist.Access) (*runner, Rule, int) {
	t.Helper()
	return startAccessRuleOn(t, acc, "127.0.0.1", []string{"tcp4", "udp4"})
}

// startAccessRuleOn 在 listenAddr 上按 protocols 启动规则；listenAddr 为空且协议为 tcp/udp 时为双栈监听。
func startAccessRuleOn(t *testing.T, acc iplist.Access, listenAddr string, protocols []string) (*runner, Rule, int) {
	t.Helper()
	tcpPort, closeTCP := tcpEcho(t)
	t.Cleanup(closeTCP)
	// TCP 与 UDP 共用目标端口：在 TCP 回显的同号端口上再起一个 UDP 回显
	pc, err := net.ListenPacket("udp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(tcpPort)))
	if err != nil {
		t.Skipf("无法在同号端口上监听 UDP: %v", err)
	}
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = pc.WriteTo(buf[:n], addr)
		}
	}()
	t.Cleanup(func() { pc.Close() })

	listenPort := freePort(t, "tcp")
	rule := Rule{
		Protocols:         protocols,
		ListenAddress:     listenAddr,
		ListenPorts:       strconv.Itoa(listenPort),
		TargetAddresses:   []string{"127.0.0.1"},
		TargetPorts:       strconv.Itoa(tcpPort),
		UDPSessionTimeout: 1,
		Access:            acc,
	}
	if err := rule.Validate(); err != nil {
		t.Fatal(err)
	}
	r := testRunner(rule)
	if err := r.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.stop)
	return r, rule, listenPort
}

func TestAccessWhitelistHotUpdate(t *testing.T) {
	r, rule, port := startAccessRule(t, iplist.Access{IPMode: iplist.ModeWhitelist, IPList: []string{"10.0.0.0/8"}})
	if tcpEchoes(t, port) || udpEchoes(t, port) {
		t.Fatal("不在白名单中的来源应被拒绝")
	}
	if s := r.snapshot(); s.Denied < 2 || s.TotalConns != 0 {
		t.Fatalf("拒绝次数应计入 Denied 且不计入连接数: %+v", s)
	}

	// 修改名单只热替换，监听不中断
	rule.Access = iplist.Access{IPMode: iplist.ModeWhitelist, IPList: []string{"10.0.0.0/8", "127.0.0.1"}}
	if !r.canUpdate(rule, r.settings) {
		t.Fatal("只修改名单时应能热更新")
	}
	r.update(rule, r.settings, r.current().log)
	if !tcpEchoes(t, port) || !udpEchoes(t, port) {
		t.Fatal("加入白名单后应放行")
	}

	// 黑名单：命中即拒绝
	rule.Access = iplist.Access{IPMode: iplist.ModeBlacklist, IPList: []string{"127.0.0.0/8"}}
	r.update(rule, r.settings, r.current().log)
	if tcpEchoes(t, port) {
		t.Fatal("命中黑名单的来源应被拒绝")
	}
	// 已有 UDP 会话的上行同样被拦下
	if udpEchoes(t, port) {
		t.Fatal("命中黑名单后 UDP 数据包应被丢弃")
	}
}

func TestAccessSubscriptionRefresh(t *testing.T) {
	set, _, _ := iplist.Parse(strings.NewReader("127.0.0.0/8\n"))
	iplist.Default.Store("t-pf", set)
	t.Cleanup(func() { iplist.Default.Store("t-pf", nil) })

	_, _, port := startAccessRule(t, iplist.Access{IPMode: iplist.ModeBlacklist, IPSets: []string{"t-pf"}})
	if tcpEchoes(t, port) {
		t.Fatal("命中订阅黑名单的来源应被拒绝")
	}
	// 订阅列表刷新后立即生效，规则不必重启
	set2, _, _ := iplist.Parse(strings.NewReader("192.0.2.0/24\n"))
	iplist.Default.Store("t-pf", set2)
	if !tcpEchoes(t, port) {
		t.Fatal("列表刷新后已移出的来源应放行")
	}
}

func TestAccessBanWins(t *testing.T) {
	_, _, port := startAccessRule(t, iplist.Access{IPMode: iplist.ModeWhitelist, IPList: []string{"127.0.0.0/8"}})
	if !tcpEchoes(t, port) {
		t.Fatal("白名单内的来源应放行")
	}
	if _, _, err := banlist.Default.Ban("127.0.0.1", banlist.Options{Duration: time.Hour, Force: true, Source: banlist.SourceManual}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { banlist.Default.Unban("127.0.0.1") })
	// 封禁优先于白名单
	if tcpEchoes(t, port) || udpEchoes(t, port) {
		t.Fatal("被封禁的地址即使在白名单中也应被拒绝")
	}
}

func TestModulePublishesIPSetUsers(t *testing.T) {
	iplist.Default.SetCatalog([]iplist.Info{{Key: "t-cn", Name: "cn"}})
	t.Cleanup(func() {
		iplist.Default.SetCatalog(nil)
		iplist.Default.SetUsers(ID, nil)
	})
	m := New()
	m.cfg.Rules = []Rule{
		{Key: "a", Name: "ssh", Access: iplist.Access{IPMode: iplist.ModeWhitelist, IPSets: []string{"t-cn"}}},
		{Key: "b", Name: "off", Access: iplist.Access{IPSets: []string{"t-cn"}}},
	}
	m.publishIPSetUsersLocked()
	if got := strings.Join(iplist.Default.Users("t-cn"), ","); got != "portforward / ssh" {
		t.Errorf("引用方 = %q", got)
	}
	bad := Rule{Access: iplist.Access{IPMode: iplist.ModeBlacklist, IPSets: []string{"t-gone"}}}
	if err := bad.Access.CheckSets(); err == nil {
		t.Error("引用不存在的订阅列表应报错")
	}
}

// 订阅列表未载入或已删除时按空列表处理：白名单拒绝，黑名单放行
func TestAccessMissingSubscription(t *testing.T) {
	set, _, _ := iplist.Parse(strings.NewReader("127.0.0.0/8\n"))
	iplist.Default.Store("t-pf-gone", set)
	iplist.Default.Store("t-pf-gone", nil) // 已删除
	for _, key := range []string{"t-pf-never", "t-pf-gone"} {
		r, _, port := startAccessRule(t, iplist.Access{IPMode: iplist.ModeWhitelist, IPSets: []string{key}})
		if tcpEchoes(t, port) || udpEchoes(t, port) {
			t.Fatalf("%s: 白名单引用的列表不可用时应拒绝", key)
		}
		if s := r.snapshot(); s.Denied != 2 {
			t.Fatalf("%s: 拒绝次数应为 2（TCP 1 次 + UDP 1 个数据包），实际 %d", key, s.Denied)
		}
		r2, _, port2 := startAccessRule(t, iplist.Access{IPMode: iplist.ModeBlacklist, IPSets: []string{key}})
		if !tcpEchoes(t, port2) || !udpEchoes(t, port2) {
			t.Fatalf("%s: 黑名单引用的列表不可用时应放行", key)
		}
		if s := r2.snapshot(); s.Denied != 0 {
			t.Fatalf("%s: 放行时不应计入拒绝，实际 %d", key, s.Denied)
		}
	}
}

// IPv4 连接经双栈监听到达时是 IPv4 映射的 IPv6 地址，应按 IPv4 判断；纯 IPv6 来源按 IPv6 网段判断
func TestAccessIPv6Sources(t *testing.T) {
	if ln, err := net.Listen("tcp6", "[::1]:0"); err != nil {
		t.Skip("本机不支持 IPv6 回环")
	} else {
		ln.Close()
	}
	r, rule, port := startAccessRuleOn(t, iplist.Access{IPMode: iplist.ModeWhitelist, IPList: []string{"127.0.0.1"}}, "", []string{"tcp", "udp"})
	if !tcpEchoesFrom(t, "127.0.0.1", port) || !udpEchoesFrom(t, "127.0.0.1", port) {
		t.Fatal("IPv4 映射地址应按 IPv4 命中白名单")
	}
	if tcpEchoesFrom(t, "::1", port) || udpEchoesFrom(t, "::1", port) {
		t.Fatal("不在白名单中的 IPv6 来源应被拒绝")
	}
	rule.Access = iplist.Access{IPMode: iplist.ModeWhitelist, IPList: []string{"::1/128"}}
	r.update(rule, r.settings, r.current().log)
	if !tcpEchoesFrom(t, "::1", port) || !udpEchoesFrom(t, "::1", port) {
		t.Fatal("IPv6 来源应命中 IPv6 网段")
	}
	if tcpEchoesFrom(t, "127.0.0.1", port) {
		t.Fatal("IPv4 来源不应命中 IPv6 网段")
	}
	rule.Access = iplist.Access{IPMode: iplist.ModeBlacklist, IPList: []string{"::ffff:127.0.0.1"}}
	r.update(rule, r.settings, r.current().log)
	if tcpEchoesFrom(t, "127.0.0.1", port) {
		t.Fatal("以 IPv4 映射形式填写的黑名单也应拦下 IPv4 来源")
	}
}

// UDP：被拒来源不建立会话；已有会话在名单收紧后上行被丢弃，会话保留到空闲超时，名单放开后继续使用原会话
func TestAccessUDPSessions(t *testing.T) {
	r, rule, port := startAccessRule(t, iplist.Access{IPMode: iplist.ModeWhitelist, IPList: []string{"10.0.0.0/8"}})
	conn, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	echo := func() bool {
		if _, err := conn.Write([]byte("ping")); err != nil {
			t.Fatal(err)
		}
		_ = conn.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
		buf := make([]byte, 16)
		n, err := conn.Read(buf)
		return err == nil && string(buf[:n]) == "ping"
	}
	if echo() || echo() {
		t.Fatal("被拒来源不应收到回显")
	}
	if s := r.snapshot(); s.TotalConns != 0 || s.ActiveConns != 0 || s.Denied != 2 {
		t.Fatalf("被拒来源不应建立会话: %+v", s)
	}

	rule.Access = iplist.Access{IPMode: iplist.ModeWhitelist, IPList: []string{"127.0.0.1"}}
	r.update(rule, r.settings, r.current().log)
	if !echo() {
		t.Fatal("放行后应建立会话")
	}
	if s := r.snapshot(); s.TotalConns != 1 || s.ActiveConns != 1 {
		t.Fatalf("应建立 1 个会话: %+v", s)
	}

	rule.Access = iplist.Access{IPMode: iplist.ModeBlacklist, IPList: []string{"127.0.0.1"}}
	r.update(rule, r.settings, r.current().log)
	if echo() {
		t.Fatal("名单收紧后已有会话的上行应被丢弃")
	}
	if s := r.snapshot(); s.ActiveConns != 1 || s.Denied != 3 {
		t.Fatalf("会话应保留到空闲超时且计入拒绝: %+v", s)
	}

	rule.Access = iplist.Access{}
	r.update(rule, r.settings, r.current().log)
	if !echo() {
		t.Fatal("关闭名单后应继续使用原会话")
	}
	if s := r.snapshot(); s.TotalConns != 1 {
		t.Fatalf("应复用原会话而不是新建: %+v", s)
	}
}

func TestAccessValidate(t *testing.T) {
	base := func(a iplist.Access) Rule {
		return Rule{Protocols: []string{"tcp"}, ListenPorts: "1", TargetAddresses: []string{"127.0.0.1"}, Access: a}
	}
	for name, a := range map[string]iplist.Access{
		"模式非法":  {IPMode: "allow", IPList: []string{"10.0.0.1"}},
		"网段非法":  {IPMode: iplist.ModeBlacklist, IPList: []string{"10.0.0.0/40"}},
		"IP 非法": {IPMode: iplist.ModeBlacklist, IPList: []string{"10.0.0.256"}},
		"白名单为空": {IPMode: iplist.ModeWhitelist, IPList: []string{" ", ""}},
	} {
		r := base(a)
		if err := r.Validate(); err == nil {
			t.Errorf("%s 应校验失败", name)
		}
	}
	ok := base(iplist.Access{IPMode: iplist.ModeWhitelist, IPList: []string{" 10.0.0.0/8 ", "2001:db8::1"}, IPSets: []string{"a", "a"}})
	if err := ok.Validate(); err != nil || len(ok.IPList) != 2 || len(ok.IPSets) != 1 {
		t.Fatalf("合法配置应通过并规范化: %v %+v", err, ok.Access)
	}
	if err := ok.Access.CheckSets(); err == nil {
		t.Error("引用不存在的订阅列表应报错")
	}
}
