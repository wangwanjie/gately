package easytier

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/wangwanjie/gately/internal/crud"
)

// 以下结论对照 EasyTier v2.6.4（在线安装默认安装的最新版本）的 easytier-core --help 与实测，是参数设计的依据：
//
//   - 网络密钥通过环境变量 ET_NETWORK_SECRET 传给 easytier-core，不出现在进程参数里（ps 与进程列表可见）。
//   - 所有取值参数使用 --name=value 形式，值以 - 开头也不会被当作另一个参数。
//   - --socks5 只接受端口并监听全部地址，没有认证，等于对外开放一个能访问组网的代理，所以不提供该选项；
//     无 TUN 设备时用“端口转发”（--port-forward）访问组网内的服务，绑定地址由用户指定，建议 127.0.0.1。
//   - -l/--listeners 只写端口号会同时监听 tcp/udp/ws/wss/wg 多个端口，所以监听地址必须写成 协议://地址:端口；
//     不填任何监听地址时传 --no-listener，只主动连接其他节点（打洞与中继照常工作）。
//   - 管理端口（--rpc-portal）只监听 127.0.0.1 的随机空闲端口并限定来源为本机回环地址，状态由 easytier-cli 经它读取。
//     easytier-cli node 的输出含网络密钥（config 字段），解析时只取需要的字段，不转发到界面。
//   - 多个实例各自是一个独立进程，没有共享的系统状态，除监听端口、网卡名、静态虚拟 IP 外互不影响。

// Settings 是模块级（程序级）设置，所有实例共用。
type Settings struct {
	// CorePath 为空时依次在配置目录 bin 与 PATH 下查找 easytier-core；easytier-cli 在同一目录或同样位置查找
	CorePath string `json:"corePath"`
	// InstallProxy 为在线安装使用的下载代理，空表示沿用全局“更新代理”；只通过安装接口修改
	InstallProxy string `json:"installProxy"`
}

// Options 是一个 easytier-core 进程的运行参数。
type Options struct {
	// NetworkName 与 NetworkSecret 共同决定加入哪个组网：名称与密钥相同的节点互通
	NetworkName   string `json:"networkName"`
	NetworkSecret string `json:"networkSecret"`
	// Peers 是启动时主动连接的节点地址，每行一个，例如 tcp://public.easytier.cn:11010、udp://1.2.3.4:11010
	Peers string `json:"peers"`
	// Listeners 是监听地址，每行一个，例如 tcp://0.0.0.0:11010；留空不监听，只主动连接其他节点
	Listeners string `json:"listeners"`
	// DHCP 让 EasyTier 在组网内自动分配虚拟 IP；关闭且 IPv4 留空时本节点只转发数据，不创建网卡
	DHCP bool `json:"dhcp"`
	// IPv4 为固定的虚拟 IP，可带掩码长度，例如 10.126.126.2/24；DHCP 开启时忽略
	IPv4     string `json:"ipv4"`
	Hostname string `json:"hostname"`
	// NoTun 使用用户态网络，不创建 TUN 网卡（无需特权，适合容器）；此时本机经“端口转发”访问组网内的服务
	NoTun bool `json:"noTun"`
	// DevName 为 TUN 网卡名，留空由 EasyTier 决定
	DevName string `json:"devName"`
	// ProxyNetworks 是共享给组网的本地网段（子网代理），每行一个，例如 192.168.1.0/24，
	// 也可写成 192.168.1.0/24->10.1.0.0/24 把网段映射为另一个网段
	ProxyNetworks string `json:"proxyNetworks"`
	// PortForwards 是把本机端口转发到组网内某地址的规则，每行一个，例如 tcp://127.0.0.1:2222/10.126.126.1:22
	PortForwards string `json:"portForwards"`
	// EnableExitNode 允许本机作为出口节点，其他节点可以把全部流量交给本机转发
	EnableExitNode bool `json:"enableExitNode"`
	// ExitNodes 是使用的出口节点（虚拟 IPv4），每行一个，按顺序决定优先级
	ExitNodes string `json:"exitNodes"`
	// LatencyFirst 选择延迟最低的路径转发，默认选择跳数最少的路径
	LatencyFirst bool `json:"latencyFirst"`
	// KCPProxy 与 QUICProxy 用 KCP、QUIC 代理 TCP 流量，在 UDP 丢包的网络上降低延迟、提高吞吐
	KCPProxy  bool `json:"kcpProxy"`
	QUICProxy bool `json:"quicProxy"`
	// ExtraArgs 追加到 easytier-core 之后，以空白分隔
	ExtraArgs string `json:"extraArgs"`
	// Env 是传给进程的环境变量，每行一个 KEY=VALUE
	Env string `json:"env"`
}

// Instance 是一个 EasyTier 实例：一个独立的 easytier-core 进程，加入一个 EasyTier 组网。
type Instance struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Group 为列表中的分组名称，只影响管理界面的展示，留空为未分组
	Group  string `json:"group"`
	Enable bool   `json:"enable"`
	Options
	Remark string `json:"remark"`
}

// DefaultOptions 是新实例的默认参数：自动分配虚拟 IP，不监听端口，只主动连接。
func DefaultOptions() Options {
	return Options{DHCP: true}
}

// fileConfig 是 easytier.json 的内容。
type fileConfig struct {
	Settings  Settings   `json:"settings"`
	Instances []Instance `json:"instances"`
}

var (
	hostnameRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)
	envNameRe  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	// networkNameRe 限制组网名称的字符，避免空白与控制字符
	networkNameRe = regexp.MustCompile(`^[A-Za-z0-9._@:+-]{1,64}$`)
	devNameRe     = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,15}$`)
)

// maxSecretLen 是网络密钥的最大长度。
const maxSecretLen = 128

// maxLines 是多行字段的最大行数，防止超长参数列表。
const maxLines = 64

// lines 把多行文本拆成非空行，忽略空行与 # 开头的注释行，并去掉首尾空白。
func lines(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

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

// peerSchemes 与 listenerSchemes 是 EasyTier 支持的连接协议。
var (
	peerSchemes     = []string{"tcp", "udp", "ws", "wss", "quic", "wg", "faketcp", "http", "https", "txt", "srv"}
	listenerSchemes = []string{"tcp", "udp", "ws", "wss", "quic", "wg", "faketcp"}
)

func hasScheme(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// checkURL 校验 协议://主机:端口 形式的地址，返回规范化后的原文（去掉首尾空白）。
// needPort 为 true 时必须带端口；http、https、txt、srv 形式的节点发现地址允许不带端口。
func checkURL(raw string, schemes []string, needPort bool) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || !hasScheme(schemes, strings.ToLower(u.Scheme)) {
		return fmt.Errorf("地址 %q 格式应为 协议://主机:端口，协议可选 %s", raw, strings.Join(schemes, "、"))
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("地址 %q 不能带用户信息或查询参数", raw)
	}
	if strings.ContainsAny(raw, " \t\r\x00") {
		return fmt.Errorf("地址 %q 含有空白字符", raw)
	}
	port := u.Port()
	if port == "" {
		if needPort {
			return fmt.Errorf("地址 %q 缺少端口", raw)
		}
		return nil
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("地址 %q 的端口需在 1-65535 之间", raw)
	}
	return nil
}

// checkProxyNetwork 校验子网代理：CIDR，或 CIDR->CIDR 映射（两边掩码长度需一致）。
func checkProxyNetwork(s string) error {
	real, mapped, hasMap := strings.Cut(s, "->")
	a, err := netip.ParsePrefix(strings.TrimSpace(real))
	if err != nil || !a.Addr().Is4() {
		return fmt.Errorf("子网代理 %q 格式应为 192.168.1.0/24（IPv4 网段）", s)
	}
	if !hasMap {
		return nil
	}
	b, err := netip.ParsePrefix(strings.TrimSpace(mapped))
	if err != nil || !b.Addr().Is4() || a.Bits() != b.Bits() {
		return fmt.Errorf("子网代理 %q 映射后的网段需是掩码长度相同的 IPv4 网段，例如 192.168.1.0/24->10.1.0.0/24", s)
	}
	return nil
}

// checkPortForward 校验端口转发：协议://绑定地址:端口/目标地址:端口，协议为 tcp 或 udp。
func checkPortForward(s string) error {
	bad := fmt.Errorf("端口转发 %q 格式应为 tcp://127.0.0.1:2222/10.126.126.1:22（协议 tcp 或 udp）", s)
	proto, rest, ok := strings.Cut(s, "://")
	if !ok || (proto != "tcp" && proto != "udp") {
		return bad
	}
	bind, dst, ok := strings.Cut(rest, "/")
	if !ok {
		return bad
	}
	for _, hp := range []string{bind, dst} {
		host, port, err := net.SplitHostPort(hp)
		if err != nil || net.ParseIP(host) == nil {
			return bad
		}
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return bad
		}
	}
	return nil
}

// Validate 校验并规范化程序设置。
func (s *Settings) Validate() {
	s.CorePath = strings.TrimSpace(s.CorePath)
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

// Validate 校验并规范化单个进程的参数。
func (s *Options) Validate() error {
	s.NetworkName = strings.TrimSpace(s.NetworkName)
	s.NetworkSecret = strings.TrimSpace(s.NetworkSecret)
	s.IPv4 = strings.TrimSpace(s.IPv4)
	s.Hostname = strings.TrimSpace(s.Hostname)
	s.DevName = strings.TrimSpace(s.DevName)
	s.ExtraArgs = strings.TrimSpace(s.ExtraArgs)
	s.Env = strings.TrimSpace(s.Env)
	if !networkNameRe.MatchString(s.NetworkName) {
		return errors.New("组网名称为 1-64 个字符，只能包含字母、数字和 . _ @ : + -")
	}
	if len(s.NetworkSecret) > maxSecretLen || strings.ContainsAny(s.NetworkSecret, "\r\n\x00") {
		return fmt.Errorf("网络密钥不能超过 %d 个字符，且不能换行", maxSecretLen)
	}
	if s.Hostname != "" && !hostnameRe.MatchString(s.Hostname) {
		return errors.New("主机名只能包含字母、数字和连字符")
	}
	if s.DevName != "" && !devNameRe.MatchString(s.DevName) {
		return errors.New("网卡名只能包含字母、数字和 _ . -，最长 15 个字符")
	}
	if s.IPv4 != "" {
		addr := s.IPv4
		if strings.Contains(addr, "/") {
			p, err := netip.ParsePrefix(addr)
			if err != nil || !p.Addr().Is4() {
				return errors.New("虚拟 IP 格式应为 10.126.126.2 或 10.126.126.2/24")
			}
		} else if a, err := netip.ParseAddr(addr); err != nil || !a.Is4() {
			return errors.New("虚拟 IP 格式应为 10.126.126.2 或 10.126.126.2/24")
		}
	}
	for _, f := range []struct {
		text *string
		what string
		each func(string) error
	}{
		{&s.Peers, "节点地址", func(l string) error { return checkURL(l, peerSchemes, false) }},
		{&s.Listeners, "监听地址", func(l string) error { return checkURL(l, listenerSchemes, true) }},
		{&s.ProxyNetworks, "子网代理", checkProxyNetwork},
		{&s.PortForwards, "端口转发", checkPortForward},
		{&s.ExitNodes, "出口节点", func(l string) error {
			if a, err := netip.ParseAddr(l); err != nil || !a.Is4() {
				return fmt.Errorf("出口节点 %q 需是组网内的虚拟 IPv4 地址", l)
			}
			return nil
		}},
	} {
		l := lines(*f.text)
		*f.text = strings.Join(l, "\n")
		if len(l) > maxLines {
			return fmt.Errorf("%s最多 %d 行", f.what, maxLines)
		}
		for _, line := range l {
			if err := f.each(line); err != nil {
				return err
			}
		}
	}
	if _, err := parseEnv(s.Env); err != nil {
		return err
	}
	for _, a := range strings.Fields(s.ExtraArgs) {
		// 写在附加参数里的密钥会出现在进程命令行中，本机任何用户都能用 ps 看到
		if a == "--network-secret" || strings.HasPrefix(a, "--network-secret=") {
			return errors.New("网络密钥请填写在「网络密钥」中，不要写在附加参数里")
		}
	}
	// 没有密钥时，知道组网名称的人就能加入组网；共享本地网段或作为出口节点会把局域网与本机出口交给他们
	if s.NetworkSecret == "" && (s.ProxyNetworks != "" || s.EnableExitNode) {
		return errors.New("共享本地网段或允许作为出口节点时必须设置网络密钥")
	}
	return nil
}

// Args 生成 easytier-core 的命令行参数。取值参数都用 --name=value 形式；rpcAddr 是管理端口的监听地址。
// 网络密钥不在其中，见 EnvList。
func (s *Options) Args(rpcAddr string) []string {
	args := []string{"--network-name=" + s.NetworkName}
	if s.Hostname != "" {
		args = append(args, "--hostname="+s.Hostname)
	}
	switch {
	case s.DHCP:
		args = append(args, "--dhcp=true")
	case s.IPv4 != "":
		args = append(args, "--ipv4="+s.IPv4)
	}
	for _, p := range lines(s.Peers) {
		args = append(args, "--peers="+p)
	}
	if l := lines(s.Listeners); len(l) == 0 {
		args = append(args, "--no-listener")
	} else {
		for _, v := range l {
			args = append(args, "--listeners="+v)
		}
	}
	if s.NoTun {
		args = append(args, "--no-tun=true")
	}
	if s.DevName != "" {
		args = append(args, "--dev-name="+s.DevName)
	}
	for _, v := range lines(s.ProxyNetworks) {
		args = append(args, "--proxy-networks="+v)
	}
	for _, v := range lines(s.PortForwards) {
		args = append(args, "--port-forward="+v)
	}
	if s.EnableExitNode {
		args = append(args, "--enable-exit-node=true")
	}
	for _, v := range lines(s.ExitNodes) {
		args = append(args, "--exit-nodes="+v)
	}
	if s.LatencyFirst {
		args = append(args, "--latency-first=true")
	}
	if s.KCPProxy {
		args = append(args, "--enable-kcp-proxy=true")
	}
	if s.QUICProxy {
		args = append(args, "--enable-quic-proxy=true")
	}
	// 管理端口只监听本机回环地址，并限定来源，状态由 easytier-cli 经它读取
	args = append(args, "--rpc-portal="+rpcAddr, "--rpc-portal-whitelist=127.0.0.1/32,::1/128")
	return append(args, strings.Fields(s.ExtraArgs)...)
}

// EnvList 返回传给 easytier-core 的环境变量：网络密钥，最后是用户填写的变量，同名时以用户的为准。
// 格式错误的行已在 Validate 中拒绝。
func (s *Options) EnvList() []string {
	var env []string
	if s.NetworkSecret != "" {
		env = append(env, "ET_NETWORK_SECRET="+s.NetworkSecret)
	}
	user, _ := parseEnv(s.Env)
	return append(env, user...)
}

// listenPorts 返回实例监听的端口集合（协议无关），用于冲突检查；wg 与 ws 等协议各自占用的端口都按声明的端口计。
func (s *Options) listenPorts() map[string]bool {
	out := map[string]bool{}
	for _, l := range lines(s.Listeners) {
		if u, err := url.Parse(l); err == nil && u.Port() != "" {
			out[strings.ToLower(u.Scheme)+":"+u.Port()] = true
		}
	}
	return out
}

// tcpOpenPorts 返回实例会对外监听的 TCP 端口：走 TCP 传输的监听地址，以及绑定到非回环地址的 TCP 端口转发。
// 入侵防护的扫描检测据此把这些端口视为开放端口，进程刚启动、监听尚未出现在系统监听表里时其他节点的连接不会被当成端口扫描。
func (s *Options) tcpOpenPorts() []int {
	var out []int
	add := func(host, port string) {
		n, err := strconv.Atoi(port)
		if err != nil || n <= 0 || n > 65535 {
			return
		}
		if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil && ip.IsLoopback() {
			return
		}
		out = append(out, n)
	}
	for _, l := range lines(s.Listeners) {
		if u, err := url.Parse(l); err == nil && transport(strings.ToLower(u.Scheme)) == "tcp" {
			add(u.Hostname(), u.Port())
		}
	}
	for _, l := range lines(s.PortForwards) {
		proto, rest, _ := strings.Cut(l, "://")
		if strings.ToLower(proto) != "tcp" {
			continue
		}
		bind, _, _ := strings.Cut(rest, "/")
		if host, port, err := net.SplitHostPort(bind); err == nil {
			add(host, port)
		}
	}
	return out
}

// forwardBinds 返回端口转发的本机绑定端口集合，形如 tcp:2222。
func (s *Options) forwardBinds() map[string]bool {
	out := map[string]bool{}
	for _, l := range lines(s.PortForwards) {
		proto, rest, _ := strings.Cut(l, "://")
		bind, _, _ := strings.Cut(rest, "/")
		if _, port, err := net.SplitHostPort(bind); err == nil {
			out[proto+":"+port] = true
		}
	}
	return out
}

// transport 返回协议使用的传输层：udp、quic、wg 走 UDP，其余（tcp、ws、wss、faketcp）走 TCP。
func transport(scheme string) string {
	switch scheme {
	case "udp", "quic", "wg":
		return "udp"
	}
	return "tcp"
}

// shared 返回两个集合共有的一个元素，没有时为空串。
func shared(a, b map[string]bool) string {
	for k := range a {
		if b[k] {
			return k
		}
	}
	return ""
}

// transportPorts 把 协议:端口 集合按传输层归并为 传输层:端口，同一传输层的同一端口只能被一个实例监听。
func transportPorts(ports map[string]bool) map[string]bool {
	out := map[string]bool{}
	for k := range ports {
		scheme, port, _ := strings.Cut(k, ":")
		out[transport(scheme)+":"+port] = true
	}
	return out
}

// checkConflicts 检查实例 in 与列表中其他实例是否冲突：
//   - 名称不能重复；
//   - 监听地址的端口（按传输层归并）不能被两个同时启用的实例使用；
//   - 端口转发绑定的本机端口不能被两个同时启用的实例使用；
//   - 网卡名不能被两个实例同时启用时共用。
//
// running 为 true 时用于启动前的检查：只与列表中排在 in 之前且已启用的实例比较、不检查名称，
// 让手动改坏的配置中排在前面的实例照常运行，后面冲突的实例不启动并显示原因。
func checkConflicts(list []Instance, in *Instance, running bool) error {
	listen := transportPorts(in.listenPorts())
	binds := in.forwardBinds()
	for i := range list {
		o := &list[i]
		if o == in || (in.Key != "" && o.Key == in.Key) {
			if running {
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
		if !in.Enable || !o.Enable {
			continue
		}
		if k := shared(listen, transportPorts(o.listenPorts())); k != "" {
			return fmt.Errorf("监听端口 %s 已被实例 %s 使用，请为其中一个改用其他端口", k, o.Name)
		}
		if k := shared(binds, o.forwardBinds()); k != "" {
			return fmt.Errorf("端口转发的本机端口 %s 已被实例 %s 使用", k, o.Name)
		}
		if in.DevName != "" && !in.NoTun && !o.NoTun && in.DevName == o.DevName {
			return fmt.Errorf("网卡名 %s 已被实例 %s 使用", in.DevName, o.Name)
		}
	}
	return nil
}
