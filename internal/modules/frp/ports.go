package frp

import (
	"net"
	"net/netip"
	"sort"
	"time"

	"github.com/wangwanjie/gately/internal/openports"
	"github.com/wangwanjie/gately/internal/trustsrc"
)

// maxLearnedPorts 为单个 frps 实例记住的代理远程端口上限，防止异常客户端登记大量端口撑大防火墙集合。
const maxLearnedPorts = 4096

// maxAllowRange 为登记 allowPorts 范围时单个范围的最大宽度。
// 范围过宽（例如 1-65535）时登记会让扫描检测对几乎所有端口失效，这种范围只靠 frpc 实际申请过的端口（learnProxyPort）登记。
const maxAllowRange = 1024

// serverOwner 与 proxyOwner 是 frps 实例在 openports 中的登记名：
// 前者登记配置里确定的端口，后者登记 frpc 实际申请过的代理远程端口。
func serverOwner(key string) string { return "frps:" + key }
func proxyOwner(key string) string  { return "frps-proxy:" + key }

// serverOpenRanges 返回 frps 配置里确定会监听的端口：控制端口、vhost 端口、允许的远程端口范围，
// 以及对外监听的控制台端口。入侵防护的扫描检测据此把这些端口视为开放端口，
// frpc 掉线、frps 重启期间到这些端口的重连不会被当成端口扫描。
func serverOpenRanges(s ServerSettings) []openports.Range {
	var out []openports.Range
	add := func(p int) {
		if validPort(p) {
			out = append(out, openports.Range{Lo: p, Hi: p})
		}
	}
	add(s.BindPort)
	add(s.VhostHTTPPort)
	add(s.VhostHTTPSPort)
	if s.DashboardPort != 0 {
		// 控制台只监听回环地址时外部访问不到，不登记
		if ip := net.ParseIP(s.DashboardAddr); ip == nil || !ip.IsLoopback() {
			add(s.DashboardPort)
		}
	}
	ranges, _ := ParseAllowPorts(s.AllowPorts)
	for _, r := range ranges {
		if r.End-r.Start+1 <= maxAllowRange {
			out = append(out, openports.Range{Lo: r.Start, Hi: r.End})
		}
	}
	return out
}

// learnState 为运行中的 frps 实例学习代理端口所需的状态。
type learnState struct {
	// allow 为实例的 allowPorts 范围，为空表示 frps 不限制远程端口
	allow []PortRange
	// ports 为 frpc 申请过的 TCP 代理远程端口
	ports map[int]bool
	// bindPort 与 token 为 frpc 连接的端口与令牌，trusted 为用正确令牌登录过的 frpc 来源地址及其最近一次登录时间
	bindPort int
	token    string
	trusted  map[netip.Addr]time.Time
}

// allows 判断端口是否在 allowPorts 范围内。frps 在校验 allowPorts 之前调用插件，
// 范围外的申请随后会被 frps 拒绝，不能登记，否则持有令牌的客户端可以让扫描检测对任意端口失效。
func (st *learnState) allows(port int) bool {
	if len(st.allow) == 0 {
		return true
	}
	for _, r := range st.allow {
		if r.Start <= port && port <= r.End {
			return true
		}
	}
	return false
}

// publishServerPorts 在 frps 实例运行时登记它的固定端口，停用时撤销登记。
func (m *Module) publishServerPorts(key string, s ServerSettings, running bool) {
	if !running {
		m.clearServerPorts(key)
		return
	}
	m.armLearn(key, s)
	openports.Set(serverOwner(key), openports.Entry{Ranges: serverOpenRanges(s)})
}

// armLearn 允许实例 key 学习代理端口，在启动 frps 之前调用，避免 frpc 抢在登记之前申请的端口被漏掉。
// 实例已在学习时只更新 allowPorts，并忘记不再允许的端口。
func (m *Module) armLearn(key string, s ServerSettings) {
	allow, _ := ParseAllowPorts(s.AllowPorts)
	m.learnMu.Lock()
	defer m.learnMu.Unlock()
	if m.learned == nil {
		m.learned = map[string]*learnState{}
	}
	st := m.learned[key]
	if st == nil {
		m.learned[key] = &learnState{allow: allow, ports: map[int]bool{}, bindPort: s.BindPort, token: s.Token}
		return
	}
	st.allow = allow
	if st.bindPort != s.BindPort || st.token != s.Token {
		// 换了连接端口或令牌后原来的登录随 frps 重启失效，可信来源等 frpc 用新设置重新登录后再登记
		st.bindPort, st.token = s.BindPort, s.Token
		st.trusted = nil
		trustsrc.Clear(trustOwner(key))
	}
	changed := false
	for p := range st.ports {
		if !st.allows(p) {
			delete(st.ports, p)
			changed = true
		}
	}
	if changed {
		openports.SetPorts(proxyOwner(key), sortedPorts(st.ports))
	}
}

// clearServerPorts 撤销 frps 实例登记的全部端口，并忘记它的代理端口。
// 在 learnMu 内删除状态并撤销登记：停止时仍在处理的 NewProxy 请求拿到锁后找不到状态，不会把端口重新登记回去。
func (m *Module) clearServerPorts(key string) {
	openports.Clear(serverOwner(key))
	m.learnMu.Lock()
	delete(m.learned, key)
	openports.Clear(proxyOwner(key))
	trustsrc.Clear(trustOwner(key))
	m.learnMu.Unlock()
}

// learnProxyPort 记住 frpc 在实例 key 上申请的 TCP 代理远程端口，并立即登记为开放端口。
// 端口随 frpc 的登录与断线反复开关：登记后即使 frpc 掉线、frps 暂时关闭监听，
// 家里客户端重连这些端口时不会被扫描检测当成未开放端口。只有运行中的实例会学习，实例停止或删除时忘记。
func (m *Module) learnProxyPort(key string, port int) {
	if !validPort(port) {
		return
	}
	m.learnMu.Lock()
	defer m.learnMu.Unlock()
	st := m.learned[key]
	if st == nil || st.ports[port] || len(st.ports) >= maxLearnedPorts || !st.allows(port) {
		return
	}
	st.ports[port] = true
	openports.SetPorts(proxyOwner(key), sortedPorts(st.ports))
}

// maxTrusted 为单个 frps 实例登记的可信来源上限，超出时忘记最早登录的地址（家宽地址变化后旧地址会留在这里）
const maxTrusted = 32

// trustGrace 为 frpc 登录后无条件视为可信的时间；之后只在它与 frps 之间还有已建立的连接时可信。
// 这段时间覆盖 frpc 刚登录、断线重连的空档，也让读不到连接状态的系统在登录后的一段时间内生效
const trustGrace = 10 * time.Minute

// trustOwner 是 frps 实例在 trustsrc 中的登记名。
func trustOwner(key string) string { return "frps-login:" + key }

// trustClient 把用正确令牌登录实例 key 的 frpc 来源地址登记为可信来源：入侵防护的诱饵端口与扫描检测不再统计它。
// 只有运行中的实例会登记，实例停止、删除或更换连接端口时撤销；回环与未指定地址（frps 在反向代理之后）不登记。
func (m *Module) trustClient(key string, a netip.Addr) {
	a = a.Unmap()
	if !a.IsValid() || a.IsLoopback() || a.IsUnspecified() {
		return
	}
	m.learnMu.Lock()
	defer m.learnMu.Unlock()
	st := m.learned[key]
	if st == nil || !validPort(st.bindPort) {
		return
	}
	if st.trusted == nil {
		st.trusted = map[netip.Addr]time.Time{}
	}
	st.trusted[a] = time.Now()
	for len(st.trusted) > maxTrusted {
		var oldest netip.Addr
		var at time.Time
		for addr, t := range st.trusted {
			if !oldest.IsValid() || t.Before(at) {
				oldest, at = addr, t
			}
		}
		delete(st.trusted, oldest)
	}
	list := make([]trustsrc.Source, 0, len(st.trusted))
	for addr, t := range st.trusted {
		list = append(list, trustsrc.Source{Addr: addr, Port: st.bindPort, Until: t.Add(trustGrace)})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Addr.Less(list[j].Addr) })
	trustsrc.Set(trustOwner(key), list)
}

func sortedPorts(set map[int]bool) []int {
	list := make([]int, 0, len(set))
	for p := range set {
		list = append(list, p)
	}
	sort.Ints(list)
	return list
}
