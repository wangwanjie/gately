package portforward

import (
	"fmt"
	"net"
	"slices"
	"strings"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/iplist"
	"github.com/wangwanjie/gately/internal/l4proxy"
)

// 支持的协议，语义与 Go 网络类型一致：tcp/udp 为双栈，tcp4/udp4 仅 IPv4，tcp6/udp6 仅 IPv6。
var validProtocols = map[string]bool{
	"tcp": true, "tcp4": true, "tcp6": true,
	"udp": true, "udp4": true, "udp6": true,
}

// Rule 是一条端口转发规则。
type Rule struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Enable bool   `json:"enable"`
	// Group 为列表中的分组名称，只影响管理界面的展示，留空为未分组
	Group         string   `json:"group"`
	Protocols     []string `json:"protocols"`
	ListenAddress string   `json:"listenAddress"`
	ListenPorts   string   `json:"listenPorts"`
	// AutoFirewall 为 true 时在本机防火墙（nftables / iptables / ip6tables 的 input 链）放行监听成功的端口，
	// 连续端口合并为一条规则，绑定具体地址时只放行发往该地址的连接，只监听回环地址时不放行；规则停止时撤销
	AutoFirewall    bool     `json:"autoFirewall"`
	TargetAddresses []string `json:"targetAddresses"`
	TargetPorts     string   `json:"targetPorts"`
	// MaxConnections 是该规则每个 TCP 端口的最大并发连接数，0 表示使用全局默认值
	MaxConnections int `json:"maxConnections"`
	// MaxConnsPerSource 是每个 TCP 端口上单个来源（IPv4 地址、IPv6 /64）的最大并发连接数，0 表示使用全局默认值；
	// 回环与内网来源不受此限制
	MaxConnsPerSource int `json:"maxConnsPerSource"`
	// UDPSessionTimeout 是 UDP 会话空闲超时（秒），0 表示使用全局默认值
	UDPSessionTimeout int `json:"udpSessionTimeout"`
	// ProxyProtocol 非空时，TCP 连接目标后先发送 PROXY 协议头（v1 或 v2），把真实客户端地址告诉下游
	// （haproxy accept-proxy、nginx proxy_protocol、Gately Web 服务"接收 PROXY 协议"）。目标必须支持，否则连接会被拒绝。
	ProxyProtocol string `json:"proxyProtocol"`
	// Mode 只用于拒绝旧版接口提交的分流规则：SNI 与协议分流已移到独立的「协议分流」模块，这里必须为空
	Mode string `json:"mode,omitempty"`
	// Access 为来源 IP 访问控制（白名单 / 黑名单），在连接目标之前判断；封禁表中的地址无论何种模式都会被拒绝
	iplist.Access
	Remark string `json:"remark"`
}

// Settings 是模块级配置。
type Settings struct {
	Enable                bool `json:"enable"`
	DefaultMaxConnections int  `json:"defaultMaxConnections"`
	// DefaultMaxConnsPerSource 是单个来源的默认最大并发连接数，0 表示取所在端口最大连接数的一半：
	// 单个来源最多占用一半名额，其余来源总能连接；家庭使用中一个来源的并发连接远少于这个数
	DefaultMaxConnsPerSource int `json:"defaultMaxConnsPerSource"`
	// PerSourceExempt 为不受单来源上限约束的来源（IP 或 CIDR），例如把所有访客汇聚成同一个来源的公网中转或前置代理；
	// 回环与内网来源本来就不受限制，不必填写
	PerSourceExempt          []string `json:"perSourceExempt"`
	DefaultUDPSessionTimeout int      `json:"defaultUDPSessionTimeout"`
	MaxUDPSessions           int      `json:"maxUDPSessions"`
	// DialTimeout 是连接目标的超时（秒）
	DialTimeout int `json:"dialTimeout"`
	// FirstDataTimeout 是 TCP 连接建立后等待任一方发送数据的最长时间（秒），超时仍没有任何数据的空连接被断开，
	// 0 表示默认 60 秒。正常协议总有一方在连接后立即发言，只建立连接不发数据的连接只会占用名额
	FirstDataTimeout int `json:"firstDataTimeout"`
}

// 连接参数的默认值与上限
const (
	defaultFirstDataTimeout = 60
	maxFirstDataTimeout     = 3600
)

// fileConfig 是 portforward.json 的结构。
type fileConfig struct {
	Settings Settings `json:"settings"`
	Rules    []Rule   `json:"rules"`
}

func defaultSettings() Settings {
	return Settings{
		Enable:                   true,
		DefaultMaxConnections:    256,
		DefaultUDPSessionTimeout: 60,
		MaxUDPSessions:           1024,
		DialTimeout:              5,
		FirstDataTimeout:         defaultFirstDataTimeout,
	}
}

// normalize 填充缺省值，保证运行时不出现 0 值导致的异常行为。
func (s *Settings) normalize() {
	d := defaultSettings()
	if s.DefaultMaxConnections <= 0 {
		s.DefaultMaxConnections = d.DefaultMaxConnections
	}
	if s.DefaultUDPSessionTimeout <= 0 {
		s.DefaultUDPSessionTimeout = d.DefaultUDPSessionTimeout
	}
	if s.MaxUDPSessions <= 0 {
		s.MaxUDPSessions = d.MaxUDPSessions
	}
	if s.DialTimeout <= 0 {
		s.DialTimeout = d.DialTimeout
	}
	if s.DefaultMaxConnsPerSource < 0 {
		s.DefaultMaxConnsPerSource = 0
	}
	s.PerSourceExempt = cleanExempt(s.PerSourceExempt)
	if s.FirstDataTimeout <= 0 {
		s.FirstDataTimeout = d.FirstDataTimeout
	}
	s.FirstDataTimeout = min(s.FirstDataTimeout, maxFirstDataTimeout)
}

// cleanExempt 去掉单来源豁免列表中的空白与重复项，保持原有顺序。
func cleanExempt(list []string) []string {
	var out []string
	for _, v := range list {
		v = strings.TrimSpace(v)
		if v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// validate 校验全局设置中用户填写的内容。
func (s *Settings) validate() error {
	// 逐项校验，报错时指出是哪一项
	for _, v := range s.PerSourceExempt {
		if _, err := l4proxy.ParseExempt([]string{v}); err != nil {
			return fmt.Errorf("不受单来源限制的来源 %q 不是 IP 或 CIDR", v)
		}
	}
	return nil
}

// Validate 校验规则并清理输入中的空白。
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
			return fmt.Errorf("不支持的协议 %q", p)
		}
		if seen[p] {
			return fmt.Errorf("协议 %q 重复", p)
		}
		seen[p] = true
	}
	if r.ListenAddress != "" && net.ParseIP(r.ListenAddress) == nil {
		return fmt.Errorf("监听地址 %q 不是有效 IP", r.ListenAddress)
	}
	// 目标地址写成「主机:端口」时把端口拆到目标端口；与已填的目标端口冲突时拒绝
	targets, port, err := l4proxy.SplitTargets(r.TargetAddresses)
	if err != nil {
		return err
	}
	if r.TargetPorts, err = l4proxy.MergeTargetPorts(r.TargetPorts, port); err != nil {
		return err
	}
	r.TargetAddresses = targets
	if _, err := l4proxy.MapPorts(r.ListenPorts, r.TargetPorts); err != nil {
		return err
	}
	if err := l4proxy.ValidateProxyProtocol(r.ProxyProtocol); err != nil {
		return err
	}
	if r.Mode != "" {
		return fmt.Errorf("SNI 与协议分流规则已移到「协议分流」模块，请在那里添加")
	}
	if len(r.TargetAddresses) == 0 {
		return fmt.Errorf("至少填写一个目标地址")
	}
	if r.MaxConnections < 0 || r.MaxConnsPerSource < 0 || r.UDPSessionTimeout < 0 {
		return fmt.Errorf("连接数与超时不能为负数")
	}
	return r.Access.Normalize()
}
