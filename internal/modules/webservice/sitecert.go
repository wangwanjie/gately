package webservice

import (
	"crypto/tls"
	"crypto/x509"
	"time"
)

// SiteCert 是 HTTPS 站点某个域名匹配到的证书，供管理界面在站点行显示证书与剩余天数、缺证书时提示申请。
// 匹配方式与 HTTPS 握手时选择证书、网络自检检查证书覆盖相同（certCheckName + 证书模块的 GetCertificate），不另做一套规则。
type SiteCert struct {
	Domain string `json:"domain"`
	// Name 为匹配到的证书的主名称（证书的第一个 DNS 名称或 CN），没有匹配的证书时为空
	Name string `json:"name,omitempty"`
	// NotAfter 为匹配到的证书的到期时间；没有匹配的证书时为零值（不输出）
	NotAfter time.Time `json:"notAfter,omitzero"`
}

// siteCertsLocked 返回 HTTPS 监听中各站点域名的证书匹配结果，键为站点 key；非 HTTPS 监听或没有证书模块时为 nil。
// 不能匹配证书的域名写法（如兜底的 *）跳过。调用方持有 mu。
func (m *Module) siteCertsLocked(l *Listener) map[string][]SiteCert {
	if m.certs == nil || !l.TLS {
		return nil
	}
	out := map[string][]SiteCert{}
	for _, s := range l.Sites {
		var list []SiteCert
		for _, d := range s.Domains {
			name, ok := certCheckName(d)
			if !ok {
				continue
			}
			sc := SiteCert{Domain: d}
			if c, err := m.certs.GetCertificate(&tls.ClientHelloInfo{ServerName: name}); err == nil && c != nil {
				sc.Name, sc.NotAfter = certSummary(c, d)
			}
			list = append(list, sc)
		}
		if len(list) > 0 {
			out[s.Key] = list
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// certSummary 返回证书的主名称与到期时间；证书解析失败时主名称用站点域名代替，到期时间为零值。
func certSummary(c *tls.Certificate, domain string) (string, time.Time) {
	leaf := c.Leaf
	if leaf == nil && len(c.Certificate) > 0 {
		leaf, _ = x509.ParseCertificate(c.Certificate[0])
	}
	if leaf == nil {
		return domain, time.Time{}
	}
	if len(leaf.DNSNames) > 0 {
		return leaf.DNSNames[0], leaf.NotAfter
	}
	if leaf.Subject.CommonName != "" {
		return leaf.Subject.CommonName, leaf.NotAfter
	}
	return domain, leaf.NotAfter
}
