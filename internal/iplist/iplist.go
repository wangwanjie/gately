// Package iplist 解析 IP 列表文本（每行一个 IP、CIDR 或地址范围），合并为有序区间后按二分查找判断地址是否在列表中，
// 供 Web 服务站点与 frps 实例的访问控制引用订阅的省市、运营商或黑名单 IP 列表。
package iplist

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"slices"
	"strings"
	"sync"
)

// MaxEntries 是单个列表允许的最多条目数，防止误填超大文件占满内存。
const MaxEntries = 1_000_000

// span 是一段闭区间 [lo, hi]，lo 与 hi 属于同一地址族。
type span struct {
	lo, hi netip.Addr
}

// Set 是解析后的 IP 列表，只读，可并发查询。
type Set struct {
	v4, v6 []span
	// entries 为原始有效条目数（合并前），用于展示
	entries int
}

// Len 返回解析出的有效条目数。
func (s *Set) Len() int {
	if s == nil {
		return 0
	}
	return s.entries
}

// Contains 判断地址是否在列表中。IPv4 映射的 IPv6 地址按 IPv4 处理。
func (s *Set) Contains(a netip.Addr) bool {
	if s == nil || !a.IsValid() {
		return false
	}
	a = a.Unmap()
	list := s.v6
	if a.Is4() {
		list = s.v4
	}
	// 找到第一个上界不小于 a 的区间，再看下界是否覆盖 a
	i, _ := slices.BinarySearchFunc(list, a, func(sp span, t netip.Addr) int { return sp.hi.Compare(t) })
	return i < len(list) && list[i].lo.Compare(a) <= 0
}

// Parse 读取 IP 列表。支持 IP、CIDR 与 “起始-结束” 范围，# ; // 之后为注释，空行忽略；
// 每行只取第一个字段，因此 “1.2.3.0/24 广东电信” 这类带说明的行也能识别。无法识别的行计入 skipped。
func Parse(r io.Reader) (set *Set, skipped int, err error) {
	set = &Set{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		for _, mark := range []string{"#", ";", "//"} {
			if i := strings.Index(line, mark); i >= 0 {
				line = line[:i]
			}
		}
		fields := strings.FieldsFunc(line, func(r rune) bool { return r == ' ' || r == '\t' || r == ',' })
		if len(fields) == 0 {
			continue
		}
		sp, ok := parseEntry(fields[0])
		if !ok {
			skipped++
			continue
		}
		if set.entries++; set.entries > MaxEntries {
			return nil, skipped, fmt.Errorf("条目超过 %d 条上限", MaxEntries)
		}
		if sp.lo.Is4() {
			set.v4 = append(set.v4, sp)
		} else {
			set.v6 = append(set.v6, sp)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, skipped, err
	}
	if set.entries == 0 {
		return nil, skipped, errors.New("没有识别到任何 IP 或网段")
	}
	set.v4, set.v6 = merge(set.v4), merge(set.v6)
	return set, skipped, nil
}

// parseEntry 把单个条目解析为区间。
func parseEntry(s string) (span, bool) {
	if lo, hi, ok := strings.Cut(s, "-"); ok {
		a, err1 := netip.ParseAddr(strings.TrimSpace(lo))
		b, err2 := netip.ParseAddr(strings.TrimSpace(hi))
		a, b = a.Unmap(), b.Unmap()
		if err1 != nil || err2 != nil || a.Is4() != b.Is4() || a.Compare(b) > 0 {
			return span{}, false
		}
		return span{a, b}, true
	}
	if strings.Contains(s, "/") {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return span{}, false
		}
		if p.Addr().Is4In6() {
			// ::ffff:1.2.3.0/120 这类写法按 IPv4 网段处理
			if p.Bits() < 96 {
				return span{}, false
			}
			p = netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96)
		}
		p = p.Masked()
		return span{p.Addr(), lastAddr(p)}, true
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return span{}, false
	}
	a = a.Unmap()
	return span{a, a}, true
}

// lastAddr 返回网段中的最后一个地址。
func lastAddr(p netip.Prefix) netip.Addr {
	b := p.Addr().AsSlice()
	for i := p.Bits(); i < len(b)*8; i++ {
		b[i/8] |= 1 << (7 - uint(i%8))
	}
	a, _ := netip.AddrFromSlice(b)
	return a
}

// merge 排序并合并重叠或相邻的区间。
func merge(list []span) []span {
	if len(list) == 0 {
		return nil
	}
	slices.SortFunc(list, func(a, b span) int { return a.lo.Compare(b.lo) })
	out := list[:1]
	for _, sp := range list[1:] {
		last := &out[len(out)-1]
		// 与上一段重叠或紧邻（上一段结束地址的下一个就是本段起点）时合并
		if next := last.hi.Next(); sp.lo.Compare(last.hi) <= 0 || (next.IsValid() && next == sp.lo) {
			if sp.hi.Compare(last.hi) > 0 {
				last.hi = sp.hi
			}
			continue
		}
		out = append(out, sp)
	}
	return slices.Clip(out)
}

// Registry 保存按 key 索引的已加载列表，查询时无锁读取。
type Registry struct {
	sets sync.Map // key -> *Set

	mu sync.RWMutex
	// catalog 为订阅列表目录（key -> 名称），users 为 模块 -> 列表 key -> 引用方描述
	catalog map[string]string
	users   map[string]map[string][]string
}

// Default 是进程内共享的列表注册表。
var Default = &Registry{}

// Store 设置（或替换）列表；set 为 nil 时删除。
func (r *Registry) Store(key string, set *Set) {
	if set == nil {
		r.sets.Delete(key)
		return
	}
	r.sets.Store(key, set)
}

// Get 返回已加载的列表，未加载时 ok 为 false。
func (r *Registry) Get(key string) (*Set, bool) {
	v, ok := r.sets.Load(key)
	if !ok {
		return nil, false
	}
	return v.(*Set), true
}
