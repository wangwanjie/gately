package gen

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"time"
)

// selfSigned 生成自签证书，返回证书与私钥的 PEM。notBefore 与 notAfter 决定剩余有效期。
func selfSigned(cn string, names []string, notBefore, notAfter time.Time) (certPEM, keyPEM string) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	must(err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 100))
	must(err)
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: cn, Organization: []string{"Gately Demo"}},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, n := range names {
		if ip := net.ParseIP(n); ip != nil {
			tpl.IPAddresses = append(tpl.IPAddresses, ip)
		} else {
			tpl.DNSNames = append(tpl.DNSNames, n)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	must(err)
	kb, err := x509.MarshalECPrivateKey(key)
	must(err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}))
}

func (g *generator) ssl() error {
	day := 24 * time.Hour
	mk := func(key, remark, group, source string, names []string, issued, left time.Duration, extra obj) obj {
		crt, k := selfSigned(names[0], names, g.now.Add(-issued), g.now.Add(left))
		c := obj{
			"key": key, "remark": remark, "group": group, "enable": true, "source": source,
			"certPem": crt, "keyPem": k,
			"mapping":   obj{"enable": false, "dir": "", "script": ""},
			"updatedAt": g.ago(issued),
			"deploys":   []any{},
		}
		for k, v := range extra {
			c[k] = v
		}
		return c
	}
	acme := func(domains []string, provider, id, challenge string) obj {
		return obj{
			"domains": domains, "email": "admin@example.com", "ca": "letsencrypt", "keyType": "P256",
			"challenge": challenge, "provider": provider, "zone": "example.com",
			"credentials": obj{"id": id, "secret": "demo-not-a-real-secret"},
			"resolvers":   []string{"198.51.100.53:53"}, "propagationTimeout": 120,
		}
	}
	certs := []any{
		mk("c-wild", "example.com 泛域名证书", "公网", "acme", strs("*.example.com", "example.com"), 45*day, 45*day,
			obj{"acme": acme(strs("*.example.com", "example.com"), "cloudflare", "demo@example.com", "dns")}),
		mk("c-lab", "实验室泛域名证书", "公网", "acme", strs("*.lab.example.com"), 50*day, 40*day,
			obj{"acme": acme(strs("*.lab.example.com"), "alidns", "LTAIdemoAccessKeyId", "dns")}),
		mk("c-blog", "博客证书（HTTP 验证）", "公网", "acme", strs("blog.example.net", "www.example.net"), 20*day, 70*day,
			obj{"acme": obj{"domains": strs("blog.example.net", "www.example.net"), "email": "admin@example.com", "ca": "zerossl", "keyType": "P256",
				"challenge": "http", "httpPort": 80, "resolvers": []string{}}}),
		mk("c-intra", "内网服务证书（上传）", "内网", "upload", strs("nas.example.com", "router.example.com", "192.0.2.1"), 300*day, 65*day, obj{}),
		mk("c-old", "旧站点证书", "内网", "upload", strs("old.example.org"), 320*day, 45*day, obj{}),
	}
	return g.write("ssl", obj{
		"settings": obj{"enable": true, "renewBeforeDays": 30, "shortLivedRenewHours": 24, "defaultEmail": "admin@example.com"},
		"certs":    certs,
	})
}
