package protoroute

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/wangwanjie/gately/internal/logbuf"
	"golang.org/x/crypto/ssh"
)

// selfSignedTLS 启动一个 TLS 回显服务，握手后先写出 name 再回显，用于确认连接被分到哪个后端。
// acceptProxy 为 true 时在 TLS 之下解析 PROXY 协议，并把看到的客户端地址发到 seen。
func selfSignedTLS(t *testing.T, name string, acceptProxy bool, seen chan<- string) int {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), DNSNames: []string{"*"}}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	var ln net.Listener
	ln, err = net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	port := ln.Addr().(*net.TCPAddr).Port
	if acceptProxy {
		ln = &proxyproto.Listener{Listener: ln}
	}
	tln := tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{cert}})
	go func() {
		for {
			c, err := tln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				if err := c.(*tls.Conn).Handshake(); err != nil {
					return
				}
				if seen != nil {
					seen <- c.RemoteAddr().String()
				}
				_, _ = c.Write([]byte(name + "\n"))
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	return port
}

// tagEcho 启动一个客户端先发言的后端：收到第一段数据后回复 "name\n"，再原样回显收到的全部数据。
func tagEcho(t *testing.T, name string) int {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				buf := make([]byte, 64*1024)
				n, err := c.Read(buf)
				if err != nil {
					return
				}
				_, _ = c.Write(append([]byte(name+"\n"), buf[:n]...))
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

// bannerServer 启动一个服务端先发言的后端（模拟 VNC 的 RFB 版本号），然后回显。
func bannerServer(t *testing.T, banner string) int {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = c.Write([]byte(banner))
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

// sshServer 启动一个 x/crypto/ssh 服务端，接受任意密码，对 "ping" 全局请求回复 true 与 name。
func sshServer(t *testing.T, name string) int {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { return nil, nil }}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				sc, chans, reqs, err := ssh.NewServerConn(c, cfg)
				if err != nil {
					return
				}
				defer sc.Close()
				go ssh.DiscardRequests(nil)
				go func() {
					for ch := range chans {
						_ = ch.Reject(ssh.Prohibited, "no channels")
					}
				}()
				for req := range reqs {
					_ = req.Reply(req.Type == "ping", []byte(name))
				}
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

// httpServer 启动一个 HTTP 后端，响应体为 "name Host"。
func httpServer(t *testing.T, name string) int {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "%s %s", name, r.Host)
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { srv.Close() })
	return ln.Addr().(*net.TCPAddr).Port
}

// 端到端：真实 TLS 握手经 SNI 分流透传到不同后端，各后端的 PROXY 协议设置互不影响
func TestSNIForward(t *testing.T) {
	seenA := make(chan string, 4)
	portA := selfSignedTLS(t, "anylink", true, seenA)
	portB := selfSignedTLS(t, "nginx", false, nil)
	portDefault := selfSignedTLS(t, "ocserv", false, nil)
	listenPort := freePort(t)

	rule := Rule{
		Protocols:       []string{"tcp4"},
		ListenAddress:   "127.0.0.1",
		ListenPorts:     strconv.Itoa(listenPort),
		TargetAddresses: []string{"127.0.0.1"},
		TargetPorts:     strconv.Itoa(portDefault),
		Routes: []Route{
			{Name: "anylink", Domains: []string{"vpn.example.com"}, TargetAddresses: []string{"127.0.0.1"}, TargetPort: portA, ProxyProtocol: "v2"},
			{Name: "nginx", Domains: []string{"*.example.com"}, TargetAddresses: []string{"127.0.0.1"}, TargetPort: portB},
		},
	}
	if err := rule.Validate(); err != nil {
		t.Fatal(err)
	}
	r := testRunner(rule)
	if err := r.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer r.stop()

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort))
	dial := func(sni string) string {
		t.Helper()
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 3 * time.Second}, "tcp4", addr, &tls.Config{ServerName: sni, InsecureSkipVerify: true})
		if err != nil {
			t.Fatalf("%s 握手失败: %v", sni, err)
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		line := make([]byte, 64)
		n, err := conn.Read(line)
		if err != nil {
			t.Fatalf("%s 读取失败: %v", sni, err)
		}
		// 握手后的数据双向透传
		payload := bytes.Repeat([]byte("x"), 256*1024)
		go func() { _, _ = conn.Write(payload) }()
		echo := make([]byte, len(payload))
		if _, err := io.ReadFull(conn, echo); err != nil || !bytes.Equal(echo, payload) {
			t.Fatalf("%s 回显失败: %v", sni, err)
		}
		return strings.TrimSpace(string(line[:n]))
	}
	if got := dial("vpn.example.com"); got != "anylink" {
		t.Fatalf("vpn.example.com 分到 %s", got)
	}
	select {
	case got := <-seenA:
		if !strings.HasPrefix(got, "127.0.0.1:") {
			t.Fatalf("PROXY 协议传递的客户端地址错误: %s", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("后端未收到连接")
	}
	if got := dial("Gitea.Example.com"); got != "nginx" {
		t.Fatalf("通配域名分到 %s", got)
	}
	if got := dial("other.org"); got != "ocserv" {
		t.Fatalf("未命中应走默认目标，实际 %s", got)
	}

	// 等统计落地后检查各路由命中数
	deadline := time.Now().Add(2 * time.Second)
	for r.stats.ActiveConns.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	s := r.snapshot()
	if len(s.RouteConns) != 2 || s.RouteConns[0] != 1 || s.RouteConns[1] != 1 || s.DefaultConns != 1 || s.TotalConns != 3 {
		t.Fatalf("统计错误: %+v", s)
	}
}

// 非 TLS 客户端：先发数据的立即走默认目标；不发数据的等待超时后走默认目标；没有默认目标时断开
func TestSNIForwardNonTLS(t *testing.T) {
	old := defaultSniffTimeout
	defaultSniffTimeout = 300 * time.Millisecond
	defer func() { defaultSniffTimeout = old }()

	// 默认目标是一个先发言的服务，模拟 SSH
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = c.Write([]byte("BANNER\n"))
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	listenPort := freePort(t)
	rule := Rule{Protocols: []string{"tcp4"}, ListenAddress: "127.0.0.1", ListenPorts: strconv.Itoa(listenPort),
		TargetAddresses: []string{"127.0.0.1"}, TargetPorts: strconv.Itoa(ln.Addr().(*net.TCPAddr).Port),
		Routes: []Route{{Domains: []string{"a.example.com"}, TargetAddresses: []string{"127.0.0.1"}, TargetPort: 1}}}
	if err := rule.Validate(); err != nil {
		t.Fatal(err)
	}
	r := testRunner(rule)
	if err := r.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer r.stop()
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort))

	// 客户端先发言：首字节不是 TLS，预读的数据也要完整送达
	conn, err := net.Dial("tcp4", addr)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = conn.Write([]byte("ping"))
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, len("BANNER\nping"))
	if _, err := io.ReadFull(conn, buf); err != nil || string(buf) != "BANNER\nping" {
		t.Fatalf("非 TLS 转发失败: %q %v", buf, err)
	}
	conn.Close()

	// 服务端先发言：等待超时后转到默认目标
	conn, err = net.Dial("tcp4", addr)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf = make([]byte, len("BANNER\n"))
	if _, err := io.ReadFull(conn, buf); err != nil || string(buf) != "BANNER\n" {
		t.Fatalf("超时后应转到默认目标: %q %v", buf, err)
	}
	conn.Close()

	// 没有默认目标：未命中直接断开
	r.stop()
	rule.TargetAddresses = nil
	r = testRunner(rule)
	if err := r.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer r.stop()
	conn, err = net.Dial("tcp4", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = conn.Write([]byte("ping"))
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if n, err := conn.Read(make([]byte, 8)); err == nil {
		t.Fatalf("应断开连接，实际读到 %d 字节", n)
	}
	time.Sleep(50 * time.Millisecond)
	if r.rejected.Load() != 1 {
		t.Fatalf("拒绝计数 = %d", r.rejected.Load())
	}
}

// 端到端：一个端口上同时承载 TLS（按 SNI 与 ALPN）、HTTP（按 Host）、SSH、RDP、SOCKS、OpenVPN、XMPP、MQTT、PostgreSQL、
// 服务端先发言的协议（等待超时）与无法识别的数据，各自转到对应的后端，预读的数据完整送达
func TestProtocolForward(t *testing.T) {
	tlsWeb := selfSignedTLS(t, "tls-web", false, nil)
	tlsACME := selfSignedTLS(t, "tls-acme", false, nil)
	httpFiles := httpServer(t, "http-files")
	httpAll := httpServer(t, "http-all")
	sshPort := sshServer(t, "sshd")
	vnc := bannerServer(t, "RFB 003.008\n")
	// 客户端先发言、由带标记的回显后端验证的协议；Minecraft 另按服务器地址分到两个后端
	tagged := []string{ProtoRDP, ProtoSOCKS, ProtoOpenVPN, ProtoXMPP, ProtoMQTT, ProtoPostgres, ProtoRTSP, ProtoSIP,
		ProtoRedis, ProtoAMQP, ProtoSMB, ProtoGit, ProtoBitTorrent, ProtoADB, ProtoIRC}
	tags := map[string]int{}
	for _, name := range append(append([]string{}, tagged...), "mc-a", "mc-all", "default") {
		tags[name] = tagEcho(t, name)
	}
	listenPort := freePort(t)
	r1 := func(name, proto string, port int) Route {
		return Route{Name: name, Protocol: proto, TargetAddresses: []string{"127.0.0.1"}, TargetPort: port}
	}
	routes := []Route{
		{Name: "tls-web", Protocol: "tls", Domains: []string{"*.example.com"}, TargetAddresses: []string{"127.0.0.1"}, TargetPort: tlsWeb},
		{Name: "tls-acme", Protocol: "tls", ALPN: []string{"acme-tls/1"}, TargetAddresses: []string{"127.0.0.1"}, TargetPort: tlsACME},
		{Name: "http-files", Protocol: "http", Domains: []string{"files.example.com"}, TargetAddresses: []string{"127.0.0.1"}, TargetPort: httpFiles},
		r1("http-all", "http", httpAll),
		r1("ssh", "ssh", sshPort),
	}
	for _, proto := range tagged {
		routes = append(routes, r1(proto, proto, tags[proto]))
	}
	routes = append(routes,
		Route{Name: "mc-a", Protocol: "minecraft", Domains: []string{"a.mc.example.com"}, TargetAddresses: []string{"127.0.0.1"}, TargetPort: tags["mc-a"]},
		r1("mc-all", "minecraft", tags["mc-all"]),
		r1("vnc", "timeout", vnc),
	)
	rule := Rule{
		Protocols: []string{"tcp4"}, ListenAddress: "127.0.0.1", ListenPorts: strconv.Itoa(listenPort),
		TargetAddresses: []string{"127.0.0.1"}, TargetPorts: strconv.Itoa(tags["default"]),
		// 识别超时可以带小数
		SniffTimeout: 0.5,
		Routes:       routes,
	}
	if err := rule.Validate(); err != nil {
		t.Fatal(err)
	}
	r := testRunner(rule)
	if err := r.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer r.stop()
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort))

	// TLS：按 SNI 与 ALPN 分到不同后端
	tlsDial := func(sni string, alpn ...string) string {
		t.Helper()
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 3 * time.Second}, "tcp4", addr, &tls.Config{ServerName: sni, NextProtos: alpn, InsecureSkipVerify: true})
		if err != nil {
			t.Fatalf("TLS %s 握手失败: %v", sni, err)
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		line := make([]byte, 64)
		n, err := conn.Read(line)
		if err != nil {
			t.Fatalf("TLS %s 读取失败: %v", sni, err)
		}
		return strings.TrimSpace(string(line[:n]))
	}
	if got := tlsDial("www.example.com", "h2"); got != "tls-web" {
		t.Fatalf("TLS 通配域名分到 %s", got)
	}
	if got := tlsDial("other.org", "acme-tls/1"); got != "tls-acme" {
		t.Fatalf("TLS ALPN 分到 %s", got)
	}

	// HTTP：按 Host 分流，同一连接上的多个请求都到同一后端
	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp4", addr)
		},
	}, Timeout: 3 * time.Second}
	httpGet := func(host string) string {
		t.Helper()
		resp, err := client.Get("http://" + host + "/")
		if err != nil {
			t.Fatalf("HTTP %s: %v", host, err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return string(body)
	}
	if got := httpGet("files.example.com"); got != "http-files files.example.com" {
		t.Fatalf("HTTP Host 分到 %q", got)
	}
	if got := httpGet("other.example.com:8080"); got != "http-all other.example.com:8080" {
		t.Fatalf("HTTP 其他 Host 分到 %q", got)
	}

	// SSH：真实握手并完成一次全局请求
	sc, err := ssh.Dial("tcp4", addr, &ssh.ClientConfig{User: "u", Auth: []ssh.AuthMethod{ssh.Password("p")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 3 * time.Second})
	if err != nil {
		t.Fatalf("SSH 握手失败: %v", err)
	}
	ok, reply, err := sc.SendRequest("ping", true, nil)
	sc.Close()
	if err != nil || !ok || string(reply) != "sshd" {
		t.Fatalf("SSH 请求: %v %q %v", ok, reply, err)
	}

	// 客户端先发言的协议：后端回复自己的名称并回显首包
	samples := protocolSamples(t)
	send := func(sample []byte, backend string) {
		t.Helper()
		conn, err := net.Dial("tcp4", addr)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		if _, err := conn.Write(sample); err != nil {
			t.Fatal(err)
		}
		want := append([]byte(backend+"\n"), sample...)
		got := make([]byte, len(want))
		if _, err := io.ReadFull(conn, got); err != nil || !bytes.Equal(got, want) {
			t.Fatalf("应分到 %s，收到 %.60q，%v", backend, got, err)
		}
	}
	for _, proto := range tagged {
		for _, sample := range samples[proto] {
			send(sample, proto)
		}
	}
	// Minecraft 按握手包中的服务器地址分流，地址不匹配与旧版查询走未填域名的路由
	send(mcHandshake(765, "A.MC.example.com", 25565, 2), "mc-a")
	send(mcHandshake(765, "a.mc.example.com\x00FML3\x00", 25565, 2), "mc-a")
	send(mcHandshake(765, "other.example.com", 25565, 1), "mc-all")
	send(mustHex(t, "fe 01 fa"), "mc-all")

	// 客户端不发数据：等待规则设置的 0.5 秒后转到等待超时路由
	conn, err := net.Dial("tcp4", addr)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	banner := make([]byte, len("RFB 003.008\n"))
	if _, err := io.ReadFull(conn, banner); err != nil || string(banner) != "RFB 003.008\n" {
		t.Fatalf("等待超时路由: %q %v", banner, err)
	}
	if d := time.Since(start); d < 450*time.Millisecond || d > 2*time.Second {
		t.Fatalf("等待时长 %v，期望约 0.5 秒", d)
	}
	conn.Close()

	// 无法识别的数据：发完后半关闭，默认目标收到完整数据
	tc, err := net.DialTCP("tcp4", nil, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: listenPort})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = tc.Write([]byte("random-bytes"))
	_ = tc.CloseWrite()
	_ = tc.SetReadDeadline(time.Now().Add(3 * time.Second))
	all, _ := io.ReadAll(tc)
	tc.Close()
	if string(all) != "default\nrandom-bytes" {
		t.Fatalf("默认目标收到 %q", all)
	}

	// 各路由命中数：TLS 2 条各 1、HTTP 各 1、SSH 1、其余协议按样本数、Minecraft 两条各 2、等待超时 1，默认目标 1
	deadline := time.Now().Add(2 * time.Second)
	for r.stats.ActiveConns.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	client.CloseIdleConnections()
	s := r.snapshot()
	want := []int64{1, 1, 1, 1, 1}
	for _, proto := range tagged {
		want = append(want, int64(len(samples[proto])))
	}
	want = append(want, 2, 2, 1)
	if fmt.Sprint(s.RouteConns) != fmt.Sprint(want) || s.DefaultConns != 1 {
		t.Fatalf("路由统计 %v 默认 %d，期望 %v 默认 1", s.RouteConns, s.DefaultConns, want)
	}
}

// freePort 获取一个当前空闲的 TCP 端口。
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// testRunner 创建使用默认全局设置、连接超时 1 秒的规则运行器。
func testRunner(rule Rule) *runner {
	s := defaultSettings()
	s.DialTimeout = 1
	return newRunner(rule, s, logbuf.New(100, log.New(io.Discard, "", 0)).Logger("test"))
}

// 规则停止时，仍在等待识别的连接立即关闭，停止不必等到识别超时
func TestStopDoesNotWaitForSniff(t *testing.T) {
	old := defaultSniffTimeout
	defaultSniffTimeout = 30 * time.Second
	defer func() { defaultSniffTimeout = old }()

	listenPort := freePort(t)
	rule := Rule{Protocols: []string{"tcp4"}, ListenAddress: "127.0.0.1", ListenPorts: strconv.Itoa(listenPort),
		TargetAddresses: []string{"127.0.0.1"}, TargetPorts: "1",
		Routes: []Route{{Domains: []string{"a.example.com"}, TargetAddresses: []string{"127.0.0.1"}, TargetPort: 1}}}
	if err := rule.Validate(); err != nil {
		t.Fatal(err)
	}
	r := testRunner(rule)
	if err := r.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	// 客户端连上后什么都不发，连接停在识别阶段
	conn, err := net.Dial("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort)))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	deadline := time.Now().Add(2 * time.Second)
	for r.stats.ActiveConns.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	start := time.Now()
	r.stop()
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("停止规则等待了 %v，应立即关闭识别中的连接", d)
	}
}
