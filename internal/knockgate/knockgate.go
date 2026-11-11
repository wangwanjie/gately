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
	"sync"
	"time"
)

// maxGrants 为同时保存的授权数上限，超过时先清理到期授权，仍然超过则拒绝新授权（防止内存被耗尽）
const maxGrants = 65536

// Rule 是一条可被引用的敲门规则。
type Rule struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Enable 为规则是否启用；停用的规则不会产生新的授权
	Enable bool `json:"enable"`
}

// Grant 是一个来源地址持有的授权。
type Grant struct {
	Addr  netip.Addr `json:"addr"`
	Rule  string     `json:"rule"`
	Until time.Time  `json:"until"`
}

// Submitter 处理经其他通道（协议分流的 TCP 连接）收到的敲门报文，via 为通道说明（用于日志）。
type Submitter func(packet []byte, src netip.Addr, via string) error

// ErrUnavailable 表示敲门模块没有运行，无法处理报文。
var ErrUnavailable = errors.New("端口敲门模块未运行")

// Registry 是授权登记表，零值不可用，使用 New 创建。
type Registry struct {
	mu     sync.RWMutex
	grants map[grantKey]time.Time
	rules  []Rule
	submit Submitter
	// users 为各模块对敲门规则的引用：模块 -> 规则 key -> 引用方描述
	users map[string]map[string][]string
	// now 可在测试中替换
	now func() time.Time
}

type grantKey struct {
	addr netip.Addr
	rule string
}

// Default 是进程共享的登记表。
var Default = New()

// New 创建空登记表。
func New() *Registry {
	return &Registry{grants: map[grantKey]time.Time{}, now: time.Now}
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

// Allow 登记 addr 在 until 之前持有 rule 的授权；已有更晚到期的授权时保留较晚的时间。
func (r *Registry) Allow(addr netip.Addr, rule string, until time.Time) error {
	addr = addr.Unmap()
	r.mu.Lock()
	defer r.mu.Unlock()
	k := grantKey{addr, rule}
	if old, ok := r.grants[k]; ok {
		if until.After(old) {
			r.grants[k] = until
		}
		return nil
	}
	if len(r.grants) >= maxGrants {
		r.pruneLocked()
		if len(r.grants) >= maxGrants {
			return errors.New("授权数量已达上限")
		}
	}
	r.grants[k] = until
	return nil
}

// Allowed 判断 addr 当前是否持有 rule 的有效授权。
func (r *Registry) Allowed(addr netip.Addr, rule string) bool {
	if rule == "" || !addr.IsValid() {
		return false
	}
	r.mu.RLock()
	until, ok := r.grants[grantKey{addr.Unmap(), rule}]
	now := r.now
	r.mu.RUnlock()
	return ok && now().Before(until)
}

// Revoke 撤销 addr 持有的 rule 授权；rule 为空时撤销该地址的全部授权。返回撤销的数量。
func (r *Registry) Revoke(addr netip.Addr, rule string) int {
	addr = addr.Unmap()
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for k := range r.grants {
		if k.addr == addr && (rule == "" || k.rule == rule) {
			delete(r.grants, k)
			n++
		}
	}
	return n
}

// RevokeRule 撤销某条规则的全部授权（规则删除、停用或更换密钥时调用）。
func (r *Registry) RevokeRule(rule string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k := range r.grants {
		if k.rule == rule {
			delete(r.grants, k)
		}
	}
}

// Clear 撤销全部授权（敲门模块停止时调用）。
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.grants = map[grantKey]time.Time{}
}

// List 返回未到期的授权，按到期时间从晚到早排列。
func (r *Registry) List() []Grant {
	r.mu.RLock()
	now := r.now()
	out := make([]Grant, 0, len(r.grants))
	for k, until := range r.grants {
		if now.Before(until) {
			out = append(out, Grant{Addr: k.addr, Rule: k.rule, Until: until})
		}
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Until.Equal(out[j].Until) {
			return out[i].Until.After(out[j].Until)
		}
		return out[i].Addr.Less(out[j].Addr)
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
	for k, until := range r.grants {
		if !now.Before(until) {
			delete(r.grants, k)
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
