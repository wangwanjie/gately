package banlist

import (
	"net"
	"net/netip"
	"testing"
	"time"
)

func TestParseKey(t *testing.T) {
	cases := map[string]string{
		"1.2.3.4":                "1.2.3.4",
		" ::ffff:1.2.3.4 ":       "1.2.3.4",
		"2001:db8::1":            "2001:db8::/64",
		"2001:db8:0:0:5::/80":    "2001:db8::/64",
		"1.2.3.4/32":             "1.2.3.4",
		"1.2.3.9/24":             "1.2.3.0/24",
		"2001:db8::/48":          "2001:db8::/48",
		"::ffff:10.0.0.0/104":    "10.0.0.0/8",
		"2001:db8:1:2:3:4:5:6/1": "::/1",
	}
	for in, want := range cases {
		got, err := ParseKey(in)
		if err != nil || got != want {
			t.Errorf("ParseKey(%q) = %q, %v，期望 %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "abc", "0.0.0.0/0", "::/0", "fe80::1%eth0", "1.2.3.4/33"} {
		if _, err := ParseKey(bad); err == nil {
			t.Errorf("ParseKey(%q) 应报错", bad)
		}
	}
}

func TestBanLookupAndUnban(t *testing.T) {
	s := New()
	var changes []Change
	s.Subscribe(func(c Change) { changes = append(changes, c) })
	if _, added, err := s.Ban("1.2.3.4", Options{Duration: time.Hour, Source: SourceManual}); err != nil || !added {
		t.Fatalf("封禁失败: %v %v", added, err)
	}
	if _, _, err := s.Ban("2001:db8::/64", Options{Duration: time.Hour}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Ban("10.1.0.0/16", Options{Permanent: true}); err != nil {
		t.Fatal(err)
	}
	for addr, want := range map[string]bool{
		"1.2.3.4": true, "::ffff:1.2.3.4": true, "1.2.3.5": false,
		"2001:db8::ffff": true, "2001:db8:0:1::1": false,
		"10.1.200.3": true, "10.2.0.1": false,
	} {
		if got := s.Banned(netip.MustParseAddr(addr)); got != want {
			t.Errorf("Banned(%s) = %v，期望 %v", addr, got, want)
		}
	}
	if !s.BannedAddr(&net.TCPAddr{IP: net.ParseIP("1.2.3.4"), Port: 80}) || s.BannedAddr(&net.UDPAddr{IP: net.ParseIP("8.8.8.8"), Port: 53}) {
		t.Fatal("BannedAddr 判断错误")
	}
	if s.Len() != 3 || len(changes) != 3 {
		t.Fatalf("记录数 %d、变化数 %d 不符", s.Len(), len(changes))
	}
	if !s.Unban("10.1.0.0/16") || s.Banned(netip.MustParseAddr("10.1.200.3")) {
		t.Fatal("解除网段封禁失败")
	}
	if len(changes) != 4 || len(changes[3].Removed) != 1 || changes[3].Expired {
		t.Fatalf("解除封禁应通知一次，且不算到期清除: %+v", changes)
	}
}

// 到期清除的通知带 Expired 标记，供持久化方区分；封禁与解封的通知不带
func TestPruneMarksExpired(t *testing.T) {
	s := New()
	var changes []Change
	s.Subscribe(func(c Change) { changes = append(changes, c) })
	if _, _, err := s.Ban("1.2.3.4", Options{Duration: 20 * time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Ban("5.6.7.8", Options{Duration: time.Hour}); err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 || changes[0].Expired || changes[1].Expired {
		t.Fatalf("封禁通知不应带到期标记: %+v", changes)
	}
	// 没有到期记录时不通知
	if got := s.Prune(); len(got) != 0 || len(changes) != 2 {
		t.Fatalf("没有到期记录时不应通知: %v %+v", got, changes)
	}
	time.Sleep(40 * time.Millisecond)
	got := s.Prune()
	if len(got) != 1 || got[0].Key != "1.2.3.4" {
		t.Fatalf("应清除到期记录: %+v", got)
	}
	if len(changes) != 3 || !changes[2].Expired || len(changes[2].Removed) != 1 || len(changes[2].Added) != 0 {
		t.Fatalf("到期清除应通知一次并带到期标记: %+v", changes)
	}
	if !s.Unban("5.6.7.8") || changes[3].Expired {
		t.Fatalf("手动解封不应带到期标记: %+v", changes)
	}
}

func TestIncrementAndExtend(t *testing.T) {
	s := New()
	o := Options{Duration: time.Hour, Increment: true, MaxDuration: 3 * time.Hour}
	e1, _, _ := s.Ban("5.5.5.5", o)
	if e1.Count != 1 || time.Until(e1.Until) > time.Hour+time.Second {
		t.Fatalf("首次封禁应为 1 小时: %+v", e1)
	}
	// 封禁期间再次触发不重复计数，时长不变时不算变化
	if e, added, _ := s.Ban("5.5.5.5", o); added || e.Count != 1 {
		t.Fatalf("封禁期间再次触发不应计数: %+v %v", e, added)
	}
	s.Unban("5.5.5.5")
	e2, _, _ := s.Ban("5.5.5.5", o)
	if e2.Count != 2 || time.Until(e2.Until) < 2*time.Hour-time.Second {
		t.Fatalf("第二次应翻倍为 2 小时: %+v", e2)
	}
	s.Unban("5.5.5.5")
	s.Unban("5.5.5.5")
	e3, _, _ := s.Ban("5.5.5.5", o)
	if e3.Count != 3 || time.Until(e3.Until) > 3*time.Hour+time.Second {
		t.Fatalf("第三次应受上限 3 小时限制: %+v", e3)
	}
	// 更长的封禁会延长已有记录，并保留首次封禁时间
	e4, added, _ := s.Ban("5.5.5.5", Options{Permanent: true})
	if !added || !e4.Permanent() || !e4.Created.Equal(e3.Created) {
		t.Fatalf("永久封禁应覆盖有期限的封禁: %+v", e4)
	}
	if _, added, _ := s.Ban("5.5.5.5", Options{Duration: time.Minute}); added {
		t.Fatal("较短的封禁不应覆盖永久封禁")
	}
}

func TestIgnoreAndPrune(t *testing.T) {
	s := New()
	white := []netip.Prefix{netip.MustParsePrefix("192.168.0.0/16"), netip.MustParsePrefix("2001:db8:1:2::5/128")}
	s.SetIgnore(func(p netip.Prefix) bool {
		for _, w := range white {
			if w.Overlaps(p) {
				return true
			}
		}
		return false
	})
	if _, _, err := s.Ban("192.168.1.2", Options{Duration: time.Hour}); err != ErrIgnored {
		t.Fatalf("白名单地址应被跳过: %v", err)
	}
	// 白名单中的单个 IPv6 地址保护它所在的整个 /64，同网段其他设备触发的封禁也被跳过
	if _, _, err := s.Ban(KeyFor(netip.MustParseAddr("2001:db8:1:2::99")), Options{Duration: time.Hour}); err != ErrIgnored {
		t.Fatalf("与白名单重叠的 /64 应被跳过: %v", err)
	}
	if !s.Ignored(netip.MustParseAddr("2001:db8:1:2::99")) || s.Ignored(netip.MustParseAddr("2001:db8:1:3::1")) {
		t.Fatal("Ignored 应按 /64 判断重叠")
	}
	if _, _, err := s.Ban("192.168.1.2", Options{Duration: time.Hour, Force: true}); err != nil {
		t.Fatalf("手动封禁不受白名单限制: %v", err)
	}
	s.Ban("7.7.7.7", Options{Duration: time.Millisecond})
	time.Sleep(5 * time.Millisecond)
	if s.Banned(netip.MustParseAddr("7.7.7.7")) {
		t.Fatal("到期后不应再封禁")
	}
	removed := s.Prune()
	if len(removed) != 1 || removed[0].Key != "7.7.7.7" || s.Len() != 1 {
		t.Fatalf("Prune 结果不符: %+v", removed)
	}
}

func TestExportImport(t *testing.T) {
	s := New()
	s.Ban("1.1.1.1", Options{Duration: time.Hour, Reason: "r"})
	s.Ban("2.2.2.2", Options{Duration: time.Millisecond})
	time.Sleep(5 * time.Millisecond)
	entries, offenses := s.Export()
	d := New()
	d.Import(entries, offenses)
	if d.Len() != 1 || !d.Banned(netip.MustParseAddr("1.1.1.1")) {
		t.Fatalf("导入后应只有未到期的记录: %+v", d.List())
	}
	// 累计次数一并导入，再次封禁时继续递增
	d.Unban("1.1.1.1")
	if e, _, _ := d.Ban("1.1.1.1", Options{Duration: time.Hour, Increment: true}); e.Count != 2 {
		t.Fatalf("累计次数应为 2: %+v", e)
	}
}

func TestListener(t *testing.T) {
	s := New()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	wrapped := s.Listener(ln)
	defer wrapped.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := wrapped.Accept()
		if err == nil {
			accepted <- c
		}
	}()
	s.Ban("127.0.0.1", Options{Duration: time.Hour, Force: true})
	c1, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	// 被封禁的连接会被服务端直接关闭
	_ = c1.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := c1.Read(make([]byte, 1)); err == nil {
		t.Fatal("被封禁的连接应被关闭")
	}
	c1.Close()
	s.Unban("127.0.0.1")
	c2, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	select {
	case c := <-accepted:
		c.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("解除封禁后应能建立连接")
	}
}
