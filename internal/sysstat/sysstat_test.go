package sysstat

import (
	"encoding/json"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestSnapshot 确认采集不会出错，且基本字段有值。
func TestSnapshot(t *testing.T) {
	c := New(t.TempDir())
	s := c.Snapshot()
	if s.CPU.Cores <= 0 || s.Process.PID <= 0 {
		t.Fatalf("基本字段为空: %+v", s)
	}
	// 紧接着再采会与上一次基准相隔仅几毫秒，/proc/stat 的计数可能还没变化，使用率为空；等够最小采样间隔
	time.Sleep(minInterval)
	s = c.Snapshot()
	// Linux 上 /proc 可读，内存、使用率与磁盘都应有值（CI 在 Linux 上运行）
	if runtime.GOOS == "linux" {
		if s.Memory == nil || s.CPU.Usage == nil || s.Process.CPU == nil || s.Disk == nil || s.Process.RSS == 0 {
			t.Fatalf("Linux 采集字段缺失: %+v", s)
		}
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	t.Log(string(b))
}

func TestIsVirtual(t *testing.T) {
	cases := map[string]bool{
		// OpenWrt 的局域网、广域网网桥要展示
		"br-lan": false, "br-wan": false, "br-guest": false, "eth0": false, "tailscale0": false,
		// Docker 自定义网络网桥与常见虚拟网卡隐藏
		"br-1a2b3c4d5e6f": true, "docker0": true, "veth12ab": true, "virbr0": true,
		// 长度或字符不符合 Docker 规则的 br- 网卡按真实网卡处理
		"br-1a2b3c4d5e6": false, "br-1A2B3C4D5E6F": false,
	}
	for name, want := range cases {
		if got := isVirtual(name); got != want {
			t.Errorf("isVirtual(%q) = %v，期望 %v", name, got, want)
		}
	}
}

// 宿主机的路由表：默认路由走 eth0，docker0 只有网段路由；IPv6 默认路由走 eth0，lo 上有内核自带的不可达默认路由
const hostRouteV4 = `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
eth0	00000000	0101A8C0	0003	0	0	100	00000000	0	0	0
eth0	0001A8C0	00000000	0001	0	0	100	00FFFFFF	0	0	0
docker0	000011AC	00000000	0001	0	0	0	0000FFFF	0	0	0
`

const hostRouteV6 = `fe800000000000000000000000000000 40 00000000000000000000000000000000 00 00000000000000000000000000000000 00000100 00000001 00000000 00000001     eth0
00000000000000000000000000000000 00 00000000000000000000000000000000 00 fe800000000000000000000000000001 00000400 00000001 00000000 00000003     eth0
00000000000000000000000000000000 00 00000000000000000000000000000000 00 00000000000000000000000000000000 ffffffff 00000001 00000000 00200200       lo
`

// MikroTik RouterOS 容器内的路由表：唯一的网卡 veth33，默认路由经 10.10.10.1
const routerosRouteV4 = `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
veth33	00000000	010A0A0A	0003	0	0	0	00000000	0	0	0
veth33	000A0A0A	00000000	0001	0	0	0	00FFFFFF	0	0	0
`

// Docker 容器内的路由表：默认路由经 eth0
const dockerRouteV4 = `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
eth0	00000000	010011AC	0003	0	0	0	00000000	0	0	0
eth0	000011AC	00000000	0001	0	0	0	0000FFFF	0	0	0
`

func TestParseDefaultRoutes(t *testing.T) {
	cases := []struct {
		name   string
		v4, v6 string
		want   []string
	}{
		{"宿主机", hostRouteV4, hostRouteV6, []string{"eth0"}},
		{"RouterOS 容器", routerosRouteV4, "", []string{"veth33"}},
		{"Docker 容器", dockerRouteV4, "", []string{"eth0"}},
		// 默认路由未启用（Flags 不含 RTF_UP）或为拒绝路由时不算
		{"未启用与拒绝路由", "eth1\t00000000\t0101A8C0\t0002\t0\t0\t0\t00000000\t0\t0\t0\neth2\t00000000\t00000000\t0201\t0\t0\t0\t00000000\t0\t0\t0\n", "", nil},
		{"读取失败", "", "", nil},
	}
	for _, c := range cases {
		got := parseDefaultRoutes(c.v4, c.v6)
		if len(got) != len(c.want) {
			t.Errorf("%s：得到 %v，期望 %v", c.name, got, c.want)
			continue
		}
		for _, n := range c.want {
			if !got[n] {
				t.Errorf("%s：缺少 %s，得到 %v", c.name, n, got)
			}
		}
	}
}

func mustCIDR(t *testing.T, s string) *net.IPNet {
	t.Helper()
	ip, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatal(err)
	}
	n.IP = ip
	return n
}

func TestBuildInterfaces(t *testing.T) {
	up := net.FlagUp | net.FlagBroadcast | net.FlagMulticast
	lo := nicInfo{name: "lo", flags: net.FlagUp | net.FlagLoopback, mtu: 65536, addrs: []*net.IPNet{mustCIDR(t, "127.0.0.1/8"), mustCIDR(t, "::1/128")}}
	now := time.Now()
	prev := rawSample{at: now.Add(-2 * time.Second), ifBytes: map[string][2]uint64{"eth0": {1000, 2000}, "veth33": {1000, 2000}, "veth12ab": {0, 0}}}
	cur := rawSample{at: now, ifBytes: map[string][2]uint64{"eth0": {3000, 6000}, "veth33": {5000, 4000}, "veth12ab": {800, 800}}}

	cases := []struct {
		name string
		nics []nicInfo
		v4   string
		v6   string
		want []string
	}{
		{
			// 宿主机：docker0 有地址但不承载默认路由；veth 对端只有链路本地地址，两者都排除
			name: "宿主机 docker0 与 veth",
			nics: []nicInfo{
				lo,
				{name: "eth0", flags: up, addrs: []*net.IPNet{mustCIDR(t, "192.168.1.10/24"), mustCIDR(t, "fe80::1/64")}},
				{name: "docker0", flags: up, addrs: []*net.IPNet{mustCIDR(t, "172.17.0.1/16")}},
				{name: "veth12ab", flags: up, addrs: []*net.IPNet{mustCIDR(t, "fe80::2/64")}},
			},
			v4: hostRouteV4, v6: hostRouteV6,
			want: []string{"eth0"},
		},
		{
			// 宿主机上手工配了地址的 veth（如 netns 实验），不承载默认路由，仍排除以免重复计数
			name: "宿主机带地址的 veth",
			nics: []nicInfo{
				{name: "eth0", flags: up, addrs: []*net.IPNet{mustCIDR(t, "192.168.1.10/24")}},
				{name: "veth12ab", flags: up, addrs: []*net.IPNet{mustCIDR(t, "10.200.0.1/24")}},
			},
			v4: hostRouteV4, v6: hostRouteV6,
			want: []string{"eth0"},
		},
		{
			name: "Docker 容器内 eth0",
			nics: []nicInfo{lo, {name: "eth0", flags: up, addrs: []*net.IPNet{mustCIDR(t, "172.17.0.2/16")}}},
			v4:   dockerRouteV4,
			want: []string{"eth0"},
		},
		{
			// RouterOS 容器：veth33 承载默认路由要保留；veth33_cuv6 没有可路由地址
			name: "RouterOS 容器内 veth33",
			nics: []nicInfo{
				lo,
				{name: "veth33", flags: up, addrs: []*net.IPNet{mustCIDR(t, "10.10.10.33/24"), mustCIDR(t, "fe80::788c:70ff:fe04:9dad/64")}},
				{name: "veth33_cuv6", flags: up, addrs: []*net.IPNet{mustCIDR(t, "fe80::58ca:c5ff:fe17:9a08/64")}},
			},
			v4:   routerosRouteV4,
			want: []string{"veth33"},
		},
		{
			// 读不到路由表或容器没有默认路由时，只剩虚拟网卡也要展示
			name: "无默认路由时退而展示 veth",
			nics: []nicInfo{lo, {name: "veth33", flags: up, addrs: []*net.IPNet{mustCIDR(t, "10.10.10.33/24")}}},
			want: []string{"veth33"},
		},
		{
			name: "未启用的网卡排除",
			nics: []nicInfo{{name: "eth0", flags: up, addrs: []*net.IPNet{mustCIDR(t, "192.168.1.10/24")}}, {name: "eth1", flags: net.FlagBroadcast, addrs: []*net.IPNet{mustCIDR(t, "192.168.2.10/24")}}},
			want: []string{"eth0"},
		},
	}
	for _, c := range cases {
		got := buildInterfaces(c.nics, parseDefaultRoutes(c.v4, c.v6), cur, prev)
		names := []string{}
		for _, it := range got {
			names = append(names, it.Name)
		}
		if strings.Join(names, ",") != strings.Join(c.want, ",") {
			t.Errorf("%s：得到 %v，期望 %v", c.name, names, c.want)
		}
	}

	// 容器内 veth33 的速率按两次计数差计算：收 (5000-1000)/2，发 (4000-2000)/2
	got := buildInterfaces([]nicInfo{{name: "veth33", flags: up, addrs: []*net.IPNet{mustCIDR(t, "10.10.10.33/24")}}}, parseDefaultRoutes(routerosRouteV4, ""), cur, prev)
	if len(got) != 1 || got[0].RxRate == nil || *got[0].RxRate != 2000 || *got[0].TxRate != 1000 {
		t.Fatalf("veth33 速率不对: %+v", got)
	}
}
