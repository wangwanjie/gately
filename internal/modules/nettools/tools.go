package nettools

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"

	"github.com/wangwanjie/gately/internal/outbound"
	"github.com/wangwanjie/gately/internal/resolver"
)

// ---- Ping ----

func pingTool(ctx context.Context, m *Module, req Request, out output) error {
	addrs, err := m.resolve(ctx, req.Target)
	if err != nil {
		return err
	}
	target := addrs[0]
	count := req.Count
	if count <= 0 {
		count = 4
	}
	if count > 20 {
		count = 20
	}
	conn, v6, err := listenICMP(target)
	if err != nil {
		return fmt.Errorf("无法创建 ICMP 套接字（%v）。容器内需要 NET_RAW 权限或设置 net.ipv4.ping_group_range；也可改用 TCP 端口探测", err)
	}
	defer conn.Close()
	out.Line("PING %s (%s)", req.Target, target)

	var sent, recv int
	var minRTT, maxRTT, sum time.Duration
	pid := os.Getpid() & 0xffff
	for seq := 1; seq <= count; seq++ {
		if ctx.Err() != nil {
			break
		}
		sent++
		rtt, from, err := echoOnce(ctx, conn, target, v6, pid, seq, 2*time.Second)
		if err != nil {
			out.Line("seq=%d 请求超时", seq)
		} else {
			recv++
			sum += rtt
			if recv == 1 || rtt < minRTT {
				minRTT = rtt
			}
			if rtt > maxRTT {
				maxRTT = rtt
			}
			out.Line("来自 %s: seq=%d 时间=%.2f ms", from, seq, float64(rtt.Microseconds())/1000)
		}
		if seq < count {
			select {
			case <-ctx.Done():
			case <-time.After(700 * time.Millisecond):
			}
		}
	}
	loss := 0.0
	if sent > 0 {
		loss = float64(sent-recv) / float64(sent) * 100
	}
	out.Line("--- 已发送 %d，已接收 %d，丢包率 %.0f%%", sent, recv, loss)
	if recv > 0 {
		out.Line("往返时间 最小/平均/最大 = %.2f/%.2f/%.2f ms",
			float64(minRTT.Microseconds())/1000, float64((sum/time.Duration(recv)).Microseconds())/1000, float64(maxRTT.Microseconds())/1000)
	}
	if recv == 0 {
		return errors.New("目标无响应")
	}
	return nil
}

// listenICMP 先尝试无特权的 UDP 型 ICMP 套接字，失败再尝试原始套接字。
func listenICMP(target netip.Addr) (*icmp.PacketConn, bool, error) {
	v6 := target.Is6()
	nets := []string{"udp4", "ip4:icmp"}
	listen := "0.0.0.0"
	if v6 {
		nets = []string{"udp6", "ip6:ipv6-icmp"}
		listen = "::"
	}
	var firstErr error
	for _, n := range nets {
		c, err := icmp.ListenPacket(n, listen)
		if err == nil {
			return c, v6, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return nil, v6, firstErr
}

// echoOnce 发送一个回显请求并等待应答。
func echoOnce(ctx context.Context, conn *icmp.PacketConn, target netip.Addr, v6 bool, id, seq int, timeout time.Duration) (time.Duration, string, error) {
	var typ icmp.Type = ipv4.ICMPTypeEcho
	proto := 1
	if v6 {
		typ, proto = ipv6.ICMPTypeEchoRequest, 58
	}
	msg := icmp.Message{Type: typ, Code: 0, Body: &icmp.Echo{ID: id, Seq: seq, Data: []byte("gately-ping")}}
	b, err := msg.Marshal(nil)
	if err != nil {
		return 0, "", err
	}
	// 原始套接字需要 IPAddr，UDP 型需要 UDPAddr；通过本地地址类型区分
	var dst net.Addr = &net.UDPAddr{IP: target.AsSlice()}
	if _, ok := conn.LocalAddr().(*net.IPAddr); ok {
		dst = &net.IPAddr{IP: target.AsSlice()}
	}
	start := time.Now()
	if _, err := conn.WriteTo(b, dst); err != nil {
		return 0, "", err
	}
	deadline := start.Add(timeout)
	buf := make([]byte, 1500)
	for {
		if ctx.Err() != nil {
			return 0, "", ctx.Err()
		}
		_ = conn.SetReadDeadline(deadline)
		n, peer, err := conn.ReadFrom(buf)
		if err != nil {
			return 0, "", err
		}
		rm, err := icmp.ParseMessage(proto, buf[:n])
		if err != nil {
			continue
		}
		if rm.Type != ipv4.ICMPTypeEchoReply && rm.Type != ipv6.ICMPTypeEchoReply {
			continue
		}
		echo, ok := rm.Body.(*icmp.Echo)
		if !ok || echo.Seq != seq {
			continue
		}
		host := peer.String()
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		return time.Since(start), host, nil
	}
}

// ---- Traceroute ----

func tracerouteTool(ctx context.Context, m *Module, req Request, out output) error {
	addrs, err := m.resolve(ctx, req.Target)
	if err != nil {
		return err
	}
	// 直接传 IP 作为参数，避免主机名被当作命令行选项
	ip := addrs[0].String()
	var name string
	var args []string
	switch {
	case runtime.GOOS == "windows":
		name, args = "tracert", []string{"-d", "-h", "20", "-w", "2000", ip}
	case has("traceroute"):
		name, args = "traceroute", []string{"-n", "-w", "2", "-q", "1", "-m", "20", ip}
	case has("tracepath"):
		name, args = "tracepath", []string{"-n", ip}
	default:
		return errors.New("系统中没有 traceroute / tracepath 命令（Docker 镜像可安装 traceroute 包）")
	}
	out.Line("$ %s %s", name, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, name, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		out.Line("%s", sc.Text())
	}
	return cmd.Wait()
}

func has(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// ---- TCP 端口探测 ----

// ParsePorts 解析 "22,80,8000-8100"，最多返回 1024 个端口。
func ParsePorts(s string) ([]int, error) {
	seen := map[int]bool{}
	var out []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lo, hi := part, part
		if a, b, ok := strings.Cut(part, "-"); ok {
			lo, hi = a, b
		}
		l, err1 := strconv.Atoi(strings.TrimSpace(lo))
		h, err2 := strconv.Atoi(strings.TrimSpace(hi))
		if err1 != nil || err2 != nil || l < 1 || h > 65535 || l > h {
			return nil, fmt.Errorf("端口 %q 无效", part)
		}
		for p := l; p <= h; p++ {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
			if len(out) > 1024 {
				return nil, errors.New("一次最多探测 1024 个端口")
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("请填写要探测的端口")
	}
	return out, nil
}

// PortResult 是端口探测结果。
type PortResult struct {
	Port    int    `json:"port"`
	Open    bool   `json:"open"`
	RTTms   int64  `json:"rttMs,omitempty"`
	Service string `json:"service,omitempty"`
}

var wellKnown = map[int]string{
	21: "ftp", 22: "ssh", 23: "telnet", 25: "smtp", 53: "dns", 80: "http", 110: "pop3", 143: "imap", 443: "https",
	445: "smb", 465: "smtps", 587: "submission", 993: "imaps", 995: "pop3s", 1433: "mssql", 3306: "mysql",
	3389: "rdp", 5432: "postgres", 5900: "vnc", 6379: "redis", 8080: "http-alt", 8443: "https-alt", 27017: "mongodb",
}

func tcpTool(ctx context.Context, m *Module, req Request, out output) error {
	addrs, err := m.resolve(ctx, req.Target)
	if err != nil {
		return err
	}
	ports, err := ParsePorts(req.Ports)
	if err != nil {
		return err
	}
	host := addrs[0]
	out.Line("探测 %s (%s) 的 %d 个端口", req.Target, host, len(ports))
	results := make([]PortResult, len(ports))
	sem := make(chan struct{}, 100)
	var wg sync.WaitGroup
	var open atomic.Int32
	for i, p := range ports {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			r := PortResult{Port: p}
			if ctx.Err() == nil {
				start := time.Now()
				d := net.Dialer{Timeout: 1500 * time.Millisecond}
				if c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host.String(), strconv.Itoa(p))); err == nil {
					c.Close()
					r.Open, r.RTTms, r.Service = true, time.Since(start).Milliseconds(), wellKnown[p]
					open.Add(1)
				}
			}
			results[i] = r
		}()
	}
	wg.Wait()
	for _, r := range results {
		if r.Open {
			svc := r.Service
			if svc != "" {
				svc = " (" + svc + ")"
			}
			out.Line("端口 %d 开放%s  %d ms", r.Port, svc, r.RTTms)
		}
	}
	out.Line("--- 共 %d 个端口，%d 个开放", len(ports), open.Load())
	// 结果只保留开放端口，避免大范围探测时体积过大
	var openOnly []PortResult
	for _, r := range results {
		if r.Open {
			openOnly = append(openOnly, r)
		}
	}
	out.SetResult(openOnly)
	return nil
}

// ---- DNS ----

func dnsTool(ctx context.Context, m *Module, req Request, out output) error {
	name := strings.TrimSpace(req.Target)
	if name == "" {
		return errors.New("请填写要查询的域名或 IP")
	}
	var r *net.Resolver = net.DefaultResolver
	via := "系统解析器"
	if strings.TrimSpace(req.Server) != "" {
		srv, err := resolver.Parse(req.Server)
		if err != nil {
			return err
		}
		r = resolver.New([]resolver.Server{srv})
		via = strings.TrimSpace(req.Server)
	}
	typ := strings.ToUpper(strings.TrimSpace(req.Type))
	if typ == "" {
		typ = "A"
	}
	out.Line("查询 %s %s（经 %s）", name, typ, via)
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	start := time.Now()
	var lines []string
	var err error
	switch typ {
	case "A", "AAAA":
		network := "ip4"
		if typ == "AAAA" {
			network = "ip6"
		}
		var ips []netip.Addr
		ips, err = r.LookupNetIP(ctx, network, name)
		for _, ip := range ips {
			lines = append(lines, ip.String())
		}
	case "CNAME":
		var c string
		c, err = r.LookupCNAME(ctx, name)
		lines = append(lines, c)
	case "MX":
		var mx []*net.MX
		mx, err = r.LookupMX(ctx, name)
		for _, x := range mx {
			lines = append(lines, fmt.Sprintf("%d %s", x.Pref, x.Host))
		}
	case "TXT":
		lines, err = r.LookupTXT(ctx, name)
	case "NS":
		var ns []*net.NS
		ns, err = r.LookupNS(ctx, name)
		for _, x := range ns {
			lines = append(lines, x.Host)
		}
	case "PTR":
		lines, err = r.LookupAddr(ctx, name)
	default:
		return fmt.Errorf("不支持的记录类型 %q", typ)
	}
	if err != nil {
		return err
	}
	sort.Strings(lines)
	for _, l := range lines {
		out.Line("%s", l)
	}
	out.Line("--- %d 条记录，耗时 %d ms", len(lines), time.Since(start).Milliseconds())
	return nil
}

// ---- 测速 ----

// defaultSpeedURL 为默认测速地址。Cloudflare 的 __down 要求 bytes 小于 1 亿，超过会返回 403；
// 单次下载完后会重新请求，所以单次大小只影响请求次数，不限制测速时长。
const defaultSpeedURL = "https://speed.cloudflare.com/__down?bytes=50000000"

// speedHost 返回测速地址的主机名，用于任务标题；留空时为默认测速节点。
func speedHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = defaultSpeedURL
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return u.Host
}

func speedTool(ctx context.Context, m *Module, req Request, out output) error {
	target := strings.TrimSpace(req.URL)
	if target == "" {
		target = defaultSpeedURL
	}
	u, err := parseHTTPURL(target)
	if err != nil {
		return err
	}
	if _, err := m.resolve(ctx, u.Hostname()); err != nil {
		return err
	}
	secs := req.Seconds
	if secs <= 0 {
		secs = 8
	}
	if secs > 30 {
		secs = 30
	}
	const streams = 4
	out.Line("下载测速：%s，%d 路并发，持续 %d 秒", u.Host, streams, secs)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(secs)*time.Second)
	defer cancel()
	var total atomic.Int64
	var wg sync.WaitGroup
	var firstErr atomic.Value
	for i := 0; i < streams; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				rq, _ := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
				resp, err := speedClient.Do(rq)
				if err == nil && resp.StatusCode != http.StatusOK {
					resp.Body.Close()
					err = fmt.Errorf("服务器返回 HTTP %d", resp.StatusCode)
				}
				if err != nil {
					if ctx.Err() == nil {
						firstErr.CompareAndSwap(nil, err)
					}
					return
				}
				n, _ := io.Copy(countWriter{&total}, resp.Body)
				resp.Body.Close()
				if n == 0 {
					return
				}
			}
		}()
	}
	start := time.Now()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var last int64
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
loop:
	for {
		select {
		case <-ticker.C:
			cur := total.Load()
			out.Line("%.1f Mbps", float64(cur-last)*8/1e6)
			last = cur
		case <-done:
			break loop
		}
	}
	elapsed := time.Since(start).Seconds()
	got := total.Load()
	if got == 0 {
		if e, ok := firstErr.Load().(error); ok {
			return e
		}
		return errors.New("没有收到数据")
	}
	out.Line("--- 共下载 %.1f MB，平均 %.1f Mbps", float64(got)/1e6, float64(got)*8/1e6/elapsed)
	return nil
}

// speedClient 不跟随重定向，避免被引导到禁止探测的地址。
var speedClient = &http.Client{Transport: outbound.NewTransport(), CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

type countWriter struct{ n *atomic.Int64 }

func (c countWriter) Write(p []byte) (int, error) {
	c.n.Add(int64(len(p)))
	return len(p), nil
}

func parseHTTPURL(s string) (*url.URL, error) {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return nil, errors.New("地址需为 http(s):// 开头的完整 URL")
	}
	return u, nil
}
