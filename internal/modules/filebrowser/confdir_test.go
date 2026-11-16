package filebrowser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// File Browser 的共享目录由外部程序直接读取，无法隐藏其中的配置目录：与配置目录重叠（是它、在其中或包含它）一律拒绝；
// 升级前保存的这类目录不交给 File Browser，其余共享目录照常使用。
func TestSourcesOutsideConfigDir(t *testing.T) {
	base := t.TempDir()
	conf := filepath.Join(base, "conf")
	share := filepath.Join(base, "share")
	if err := os.MkdirAll(share, 0o755); err != nil {
		t.Fatal(err)
	}
	m, _ := newTestModule(t, conf)
	for name, p := range map[string]string{
		"配置目录": conf, "配置目录内": filepath.Join(conf, "x"), "包含配置目录": base, "根目录": "/",
	} {
		i := validInstance()
		i.Sources = []Source{{Path: p}}
		err := i.Validate()
		if err == nil || !strings.Contains(err.Error(), "配置目录") {
			t.Errorf("%s（%s）应被拒绝，实际 %v", name, p, err)
		}
	}
	ok := validInstance()
	ok.Sources = []Source{{Path: share}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("无关目录应能使用: %v", err)
	}

	// 升级前保存的实例：一个共享目录包含配置目录，一个正常
	bin := filepath.Join(conf, "bin", "filebrowser")
	writeScript(t, bin, quantumVersionScript+"exec sleep 30\n")
	m.mu.Lock()
	m.cfg.Instances = []Instance{{Key: "old", Name: "old", Enable: true, Port: 18080, AdminUser: "admin", AdminPassword: "Secret123",
		AuthMethod: AuthPassword, BaseURL: "/", Sources: []Source{{Path: base, Name: "all"}, {Path: share, Name: "share"}}}}
	m.mu.Unlock()
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	cfgFile := filepath.Join(m.dataDir("old"), "config.yaml")
	waitFor(t, "写出配置", func() bool {
		_, err := os.Stat(cfgFile)
		return err == nil
	})
	b, err := os.ReadFile(cfgFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"path": "`+base+`"`) || !strings.Contains(string(b), `"path": "`+share+`"`) {
		t.Fatalf("包含配置目录的共享目录不应交给 File Browser，其余照常: %s", b)
	}
	// 只剩下与配置目录重叠的共享目录时不启动
	m.mu.Lock()
	m.cfg.Instances[0].Sources = []Source{{Path: base, Name: "all"}}
	m.applyLocked("old")
	errText := m.runs["old"].err
	m.mu.Unlock()
	if !strings.Contains(errText, "配置目录") {
		t.Fatalf("没有可用的共享目录时应报错: %q", errText)
	}
}
