package stun

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/reuseport"
	stunproto "github.com/wangwanjie/gately/internal/stun"
)

// udpDispatcher 读取通道端口上的所有数据包：STUN 响应交给等待者，其余转发给目标。
type udpDispatcher struct {
	r  *runner
	pc net.PacketConn

	mu      sync.Mutex
	waiters map[stunproto.TxID]chan []byte
	token   []byte
	tokenCh chan struct{}

	sessMu   sync.Mutex
	sessions map[string]*udpSession
	wg       sync.WaitGroup
}

type udpSession struct {
	conn       net.Conn
	lastActive atomic.Int64
}

// sessionUDP 建立一次 UDP 穿透会话。UDP 的 NAT 映射通过周期性 STUN 请求维持。
func (r *runner) sessionUDP(ctx context.Context) error {
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
	pc, err := reuseport.ListenPacket(sctx, "udp4", net.JoinHostPort(listenIP, strconv.Itoa(r.port)))
	if err != nil {
		return fmt.Errorf("监听通道端口失败: %w", err)
	}
	r.port = pc.LocalAddr().(*net.UDPAddr).Port
	r.mu.Lock()
	r.state.LocalAddr = net.JoinHostPort(listenIP, strconv.Itoa(r.port))
	r.mu.Unlock()

	d := &udpDispatcher{r: r, pc: pc, waiters: map[stunproto.TxID]chan []byte{}, sessions: map[string]*udpSession{}}
	d.wg.Add(1)
	go d.readLoop(sctx)
	defer func() {
		cancel()
		pc.Close()
		d.closeSessions()
		d.wg.Wait()
	}()

	r.openFirewall()
	unmap := r.mapPort(sctx, "udp", r.port)
	defer unmap()
	pub, err := d.query(sctx, timeout)
	if err != nil {
		return err
	}
	pm := &publicMapping{r: r, proto: "udp"}
	defer pm.close()
	pm.update(sctx, pub)

	if !r.rule.DisableCheck {
		// UDP 回环（hairpin）在很多 NAT 上不受支持，检测失败只告警
		if err := d.selfCheck(pub, timeout); err != nil {
			r.log.Warnf("UDP 通道自检未通过（部分网络不支持回环，可忽略）: %v", err)
		}
	}
	r.publish(ctx, pub)

	hb := time.NewTicker(time.Duration(r.rule.HeartbeatInterval) * time.Millisecond)
	defer hb.Stop()
	failures := 0
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-hb.C:
			np, err := d.query(sctx, timeout)
			if err != nil {
				failures++
				r.log.Warnf("UDP 心跳失败(%d/%d): %v", failures, maxCheckFailures, err)
				if failures >= maxCheckFailures {
					return errors.New("连续 UDP 心跳失败")
				}
				continue
			}
			failures = 0
			if np != pub {
				r.log.Warnf("公网地址改变: %s -> %s", pub, np)
				pub = np
				pm.update(sctx, pub)
				r.publish(ctx, pub)
			}
		}
	}
}

// query 依次向 STUN 服务器发送 Binding 请求，响应由 readLoop 分发。
func (d *udpDispatcher) query(ctx context.Context, timeout time.Duration) (netip.AddrPort, error) {
	var errs []error
	for _, server := range d.r.orderedServers() {
		ua, err := net.ResolveUDPAddr("udp4", server)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", server, err))
			continue
		}
		req, id := stunproto.NewBindingRequest()
		ch := make(chan []byte, 1)
		d.mu.Lock()
		d.waiters[id] = ch
		d.mu.Unlock()
		_, err = d.pc.WriteTo(req, ua)
		var resp []byte
		if err == nil {
			select {
			case resp = <-ch:
			case <-time.After(timeout):
				err = errors.New("超时")
			case <-ctx.Done():
				err = ctx.Err()
			}
		}
		d.mu.Lock()
		delete(d.waiters, id)
		d.mu.Unlock()
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", server, err))
			if ctx.Err() != nil {
				break
			}
			continue
		}
		ap, err := stunproto.ParseBindingResponse(resp, id)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", server, err))
			continue
		}
		d.r.rememberServer(server)
		return netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port()), nil
	}
	return netip.AddrPort{}, fmt.Errorf("所有 STUN 服务器查询失败: %w", errors.Join(errs...))
}

// selfCheck 从另一个套接字向自身公网地址发送随机令牌，确认能收到。
func (d *udpDispatcher) selfCheck(pub netip.AddrPort, timeout time.Duration) error {
	token := make([]byte, 16)
	_, _ = rand.Read(token)
	ch := make(chan struct{}, 1)
	d.mu.Lock()
	d.token, d.tokenCh = token, ch
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		d.token, d.tokenCh = nil, nil
		d.mu.Unlock()
	}()
	c, err := net.Dial("udp4", pub.String())
	if err != nil {
		return err
	}
	defer c.Close()
	for i := 0; i < 3; i++ {
		_, _ = c.Write(token)
		select {
		case <-ch:
			return nil
		case <-time.After(timeout / 3):
		}
	}
	return errors.New("未收到回环数据包")
}

func (d *udpDispatcher) readLoop(ctx context.Context) {
	defer d.wg.Done()
	buf := make([]byte, 64*1024)
	for {
		n, from, err := d.pc.ReadFrom(buf)
		if err != nil {
			return
		}
		pkt := buf[:n]
		if id, ok := stunproto.MessageTxID(pkt); ok {
			d.mu.Lock()
			ch := d.waiters[id]
			d.mu.Unlock()
			if ch != nil {
				select {
				case ch <- append([]byte{}, pkt...):
				default:
				}
				continue
			}
		}
		d.mu.Lock()
		if d.token != nil && bytes.Equal(pkt, d.token) {
			select {
			case d.tokenCh <- struct{}{}:
			default:
			}
			d.mu.Unlock()
			continue
		}
		d.mu.Unlock()
		// 不转发被封禁地址的数据包
		if d.r.rule.DisableForward || banlist.Default.BannedAddr(from) {
			continue
		}
		// 再按来源 IP 名单判断；每个数据包都判断，名单变化后已有会话的上行也立即被拦下
		if !d.r.allows(from) {
			continue
		}
		d.forward(ctx, from, pkt)
	}
}

func (d *udpDispatcher) forward(ctx context.Context, from net.Addr, pkt []byte) {
	key := from.String()
	d.sessMu.Lock()
	s := d.sessions[key]
	if s == nil {
		// 拨号（可能需要解析域名）放到锁外；只有读循环会新建会话，解锁期间不会出现重复会话
		d.sessMu.Unlock()
		var conn net.Conn
		var err error
		for _, host := range d.r.rule.TargetAddresses {
			conn, err = net.Dial("udp", net.JoinHostPort(host, strconv.Itoa(d.r.rule.TargetPort)))
			if err == nil {
				break
			}
		}
		if conn == nil {
			d.r.log.Warnf("%s 建立 UDP 会话失败: %v", key, err)
			return
		}
		d.sessMu.Lock()
		if ctx.Err() != nil {
			d.sessMu.Unlock()
			conn.Close()
			return
		}
		s = &udpSession{conn: conn}
		s.lastActive.Store(time.Now().UnixNano())
		d.sessions[key] = s
		d.r.stats.active.Add(1)
		d.r.stats.total.Add(1)
		d.wg.Add(1)
		go d.pump(ctx, from, key, s)
	}
	d.sessMu.Unlock()
	s.lastActive.Store(time.Now().UnixNano())
	if n, err := s.conn.Write(pkt); err == nil {
		d.r.stats.in.Add(int64(n))
	}
}

// pump 把目标的回包发回客户端，空闲超时后关闭会话。
func (d *udpDispatcher) pump(ctx context.Context, client net.Addr, key string, s *udpSession) {
	defer d.wg.Done()
	defer d.r.stats.active.Add(-1)
	defer func() {
		d.sessMu.Lock()
		if d.sessions[key] == s {
			delete(d.sessions, key)
		}
		d.sessMu.Unlock()
		s.conn.Close()
	}()
	idle := time.Duration(d.r.rule.UDPSessionTimeout) * time.Second
	if idle <= 0 {
		idle = 60 * time.Second
	}
	buf := make([]byte, 64*1024)
	for {
		_ = s.conn.SetReadDeadline(time.Now().Add(idle))
		n, err := s.conn.Read(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() && ctx.Err() == nil &&
				time.Since(time.Unix(0, s.lastActive.Load())) < idle {
				continue
			}
			return
		}
		s.lastActive.Store(time.Now().UnixNano())
		if _, err := d.pc.WriteTo(buf[:n], client); err != nil {
			return
		}
		d.r.stats.out.Add(int64(n))
	}
}

func (d *udpDispatcher) closeSessions() {
	d.sessMu.Lock()
	for _, s := range d.sessions {
		s.conn.Close()
	}
	d.sessMu.Unlock()
}
