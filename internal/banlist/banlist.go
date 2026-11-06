// Package banlist 是进程内共享的 IP 封禁表：入侵防护模块、Web 服务认证等来源往里写入封禁，
// 端口转发、Web 服务、STUN、WebDAV、FTP 与管理后台在接受连接或处理请求时查询它。
//
// 封禁表本身只在内存中；持久化与同步到系统防火墙由入侵防护模块通过 Subscribe 完成，
// 该模块关闭时封禁仍对 Gately 自身的服务生效，只是重启后不保留。
package banlist

import (
	"errors"
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"
)

// 封禁来源
const (
	SourceManual = "manual"
	SourceWeb    = "webservice"
	SourceTrap   = "trap"
	SourceScan   = "scan"
	// SourceJailPrefix 后接规则 key，表示由日志规则触发
	SourceJailPrefix = "jail:"
)

const (
	// offenseTTL 内没有再次被封禁，累计次数清零
	offenseTTL = 30 * 24 * time.Hour
	// defaultMaxDuration 是递增封禁未指定上限时的最长时长
	defaultMaxDuration = 30 * 24 * time.Hour
)

// ErrIgnored 表示地址在白名单中，自动封禁被跳过。
var ErrIgnored = errors.New("该地址在白名单中，不会被封禁")

// Entry 是一条封禁记录。
type Entry struct {
	// Key 为 IPv4 地址、IPv6 /64 网段，或手动填写的 CIDR 网段
	Key string `json:"key"`
	// Until 为解封时间，零值表示永久封禁
	Until   time.Time `json:"until,omitzero"`
	Created time.Time `json:"created"`
	Reason  string    `json:"reason"`
	// Source 为封禁来源，见 Source* 常量
	Source string `json:"source"`
	// Count 为该地址累计被封禁的次数（含本次），用于递增封禁时长
	Count int `json:"count"`
}

// Permanent 判断是否为永久封禁。
func (e Entry) Permanent() bool { return e.Until.IsZero() }

// Expired 判断封禁是否已到期。
func (e Entry) Expired(now time.Time) bool { return !e.Until.IsZero() && !now.Before(e.Until) }

// Offense 是一个地址的累计封禁次数，封禁解除后仍保留一段时间。
type Offense struct {
	Count int       `json:"count"`
	Last  time.Time `json:"last"`
}

// Options 是一次封禁的参数。
type Options struct {
	// Duration 为封禁时长；Permanent 为 true 时忽略
	Duration  time.Duration
	Permanent bool
	Reason    string
	Source    string
	// Increment 为 true 时按累计次数翻倍封禁时长，最长 MaxDuration（0 表示 30 天）
	Increment   bool
	MaxDuration time.Duration
	// Force 为 true 时不检查白名单（手动封禁）
	Force bool
}

// Change 描述一次封禁表变化，Added 中包含时长被延长的记录。
type Change struct {
	Added   []Entry
	Removed []Entry
	// Expired 为 true 表示这次变化只是到期记录被清除（Prune）。持久化方可以不为此立即写文件：
	// 加载时本来就会跳过到期的记录，等下一次真正的变化或停止时再一并写入即可。
	Expired bool
}

// Store 是封禁表，可并发使用。
type Store struct {
	mu      sync.RWMutex
	entries map[string]Entry
	// nets 为不是单个地址键的网段（手动填写的 CIDR），查询时逐个比对
	nets     []netip.Prefix
	offenses map[string]Offense
	ignore   func(netip.Prefix) bool

	hookMu sync.Mutex
	hooks  map[int]func(Change)
	nextID int
}

// New 创建空的封禁表。
func New() *Store {
	return &Store{entries: map[string]Entry{}, offenses: map[string]Offense{}, hooks: map[int]func(Change){}}
}

// Default 是进程级封禁表。
var Default = New()

// KeyFor 返回地址的封禁键：IPv4 为地址本身，IPv6 按 /64 聚合，避免攻击者在同一网段内轮换地址绕过封禁。
func KeyFor(a netip.Addr) string {
	a = a.Unmap()
	if a.Is6() {
		p, _ := a.Prefix(64)
		return p.String()
	}
	return a.String()
}

// ParseKey 把用户输入的 IP 或 CIDR 规范化为封禁键。IPv4 /32 与 IPv6 /64 及更长的前缀按单个地址处理。
func ParseKey(s string) (string, error) {
	s = strings.TrimSpace(s)
	if a, err := netip.ParseAddr(s); err == nil {
		if a.Zone() != "" {
			return "", errors.New("IP 地址不能带区域标识")
		}
		return KeyFor(a), nil
	}
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return "", errors.New("IP 地址或网段格式无效")
	}
	p = netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-unmapBits(p.Addr()))
	if !p.IsValid() {
		return "", errors.New("IP 地址或网段格式无效")
	}
	if p.Addr().Is4() && p.Bits() == 32 || p.Addr().Is6() && p.Bits() >= 64 {
		return KeyFor(p.Addr()), nil
	}
	if p.Bits() == 0 {
		return "", errors.New("不能封禁全部地址")
	}
	return p.Masked().String(), nil
}

// unmapBits 返回 IPv4 映射地址的前缀需要减去的位数。
func unmapBits(a netip.Addr) int {
	if a.Is4In6() {
		return 96
	}
	return 0
}

// KeyPrefix 把封禁键解析为网段；单个 IPv4 地址返回 /32。
func KeyPrefix(key string) (netip.Prefix, bool) {
	if a, err := netip.ParseAddr(key); err == nil {
		return netip.PrefixFrom(a, a.BitLen()), true
	}
	p, err := netip.ParsePrefix(key)
	return p, err == nil
}

// IsSingle 判断封禁键是否代表单个来源（IPv4 地址或 IPv6 /64），这类键可用精确匹配查询。
func IsSingle(key string) bool {
	p, ok := KeyPrefix(key)
	return ok && (p.Addr().Is4() && p.Bits() == 32 || p.Addr().Is6() && p.Bits() == 64)
}

// SetIgnore 设置白名单判断函数，自动封禁前调用；nil 表示没有白名单。
// 函数收到的是将被封禁的网段（IPv6 为 /64），与白名单有重叠即应返回 true，
// 否则白名单中的单个 IPv6 地址会随所在 /64 一起被封禁。
func (s *Store) SetIgnore(f func(netip.Prefix) bool) {
	s.mu.Lock()
	s.ignore = f
	s.mu.Unlock()
}

// Ignored 判断地址（按封禁粒度，IPv6 为所在的 /64）是否与白名单重叠。
func (s *Store) Ignored(a netip.Addr) bool {
	p, _ := KeyPrefix(KeyFor(a))
	return s.ignoredPrefix(p)
}

func (s *Store) ignoredPrefix(p netip.Prefix) bool {
	s.mu.RLock()
	f := s.ignore
	s.mu.RUnlock()
	return f != nil && f(p)
}

// Subscribe 注册变化回调，返回取消函数。回调在封禁表锁外同步执行，应尽快返回。
func (s *Store) Subscribe(f func(Change)) func() {
	s.hookMu.Lock()
	id := s.nextID
	s.nextID++
	s.hooks[id] = f
	s.hookMu.Unlock()
	return func() {
		s.hookMu.Lock()
		delete(s.hooks, id)
		s.hookMu.Unlock()
	}
}

func (s *Store) notify(c Change) {
	if len(c.Added) == 0 && len(c.Removed) == 0 {
		return
	}
	s.hookMu.Lock()
	hs := make([]func(Change), 0, len(s.hooks))
	for _, h := range s.hooks {
		hs = append(hs, h)
	}
	s.hookMu.Unlock()
	for _, h := range hs {
		h(c)
	}
}

// Banned 判断地址当前是否被封禁。
func (s *Store) Banned(a netip.Addr) bool {
	if !a.IsValid() {
		return false
	}
	a = a.Unmap()
	now := time.Now()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.entries) == 0 {
		return false
	}
	if e, ok := s.entries[KeyFor(a)]; ok && !e.Expired(now) {
		return true
	}
	for _, p := range s.nets {
		if p.Contains(a) {
			if e, ok := s.entries[p.String()]; ok && !e.Expired(now) {
				return true
			}
		}
	}
	return false
}

// BannedAddr 判断 net.Addr（TCP / UDP 地址或 host:port 字符串）是否被封禁。
func (s *Store) BannedAddr(addr net.Addr) bool {
	switch v := addr.(type) {
	case *net.TCPAddr:
		a, ok := netip.AddrFromSlice(v.IP)
		return ok && s.Banned(a)
	case *net.UDPAddr:
		a, ok := netip.AddrFromSlice(v.IP)
		return ok && s.Banned(a)
	case nil:
		return false
	}
	ap, err := netip.ParseAddrPort(addr.String())
	return err == nil && s.Banned(ap.Addr())
}

// Ban 封禁 key（应为 ParseKey / KeyFor 的结果）。地址已被封禁时只延长时长、不重复计数；
// added 为 false 表示没有任何变化（已有的封禁更长）。
func (s *Store) Ban(key string, o Options) (e Entry, added bool, err error) {
	p, ok := KeyPrefix(key)
	if !ok {
		return Entry{}, false, errors.New("IP 地址或网段格式无效")
	}
	if !o.Force && s.ignoredPrefix(p) {
		return Entry{}, false, ErrIgnored
	}
	now := time.Now()
	s.mu.Lock()
	old, exists := s.entries[key]
	if exists && old.Expired(now) {
		exists = false
	}
	off := s.offenses[key]
	if now.Sub(off.Last) > offenseTTL {
		off = Offense{}
	}
	count := off.Count
	if !exists {
		count++
	}
	e = Entry{Key: key, Created: now, Reason: o.Reason, Source: o.Source, Count: count}
	if !o.Permanent {
		d := o.Duration
		if o.Increment && count > 1 {
			limit := o.MaxDuration
			if limit <= 0 {
				limit = defaultMaxDuration
			}
			for i := 1; i < count && d < limit; i++ {
				d *= 2
			}
			d = min(d, limit)
		}
		e.Until = now.Add(d)
	}
	if exists {
		// 已有封禁更长（或永久）时保持不变；只延长不到一分钟的也不算变化，避免反复触发时频繁改写防火墙
		if old.Permanent() || !e.Permanent() && !e.Until.After(old.Until.Add(time.Minute)) {
			s.mu.Unlock()
			return old, false, nil
		}
		e.Created = old.Created
	}
	s.entries[key] = e
	s.offenses[key] = Offense{Count: count, Last: now}
	s.rebuildNetsLocked()
	s.mu.Unlock()
	s.notify(Change{Added: []Entry{e}})
	return e, true, nil
}

// Unban 解除封禁，返回是否存在该记录。累计次数保留，用于之后的递增封禁。
func (s *Store) Unban(key string) bool {
	s.mu.Lock()
	e, ok := s.entries[key]
	if ok {
		delete(s.entries, key)
		s.rebuildNetsLocked()
	}
	s.mu.Unlock()
	if ok {
		s.notify(Change{Removed: []Entry{e}})
	}
	return ok
}

// Get 返回 key 对应的未过期记录。
func (s *Store) Get(key string) (Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[key]
	if !ok || e.Expired(time.Now()) {
		return Entry{}, false
	}
	return e, true
}

// Prune 删除已到期的记录与过期的累计次数，返回删除的记录。
func (s *Store) Prune() []Entry {
	now := time.Now()
	var removed []Entry
	s.mu.Lock()
	for k, e := range s.entries {
		if e.Expired(now) {
			removed = append(removed, e)
			delete(s.entries, k)
		}
	}
	for k, o := range s.offenses {
		if _, banned := s.entries[k]; !banned && now.Sub(o.Last) > offenseTTL {
			delete(s.offenses, k)
		}
	}
	if len(removed) > 0 {
		s.rebuildNetsLocked()
	}
	s.mu.Unlock()
	s.notify(Change{Removed: removed, Expired: true})
	return removed
}

// List 返回尚未到期的封禁，最近封禁的在前。
func (s *Store) List() []Entry {
	now := time.Now()
	s.mu.RLock()
	out := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		if !e.Expired(now) {
			out = append(out, e)
		}
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Created.Equal(out[j].Created) {
			return out[i].Created.After(out[j].Created)
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// Len 返回尚未到期的封禁数量。
func (s *Store) Len() int {
	now := time.Now()
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, e := range s.entries {
		if !e.Expired(now) {
			n++
		}
	}
	return n
}

// Export 返回全部记录与累计次数，用于持久化。
func (s *Store) Export() ([]Entry, map[string]Offense) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	offenses := make(map[string]Offense, len(s.offenses))
	for k, o := range s.offenses {
		offenses[k] = o
	}
	return entries, offenses
}

// Import 合并持久化的记录（跳过已到期与格式无效的），已存在的键以内存中的为准。
func (s *Store) Import(entries []Entry, offenses map[string]Offense) {
	now := time.Now()
	var added []Entry
	s.mu.Lock()
	for _, e := range entries {
		if e.Expired(now) {
			continue
		}
		if _, ok := KeyPrefix(e.Key); !ok {
			continue
		}
		if _, ok := s.entries[e.Key]; ok {
			continue
		}
		s.entries[e.Key] = e
		added = append(added, e)
	}
	for k, o := range offenses {
		if cur, ok := s.offenses[k]; !ok || o.Last.After(cur.Last) {
			s.offenses[k] = o
		}
	}
	s.rebuildNetsLocked()
	s.mu.Unlock()
	s.notify(Change{Added: added})
}

// rebuildNetsLocked 重建网段列表，调用方需持有写锁。
func (s *Store) rebuildNetsLocked() {
	s.nets = s.nets[:0]
	for k := range s.entries {
		if IsSingle(k) {
			continue
		}
		if p, ok := KeyPrefix(k); ok {
			s.nets = append(s.nets, p)
		}
	}
}

// Listener 包装监听器：被封禁地址的连接在 Accept 时直接关闭，不交给上层处理。
func (s *Store) Listener(ln net.Listener) net.Listener {
	return &listener{Listener: ln, s: s}
}

type listener struct {
	net.Listener
	s *Store
}

func (l *listener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if l.s.BannedAddr(c.RemoteAddr()) {
			c.Close()
			continue
		}
		return c, nil
	}
}
