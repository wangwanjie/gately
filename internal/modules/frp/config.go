package frp

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/wangwanjie/gately/internal/binstall"
	"github.com/wangwanjie/gately/internal/crud"
)

// 代理类型。
const (
	TypeTCP   = "tcp"
	TypeUDP   = "udp"
	TypeHTTP  = "http"
	TypeHTTPS = "https"
	TypeSTCP  = "stcp"
	TypeXTCP  = "xtcp"
)

var proxyTypes = map[string]bool{TypeTCP: true, TypeUDP: true, TypeHTTP: true, TypeHTTPS: true, TypeSTCP: true, TypeXTCP: true}

var protocols = map[string]bool{"": true, "tcp": true, "kcp": true, "quic": true, "websocket": true, "wss": true}

// ClientSettings 是 frpc（客户端）与 frps 之间的连接参数。
type ClientSettings struct {
	ServerAddr string `json:"serverAddr"`
	ServerPort int    `json:"serverPort"`
	Token      string `json:"token"`
	User       string `json:"user"`
	// Protocol 为 frpc 与 frps 之间的传输协议，留空使用 tcp
	Protocol string `json:"protocol"`
}

// ServerSettings 是 frps（服务端）的监听与认证参数。
type ServerSettings struct {
	// BindAddr 是 frpc 连接端口的监听地址，留空监听所有地址；
	// 填 127.0.0.1 时可放在反向代理之后，让 frpc 用 websocket / wss 连接
	BindAddr string `json:"bindAddr"`
	// BindPort 是 frpc 连接的端口
	BindPort int    `json:"bindPort"`
	Token    string `json:"token"`
	// ProxyBindAddr 是 HTTP / HTTPS 端口与 tcp / udp 代理远程端口的监听地址，留空监听所有地址
	ProxyBindAddr string `json:"proxyBindAddr"`
	// VhostHTTPPort 与 VhostHTTPSPort 为 0 时不启用 http / https 代理
	VhostHTTPPort  int `json:"vhostHttpPort"`
	VhostHTTPSPort int `json:"vhostHttpsPort"`
	// SubdomainHost 是子域名代理的根域名
	SubdomainHost string `json:"subdomainHost"`
	// AllowPorts 限制 frpc 可申请的远程端口，例如 "6000-6100,7000"，留空不限制
	AllowPorts string `json:"allowPorts"`
	// 控制台端口为 0 时关闭
	DashboardPort int    `json:"dashboardPort"`
	DashboardAddr string `json:"dashboardAddr"`
	DashboardUser string `json:"dashboardUser"`
	DashboardPwd  string `json:"dashboardPwd"`
}

// Client 是一个 frpc 实例：连接一台 frps，运行一个独立的 frpc 进程，代理通过 Proxy.Client 归属到实例。
type Client struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Group 为列表中的分组名称，只影响管理界面的展示，留空为未分组
	Group  string `json:"group"`
	Enable bool   `json:"enable"`
	ClientSettings
	Remark string `json:"remark"`
}

// Server 是一个 frps 实例，运行一个独立的 frps 进程。
type Server struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Group 为列表中的分组名称，只影响管理界面的展示，留空为未分组
	Group  string `json:"group"`
	Enable bool   `json:"enable"`
	ServerSettings
	Remark string `json:"remark"`
}

// Settings 是模块级设置。
type Settings struct {
	// FrpcPath、FrpsPath 为空时依次在配置目录 bin 与 PATH 下查找 frpc、frps
	FrpcPath string `json:"frpcPath"`
	FrpsPath string `json:"frpsPath"`
}

// Proxy 是一条 frpc 代理，把本地服务暴露到 frps。
type Proxy struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Group 为列表中的分组名称，只影响管理界面的展示，留空为未分组
	Group     string `json:"group"`
	Type      string `json:"type"`
	Enable    bool   `json:"enable"`
	LocalIP   string `json:"localIp"`
	LocalPort int    `json:"localPort"`
	// RemotePort 用于 tcp 与 udp
	RemotePort int `json:"remotePort"`
	// CustomDomains 与 Subdomain 用于 http 与 https
	CustomDomains []string `json:"customDomains"`
	Subdomain     string   `json:"subdomain"`
	// SecretKey 用于 stcp 与 xtcp，访问端需使用相同密钥
	SecretKey string `json:"secretKey"`
	Remark    string `json:"remark"`
	// Client 是代理所属 frpc 实例的 key
	Client string `json:"client"`
}

type fileConfig struct {
	Settings Settings `json:"settings"`
	// Install 为在线安装设置
	Install binstall.Config `json:"install"`
	Clients []Client        `json:"clients"`
	Servers []Server        `json:"servers"`
	Proxies []Proxy         `json:"proxies"`
	// LegacyClient、LegacyServer 是 0.5.9 及更早版本的单实例配置，仅在加载时读取并迁移为实例
	LegacyClient *v1Client `json:"client,omitempty"`
	LegacyServer *v1Server `json:"server,omitempty"`
}

// v1Client 是旧版单实例 frpc 配置。
type v1Client struct {
	Enable  bool   `json:"enable"`
	BinPath string `json:"binPath"`
	ClientSettings
}

// v1Server 是旧版单实例 frps 配置。
type v1Server struct {
	Enable  bool   `json:"enable"`
	BinPath string `json:"binPath"`
	ServerSettings
}

func validPort(p int) bool { return p >= 1 && p <= 65535 }

// Validate 校验并规范化连接参数；服务端地址是否必填由 Client.Validate 按启用状态判断。
func (c *ClientSettings) Validate() error {
	c.ServerAddr = strings.TrimSpace(c.ServerAddr)
	c.User = strings.TrimSpace(c.User)
	if c.ServerPort == 0 {
		c.ServerPort = 7000
	}
	if !validPort(c.ServerPort) {
		return errors.New("服务端端口无效")
	}
	if !protocols[c.Protocol] {
		return fmt.Errorf("不支持的传输协议 %q", c.Protocol)
	}
	return nil
}

// Validate 校验并规范化 frpc 实例。
func (c *Client) Validate() error {
	if err := crud.NormalizeGroup(&c.Group); err != nil {
		return err
	}
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return errors.New("名称不能为空")
	}
	if err := c.ClientSettings.Validate(); err != nil {
		return err
	}
	if c.Enable && c.ServerAddr == "" {
		return errors.New("启用 frpc 时需要填写服务端地址")
	}
	return nil
}

// Validate 校验并规范化 frps 实例。
func (s *Server) Validate() error {
	if err := crud.NormalizeGroup(&s.Group); err != nil {
		return err
	}
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" {
		return errors.New("名称不能为空")
	}
	return s.ServerSettings.Validate()
}

// Validate 校验并规范化服务端配置。
func (s *ServerSettings) Validate() error {
	s.BindAddr = strings.TrimSpace(s.BindAddr)
	s.ProxyBindAddr = strings.TrimSpace(s.ProxyBindAddr)
	s.SubdomainHost = strings.TrimSpace(s.SubdomainHost)
	s.DashboardAddr = strings.TrimSpace(s.DashboardAddr)
	s.DashboardUser = strings.TrimSpace(s.DashboardUser)
	if s.BindPort == 0 {
		s.BindPort = 7000
	}
	if !validPort(s.BindPort) {
		return errors.New("绑定端口无效")
	}
	if s.BindAddr != "" && net.ParseIP(s.BindAddr) == nil {
		return errors.New("监听地址无效")
	}
	if s.ProxyBindAddr != "" && net.ParseIP(s.ProxyBindAddr) == nil {
		return errors.New("代理监听地址无效")
	}
	for _, p := range []int{s.VhostHTTPPort, s.VhostHTTPSPort, s.DashboardPort} {
		if p != 0 && !validPort(p) {
			return errors.New("端口无效")
		}
	}
	if _, err := ParseAllowPorts(s.AllowPorts); err != nil {
		return err
	}
	if s.DashboardAddr == "" {
		// 控制台默认仅本机可访问
		s.DashboardAddr = "127.0.0.1"
	}
	if net.ParseIP(s.DashboardAddr) == nil {
		return errors.New("控制台监听地址无效")
	}
	if s.DashboardPort != 0 && (s.DashboardUser == "" || s.DashboardPwd == "") {
		return errors.New("启用控制台时需要填写用户名和密码")
	}
	return nil
}

// PortRange 是闭区间端口范围。
type PortRange struct{ Start, End int }

// ParseAllowPorts 解析 "6000-6100,7000" 形式的端口范围。
func ParseAllowPorts(s string) ([]PortRange, error) {
	var out []PortRange
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lo, hi, isRange := strings.Cut(part, "-")
		start, err := strconv.Atoi(strings.TrimSpace(lo))
		end := start
		if err == nil && isRange {
			end, err = strconv.Atoi(strings.TrimSpace(hi))
		}
		if err != nil || !validPort(start) || !validPort(end) || start > end {
			return nil, fmt.Errorf("允许端口范围 %q 格式错误", part)
		}
		out = append(out, PortRange{start, end})
	}
	return out, nil
}

// Validate 校验并规范化代理配置。
func (p *Proxy) Validate() error {
	if err := crud.NormalizeGroup(&p.Group); err != nil {
		return err
	}
	p.Name = strings.TrimSpace(p.Name)
	p.LocalIP = strings.TrimSpace(p.LocalIP)
	p.Subdomain = strings.TrimSpace(p.Subdomain)
	if p.Name == "" {
		return errors.New("代理名称不能为空")
	}

	if !proxyTypes[p.Type] {
		return fmt.Errorf("不支持的代理类型 %q", p.Type)
	}
	if p.LocalIP == "" {
		p.LocalIP = "127.0.0.1"
	}
	if !validPort(p.LocalPort) {
		return errors.New("本地端口无效")
	}
	var domains []string
	for _, d := range p.CustomDomains {
		if d = strings.TrimSpace(d); d != "" {
			domains = append(domains, d)
		}
	}
	p.CustomDomains = domains
	switch p.Type {
	case TypeTCP, TypeUDP:
		if !validPort(p.RemotePort) {
			return errors.New("远程端口无效")
		}
	case TypeHTTP, TypeHTTPS:
		if len(p.CustomDomains) == 0 && p.Subdomain == "" {
			return errors.New("http / https 代理需要填写自定义域名或子域名")
		}
	case TypeSTCP, TypeXTCP:
		if p.SecretKey == "" {
			return errors.New("stcp / xtcp 代理需要填写密钥")
		}
	}
	return nil
}

// quote 生成 TOML 基本字符串。
func quote(s string) string { return strconv.Quote(s) }

// ClientTOML 生成 frpc 配置文件内容（frp 0.52+ 的 TOML 格式）。
func ClientTOML(c ClientSettings, proxies []Proxy) string {
	var b strings.Builder
	fmt.Fprintf(&b, "serverAddr = %s\n", quote(c.ServerAddr))
	fmt.Fprintf(&b, "serverPort = %d\n", c.ServerPort)
	if c.User != "" {
		fmt.Fprintf(&b, "user = %s\n", quote(c.User))
	}
	// 首次登录失败时 frpc 不退出而是持续重试，服务端暂时不可达也不会让进程反复重启
	b.WriteString("loginFailExit = false\n")
	if c.Token != "" {
		b.WriteString("auth.method = \"token\"\n")
		fmt.Fprintf(&b, "auth.token = %s\n", quote(c.Token))
	}
	if c.Protocol != "" {
		fmt.Fprintf(&b, "transport.protocol = %s\n", quote(c.Protocol))
	}
	names := map[string]bool{}
	for _, p := range proxies {
		// 停用与重名的代理不写入配置
		if !p.Enable || names[p.Name] {
			continue
		}
		names[p.Name] = true
		b.WriteString("\n[[proxies]]\n")
		fmt.Fprintf(&b, "name = %s\n", quote(p.Name))
		fmt.Fprintf(&b, "type = %s\n", quote(p.Type))
		fmt.Fprintf(&b, "localIP = %s\n", quote(p.LocalIP))
		fmt.Fprintf(&b, "localPort = %d\n", p.LocalPort)
		switch p.Type {
		case TypeTCP, TypeUDP:
			fmt.Fprintf(&b, "remotePort = %d\n", p.RemotePort)
		case TypeHTTP, TypeHTTPS:
			if len(p.CustomDomains) > 0 {
				quoted := make([]string, len(p.CustomDomains))
				for i, d := range p.CustomDomains {
					quoted[i] = quote(d)
				}
				fmt.Fprintf(&b, "customDomains = [%s]\n", strings.Join(quoted, ", "))
			}
			if p.Subdomain != "" {
				fmt.Fprintf(&b, "subdomain = %s\n", quote(p.Subdomain))
			}
		case TypeSTCP, TypeXTCP:
			fmt.Fprintf(&b, "secretKey = %s\n", quote(p.SecretKey))
		}
	}
	return b.String()
}

// ServerTOML 生成 frps 配置文件内容。
func ServerTOML(s ServerSettings) string {
	var b strings.Builder
	if s.BindAddr != "" {
		fmt.Fprintf(&b, "bindAddr = %s\n", quote(s.BindAddr))
	}
	fmt.Fprintf(&b, "bindPort = %d\n", s.BindPort)
	if s.ProxyBindAddr != "" {
		fmt.Fprintf(&b, "proxyBindAddr = %s\n", quote(s.ProxyBindAddr))
	} else if s.BindAddr != "" {
		// frps 的 proxyBindAddr 默认跟随 bindAddr；这里留空表示监听所有地址，需显式写出
		b.WriteString("proxyBindAddr = \"0.0.0.0\"\n")
	}
	if s.VhostHTTPPort != 0 {
		fmt.Fprintf(&b, "vhostHTTPPort = %d\n", s.VhostHTTPPort)
	}
	if s.VhostHTTPSPort != 0 {
		fmt.Fprintf(&b, "vhostHTTPSPort = %d\n", s.VhostHTTPSPort)
	}
	if s.SubdomainHost != "" {
		fmt.Fprintf(&b, "subDomainHost = %s\n", quote(s.SubdomainHost))
	}
	if s.Token != "" {
		b.WriteString("auth.method = \"token\"\n")
		fmt.Fprintf(&b, "auth.token = %s\n", quote(s.Token))
	}
	if ranges, _ := ParseAllowPorts(s.AllowPorts); len(ranges) > 0 {
		items := make([]string, len(ranges))
		for i, r := range ranges {
			items[i] = fmt.Sprintf("{ start = %d, end = %d }", r.Start, r.End)
		}
		fmt.Fprintf(&b, "allowPorts = [%s]\n", strings.Join(items, ", "))
	}
	if s.DashboardPort != 0 {
		fmt.Fprintf(&b, "webServer.addr = %s\n", quote(s.DashboardAddr))
		fmt.Fprintf(&b, "webServer.port = %d\n", s.DashboardPort)
		fmt.Fprintf(&b, "webServer.user = %s\n", quote(s.DashboardUser))
		fmt.Fprintf(&b, "webServer.password = %s\n", quote(s.DashboardPwd))
	}
	return b.String()
}
