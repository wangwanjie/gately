package knockgate

import (
	"errors"
	"net/netip"
	"testing"
	"time"
)

func TestRegistry(t *testing.T) {
	r := New()
	now := time.Unix(1000, 0)
	r.SetClock(func() time.Time { return now })
	a := netip.MustParseAddr("::ffff:203.0.113.1")
	if err := r.Allow(a, "k1", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if !r.Allowed(netip.MustParseAddr("203.0.113.1"), "k1") {
		t.Fatal("IPv4 映射地址应按 IPv4 登记")
	}
	if r.Allowed(netip.MustParseAddr("203.0.113.1"), "k2") || r.Allowed(netip.MustParseAddr("203.0.113.2"), "k1") || r.Allowed(netip.MustParseAddr("203.0.113.1"), "") {
		t.Fatal("授权只对登记的地址与规则有效")
	}
	// 较短的授权不缩短已有授权
	_ = r.Allow(a, "k1", now.Add(time.Second))
	now = now.Add(30 * time.Second)
	if !r.Allowed(a, "k1") {
		t.Fatal("不应被较短的授权缩短")
	}
	if l := r.List(); len(l) != 1 || l[0].Rule != "k1" {
		t.Fatalf("列表不对: %+v", l)
	}
	now = now.Add(31 * time.Second)
	if r.Allowed(a, "k1") || len(r.List()) != 0 {
		t.Fatal("到期后应失效")
	}
	if r.Prune() != 1 {
		t.Fatal("应清理 1 个到期授权")
	}
	_ = r.Allow(a, "k1", now.Add(time.Minute))
	_ = r.Allow(a, "k2", now.Add(time.Minute))
	if r.Revoke(a, "k1") != 1 || !r.Allowed(a, "k2") {
		t.Fatal("按规则撤销")
	}
	r.RevokeRule("k2")
	if r.Allowed(a, "k2") {
		t.Fatal("撤销规则全部授权")
	}
}

func TestRulesAndSubmit(t *testing.T) {
	r := New()
	r.SetRules([]Rule{{Key: "k1", Name: "SSH", Enable: true}})
	if n, ok := r.RuleName("k1"); !ok || n != "SSH" {
		t.Fatal("规则目录")
	}
	if _, ok := r.RuleName("x"); ok {
		t.Fatal("不存在的规则")
	}
	if err := r.Submit([]byte("x"), netip.MustParseAddr("1.2.3.4"), "t"); !errors.Is(err, ErrUnavailable) {
		t.Fatal("没有处理函数时应不可用")
	}
	var got netip.Addr
	r.SetSubmitter(func(_ []byte, src netip.Addr, _ string) error { got = src; return nil })
	_ = r.Submit([]byte("x"), netip.MustParseAddr("::ffff:1.2.3.4"), "t")
	if got != netip.MustParseAddr("1.2.3.4") {
		t.Fatalf("来源应去掉映射: %s", got)
	}
}

func TestAllowCap(t *testing.T) {
	r := New()
	now := time.Unix(1000, 0)
	r.SetClock(func() time.Time { return now })
	base := netip.MustParseAddr("2001:db8::")
	for i := range maxGrants {
		b := base.As16()
		b[12], b[13], b[14], b[15] = byte(i>>24), byte(i>>16), byte(i>>8), byte(i)
		if err := r.Allow(netip.AddrFrom16(b), "k", now.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Allow(netip.MustParseAddr("192.0.2.1"), "k", now.Add(time.Second)); err == nil {
		t.Fatal("超过上限应拒绝")
	}
	now = now.Add(2 * time.Second)
	if err := r.Allow(netip.MustParseAddr("192.0.2.1"), "k", now.Add(time.Second)); err != nil {
		t.Fatalf("到期授权清理后应能登记: %v", err)
	}
}
