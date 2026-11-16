package portforward

import (
	"context"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// 目标地址写成「地址:端口」、目标端口留空时，端口拆到目标端口，转发能连通。
func TestValidateSplitsHostPortTarget(t *testing.T) {
	echoPort, closeEcho := tcpEcho(t)
	defer closeEcho()
	listen := freePort(t, "tcp")
	rule := Rule{
		Key: "k", Enable: true, Protocols: []string{"tcp"}, ListenAddress: "127.0.0.1",
		ListenPorts:     strconv.Itoa(listen),
		TargetAddresses: []string{"127.0.0.1:" + strconv.Itoa(echoPort)},
	}
	if err := rule.Validate(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(rule.TargetAddresses, ",") != "127.0.0.1" || rule.TargetPorts != strconv.Itoa(echoPort) {
		t.Fatalf("拆分结果 %v / %q", rule.TargetAddresses, rule.TargetPorts)
	}
	m := startedModule(t, rule)
	c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(listen), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := echoOnce(t, c, []byte("hello")); err != nil {
		t.Fatalf("拆分后应能连通: %v", err)
	}
	m.mu.Lock()
	v := m.viewLocked(m.cfg.Rules[0])
	m.mu.Unlock()
	if !v.Running || v.Error != "" {
		t.Fatalf("状态 = %+v", v)
	}
}

// 目标端口已填且与地址里的端口不同、或写法无法识别时拒绝保存。
func TestValidateRejectsConflictingTarget(t *testing.T) {
	for _, c := range []struct {
		targets []string
		ports   string
	}{
		{[]string{"127.0.0.1:19210"}, "19221"},
		{[]string{"127.0.0.1:19210:19221"}, ""},
		{[]string{"http://127.0.0.1:8080"}, ""},
	} {
		rule := Rule{Protocols: []string{"tcp"}, ListenPorts: "19221", TargetAddresses: c.targets, TargetPorts: c.ports}
		if err := rule.Validate(); err == nil {
			t.Fatalf("%v / %q 应被拒绝", c.targets, c.ports)
		}
	}
	// 与目标端口相同时接受
	rule := Rule{Protocols: []string{"tcp"}, ListenPorts: "19221", TargetAddresses: []string{"[::1]:19210"}, TargetPorts: "19210"}
	if err := rule.Validate(); err != nil || rule.TargetAddresses[0] != "::1" {
		t.Fatalf("相同端口应接受: %v %v", err, rule.TargetAddresses)
	}
}

// 旧版本已保存的「地址:端口」目标：启动时记录告警，规则状态显示错误，不能只显示运行中。
func TestLegacyHostPortTargetReported(t *testing.T) {
	st, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	listen := freePort(t, "tcp")
	legacy := fileConfig{Settings: defaultSettings(), Rules: []Rule{{
		Key: "k", Name: "旧规则", Enable: true, Protocols: []string{"tcp"}, ListenAddress: "127.0.0.1",
		ListenPorts: strconv.Itoa(listen), TargetAddresses: []string{"127.0.0.1:19210"},
	}}}
	if err := st.Save(ID, legacy); err != nil {
		t.Fatal(err)
	}
	logs := logbuf.New(100, log.New(io.Discard, "", 0))
	m := New()
	if err := m.Init(module.Env{Store: st, Logs: logs}); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	m.mu.Lock()
	v := m.viewLocked(m.cfg.Rules[0])
	m.mu.Unlock()
	if !strings.Contains(v.Error, "127.0.0.1:19210") {
		t.Fatalf("规则状态应显示目标地址错误: %+v", v)
	}
	found := false
	for _, e := range logs.Since(0) {
		if e.Level == logbuf.LevelWarn && strings.Contains(e.Message, "127.0.0.1:19210") {
			found = true
		}
	}
	if !found {
		t.Fatal("启动时应记录告警")
	}
	// 编辑保存后（接口校验会拆出端口）错误随之消失
	fixed := m.cfg.Rules[0]
	editRule(t, m, fixed)
	m.mu.Lock()
	v = m.viewLocked(m.cfg.Rules[0])
	m.mu.Unlock()
	if v.Error != "" || !v.Running || v.TargetPorts != "19210" {
		t.Fatalf("修正后状态 = %+v", v)
	}
}
