package stun

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/webhook"
)

// 协议取值
const (
	ProtoTCP4 = "tcp4"
	ProtoUDP4 = "udp4"
)

// 通道绑定方式
const (
	BindIP        = "ip"
	BindInterface = "interface"
)

// Bind 指定穿透通道使用的本地地址，多拨或多线路时用来选择出口。
type Bind struct {
	Mode string `json:"mode"`
	// IP 为空表示所有地址
	IP        string `json:"ip"`
	Interface string `json:"interface"`
	// Regex 选择网卡上的地址，兼容 Lucky 写法：留空取第 1 个、数字 n 取第 n 个、24* 前缀、*24 后缀，其余按正则
	Regex string `json:"regex"`
}

// Rule 是一条 STUN 穿透规则。
type Rule struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Group 为列表中的分组名称，只影响管理界面的展示，留空为未分组
	Group    string `json:"group"`
	Enable   bool   `json:"enable"`
	Protocol string `json:"protocol"`
	Bind     Bind   `json:"bind"`
	// LocalPort 为穿透通道本地端口，0 表示随机
	LocalPort        int      `json:"localPort"`
	UseGlobalServers bool     `json:"useGlobalServers"`
	StunServers      []string `json:"stunServers"`
	// KeepAliveServers 仅 TCP 使用：保持 NAT 映射的长连接目标，格式 host:port，端口缺省为 80
	KeepAliveServers []string `json:"keepAliveServers"`

	// DisableForward 为 true 时不使用内置转发，需在路由器上把通道端口映射到目标
	DisableForward  bool     `json:"disableForward"`
	TargetAddresses []string `json:"targetAddresses"`
	TargetPort      int      `json:"targetPort"`

	UPnP bool `json:"upnp"`
	// UPnPGateway 为网关 IP，留空自动发现
	UPnPGateway string `json:"upnpGateway"`
	// UPnPLocalHost 覆盖映射的内部地址（Docker 桥接网络时填宿主机局域网 IP）
	UPnPLocalHost string `json:"upnpLocalHost"`
	// UPnPControlURL 直接指定 IGD 控制地址，跳过发现
	UPnPControlURL string `json:"upnpControlUrl"`
	// UPnPInternalPort 非 0 时把路由器端口直接映射到本机该端口（绕过内置转发）
	UPnPInternalPort int    `json:"upnpInternalPort"`
	NATPMP           bool   `json:"natpmp"`
	NATPMPGateway    string `json:"natpmpGateway"`

	// 以下时间单位为毫秒
	HeartbeatInterval int  `json:"heartbeatInterval"`
	StunTimeout       int  `json:"stunTimeout"`
	RetryInterval     int  `json:"retryInterval"`
	AutoRetry         bool `json:"autoRetry"`
	// DisableCheck 关闭通道有效性检测（连接自身公网地址验证可达）
	DisableCheck bool `json:"disableCheck"`

	UseGlobalWebhook bool           `json:"useGlobalWebhook"`
	Webhook          webhook.Config `json:"webhook"`
	// WebhookOnlyOnChange 为 true 时只在公网地址变化时触发通知
	WebhookOnlyOnChange bool `json:"webhookOnlyOnChange"`
	ScriptEnable        bool `json:"scriptEnable"`
	// Script 以 sh -c 执行，变量通过环境变量传入：ruleName time ip port ipAddr
	Script string `json:"script"`

	MaxConnections    int    `json:"maxConnections"`
	UDPSessionTimeout int    `json:"udpSessionTimeout"`
	Remark            string `json:"remark"`
}

// Settings 是模块级配置。
type Settings struct {
	Enable           bool           `json:"enable"`
	StunServers      []string       `json:"stunServers"`
	KeepAliveServers []string       `json:"keepAliveServers"`
	Webhook          webhook.Config `json:"webhook"`
	// AddrCheckInterval 为重新查询公网地址的周期（秒）
	AddrCheckInterval int `json:"addrCheckInterval"`
}

type fileConfig struct {
	Settings Settings `json:"settings"`
	Rules    []Rule   `json:"rules"`
}

// 默认值均为第三方公共服务，用户可在设置中修改
var (
	// 2026-10 实测均支持 TCP（freeswitch 的 UDP 无响应，UDP 规则会自动切换到下一个）
	defaultStunServers = []string{
		"stun.nextcloud.com:3478",
		"stun.hot-chilli.net:3478",
		"stun.voip.blackberry.com:3478",
		"stun.radiojar.com:3478",
		"stun.freeswitch.org:3478",
	}
	defaultKeepAliveServers = []string{"www.baidu.com:80", "www.qq.com:80"}
)

func defaultSettings() Settings {
	return Settings{
		Enable:            true,
		StunServers:       append([]string{}, defaultStunServers...),
		KeepAliveServers:  append([]string{}, defaultKeepAliveServers...),
		AddrCheckInterval: 60,
	}
}

func (s *Settings) normalize() {
	s.StunServers = cleanList(s.StunServers)
	s.KeepAliveServers = cleanList(s.KeepAliveServers)
	if len(s.StunServers) == 0 {
		s.StunServers = append([]string{}, defaultStunServers...)
	}
	if len(s.KeepAliveServers) == 0 {
		s.KeepAliveServers = append([]string{}, defaultKeepAliveServers...)
	}
	if s.AddrCheckInterval < 10 {
		s.AddrCheckInterval = 60
	}
}

func cleanList(in []string) []string {
	var out []string
	for _, v := range in {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// withDefaultPort 为缺少端口的地址补上默认端口。
func withDefaultPort(addr, port string) string {
	if _, _, err := net.SplitHostPort(addr); err == nil {
		return addr
	}
	return net.JoinHostPort(strings.Trim(addr, "[]"), port)
}

// Validate 校验并补全默认值。
func (r *Rule) Validate() error {
	if err := crud.NormalizeGroup(&r.Group); err != nil {
		return err
	}
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return errors.New("规则名称不能为空")
	}
	switch r.Protocol {
	case ProtoTCP4, ProtoUDP4:
	case "":
		r.Protocol = ProtoTCP4
	default:
		return fmt.Errorf("不支持的协议 %q", r.Protocol)
	}
	switch r.Bind.Mode {
	case "", BindIP:
		r.Bind.Mode = BindIP
		r.Bind.IP = strings.TrimSpace(r.Bind.IP)
		if r.Bind.IP != "" {
			ip := net.ParseIP(r.Bind.IP)
			if ip == nil || ip.To4() == nil {
				return fmt.Errorf("通道 IP %q 不是有效的 IPv4 地址", r.Bind.IP)
			}
		}
	case BindInterface:
		if strings.TrimSpace(r.Bind.Interface) == "" {
			return errors.New("请选择通道网卡")
		}
	default:
		return fmt.Errorf("未知的绑定方式 %q", r.Bind.Mode)
	}
	if r.LocalPort < 0 || r.LocalPort > 65535 || r.TargetPort < 0 || r.TargetPort > 65535 || r.UPnPInternalPort < 0 || r.UPnPInternalPort > 65535 {
		return errors.New("端口超出范围")
	}
	r.StunServers = cleanList(r.StunServers)
	r.KeepAliveServers = cleanList(r.KeepAliveServers)
	if !r.UseGlobalServers && len(r.StunServers) == 0 {
		return errors.New("请填写 STUN 服务器或使用全局列表")
	}
	for i, s := range r.StunServers {
		r.StunServers[i] = withDefaultPort(s, "3478")
	}
	for i, s := range r.KeepAliveServers {
		r.KeepAliveServers[i] = withDefaultPort(s, "80")
	}
	if r.DisableForward {
		if r.LocalPort == 0 {
			return errors.New("不使用内置转发时通道本地端口不能为 0")
		}
	} else {
		r.TargetAddresses = cleanList(r.TargetAddresses)
		if len(r.TargetAddresses) == 0 || r.TargetPort == 0 {
			return errors.New("请填写转发目标地址和端口")
		}
	}
	if r.HeartbeatInterval == 0 {
		r.HeartbeatInterval = 3000
	}
	if r.StunTimeout == 0 {
		r.StunTimeout = 3000
	}
	if r.RetryInterval == 0 {
		r.RetryInterval = 3000
	}
	if r.HeartbeatInterval < 500 || r.StunTimeout < 500 || r.RetryInterval < 500 {
		return errors.New("心跳、超时与重试间隔不能小于 500 毫秒")
	}
	if r.MaxConnections < 0 || r.UDPSessionTimeout < 0 {
		return errors.New("连接数与超时不能为负数")
	}
	if r.ScriptEnable && strings.TrimSpace(r.Script) == "" {
		return errors.New("已启用脚本但脚本内容为空")
	}
	return r.Webhook.Validate()
}

// selectIP 按配置的选择规则从地址列表中选择一个。
func selectIP(addrs []string, rule string) (string, error) {
	if len(addrs) == 0 {
		return "", errors.New("网卡上没有可用的 IPv4 地址")
	}
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return addrs[0], nil
	}
	if n, err := strconv.Atoi(rule); err == nil {
		if n < 1 || n > len(addrs) {
			return "", fmt.Errorf("网卡只有 %d 个地址，无法选择第 %d 个", len(addrs), n)
		}
		return addrs[n-1], nil
	}
	if strings.HasSuffix(rule, "*") && !strings.ContainsAny(rule[:len(rule)-1], `*\[](){}^$|+?`) {
		for _, a := range addrs {
			if strings.HasPrefix(a, strings.TrimSuffix(rule, "*")) {
				return a, nil
			}
		}
		return "", fmt.Errorf("没有以 %s 开头的地址", strings.TrimSuffix(rule, "*"))
	}
	if strings.HasPrefix(rule, "*") && !strings.ContainsAny(rule[1:], `*\[](){}^$|+?`) {
		for _, a := range addrs {
			if strings.HasSuffix(a, strings.TrimPrefix(rule, "*")) {
				return a, nil
			}
		}
		return "", fmt.Errorf("没有以 %s 结尾的地址", strings.TrimPrefix(rule, "*"))
	}
	re, err := regexp.Compile(rule)
	if err != nil {
		return "", err
	}
	for _, a := range addrs {
		if re.MatchString(a) {
			return a, nil
		}
	}
	return "", fmt.Errorf("没有匹配 %q 的地址", rule)
}

// interfaceIPv4s 允许测试替换网卡地址来源。
var interfaceIPv4s = func(name string) ([]string, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, err
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
			out = append(out, ipn.IP.String())
		}
	}
	return out, nil
}

// resolveBindIP 返回通道绑定的本地 IP，空字符串表示所有地址。
func (b Bind) resolveBindIP() (string, error) {
	if b.Mode != BindInterface {
		return b.IP, nil
	}
	addrs, err := interfaceIPv4s(b.Interface)
	if err != nil {
		return "", fmt.Errorf("读取网卡 %s 失败: %w", b.Interface, err)
	}
	return selectIP(addrs, b.Regex)
}
