package stun

import (
	"net"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/iplist"
	"github.com/wangwanjie/gately/internal/knockgate"
)

// UDP 内置转发只在新建会话时检查敲门：授权到期后已有会话照常转发，新会话被拒绝。
func TestUDPKnockSessionKept(t *testing.T) {
	knockgate.Default.Clear()
	var clock atomic.Int64
	clock.Store(time.Now().UnixNano())
	knockgate.Default.SetClock(func() time.Time { return time.Unix(0, clock.Load()) })
	t.Cleanup(func() {
		knockgate.Default.SetClock(nil)
		knockgate.Default.Clear()
	})
	e := newTestEnv(t)
	m := startAccessModule(t, udpRule(e, "u", iplist.Access{Knock: "k1"}), e.settings())
	r := m.runners["u"]
	pub := r.snapshot().PublicAddr
	echo := func(c net.Conn) bool {
		_, _ = c.Write([]byte("ping"))
		_ = c.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
		buf := make([]byte, 16)
		n, err := c.Read(buf)
		return err == nil && string(buf[:n]) == "ping"
	}
	conn, err := net.Dial("udp4", pub)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if echo(conn) {
		t.Fatal("未敲门不应建立会话")
	}
	_ = knockgate.Default.Allow(netip.MustParseAddr("127.0.0.1"), "k1", time.Unix(0, clock.Load()).Add(time.Minute))
	if !echo(conn) {
		t.Fatal("敲门后应建立会话")
	}
	clock.Add(int64(2 * time.Minute))
	if !echo(conn) {
		t.Fatal("授权到期后已有会话应继续转发")
	}
	other, err := net.Dial("udp4", pub)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if echo(other) {
		t.Fatal("授权到期后不应新建会话")
	}
	if s := r.snapshot().Stats; s.TotalConns != 1 || s.Denied < 2 {
		t.Fatalf("统计不对: %+v", s)
	}
}
