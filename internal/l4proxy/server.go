package l4proxy

import (
	"context"
	"errors"
	"io"
	"net"
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

// ServeTCP 在 addr 上监听 TCP，每个连接在独立协程中交给 handle 处理，handle 返回后连接计数释放。
// 被封禁地址的连接在 Accept 时直接关闭；并发连接超过 maxConns 时新连接被拒绝，并通过 warnf 记录。
// handle 负责关闭连接。
func (s *Server) ServeTCP(ctx context.Context, network, addr string, maxConns int, warnf func(format string, args ...any), handle func(net.Conn)) error {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, network, addr)
	if err != nil {
		return err
	}
	ln = banlist.Default.Listener(ln)
	s.Track(ln)
	sem := make(chan struct{}, maxConns)
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
			select {
			case sem <- struct{}{}:
			default:
				warnf("%s 超过最大连接数 %d，拒绝 %s", addr, maxConns, conn.RemoteAddr())
				conn.Close()
				continue
			}
			s.Go(func() {
				defer func() { <-sem }()
				handle(conn)
			})
		}
	})
	return nil
}
