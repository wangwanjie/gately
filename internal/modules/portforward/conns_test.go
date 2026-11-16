package portforward

import (
	"testing"

	"github.com/wangwanjie/gately/internal/l4proxy"
)

// 单来源上限：未设置时取最大连接数的一半，规则设置优先于全局默认值。
func TestEffectiveConnLimits(t *testing.T) {
	s := defaultSettings()
	cases := []struct {
		rule Rule
		def  int
		want l4proxy.Limits
	}{
		{Rule{}, 0, l4proxy.Limits{Max: 256, PerSource: 128}},
		{Rule{MaxConnections: 10}, 0, l4proxy.Limits{Max: 10, PerSource: 5}},
		{Rule{MaxConnections: 1}, 0, l4proxy.Limits{Max: 1, PerSource: 1}},
		{Rule{}, 32, l4proxy.Limits{Max: 256, PerSource: 32}},
		{Rule{MaxConnsPerSource: 8}, 32, l4proxy.Limits{Max: 256, PerSource: 8}},
		{Rule{MaxConnections: 4, MaxConnsPerSource: 100}, 0, l4proxy.Limits{Max: 4, PerSource: 4}},
	}
	for _, c := range cases {
		st := s
		st.DefaultMaxConnsPerSource = c.def
		if got, _ := effectiveLimits(c.rule, st); got != c.want {
			t.Fatalf("%+v / 默认 %d: %+v，应为 %+v", c.rule, c.def, got, c.want)
		}
	}
	z := Settings{DefaultMaxConnsPerSource: -3}
	z.normalize()
	if z.DefaultMaxConnsPerSource != 0 {
		t.Fatalf("规范化结果不对: %+v", z)
	}
	bad := Rule{Protocols: []string{"tcp"}, ListenPorts: "1", TargetPorts: "1", TargetAddresses: []string{"127.0.0.1"}, MaxConnsPerSource: -1}
	if err := bad.Validate(); err == nil {
		t.Fatal("单来源上限为负数应校验失败")
	}
}
