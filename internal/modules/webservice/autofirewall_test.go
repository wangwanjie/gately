package webservice

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/fwopen"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/luckyimport"
	"github.com/wangwanjie/gately/internal/module"
)

// firewallOf 等待并返回监听在列表接口中的放行说明（放行在后台进行，最初为“正在放行”）。
func firewallOf(t *testing.T, m *Module, key string) string {
	t.Helper()
	var s string
	for i := 0; i < 200; i++ {
		m.mu.Lock()
		for i := range m.cfg.Listeners {
			if m.cfg.Listeners[i].Key == key {
				s = m.viewLocked(&m.cfg.Listeners[i]).(ListenerView).Firewall
			}
		}
		m.mu.Unlock()
		if !strings.Contains(s, "正在放行") {
			return s
		}
		time.Sleep(10 * time.Millisecond)
	}
	return s
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func newFirewallModule(t *testing.T, ls ...Listener) *Module {
	t.Helper()
	st, _ := config.Open(t.TempDir())
	m := New(nil, nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	for i := range ls {
		if err := ls[i].Validate(); err != nil {
			t.Fatal(err)
		}
	}
	m.cfg.Listeners = ls
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Stop() })
	return m
}

func TestListenerAutoFirewall(t *testing.T) {
	site := []Site{{Enable: true, Domains: []string{"a.test"}, Type: TypeText, Text: "ok"}}
	// 监听所有地址并开启自动放行：登记了放行（测试后端不改防火墙，结果为失败说明）
	wild := Listener{Key: "wild", Enable: true, Network: "tcp4", Port: freePort(t), AutoFirewall: true, Sites: site}
	// 只监听回环地址：无需放行
	loop := Listener{Key: "loop", Enable: true, Network: "tcp4", ListenIP: "127.0.0.1", Port: freePort(t), AutoFirewall: true, Sites: site}
	// 未开启
	off := Listener{Key: "off", Enable: true, Network: "tcp4", Port: freePort(t), Sites: site}
	m := newFirewallModule(t, wild, loop, off)

	if s := firewallOf(t, m, "wild"); !strings.Contains(s, "TCP") || !strings.Contains(s, " (IPv4)") {
		t.Fatalf("wild firewall = %q", s)
	}
	// 只监听回环地址：不放行，只给出说明
	if s := firewallOf(t, m, "loop"); s != fwopen.LoopbackNote || m.fw.Status("loop") != "" {
		t.Fatalf("loopback listener should not be opened: %q", s)
	}
	if s := firewallOf(t, m, "off"); s != "" {
		t.Fatalf("disabled auto firewall: %q", s)
	}

	// 关闭开关走热更新，不重启监听，放行随即撤销
	m.mu.Lock()
	r := m.runners["wild"]
	m.cfg.Listeners[0].AutoFirewall = false
	m.restartLocked("wild")
	same := m.runners["wild"] == r
	m.mu.Unlock()
	if !same {
		t.Fatal("切换自动放行不应重启监听")
	}
	if s := firewallOf(t, m, "wild"); s != "" {
		t.Fatalf("after disabling: %q", s)
	}
	m.mu.Lock()
	m.cfg.Listeners[0].AutoFirewall = true
	m.restartLocked("wild")
	m.mu.Unlock()
	if s := firewallOf(t, m, "wild"); s == "" {
		t.Fatal("re-enabled auto firewall should register")
	}

	// 停用监听后撤销
	m.mu.Lock()
	m.cfg.Listeners[0].Enable = false
	m.restartLocked("wild")
	m.mu.Unlock()
	if s := m.fw.Status("wild"); s != "" {
		t.Fatalf("stopped listener still registered: %q", s)
	}
}

// 端口被其他程序占用导致监听失败时绝不放行：否则会把占用端口的程序（如路由器管理页面）暴露到公网。
func TestListenerAutoFirewallSkipsFailedListen(t *testing.T) {
	busy, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	port := busy.Addr().(*net.TCPAddr).Port
	l := Listener{Key: "busy", Enable: true, Network: "tcp4", Port: port, AutoFirewall: true,
		Sites: []Site{{Enable: true, Domains: []string{"a.test"}, Type: TypeText, Text: "ok"}}}
	m := newFirewallModule(t, l)
	m.mu.Lock()
	_, running := m.runners["busy"]
	errMsg := m.errors["busy"]
	m.mu.Unlock()
	if running || errMsg == "" {
		t.Fatalf("listener should fail: running=%v err=%q", running, errMsg)
	}
	if s := m.fw.Status("busy"); s != "" {
		t.Fatalf("failed listener must not be opened: %q", s)
	}
}

// 模块停止时同步撤销全部放行。
func TestListenerAutoFirewallStop(t *testing.T) {
	l := Listener{Key: "s", Enable: true, Network: "tcp4", Port: freePort(t), AutoFirewall: true,
		Sites: []Site{{Enable: true, Domains: []string{"a.test"}, Type: TypeText, Text: "ok"}}}
	m := newFirewallModule(t, l)
	if firewallOf(t, m, "s") == "" {
		t.Fatal("should register")
	}
	m.Stop()
	if s := m.fw.Status("s"); s != "" {
		t.Fatalf("after stop: %q", s)
	}
}

func TestConvertLuckyAutoFirewall(t *testing.T) {
	raw, _ := json.Marshal([]map[string]any{
		{"RuleName": "a", "Enable": true, "ListenPort": 16666, "Network": "tcp", "AutoOptionsFirewall": true,
			"DefaultProxy": map[string]any{"Locations": []string{"http://192.168.1.8:80"}, "WebServiceType": "reverseproxy"}},
		{"RuleName": "b", "Enable": true, "ListenPort": 16667, "Network": "tcp",
			"DefaultProxy": map[string]any{"Locations": []string{"http://192.168.1.8:80"}, "WebServiceType": "reverseproxy"}},
	})
	ls, _, _, err := convertLuckyListeners(raw, luckyimport.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(ls) != 2 || !ls[0].AutoFirewall || ls[1].AutoFirewall {
		t.Fatalf("listeners = %+v", ls)
	}
}
