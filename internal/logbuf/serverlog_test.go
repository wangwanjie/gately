package logbuf

import (
	"strings"
	"testing"
	"time"
)

func TestServerErrorLog(t *testing.T) {
	b := New(100, nil)
	lg := b.Logger("site").ServerErrorLog()
	for i := 0; i < 5; i++ {
		lg.Printf(`http: TLS handshake error from [240e::1]:5268%d: 没有匹配 "nas.example.com" 的证书`, i)
	}
	lg.Printf("http: TLS handshake error from 1.2.3.4:5: EOF")
	lg.Printf("http: panic serving 1.2.3.4:5: boom")
	es := b.Since(0)
	if len(es) != 3 {
		t.Fatalf("应记录 3 条（同类握手错误节流），实际 %d: %+v", len(es), es)
	}
	if es[0].Level != LevelWarn || es[0].Source != "site" || !strings.Contains(es[0].Message, "nas.example.com") {
		t.Errorf("握手错误记录不正确: %+v", es[0])
	}
	if es[1].Level != LevelDebug {
		t.Errorf("EOF 应为调试级别: %+v", es[1])
	}
	if es[2].Level != LevelWarn {
		t.Errorf("其他错误应为警告: %+v", es[2])
	}

	// 节流期结束后输出被合并的次数
	serverLogThrottle = 0
	defer func() { serverLogThrottle = time.Minute }()
	lg.Printf(`http: TLS handshake error from [240e::1]:1: 没有匹配 "nas.example.com" 的证书`)
	last := b.Since(0)[3]
	if !strings.Contains(last.Message, "另有 4 次") {
		t.Errorf("应提示被合并的次数: %s", last.Message)
	}
}

// TestServerErrorLogClientNoise 验证访客侧原因（旧协议、无共同套件、明文请求等）记调试级别并节流，
// 对端拒绝证书等可能反映服务端问题的原因仍为警告。
func TestServerErrorLogClientNoise(t *testing.T) {
	noise := []string{
		"tls: client offered only unsupported versions: [302 301]",
		"remote error: tls: protocol version not supported",
		"tls: unsupported SSLv2 handshake received",
		"tls: no cipher suite supported by both client and server; client offered: [16 33 67]",
		"tls: no key exchanges supported by both client and server",
		"tls: no supported elliptic curves offered",
		`tls: client requested unsupported application protocols (["spdy/3" "h2c"])`,
		"client sent an HTTP request to an HTTPS server",
		"tls: first record does not look like a TLS handshake",
		"unexpected EOF",
	}
	for _, r := range noise {
		if lv, _, _ := classifyServerLog("http: TLS handshake error from 1.2.3.4:5: " + r); lv != LevelDebug {
			t.Errorf("%q 应为调试级别，实际 %s", r, lv)
		}
	}
	for _, r := range []string{"remote error: tls: bad certificate", `没有匹配 "a.test" 的证书`} {
		if lv, _, _ := classifyServerLog("http: TLS handshake error from 1.2.3.4:5: " + r); lv != LevelWarn {
			t.Errorf("%q 应为警告，实际 %s", r, lv)
		}
	}

	// 噪声同样节流：同一原因一分钟内只记一条
	b := New(100, nil)
	lg := b.Logger("site").ServerErrorLog()
	for i := 0; i < 5; i++ {
		lg.Printf("http: TLS handshake error from 1.2.3.%d:5: tls: unsupported SSLv2 handshake received", i)
	}
	if es := b.Since(0); len(es) != 1 || es[0].Level != LevelDebug {
		t.Fatalf("同类噪声应合并为一条调试日志: %+v", es)
	}
}

// TestServerErrorLogWithHook 验证调用方判定的噪声按调试级别记录，且不同原因共用一个节流键。
func TestServerErrorLogWithHook(t *testing.T) {
	b := New(100, nil)
	lg := b.Logger("site").ServerErrorLogWith(func(reason string) bool {
		return !strings.Contains(reason, "mine.test")
	})
	for i := 0; i < 5; i++ {
		lg.Printf(`http: TLS handshake error from 1.2.3.4:5: 没有匹配 "scan%d.example" 的证书`, i)
	}
	lg.Printf(`http: TLS handshake error from 1.2.3.4:5: 没有匹配 "mine.test" 的证书`)
	es := b.Since(0)
	if len(es) != 2 {
		t.Fatalf("应有 1 条噪声与 1 条警告，实际 %+v", es)
	}
	if es[0].Level != LevelDebug || es[1].Level != LevelWarn || !strings.Contains(es[1].Message, "mine.test") {
		t.Fatalf("级别不正确: %+v", es)
	}
}

// TestServerErrorLogKeepsFields 验证 net/http 错误日志带上 Logger 的附加字段（如监听器 key），按监听器过滤时不会漏掉握手错误。
func TestServerErrorLogKeepsFields(t *testing.T) {
	b := New(100, nil)
	lg := b.Logger("HTTPS").With(Fields{FieldListener: "v6key"}).ServerErrorLog()
	lg.Printf(`http: TLS handshake error from [240e::1]:5: 没有匹配 "a.test" 的证书`)
	lg.Printf("http: TLS handshake error from [240e::1]:6: EOF")
	for _, e := range b.Since(0) {
		if e.Fields[FieldListener] != "v6key" {
			t.Fatalf("应带监听器字段: %+v", e)
		}
	}
	f := &Filter{Listener: "v4key"}
	for _, e := range b.Since(0) {
		if f.Match(&e) {
			t.Fatalf("其他监听器的过滤不应匹配: %+v", e)
		}
	}
}

func TestParseTLSFailure(t *testing.T) {
	for msg, want := range map[string][3]string{
		"http: TLS handshake error from 1.2.3.4:5678: EOF":                                                           {"1.2.3.4:5678", "EOF", "ok"},
		"http: TLS handshake error from [2001:db8::1]:443: tls: no cipher suite supported by both client and server": {"[2001:db8::1]:443", "tls: no cipher suite supported by both client and server", "ok"},
		"http: TLS handshake error from 1.2.3.4:5: read tcp 10.0.0.1:443->1.2.3.4:5: i/o timeout":                    {"1.2.3.4:5", "read tcp 10.0.0.1:443->1.2.3.4:5: i/o timeout", "ok"},
		"http: Accept error: too many open files":                                                                    {"", "", ""},
		"http: TLS handshake error from nocolon":                                                                     {"", "", ""},
	} {
		addr, reason, ok := parseTLSFailure(msg)
		got := [3]string{addr, reason, ""}
		if ok {
			got[2] = "ok"
		}
		if got != want {
			t.Errorf("%q 解析为 %v，期望 %v", msg, got, want)
		}
	}
}

// TestServerErrorLogHook 验证每次握手失败都调用回调（不受节流影响），其他错误日志不调用。
func TestServerErrorLogHook(t *testing.T) {
	buf := New(100, nil)
	var calls []string
	lg := buf.Logger("t").ServerErrorLogHook(nil, func(addr, reason string) { calls = append(calls, addr+"|"+reason) })
	for i := 0; i < 5; i++ {
		lg.Printf("http: TLS handshake error from %s: %v", "1.2.3.4:9", "EOF")
	}
	lg.Printf("http: Accept error: boom")
	if len(calls) != 5 || calls[0] != "1.2.3.4:9|EOF" {
		t.Fatalf("回调次数或内容不符: %v", calls)
	}
}
