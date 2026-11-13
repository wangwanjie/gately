package webservice

import (
	"crypto/tls"
	"io"
	"log"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/webprobe"
)

// tlsFailRecorder 收集握手失败上报，按类别等待。
type tlsFailRecorder struct {
	mu  sync.Mutex
	got []webprobe.TLSFail
}

func (r *tlsFailRecorder) add(f webprobe.TLSFail) {
	r.mu.Lock()
	r.got = append(r.got, f)
	r.mu.Unlock()
}

func (r *tlsFailRecorder) count(kind string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, f := range r.got {
		if f.Kind == kind {
			n++
		}
	}
	return n
}

func (r *tlsFailRecorder) wait(t *testing.T, kind string, n int) []webprobe.TLSFail {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if r.count(kind) >= n {
			r.mu.Lock()
			defer r.mu.Unlock()
			return append([]webprobe.TLSFail(nil), r.got...)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("没有等到 %d 次 %s 握手失败上报，已收到 %d", n, kind, r.count(kind))
	return nil
}

// TestTLSFailureReported 验证真实握手失败被分类并上报给入侵防护，来源地址与监听 key 正确，
// 且每次失败都上报（不受日志节流影响）。
func TestTLSFailureReported(t *testing.T) {
	rec := &tlsFailRecorder{}
	webprobe.SetTLSHandler(rec.add)
	defer webprobe.SetTLSHandler(nil)

	logs := logbuf.New(200, log.New(io.Discard, "", 0))
	l := Listener{TLS: true, MinTLS: "1.2", Sites: []Site{{Enable: true, Domains: []string{"secure.test"}, Type: TypeText, Text: "ok"}}}
	addr, _ := startRunnerLogs(t, l, sslLikeCerts{selfSigned(t, "secure.test")}, nil, logs)

	// 连上后直接关闭：EOF
	for i := 0; i < 3; i++ {
		if c, err := net.Dial("tcp", addr); err == nil {
			c.Close()
		}
	}
	got := rec.wait(t, webprobe.TLSEOF, 3)
	for _, f := range got {
		if f.Kind == webprobe.TLSEOF && (f.Addr.String() != "127.0.0.1" || f.Reason == "") {
			t.Fatalf("上报内容不符: %+v", f)
		}
	}
	// 明文 HTTP 打 HTTPS 端口：协议错误
	if c, err := net.Dial("tcp", addr); err == nil {
		_, _ = c.Write([]byte("GET / HTTP/1.1\r\nHost: secure.test\r\n\r\n"))
		_, _ = io.ReadAll(c)
		c.Close()
	}
	rec.wait(t, webprobe.TLSProtocol, 1)
	// 过旧的协议版本：协议错误
	old := &tls.Config{InsecureSkipVerify: true, ServerName: "secure.test", MinVersion: tls.VersionTLS10, MaxVersion: tls.VersionTLS10}
	handshake(addr, old)
	rec.wait(t, webprobe.TLSProtocol, 2)
	// 陌生域名没有证书：nocert（同一原因日志只记一次，上报仍是每次）
	for i := 0; i < 3; i++ {
		handshake(addr, &tls.Config{InsecureSkipVerify: true, ServerName: "scan.example"})
	}
	rec.wait(t, webprobe.TLSNoCert, 3)
	// 站点明确填写的域名缺少证书是配置问题，归为 other，不会被「未知域名」类规则误封
	l2 := Listener{TLS: true, Sites: []Site{{Enable: true, Domains: []string{"nocert.test"}, Type: TypeText, Text: "x"}}}
	addr2, _ := startRunnerLogs(t, l2, sslLikeCerts{selfSigned(t, "secure.test")}, nil, logs)
	handshake(addr2, &tls.Config{InsecureSkipVerify: true, ServerName: "nocert.test"})
	rec.wait(t, webprobe.TLSOther, 1)
	if n := rec.count(webprobe.TLSNoCert); n != 3 {
		t.Fatalf("站点域名缺少证书不应算 nocert，nocert 共 %d 次", n)
	}
}

// TestTLSFailureNoHandler 验证没有注册处理函数时握手失败照常记日志、不崩溃。
func TestTLSFailureNoHandler(t *testing.T) {
	webprobe.SetTLSHandler(nil)
	logs := logbuf.New(200, log.New(io.Discard, "", 0))
	l := Listener{TLS: true, Sites: []Site{{Enable: true, Domains: []string{"secure.test"}, Type: TypeText, Text: "ok"}}}
	addr, _ := startRunnerLogs(t, l, sslLikeCerts{selfSigned(t, "secure.test")}, nil, logs)
	if c, err := net.Dial("tcp", addr); err == nil {
		_, _ = c.Write([]byte("GET / HTTP/1.1\r\n\r\n"))
		_, _ = io.ReadAll(c)
		c.Close()
	}
	waitLog(t, logs, "HTTP request to an HTTPS server")
}
