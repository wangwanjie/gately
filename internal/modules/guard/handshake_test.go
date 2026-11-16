package guard

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/trustsrc"
)

// 确认握手的监听：诱饵与扫描各一个端口，IPv4 与 IPv6 都能接受；回调收到类别与来源，连接随即以 RST 关闭。
func TestHandshakeListener(t *testing.T) {
	type hit struct {
		kind string
		src  netip.Addr
	}
	got := make(chan hit, 4)
	h, err := startHandshake(func(kind string, src netip.Addr, _ int) { got <- hit{kind, src} })
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		h.stop()
		h.wait()
	}()
	if !h.ports.ok() || h.ports.trap == h.ports.scan {
		t.Fatalf("应监听两个不同的端口: %+v", h.ports)
	}
	c, err := net.Dial("tcp4", "127.0.0.1:"+strconv.Itoa(h.ports.trap))
	if err != nil {
		t.Fatal(err)
	}
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := c.Read(make([]byte, 1)); err == nil || errors.Is(err, io.EOF) && runtime.GOOS == "linux" {
		// Linux 上 SO_LINGER 0 的关闭发送 RST，读到的是连接被重置
		t.Fatalf("连接应被立即重置: %v", err)
	}
	c.Close()
	if x := <-got; x.kind != hsTrap || x.src != netip.MustParseAddr("127.0.0.1") {
		t.Fatalf("回调不符: %+v", x)
	}
	if c6, err := net.Dial("tcp6", "[::1]:"+strconv.Itoa(h.ports.scan)); err == nil {
		c6.Close()
		if x := <-got; x.kind != hsScan || x.src != netip.MustParseAddr("::1") {
			t.Fatalf("IPv6 回调不符: %+v", x)
		}
	} else {
		t.Logf("本机没有 IPv6 回环，跳过 IPv6: %v", err)
	}
}

// handshakeFirewall 返回规则已生效、重定向可用的防火墙，importFn 收集封禁。
func handshakeFirewall(t *testing.T, s Settings) (*firewall, func() map[string][]string) {
	t.Helper()
	fw := activeFirewall(t, &fakeNFT{})
	fw.cfg = s
	fw.redirOK = true
	var mu sync.Mutex
	bans := map[string][]string{}
	fw.importFn = func(source string, addrs []netip.Addr, ports portHits) {
		mu.Lock()
		defer mu.Unlock()
		for _, a := range addrs {
			bans[source] = append(bans[source], portBanReason(source, ports[banlist.KeyFor(a)])+" "+a.String())
		}
	}
	return fw, func() map[string][]string {
		fw.flushPending()
		mu.Lock()
		defer mu.Unlock()
		out := map[string][]string{}
		for k, v := range bans {
			out[k] = append([]string{}, v...)
		}
		return out
	}
}

// 完成握手的诱饵端口连接即计入封禁（带命中端口）；扫描检测在一分钟内完成握手的连接超过阈值才封禁；
// IPv6 严格模式（就绪后）第一次即封禁；触发次数按握手确认累计。
func TestHandshakeBans(t *testing.T) {
	s := defaultSettings()
	s.Trap.Enable = true
	s.Scan.Enable, s.Scan.Threshold, s.Scan.IPv6Strict = true, 3, true
	fw, bans := handshakeFirewall(t, s)

	fw.onHandshake(hsTrap, netip.MustParseAddr("203.0.113.5"), 23)
	if got := bans()[banlist.SourceTrap]; !slices.Equal(got, []string{"连接诱饵端口 23 203.0.113.5"}) {
		t.Fatalf("诱饵端口应封禁: %v", got)
	}
	scanner := netip.MustParseAddr("198.51.100.7")
	for i, port := range []int{1001, 1002, 1003} {
		fw.onHandshake(hsScan, scanner, port)
		if n := len(bans()[banlist.SourceScan]); n != 0 {
			t.Fatalf("第 %d 次未超过阈值，不应封禁", i+1)
		}
	}
	fw.onHandshake(hsScan, scanner, 1004)
	if got := bans()[banlist.SourceScan]; !slices.Equal(got, []string{"端口扫描（端口 1001, 1002, 1003, 1004） 198.51.100.7"}) {
		t.Fatalf("超过阈值应封禁: %v", got)
	}
	// 严格模式未就绪时 IPv6 按阈值；就绪后第一次即封禁
	v6 := netip.MustParseAddr("2001:db8:9::1")
	fw.onHandshake(hsScan, v6, 80)
	if n := len(bans()[banlist.SourceScan]); n != 1 {
		t.Fatalf("严格模式就绪前 IPv6 应按阈值计数: %d", n)
	}
	fw.mu.Lock()
	fw.strictReady = true
	fw.mu.Unlock()
	fw.onHandshake(hsScan, netip.MustParseAddr("2001:db8:a::1"), 81)
	if got := bans()[banlist.SourceScan]; len(got) != 2 || !strings.HasSuffix(got[1], "2001:db8:a::1") {
		t.Fatalf("严格模式下 IPv6 第一次完成握手即封禁: %v", got)
	}
	if st := fw.snapshot(); st.Hits[hsTrap] != 1 || st.Hits[hsScan] != 6 {
		t.Fatalf("触发次数应按握手确认累计: %v", st.Hits)
	}
	// 计数窗口一分钟：窗口过期后重新计数
	fw.mu.Lock()
	fw.strictReady = false
	fw.scans[banlist.KeyFor(v6)].start = time.Now().Add(-2 * scanWindow)
	fw.mu.Unlock()
	for range 3 {
		fw.onHandshake(hsScan, v6, 82)
	}
	fw.mu.Lock()
	n := fw.scans[banlist.KeyFor(v6)].n
	fw.mu.Unlock()
	if n != 3 {
		t.Fatalf("窗口过期后应重新计数: %d", n)
	}
}

// 不处理的来源：白名单（含内网）、可信来源（含刚登录管理后台的地址）、回环；规则未生效或重定向不可用时不处理。
func TestHandshakeSkips(t *testing.T) {
	s := defaultSettings()
	s.Trap.Enable = true
	s.IgnoreList = []string{"203.0.113.0/24"}
	fw, bans := handshakeFirewall(t, s)
	trustsrc.Set("test-hs", []trustsrc.Source{{Addr: netip.MustParseAddr("198.51.100.20"), Until: time.Now().Add(time.Hour)}})
	t.Cleanup(func() { trustsrc.Clear("test-hs") })
	fw.refreshTrust(context.Background())
	for _, a := range []string{"203.0.113.9", "192.168.1.5", "127.0.0.1", "::1", "198.51.100.20"} {
		fw.onHandshake(hsTrap, netip.MustParseAddr(a), 23)
	}
	if got := bans(); len(got) != 0 {
		t.Fatalf("白名单、内网、回环与可信来源不应封禁: %v", got)
	}
	fw.mu.Lock()
	fw.redirOK = false
	fw.mu.Unlock()
	fw.onHandshake(hsTrap, netip.MustParseAddr("198.51.100.30"), 23)
	fw.mu.Lock()
	fw.redirOK, fw.status.Active = true, false
	fw.mu.Unlock()
	fw.onHandshake(hsTrap, netip.MustParseAddr("198.51.100.31"), 23)
	if got := bans(); len(got) != 0 {
		t.Fatalf("规则未生效或重定向不可用时不应处理: %v", got)
	}
}

// 写入规则：诱饵端口或扫描检测开启时启动确认握手的监听并写入重定向链；内核不支持重定向时去掉重定向链重写一次，
// 规则照常生效，状态标明重定向不可用并提示一次，诱饵端口交回 Gately 直接监听（kernelTrap 为 false）；关闭后停止监听。
func TestFirewallRedirectFallback(t *testing.T) {
	cleanDefault(t)
	unsupported := true
	fake := &fakeNFT{fail: func(stdin string, _ []string) error {
		if unsupported && strings.Contains(stdin, "redirect to") {
			return errorString("Error: Could not process rule: No such file or directory\nchain redir {")
		}
		return nil
	}}
	fw, buf := inactiveFirewall(t, fake)
	old := listeningPortsFn
	listeningPortsFn = func() (map[int]bool, error) { return map[int]bool{22: true}, nil }
	t.Cleanup(func() { listeningPortsFn = old })
	s := defaultSettings()
	s.Firewall = true
	s.Trap.Enable, s.Trap.Ports = true, "23"
	s.Scan.Enable = true
	fw.apply(context.Background(), s)
	st := fw.snapshot()
	if !st.Active || st.Redirect || fw.kernelTrap() {
		t.Fatalf("不支持重定向时规则应生效但重定向不可用: %+v", st)
	}
	if !strings.Contains(strings.Join(logsAt(buf, "warn"), "\n"), "不支持把诱饵端口与可疑来源的连接重定向") {
		t.Errorf("应提示不支持重定向: %v", logsAt(buf, "warn"))
	}
	fw.apply(context.Background(), s)
	if n := strings.Count(strings.Join(logsAt(buf, "warn"), "\n"), "不支持把诱饵端口"); n != 1 {
		t.Errorf("相同错误只提示一次: %d", n)
	}
	unsupported = false
	fw.apply(context.Background(), s)
	if st := fw.snapshot(); !st.Redirect || !fw.kernelTrap() {
		t.Fatalf("支持重定向时应写入重定向链: %+v", st)
	}
	fw.mu.Lock()
	ports := fw.hs.ports
	fw.mu.Unlock()
	if !strings.Contains(fake.last(), "redirect to :"+strconv.Itoa(ports.trap)) || !strings.Contains(fake.last(), "redirect to :"+strconv.Itoa(ports.scan)) {
		t.Fatalf("重定向应指向确认握手的监听端口 %+v:\n%s", ports, fake.last())
	}
	// 确认握手的监听端口不算作开放端口
	fw.mu.Lock()
	listeningPortsFn = func() (map[int]bool, error) { return map[int]bool{22: true, ports.trap: true, ports.scan: true}, nil }
	l, _ := fw.listeningLocked()
	fw.mu.Unlock()
	if l[ports.trap] || l[ports.scan] || !l[22] {
		t.Fatalf("确认握手的端口不应算作开放端口: %v", l)
	}
	s.Trap.Enable, s.Scan.Enable = false, false
	fw.apply(context.Background(), s)
	fw.mu.Lock()
	hs := fw.hs
	fw.mu.Unlock()
	if hs != nil {
		t.Fatal("诱饵端口与扫描检测都关闭时应停止监听")
	}
	if c, err := net.DialTimeout("tcp4", "127.0.0.1:"+strconv.Itoa(ports.trap), 300*time.Millisecond); err == nil {
		c.Close()
		t.Fatal("停止后端口不应再接受连接")
	}
	fw.stop()
}
