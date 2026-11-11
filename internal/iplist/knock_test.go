package iplist

import (
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/knockgate"
)

// 访问控制中的敲门：只设敲门时也会编译过滤器；名单与敲门同时设置时两者都要满足，并区分拒绝原因。
func TestFilterKnock(t *testing.T) {
	knockgate.Default.Clear()
	t.Cleanup(knockgate.Default.Clear)
	knockgate.Default.SetRules([]knockgate.Rule{{Key: "k1", Name: "SSH"}})
	t.Cleanup(func() { knockgate.Default.SetRules(nil) })
	in := &net.TCPAddr{IP: net.ParseIP("203.0.113.5"), Port: 1}
	out := &net.TCPAddr{IP: net.ParseIP("198.51.100.5"), Port: 1}

	if (&Access{}).Compile() != nil {
		t.Fatal("未设置时应不限制")
	}
	f := (&Access{Knock: "k1"}).Compile()
	if f == nil || f.Reason(in) != DenyKnock {
		t.Fatal("只设敲门时未敲门应拒绝")
	}
	_ = knockgate.Default.Allow(netip.MustParseAddr("203.0.113.5"), "k1", time.Now().Add(time.Minute))
	if !f.Allows(in) || f.Allows(out) {
		t.Fatal("只放行敲门成功的地址")
	}
	if f.Reason(&net.UnixAddr{Name: "x"}) != DenyKnock {
		t.Fatal("要求敲门时无法解析的地址应拒绝")
	}
	w := (&Access{IPMode: ModeWhitelist, IPList: []string{"198.51.100.0/24"}, Knock: "k1"}).Compile()
	if w.Reason(in) != DenyList {
		t.Fatal("不在白名单时按名单拒绝")
	}
	if w.Reason(out) != DenyKnock {
		t.Fatal("在白名单但未敲门时按敲门拒绝")
	}
	a := Access{Knock: " k1 "}
	if err := a.Normalize(); err != nil || a.Knock != "k1" {
		t.Fatalf("应去掉空白: %v %q", err, a.Knock)
	}
	if err := a.CheckSets(); err != nil {
		t.Fatal(err)
	}
	a.Knock = "nope"
	if err := a.CheckSets(); err == nil {
		t.Fatal("引用不存在的敲门规则应报错")
	}
}

// PublishUsers 同时登记敲门规则的引用。
func TestPublishKnockUsers(t *testing.T) {
	type rule struct {
		Access
		name string
	}
	items := []rule{{Access{Knock: "k1"}, "a"}, {Access{}, "b"}, {Access{Knock: "k1"}, "c"}}
	PublishUsers("test", items, func(r *rule) (*Access, string) { return &r.Access, "test / " + r.name })
	t.Cleanup(func() { knockgate.Default.SetUsers("test", nil) })
	if got := knockgate.Default.Users("k1"); len(got) != 2 || got[0] != "test / a" || got[1] != "test / c" {
		t.Fatalf("引用不对: %v", got)
	}
}
