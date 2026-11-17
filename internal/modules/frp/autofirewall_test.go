package frp

import (
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/fwopen"
)

// waitFirewall 等待放行状态满足条件（放行由后台协程异步写入）。
func waitFirewall(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("等待超时: %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// frps 运行且开启自动放行时登记放行，关闭开关或实例停止时撤销；未开启时从不登记。
func TestServerAutoFirewallLifecycle(t *testing.T) {
	m := &Module{fw: fwopen.NewGroup("frps-test")}
	t.Cleanup(m.fw.ClearAll)
	s := ServerSettings{BindPort: 17000, KCPBindPort: 17000, QUICBindPort: 17001}

	// 未开启：不登记，状态为空
	m.publishServerPorts("a", s, true)
	if st := m.fw.Status("a"); st != "" {
		t.Fatalf("未开启自动放行时不应登记: %q", st)
	}

	// 开启后登记（测试环境没有可用的防火墙后端，放行结果为失败，但已登记）
	s.AutoFirewall = true
	m.publishServerPorts("a", s, true)
	if m.fw.Status("a") == "" {
		t.Fatal("开启自动放行后应登记")
	}
	waitFirewall(t, "放行处理完成", func() bool { return m.fw.Status("a") != "正在放行防火墙端口" })

	// 关闭开关后撤销
	s.AutoFirewall = false
	m.syncServerFirewall("a", s)
	waitFirewall(t, "关闭开关后撤销", func() bool { return m.fw.Status("a") == "" })

	// 实例停止时撤销
	s.AutoFirewall = true
	m.publishServerPorts("a", s, true)
	if m.fw.Status("a") == "" {
		t.Fatal("重新开启后应登记")
	}
	m.publishServerPorts("a", s, false)
	waitFirewall(t, "实例停止后撤销", func() bool { return m.fw.Status("a") == "" })
}

// 实例视图带放行状态；只监听回环地址的实例说明无需放行。
func TestServerViewFirewallStatus(t *testing.T) {
	m := &Module{fw: fwopen.NewGroup("frps-view")}
	t.Cleanup(m.fw.ClearAll)
	srv := &Server{Key: "lo", ServerSettings: ServerSettings{BindAddr: "127.0.0.1", BindPort: 17000, AutoFirewall: true}}
	if v := m.serverView(srv); v.Firewall != fwopen.LoopbackNote || v.FirewallFailed {
		t.Fatalf("回环实例应说明无需放行: %+v", v)
	}
	srv = &Server{Key: "pub", ServerSettings: ServerSettings{BindPort: 17000, AutoFirewall: true}}
	m.syncServerFirewall(srv.Key, srv.ServerSettings)
	if v := m.serverView(srv); v.Firewall == "" {
		t.Fatalf("已登记的实例应显示放行状态: %+v", v)
	}
	srv.AutoFirewall = false
	if v := m.serverView(srv); v.Firewall != "" || v.FirewallFailed {
		t.Fatalf("未开启自动放行时不显示状态: %+v", v)
	}
}
