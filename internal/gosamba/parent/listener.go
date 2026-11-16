// Gately 修改：仅在 Linux 与 macOS 编译（构建约束）；连接处理协程中的 panic 只关闭该连接。

//go:build linux || darwin

package parent

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"sync"
	"syscall"
	"time"
)

// Backoff bounds applied after a temporary Accept failure. The first retry is
// almost immediate so an isolated ECONNABORTED costs nothing, and the doubling
// cap keeps a sustained fd exhaustion from spinning the accept loop at 100% CPU
// while still recovering within a second of descriptors freeing up.
const (
	acceptRetryDelayMin = 5 * time.Millisecond
	acceptRetryDelayMax = 1 * time.Second
)

// isTemporaryAcceptError reports whether an Accept error is a transient,
// per-connection condition that must NOT bring the server down.
//
// The critical cases are EMFILE/ENFILE: running out of file descriptors is a
// load condition, and returning from Serve would turn "one client too many"
// into "the whole SMB server is gone" — an unauthenticated denial of service.
// ECONNABORTED (peer vanished between the SYN and our accept) and the memory
// pressure errnos are equally survivable.
//
// net.Error.Temporary() would cover most of this but is deprecated and was
// never well defined, so the errnos are matched explicitly. Timeout() is still
// meaningful and is honoured for listeners with a deadline set.
func isTemporaryAcceptError(err error) bool {
	switch {
	case errors.Is(err, syscall.EMFILE), // per-process fd limit
		errors.Is(err, syscall.ENFILE),       // system-wide fd limit
		errors.Is(err, syscall.ENOBUFS),      // out of socket buffers
		errors.Is(err, syscall.ENOMEM),       // out of memory
		errors.Is(err, syscall.ECONNABORTED), // peer gave up before accept
		errors.Is(err, syscall.ECONNRESET),   // ditto, RST instead of FIN
		errors.Is(err, syscall.EINTR),        // interrupted by a signal
		errors.Is(err, syscall.EAGAIN),       // nothing pending after all
		errors.Is(err, syscall.EPERM):        // firewall rejected this SYN
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	return false
}

// ConnHandler handles a single accepted connection. It must close the
// connection before returning.
type ConnHandler func(ctx context.Context, c net.Conn, log *slog.Logger, maxFrame uint32)

// Listener owns the accept loop. Reusable across plans.
type Listener struct {
	Log      *slog.Logger
	MaxFrame uint32
	Handler  ConnHandler

	// ReExec, when true, serves each accepted connection by re-exec'ing a
	// worker process (which drops privileges to the authenticated user) instead
	// of calling Handler in-process. The default (false) preserves the original
	// in-process serving path byte-for-byte.
	ReExec bool
}

// Serve runs the accept loop until ctx is cancelled or ln returns a permanent
// error.
func (s *Listener) Serve(ctx context.Context, ln net.Listener) error {
	if s.Handler == nil {
		return errors.New("listener: Handler is nil")
	}
	if s.MaxFrame == 0 {
		return errors.New("listener: MaxFrame is zero")
	}

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	var wg sync.WaitGroup
	var retryDelay time.Duration
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				wg.Wait()
				return nil
			}
			if isTemporaryAcceptError(err) {
				// Shed this one connection, not the server. Back off so a
				// persistent condition (fd exhaustion) does not spin.
				if retryDelay == 0 {
					retryDelay = acceptRetryDelayMin
				} else if retryDelay < acceptRetryDelayMax {
					retryDelay *= 2
					retryDelay = min(retryDelay, acceptRetryDelayMax)
				}
				s.Log.Warn("accept failed transiently; retrying", "err", err, "retry_in", retryDelay)
				timer := time.NewTimer(retryDelay)
				select {
				case <-timer.C:
				case <-ctx.Done():
					timer.Stop()
					wg.Wait()
					return nil
				}
				continue
			}
			s.Log.Error("accept failed", "err", err)
			wg.Wait()
			return err
		}
		// A successful accept means the transient condition cleared.
		retryDelay = 0
		// SMB is roundtrip-bound; explicitly disable Nagle. Go enables this
		// by default but proxies/tunnels can re-enable it.
		if tc, ok := conn.(*net.TCPConn); ok {
			_ = tc.SetNoDelay(true)
			_ = tc.SetKeepAlive(true)
		}
		if s.ReExec {
			// Hand the connection off to a worker process that owns it entirely
			// and drops privileges after auth. The parent does NOT serve it.
			if err := reExecWorker(conn, s.Log); err != nil {
				if errors.Is(err, errWorkerLimit) {
					// Load shedding, not a failure: an unauthenticated peer must
					// not be able to fork the host to death by opening sockets.
					s.Log.Warn("worker limit reached; refusing connection",
						"limit", maxConcurrentWorkers, "remote", conn.RemoteAddr().String())
				} else {
					s.Log.Error("re-exec worker failed; dropping connection", "err", err)
				}
			}
			_ = conn.Close() // parent always closes its copy of the fd
			continue
		}
		wg.Add(1)
		go func(c net.Conn) {
			defer wg.Done()
			// Gately 新增：处理函数中漏网的 panic 只关闭这一个连接，不结束整个进程
			defer recoverConnPanic(s.Log, "handler", func() { _ = c.Close() })
			s.Handler(ctx, c, s.Log, s.MaxFrame)
		}(conn)
	}
}
