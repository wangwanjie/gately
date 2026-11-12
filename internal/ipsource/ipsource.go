// Package ipsource 获取本机公网 IP，支持查询 URL、读取网卡和执行命令三种方式。
package ipsource

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/outbound"
	"github.com/wangwanjie/gately/internal/script"
)

// 获取方式
const (
	TypeURL       = "url"
	TypeInterface = "netInterface"
	TypeCommand   = "command"
)

// Family 表示地址族。
type Family int

// 地址族取值
const (
	IPv4 Family = 4
	IPv6 Family = 6
)

// DefaultIPv4URLs 是默认的 IPv4 查询地址，均为第三方公共服务，用户可在界面修改。
var DefaultIPv4URLs = []string{
	"https://ipv4.ddnspod.com",
	"https://api.ipify.org",
	"https://ipv4.icanhazip.com",
	"https://myip.ipip.net",
}

// DefaultIPv6URLs 是默认的 IPv6 查询地址。
var DefaultIPv6URLs = []string{
	"https://ipv6.ddnspod.com",
	"https://api6.ipify.org",
	"https://ipv6.icanhazip.com",
	"https://v6.ident.me",
}

// retiredURLs 记录已停止服务的查询地址及其替代地址。
// ipw.cn 在 2026-11 已不再解析，域名一旦被他人接手就可能返回伪造的 IP，旧配置里的地址需要换掉。
var retiredURLs = map[string]string{
	"https://4.ipw.cn": "https://ipv4.ddnspod.com",
	"http://4.ipw.cn":  "https://ipv4.ddnspod.com",
	"https://6.ipw.cn": "https://ipv6.ddnspod.com",
	"http://6.ipw.cn":  "https://ipv6.ddnspod.com",
}

// ReplaceRetiredURLs 把已停止服务的查询地址换成替代地址，替代地址已在列表中时直接去掉旧地址。
// 返回新列表以及是否有改动。
func ReplaceRetiredURLs(urls []string) ([]string, bool) {
	present := make(map[string]bool, len(urls))
	for _, u := range urls {
		present[strings.TrimSpace(u)] = true
	}
	changed := false
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		repl, ok := retiredURLs[strings.TrimRight(strings.TrimSpace(u), "/")]
		if !ok {
			out = append(out, u)
			continue
		}
		changed = true
		if present[repl] {
			continue
		}
		present[repl] = true
		out = append(out, repl)
	}
	return out, changed
}

// Source 描述一种获取方式。
type Source struct {
	Enable bool   `json:"enable"`
	Type   string `json:"type"`
	// URLs 按顺序尝试，直到成功
	URLs []string `json:"urls"`
	// Interface 为网卡名，Type 为 netInterface 时使用
	Interface string `json:"interface"`
	// Regex 用于从多个候选地址中筛选，留空取第一个合适的地址
	Regex string `json:"regex"`
	// Command 通过 sh -c（Windows 为 cmd /C）执行，从输出中提取 IP
	Command string `json:"command"`
}

// Validate 校验配置。
func (s *Source) Validate() error {
	if !s.Enable {
		return nil
	}
	switch s.Type {
	case TypeURL:
		var urls []string
		for _, u := range s.URLs {
			if u = strings.TrimSpace(u); u != "" {
				urls = append(urls, u)
			}
		}
		if len(urls) == 0 {
			return errors.New("至少填写一个查询地址")
		}
		s.URLs = urls
	case TypeInterface:
		if strings.TrimSpace(s.Interface) == "" {
			return errors.New("请选择网卡")
		}
	case TypeCommand:
		if strings.TrimSpace(s.Command) == "" {
			return errors.New("命令不能为空")
		}
	default:
		return fmt.Errorf("未知的获取方式 %q", s.Type)
	}
	if s.Regex != "" {
		if _, err := regexp.Compile(s.Regex); err != nil {
			return fmt.Errorf("筛选正则无效: %w", err)
		}
	}
	return nil
}

// Get 按配置获取一个地址。
func Get(ctx context.Context, s Source, fam Family) (netip.Addr, error) {
	var (
		cands []netip.Addr
		err   error
	)
	switch s.Type {
	case TypeURL:
		return fromURLs(ctx, s.URLs, fam, s.Regex)
	case TypeInterface:
		cands, err = fromInterface(s.Interface, fam)
	case TypeCommand:
		cands, err = fromCommand(ctx, s.Command, fam)
	default:
		return netip.Addr{}, fmt.Errorf("未知的获取方式 %q", s.Type)
	}
	if err != nil {
		return netip.Addr{}, err
	}
	return pick(cands, fam, s.Regex)
}

var (
	reIPv4 = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	// 宽松匹配候选 IPv6 片段，再交给 netip 严格解析
	reIPv6 = regexp.MustCompile(`[0-9a-fA-F:]*:[0-9a-fA-F:.]+`)
)

// Extract 从任意文本中提取指定地址族的地址，保持出现顺序并去重。
func Extract(text string, fam Family) []netip.Addr {
	re := reIPv4
	if fam == IPv6 {
		re = reIPv6
	}
	seen := map[netip.Addr]bool{}
	var out []netip.Addr
	for _, m := range re.FindAllString(text, -1) {
		a, err := netip.ParseAddr(strings.Trim(m, ":"))
		if err != nil {
			a, err = netip.ParseAddr(m)
			if err != nil {
				continue
			}
		}
		if fam == IPv4 && !a.Is4() {
			continue
		}
		if fam == IPv6 && (!a.Is6() || a.Is4In6()) {
			continue
		}
		if !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	return out
}

// IsPublic 判断地址是否可用于公网解析（排除回环、链路本地、私有、ULA 等）。
func IsPublic(a netip.Addr) bool {
	if !a.IsValid() || a.IsLoopback() || a.IsLinkLocalUnicast() || a.IsMulticast() || a.IsUnspecified() || a.IsPrivate() {
		return false
	}
	// 100.64.0.0/10 运营商级 NAT
	if a.Is4() && netip.MustParsePrefix("100.64.0.0/10").Contains(a) {
		return false
	}
	return a.IsGlobalUnicast()
}

// pick 从候选中按正则与公网规则选出一个。
// 正则非空时只按正则筛选（允许用户刻意选择内网地址）；否则优先公网地址，没有时报错。
func pick(cands []netip.Addr, fam Family, pattern string) (netip.Addr, error) {
	var filtered []netip.Addr
	for _, a := range cands {
		if (fam == IPv4 && a.Is4()) || (fam == IPv6 && a.Is6() && !a.Is4In6()) {
			filtered = append(filtered, a)
		}
	}
	if pattern != "" {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return netip.Addr{}, err
		}
		for _, a := range filtered {
			if re.MatchString(a.String()) {
				return a, nil
			}
		}
		return netip.Addr{}, fmt.Errorf("没有符合正则 %q 的地址（候选: %v）", pattern, filtered)
	}
	for _, a := range filtered {
		if IsPublic(a) {
			return a, nil
		}
	}
	return netip.Addr{}, fmt.Errorf("未找到公网 IPv%d 地址（候选: %v）", fam, filtered)
}

// httpClientFor 返回强制使用指定地址族拨号的客户端，确保查询到的是对应协议的出口地址。
func httpClientFor(fam Family) *http.Client {
	network := "tcp4"
	if fam == IPv6 {
		network = "tcp6"
	}
	// 拨号层校验解析后的真实 IP，全局开关开启时拒绝内网与链路本地地址
	dialer := outbound.NewDialer(10 * time.Second)
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		},
		TLSHandshakeTimeout: 10 * time.Second,
	}
	return &http.Client{Transport: tr, Timeout: 15 * time.Second}
}

// clientForTest 允许测试替换 HTTP 客户端。
var clientForTest func(Family) *http.Client

func fromURLs(ctx context.Context, urls []string, fam Family, pattern string) (netip.Addr, error) {
	client := httpClientFor(fam)
	if clientForTest != nil {
		client = clientForTest(fam)
	}
	var errs []string
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			errs = append(errs, u+": "+err.Error())
			continue
		}
		// 部分查询服务对浏览器 UA 返回网页，使用 curl UA 获得纯文本
		req.Header.Set("User-Agent", "curl/8.0 (Gately)")
		resp, err := client.Do(req)
		if err != nil {
			errs = append(errs, u+": "+err.Error())
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
		addr, err := pick(Extract(string(body), fam), fam, pattern)
		if err != nil {
			errs = append(errs, u+": "+err.Error())
			continue
		}
		return addr, nil
	}
	return netip.Addr{}, fmt.Errorf("所有查询地址均失败: %s", strings.Join(errs, "; "))
}

// interfaceAddrs 允许测试替换网卡地址来源。
var interfaceAddrs = func(name string) ([]net.Addr, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, err
	}
	return iface.Addrs()
}

func fromInterface(name string, fam Family) ([]netip.Addr, error) {
	addrs, err := interfaceAddrs(name)
	if err != nil {
		return nil, fmt.Errorf("读取网卡 %s 失败: %w", name, err)
	}
	var out []netip.Addr
	for _, a := range addrs {
		var ip net.IP
		switch v := a.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if addr, ok := netip.AddrFromSlice(ip); ok {
			addr = addr.Unmap()
			if (fam == IPv4 && addr.Is4()) || (fam == IPv6 && addr.Is6()) {
				out = append(out, addr)
			}
		}
	}
	return out, nil
}

func fromCommand(ctx context.Context, command string, fam Family) ([]netip.Addr, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	script.Prepare(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("执行命令失败: %w", err)
	}
	return Extract(string(out), fam), nil
}

// InterfaceInfo 是网卡及其地址，供界面选择。
type InterfaceInfo struct {
	Name  string   `json:"name"`
	Addrs []string `json:"addrs"`
}

// ListInterfaces 列出处于启用状态的网卡。
func ListInterfaces() ([]InterfaceInfo, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []InterfaceInfo
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		info := InterfaceInfo{Name: iface.Name, Addrs: []string{}}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			info.Addrs = append(info.Addrs, a.String())
		}
		out = append(out, info)
	}
	return out, nil
}
