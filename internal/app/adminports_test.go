package app

import (
	"slices"
	"testing"
)

// 管理界面的 HTTP 与 HTTPS 端口都要登记（含只监听回环地址的情况），供入侵防护计算诱饵端口时避开
func TestAdminPorts(t *testing.T) {
	cases := []struct {
		s    Settings
		want []int
	}{
		{Settings{Listen: ":16680"}, []int{16680}},
		{Settings{Listen: "127.0.0.1:19700", HTTPSListen: "[::]:16443"}, []int{19700, 16443}},
		{Settings{Listen: "0.0.0.0:http"}, []int{80}},
		{Settings{Listen: "bad", HTTPSListen: ""}, nil},
	}
	for _, c := range cases {
		if got := adminPorts(c.s); !slices.Equal(got, c.want) {
			t.Errorf("adminPorts(%+v) = %v，应为 %v", c.s, got, c.want)
		}
	}
}
