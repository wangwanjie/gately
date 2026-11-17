package nettools

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/demo"
	"github.com/wangwanjie/gately/internal/httpx"
)

// maxScanHosts 限制一次扫描的地址数量（/22）。
const maxScanHosts = 1024

// scanPorts 是用于判断主机存活的常见端口。
var scanPorts = []int{80, 443, 22, 445, 139, 3389, 8080, 62078, 5000, 53}

// Host 是局域网扫描发现的设备。
type Host struct {
	IP       string `json:"ip"`
	MAC      string `json:"mac,omitempty"`
	Hostname string `json:"hostname,omitempty"`
	Ports    []int  `json:"ports,omitempty"`
	RTTms    int64  `json:"rttMs"`
}

func scanTool(ctx context.Context, m *Module, req Request, out output) error {
	prefix, err := scanPrefix(req.CIDR)
	if err != nil {
		return err
	}
	var targets []netip.Addr
	for a := prefix.Masked().Addr(); prefix.Contains(a); a = a.Next() {
		targets = append(targets, a)
		if len(targets) > maxScanHosts+2 {
			return fmt.Errorf("网段过大，一次最多扫描 %d 个地址（相当于 /22）", maxScanHosts)
		}
	}
	// 去掉网络地址与广播地址
	if prefix.Addr().Is4() && prefix.Bits() < 31 && len(targets) > 2 {
		targets = targets[1 : len(targets)-1]
	}
	out.Line("扫描 %s，共 %d 个地址", prefix, len(targets))

	var mu sync.Mutex
	hosts := map[string]*Host{}
	sem := make(chan struct{}, 200)
	var wg sync.WaitGroup
	for _, ip := range targets {
		if m.denied(ip) {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if h := probeHost(ctx, ip); h != nil {
				mu.Lock()
				hosts[h.IP] = h
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// 连接尝试已触发 ARP 解析，此时读取邻居表获取 MAC
	arp := readARP()
	for ip, mac := range arp {
		if h, ok := hosts[ip]; ok {
			h.MAC = mac
		} else if a, err := netip.ParseAddr(ip); err == nil && prefix.Contains(a) && !mac00(mac) {
			// 邻居表里有记录但没有开放常见端口的主机，同样视为在线
			hosts[ip] = &Host{IP: ip, MAC: mac}
		}
	}
	list := make([]*Host, 0, len(hosts))
	for _, h := range hosts {
		list = append(list, h)
	}
	sort.Slice(list, func(i, j int) bool {
		a, _ := netip.ParseAddr(list[i].IP)
		b, _ := netip.ParseAddr(list[j].IP)
		return a.Less(b)
	})
	// 反向解析主机名
	var rg sync.WaitGroup
	for _, h := range list {
		rg.Add(1)
		go func() {
			defer rg.Done()
			rctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
			defer cancel()
			if names, err := net.DefaultResolver.LookupAddr(rctx, h.IP); err == nil && len(names) > 0 {
				h.Hostname = strings.TrimSuffix(names[0], ".")
			}
		}()
	}
	rg.Wait()
	for _, h := range list {
		out.Line("%-15s  %-17s  %s  %v", h.IP, orDash(h.MAC), orDash(h.Hostname), h.Ports)
	}
	out.Line("--- 发现 %d 台在线设备", len(list))
	out.SetResult(list)
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func mac00(mac string) bool {
	return mac == "" || mac == "00:00:00:00:00:00" || strings.EqualFold(mac, "ff:ff:ff:ff:ff:ff") || mac == "(incomplete)"
}

// probeHost 依次尝试常见端口。连接成功或被拒绝（对端回复 RST）都说明主机在线。
func probeHost(ctx context.Context, ip netip.Addr) *Host {
	var h *Host
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, p := range scanPorts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			d := net.Dialer{Timeout: 500 * time.Millisecond}
			c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), strconv.Itoa(p)))
			alive, open := false, false
			if err == nil {
				c.Close()
				alive, open = true, true
			} else if strings.Contains(err.Error(), "refused") {
				alive = true
			}
			if !alive {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if h == nil {
				h = &Host{IP: ip.String(), RTTms: time.Since(start).Milliseconds()}
			}
			if open {
				h.Ports = append(h.Ports, p)
			}
		}()
	}
	wg.Wait()
	if h != nil {
		sort.Ints(h.Ports)
	}
	return h
}

// scanPrefix 解析扫描网段：留空时取本机第一个私有 IPv4 网卡所在网段；只允许私有网段。
func scanPrefix(cidr string) (netip.Prefix, error) {
	cidr = strings.TrimSpace(cidr)
	if cidr == "" {
		nets := localNetworks()
		if len(nets) == 0 {
			return netip.Prefix{}, errors.New("没有找到可扫描的本机网段，请手动填写 CIDR")
		}
		cidr = nets[0]
	}
	p, err := netip.ParsePrefix(cidr)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("网段 %q 无效", cidr)
	}
	a := p.Addr().Unmap()
	if !a.Is4() {
		return netip.Prefix{}, errors.New("目前只支持扫描 IPv4 网段")
	}
	if !a.IsPrivate() {
		return netip.Prefix{}, errors.New("出于安全考虑，只允许扫描私有网段（10/8、172.16/12、192.168/16）")
	}
	return netip.PrefixFrom(a, p.Bits()).Masked(), nil
}

// localNetworks 返回本机各私有 IPv4 网卡所在的网段（过大的网段收缩为 /24）。
func localNetworks() []string {
	var out []string
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil {
				continue
			}
			ip, _ := netip.AddrFromSlice(ipn.IP.To4())
			if !ip.IsPrivate() {
				continue
			}
			ones, _ := ipn.Mask.Size()
			if ones < 24 {
				ones = 24
			}
			out = append(out, netip.PrefixFrom(ip, ones).Masked().String())
		}
	}
	return out
}

func (m *Module) handleNetworks(w http.ResponseWriter, _ *http.Request) {
	// 演示实例对公众开放：返回文档保留地址的示例网段，不暴露宿主机所在的内网
	if demo.Enabled() {
		httpx.OK(w, []string{"192.0.2.0/24"})
		return
	}
	nets := localNetworks()
	if nets == nil {
		nets = []string{}
	}
	httpx.OK(w, nets)
}

var arpLine = regexp.MustCompile(`(?i)\(?(\d+\.\d+\.\d+\.\d+)\)?\s+(?:at\s+)?([0-9a-f]{1,2}(?:[:-][0-9a-f]{1,2}){5})`)

// readARP 读取系统邻居表，返回 IP→MAC。
func readARP() map[string]string {
	out := map[string]string{}
	if f, err := os.Open("/proc/net/arp"); err == nil {
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Scan() // 表头
		for sc.Scan() {
			fs := strings.Fields(sc.Text())
			// IP address, HW type, Flags, HW address, Mask, Device
			if len(fs) >= 4 && fs[2] != "0x0" && !mac00(fs[3]) {
				out[fs[0]] = strings.ToLower(fs[3])
			}
		}
		return out
	}
	if !has("arp") {
		return out
	}
	b, err := exec.Command("arp", "-an").Output()
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(b), "\n") {
		if m := arpLine.FindStringSubmatch(line); m != nil {
			if mac := normalizeMAC(m[2]); !mac00(mac) {
				out[m[1]] = mac
			}
		}
	}
	return out
}

// normalizeMAC 补齐 macOS 省略的前导零，统一为小写冒号分隔。
func normalizeMAC(s string) string {
	parts := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return r == ':' || r == '-' })
	for i, p := range parts {
		if len(p) == 1 {
			parts[i] = "0" + p
		}
	}
	return strings.Join(parts, ":")
}
