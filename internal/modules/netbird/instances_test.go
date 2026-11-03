package netbird

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
)

// moduleIn 在 dir 中初始化模块（不启动）。
func moduleIn(t *testing.T, dir string) *Module {
	t.Helper()
	store, err := config.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := New()
	if err := m.Init(testEnv(store)); err != nil {
		t.Fatal(err)
	}
	return m
}

// serve 注册模块接口并返回测试服务，接口路径相对模块前缀，例如 /instances。
func serve(t *testing.T, m *Module) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	m.Routes(testRouter{mux})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func doJSON(t *testing.T, method, url string, body, out any) int {
	t.Helper()
	var r io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		r = strings.NewReader(string(data))
	}
	req, _ := http.NewRequest(method, url, r)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

// errResp 发送请求并返回状态码与错误信息。
func errResp(t *testing.T, method, url string, body any) (int, string) {
	t.Helper()
	var out struct {
		Error string `json:"error"`
	}
	code := doJSON(t, method, url, body, &out)
	return code, out.Error
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

// TestMigrateLegacy 验证旧版单实例配置迁移为名为 netbird 的实例并立即保存：程序设置移入共享设置，
// 状态目录与控制地址保持升级前的默认位置，守护进程参数与环境变量与升级前一致；再次启动不会重复迁移。
func TestMigrateLegacy(t *testing.T) {
	dir := t.TempDir()
	legacy := `{"enable":true,"manageDaemon":true,"netbirdPath":"/opt/nb/netbird","stateDir":"","daemonAddr":"",
	"userspace":true,"localForwarding":true,"socks5":"127.0.0.1:1081","setupKey":"NB-KEY","managementUrl":"https://nb.example.com",
	"hostname":"nas","port":51830,"acceptRoutes":true,"serverRoutes":false,"acceptDns":false,"blockInbound":true,
	"extraUpArgs":"--allow-server-ssh","daemonExtraArgs":"--log-level=debug","daemonEnv":"NB_LOG_LEVEL=debug","installProxy":"http://127.0.0.1:7890"}`
	if err := os.WriteFile(filepath.Join(dir, "netbird.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	m := moduleIn(t, dir)
	if len(m.cfg.Instances) != 1 {
		t.Fatalf("应迁移出一个实例: %+v", m.cfg.Instances)
	}
	in := m.cfg.Instances[0]
	if in.Name != "netbird" || !in.Enable || in.Key == "" || !in.LegacyStateDir || !in.Userspace || !in.ManageDaemon ||
		in.SetupKey != "NB-KEY" || in.ManagementURL != "https://nb.example.com" || in.Port != 51830 || in.ServerRoutes ||
		!in.AcceptRoutes || !in.BlockInbound || in.Socks5 != "127.0.0.1:1081" || in.DaemonEnv != "NB_LOG_LEVEL=debug" {
		t.Errorf("实例字段迁移错误: %+v", in)
	}
	if m.cfg.Settings != (Settings{NetbirdPath: "/opt/nb/netbird", InstallProxy: "http://127.0.0.1:7890"}) {
		t.Errorf("程序设置迁移错误: %+v", m.cfg.Settings)
	}
	sock := "unix://" + filepath.Join(dir, "netbird", "netbird.sock")
	if got := in.DaemonArgs(dir); !slices.Equal(got, []string{"service", "run", "--daemon-addr=" + sock, "--log-file=console", "--log-level=debug"}) {
		t.Errorf("迁移后守护进程参数应与升级前一致: %v", got)
	}
	wantEnv := []string{"NB_STATE_DIR=" + filepath.Join(dir, "netbird"), "NB_USE_NETSTACK_MODE=true", "NB_ENABLE_NETSTACK_LOCAL_FORWARDING=true",
		"NB_SOCKS5_LISTENER_ADDRESS=127.0.0.1", "NB_SOCKS5_LISTENER_PORT=1081", "NB_LOG_LEVEL=debug"}
	if got := in.DaemonEnvList(dir); !slices.Equal(got, wantEnv) {
		t.Errorf("迁移后守护进程环境变量应与升级前一致: %v", got)
	}
	if got := in.CLIArgs(dir); !slices.Equal(got, []string{"--daemon-addr=" + sock}) {
		t.Errorf("迁移后 CLI 参数应与升级前一致: %v", got)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "netbird.json"))
	var saved map[string]json.RawMessage
	_ = json.Unmarshal(data, &saved)
	if _, ok := saved["instances"]; !ok || saved["enable"] != nil {
		t.Errorf("迁移结果应立即保存为新格式: %s", data)
	}
	m2 := moduleIn(t, dir)
	if len(m2.cfg.Instances) != 1 || m2.cfg.Instances[0].Key != in.Key {
		t.Errorf("再次启动不应重复迁移: %+v", m2.cfg.Instances)
	}

	// 旧版缺少的字段取默认值（只写了 enable 的手写配置）
	dir2 := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir2, "netbird.json"), []byte(`{"enable":false}`), 0o600)
	if m3 := moduleIn(t, dir2); len(m3.cfg.Instances) != 1 || m3.cfg.Instances[0].Options != DefaultOptions() {
		t.Errorf("缺少的字段应取默认值: %+v", m3.cfg.Instances)
	}
	// 没有配置文件时没有实例
	if m4 := moduleIn(t, t.TempDir()); len(m4.cfg.Instances) != 0 {
		t.Errorf("全新安装不应有实例: %+v", m4.cfg.Instances)
	}
}

// TestDefaultPaths 验证新实例的默认状态目录与控制地址按实例区分，迁移的实例沿用旧位置，填写的路径优先；
// key 含路径分隔符或点时不拼出默认目录（防止目录穿越）。
func TestDefaultPaths(t *testing.T) {
	a := Instance{Key: "aaa", Options: DefaultOptions()}
	b := Instance{Key: "bbb", Options: DefaultOptions()}
	old := Instance{Key: "ccc", Options: DefaultOptions(), LegacyStateDir: true}
	if a.stateDir("/etc/gately") != "/etc/gately/netbird/aaa" || a.daemonAddr("/etc/gately") != "unix:///etc/gately/netbird/aaa/netbird.sock" {
		t.Errorf("新实例默认路径错误: %s %s", a.stateDir("/etc/gately"), a.daemonAddr("/etc/gately"))
	}
	if a.stateDir("/x") == b.stateDir("/x") || a.daemonAddr("/x") == b.daemonAddr("/x") {
		t.Error("不同实例的默认路径不应相同")
	}
	if old.stateDir("/etc/gately") != "/etc/gately/netbird" {
		t.Errorf("迁移实例应沿用旧默认目录: %s", old.stateDir("/etc/gately"))
	}
	custom := Instance{Key: "ddd", Options: Options{ManageDaemon: true, StateDir: "/vol1/nb/state"}}
	if custom.daemonAddr("/x") != "unix:///vol1/nb/state/netbird.sock" {
		t.Errorf("自定义状态目录下的控制地址错误: %s", custom.daemonAddr("/x"))
	}
	for _, key := range []string{"", "..", "../evil", `a\b`, "a/b", "."} {
		bad := Instance{Key: key, Options: DefaultOptions()}
		if d := bad.defaultStateDir("/etc/gately"); d != "" {
			t.Errorf("key %q 不应拼出默认目录: %s", key, d)
		}
	}
}

// TestConflictRules 通过接口验证实例之间的冲突规则：两个启用的内核模式实例、共用状态目录与控制地址、
// 相同的有效 WireGuard 端口（0 视为 51820）、相同的 SOCKS5 监听（留空视为 127.0.0.1:1080）与重名都被拒绝；
// 修改实例自身不与自己冲突；启用（单个与批量）与复制时同样检查。
func TestConflictRules(t *testing.T) {
	m := moduleIn(t, t.TempDir())
	srv := serve(t, m)
	base := srv.URL + "/instances"
	stateA := filepath.Join(t.TempDir(), "state")
	kernel := func(name, state string, port int, enable bool) Instance {
		o := DefaultOptions()
		o.Userspace, o.StateDir, o.Port, o.ManagementURL = false, state, port, "https://nb.vanjay.cn"
		return Instance{Name: name, Enable: enable, Options: o}
	}
	user := func(name string, port int, socks string) Instance {
		o := DefaultOptions()
		o.Port, o.Socks5 = port, socks
		return Instance{Name: name, Enable: true, Options: o}
	}
	var a InstanceView
	if code := doJSON(t, "POST", base, kernel("hk", stateA, 0, true), &a); code != 200 || a.Key == "" {
		t.Fatalf("第一个内核实例应创建成功: %d", code)
	}
	var b InstanceView
	if code := doJSON(t, "POST", base, user("local", 51821, ""), &b); code != 200 {
		t.Fatalf("用户态实例应创建成功: %d", code)
	}
	bSock := "unix://" + filepath.Join(m.store.Dir(), "netbird", b.Key, "netbird.sock")

	sharedState := user("x", 51823, "127.0.0.1:1090")
	sharedState.StateDir = stateA + "/"
	sharedAddr := user("x", 51825, "127.0.0.1:1091")
	sharedAddr.DaemonAddr = bSock
	cases := []struct {
		name string
		body Instance
		want string
	}{
		{"两个启用的内核实例", kernel("hk2", filepath.Join(t.TempDir(), "s"), 51822, true), "内核模式"},
		{"共用状态目录（末尾斜杠规范化后相同）", sharedState, "状态目录"},
		{"共用控制地址", sharedAddr, "控制地址"},
		{"端口 51820 与默认端口 0 相同", user("x", 51820, "127.0.0.1:1092"), "WireGuard"},
		{"两个默认端口 0", user("x", 0, "127.0.0.1:1093"), "WireGuard"},
		{"端口相同", user("x", 51821, "127.0.0.1:1094"), "WireGuard"},
		{"SOCKS5 127.0.0.1:1080 与留空相同", user("x", 51826, "127.0.0.1:1080"), "SOCKS5"},
		{"SOCKS5 0.0.0.0:1080 与 127.0.0.1:1080 抢同一端口", user("x", 51827, "0.0.0.0:1080"), "SOCKS5"},
		{"两个 SOCKS5 都留空", user("x", 51828, ""), "SOCKS5"},
		{"重名", user("local", 51829, "127.0.0.1:1095"), "已存在"},
	}
	for _, c := range cases {
		if code, msg := errResp(t, "POST", base, c.body); code != 400 || !strings.Contains(msg, c.want) {
			t.Errorf("%s 应被拒绝: %d %s", c.name, code, msg)
		}
	}
	// 端口、SOCKS5 都不同的用户态实例可以与内核实例并存；不同地址的 SOCKS5 监听同一端口也可以
	var okv InstanceView
	if code := doJSON(t, "POST", base, user("ok", 51830, "127.0.0.2:1080"), &okv); code != 200 {
		t.Fatalf("不冲突的用户态实例应创建成功: %d", code)
	}

	// 修改实例自身（请求体不带 key）不与自己冲突
	upd := kernel("hk", stateA, 0, true)
	upd.Hostname = "nas"
	if code, msg := errResp(t, "PUT", base+"/"+a.Key, upd); code != 200 {
		t.Fatalf("修改自身应成功: %d %s", code, msg)
	}
	// 把用户态实例改成内核模式被拒绝
	bad := user("local", 51821, "")
	bad.Userspace = false
	if code, msg := errResp(t, "PUT", base+"/"+b.Key, bad); code != 400 || !strings.Contains(msg, "内核模式") {
		t.Errorf("改为内核模式应被拒绝: %d %s", code, msg)
	}
	// 修改时改成别的实例的端口被拒绝
	if code, msg := errResp(t, "PUT", base+"/"+b.Key, user("local", 0, "")); code != 400 || !strings.Contains(msg, "WireGuard") {
		t.Errorf("修改为已用端口应被拒绝: %d %s", code, msg)
	}

	// 停用的内核实例可以保存，启用时被拒绝
	var c InstanceView
	if code := doJSON(t, "POST", base, kernel("spare", filepath.Join(t.TempDir(), "s"), 51840, false), &c); code != 200 {
		t.Fatalf("停用的内核实例应能保存: %d", code)
	}
	if code, msg := errResp(t, "PUT", base+"/"+c.Key+"/enable", map[string]bool{"enable": true}); code != 400 || !strings.Contains(msg, "内核模式") {
		t.Errorf("启用第二个内核实例应被拒绝: %d %s", code, msg)
	}
	if code, _ := errResp(t, "PUT", base+"-enable", map[string]any{"keys": []string{c.Key}, "enable": true}); code != 400 {
		t.Errorf("批量启用第二个内核实例应被拒绝: %d", code)
	}
	if instOf(m, c.Key).Enable {
		t.Error("被拒绝的启用不应生效")
	}
	// 停用第一个后可以启用；批量同时启用两个内核实例被拒绝且都不生效
	if code, _ := errResp(t, "PUT", base+"/"+a.Key+"/enable", map[string]bool{"enable": false}); code != 200 {
		t.Fatal(code)
	}
	if code, _ := errResp(t, "PUT", base+"-enable", map[string]any{"keys": []string{a.Key, c.Key}, "enable": true}); code != 400 {
		t.Errorf("批量同时启用两个内核实例应被拒绝: %d", code)
	}
	if instOf(m, a.Key).Enable || instOf(m, c.Key).Enable {
		t.Error("被拒绝的批量启用应全部撤销")
	}
	if code, msg := errResp(t, "PUT", base+"/"+c.Key+"/enable", map[string]bool{"enable": true}); code != 200 {
		t.Errorf("腾出后应能启用: %d %s", code, msg)
	}

	// 两个外部守护进程都使用系统默认控制地址；外部实例的端口与 SOCKS5 不参与比较
	ext := Instance{Name: "ext1"}
	if code, msg := errResp(t, "POST", base, ext); code != 200 {
		t.Fatal(code, msg)
	}
	ext.Name = "ext2"
	if code, msg := errResp(t, "POST", base, ext); code != 400 || !strings.Contains(msg, "系统默认") {
		t.Errorf("两个外部实例共用系统默认控制地址应被拒绝: %d %s", code, msg)
	}
	ext.DaemonAddr = systemDaemonAddr
	if code, msg := errResp(t, "POST", base, ext); code != 400 || !strings.Contains(msg, "系统默认") {
		t.Errorf("填写的地址与系统默认相同也应被拒绝: %d %s", code, msg)
	}

	// 新增时请求体中的 legacyStateDir 不采信
	forged := user("forged", 51850, "127.0.0.1:1100")
	forged.LegacyStateDir = true
	var fv InstanceView
	if code := doJSON(t, "POST", base, forged, &fv); code != 200 || fv.LegacyStateDir || instOf(m, fv.Key).LegacyStateDir {
		t.Errorf("新增时不应采信 legacyStateDir: %d %+v", code, fv)
	}

	// 复制：副本不沿用迁移实例的旧默认目录，沿用设置密钥；与模板端口相同时被拒绝
	m.mu.Lock()
	lo := DefaultOptions()
	lo.Port, lo.Socks5, lo.SetupKey = 51860, "127.0.0.1:1110", "NB-SECRET"
	m.cfg.Instances = append(m.cfg.Instances, Instance{Key: "legacy", Name: "netbird", Enable: true, Options: lo, LegacyStateDir: true})
	m.mu.Unlock()
	cp := Instance{Name: "netbird-2", Enable: true, Options: lo, LegacyStateDir: true}
	cp.SetupKey = "********"
	if code, msg := errResp(t, "POST", base+"/legacy/copy", cp); code != 400 || !strings.Contains(msg, "WireGuard") {
		t.Errorf("副本与模板端口相同应被拒绝: %d %s", code, msg)
	}
	cp.Port = 51861
	if code, msg := errResp(t, "POST", base+"/legacy/copy", cp); code != 400 || !strings.Contains(msg, "SOCKS5") {
		t.Errorf("副本与模板 SOCKS5 相同应被拒绝: %d %s", code, msg)
	}
	cp.Socks5 = "127.0.0.1:1111"
	var cv InstanceView
	if code := doJSON(t, "POST", base+"/legacy/copy", cp, &cv); code != 200 || cv.LegacyStateDir {
		t.Errorf("副本应使用自己的默认目录: %d %+v", code, cv)
	}
	if got := instOf(m, cv.Key); got == nil || got.SetupKey != "NB-SECRET" || got.LegacyStateDir {
		t.Errorf("副本应沿用模板的设置密钥: %+v", got)
	}
	// 修改迁移实例时保留 legacyStateDir（请求体通常不带）
	lb := Instance{Name: "netbird", Enable: true, Options: lo}
	lb.SetupKey = "********"
	if code, msg := errResp(t, "PUT", base+"/legacy", lb); code != 200 || !instOf(m, "legacy").LegacyStateDir || instOf(m, "legacy").SetupKey != "NB-SECRET" {
		t.Errorf("修改迁移实例应保留旧目录标记与密钥: %d %s %+v", code, msg, instOf(m, "legacy"))
	}
}

// fakeDaemonScript 写一个模拟 netbird 的脚本：service run 常驻并把自己的 PID 写到状态目录（NB_STATE_DIR），
// status 返回空闲，up 立即成功。用于观察每个实例的守护进程是否被重启。
func fakeDaemonScript(t *testing.T, dir string) {
	t.Helper()
	writeScript(t, dir, `case " $* " in
*" service run "*) echo $$ > "$NB_STATE_DIR/pid"; exec sleep 60 ;;
*" status --json "*) echo '{"daemonStatus":"Idle","peers":{"details":[]}}' ;;
*" up "*) exit 0 ;;
*" version "*) echo 0.82.0 ;;
esac
`)
}

// pidOf 返回实例托管的守护进程的 PID，未运行时为 0。
func pidOf(m *Module, key string) int {
	if rt := runOf(m, key); rt != nil {
		return rt.daemon.State().PID
	}
	return 0
}

// TestRestartOnlyAffected 验证按实例启停：修改一个实例只重启它自己的守护进程（另一个的 PID 不变），
// 只改分组或备注不重启，重启接口只重启指定实例，删除只停止被删的实例，模块停止时全部停止。
func TestRestartOnlyAffected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("使用 shell 脚本模拟程序")
	}
	dir := t.TempDir()
	m := moduleIn(t, dir)
	fakeDaemonScript(t, dir)
	ua, ub := DefaultOptions(), DefaultOptions()
	ua.Port, ub.Port, ub.Socks5 = 51830, 51831, "127.0.0.1:1081"
	m.cfg.Instances = []Instance{
		{Key: "a", Name: "hk", Enable: true, Options: ua},
		{Key: "b", Name: "local", Enable: true, Options: ub},
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	srv := serve(t, m)
	base := srv.URL + "/instances"
	waitFor(t, "两个守护进程运行", func() bool { return pidOf(m, "a") != 0 && pidOf(m, "b") != 0 })
	pa, pb := pidOf(m, "a"), pidOf(m, "b")
	// 每个实例的守护进程使用自己的状态目录
	for _, k := range []string{"a", "b"} {
		waitFor(t, "状态目录中的 PID 文件 "+k, func() bool {
			_, err := os.Stat(filepath.Join(dir, "netbird", k, "pid"))
			return err == nil
		})
	}

	body := *instOf(m, "a")
	body.Hostname = "nas"
	if code := doJSON(t, "PUT", base+"/a", body, nil); code != 200 {
		t.Fatal(code)
	}
	waitFor(t, "a 重启", func() bool { p := pidOf(m, "a"); return p != 0 && p != pa })
	if pidOf(m, "b") != pb {
		t.Error("修改 a 不应重启 b")
	}
	pa = pidOf(m, "a")

	body = *instOf(m, "b")
	body.Remark, body.Group = "备注", "家里"
	if code := doJSON(t, "PUT", base+"/b", body, nil); code != 200 {
		t.Fatal(code)
	}
	if pidOf(m, "b") != pb || pidOf(m, "a") != pa {
		t.Error("只改分组与备注不应重启")
	}

	if code := doJSON(t, "POST", base+"/b/restart", nil, nil); code != 200 {
		t.Fatal(code)
	}
	waitFor(t, "b 重启", func() bool { p := pidOf(m, "b"); return p != 0 && p != pb })
	if pidOf(m, "a") != pa {
		t.Error("重启接口应只重启 b")
	}
	if code := doJSON(t, "POST", base+"/zzz/restart", nil, nil); code != 404 {
		t.Errorf("不存在的实例应返回 404: %d", code)
	}
	pb = pidOf(m, "b")

	// 程序设置不变时保存不重启
	if code := doJSON(t, "PUT", srv.URL+"/settings", Settings{}, nil); code != 200 {
		t.Fatal(code)
	}
	if pidOf(m, "a") != pa || pidOf(m, "b") != pb {
		t.Error("程序路径未变时不应重启")
	}

	if code := doJSON(t, "DELETE", base+"/a", nil, nil); code != 200 {
		t.Fatal(code)
	}
	if runOf(m, "a") != nil || pidOf(m, "b") != pb {
		t.Error("删除 a 应只停止 a")
	}

	// 按实例的状态接口
	var st StatusView
	if code := doJSON(t, "GET", base+"/b/status", nil, &st); code != 200 || st.Key != "b" || !st.Daemon.Running || st.DaemonStatus != "Idle" {
		t.Errorf("实例状态错误: %d %+v", code, st)
	}
	var all []StatusView
	if code := doJSON(t, "GET", srv.URL+"/status", nil, &all); code != 200 || len(all) != 1 || all[0].Name != "local" {
		t.Errorf("全部实例状态错误: %d %+v", code, all)
	}
	if code := doJSON(t, "GET", base+"/a/status", nil, nil); code != 404 {
		t.Errorf("已删除实例的状态应返回 404: %d", code)
	}

	_ = m.Stop()
	if len(m.runs) != 0 {
		t.Errorf("模块停止后应停止全部实例: %d", len(m.runs))
	}
}

// TestRunningConflict 验证手动改出冲突的配置时，排在前面的照常启动，后面的不启动并显示原因；前一个停用后后一个自动启动。
func TestRunningConflict(t *testing.T) {
	m := moduleIn(t, t.TempDir())
	m.cfg.Settings.NetbirdPath = filepath.Join(t.TempDir(), "missing", "netbird")
	k := Options{ManageDaemon: true}
	k2 := k
	k2.Port = 51900
	m.cfg.Instances = []Instance{
		{Key: "a", Name: "first", Enable: true, Options: k},
		{Key: "b", Name: "second", Enable: true, Options: k2},
		// 与 a 端口相同（都是默认 51820）的用户态实例
		{Key: "c", Name: "third", Enable: true, Options: DefaultOptions()},
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	if runOf(m, "a").conflict != "" || !strings.Contains(runOf(m, "b").conflict, "first") || !strings.Contains(runOf(m, "c").conflict, "WireGuard") {
		t.Fatalf("后面冲突的实例不应启动: %q %q %q", runOf(m, "a").conflict, runOf(m, "b").conflict, runOf(m, "c").conflict)
	}
	if e := runOf(m, "b").getErr(); !strings.Contains(e, "内核模式") {
		t.Errorf("冲突原因应显示在实例错误中: %q", e)
	}
	m.mu.Lock()
	m.cfg.Instances[0].Enable = false
	m.applyLocked("a")
	m.mu.Unlock()
	if runOf(m, "b").conflict != "" || runOf(m, "c").conflict != "" {
		t.Errorf("冲突解除后应重新启动: %q %q", runOf(m, "b").conflict, runOf(m, "c").conflict)
	}
}

// TestRemoveInstanceDirs 验证删除实例时只删除默认目录下的状态：自定义目录与迁移来的旧目录保留，
// 手动改坏的 key（含 .. 或路径分隔符）不会删到配置目录以外。
func TestRemoveInstanceDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("使用 shell 脚本模拟程序")
	}
	dir := t.TempDir()
	m := moduleIn(t, dir)
	fakeDaemonScript(t, dir)
	custom := filepath.Join(t.TempDir(), "custom-state")
	outside := filepath.Join(dir, "outside")
	_ = os.MkdirAll(outside, 0o700)
	_ = os.WriteFile(filepath.Join(outside, "keep"), nil, 0o600)
	o := func(port int, socks string) Options {
		x := DefaultOptions()
		x.Port, x.Socks5 = port, socks
		return x
	}
	co := o(51832, "127.0.0.1:1082")
	co.StateDir = custom
	m.cfg.Instances = []Instance{
		{Key: "fresh", Name: "fresh", Enable: true, Options: o(51830, "127.0.0.1:1080")},
		{Key: "legacy", Name: "netbird", Enable: true, Options: o(51831, "127.0.0.1:1081"), LegacyStateDir: true},
		{Key: "custom", Name: "custom", Enable: true, Options: co},
		// 手动改坏的 key：不拼默认目录，启动失败，删除时也不会碰到 <配置目录>/outside
		{Key: "../outside", Name: "evil", Enable: true, Options: o(51833, "127.0.0.1:1083")},
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	srv := serve(t, m)
	base := srv.URL + "/instances"
	waitFor(t, "守护进程运行", func() bool { return pidOf(m, "fresh") != 0 && pidOf(m, "legacy") != 0 && pidOf(m, "custom") != 0 })
	if e := runOf(m, "../outside").getErr(); !strings.Contains(e, "状态目录") {
		t.Errorf("key 非法的实例不应启动: %q", e)
	}
	freshDir := filepath.Join(dir, "netbird", "fresh")
	if _, err := os.Stat(freshDir); err != nil {
		t.Fatalf("默认状态目录应已创建: %v", err)
	}
	for _, k := range []string{"fresh", "legacy", "custom"} {
		if code := doJSON(t, "DELETE", base+"/"+k, nil, nil); code != 200 {
			t.Fatalf("删除 %s 失败: %d", k, code)
		}
	}
	if _, err := os.Stat(freshDir); !os.IsNotExist(err) {
		t.Errorf("默认状态目录应被删除: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "netbird", "pid")); err != nil {
		t.Errorf("迁移来的旧目录应保留: %v", err)
	}
	if _, err := os.Stat(filepath.Join(custom, "pid")); err != nil {
		t.Errorf("自定义状态目录应保留: %v", err)
	}
	// 非法 key 在路径中无法表达，直接调用删除回调
	m.mu.Lock()
	m.cfg.Instances = nil
	m.removeLocked("../outside")
	m.mu.Unlock()
	if _, err := os.Stat(filepath.Join(outside, "keep")); err != nil {
		t.Errorf("配置目录以外的目录不应被删除: %v", err)
	}
}

// TestResetInstances 验证恢复默认配置会清空实例与程序设置并删除默认状态目录（含迁移自旧版的目录），自定义状态目录保留。
func TestResetInstances(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "netbird.json"), []byte(`{"enable":false,"hostname":"nas","setupKey":"k","installProxy":"http://127.0.0.1:7890"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	m := moduleIn(t, dir)
	legacy := filepath.Join(dir, "netbird")
	_ = os.MkdirAll(filepath.Join(legacy, "k2"), 0o700)
	_ = os.WriteFile(filepath.Join(legacy, "default.json"), []byte("{}"), 0o600)
	custom := filepath.Join(t.TempDir(), "nb")
	_ = os.MkdirAll(custom, 0o700)
	co := DefaultOptions()
	co.StateDir = custom
	m.cfg.Instances = append(m.cfg.Instances, Instance{Key: "k2", Name: "b", Options: DefaultOptions()}, Instance{Key: "k3", Name: "c", Options: co})

	m.mu.Lock()
	err := m.resetLocked()
	m.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(m.cfg.Instances) != 0 || m.cfg.Settings != (Settings{}) {
		t.Errorf("实例与程序设置应清空: %+v", m.cfg)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Errorf("默认状态目录应被删除: %v", err)
	}
	if _, err := os.Stat(custom); err != nil {
		t.Errorf("自定义状态目录应保留: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "netbird.json"))
	if !strings.Contains(string(data), `"instances": []`) {
		t.Errorf("恢复默认后应保存为空的新格式: %s", data)
	}
}

// TestProgramSettings 验证程序设置接口只保存程序路径，下载代理只能经安装接口修改。
func TestProgramSettings(t *testing.T) {
	m := moduleIn(t, t.TempDir())
	m.cfg.Settings.InstallProxy = "http://127.0.0.1:7890"
	srv := serve(t, m)
	if code := doJSON(t, "PUT", srv.URL+"/settings", map[string]string{"netbirdPath": " /opt/nb/netbird ", "installProxy": "http://evil:1"}, nil); code != 200 {
		t.Fatal(code)
	}
	if m.cfg.Settings != (Settings{NetbirdPath: "/opt/nb/netbird", InstallProxy: "http://127.0.0.1:7890"}) {
		t.Errorf("程序设置保存错误: %+v", m.cfg.Settings)
	}
	var v SettingsView
	if code := doJSON(t, "GET", srv.URL+"/settings", nil, &v); code != 200 || v.Settings.NetbirdPath != "/opt/nb/netbird" || v.DefaultManagementURL != DefaultManagementURL {
		t.Errorf("读取程序设置错误: %d %+v", code, v)
	}
	// 旧版单实例的字段不再被程序设置接口接受
	if code := doJSON(t, "PUT", srv.URL+"/settings", map[string]any{"enable": true}, nil); code != 400 {
		t.Errorf("未知字段应被拒绝: %d", code)
	}
}
