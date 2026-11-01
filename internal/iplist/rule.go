package iplist

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strings"
)

// Access 是规则级的来源 IP 访问控制配置，嵌入端口转发、协议分流、STUN 穿透等四层入口的规则。
// 封禁表优先于名单：入口先按封禁表拒绝，再按名单判断。
type Access struct {
	// IPMode 为来源 IP 访问控制模式：""（关闭）、whitelist（只允许名单内来源）、blacklist（拒绝名单内来源）
	IPMode string `json:"ipMode"`
	// IPList 为手填的 IP 或网段
	IPList []string `json:"ipList"`
	// IPSets 为引用的订阅 IP 列表 key（在 Web 服务的 IP 列表中维护），与 IPList 一起参与判断
	IPSets []string `json:"ipSets"`
}

// Normalize 校验访问控制配置，并去掉名单中的空项与重复的订阅列表。
func (a *Access) Normalize() error {
	if !ValidMode(a.IPMode) {
		return fmt.Errorf("未知的访问控制模式 %q", a.IPMode)
	}
	var list []string
	for _, x := range a.IPList {
		if x = strings.TrimSpace(x); x != "" {
			list = append(list, x)
		}
	}
	a.IPList = list
	if _, err := ParsePrefixes(a.IPList); err != nil {
		return err
	}
	var sets []string
	for _, k := range a.IPSets {
		if k = strings.TrimSpace(k); k != "" && !slices.Contains(sets, k) {
			sets = append(sets, k)
		}
	}
	a.IPSets = sets
	if a.IPMode == ModeWhitelist && len(a.IPList) == 0 && len(a.IPSets) == 0 {
		return errors.New("白名单模式需要填写 IP / 网段或选择订阅列表")
	}
	return nil
}

// CheckSets 检查启用访问控制时引用的订阅列表都存在，供保存规则时持锁调用。
func (a *Access) CheckSets() error {
	if a.IPMode == ModeOff {
		return nil
	}
	for _, k := range a.IPSets {
		if _, ok := Default.Name(k); !ok {
			return fmt.Errorf("引用的 IP 列表 %q 不存在", k)
		}
	}
	return nil
}

// AddUsers 把规则对订阅列表的引用追加到 users（列表 key -> 引用方描述），访问控制关闭时不登记。
func (a *Access) AddUsers(users map[string][]string, label string) {
	if a.IPMode == ModeOff {
		return
	}
	for _, k := range a.IPSets {
		users[k] = append(users[k], label)
	}
}

// Filter 是编译后的访问控制，运行中只读，可在多个连接间并发使用。
// 订阅列表按 key 在注册表中实时查询，列表刷新后无需重新编译即可生效。
type Filter struct {
	mode     string
	prefixes []netip.Prefix
	sets     []string
}

// Compile 编译访问控制；关闭时返回 nil（nil Filter 放行所有来源）。
// 配置已校验过，网段解析失败只可能来自手改的配置文件，按空名单处理。
func (a *Access) Compile() *Filter {
	if a.IPMode == ModeOff {
		return nil
	}
	prefixes, _ := ParsePrefixes(a.IPList)
	return &Filter{mode: a.IPMode, prefixes: prefixes, sets: slices.Clone(a.IPSets)}
}

// Allows 判断来源地址是否允许。无法解析出 IP 的地址在白名单下按不在名单中处理，其它模式放行。
func (f *Filter) Allows(addr net.Addr) bool {
	if f == nil {
		return true
	}
	a, ok := AddrOf(addr)
	if !ok {
		return f.mode != ModeWhitelist
	}
	return Default.Allowed(f.mode, a, f.prefixes, f.sets)
}

// AddrOf 取出 TCP/UDP 地址中的 IP（IPv4 映射地址还原为 IPv4）。
func AddrOf(addr net.Addr) (netip.Addr, bool) {
	switch v := addr.(type) {
	case *net.TCPAddr:
		if a, ok := netip.AddrFromSlice(v.IP); ok {
			return a.Unmap(), true
		}
		return netip.Addr{}, false
	case *net.UDPAddr:
		if a, ok := netip.AddrFromSlice(v.IP); ok {
			return a.Unmap(), true
		}
		return netip.Addr{}, false
	case nil:
		return netip.Addr{}, false
	}
	ap, err := netip.ParseAddrPort(addr.String())
	if err != nil {
		return netip.Addr{}, false
	}
	return ap.Addr().Unmap(), true
}

// PublishUsers 向注册表登记 owner 模块中各条规则对订阅列表的引用。
// get 返回规则的访问控制与引用方描述（如“portforward / 规则名”），访问控制关闭的规则不登记。
func PublishUsers[T any](owner string, items []T, get func(*T) (*Access, string)) {
	users := map[string][]string{}
	for i := range items {
		a, label := get(&items[i])
		a.AddUsers(users, label)
	}
	Default.SetUsers(owner, users)
}
