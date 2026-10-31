package app

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/wangwanjie/gately/internal/apitoken"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/portforward"
	"github.com/wangwanjie/gately/internal/modules/protoroute"
	"github.com/wangwanjie/gately/internal/modules/wol"
)

// 升级后首次启动：端口转发中的 SNI 分流规则迁到协议分流模块，端口转发被关闭时协议分流也关闭，
// 拥有端口转发权限的 API 令牌获得协议分流的同级权限；再次启动不重复处理
func TestMigrateSNIRulesOnStart(t *testing.T) {
	dir := t.TempDir()
	a, err := New(Options{ConfigDir: dir, Modules: []module.Module{portforward.New()}})
	if err != nil {
		t.Fatal(err)
	}
	a.logger = log.New(io.Discard, "", 0)
	if _, _, err := a.tokens.Create(apitoken.Input{Name: "pf", Scopes: []string{"portforward:write", "logs:read"}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.tokens.Create(apitoken.Input{Name: "all", Scopes: []string{"*:read"}}); err != nil {
		t.Fatal(err)
	}
	s := a.settingsSnapshot()
	s.DisabledModules = []string{portforward.ID}
	if err := a.store.Save(settingsName, s); err != nil {
		t.Fatal(err)
	}
	legacy := `{"settings": {"enable": true}, "rules": [
	  {"key": "sni1", "name": "443", "enable": true, "protocols": ["tcp"], "listenPorts": "443", "targetAddresses": ["127.0.0.1"],
	   "mode": "sni", "sniRoutes": [{"name": "vpn", "domains": ["vpn.example.com"], "targetAddresses": ["127.0.0.1"], "targetPort": 4443}]}]}`
	if err := os.WriteFile(filepath.Join(dir, "portforward.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		a, err = New(Options{ConfigDir: dir, Modules: []module.Module{portforward.New(), protoroute.New()}})
		if err != nil {
			t.Fatal(err)
		}
		s = a.settingsSnapshot()
		if !slices.Contains(s.DisabledModules, protoroute.ID) || !slices.Contains(s.DisabledModules, portforward.ID) {
			t.Fatalf("第 %d 次启动模块开关: %v", i+1, s.DisabledModules)
		}
		for _, v := range a.tokens.List() {
			switch v.Name {
			case "pf":
				if !slices.Equal(v.Scopes, []string{"logs:read", "portforward:write", "protoroute:write"}) {
					t.Fatalf("第 %d 次启动令牌权限: %v", i+1, v.Scopes)
				}
			case "all":
				if !slices.Equal(v.Scopes, []string{"*:read"}) {
					t.Fatalf("通配权限不应改动: %v", v.Scopes)
				}
			}
		}
	}
	var cfg struct {
		Rules []protoroute.Rule `json:"rules"`
	}
	data, _ := os.ReadFile(filepath.Join(dir, "protoroute.json"))
	if err := json.Unmarshal(data, &cfg); err != nil || len(cfg.Rules) != 1 || cfg.Rules[0].Routes[0].Domains[0] != "vpn.example.com" {
		t.Fatalf("协议分流配置: %s %v", data, err)
	}
}

// 迁移后、补齐令牌权限前启动失败（其他模块配置损坏）：修好后再次启动仍补齐模块开关与令牌权限
func TestMigrateResumesAfterFailedStart(t *testing.T) {
	dir := t.TempDir()
	a, err := New(Options{ConfigDir: dir, Modules: []module.Module{portforward.New()}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.tokens.Create(apitoken.Input{Name: "pf", Scopes: []string{"portforward:read"}}); err != nil {
		t.Fatal(err)
	}
	s := a.settingsSnapshot()
	s.DisabledModules = []string{portforward.ID}
	if err := a.store.Save(settingsName, s); err != nil {
		t.Fatal(err)
	}
	legacy := `{"rules": [{"key": "sni1", "name": "443", "enable": true, "protocols": ["tcp"], "listenPorts": "443",
	  "targetAddresses": ["127.0.0.1"], "mode": "sni", "sniRoutes": [{"domains": ["a.example.com"], "targetAddresses": ["127.0.0.1"], "targetPort": 4443}]}]}`
	if err := os.WriteFile(filepath.Join(dir, "portforward.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(dir, "wol.json")
	if err := os.WriteFile(broken, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	mods := func() []module.Module { return []module.Module{portforward.New(), protoroute.New(), wol.New()} }
	if _, err := New(Options{ConfigDir: dir, Modules: mods()}); err == nil {
		t.Fatal("wol 配置损坏时启动应失败")
	}
	// 规则已经搬走，下次启动 Migrate 不会再报告这次迁移
	if data, _ := os.ReadFile(filepath.Join(dir, "portforward.json")); json.Valid(data) && string(data) == legacy {
		t.Fatal("首次启动应已改写端口转发配置")
	}
	if err := os.Remove(broken); err != nil {
		t.Fatal(err)
	}
	a, err = New(Options{ConfigDir: dir, Modules: mods()})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(a.settingsSnapshot().DisabledModules, protoroute.ID) {
		t.Fatalf("协议分流应随端口转发关闭: %v", a.settingsSnapshot().DisabledModules)
	}
	for _, v := range a.tokens.List() {
		if v.Name == "pf" && !slices.Contains(v.Scopes, "protoroute:read") {
			t.Fatalf("令牌应补上协议分流权限: %v", v.Scopes)
		}
	}
	var mf migrationsFile
	if _, err := a.store.Load(migrationsName, &mf); err != nil || len(mf.Pending) != 0 {
		t.Fatalf("补齐后迁移记录应清空: %+v %v", mf, err)
	}
}

// 端口转发接口不再接受分流规则
func TestPortForwardRejectsSNIMode(t *testing.T) {
	_, srv, c := startModulesApp(t, t.TempDir())
	rule := map[string]any{"name": "x", "enable": false, "protocols": []string{"tcp"}, "listenPorts": "4443",
		"targetAddresses": []string{"127.0.0.1"}, "mode": "sni", "sniRoutes": []any{}}
	code, body := doJSON(t, c, "POST", srv.URL+"/api/portforward/rules", rule)
	if code != 400 {
		t.Fatalf("应拒绝分流规则: %d %s", code, body)
	}
}
