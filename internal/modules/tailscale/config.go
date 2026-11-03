package tailscale

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/wangwanjie/gately/internal/crud"
)

// Settings 是模块级（程序级）设置，所有实例共用。
type Settings struct {
	// TailscaledPath、TailscalePath 为空时依次在配置目录 bin 与 PATH 下查找
	TailscaledPath string `json:"tailscaledPath"`
	TailscalePath  string `json:"tailscalePath"`
	// InstallProxy 为在线安装使用的下载代理，空表示沿用全局“更新代理”；只通过安装接口修改
	InstallProxy string `json:"installProxy"`
}

// Options 是一个 tailscaled 的运行与登录参数。
type Options struct {
	// ManageDaemon 为 true 时由 Gately 运行 tailscaled；为 false 时使用系统已有的 tailscaled，只负责 tailscale up 与状态展示
	ManageDaemon bool `json:"manageDaemon"`
	// StateDir 保存登录状态，留空为配置目录下的 tailscale/<实例 key>；可填已有的状态目录以沿用原节点身份
	StateDir string `json:"stateDir"`
	// Socket 为 tailscaled 的控制套接字，托管时留空为 StateDir 下的 tailscaled.sock，外部守护进程留空使用系统默认
	Socket string `json:"socket"`
	// Userspace 使用用户态网络，无需 TUN 设备与特权，适合容器；此时本机服务经 Socks5 访问其他节点
	Userspace bool `json:"userspace"`
	// Socks5 为用户态网络下的 SOCKS5 代理监听地址，例如 localhost:1055
	Socks5 string `json:"socks5"`
	// AuthKey 为认证密钥，留空时需要在状态面板中打开登录链接完成认证
	AuthKey     string `json:"authKey"`
	Hostname    string `json:"hostname"`
	LoginServer string `json:"loginServer"`
	// AdvertiseRoutes 是向 tailnet 发布的子网路由（CIDR）
	AdvertiseRoutes   []string `json:"advertiseRoutes"`
	AdvertiseExitNode bool     `json:"advertiseExitNode"`
	AcceptRoutes      bool     `json:"acceptRoutes"`
	AcceptDNS         bool     `json:"acceptDns"`
	// ExtraUpArgs 追加到 tailscale up 之后，以空白分隔
	ExtraUpArgs string `json:"extraUpArgs"`
	// Port 为 tailscaled 的 WireGuard UDP 监听端口，0 表示由 tailscaled 自行选择；固定后便于在防火墙放行直连
	Port int `json:"port"`
	// DaemonExtraArgs 追加到 tailscaled 命令行之后，以空白分隔
	DaemonExtraArgs string `json:"daemonExtraArgs"`
	// DaemonEnv 是传给 tailscaled 的环境变量，每行一个 KEY=VALUE，例如 TS_DEBUG_FIREWALL_MODE=nftables
	DaemonEnv string `json:"daemonEnv"`
}

// Instance 是一个 tailscaled 实例：独立的状态目录、控制套接字与进程，可分别加入不同的 tailnet。
type Instance struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Group 为列表中的分组名称，只影响管理界面的展示，留空为未分组
	Group  string `json:"group"`
	Enable bool   `json:"enable"`
	Options
	Remark string `json:"remark"`
	// LegacyStateDir 表示实例由 0.6.4 及更早版本的单实例配置迁移而来：stateDir 留空时沿用升级前的默认目录
	// <配置目录>/tailscale，节点身份不变。由服务端维护，修改时保留原值，复制时清除
	LegacyStateDir bool `json:"legacyStateDir,omitempty"`
}

// DefaultOptions 是新实例的默认参数：托管 tailscaled 并使用用户态网络。
func DefaultOptions() Options {
	return Options{ManageDaemon: true, Userspace: true}
}

// fileConfig 是 tailscale.json 的内容。
type fileConfig struct {
	Settings  Settings   `json:"settings"`
	Instances []Instance `json:"instances"`
}

// v1Settings 是 0.6.4 及更早版本的单实例配置（tailscale.json 顶层即为全部字段），仅在加载时读取并迁移为实例。
type v1Settings struct {
	Enable         bool   `json:"enable"`
	TailscaledPath string `json:"tailscaledPath"`
	TailscalePath  string `json:"tailscalePath"`
	InstallProxy   string `json:"installProxy"`
	Options
}

// legacyName 是旧版单实例配置迁移出的实例名称。
const legacyName = "tailscale"

// migrateLegacy 把旧版单实例配置迁移为名为 tailscale 的实例：程序路径与下载代理移入共享设置，
// 其余参数原样成为实例参数，并标记沿用旧的默认状态目录，迁移后的命令行与升级前完全一致。
func migrateLegacy(old v1Settings) fileConfig {
	return fileConfig{
		Settings: Settings{TailscaledPath: old.TailscaledPath, TailscalePath: old.TailscalePath, InstallProxy: old.InstallProxy},
		Instances: []Instance{{
			Key:            crud.NewKey(),
			Name:           legacyName,
			Enable:         old.Enable,
			Options:        old.Options,
			LegacyStateDir: true,
		}},
	}
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

// Validate 校验并规范化程序设置。
func (s *Settings) Validate() {
	s.TailscaledPath = strings.TrimSpace(s.TailscaledPath)
	s.TailscalePath = strings.TrimSpace(s.TailscalePath)
}

// Validate 校验并规范化实例。与其他实例之间的冲突由 checkConflicts 检查。
func (in *Instance) Validate() error {
	if err := crud.NormalizeGroup(&in.Group); err != nil {
		return err
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Remark = strings.TrimSpace(in.Remark)
	if in.Name == "" {
		return errors.New("名称不能为空")
	}
	return in.Options.Validate()
}

// Validate 校验并规范化单个 tailscaled 的参数。
func (s *Options) Validate() error {
	s.StateDir = strings.TrimSpace(s.StateDir)
	s.Socket = strings.TrimSpace(s.Socket)
	s.Socks5 = strings.TrimSpace(s.Socks5)
	s.AuthKey = strings.TrimSpace(s.AuthKey)
	s.Hostname = strings.TrimSpace(s.Hostname)
	s.LoginServer = strings.TrimSpace(s.LoginServer)
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
	if s.StateDir != "" {
		s.StateDir = filepath.Clean(s.StateDir)
	}
	if s.Socket != "" && !filepath.IsAbs(s.Socket) {
		return errors.New("控制套接字需为绝对路径")
	}
	if s.Socket != "" {
		s.Socket = filepath.Clean(s.Socket)
	}
	if s.Socks5 != "" {
		if _, _, err := net.SplitHostPort(s.Socks5); err != nil {
			return errors.New("SOCKS5 地址格式应为 主机:端口")
		}
	}
	if s.Hostname != "" && !hostnameRe.MatchString(s.Hostname) {
		return errors.New("主机名只能包含字母、数字和连字符")
	}
	if s.LoginServer != "" {
		u, err := url.Parse(s.LoginServer)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return errors.New("控制服务器地址格式应为 https://headscale.example.com")
		}
	}
	var routes []string
	for _, r := range s.AdvertiseRoutes {
		if r = strings.TrimSpace(r); r == "" {
			continue
		}
		p, err := netip.ParsePrefix(r)
		if err != nil {
			return fmt.Errorf("子网路由 %q 不是有效的 CIDR", r)
		}
		routes = append(routes, p.Masked().String())
	}
	s.AdvertiseRoutes = routes
	return nil
}

// defaultStateDir 返回 stateDir 留空时使用的状态目录：迁移自旧版单实例配置的实例为 <配置目录>/tailscale，
// 其他实例为 <配置目录>/tailscale/<实例 key>，互不覆盖。key 尚未分配（新增时的校验）时返回空串。
func (in *Instance) defaultStateDir(configDir string) string {
	base := filepath.Join(configDir, "tailscale")
	if in.LegacyStateDir {
		return base
	}
	if in.Key == "" || strings.ContainsAny(in.Key, `/\.`) {
		return ""
	}
	return filepath.Join(base, in.Key)
}

// stateDir 返回实际使用的状态目录，key 尚未分配且未填写时为空串。
func (in *Instance) stateDir(configDir string) string {
	if in.StateDir != "" {
		return in.StateDir
	}
	return in.defaultStateDir(configDir)
}

// socket 返回实际使用的控制套接字路径，空串表示使用 tailscale 命令的系统默认值（外部守护进程），
// 或托管时 key 尚未分配。
func (in *Instance) socket(configDir string) string {
	if in.Socket != "" {
		return in.Socket
	}
	if in.ManageDaemon {
		if dir := in.stateDir(configDir); dir != "" {
			return filepath.Join(dir, "tailscaled.sock")
		}
	}
	return ""
}

// kernel 表示实例由 Gately 托管 tailscaled 且使用内核 TUN 网络。
func (in *Instance) kernel() bool {
	return in.ManageDaemon && !in.Userspace
}

// DaemonArgs 生成 tailscaled 的命令行参数。
func (in *Instance) DaemonArgs(configDir string) []string {
	s := &in.Options
	args := []string{"--statedir=" + in.stateDir(configDir), "--socket=" + in.socket(configDir)}
	if s.Userspace {
		args = append(args, "--tun=userspace-networking")
	}
	if s.Socks5 != "" {
		args = append(args, "--socks5-server="+s.Socks5)
	}
	if s.Port > 0 {
		args = append(args, fmt.Sprintf("--port=%d", s.Port))
	}
	return append(args, strings.Fields(s.DaemonExtraArgs)...)
}

// DaemonEnvList 返回传给 tailscaled 的环境变量（KEY=VALUE 形式），格式错误的行已在 Validate 中拒绝。
func (s *Options) DaemonEnvList() []string {
	env, _ := parseEnv(s.DaemonEnv)
	return env
}

// CLIArgs 生成 tailscale 命令的全局参数。
func (in *Instance) CLIArgs(configDir string) []string {
	if sock := in.socket(configDir); sock != "" {
		return []string{"--socket=" + sock}
	}
	return nil
}

// UpArgs 生成 tailscale up 的参数。使用 --reset 让界面配置成为唯一的事实来源，
// 否则 tailscale 会要求列出所有非默认参数。
func (s *Options) UpArgs() []string {
	args := []string{"up", "--reset", fmt.Sprintf("--accept-routes=%t", s.AcceptRoutes), fmt.Sprintf("--accept-dns=%t", s.AcceptDNS)}
	if s.AuthKey != "" {
		args = append(args, "--auth-key="+s.AuthKey)
	}
	if s.Hostname != "" {
		args = append(args, "--hostname="+s.Hostname)
	}
	if s.LoginServer != "" {
		args = append(args, "--login-server="+s.LoginServer)
	}
	if len(s.AdvertiseRoutes) > 0 {
		args = append(args, "--advertise-routes="+strings.Join(s.AdvertiseRoutes, ","))
	}
	if s.AdvertiseExitNode {
		args = append(args, "--advertise-exit-node")
	}
	return append(args, strings.Fields(s.ExtraUpArgs)...)
}

// systemSocket 代表外部守护进程留空控制套接字时使用的系统默认套接字，用于冲突比较。
const systemSocket = "\x00system"

// socketID 返回用于冲突比较的控制套接字，空串表示无法确定（新增实例尚未分配 key），不参与比较。
func (in *Instance) socketID(configDir string) string {
	if !in.ManageDaemon && in.Socket == "" {
		return systemSocket
	}
	return in.socket(configDir)
}

// checkConflicts 检查实例 in 与列表中其他实例是否冲突：
//   - 名称不能重复；
//   - 控制套接字、托管实例的状态目录与固定的 WireGuard 端口不能被两个实例共用（无论是否启用，免得启用时才发现）；
//   - 同时启用的内核模式实例最多一个：tailscaled 使用固定的路由表 52 与防火墙标记，两个内核实例会互相覆盖路由与规则。
//
// running 为 true 时用于启动前的检查：只与列表中排在 in 之前且已启用的实例比较、不检查名称，
// 让手动改坏的配置中排在前面的实例照常运行，后面冲突的实例不启动并显示原因。
func checkConflicts(list []Instance, in *Instance, configDir string, running bool) error {
	sock := in.socketID(configDir)
	var dir string
	if in.ManageDaemon {
		dir = in.stateDir(configDir)
	}
	for i := range list {
		o := &list[i]
		if o == in || (in.Key != "" && o.Key == in.Key) {
			if running {
				// 只比较排在前面的实例
				break
			}
			continue
		}
		if running && !o.Enable {
			continue
		}
		if !running && o.Name == in.Name {
			return fmt.Errorf("实例名称 %s 已存在", in.Name)
		}
		if in.Enable && o.Enable && in.kernel() && o.kernel() {
			return fmt.Errorf("内核模式的实例同时只能启用一个，实例 %s 已启用：tailscaled 的路由表与防火墙标记是固定的，两个实例会互相覆盖，请为其中一个开启用户态网络", o.Name)
		}
		// 先比较状态目录：共用状态目录时默认的控制套接字也相同，提示状态目录更直接
		if dir != "" && o.ManageDaemon && dir == o.stateDir(configDir) {
			return fmt.Errorf("状态目录 %s 已被实例 %s 使用", dir, o.Name)
		}
		if sock != "" && sock == o.socketID(configDir) {
			if sock == systemSocket {
				return fmt.Errorf("实例 %s 已使用系统默认的控制套接字，接入另一个外部 tailscaled 时请填写它的控制套接字", o.Name)
			}
			return fmt.Errorf("控制套接字 %s 已被实例 %s 使用", sock, o.Name)
		}
		if in.ManageDaemon && o.ManageDaemon && in.Port > 0 && in.Port == o.Port {
			return fmt.Errorf("WireGuard 端口 %d 已被实例 %s 使用", in.Port, o.Name)
		}
	}
	return nil
}
