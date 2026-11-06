package webservice

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/logbuf"
)

// sslLikeCerts 按证书模块的错误格式（没有匹配 %q 的证书）模拟证书来源，只有 secure.test 有证书。
type sslLikeCerts struct{ cert *tls.Certificate }

func (f sslLikeCerts) GetCertificate(h *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if h.ServerName == "secure.test" {
		return f.cert, nil
	}
	return nil, fmt.Errorf("没有匹配 %q 的证书", h.ServerName)
}

// handshake 以给定配置握手一次，失败是预期结果。
func handshake(addr string, cfg *tls.Config) {
	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return
	}
	defer c.Close()
	tc := tls.Client(c, cfg)
	_ = tc.SetDeadline(time.Now().Add(2 * time.Second))
	_ = tc.Handshake()
}

// waitLog 等待包含 sub 的日志出现并返回其级别（net/http 异步写错误日志）。
func waitLog(t *testing.T, logs *logbuf.Buffer, sub string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, e := range logs.Since(0) {
			if strings.Contains(e.Message, sub) {
				return e.Level
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("没有等到包含 %q 的日志", sub)
	return ""
}

// TestScannerHandshakeLogLevels 验证公网扫描器造成的握手失败只记调试级别，
// 站点中填写了域名却缺少证书时仍记警告，便于发现真正的配置问题。
func TestScannerHandshakeLogLevels(t *testing.T) {
	logs := logbuf.New(200, log.New(io.Discard, "", 0))
	l := Listener{TLS: true, MinTLS: "1.2",
		Default: Site{Enable: true, Type: TypeText, Text: "default"},
		Sites: []Site{
			{Enable: true, Domains: []string{"secure.test"}, Type: TypeText, Text: "ok"},
			{Enable: true, Domains: []string{"nocert.test"}, Type: TypeText, Text: "x"},
			{Enable: true, Domains: []string{"*.wild.test"}, Type: TypeText, Text: "x"},
			{Enable: true, Domains: []string{"*"}, Type: TypeText, Text: "any"},
			{Enable: false, Domains: []string{"off.test"}, Type: TypeText, Text: "x"},
		}}
	addr, _ := startRunnerLogs(t, l, sslLikeCerts{selfSigned(t, "secure.test")}, nil, logs)

	base := func(name string) *tls.Config {
		return &tls.Config{InsecureSkipVerify: true, ServerName: name}
	}
	// 站点填写的域名缺少证书：配置问题，警告
	handshake(addr, base("nocert.test"))
	if lv := waitLog(t, logs, `"nocert.test"`); lv != logbuf.LevelWarn {
		t.Errorf("站点域名缺少证书应为警告，实际 %s", lv)
	}
	handshake(addr, base("a.wild.test"))
	if lv := waitLog(t, logs, `"a.wild.test"`); lv != logbuf.LevelWarn {
		t.Errorf("通配站点域名缺少证书应为警告，实际 %s", lv)
	}
	// 不属于任何站点（* 站点、默认站点、停用站点不算）的域名：扫描噪声，调试
	handshake(addr, base("www.nexusmods.com"))
	if lv := waitLog(t, logs, `"www.nexusmods.com"`); lv != logbuf.LevelDebug {
		t.Errorf("陌生域名缺少证书应为调试级别，实际 %s", lv)
	}
	// 只支持 TLS 1.0 的客户端
	old := base("secure.test")
	old.MinVersion, old.MaxVersion = tls.VersionTLS10, tls.VersionTLS10
	handshake(addr, old)
	if lv := waitLog(t, logs, "unsupported versions"); lv != logbuf.LevelDebug {
		t.Errorf("协议版本过旧应为调试级别，实际 %s", lv)
	}
	// 往 HTTPS 端口发明文 HTTP
	if c, err := net.Dial("tcp", addr); err == nil {
		_, _ = c.Write([]byte("GET / HTTP/1.1\r\nHost: secure.test\r\n\r\n"))
		_, _ = io.ReadAll(c)
		c.Close()
	}
	if lv := waitLog(t, logs, "HTTP request to an HTTPS server"); lv != logbuf.LevelDebug {
		t.Errorf("明文请求 HTTPS 端口应为调试级别，实际 %s", lv)
	}
	// 正常握手不受影响
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: base("secure.test")}}
	resp, err := client.Get("https://" + addr + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// 同一分钟内的多个陌生域名合并为一条，不随域名数量增长
	before := len(logs.Since(0))
	for i := 0; i < 20; i++ {
		handshake(addr, base(fmt.Sprintf("scan%d.example", i)))
	}
	time.Sleep(200 * time.Millisecond)
	if n := len(logs.Since(0)) - before; n > 0 {
		t.Errorf("陌生域名应按同一键节流，却新增 %d 条", n)
	}
}

// TestNoSNIHandshakeLogLevel 验证不带 SNI 的握手：站点没有填写 IP 时为扫描噪声，填写了 IP 时缺少 IP 证书是配置问题。
func TestNoSNIHandshakeLogLevel(t *testing.T) {
	for _, c := range []struct {
		domains []string
		want    string
	}{
		{[]string{"secure.test"}, logbuf.LevelDebug},
		{[]string{"secure.test", "127.0.0.1"}, logbuf.LevelWarn},
	} {
		logs := logbuf.New(200, log.New(io.Discard, "", 0))
		l := Listener{TLS: true, Sites: []Site{{Enable: true, Domains: c.domains, Type: TypeText, Text: "ok"}}}
		addr, _ := startRunnerLogs(t, l, sslLikeCerts{selfSigned(t, "secure.test")}, nil, logs)
		handshake(addr, &tls.Config{InsecureSkipVerify: true})
		if lv := waitLog(t, logs, `没有匹配 "" 的证书`); lv != c.want {
			t.Errorf("站点域名 %v 时无 SNI 握手失败应为 %s，实际 %s", c.domains, c.want, lv)
		}
	}
}
