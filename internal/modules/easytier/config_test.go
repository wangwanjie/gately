package easytier

import (
	"slices"
	"strings"
	"testing"
)

func validOptions() Options {
	o := DefaultOptions()
	o.NetworkName = "home"
	o.NetworkSecret = "s3cret"
	return o
}

// TestArgs 验证命令行参数：取值参数用 --name=value、网络密钥只走环境变量、不监听时带 --no-listener、管理端口限定本机。
func TestArgs(t *testing.T) {
	o := validOptions()
	o.Hostname = "nas"
	o.Peers = "tcp://public.example.com:11010\n# 注释\n\nudp://1.2.3.4:11010"
	o.ProxyNetworks = "192.168.1.0/24"
	o.PortForwards = "tcp://127.0.0.1:2222/10.126.126.1:22"
	o.LatencyFirst, o.KCPProxy, o.QUICProxy, o.EnableExitNode, o.NoTun = true, true, true, true, true
	o.ExitNodes = "10.126.126.9"
	o.DevName = "et0"
	o.ExtraArgs = "--mtu=1300  --multi-thread=true"
	o.Env = "# c\nET_FILE_LOG_LEVEL=debug"
	if err := o.Validate(); err != nil {
		t.Fatal(err)
	}
	args := o.Args("127.0.0.1:15888")
	for _, want := range []string{
		"--network-name=home", "--hostname=nas", "--dhcp=true", "--peers=tcp://public.example.com:11010", "--peers=udp://1.2.3.4:11010",
		"--no-listener", "--no-tun=true", "--dev-name=et0", "--proxy-networks=192.168.1.0/24", "--port-forward=tcp://127.0.0.1:2222/10.126.126.1:22",
		"--enable-exit-node=true", "--exit-nodes=10.126.126.9", "--latency-first=true", "--enable-kcp-proxy=true", "--enable-quic-proxy=true",
		"--rpc-portal=127.0.0.1:15888", "--rpc-portal-whitelist=127.0.0.1/32,::1/128", "--mtu=1300", "--multi-thread=true",
	} {
		if !slices.Contains(args, want) {
			t.Errorf("参数缺少 %s: %v", want, args)
		}
	}
	if slices.ContainsFunc(args, func(a string) bool { return strings.Contains(a, "s3cret") }) {
		t.Errorf("网络密钥不应出现在命令行: %v", args)
	}
	if env := o.EnvList(); !slices.Equal(env, []string{"ET_NETWORK_SECRET=s3cret", "ET_FILE_LOG_LEVEL=debug"}) {
		t.Errorf("环境变量错误: %v", env)
	}

	// 默认：自动分配 IP、不创建多余参数；有监听地址时不带 --no-listener
	d := validOptions()
	d.Listeners = "tcp://0.0.0.0:11010\nudp://0.0.0.0:11010"
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	got := d.Args("127.0.0.1:1")
	if slices.Contains(got, "--no-listener") || !slices.Contains(got, "--listeners=tcp://0.0.0.0:11010") || !slices.Contains(got, "--listeners=udp://0.0.0.0:11010") {
		t.Errorf("监听参数错误: %v", got)
	}
	for _, unwanted := range []string{"--no-tun=true", "--latency-first=true", "--enable-exit-node=true", "--enable-kcp-proxy=true"} {
		if slices.Contains(got, unwanted) {
			t.Errorf("默认不应带 %s: %v", unwanted, got)
		}
	}

	// 固定 IP：DHCP 关闭时使用 --ipv4；两者都没有时都不带（只转发）
	f := validOptions()
	f.DHCP, f.IPv4 = false, "10.126.126.2/24"
	_ = f.Validate()
	if a := f.Args("127.0.0.1:1"); !slices.Contains(a, "--ipv4=10.126.126.2/24") || slices.Contains(a, "--dhcp=true") {
		t.Errorf("固定 IP 参数错误: %v", a)
	}
	f.IPv4 = ""
	if a := f.Args("127.0.0.1:1"); slices.ContainsFunc(a, func(s string) bool { return strings.HasPrefix(s, "--ipv4") || strings.HasPrefix(s, "--dhcp") }) {
		t.Errorf("无 IP 时不应带 --ipv4 与 --dhcp: %v", a)
	}
	// 空密钥不设置环境变量
	e := validOptions()
	e.NetworkSecret = ""
	if env := e.EnvList(); len(env) != 0 {
		t.Errorf("空密钥不应设置环境变量: %v", env)
	}
}

// TestValidate 验证输入校验：参数注入、危险监听、格式错误都被拒绝。
func TestValidate(t *testing.T) {
	bad := map[string]func(o *Options){
		"空组网名":      func(o *Options) { o.NetworkName = "" },
		"组网名含空格":    func(o *Options) { o.NetworkName = "my net" },
		"组网名以参数形式":  func(o *Options) { o.NetworkName = "x --rpc-portal=0.0.0.0:1" },
		"组网名超长":     func(o *Options) { o.NetworkName = strings.Repeat("a", 65) },
		"密钥含换行":     func(o *Options) { o.NetworkSecret = "a\nb" },
		"密钥超长":      func(o *Options) { o.NetworkSecret = strings.Repeat("a", maxSecretLen+1) },
		"主机名非法":     func(o *Options) { o.Hostname = "a_b" },
		"网卡名超长":     func(o *Options) { o.DevName = strings.Repeat("a", 16) },
		"网卡名含空格":    func(o *Options) { o.DevName = "et 0" },
		"节点地址是参数":   func(o *Options) { o.Peers = "--rpc-portal=0.0.0.0:1" },
		"节点地址无协议":   func(o *Options) { o.Peers = "1.2.3.4:11010" },
		"节点地址协议不支持": func(o *Options) { o.Peers = "file:///etc/passwd" },
		"节点地址端口越界":  func(o *Options) { o.Peers = "tcp://1.2.3.4:70000" },
		"节点地址带空白":   func(o *Options) { o.Peers = "tcp://1.2.3.4:11010 --x" },
		"节点地址带用户信息": func(o *Options) { o.Peers = "tcp://u:p@1.2.3.4:11010" },
		"监听只有端口":    func(o *Options) { o.Listeners = "11010" },
		"监听缺少端口":    func(o *Options) { o.Listeners = "tcp://0.0.0.0" },
		"监听协议冒号写法":  func(o *Options) { o.Listeners = "wg:11011" },
		"监听发现协议":    func(o *Options) { o.Listeners = "http://0.0.0.0:80" },
		"虚拟IP非法":    func(o *Options) { o.DHCP, o.IPv4 = false, "10.0.0.256" },
		"虚拟IP是IPv6": func(o *Options) { o.DHCP, o.IPv4 = false, "fd00::1" },
		"子网代理非CIDR": func(o *Options) { o.ProxyNetworks = "192.168.1.1" },
		"子网代理映射掩码不同": func(o *Options) {
			o.ProxyNetworks = "192.168.1.0/24->10.0.0.0/16"
		},
		"子网代理IPv6":  func(o *Options) { o.ProxyNetworks = "fd00::/64" },
		"端口转发缺目标":   func(o *Options) { o.PortForwards = "tcp://127.0.0.1:2222" },
		"端口转发协议错":   func(o *Options) { o.PortForwards = "http://127.0.0.1:2222/10.0.0.1:22" },
		"端口转发主机名":   func(o *Options) { o.PortForwards = "tcp://localhost:2222/10.0.0.1:22" },
		"端口转发端口0":   func(o *Options) { o.PortForwards = "tcp://127.0.0.1:0/10.0.0.1:22" },
		"出口节点非IP":   func(o *Options) { o.ExitNodes = "nas" },
		"环境变量格式":    func(o *Options) { o.Env = "BAD LINE" },
		"环境变量名含横线":  func(o *Options) { o.Env = "A-B=1" },
		"附加参数写密钥":   func(o *Options) { o.ExtraArgs = "--no-tun --network-secret=abc" },
		"附加参数写密钥分开": func(o *Options) { o.ExtraArgs = "--network-secret abc" },
		"无密钥共享网段":   func(o *Options) { o.NetworkSecret, o.ProxyNetworks = "", "192.168.1.0/24" },
		"无密钥作出口节点":  func(o *Options) { o.NetworkSecret, o.EnableExitNode = "", true },
		"节点地址过多": func(o *Options) {
			o.Peers = strings.Repeat("tcp://1.2.3.4:1\n", maxLines+1)
		},
	}
	for name, edit := range bad {
		o := validOptions()
		edit(&o)
		if err := o.Validate(); err == nil {
			t.Errorf("%s 应被拒绝", name)
		}
	}
	good := validOptions()
	good.Hostname = "nas-1"
	good.NetworkName = "My_Net-1.0"
	good.Peers = "tcp://public.easytier.cn:11010\nwss://example.com:443\nhttps://example.com/nodes\nsrv://example.com\ntxt://example.com"
	good.Listeners = "tcp://0.0.0.0:11010\nquic://[::]:11012\nwg://0.0.0.0:11011\nws://0.0.0.0:11013"
	good.ProxyNetworks = "192.168.1.0/24\n192.168.2.0/24->10.1.0.0/24"
	good.PortForwards = "udp://0.0.0.0:5353/10.126.126.1:53\ntcp://[::1]:2222/10.126.126.1:22"
	good.ExitNodes = "10.126.126.9"
	good.DHCP, good.IPv4 = false, "10.126.126.2"
	if err := good.Validate(); err != nil {
		t.Errorf("合法参数被拒绝: %v", err)
	}
	// 多行字段规范化：去空行、注释与首尾空白
	n := validOptions()
	n.Peers = "  tcp://a.com:1  \n\n# x\n"
	if err := n.Validate(); err != nil || n.Peers != "tcp://a.com:1" {
		t.Errorf("多行字段应规范化: %q %v", n.Peers, err)
	}
	// 实例名称与分组
	in := Instance{Name: " ", Options: validOptions()}
	if err := in.Validate(); err == nil {
		t.Error("空名称应被拒绝")
	}
	in = Instance{Name: "home", Group: strings.Repeat("长", 65), Options: validOptions()}
	if err := in.Validate(); err == nil {
		t.Error("超长分组应被拒绝")
	}
}

func inst(key, name string, edit func(*Instance)) Instance {
	in := Instance{Key: key, Name: name, Enable: true, Options: validOptions()}
	if edit != nil {
		edit(&in)
	}
	return in
}

// TestConflicts 验证冲突检查：监听端口按传输层判断、端口转发绑定端口、网卡名、名称重复。
func TestConflicts(t *testing.T) {
	list := []Instance{
		inst("a", "a", func(in *Instance) {
			in.Listeners = "tcp://0.0.0.0:11010\nudp://0.0.0.0:11010"
			in.PortForwards = "tcp://127.0.0.1:2222/10.0.0.1:22"
			in.DevName = "et0"
		}),
	}
	cases := []struct {
		name string
		edit func(*Instance)
		bad  bool
	}{
		{"同名", func(in *Instance) { in.Name = "a" }, true},
		{"TCP 端口相同", func(in *Instance) { in.Listeners = "tcp://0.0.0.0:11010" }, true},
		{"ws 与 tcp 同端口都走 TCP", func(in *Instance) { in.Listeners = "ws://0.0.0.0:11010" }, true},
		{"quic 与 udp 同端口都走 UDP", func(in *Instance) { in.Listeners = "quic://0.0.0.0:11010" }, true},
		{"不同端口", func(in *Instance) { in.Listeners = "tcp://0.0.0.0:11020\nudp://0.0.0.0:11020" }, false},
		{"端口转发绑定相同端口", func(in *Instance) { in.PortForwards = "tcp://127.0.0.1:2222/10.0.0.2:22" }, true},
		{"端口转发 UDP 与 TCP 同号不冲突", func(in *Instance) { in.PortForwards = "udp://127.0.0.1:2222/10.0.0.2:22" }, false},
		{"网卡名相同", func(in *Instance) { in.DevName = "et0" }, true},
		{"网卡名相同但新实例用户态", func(in *Instance) { in.DevName = "et0"; in.NoTun = true }, false},
		{"新实例停用时不检查端口", func(in *Instance) { in.Enable = false; in.Listeners = "tcp://0.0.0.0:11010" }, false},
	}
	for _, c := range cases {
		in := inst("b", "b", c.edit)
		err := checkConflicts(list, &in, false)
		if (err != nil) != c.bad {
			t.Errorf("%s: 错误=%v，期望冲突=%v", c.name, err, c.bad)
		}
	}
	// 修改自身不与自己冲突
	self := list[0]
	if err := checkConflicts(list, &self, false); err != nil {
		t.Errorf("实例不应与自己冲突: %v", err)
	}
	// 启动前检查：只与排在前面且已启用的实例比较
	two := []Instance{list[0], inst("b", "b", func(in *Instance) { in.Listeners = "tcp://0.0.0.0:11010" })}
	if err := checkConflicts(two, &two[0], true); err != nil {
		t.Errorf("排在前面的实例应照常运行: %v", err)
	}
	if err := checkConflicts(two, &two[1], true); err == nil {
		t.Error("排在后面冲突的实例不应启动")
	}
	two[0].Enable = false
	if err := checkConflicts(two, &two[1], true); err != nil {
		t.Errorf("前面的实例停用后不应冲突: %v", err)
	}
}

// TestMaskLine 验证 easytier-core 启动时打印的配置中的密钥被隐藏，其他行原样保留。
func TestMaskLine(t *testing.T) {
	cases := map[string]string{
		`network_secret = "s3cret"`:                            `network_secret = "********"`,
		`  network_secret="abc def"`:                           `  network_secret="********"`,
		`local_private_key = "AAAA=="`:                         `local_private_key = "********"`,
		`credential = "xx"`:                                    `credential = "********"`,
		`network_name = "home"`:                                `network_name = "home"`,
		`listeners = [`:                                        `listeners = [`,
		`INFO new listener added listener=tcp://0.0.0.0:11010`: `INFO new listener added listener=tcp://0.0.0.0:11010`,
	}
	for in, want := range cases {
		if got := maskLine(in); got != want {
			t.Errorf("maskLine(%q) = %q，期望 %q", in, got, want)
		}
	}
	if got := maskLine(`network_secret = "s3cret"`); strings.Contains(got, "s3cret") {
		t.Errorf("密钥未隐藏: %s", got)
	}
}

// 对外监听的 TCP 端口与非回环地址的 TCP 端口转发登记为开放端口；UDP 传输与回环地址不登记
func TestTCPOpenPorts(t *testing.T) {
	o := Options{
		Listeners:    "tcp://0.0.0.0:11010\nudp://0.0.0.0:11010\nws://0.0.0.0:11011\nwss://[::]:11012\nquic://0.0.0.0:11013\nwg://0.0.0.0:11014\ntcp://127.0.0.1:11015\nfaketcp://0.0.0.0:11016",
		PortForwards: "tcp://0.0.0.0:2222/10.126.126.2:22\nudp://0.0.0.0:5353/10.126.126.2:53\ntcp://127.0.0.1:3333/10.126.126.2:22\ntcp://[::]:4444/10.126.126.2:22",
	}
	got := o.tcpOpenPorts()
	slices.Sort(got)
	want := []int{2222, 4444, 11010, 11011, 11012, 11016}
	if !slices.Equal(got, want) {
		t.Fatalf("得到 %v，期望 %v", got, want)
	}
	if got := (&Options{}).tcpOpenPorts(); len(got) != 0 {
		t.Fatalf("没有监听时应为空: %v", got)
	}
}
