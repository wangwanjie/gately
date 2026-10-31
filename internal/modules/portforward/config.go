package portforward

import (
	"fmt"
	"net"
	"strings"

	"github.com/wangwanjie/gately/internal/crud"
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
	Group           string   `json:"group"`
	Protocols       []string `json:"protocols"`
	ListenAddress   string   `json:"listenAddress"`
	ListenPorts     string   `json:"listenPorts"`
	TargetAddresses []string `json:"targetAddresses"`
	TargetPorts     string   `json:"targetPorts"`
	// MaxConnections 是该规则每个 TCP 端口的最大并发连接数，0 表示使用全局默认值
	MaxConnections int `json:"maxConnections"`
	// UDPSessionTimeout 是 UDP 会话空闲超时（秒），0 表示使用全局默认值
	UDPSessionTimeout int `json:"udpSessionTimeout"`
	// ProxyProtocol 非空时，TCP 连接目标后先发送 PROXY 协议头（v1 或 v2），把真实客户端地址告诉下游
	// （haproxy accept-proxy、nginx proxy_protocol、Gately Web 服务"接收 PROXY 协议"）。目标必须支持，否则连接会被拒绝。
	ProxyProtocol string `json:"proxyProtocol"`
	// Mode 只用于拒绝旧版接口提交的分流规则：SNI 与协议分流已移到独立的「协议分流」模块，这里必须为空
	Mode   string `json:"mode,omitempty"`
	Remark string `json:"remark"`
}

// Settings 是模块级配置。
type Settings struct {
	Enable                   bool `json:"enable"`
	DefaultMaxConnections    int  `json:"defaultMaxConnections"`
	DefaultUDPSessionTimeout int  `json:"defaultUDPSessionTimeout"`
	MaxUDPSessions           int  `json:"maxUDPSessions"`
	// DialTimeout 是连接目标的超时（秒）
	DialTimeout int `json:"dialTimeout"`
}

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
	if _, err := l4proxy.MapPorts(r.ListenPorts, r.TargetPorts); err != nil {
		return err
	}
	r.TargetAddresses = l4proxy.CleanTargets(r.TargetAddresses)
	if err := l4proxy.ValidateProxyProtocol(r.ProxyProtocol); err != nil {
		return err
	}
	if r.Mode != "" {
		return fmt.Errorf("SNI 与协议分流规则已移到「协议分流」模块，请在那里添加")
	}
	if len(r.TargetAddresses) == 0 {
		return fmt.Errorf("至少填写一个目标地址")
	}
	if r.MaxConnections < 0 || r.UDPSessionTimeout < 0 {
		return fmt.Errorf("连接数与超时不能为负数")
	}
	return nil
}
