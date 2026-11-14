// Package knockgate 是端口敲门（SPA）授权的进程内登记表，供其他模块按敲门结果放行连接。
//
// 敲门模块在来源地址敲门成功后登记授权（地址 + 敲门规则 + 到期时间）；协议分流的路由、端口转发规则
// 选择了“需要敲门”时，新连接只有来源地址持有该规则的有效授权才会转发。与内核防火墙拦截不同，
// 这一层在 Gately 进程内判断，不依赖 nftables / iptables 与 NET_ADMIN 权限，Gately 停止时也不会残留拦截规则。
//
// 协议分流识别到经 TCP 发来的敲门报文（fwknop 客户端的 -P tcp）时，通过 Submit 交给敲门模块处理。
//
// 本包不依赖敲门模块：使用定制构建去掉敲门模块时登记表始终为空，选择了敲门的路由与规则拒绝全部连接。
package knockgate

import (
	"errors"
	"net/netip"
	"slices"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// maxGrants 为同时保存的授权数上限，超过时先清理到期授权，仍然超过则拒绝新授权（防止内存被耗尽）
const maxGrants = 65536

// MaxWebGrantsPerUser 为同一用户经网页登录对同一规则同时持有的授权数上限：超过时替换最早到期的一个。
// 账号被盗用后不停更换出口地址登录，也只能占用有限的授权，不会把登记表挤满。
const MaxWebGrantsPerUser = 16

// 授权来源，见 Grant.Source
const (
	// SourceSPA 为敲门报文（fwknop 客户端）取得的授权
	SourceSPA = "spa"
	// SourceWeb 为在放行入口站点通过 Gately 认证后取得的授权
	SourceWeb = "web"
)

// Rule 是一条可被引用的敲门规则。
type Rule struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Enable 为规则是否启用；停用的规则不会产生新的授权
	Enable bool `json:"enable"`
}

// Grant 是一个来源地址（或 IPv6 网段）持有的授权。
type Grant struct {
	// Prefix 为放行的地址：单个地址为 /32、/128，网页登录按设置可放行整个 IPv6 网段（如 /64）
	Prefix netip.Prefix `json:"prefix"`
	Rule   string       `json:"rule"`
	Until  time.Time    `json:"until"`
	// Source 为授权来源：SourceSPA 或 SourceWeb
	Source string `json:"source"`
	// Owner 为网页登录放行的用户与入口站点；敲门报文的授权为零值
	Owner
	// Addrs 为网页登录放行时实际登录（或探测）的单个地址：放行网段时进程内按网段判断，
	// 系统防火墙（敲门规则开启防火墙拦截时）只写入这些地址，见 MaxWebAddrs
	Addrs []netip.Addr `json:"addrs,omitempty"`
}

// MaxWebAddrs 为一个网页登录授权记录的实际地址数上限，超过时去掉最早的。
const MaxWebAddrs = 8

// Owner 是网页登录授权的持有者。
type Owner struct {
	// User 为登录的用户身份（用户名、共享用户名或 提供方:标识）
	User string `json:"user,omitempty"`
	// Site 为放行入口站点的 key，SiteName 为其名称（用于展示）
	Site     string `json:"site,omitempty"`
	SiteName string `json:"siteName,omitempty"`
	// Cred 为登录时用户凭据的指纹：修改密码、删除用户后据此撤销，不对外输出
	Cred string `json:"-"`
	// Aliases 为第三方登录身份的其他标识（如 GitHub 数字 ID），撤销检查时按允许列表匹配，不对外输出
	Aliases []string `json:"-"`
}

// Addr 返回授权的地址（网段时为网络地址）。
func (g Grant) Addr() netip.Addr { return g.Prefix.Addr() }

// Single 判断授权是否只针对单个地址。
func (g Grant) Single() bool { return g.Prefix.Bits() == g.Prefix.Addr().BitLen() }

// String 返回授权地址的显示形式：单个地址不带前缀长度，网段带前缀长度。
func (g Grant) String() string {
	if g.Single() {
		return g.Prefix.Addr().String()
	}
	return g.Prefix.String()
}

// Submitter 处理经其他通道（协议分流的 TCP 连接）收到的敲门报文，via 为通道说明（用于日志）。
type Submitter func(packet []byte, src netip.Addr, via string) error

// ErrUnavailable 表示敲门模块没有运行，无法处理报文。
var ErrUnavailable = errors.New("端口敲门模块未运行")

// Registry 是授权登记表，零值不可用，使用 New 创建。
type Registry struct {
	mu     sync.RWMutex
	grants map[grantKey]grantInfo
	// v6Bits 为登记表中 IPv6 网段授权（前缀长度小于 128）各前缀长度的个数：Allowed 只需按出现过的长度逐个查找，
	// 通常只有一两种，判断仍是常数次 map 查找
	v6Bits map[int]int
	rules  []Rule
	submit Submitter
	// users 为各模块对敲门规则的引用：模块 -> 规则 key -> 引用方描述
	users map[string]map[string][]string
	// spaPorts 为各模块接收敲门报文的 TCP 端口：模块 -> 端口（协议分流中带敲门路由的启用规则的监听端口）
	spaPorts map[string][]int
	// protected 为敲门模块在系统防火墙中拦截的端口，形如 "tcp/22"
	protected []string
	// now 可在测试中替换
	now func() time.Time
	// onChange 在授权增加、延长或撤销后调用（不持有锁），见 SetOnChange
	onChange atomic.Pointer[func()]
}

type grantKey struct {
	prefix netip.Prefix
	rule   string
}

type grantInfo struct {
	until  time.Time
	source string
	owner  Owner
	addrs  []netip.Addr
}

// Default 是进程共享的登记表。
var Default = New()

// New 创建空登记表。
func New() *Registry {
	return &Registry{grants: map[grantKey]grantInfo{}, v6Bits: map[int]int{}, now: time.Now}
}

// SetClock 替换判断到期使用的时钟，供测试使用；nil 恢复为系统时钟。
func (r *Registry) SetClock(now func() time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if now == nil {
		now = time.Now
	}
	r.now = now
}

// SetRules 发布当前的敲门规则目录，供其他模块的编辑界面选择与校验引用。
func (r *Registry) SetRules(rules []Rule) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rules = slices.Clone(rules)
}

// Rules 返回敲门规则目录。
func (r *Registry) Rules() []Rule {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := slices.Clone(r.rules)
	if out == nil {
		out = []Rule{}
	}
	return out
}

// RuleName 返回规则名称，规则不存在时返回空串与 false。
func (r *Registry) RuleName(key string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, x := range r.rules {
		if x.Key == key {
			return x.Name, true
		}
	}
	return "", false
}

// ruleEnabledLocked 判断规则存在且已启用。调用方持有 r.mu。
func (r *Registry) ruleEnabledLocked(key string) bool {
	for _, x := range r.rules {
		if x.Key == key {
			return x.Enable
		}
	}
	return false
}

// singlePrefix 返回只含 addr 一个地址的网段。
func singlePrefix(addr netip.Addr) netip.Prefix {
	addr = addr.Unmap()
	return netip.PrefixFrom(addr, addr.BitLen())
}

// normPrefix 把 IPv4 映射地址（::ffff:a.b.c.d/n）还原为 IPv4 网段并清零主机位；无效时返回零值。
func normPrefix(p netip.Prefix) netip.Prefix {
	if !p.IsValid() {
		return netip.Prefix{}
	}
	a, bits := p.Addr(), p.Bits()
	if a.Is4In6() {
		if bits < 96 {
			return netip.Prefix{}
		}
		a, bits = a.Unmap(), bits-96
	}
	return netip.PrefixFrom(a, bits).Masked()
}

// Allow 登记 addr 在 until 之前持有 rule 的授权（敲门报文取得）；已有更晚到期的授权时保留较晚的时间。
func (r *Registry) Allow(addr netip.Addr, rule string, until time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.putLocked(grantKey{singlePrefix(addr), rule}, grantInfo{until: until, source: SourceSPA})
}

// ErrRuleDisabled 表示规则不存在或已停用，不能登记网页登录的授权。
var ErrRuleDisabled = errors.New("敲门规则不存在或已停用")

// AllowWeb 登记网页登录取得的授权：p 为放行的地址或 IPv6 网段，o 为登录的用户与入口站点，seen 为实际登录（或探测）的
// 单个地址（应在 p 内，否则不记录）。规则不存在或已停用时返回 ErrRuleDisabled。同一用户对同一规则的授权超过
// MaxWebGrantsPerUser 个时，替换其中最早到期的一个。同一网段已有授权时延长到较晚的到期时间、追加实际地址，
// 并改记为本次登录的用户。
func (r *Registry) AllowWeb(p netip.Prefix, rule string, until time.Time, o Owner, seen netip.Addr) error {
	p = normPrefix(p)
	if !p.IsValid() {
		return errors.New("地址无效")
	}
	seen = seen.Unmap()
	if !seen.IsValid() || !p.Contains(seen) {
		seen = netip.Addr{}
	}
	err := r.allowWeb(p, rule, until, o, seen)
	if err == nil {
		r.notify()
	}
	return err
}

func (r *Registry) allowWeb(p netip.Prefix, rule string, until time.Time, o Owner, seen netip.Addr) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.ruleEnabledLocked(rule) {
		return ErrRuleDisabled
	}
	k := grantKey{p, rule}
	info := grantInfo{until: until, source: SourceWeb, owner: o}
	if seen.IsValid() {
		info.addrs = []netip.Addr{seen}
	}
	if old, ok := r.grants[k]; ok {
		if old.until.After(until) {
			info.until = old.until
		}
		// 敲门报文对同一地址的授权改记为网页登录：保留较晚的到期时间
		info.addrs = addAddr(old.addrs, seen)
		r.grants[k] = info
		return nil
	}
	// 同一用户的授权过多时替换最早到期的一个：只在新登记地址时扫描，续期不需要
	var oldest grantKey
	n := 0
	for gk, gi := range r.grants {
		if gi.source != SourceWeb || gi.owner.User != o.User || gk.rule != rule {
			continue
		}
		if n == 0 || gi.until.Before(r.grants[oldest].until) {
			oldest = gk
		}
		n++
	}
	if n >= MaxWebGrantsPerUser {
		r.deleteLocked(oldest)
	}
	return r.putLocked(k, info)
}

// addAddr 把 a 追加到地址列表（已存在时移到末尾），超过 MaxWebAddrs 时去掉最早的。返回新列表，不修改原列表。
func addAddr(list []netip.Addr, a netip.Addr) []netip.Addr {
	out := make([]netip.Addr, 0, len(list)+1)
	for _, x := range list {
		if x != a {
			out = append(out, x)
		}
	}
	if a.IsValid() {
		out = append(out, a)
	}
	if len(out) > MaxWebAddrs {
		out = out[len(out)-MaxWebAddrs:]
	}
	return out
}

// SetOnChange 设置授权变化（新增、延长、撤销）后的回调，nil 取消。回调在不持有锁时同步调用，应只做轻量的通知
// （敲门模块据此把网页登录的授权同步到系统防火墙）。敲门报文的授权由敲门模块自己写防火墙，Allow 不触发回调。
func (r *Registry) SetOnChange(f func()) {
	if f == nil {
		r.onChange.Store(nil)
		return
	}
	r.onChange.Store(&f)
}

func (r *Registry) notify() {
	if f := r.onChange.Load(); f != nil {
		(*f)()
	}
}

// putLocked 登记或延长授权。调用方持有 r.mu。
func (r *Registry) putLocked(k grantKey, info grantInfo) error {
	if old, ok := r.grants[k]; ok {
		if info.until.After(old.until) {
			old.until = info.until
		}
		r.grants[k] = old
		return nil
	}
	if len(r.grants) >= maxGrants {
		r.pruneLocked()
		if len(r.grants) >= maxGrants {
			return errors.New("授权数量已达上限")
		}
	}
	r.grants[k] = info
	if a := k.prefix.Addr(); a.Is6() && k.prefix.Bits() < 128 {
		r.v6Bits[k.prefix.Bits()]++
	}
	return nil
}

// deleteLocked 删除一个授权。调用方持有 r.mu。
func (r *Registry) deleteLocked(k grantKey) {
	if _, ok := r.grants[k]; !ok {
		return
	}
	delete(r.grants, k)
	if a := k.prefix.Addr(); a.Is6() && k.prefix.Bits() < 128 {
		b := k.prefix.Bits()
		if r.v6Bits[b]--; r.v6Bits[b] <= 0 {
			delete(r.v6Bits, b)
		}
	}
}

// Allowed 判断 addr 当前是否持有 rule 的有效授权（单个地址的授权，或包含该地址的 IPv6 网段授权）。
func (r *Registry) Allowed(addr netip.Addr, rule string) bool {
	if rule == "" || !addr.IsValid() {
		return false
	}
	addr = addr.Unmap()
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := r.now()
	if info, ok := r.grants[grantKey{singlePrefix(addr), rule}]; ok && now.Before(info.until) {
		return true
	}
	if !addr.Is6() {
		return false
	}
	for bits := range r.v6Bits {
		p, err := addr.Prefix(bits)
		if err != nil {
			continue
		}
		if info, ok := r.grants[grantKey{p, rule}]; ok && now.Before(info.until) {
			return true
		}
	}
	return false
}

// Remaining 返回 p 持有 rule 的授权的剩余时长，没有有效授权时返回 0。
func (r *Registry) Remaining(p netip.Prefix, rule string) time.Duration {
	p = normPrefix(p)
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, ok := r.grants[grantKey{p, rule}]
	if !ok {
		return 0
	}
	return max(info.until.Sub(r.now()), 0)
}

// Lookup 返回 p 持有 rule 的授权的剩余时长、单个地址 seen 是否已记录在该授权上（见 Grant.Addrs），
// 以及记录的地址是否已达上限 MaxWebAddrs；没有有效授权时剩余时长为 0。
func (r *Registry) Lookup(p netip.Prefix, rule string, seen netip.Addr) (rem time.Duration, known, full bool) {
	p = normPrefix(p)
	seen = seen.Unmap()
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, ok := r.grants[grantKey{p, rule}]
	if !ok {
		return 0, false, false
	}
	return max(info.until.Sub(r.now()), 0), slices.Contains(info.addrs, seen), len(info.addrs) >= MaxWebAddrs
}

// Revoke 撤销 addr 持有的 rule 授权；rule 为空时撤销该地址的全部授权。返回撤销的数量。
// 只撤销单个地址的授权，网段授权用 RevokePrefix 撤销。
func (r *Registry) Revoke(addr netip.Addr, rule string) int {
	return r.RevokePrefix(singlePrefix(addr), rule)
}

// RevokePrefix 撤销 p（单个地址或网段）持有的 rule 授权；rule 为空时撤销 p 的全部授权。返回撤销的数量。
func (r *Registry) RevokePrefix(p netip.Prefix, rule string) int {
	p = normPrefix(p)
	return r.RevokeFunc(func(g Grant) bool {
		return g.Prefix == p && (rule == "" || g.Rule == rule)
	})
}

// RevokeFunc 撤销 match 返回 true 的授权，返回撤销的数量。match 在持有登记表锁时调用，不能再调用登记表的方法。
func (r *Registry) RevokeFunc(match func(Grant) bool) int {
	r.mu.Lock()
	n := 0
	for k, info := range r.grants {
		if match(grantOf(k, info)) {
			r.deleteLocked(k)
			n++
		}
	}
	r.mu.Unlock()
	if n > 0 {
		r.notify()
	}
	return n
}

// RevokeRule 撤销某条规则的全部授权（规则删除、停用或更换密钥时调用）。
func (r *Registry) RevokeRule(rule string) {
	r.RevokeFunc(func(g Grant) bool { return g.Rule == rule })
}

// Clear 撤销全部授权（敲门模块停止时调用）。
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.grants = map[grantKey]grantInfo{}
	r.v6Bits = map[int]int{}
}

func grantOf(k grantKey, info grantInfo) Grant {
	return Grant{Prefix: k.prefix, Rule: k.rule, Until: info.until, Source: info.source, Owner: info.owner, Addrs: slices.Clone(info.addrs)}
}

// List 返回未到期的授权，按到期时间从晚到早排列。
func (r *Registry) List() []Grant {
	r.mu.RLock()
	now := r.now()
	out := make([]Grant, 0, len(r.grants))
	for k, info := range r.grants {
		if now.Before(info.until) {
			out = append(out, grantOf(k, info))
		}
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Until.Equal(out[j].Until) {
			return out[i].Until.After(out[j].Until)
		}
		if out[i].Prefix != out[j].Prefix {
			if out[i].Prefix.Addr() != out[j].Prefix.Addr() {
				return out[i].Prefix.Addr().Less(out[j].Prefix.Addr())
			}
			return out[i].Prefix.Bits() < out[j].Prefix.Bits()
		}
		return out[i].Rule < out[j].Rule
	})
	return out
}

// Prune 清除已到期的授权，返回清除的数量。
func (r *Registry) Prune() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pruneLocked()
}

func (r *Registry) pruneLocked() int {
	now := r.now()
	n := 0
	for k, info := range r.grants {
		if !now.Before(info.until) {
			r.deleteLocked(k)
			n++
		}
	}
	return n
}

// SetUsers 登记 owner（模块）对敲门规则的引用：users 为 规则 key -> 引用方描述（如“端口转发 / SSH”）。
// 敲门模块据此展示规则的引用方，并阻止删除仍被引用的规则。
func (r *Registry) SetUsers(owner string, users map[string][]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.users == nil {
		r.users = map[string]map[string][]string{}
	}
	if len(users) == 0 {
		delete(r.users, owner)
		return
	}
	r.users[owner] = users
}

// Users 返回引用该规则的描述，按模块名排序。
func (r *Registry) Users(rule string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	owners := make([]string, 0, len(r.users))
	for o := range r.users {
		owners = append(owners, o)
	}
	slices.Sort(owners)
	out := []string{}
	for _, o := range owners {
		out = append(out, r.users[o][rule]...)
	}
	return out
}

// SetSubmitter 设置处理经其他通道收到的敲门报文的函数，nil 表示敲门模块未运行。
func (r *Registry) SetSubmitter(f Submitter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.submit = f
}

// Submit 把经其他通道收到的报文交给敲门模块处理；模块未运行时返回 ErrUnavailable。
func (r *Registry) Submit(packet []byte, src netip.Addr, via string) error {
	r.mu.RLock()
	f := r.submit
	r.mu.RUnlock()
	if f == nil {
		return ErrUnavailable
	}
	return f(packet, src.Unmap(), via)
}

// SetSPAPorts 登记 owner（模块）接收敲门报文的 TCP 端口。敲门模块不会在防火墙中拦截这些端口：
// 拦截后经这些端口发来的敲门报文（fwknop -P tcp）会在建立连接前被丢弃，永远无法敲门。
func (r *Registry) SetSPAPorts(owner string, ports []int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.spaPorts == nil {
		r.spaPorts = map[string][]int{}
	}
	if len(ports) == 0 {
		delete(r.spaPorts, owner)
		return
	}
	r.spaPorts[owner] = slices.Clone(ports)
}

// SPAPorts 返回各模块登记的接收敲门报文的 TCP 端口，升序、去重。
func (r *Registry) SPAPorts() []int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []int
	for _, ports := range r.spaPorts {
		out = append(out, ports...)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// SetProtected 发布敲门模块在系统防火墙中拦截的端口（形如 "tcp/22"），供协议分流保存敲门路由时检查冲突。
func (r *Registry) SetProtected(ports []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.protected = slices.Clone(ports)
}

// Protected 判断端口是否被敲门模块的防火墙拦截。
func (r *Registry) Protected(proto string, port int) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Contains(r.protected, proto+"/"+strconv.Itoa(port))
}
