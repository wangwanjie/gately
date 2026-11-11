package portforward

import (
	"net/netip"
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
