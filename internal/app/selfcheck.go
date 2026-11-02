package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/ipsource"
	"github.com/wangwanjie/gately/internal/module"
)

// cgnatPrefix 是运营商级 NAT 使用的地址段。
var cgnatPrefix = netip.MustParsePrefix("100.64.0.0/10")

// handleSelfCheck 执行网络与配置自检并返回结果。该操作会访问配置的公网 IP 查询服务，因此只在用户主动触发时运行。
func (a *App) handleSelfCheck(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	httpx.OK(w, map[string]any{"results": a.runSelfCheck(ctx)})
}

func (a *App) runSelfCheck(ctx context.Context) []module.CheckResult {
	var (
		mu  sync.Mutex
		out []module.CheckResult
		wg  sync.WaitGroup
	)
	add := func(rs ...module.CheckResult) {
		mu.Lock()
		out = append(out, rs...)
		mu.Unlock()
	}

	// 公网地址（IPv4 / IPv6）并行查询，后续检查依赖它们
	var env module.CheckEnv
	wg.Add(2)
	go func() {
		defer wg.Done()
		ip, err := ipsource.Get(ctx, ipsource.Source{Enable: true, Type: ipsource.TypeURL, URLs: ipsource.DefaultIPv4URLs}, ipsource.IPv4)
		if err != nil {
			add(module.CheckResult{ID: "net.publicIPv4", Status: module.CheckFail, Params: map[string]string{"error": err.Error()}})
			return
		}
		env.PublicIPv4 = ip.String()
		add(module.CheckResult{ID: "net.publicIPv4", Status: module.CheckOK, Params: map[string]string{"ip": ip.String()}})
	}()
	go func() {
		defer wg.Done()
		ip, err := ipsource.Get(ctx, ipsource.Source{Enable: true, Type: ipsource.TypeURL, URLs: ipsource.DefaultIPv6URLs}, ipsource.IPv6)
		if err != nil {
			add(module.CheckResult{ID: "net.publicIPv6", Status: module.CheckInfo})
			return
		}
		env.PublicIPv6 = ip.String()
		add(module.CheckResult{ID: "net.publicIPv6", Status: module.CheckOK, Params: map[string]string{"ip": ip.String()}})
	}()
	wg.Wait()

	// 本机网卡与 NAT 情况
	add(natChecks(env.PublicIPv4, env.PublicIPv6)...)
	add(clockCheck(ctx))
	add(portCheck(80), portCheck(443))

	// 各模块的专项检查
	// 已关闭的模块不参与检查
	for _, e := range a.modules {
		if sc, ok := e.mod.(module.SelfChecker); ok && e.enabled {
			add(sc.SelfCheck(ctx, env)...)
		}
	}
	// 其余模块：把摘要里的警告与错误提示出来
	for _, e := range a.modules {
		sm, ok := e.mod.(module.Summarizer)
		if !ok || !e.enabled {
			continue
		}
		for _, it := range sm.Summary() {
			if it.Level == "warn" || it.Level == "error" {
				st := module.CheckWarn
				if it.Level == "error" {
					st = module.CheckFail
				}
				add(module.CheckResult{ID: "module.summary", Status: st, Params: map[string]string{"module": e.mod.Title(), "label": it.Label, "value": it.Value}})
			}
		}
	}
	return out
}

// tailscaleULA 是 Tailscale 分配的 IPv6 地址段；同一网卡上出现它，说明该网卡是 Tailscale 虚拟网卡。
var tailscaleULA = netip.MustParsePrefix("fd7a:115c:a1e0::/48")

// ifaceAddr 是一张网卡的名称与地址。
type ifaceAddr struct {
	name  string
	addrs []netip.Addr
}

// listIfaceAddrs 列出本机各网卡的地址，测试可替换。
var listIfaceAddrs = func() []ifaceAddr {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []ifaceAddr
	for _, ifc := range ifaces {
		addrs, _ := ifc.Addrs()
		ia := ifaceAddr{name: ifc.Name}
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				if ip, ok := netip.AddrFromSlice(ipn.IP); ok {
					ia.addrs = append(ia.addrs, ip.Unmap())
				}
			}
		}
		out = append(out, ia)
	}
	return out
}

// isOverlayIface 判断网卡是否为 Tailscale、NetBird（Linux 与 Windows 下为 wt0）等组网虚拟网卡：这类网卡的 100.64.0.0/10 地址不代表运营商 NAT。
func isOverlayIface(ia ifaceAddr) bool {
	if strings.HasPrefix(ia.name, "tailscale") || strings.HasPrefix(ia.name, "ts") || strings.HasPrefix(ia.name, "utun") || strings.HasPrefix(ia.name, "zt") || strings.HasPrefix(ia.name, "wt") {
		return true
	}
	for _, a := range ia.addrs {
		if tailscaleULA.Contains(a) {
			return true
		}
	}
	return false
}

// natChecks 判断本机是否直接持有公网 IPv4 / IPv6，以及是否位于运营商级 NAT 之后。
func natChecks(publicIPv4, publicIPv6 string) []module.CheckResult {
	var direct4, direct6 bool
	cgnat := ""
	for _, ia := range listIfaceAddrs() {
		overlay := isOverlayIface(ia)
		for _, ip := range ia.addrs {
			switch ip.String() {
			case publicIPv4:
				direct4 = publicIPv4 != ""
			case publicIPv6:
				direct6 = publicIPv6 != ""
			}
			if ip.Is4() && cgnatPrefix.Contains(ip) && !overlay {
				cgnat = ip.String()
			}
		}
	}
	var out []module.CheckResult
	if direct6 {
		out = append(out, module.CheckResult{ID: "net.direct6", Status: module.CheckOK, Params: map[string]string{"ip": publicIPv6}})
	}
	if publicIPv4 == "" {
		return out
	}
	switch {
	case direct4:
		out = append(out, module.CheckResult{ID: "net.direct", Status: module.CheckOK, Params: map[string]string{"ip": publicIPv4}})
	case cgnat != "":
		out = append(out, module.CheckResult{ID: "net.behindNat", Status: module.CheckInfo}, module.CheckResult{ID: "net.cgnat", Status: module.CheckWarn, Params: map[string]string{"ip": cgnat}})
	default:
		out = append(out, module.CheckResult{ID: "net.behindNat", Status: module.CheckInfo, Params: map[string]string{"ip": publicIPv4}})
	}
	return out
}

// clockCheck 用 HTTP 响应的 Date 头估算本机时钟偏差；偏差过大会让两步验证码、证书校验失败。
// 依次尝试各个公网 IP 查询地址，取第一个带有效 Date 头的响应（不论状态码），全部失败时只作提示。
func clockCheck(ctx context.Context) module.CheckResult {
	client := &http.Client{Timeout: 8 * time.Second}
	for _, u := range append(append([]string{}, ipsource.DefaultIPv4URLs...), ipsource.DefaultIPv6URLs...) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			continue
		}
		start := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()
		t, err := http.ParseTime(resp.Header.Get("Date"))
		if err != nil {
			continue
		}
		// Date 头精度为秒，取请求往返的中点作为本机时间
		local := start.Add(time.Since(start) / 2)
		skew := int(local.Sub(t).Abs().Seconds())
		params := map[string]string{"seconds": strconv.Itoa(skew)}
		if skew > 60 {
			return module.CheckResult{ID: "time.clock", Status: module.CheckWarn, Params: params}
		}
		return module.CheckResult{ID: "time.clock", Status: module.CheckOK, Params: params}
	}
	return module.CheckResult{ID: "time.clock", Status: module.CheckInfo}
}

// portCheck 检查本机端口能否监听：空闲、已被占用，或没有权限。
func portCheck(port int) module.CheckResult {
	id := "port." + strconv.Itoa(port)
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err == nil {
		ln.Close()
		return module.CheckResult{ID: id, Status: module.CheckOK}
	}
	switch {
	case errors.Is(err, syscall.EADDRINUSE):
		return module.CheckResult{ID: id, Status: module.CheckInfo, Params: map[string]string{"state": "inuse"}}
	case errors.Is(err, syscall.EACCES):
		return module.CheckResult{ID: id, Status: module.CheckWarn, Params: map[string]string{"state": "denied"}}
	}
	return module.CheckResult{ID: id, Status: module.CheckWarn, Params: map[string]string{"state": "error", "error": err.Error()}}
}
