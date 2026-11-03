package tailscale

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestMigrateLegacy 验证旧版单实例配置迁移为名为 tailscale 的实例并立即保存：程序设置移入共享设置，
// 状态目录与控制套接字保持升级前的默认位置，命令行与升级前一致；再次启动不会重复迁移。
func TestMigrateLegacy(t *testing.T) {
	dir := t.TempDir()
	legacy := `{"enable":true,"manageDaemon":true,"tailscaledPath":"/opt/ts/tailscaled","tailscalePath":"/opt/ts/tailscale",
	"stateDir":"","socket":"","userspace":false,"socks5":"","authKey":"tskey-abc","hostname":"nas","loginServer":"https://hs.example.com",
	"advertiseRoutes":["192.168.18.0/24"],"advertiseExitNode":false,"acceptRoutes":false,"acceptDns":false,"extraUpArgs":"",
	"port":41641,"daemonExtraArgs":"","daemonEnv":"TS_DEBUG_FIREWALL_MODE=nftables","installProxy":"http://127.0.0.1:7890"}`
	if err := os.WriteFile(filepath.Join(dir, "tailscale.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	m := newModule(t, dir)
	if len(m.cfg.Instances) != 1 {
		t.Fatalf("应迁移出一个实例: %+v", m.cfg.Instances)
	}
	in := m.cfg.Instances[0]
	if in.Name != "tailscale" || !in.Enable || in.Key == "" || !in.LegacyStateDir || in.Userspace || !in.ManageDaemon ||
		in.AuthKey != "tskey-abc" || in.LoginServer != "https://hs.example.com" || in.Port != 41641 ||
		!slices.Equal(in.AdvertiseRoutes, []string{"192.168.18.0/24"}) || in.DaemonEnv != "TS_DEBUG_FIREWALL_MODE=nftables" {
		t.Errorf("实例字段迁移错误: %+v", in)
	}
	if m.cfg.Settings != (Settings{TailscaledPath: "/opt/ts/tailscaled", TailscalePath: "/opt/ts/tailscale", InstallProxy: "http://127.0.0.1:7890"}) {
		t.Errorf("程序设置迁移错误: %+v", m.cfg.Settings)
	}
	want := []string{"--statedir=" + filepath.Join(dir, "tailscale"), "--socket=" + filepath.Join(dir, "tailscale", "tailscaled.sock"), "--port=41641"}
	if got := in.DaemonArgs(dir); !slices.Equal(got, want) {
		t.Errorf("迁移后 tailscaled 参数应与升级前一致: %v", got)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "tailscale.json"))
	var saved map[string]json.RawMessage
	_ = json.Unmarshal(data, &saved)
	if _, ok := saved["instances"]; !ok || saved["enable"] != nil {
		t.Errorf("迁移结果应立即保存为新格式: %s", data)
	}
	m2 := newModule(t, dir)
	if len(m2.cfg.Instances) != 1 || m2.cfg.Instances[0].Key != in.Key {
		t.Errorf("再次启动不应重复迁移: %+v", m2.cfg.Instances)
	}

	// 没有配置文件时没有实例
	if m3 := newModule(t, t.TempDir()); len(m3.cfg.Instances) != 0 {
		t.Errorf("全新安装不应有实例: %+v", m3.cfg.Instances)
	}
}

// TestDefaultPaths 验证新实例的默认状态目录与控制套接字按实例区分，迁移的实例沿用旧位置，填写的路径优先。
func TestDefaultPaths(t *testing.T) {
	a := Instance{Key: "aaa", Options: DefaultOptions()}
	b := Instance{Key: "bbb", Options: DefaultOptions()}
	old := Instance{Key: "ccc", Options: DefaultOptions(), LegacyStateDir: true}
	if a.stateDir("/etc/gately") != "/etc/gately/tailscale/aaa" || a.socket("/etc/gately") != "/etc/gately/tailscale/aaa/tailscaled.sock" {
		t.Errorf("新实例默认路径错误: %s %s", a.stateDir("/etc/gately"), a.socket("/etc/gately"))
	}
	if a.stateDir("/x") == b.stateDir("/x") || a.socket("/x") == b.socket("/x") {
		t.Error("不同实例的默认路径不应相同")
	}
	if old.stateDir("/etc/gately") != "/etc/gately/tailscale" {
		t.Errorf("迁移实例应沿用旧默认目录: %s", old.stateDir("/etc/gately"))
	}
	custom := Instance{Key: "ddd", Options: Options{ManageDaemon: true, StateDir: "/vol1/1000/MyDocker/tailscale/state-local"}}
	if custom.socket("/x") != "/vol1/1000/MyDocker/tailscale/state-local/tailscaled.sock" {
		t.Errorf("自定义状态目录下的套接字错误: %s", custom.socket("/x"))
	}
}

// TestConflictRules 通过接口验证实例之间的冲突规则：两个启用的内核模式实例、共用控制套接字、状态目录、
// 固定端口与重名都被拒绝；修改实例自身不与自己冲突；启用（单个与批量）时同样检查内核模式实例数量。
func TestConflictRules(t *testing.T) {
	m := newModule(t, t.TempDir())
	srv := serve(t, m)
	base := srv.URL + "/api/tailscale/instances"
	stateA, stateB := filepath.Join(t.TempDir(), "state"), filepath.Join(t.TempDir(), "state-local")
	kernel := func(name, state string, port int, enable bool) Instance {
		return Instance{Name: name, Enable: enable, Options: Options{ManageDaemon: true, StateDir: state, Port: port,
			LoginServer: "https://headscale.vanjay.cn", AdvertiseRoutes: []string{"192.168.18.0/24"}, DaemonEnv: "TS_DEBUG_FIREWALL_MODE=nftables"}}
	}
	var a InstanceView
	if code := doJSON(t, "POST", base, kernel("hk", stateA, 41641, true), &a); code != 200 || a.Key == "" {
		t.Fatalf("第一个内核实例应创建成功: %d", code)
	}
	user := Instance{Name: "local", Enable: true, Options: Options{ManageDaemon: true, Userspace: true, StateDir: stateB, Port: 41642,
		LoginServer: "https://headscale.ddns.vanjay.cn:4433", AdvertiseRoutes: []string{"192.168.18.0/24"}}}
	var b InstanceView
	if code := doJSON(t, "POST", base, user, &b); code != 200 {
		t.Fatalf("用户态实例应创建成功: %d", code)
	}

	errOf := func(method, url string, body any) (int, string) {
		var out struct {
			Error string `json:"error"`
		}
		code := doJSON(t, method, url, body, &out)
		return code, out.Error
	}
	cases := []struct {
		name string
		body Instance
		want string
	}{
		{"两个启用的内核实例", kernel("hk2", filepath.Join(t.TempDir(), "s"), 0, true), "内核模式"},
		{"共用状态目录", Instance{Name: "x", Options: Options{ManageDaemon: true, Userspace: true, StateDir: stateA}}, "状态目录"},
		{"共用端口", Instance{Name: "x", Options: Options{ManageDaemon: true, Userspace: true, Port: 41642}}, "WireGuard"},
		{"共用控制套接字", Instance{Name: "x", Options: Options{ManageDaemon: true, Userspace: true, Socket: filepath.Join(stateB, "tailscaled.sock")}}, "控制套接字"},
		{"重名", Instance{Name: "local", Options: DefaultOptions()}, "已存在"},
	}
	for _, c := range cases {
		if code, msg := errOf("POST", base, c.body); code != 400 || !strings.Contains(msg, c.want) {
			t.Errorf("%s 应被拒绝: %d %s", c.name, code, msg)
		}
	}

	// 修改实例自身（请求体不带 key）不与自己冲突
	upd := kernel("hk", stateA, 41641, true)
	upd.Hostname = "nas"
	if code, msg := errOf("PUT", base+"/"+a.Key, upd); code != 200 {
		t.Fatalf("修改自身应成功: %d %s", code, msg)
	}
	// 把用户态实例改成内核模式被拒绝
	bad := user
	bad.Userspace = false
	if code, msg := errOf("PUT", base+"/"+b.Key, bad); code != 400 || !strings.Contains(msg, "内核模式") {
		t.Errorf("改为内核模式应被拒绝: %d %s", code, msg)
	}

	// 停用的内核实例可以保存，启用时被拒绝
	var c InstanceView
	if code := doJSON(t, "POST", base, kernel("spare", filepath.Join(t.TempDir(), "s"), 0, false), &c); code != 200 {
		t.Fatalf("停用的内核实例应能保存: %d", code)
	}
	if code, msg := errOf("PUT", base+"/"+c.Key+"/enable", map[string]bool{"enable": true}); code != 400 || !strings.Contains(msg, "内核模式") {
		t.Errorf("启用第二个内核实例应被拒绝: %d %s", code, msg)
	}
	if code, _ := errOf("PUT", base+"-enable", map[string]any{"keys": []string{c.Key}, "enable": true}); code != 400 {
		t.Errorf("批量启用第二个内核实例应被拒绝: %d", code)
	}
	if instOf(m, c.Key).Enable {
		t.Error("被拒绝的启用不应生效")
	}
	// 停用第一个后可以启用
	if code, _ := errOf("PUT", base+"/"+a.Key+"/enable", map[string]bool{"enable": false}); code != 200 {
		t.Fatal(code)
	}
	if code, msg := errOf("PUT", base+"/"+c.Key+"/enable", map[string]bool{"enable": true}); code != 200 {
		t.Errorf("腾出后应能启用: %d %s", code, msg)
	}

	// 两个外部守护进程都使用系统默认套接字
	ext := Instance{Name: "ext1", Options: Options{}}
	if code, _ := errOf("POST", base, ext); code != 200 {
		t.Fatal(code)
	}
	ext.Name = "ext2"
	if code, msg := errOf("POST", base, ext); code != 400 || !strings.Contains(msg, "系统默认") {
		t.Errorf("两个外部实例共用系统默认套接字应被拒绝: %d %s", code, msg)
	}

	// 复制时副本不沿用迁移实例的旧默认目录
	m.mu.Lock()
	m.cfg.Instances = append(m.cfg.Instances, Instance{Key: "legacy", Name: "tailscale", Options: Options{ManageDaemon: true, Userspace: true, AuthKey: "tskey-x"}, LegacyStateDir: true})
	m.mu.Unlock()
	cp := Instance{Name: "tailscale-2", Options: Options{ManageDaemon: true, Userspace: true, AuthKey: "********"}, LegacyStateDir: true}
	var cv InstanceView
	if code := doJSON(t, "POST", base+"/legacy/copy", cp, &cv); code != 200 || cv.LegacyStateDir {
		t.Errorf("副本应使用自己的默认目录: %d %+v", code, cv)
	}
	if got := instOf(m, cv.Key); got == nil || got.AuthKey != "tskey-x" {
		t.Errorf("副本应沿用模板的认证密钥: %+v", got)
	}
}

// TestRestartOnlyAffected 验证按实例启停：修改一个实例只重启它自己，只改分组或备注不重启，删除只停止被删的实例，
// 模块停止时全部停止。tailscaled 用常驻的脚本代替，tailscale 命令总是失败（up 一直等待守护进程就绪），
// 通过运行时是否重建判断是否重启。
func TestRestartOnlyAffected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("使用 shell 脚本模拟程序")
	}
	m := newModule(t, t.TempDir())
	bin := t.TempDir()
	m.cfg.Settings.TailscaledPath = writeScript(t, filepath.Join(bin, "tailscaled"), "exec sleep 60")
	m.cfg.Settings.TailscalePath = writeScript(t, filepath.Join(bin, "tailscale"), "exit 1")
	m.cfg.Instances = []Instance{
		{Key: "a", Name: "hk", Enable: true, Options: Options{ManageDaemon: true, Port: 41641}},
		{Key: "b", Name: "local", Enable: true, Options: Options{ManageDaemon: true, Userspace: true, Port: 41642}},
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	srv := serve(t, m)
	base := srv.URL + "/api/tailscale/instances"
	ra, rb := runOf(m, "a"), runOf(m, "b")
	if ra == nil || rb == nil || ra.sig == "" || rb.sig == "" || ra.getErr() != "" {
		t.Fatalf("两个实例都应启动: %+v %+v", ra, rb)
	}
	waitFor(t, "两个 tailscaled 运行", func() bool { return ra.daemon.State().Running && rb.daemon.State().Running })

	body := m.cfg.Instances[0]
	body.Hostname = "nas"
	if code := doJSON(t, "PUT", base+"/a", body, nil); code != 200 {
		t.Fatal(code)
	}
	if runOf(m, "a") == ra || runOf(m, "b") != rb {
		t.Error("修改 a 应只重启 a")
	}
	ra = runOf(m, "a")

	body = m.cfg.Instances[1]
	body.Remark, body.Group = "备注", "家里"
	if code := doJSON(t, "PUT", base+"/b", body, nil); code != 200 {
		t.Fatal(code)
	}
	if runOf(m, "b") != rb || runOf(m, "a") != ra {
		t.Error("只改分组与备注不应重启")
	}

	if code := doJSON(t, "POST", base+"/b/restart", nil, nil); code != 200 {
		t.Fatal(code)
	}
	if runOf(m, "b") == rb || runOf(m, "a") != ra {
		t.Error("重启接口应只重启 b")
	}
	if code := doJSON(t, "POST", base+"/zzz/restart", nil, nil); code != 404 {
		t.Errorf("不存在的实例应返回 404: %d", code)
	}
	rb = runOf(m, "b")

	if code := doJSON(t, "DELETE", base+"/a", nil, nil); code != 200 {
		t.Fatal(code)
	}
	if _, ok := runsOf(m)["a"]; ok || runOf(m, "b") != rb {
		t.Error("删除 a 应只停止 a")
	}

	// 按实例的状态接口
	var st StatusView
	if code := doJSON(t, "GET", base+"/b/status", nil, &st); code != 200 || st.Key != "b" || !st.Daemon.Running || st.Error == "" {
		t.Errorf("实例状态错误: %d %+v", code, st)
	}
	var all []StatusView
	if code := doJSON(t, "GET", srv.URL+"/api/tailscale/status", nil, &all); code != 200 || len(all) != 1 || all[0].Name != "local" {
		t.Errorf("全部实例状态错误: %d %+v", code, all)
	}

	_ = m.Stop()
	if len(m.runs) != 0 {
		t.Errorf("模块停止后应停止全部实例: %d", len(m.runs))
	}
}

// TestRunningConflict 验证手动改出两个启用的内核实例时，排在前面的照常启动，后面的不启动并显示原因；
// 前一个停用后后一个自动启动。
func TestRunningConflict(t *testing.T) {
	m := newModule(t, t.TempDir())
	m.cfg.Settings.TailscalePath = filepath.Join(t.TempDir(), "missing", "tailscale")
	m.cfg.Instances = []Instance{
		{Key: "a", Name: "first", Enable: true, Options: Options{ManageDaemon: true}},
		{Key: "b", Name: "second", Enable: true, Options: Options{ManageDaemon: true}},
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	if runOf(m, "a").conflict != "" || !strings.Contains(runOf(m, "b").conflict, "first") {
		t.Fatalf("后面的内核实例应因冲突不启动: %q %q", runOf(m, "a").conflict, runOf(m, "b").conflict)
	}
	m.mu.Lock()
	m.cfg.Instances[0].Enable = false
	m.applyLocked("a")
	m.mu.Unlock()
	if runOf(m, "b").conflict != "" {
		t.Errorf("冲突解除后应重新启动: %q", runOf(m, "b").conflict)
	}
}

// runOf、runsOf、instOf 在持锁状态下读取运行时与配置，测试中的接口请求在其他协程中修改它们
func runOf(m *Module, key string) *instRuntime {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runs[key]
}

func runsOf(m *Module) map[string]*instRuntime {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]*instRuntime{}
	for k, v := range m.runs {
		out[k] = v
	}
	return out
}

func instOf(m *Module, key string) *Instance {
	m.mu.Lock()
	defer m.mu.Unlock()
	if in := m.findInstance(key); in != nil {
		v := *in
		return &v
	}
	return nil
}

// writeScript 写一个可执行的 shell 脚本并返回路径。
func writeScript(t *testing.T, path, body string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("等待超时: %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
