package stun

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/iplist"
	"github.com/wangwanjie/gately/internal/netpipe"
	"github.com/wangwanjie/gately/internal/openports"
	"github.com/wangwanjie/gately/internal/reuseport"
	stunproto "github.com/wangwanjie/gately/internal/stun"
)

// 连续多少次地址检测失败后重建会话
const maxCheckFailures = 3

// sessionTCP 建立一次 TCP 穿透会话，正常运行时阻塞，出错时返回。
//
// 流程：在通道端口监听（内置转发）→ UPnP/NAT-PMP 映射 → 从同一端口连接保活服务器
// → 从同一端口查询 STUN → 有效性检测 → 发布地址 → 循环心跳与地址检测。
func (r *runner) sessionTCP(ctx context.Context) error {
	bindIP, err := r.rule.Bind.resolveBindIP()
	if err != nil {
		return err
	}
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	timeout := time.Duration(r.rule.StunTimeout) * time.Millisecond

	listenIP := bindIP
	if listenIP == "" {
		listenIP = "0.0.0.0"
	}
	var ln net.Listener
	if !r.rule.DisableForward {
		ln, err = reuseport.Listen(sctx, "tcp4", net.JoinHostPort(listenIP, strconv.Itoa(r.port)))
		if err != nil {
			return fmt.Errorf("监听通道端口失败: %w", err)
		}
		defer ln.Close()
		r.port = ln.Addr().(*net.TCPAddr).Port
		// 通道端口监听成功后立即视为开放端口，访客随即连接时不会被扫描检测当成未开放端口
		defer openports.Hold(ln.Addr())()
	}
	local := localTCPAddr(bindIP, r.port)
	r.mu.Lock()
	r.state.LocalAddr = net.JoinHostPort(listenIP, strconv.Itoa(r.port))
	r.mu.Unlock()

	// 先在路由器建立映射，路由器的 NAT 会复用该端口处理后续出站连接，公网端口更稳定
	r.openFirewall()
	unmap := r.mapPort(sctx, "tcp", r.port)
	defer unmap()

	ka, err := r.dialKeepAlive(sctx, local, timeout)
	if err != nil {
		return err
	}
	defer func() { ka.close() }()

	pub, err := r.queryTCP(sctx, local, timeout)
	if err != nil {
		return err
	}
	pm := &publicMapping{r: r, proto: "tcp"}
	defer pm.close()
	pm.update(sctx, pub)

	var wg sync.WaitGroup
	if ln != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.acceptLoop(sctx, ln)
		}()
		// 先取消上下文结束已有转发、关闭监听使 acceptLoop 退出，再等待
		defer func() {
			cancel()
			ln.Close()
			wg.Wait()
		}()
	}

	if !r.rule.DisableCheck {
		// 检测连接来自 Gately 自身，名单拒绝它属于正常现象，不计入拒绝次数（见 selfCheckDenied）
		r.selfCheckPub.Store(pub.Addr())
		r.selfCheckUntil.Store(time.Now().Add(timeout + time.Second).UnixNano())
		if err := checkTCP(sctx, pub, timeout); err != nil {
			return fmt.Errorf("通道有效性检测失败（公网地址 %s 不可达，可能不是 NAT1 网络或未做端口映射）: %w", pub, err)
		}
	}
	r.publish(ctx, pub)

	hb := time.NewTicker(time.Duration(r.rule.HeartbeatInterval) * time.Millisecond)
	defer hb.Stop()
	check := time.NewTicker(time.Duration(r.settings.AddrCheckInterval) * time.Second)
	defer check.Stop()
	failures := 0
	recheck := func() error {
		np, err := r.queryTCP(sctx, local, timeout)
		if err != nil {
			failures++
			r.log.Warnf("地址检测失败(%d/%d): %v", failures, maxCheckFailures, err)
			if failures >= maxCheckFailures {
				return errors.New("连续检测公网地址失败")
			}
			return nil
		}
		failures = 0
		if np != pub {
			r.log.Warnf("公网地址改变: %s -> %s", pub, np)
			pub = np
			pm.update(sctx, pub)
		}
		r.publish(ctx, pub)
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-hb.C:
			if err := ka.probe(timeout); err != nil {
				r.log.Warnf("保活连接异常，重新建立: %v", err)
				ka.close()
				ka, err = r.dialKeepAlive(sctx, local, timeout)
				if err != nil {
					return fmt.Errorf("保活连接断开: %w", err)
				}
				// 保活中断期间映射可能已变化，立即重新检测
				if err := recheck(); err != nil {
					return err
				}
			}
		case <-check.C:
			if err := recheck(); err != nil {
				return err
			}
		}
	}
}

// setNoLinger 让关闭连接时直接发送 RST，避免本地端口进入 TIME_WAIT 导致下次从同一端口连接失败。
func setNoLinger(c net.Conn) {
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.SetLinger(0)
	}
}

// queryTCP 从通道端口依次向 STUN 服务器查询映射地址。
func (r *runner) queryTCP(ctx context.Context, local *net.TCPAddr, timeout time.Duration) (netip.AddrPort, error) {
	var errs []error
	for _, server := range r.orderedServers() {
		c, err := reuseport.Dial(ctx, "tcp4", local, server, timeout)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", server, err))
			continue
		}
		setNoLinger(c)
		_ = c.SetDeadline(time.Now().Add(timeout))
		ap, err := stunproto.BindingTCP(c)
		c.Close()
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", server, err))
			continue
		}
		r.rememberServer(server)
		return netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port()), nil
	}
	if ctx.Err() != nil {
		return netip.AddrPort{}, ctx.Err()
	}
	return netip.AddrPort{}, fmt.Errorf("所有 STUN 服务器查询失败: %w", errors.Join(errs...))
}

// keepAlive 是从通道端口发起的 HTTP 长连接，定期发送 HEAD 请求维持 NAT 映射。
type keepAlive struct {
	conn net.Conn
	br   *bufio.Reader
	host string
}

func (r *runner) dialKeepAlive(ctx context.Context, local *net.TCPAddr, timeout time.Duration) (*keepAlive, error) {
	var errs []error
	for _, server := range r.keepAliveServers() {
		host, port, _ := net.SplitHostPort(server)
		c, err := reuseport.Dial(ctx, "tcp4", local, server, timeout)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", server, err))
			continue
		}
		setNoLinger(c)
		if port == "443" {
			tc := tls.Client(c, &tls.Config{ServerName: host})
			_ = tc.SetDeadline(time.Now().Add(timeout))
			if err := tc.Handshake(); err != nil {
				c.Close()
				errs = append(errs, fmt.Errorf("%s: %w", server, err))
				continue
			}
			c = tc
		}
		ka := &keepAlive{conn: c, br: bufio.NewReader(c), host: host}
		if err := ka.probe(timeout); err != nil {
			c.Close()
			errs = append(errs, fmt.Errorf("%s: %w", server, err))
			continue
		}
		return ka, nil
	}
	return nil, fmt.Errorf("无法建立保活连接: %w", errors.Join(errs...))
}

// probe 发送一次 HEAD 请求并读取响应头。
func (k *keepAlive) probe(timeout time.Duration) error {
	_ = k.conn.SetDeadline(time.Now().Add(timeout))
	req := "HEAD / HTTP/1.1\r\nHost: " + k.host + "\r\nConnection: keep-alive\r\nUser-Agent: Mozilla/5.0 (Gately)\r\n\r\n"
	if _, err := k.conn.Write([]byte(req)); err != nil {
		return err
	}
	resp, err := http.ReadResponse(k.br, &http.Request{Method: http.MethodHead})
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.Close {
		return errors.New("服务器关闭了长连接")
	}
	return nil
}

func (k *keepAlive) close() {
	if k != nil && k.conn != nil {
		k.conn.Close()
	}
}

// checkTCP 连接自身的公网地址，验证外部可达。
func checkTCP(ctx context.Context, pub netip.AddrPort, timeout time.Duration) error {
	d := net.Dialer{Timeout: timeout}
	c, err := d.DialContext(ctx, "tcp4", pub.String())
	if err != nil {
		return err
	}
	return c.Close()
}

func (r *runner) acceptLoop(ctx context.Context, ln net.Listener) {
	maxConns := r.rule.MaxConnections
	if maxConns <= 0 {
		maxConns = 256
	}
	sem := make(chan struct{}, maxConns)
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		c, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		// 被封禁地址的连接直接关闭
		if banlist.Default.BannedAddr(c.RemoteAddr()) {
			c.Close()
			continue
		}
		// 封禁优先，再按来源 IP 名单判断，不命中时不连接目标
		if !r.filter.Load().Allows(c.RemoteAddr()) {
			if !r.selfCheckDenied(c.RemoteAddr()) {
				r.stats.denied.Add(1)
				r.log.Debugf("%s 不满足来源 IP 访问控制，已断开", c.RemoteAddr())
			}
			c.Close()
			continue
		}
		select {
		case sem <- struct{}{}:
		default:
			r.log.Warnf("超过最大连接数 %d，拒绝 %s", maxConns, c.RemoteAddr())
			c.Close()
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			r.forwardTCP(ctx, c)
		}()
	}
}

// selfCheckDenied 判断被名单拒绝的连接是否为本次通道有效性检测发起的连接：检测窗口内、来源为本机或自身公网 IP 的
// 第一个被拒连接视为检测连接（检测只连一次，且先于其它连接进入 accept 队列）。经路由器回流时来源可能是路由器地址，
// 此时无法区分，检测连接会计入一次拒绝。
func (r *runner) selfCheckDenied(addr net.Addr) bool {
	if time.Now().UnixNano() >= r.selfCheckUntil.Load() {
		return false
	}
	ip, ok := iplist.AddrOf(addr)
	if !ok {
		return false
	}
	pub, _ := r.selfCheckPub.Load().(netip.Addr)
	if ip != pub && !isLocalIP(ip) {
		return false
	}
	// 只认第一个，之后的同源连接照常计数
	return r.selfCheckUntil.Swap(0) != 0
}

// isLocalIP 判断地址是否为本机地址（回环或本机网卡上的地址）。
func isLocalIP(ip netip.Addr) bool {
	if ip.IsLoopback() {
		return true
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok {
			if x, ok := netip.AddrFromSlice(n.IP); ok && x.Unmap() == ip {
				return true
			}
		}
	}
	return false
}

func (r *runner) forwardTCP(ctx context.Context, client net.Conn) {
	defer client.Close()
	r.stats.active.Add(1)
	r.stats.total.Add(1)
	defer r.stats.active.Add(-1)
	var target net.Conn
	var err error
	for _, host := range r.rule.TargetAddresses {
		target, err = net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(r.rule.TargetPort)), 5*time.Second)
		if err == nil {
			break
		}
	}
	if target == nil {
		r.log.Warnf("%s 连接目标失败: %v", client.RemoteAddr(), err)
		return
	}
	defer target.Close()
	r.log.Debugf("%s -> %s", client.RemoteAddr(), target.RemoteAddr())
	in, out := netpipe.Join(ctx, client, target)
	r.stats.in.Add(in)
	r.stats.out.Add(out)
}
