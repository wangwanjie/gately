package webservice

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"
	"net/textproto"
	"strings"
	"sync/atomic"

	"golang.org/x/net/http/httpguts"
)

// defaultRealIPHeader 为配置了可信代理但没有指定头名时使用的头：
// 它的最右一项由最近的代理追加，自右向左跳过可信代理即可得到访客地址，客户端在左侧伪造的值不会被采用。
const defaultRealIPHeader = "X-Forwarded-For"

// realIPPolicy 为编译后的可信代理真实 IP 设置。
type realIPPolicy struct {
	from    []netip.Prefix
	headers []string
	// portHeader 为可信代理写入访客源端口的请求头，为空表示代理不提供端口（见 resolvePort）
	portHeader string
}

// realIPSource 在模块的全部监听器间共享当前生效的设置，保存全局设置时整体替换，不需要重新监听。
type realIPSource struct {
	p atomic.Pointer[realIPPolicy]
}

// ctxKeyPeer 在请求上下文中保存改写客户端地址前的原始对端（见 peerInfo）。
type ctxKeyPeer struct{}

// peerInfo 记录请求的原始对端：经可信 PROXY 来源进入时为 PROXY 头中的源地址，否则为 TCP 对端。
type peerInfo struct {
	addr netip.Addr
	// trusted 表示原始对端在可信代理列表中，请求中的真实 IP 头已被采信
	trusted bool
}

// normalizeRealIP 校验并规范化可信代理与真实 IP 头设置，返回规范化后的两个列表。
// 可信代理为空表示不启用，头名一并清空；可信代理非空而没有头名时使用 X-Forwarded-For。
func normalizeRealIP(from, headers []string) ([]string, []string, error) {
	from = cleanList(from)
	prefixes, err := parsePrefixes(from)
	if err != nil {
		return nil, nil, fmt.Errorf("可信代理：%v", err)
	}
	for i, p := range prefixes {
		// 0.0.0.0/0、::/0 等于信任所有来源，任何人都能用请求头伪造客户端地址
		if p.Bits() == 0 {
			return nil, nil, fmt.Errorf("可信代理不能包含 %s，这会让任何来源都能伪造客户端地址", from[i])
		}
	}
	if len(from) == 0 {
		return nil, nil, nil
	}
	var names []string
	seen := map[string]bool{}
	for _, h := range headers {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		if !httpguts.ValidHeaderFieldName(h) {
			return nil, nil, fmt.Errorf("无效的请求头名称 %q", h)
		}
		// 保留用户填写的大小写便于界面显示，按规范形式去重（读取请求头时不区分大小写）
		key := textproto.CanonicalMIMEHeaderKey(h)
		if !seen[key] {
			seen[key] = true
			names = append(names, h)
		}
	}
	if len(names) == 0 {
		names = []string{defaultRealIPHeader}
	}
	return from, names, nil
}

// normalizeRealPort 校验并规范化真实端口请求头：未启用真实 IP（可信代理为空）时清空。
func normalizeRealPort(from []string, header string) (string, error) {
	header = strings.TrimSpace(header)
	if len(cleanList(from)) == 0 || header == "" {
		return "", nil
	}
	if !httpguts.ValidHeaderFieldName(header) {
		return "", fmt.Errorf("无效的请求头名称 %q", header)
	}
	return header, nil
}

// set 按全局设置更新当前生效的策略；设置已在保存时校验，这里解析失败时视为未启用。
// portHeader 为可信代理写入访客源端口的请求头，可为空。
func (s *realIPSource) set(from, headers []string, portHeader string) {
	prefixes, err := parsePrefixes(from)
	if err != nil || len(prefixes) == 0 {
		s.p.Store(nil)
		return
	}
	if len(headers) == 0 {
		headers = []string{defaultRealIPHeader}
	}
	if !httpguts.ValidHeaderFieldName(portHeader) {
		portHeader = ""
	}
	s.p.Store(&realIPPolicy{from: prefixes, headers: headers, portHeader: portHeader})
}

// apply 在请求进入站点路由前确定访客的真实地址：原始对端在可信代理列表中时，按配置的头依次查找，
// 找到即改写 r.RemoteAddr，此后的日志、IP 名单、仅内网访问、国家过滤、认证失败封禁、统计与路径探测都使用该地址。
// 原始对端是 PROXY 协议处理后的地址：协议分流经回环转来并带 PROXY 头时，判断的是 PROXY 头中的访客地址，而非回环。
// 对端不可信或请求头中取不到有效地址时保持原地址，请求头一律不采信。
func (s *realIPSource) apply(r *http.Request) *http.Request {
	if s == nil {
		return r
	}
	p := s.p.Load()
	if p == nil {
		return r
	}
	peer := clientAddr(r)
	info := peerInfo{addr: peer.Addr()}
	if !p.trusts(info.addr) {
		return r
	}
	info.trusted = true
	r = r.WithContext(context.WithValue(r.Context(), ctxKeyPeer{}, info))
	if a, ok := p.resolve(r.Header); ok {
		// 访客的源端口只能由可信代理在端口头中告知；代理没有提供时端口记为 0（未知），
		// 不能沿用对端端口：那是代理到 Gately 这段连接的端口，向后端传递或在实时连接中显示都会误导
		r.RemoteAddr = netip.AddrPortFrom(a, p.resolvePort(r.Header)).String()
	}
	return r
}

// trusts 判断地址是否在可信代理列表中。
func (p *realIPPolicy) trusts(a netip.Addr) bool {
	if !a.IsValid() {
		return false
	}
	a = a.Unmap()
	for _, pf := range p.from {
		if pf.Contains(a) {
			return true
		}
	}
	return false
}

// resolve 按配置顺序查找第一个能给出有效地址的头。每个头的值（可能有多行、逗号分隔多项）自右向左查看：
// 跳过同样在可信代理列表中的地址，第一个不可信的地址即访客；最左侧的值可由客户端随意填写，只在其余各项都是可信代理时才会用到。
// 遇到无法解析的项时不再继续向左，该头视为无效，改看下一个头。
func (p *realIPPolicy) resolve(h http.Header) (netip.Addr, bool) {
	for _, name := range p.headers {
		var items []string
		for _, v := range h.Values(name) {
			items = append(items, strings.Split(v, ",")...)
		}
		var cand netip.Addr
		valid := len(items) > 0
		for i := len(items) - 1; i >= 0; i-- {
			a, ok := parseHeaderAddr(items[i])
			if !ok {
				valid = false
				break
			}
			if !p.trusts(a) {
				return a, true
			}
			cand = a
		}
		// 全部项都是可信代理时取最左一项
		if valid && cand.IsValid() {
			return cand, true
		}
	}
	return netip.Addr{}, false
}

// resolvePort 从端口头取得访客源端口，取不到或无效时返回 0。
// 代理通常覆盖写入该头（如 nginx 的 proxy_set_header X-Real-Port $remote_port），有多项时取最右一项，
// 即最近的代理写入的值；左侧的值可能是客户端自带的，不采用。
func (p *realIPPolicy) resolvePort(h http.Header) uint16 {
	if p.portHeader == "" {
		return 0
	}
	vals := h.Values(p.portHeader)
	if len(vals) == 0 {
		return 0
	}
	items := strings.Split(vals[len(vals)-1], ",")
	return parsePort(items[len(items)-1])
}

// parsePort 解析 1–65535 的十进制端口号，无效时返回 0。只接受纯数字，拒绝符号、空格内夹杂与前导加号。
func parsePort(s string) uint16 {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 5 {
		return 0
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	if n < 1 || n > 65535 {
		return 0
	}
	return uint16(n)
}

// parseHeaderAddr 解析头中的一项地址，兼容带端口（1.2.3.4:5678、[::1]:80）与方括号包裹的 IPv6。
func parseHeaderAddr(s string) (netip.Addr, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return netip.Addr{}, false
	}
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return ap.Addr().Unmap(), true
	}
	s = strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
	a, err := netip.ParseAddr(s)
	if err != nil || a.Zone() != "" {
		return netip.Addr{}, false
	}
	return a.Unmap(), true
}

// peerOf 返回请求的原始对端。没有经过 apply 改写时就是当前客户端地址。
func peerOf(r *http.Request) peerInfo {
	if info, ok := r.Context().Value(ctxKeyPeer{}).(peerInfo); ok {
		return info
	}
	return peerInfo{addr: clientAddr(r).Addr()}
}
