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

	"github.com/wangwanjie/gately/internal/crud"
)

// DefaultManagementURL 是 NetBird 官方云的管理服务地址。
const DefaultManagementURL = "https://api.netbird.io:443"

// 以下结论对照 NetBird v0.82.0（在线安装默认安装的最新版本）源码，是多实例隔离方式的依据：
//
//   - 守护进程（netbird service run）的全部持久状态都在 NB_STATE_DIR 下：profile 配置 default.json、
//     active_profile.json、state.json 与 service.json（client/internal/profilemanager/service.go、
//     client/configs/configs.go）。因此每个实例使用独立的状态目录，配置与 WireGuard 密钥就互不影响。
//   - netbird 命令通过 --daemon-addr 连接守护进程（默认 unix:///var/run/netbird.sock）；unix 套接字不存在时
//     会在 /var/run/netbird 下找唯一的 .sock 代替。实例默认把套接字放在自己的状态目录里，不会被误连。
//   - netbird up 以 root 运行时向守护进程询问当前 profile；非 root 时读取用户配置目录中的 active_profile.json
//     （只读，额外参数带 --profile 时才写入）。两个实例都使用 default profile，因此互不影响。
//   - 内核模式（未设置 NB_USE_NETSTACK_MODE）在 Linux 上使用固定的路由表 7120（0x1BD0，名称 netbird）、
//     固定的防火墙标记基数、名为 netbird 的 nftables 表，默认网卡名 wt0（macOS 为 utun100），两个内核实例
//     会互相覆盖路由、规则与网卡，所以同时只能启用一个内核实例。nftables 表名可用 NB_NFTABLES_TABLE 改，
//     网卡名可用 netbird up --interface-name 改，但路由表编号（systemops_linux.go 的 NetbirdVPNTableID）
//     与防火墙标记没有可配置项，换名也无法让两个内核实例共存，因此不额外为内核实例分配网卡名或表名。
//   - 用户态网络（netstack）不创建系统网卡、不使用策略路由与内核防火墙（改用用户态过滤器），也不修改系统 DNS，
//     多个用户态实例可以与一个内核实例并存；它们只在 SOCKS5 代理（默认 127.0.0.1:1080）上可能冲突。
//   - WireGuard 端口默认 51820；端口被占用时 netbird 会悄悄改用系统分配的随机端口（client/internal/connect.go
//     的 freePort），防火墙放行的端口就失效了，所以两个托管实例的有效端口不能相同。

// Settings 是模块级（程序级）设置，所有实例共用。
type Settings struct {
	// NetbirdPath 为空时依次在配置目录 bin 与 PATH 下查找
	NetbirdPath string `json:"netbirdPath"`
	// InstallProxy 为在线安装使用的下载代理，空表示沿用全局“更新代理”；只通过安装接口修改
	InstallProxy string `json:"installProxy"`
}

// Options 是一个 NetBird 守护进程的运行与登录参数。
type Options struct {
	// ManageDaemon 为 true 时由 Gately 运行 netbird 守护进程（netbird service run）；
	// 为 false 时使用系统已安装的 NetBird 服务，只负责 netbird up 与状态展示
	ManageDaemon bool `json:"manageDaemon"`
	// StateDir 保存登录状态与 WireGuard 密钥，留空为配置目录下的 netbird/<实例 key>；通过 NB_STATE_DIR 传给守护进程。
	// 可填已有的状态目录以沿用原节点身份
	StateDir string `json:"stateDir"`
	// DaemonAddr 为守护进程的控制地址，形如 unix:///path/netbird.sock 或 tcp://127.0.0.1:41731；
	// 托管时留空为状态目录下的 netbird.sock，外部守护进程留空使用系统默认
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
}

// Instance 是一个 NetBird 实例：独立的状态目录、控制地址与守护进程，可分别加入不同的 NetBird 网络。
type Instance struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Group 为列表中的分组名称，只影响管理界面的展示，留空为未分组
	Group  string `json:"group"`
	Enable bool   `json:"enable"`
	Options
	Remark string `json:"remark"`
	// LegacyStateDir 表示实例由 0.6.4 及更早版本的单实例配置迁移而来：stateDir 留空时沿用升级前的默认目录
	// <配置目录>/netbird，节点身份不变。由服务端维护，修改时保留原值，复制时清除
	LegacyStateDir bool `json:"legacyStateDir,omitempty"`
}

// DefaultOptions 是新实例的默认参数：托管守护进程并使用用户态网络，允许从组网访问本机服务，允许作为路由节点。
func DefaultOptions() Options {
	return Options{ManageDaemon: true, Userspace: true, LocalForwarding: true, ServerRoutes: true}
}

// fileConfig 是 netbird.json 的内容。
type fileConfig struct {
	Settings  Settings   `json:"settings"`
	Instances []Instance `json:"instances"`
}

// v1Settings 是 0.6.4 及更早版本的单实例配置（netbird.json 顶层即为全部字段），仅在加载时读取并迁移为实例。
type v1Settings struct {
	Enable       bool   `json:"enable"`
	NetbirdPath  string `json:"netbirdPath"`
	InstallProxy string `json:"installProxy"`
	Options
}

// legacyName 是旧版单实例配置迁移出的实例名称。
const legacyName = "netbird"

// migrateLegacy 把旧版单实例配置迁移为名为 netbird 的实例：程序路径与下载代理移入共享设置，
// 其余参数原样成为实例参数，并标记沿用旧的默认状态目录，迁移后的命令行与环境变量与升级前完全一致。
func migrateLegacy(old v1Settings) fileConfig {
	return fileConfig{
		Settings: Settings{NetbirdPath: old.NetbirdPath, InstallProxy: old.InstallProxy},
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

// Validate 校验并规范化程序设置。
func (s *Settings) Validate() {
	s.NetbirdPath = strings.TrimSpace(s.NetbirdPath)
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

// Validate 校验并规范化单个守护进程的参数。
func (s *Options) Validate() error {
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
	if s.StateDir != "" {
		// 规范化后再比较，/a/b/ 与 /a//b 视为同一目录
		s.StateDir = filepath.Clean(s.StateDir)
	}
	if s.DaemonAddr != "" && !validDaemonAddr(s.DaemonAddr) {
		return errors.New("控制地址格式应为 unix:///var/run/netbird.sock 或 tcp://127.0.0.1:41731")
	}
	if p, ok := strings.CutPrefix(s.DaemonAddr, "unix://"); ok {
		s.DaemonAddr = "unix://" + filepath.Clean(p)
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

// defaultStateDir 返回 stateDir 留空时使用的状态目录：迁移自旧版单实例配置的实例为 <配置目录>/netbird，
// 其他实例为 <配置目录>/netbird/<实例 key>，互不覆盖。key 尚未分配（新增时的校验）或含路径分隔符、点
// （手动改坏的配置，防止拼出配置目录以外的路径）时返回空串。
func (in *Instance) defaultStateDir(configDir string) string {
	base := filepath.Join(configDir, "netbird")
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

// daemonAddr 返回实际使用的控制地址，空串表示使用 netbird 命令的系统默认值（外部守护进程），
// 或托管时 key 尚未分配。
func (in *Instance) daemonAddr(configDir string) string {
	if in.DaemonAddr != "" {
		return in.DaemonAddr
	}
	if in.ManageDaemon {
		if dir := in.stateDir(configDir); dir != "" {
			return "unix://" + filepath.Join(dir, "netbird.sock")
		}
	}
	return ""
}

// kernel 表示实例由 Gately 托管守护进程且使用内核 WireGuard 网卡。
func (in *Instance) kernel() bool {
	return in.ManageDaemon && !in.Userspace
}

// managementURL 返回实际使用的管理服务地址。
func (s *Options) managementURL() string {
	if s.ManagementURL != "" {
		return s.ManagementURL
	}
	return DefaultManagementURL
}

// DaemonArgs 生成托管守护进程的命令行参数；日志输出到标准输出，由 Gately 收集。
func (in *Instance) DaemonArgs(configDir string) []string {
	args := []string{"service", "run", "--daemon-addr=" + in.daemonAddr(configDir), "--log-file=console"}
	return append(args, strings.Fields(in.DaemonExtraArgs)...)
}

// DaemonEnvList 返回传给守护进程的环境变量（KEY=VALUE 形式）：状态目录、用户态网络及其本机转发与 SOCKS5 地址，
// 最后是用户填写的变量，同名时以用户的为准。格式错误的行已在 Validate 中拒绝。
func (in *Instance) DaemonEnvList(configDir string) []string {
	env := []string{"NB_STATE_DIR=" + in.stateDir(configDir)}
	if in.Userspace {
		env = append(env, "NB_USE_NETSTACK_MODE=true")
		if in.LocalForwarding {
			env = append(env, "NB_ENABLE_NETSTACK_LOCAL_FORWARDING=true")
		}
		if host, port, err := net.SplitHostPort(in.Socks5); err == nil {
			env = append(env, "NB_SOCKS5_LISTENER_ADDRESS="+host, "NB_SOCKS5_LISTENER_PORT="+port)
		}
	}
	user, _ := parseEnv(in.DaemonEnv)
	return append(env, user...)
}

// CLIArgs 生成 netbird 命令的全局参数。
func (in *Instance) CLIArgs(configDir string) []string {
	if addr := in.daemonAddr(configDir); addr != "" {
		return []string{"--daemon-addr=" + addr}
	}
	return nil
}

// UpArgs 生成 netbird up 的参数。开关类参数都显式给出取值，让界面配置成为唯一的事实来源：
// netbird 只更新命令行中出现的参数，未出现的沿用上次保存的值。设置密钥经环境变量传递，见 UpEnv。
func (s *Options) UpArgs() []string {
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
func (s *Options) UpEnv() []string {
	env := append([]string{}, headlessEnv...)
	if s.SetupKey != "" {
		env = append(env, "NB_SETUP_KEY="+s.SetupKey)
	}
	return env
}

// 以下是冲突比较使用的 NetBird 默认值。
const (
	// defaultWgPort 是 port 为 0 时 NetBird 使用的 WireGuard 端口
	defaultWgPort = 51820
	// defaultSocks5 是用户态网络下 socks5 留空时 NetBird 使用的 SOCKS5 监听地址
	defaultSocks5 = "127.0.0.1:1080"
	// systemDaemonAddr 代表外部守护进程留空控制地址时使用的系统默认地址（netbird 的 --daemon-addr 默认值）
	systemDaemonAddr = "unix:///var/run/netbird.sock"
)

// wgPort 返回实际使用的 WireGuard 端口，0 视为 NetBird 默认的 51820。
func (s *Options) wgPort() int {
	if s.Port > 0 {
		return s.Port
	}
	return defaultWgPort
}

// socks5Addr 返回用户态网络下实际使用的 SOCKS5 监听地址，留空为 127.0.0.1:1080。
func (s *Options) socks5Addr() string {
	if s.Socks5 != "" {
		return s.Socks5
	}
	return defaultSocks5
}

// socksOverlap 判断两个 SOCKS5 监听地址是否会抢同一个端口：端口相同，且地址相同或其中一个监听全部地址。
func socksOverlap(a, b string) bool {
	ha, pa, err1 := net.SplitHostPort(a)
	hb, pb, err2 := net.SplitHostPort(b)
	if err1 != nil || err2 != nil || pa != pb {
		return false
	}
	ia, ib := net.ParseIP(ha), net.ParseIP(hb)
	if ia == nil || ib == nil {
		return ha == hb
	}
	return ia.Equal(ib) || ia.IsUnspecified() || ib.IsUnspecified()
}

// daemonAddrID 返回用于冲突比较的控制地址，空串表示无法确定（新增实例尚未分配 key），不参与比较。
func (in *Instance) daemonAddrID(configDir string) string {
	if !in.ManageDaemon && in.DaemonAddr == "" {
		return systemDaemonAddr
	}
	return in.daemonAddr(configDir)
}

// checkConflicts 检查实例 in 与列表中其他实例是否冲突：
//   - 名称不能重复；
//   - 托管实例的状态目录、控制地址不能被两个实例共用（无论是否启用，免得启用时才发现）；
//   - 托管实例的有效 WireGuard 端口（0 视为 51820）不能相同：端口被占用时 netbird 会悄悄改用随机端口；
//   - 托管的用户态实例的 SOCKS5 监听地址（留空视为 127.0.0.1:1080）不能抢同一个端口；
//   - 同时启用的内核模式实例最多一个：NetBird 内核模式使用固定的路由表、防火墙标记与 nftables 表，默认网卡名 wt0。
//
// running 为 true 时用于启动前的检查：只与列表中排在 in 之前且已启用的实例比较、不检查名称，
// 让手动改坏的配置中排在前面的实例照常运行，后面冲突的实例不启动并显示原因。
func checkConflicts(list []Instance, in *Instance, configDir string, running bool) error {
	addr := in.daemonAddrID(configDir)
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
			return fmt.Errorf("内核模式的实例同时只能启用一个，实例 %s 已启用：NetBird 内核模式使用固定的路由表、防火墙标记与 nftables 表，默认网卡名都是 wt0，两个实例会互相覆盖，请为其中一个开启用户态网络", o.Name)
		}
		// 先比较状态目录：共用状态目录时默认的控制地址也相同，提示状态目录更直接
		if dir != "" && o.ManageDaemon && dir == o.stateDir(configDir) {
			return fmt.Errorf("状态目录 %s 已被实例 %s 使用", dir, o.Name)
		}
		if addr != "" && addr == o.daemonAddrID(configDir) {
			if !o.ManageDaemon && o.DaemonAddr == "" {
				return fmt.Errorf("实例 %s 已使用系统默认的控制地址，接入另一个外部 NetBird 服务时请填写它的控制地址", o.Name)
			}
			return fmt.Errorf("控制地址 %s 已被实例 %s 使用", addr, o.Name)
		}
		if in.ManageDaemon && o.ManageDaemon && in.wgPort() == o.wgPort() {
			return fmt.Errorf("WireGuard 端口 %d 已被实例 %s 使用（0 表示默认的 51820），请为其中一个填写其他端口", in.wgPort(), o.Name)
		}
		if in.ManageDaemon && in.Userspace && o.ManageDaemon && o.Userspace && socksOverlap(in.socks5Addr(), o.socks5Addr()) {
			return fmt.Errorf("SOCKS5 地址 %s 与实例 %s 的 %s 冲突（留空表示 127.0.0.1:1080），请为其中一个填写其他端口", in.socks5Addr(), o.Name, o.socks5Addr())
		}
	}
	return nil
}
