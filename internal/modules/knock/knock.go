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
	"encoding/base64"
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
	// Ports 为敲门可以请求放行的端口（如 tcp/22），客户端 -A 参数中的端口必须全部在此列表中
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
	RequireAddr bool   `json:"requireAddr"`
	Remark      string `json:"remark"`
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
	if len(ports) == 0 {
		return errors.New("请至少填写一个端口，如 tcp/22")
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
	return nil
}

// spaKey 解码规则中的密钥。
func (r *Rule) spaKey() (spa.Key, error) {
	enc, err := decodeKey(r.EncKey)
	if err != nil {
		return spa.Key{}, fmt.Errorf("加密密钥需为 base64 编码: %w", err)
	}
	mac, err := decodeKey(r.HMACKey)
	if err != nil {
		return spa.Key{}, fmt.Errorf("HMAC 密钥需为 base64 编码: %w", err)
	}
	k := spa.Key{Enc: enc, HMAC: mac, HMACType: r.HMACType}
	return k, k.Validate()
}

// decodeKey 解码 base64 密钥，兼容省略了末尾 "=" 的写法。
func decodeKey(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.RawStdEncoding.DecodeString(strings.TrimRight(s, "="))
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
}

// snapshot 是收包协程无锁读取的配置快照。
type snapshot struct {
	settings Settings
	rules    []*compiledRule
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
	stateMu  sync.Mutex
	stats    map[string]*ruleStats
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
	return &Module{now: time.Now, stats: map[string]*ruleStats{}, fwGrants: map[fwGrantKey]time.Time{},
		replay: map[string]time.Time{}, failSeen: map[netip.Addr]time.Time{}, bucket: tokenBucket{rate: 200, burst: 400},
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
	for i := range m.cfg.Rules {
		r := &m.cfg.Rules[i]
		if err := r.Validate(); err != nil {
			m.logs.Add(logbuf.LevelWarn, r.Name, "规则无效，已停用: "+err.Error())
			r.Enable = false
		}
	}
	crud.AssignKeys(len(m.cfg.Rules), func(i int) *string { return &m.cfg.Rules[i].Key }, "k")
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
	go m.loop(m.ctx)
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
		snap.rules = append(snap.rules, &compiledRule{rule: r, key: k, ports: ports})
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
	// 没有启用的规则时不监听，避免无意义地暴露端口
	wantPort := 0
	if len(snap.rules) > 0 {
		wantPort = s.Port
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
	m.stateMu.Lock()
	ok := m.perSrc.take(src, now) && m.bucket.take(now)
	m.stateMu.Unlock()
	if !ok {
		m.count(func(c *Counters) { c.Ignored++ })
		return errLimited
	}
	snap := m.snap.Load()
	var (
		rule *compiledRule
		msg  *spa.Message
		err  error
	)
	for _, r := range snap.rules {
		msg, err = spa.Decode(packet, r.key)
		if errors.Is(err, spa.ErrHMAC) || errors.Is(err, spa.ErrFormat) {
			continue
		}
		rule = r
		break
	}
	if rule == nil {
		m.reject(nil, src, transport, via, "没有规则的密钥能通过 HMAC 认证")
		return errors.New("认证失败")
	}
	if err != nil {
		// HMAC 正确而解密失败：HMAC 密钥一致，加密密钥不一致
		m.reject(rule, src, transport, via, "HMAC 认证通过但解密失败，请检查加密密钥是否一致")
		return errors.New("解密失败")
	}
	if reason := m.check(rule, msg, snap.settings, now); reason != "" {
		m.reject(rule, src, transport, via, reason)
		return errors.New(reason)
	}
	addr := msg.Addr
	if !addr.IsValid() || addr.IsUnspecified() {
		addr = src
	}
	if reason := checkAddr(addr); reason != "" {
		m.reject(rule, src, transport, via, reason)
		return errors.New(reason)
	}
	ttl := effectiveTimeout(rule.rule, msg.ClientTimeout)
	m.grant(rule, addr, src, msg, ttl, via)
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

// grant 登记授权、写防火墙并记录日志与事件。
func (m *Module) grant(r *compiledRule, addr, src netip.Addr, msg *spa.Message, ttl time.Duration, via string) {
	now := m.now()
	until := now.Add(ttl)
	if err := knockgate.Default.Allow(addr, r.rule.Key, until); err != nil {
		m.logs.Logger(r.rule.Name).Warnf("登记 %s 的授权失败: %v", addr, err)
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
	m.logs.Logger(r.rule.Name).Infof("敲门成功：放行 %s 访问 %s %d 秒，用户 %s，经 %s%s", addr, ports, int(ttl/time.Second), user, via, extra)
	events.Publish(events.Event{
		Type:    events.KnockOpened,
		Source:  r.rule.Name,
		Level:   events.LevelInfo,
		Title:   fmt.Sprintf("端口敲门放行了 %s", addr),
		Message: fmt.Sprintf("%s：%s 敲门成功，放行 %s %d 秒", r.rule.Name, addr, ports, int(ttl/time.Second)),
		Fields:  map[string]string{"ip": addr.String(), "rule": r.rule.Name, "ports": ports},
	})
}

// failInterval 为同一来源两次失败日志与失败事件之间的最短间隔：持续发送伪造报文的来源每个间隔只记一次，
// 入侵防护按事件计数时也足以在几分钟内封禁
const failInterval = 5 * time.Second

// reject 统计一次被拒绝的敲门；同一来源每 failInterval 最多记录一条日志、发布一个失败事件（供入侵防护计数封禁）。
//
// 失败事件会被入侵防护用来封禁事件中的地址，所以只为可信的来源发布：经 TCP（协议分流）发来的报文来源经过握手，
// 照常发布；UDP 报文的来源地址可以伪造，只有通过 HMAC 认证（r 不为空，报文出自持有密钥的客户端）但被规则拒绝时
// 才发布。HMAC 认证失败的 UDP 包任何人都能以任意来源地址发出，若发布事件，攻击者可以借入侵防护封禁任意 IP。
// HMAC 认证失败的报文不逐条记录日志，由 flushInvalid 每分钟汇总一条。
func (m *Module) reject(r *compiledRule, src netip.Addr, transport, via, reason string) {
	now := m.now()
	m.stateMu.Lock()
	m.counters.Rejected++
	name := ""
	if r != nil {
		name = r.rule.Name
		m.statsLocked(r.rule.Key).Rejected++
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
		m.logs.Logger(name).Warnf("拒绝来自 %s 的敲门（经 %s）：%s", src, via, reason)
	}
	events.Publish(events.Event{
		Type:    events.KnockFailed,
		Source:  name,
		Level:   events.LevelWarn,
		Title:   fmt.Sprintf("端口敲门拒绝了 %s", src),
		Message: reason,
		Fields:  map[string]string{"ip": src.String(), "reason": reason, "via": transport},
	})
}

func (m *Module) count(f func(*Counters)) {
	m.stateMu.Lock()
	f(&m.counters)
	m.stateMu.Unlock()
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
	var del []fwGrant
	m.stateMu.Lock()
	for k := range m.fwGrants {
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
	return n
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
	m.stateMu.Unlock()
}

// tokenBucket 是简单的令牌桶，限制每秒尝试解密的报文数，防止伪造报文洪水占满 CPU。调用方持有 stateMu。
type tokenBucket struct {
	rate, burst float64
	tokens      float64
	last        time.Time
}

func (b *tokenBucket) take(now time.Time) bool {
	if b.last.IsZero() {
		b.tokens, b.last = b.burst, now
	}
	if d := now.Sub(b.last).Seconds(); d > 0 {
		b.tokens = min(b.burst, b.tokens+d*b.rate)
		b.last = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// 按来源网段限速的参数：同一网段每秒最多认证 srcRate 个报文；网段数超过 srcCap 时，新网段共用 overflow 桶
const (
	srcRate      = 10
	srcBurst     = 20
	srcCap       = 4096
	overflowRate = 50
)

// srcLimiter 按来源网段（IPv4 /24、IPv6 /64）分桶限速，网段数有上限，空闲的桶定期清理。调用方持有 stateMu。
type srcLimiter struct {
	buckets  map[netip.Prefix]*tokenBucket
	overflow tokenBucket
}

func newSrcLimiter() *srcLimiter {
	return &srcLimiter{buckets: map[netip.Prefix]*tokenBucket{}, overflow: tokenBucket{rate: overflowRate, burst: overflowRate * 2}}
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
			l.prune(now)
		}
		if len(l.buckets) >= srcCap {
			// 网段过多（大量伪造来源）：新网段共用一个桶，已有网段的桶不受影响
			return l.overflow.take(now)
		}
		b = &tokenBucket{rate: srcRate, burst: srcBurst}
		l.buckets[p] = b
	}
	return b.take(now)
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
