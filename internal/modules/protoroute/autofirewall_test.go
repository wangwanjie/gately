package protoroute

import (
	"io"
	"log"
	"net"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/fwopen"
	"github.com/wangwanjie/gately/internal/logbuf"
)

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
	// 两个端口：一个空闲，一个被占用（不放行）
	busy, err := net.Listen("tcp6", "[::]:0")
	if err != nil {
		t.Skip("没有 IPv6:", err)
	}
	defer busy.Close()
	busyPort := busy.Addr().(*net.TCPAddr).Port
	free := freePort(t)
	rule := validRule(t, Rule{
		Key: "k1", Name: "edge", Enable: true, AutoFirewall: true,
		Protocols: []string{"tcp6"}, ListenPorts: strconv.Itoa(free) + "," + strconv.Itoa(busyPort),
		TargetAddresses: []string{"127.0.0.1"}, TargetPorts: "1",
	})
	m := New()
	m.ctx = t.Context()
	m.logs = logbuf.New(100, log.New(io.Discard, "", 0))
	m.cfg.Settings = defaultSettings()
	m.cfg.Rules = []Rule{rule}
	m.startRuleLocked(rule)
	r := m.runners["k1"]
	if r == nil {
		t.Fatalf("规则未启动: %v", m.errors)
	}
	defer m.Stop()
	want := []fwopen.Port{{Proto: "tcp", Port: free, Family: "6"}}
	if got := r.firewallPorts(); !reflect.DeepEqual(got, want) {
		t.Fatalf("firewallPorts = %+v, want %+v", got, want)
	}
	if s := firewallStatus(t, m, "k1"); !strings.Contains(s, "TCP "+strconv.Itoa(free)+" (IPv6)") {
		t.Fatalf("firewall = %q", s)
	}

	// 有端口监听失败的规则保存时会重启以便重试；放开占用后重启，两个端口都放行
	busy.Close()
	m.mu.Lock()
	m.restartLocked("k1")
	r = m.runners["k1"]
	m.mu.Unlock()
	if len(r.listenErrors) != 0 {
		t.Fatalf("重启后仍有监听失败: %v", r.listenErrors)
	}
	if got := len(r.firewallPorts()); got != 1 && got != 2 {
		t.Fatalf("firewallPorts = %+v", r.firewallPorts())
	}

	// 全部端口正常时切换开关走热更新
	rule.AutoFirewall = false
	rule = validRule(t, rule)
	m.mu.Lock()
	m.cfg.Rules = []Rule{rule}
	m.restartLocked("k1")
	same := m.runners["k1"] == r
	m.mu.Unlock()
	if !same {
		t.Fatal("切换自动放行不应重启监听")
	}
	if s := firewallStatus(t, m, "k1"); s != "" {
		t.Fatalf("after disabling: %q", s)
	}
}
