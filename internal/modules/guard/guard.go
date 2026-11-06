// Package guard 实现入侵防护模块（类似 fail2ban）：按日志规则、诱饵端口与端口扫描检测发现恶意 IP，
// 写入共享封禁表（internal/banlist），并同步到系统防火墙（nftables）在内核层丢弃其全部流量。
//
// 没有 nftables 或权限不足的平台（Windows、macOS、未授予 NET_ADMIN 的容器等）只在 Gately 自身的服务上生效，
// 诱饵端口退化为用户态监听，端口扫描检测不可用。
package guard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/fsperm"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/webprobe"
)

// ID 是模块标识。
const ID = "guard"

// 时长上下限（秒）
const (
	defaultBanSeconds    = 3600
	defaultMaxBanSeconds = 7 * 24 * 3600
	maxBanSeconds        = 365 * 24 * 3600
	defaultScanThreshold = 12
	// defaultTrapPorts 是常被扫描、NAS 与家用服务器通常不会对外开放的端口；正在监听的端口会自动排除
	defaultTrapPorts = "23,135,1433,1521,3389,5900,6379,11211"
)

// TrapSettings 是诱饵端口设置：任何来源连接这些端口即被封禁。
type TrapSettings struct {
	Enable bool `json:"enable"`
	// Ports 为端口列表，逗号或换行分隔，支持 5900-5910 这样的范围
	Ports string `json:"ports"`
	// BanSeconds 为封禁时长，0 表示沿用默认时长
	BanSeconds int `json:"banSeconds"`
}

// ScanSettings 是端口扫描检测设置：同一来源每分钟连接本机未开放端口的次数超过阈值即被封禁。
type ScanSettings struct {
	Enable bool `json:"enable"`
	// Threshold 为每分钟允许的未开放端口连接次数
	Threshold int `json:"threshold"`
	// ExcludePorts 为额外视为开放的端口（例如由其他防火墙规则转发的端口），格式同诱饵端口
	ExcludePorts string `json:"excludePorts"`
	BanSeconds   int    `json:"banSeconds"`
}

// Settings 是模块全局设置。
type Settings struct {
	// Firewall 为 true 时把封禁同步到 nftables，在内核层丢弃被封禁地址的全部流量
	Firewall bool `json:"firewall"`
	// BanSeconds 为默认封禁时长
	BanSeconds int `json:"banSeconds"`
	// Increment 为 true 时同一地址再次被封禁的时长翻倍，最长 MaxBanSeconds
	Increment     bool `json:"increment"`
	MaxBanSeconds int  `json:"maxBanSeconds"`
	// IgnorePrivate 为 true 时内网、回环与链路本地地址不会被自动封禁
	IgnorePrivate bool `json:"ignorePrivate"`
	// IgnoreList 为白名单 IP 或网段
	IgnoreList []string     `json:"ignoreList"`
	Trap       TrapSettings `json:"trap"`
	Scan       ScanSettings `json:"scan"`
}

type fileConfig struct {
	Settings Settings `json:"settings"`
	Jails    []Jail   `json:"jails"`
}

func defaultSettings() Settings {
	return Settings{
		Firewall: true, BanSeconds: defaultBanSeconds, Increment: true, MaxBanSeconds: defaultMaxBanSeconds,
		IgnorePrivate: true, IgnoreList: []string{},
		Trap: TrapSettings{Ports: defaultTrapPorts},
		Scan: ScanSettings{Threshold: defaultScanThreshold},
	}
}

// privatePrefixes 是“忽略内网地址”包含的网段：回环、私有地址、运营商级 NAT（Tailscale 也使用该段）与链路本地地址。
var privatePrefixes = mustPrefixes("127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10", "169.254.0.0/16",
	"::1/128", "fc00::/7", "fe80::/10")

func mustPrefixes(list ...string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(list))
	for _, s := range list {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}

// Validate 校验并规范化设置。
func (s *Settings) Validate() error {
	if s.BanSeconds == 0 {
		s.BanSeconds = defaultBanSeconds
	}
	if s.BanSeconds < 60 || s.BanSeconds > maxBanSeconds {
		return errors.New("默认封禁时长应在 1 分钟到 365 天之间")
	}
	if s.MaxBanSeconds == 0 {
		s.MaxBanSeconds = defaultMaxBanSeconds
	}
	if s.MaxBanSeconds < s.BanSeconds || s.MaxBanSeconds > maxBanSeconds {
		return errors.New("最长封禁时长应不小于默认时长，且不超过 365 天")
	}
	var list []string
	for _, v := range s.IgnoreList {
		if v = strings.TrimSpace(v); v == "" {
			continue
		}
		if _, err := parsePrefix(v); err != nil {
			return fmt.Errorf("白名单 %q 无效: %w", v, err)
		}
		list = append(list, v)
	}
	if list == nil {
		list = []string{}
	}
	s.IgnoreList = list
	// 0 表示沿用默认时长，-1 表示永久封禁（见 banOptions），其余取值限定在 1 分钟到 365 天
	for _, sec := range []int{s.Trap.BanSeconds, s.Scan.BanSeconds} {
		if sec != 0 && sec != -1 && (sec < 60 || sec > maxBanSeconds) {
			return errors.New("封禁时长应在 1 分钟到 365 天之间")
		}
	}
	ports, err := parsePorts(s.Trap.Ports)
	if err != nil {
		return fmt.Errorf("诱饵端口: %w", err)
	}
	if s.Trap.Enable && len(ports) == 0 {
		return errors.New("启用诱饵端口时至少填写一个端口")
	}
	s.Trap.Ports = formatPorts(ports)
	ex, err := parsePorts(s.Scan.ExcludePorts)
	if err != nil {
		return fmt.Errorf("排除端口: %w", err)
	}
	s.Scan.ExcludePorts = formatPorts(ex)
	if s.Scan.Threshold == 0 {
		s.Scan.Threshold = defaultScanThreshold
	}
	if s.Scan.Threshold < 3 || s.Scan.Threshold > 1000 {
		return errors.New("扫描检测阈值应为每分钟 3~1000 次")
	}
	return nil
}

// parsePrefix 解析 IP 或 CIDR 为网段，IPv4 映射地址按 IPv4 处理。
func parsePrefix(s string) (netip.Prefix, error) {
	s = strings.TrimSpace(s)
	if a, err := netip.ParseAddr(s); err == nil {
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

// ignoreSet 是编译后的白名单。
type ignoreSet struct {
	prefixes []netip.Prefix
}

// ignoreSet 编译白名单；忽略内网地址时一并加入本机网卡所在的 IPv6 网段 local。
func (s Settings) ignoreSet(local []netip.Prefix) *ignoreSet {
	set := &ignoreSet{}
	if s.IgnorePrivate {
		set.prefixes = append(set.prefixes, privatePrefixes...)
		set.prefixes = append(set.prefixes, local...)
	}
	for _, v := range s.IgnoreList {
		if p, err := parsePrefix(v); err == nil {
			set.prefixes = append(set.prefixes, p)
		}
	}
	return set
}

// overlaps 判断将被封禁的网段是否与白名单重叠。封禁按 IPv6 /64 进行，白名单中的单个地址也要保护它所在的 /64。
func (s *ignoreSet) overlaps(p netip.Prefix) bool {
	if p.Addr().IsLoopback() || p.Addr().IsUnspecified() {
		return true
	}
	for _, q := range s.prefixes {
		if q.Overlaps(p) {
			return true
		}
	}
	return false
}

// contains 按封禁粒度判断地址是否在白名单中。
func (s *ignoreSet) contains(a netip.Addr) bool {
	p, _ := banlist.KeyPrefix(banlist.KeyFor(a))
	return s.overlaps(p)
}

// localPrefixes 返回本机网卡所在的 IPv6 网段（/64 或更长的前缀，按 /64 计），可在测试中替换。
// 家庭网络常把同一个公网 /64 分给局域网内所有设备，而封禁按 /64 进行，
// 不把它视为内网的话，一台设备触发封禁就会连带整个局域网。更短的前缀（如 VPS 共用的 /48）不加入。
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
			if !ok || ipn.IP.To4() != nil {
				continue
			}
			ip, ok := netip.AddrFromSlice(ipn.IP)
			bits, _ := ipn.Mask.Size()
			if !ok || bits < 64 || !ip.IsGlobalUnicast() || ip.IsPrivate() {
				continue
			}
			p := netip.PrefixFrom(ip, 64).Masked()
			if !slices.Contains(out, p) {
				out = append(out, p)
			}
		}
	}
	slices.SortFunc(out, func(a, b netip.Prefix) int { return a.Addr().Compare(b.Addr()) })
	return out
}

// Deps 是由装配处注入的外部能力，避免模块之间直接依赖。
type Deps struct {
	// FollowDocker 持续读取容器新产生的日志行，直到 ctx 取消或出错；Docker 模块关闭时为 nil
	FollowDocker func(ctx context.Context, container string, onLine func(string)) error
}

// Module 是入侵防护模块。
type Module struct {
	mu    sync.Mutex
	store *config.Store
	logs  *logbuf.Buffer
	cfg   fileConfig
	deps  Deps
	// settings 为当前设置的快照，供日志规则、诱饵端口等协程无锁读取（这些协程在 m.mu 持有期间可能被等待退出）
	settings atomic.Pointer[Settings]

	ctx     context.Context
	cancel  context.CancelFunc
	started bool
	// local 为本机网卡所在的 IPv6 网段，定期刷新（受 m.mu 保护）
	local []netip.Prefix
	jails map[string]*jailRunner
	// webJails 为启用中的 Web 路径探测规则快照，每个请求都会读取，不能经过 m.mu
	webJails atomic.Pointer[[]*jailRunner]
	fw       *firewall
	trap     *userTrap
	unsub    []func()

	// saveTimer 延迟保存封禁表，合并短时间内的多次变化；fileMu 保证同一时间只有一次写文件
	saveMu    sync.Mutex
	saveTimer *time.Timer
	fileMu    sync.Mutex
}

// New 创建模块。
func New(deps Deps) *Module {
	return &Module{deps: deps, jails: map[string]*jailRunner{}}
}

// ID 实现 module.Module。
func (m *Module) ID() string { return ID }

// Title 实现 module.Module。
func (m *Module) Title() string { return "入侵防护" }

// Init 实现 module.Module。
func (m *Module) Init(env module.Env) error {
	m.store, m.logs = env.Store, env.Logs
	m.cfg = fileConfig{Settings: defaultSettings(), Jails: []Jail{}}
	if _, err := m.store.Load(ID, &m.cfg); err != nil {
		return err
	}
	if err := m.cfg.Settings.Validate(); err != nil {
		m.logs.Add(logbuf.LevelWarn, "", "设置无效，已恢复默认值: "+err.Error())
		m.cfg.Settings = defaultSettings()
	}
	if m.cfg.Jails == nil {
		m.cfg.Jails = []Jail{}
	}
	s := m.cfg.Settings
	m.settings.Store(&s)
	m.fw = newFirewall(m.logs.Logger("防火墙"))
	return nil
}

// bansPath 是封禁表的持久化文件。
func (m *Module) bansPath() string {
	return filepath.Join(m.store.Dir(), ID, "bans.json")
}

type bansFile struct {
	Bans     []banlist.Entry            `json:"bans"`
	Offenses map[string]banlist.Offense `json:"offenses"`
}

func (m *Module) loadBans() {
	data, err := os.ReadFile(m.bansPath())
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			m.logs.Add(logbuf.LevelWarn, "", "读取封禁记录失败: "+err.Error())
		}
		return
	}
	var f bansFile
	if err := json.Unmarshal(data, &f); err != nil {
		m.logs.Add(logbuf.LevelWarn, "", "封禁记录格式无效: "+err.Error())
		return
	}
	banlist.Default.Import(f.Bans, f.Offenses)
}

// saveBans 立即把封禁表写入文件。
func (m *Module) saveBans() {
	m.fileMu.Lock()
	defer m.fileMu.Unlock()
	bans, offenses := banlist.Default.Export()
	// 不缩进：封禁多的机器上文件有数百 KB，每次封禁都整份重写，缩进会让写入量多出约三分之一
	data, err := json.Marshal(bansFile{Bans: bans, Offenses: offenses})
	if err != nil {
		return
	}
	dir := filepath.Dir(m.bansPath())
	if err := fsperm.MkdirAll(dir, 0o700); err != nil {
		m.logs.Add(logbuf.LevelWarn, "", "保存封禁记录失败: "+err.Error())
		return
	}
	tmp := m.bansPath() + ".tmp"
	if err := fsperm.WriteFile(tmp, data, 0o600); err != nil {
		m.logs.Add(logbuf.LevelWarn, "", "保存封禁记录失败: "+err.Error())
		return
	}
	if err := os.Rename(tmp, m.bansPath()); err != nil {
		m.logs.Add(logbuf.LevelWarn, "", "保存封禁记录失败: "+err.Error())
	}
}

// saveDelay 是封禁表变化后延迟写文件的时间，期间的变化合并为一次写入；测试中缩短。
var saveDelay = 2 * time.Second

// scheduleSave 在 saveDelay 后保存封禁表，期间的变化合并为一次写入。
func (m *Module) scheduleSave() {
	m.saveMu.Lock()
	defer m.saveMu.Unlock()
	if m.saveTimer != nil {
		return
	}
	m.saveTimer = time.AfterFunc(saveDelay, func() {
		m.saveMu.Lock()
		m.saveTimer = nil
		m.saveMu.Unlock()
		m.saveBans()
	})
}

// Start 实现 module.Module。
func (m *Module) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.started = true
	s := m.cfg.Settings
	m.local = localPrefixes()
	banlist.Default.SetIgnore(s.ignoreSet(m.local).overlaps)
	m.loadBans()
	m.unsub = append(m.unsub, banlist.Default.Subscribe(m.onBanChange))
	m.unsub = append(m.unsub, events.Subscribe(m.onEvent))
	webprobe.SetHandler(m.onWebHit)
	m.unsub = append(m.unsub, func() { webprobe.SetHandler(nil) })
	m.fw.start(m.ctx, s, m.local, m.importKernelBans, m.onFirewallActive)
	m.applyTrapLocked()
	for _, j := range m.cfg.Jails {
		m.startJailLocked(j)
	}
	go m.pruneLoop(m.ctx)
	return nil
}

// Stop 实现 module.Module。
func (m *Module) Stop() error {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return nil
	}
	m.started = false
	for _, u := range m.unsub {
		u()
	}
	m.unsub = nil
	for k, j := range m.jails {
		j.stop()
		delete(m.jails, k)
	}
	m.refreshWebLocked()
	if m.trap != nil {
		m.trap.stop()
		m.trap = nil
	}
	m.cancel()
	m.mu.Unlock()
	m.fw.stop()
	banlist.Default.SetIgnore(nil)
	m.saveMu.Lock()
	if m.saveTimer != nil {
		m.saveTimer.Stop()
		m.saveTimer = nil
	}
	m.saveMu.Unlock()
	m.saveBans()
	return nil
}

func (m *Module) saveLocked() error { return m.store.Save(ID, m.cfg) }

// pruneLoop 定期清除到期的封禁（到期记录由防火墙同步逻辑从 nftables 中移除），并每分钟刷新一次本机所在的 IPv6 网段。
func (m *Module) pruneLoop(ctx context.Context) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	tick := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			for _, e := range banlist.Default.Prune() {
				m.logs.Logger("").Infof("%s 的封禁已到期", e.Key)
			}
			tick++
			if tick%4 == 0 {
				m.refreshLocal()
			}
		}
	}
}

// refreshLocal 在本机 IPv6 网段变化（例如运营商重新分配前缀）时更新白名单。
func (m *Module) refreshLocal() {
	local := localPrefixes()
	m.mu.Lock()
	if !m.started || slices.Equal(local, m.local) {
		m.mu.Unlock()
		return
	}
	m.local = local
	s := m.cfg.Settings
	banlist.Default.SetIgnore(s.ignoreSet(local).overlaps)
	m.mu.Unlock()
	m.fw.setLocal(m.ctx, local)
}

// localSnapshot 返回本机所在的 IPv6 网段，调用方需持有 m.mu。
func (m *Module) localSnapshotLocked() []netip.Prefix {
	return append([]netip.Prefix{}, m.local...)
}

// onFirewallActive 在防火墙规则生效状态变化时调用：生效后由内核处理诱饵端口，不再由 Gately 监听。
func (m *Module) onFirewallActive() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		m.applyTrapLocked()
	}
}

// onBanChange 在封禁表变化时同步防火墙、保存文件。
func (m *Module) onBanChange(c banlist.Change) {
	m.fw.change(c)
	if c.Expired && len(c.Added) == 0 {
		// 只是到期清除：文件里留着到期记录无害（加载时会跳过），不为此单独重写文件，
		// 等下一次封禁、解封或模块停止时一并写入
		return
	}
	m.scheduleSave()
}

// banOptions 返回一次自动封禁的参数：sec 为封禁秒数，0 表示沿用默认时长，-1 表示永久。
func (s Settings) banOptions(sec int, reason, source string) banlist.Options {
	o := banlist.Options{Reason: reason, Source: source, Increment: s.Increment, MaxDuration: time.Duration(s.MaxBanSeconds) * time.Second}
	switch {
	case sec < 0:
		o.Permanent = true
	case sec == 0:
		o.Duration = time.Duration(s.BanSeconds) * time.Second
	default:
		o.Duration = time.Duration(sec) * time.Second
	}
	return o
}

// cur 返回当前设置。
func (m *Module) cur() Settings {
	return *m.settings.Load()
}

// ban 按设置封禁地址并记录日志、发布事件。不获取 m.mu，可在被 m.mu 持有者等待的协程中调用。
func (m *Module) ban(a netip.Addr, sec int, reason, source, jailName string) {
	s := m.cur()
	e, added, err := banlist.Default.Ban(banlist.KeyFor(a), s.banOptions(sec, reason, source))
	if err != nil || !added {
		return
	}
	until := "永久"
	if !e.Permanent() {
		until = e.Until.Local().Format("2006-01-02 15:04:05")
	}
	m.logs.Logger(jailName).Warnf("封禁 %s 至 %s（%s，第 %d 次）", e.Key, until, reason, e.Count)
	events.Publish(events.Event{
		Type:    events.GuardBanned,
		Source:  jailName,
		Level:   events.LevelWarn,
		Title:   fmt.Sprintf("入侵防护封禁了 %s", e.Key),
		Message: fmt.Sprintf("%s：%s，封禁至 %s", jailName, reason, until),
		Fields:  map[string]string{"ip": e.Key, "reason": reason},
	})
}

// importKernelBans 接收防火墙中由诱饵端口或扫描检测规则直接加入的地址，写入封禁表（按设置递增时长）。
func (m *Module) importKernelBans(source string, addrs []netip.Addr) {
	s := m.cur()
	reason, sec, name := "连接诱饵端口", s.Trap.BanSeconds, "诱饵端口"
	if source == banlist.SourceScan {
		reason, sec, name = "端口扫描", s.Scan.BanSeconds, "扫描检测"
	}
	var keys []string
	for _, a := range addrs {
		e, added, err := banlist.Default.Ban(banlist.KeyFor(a), s.banOptions(sec, reason, source))
		if err != nil || !added {
			continue
		}
		keys = append(keys, e.Key)
		m.logs.Logger(name).Warnf("封禁 %s（%s，第 %d 次）", e.Key, reason, e.Count)
	}
	if len(keys) == 0 {
		return
	}
	// 扫描往往成批出现，每批只发一条通知
	shown := keys
	if len(shown) > 10 {
		shown = shown[:10]
	}
	msg := strings.Join(shown, "、")
	if len(keys) > len(shown) {
		msg = fmt.Sprintf("%s 等 %d 个地址", msg, len(keys))
	}
	events.Publish(events.Event{
		Type:    events.GuardBanned,
		Source:  name,
		Level:   events.LevelWarn,
		Title:   fmt.Sprintf("入侵防护因%s封禁了 %d 个 IP", reason, len(keys)),
		Message: msg,
		Fields:  map[string]string{"ip": keys[0], "reason": reason, "count": fmt.Sprint(len(keys))},
	})
}

// onEvent 把 Gately 内部事件（管理后台登录失败、WAF 拦截）交给来源为“内部事件”的规则计数。
func (m *Module) onEvent(e events.Event) {
	ip := e.Fields["ip"]
	if ip == "" {
		return
	}
	a, ok := parseAddr(ip)
	if !ok {
		return
	}
	m.mu.Lock()
	var hit []*jailRunner
	for _, j := range m.jails {
		// 停用的规则只是占位（m 为 nil），不参与计数
		if j.m != nil && j.jail.Source == SourceEvent && j.jail.Target == e.Type {
			hit = append(hit, j)
		}
	}
	m.mu.Unlock()
	for _, j := range hit {
		j.record(a, e.Message)
	}
}

// parseAddr 解析 IP 或 host:port 形式的地址。
func parseAddr(s string) (netip.Addr, bool) {
	s = strings.TrimSpace(s)
	if a, err := netip.ParseAddr(s); err == nil {
		return a.Unmap(), true
	}
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return ap.Addr().Unmap(), true
	}
	return netip.Addr{}, false
}
