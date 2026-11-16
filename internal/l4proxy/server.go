package l4proxy

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
)

// Stats 是一条规则的运行统计，所有字段使用原子操作。
type Stats struct {
	ActiveConns atomic.Int64
	TotalConns  atomic.Int64
	BytesIn     atomic.Int64 // 客户端 -> 目标
	BytesOut    atomic.Int64 // 目标 -> 客户端
}

// Server 管理一条规则的全部监听器与连接协程，Stop 时一并关闭并等待退出。
type Server struct {
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	mu      sync.Mutex
	closers []io.Closer
}

// Context 返回随 Stop 取消的子上下文，规则的监听与连接都应使用它。只能调用一次。
func (s *Server) Context(parent context.Context) context.Context {
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	return ctx
}

// Track 登记一个需要在 Stop 时关闭的对象（监听器、UDP 套接字等）。
func (s *Server) Track(c io.Closer) {
	s.mu.Lock()
	s.closers = append(s.closers, c)
	s.mu.Unlock()
}

// Go 启动一个 Stop 时需要等待其退出的协程。
func (s *Server) Go(f func()) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		f()
	}()
}

// Stop 取消上下文、关闭全部登记的对象，并等待协程退出。
func (s *Server) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Lock()
	for _, c := range s.closers {
		_ = c.Close()
	}
	s.closers = nil
	s.mu.Unlock()
	s.wg.Wait()
}

// Limits 是一个 TCP 监听的并发连接上限。
type Limits struct {
	// Max 为并发连接总数上限
	Max int
	// PerSource 为单个来源（IPv4 地址、IPv6 /64）的并发连接上限，0 表示不限制。
	// 回环、内网与链路本地地址不受此限制：它们是本机进程或局域网设备，不是这个上限要防的互联网来源，
	// Docker 网桥等场景下大量正常连接也会共用同一个内网地址
	PerSource int
}

// PerSourceLimit 计算单来源上限：rule 为规则设置，def 为全局默认值（均为 0 表示未设置），total 为总上限。
// 都未设置时取总上限的一半（至少 1）；结果不超过总上限。
func PerSourceLimit(rule, def, total int) int {
	n := rule
	if n <= 0 {
		n = def
	}
	if n <= 0 {
		n = total / 2
	}
	return min(max(n, 1), total)
}

// remoteAddrOf 返回连接的来源地址，可在测试中替换。
var remoteAddrOf = func(c net.Conn) (netip.Addr, bool) {
	ap, err := netip.ParseAddrPort(c.RemoteAddr().String())
	if err != nil {
		return netip.Addr{}, false
	}
	return ap.Addr().Unmap(), true
}

// sourceLimitWarnEvery 为单来源超限警告的最短间隔：同一监听持续被单个来源占满时不逐个连接记录
const sourceLimitWarnEvery = 10 * time.Second

// sourceLimiter 按来源（IPv4 地址、IPv6 /64）统计并发连接数。
type sourceLimiter struct {
	limit int
	mu    sync.Mutex
	count map[netip.Prefix]int
	// warnAt 为最近一次记录超限警告的时间，suppressed 为其后被拒绝而未单独记录的连接数
	warnAt     time.Time
	suppressed int
}

// exemptSource 判断来源是否不受单来源上限约束：回环、内网（含运营商级 NAT 与 IPv6 ULA）与链路本地地址。
func exemptSource(a netip.Addr) bool {
	return a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsUnspecified() || cgnat.Contains(a)
}

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// sourceKey 返回来源的统计键：IPv4 为地址本身，IPv6 按 /64 合并（同一用户通常持有整个 /64，可以随意更换地址）。
func sourceKey(a netip.Addr) netip.Prefix {
	if a.Is4() {
		return netip.PrefixFrom(a, 32)
	}
	p, _ := a.Prefix(64)
	return p
}

// acquire 为来源占用一个名额，超过上限返回 false。ok 为 false（无法解析地址）或来源豁免时总是成功，release 为空操作。
func (l *sourceLimiter) acquire(a netip.Addr, ok bool) (release func(), allowed bool) {
	a = a.Unmap()
	if l == nil || !ok || exemptSource(a) {
		return func() {}, true
	}
	key := sourceKey(a)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.count[key] >= l.limit {
		return nil, false
	}
	l.count[key]++
	return func() {
		l.mu.Lock()
		if l.count[key]--; l.count[key] <= 0 {
			delete(l.count, key)
		}
		l.mu.Unlock()
	}, true
}

// shouldWarn 判断这次超限是否记录警告，返回自上次警告以来未记录的次数。
func (l *sourceLimiter) shouldWarn(now time.Time) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.warnAt.IsZero() && now.Sub(l.warnAt) < sourceLimitWarnEvery {
		l.suppressed++
		return false, 0
	}
	n := l.suppressed
	l.warnAt, l.suppressed = now, 0
	return true, n
}

// ServeTCP 在 addr 上监听 TCP，每个连接在独立协程中交给 handle 处理，handle 返回后连接计数释放。
// 被封禁地址的连接在 Accept 时直接关闭；并发连接超过 limits.Max、或单个来源超过 limits.PerSource 时新连接被拒绝，
// 并通过 warnf 记录（单来源超限每 10 秒最多记录一条）。handle 负责关闭连接。
func (s *Server) ServeTCP(ctx context.Context, network, addr string, limits Limits, warnf func(format string, args ...any), handle func(net.Conn)) error {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, network, addr)
	if err != nil {
		return err
	}
	ln = banlist.Default.Listener(ln)
	s.Track(ln)
	sem := make(chan struct{}, limits.Max)
	var perSrc *sourceLimiter
	if limits.PerSource > 0 {
		perSrc = &sourceLimiter{limit: limits.PerSource, count: map[netip.Prefix]int{}}
	}
	s.Go(func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
					return
				}
				warnf("accept 失败: %v", err)
				// 避免 fd 耗尽等错误下空转
				time.Sleep(100 * time.Millisecond)
				continue
			}
			release, ok := perSrc.acquire(remoteAddrOf(conn))
			if !ok {
				if warn, more := perSrc.shouldWarn(time.Now()); warn {
					if more > 0 {
						warnf("%s 单个来源超过最大连接数 %d，拒绝 %s（此前 10 秒内另有 %d 个连接因此被拒绝）", addr, limits.PerSource, conn.RemoteAddr(), more)
					} else {
						warnf("%s 单个来源超过最大连接数 %d，拒绝 %s", addr, limits.PerSource, conn.RemoteAddr())
					}
				}
				conn.Close()
				continue
			}
			select {
			case sem <- struct{}{}:
			default:
				release()
				warnf("%s 超过最大连接数 %d，拒绝 %s", addr, limits.Max, conn.RemoteAddr())
				conn.Close()
				continue
			}
			s.Go(func() {
				defer func() { <-sem }()
				defer release()
				handle(conn)
			})
		}
	})
	return nil
}
