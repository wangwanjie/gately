package knockgate

import (
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"testing"
	"time"
)

// newWebRegistry 返回规则 k 已启用、时钟可控的登记表。
func newWebRegistry(now *time.Time) *Registry {
	r := New()
	r.SetClock(func() time.Time { return *now })
	r.SetRules([]Rule{{Key: "k", Name: "web", Enable: true}, {Key: "off", Name: "off"}})
	return r
}

func TestAllowWebPrefix(t *testing.T) {
	now := time.Unix(1000, 0)
	r := newWebRegistry(&now)
	p := netip.MustParsePrefix("2001:db8:1:2:aaaa::1/64")
	if err := r.AllowWeb(p, "k", now.Add(time.Hour), Owner{User: "alice", Site: "s1", Cred: "c1"}, netip.Addr{}); err != nil {
		t.Fatal(err)
	}
	// 同一 /64 中的其他地址（临时地址轮换）都放行
	for _, s := range []string{"2001:db8:1:2::1", "2001:db8:1:2:ffff:ffff:ffff:ffff"} {
		if !r.Allowed(netip.MustParseAddr(s), "k") {
			t.Fatalf("%s 应在放行网段内", s)
		}
	}
	for _, s := range []string{"2001:db8:1:3::1", "203.0.113.1"} {
		if r.Allowed(netip.MustParseAddr(s), "k") {
			t.Fatalf("%s 不在放行网段内", s)
		}
	}
	l := r.List()
	if len(l) != 1 || l[0].String() != "2001:db8:1:2::/64" || l[0].Source != SourceWeb || l[0].User != "alice" || l[0].Site != "s1" || l[0].Cred != "c1" {
		t.Fatalf("列表不对: %+v", l)
	}
	if r.Remaining(p, "k") != time.Hour {
		t.Fatalf("剩余时长: %v", r.Remaining(p, "k"))
	}
	// 按网段撤销后 v6Bits 计数归零，判断不再逐个前缀长度查找
	if n := r.RevokePrefix(netip.MustParsePrefix("2001:db8:1:2::/64"), ""); n != 1 {
		t.Fatalf("撤销数量 %d", n)
	}
	if len(r.v6Bits) != 0 || r.Allowed(netip.MustParseAddr("2001:db8:1:2::1"), "k") {
		t.Fatalf("撤销后计数应清零: %v", r.v6Bits)
	}
}

func TestAllowWebRuleAndAddr(t *testing.T) {
	now := time.Unix(1000, 0)
	r := newWebRegistry(&now)
	if err := r.AllowWeb(netip.MustParsePrefix("203.0.113.1/32"), "off", now.Add(time.Hour), Owner{User: "u", Site: "s", Cred: ""}, netip.Addr{}); !errors.Is(err, ErrRuleDisabled) {
		t.Fatalf("停用的规则应拒绝: %v", err)
	}
	if err := r.AllowWeb(netip.MustParsePrefix("203.0.113.1/32"), "missing", now.Add(time.Hour), Owner{User: "u", Site: "s", Cred: ""}, netip.Addr{}); !errors.Is(err, ErrRuleDisabled) {
		t.Fatalf("不存在的规则应拒绝: %v", err)
	}
	if err := r.AllowWeb(netip.Prefix{}, "k", now.Add(time.Hour), Owner{User: "u", Site: "s", Cred: ""}, netip.Addr{}); err == nil {
		t.Fatal("无效地址应拒绝")
	}
	// IPv4 映射地址按 IPv4 登记
	if err := r.AllowWeb(netip.MustParsePrefix("::ffff:203.0.113.9/128"), "k", now.Add(time.Hour), Owner{User: "u", Site: "s", Cred: ""}, netip.Addr{}); err != nil {
		t.Fatal(err)
	}
	if !r.Allowed(netip.MustParseAddr("203.0.113.9"), "k") || r.List()[0].String() != "203.0.113.9" {
		t.Fatalf("映射地址: %+v", r.List())
	}
	// 续期取较晚的到期时间，用户改记为最近一次登录的用户
	if err := r.AllowWeb(netip.MustParsePrefix("203.0.113.9/32"), "k", now.Add(time.Minute), Owner{User: "bob", Site: "s", Cred: "c2"}, netip.Addr{}); err != nil {
		t.Fatal(err)
	}
	g := r.List()[0]
	if !g.Until.Equal(now.Add(time.Hour)) || g.User != "bob" || g.Cred != "c2" {
		t.Fatalf("续期: %+v", g)
	}
	// 敲门报文对同一地址的授权不改变来源与用户
	_ = r.Allow(netip.MustParseAddr("203.0.113.9"), "k", now.Add(2*time.Hour))
	g = r.List()[0]
	if g.Source != SourceWeb || g.User != "bob" || !g.Until.Equal(now.Add(2*time.Hour)) {
		t.Fatalf("报文续期: %+v", g)
	}
}

func TestAllowWebPerUserCap(t *testing.T) {
	now := time.Unix(1000, 0)
	r := newWebRegistry(&now)
	for i := range MaxWebGrantsPerUser + 4 {
		p := netip.MustParsePrefix(fmt.Sprintf("198.51.100.%d/32", i+1))
		// 后登记的到期更晚：超出上限时替换掉的是最早登记的
		if err := r.AllowWeb(p, "k", now.Add(time.Duration(i+1)*time.Minute), Owner{User: "mallory", Site: "s"}, netip.Addr{}); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(r.List()); n != MaxWebGrantsPerUser {
		t.Fatalf("同一用户的授权数应不超过上限: %d", n)
	}
	for i := range 4 {
		if r.Allowed(netip.MustParseAddr(fmt.Sprintf("198.51.100.%d", i+1)), "k") {
			t.Fatalf("最早到期的授权应被替换: %d", i+1)
		}
	}
	// 其他用户、敲门报文的授权不受该用户的上限影响
	if err := r.AllowWeb(netip.MustParsePrefix("192.0.2.1/32"), "k", now.Add(time.Minute), Owner{User: "alice", Site: "s", Cred: ""}, netip.Addr{}); err != nil {
		t.Fatal(err)
	}
	_ = r.Allow(netip.MustParseAddr("192.0.2.2"), "k", now.Add(time.Minute))
	if n := len(r.List()); n != MaxWebGrantsPerUser+2 {
		t.Fatalf("其他授权不应被替换: %d", n)
	}
}

func TestRevokeFuncAndPrune(t *testing.T) {
	now := time.Unix(1000, 0)
	r := newWebRegistry(&now)
	_ = r.AllowWeb(netip.MustParsePrefix("2001:db8::/56"), "k", now.Add(time.Minute), Owner{User: "a", Site: "s1", Cred: ""}, netip.Addr{})
	_ = r.AllowWeb(netip.MustParsePrefix("2001:db9::/64"), "k", now.Add(time.Hour), Owner{User: "b", Site: "s2", Cred: ""}, netip.Addr{})
	_ = r.Allow(netip.MustParseAddr("2001:dba::1"), "k", now.Add(time.Hour))
	if len(r.v6Bits) != 2 {
		t.Fatalf("前缀长度计数: %v", r.v6Bits)
	}
	if n := r.RevokeFunc(func(g Grant) bool { return g.Site == "s2" }); n != 1 {
		t.Fatalf("按站点撤销: %d", n)
	}
	now = now.Add(2 * time.Minute)
	if r.Prune() != 1 || len(r.v6Bits) != 0 {
		t.Fatalf("到期清理后计数应清零: %v", r.v6Bits)
	}
	if !r.Allowed(netip.MustParseAddr("2001:dba::1"), "k") {
		t.Fatal("报文授权应保留")
	}
	// Revoke 只撤销单个地址的授权，不误删包含它的网段
	_ = r.AllowWeb(netip.MustParsePrefix("2001:dba::/64"), "k", now.Add(time.Hour), Owner{User: "a", Site: "s1", Cred: ""}, netip.Addr{})
	if r.Revoke(netip.MustParseAddr("2001:dba::1"), "") != 1 || !r.Allowed(netip.MustParseAddr("2001:dba::1"), "k") {
		t.Fatal("撤销单个地址后网段授权仍有效")
	}
	r.Clear()
	if len(r.v6Bits) != 0 || len(r.List()) != 0 {
		t.Fatal("清空")
	}
}

// TestConcurrent 在 -race 下检查并发读写。
func TestConcurrent(t *testing.T) {
	r := New()
	r.SetRules([]Rule{{Key: "k", Enable: true}})
	var wg sync.WaitGroup
	until := time.Now().Add(time.Hour)
	for w := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 500 {
				p := netip.MustParsePrefix(fmt.Sprintf("2001:db8:%x:%x::/64", w, i))
				_ = r.AllowWeb(p, "k", until, Owner{User: fmt.Sprint("u", w), Site: "s", Cred: ""}, netip.Addr{})
				_ = r.Allowed(netip.MustParseAddr(fmt.Sprintf("2001:db8:%x:%x::5", w, i)), "k")
				if i%7 == 0 {
					r.RevokePrefix(p, "k")
				}
			}
		}()
	}
	wg.Wait()
	_ = r.List()
}

// BenchmarkAllowedPrefix 衡量存在网段授权时的判断开销（端口转发、站点准入的每个新连接都会调用）。
func BenchmarkAllowedPrefix(b *testing.B) {
	r := New()
	r.SetRules([]Rule{{Key: "k", Enable: true}})
	until := time.Now().Add(time.Hour)
	for i := range 10000 {
		_ = r.AllowWeb(netip.MustParsePrefix(fmt.Sprintf("2001:db8:%x:%x::/64", i>>8, i&0xff)), "k", until, Owner{User: fmt.Sprint("u", i), Site: "s", Cred: ""}, netip.Addr{})
		_ = r.Allow(netip.MustParseAddr(fmt.Sprintf("198.51.%d.%d", i>>8, i&0xff)), "k", until)
	}
	miss := netip.MustParseAddr("2001:db9::1")
	hit := netip.MustParseAddr("2001:db8:1:2::7")
	b.ResetTimer()
	for i := range b.N {
		if i&1 == 0 {
			r.Allowed(miss, "k")
		} else {
			r.Allowed(hit, "k")
		}
	}
}

// Lookup 报告剩余时长、地址是否已记录、地址记录是否已满（调用方据此对轮换地址节流写入）。
func TestLookupFull(t *testing.T) {
	now := time.Unix(1000, 0)
	r := newWebRegistry(&now)
	p := netip.MustParsePrefix("2001:db8:7::/64")
	until := now.Add(time.Hour)
	if rem, known, full := r.Lookup(p, "k", netip.MustParseAddr("2001:db8:7::1")); rem != 0 || known || full {
		t.Fatal("没有授权时应全部为零值")
	}
	for i := 1; i <= MaxWebAddrs; i++ {
		a := netip.AddrFrom16([16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 7, 0, 0, 0, 0, 0, 0, 0, 0, 0, byte(i)})
		if err := r.AllowWeb(p, "k", until, Owner{User: "u"}, a); err != nil {
			t.Fatal(err)
		}
		_, known, full := r.Lookup(p, "k", a)
		if !known || full != (i == MaxWebAddrs) {
			t.Fatalf("第 %d 个地址: known=%v full=%v", i, known, full)
		}
	}
	rem, known, full := r.Lookup(p, "k", netip.MustParseAddr("2001:db8:7::ff"))
	if rem != time.Hour || known || !full {
		t.Fatalf("新地址应未记录且记录已满: %v %v %v", rem, known, full)
	}
}
