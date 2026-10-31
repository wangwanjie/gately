package ssl

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net/netip"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/dnsprovider"
)

// 证书来源
const (
	SourceACME   = "acme"
	SourceUpload = "upload"
	SourceFile   = "file"
)

// 预置 CA，自定义时使用 DirectoryURL
var caDirectories = map[string]string{
	"letsencrypt":         "https://acme-v02.api.letsencrypt.org/directory",
	"letsencrypt-staging": "https://acme-staging-v02.api.letsencrypt.org/directory",
	"zerossl":             "https://acme.zerossl.com/v2/DV90",
	"google":              "https://dv.acme-v02.api.pki.goog/directory",
}

// 验证方式
const (
	ChallengeDNS  = "dns"
	ChallengeHTTP = "http"
)

// 签发使用的网络（ACMEConfig.Network），留空为自动
const (
	NetworkIPv4 = "tcp4"
	NetworkIPv6 = "tcp6"
)

var validKeyTypes = map[string]bool{"P256": true, "P384": true, "2048": true, "4096": true}

// ProfileShortLived 是 Let's Encrypt 的短期证书配置（约 6 天有效期），也是其签发 IP 地址证书的唯一配置。
const ProfileShortLived = "shortlived"

// ACME 证书配置名（draft-ietf-acme-profiles）只允许常见的标识字符
var reProfile = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// ACMEConfig 是 ACME 自动签发参数，验证方式为 DNS-01 或 HTTP-01。
type ACMEConfig struct {
	Domains []string `json:"domains"`
	Email   string   `json:"email"`
	// CA 为预置名称或 custom
	CA           string `json:"ca"`
	DirectoryURL string `json:"directoryUrl"`
	// EAB 用于 ZeroSSL、Google 等需要外部账户绑定的 CA
	EABKid  string `json:"eabKid"`
	EABHMAC string `json:"eabHmac"`
	KeyType string `json:"keyType"`
	// Profile 为 ACME 证书配置（profile），留空使用 CA 默认配置；shortlived 为短期证书，IP 地址证书需要它
	Profile string `json:"profile"`
	// Challenge 为验证方式：dns（默认，支持通配符）或 http（HTTP-01，无需 DNS 服务商，要求 80 端口可从公网访问）
	Challenge string `json:"challenge"`
	// HTTPPort 为 HTTP-01 临时监听端口，默认 80（80 端口已被 Web 服务占用时由 Web 服务应答）
	HTTPPort int `json:"httpPort"`
	// Provider 与 Credentials 为 DNS 服务商，用于写入 _acme-challenge TXT 记录
	Provider    string                  `json:"provider"`
	Credentials dnsprovider.Credentials `json:"credentials"`
	// Zone 显式指定主域名，留空自动推断
	Zone string `json:"zone"`
	// Resolvers 为检查 TXT 生效所用的递归 DNS，留空使用系统配置
	Resolvers []string `json:"resolvers"`
	// PropagationTimeout 为等待 TXT 生效的最长时间（秒）
	PropagationTimeout int `json:"propagationTimeout"`
	// SkipPropagationCheck 跳过本地生效检查，等待固定时间后直接通知 CA 验证
	SkipPropagationCheck bool `json:"skipPropagationCheck"`
	// DisableAuthoritativeCheck 只通过递归 DNS 检查 TXT 是否生效，不查询权威服务器
	DisableAuthoritativeCheck bool `json:"disableAuthoritativeCheck"`
	// Network 限定签发时的出站网络：留空自动选择，tcp4 只用 IPv4，tcp6 只用 IPv6。
	// 作用于 ACME 接口、DNS 服务商接口与 TXT 生效检查，适用于有 IPv6 地址但 IPv6 不通的主机
	Network string `json:"network,omitempty"`
}

// Mapping 把证书写到目录供其他服务使用，文件名为 <备注>.crt/.key/.pem/_issuerCertificate.crt。
type Mapping struct {
	Enable bool   `json:"enable"`
	Dir    string `json:"dir"`
	// Script 在证书文件变化后执行，例如重载 nginx
	Script string `json:"script"`
}

// Cert 是一张证书。
type Cert struct {
	Key    string `json:"key"`
	Remark string `json:"remark"`
	// Group 为列表中的分组名称，只影响管理界面的展示，留空为未分组
	Group  string `json:"group"`
	Enable bool   `json:"enable"`
	Source string `json:"source"`

	ACME ACMEConfig `json:"acme"`
	// CertFile / KeyFile 为文件来源的路径，定期重新读取
	CertFile string `json:"certFile"`
	KeyFile  string `json:"keyFile"`

	// 证书内容（PEM）。CertPEM 为包含中间证书的完整链
	CertPEM   string `json:"certPem"`
	KeyPEM    string `json:"keyPem"`
	IssuerPEM string `json:"issuerPem"`

	Mapping Mapping `json:"mapping"`
	// Deploys 为证书内容变化后自动执行的部署目标（SSH 主机、本机目录、Docker 容器）
	Deploys []Deploy `json:"deploys"`
	// UpdatedAt 为证书内容最近一次变化时间
	UpdatedAt time.Time `json:"updatedAt,omitzero"`
	// IssuedProfile 为当前证书签发时使用的 ACME 证书配置，与 ACME.Profile 不同时重新签发
	IssuedProfile string `json:"issuedProfile,omitempty"`
}

// Account 是缓存的 ACME 账户，按 CA 地址与邮箱区分。
type Account struct {
	Email        string `json:"email"`
	DirectoryURL string `json:"directoryUrl"`
	KeyPEM       string `json:"keyPem"`
	// Registration 为 lego registration.Resource 的 JSON
	Registration string `json:"registration"`
}

// Settings 是模块级配置。
type Settings struct {
	// Enable 为自动续签开关，关闭后不再自动签发（已有证书仍可使用）
	Enable bool `json:"enable"`
	// RenewBeforeDays 为到期前多少天续签
	RenewBeforeDays int `json:"renewBeforeDays"`
	// ShortLivedRenewHours 为短期证书（有效期不足 10 天）到期前多少小时续签
	ShortLivedRenewHours int    `json:"shortLivedRenewHours"`
	DefaultEmail         string `json:"defaultEmail"`
}

type fileConfig struct {
	Settings Settings           `json:"settings"`
	Certs    []Cert             `json:"certs"`
	Accounts map[string]Account `json:"accounts,omitempty"`
}

// maxShortLivedRenewHours 为短期证书续签提前小时数上限，约为 Let's Encrypt 短期证书有效期的一半
const maxShortLivedRenewHours = 80

func defaultSettings() Settings {
	return Settings{Enable: true, RenewBeforeDays: 30, ShortLivedRenewHours: 24}
}

func (s *Settings) normalize() {
	if s.RenewBeforeDays <= 0 {
		s.RenewBeforeDays = 30
	}
	if s.ShortLivedRenewHours <= 0 {
		s.ShortLivedRenewHours = 24
	}
	// Let's Encrypt 短期证书有效期约 160 小时，提前量过大会导致每次检查都重签
	if s.ShortLivedRenewHours > maxShortLivedRenewHours {
		s.ShortLivedRenewHours = maxShortLivedRenewHours
	}
}

// 备注用作映射文件名，限制字符集防止路径穿越
var reRemark = regexp.MustCompile(`^[A-Za-z0-9*_.@-]+$`)

// fileBase 返回映射文件名前缀，通配符替换为下划线。
func (c *Cert) fileBase() string {
	return strings.ReplaceAll(c.Remark, "*", "_")
}

// directoryURL 返回 ACME 目录地址。
func (a *ACMEConfig) directoryURL() string {
	if a.CA == "custom" || a.CA == "" && a.DirectoryURL != "" {
		return a.DirectoryURL
	}
	if u, ok := caDirectories[a.CA]; ok {
		return u
	}
	return caDirectories["letsencrypt"]
}

// Validate 校验配置。上传来源要求证书与私钥匹配。
func (c *Cert) Validate(settings Settings) error {
	if err := crud.NormalizeGroup(&c.Group); err != nil {
		return err
	}
	c.Remark = strings.TrimSpace(c.Remark)
	if c.Mapping.Enable {
		if c.Remark == "" {
			return errors.New("启用证书映射时备注不能为空（用作文件名）")
		}
		c.Mapping.Dir = strings.TrimSpace(c.Mapping.Dir)
		if c.Mapping.Dir == "" || !filepath.IsAbs(c.Mapping.Dir) {
			return errors.New("映射目录需为绝对路径")
		}
	}
	if c.Remark != "" && !reRemark.MatchString(c.Remark) {
		return errors.New("备注只能包含字母、数字以及 . _ - * @")
	}
	for i := range c.Deploys {
		if err := c.Deploys[i].validate(c.Remark); err != nil {
			return fmt.Errorf("部署目标 %s: %w", deployIndexLabel(i), err)
		}
	}
	switch c.Source {
	case SourceACME:
		return c.ACME.validate(settings)
	case SourceUpload:
		if c.CertPEM == "" || c.KeyPEM == "" {
			return errors.New("请上传证书与私钥")
		}
		if _, err := tls.X509KeyPair([]byte(c.CertPEM), []byte(c.KeyPEM)); err != nil {
			return fmt.Errorf("证书与私钥不匹配或格式错误: %w", err)
		}
	case SourceFile:
		if !filepath.IsAbs(c.CertFile) || !filepath.IsAbs(c.KeyFile) {
			return errors.New("证书与私钥路径需为绝对路径")
		}
	default:
		return fmt.Errorf("未知的证书来源 %q", c.Source)
	}
	return nil
}

func (a *ACMEConfig) validate(settings Settings) error {
	var domains []string
	seen := map[string]bool{}
	var ips []netip.Addr
	for _, d := range a.Domains {
		d = strings.ToLower(strings.Trim(strings.TrimSpace(d), "."))
		if d == "" {
			continue
		}
		if err := checkIPLike(d); err != nil {
			return err
		}
		// IP 地址统一成规范写法（IPv6 去掉方括号并压缩），与证书中的 IP SAN 一致
		if ip, ok := parseIPIdentifier(d); ok {
			d = ip.String()
			if !seen[d] {
				ips = append(ips, ip)
			}
		}
		if seen[d] {
			continue
		}
		if strings.Count(d, "*") > 1 || (strings.Contains(d, "*") && !strings.HasPrefix(d, "*.")) {
			return fmt.Errorf("域名 %q 格式错误，通配符只能出现在最左侧", d)
		}
		seen[d] = true
		domains = append(domains, d)
	}
	if len(domains) == 0 {
		return errors.New("至少填写一个域名")
	}
	a.Domains = domains
	a.Email = strings.TrimSpace(a.Email)
	if a.Email == "" {
		a.Email = settings.DefaultEmail
	}
	if a.Email == "" || !strings.Contains(a.Email, "@") {
		return errors.New("请填写有效的 ACME 账户邮箱")
	}
	if a.CA == "" {
		a.CA = "letsencrypt"
	}
	if a.CA == "custom" && !strings.HasPrefix(a.DirectoryURL, "https://") {
		return errors.New("自定义 CA 需填写 https 开头的目录地址")
	}
	if a.CA != "custom" {
		if _, ok := caDirectories[a.CA]; !ok {
			return fmt.Errorf("未知的 CA %q", a.CA)
		}
	}
	if (a.CA == "zerossl" || a.CA == "google") && (a.EABKid == "" || a.EABHMAC == "") {
		return errors.New("该 CA 需要填写 EAB Key ID 与 HMAC Key")
	}
	if a.KeyType == "" {
		a.KeyType = "P256"
	}
	if !validKeyTypes[a.KeyType] {
		return fmt.Errorf("不支持的密钥类型 %q", a.KeyType)
	}
	a.Profile = strings.TrimSpace(a.Profile)
	if a.Profile != "" && !reProfile.MatchString(a.Profile) {
		return fmt.Errorf("证书配置名 %q 格式错误", a.Profile)
	}
	if err := a.validateIPs(ips); err != nil {
		return err
	}
	switch a.Challenge {
	case "", ChallengeDNS:
		a.Challenge = ChallengeDNS
		if !dnsprovider.SupportsTXT(a.Provider) {
			return fmt.Errorf("DNS 服务商 %q 不支持 ACME DNS 验证", a.Provider)
		}
	case ChallengeHTTP:
		for _, d := range a.Domains {
			if strings.HasPrefix(d, "*.") {
				return fmt.Errorf("HTTP-01 验证不支持通配符域名 %q，请改用 DNS 验证", d)
			}
		}
		if a.HTTPPort < 0 || a.HTTPPort > 65535 {
			return errors.New("HTTP-01 监听端口无效")
		}
	default:
		return fmt.Errorf("未知的验证方式 %q", a.Challenge)
	}
	if a.PropagationTimeout <= 0 {
		a.PropagationTimeout = 300
	}
	switch a.Network {
	case "", NetworkIPv4, NetworkIPv6:
	default:
		return fmt.Errorf("未知的网络类型 %q", a.Network)
	}
	return nil
}

// parseIPIdentifier 判断标识是否为 IP 地址（IPv6 可带方括号），返回规范化后的地址。
func parseIPIdentifier(s string) (netip.Addr, bool) {
	s = strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
	ip, err := netip.ParseAddr(s)
	if err != nil || ip.Zone() != "" {
		return netip.Addr{}, false
	}
	return ip.Unmap(), true
}

// 只由数字和点组成的标识，按 IPv4 处理
var reIPv4Like = regexp.MustCompile(`^[0-9.]+$`)

// checkIPLike 拒绝看起来像 IP 却无法作为证书标识的写法：带 zone 的 IPv6、
// 无法解析的 IPv6（域名不含冒号）以及前导零等非规范 IPv4，避免被当作域名保存后签发失败。
func checkIPLike(d string) error {
	s := strings.TrimSuffix(strings.TrimPrefix(d, "["), "]")
	ip, err := netip.ParseAddr(s)
	if err == nil {
		if ip.Zone() != "" {
			return fmt.Errorf("IP 地址 %q 不能带区域标识", d)
		}
		return nil
	}
	if strings.Contains(s, ":") || reIPv4Like.MatchString(s) {
		return fmt.Errorf("IP 地址 %q 格式错误", d)
	}
	return nil
}

// hasIPIdentifier 判断域名列表中是否包含 IP 地址。
func hasIPIdentifier(domains []string) bool {
	for _, d := range domains {
		if _, ok := parseIPIdentifier(d); ok {
			return true
		}
	}
	return false
}

// cgnat 为运营商级 NAT 与 Tailscale 使用的共享地址段，公网无法访问
var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// validateIPs 校验 IP 地址证书的限制：只能用 HTTP-01 验证，公共 CA 只为公网地址签发，
// Let's Encrypt 只在短期证书配置下签发 IP 地址证书。
func (a *ACMEConfig) validateIPs(ips []netip.Addr) error {
	if len(ips) == 0 {
		return nil
	}
	if a.Challenge != ChallengeHTTP {
		return errors.New("IP 地址证书只能使用 HTTP-01 验证")
	}
	switch a.CA {
	case "custom":
		// 自建 CA（如内网 step-ca）可能为内网地址签发，这里不做限制
		return nil
	case "zerossl", "google":
		return errors.New("该 CA 不支持通过 ACME 签发 IP 地址证书，请改用 Let's Encrypt")
	}
	for _, ip := range ips {
		if !ip.IsGlobalUnicast() || ip.IsPrivate() || cgnat.Contains(ip) {
			return fmt.Errorf("%s 不是公网 IP 地址，公共 CA 无法验证", ip)
		}
	}
	if a.Profile != ProfileShortLived {
		return errors.New("Let's Encrypt 只为短期证书签发 IP 地址证书，请把证书配置选为短期证书（shortlived）")
	}
	return nil
}

// CertInfo 是从证书内容解析出的信息。
type CertInfo struct {
	Domains     []string  `json:"domains"`
	Issuer      string    `json:"issuer"`
	NotBefore   time.Time `json:"notBefore"`
	NotAfter    time.Time `json:"notAfter"`
	Fingerprint string    `json:"fingerprint"`
}

// parseCert 解析 PEM 证书链中的叶子证书。
func parseCert(certPEM string) (*x509.Certificate, error) {
	rest := []byte(certPEM)
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			return nil, errors.New("没有找到 PEM 证书")
		}
		if block.Type == "CERTIFICATE" {
			return x509.ParseCertificate(block.Bytes)
		}
	}
}

func certInfo(certPEM string) (*CertInfo, error) {
	leaf, err := parseCert(certPEM)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(leaf.Raw)
	domains := append([]string{}, leaf.DNSNames...)
	// IP 地址证书的标识在 IP SAN 中
	for _, ip := range leaf.IPAddresses {
		if addr, ok := netip.AddrFromSlice(ip); ok {
			domains = append(domains, addr.Unmap().String())
		}
	}
	if len(domains) == 0 && leaf.Subject.CommonName != "" {
		domains = []string{leaf.Subject.CommonName}
	}
	return &CertInfo{
		Domains:     domains,
		Issuer:      leaf.Issuer.CommonName,
		NotBefore:   leaf.NotBefore,
		NotAfter:    leaf.NotAfter,
		Fingerprint: hex.EncodeToString(sum[:]),
	}, nil
}
