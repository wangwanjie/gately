package netbird

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// DefaultManagementURL 是 NetBird 官方云的管理服务地址。
const DefaultManagementURL = "https://api.netbird.io:443"

// Settings 是 NetBird 配置。
type Settings struct {
	Enable bool `json:"enable"`
	// ManageDaemon 为 true 时由 Gately 运行 netbird 守护进程（netbird service run）；
	// 为 false 时使用系统已安装的 NetBird 服务，只负责 netbird up 与状态展示
	ManageDaemon bool   `json:"manageDaemon"`
	NetbirdPath  string `json:"netbirdPath"`
	// StateDir 保存登录状态与 WireGuard 密钥，留空为配置目录下的 netbird 子目录；通过 NB_STATE_DIR 传给守护进程
	StateDir string `json:"stateDir"`
	// DaemonAddr 为守护进程的控制地址，形如 unix:///path/netbird.sock 或 tcp://127.0.0.1:41731；
	// 托管时留空为 StateDir 下的 netbird.sock，外部守护进程留空使用系统默认
	DaemonAddr string `json:"daemonAddr"`
	// Userspace 使用用户态网络（netstack），无需 TUN 设备与特权，适合容器；此时本机服务经 SOCKS5 访问其他节点
	Userspace bool `json:"userspace"`
	// LocalForwarding 在用户态网络下把其他节点发往本机组网地址的连接转交给本机网络栈，
	// 这样才能从组网访问本机服务（包括只监听 127.0.0.1 的服务，仍受管理端访问策略约束）；内核模式下本就可以访问
	LocalForwarding bool `json:"localForwarding"`
	// Socks5 为用户态网络下 SOCKS5 代理的监听地址（IP:端口），留空为 NetBird 默认的 127.0.0.1:1080
	Socks5 string `json:"socks5"`
	// SetupKey 为管理端生成的设置密钥，留空时需要在状态面板中打开登录链接完成认证
	SetupKey string `json:"setupKey"`
	// ManagementURL 为管理服务地址，留空使用 NetBird 官方云；自建时填写自己的地址
	ManagementURL string `json:"managementUrl"`
	Hostname      string `json:"hostname"`
	// Port 为 WireGuard UDP 监听端口，0 表示使用 NetBird 默认的 51820
	Port int `json:"port"`
	// AcceptRoutes 接受管理端下发给本机的网络路由（访问其他路由节点后面的网段）
	AcceptRoutes bool `json:"acceptRoutes"`
	// ServerRoutes 允许本机作为路由节点，转发管理端配置给本机的网段
	ServerRoutes bool `json:"serverRoutes"`
	// AcceptDNS 使用管理端下发的 DNS 配置
	AcceptDNS bool `json:"acceptDns"`
	// BlockInbound 拒绝来自组网的所有入站连接，优先于管理端的访问策略
	BlockInbound bool `json:"blockInbound"`
	// ExtraUpArgs 追加到 netbird up 之后，以空白分隔
	ExtraUpArgs string `json:"extraUpArgs"`
	// DaemonExtraArgs 追加到 netbird service run 之后，以空白分隔
	DaemonExtraArgs string `json:"daemonExtraArgs"`
	// DaemonEnv 是传给守护进程的环境变量，每行一个 KEY=VALUE
	DaemonEnv string `json:"daemonEnv"`
	// InstallProxy 为在线安装使用的下载代理，空表示沿用全局“更新代理”；只通过安装接口修改
	InstallProxy string `json:"installProxy"`
}

// DefaultSettings 是首次使用的默认配置：托管守护进程并使用用户态网络，允许从组网访问本机服务，允许作为路由节点。
func DefaultSettings() Settings {
	return Settings{ManageDaemon: true, Userspace: true, LocalForwarding: true, ServerRoutes: true}
}

var hostnameRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

var envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// parseEnv 解析每行一个 KEY=VALUE 的环境变量文本，忽略空行与 # 开头的注释行。
func parseEnv(text string) ([]string, error) {
	var out []string
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		name = strings.TrimSpace(name)
		if !ok || !envNameRe.MatchString(name) {
			return nil, fmt.Errorf("环境变量第 %d 行格式应为 KEY=VALUE", i+1)
		}
		out = append(out, name+"="+strings.TrimSpace(value))
	}
	return out, nil
}

// validDaemonAddr 校验守护进程控制地址，格式与 netbird 的 --daemon-addr 一致。
func validDaemonAddr(addr string) bool {
	network, rest, ok := strings.Cut(addr, "://")
	if !ok || rest == "" {
		return false
	}
	switch network {
	case "unix":
		return filepath.IsAbs(rest)
	case "tcp":
		_, _, err := net.SplitHostPort(rest)
		return err == nil
	case "npipe":
		return true
	}
	return false
}

// Validate 校验并规范化配置。
func (s *Settings) Validate() error {
	s.NetbirdPath = strings.TrimSpace(s.NetbirdPath)
	s.StateDir = strings.TrimSpace(s.StateDir)
	s.DaemonAddr = strings.TrimSpace(s.DaemonAddr)
	s.Socks5 = strings.TrimSpace(s.Socks5)
	s.SetupKey = strings.TrimSpace(s.SetupKey)
	s.ManagementURL = strings.TrimRight(strings.TrimSpace(s.ManagementURL), "/")
	s.Hostname = strings.TrimSpace(s.Hostname)
	s.ExtraUpArgs = strings.TrimSpace(s.ExtraUpArgs)
	s.DaemonExtraArgs = strings.TrimSpace(s.DaemonExtraArgs)
	s.DaemonEnv = strings.TrimSpace(s.DaemonEnv)
	if s.Port < 0 || s.Port > 65535 {
		return errors.New("WireGuard 端口需在 0-65535 之间")
	}
	if _, err := parseEnv(s.DaemonEnv); err != nil {
		return err
	}
	if s.StateDir != "" && !filepath.IsAbs(s.StateDir) {
		return errors.New("状态目录需为绝对路径")
	}
	if s.DaemonAddr != "" && !validDaemonAddr(s.DaemonAddr) {
		return errors.New("控制地址格式应为 unix:///var/run/netbird.sock 或 tcp://127.0.0.1:41731")
	}
	if s.Socks5 != "" {
		// NetBird 只接受 IP 形式的监听地址，主机名会被忽略并退回 127.0.0.1
		host, port, err := net.SplitHostPort(s.Socks5)
		if err != nil || net.ParseIP(host) == nil {
			return errors.New("SOCKS5 地址格式应为 IP:端口，例如 127.0.0.1:1080")
		}
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return errors.New("SOCKS5 端口需在 1-65535 之间")
		}
	}
	if s.Hostname != "" && !hostnameRe.MatchString(s.Hostname) {
		return errors.New("主机名只能包含字母、数字和连字符")
	}
	if s.ManagementURL != "" {
		u, err := url.Parse(s.ManagementURL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return errors.New("管理服务地址格式应为 https://netbird.example.com")
		}
	}
	return nil
}

// stateDir 返回实际使用的状态目录。
func (s *Settings) stateDir(configDir string) string {
	if s.StateDir != "" {
		return s.StateDir
	}
	return filepath.Join(configDir, "netbird")
}

// daemonAddr 返回实际使用的控制地址，空串表示使用 netbird 命令的系统默认值。
func (s *Settings) daemonAddr(configDir string) string {
	if s.DaemonAddr != "" {
		return s.DaemonAddr
	}
	if s.ManageDaemon {
		return "unix://" + filepath.Join(s.stateDir(configDir), "netbird.sock")
	}
	return ""
}

// managementURL 返回实际使用的管理服务地址。
func (s *Settings) managementURL() string {
	if s.ManagementURL != "" {
		return s.ManagementURL
	}
	return DefaultManagementURL
}

// DaemonArgs 生成托管守护进程的命令行参数；日志输出到标准输出，由 Gately 收集。
func (s *Settings) DaemonArgs(configDir string) []string {
	args := []string{"service", "run", "--daemon-addr=" + s.daemonAddr(configDir), "--log-file=console"}
	return append(args, strings.Fields(s.DaemonExtraArgs)...)
}

// DaemonEnvList 返回传给守护进程的环境变量（KEY=VALUE 形式）：状态目录、用户态网络及其本机转发与 SOCKS5 地址，
// 最后是用户填写的变量，同名时以用户的为准。格式错误的行已在 Validate 中拒绝。
func (s *Settings) DaemonEnvList(configDir string) []string {
	env := []string{"NB_STATE_DIR=" + s.stateDir(configDir)}
	if s.Userspace {
		env = append(env, "NB_USE_NETSTACK_MODE=true")
		if s.LocalForwarding {
			env = append(env, "NB_ENABLE_NETSTACK_LOCAL_FORWARDING=true")
		}
		if host, port, err := net.SplitHostPort(s.Socks5); err == nil {
			env = append(env, "NB_SOCKS5_LISTENER_ADDRESS="+host, "NB_SOCKS5_LISTENER_PORT="+port)
		}
	}
	user, _ := parseEnv(s.DaemonEnv)
	return append(env, user...)
}

// CLIArgs 生成 netbird 命令的全局参数。
func (s *Settings) CLIArgs(configDir string) []string {
	if addr := s.daemonAddr(configDir); addr != "" {
		return []string{"--daemon-addr=" + addr}
	}
	return nil
}

// UpArgs 生成 netbird up 的参数。开关类参数都显式给出取值，让界面配置成为唯一的事实来源：
// netbird 只更新命令行中出现的参数，未出现的沿用上次保存的值。设置密钥经环境变量传递，见 UpEnv。
func (s *Settings) UpArgs() []string {
	args := []string{
		"up", "--no-browser",
		"--management-url=" + s.managementURL(),
		fmt.Sprintf("--disable-client-routes=%t", !s.AcceptRoutes),
		fmt.Sprintf("--disable-server-routes=%t", !s.ServerRoutes),
		fmt.Sprintf("--disable-dns=%t", !s.AcceptDNS),
		fmt.Sprintf("--block-inbound=%t", s.BlockInbound),
	}
	// 托管时关闭守护进程的自动连接，由 Gately 在每次启动守护进程后执行 up，确保使用最新配置；
	// 外部守护进程保持其自身设置，开机后由系统服务自行连接
	if s.ManageDaemon {
		args = append(args, "--disable-auto-connect=true")
	}
	if s.Hostname != "" {
		args = append(args, "--hostname="+s.Hostname)
	}
	if s.Port > 0 {
		args = append(args, fmt.Sprintf("--wireguard-port=%d", s.Port))
	}
	return append(args, strings.Fields(s.ExtraUpArgs)...)
}

// headlessEnv 清空图形会话相关的环境变量：Linux 下 netbird 据此判断是否为桌面环境，
// 清空后使用设备码登录，登录链接可以在任意设备上打开；否则使用需要本机浏览器回调的 PKCE 登录。
var headlessEnv = []string{"DISPLAY=", "WAYLAND_DISPLAY=", "BROWSER=", "DESKTOP_SESSION=", "XDG_CURRENT_DESKTOP=", "XDG_SESSION_TYPE="}

// UpEnv 返回执行 netbird up 时追加的环境变量。设置密钥不放在命令行中，避免出现在进程列表里。
func (s *Settings) UpEnv() []string {
	env := append([]string{}, headlessEnv...)
	if s.SetupKey != "" {
		env = append(env, "NB_SETUP_KEY="+s.SetupKey)
	}
	return env
}
