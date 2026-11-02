package ssl

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
	"github.com/miekg/dns"
	"golang.org/x/net/publicsuffix"

	"github.com/wangwanjie/gately/internal/acmehttp"
	"github.com/wangwanjie/gately/internal/dnsprovider"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// acmeUser 实现 lego 的 registration.User。
type acmeUser struct {
	email string
	key   crypto.PrivateKey
	reg   *registration.Resource
}

func (u *acmeUser) GetEmail() string                        { return u.email }
func (u *acmeUser) GetRegistration() *registration.Resource { return u.reg }
func (u *acmeUser) GetPrivateKey() crypto.PrivateKey        { return u.key }

func accountKey(dirURL, email string) string {
	h := sha256.Sum256([]byte(dirURL + "|" + strings.ToLower(email)))
	return hex.EncodeToString(h[:8])
}

// dnsSolver 把 DNS-01 验证落到 Gately 的 DNS 服务商实现上。
type dnsSolver struct {
	txt     dnsprovider.TXTProvider
	zone    string
	timeout time.Duration
	log     *logbuf.Logger
	// nameservers 是查询 SOA 使用的 DNS 服务器：证书设置了 DNS 服务器时用它，否则用系统设置
	nameservers []string
	// network 为限定的出站网络（tcp4/tcp6），留空不限定；跟随 CNAME 的查询也遵守它
	network string
	// disableCNAME 为 true 时不跟随 _acme-challenge 的 CNAME，验证记录直接写在 _acme-challenge.<域名>
	disableCNAME bool

	mu sync.Mutex
	// targets 缓存每个域名的验证记录位置，保证同一次签发中添加、生效检查与清理使用同一个位置
	targets map[string]challengeTarget
}

// challengeTarget 是验证记录实际写入的位置。
type challengeTarget struct {
	// fqdn 为验证记录的完整域名（带末尾的点）
	fqdn string
	// noFollow 表示存在 CNAME 但决定不跟随（关闭了 CNAME 跟随或 CNAME 来自泛解析），
	// 生效检查也不能再跟随递归 DNS 返回的 CNAME
	noFollow bool
}

// lookupCNAME 查询 fqdn 自身的 CNAME，返回指向的目标（小写、带末尾的点），测试中可替换。
var lookupCNAME = func(q netPreCheck, fqdn string) (string, bool) {
	r, err := q.query(fqdn, dns.TypeCNAME, q.nameservers, true)
	if err != nil || r.Rcode != dns.RcodeSuccess {
		return "", false
	}
	for _, rr := range r.Answer {
		if cn, ok := rr.(*dns.CNAME); ok && strings.EqualFold(cn.Hdr.Name, fqdn) {
			target := strings.ToLower(dns.Fqdn(cn.Target))
			return target, !strings.EqualFold(target, fqdn)
		}
	}
	return "", false
}

// target 返回 domain 的验证记录位置。domain 可以带通配符前缀（生效检查传入的是 *.example.com）。
func (s *dnsSolver) target(domain string) challengeTarget {
	domain = strings.TrimSuffix(strings.TrimPrefix(strings.ToLower(domain), "*."), ".")
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.targets[domain]; ok {
		return t
	}
	t := s.resolveTarget(domain)
	if s.targets == nil {
		s.targets = make(map[string]challengeTarget)
	}
	s.targets[domain] = t
	return t
}

// resolveTarget 按 lego 的规则跟随 _acme-challenge 的 CNAME（支持把验证委派到其他域名），
// 但第一跳的 CNAME 来自泛解析记录（*.<域名>）时不跟随：泛解析会把 _acme-challenge 也指走，
// 例如 *.example.com 指向 example.com 时验证记录会被写到 @，而 CA 查不到（issue #61）。
// 在 _acme-challenge 上写入 TXT 后该名称就真实存在，不再匹配泛解析，CA 能直接查到。
func (s *dnsSolver) resolveTarget(domain string) challengeTarget {
	base := "_acme-challenge." + domain + "."
	q := netPreCheck{network: s.network, nameservers: s.nameservers}
	first, ok := lookupCNAME(q, base)
	if !ok {
		return challengeTarget{fqdn: base}
	}
	if s.disableCNAME {
		s.infof("%s 有 CNAME 指向 %s，已设置不跟随 CNAME，验证记录直接写在 %s", base, first, base)
		return challengeTarget{fqdn: base, noFollow: true}
	}
	// 随机名称也返回同一个 CNAME，说明它是泛解析合成的结果，而不是专门为验证设置的委派
	if probe, ok := lookupCNAME(q, "_gately-"+randomLabel()+"."+domain+"."); ok && probe == first {
		s.warnf("%s 的 CNAME（指向 %s）来自泛解析记录 *.%s，不跟随，验证记录直接写在 %s", base, first, domain, base)
		return challengeTarget{fqdn: base, noFollow: true}
	}
	fqdn := first
	s.infof("%s 有 CNAME 指向 %s，验证记录写在 %s", base, first, first)
	// 与 lego 一样最多跟随 50 次，避免 CNAME 成环
	for i := 0; i < 49; i++ {
		next, ok := lookupCNAME(q, fqdn)
		if !ok {
			break
		}
		s.infof("%s 有 CNAME 指向 %s", fqdn, next)
		fqdn = next
	}
	return challengeTarget{fqdn: fqdn}
}

// randomLabel 返回用于探测泛解析的随机标签。
func randomLabel() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *dnsSolver) infof(format string, args ...any) {
	if s.log != nil {
		s.log.Infof(format, args...)
	}
}

func (s *dnsSolver) warnf(format string, args ...any) {
	if s.log != nil {
		s.log.Warnf(format, args...)
	}
}

// challengeValue 返回 DNS-01 验证记录的值：keyAuth 的 SHA-256，按无填充的 base64url 编码。
func challengeValue(keyAuth string) string {
	sum := sha256.Sum256([]byte(keyAuth))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// locate 计算验证记录所在的主域名与主机记录。
func (s *dnsSolver) locate(domain, keyAuth string) (zone, sub, value string, err error) {
	fqdn := strings.TrimSuffix(s.target(domain).fqdn, ".")
	value = challengeValue(keyAuth)
	if s.zone != "" {
		sub, ok := dnsprovider.SubForZone(fqdn, s.zone)
		if !ok {
			return "", "", "", fmt.Errorf("验证记录 %s 不属于主域名 %s", fqdn, s.zone)
		}
		return s.zone, sub, value, nil
	}
	// 未指定主域名时先按 SOA 查询验证记录所在的权威区域，能正确处理单独托管的子域名（NS 委派）与多级后缀；
	// 查询失败（例如域名尚未生效）时退回按域名段数推断
	if z, err := findZone(fqdn, s.nameservers); err == nil {
		// 本地 DNS 覆盖了上层域名时（如 dnsmasq 的 local=/example.com/），SOA 会一路查到 com 这类公共后缀；
		// 比可注册域名还短的区域不可能是用户托管的区域，丢弃并改按域名推断
		if sub, ok := dnsprovider.SubForZone(fqdn, z); ok && sub != "@" && zoneRegistrable(fqdn, z) {
			return z, sub, value, nil
		}
		if s.log != nil {
			s.log.Warnf("%s 查到的权威区域 %s 不可用，按域名推断主域名", fqdn, z)
		}
	} else if s.log != nil {
		s.log.Warnf("查询 %s 的权威区域失败，按域名推断主域名: %v", fqdn, err)
	}
	sub, zone = dnsprovider.SplitFQDN(fqdn)
	return zone, sub, value, nil
}

// zoneRegistrable 判断区域 zone 是否等于 fqdn 的可注册域名（eTLD+1）或是它的子域。
func zoneRegistrable(fqdn, zone string) bool {
	etld1, err := publicsuffix.EffectiveTLDPlusOne(strings.ToLower(fqdn))
	if err != nil {
		return false
	}
	zone = strings.ToLower(zone)
	return zone == etld1 || strings.HasSuffix(zone, "."+etld1)
}

// findZone 返回 fqdn 所在的权威区域（不带末尾的点），nameservers 为查询使用的 DNS 服务器，测试中可替换。
// lego 会缓存 SOA 结果，同一次签发的添加与清理不会重复查询
var findZone = func(fqdn string, nameservers []string) (string, error) {
	z, err := dns01.FindZoneByFqdnCustom(dns01.ToFqdn(fqdn), nameservers)
	if err != nil {
		return "", err
	}
	return strings.ToLower(strings.TrimSuffix(z, ".")), nil
}

func (s *dnsSolver) Present(domain, _, keyAuth string) error {
	zone, sub, value, err := s.locate(domain, keyAuth)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	s.log.Infof("添加验证记录 TXT %s.%s", sub, zone)
	return s.txt.AddTXT(ctx, zone, sub, value)
}

func (s *dnsSolver) CleanUp(domain, _, keyAuth string) error {
	zone, sub, value, err := s.locate(domain, keyAuth)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := s.txt.DeleteTXT(ctx, zone, sub, value); err != nil {
		s.log.Warnf("清理验证记录失败（可手动删除 TXT %s.%s）: %v", sub, zone, err)
	}
	return nil
}

// Timeout 实现 challenge.ProviderTimeout：等待 TXT 生效的超时与轮询间隔。
func (s *dnsSolver) Timeout() (time.Duration, time.Duration) {
	return s.timeout, 5 * time.Second
}

// obtainResult 是签发结果。
type obtainResult struct {
	CertPEM, KeyPEM, IssuerPEM string
	Account                    *Account
}

// acmeHTTPClient 允许测试注入信任测试 CA 的客户端。
var acmeHTTPClient func() *http.Client

// skipPropagationWait 是跳过生效检查时的固定等待时间。
var skipPropagationWait = 20 * time.Second

// obtain 使用 ACME 签发证书。acc 为已缓存账户，可为 nil；新注册的账户在结果中返回。
func obtain(cfg ACMEConfig, acc *Account, log *logbuf.Logger) (*obtainResult, error) {
	dirURL := cfg.directoryURL()
	user := &acmeUser{email: cfg.Email}
	res := &obtainResult{}
	if acc != nil && acc.KeyPEM != "" && acc.Registration != "" {
		key, err := certcrypto.ParsePEMPrivateKey([]byte(acc.KeyPEM))
		if err == nil {
			var reg registration.Resource
			if json.Unmarshal([]byte(acc.Registration), &reg) == nil {
				user.key, user.reg = key, &reg
			}
		}
	}
	if user.key == nil {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
		user.key = key
	}

	lc := lego.NewConfig(user)
	lc.CADirURL = dirURL
	lc.Certificate.KeyType = certcrypto.KeyType(cfg.KeyType)
	// IP 地址不能作为 CommonName（Let's Encrypt 的 IP 证书不带 CN），含 IP 时只用 SAN
	lc.Certificate.DisableCommonName = hasIPIdentifier(cfg.Domains)
	lc.UserAgent = "Gately"
	if acmeHTTPClient != nil {
		lc.HTTPClient = acmeHTTPClient()
	}
	if cfg.Network != "" {
		// 限定 ACME 接口只走 IPv4 或 IPv6
		lc.HTTPClient = forceNetwork(lc.HTTPClient, cfg.Network)
		log.Infof("签发只使用 %s 网络", networkName(cfg.Network))
	}
	client, err := lego.NewClient(lc)
	if err != nil {
		return nil, fmt.Errorf("连接 ACME 服务失败: %w", err)
	}

	if user.reg == nil {
		log.Infof("注册 ACME 账户 %s", cfg.Email)
		var reg *registration.Resource
		if cfg.EABKid != "" {
			reg, err = client.Registration.RegisterWithExternalAccountBinding(registration.RegisterEABOptions{
				TermsOfServiceAgreed: true, Kid: cfg.EABKid, HmacEncoded: cfg.EABHMAC,
			})
		} else {
			reg, err = client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		}
		if err != nil {
			return nil, fmt.Errorf("注册 ACME 账户失败: %w", err)
		}
		user.reg = reg
		keyDER, err := x509.MarshalPKCS8PrivateKey(user.key)
		if err != nil {
			return nil, err
		}
		regJSON, _ := json.Marshal(reg)
		res.Account = &Account{
			Email:        cfg.Email,
			DirectoryURL: dirURL,
			KeyPEM:       string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})),
			Registration: string(regJSON),
		}
	}

	if cfg.Challenge == ChallengeHTTP {
		// HTTP-01：由 Web 服务模块或临时监听应答，无需 DNS 服务商
		prov := &acmehttp.Provider{Port: cfg.HTTPPort, Logf: log.Infof}
		if err := client.Challenge.SetHTTP01Provider(prov); err != nil {
			return nil, err
		}
	} else if err := setupDNS01(client, cfg, log); err != nil {
		return nil, err
	}

	if cfg.Profile != "" {
		log.Infof("申请证书 %s（CA: %s，证书配置: %s）", strings.Join(cfg.Domains, ", "), dirURL, cfg.Profile)
	} else {
		log.Infof("申请证书 %s（CA: %s）", strings.Join(cfg.Domains, ", "), dirURL)
	}
	cert, err := client.Certificate.Obtain(certificate.ObtainRequest{Domains: cfg.Domains, Bundle: true, Profile: cfg.Profile})
	if err != nil {
		return nil, fmt.Errorf("签发失败: %w", err)
	}
	if len(cert.Certificate) == 0 || len(cert.PrivateKey) == 0 {
		return nil, errors.New("CA 返回的证书为空")
	}
	res.CertPEM = string(cert.Certificate)
	res.KeyPEM = string(cert.PrivateKey)
	res.IssuerPEM = string(cert.IssuerCertificate)
	return res, nil
}

// systemNameservers 返回系统设置的 DNS 服务器（/etc/resolv.conf），读取失败时与 lego 一样使用公共 DNS。
var systemNameservers = sync.OnceValue(func() []string {
	if c, err := dns.ClientConfigFromFile("/etc/resolv.conf"); err == nil && len(c.Servers) > 0 {
		return dns01.ParseNameservers(c.Servers)
	}
	return []string{"google-public-dns-a.google.com:53", "google-public-dns-b.google.com:53"}
})

// setupDNS01 为客户端配置 DNS-01 验证。
func setupDNS01(client *lego.Client, cfg ACMEConfig, log *logbuf.Logger) error {
	hc := &http.Client{Timeout: 30 * time.Second}
	if cfg.Network != "" {
		hc = forceNetwork(hc, cfg.Network)
	}
	p, err := dnsprovider.New(cfg.Provider, cfg.Credentials, hc)
	if err != nil {
		return err
	}
	txt, err := dnsprovider.AsTXT(p)
	if err != nil {
		return err
	}
	// lego 的递归 DNS 服务器是包级全局变量，每张证书都显式设置，避免沿用上一张证书设置的 DNS 服务器
	ns := systemNameservers()
	if len(cfg.Resolvers) > 0 {
		ns = dns01.ParseNameservers(cfg.Resolvers)
	}
	if cfg.Network != "" {
		// 只保留当前地址族的 DNS 服务器，其他地址族的服务器在该网络下无法访问
		ns = filterNameservers(ns, cfg.Network)
		if len(ns) == 0 {
			return fmt.Errorf("检查 TXT 生效所用的 DNS 服务器中没有 %s 地址，请在「检查用 DNS」中填写 %s 的 DNS 服务器", networkName(cfg.Network), networkName(cfg.Network))
		}
	}
	solver := &dnsSolver{txt: txt, zone: strings.Trim(cfg.Zone, "."), timeout: time.Duration(cfg.PropagationTimeout) * time.Second, log: log, nameservers: ns, network: cfg.Network, disableCNAME: cfg.DisableCNAME}
	opts := []dns01.ChallengeOption{dns01.AddRecursiveNameservers(ns)}
	if cfg.DisableAuthoritativeCheck {
		opts = append(opts, dns01.DisableAuthoritativeNssPropagationRequirement())
	}
	pc := netPreCheck{network: cfg.Network, nameservers: ns, authoritative: !cfg.DisableAuthoritativeCheck}
	// lego 传入的 fqdn 是它自己跟随 CNAME 得到的位置，这里统一换成验证记录实际写入的位置
	opts = append(opts, dns01.WrapPreCheck(func(domain, _, value string, check dns01.PreCheckFunc) (bool, error) {
		if cfg.SkipPropagationCheck {
			// 跳过本地检查，留出固定时间让 TXT 生效后再交给 CA 验证
			time.Sleep(skipPropagationWait)
			return true, nil
		}
		t := solver.target(domain)
		if t.noFollow {
			// 递归 DNS 可能还缓存着泛解析的 CNAME，不能跟随它去检查别的名称
			p := pc
			p.noCNAME = true
			return p.check(domain, t.fqdn, value, check)
		}
		if cfg.Network != "" {
			// lego 的默认检查不能限定地址族，改用同样规则的检查
			return pc.check(domain, t.fqdn, value, check)
		}
		return check(t.fqdn, value)
	}))
	return client.Challenge.SetDNS01Provider(solver, opts...)
}
