package easytier

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/openports"
)

type testRouter struct{ mux *http.ServeMux }

func (r testRouter) HandleFunc(pattern string, h http.HandlerFunc) { r.mux.HandleFunc(pattern, h) }

func testEnv(store *config.Store) module.Env {
	return module.Env{Store: store, Logs: logbuf.New(500, log.New(io.Discard, "", 0))}
}

func newTestModule(t *testing.T) (*Module, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("使用 shell 脚本模拟程序")
	}
	dir := t.TempDir()
	store, err := config.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := New()
	if err := m.Init(testEnv(store)); err != nil {
		t.Fatal(err)
	}
	return m, dir
}

func serve(t *testing.T, m *Module) string {
	t.Helper()
	mux := http.NewServeMux()
	m.Routes(testRouter{mux})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
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

func errResp(t *testing.T, method, url string, body any) (int, string) {
	t.Helper()
	var out struct {
		Error string `json:"error"`
	}
	code := doJSON(t, method, url, body, &out)
	return code, out.Error
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("等待超时: %s", what)
}

func runOf(m *Module, key string) *instRuntime {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runs[key]
}

func pidOf(m *Module, key string) int {
	if rt := runOf(m, key); rt != nil {
		return rt.daemon.State().PID
	}
	return 0
}

// logText 返回缓冲中全部日志的文本。
func logText(m *Module) string {
	var sb strings.Builder
	for _, e := range m.logs.Since(0) {
		sb.WriteString(e.Message + "\n")
	}
	return sb.String()
}

// peersJSON 与 nodeJSON 模拟 easytier-cli 的输出；node 输出里的 config 含网络密钥，模块不能转发它。
const peersJSON = `[
{"cidr":"10.9.0.1/24","ipv4":"10.9.0.1","hostname":"self","cost":"Local","lat_ms":"-","loss_rate":"-","rx_bytes":"-","tx_bytes":"-","tunnel_proto":"-","nat_type":"Restricted","id":"1","version":"2.6.4"},
{"cidr":"10.9.0.3/24","ipv4":"10.9.0.3","hostname":"zeta","cost":"relay(2)","lat_ms":"250.5","loss_rate":"3.0%","rx_bytes":"1 kB","tx_bytes":"2 kB","tunnel_proto":"udp","nat_type":"Symmetric","id":"3","version":"2.6.4"},
{"cidr":"10.9.0.2/24","ipv4":"10.9.0.2","hostname":"alpha","cost":"p2p","lat_ms":"23.4","loss_rate":"0.0%","rx_bytes":"5.93 kB","tx_bytes":"6.83 kB","tunnel_proto":"tcp","nat_type":"FullCone","id":"2","version":"2.6.4"}
]`

const nodeJSON = `{"peer_id":1,"ipv4_addr":"10.9.0.1/24","hostname":"self","stun_info":{"public_ip":["1.2.3.4"]},
"listeners":["ring://abc","tcp://0.0.0.0:11010"],"config":"network_secret = \"s3cret\"","version":"2.6.4"}`

// installFakes 在 dir/bin 下写入模拟的 easytier-core 与 easytier-cli。core 把参数与环境变量记到 dir/<名称>.args、<名称>.env，
// 打印含密钥的启动配置，然后常驻；cli 按子命令输出上面的 JSON，dir/cli-fail 存在时模拟管理端口尚未就绪。
func installFakes(t *testing.T, dir string) {
	t.Helper()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	core := `#!/bin/sh
D="` + dir + `"
N=""
for a in "$@"; do case "$a" in --hostname=*) N="${a#--hostname=}";; esac; done
[ -z "$N" ] && N=default
echo "$@" > "$D/$N.args"
echo "$ET_NETWORK_SECRET" > "$D/$N.env"
echo "start" >> "$D/$N.starts"
if [ -f "$D/tun-fail-once" ] && [ "$(wc -l < "$D/$N.starts" | tr -d ' ')" -lt 2 ]; then echo "ERROR CORE::INSTANCE: [abc] tun device error err=rust tun error No such device (os error 19)"; sleep 0.2; exit 1; fi
[ -f "$D/tun-fail" ] && echo "2026-11-12T22:17:19+08:00 ERROR CORE::INSTANCE: [abc] tun device error err=rust tun error Operation not permitted (os error 1)"
echo '############### TOML ###############'
echo '[network_identity]'
echo 'network_name = "home"'
echo "network_secret = \"$ET_NETWORK_SECRET\""
exec sleep 30
`
	cli := `#!/bin/sh
D="` + dir + `"
echo "$@" >> "$D/cli.calls"
if [ -f "$D/cli-fail" ]; then
  echo "Error: failed to get manage client" >&2
  echo "Caused by:" >&2
  echo "    3: Connection refused (os error 61)" >&2
  exit 1
fi
case " $* " in
*" peer "*) cat <<'EOF'
` + peersJSON + `
EOF
;;
*" node "*) cat <<'EOF'
` + nodeJSON + `
EOF
;;
esac
`
	for name, body := range map[string]string{"easytier-core": core, "easytier-cli": cli} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func readFile(dir, name string) string {
	b, _ := os.ReadFile(filepath.Join(dir, name))
	return strings.TrimSpace(string(b))
}

// TestStartAndStatus 验证启动参数、密钥处理与状态解析：密钥只经环境变量传递且不出现在日志、状态与实例列表中，
// 管理端口只监听本机回环地址，节点按直连优先排序。
func TestStartAndStatus(t *testing.T) {
	m, dir := newTestModule(t)
	installFakes(t, dir)
	o := validOptions()
	o.Hostname = "self"
	m.cfg.Instances = []Instance{{Key: "a", Name: "home", Enable: true, Options: o}}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	waitFor(t, "core 启动", func() bool { return pidOf(m, "a") != 0 && readFile(dir, "self.env") != "" })

	args := readFile(dir, "self.args")
	if strings.Contains(args, "s3cret") {
		t.Errorf("密钥不应出现在命令行: %s", args)
	}
	if readFile(dir, "self.env") != "s3cret" {
		t.Errorf("密钥应经环境变量传递: %q", readFile(dir, "self.env"))
	}
	if !strings.Contains(args, "--network-name=home") || !strings.Contains(args, "--no-listener") {
		t.Errorf("命令行参数错误: %s", args)
	}
	rpc := runOf(m, "a").rpcAddr
	host, port, err := net.SplitHostPort(rpc)
	if err != nil || host != "127.0.0.1" || port == "0" {
		t.Fatalf("管理端口应是本机回环地址: %q", rpc)
	}
	if !strings.Contains(args, "--rpc-portal="+rpc) || !strings.Contains(args, "--rpc-portal-whitelist=127.0.0.1/32,::1/128") {
		t.Errorf("管理端口参数错误: %s", args)
	}

	v, ok := m.status("a")
	if !ok || v.Status != "running" || v.Error != "" {
		t.Fatalf("状态错误: %+v", v)
	}
	if len(v.Peers) != 2 || v.Peers[0].Name != "alpha" || !v.Peers[0].Direct || v.Peers[1].Name != "zeta" || v.Peers[1].Cost != "relay(2)" || v.PeersDirect != 1 {
		t.Errorf("节点应直连优先排序: %+v", v.Peers)
	}
	if p := v.Peers[0]; p.LatencyMs != 23.4 || p.IP != "10.9.0.2" || p.Rx != "5.93 kB" || p.Tunnel != "tcp" || p.NAT != "FullCone" {
		t.Errorf("节点字段错误: %+v", p)
	}
	if v.Self == nil || v.Self.IP != "10.9.0.1" || v.Self.NAT != "Restricted" || len(v.Self.Listeners) != 1 || v.Self.Listeners[0] != "tcp://0.0.0.0:11010" || v.Self.PublicIPs[0] != "1.2.3.4" {
		t.Errorf("本机信息错误（ring 通道应被过滤）: %+v", v.Self)
	}
	if v.Version != "" && !strings.Contains(v.Version, ".") {
		t.Errorf("版本: %q", v.Version)
	}
	calls := readFile(dir, "cli.calls")
	if !strings.Contains(calls, "-p "+rpc+" -o json peer") {
		t.Errorf("cli 应经管理端口读取: %s", calls)
	}
	// 状态、实例列表与日志都不含密钥
	data, _ := json.Marshal(v)
	waitFor(t, "启动配置写入日志", func() bool { return strings.Contains(logText(m), "network_secret") })
	if strings.Contains(string(data), "s3cret") || strings.Contains(logText(m), "s3cret") {
		t.Errorf("密钥泄露:\n状态 %s\n日志 %s", data, logText(m))
	}
	if !strings.Contains(logText(m), `network_secret = "********"`) {
		t.Errorf("启动配置中的密钥应被打码: %s", logText(m))
	}
	items := m.Summary()
	if items[0].Value != "运行中" || items[0].Level != "ok" || items[1].Value != "1/2" {
		t.Errorf("摘要错误: %+v", items)
	}
}

// TestStarting 验证管理端口尚未就绪时显示“启动中”而不是错误。
func TestStarting(t *testing.T) {
	m, dir := newTestModule(t)
	installFakes(t, dir)
	_ = os.WriteFile(filepath.Join(dir, "cli-fail"), nil, 0o600)
	m.cfg.Instances = []Instance{{Key: "a", Name: "home", Enable: true, Options: validOptions()}}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	waitFor(t, "core 启动", func() bool { return pidOf(m, "a") != 0 })
	v, _ := m.status("a")
	if v.Status != "starting" || v.Error != "" || len(v.Peers) != 0 {
		t.Errorf("管理端口未就绪应为启动中: %+v", v)
	}
	if got := m.Summary(); got[0].Value != "启动中" {
		t.Errorf("摘要错误: %+v", got)
	}
}

// TestNotInstalled 验证没有程序时提示安装，停用的实例不报错。
func TestNotInstalled(t *testing.T) {
	m, _ := newTestModule(t)
	// 隔离 PATH，避免本机恰好装了 easytier
	t.Setenv("PATH", t.TempDir())
	m.cfg.Instances = []Instance{{Key: "a", Name: "on", Enable: true, Options: validOptions()}, {Key: "b", Name: "off", Options: validOptions()}}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	if v, _ := m.status("a"); !strings.Contains(v.Error, "未找到 easytier-core") || v.Status != "stopped" {
		t.Errorf("缺少程序应提示: %+v", v)
	}
	if v, _ := m.status("b"); v.Error != "" || v.Status != "stopped" {
		t.Errorf("停用的实例不应报错: %+v", v)
	}
	if got := m.Summary(); got[0].Level != "error" {
		t.Errorf("摘要应为异常: %+v", got)
	}
}

// TestAPI 验证增删改接口：密钥脱敏并可凭占位符保留、冲突与非法参数被拒绝、
// 只改某个实例只重启它自己、只改备注不重启、删除只停止被删的实例。
func TestAPI(t *testing.T) {
	m, dir := newTestModule(t)
	installFakes(t, dir)
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	base := serve(t, m) + "/instances"

	a := validOptions()
	a.Hostname = "ha"
	a.Listeners = "tcp://0.0.0.0:11010"
	b := validOptions()
	b.Hostname = "hb"
	b.NetworkName = "office"
	var va, vb InstanceView
	if code := doJSON(t, "POST", base, Instance{Name: "a", Enable: true, Options: a}, &va); code != 200 {
		t.Fatalf("新增 a 失败: %d", code)
	}
	if code := doJSON(t, "POST", base, Instance{Name: "b", Enable: true, Options: b}, &vb); code != 200 {
		t.Fatalf("新增 b 失败: %d", code)
	}
	if va.NetworkSecret == "s3cret" || vb.NetworkSecret == "" {
		t.Errorf("返回的密钥应脱敏: %q", va.NetworkSecret)
	}
	waitFor(t, "两个进程运行", func() bool { return pidOf(m, va.Key) != 0 && pidOf(m, vb.Key) != 0 })
	pa, pb := pidOf(m, va.Key), pidOf(m, vb.Key)

	var list []InstanceView
	doJSON(t, "GET", base, nil, &list)
	raw, _ := json.Marshal(list)
	if len(list) != 2 || strings.Contains(string(raw), "s3cret") {
		t.Errorf("列表不应含密钥: %s", raw)
	}
	var secrets map[string]string
	if code := doJSON(t, "GET", base+"/"+va.Key+"/secrets", nil, &secrets); code != 200 || secrets["networkSecret"] != "s3cret" {
		t.Errorf("secrets 接口应返回明文: %d %v", code, secrets)
	}

	// 冲突与非法参数
	dup := validOptions()
	dup.Listeners = "udp://0.0.0.0:11010\ntcp://0.0.0.0:11010"
	if code, msg := errResp(t, "POST", base, Instance{Name: "c", Enable: true, Options: dup}); code != 400 || !strings.Contains(msg, "11010") {
		t.Errorf("端口冲突应返回 400: %d %s", code, msg)
	}
	if code, _ := errResp(t, "POST", base, Instance{Name: "a", Options: validOptions()}); code != 400 {
		t.Errorf("名称重复应返回 400: %d", code)
	}
	evil := validOptions()
	evil.Peers = "--rpc-portal=0.0.0.0:1"
	if code, _ := errResp(t, "POST", base, Instance{Name: "evil", Options: evil}); code != 400 {
		t.Errorf("参数注入应返回 400: %d", code)
	}
	// 停用的实例可以保存同样的端口，启用时才检查
	idle := validOptions()
	idle.Listeners = "tcp://0.0.0.0:11010"
	var vi InstanceView
	if code := doJSON(t, "POST", base, Instance{Name: "idle", Options: idle}, &vi); code != 200 {
		t.Fatalf("停用实例保存失败: %d", code)
	}
	if code, _ := errResp(t, "PUT", base+"/"+vi.Key+"/enable", map[string]bool{"enable": true}); code != 400 {
		t.Errorf("启用时应检查冲突: %d", code)
	}
	if code := doJSON(t, "DELETE", base+"/"+vi.Key, nil, nil); code != 200 {
		t.Errorf("删除失败: %d", code)
	}

	// 修改 a：密钥保持占位符时沿用旧密钥，只重启 a
	upd := Instance{Key: va.Key, Name: "a", Enable: true, Options: a}
	upd.NetworkSecret = va.NetworkSecret
	upd.Hostname = "ha"
	upd.LatencyFirst = true
	if code := doJSON(t, "PUT", base+"/"+va.Key, upd, nil); code != 200 {
		t.Fatalf("修改失败: %d", code)
	}
	waitFor(t, "a 重启", func() bool { p := pidOf(m, va.Key); return p != 0 && p != pa })
	if pidOf(m, vb.Key) != pb {
		t.Error("修改 a 不应重启 b")
	}
	waitFor(t, "新参数生效", func() bool { return strings.Contains(readFile(dir, "ha.args"), "--latency-first=true") })
	if readFile(dir, "ha.env") != "s3cret" {
		t.Errorf("占位符应沿用旧密钥: %q", readFile(dir, "ha.env"))
	}
	// 只改备注与分组不重启
	pa = pidOf(m, va.Key)
	upd.Remark, upd.Group = "备注", "分组"
	if code := doJSON(t, "PUT", base+"/"+va.Key, upd, nil); code != 200 {
		t.Fatalf("修改备注失败: %d", code)
	}
	time.Sleep(200 * time.Millisecond)
	if pidOf(m, va.Key) != pa {
		t.Error("只改备注不应重启")
	}
	// 重启接口只重启指定实例
	if code := doJSON(t, "POST", base[:len(base)-len("/instances")]+"/instances/"+vb.Key+"/restart", nil, nil); code != 200 {
		t.Fatalf("重启失败: %d", code)
	}
	waitFor(t, "b 重启", func() bool { p := pidOf(m, vb.Key); return p != 0 && p != pb })
	if pidOf(m, va.Key) != pa {
		t.Error("重启 b 不应影响 a")
	}
	if code, _ := errResp(t, "POST", base[:len(base)-len("/instances")]+"/instances/nope/restart", nil); code != 404 {
		t.Errorf("实例不存在应返回 404: %d", code)
	}
	// 复制沿用密钥
	var vc InstanceView
	cp := Instance{Name: "copy", Options: b}
	cp.NetworkSecret = vb.NetworkSecret
	if code := doJSON(t, "POST", base+"/"+vb.Key+"/copy", cp, &vc); code != 200 {
		t.Fatalf("复制失败: %d", code)
	}
	var cs map[string]string
	doJSON(t, "GET", base+"/"+vc.Key+"/secrets", nil, &cs)
	if cs["networkSecret"] != "s3cret" {
		t.Errorf("复制应沿用密钥: %v", cs)
	}
	if !coversPort(11010) {
		t.Error("运行中的实例应登记 TCP 监听端口")
	}
	// 删除只停止被删的实例
	pb = pidOf(m, vb.Key)
	if code := doJSON(t, "DELETE", base+"/"+va.Key, nil, nil); code != 200 {
		t.Fatalf("删除失败: %d", code)
	}
	waitFor(t, "a 停止", func() bool { return runOf(m, va.Key) == nil })
	// 删除后撤销 a 登记的开放端口（11010），b 没有监听不受影响
	if coversPort(11010) {
		t.Error("删除实例后应撤销开放端口登记")
	}
	if pidOf(m, vb.Key) != pb {
		t.Error("删除 a 不应重启 b")
	}
}

// TestRunningConflict 验证手动改坏的配置：排在前面的实例照常运行，后面冲突的实例不启动并显示原因，冲突解除后自动启动。
func TestRunningConflict(t *testing.T) {
	m, dir := newTestModule(t)
	installFakes(t, dir)
	x := validOptions()
	x.Listeners = "tcp://0.0.0.0:11010"
	x.Hostname = "first"
	y := x
	y.Hostname = "second"
	m.cfg.Instances = []Instance{{Key: "a", Name: "first", Enable: true, Options: x}, {Key: "b", Name: "second", Enable: true, Options: y}}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	waitFor(t, "first 运行", func() bool { return pidOf(m, "a") != 0 })
	if runOf(m, "b").conflict == "" || !strings.Contains(runOf(m, "b").getErr(), "first") || pidOf(m, "b") != 0 {
		t.Fatalf("冲突的实例不应启动: %q", runOf(m, "b").conflict)
	}
	m.mu.Lock()
	m.cfg.Instances[0].Enable = false
	m.applyLocked("a")
	m.mu.Unlock()
	waitFor(t, "second 启动", func() bool { return pidOf(m, "b") != 0 })
}

// TestSettingsAndReset 验证程序路径变化后重启实例，恢复默认配置停止全部进程并清空配置。
func TestSettingsAndReset(t *testing.T) {
	m, dir := newTestModule(t)
	installFakes(t, dir)
	m.cfg.Instances = []Instance{{Key: "a", Name: "home", Enable: true, Options: validOptions()}}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	waitFor(t, "core 启动", func() bool { return pidOf(m, "a") != 0 })
	pa := pidOf(m, "a")
	base := serve(t, m)

	var sv SettingsView
	doJSON(t, "GET", base+"/settings", nil, &sv)
	if sv.Core != filepath.Join(dir, "bin", "easytier-core") {
		t.Errorf("应使用配置目录 bin 下的程序: %q", sv.Core)
	}
	// 同样的设置不重启
	if code := doJSON(t, "PUT", base+"/settings", Settings{}, nil); code != 200 {
		t.Fatal("保存设置失败")
	}
	time.Sleep(200 * time.Millisecond)
	if pidOf(m, "a") != pa {
		t.Error("设置未变不应重启")
	}
	// 下载代理只由安装接口修改
	m.mu.Lock()
	m.cfg.Settings.InstallProxy = "http://127.0.0.1:7890"
	m.mu.Unlock()
	doJSON(t, "PUT", base+"/settings", Settings{InstallProxy: "http://evil"}, nil)
	if m.cfg.Settings.InstallProxy != "http://127.0.0.1:7890" {
		t.Errorf("PUT /settings 不应修改下载代理: %q", m.cfg.Settings.InstallProxy)
	}
	if code, _ := errResp(t, "PUT", base+"/settings", map[string]any{"corePath": 1}); code != 400 {
		t.Errorf("非法请求体应返回 400: %d", code)
	}

	if code := doJSON(t, "POST", base+"/reset", nil, nil); code != 200 {
		t.Fatalf("恢复默认失败: %d", code)
	}
	waitFor(t, "进程停止", func() bool { return runOf(m, "a") == nil })
	if len(m.cfg.Instances) != 0 || m.cfg.Settings != (Settings{}) {
		t.Errorf("恢复默认后配置应清空: %+v", m.cfg)
	}
	m2 := New()
	store, _ := config.Open(dir)
	_ = m2.Init(testEnv(store))
	if len(m2.cfg.Instances) != 0 {
		t.Errorf("重置应已保存: %+v", m2.cfg)
	}
}

// TestTunError 验证 easytier-core 创建 TUN 网卡失败（进程不退出）时，原因显示在实例上；保存后重启并清除该提示。
func TestTunError(t *testing.T) {
	m, dir := newTestModule(t)
	installFakes(t, dir)
	_ = os.WriteFile(filepath.Join(dir, "tun-fail"), nil, 0o600)
	m.cfg.Instances = []Instance{{Key: "a", Name: "home", Enable: true, Options: validOptions()}}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	waitFor(t, "TUN 错误提示", func() bool { return strings.Contains(runOf(m, "a").getErr(), "Operation not permitted") })
	if e := runOf(m, "a").getErr(); !strings.Contains(e, "无法创建 TUN 网卡") || !strings.Contains(e, "用户态网络") {
		t.Errorf("提示应说明原因与解决办法: %q", e)
	}
	if v, _ := m.status("a"); v.Error == "" || v.Status != "running" {
		t.Errorf("状态应带错误: %+v", v)
	}
	if got := m.Summary(); got[0].Level != "error" {
		t.Errorf("摘要应为异常: %+v", got)
	}
	// 改用用户态网络后保存即重试，旧的错误随旧运行时清除
	_ = os.Remove(filepath.Join(dir, "tun-fail"))
	m.mu.Lock()
	m.cfg.Instances[0].NoTun = true
	m.applyLocked("a")
	m.mu.Unlock()
	time.Sleep(300 * time.Millisecond)
	if e := runOf(m, "a").getErr(); e != "" {
		t.Errorf("重启后不应再有错误: %q", e)
	}
}

// TestErrorClearedOnRestart 验证进程因 TUN 不可用退出、被守护重启后（例如补装了驱动）旧的错误提示会清除。
func TestErrorClearedOnRestart(t *testing.T) {
	m, dir := newTestModule(t)
	installFakes(t, dir)
	_ = os.WriteFile(filepath.Join(dir, "tun-fail-once"), nil, 0o600)
	m.cfg.Instances = []Instance{{Key: "a", Name: "home", Enable: true, Options: validOptions()}}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	waitFor(t, "第二次启动", func() bool { return strings.Count(readFile(dir, "default.starts"), "start") >= 2 && pidOf(m, "a") != 0 })
	waitFor(t, "错误清除", func() bool { return runOf(m, "a").getErr() == "" })
}

// TestExitedShown 验证 easytier-core 启动即退出（如参数错误）时，实例显示退出原因而不是静默的“已停止”。
func TestExitedShown(t *testing.T) {
	m, dir := newTestModule(t)
	bin := filepath.Join(dir, "bin")
	_ = os.MkdirAll(bin, 0o755)
	_ = os.WriteFile(filepath.Join(bin, "easytier-core"), []byte("#!/bin/sh\necho 'error: unexpected argument' >&2\nexit 2\n"), 0o755)
	_ = os.WriteFile(filepath.Join(bin, "easytier-cli"), []byte("#!/bin/sh\nexit 1\n"), 0o755)
	m.cfg.Instances = []Instance{{Key: "a", Name: "bad", Enable: true, Options: validOptions()}}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	waitFor(t, "退出提示", func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		return strings.Contains(m.instanceView(&m.cfg.Instances[0]).Error, "进程已退出")
	})
	if v := m.queryStatus("a"); !strings.Contains(v.Error, "exit status 2") || v.Status != "stopped" {
		t.Errorf("状态应带退出原因: %+v", v)
	}
}

// coversPort 判断端口是否已登记为开放端口
func coversPort(port int) bool {
	all, _ := openports.All()
	for _, r := range all {
		if r.Lo <= port && port <= r.Hi {
			return true
		}
	}
	return false
}
