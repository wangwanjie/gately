package visitors

import (
	"fmt"
	"testing"
	"time"
)

// 淘汰分三档：大量匿名地址各访问一次，不能挤掉认证用户与管理员；扫描类最先淘汰。
func TestEvictKeepsAuthedOverAnonymous(t *testing.T) {
	s, now := newTestStore(t, "")
	s.Apply(Settings{Days: 30, Max: 100})
	for i := 0; i < 20; i++ {
		h := Hit{Addr: ip(fmt.Sprintf("10.0.0.%d", i)), Time: *now, Site: "s", Path: "/", User: "alice"}
		if i%2 == 1 {
			h.User, h.Admin = "", "root"
		}
		s.Record(h)
	}
	*now = now.Add(time.Second)
	s.Record(Hit{Addr: ip("10.0.1.1"), Time: *now, Unmatched: true, Path: "/.env"})
	// 攻击者用大量 IPv6 地址各访问一次正常站点（不算扫描类）
	for i := 0; i < 500; i++ {
		*now = now.Add(time.Second)
		s.Record(Hit{Addr: ip(fmt.Sprintf("2001:db8::%x", i+1)), Time: *now, Site: "s", Path: "/", Status: 200})
	}
	if s.Len() > 100 {
		t.Fatalf("超过上限: %d", s.Len())
	}
	for i := 0; i < 20; i++ {
		if _, ok := s.Get(ip(fmt.Sprintf("10.0.0.%d", i))); !ok {
			t.Fatalf("认证访客 10.0.0.%d 被匿名访客挤掉", i)
		}
	}
	if _, ok := s.Get(ip("10.0.1.1")); ok {
		t.Fatal("扫描类访客应最先淘汰")
	}
	// 同一档内最久未出现的先淘汰：最新的匿名访客保留
	if _, ok := s.Get(ip("2001:db8::1f4")); !ok {
		t.Fatal("最近的匿名访客应保留")
	}
	tiers := []*Visitor{
		{Requests: 1, Noise: 1},
		{Requests: 1},
		{Requests: 1, Noise: 1, Identities: []IdentStat{{Name: "a", Kind: "admin"}}},
	}
	for want, v := range tiers {
		if v.keepTier() != want {
			t.Fatalf("保留等级 = %d，应为 %d", v.keepTier(), want)
		}
	}
}
