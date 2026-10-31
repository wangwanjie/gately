package protoroute

import (
	"fmt"
	"net"
	"strings"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/l4proxy"
)

// 支持的监听协议：只处理 TCP，tcp 为双栈，tcp4 仅 IPv4，tcp6 仅 IPv6。
var validProtocols = map[string]bool{"tcp": true, "tcp4": true, "tcp6": true}

// 识别超时的取值范围（秒），0 表示使用默认值。
const (
	minSniffTimeout = 0.1
	maxSniffTimeout = 60
)

// Rule 是一条协议分流规则：在监听端口上预读客户端最先发送的数据，识别协议（TLS 再看 SNI 与 ALPN，HTTP 再看 Host）后，
// 按路由把整条连接透传到对应后端，不解密、不终止任何协议。作用相当于 haproxy 的 req_ssl_sni 规则加上 sslh。
type Rule struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Enable bool   `json:"enable"`
	// Group 为列表中的分组名称，只影响管理界面的展示，留空为未分组
	Group         string   `json:"group"`
	Protocols     []string `json:"protocols"`
	ListenAddress string   `json:"listenAddress"`
	ListenPorts   string   `json:"listenPorts"`
	// Routes 是按顺序展示的路由表，匹配优先级见 router.go
	Routes []Route `json:"routes"`
	// TargetAddresses、TargetPorts、ProxyProtocol 是默认目标：未命中路由或无法识别协议时使用，地址留空表示直接断开
	TargetAddresses []string `json:"targetAddresses"`
	TargetPorts     string   `json:"targetPorts"`
	ProxyProtocol   string   `json:"proxyProtocol"`
	// SniffTimeout 是等待客户端发来可识别数据的秒数，可带小数（最短 0.1 秒），0 表示默认 5 秒。
	// 超时仍未收到任何数据的连接交给「等待超时」路由，因此它也是 VNC 等服务端先发送数据的协议的连接延迟；
	// 客户端先发送数据的协议通常在握手后几毫秒内就发出首包，有等待超时路由时可调到 0.3～1 秒
	SniffTimeout float64 `json:"sniffTimeout"`
	// MaxConnections 是该规则每个端口的最大并发连接数，0 表示使用全局默认值
	MaxConnections int    `json:"maxConnections"`
	Remark         string `json:"remark"`
}

// Route 是分流规则中的一条路由，作用相当于 haproxy 的一个 backend。
type Route struct {
	Name string `json:"name"`
	// Protocol 为路由匹配的协议（tls、http、ssh、rdp、socks、openvpn、xmpp、mqtt、postgresql、timeout），空按 tls 处理
	Protocol string `json:"protocol"`
	// Domains 仅 TLS（匹配 SNI）与 HTTP（匹配 Host）路由使用：精确域名、*.example.com 通配（匹配任意层级子域名）
	// 或 *（匹配所有携带域名的连接），不区分大小写；留空匹配该协议的全部连接
	Domains []string `json:"domains"`
	// ALPN 仅 TLS 路由使用：客户端声明的 ALPN 含其中任一项时才匹配，例如 h2、http/1.1、acme-tls/1、postgresql；留空不限
	ALPN            []string `json:"alpn"`
	TargetAddresses []string `json:"targetAddresses"`
	// TargetPort 为目标端口，0 表示与监听端口相同
	TargetPort int `json:"targetPort"`
	// ProxyProtocol 为发给该目标的 PROXY 协议版本（v1/v2），空为不发送
	ProxyProtocol string `json:"proxyProtocol"`
}

// Settings 是模块级配置。
type Settings struct {
	Enable                bool `json:"enable"`
	DefaultMaxConnections int  `json:"defaultMaxConnections"`
	// DialTimeout 是连接目标的超时（秒）
	DialTimeout int `json:"dialTimeout"`
}

// fileConfig 是 protoroute.json 的结构。
type fileConfig struct {
	Settings Settings `json:"settings"`
	Rules    []Rule   `json:"rules"`
}

func defaultSettings() Settings {
	return Settings{Enable: true, DefaultMaxConnections: 256, DialTimeout: 5}
}

// normalize 填充缺省值，保证运行时不出现 0 值导致的异常行为。
func (s *Settings) normalize() {
	d := defaultSettings()
	if s.DefaultMaxConnections <= 0 {
		s.DefaultMaxConnections = d.DefaultMaxConnections
	}
	if s.DialTimeout <= 0 {
		s.DialTimeout = d.DialTimeout
	}
}

// Validate 校验规则，并规范化协议、域名、ALPN 与地址。
func (r *Rule) Validate() error {
	r.Name = strings.TrimSpace(r.Name)
	if err := crud.NormalizeGroup(&r.Group); err != nil {
		return err
	}
	r.ListenAddress = strings.Trim(strings.TrimSpace(r.ListenAddress), "[]")
	if len(r.Protocols) == 0 {
		return fmt.Errorf("至少选择一种协议")
	}
	seen := map[string]bool{}
	for _, p := range r.Protocols {
		if !validProtocols[p] {
			return fmt.Errorf("协议分流仅支持 TCP，不支持 %q", p)
		}
		if seen[p] {
			return fmt.Errorf("协议 %q 重复", p)
		}
		seen[p] = true
	}
	if r.ListenAddress != "" && net.ParseIP(r.ListenAddress) == nil {
		return fmt.Errorf("监听地址 %q 不是有效 IP", r.ListenAddress)
	}
	if _, err := l4proxy.MapPorts(r.ListenPorts, r.TargetPorts); err != nil {
		return err
	}
	r.TargetAddresses = l4proxy.CleanTargets(r.TargetAddresses)
	if err := l4proxy.ValidateProxyProtocol(r.ProxyProtocol); err != nil {
		return err
	}
	if r.SniffTimeout != 0 && (r.SniffTimeout < minSniffTimeout || r.SniffTimeout > maxSniffTimeout) {
		return fmt.Errorf("识别超时需为 0（使用默认 5 秒）或 %g～%d 秒", minSniffTimeout, maxSniffTimeout)
	}
	if r.MaxConnections < 0 {
		return fmt.Errorf("连接数不能为负数")
	}
	if len(r.Routes) == 0 && len(r.TargetAddresses) == 0 {
		return fmt.Errorf("至少添加一条路由或填写默认目标")
	}
	return r.validateRoutes()
}

// validateRoutes 校验路由表，并规范化协议、域名与 ALPN。
func (r *Rule) validateRoutes() error {
	// 同一协议下同一域名（以及同一 ALPN）只能属于一条路由，否则命中结果取决于书写顺序，容易配错
	type owners struct {
		generic string
		alpn    map[string]string
	}
	domainOwner := map[string]*owners{}
	protoOwner := map[string]string{}
	for i := range r.Routes {
		rt := &r.Routes[i]
		rt.Name = strings.TrimSpace(rt.Name)
		label := rt.Name
		if label == "" {
			label = fmt.Sprintf("第 %d 条路由", i+1)
		}
		rt.Protocol = strings.ToLower(strings.TrimSpace(rt.Protocol))
		if rt.Protocol == "" {
			rt.Protocol = ProtoTLS
		}
		if !routeProtocols[rt.Protocol] {
			return fmt.Errorf("%s: 不支持的协议 %q", label, rt.Protocol)
		}
		domains, err := cleanRouteDomains(rt.Domains)
		if err != nil {
			return fmt.Errorf("%s: %v", label, err)
		}
		alpn, err := cleanALPN(rt.ALPN)
		if err != nil {
			return fmt.Errorf("%s: %v", label, err)
		}
		if !protocolUsesDomains(rt.Protocol) && len(domains) > 0 {
			return fmt.Errorf("%s: 只有 TLS、HTTP 与 Minecraft 路由可以填写域名", label)
		}
		if rt.Protocol != ProtoTLS && len(alpn) > 0 {
			return fmt.Errorf("%s: 只有 TLS 路由可以填写 ALPN", label)
		}
		rt.Domains = domains
		rt.ALPN = alpn

		if protocolUsesDomains(rt.Protocol) {
			keys := domains
			if len(keys) == 0 {
				keys = []string{""}
			}
			for _, d := range keys {
				o := domainOwner[rt.Protocol+" "+d]
				if o == nil {
					o = &owners{alpn: map[string]string{}}
					domainOwner[rt.Protocol+" "+d] = o
				}
				if len(alpn) == 0 {
					if o.generic != "" {
						return conflictError(d, "", o.generic, label)
					}
					o.generic = label
					continue
				}
				for _, p := range alpn {
					if prev, ok := o.alpn[p]; ok {
						return conflictError(d, p, prev, label)
					}
					o.alpn[p] = label
				}
			}
		} else {
			if prev, ok := protoOwner[rt.Protocol]; ok {
				return fmt.Errorf("%s 与 %s 匹配同一协议，每种协议只能有一条路由", prev, label)
			}
			protoOwner[rt.Protocol] = label
		}

		rt.TargetAddresses = l4proxy.CleanTargets(rt.TargetAddresses)
		if len(rt.TargetAddresses) == 0 {
			return fmt.Errorf("%s: 至少填写一个目标地址", label)
		}
		if rt.TargetPort < 0 || rt.TargetPort > 65535 {
			return fmt.Errorf("%s: 无效端口 %d", label, rt.TargetPort)
		}
		if err := l4proxy.ValidateProxyProtocol(rt.ProxyProtocol); err != nil {
			return fmt.Errorf("%s: %v", label, err)
		}
	}
	return nil
}

// conflictError 描述两条路由匹配相同域名（与 ALPN）的冲突；domain 为空表示两条路由都未填写域名。
func conflictError(domain, alpn, prev, label string) error {
	switch {
	case domain != "" && alpn == "":
		return fmt.Errorf("域名 %s 同时出现在 %s 与 %s", domain, prev, label)
	case domain != "":
		return fmt.Errorf("域名 %s 的 ALPN %s 同时出现在 %s 与 %s", domain, alpn, prev, label)
	case alpn == "":
		return fmt.Errorf("%s 与 %s 都未填写域名，同一协议只能有一条", prev, label)
	}
	return fmt.Errorf("ALPN %s 同时出现在 %s 与 %s", alpn, prev, label)
}

// cleanRouteDomains 规范化并校验路由的域名，丢弃空项与重复项。
func cleanRouteDomains(in []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, d := range in {
		d = normalizeSNI(d)
		if d == "" || seen[d] {
			continue
		}
		if err := validateDomain(d); err != nil {
			return nil, err
		}
		seen[d] = true
		out = append(out, d)
	}
	return out, nil
}

// validateDomain 检查域名写法：精确域名、*.后缀 或单独的 *。
func validateDomain(d string) error {
	if d == "*" {
		return nil
	}
	name := strings.TrimPrefix(d, "*.")
	if name == "" || strings.Contains(name, "*") || strings.ContainsAny(name, " /:") || strings.HasPrefix(name, ".") || strings.Contains(name, "..") {
		return fmt.Errorf("域名 %q 格式无效，支持 example.com、*.example.com 或 *", d)
	}
	return nil
}

// cleanALPN 去掉 ALPN 两侧的空白并丢弃空项与重复项。ALPN 区分大小写，按原样比较。
func cleanALPN(in []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, p := range in {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		if len(p) > 255 || strings.ContainsAny(p, " ,") {
			return nil, fmt.Errorf("ALPN %q 格式无效", p)
		}
		seen[p] = true
		out = append(out, p)
	}
	return out, nil
}
