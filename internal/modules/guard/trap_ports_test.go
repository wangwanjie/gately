package guard

import (
	"slices"
	"testing"

	"github.com/wangwanjie/gately/internal/openports"
)

// TestPortSetsExcludeRegistered 覆盖诱饵集合的保护：模块在 openports 登记的端口（例如 EasyTier 重启的空档、
// frps 端口范围）此刻没有监听也不能进入诱饵集合；界面提示的“正在被监听，已排除”只列监听中的端口。
func TestPortSetsExcludeRegistered(t *testing.T) {
	openports.SetPorts("test-trap-registered", []int{11010})
	openports.Set("test-trap-range", openports.Entry{Ranges: []openports.Range{{Lo: 11015, Hi: 11017}}})
	t.Cleanup(func() {
		openports.Clear("test-trap-registered")
		openports.Clear("test-trap-range")
	})
	s := Settings{Trap: TrapSettings{Enable: true, Ports: "11000-11020"}}
	_, trap, skipped := portSets(s, map[int]bool{11005: true})
	if got := formatPorts(trap); got != "11000-11004,11006-11009,11011-11014,11018-11020" {
		t.Fatalf("诱饵端口应去掉监听与登记的端口: %s", got)
	}
	if !slices.Equal(skipped, []int{11005}) {
		t.Fatalf("跳过列表只应包含正在监听的端口: %v", skipped)
	}
}

