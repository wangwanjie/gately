package trustsrc

import (
	"net/netip"
	"testing"
	"time"
)

// drain 清空通知通道，返回是否有待处理的信号。
func drain() bool {
	select {
	case <-wake:
		return true
	default:
		return false
	}
}

func TestSetClearNotify(t *testing.T) {
	t.Cleanup(func() {
		Clear("a")
		Clear("b")
		drain()
	})
	drain()
	until := time.Unix(1_800_000_000, 0)
	Set("a", []Source{{Addr: netip.MustParseAddr("::ffff:1.2.3.4"), Port: 7000, Until: until}, {}})
	if !drain() {
		t.Fatal("登记后应发出通知")
	}
	got := All()
	if len(got) != 1 || got[0].Addr != netip.MustParseAddr("1.2.3.4") || got[0].Port != 7000 || !got[0].Until.Equal(until) {
		t.Fatalf("登记内容 = %+v，应只有映射地址展开后的 1.2.3.4", got)
	}

	// 内容不变时不重复通知
	Set("a", []Source{{Addr: netip.MustParseAddr("1.2.3.4"), Port: 7000, Until: until}})
	if drain() {
		t.Fatal("内容不变时不应通知")
	}

	Set("b", []Source{{Addr: netip.MustParseAddr("2001:db8::1")}})
	drain()
	if n := len(All()); n != 2 {
		t.Fatalf("两个登记方共 %d 个来源，应为 2", n)
	}

	Clear("a")
	if !drain() {
		t.Fatal("撤销登记应发出通知")
	}
	Clear("a")
	if drain() {
		t.Fatal("撤销不存在的登记不应通知")
	}
	if got := All(); len(got) != 1 || got[0].Addr != netip.MustParseAddr("2001:db8::1") {
		t.Fatalf("撤销后剩余 %+v", got)
	}
}
