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
	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
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
	conn, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
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
		Protocols:         []string{"tcp4", "udp4"},
		ListenAddress:     "127.0.0.1",
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
