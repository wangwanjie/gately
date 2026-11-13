package frp

import (
	"net"
	"sort"

	"github.com/wangwanjie/gately/internal/openports"
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

// publishServerPorts 在 frps 实例运行时登记它的固定端口，停用时撤销登记。
func (m *Module) publishServerPorts(key string, s ServerSettings, running bool) {
	if !running {
		m.clearServerPorts(key)
		return
	}
	openports.Set(serverOwner(key), openports.Entry{Ranges: serverOpenRanges(s)})
}

// clearServerPorts 撤销 frps 实例登记的全部端口，并忘记它的代理端口。
func (m *Module) clearServerPorts(key string) {
	openports.Clear(serverOwner(key))
	m.learnMu.Lock()
	delete(m.learned, key)
	m.learnMu.Unlock()
	openports.Clear(proxyOwner(key))
}

// learnProxyPort 记住 frpc 在实例 key 上申请的 TCP 代理远程端口，并立即登记为开放端口。
// 端口随 frpc 的登录与断线反复开关：登记后即使 frpc 掉线、frps 暂时关闭监听，
// 家里客户端重连这些端口时不会被扫描检测当成未开放端口。实例停止或删除时忘记。
func (m *Module) learnProxyPort(key string, port int) {
	if !validPort(port) {
		return
	}
	m.learnMu.Lock()
	set := m.learned[key]
	if set == nil {
		if m.learned == nil {
			m.learned = map[string]map[int]bool{}
		}
		set = map[int]bool{}
		m.learned[key] = set
	}
	if set[port] || len(set) >= maxLearnedPorts {
		m.learnMu.Unlock()
		return
	}
	set[port] = true
	list := make([]int, 0, len(set))
	for p := range set {
		list = append(list, p)
	}
	m.learnMu.Unlock()
	sort.Ints(list)
	openports.SetPorts(proxyOwner(key), list)
}

// hasServer 判断 key 是否为当前存在的 frps 实例（读访问控制快照，不获取模块锁）。
func (m *Module) hasServer(key string) bool {
	rules := m.access.Load()
	if rules == nil {
		return false
	}
	_, ok := (*rules)[key]
	return ok
}
