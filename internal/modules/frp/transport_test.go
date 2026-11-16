package frp

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"encoding/json"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/luckyimport"
	"github.com/wangwanjie/gately/internal/module"
)

// TestServerTOMLUDPPorts 验证 kcp / quic 监听端口：只在设置时输出，输出的字段名与 frps 一致，监听地址沿用 bindAddr。
func TestServerTOMLUDPPorts(t *testing.T) {
	got := ServerTOML(ServerSettings{BindPort: 7000, KCPBindPort: 7001, QUICBindPort: 7002, BindAddr: "127.0.0.1"})
	for _, want := range []string{"kcpBindPort = 7001", "quicBindPort = 7002", `bindAddr = "127.0.0.1"`} {
		if !strings.Contains(got, want) {
			t.Errorf("配置缺少 %s\n%s", want, got)
		}
	}
	got = ServerTOML(ServerSettings{BindPort: 7000})
	if strings.Contains(got, "kcpBindPort") || strings.Contains(got, "quicBindPort") {
		t.Errorf("未设置时不应输出 UDP 端口\n%s", got)
	}
}

func TestServerValidateUDPPorts(t *testing.T) {
	ok := []ServerSettings{
		{},
		// kcp 是 UDP，可与 TCP 的 bindPort 同号
		{BindPort: 7000, KCPBindPort: 7000, QUICBindPort: 7001},
		{KCPBindPort: 65535},
		{QUICBindPort: 1},
	}
	for _, s := range ok {
		if err := s.Validate(); err != nil {
			t.Errorf("%+v 应通过: %v", s, err)
		}
	}
	bad := []ServerSettings{
		{KCPBindPort: -1},
		{KCPBindPort: 65536},
		{QUICBindPort: -5},
		{QUICBindPort: 70000},
		// 两个 UDP 监听不能同号
		{KCPBindPort: 7001, QUICBindPort: 7001},
	}
	for _, s := range bad {
		if err := s.Validate(); err == nil {
			t.Errorf("%+v 应被拒绝", s)
		}
	}
}

func TestImportServerUDPPorts(t *testing.T) {
	// 生成的配置可原样导回
	s := ServerSettings{BindPort: 7000, KCPBindPort: 7001, QUICBindPort: 7002, DashboardAddr: "127.0.0.1"}
	res, err := ParseImport(RoleServer, ServerTOML(s))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*res.Server, s) {
		t.Errorf("回导不一致: %+v", *res.Server)
	}
	for _, w := range res.Warnings {
		if strings.Contains(strings.ToLower(w), "bind") {
			t.Errorf("已处理的字段不应提示: %v", res.Warnings)
		}
	}
	// 旧版 INI 字段名
	res, err = ParseImport(RoleServer, "[common]\nbind_port = 7000\nkcp_bind_port = 7000\nquic_bind_port = 7443\n")
	if err != nil {
		t.Fatal(err)
	}
	if res.Server.KCPBindPort != 7000 || res.Server.QUICBindPort != 7443 {
		t.Errorf("旧版字段未导入: %+v", *res.Server)
	}
}

// TestRealFrpsKCPAndQUIC 用真实 frps / frpc 验证 kcp 与 quic 连接：frpc 通过对应协议登录并转发一次数据，
// 同时确认登录检查插件拿到的来源地址与令牌校验在 UDP 传输下同样生效。
// 设置 GATELY_TEST_FRPS 与 GATELY_TEST_FRPC 时运行。
func TestRealFrpsKCPAndQUIC(t *testing.T) {
	frps, frpc := os.Getenv("GATELY_TEST_FRPS"), os.Getenv("GATELY_TEST_FRPC")
	if frps == "" || frpc == "" {
		t.Skip("未设置 GATELY_TEST_FRPS / GATELY_TEST_FRPC")
	}
	freeTCP := func() int {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		return ln.Addr().(*net.TCPAddr).Port
	}
	freeUDP := func() int {
		pc, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer pc.Close()
		return pc.LocalAddr().(*net.UDPAddr).Port
	}
	// 本地回显服务，frpc 的代理指向它
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		for {
			c, err := echo.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	echoPort := echo.Addr().(*net.TCPAddr).Port

	const token = "udp-token"
	s := ServerSettings{BindAddr: "127.0.0.1", BindPort: freeTCP(), KCPBindPort: freeUDP(), QUICBindPort: freeUDP(), Token: token,
		QUICTuning: QUICTuning{QUICKeepalivePeriod: 5, QUICMaxIdleTimeout: 20, QUICMaxIncomingStreams: 500}}
	for s.QUICBindPort == s.KCPBindPort {
		s.QUICBindPort = freeUDP()
	}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	m := newAccessModule(Server{Key: "real-udp", Name: "real-udp", ServerSettings: s})
	gate, err := startLoginGate(m)
	if err != nil {
		t.Fatal(err)
	}
	defer gate.close()
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	start := func(bin, cfg string) *exec.Cmd {
		cmd := exec.Command(bin, "-c", cfg)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		return cmd
	}
	start(frps, write("s.toml", ServerTOML(s)+gate.pluginTOML("real-udp")))
	time.Sleep(500 * time.Millisecond)

	try := func(protocol string, port int, tok string) (int, bool) {
		remote := freeTCP()
		// quic 连接带上非默认的调节参数，确认 frps 与 frpc 都能解析并正常工作
		tuning := ""
		if protocol == "quic" {
			var tb strings.Builder
			s.QUICTuning.toml(&tb)
			tuning = tb.String()
		}
		cfg := fmt.Sprintf("serverAddr = \"127.0.0.1\"\nserverPort = %d\nloginFailExit = false\ntransport.protocol = %q\nauth.method = \"token\"\nauth.token = %q\n"+tuning+"[[proxies]]\nname = %q\ntype = \"tcp\"\nlocalIP = \"127.0.0.1\"\nlocalPort = %d\nremotePort = %d\n",
			port, protocol, tok, "p-"+protocol, echoPort, remote)
		cmd := start(frpc, write(protocol+"-"+tok+".toml", cfg))
		defer func() { _ = cmd.Process.Kill() }()
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			c, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(remote)), time.Second)
			if err == nil {
				_ = c.SetDeadline(time.Now().Add(2 * time.Second))
				if _, err := c.Write([]byte("ping")); err == nil {
					buf := make([]byte, 4)
					if _, err := io.ReadFull(c, buf); err == nil && string(buf) == "ping" {
						c.Close()
						return remote, true
					}
				}
				c.Close()
			}
			time.Sleep(200 * time.Millisecond)
		}
		return remote, false
	}
	if _, ok := try("kcp", s.KCPBindPort, token); !ok {
		t.Error("frpc 通过 kcp 连接后应能转发数据")
	}
	if _, ok := try("quic", s.QUICBindPort, token); !ok {
		t.Error("frpc 通过 quic 连接后应能转发数据")
	}
	// 令牌错误：UDP 传输下同样被拒绝
	if _, ok := try("kcp", s.KCPBindPort, "wrong"); ok {
		t.Error("令牌错误的 frpc 通过 kcp 不应连接成功")
	}
	if _, ok := try("quic", s.QUICBindPort, "wrong"); ok {
		t.Error("令牌错误的 frpc 通过 quic 不应连接成功")
	}
}

// TestImportLuckyFRPUDPPorts 验证 Lucky 的 KCP / QUIC 监听端口随实例导入，端口无效或相同时只丢弃对应项。
func TestImportLuckyFRPUDPPorts(t *testing.T) {
	cases := []struct {
		params    string
		kcp, quic int
		warn      string
	}{
		{`"KCPBindPort":7001,"QUICBindPort":7002`, 7001, 7002, ""},
		{`"KCPBindPort":7000`, 7000, 0, ""},
		{`"KCPBindPort":7001,"QUICBindPort":7001`, 7001, 0, "端口相同"},
		{`"KCPBindPort":70000,"QUICBindPort":7002`, 0, 7002, "KCP 监听端口无效"},
		{`"QUICBindPort":-1`, 0, 0, "QUIC 监听端口无效"},
	}
	for _, c := range cases {
		st, _ := config.Open(t.TempDir())
		m := New()
		if err := m.Init(module.Env{Store: st, Logs: logbuf.New(20, log.New(io.Discard, "", 0))}); err != nil {
			t.Fatal(err)
		}
		raw := `{"List":[{"Key":"s1","Remark":"srv","Enable":false,"Type":"server","Params":{"BindPort":7100,` + c.params + `}}]}`
		var doc luckyimport.Document
		if err := json.Unmarshal([]byte(raw), &doc); err != nil {
			t.Fatal(err)
		}
		rep, err := m.ImportLucky(doc, luckyimport.Options{})
		if err != nil || rep == nil || len(m.cfg.Servers) != 1 {
			t.Fatalf("%s: 导入失败 %v %v", c.params, rep, err)
		}
		s := m.cfg.Servers[0]
		if s.KCPBindPort != c.kcp || s.QUICBindPort != c.quic {
			t.Errorf("%s: 得到 kcp=%d quic=%d", c.params, s.KCPBindPort, s.QUICBindPort)
		}
		joined := strings.Join(rep.Warnings, "\n")
		if c.warn != "" && !strings.Contains(joined, c.warn) || strings.Contains(joined, "暂不支持") {
			t.Errorf("%s: 提示不符: %s", c.params, joined)
		}
	}
}

func TestQUICTuningValidate(t *testing.T) {
	ok := []QUICTuning{
		{},
		{QUICKeepalivePeriod: 5, QUICMaxIdleTimeout: 20, QUICMaxIncomingStreams: 1000},
		// 只改一项时按另一项的 frp 默认值判断：保活 10、空闲 30
		{QUICKeepalivePeriod: 29},
		{QUICMaxIdleTimeout: 11},
		{QUICMaxIncomingStreams: maxQUICStreamLimit},
	}
	for _, q := range ok {
		if err := q.Validate(); err != nil {
			t.Errorf("%+v 应通过: %v", q, err)
		}
	}
	bad := []QUICTuning{
		{QUICKeepalivePeriod: -1},
		{QUICKeepalivePeriod: maxQUICSeconds + 1},
		{QUICMaxIdleTimeout: -1},
		{QUICMaxIdleTimeout: maxQUICSeconds + 1},
		{QUICMaxIncomingStreams: -1},
		{QUICMaxIncomingStreams: maxQUICStreamLimit + 1},
		// 保活不小于空闲时间会让连接反复断开
		{QUICKeepalivePeriod: 30},
		{QUICMaxIdleTimeout: 10},
		{QUICKeepalivePeriod: 20, QUICMaxIdleTimeout: 20},
	}
	for _, q := range bad {
		if err := q.Validate(); err == nil {
			t.Errorf("%+v 应被拒绝", q)
		}
	}
	// 服务端与客户端的校验都生效
	if err := (&ServerSettings{QUICTuning: QUICTuning{QUICKeepalivePeriod: 30}}).Validate(); err == nil {
		t.Error("frps 应校验 quic 参数")
	}
	if err := (&ClientSettings{ServerPort: 7000, QUICTuning: QUICTuning{QUICMaxIdleTimeout: 5}}).Validate(); err == nil {
		t.Error("frpc 应校验 quic 参数")
	}
}

func TestQUICTuningTOML(t *testing.T) {
	q := QUICTuning{QUICKeepalivePeriod: 5, QUICMaxIdleTimeout: 20, QUICMaxIncomingStreams: 1000}
	srv := ServerTOML(ServerSettings{BindPort: 7000, QUICBindPort: 7001, QUICTuning: q})
	for _, want := range []string{"transport.quic.keepalivePeriod = 5", "transport.quic.maxIdleTimeout = 20", "transport.quic.maxIncomingStreams = 1000"} {
		if !strings.Contains(srv, want) {
			t.Errorf("frps 配置缺少 %s\n%s", want, srv)
		}
	}
	// 没有开启 quic 监听时不输出调节参数
	if got := ServerTOML(ServerSettings{BindPort: 7000, QUICTuning: q}); strings.Contains(got, "quic") {
		t.Errorf("未开启 quic 监听时不应输出\n%s", got)
	}
	// 默认值不输出
	if got := ServerTOML(ServerSettings{BindPort: 7000, QUICBindPort: 7001}); strings.Contains(got, "transport.quic") {
		t.Errorf("未设置时不应输出\n%s", got)
	}
	cli := ClientTOML(ClientSettings{ServerAddr: "a", ServerPort: 7001, Protocol: "quic", QUICTuning: q}, nil)
	if !strings.Contains(cli, "transport.quic.maxIdleTimeout = 20") {
		t.Errorf("frpc 配置缺少 quic 参数\n%s", cli)
	}
	// 协议不是 quic 时不输出
	if got := ClientTOML(ClientSettings{ServerAddr: "a", ServerPort: 7000, QUICTuning: q}, nil); strings.Contains(got, "quic") {
		t.Errorf("协议不是 quic 时不应输出\n%s", got)
	}
}

func TestImportQUICTuning(t *testing.T) {
	q := QUICTuning{QUICKeepalivePeriod: 5, QUICMaxIdleTimeout: 20, QUICMaxIncomingStreams: 1000}
	s := ServerSettings{BindPort: 7000, QUICBindPort: 7001, QUICTuning: q, DashboardAddr: "127.0.0.1"}
	res, err := ParseImport(RoleServer, ServerTOML(s))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*res.Server, s) || len(res.Warnings) != 0 {
		t.Errorf("frps 回导不一致: %+v %v", *res.Server, res.Warnings)
	}
	c := ClientSettings{ServerAddr: "a.example.com", ServerPort: 7001, Protocol: "quic", QUICTuning: q}
	cres, err := ParseImport(RoleClient, ClientTOML(c, nil))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*cres.Client, c) || len(cres.Warnings) != 0 {
		t.Errorf("frpc 回导不一致: %+v %v", *cres.Client, cres.Warnings)
	}
}
