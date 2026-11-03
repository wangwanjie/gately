package netbird

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/binstall"
	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

func TestArgs(t *testing.T) {
	s := DefaultSettings()
	s.SetupKey = "A1B2C3"
	s.Hostname = "gately"
	s.ManagementURL = "https://nb.example.com/"
	s.AcceptRoutes = true
	s.Port = 51821
	s.Socks5 = "0.0.0.0:1081"
	s.ExtraUpArgs = "--allow-server-ssh  --mtu=1400"
	s.DaemonEnv = "NB_LOG_LEVEL=debug"
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	up := s.UpArgs()
	for _, want := range []string{
		"up", "--no-browser", "--management-url=https://nb.example.com", "--disable-client-routes=false", "--disable-server-routes=false",
		"--disable-dns=true", "--block-inbound=false", "--disable-auto-connect=true", "--hostname=gately", "--wireguard-port=51821", "--allow-server-ssh", "--mtu=1400",
	} {
		if !slices.Contains(up, want) {
			t.Errorf("up 参数缺少 %s: %v", want, up)
		}
	}
	// 设置密钥只经环境变量传递，不出现在命令行中
	if slices.ContainsFunc(up, func(a string) bool { return strings.Contains(a, "A1B2C3") }) {
		t.Errorf("设置密钥不应出现在命令行: %v", up)
	}
	if env := s.UpEnv(); !slices.Contains(env, "NB_SETUP_KEY=A1B2C3") || !slices.Contains(env, "DISPLAY=") {
		t.Errorf("up 环境变量错误: %v", env)
	}

	d := s.DaemonArgs("/etc/gately")
	if want := []string{"service", "run", "--daemon-addr=unix:///etc/gately/netbird/netbird.sock", "--log-file=console"}; !slices.Equal(d, want) {
		t.Errorf("守护进程参数错误: %v", d)
	}
	wantEnv := []string{"NB_STATE_DIR=/etc/gately/netbird", "NB_USE_NETSTACK_MODE=true", "NB_ENABLE_NETSTACK_LOCAL_FORWARDING=true", "NB_SOCKS5_LISTENER_ADDRESS=0.0.0.0", "NB_SOCKS5_LISTENER_PORT=1081", "NB_LOG_LEVEL=debug"}
	if env := s.DaemonEnvList("/etc/gately"); !slices.Equal(env, wantEnv) {
		t.Errorf("守护进程环境变量错误: %v", env)
	}
	if got := s.CLIArgs("/etc/gately"); !slices.Equal(got, []string{"--daemon-addr=unix:///etc/gately/netbird/netbird.sock"}) {
		t.Errorf("CLI 参数错误: %v", got)
	}

	// 默认：官方云地址、不固定端口、不带 SOCKS5 变量；未填设置密钥时没有 NB_SETUP_KEY
	def := DefaultSettings()
	if up := def.UpArgs(); !slices.Contains(up, "--management-url="+DefaultManagementURL) || slices.ContainsFunc(up, func(a string) bool { return strings.HasPrefix(a, "--wireguard-port") }) {
		t.Errorf("默认 up 参数错误: %v", up)
	}
	def.LocalForwarding = false
	if env := def.DaemonEnvList("/x"); !slices.Equal(env, []string{"NB_STATE_DIR=/x/netbird", "NB_USE_NETSTACK_MODE=true"}) {
		t.Errorf("默认环境变量错误: %v", env)
	}
	if env := def.UpEnv(); slices.ContainsFunc(env, func(a string) bool { return strings.HasPrefix(a, "NB_SETUP_KEY") }) {
		t.Errorf("未填设置密钥时不应传 NB_SETUP_KEY: %v", env)
	}

	// 使用外部守护进程时不指定控制地址，沿用 netbird 的系统默认值；关闭用户态网络时不带 netstack 变量
	ext := Settings{ManageDaemon: false}
	if got := ext.CLIArgs("/x"); got != nil {
		t.Errorf("外部守护进程默认不应带 --daemon-addr: %v", got)
	}
	// 外部守护进程保留其自身的自动连接设置
	if up := ext.UpArgs(); slices.ContainsFunc(up, func(a string) bool { return strings.HasPrefix(a, "--disable-auto-connect") }) {
		t.Errorf("外部守护进程不应修改自动连接: %v", up)
	}
	kernel := Settings{ManageDaemon: true, StateDir: "/data/nb"}
	if env := kernel.DaemonEnvList("/x"); !slices.Equal(env, []string{"NB_STATE_DIR=/data/nb"}) {
		t.Errorf("内核模式环境变量错误: %v", env)
	}
}

func TestValidate(t *testing.T) {
	bad := []Settings{
		{StateDir: "relative/dir"},
		{Socks5: "no-port"},
		{Socks5: "localhost:1080"},
		{Socks5: "127.0.0.1:0"},
		{Hostname: "bad_name"},
		{ManagementURL: "nb.example.com"},
		{DaemonAddr: "/var/run/netbird.sock"},
		{DaemonAddr: "unix://relative.sock"},
		{DaemonAddr: "tcp://127.0.0.1"},
		{Port: 70000},
		{DaemonEnv: "NOEQUALS"},
	}
	for i, s := range bad {
		if err := s.Validate(); err == nil {
			t.Errorf("用例 %d 应被拒绝: %+v", i, s)
		}
	}
	good := Settings{DaemonAddr: "tcp://127.0.0.1:41731", Socks5: "[::1]:1080", ManagementURL: "http://192.168.1.2"}
	if err := good.Validate(); err != nil {
		t.Errorf("合法配置被拒绝: %v", err)
	}
}

func TestParseAuthLine(t *testing.T) {
	// 设备码登录（Linux 无图形会话），链接中不含代码时另行输出
	url, code := parseAuthLine("https://login.example.com/activate and enter the code ABCD-EFGH to authenticate.")
	if url != "https://login.example.com/activate" || code != "ABCD-EFGH" {
		t.Errorf("设备码解析错误: %q %q", url, code)
	}
	// 链接已包含代码
	url, code = parseAuthLine("https://nb.example.com/oauth2/device?user_code=WXYZ-1234 ")
	if url != "https://nb.example.com/oauth2/device?user_code=WXYZ-1234" || code != "" {
		t.Errorf("链接解析错误: %q %q", url, code)
	}
	if url, _ := parseAuthLine("Use this URL to log in:"); url != "" {
		t.Errorf("没有链接的行不应解析出链接: %q", url)
	}
}

func TestStatusParse(t *testing.T) {
	raw := `2026-11-03T00:37:04 WARN some log line
{"peers":{"total":2,"connected":1,"details":[
 {"fqdn":"nas.netbird.cloud","netbirdIp":"100.92.1.2","publicKey":"k","status":"Connected","lastStatusUpdate":"2026-11-03T00:00:00Z",
  "connectionType":"P2P","relayAddress":"","transferReceived":100,"transferSent":200,"latency":12500000,"networks":["192.168.1.0/24"]},
 {"fqdn":"phone.netbird.cloud","netbirdIp":"100.92.1.3","status":"Idle","lastStatusUpdate":"0001-01-01T00:00:00Z","connectionType":"-","relayAddress":"rels://relay"}]},
 "cliVersion":"0.82.0","daemonVersion":"0.82.0","daemonStatus":"Connected",
 "management":{"url":"https://api.netbird.io:443","connected":true,"error":""},"signal":{"url":"https://signal.netbird.io:443","connected":true,"error":""},
 "relays":{"total":2,"available":1},"netbirdIp":"100.92.1.1/16","netbirdIpv6":"fd00::1/64","fqdn":"gately.netbird.cloud","usesKernelInterface":false,"wireguardPort":51820}`
	dir := t.TempDir()
	cli := filepath.Join(dir, "netbird")
	if err := os.WriteFile(cli, []byte("#!/bin/sh\ncat <<'EOF'\n"+raw+"\nEOF\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := runStatus(cli, nil)
	if err != nil {
		t.Fatal(err)
	}
	if st.DaemonStatus != "Connected" || len(st.Peers.Details) != 2 || st.Relays.Available != 1 {
		t.Fatalf("解析错误: %+v", st)
	}
	p := peerView(st.Peers.Details[0])
	if p.Name != "nas" || !p.Online || p.ConnType != "P2P" || p.LatencyMs != 12.5 || p.Relay != "" || p.LastSeen.IsZero() || !slices.Equal(p.Routes, []string{"192.168.1.0/24"}) {
		t.Errorf("节点视图错误: %+v", p)
	}
	idle := peerView(st.Peers.Details[1])
	if idle.Online || idle.ConnType != "" || !idle.LastSeen.IsZero() {
		t.Errorf("空闲节点视图错误: %+v", idle)
	}
	if got := ips(st.IP, st.IPv6); !slices.Equal(got, []string{"100.92.1.1", "fd00::1"}) {
		t.Errorf("本机地址应去掉前缀长度: %v", got)
	}

	// 守护进程不可达时输出不是 JSON，返回原始提示
	if err := os.WriteFile(cli, []byte("#!/bin/sh\necho 'failed to connect to daemon'\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := runStatus(cli, nil); err == nil || !strings.Contains(err.Error(), "failed to connect") {
		t.Errorf("应返回原始错误: %v", err)
	}
}

// TestAssetName 对照 v0.82.0 的发布文件名验证各平台的包名。
func TestAssetName(t *testing.T) {
	cases := map[binstall.Platform]string{
		{OS: "linux", Arch: "amd64"}:         "netbird_0.82.0_linux_amd64.tar.gz",
		{OS: "linux", Arch: "arm64"}:         "netbird_0.82.0_linux_arm64.tar.gz",
		{OS: "linux", Arch: "arm", ARM: "7"}: "netbird_0.82.0_linux_armv6.tar.gz",
		{OS: "linux", Arch: "386"}:           "netbird_0.82.0_linux_386.tar.gz",
		{OS: "linux", Arch: "mipsle"}:        "netbird_0.82.0_linux_mipsle_softfloat.tar.gz",
		{OS: "linux", Arch: "mips64"}:        "netbird_0.82.0_linux_mips64_softfloat.tar.gz",
		{OS: "darwin", Arch: "arm64"}:        "netbird_0.82.0_darwin_arm64.tar.gz",
		{OS: "windows", Arch: "amd64"}:       "netbird_0.82.0_windows_amd64.tar.gz",
	}
	for p, want := range cases {
		if got, err := assetName("0.82.0", p); err != nil || got != want {
			t.Errorf("%v: %q %v，期望 %q", p, got, err, want)
		}
	}
	for _, p := range []binstall.Platform{{OS: "linux", Arch: "arm", ARM: "5"}, {OS: "linux", Arch: "riscv64"}, {OS: "freebsd", Arch: "amd64"}, {OS: "windows", Arch: "386"}} {
		if _, err := assetName("0.82.0", p); err == nil {
			t.Errorf("%v 上游没有提供，应报错", p)
		}
	}
	if m := nbVersionRe.FindStringSubmatch("0.82.0\n"); m == nil || m[1] != "0.82.0" {
		t.Errorf("版本解析错误: %v", m)
	}
}

func newTestModule(t *testing.T) (*Module, string) {
	dir := t.TempDir()
	store, err := config.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := New()
	if err := m.Init(module.Env{Store: store, Logs: logbuf.New(100, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	return m, dir
}

// TestReset 验证恢复默认配置会恢复默认设置并删除默认状态目录，自定义状态目录保留。
func TestReset(t *testing.T) {
	m, dir := newTestModule(t)
	state := filepath.Join(dir, "netbird")
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(state, "default.json"), []byte("{}"), 0o600)
	m.settings.Hostname, m.settings.SetupKey, m.settings.InstallProxy, m.settings.ServerRoutes = "nas", "key", "http://127.0.0.1:7890", false

	m.mu.Lock()
	err := m.resetLocked()
	m.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if m.settings != DefaultSettings() {
		t.Errorf("设置未恢复默认: %+v", m.settings)
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Errorf("默认状态目录应被删除: %v", err)
	}

	custom := filepath.Join(t.TempDir(), "nb")
	_ = os.MkdirAll(custom, 0o700)
	m.settings.StateDir = custom
	m.mu.Lock()
	err = m.resetLocked()
	m.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(custom); err != nil {
		t.Errorf("自定义状态目录应保留: %v", err)
	}
}

type testRouter struct{ mux *http.ServeMux }

func (r testRouter) HandleFunc(pattern string, h http.HandlerFunc) { r.mux.HandleFunc(pattern, h) }

// TestSettingsSecret 验证读取设置时密钥脱敏、保存脱敏值时沿用原密钥，以及查看明文接口。
func TestSettingsSecret(t *testing.T) {
	m, _ := newTestModule(t)
	m.settings.SetupKey = "SECRET-KEY"
	mux := http.NewServeMux()
	m.Routes(testRouter{mux})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/settings", nil))
	if strings.Contains(rec.Body.String(), "SECRET-KEY") {
		t.Fatalf("读取设置不应返回密钥明文: %s", rec.Body.String())
	}
	var got struct {
		Settings Settings `json:"settings"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	got.Settings.Hostname = "nas"
	body, _ := json.Marshal(got.Settings)
	rec = httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/settings", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 || m.settings.SetupKey != "SECRET-KEY" || m.settings.Hostname != "nas" {
		t.Fatalf("保存脱敏值应沿用原密钥: %d %s %+v", rec.Code, rec.Body.String(), m.settings)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/secrets", nil))
	var secrets map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &secrets)
	if rec.Code != 200 || secrets["setupKey"] != "SECRET-KEY" {
		t.Fatalf("应返回设置密钥明文: %d %s", rec.Code, rec.Body.String())
	}
}

// TestStatusNotEnabled 验证未启用且守护进程不可达时不报错。
func TestStatusNotEnabled(t *testing.T) {
	m, dir := newTestModule(t)
	bin := filepath.Join(dir, "bin")
	_ = os.MkdirAll(bin, 0o755)
	_ = os.WriteFile(filepath.Join(bin, "netbird"), []byte("#!/bin/sh\necho 'connection refused'\nexit 1\n"), 0o755)
	v := m.queryStatus()
	if v.Error != "" || v.CLI == "" {
		t.Errorf("未启用时不应报错: %+v", v)
	}
	if got := m.Summary(); got[0].Value != "未启用" {
		t.Errorf("摘要错误: %+v", got)
	}
}

// fakeNetbird 写一个模拟 netbird 的脚本，up、down 各记一行到 calls 文件，service run 每次启动记一行到 starts。
// crash 为 true 时第一次启动的守护进程很快异常退出；firstConnected 为 true 时第一次启动的守护进程
// 报告已连接（模拟按旧配置自动连接），之后启动的报告空闲。
func fakeNetbird(t *testing.T, dir string, crash, firstConnected bool) {
	t.Helper()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	status := "Idle"
	if firstConnected {
		status = "Connected"
	}
	script := `#!/bin/sh
D="` + dir + `"
starts() { [ -f "$D/starts" ] && wc -l < "$D/starts" | tr -d ' ' || echo 0; }
case " $* " in
*" service run "*)
  echo start >> "$D/starts"
  if [ "` + strconv.FormatBool(crash) + `" = true ] && [ "$(starts)" -lt 2 ]; then sleep 0.3; exit 1; fi
  exec sleep 30 ;;
*" status --json "*)
  s=Idle; [ "$(starts)" -lt 2 ] && s=` + status + `
  echo "{\"daemonStatus\":\"$s\",\"peers\":{\"details\":[]}}" ;;
*" up "*)
  if [ -f "$D/upfail" ] && [ ! -f "$D/calls" ]; then echo fail >> "$D/calls"; echo "login backoff cycle failed: connection refused"; exit 1; fi
  echo up >> "$D/calls" ;;
*" down "*) echo down >> "$D/calls" ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "netbird"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// waitFile 等待 dir 下的文件内容变为 want。
func waitFile(t *testing.T, dir, name, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var got string
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(filepath.Join(dir, name))
		if got = strings.TrimSpace(string(b)); got == want {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("%s 为 %q，期望 %q", name, got, want)
}

func startFake(t *testing.T, managed, crash, firstConnected bool) (*Module, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("需要 sh")
	}
	m, dir := newTestModule(t)
	fakeNetbird(t, dir, crash, firstConnected)
	m.settings.Enable, m.settings.ManageDaemon = true, managed
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop() })
	return m, dir
}

// TestManagedRestartRunsUp 验证托管的守护进程异常退出被重启后会再次执行 up，空闲时不先断开。
func TestManagedRestartRunsUp(t *testing.T) {
	_, dir := startFake(t, true, true, false)
	waitFile(t, dir, "calls", "up\nup")
}

// TestManagedFirstConnectedRestartsOnce 验证托管的守护进程按旧配置自动连接时先断开再 up（保存关闭自动连接），
// 然后在用户态网络下重启一次守护进程，新进程空闲时直接 up，不再重启。
func TestManagedFirstConnectedRestartsOnce(t *testing.T) {
	_, dir := startFake(t, true, false, true)
	waitFile(t, dir, "calls", "down\nup\nup")
	time.Sleep(time.Second)
	waitFile(t, dir, "starts", "start\nstart")
}

// TestExternalDaemonDownThenUp 验证外部守护进程已连接时先断开再执行 up，让新配置生效，且不启动守护进程。
func TestExternalDaemonDownThenUp(t *testing.T) {
	_, dir := startFake(t, false, false, true)
	waitFile(t, dir, "calls", "down\nup")
	if _, err := os.Stat(filepath.Join(dir, "starts")); err == nil {
		t.Error("外部守护进程模式不应启动 netbird service run")
	}
}

// TestUpRetriesAfterFailure 验证 netbird up 失败（如管理服务暂时不可达）后自动重试，
// 错误提示带有 netbird 输出的原因，成功后清除。
func TestUpRetriesAfterFailure(t *testing.T) {
	old := upRetryMin
	upRetryMin = 1500 * time.Millisecond
	defer func() { upRetryMin = old }()
	if runtime.GOOS == "windows" {
		t.Skip("需要 sh")
	}
	m, dir := newTestModule(t)
	fakeNetbird(t, dir, false, false)
	if err := os.WriteFile(filepath.Join(dir, "upfail"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	m.settings.Enable = true
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop() })
	waitFile(t, dir, "calls", "fail")
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(m.getErr(), "connection refused") && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if e := m.getErr(); !strings.Contains(e, "connection refused") || !strings.Contains(e, "后重试") {
		t.Errorf("失败时应提示原因与重试: %q", e)
	}
	waitFile(t, dir, "calls", "fail\nup")
	deadline = time.Now().Add(3 * time.Second)
	for m.getErr() != "" && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if e := m.getErr(); e != "" {
		t.Errorf("重试成功后应清除错误: %q", e)
	}
}

// TestValidateOptionLike 验证以 - 开头或带空白的主机名、管理服务地址被拒绝，不会被 netbird 当作选项或拆成多个参数。
func TestValidateOptionLike(t *testing.T) {
	bad := []Settings{
		{Hostname: "-x"},
		{Hostname: "a b"},
		{ManagementURL: "-https://nb.example.com"},
		{ManagementURL: "--setup-key=abc"},
		{ManagementURL: "https://nb example.com"},
		{ManagementURL: "ftp://nb.example.com"},
		{Socks5: "0.0.0.0:70000"},
		{Port: -1},
	}
	for i, s := range bad {
		if err := s.Validate(); err == nil {
			t.Errorf("用例 %d 应被拒绝: %+v", i, s)
		}
	}
	// 管理服务地址作为 --management-url= 的取值整体传递
	s := Settings{ManagementURL: " https://nb.example.com:33073/ "}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if up := s.UpArgs(); !slices.Contains(up, "--management-url=https://nb.example.com:33073") {
		t.Errorf("管理服务地址应去掉空白与末尾斜杠: %v", up)
	}
}

// TestSocksAlwaysPublic 验证识别 0.75.0 之前总把 SOCKS5 监听在 0.0.0.0 的 NetBird 版本。
func TestSocksAlwaysPublic(t *testing.T) {
	cases := map[string]bool{
		"0.74.3":      true,
		"0.60.0":      true,
		"v0.74.0\n":   true,
		"0.75.0":      false,
		"0.82.0":      false,
		"1.0.0":       false,
		"development": false,
		"":            false,
	}
	for in, want := range cases {
		if got := socksAlwaysPublic(in); got != want {
			t.Errorf("%q: %v，期望 %v", in, got, want)
		}
	}
}

// writeScript 在 dir/bin 下写入模拟的 netbird 脚本，脚本中的 $D 为 dir。
func writeScript(t *testing.T, dir, body string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("需要 sh")
	}
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nD=\"" + dir + "\"\n" + body
	if err := os.WriteFile(filepath.Join(bin, "netbird"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// waitFor 轮询直到 cond 成立，超时则失败。
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("等待超时: %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// logText 返回模块日志缓冲中的全部内容。
func logText(m *Module) string {
	var b strings.Builder
	for _, e := range m.logBuf.Since(0) {
		b.WriteString(e.Message)
		b.WriteByte('\n')
	}
	return b.String()
}

// TestSetupKeyOnlyInEnv 验证设置密钥经 NB_SETUP_KEY 传给 netbird up，命令行参数与日志中都不出现，
// 并清空图形会话变量让 Linux 使用设备码登录。
func TestSetupKeyOnlyInEnv(t *testing.T) {
	m, dir := newTestModule(t)
	writeScript(t, dir, `case " $* " in
*" status --json "*) echo '{"daemonStatus":"Idle","peers":{"details":[]}}' ;;
*" up "*)
  printf '%s' "$NB_SETUP_KEY" > "$D/key"
  printf '[%s]' "$DISPLAY" > "$D/display"
  echo "up args: $*" ;;
esac
`)
	t.Setenv("DISPLAY", ":0")
	m.settings.Enable, m.settings.ManageDaemon, m.settings.SetupKey = true, false, "NB-SECRET-123"
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop() })
	waitFile(t, dir, "key", "NB-SECRET-123")
	waitFile(t, dir, "display", "[]")
	waitFor(t, "up 完成", func() bool { return strings.Contains(logText(m), "up args:") })
	if logs := logText(m); strings.Contains(logs, "NB-SECRET-123") {
		t.Errorf("日志中不应出现设置密钥:\n%s", logs)
	}
}

// TestReadyTimeoutClearedWhenReady 验证等待守护进程超时的提示在守护进程就绪后清除，
// 即使 up 仍在等待登录；登录链接与设备码可从状态中取得，停止模块时一并清空。
func TestReadyTimeoutClearedWhenReady(t *testing.T) {
	old := readyTimeout
	readyTimeout = 200 * time.Millisecond
	defer func() { readyTimeout = old }()
	m, dir := newTestModule(t)
	writeScript(t, dir, `case " $* " in
*" status --json "*)
  [ -f "$D/ready" ] || { echo 'failed to connect to daemon'; exit 1; }
  echo '{"daemonStatus":"NeedsLogin","peers":{"details":[]}}' ;;
*" up "*)
  echo "Please do the SSO login in your browser."
  echo "https://login.example.com/activate and enter the code ABCD-EFGH to authenticate."
  exec sleep 30 ;;
esac
`)
	m.settings.Enable, m.settings.ManageDaemon = true, false
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop() })
	waitFor(t, "超时提示", func() bool { return strings.Contains(m.getErr(), "超时") })
	if err := os.WriteFile(filepath.Join(dir, "ready"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "登录链接", func() bool { u, _ := m.getAuth(); return u != "" })
	if u, c := m.getAuth(); u != "https://login.example.com/activate" || c != "ABCD-EFGH" {
		t.Errorf("登录链接错误: %q %q", u, c)
	}
	if e := m.getErr(); e != "" {
		t.Errorf("守护进程就绪后应清除超时提示: %q", e)
	}
	v := m.queryStatus()
	if v.AuthURL == "" || v.DaemonStatus != "NeedsLogin" {
		t.Errorf("状态应带登录链接: %+v", v)
	}

	// 停止时取消阻塞中的 up，并清空登录链接
	start := time.Now()
	_ = m.Stop()
	if d := time.Since(start); d > 8*time.Second {
		t.Errorf("停止耗时过长: %s", d)
	}
	m.mu.Lock()
	pending := m.upCancel != nil
	m.mu.Unlock()
	if u, _ := m.getAuth(); u != "" || pending {
		t.Errorf("停止后应清空登录链接并结束 up 协程: %q %v", u, pending)
	}
}

// TestUpBackoffDoubles 验证 up 持续失败时重试间隔逐次加倍。
func TestUpBackoffDoubles(t *testing.T) {
	old := upRetryMin
	upRetryMin = 100 * time.Millisecond
	defer func() { upRetryMin = old }()
	m, dir := newTestModule(t)
	writeScript(t, dir, `case " $* " in
*" status --json "*) echo '{"daemonStatus":"Idle","peers":{"details":[]}}' ;;
*" up "*) echo up >> "$D/calls"; echo "management unreachable"; exit 1 ;;
esac
`)
	m.settings.Enable, m.settings.ManageDaemon = true, false
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop() })
	for _, want := range []string{"100ms 后重试", "200ms 后重试", "400ms 后重试"} {
		waitFor(t, want, func() bool { return strings.Contains(logText(m), want) })
	}
	if e := m.getErr(); !strings.Contains(e, "management unreachable") {
		t.Errorf("错误应带 netbird 输出: %q", e)
	}
}

// TestLogout 验证退出登录接口带上控制地址调用 netbird logout，失败时返回 netbird 的输出。
func TestLogout(t *testing.T) {
	m, dir := newTestModule(t)
	writeScript(t, dir, `case " $* " in
*" logout "*)
  echo "$*" >> "$D/calls"
  [ -f "$D/fail" ] && { echo "not logged in"; exit 1; }
  exit 0 ;;
esac
`)
	mux := http.NewServeMux()
	m.Routes(testRouter{mux})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/logout", nil))
	if rec.Code != 200 {
		t.Fatalf("退出登录失败: %d %s", rec.Code, rec.Body.String())
	}
	waitFile(t, dir, "calls", "--daemon-addr=unix://"+filepath.Join(dir, "netbird", "netbird.sock")+" logout")

	_ = os.WriteFile(filepath.Join(dir, "fail"), nil, 0o600)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/logout", nil))
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "not logged in") {
		t.Errorf("失败时应返回 netbird 的输出: %d %s", rec.Code, rec.Body.String())
	}
}

// TestSummaryConnected 验证已连接时总览显示在线节点数与节点总数。
func TestSummaryConnected(t *testing.T) {
	m, dir := newTestModule(t)
	writeScript(t, dir, `echo '{"daemonStatus":"Connected","netbirdIp":"100.92.0.1/16","peers":{"total":2,"connected":1,"details":[
{"fqdn":"a.netbird.cloud.","netbirdIp":"100.92.0.2","status":"Idle"},{"fqdn":"b.netbird.cloud.","netbirdIp":"100.92.0.3","status":"Connected","connectionType":"Relayed","relayAddress":"rels://r"}]}}'
`)
	m.settings.Enable, m.settings.ManageDaemon = true, false
	got := m.Summary()
	if len(got) != 2 || got[0].Value != "已连接" || got[1].Value != "1/2" {
		t.Fatalf("摘要错误: %+v", got)
	}
	v := m.status()
	// 在线节点排在前面，中继连接带中继地址，FQDN 去掉末尾的点
	if len(v.Peers) != 2 || v.Peers[0].Name != "b" || v.Peers[0].Relay != "rels://r" || v.Peers[1].FQDN != "a.netbird.cloud" {
		t.Errorf("节点排序或视图错误: %+v", v.Peers)
	}
	if v.Self == nil || v.Self.IPs[0] != "100.92.0.1" {
		t.Errorf("本机信息错误: %+v", v.Self)
	}
}

// TestOldVersionWarnsPublicSocks 验证托管守护进程使用用户态网络且 NetBird 版本过旧时在日志中提示 SOCKS5 对外暴露。
func TestOldVersionWarnsPublicSocks(t *testing.T) {
	m, dir := newTestModule(t)
	writeScript(t, dir, `case " $* " in
*" version "*) echo 0.70.0 ;;
*" service run "*) exec sleep 30 ;;
*" status --json "*) echo '{"daemonStatus":"Idle","peers":{"details":[]}}' ;;
esac
`)
	m.settings.Enable = true
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop() })
	if logs := logText(m); !strings.Contains(logs, "0.0.0.0") {
		t.Errorf("旧版本应提示 SOCKS5 对外暴露:\n%s", logs)
	}
}
