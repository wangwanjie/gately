package portforward

import (
	"net"
	"net/netip"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/iplist"
	"github.com/wangwanjie/gately/internal/knockgate"
)

// 规则要求敲门：没有有效授权的来源 TCP 连接与 UDP 数据包都被拒绝，敲门授权到期后再次拒绝。
func TestKnockGate(t *testing.T) {
	knockgate.Default.Clear()
	now := time.Now()
	knockgate.Default.SetClock(func() time.Time { return now })
	t.Cleanup(func() {
		knockgate.Default.SetClock(nil)
		knockgate.Default.Clear()
	})
	_, _, port := startAccessRule(t, iplist.Access{Knock: "k1"})
	if tcpEchoes(t, port) || udpEchoes(t, port) {
		t.Fatal("未敲门应拒绝")
	}
	_ = knockgate.Default.Allow(netip.MustParseAddr("127.0.0.1"), "k1", now.Add(time.Minute))
	if !tcpEchoes(t, port) || !udpEchoes(t, port) {
		t.Fatal("敲门后应放行")
	}
	// 其他规则的授权不算
	knockgate.Default.Revoke(netip.MustParseAddr("127.0.0.1"), "")
	_ = knockgate.Default.Allow(netip.MustParseAddr("127.0.0.1"), "k2", now.Add(time.Minute))
	if tcpEchoes(t, port) {
		t.Fatal("其他敲门规则的授权不应放行")
	}
}

// UDP 只在新建会话时检查敲门：授权到期后已有会话照常转发（与防火墙拦截保持已建立连接一致），
// 新来源（新会话）仍被拒绝；黑白名单照旧逐包判断。
func TestKnockUDPSessionKept(t *testing.T) {
	knockgate.Default.Clear()
	var clock atomic.Int64
	clock.Store(time.Now().UnixNano())
	knockgate.Default.SetClock(func() time.Time { return time.Unix(0, clock.Load()) })
	t.Cleanup(func() {
		knockgate.Default.SetClock(nil)
		knockgate.Default.Clear()
	})
	r, _, port := startAccessRule(t, iplist.Access{Knock: "k1"})
	_ = knockgate.Default.Allow(netip.MustParseAddr("127.0.0.1"), "k1", time.Unix(0, clock.Load()).Add(time.Minute))
	conn, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	echo := func(c net.Conn) bool {
		_, _ = c.Write([]byte("ping"))
		_ = c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		buf := make([]byte, 16)
		n, err := c.Read(buf)
		return err == nil && string(buf[:n]) == "ping"
	}
	if !echo(conn) {
		t.Fatal("敲门后应建立会话")
	}
	// 授权到期
	clock.Add(int64(2 * time.Minute))
	if !echo(conn) {
		t.Fatal("授权到期后已有会话应继续转发")
	}
	// 同一来源地址的新端口是新会话，需要有效授权
	other, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if echo(other) {
		t.Fatal("授权到期后不应新建会话")
	}
	if r.denied.Load() == 0 {
		t.Fatal("拒绝新建会话应计数")
	}
}
