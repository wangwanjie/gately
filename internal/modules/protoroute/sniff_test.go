package protoroute

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"errors"
	mrand "math/rand"
	"net"
	"strings"
	"testing"
	"time"
)

func mustHex(t testing.TB, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// openVPNReset 构造 TCP 模式 OpenVPN 客户端的硬重置包：2 字节长度 + 操作码字节 + 会话 ID + 其余载荷。
func openVPNReset(opcode byte, extra int) []byte {
	body := []byte{opcode << 3}
	body = append(body, 1, 2, 3, 4, 5, 6, 7, 8)   // 会话 ID
	body = append(body, make([]byte, 5+extra)...) // ACK 数组长度 + 包 ID，以及 tls-auth/tls-crypt 的附加数据
	out := binary.BigEndian.AppendUint16(nil, uint16(len(body)))
	return append(out, body...)
}

// pgStartup 构造 PostgreSQL 3.0 启动消息。
func pgStartup(params ...string) []byte {
	body := binary.BigEndian.AppendUint32(nil, pgProtocol3)
	for _, p := range params {
		body = append(body, p...)
		body = append(body, 0)
	}
	body = append(body, 0)
	return append(binary.BigEndian.AppendUint32(nil, uint32(len(body)+4)), body...)
}

// protocolSamples 返回各协议真实客户端的首包样本。TLS 用 crypto/tls 现场生成。
func protocolSamples(t *testing.T) map[string][][]byte {
	return map[string][][]byte{
		ProtoTLS: {
			captureClientHello(t, &tls.Config{ServerName: "a.example.com", InsecureSkipVerify: true}),
			captureClientHello(t, &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}}),
			// TLS 1.0 记录层版本，老客户端常见
			mustHex(t, "16 03 01 00 05 01 00 00 01 00"),
		},
		ProtoHTTP: {
			[]byte("GET / HTTP/1.1\r\nHost: a.example.com\r\n\r\n"),
			[]byte("POST /api HTTP/1.1\r\nHost: b\r\nContent-Length: 2\r\n\r\nhi"),
			[]byte("PROPFIND /dav/ HTTP/1.1\r\nHost: nas\r\nDepth: 1\r\n\r\n"),
			[]byte("CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\n\r\n"),
			[]byte("OPTIONS * HTTP/1.1\r\nHost: x\r\n\r\n"),
			[]byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"),
		},
		ProtoSSH: {
			[]byte("SSH-2.0-OpenSSH_9.6\r\n"),
			[]byte("SSH-2.0-Go\r\n"),
			[]byte("SSH-1.99-PuTTY_Release_0.81\r\n"),
		},
		ProtoRDP: {
			// mstsc：TPKT + X.224 CR + Cookie + RDP_NEG_REQ
			append(append(mustHex(t, "03 00 00 2a 25 e0 00 00 00 00 00"), "Cookie: mstshash=user\r\n"...), mustHex(t, "01 00 08 00 03 00 00 00")...),
			// FreeRDP：没有 Cookie
			mustHex(t, "03 00 00 13 0e e0 00 00 00 00 00 01 00 08 00 03 00 00 00"),
		},
		ProtoSOCKS: {
			mustHex(t, "05 01 00"),
			mustHex(t, "05 02 00 02"),
			// SOCKS4 CONNECT 93.184.216.34:80，用户 ID 为空
			mustHex(t, "04 01 00 50 5d b8 d8 22 00"),
			// SOCKS4a：IP 为 0.0.0.x，用户 ID 后附域名
			append(mustHex(t, "04 01 01 bb 00 00 00 01 75 00"), "example.com\x00"...),
		},
		ProtoOpenVPN: {
			openVPNReset(7, 0),
			openVPNReset(7, 40),   // tls-auth
			openVPNReset(10, 600), // tls-crypt-v2 附带客户端密钥
		},
		ProtoXMPP: {
			[]byte("<?xml version='1.0'?><stream:stream to='example.com' xmlns='jabber:client' xmlns:stream='http://etherx.jabber.org/streams' version='1.0'>"),
			[]byte("<stream:stream xmlns='jabber:client' to='example.com' version='1.0'>"),
		},
		ProtoMQTT: {
			mustHex(t, "10 0c 00 04 4d 51 54 54 04 02 00 3c 00 00"),
			mustHex(t, "10 0e 00 06 4d 51 49 73 64 70 03 02 00 3c 00 00"),
			// 剩余长度为两字节变长编码（MQTT 5 带属性与长客户端 ID）
			append(mustHex(t, "10 80 01 00 04 4d 51 54 54 05 02 00 3c"), make([]byte, 120)...),
		},
		ProtoPostgres: {
			mustHex(t, "00 00 00 08 04 d2 16 2f"), // SSLRequest
			mustHex(t, "00 00 00 08 04 d2 16 30"), // GSSENCRequest
			pgStartup("user", "postgres", "database", "app"),
		},
		ProtoRTSP: {
			[]byte("OPTIONS rtsp://192.168.1.20:554/stream1 RTSP/1.0\r\nCSeq: 1\r\n\r\n"),
			[]byte("DESCRIBE rtsp://cam.example.com/live RTSP/1.0\r\nCSeq: 2\r\nAccept: application/sdp\r\n\r\n"),
			[]byte("SETUP rtsp://cam/live/track1 RTSP/2.0\r\n"),
		},
		ProtoSIP: {
			[]byte("REGISTER sip:pbx.example.com SIP/2.0\r\nVia: SIP/2.0/TCP 10.0.0.2:5060\r\n\r\n"),
			[]byte("INVITE sip:100@pbx.example.com SIP/2.0\r\n"),
			[]byte("OPTIONS sip:pbx.example.com SIP/2.0\r\n"),
		},
		ProtoMinecraft: {
			mcHandshake(765, "mc.example.com", 25565, 2),
			mcHandshake(47, "Play.Example.COM.", 25565, 1),
			mcHandshake(763, "mc.example.com\x00FML3\x00", 25565, 2),
			mcHandshake(767, strings.Repeat("a", 200)+".example.com", 25565, 2), // 包长度为两字节变长整数
			mustHex(t, "fe 01 fa"),                                              // 1.6 及更早的服务器列表查询
		},
		ProtoRedis: {
			[]byte("*1\r\n$4\r\nPING\r\n"),
			[]byte("*3\r\n$3\r\nSET\r\n$1\r\nk\r\n$1\r\nv\r\n"),
			[]byte("*2\r\n$4\r\nAUTH\r\n$6\r\nsecret\r\n"),
		},
		ProtoAMQP: {
			[]byte("AMQP\x00\x00\x09\x01"),
			[]byte("AMQP\x03\x01\x00\x00"),
			[]byte("AMQP\x00\x01\x00\x00"),
		},
		ProtoSMB: {
			append(mustHex(t, "00 00 00 b4 fe 53 4d 42 40 00"), make([]byte, 170)...), // SMB2 NEGOTIATE
			append(mustHex(t, "00 00 00 54 ff 53 4d 42 72 00"), make([]byte, 74)...),  // SMB1 NEGOTIATE
		},
		ProtoGit: {
			[]byte("003egit-upload-pack /project.git\x00host=git.example.com\x00"),
			[]byte("0040git-receive-pack /project.git\x00host=git.example.com\x00"),
		},
		ProtoBitTorrent: {
			append([]byte("\x13BitTorrent protocol"), make([]byte, 48)...),
		},
		ProtoADB: {
			append([]byte("CNXN\x01\x00\x00\x01\x00\x00\x10\x00"), make([]byte, 12)...),
		},
		ProtoIRC: {
			[]byte("CAP LS 302\r\nNICK alice\r\nUSER alice 0 * :Alice\r\n"),
			[]byte("NICK alice\r\n"),
			[]byte("PASS secret\r\n"),
		},
	}
}

// mcHandshake 构造 Minecraft Java 版的握手包。
func mcHandshake(version int, host string, port uint16, next int) []byte {
	varint := func(v int) []byte {
		var out []byte
		for {
			c := byte(v & 0x7F)
			v >>= 7
			if v != 0 {
				c |= 0x80
			}
			out = append(out, c)
			if v == 0 {
				return out
			}
		}
	}
	body := []byte{0x00}
	body = append(body, varint(version)...)
	body = append(body, varint(len(host))...)
	body = append(body, host...)
	body = binary.BigEndian.AppendUint16(body, port)
	body = append(body, varint(next)...)
	return append(varint(len(body)), body...)
}

// 每个协议样本：完整数据识别为该协议；逐字节递增的任一前缀要么还需更多数据，要么已识别为同一协议，
// 保证数据被拆成多个包到达时不会误判；其他协议的特征检查对它都返回否，保证各协议特征互不重叠。
func TestDetectProtocolSamples(t *testing.T) {
	for proto, samples := range protocolSamples(t) {
		for i, sample := range samples {
			got, done := detectProtocol(sample)
			if !done || got != proto {
				t.Fatalf("%s 样本 %d 识别为 %q（done=%v）", proto, i, got, done)
			}
			for n := 1; n < len(sample); n++ {
				got, done := detectProtocol(sample[:n])
				if done && got != proto {
					t.Fatalf("%s 样本 %d 前 %d 字节误判为 %q", proto, i, n, got)
				}
			}
			for _, p := range probes {
				if p.proto != proto && p.check(sample) != probeNo {
					t.Fatalf("%s 样本 %d 同时符合 %s 的特征", proto, i, p.proto)
				}
			}
		}
	}
}

// 不属于任何已知协议的数据应立即判定为无法识别，交给默认目标，不能等到超时
func TestDetectProtocolUnknown(t *testing.T) {
	samples := map[string][]byte{
		"小写方法名":              []byte("get / HTTP/1.1\r\n"),
		"普通文本":               []byte("ping"),
		"JSON":               []byte(`{"jsonrpc":"2.0"}`),
		"Redis 行内命令":         []byte("PING\r\n"),
		"RTSP 小写版本":          []byte("OPTIONS rtsp://cam rtsp/1.0\r\n"),
		"SIP 版本错误":           []byte("REGISTER sip:a SIP/3.0\r\n"),
		"未知方法":               []byte("FOO / HTTP/1.1\r\n"),
		"HTTP 方法非 HTTP 版本":   []byte("GET / FTP/1.0\r\n"),
		"AMQP 协议号错误":         []byte("AMQP\x09\x00\x00\x00"),
		"SMB 签名错误":           mustHex(t, "00 00 00 54 fe 53 4d 43"),
		"Git 未知服务":           []byte("003egit-foo-pack /x\x00"),
		"ADB 其他命令":           []byte("CNXX"),
		"IRC 小写":             []byte("nick alice\r\n"),
		"Minecraft 包 ID 非 0": []byte{0x08, 0x01, 0x80, 0x06, 0x01, 'a', 0x63, 0xDD, 0x02},
		"Minecraft 下一状态非法":   append([]byte{}, mcHandshake(765, "a.com", 25565, 9)...),
		"Minecraft 地址含控制字符":  mcHandshake(765, "a\x01b", 25565, 2),
		"非 TLS 握手":           mustHex(t, "16 01 00 00"),
		"TLS 告警":             mustHex(t, "15 03 03 00 02 02 28"),
		"SOCKS5 无方法":         mustHex(t, "05 00 00 00"),
		"SOCKS4 未知命令":        mustHex(t, "04 09 00 50"),
		"RDP 非连接请求":          mustHex(t, "03 00 00 13 0e d0 00 00"),
		"OpenVPN 数据包":        append(mustHex(t, "00 20 48"), make([]byte, 32)...),
		"OpenVPN 非零密钥":       append(mustHex(t, "00 0e 39"), make([]byte, 14)...),
		"MQTT 其他协议名":         mustHex(t, "10 0c 00 04 41 4d 51 50"),
		"Postgres 未知码":       mustHex(t, "00 00 00 08 00 00 00 01"),
		"全零":                 make([]byte, 16),
		"HTML":               []byte("<html><body>"),
		"SSH 小写":             []byte("ssh-2.0-x\r\n"),
	}
	for name, b := range samples {
		got, done := detectProtocol(b)
		if !done || got != "" {
			t.Fatalf("%s: 识别为 %q（done=%v），期望无法识别", name, got, done)
		}
	}
	// 没有任何特征的随机数据（例如 Shadowsocks、VMess）：绝大多数立即判定为无法识别，不能被识别成已知协议
	rng := mrand.New(mrand.NewSource(1))
	waiting := 0
	for i := 0; i < 20000; i++ {
		b := make([]byte, 64)
		rng.Read(b)
		got, done := detectProtocol(b)
		if done && got != "" {
			// 首字节恰好为 0x05 等少数情况可能符合特征，这里只允许极少量
			waiting++
			continue
		}
		if !done {
			waiting++
		}
	}
	if waiting > 20 {
		t.Fatalf("随机数据有 %d/20000 个被识别或需要等待", waiting)
	}
}

func FuzzDetectProtocol(f *testing.F) {
	f.Add([]byte("GET / HTTP/1.1\r\n"))
	f.Add([]byte("SSH-2.0-x"))
	f.Add([]byte{0x16, 3, 1, 0, 5})
	f.Add([]byte{0x10, 0xff, 0xff, 0xff, 0xff, 0x01})
	f.Add([]byte{0x05, 0xff})
	f.Add([]byte{0x04, 0x01, 0, 0, 0, 0, 0, 0})
	// SOCKS4 BIND 且端口高字节恰好是 OpenVPN 硬重置的操作码字节
	f.Add([]byte("\x04\x02800000"))
	f.Add([]byte{0x03, 0x00, 0x38, 0x00, 0x00, 0xe0})
	// SOCKS4 用户 ID 超过 255 字节后才出现结尾的 0
	f.Add(append(append([]byte{0x04, 0x01}, bytes.Repeat([]byte("0"), 270)...), 0))
	// 方法名重叠的请求行与各种二进制开头
	f.Add([]byte("OPTIONS rtsp://a RTSP/1.0\r\n"))
	f.Add([]byte("OPTIONS sip:a SIP/2.0\r\n"))
	f.Add(mcHandshake(765, "mc.example.com", 25565, 2))
	f.Add([]byte{0x07, 0x00, 0x38, 0x01, 'a', 0, 0, 1})
	f.Add([]byte{0x10, 0x00, 0x00, 0x04, 'M', 'Q', 'T', 'T'})
	f.Add([]byte("*1\r\n$4\r\nPING\r\n"))
	f.Add(mustHex(f, "00 00 00 54 ff 53 4d 42"))
	f.Add([]byte("003egit-upload-pack /"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) == 0 {
			return
		}
		// 每个特征检查必须单调：某个前缀已判定是或否，更长的数据结果相同，这样数据分几次到达都不会误判
		for _, p := range probes {
			full := p.check(b)
			for n := 1; n < len(b); n++ {
				if part := p.check(b[:n]); part != probeMore && part != full {
					t.Fatalf("%s: 前 %d 字节为 %d，完整数据为 %d", p.proto, n, part, full)
				}
			}
		}
		// 已识别的协议在任一前缀上也只能是同一结果
		proto, done := detectProtocol(b)
		for n := 1; n < len(b); n++ {
			if p, d := detectProtocol(b[:n]); d && (!done || p != proto) {
				t.Fatalf("前 %d 字节识别为 %q，完整数据为 %q（done=%v）", n, p, proto, done)
			}
		}
		_, _ = httpHost(b)
	})
}

// 方法名相同的请求行按末尾的版本号区分为 HTTP、RTSP、SIP；请求行超长且没有换行时只有 HTTP 方法按 HTTP 处理
func TestRequestLineProtocols(t *testing.T) {
	long := append([]byte("GET /"), bytes.Repeat([]byte("a"), maxRequestLine)...)
	cases := []struct {
		in   []byte
		want string
		done bool
	}{
		{[]byte("OPTIONS * HTTP/1.1\r\n"), ProtoHTTP, true},
		{[]byte("OPTIONS rtsp://cam/ RTSP/1.0\r\n"), ProtoRTSP, true},
		{[]byte("OPTIONS sip:pbx SIP/2.0\r\n"), ProtoSIP, true},
		{[]byte("UPDATE /x HTTP/1.1\n"), ProtoHTTP, true},
		{[]byte("UPDATE sip:a SIP/2.0\n"), ProtoSIP, true},
		{[]byte("GET /\r\n"), ProtoHTTP, true}, // HTTP/0.9 没有版本号
		{long, ProtoHTTP, true},
		{append([]byte("DESCRIBE rtsp://"), bytes.Repeat([]byte("a"), maxRequestLine)...), "", true},
		// 请求行还没读完时不能下结论
		{[]byte("OPTIONS rtsp://cam/ RTSP/1.0"), "", false},
		{[]byte("GET / HTTP/1.1"), "", false},
	}
	for _, c := range cases {
		got, done := detectProtocol(c.in)
		if got != c.want || done != c.done {
			t.Fatalf("%.40q: 识别为 (%q, %v)，期望 (%q, %v)", c.in, got, done, c.want, c.done)
		}
	}
}

func TestMinecraftHost(t *testing.T) {
	cases := map[string]string{
		"Mc.Example.COM.":                           "mc.example.com",
		"mc.example.com\x00FML3\x00":                "mc.example.com",
		"mc.example.com///203.0.113.5///1700000000": "mc.example.com",
		"203.0.113.9":                               "203.0.113.9",
	}
	for in, want := range cases {
		host, res := parseMinecraftHandshake(mcHandshake(765, in, 25565, 2))
		if res != probeYes || host != want {
			t.Fatalf("%q: 得到 (%q, %v)，期望 %q", in, host, res, want)
		}
	}
	// 包长度与字段长度对不上：多出的字节说明不是握手包
	bad := mcHandshake(765, "a.com", 25565, 2)
	bad[0]++
	bad = append(bad, 0)
	if _, res := parseMinecraftHandshake(bad); res != probeNo {
		t.Fatalf("长度不符应判否，得到 %v", res)
	}
}

func TestHTTPHost(t *testing.T) {
	cases := []struct {
		in   string
		host string
		done bool
	}{
		{"GET / HTTP/1.1\r\nHost: A.Example.COM\r\n\r\n", "a.example.com", true},
		{"GET / HTTP/1.1\r\nUser-Agent: x\r\nhost:b.example.com:8080\r\n", "b.example.com", true},
		{"GET / HTTP/1.1\r\nHost: [::1]:8080\r\n\r\n", "::1", true},
		{"GET / HTTP/1.1\r\nHost: 192.168.1.2\r\n\r\n", "192.168.1.2", true},
		{"GET / HTTP/1.1\nHost: nas.local.\n\n", "nas.local", true},
		{"GET / HTTP/1.0\r\nAccept: */*\r\n\r\n", "", true},
		{"PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n", "", true},
		{"GET / HTTP/1.1\r\nAccept: */*\r\n", "", false},
		{"GET / HTTP/1.1\r\nHo", "", false},
		{"GET /very-long", "", false},
	}
	for _, c := range cases {
		host, done := httpHost([]byte(c.in))
		if host != c.host || done != c.done {
			t.Fatalf("%q: 得到 (%q, %v)，期望 (%q, %v)", c.in, host, done, c.host, c.done)
		}
	}
}

// pipeSniff 通过 net.Pipe 让客户端按 chunks 分批写入（每次写入都会单独被读到），返回识别结果。
func pipeSniff(t *testing.T, timeout time.Duration, closeAfter bool, chunks ...[]byte) ([]byte, sniffResult, error) {
	t.Helper()
	client, server := net.Pipe()
	defer server.Close()
	go func() {
		for _, c := range chunks {
			if _, err := client.Write(c); err != nil {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		if closeAfter {
			client.Close()
		}
	}()
	defer client.Close()
	return sniffConn(server, timeout)
}

func TestSniffConn(t *testing.T) {
	// 逐字节到达的 SSH 版本标识
	var chunks [][]byte
	for _, c := range []byte("SSH-2.0-OpenSSH_9.6\r\n") {
		chunks = append(chunks, []byte{c})
	}
	raw, res, err := pipeSniff(t, time.Second, false, chunks...)
	if err != nil || res.Proto != ProtoSSH || string(raw) != "SSH-" {
		t.Fatalf("SSH: %q %+v %v", raw, res, err)
	}

	// 请求头分多次到达，Host 在第二个包里
	raw, res, err = pipeSniff(t, time.Second, false, []byte("GET /x HTTP/1.1\r\nAccept: */*\r\n"), []byte("Host: Files.Example.com\r\n\r\n"))
	if err != nil || res.Proto != ProtoHTTP || res.Host != "files.example.com" || !bytes.HasSuffix(raw, []byte("\r\n\r\n")) {
		t.Fatalf("HTTP: %q %+v %v", raw, res, err)
	}

	// 请求头发了一半就停住：超时后按没有 Host 的 HTTP 处理，已读数据完整保留
	raw, res, err = pipeSniff(t, 200*time.Millisecond, false, []byte("GET / HTTP/1.1\r\nAccept: */*\r\n"))
	if err != nil || res.Proto != ProtoHTTP || res.Host != "" || string(raw) != "GET / HTTP/1.1\r\nAccept: */*\r\n" {
		t.Fatalf("HTTP 半截: %q %+v %v", raw, res, err)
	}

	// ClientHello 与紧随其后的数据在同一个包里，后面的数据不能丢
	hello := captureClientHello(t, &tls.Config{ServerName: "a.example.com", NextProtos: []string{"h2", "http/1.1"}, InsecureSkipVerify: true})
	withTail := append(append([]byte{}, hello...), "early"...)
	raw, res, err = pipeSniff(t, time.Second, false, withTail)
	if err != nil || res.Proto != ProtoTLS || res.Host != "a.example.com" || strings.Join(res.ALPN, ",") != "h2,http/1.1" || !bytes.Equal(raw, withTail) {
		t.Fatalf("TLS: %d/%d 字节 %+v %v", len(raw), len(withTail), res, err)
	}

	// ClientHello 拆成两半分别到达
	raw, res, err = pipeSniff(t, time.Second, false, hello[:3], hello[3:100], hello[100:])
	if err != nil || res.Host != "a.example.com" || !bytes.Equal(raw, hello) {
		t.Fatalf("TLS 分片: %+v %v", res, err)
	}

	// 客户端一直不发数据：等待超时
	start := time.Now()
	raw, res, err = pipeSniff(t, 150*time.Millisecond, false)
	if err != nil || res.Proto != ProtoTimeout || len(raw) != 0 || time.Since(start) < 150*time.Millisecond {
		t.Fatalf("等待超时: %q %+v %v", raw, res, err)
	}

	// 只发了能与某个协议对上的前缀就停住：超时后按无法识别处理，不当作等待超时
	raw, res, err = pipeSniff(t, 150*time.Millisecond, false, []byte("SS"))
	if err != nil || res.Proto != "" || string(raw) != "SS" {
		t.Fatalf("半截前缀: %q %+v %v", raw, res, err)
	}

	// 未发送任何数据就断开：放弃连接
	if _, _, err = pipeSniff(t, time.Second, true); !errors.Is(err, errClientGone) {
		t.Fatalf("断开: %v", err)
	}
	// ClientHello 发到一半断开：放弃连接
	if _, _, err = pipeSniff(t, time.Second, true, hello[:50]); !errors.Is(err, errClientGone) {
		t.Fatalf("ClientHello 中途断开: %v", err)
	}
	// 发完数据后断开：按已收到的数据处理
	raw, res, err = pipeSniff(t, time.Second, true, []byte("S"))
	if err != nil || res.Proto != "" || string(raw) != "S" {
		t.Fatalf("发完即断开: %q %+v %v", raw, res, err)
	}
}
