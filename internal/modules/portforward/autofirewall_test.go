package portforward

import (
	"encoding/json"
	"net"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/fwopen"
)

// consecutivePorts 找到 n 个连续的、TCP 与 UDP 都空闲的端口。
func consecutivePorts(t *testing.T, n int) int {
	t.Helper()
	for try := 0; try < 50; try++ {
		base := freePort(t, "tcp")
		if base+n > 65535 {
			continue
		}
		ok := true
		for p := base; p < base+n && ok; p++ {
			ln, err := net.Listen("tcp", ":"+strconv.Itoa(p))
			if err != nil {
				ok = false
				break
			}
			ln.Close()
			pc, err := net.ListenPacket("udp", ":"+strconv.Itoa(p))
			if err != nil {
				ok = false
				break
			}
			pc.Close()
		}
		if ok {
			return base
		}
	}
	t.Fatal("找不到连续的空闲端口")
	return 0
}

// 只放行监听成功的端口，连续端口合并成区间；被占用的端口不放行。
func TestFirewallPortsOnlyListening(t *testing.T) {
	base := consecutivePorts(t, 4)
	busy, err := net.Listen("tcp4", "0.0.0.0:"+strconv.Itoa(base+3))
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	rule := Rule{
		Protocols:       []string{"tcp4", "udp6"},
		ListenPorts:     strconv.Itoa(base) + "-" + strconv.Itoa(base+3),
		TargetAddresses: []string{"127.0.0.1"},
		TargetPorts:     "1",
		AutoFirewall:    true,
	}
	r := testRunner(rule)
	if err := r.start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer r.stop()
	want := []fwopen.Port{
		{Proto: "tcp", Port: base, End: base + 2, Family: "4"},
		{Proto: "udp", Port: base, End: base + 3, Family: "6"},
	}
	if got := r.firewallPorts(); !reflect.DeepEqual(got, want) {
		t.Fatalf("firewallPorts = %+v, want %+v", got, want)
	}

	// 只监听回环地址时不放行
	rule.ListenAddress = "127.0.0.1"
	rule.Protocols = []string{"tcp4"}
	rule.ListenPorts = strconv.Itoa(freePort(t, "tcp"))
	r2 := testRunner(rule)
	if err := r2.start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer r2.stop()
	if got := r2.firewallPorts(); len(got) != 0 {
		t.Fatalf("loopback ports = %+v", got)
	}
}

func firewallStatus(t *testing.T, m *Module, key string) string {
	t.Helper()
	var s string
	for i := 0; i < 200; i++ {
		m.mu.Lock()
		for _, rule := range m.cfg.Rules {
			if rule.Key == key {
				s = m.viewLocked(rule).Firewall
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

func TestModuleAutoFirewall(t *testing.T) {
	rule := Rule{
		Key: "k", Enable: true, Protocols: []string{"tcp4"},
		ListenPorts: strconv.Itoa(freePort(t, "tcp")), TargetAddresses: []string{"127.0.0.1"}, TargetPorts: "1",
		AutoFirewall: true,
	}
	m := startedModule(t, rule)
	if s := firewallStatus(t, m, "k"); !strings.Contains(s, "TCP") {
		t.Fatalf("firewall = %q", s)
	}
	// 切换开关热更新，不重启监听
	r := m.runners["k"]
	rule.AutoFirewall = false
	editRule(t, m, rule)
	if m.runners["k"] != r {
		t.Fatal("切换自动放行不应重启监听")
	}
	if s := firewallStatus(t, m, "k"); s != "" {
		t.Fatalf("after disabling: %q", s)
	}
	rule.AutoFirewall = true
	editRule(t, m, rule)
	if firewallStatus(t, m, "k") == "" {
		t.Fatal("re-enable should register")
	}
	_ = m.Stop()
	if s := m.fw.Status("k"); s != "" {
		t.Fatalf("after stop: %q", s)
	}
}

func TestConvertLuckyOpenFirewallPorts(t *testing.T) {
	raw, _ := json.Marshal([]map[string]any{
		{"Name": "a", "Enable": true, "ForwardTypes": []string{"tcp"}, "ListenPorts": "8080", "TargetAddressList": []string{"192.168.1.2"}, "TargetPorts": "80", "OpenFirewallPorts": true},
		{"Name": "b", "Enable": true, "ForwardTypes": []string{"tcp"}, "ListenPorts": "8081", "TargetAddressList": []string{"192.168.1.2"}, "TargetPorts": "80"},
	})
	rules, _, err := convertLuckyRules(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 || !rules[0].AutoFirewall || rules[1].AutoFirewall {
		t.Fatalf("rules = %+v", rules)
	}
}
