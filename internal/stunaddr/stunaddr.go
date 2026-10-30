// Package stunaddr 保存各条 STUN 穿透规则当前的公网地址，
// 供 Web 服务的重定向目标、动态域名的记录值与 Webhook 等按规则名引用（{STUN_规则名_PORT} 等变量）。
package stunaddr

import (
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type entry struct {
	name string
	addr netip.AddrPort
}

var (
	mu sync.RWMutex
	// addrs 以规则 key 为键：规则改名或重启时按 key 覆盖、删除，不会残留旧名字
	addrs = map[string]entry{}
)

// Set 记录规则当前的公网地址。
func Set(key, name string, addr netip.AddrPort) {
	mu.Lock()
	defer mu.Unlock()
	addrs[key] = entry{name: name, addr: addr}
}

// Delete 移除规则的地址（规则停止或穿透失败且不再重试时调用）。
func Delete(key string) {
	mu.Lock()
	defer mu.Unlock()
	delete(addrs, key)
}

// Lookup 按规则名查找公网地址；没有同名规则时再按 key 查找。
// 多条规则同名时取 key 排序最小的一条，保证结果稳定。
func Lookup(name string) (netip.AddrPort, bool) {
	mu.RLock()
	defer mu.RUnlock()
	var keys []string
	for k, e := range addrs {
		if e.name == name {
			keys = append(keys, k)
		}
	}
	if len(keys) > 0 {
		sort.Strings(keys)
		return addrs[keys[0]].addr, true
	}
	if e, ok := addrs[name]; ok {
		return e.addr, true
	}
	return netip.AddrPort{}, false
}

// 变量后缀：公网端口、IP、IP:端口
var varParts = []string{"_PORT", "_IP", "_ADDR"}

// ParseVar 解析 STUN_规则名_PORT / _IP / _ADDR 这类变量名（不含花括号），规则名中可以含下划线。
func ParseVar(name string) (rule, part string, ok bool) {
	rest, found := strings.CutPrefix(name, "STUN_")
	if !found {
		return "", "", false
	}
	for _, p := range varParts {
		if r, found := strings.CutSuffix(rest, p); found && r != "" {
			return r, p[1:], true
		}
	}
	return "", "", false
}

// Format 按变量后缀取地址的端口、IP 或 IP:端口。
func Format(addr netip.AddrPort, part string) string {
	switch part {
	case "PORT":
		return strconv.Itoa(int(addr.Port()))
	case "IP":
		return addr.Addr().String()
	}
	return addr.String()
}

// Resolve 解析一个变量名：isVar 表示是否为 STUN 变量，found 表示对应规则当前有公网地址。
func Resolve(name string) (value string, isVar, found bool) {
	rule, part, ok := ParseVar(name)
	if !ok {
		return "", false, false
	}
	addr, found := Lookup(rule)
	if !found {
		return "", true, false
	}
	return Format(addr, part), true, true
}

// Vars 扫描文本中引用的 {STUN_规则名_PORT} 等变量，返回能解析的变量值（键不含花括号）
// 与暂无公网地址的规则名（规则不存在、未启用或还没有穿透成功）。
func Vars(texts ...string) (vals map[string]string, missing []string) {
	vals = map[string]string{}
	seen := map[string]bool{}
	for _, s := range texts {
		for {
			start := strings.IndexByte(s, '{')
			if start < 0 {
				break
			}
			end := strings.IndexByte(s[start:], '}')
			if end < 0 {
				break
			}
			name := s[start+1 : start+end]
			s = s[start+end+1:]
			v, isVar, found := Resolve(name)
			if !isVar {
				continue
			}
			if found {
				vals[name] = v
				continue
			}
			if rule, _, _ := ParseVar(name); !seen[rule] {
				seen[rule] = true
				missing = append(missing, rule)
			}
		}
	}
	return vals, missing
}

// References 判断文本是否引用了指定规则名的 STUN 变量。
func References(text, rule string) bool {
	for _, p := range varParts {
		if strings.Contains(text, "{STUN_"+rule+p+"}") {
			return true
		}
	}
	return false
}
