package protoroute

import (
	"context"
	"io"
	"log"
	"strconv"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/logbuf"
)

// 默认目标与路由目标写成「地址:端口」时拆出端口；与已填端口冲突或写法无法识别时拒绝。
func TestValidateSplitsHostPortTargets(t *testing.T) {
	rule := Rule{
		Protocols: []string{"tcp"}, ListenPorts: "443",
		TargetAddresses: []string{"127.0.0.1:8443"},
		Routes:          []Route{{Name: "ssh", Protocol: "ssh", TargetAddresses: []string{"[::1]:22"}}},
	}
	if err := rule.Validate(); err != nil {
		t.Fatal(err)
	}
	if rule.TargetAddresses[0] != "127.0.0.1" || rule.TargetPorts != "8443" {
		t.Fatalf("默认目标 %v / %q", rule.TargetAddresses, rule.TargetPorts)
	}
	if rt := rule.Routes[0]; rt.TargetAddresses[0] != "::1" || rt.TargetPort != 22 {
		t.Fatalf("路由目标 %v / %d", rt.TargetAddresses, rt.TargetPort)
	}
	bad := []Rule{
		{Protocols: []string{"tcp"}, ListenPorts: "443", TargetAddresses: []string{"127.0.0.1:8443"}, TargetPorts: "9443"},
		{Protocols: []string{"tcp"}, ListenPorts: "443", Routes: []Route{{Name: "ssh", Protocol: "ssh", TargetAddresses: []string{"127.0.0.1:22"}, TargetPort: 2222}}},
		{Protocols: []string{"tcp"}, ListenPorts: "443", Routes: []Route{{Name: "ssh", Protocol: "ssh", TargetAddresses: []string{"127.0.0.1:22:23"}}}},
	}
	for i, r := range bad {
		if err := r.Validate(); err == nil {
			t.Fatalf("用例 %d 应被拒绝", i)
		}
	}
}

// 旧版本保存的「地址:端口」目标：启动时告警，规则状态显示错误；修正后错误消失。
func TestLegacyHostPortTargetReported(t *testing.T) {
	listenPort := freePort(t)
	rule := Rule{
		Key: "k1", Name: "edge", Enable: true,
		Protocols: []string{"tcp4"}, ListenAddress: "127.0.0.1", ListenPorts: strconv.Itoa(listenPort),
		Routes: []Route{{Name: "ssh", Protocol: "ssh", TargetAddresses: []string{"127.0.0.1:22"}}},
	}
	m := New()
	m.ctx = context.Background()
	m.logs = logbuf.New(100, log.New(io.Discard, "", 0))
	m.cfg.Settings = defaultSettings()
	m.cfg.Rules = []Rule{rule}
	m.startRuleLocked(rule)
	defer m.stopRuleLocked("k1")
	if m.runners["k1"] == nil {
		t.Fatalf("规则应照常启动: %v", m.errors)
	}
	if v := m.viewLocked(rule); !strings.Contains(v.Error, "127.0.0.1:22") {
		t.Fatalf("规则状态应显示目标地址错误: %q", v.Error)
	}
	warned := false
	for _, e := range m.logs.Since(0) {
		if e.Level == logbuf.LevelWarn && strings.Contains(e.Message, "127.0.0.1:22") {
			warned = true
		}
	}
	if !warned {
		t.Fatal("启动时应记录告警")
	}
	rule = validRule(t, rule)
	m.cfg.Rules = []Rule{rule}
	m.restartLocked("k1")
	if v := m.viewLocked(rule); v.Error != "" {
		t.Fatalf("修正后应无错误: %q", v.Error)
	}
}
