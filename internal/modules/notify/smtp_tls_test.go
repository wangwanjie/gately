package notify

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

// localhostCert 生成只对 localhost 有效的自签名证书，模拟自建邮件服务的默认证书。
func localhostCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// TestSMTPInsecureSkipVerify：服务器证书只对 localhost 有效时，默认校验失败，开启 insecureSkipVerify 后能发出邮件（issue #115）。
func TestSMTPInsecureSkipVerify(t *testing.T) {
	cert := localhostCert(t)
	for _, skip := range []bool{false, true} {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		tln := tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{cert}})
		mail := serveFakeSMTP(tln)
		host, port, _ := net.SplitHostPort(ln.Addr().String())
		ch := Channel{Kind: KindSMTP, Host: host, Security: SecurityTLS, From: "a@example.com", Target: "b@example.com", InsecureSkipVerify: skip}
		ch.Port, _ = strconv.Atoi(port)
		err = send(context.Background(), ch, testMsg())
		if !skip {
			if err == nil || !strings.Contains(err.Error(), "certificate") {
				t.Fatalf("证书不符时应校验失败，得到 %v", err)
			}
		} else {
			if err != nil {
				t.Fatalf("跳过证书校验后发送失败: %v", err)
			}
			select {
			case body := <-mail:
				if !strings.Contains(body, "Subject:") {
					t.Fatalf("邮件内容不完整: %q", body)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("SMTP 服务没有收到邮件")
			}
		}
		ln.Close()
	}
}
