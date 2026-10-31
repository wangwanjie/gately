package protoroute

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// legacyPortForward 是旧版端口转发配置：一条普通转发规则、一条 SNI 分流规则，全局设置关闭了模块。
const legacyPortForward = `{
  "settings": {"enable": false, "defaultMaxConnections": 100, "defaultUDPSessionTimeout": 60, "maxUDPSessions": 1024, "dialTimeout": 3},
  "rules": [
    {"key": "plain", "name": "ssh", "enable": true, "protocols": ["tcp", "udp"], "listenPorts": "2222",
     "targetAddresses": ["192.168.1.2"], "targetPorts": "22", "maxConnections": 0, "udpSessionTimeout": 0,
     "proxyProtocol": "", "mode": "", "sniRoutes": null, "remark": "保留"},
    {"key": "sni1", "name": "443", "enable": true, "group": "入口", "protocols": ["tcp4"], "listenAddress": "192.0.2.10",
     "listenPorts": "443", "targetAddresses": ["127.0.0.1"], "targetPorts": "8443", "maxConnections": 512,
     "proxyProtocol": "v2", "mode": "sni", "remark": "替代 haproxy",
     "sniRoutes": [{"name": "ocserv", "domains": ["tz.example.com"], "targetAddresses": ["127.0.0.1"], "targetPort": 4443, "proxyProtocol": "v2"}]}
  ]
}
`

func writeFile(t *testing.T, dir, name, data string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateFromPortForward(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "portforward.json", legacyPortForward)
	store, err := config.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := New()
	from, err := m.Migrate(store)
	if err != nil || from != "portforward" || m.migrated != 1 {
		t.Fatalf("迁移结果 %q %d %v", from, m.migrated, err)
	}

	// 分流规则完整搬到本模块，路由协议补为 tls，全局设置沿用端口转发
	var cfg fileConfig
	if _, err := store.Load(ID, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Rules) != 1 {
		t.Fatalf("规则数 %d", len(cfg.Rules))
	}
	r := cfg.Rules[0]
	if r.Key != "sni1" || r.Name != "443" || !r.Enable || r.Group != "入口" || r.ListenAddress != "192.0.2.10" || r.ListenPorts != "443" ||
		r.TargetPorts != "8443" || r.MaxConnections != 512 || r.ProxyProtocol != "v2" || r.Remark != "替代 haproxy" || len(r.Routes) != 1 {
		t.Fatalf("迁移后的规则: %+v", r)
	}
	rt := r.Routes[0]
	if rt.Protocol != ProtoTLS || rt.Name != "ocserv" || rt.Domains[0] != "tz.example.com" || rt.TargetPort != 4443 || rt.ProxyProtocol != "v2" {
		t.Fatalf("迁移后的路由: %+v", rt)
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("迁移后的规则应能通过校验: %v", err)
	}
	if cfg.Settings.Enable || cfg.Settings.DefaultMaxConnections != 100 || cfg.Settings.DialTimeout != 3 {
		t.Fatalf("全局设置: %+v", cfg.Settings)
	}

	// 端口转发只剩普通规则，规则内容与全局设置原样保留
	var pf struct {
		Settings map[string]any   `json:"settings"`
		Rules    []map[string]any `json:"rules"`
	}
	if _, err := store.Load("portforward", &pf); err != nil {
		t.Fatal(err)
	}
	if len(pf.Rules) != 1 || pf.Rules[0]["key"] != "plain" || pf.Rules[0]["remark"] != "保留" || pf.Settings["defaultUDPSessionTimeout"] != float64(60) {
		t.Fatalf("端口转发配置: %+v", pf)
	}

	// 再次启动没有可迁移的规则，不改动任何文件
	before, _ := os.ReadFile(filepath.Join(dir, "protoroute.json"))
	m2 := New()
	if from, err := m2.Migrate(store); err != nil || from != "" || m2.migrated != 0 {
		t.Fatalf("重复迁移: %q %d %v", from, m2.migrated, err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "protoroute.json"))
	if string(before) != string(after) {
		t.Fatal("没有可迁移的规则时不应改写配置")
	}

	// 迁移后模块能正常加载
	if err := m.Init(testEnv(store)); err != nil {
		t.Fatal(err)
	}
	if len(m.cfg.Rules) != 1 {
		t.Fatalf("加载规则数 %d", len(m.cfg.Rules))
	}
}

// 上次迁移写完本模块配置后中断：端口转发仍保留分流规则，再次迁移按 key 覆盖，不产生重复规则，本模块已有设置保持不变
func TestMigrateResumesAfterInterrupt(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "portforward.json", legacyPortForward)
	writeFile(t, dir, "protoroute.json", `{"settings": {"enable": true, "defaultMaxConnections": 64, "dialTimeout": 9},
	  "rules": [{"key": "sni1", "name": "旧", "enable": false, "protocols": ["tcp"], "listenPorts": "443", "routes": []},
	            {"key": "other", "name": "其他", "enable": true, "protocols": ["tcp"], "listenPorts": "8443", "targetAddresses": ["x"]}]}`)
	store, err := config.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := migrateFromPortForward(store); err != nil || n != 1 {
		t.Fatalf("迁移 %d %v", n, err)
	}
	var cfg fileConfig
	if _, err := store.Load(ID, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Rules) != 2 || cfg.Rules[0].Key != "sni1" || cfg.Rules[0].Name != "443" || cfg.Rules[1].Key != "other" {
		t.Fatalf("规则: %+v", cfg.Rules)
	}
	if !cfg.Settings.Enable || cfg.Settings.DefaultMaxConnections != 64 || cfg.Settings.DialTimeout != 9 {
		t.Fatalf("已有设置被覆盖: %+v", cfg.Settings)
	}
}

func TestMigrateNothing(t *testing.T) {
	dir := t.TempDir()
	store, err := config.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// 没有端口转发配置
	if n, err := migrateFromPortForward(store); err != nil || n != 0 {
		t.Fatalf("无配置: %d %v", n, err)
	}
	// 只有普通转发规则：不创建本模块配置，也不改写端口转发配置
	plain := `{"settings": {"enable": true}, "rules": [{"key": "a", "protocols": ["tcp"], "listenPorts": "80", "targetAddresses": ["x"]}]}`
	writeFile(t, dir, "portforward.json", plain)
	if n, err := migrateFromPortForward(store); err != nil || n != 0 {
		t.Fatalf("普通规则: %d %v", n, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "protoroute.json")); !os.IsNotExist(err) {
		t.Fatal("不应创建协议分流配置")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "portforward.json"))
	if string(data) != plain {
		t.Fatal("不应改写端口转发配置")
	}
	// 配置损坏时报错，交给启动流程处理
	writeFile(t, dir, "portforward.json", `{"rules": [1]}`)
	if _, err := migrateFromPortForward(store); err == nil {
		t.Fatal("损坏的配置应报错")
	}
}

func testEnv(store *config.Store) module.Env {
	return module.Env{Store: store, Logs: logbuf.New(100, log.New(io.Discard, "", 0))}
}
