package stun

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/luckyimport"
	"github.com/wangwanjie/gately/internal/module"
	stunproto "github.com/wangwanjie/gately/internal/stun"
	"github.com/wangwanjie/gately/internal/stunaddr"
	"github.com/wangwanjie/gately/internal/webhook"
)

func TestSelectIP(t *testing.T) {
	addrs := []string{"192.168.1.5", "10.0.0.24", "100.64.3.1"}
	cases := map[string]string{
		"":         "192.168.1.5",
		"2":        "10.0.0.24",
		"100*":     "100.64.3.1",
		"*24":      "10.0.0.24",
		`^10\.0\.`: "10.0.0.24",
	}
	for rule, want := range cases {
		got, err := selectIP(addrs, rule)
		if err != nil || got != want {
			t.Errorf("selectIP(%q)=%q %v want %q", rule, got, err, want)
		}
	}
	for _, bad := range []string{"4", "0", "172*", "*99", "("} {
		if _, err := selectIP(addrs, bad); err == nil {
			t.Errorf("selectIP(%q) 应报错", bad)
		}
	}
}

func TestValidate(t *testing.T) {
	ok := Rule{Name: "a", UseGlobalServers: true, TargetAddresses: []string{"127.0.0.1"}, TargetPort: 22, StunServers: []string{"stun.example.com"}}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	if ok.Protocol != ProtoTCP4 || ok.HeartbeatInterval == 0 || ok.StunServers[0] != "stun.example.com:3478" {
		t.Fatalf("默认值补全错误: %+v", ok)
	}
	bad := []Rule{
		{UseGlobalServers: true, TargetAddresses: []string{"x"}, TargetPort: 1},
		{Name: "a", Protocol: "tcp6", UseGlobalServers: true, TargetAddresses: []string{"x"}, TargetPort: 1},
		{Name: "a", UseGlobalServers: true},
		{Name: "a", UseGlobalServers: true, DisableForward: true},
		{Name: "a", TargetAddresses: []string{"x"}, TargetPort: 1},
		{Name: "a", UseGlobalServers: true, TargetAddresses: []string{"x"}, TargetPort: 1, ScriptEnable: true},
		{Name: "a", UseGlobalServers: true, TargetAddresses: []string{"x"}, TargetPort: 1, Bind: Bind{Mode: BindIP, IP: "::1"}},
	}
	for i, r := range bad {
		if err := r.Validate(); err == nil {
			t.Errorf("第 %d 个非法规则未报错", i)
		}
	}
}

// testEnv 是端到端测试需要的本地服务：STUN 服务器、保活 HTTP 服务器、回显目标与 Webhook 接收端。
type testEnv struct {
	stun      *stunproto.Server
	keepAlive *httptest.Server
	tcpEcho   net.Listener
	udpEcho   net.PacketConn
	hook      *httptest.Server
	hookMu    sync.Mutex
	hookCalls []string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	e := &testEnv{}
	var err error
	e.stun, err = stunproto.ListenServer("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	e.keepAlive = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	e.tcpEcho, err = net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := e.tcpEcho.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	e.udpEcho, err = net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		buf := make([]byte, 2048)
		for {
			n, a, err := e.udpEcho.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = e.udpEcho.WriteTo(buf[:n], a)
		}
	}()
	e.hook = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e.hookMu.Lock()
		e.hookCalls = append(e.hookCalls, r.URL.RawQuery)
		e.hookMu.Unlock()
	}))
	t.Cleanup(func() {
		e.stun.Close()
		e.keepAlive.Close()
		e.tcpEcho.Close()
		e.udpEcho.Close()
		e.hook.Close()
	})
	return e
}

func (e *testEnv) hooks() []string {
	e.hookMu.Lock()
	defer e.hookMu.Unlock()
	return append([]string{}, e.hookCalls...)
}

func (e *testEnv) settings() Settings {
	return Settings{
		Enable:            true,
		StunServers:       []string{e.stun.Addr()},
		KeepAliveServers:  []string{strings.TrimPrefix(e.keepAlive.URL, "http://")},
		AddrCheckInterval: 1,
	}
}

func testLogger() (*logbuf.Buffer, *logbuf.Logger) {
	b := logbuf.New(500, log.New(io.Discard, "", 0))
	return b, b.Logger("t")
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("等待超时: %s", what)
}

func TestTCPTraversalEndToEnd(t *testing.T) {
	for _, bindIP := range []string{"127.0.0.1", ""} {
		t.Run("bind="+bindIP, func(t *testing.T) {
			e := newTestEnv(t)
			out := filepath.Join(t.TempDir(), "script.out")
			rule := Rule{
				Name: "nas ssh", Enable: true, Protocol: ProtoTCP4,
				Bind:              Bind{Mode: BindIP, IP: bindIP},
				UseGlobalServers:  true,
				TargetAddresses:   []string{"127.0.0.1"},
				TargetPort:        e.tcpEcho.Addr().(*net.TCPAddr).Port,
				HeartbeatInterval: 500, StunTimeout: 2000, RetryInterval: 500, AutoRetry: true,
				Webhook:             webhook.Config{Enable: true, URL: e.hook.URL + "/?a=#{ipAddr}&n=#{ruleName}&p={port}", Method: "GET"},
				WebhookOnlyOnChange: true,
				ScriptEnable:        true,
				Script:              `echo "${ruleName}|${port}|${ipAddr}" >> ` + out,
			}
			if err := rule.Validate(); err != nil {
				t.Fatal(err)
			}
			buf, lg := testLogger()
			r := newRunner(rule, e.settings(), lg)
			r.start(context.Background())
			defer r.stop()

			waitFor(t, "穿透成功", func() bool { return r.snapshot().Status == StatusOK })
			st := r.snapshot()
			pub := netip.MustParseAddrPort(st.PublicAddr)
			if pub.Port() != uint16(r.port) {
				t.Fatalf("公网端口 %d 应等于通道端口 %d", pub.Port(), r.port)
			}
			// Web 服务重定向按规则名读取的地址应与当前公网地址一致
			if got, ok := stunaddr.Lookup("nas ssh"); !ok || got != pub {
				t.Fatalf("stunaddr 中的地址应为 %v，实际 %v %v", pub, got, ok)
			}

			// 通过公网地址访问，经内置转发到达回显服务
			c, err := net.Dial("tcp4", pub.String())
			if err != nil {
				t.Fatal(err)
			}
			_, _ = c.Write([]byte("ping"))
			got := make([]byte, 4)
			_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
			if _, err := io.ReadFull(c, got); err != nil || string(got) != "ping" {
				t.Fatalf("转发失败: %q %v", got, err)
			}
			c.Close()

			waitFor(t, "Webhook", func() bool { return len(e.hooks()) == 1 })
			q, _ := url.ParseQuery(e.hooks()[0])
			if q.Get("a") != st.PublicAddr || q.Get("n") != "nas ssh" || q.Get("p") != strings.Split(st.PublicAddr, ":")[1] {
				t.Fatalf("Webhook 参数错误: %v", q)
			}
			waitFor(t, "脚本执行", func() bool {
				b, _ := os.ReadFile(out)
				return len(b) > 0
			})
			b, _ := os.ReadFile(out)
			if want := "nas ssh|" + st.PublicAddr[strings.LastIndex(st.PublicAddr, ":")+1:] + "|" + st.PublicAddr; strings.TrimSpace(string(b)) != want {
				t.Fatalf("脚本环境变量错误: %q want %q", b, want)
			}

			// 地址未变：地址检测不应重复触发 Webhook
			time.Sleep(1500 * time.Millisecond)
			if n := len(e.hooks()); n != 1 {
				t.Fatalf("地址未变化不应重复通知，实际 %d 次", n)
			}

			// 模拟 NAT 映射变化
			e.stun.SetMapAddr(func(a netip.AddrPort) netip.AddrPort { return netip.MustParseAddrPort("203.0.113.9:40000") })
			waitFor(t, "地址变化", func() bool { return r.snapshot().PublicAddr == "203.0.113.9:40000" })
			waitFor(t, "变化后 Webhook", func() bool { return len(e.hooks()) == 2 })
			if len(r.snapshot().History) != 2 {
				t.Fatalf("历史记录应有 2 条: %+v", r.snapshot().History)
			}
			if got, _ := stunaddr.Lookup("nas ssh"); got.String() != "203.0.113.9:40000" {
				t.Fatalf("地址变化后 stunaddr 应同步更新，实际 %v", got)
			}
			// 通知脚本在后台执行，等第二次写完再结束，否则 TempDir 清理时脚本可能仍在写文件
			waitFor(t, "变化后脚本执行", func() bool {
				b, _ := os.ReadFile(out)
				return strings.Count(string(b), "\n") >= 2
			})
			// 规则停止后不再提供地址
			r.stop()
			if _, ok := stunaddr.Lookup("nas ssh"); ok {
				t.Fatal("规则停止后 stunaddr 应删除地址")
			}
			_ = buf
		})
	}
}

func TestTCPCheckFailureRetries(t *testing.T) {
	e := newTestEnv(t)
	// 返回一个不可达的公网地址，有效性检测应失败并进入重试
	e.stun.SetMapAddr(func(a netip.AddrPort) netip.AddrPort { return netip.MustParseAddrPort("127.0.0.1:1") })
	rule := Rule{
		Name: "x", Protocol: ProtoTCP4, UseGlobalServers: true,
		TargetAddresses: []string{"127.0.0.1"}, TargetPort: 9,
		HeartbeatInterval: 500, StunTimeout: 1000, RetryInterval: 500, AutoRetry: true,
	}
	_ = rule.Validate()
	// 模拟上一次穿透成功留下的地址：进入重试后应被清掉，引用它的重定向返回 503 而不是跳到旧端口
	stunaddr.Set(rule.Key, rule.Name, netip.MustParseAddrPort("203.0.113.5:40000"))
	defer stunaddr.Delete(rule.Key)
	_, lg := testLogger()
	r := newRunner(rule, e.settings(), lg)
	r.start(context.Background())
	defer r.stop()
	waitFor(t, "进入重试", func() bool {
		s := r.snapshot()
		return s.Status == StatusRetrying && strings.Contains(s.Error, "有效性检测失败")
	})
	if a, ok := stunaddr.Lookup("x"); ok {
		t.Fatalf("重试期间 stunaddr 不应再提供旧地址，实际 %v", a)
	}

	// 关闭检测后同样的地址视为成功
	rule.DisableCheck = true
	r2 := newRunner(rule, e.settings(), lg)
	r2.start(context.Background())
	defer r2.stop()
	waitFor(t, "关闭检测后成功", func() bool { return r2.snapshot().Status == StatusOK })
}

func TestTCPNoRetryFails(t *testing.T) {
	e := newTestEnv(t)
	s := e.settings()
	s.StunServers = []string{"127.0.0.1:1"}
	rule := Rule{Name: "x", Protocol: ProtoTCP4, UseGlobalServers: true, TargetAddresses: []string{"127.0.0.1"}, TargetPort: 9, StunTimeout: 500, AutoRetry: false}
	_ = rule.Validate()
	_, lg := testLogger()
	r := newRunner(rule, s, lg)
	r.start(context.Background())
	defer r.stop()
	waitFor(t, "失败状态", func() bool { return r.snapshot().Status == StatusFailed })
}

func TestUDPTraversalEndToEnd(t *testing.T) {
	e := newTestEnv(t)
	rule := Rule{
		Name: "game", Protocol: ProtoUDP4, Bind: Bind{Mode: BindIP, IP: "127.0.0.1"}, UseGlobalServers: true,
		TargetAddresses: []string{"127.0.0.1"}, TargetPort: e.udpEcho.LocalAddr().(*net.UDPAddr).Port,
		HeartbeatInterval: 500, StunTimeout: 1000, RetryInterval: 500, AutoRetry: true, UDPSessionTimeout: 2,
	}
	if err := rule.Validate(); err != nil {
		t.Fatal(err)
	}
	_, lg := testLogger()
	r := newRunner(rule, e.settings(), lg)
	r.start(context.Background())
	defer r.stop()
	waitFor(t, "UDP 穿透成功", func() bool { return r.snapshot().Status == StatusOK })
	pub := r.snapshot().PublicAddr

	c, err := net.Dial("udp4", pub)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for i := 0; i < 3; i++ {
		_, _ = c.Write([]byte("hello"))
		buf := make([]byte, 64)
		_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
		n, err := c.Read(buf)
		if err != nil || !bytes.Equal(buf[:n], []byte("hello")) {
			t.Fatalf("UDP 转发失败: %q %v", buf[:n], err)
		}
	}
	// 入站字节在写给目标后才累加，回显可能先于累加到达客户端，因此等待统计到位
	waitFor(t, "UDP 统计", func() bool {
		s := r.snapshot().Stats
		return s.TotalConns == 1 && s.BytesIn == 15
	})
	// 心跳期间地址变化
	e.stun.SetMapAddr(func(a netip.AddrPort) netip.AddrPort { return netip.MustParseAddrPort("198.51.100.1:5000") })
	waitFor(t, "UDP 地址变化", func() bool { return r.snapshot().PublicAddr == "198.51.100.1:5000" })
}

func TestUPnPMappingLifecycle(t *testing.T) {
	e := newTestEnv(t)
	var mu sync.Mutex
	var actions []string
	igd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		actions = append(actions, r.Header.Get("SOAPAction")+" "+string(b))
		mu.Unlock()
		_, _ = w.Write([]byte(`<ok/>`))
	}))
	defer igd.Close()
	rule := Rule{
		Name: "upnp", Protocol: ProtoTCP4, Bind: Bind{Mode: BindIP, IP: "127.0.0.1"}, UseGlobalServers: true,
		TargetAddresses: []string{"127.0.0.1"}, TargetPort: e.tcpEcho.Addr().(*net.TCPAddr).Port,
		HeartbeatInterval: 500, StunTimeout: 1000, RetryInterval: 500, AutoRetry: true,
		UPnP: true, UPnPControlURL: igd.URL + "/ctl", UPnPLocalHost: "192.168.1.8",
	}
	_ = rule.Validate()
	_, lg := testLogger()
	r := newRunner(rule, e.settings(), lg)
	r.start(context.Background())
	waitFor(t, "穿透成功", func() bool { return r.snapshot().Status == StatusOK })
	if !strings.Contains(r.snapshot().Mapping, "UPnP TCP") {
		t.Fatalf("映射状态错误: %q", r.snapshot().Mapping)
	}
	r.stop()
	mu.Lock()
	defer mu.Unlock()
	if len(actions) < 2 || !strings.Contains(actions[0], "AddPortMapping") || !strings.Contains(actions[0], "<NewInternalClient>192.168.1.8</NewInternalClient>") ||
		!strings.Contains(actions[len(actions)-1], "DeletePortMapping") {
		t.Fatalf("UPnP 调用序列错误: %v", actions)
	}
}

func TestBehindOuterNAT(t *testing.T) {
	pub := netip.MustParseAddr("14.19.16.164")
	cases := []struct {
		gw   string
		want bool
	}{
		{"100.70.13.98", true},
		{"14.19.16.164", false},
		{"::ffff:14.19.16.164", false},
		{"0.0.0.0", false},
		{"", false},
	}
	for _, c := range cases {
		var gw netip.Addr
		if c.gw != "" {
			gw = netip.MustParseAddr(c.gw)
		}
		if got := behindOuterNAT(gw, pub); got != c.want {
			t.Errorf("behindOuterNAT(%q)=%v want %v", c.gw, got, c.want)
		}
	}
}

// 网关外网地址与公网地址不同（外层还有 CGNAT）时不按公网端口追加映射，相同时照常追加。
func TestPublicMappingOuterNAT(t *testing.T) {
	for _, tc := range []struct {
		gatewayIP string
		wantAdd   bool
	}{
		{"100.70.13.98", false},
		{"14.19.16.164", true},
	} {
		t.Run(tc.gatewayIP, func(t *testing.T) {
			var mu sync.Mutex
			var adds, queries int
			igd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				switch action := r.Header.Get("SOAPAction"); {
				case strings.Contains(action, "GetExternalIPAddress"):
					queries++
					_, _ = w.Write([]byte(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:GetExternalIPAddressResponse xmlns:u="x"><NewExternalIPAddress>` +
						tc.gatewayIP + `</NewExternalIPAddress></u:GetExternalIPAddressResponse></s:Body></s:Envelope>`))
				case strings.Contains(action, "AddPortMapping"):
					adds++
					_, _ = w.Write([]byte(`<ok/>`))
				default:
					_, _ = w.Write([]byte(`<ok/>`))
				}
			}))
			defer igd.Close()
			rule := Rule{Name: "nat", UPnP: true, UPnPControlURL: igd.URL + "/ctl", UPnPLocalHost: "192.168.1.8"}
			_, lg := testLogger()
			r := newRunner(rule, Settings{}, lg)
			r.port = 34811
			pm := &publicMapping{r: r, proto: "tcp"}
			defer pm.close()
			pub := netip.MustParseAddrPort("14.19.16.164:29847")
			pm.update(context.Background(), pub)
			// 复查得到相同地址时不再重复查询网关
			pm.update(context.Background(), pub)
			mu.Lock()
			if (adds > 0) != tc.wantAdd {
				mu.Unlock()
				t.Fatalf("追加映射次数 %d，期望追加=%v", adds, tc.wantAdd)
			}
			if queries != 1 {
				mu.Unlock()
				t.Fatalf("网关外网地址查询 %d 次，期望 1 次", queries)
			}
			mu.Unlock()
			if tc.wantAdd {
				return
			}
			// 跳过追加映射后公网 IP 变了（端口不变），要重新查询网关再判断
			pm.update(context.Background(), netip.MustParseAddrPort("14.19.16.165:29847"))
			mu.Lock()
			defer mu.Unlock()
			if queries != 2 {
				t.Fatalf("公网 IP 变化后应重新查询网关，实际共查询 %d 次", queries)
			}
		})
	}
}

func TestConvertLucky(t *testing.T) {
	raw := json.RawMessage(`[
	 {"Name":"ssh","Enable":true,"StunType":"tcp4","UseGlobalStunServerList":true,"StunServerList":["stun.a.com"],"TcpKeepAliveServerList":["qq.com"],
	  "StunListenType":"ip","ListenIP":"","ListenPort":0,"SpecifyNetworkInterface":"vmbr0","TargetAddressList":["192.168.1.2"],"TargetPort":22,
	  "UPnP":true,"UPnPGawayIP":"192.168.1.1","StunHeartbeatInterval":2300,"StunTimeout":3000,"StunRetryInterval":3000,"StunAutoRetry":true,
	  "WebhookEnable":false,"WebhookOnlyAddrChange":true,"CallScript":true,"CallScriptContent":"/etc/lucky/custom_scripts/n.sh --port \"${port}\"",
	  "Options":{"SingleProxyMaxTCPConnections":256,"UDPSessionTimeout":30000}},
	 {"Name":"bad","StunType":"tcp6"},
	 {"Name":"nic","StunType":"udp4","UseGlobalStunServerList":true,"StunListenType":"networkInterface","SpecifyNetworkInterface":"eth1","NetworkInterfaceReg":"2",
	  "TargetAddressList":["10.0.0.2"],"TargetPort":53,"Options":{"TCPListenTLS":true}}
	]`)
	rules, warns, found, err := convertLuckyRules(raw)
	if err != nil {
		t.Fatal(err)
	}
	if found != 3 || len(rules) != 2 {
		t.Fatalf("found=%d imported=%d %v", found, len(rules), warns)
	}
	a := rules[0]
	if !a.Enable || !a.UPnP || a.UPnPGateway != "192.168.1.1" || a.HeartbeatInterval != 2300 || !a.ScriptEnable || a.UDPSessionTimeout != 30 ||
		a.KeepAliveServers[0] != "qq.com:80" || a.Bind.Mode != BindIP {
		t.Fatalf("规则 1 映射错误: %+v", a)
	}
	b := rules[1]
	if b.Bind.Mode != BindInterface || b.Bind.Interface != "eth1" || b.Bind.Regex != "2" || b.Protocol != ProtoUDP4 {
		t.Fatalf("规则 2 映射错误: %+v", b)
	}
	joined := strings.Join(warns, "\n")
	for _, w := range []string{"tcp6", "/etc/lucky/", "TLS"} {
		if !strings.Contains(joined, w) {
			t.Errorf("缺少告警 %q:\n%s", w, joined)
		}
	}
}

// TestImportRealSample 用真实 lucky_stun.lkcf 做导入预演，仅在设置 GATELY_LUCKY_STUN_SAMPLE 时运行。
func TestImportRealSample(t *testing.T) {
	path := os.Getenv("GATELY_LUCKY_STUN_SAMPLE")
	if path == "" {
		t.Skip("未设置 GATELY_LUCKY_STUN_SAMPLE")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := luckyimport.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	st, _ := config.Open(t.TempDir())
	m := New()
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(10, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	rep, err := m.ImportLucky(doc, luckyimport.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("found=%d imported=%d global servers=%d", rep.Found, rep.Imported, len(m.cfg.Settings.StunServers))
	for _, w := range rep.Warnings {
		t.Log("warning:", w)
	}
	if rep.Imported != rep.Found {
		t.Fatal("有规则未能导入")
	}
}

// 存在活动转发连接时停止规则不应卡住
func TestTCPStopWithActiveConnection(t *testing.T) {
	e := newTestEnv(t)
	rule := Rule{
		Name: "hold", Enable: true, Protocol: ProtoTCP4,
		Bind: Bind{Mode: BindIP, IP: "127.0.0.1"}, UseGlobalServers: true,
		TargetAddresses: []string{"127.0.0.1"}, TargetPort: e.tcpEcho.Addr().(*net.TCPAddr).Port,
		HeartbeatInterval: 500, StunTimeout: 2000, RetryInterval: 500,
	}
	if err := rule.Validate(); err != nil {
		t.Fatal(err)
	}
	_, lg := testLogger()
	r := newRunner(rule, e.settings(), lg)
	r.start(context.Background())
	waitFor(t, "穿透成功", func() bool { return r.snapshot().Status == StatusOK })
	c, err := net.Dial("tcp4", r.snapshot().PublicAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, _ = c.Write([]byte("x"))
	waitFor(t, "连接建立", func() bool { return r.stats.active.Load() == 1 })
	done := make(chan struct{})
	go func() {
		r.stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("有活动连接时停止规则超时")
	}
}

// 路由器出站不保持端口时，应按 STUN 观察到的公网端口追加映射，结束时一并删除
func TestUPnPMapsObservedPublicPort(t *testing.T) {
	e := newTestEnv(t)
	e.stun.SetMapAddr(func(a netip.AddrPort) netip.AddrPort { return netip.AddrPortFrom(a.Addr(), 45678) })
	var mu sync.Mutex
	var actions []string
	igd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		actions = append(actions, r.Header.Get("SOAPAction")+" "+string(b))
		mu.Unlock()
		_, _ = w.Write([]byte(`<ok/>`))
	}))
	defer igd.Close()
	rule := Rule{
		Name: "nat", Protocol: ProtoTCP4, Bind: Bind{Mode: BindIP, IP: "127.0.0.1"}, UseGlobalServers: true,
		TargetAddresses: []string{"127.0.0.1"}, TargetPort: e.tcpEcho.Addr().(*net.TCPAddr).Port,
		HeartbeatInterval: 500, StunTimeout: 1000, RetryInterval: 500, AutoRetry: true, DisableCheck: true,
		UPnP: true, UPnPControlURL: igd.URL + "/ctl", UPnPLocalHost: "192.168.1.8",
	}
	_ = rule.Validate()
	_, lg := testLogger()
	r := newRunner(rule, e.settings(), lg)
	r.start(context.Background())
	waitFor(t, "穿透成功", func() bool { return r.snapshot().Status == StatusOK })
	local := r.port
	r.stop()
	mu.Lock()
	defer mu.Unlock()
	joined := strings.Join(actions, "\n")
	want := "<NewExternalPort>45678</NewExternalPort>"
	if !strings.Contains(joined, want) || !strings.Contains(joined, fmt.Sprintf("<NewInternalPort>%d</NewInternalPort>", local)) {
		t.Fatalf("应按公网端口 45678 映射到通道端口 %d: %v", local, actions)
	}
	deletes := 0
	for _, a := range actions {
		if strings.Contains(a, "DeletePortMapping") {
			deletes++
		}
	}
	if deletes < 2 {
		t.Fatalf("结束时应删除两条映射: %v", actions)
	}
}
