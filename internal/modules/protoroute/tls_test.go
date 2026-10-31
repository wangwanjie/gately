package protoroute

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"
)

// captureClientHello 让 crypto/tls 客户端发出一个真实的 ClientHello 并返回原始字节。
func captureClientHello(t *testing.T, cfg *tls.Config) []byte {
	t.Helper()
	client, server := net.Pipe()
	defer server.Close()
	go func() {
		defer client.Close()
		_ = tls.Client(client, cfg).Handshake()
	}()
	_ = server.SetReadDeadline(time.Now().Add(3 * time.Second))
	raw, _, err := peekClientHello(server)
	if err != nil {
		t.Fatalf("读取 ClientHello 失败: %v", err)
	}
	return raw
}

func TestPeekClientHello(t *testing.T) {
	raw := captureClientHello(t, &tls.Config{ServerName: "VPN.Example.COM", NextProtos: []string{"h2", "http/1.1"}, InsecureSkipVerify: true})
	got, hello, err := peekClientHello(bytes.NewReader(append(append([]byte{}, raw...), "trailing"...)))
	if err != nil {
		t.Fatal(err)
	}
	if hello.ServerName != "vpn.example.com" {
		t.Fatalf("SNI = %q", hello.ServerName)
	}
	if strings.Join(hello.ALPN, ",") != "h2,http/1.1" {
		t.Fatalf("ALPN = %v", hello.ALPN)
	}
	// 只读到 ClientHello 结束，不能吞掉后续数据
	if !bytes.Equal(got, raw) {
		t.Fatalf("预读字节数 %d，期望 %d", len(got), len(raw))
	}

	// 不带 SNI（例如直接用 IP 访问）
	raw = captureClientHello(t, &tls.Config{InsecureSkipVerify: true})
	if _, hello, err = peekClientHello(bytes.NewReader(raw)); err != nil || hello.ServerName != "" {
		t.Fatalf("无 SNI: %q %v", hello.ServerName, err)
	}
}

// ClientHello 被拆成多条 TLS 记录时也要能拼接解析
func TestPeekClientHelloFragmented(t *testing.T) {
	raw := captureClientHello(t, &tls.Config{ServerName: "a.example.com", InsecureSkipVerify: true})
	msg := raw[tlsRecordHeaderLen:]
	var split []byte
	for len(msg) > 0 {
		n := min(len(msg), 100)
		hdr := []byte{0x16, 3, 1, 0, 0}
		binary.BigEndian.PutUint16(hdr[3:], uint16(n))
		split = append(split, hdr...)
		split = append(split, msg[:n]...)
		msg = msg[n:]
	}
	got, hello, err := peekClientHello(bytes.NewReader(split))
	if err != nil || hello.ServerName != "a.example.com" {
		t.Fatalf("分片解析失败: %q %v", hello.ServerName, err)
	}
	if !bytes.Equal(got, split) {
		t.Fatal("分片时预读字节应为全部记录")
	}
}

func TestPeekClientHelloNotTLS(t *testing.T) {
	got, _, err := peekClientHello(strings.NewReader("SSH-2.0-OpenSSH\r\n"))
	if err != errNotTLS || string(got) != "S" {
		t.Fatalf("非 TLS 应立即返回且只读 1 字节: %q %v", got, err)
	}
	// 截断的数据不能 panic
	raw := captureClientHello(t, &tls.Config{ServerName: "x.example.com", InsecureSkipVerify: true})
	for i := 0; i < len(raw); i += 7 {
		_, _, _ = peekClientHello(bytes.NewReader(raw[:i]))
	}
	// 篡改长度字段不能 panic
	for i := tlsRecordHeaderLen; i < len(raw); i++ {
		bad := append([]byte{}, raw...)
		bad[i] ^= 0xff
		_, _, _ = peekClientHello(bytes.NewReader(bad))
	}
}
