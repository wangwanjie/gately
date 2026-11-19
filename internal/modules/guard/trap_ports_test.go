package guard

import (
	"net"
	"slices"
	"strconv"
	"testing"
	"time"

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

// freePorts 返回 n 个当前空闲的 TCP 端口。
func freePorts(t *testing.T, n int) []int {
	t.Helper()
	var out []int
	var lns []net.Listener
	for range n {
		ln, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatal(err)
		}
		lns = append(lns, ln)
		out = append(out, ln.Addr().(*net.TCPAddr).Port)
	}
	for _, ln := range lns {
		ln.Close()
	}
	return out
}

// TestUserTrapFollowsPorts 覆盖用户态诱饵（防火墙不可用时）跟随端口登记刷新：模块登记端口后诱饵让出该端口，
// 模块可以重新监听；端口表没变时保持现有监听，不重启。
func TestUserTrapFollowsPorts(t *testing.T) {
	m := newTestModule(t)
	old := listeningPortsFn
	listeningPortsFn = func() (map[int]bool, error) { return map[int]bool{}, nil }
	t.Cleanup(func() { listeningPortsFn = old })
	ports := freePorts(t, 2)
	m.mu.Lock()
	m.cfg.Settings.Trap = TrapSettings{Enable: true, Ports: strconv.Itoa(ports[0]) + "," + strconv.Itoa(ports[1])}
	m.applyTrapLocked()
	first := m.trap
	m.mu.Unlock()
	if first == nil || len(first.listening()) != 2 {
		t.Fatalf("应监听两个诱饵端口: %+v", first)
	}

	m.refreshTrap()
	if m.trap != first {
		t.Fatal("端口表没变时不应重启诱饵监听")
	}

	openports.SetPorts("test-trap-module", []int{ports[1]})
	t.Cleanup(func() { openports.Clear("test-trap-module") })
	// 不手动刷新：防火墙轮询协程收到登记变化通知后应自行刷新用户态诱饵
	var got []int
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		m.mu.Lock()
		got = m.trap.listening()
		m.mu.Unlock()
		if slices.Equal(got, []int{ports[0]}) {
			break
		}
	}
	if !slices.Equal(got, []int{ports[0]}) {
		t.Fatalf("登记的端口应从诱饵中移除: %v", got)
	}
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(ports[1]))
	if err != nil {
		t.Fatalf("诱饵让出端口后模块应能重新监听: %v", err)
	}
	ln.Close()

	m.mu.Lock()
	m.cfg.Settings.Trap.Enable = false
	m.mu.Unlock()
	m.refreshTrap()
	if m.trap != nil {
		t.Fatal("关闭诱饵端口后刷新应停止监听")
	}
}
