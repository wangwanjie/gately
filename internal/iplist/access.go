package iplist

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"
)

// 访问控制模式，Web 服务站点与 frps 实例共用
const (
	ModeOff       = ""
	ModeWhitelist = "whitelist"
	ModeBlacklist = "blacklist"
)

// ValidMode 判断访问控制模式是否有效。
func ValidMode(mode string) bool {
	return mode == ModeOff || mode == ModeWhitelist || mode == ModeBlacklist
}

// ParsePrefixes 解析手填的 IP 或网段，空行忽略；单个 IP 按完整前缀处理。
func ParsePrefixes(list []string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, s := range list {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if strings.Contains(s, "/") {
			p, err := netip.ParsePrefix(s)
			if err != nil {
				return nil, fmt.Errorf("无效的网段 %q", s)
			}
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(s)
		if err != nil {
			return nil, fmt.Errorf("无效的 IP %q", s)
		}
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}

// Match 判断地址是否命中手填网段或任一引用的订阅列表；尚未载入的订阅列表按空列表处理。
func (r *Registry) Match(a netip.Addr, prefixes []netip.Prefix, sets []string) bool {
	a = a.Unmap()
	for _, p := range prefixes {
		if p.Contains(a) {
			return true
		}
	}
	for _, key := range sets {
		if set, ok := r.Get(key); ok && set.Contains(a) {
			return true
		}
	}
	return false
}

// Allowed 按访问控制模式判断地址是否允许：白名单需命中名单，黑名单需不命中，关闭时一律允许。
func (r *Registry) Allowed(mode string, a netip.Addr, prefixes []netip.Prefix, sets []string) bool {
	switch mode {
	case ModeWhitelist:
		return r.Match(a, prefixes, sets)
	case ModeBlacklist:
		return !r.Match(a, prefixes, sets)
	}
	return true
}

// Info 是订阅列表的目录项。
type Info struct {
	Key  string
	Name string
}

// SetCatalog 发布订阅列表目录（由维护列表的 Web 服务在列表增删改后调用），
// 供其它模块校验引用的列表是否存在；目录与模块自身的锁无关，查询时不会形成锁顺序问题。
func (r *Registry) SetCatalog(list []Info) {
	m := make(map[string]string, len(list))
	for _, x := range list {
		m[x.Key] = x.Name
	}
	r.mu.Lock()
	r.catalog = m
	r.mu.Unlock()
}

// Name 返回订阅列表的名称，列表不存在时 ok 为 false。
func (r *Registry) Name(key string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	name, ok := r.catalog[key]
	return name, ok
}

// SetUsers 登记 owner（模块）对订阅列表的引用：users 为 列表 key -> 引用方描述。
// Web 服务据此展示列表的引用方，并阻止删除仍被引用的列表。
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

// Users 返回其它模块中引用该列表的描述，按模块名排序。
func (r *Registry) Users(key string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	owners := make([]string, 0, len(r.users))
	for o := range r.users {
		owners = append(owners, o)
	}
	slices.Sort(owners)
	var out []string
	for _, o := range owners {
		out = append(out, r.users[o][key]...)
	}
	return out
}
