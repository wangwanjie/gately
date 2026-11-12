package easytier

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRealBinaries 用真实的 easytier-core 与 easytier-cli 验证两个实例组网互通。
// 设置环境变量 EASYTIER_BIN_DIR 为包含这两个程序的目录时才运行（用户态网络，不需要特权）。
func TestRealBinaries(t *testing.T) {
	binDir := os.Getenv("EASYTIER_BIN_DIR")
	if binDir == "" {
		t.Skip("未设置 EASYTIER_BIN_DIR")
	}
	m, _ := newTestModule(t)
	m.cfg.Settings.CorePath = filepath.Join(binDir, "easytier-core")
	a := validOptions()
	a.DHCP, a.IPv4, a.NoTun, a.Hostname = false, "10.77.1.1", true, "real-a"
	a.Listeners = "tcp://127.0.0.1:23010"
	b := validOptions()
	b.DHCP, b.IPv4, b.NoTun, b.Hostname = false, "10.77.1.2", true, "real-b"
	b.Peers = "tcp://127.0.0.1:23010"
	b.PortForwards = "tcp://127.0.0.1:23022/10.77.1.1:22"
	m.cfg.Instances = []Instance{{Key: "a", Name: "a", Enable: true, Options: a}, {Key: "b", Name: "b", Enable: true, Options: b}}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	var v StatusView
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		for _, rt := range m.runs {
			rt.cacheAt = time.Time{}
		}
		m.mu.Unlock()
		v, _ = m.status("b")
		if len(v.Peers) == 1 && v.Peers[0].Direct {
			break
		}
		time.Sleep(time.Second)
	}
	if v.Status != "running" || len(v.Peers) != 1 || v.Peers[0].Name != "real-a" || !v.Peers[0].Direct || v.Peers[0].IP != "10.77.1.1" {
		t.Fatalf("两个实例应互相直连: %+v\n%s", v, logText(m))
	}
	if v.Self == nil || v.Self.IP != "10.77.1.2" || v.Version == "" {
		t.Errorf("本机信息错误: %+v", v.Self)
	}
	if a, _ := m.status("a"); len(a.Peers) != 1 || a.Self == nil || len(a.Self.Listeners) != 1 || a.Self.Listeners[0] != "tcp://127.0.0.1:23010" {
		t.Errorf("a 的监听地址与节点错误: %+v", a)
	}
	// 真实程序启动时会把含网络密钥的配置打印出来，日志里必须已打码
	if txt := logText(m); strings.Contains(txt, "s3cret") || !strings.Contains(txt, "network_secret") {
		t.Errorf("日志应含打码后的启动配置且不含密钥:\n%s", txt)
	}
	t.Logf("b 的状态: %+v", v)
}
