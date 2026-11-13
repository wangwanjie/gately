package webprobe

import "testing"

func TestClassifyTLS(t *testing.T) {
	cases := map[string]string{
		"EOF":            TLSEOF,
		"unexpected EOF": TLSEOF,
		"read tcp 1.2.3.4:443->5.6.7.8:1: read: connection reset by peer": TLSReset,
		"write tcp 1.2.3.4:443->5.6.7.8:1: write: broken pipe":            TLSReset,
		"read tcp 1.2.3.4:443->5.6.7.8:1: i/o timeout":                    TLSTimeout,
		"client sent an HTTP request to an HTTPS server":                  TLSProtocol,
		"tls: first record does not look like a TLS handshake":            TLSProtocol,
		"tls: unsupported SSLv2 handshake received":                       TLSProtocol,
		"tls: client offered only unsupported versions: [301 302]":        TLSProtocol,
		"tls: oversized record received with length 20527":                TLSProtocol,
		"tls: no cipher suite supported by both client and server":        TLSNegotiation,
		"tls: client requested unsupported application protocols ([x])":   TLSNegotiation,
		"tls: no supported elliptic curves offered":                       TLSNegotiation,
		"remote error: tls: bad certificate":                              TLSAlert,
		"remote error: tls: unknown certificate authority":                TLSAlert,
		"something new": TLSOther,
		"":              TLSOther,
	}
	for reason, want := range cases {
		if got := ClassifyTLS(reason); got != want {
			t.Errorf("%q 归为 %s，期望 %s", reason, got, want)
		}
	}
	// 每个类别常量都在 TLSKinds 中，且互不重复
	seen := map[string]bool{}
	for _, k := range TLSKinds {
		if seen[k] {
			t.Errorf("类别 %s 重复", k)
		}
		seen[k] = true
	}
}

func TestTLSHandler(t *testing.T) {
	if TLSEnabled() {
		t.Fatal("未注册时不应启用")
	}
	ReportTLS(TLSFail{Kind: TLSEOF}) // 未注册时不能崩溃
	var got []TLSFail
	SetTLSHandler(func(f TLSFail) { got = append(got, f) })
	defer SetTLSHandler(nil)
	if !TLSEnabled() {
		t.Fatal("注册后应启用")
	}
	ReportTLS(TLSFail{Kind: TLSReset, Listener: "l1"})
	if len(got) != 1 || got[0].Listener != "l1" {
		t.Fatalf("处理函数未被调用: %+v", got)
	}
	SetTLSHandler(nil)
	if TLSEnabled() {
		t.Fatal("取消后不应启用")
	}
}
