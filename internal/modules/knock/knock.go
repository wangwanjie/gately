// Package knock 实现端口敲门模块：接收与 fwknop 兼容的单包授权（SPA）报文，认证通过后在限定时间内
// 为来源地址放行受保护的端口。
//
// 放行有两种方式，可同时使用：
//   - 系统防火墙：规则开启“防火墙拦截”时，受保护端口在 nftables（内核没有 nf_tables 时用 iptables + ipset）
//     中默认丢弃，敲门成功的地址按 地址 + 协议 + 端口 写入带超时的集合元素。用于保护宿主机自己的 sshd 等服务；
//   - Gately 自身：协议分流的路由、端口转发规则可以选择“需要敲门”，只转发持有该规则有效授权的来源地址
//     （见 internal/knockgate），不依赖防火墙权限。
//
// 报文经本模块监听的 UDP 端口（fwknop 默认 62201）发来，也可以经协议分流的 TCP 端口发来（fwknop 的 -P tcp）。
// 已建立的连接在授权到期后照常保持，与 fwknop 的语义一致。
package knock

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/fwopen"
	"github.com/wangwanjie/gately/internal/knockgate"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/spa"
)

// ID 是模块标识。
const ID = "knock"

// 默认值与取值范围
const (
	defaultPort       = 62201
	defaultTimeout    = 30
	defaultMaxTimeout = 300
	defaultMaxAge     = 120
	minTimeout        = 5
	maxTimeoutLimit   = 86400
	minMaxAge         = 10
	maxMaxAge         = 3600
	maxRulePorts      = 16
	maxNameLen        = 64
	maxUserLen        = 64
	// maxIdentities 为每条规则最多的身份数，maxTotalKeys 为全部启用规则接受敲门的密钥总数上限：
	// 伪造报文要用每个密钥各做一次 HMAC 校验才能丢弃，密钥总数决定单个报文的最大处理成本（见 packetCost）
	maxIdentities = 64
	maxTotalKeys  = 1024
	// replayCap 为防重放缓存最多保存的摘要数；摘要按报文时间戳加 maxMaxAge 保留，见 check
	replayCap = 100000
)

// Settings 是模块全局设置。
type Settings struct {
	// Port 为接收敲门报文的 UDP 端口（fwknop 客户端默认 62201），0 表示不监听 UDP，只接收经协议分流发来的报文
	Port int `json:"port"`
	// AutoFirewall 为 true 时在系统防火墙（OpenWrt fw4、firewalld 等）放行敲门端口；
	// 防火墙拦截生效后也放行受保护端口，使是否可访问只由敲门决定
	AutoFirewall bool `json:"autoFirewall"`
	// AllowLAN 为 true 时内网、回环地址与本机网卡所在的公网 IPv6 网段无需敲门即可访问受保护端口
	AllowLAN bool `json:"allowLan"`
	// AllowList 为无需敲门即可访问受保护端口的 IP 或网段
	AllowList []string `json:"allowList"`
	// MaxAge 为报文时间戳与本机时间允许的最大偏差（秒），超出视为过期或重放
	MaxAge int `json:"maxAge"`
}

func defaultSettings() Settings {
	return Settings{Port: defaultPort, AutoFirewall: true, AllowLAN: true, AllowList: []string{}, MaxAge: defaultMaxAge}
}

// Validate 校验并规范化设置。
func (s *Settings) Validate() error {
	if s.Port < 0 || s.Port > 65535 {
		return errors.New("敲门端口应在 1～65535 之间，0 表示不监听")
	}
	if s.MaxAge == 0 {
		s.MaxAge = defaultMaxAge
	}
	if s.MaxAge < minMaxAge || s.MaxAge > maxMaxAge {
		return fmt.Errorf("报文有效期应在 %d～%d 秒之间", minMaxAge, maxMaxAge)
	}
	list := []string{}
	for _, v := range s.AllowList {
		if v = strings.TrimSpace(v); v == "" {
			continue
		}
		p, err := parsePrefix(v)
		if err != nil {
			return fmt.Errorf("白名单 %q 无效: %w", v, err)
		}
		if p.Bits() == 0 {
			return fmt.Errorf("白名单 %q 包含全部地址，等于不拦截", v)
		}
		list = append(list, v)
	}
	s.AllowList = list
	return nil
}

// Rule 是一条敲门规则：一组共享密钥及其可以放行的端口。
type Rule struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Group 为列表中的分组名称，只影响展示
	Group  string `json:"group,omitempty"`
	Enable bool   `json:"enable"`
	// Ports 为敲门可以请求放行的端口（如 tcp/22），客户端 -A 参数中的端口必须全部在此列表中。
	// 留空时规则不接受敲门报文，只能在 Web 站点的放行入口登录取得授权（不开启防火墙拦截时才能留空）
	Ports []string `json:"ports"`
	// Firewall 为 true 时在系统防火墙中拦截这些端口，只放行敲门成功的地址；
	// 为 false 时不改动防火墙，只供协议分流路由、端口转发规则按敲门结果放行
	Firewall bool `json:"firewall"`
	// EncKey、HMACKey 为 base64 编码的加密密钥与 HMAC 密钥（对应 fwknop 的 KEY_BASE64、HMAC_KEY_BASE64），留空时自动生成
	EncKey  string `json:"encKey"`
	HMACKey string `json:"hmacKey"`
	// HMACType 为 HMAC 摘要算法：sha256（默认）、sha384、sha512
	HMACType string `json:"hmacType"`
	// Timeout 为放行时长（秒）
	Timeout int `json:"timeout"`
	// MaxTimeout 为客户端用 -f 请求的放行时长上限（秒），0 表示忽略客户端请求的时长
	MaxTimeout int `json:"maxTimeout"`
	// User 非空时只接受报文中用户名为该值的敲门（fwknop 默认发送本机登录名，可用 -U 指定）
	User string `json:"user"`
	// RequireAddr 为 true 时拒绝不带地址的报文（fwknop -s），客户端必须用 -a 或 -R 写明要放行的地址，
	// 防止中途截获报文的人抢先从自己的地址重放
	RequireAddr bool `json:"requireAddr"`
	// Identities 为规则的身份：每个身份有自己的一组密钥，敲门时按密钥识别是谁，日志、事件与授权列表显示身份名称；
	// 停用、删除身份或更换其密钥只撤销该身份的放行，其他人不受影响。身份使用规则的 HMAC 算法、端口与放行时长
	Identities []Identity `json:"identities,omitempty"`
	// SharedKeyOff 为 true 时规则自身的密钥（共享密钥）不再接受敲门，只接受各身份的密钥
	SharedKeyOff bool `json:"sharedKeyOff,omitempty"`
	// SingleAddr 为 true 时每个身份（共享密钥算一个）同时只放行一个地址：从新地址敲门成功后，该身份此前经敲门报文
	// 放行的其他地址立即撤销，防止同一份密钥被多人同时使用。已建立的连接不受影响
	SingleAddr bool   `json:"singleAddr,omitempty"`
	Remark     string `json:"remark"`
}

// Identity 是规则中的一个身份（一个人或一台设备）及其密钥。
type Identity struct {
	// ID 为身份在规则内的标识，留空时自动分配
	ID     string `json:"id"`
	Name   string `json:"name"`
	Enable bool   `json:"enable"`
	// EncKey、HMACKey 含义与规则的同名字段相同，留空时自动生成
	EncKey  string `json:"encKey"`
	HMACKey string `json:"hmacKey"`
	Remark  string `json:"remark,omitempty"`
}

// spaKey 解码身份的密钥，HMAC 算法使用规则的设置。
func (id *Identity) spaKey(hmacType string) (spa.Key, error) {
	return decodeSPAKey(id.EncKey, id.HMACKey, hmacType)
}

// Validate 校验并规范化规则；密钥留空时生成新密钥。
func (r *Rule) Validate() error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return errors.New("请填写名称")
	}
	if utf8.RuneCountInString(r.Name) > maxNameLen {
		return fmt.Errorf("名称不能超过 %d 个字符", maxNameLen)
	}
	if err := crud.NormalizeGroup(&r.Group); err != nil {
		return err
	}
	ports, err := parsePorts(r.Ports)
	if err != nil {
		return err
	}
	if len(ports) == 0 && r.Firewall {
		return errors.New("开启防火墙拦截时请至少填写一个端口，如 tcp/22")
	}
	r.Ports = formatPorts(ports)
	if r.HMACType == "" {
		r.HMACType = spa.HMACSHA256
	}
	if !spa.ValidHMACType(r.HMACType) {
		return fmt.Errorf("不支持的 HMAC 算法 %q", r.HMACType)
	}
	if r.EncKey = strings.TrimSpace(r.EncKey); r.EncKey == "" {
		r.EncKey = genKey(32)
	}
	if r.HMACKey = strings.TrimSpace(r.HMACKey); r.HMACKey == "" {
		r.HMACKey = genKey(64)
	}
	if _, err := r.spaKey(); err != nil {
		return err
	}
	// 统一保存为标准 base64：粘贴时带进来的换行、空格与省略的末尾 "=" 都去掉或补齐，
	// 生成的客户端命令与 ~/.fwknoprc 中的密钥是一个完整的单词
	r.EncKey, _ = normalizeKey(r.EncKey)
	r.HMACKey, _ = normalizeKey(r.HMACKey)
	if r.Timeout == 0 {
		r.Timeout = defaultTimeout
	}
	if r.Timeout < minTimeout || r.Timeout > maxTimeoutLimit {
		return fmt.Errorf("放行时长应在 %d 秒到 %d 秒之间", minTimeout, maxTimeoutLimit)
	}
	if r.MaxTimeout < 0 || r.MaxTimeout > maxTimeoutLimit {
		return fmt.Errorf("客户端可请求的最长时长应在 0～%d 秒之间", maxTimeoutLimit)
	}
	if r.MaxTimeout > 0 && r.MaxTimeout < r.Timeout {
		return errors.New("客户端可请求的最长时长不能小于放行时长")
	}
	r.User = strings.TrimSpace(r.User)
	if len(r.User) > maxUserLen || strings.ContainsFunc(r.User, func(c rune) bool { return c < 0x20 || c > 0x7e }) {
		return fmt.Errorf("用户名只能包含可见 ASCII 字符，且不超过 %d 个字符", maxUserLen)
	}
	r.Remark = strings.TrimSpace(r.Remark)
	return r.validateIdentities()
}

// validateIdentities 校验并规范化规则的身份：名称必填且不重复，密钥留空时生成，各密钥（含共享密钥）互不相同。
func (r *Rule) validateIdentities() error {
	if r.Identities == nil {
		r.Identities = []Identity{}
	}
	if len(r.Identities) > maxIdentities {
		return fmt.Errorf("每条规则最多 %d 个身份", maxIdentities)
	}
	if len(r.Identities) > 0 && len(r.Ports) == 0 {
		return errors.New("规则没有填写端口时不接受敲门报文，不能添加身份")
	}
	names := map[string]bool{}
	// HMAC 密钥决定报文匹配到哪个身份：重复时只会匹配到前一个，身份识别失去意义。值为身份名称，共享密钥为空串
	macs := map[string]string{r.HMACKey: ""}
	enabled := 0
	for i := range r.Identities {
		id := &r.Identities[i]
		id.Name = strings.TrimSpace(id.Name)
		if id.Name == "" {
			return fmt.Errorf("第 %d 个身份请填写名称", i+1)
		}
		if utf8.RuneCountInString(id.Name) > maxNameLen || strings.ContainsFunc(id.Name, func(c rune) bool { return c < 0x20 }) {
			return fmt.Errorf("身份「%s」的名称不能超过 %d 个字符，且不能包含控制字符", id.Name, maxNameLen)
		}
		lower := strings.ToLower(id.Name)
		if names[lower] {
			return fmt.Errorf("身份名称「%s」重复", id.Name)
		}
		names[lower] = true
		if id.EncKey = strings.TrimSpace(id.EncKey); id.EncKey == "" {
			id.EncKey = genKey(32)
		}
		if id.HMACKey = strings.TrimSpace(id.HMACKey); id.HMACKey == "" {
			id.HMACKey = genKey(64)
		}
		if _, err := id.spaKey(r.HMACType); err != nil {
			return fmt.Errorf("身份「%s」: %w", id.Name, err)
		}
		id.EncKey, _ = normalizeKey(id.EncKey)
		id.HMACKey, _ = normalizeKey(id.HMACKey)
		if other, dup := macs[id.HMACKey]; dup {
			if other == "" {
				return fmt.Errorf("身份「%s」的 HMAC 密钥与共享密钥相同，请重新生成", id.Name)
			}
			return fmt.Errorf("身份「%s」的 HMAC 密钥与身份「%s」相同，请重新生成", id.Name, other)
		}
		macs[id.HMACKey] = id.Name
		id.Remark = strings.TrimSpace(id.Remark)
		if id.Enable {
			enabled++
		}
	}
	crud.AssignKeys(len(r.Identities), func(i int) *string { return &r.Identities[i].ID }, "i")
	if r.SharedKeyOff && len(r.Ports) > 0 && enabled == 0 {
		return errors.New("停用共享密钥时请至少添加一个启用的身份，否则规则不接受任何敲门")
	}
	return nil
}

// spaKey 解码规则中的密钥（共享密钥）。
func (r *Rule) spaKey() (spa.Key, error) {
	return decodeSPAKey(r.EncKey, r.HMACKey, r.HMACType)
}

// decodeSPAKey 解码一组 base64 密钥并校验长度。
func decodeSPAKey(encKey, hmacKey, hmacType string) (spa.Key, error) {
	enc, err := decodeKey(encKey)
	if err != nil {
		return spa.Key{}, fmt.Errorf("加密密钥需为 base64 编码: %w", err)
	}
	mac, err := decodeKey(hmacKey)
	if err != nil {
		return spa.Key{}, fmt.Errorf("HMAC 密钥需为 base64 编码: %w", err)
	}
	k := spa.Key{Enc: enc, HMAC: mac, HMACType: hmacType}
	return k, k.Validate()
}

// acceptKeys 返回规则接受敲门的密钥数：没有端口的规则不接受敲门报文，为 0。
func (r *Rule) acceptKeys() int {
	if !r.Enable || len(r.Ports) == 0 {
		return 0
	}
	n := 0
	if !r.SharedKeyOff {
		n++
	}
	for _, id := range r.Identities {
		if id.Enable {
			n++
		}
	}
	return n
}

// decodeKey 解码 base64 密钥，忽略其中的空白与换行（从终端或文档复制时常带进来），兼容省略了末尾 "=" 的写法。
func decodeKey(s string) ([]byte, error) {
	s = strings.Join(strings.Fields(s), "")
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.RawStdEncoding.DecodeString(strings.TrimRight(s, "="))
}

// normalizeKey 把 base64 密钥规范化为不含空白、带末尾填充的标准编码。
func normalizeKey(s string) (string, error) {
	b, err := decodeKey(s)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// genKey 生成 n 字节随机密钥（base64），与 fwknop --key-gen 的长度一致：加密密钥 32 字节、HMAC 密钥 64 字节。
func genKey(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

// parsePorts 解析并去重 “tcp/22” 形式的端口列表，按协议、端口排序。
func parsePorts(in []string) ([]spa.Access, error) {
	var out []spa.Access
	for _, raw := range in {
		for _, s := range strings.FieldsFunc(raw, func(c rune) bool { return c == ',' || c == '\n' || c == ' ' || c == '\t' }) {
			// 只写端口号时按 TCP 处理
			if !strings.Contains(s, "/") {
				s = "tcp/" + s
			}
			a, err := spa.ParseAccess(s)
			if err != nil {
				return nil, fmt.Errorf("端口 %q 无效，格式为 tcp/22 或 udp/51820", s)
			}
			if !slices.Contains(out, a) {
				out = append(out, a)
			}
		}
	}
	if len(out) > maxRulePorts {
		return nil, fmt.Errorf("每条规则最多 %d 个端口", maxRulePorts)
	}
	sortAccess(out)
	return out, nil
}

func sortAccess(a []spa.Access) {
	slices.SortFunc(a, func(x, y spa.Access) int {
		if x.Proto != y.Proto {
			return strings.Compare(x.Proto, y.Proto)
		}
		return x.Port - y.Port
	})
}

func formatPorts(a []spa.Access) []string {
	out := make([]string, 0, len(a))
	for _, x := range a {
		out = append(out, x.String())
	}
	return out
}

// parsePrefix 解析 IP 或 CIDR 为网段，IPv4 映射地址按 IPv4 处理。
func parsePrefix(s string) (netip.Prefix, error) {
	s = strings.TrimSpace(s)
	if a, err := netip.ParseAddr(s); err == nil {
		if a.Zone() != "" {
			return netip.Prefix{}, errors.New("不能带网卡区域标识")
		}
		a = a.Unmap()
		return netip.PrefixFrom(a, a.BitLen()), nil
	}
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return netip.Prefix{}, errors.New("需为 IP 地址或 CIDR 网段")
	}
	if p.Addr().Is4In6() {
		if p.Bits() < 96 {
			return netip.Prefix{}, errors.New("需为 IP 地址或 CIDR 网段")
		}
		p = netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96)
	}
	return p.Masked(), nil
}

// lanPrefixes 是“内网免敲门”包含的网段：私有地址、链路本地与 IPv6 ULA。回环地址由防火墙规则单独放行（iif lo）。
// 不含运营商级 NAT 的 100.64.0.0/10：CGNAT 下同一运营商的其他用户也使用这段地址，纳入后他们无需敲门；
// Tailscale 同样使用该段，需要时由用户加到免敲门白名单。
var lanPrefixes = []netip.Prefix{
	netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("172.16.0.0/12"), netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("fc00::/7"), netip.MustParsePrefix("fe80::/10"),
}

// localPrefixes 返回本机网卡所在的公网 IPv6 网段（家庭网络常把同一个 /64 分给局域网全部设备），可在测试中替换。
var localPrefixes = func() []netip.Prefix {
	var out []netip.Prefix
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			p, ok := localPrefix(ipn)
			if ok && !slices.Contains(out, p) {
				out = append(out, p)
			}
		}
	}
	slices.SortFunc(out, func(a, b netip.Prefix) int { return a.Addr().Compare(b.Addr()) })
	return out
}

// localPrefix 返回网卡地址按实际前缀长度计算的免敲门网段。只处理公网 IPv6：网卡前缀为 /64 或更短时
// （家庭宽带把 /64 分给局域网）按该前缀纳入；更长的前缀（云主机常见的 /124、/128）只纳入网卡自身的前缀，
// 不扩大到 /64，免得同一 /64 中其他租户的主机无需敲门。
func localPrefix(ipn *net.IPNet) (netip.Prefix, bool) {
	if ipn.IP.To4() != nil {
		return netip.Prefix{}, false
	}
	ip, ok := netip.AddrFromSlice(ipn.IP)
	ones, total := ipn.Mask.Size()
	if !ok || total != 128 || ones == 0 || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return netip.Prefix{}, false
	}
	return netip.PrefixFrom(ip, ones).Masked(), true
}

// exemptPrefixes 返回无需敲门的网段。
func (s Settings) exemptPrefixes(local []netip.Prefix) []netip.Prefix {
	var out []netip.Prefix
	if s.AllowLAN {
		out = append(out, lanPrefixes...)
		out = append(out, local...)
	}
	for _, v := range s.AllowList {
		if p, err := parsePrefix(v); err == nil {
			out = append(out, p)
		}
	}
	return out
}

type fileConfig struct {
	Settings Settings `json:"settings"`
	Rules    []Rule   `json:"rules"`
}

// compiledRule 是收包协程使用的规则快照。
type compiledRule struct {
	rule  Rule
	key   spa.Key
	ports []spa.Access
	// keys 为规则接受敲门的密钥：共享密钥（未停用时）在前，其后为启用的身份，按顺序尝试
	keys []compiledKey
	// shared 为共享密钥的凭据标识，停用共享密钥时也计算，用于识别编辑前后凭据是否变化
	shared string
}

// compiledKey 是一个接受敲门的密钥。
type compiledKey struct {
	key spa.Key
	// id、name 为身份的 ID 与名称，共享密钥为空
	id, name string
	// cred 为凭据标识：身份 ID（共享密钥为空）加密钥指纹，记录在授权上（knockgate.Owner.Cred）。
	// 编辑规则后按它判断哪些授权的密钥已失效，单地址放行按它找出同一身份的其他授权
	cred string
}

// credID 返回凭据标识：id 为身份 ID（共享密钥为空），指纹取密钥与 HMAC 算法的 SHA-256 前 8 字节。
// 更换任一密钥或算法后指纹改变，旧授权随之失效。
func credID(id string, k spa.Key) string {
	h := sha256.New()
	h.Write([]byte(k.HMACType))
	h.Write([]byte{0})
	h.Write(k.Enc)
	h.Write([]byte{0})
	h.Write(k.HMAC)
	return id + ":" + hex.EncodeToString(h.Sum(nil)[:8])
}

// identityOf 返回凭据标识中的身份 ID（共享密钥为空串），ok 为 false 表示不是敲门报文的凭据标识。
func identityOf(cred string) (id string, ok bool) {
	i := strings.LastIndexByte(cred, ':')
	if i < 0 {
		return "", false
	}
	return cred[:i], true
}

// snapshot 是收包协程无锁读取的配置快照。
type snapshot struct {
	settings Settings
	rules    []*compiledRule
	// keys 为全部规则接受敲门的密钥（按规则、规则内顺序排列），refs 为对应的规则与密钥，收包时用 spa.DecodeAny 依次尝试；
	// 密钥数决定伪造报文的校验成本（见 packetCost）
	keys []spa.Key
	refs []keyRef
}

// keyRef 指向快照中某条规则的某个密钥。
type keyRef struct {
	rule *compiledRule
	key  *compiledKey
}

// ruleStats 是规则的运行统计（内存中，重启清零）。
type ruleStats struct {
	Knocks   uint64    `json:"knocks"`
	Rejected uint64    `json:"rejected"`
	LastAt   time.Time `json:"lastAt,omitzero"`
	LastAddr string    `json:"lastAddr,omitempty"`
}

// fwGrantKey 是写入防火墙的一项放行。
type fwGrantKey struct {
	addr   netip.Addr
	access spa.Access
	rule   string
	// web 为网页登录授权同步来的放行（见 syncWebFirewall），与同一地址的敲门报文放行分开记录，撤销一方不影响另一方
	web bool
}

// Module 是端口敲门模块。
type Module struct {
	mu    sync.Mutex
	store *config.Store
	logs  *logbuf.Buffer
	cfg   fileConfig

	ctx     context.Context
	cancel  context.CancelFunc
	started bool
	local   []netip.Prefix
	// spaPorts 为上次计算防火墙规则时协议分流接收敲门报文的端口，变化时重新计算
	spaPorts []int

	snap atomic.Pointer[snapshot]
	fw   *firewall
	open portOpener

	// conn 为 UDP 监听，listenErr 为监听失败的原因；均受 mu 保护
	conn      net.PacketConn
	connPort  int
	connDone  chan struct{}
	listenErr string

	// stateMu 保护运行统计、防火墙放行记录与限速状态，收包协程与接口都会访问
	stateMu sync.Mutex
	stats   map[string]*ruleStats
	// idStats 为各规则中身份的运行统计：规则 key -> 身份 ID
	idStats  map[string]map[string]*ruleStats
	fwGrants map[fwGrantKey]time.Time
	replay   map[string]time.Time
	// startedAt 为模块本次启动的时间：防重放缓存只在内存中，早于此刻的报文无法确认是否已被使用过
	startedAt time.Time
	counters  Counters
	// bucket 为全局令牌桶，perSrc 为按来源网段的令牌桶，均在 HMAC 认证之前判断
	bucket tokenBucket
	perSrc *srcLimiter
	// failSeen 为各来源最近一次记录失败日志、发布失败事件的时间，用于限速
	failSeen map[netip.Addr]time.Time
	// invalid 汇总 HMAC 认证失败的报文，按时间窗记一条日志
	invalid invalidStats

	// singleMu 串行化开启了单地址放行的规则的授权登记（见 grant）
	singleMu sync.Mutex

	// webKick 在网页登录授权变化后通知 loop 同步防火墙放行（见 syncWebFirewall）
	webKick chan struct{}

	// now 可在测试中替换
	now func() time.Time
}

// portOpener 在系统防火墙（fw4、firewalld 等）自动放行端口，即 *fwopen.Group，可在测试中替换。
type portOpener interface {
	Want(key string, ports []fwopen.Port, log *logbuf.Logger)
	Status(key string) string
	ClearAll()
}

// Counters 是全部报文的处理结果统计。
type Counters struct {
	// Accepted 为认证通过并放行的报文数
	Accepted uint64 `json:"accepted"`
	// Rejected 为格式像敲门报文、但认证失败或被规则拒绝的报文数
	Rejected uint64 `json:"rejected"`
	// Ignored 为不是敲门报文、来源已被封禁或超出处理速率而直接丢弃的报文数
	Ignored uint64 `json:"ignored"`
}

// New 创建模块。
func New() *Module {
	return &Module{now: time.Now, stats: map[string]*ruleStats{}, idStats: map[string]map[string]*ruleStats{}, fwGrants: map[fwGrantKey]time.Time{}, webKick: make(chan struct{}, 1),
		replay: map[string]time.Time{}, failSeen: map[netip.Addr]time.Time{}, bucket: tokenBucket{rate: globalRate, burst: globalBurst},
		perSrc: newSrcLimiter()}
}

// ID 实现 module.Module。
func (m *Module) ID() string { return ID }

// Title 实现 module.Module。
func (m *Module) Title() string { return "端口敲门" }

// Init 实现 module.Module。
func (m *Module) Init(env module.Env) error {
	m.store, m.logs = env.Store, env.Logs
	m.cfg = fileConfig{Settings: defaultSettings(), Rules: []Rule{}}
	if _, err := m.store.Load(ID, &m.cfg); err != nil {
		return err
	}
	if err := m.cfg.Settings.Validate(); err != nil {
		m.logs.Add(logbuf.LevelWarn, "", "设置无效，已恢复默认值: "+err.Error())
		m.cfg.Settings = defaultSettings()
	}
	if m.cfg.Rules == nil {
		m.cfg.Rules = []Rule{}
	}
	// 配置文件中密钥留空（手写配置或导入）时 Validate 会生成新密钥：必须保存，否则每次启动密钥都不同，
	// 已配置好的客户端在重启后全部失效
	generated := false
	for i := range m.cfg.Rules {
		r := &m.cfg.Rules[i]
		noKey := strings.TrimSpace(r.EncKey) == "" || strings.TrimSpace(r.HMACKey) == ""
		for _, id := range r.Identities {
			// 身份的密钥留空同样会生成新密钥，ID 留空会分配新 ID，都要保存
			if strings.TrimSpace(id.EncKey) == "" || strings.TrimSpace(id.HMACKey) == "" || id.ID == "" {
				noKey = true
			}
		}
		if err := r.Validate(); err != nil {
			m.logs.Add(logbuf.LevelWarn, r.Name, "规则无效，已停用: "+err.Error())
			r.Enable = false
			continue
		}
		if noKey {
			generated = true
			m.logs.Logger(r.Name).Infof("规则没有密钥，已生成新密钥并保存")
		}
	}
	crud.AssignKeys(len(m.cfg.Rules), func(i int) *string { return &m.cfg.Rules[i].Key }, "k")
	if generated {
		if err := m.saveLocked(); err != nil {
			m.logs.Logger("").Warnf("保存生成的密钥失败，下次启动会重新生成: %v", err)
		}
	}
	m.fw = newFirewall(m.logs.Logger("防火墙"))
	m.open = fwopen.NewGroup("knock")
	fwopen.Setup(m.store.Dir())
	m.publishLocked()
	return nil
}

// Start 实现 module.Module。
func (m *Module) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.started = true
	m.stateMu.Lock()
	m.startedAt = m.now()
	m.stateMu.Unlock()
	m.local = localPrefixes()
	m.fw.start(m.ctx, m.onFirewallActive)
	m.applyLocked()
	knockgate.Default.SetSubmitter(m.submit)
	knockgate.Default.SetOnChange(m.kickWeb)
	go m.loop(m.ctx)
	m.kickWeb()
	return nil
}

// Stop 实现 module.Module。停止监听并删除防火墙规则，撤销全部授权。
func (m *Module) Stop() error {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return nil
	}
	m.started = false
	knockgate.Default.SetSubmitter(nil)
	knockgate.Default.SetOnChange(nil)
	m.closeListenerLocked()
	m.cancel()
	m.mu.Unlock()
	// 先同步撤销系统防火墙中对受保护端口的自动放行，再删除拦截规则：反过来时两步之间受保护端口完全开放
	m.open.ClearAll()
	m.fw.stop()
	knockgate.Default.Clear()
	knockgate.Default.SetProtected(nil)
	m.stateMu.Lock()
	m.fwGrants = map[fwGrantKey]time.Time{}
	m.stateMu.Unlock()
	return nil
}

func (m *Module) saveLocked() error { return m.store.Save(ID, m.cfg) }

// publishLocked 更新收包快照与对外发布的规则目录。调用方持有 m.mu。
func (m *Module) publishLocked() {
	snap := &snapshot{settings: m.cfg.Settings}
	cat := make([]knockgate.Rule, 0, len(m.cfg.Rules))
	for _, r := range m.cfg.Rules {
		cat = append(cat, knockgate.Rule{Key: r.Key, Name: r.Name, Enable: r.Enable})
		if !r.Enable {
			continue
		}
		k, err := r.spaKey()
		if err != nil {
			continue
		}
		ports, _ := parsePorts(r.Ports)
		cr := &compiledRule{rule: r, key: k, ports: ports, shared: credID("", k)}
		if !r.SharedKeyOff {
			cr.keys = append(cr.keys, compiledKey{key: k, cred: cr.shared})
		}
		for _, id := range r.Identities {
			if !id.Enable {
				continue
			}
			ik, err := id.spaKey(r.HMACType)
			if err != nil {
				continue
			}
			cr.keys = append(cr.keys, compiledKey{key: ik, id: id.ID, name: id.Name, cred: credID(id.ID, ik)})
		}
		snap.rules = append(snap.rules, cr)
		// 没有端口的规则也参与尝试（只有共享密钥）：匹配后由 check 给出“只接受网页登录”的明确拒绝原因
		for i := range cr.keys {
			snap.keys = append(snap.keys, cr.keys[i].key)
			snap.refs = append(snap.refs, keyRef{rule: cr, key: &cr.keys[i]})
		}
	}
	m.snap.Store(snap)
	knockgate.Default.SetRules(cat)
}

// applyLocked 按当前配置更新监听、防火墙与自动放行。调用方持有 m.mu。
func (m *Module) applyLocked() {
	m.publishLocked()
	if !m.started {
		return
	}
	s := m.cfg.Settings
	snap := m.snap.Load()
	// 没有接受敲门报文的启用规则时不监听，避免无意义地暴露端口：没有端口的规则只能经网页登录放行
	wantPort := 0
	for _, r := range snap.rules {
		if len(r.ports) > 0 {
			wantPort = s.Port
			break
		}
	}
	if wantPort != m.connPort || (wantPort != 0 && m.conn == nil) {
		m.closeListenerLocked()
		if wantPort != 0 {
			m.openListenerLocked(wantPort)
		}
	}
	m.spaPorts = knockgate.Default.SPAPorts()
	m.fw.apply(m.ctx, m.planLocked(), m.liveFWGrants)
	m.updateOpenLocked()
	// 规则的端口、防火墙拦截开关可能变化：重新同步网页登录授权的防火墙放行
	m.kickWeb()
}

// planLocked 计算防火墙规则：开启防火墙拦截的启用规则的端口与免敲门网段（放行由 liveFWGrants 在写入时取快照）。
// 本模块的 UDP 敲门端口与协议分流接收敲门报文的 TCP 端口始终排除：保存规则时已检查冲突，
// 这里兜底处理手改配置文件、之后修改敲门端口等情况，拦截这些端口会让敲门报文本身无法送达。
func (m *Module) planLocked() fwPlan {
	plan := fwPlan{}
	exclude := knockExcluded(m.cfg.Settings.Port)
	for _, r := range m.snap.Load().rules {
		if !r.rule.Firewall {
			continue
		}
		for _, a := range r.ports {
			if !slices.Contains(plan.Ports, a) && !slices.Contains(exclude, a) {
				plan.Ports = append(plan.Ports, a)
			}
		}
	}
	sortAccess(plan.Ports)
	knockgate.Default.SetProtected(formatPorts(plan.Ports))
	if len(plan.Ports) == 0 {
		return plan
	}
	plan.Exempt = m.cfg.Settings.exemptPrefixes(m.local)
	return plan
}

// liveFWGrants 返回仍有效、且端口在 ports 中的防火墙放行，并清除到期记录。
// 由 firewall.apply 在持有防火墙锁时调用：快照与清空待写入放行在同一把锁内完成，收包协程在快照之后登记的放行
// 留在待写入队列中，不会被重建时用旧快照覆盖而丢失；快照之前登记的放行已在 fwGrants 中，包含在快照里。
func (m *Module) liveFWGrants(ports []spa.Access) []fwGrant {
	now := m.now()
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	var out []fwGrant
	for k, until := range m.fwGrants {
		if !now.Before(until) || !slices.Contains(ports, k.access) {
			delete(m.fwGrants, k)
			continue
		}
		out = append(out, fwGrant{Addr: k.addr, Access: k.access, Until: until})
	}
	return out
}

// knockExcluded 返回不能在防火墙中拦截的端口：UDP 敲门端口（port 为 0 时没有）与协议分流接收敲门报文的 TCP 端口。
func knockExcluded(port int) []spa.Access {
	var out []spa.Access
	if port != 0 {
		out = append(out, spa.Access{Proto: "udp", Port: port})
	}
	for _, p := range knockgate.Default.SPAPorts() {
		out = append(out, spa.Access{Proto: "tcp", Port: p})
	}
	return out
}

// updateOpenLocked 按设置在系统防火墙放行敲门端口；拦截规则生效后也放行受保护端口（访问仍由敲门控制）。
func (m *Module) updateOpenLocked() {
	s := m.cfg.Settings
	var listen []fwopen.Port
	if s.AutoFirewall && m.conn != nil {
		listen = []fwopen.Port{{Proto: "udp", Port: m.connPort}}
	}
	m.open.Want("listen", listen, m.logs.Logger(""))
	var ports []fwopen.Port
	if s.AutoFirewall && m.fw.active() {
		for _, a := range m.fw.ports() {
			ports = append(ports, fwopen.Port{Proto: a.Proto, Port: a.Port})
		}
	}
	m.open.Want("ports", ports, m.logs.Logger("防火墙"))
}

// onFirewallActive 在防火墙拦截规则生效状态变化（重试成功、被外部删除后重写失败）时调用，同步自动放行。
func (m *Module) onFirewallActive() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		m.updateOpenLocked()
	}
}

func (m *Module) openListenerLocked(port int) {
	conn, err := net.ListenPacket("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		m.listenErr = err.Error()
		m.connPort = port
		m.logs.Logger("").Errorf("监听 UDP %d 失败: %v", port, err)
		return
	}
	m.conn, m.connPort, m.listenErr = conn, port, ""
	m.connDone = make(chan struct{})
	m.logs.Logger("").Infof("开始在 UDP %d 接收敲门报文", port)
	go m.serve(conn, m.connDone)
}

func (m *Module) closeListenerLocked() {
	if m.conn != nil {
		_ = m.conn.Close()
		<-m.connDone
		m.logs.Logger("").Infof("已停止在 UDP %d 接收敲门报文", m.connPort)
	}
	m.conn, m.connDone, m.connPort, m.listenErr = nil, nil, 0, ""
}

// serve 读取 UDP 报文并逐个处理。处理只做内存与哈希计算，放行写防火墙由防火墙协程异步完成，不会阻塞收包。
func (m *Module) serve(conn net.PacketConn, done chan struct{}) {
	defer close(done)
	buf := make([]byte, spa.MaxPacketLen+1)
	for {
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		ua, ok := addr.(*net.UDPAddr)
		if !ok {
			continue
		}
		src, ok := netip.AddrFromSlice(ua.IP)
		if !ok {
			continue
		}
		_ = m.handle(buf[:n], src.Unmap(), transportUDP, "UDP")
	}
}

// submit 处理经协议分流发来的报文（knockgate.Submitter）。协议分流只经 TCP 连接转交报文，来源地址经过握手。
func (m *Module) submit(packet []byte, src netip.Addr, via string) error {
	return m.handle(packet, src, transportTCP, via)
}

// 报文的传输方式，写入失败事件的 via 字段。UDP 报文的来源地址可以伪造，TCP 连接的来源地址经过三次握手、无法伪造
const (
	transportUDP = "udp"
	transportTCP = "tcp"
)

// 处理结果，供调用方（协议分流）记录日志
var (
	errIgnored = errors.New("不是有效的敲门报文")
	errBanned  = errors.New("来源已被封禁")
	errLimited = errors.New("敲门报文过多，已丢弃")
)

// handle 认证并处理一个报文。transport 为 transportUDP 或 transportTCP，via 为日志中的通道说明。
func (m *Module) handle(packet []byte, src netip.Addr, transport, via string) error {
	if !src.IsValid() {
		return errIgnored
	}
	if spa.Prefilter(packet) != nil {
		m.count(func(c *Counters) { c.Ignored++ })
		return errIgnored
	}
	if banlist.Default.Banned(src) {
		m.count(func(c *Counters) { c.Ignored++ })
		return errBanned
	}
	now := m.now()
	// 先按来源网段限速，再过全局令牌桶：单个来源（或伪造来源集中的网段）的洪水只耗尽自己网段的令牌，
	// 不会占满全局速率、挤掉其他来源的合法敲门
	snap := m.snap.Load()
	m.stateMu.Lock()
	ok := m.perSrc.take(src, now) && m.bucket.takeN(now, packetCost(len(packet), len(snap.keys)))
	m.stateMu.Unlock()
	if !ok {
		m.count(func(c *Counters) { c.Ignored++ })
		return errLimited
	}
	i, msg, err := spa.DecodeAny(packet, snap.keys)
	if i < 0 {
		m.reject(nil, nil, src, transport, via, "没有规则的密钥能通过 HMAC 认证")
		return errors.New("认证失败")
	}
	rule, key := snap.refs[i].rule, snap.refs[i].key
	if err != nil {
		// HMAC 正确而解密失败：HMAC 密钥一致，加密密钥不一致
		m.reject(rule, key, src, transport, via, "HMAC 认证通过但解密失败，请检查加密密钥是否一致")
		return errors.New("解密失败")
	}
	if reason := m.check(rule, msg, snap.settings, now); reason != "" {
		m.reject(rule, key, src, transport, via, reason)
		return errors.New(reason)
	}
	addr := msg.Addr
	if !addr.IsValid() || addr.IsUnspecified() {
		addr = src
	}
	if reason := checkAddr(addr); reason != "" {
		m.reject(rule, key, src, transport, via, reason)
		return errors.New(reason)
	}
	ttl := effectiveTimeout(rule.rule, msg.ClientTimeout)
	m.grant(rule, key, addr, src, msg, ttl, via)
	return nil
}

// check 校验已解密消息：类型、时间戳、防重放、用户名、地址与端口，返回拒绝原因，空表示通过。
func (m *Module) check(r *compiledRule, msg *spa.Message, s Settings, now time.Time) string {
	maxAge := time.Duration(s.MaxAge) * time.Second
	ts := time.Unix(msg.Timestamp, 0)
	if d := now.Sub(ts); d > maxAge || d < -maxAge {
		return fmt.Sprintf("报文时间戳与本机时间相差 %s，超过有效期 %d 秒（请检查两端的时钟）", d.Round(time.Second).Abs(), s.MaxAge)
	}
	// 防重放：每个报文的摘要只接受一次。摘要按报文时间戳加允许的最大有效期（maxMaxAge）保留，而不是写入时的 MaxAge：
	// 之后调大有效期时，旧报文的摘要仍在缓存中，不会因此可以重放
	m.stateMu.Lock()
	// 缓存只在内存中，重启后为空：时间戳早于本次启动的报文可能在重启前已经用过，一律拒绝。
	// 客户端时钟比本机慢时，重启后的头几秒内新敲门也会被拒绝，重新敲门即可
	if ts.Unix() < m.startedAt.Unix() {
		m.stateMu.Unlock()
		return "报文时间早于 Gately 本次启动，无法确认是否为重放，请重新敲门"
	}
	if until, seen := m.replay[msg.Digest]; seen && now.Before(until) {
		m.stateMu.Unlock()
		return "重放的报文（同一报文只能使用一次）"
	}
	if len(m.replay) >= replayCap {
		for k, until := range m.replay {
			if !now.Before(until) {
				delete(m.replay, k)
			}
		}
	}
	if len(m.replay) >= replayCap {
		m.stateMu.Unlock()
		return "防重放缓存已满，暂时拒绝新的敲门"
	}
	m.replay[msg.Digest] = ts.Add(maxMaxAge*time.Second + time.Second)
	m.stateMu.Unlock()
	if !msg.IsAccess() {
		switch msg.Type {
		case spa.CommandMsg:
			return "不支持命令消息（fwknop -C），Gately 不执行远程命令"
		default:
			return "不支持 NAT 访问请求（fwknop -N）"
		}
	}
	if r.rule.User != "" && msg.User != r.rule.User {
		return fmt.Sprintf("用户名 %q 与规则要求的不一致", msg.User)
	}
	if r.rule.RequireAddr && (!msg.Addr.IsValid() || msg.Addr.IsUnspecified()) {
		return "规则要求报文写明放行地址（fwknop -a 或 -R），拒绝 -s 报文"
	}
	if len(r.ports) == 0 {
		return "规则没有填写端口，只接受 Web 站点放行入口的登录，不接受敲门报文"
	}
	for _, a := range msg.Access {
		if !slices.Contains(r.ports, a) {
			return fmt.Sprintf("请求的端口 %s 不在规则允许的范围内", a)
		}
	}
	return ""
}

// checkAddr 拒绝无法作为放行对象的地址。
func checkAddr(a netip.Addr) string {
	if a.IsMulticast() || a.IsUnspecified() || a.IsLoopback() || a.IsInterfaceLocalMulticast() {
		return fmt.Sprintf("不能放行地址 %s", a)
	}
	return ""
}

// effectiveTimeout 返回放行时长：客户端请求了时长（-f）时，比规则时长短就采用；更长时以 MaxTimeout 为上限，MaxTimeout 为 0 时不采用。
func effectiveTimeout(r Rule, client int) time.Duration {
	sec := r.Timeout
	switch {
	case client <= 0:
	case client < r.Timeout:
		sec = max(client, minTimeout)
	case r.MaxTimeout > 0:
		sec = min(client, r.MaxTimeout)
	}
	return time.Duration(sec) * time.Second
}

// grant 登记授权、写防火墙并记录日志与事件。规则开启“单地址放行”时先撤销同一身份此前放行的其他地址。
func (m *Module) grant(r *compiledRule, key *compiledKey, addr, src netip.Addr, msg *spa.Message, ttl time.Duration, via string) {
	if r.rule.SingleAddr {
		// 登记与撤销其他地址必须串行：同一身份从两个地址同时敲门（协议分流的 TCP 报文可并发处理）时，
		// 交错执行会让两次敲门互相撤销，两个地址都没有放行
		m.singleMu.Lock()
		defer m.singleMu.Unlock()
	}
	now := m.now()
	until := now.Add(ttl)
	if err := knockgate.Default.AllowSPA(addr, r.rule.Key, until, knockgate.Owner{User: key.name, Cred: key.cred}); err != nil {
		m.logs.Logger(r.rule.Name).Warnf("登记 %s 的授权失败: %v", addr, err)
	}
	var replaced []netip.Addr
	if r.rule.SingleAddr {
		replaced = m.revokeOtherAddrs(r.rule.Key, key.id, addr)
	}
	access := msg.Access
	var fwList []fwGrant
	if r.rule.Firewall {
		m.stateMu.Lock()
		for _, a := range access {
			k := fwGrantKey{addr: addr, access: a, rule: r.rule.Key}
			if old, ok := m.fwGrants[k]; !ok || until.After(old) {
				m.fwGrants[k] = until
			}
			fwList = append(fwList, fwGrant{Addr: addr, Access: a, Until: until})
		}
		m.stateMu.Unlock()
		m.fw.grant(fwList)
	}
	m.stateMu.Lock()
	m.counters.Accepted++
	st := m.statsLocked(r.rule.Key)
	st.Knocks++
	st.LastAt, st.LastAddr = now, addr.String()
	if key.id != "" {
		ist := m.identityStatsLocked(r.rule.Key, key.id)
		ist.Knocks++
		ist.LastAt, ist.LastAddr = now, addr.String()
	}
	m.stateMu.Unlock()
	ports := strings.Join(formatPorts(access), ", ")
	extra := ""
	if addr != src {
		extra = fmt.Sprintf("（报文来自 %s）", src)
	}
	user := msg.User
	if user == "" {
		user = "-"
	}
	log := m.logs.Logger(r.rule.Name)
	// 日志按整句翻译，有无身份分成两个格式串
	if key.name != "" {
		log.Infof("敲门成功：放行 %s 访问 %s %d 秒，身份 %s，用户 %s，经 %s%s", addr, ports, int(ttl/time.Second), key.name, user, via, extra)
	} else {
		log.Infof("敲门成功：放行 %s 访问 %s %d 秒，用户 %s，经 %s%s", addr, ports, int(ttl/time.Second), user, via, extra)
	}
	if len(replaced) > 0 {
		if key.name != "" {
			log.Infof("单地址放行：已撤销身份 %s 此前放行的 %s", key.name, joinAddrs(replaced))
		} else {
			log.Infof("单地址放行：已撤销共享密钥此前放行的 %s", joinAddrs(replaced))
		}
	}
	fields := map[string]string{"ip": addr.String(), "rule": r.rule.Name, "ports": ports}
	msgText := fmt.Sprintf("%s：%s 敲门成功，放行 %s %d 秒", r.rule.Name, addr, ports, int(ttl/time.Second))
	if key.name != "" {
		fields["identity"] = key.name
		msgText = fmt.Sprintf("%s：%s（%s）敲门成功，放行 %s %d 秒", r.rule.Name, addr, key.name, ports, int(ttl/time.Second))
	}
	events.Publish(events.Event{
		Type:    events.KnockOpened,
		Source:  r.rule.Name,
		Level:   events.LevelInfo,
		Title:   fmt.Sprintf("端口敲门放行了 %s", addr),
		Message: msgText,
		Fields:  fields,
	})
}

func joinAddrs(list []netip.Addr) string {
	out := make([]string, 0, len(list))
	for _, a := range list {
		out = append(out, a.String())
	}
	return strings.Join(out, "、")
}

// revokeOtherAddrs 撤销 rule 中身份 id（共享密钥为空）经敲门报文放行的、除 keep 之外的其他地址及其防火墙放行，
// 返回撤销的地址。网页登录的授权不受影响。
func (m *Module) revokeOtherAddrs(rule, id string, keep netip.Addr) []netip.Addr {
	var addrs []netip.Addr
	keepPrefix := netip.PrefixFrom(keep, keep.BitLen())
	knockgate.Default.RevokeFunc(func(g knockgate.Grant) bool {
		if g.Rule != rule || g.Source != knockgate.SourceSPA || g.Prefix == keepPrefix {
			return false
		}
		if gid, ok := identityOf(g.Cred); !ok || gid != id {
			return false
		}
		addrs = append(addrs, g.Addr())
		return true
	})
	for _, a := range addrs {
		m.dropFW(a, rule)
	}
	return addrs
}

// failInterval 为同一来源两次失败日志与失败事件之间的最短间隔：持续发送伪造报文的来源每个间隔只记一次，
// 入侵防护按事件计数时也足以在几分钟内封禁
const failInterval = 5 * time.Second

// reject 统计一次被拒绝的敲门；同一来源每 failInterval 最多记录一条日志、发布一个失败事件（供入侵防护计数封禁）。
//
// 失败事件会被入侵防护用来封禁事件中的地址，所以只为来源不可伪造的报文发布：经 TCP（协议分流）发来的报文来源经过握手，
// 照常发布；UDP 报文的来源地址可以伪造，一律只写日志、不发布。HMAC 认证失败的 UDP 包任何人都能以任意来源地址发出；
// 通过 HMAC 认证的 UDP 包也可能是抓到的合法报文被重放、再伪造来源发出的，发布事件同样会让攻击者借入侵防护封禁任意 IP。
// HMAC 认证失败的报文不逐条记录日志，由 flushInvalid 每分钟汇总一条。
//
// key 为匹配到的密钥（r 为 nil 时为 nil），身份的密钥同时计入该身份的拒绝次数，日志中写明身份。
func (m *Module) reject(r *compiledRule, key *compiledKey, src netip.Addr, transport, via, reason string) {
	now := m.now()
	m.stateMu.Lock()
	m.counters.Rejected++
	name, ident := "", ""
	if r != nil {
		name = r.rule.Name
		m.statsLocked(r.rule.Key).Rejected++
		if key != nil && key.id != "" {
			ident = key.name
			m.identityStatsLocked(r.rule.Key, key.id).Rejected++
		}
	}
	last, seen := m.failSeen[src]
	report := !seen || now.Sub(last) >= failInterval
	if r == nil {
		// HMAC 认证失败：来源可能是伪造的，也可能是扫描器的随机数据，不逐个记录日志，按时间窗汇总（见 flushInvalid）
		m.invalid.add(src, now)
		if transport != transportTCP {
			report = false
		}
	}
	if report {
		if len(m.failSeen) >= 4096 {
			for k, t := range m.failSeen {
				if now.Sub(t) >= failInterval {
					delete(m.failSeen, k)
				}
			}
		}
		if len(m.failSeen) < 4096 {
			m.failSeen[src] = now
		} else {
			// 来源过多（分布式伪造）：不再逐个记录，避免日志与事件被刷屏
			report = false
		}
	}
	m.stateMu.Unlock()
	if !report {
		return
	}
	if r != nil {
		if ident != "" {
			m.logs.Logger(name).Warnf("拒绝来自 %s 的敲门（身份 %s，经 %s）：%s", src, ident, via, reason)
		} else {
			m.logs.Logger(name).Warnf("拒绝来自 %s 的敲门（经 %s）：%s", src, via, reason)
		}
	}
	if transport != transportTCP {
		// UDP 来源可以伪造（含重放的合法报文），只写日志，不发布供入侵防护计数的事件
		return
	}
	fields := map[string]string{"ip": src.String(), "reason": reason, "via": transport}
	if ident != "" {
		fields["identity"] = ident
	}
	events.Publish(events.Event{
		Type:    events.KnockFailed,
		Source:  name,
		Level:   events.LevelWarn,
		Title:   fmt.Sprintf("端口敲门拒绝了 %s", src),
		Message: reason,
		Fields:  fields,
	})
}

func (m *Module) count(f func(*Counters)) {
	m.stateMu.Lock()
	f(&m.counters)
	m.stateMu.Unlock()
}

// identityStatsLocked 返回规则中某个身份的运行统计。调用方持有 stateMu。
func (m *Module) identityStatsLocked(rule, id string) *ruleStats {
	byID := m.idStats[rule]
	if byID == nil {
		byID = map[string]*ruleStats{}
		m.idStats[rule] = byID
	}
	st := byID[id]
	if st == nil {
		st = &ruleStats{}
		byID[id] = st
	}
	return st
}

func (m *Module) statsLocked(key string) *ruleStats {
	st := m.stats[key]
	if st == nil {
		st = &ruleStats{}
		m.stats[key] = st
	}
	return st
}

// loop 定期清理到期的授权、防重放缓存与限速记录，并刷新本机所在的 IPv6 网段。
func (m *Module) loop(ctx context.Context) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	tick := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.webKick:
			m.syncWebFirewall()
			continue
		case <-t.C:
		}
		now := m.now()
		knockgate.Default.Prune()
		m.stateMu.Lock()
		for k, until := range m.replay {
			if !now.Before(until) {
				delete(m.replay, k)
			}
		}
		for k, until := range m.fwGrants {
			if !now.Before(until) {
				delete(m.fwGrants, k)
			}
		}
		for k, at := range m.failSeen {
			if now.Sub(at) >= failInterval {
				delete(m.failSeen, k)
			}
		}
		m.perSrc.prune(now)
		m.stateMu.Unlock()
		m.flushInvalid(now)
		tick++
		m.refreshSPAPorts()
		if tick%4 == 0 {
			m.refreshLocal()
		}
	}
}

// refreshSPAPorts 在协议分流接收敲门报文的端口变化后重新计算防火墙规则（排除这些端口）。
func (m *Module) refreshSPAPorts() {
	ports := knockgate.Default.SPAPorts()
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.started || slices.Equal(ports, m.spaPorts) {
		return
	}
	m.spaPorts = ports
	m.fw.apply(m.ctx, m.planLocked(), m.liveFWGrants)
	m.updateOpenLocked()
}

// refreshLocal 在本机 IPv6 网段变化（运营商重新分配前缀）时更新免敲门网段。
func (m *Module) refreshLocal() {
	local := localPrefixes()
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.started || slices.Equal(local, m.local) {
		return
	}
	m.local = local
	if m.cfg.Settings.AllowLAN {
		m.fw.apply(m.ctx, m.planLocked(), m.liveFWGrants)
	}
}

// revoke 撤销 addr 持有的授权（rule 为空时撤销该地址的全部授权），并从防火墙中删除对应放行。返回撤销的数量。
func (m *Module) revoke(addr netip.Addr, rule string) int {
	n := knockgate.Default.Revoke(addr, rule)
	m.dropFW(addr, rule)
	return n
}

// dropFW 删除 addr 经敲门报文取得的 rule（为空时全部规则）防火墙放行；同一地址、端口仍被其他规则或网页登录放行时保留。
func (m *Module) dropFW(addr netip.Addr, rule string) {
	var del []fwGrant
	m.stateMu.Lock()
	for k := range m.fwGrants {
		// 网页登录授权的放行由 syncWebFirewall 按登记表同步：这里撤销的只是单个地址的授权，
		// 同一地址所在 IPv6 网段的网页授权仍然有效，不能删掉它写入的放行
		if k.web {
			continue
		}
		if k.addr == addr && (rule == "" || k.rule == rule) {
			delete(m.fwGrants, k)
			del = append(del, fwGrant{Addr: k.addr, Access: k.access})
		}
	}
	// 同一地址、端口仍被其他规则放行时保留防火墙中的元素
	keep := del[:0]
	for _, g := range del {
		still := false
		for k := range m.fwGrants {
			if k.addr == g.Addr && k.access == g.Access {
				still = true
				break
			}
		}
		if !still {
			keep = append(keep, g)
		}
	}
	m.stateMu.Unlock()
	m.fw.revoke(keep)
}

// kickWeb 通知 loop 同步网页登录授权的防火墙放行；已有待处理的通知时直接返回，不阻塞调用方（授权登记表的回调）。
func (m *Module) kickWeb() {
	select {
	case m.webKick <- struct{}{}:
	default:
	}
}

// syncWebFirewall 把网页登录（Web 站点的放行入口）取得的授权同步到系统防火墙：规则开启了防火墙拦截时，
// 为授权上记录的每个实际地址放行规则的全部端口，到期时间与授权相同；授权撤销或到期后删除对应放行。
//
// 只写入实际登录、探测过的单个地址：进程内按网段（默认 IPv6 /64）判断，系统防火墙的放行集合只存单个地址
// （nftables 拼接集合、ipset hash:ip 在各系统上都支持，群晖只有 hash:ip）。IPv6 临时地址轮换后重新打开放行页面即可。
func (m *Module) syncWebFirewall() {
	fwRules := map[string][]spa.Access{}
	for _, r := range m.snap.Load().rules {
		if r.rule.Firewall && len(r.ports) > 0 {
			fwRules[r.rule.Key] = r.ports
		}
	}
	now := m.now()
	want := map[fwGrantKey]time.Time{}
	for _, g := range knockgate.Default.List() {
		ports := fwRules[g.Rule]
		if g.Source != knockgate.SourceWeb || len(ports) == 0 || !now.Before(g.Until) {
			continue
		}
		for _, a := range g.Addrs {
			for _, p := range ports {
				k := fwGrantKey{addr: a, access: p, rule: g.Rule, web: true}
				if g.Until.After(want[k]) {
					want[k] = g.Until
				}
			}
		}
	}
	var add, del []fwGrant
	m.stateMu.Lock()
	for k := range m.fwGrants {
		if !k.web {
			continue
		}
		if _, ok := want[k]; ok {
			continue
		}
		delete(m.fwGrants, k)
		// 同一地址、端口仍有其他放行（敲门报文或其他规则）时，按剩下的最晚到期时间重新写入：
		// 直接保留会沿用撤销前更长的到期时间
		var keep time.Time
		for o, until := range m.fwGrants {
			if o.addr == k.addr && o.access == k.access && until.After(keep) {
				keep = until
			}
		}
		del = append(del, fwGrant{Addr: k.addr, Access: k.access})
		if !keep.IsZero() {
			add = append(add, fwGrant{Addr: k.addr, Access: k.access, Until: keep})
		}
	}
	for k, until := range want {
		if old, ok := m.fwGrants[k]; ok && !until.After(old) {
			continue
		}
		m.fwGrants[k] = until
		add = append(add, fwGrant{Addr: k.addr, Access: k.access, Until: until})
	}
	m.stateMu.Unlock()
	// 防火墙后台协程先删后加：先撤销再按剩余到期时间写回的放行不会被删掉
	m.fw.revoke(del)
	m.fw.grant(add)
	if len(add) > 0 || len(del) > 0 {
		m.logs.Logger("防火墙").Debugf("同步网页登录放行：写入 %d 项，删除 %d 项", len(add), len(del))
	}
}

// revokeRuleLocked 在规则删除、停用或修改后撤销它产生的全部授权：密钥可能已更换，旧授权不应继续有效。
func (m *Module) revokeRuleLocked(key string) {
	knockgate.Default.RevokeRule(key)
	m.stateMu.Lock()
	for k := range m.fwGrants {
		if k.rule == key {
			delete(m.fwGrants, k)
		}
	}
	delete(m.stats, key)
	delete(m.idStats, key)
	m.stateMu.Unlock()
}

// 全局令牌桶按 HMAC 校验量计费，防止伪造报文洪水占满 CPU：每个报文消耗「启用的规则数 × 长度单位数」个令牌
// （HMAC 认证失败的报文要用每条规则的密钥各算一次，成本与规则数、报文长度成正比）。
// 每秒 globalRate 个单位：Apple M2 上一个单位（一次 200 字节报文的 HMAC-SHA256 连同解析与统计）约 1.3 微秒，
// 预算用满时约占单核 3%；单条规则、常见长度（150～300 字节）的报文每秒可认证 2 万个，比原来的 200 个高两个数量级，
// 分散在大量网段的伪造洪水要达到这个速率才能挤掉合法敲门。性能较弱的路由器上预算用满时 CPU 占用相应升高。
const (
	globalRate  = 20000
	globalBurst = 2 * globalRate
	// hmacUnitLen 为一个计费单位对应的报文长度
	hmacUnitLen = 512
)

// packetCost 返回长度为 n 的报文在 rules 条启用规则下消耗的令牌数，至少为 1，不超过桶容量。
func packetCost(n, rules int) float64 {
	units := max((n+hmacUnitLen-1)/hmacUnitLen, 1)
	return min(float64(max(rules, 1)*units), globalBurst)
}

// tokenBucket 是简单的令牌桶。调用方持有 stateMu。
type tokenBucket struct {
	rate, burst float64
	tokens      float64
	last        time.Time
}

// level 返回 now 时桶中的令牌数（按速率补充后），不修改桶。
func (b *tokenBucket) level(now time.Time) float64 {
	if b.last.IsZero() {
		return b.burst
	}
	if d := now.Sub(b.last).Seconds(); d > 0 {
		return min(b.burst, b.tokens+d*b.rate)
	}
	return b.tokens
}

func (b *tokenBucket) take(now time.Time) bool {
	return b.takeN(now, 1)
}

// takeN 取出 n 个令牌，不足时不取并返回 false。n 超过桶容量时按容量计，避免永远无法通过。
func (b *tokenBucket) takeN(now time.Time, n float64) bool {
	if b.last.IsZero() {
		b.tokens, b.last = b.burst, now
	}
	if d := now.Sub(b.last).Seconds(); d > 0 {
		b.tokens = min(b.burst, b.tokens+d*b.rate)
		b.last = now
	}
	n = min(n, b.burst)
	if b.tokens < n {
		return false
	}
	b.tokens -= n
	return true
}

// 按来源网段限速的参数：同一网段每秒最多认证 srcRate 个报文；网段数达到 srcCap 时，新网段挤掉一个最空闲的网段
const (
	srcRate  = 10
	srcBurst = 20
	srcCap   = 4096
	// evictSample 为网段数满了以后挑选淘汰对象时抽查的桶数
	evictSample = 8
)

// srcLimiter 按来源网段（IPv4 /24、IPv6 /64）分桶限速，网段数有上限，空闲的桶定期清理。调用方持有 stateMu。
//
// 网段数满了以后，新网段从随机抽查的几个桶中淘汰令牌最多（最空闲）的一个，自己占用新桶，而不是共用一个小桶：
// 伪造来源的洪水每个网段通常只发一两个报文，表里几乎全是只用过一个令牌的桶，持续洪水的来源令牌接近耗尽，
// 抽查时几乎不会被选中，所以按来源限速对它仍然有效；合法敲门的新网段也总能拿到自己的桶，
// 只和洪水一起受全局令牌桶（HMAC 校验预算）约束，不会落进一个被洪水占满的共用桶。
type srcLimiter struct {
	buckets map[netip.Prefix]*tokenBucket
	// evictions 为淘汰次数，供测试检查
	evictions int
}

func newSrcLimiter() *srcLimiter {
	return &srcLimiter{buckets: map[netip.Prefix]*tokenBucket{}}
}

// srcPrefix 返回来源地址所属的限速网段。
func srcPrefix(a netip.Addr) netip.Prefix {
	bits := 64
	if a.Is4() {
		bits = 24
	}
	p, _ := a.Prefix(bits)
	return p
}

func (l *srcLimiter) take(src netip.Addr, now time.Time) bool {
	p := srcPrefix(src)
	b := l.buckets[p]
	if b == nil {
		if len(l.buckets) >= srcCap {
			l.evict(now)
		}
		b = &tokenBucket{rate: srcRate, burst: srcBurst}
		l.buckets[p] = b
	}
	return b.take(now)
}

// evict 从随机位置起抽查最多 evictSample 个桶，删除其中令牌最多的一个；遇到已经回满的桶直接删除。
// map 的遍历起点是随机的，抽查不需要额外的数据结构，代价与网段总数无关。
func (l *srcLimiter) evict(now time.Time) {
	var (
		victim netip.Prefix
		best   = -1.0
		n      = 0
	)
	for p, b := range l.buckets {
		lv := b.level(now)
		if lv > best {
			victim, best = p, lv
		}
		n++
		if n >= evictSample || lv >= b.burst {
			break
		}
	}
	if best >= 0 {
		delete(l.buckets, victim)
		l.evictions++
	}
}

// prune 删除已经回满（空闲足够久）的桶：重新创建时同样是满的，删除不改变限速结果。
func (l *srcLimiter) prune(now time.Time) {
	full := time.Duration(float64(time.Second) * srcBurst / srcRate)
	for p, b := range l.buckets {
		if now.Sub(b.last) >= full {
			delete(l.buckets, p)
		}
	}
}

// invalidWindow 为 HMAC 认证失败汇总日志的时间窗
const invalidWindow = time.Minute

// invalidStats 汇总一个时间窗内 HMAC 认证失败的报文数与来源数（来源最多记录 srcCap 个）。调用方持有 stateMu。
type invalidStats struct {
	since   time.Time
	packets int
	sources map[netip.Addr]struct{}
	sample  netip.Addr
}

func (s *invalidStats) add(src netip.Addr, now time.Time) {
	if s.packets == 0 {
		s.since, s.sources, s.sample = now, map[netip.Addr]struct{}{}, src
	}
	s.packets++
	if len(s.sources) < srcCap {
		s.sources[src] = struct{}{}
	}
}

// flushInvalid 在时间窗结束后把 HMAC 认证失败的报文汇总成一条日志：伪造来源的洪水不会逐个来源刷屏。
func (m *Module) flushInvalid(now time.Time) {
	m.stateMu.Lock()
	st := m.invalid
	due := st.packets > 0 && now.Sub(st.since) >= invalidWindow
	if due {
		m.invalid = invalidStats{}
	}
	m.stateMu.Unlock()
	if !due {
		return
	}
	m.logs.Logger("").Warnf("最近 1 分钟收到 %[1]d 个来源的 %[2]d 个无效敲门报文（没有规则的密钥能通过 HMAC 认证），其中一个来源为 %[3]s", len(st.sources), st.packets, st.sample)
}
