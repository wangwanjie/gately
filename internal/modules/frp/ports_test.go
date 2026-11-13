package frp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/openports"
)

// covers 判断端口是否落在任一登记范围内
func covers(port int) bool {
	all, _ := openports.All()
	for _, r := range all {
		if r.Lo <= port && port <= r.Hi {
			return true
		}
	}
	return false
}

func postPlugin(t *testing.T, m *Module, key string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/frps/tok/"+key, bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	m.handleLogin(rec, req, key)
	return rec
}

func newProxyBody(typ string, port int) map[string]any {
	return map[string]any{"op": "NewProxy", "content": map[string]any{"proxy_name": "x", "proxy_type": typ, "remote_port": port}}
}

// frpc 申请 TCP 代理的远程端口后立即登记为开放端口；插件响应保持“不修改”，不影响代理创建
func TestNewProxyLearnsTCPPort(t *testing.T) {
	m := newAccessModule(Server{Key: "k1", Name: "k1"})
	t.Cleanup(func() { m.clearServerPorts("k1") })
	rec := postPlugin(t, m, "k1", newProxyBody("tcp", 6029))
	var resp loginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.Reject || !resp.Unchange {
		t.Fatalf("NewProxy 应原样放行: %s err=%v", rec.Body.String(), err)
	}
	if !covers(6029) {
		t.Fatal("申请的 TCP 远程端口应登记为开放端口")
	}
	// 重复申请不改变结果；其他类型（http、udp）与未指定端口（0）不登记
	postPlugin(t, m, "k1", newProxyBody("tcp", 6029))
	postPlugin(t, m, "k1", newProxyBody("http", 6040))
	postPlugin(t, m, "k1", newProxyBody("udp", 6041))
	postPlugin(t, m, "k1", newProxyBody("tcp", 0))
	postPlugin(t, m, "k1", newProxyBody("tcp", 70000))
	for _, p := range []int{6040, 6041, 70000} {
		if covers(p) {
			t.Fatalf("端口 %d 不应登记", p)
		}
	}
}

// 未知实例（已删除或伪造的路径）上的 NewProxy 不登记任何端口
func TestNewProxyUnknownServerIgnored(t *testing.T) {
	m := newAccessModule(Server{Key: "k1", Name: "k1"})
	postPlugin(t, m, "ghost", newProxyBody("tcp", 6055))
	if covers(6055) {
		t.Fatal("未知实例不应登记端口")
	}
}

// 实例停用、删除或模块停止时撤销登记，并忘记学到的端口
func TestClearServerPortsForgets(t *testing.T) {
	m := newAccessModule(Server{Key: "k2", Name: "k2"})
	t.Cleanup(func() { m.clearServerPorts("k2") })
	postPlugin(t, m, "k2", newProxyBody("tcp", 6061))
	m.publishServerPorts("k2", ServerSettings{BindPort: 7000, AllowPorts: "6100-6110"}, true)
	if !covers(6061) || !covers(7000) || !covers(6105) {
		t.Fatal("运行中应登记控制端口、允许范围与学到的代理端口")
	}
	m.publishServerPorts("k2", ServerSettings{BindPort: 7000}, false)
	if covers(6061) || covers(7000) || covers(6105) {
		t.Fatal("停用后应撤销全部登记")
	}
	// 重新运行后学到的端口已被忘记，需要 frpc 重新申请
	m.publishServerPorts("k2", ServerSettings{BindPort: 7000}, true)
	if covers(6061) {
		t.Fatal("停用时学到的端口应被忘记")
	}
}

// 单个实例记住的代理端口有上限，防止异常客户端撑大防火墙集合
func TestLearnedPortsCapped(t *testing.T) {
	m := newAccessModule(Server{Key: "k3", Name: "k3"})
	t.Cleanup(func() { m.clearServerPorts("k3") })
	for p := 1; p <= maxLearnedPorts+500; p++ {
		m.learnProxyPort("k3", p)
	}
	m.learnMu.Lock()
	n := len(m.learned["k3"])
	m.learnMu.Unlock()
	if n != maxLearnedPorts {
		t.Fatalf("记住的端口数应封顶 %d，得到 %d", maxLearnedPorts, n)
	}
}

func TestServerOpenRanges(t *testing.T) {
	got := serverOpenRanges(ServerSettings{BindPort: 7000, VhostHTTPPort: 8080, VhostHTTPSPort: 8443,
		DashboardPort: 7500, DashboardAddr: "127.0.0.1", AllowPorts: "6000-6100,7001"})
	want := []openports.Range{{Lo: 7000, Hi: 7000}, {Lo: 8080, Hi: 8080}, {Lo: 8443, Hi: 8443}, {Lo: 6000, Hi: 6100}, {Lo: 7001, Hi: 7001}}
	if !slices.Equal(got, want) {
		t.Fatalf("得到 %v，期望 %v", got, want)
	}
	// 过宽的 allowPorts 范围不登记，否则扫描检测对几乎所有端口失效
	got = serverOpenRanges(ServerSettings{BindPort: 7000, AllowPorts: "1-65535,6000-7023,8000-8100"})
	want = []openports.Range{{Lo: 7000, Hi: 7000}, {Lo: 6000, Hi: 7023}, {Lo: 8000, Hi: 8100}}
	if !slices.Equal(got, want) {
		t.Fatalf("得到 %v，期望 %v", got, want)
	}
	// 控制台对外监听时一并登记
	got = serverOpenRanges(ServerSettings{BindPort: 7000, DashboardPort: 7500, DashboardAddr: "0.0.0.0"})
	if !slices.Contains(got, openports.Range{Lo: 7500, Hi: 7500}) {
		t.Fatalf("对外监听的控制台端口应登记: %v", got)
	}
}

// 登录插件段同时声明 Login 与 NewProxy 两个操作
func TestPluginTOMLOps(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	g := &loginGate{token: "tok", ln: ln}
	toml := g.pluginTOML("k")
	if !strings.Contains(toml, `ops = ["Login", "NewProxy"]`) {
		t.Fatalf("插件段缺少操作声明:\n%s", toml)
	}
}

// 用真实 frps / frpc 验证 NewProxy 插件的请求格式：设置 GATELY_TEST_FRPS 与 GATELY_TEST_FRPC 指向两个程序时运行。
// frpc 申请远程端口后，端口应立即登记为开放端口；停用实例后撤销。
func TestRealFrpsLearnsProxyPort(t *testing.T) {
	frps, frpc := os.Getenv("GATELY_TEST_FRPS"), os.Getenv("GATELY_TEST_FRPC")
	if frps == "" || frpc == "" {
		t.Skip("未设置 GATELY_TEST_FRPS / GATELY_TEST_FRPC")
	}
	free := func() int {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		return ln.Addr().(*net.TCPAddr).Port
	}
	bind, remote := free(), free()
	m := newAccessModule(Server{Key: "real", Name: "real"})
	gate, err := startLoginGate(m)
	if err != nil {
		t.Fatal(err)
	}
	defer gate.close()
	dir := t.TempDir()
	srvCfg := ServerTOML(ServerSettings{BindPort: bind, BindAddr: "127.0.0.1"}) + gate.pluginTOML("real")
	cliCfg := fmt.Sprintf("serverAddr = \"127.0.0.1\"\nserverPort = %d\nloginFailExit = false\n[[proxies]]\nname = \"t\"\ntype = \"tcp\"\nlocalIP = \"127.0.0.1\"\nlocalPort = 9\nremotePort = %d\n", bind, remote)
	if err := os.WriteFile(filepath.Join(dir, "s.toml"), []byte(srvCfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "c.toml"), []byte(cliCfg), 0o600); err != nil {
		t.Fatal(err)
	}
	start := func(bin, cfg string) *exec.Cmd {
		cmd := exec.Command(bin, "-c", filepath.Join(dir, cfg))
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		return cmd
	}
	start(frps, "s.toml")
	time.Sleep(500 * time.Millisecond)
	start(frpc, "c.toml")
	t.Cleanup(func() { m.clearServerPorts("real") })
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if covers(remote) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !covers(remote) {
		t.Fatalf("frpc 申请的远程端口 %d 应登记为开放端口", remote)
	}
	// frps 的监听确实出现了该端口（代理创建没有被插件拦下）
	var conn net.Conn
	for i := 0; i < 50 && conn == nil; i++ {
		if c, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(remote)), time.Second); err == nil {
			conn = c
		} else {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if conn == nil {
		t.Fatal("代理远程端口没有监听，NewProxy 插件不应影响代理创建")
	}
	conn.Close()
	m.publishServerPorts("real", ServerSettings{BindPort: bind}, false)
	if covers(remote) {
		t.Fatal("停用后应撤销登记")
	}
}
