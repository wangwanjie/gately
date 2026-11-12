// Gately 修改：仅在 Linux 与 macOS 编译（构建约束）。

//go:build linux || darwin

// Package parent contains the privileged SMB-server bootstrap.
package parent

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wangwanjie/gately/internal/gosamba/config"
	"github.com/wangwanjie/gately/internal/gosamba/smb2"
	"github.com/wangwanjie/gately/internal/gosamba/smb3"
	"github.com/wangwanjie/gately/internal/gosamba/transport"
)

const (
	// idleTimeout bounds how long a connection may go without delivering a
	// complete frame. It is reset before every read, so it only fires on a
	// genuinely idle or stalled peer (slow-loris), never on a busy one.
	idleTimeout = 5 * time.Minute
	// writeTimeout bounds a single response write, so a peer that stops
	// reading cannot pin the connection through TCP backpressure. One frame is
	// at most MaxIOSize, which completes far inside this window on any usable
	// link.
	writeTimeout = 2 * time.Minute
)

// bufRW is a tiny io.ReadWriter that pairs a buffered reader with the raw
// writer (so writes don't pass through the buffer and stall waiting for
// bufio's internal flushing). It also applies a per-write deadline when the
// underlying writer is a net.Conn.
type bufRW struct {
	r io.Reader
	w io.Writer
	c net.Conn
}

func (b *bufRW) Read(p []byte) (int, error) { return b.r.Read(p) }

func (b *bufRW) Write(p []byte) (int, error) {
	if b.c != nil {
		_ = b.c.SetWriteDeadline(time.Now().Add(writeTimeout))
	}
	return b.w.Write(p)
}

// ConnOptions controls per-connection protocol behavior.
type ConnOptions struct {
	RequireEncryption bool
	RequireSigning    bool
	MaxIOSize         uint32
	ServerStartTime   uint64
	Users             []config.UserConfig
	Shares            []config.ShareConfig

	// Durable is the server-scoped durable-handle table, created once and
	// shared across every ServeConn so durable opens survive a dropped TCP
	// connection. If nil, durable handles are not granted.
	Durable *DurableTable
	// DurableTimeout caps how long a reclaimed handle stays alive.
	DurableTimeout time.Duration

	// Sessions is the server-scoped session index, created once alongside
	// Durable and shared across every ServeConn. It is what lets a client
	// reconnecting on a NEW TCP connection name its old session in
	// PreviousSessionId and have the server close it (MS-SMB2 §3.3.5.5.3).
	//
	// If nil, ServeConn makes one of its own, which narrows the reach of
	// PreviousSessionId to this one connection but changes nothing else. That
	// is the right degradation for the re-exec worker, where one process serves
	// exactly one connection and there is no wider scope to have.
	Sessions *SessionIndex

	// OnAuthenticated, if non-nil, is invoked exactly once per session-setup
	// that resolves a user (the first time the connection authenticates). The
	// worker uses this hook to drop privileges to the authenticated user's
	// uid/gid. Returning a non-nil error tears down the connection. Default
	// (nil) is a no-op — the standard in-process serving path is unchanged.
	OnAuthenticated func(sess *Session) error

	// AuthLimiter 为 Gately 新增：按来源地址限制认证失败次数，为空时不限流。
	AuthLimiter AuthLimiter
}

// Bounds on per-connection concurrency.
//
// The real ceiling on how many requests a client may have outstanding is the
// credit window the server grants it (creditWindow, 512). Running that many
// file operations at once on one connection is pointless — the disk and the
// page cache saturate long before — so the pool is sized to the machine and
// then clamped to the credit window, above which extra workers could never be
// fed anyway. The floor is set above the eight concurrent transfers the macOS
// client pipelines, so a small machine still serves that pipeline in parallel.
const (
	// minConnWorkers must stay above maxSharingWaits (sharewait.go) with room
	// to spare, or a connection could park every worker on a contended file
	// and have none left to run the CLOSE that would free it.
	minConnWorkers = 8
	maxConnWorkers = 64
)

// connWorkers is how many inbound frames one connection may execute at once.
func connWorkers() int {
	n := runtime.NumCPU() * 2
	if n < minConnWorkers {
		n = minConnWorkers
	}
	if n > maxConnWorkers {
		n = maxConnWorkers
	}
	if n > creditWindow {
		n = creditWindow
	}
	return n
}

// connPool is one connection's bounded worker pool.
//
// The unit of work is a whole inbound frame, never a single message: the
// members of a compound chain share a "previous handle" FileID and inherit each
// other's status, so they must run in order, and running one frame per
// goroutine gives that ordering for free. Separate frames carry independent
// MessageIds and SMB2 explicitly allows them to be answered out of order.
type connPool struct {
	sem chan struct{}
	wg  sync.WaitGroup
}

func newConnPool(n int) *connPool { return &connPool{sem: make(chan struct{}, n)} }

// submit runs fn on the pool, waiting for a free slot.
//
// Blocking the caller — the connection's read goroutine — is the backpressure:
// with every worker busy the server simply stops taking frames off the socket,
// which is what bounds the work in flight regardless of how many credits the
// client is holding.
func (p *connPool) submit(fn func()) {
	p.sem <- struct{}{}
	p.wg.Add(1)
	go func() {
		defer func() {
			<-p.sem
			p.wg.Done()
		}()
		fn()
	}()
}

// drain waits for every frame already submitted to finish. Only the read
// goroutine calls submit, so it is also the only caller of drain.
func (p *connPool) drain() { p.wg.Wait() }

// frameNeedsSerialExecution reports whether a frame must run on the read
// goroutine with the pool drained, rather than concurrently with other frames.
//
// SESSION_SETUP is the one command that cannot run beside anything else: the
// NTLM exchange is a multi-leg state machine keyed on the session, and the
// post-auth hook drops the process's privileges, which must not happen while
// another request is part-way through a file operation under the old identity.
//
// Anything this cannot confidently parse is serialized too. The serial path
// walks the same chain and produces the proper protocol error for it, so being
// wrong here costs a little concurrency and never correctness.
func frameNeedsSerialExecution(frame []byte) bool {
	for off := 0; off < len(frame); {
		if len(frame)-off < smb2.HeaderSize {
			return true
		}
		hdr, err := smb2.DecodeHeader(frame[off : off+smb2.HeaderSize])
		if err != nil || hdr.Command == smb2.CommandSessionSetup {
			return true
		}
		if hdr.NextCommand == 0 {
			return false
		}
		if hdr.NextCommand < smb2.HeaderSize {
			return true
		}
		next := off + int(hdr.NextCommand)
		if next > len(frame) {
			return true
		}
		off = next
	}
	return true
}

// releaseConnOpens disposes of every handle a dropped connection still owns.
//
// It closes each file descriptor that is NOT held by a live durable-table entry
// and gives up that handle's share-mode reservation. Durable opens are left
// alone: the table owns their fd so the client can reconnect and reclaim them,
// and a handle awaiting reclaim is still an open for the purposes of the
// sharing-access check (MS-SMB2 §3.3.5.9), so it keeps its deny mode until the
// durable entry expires. Detach starts that countdown — until now the entry was
// attached to this (live) connection and could not expire, so the sweeper could
// never close an fd still in use.
//
// This is the connection-teardown arm of handle disposal; the others are
// handleClose, Dispatcher.releaseOpens (TREE_DISCONNECT / LOGOFF / superseded
// session) and releaseOpen (durable expiry). Every one of them must drop the
// share-mode reservation: an entry nobody can release leaves the file
// permanently unopenable by every client, which is worse than the missing
// check it replaced.
func releaseConnOpens(sessions *SessionTable, durable *DurableTable) {
	if sessions == nil {
		return
	}
	// Take the handles out of each session's map rather than iterating it in
	// place. The map is the ownership token (see sessionHost.closeSession): a
	// cross-connection teardown superseding one of these sessions right now
	// calls TakeAllOpens too, and whichever of the two empties the map owns the
	// release. Iterating without removing would let both release the same
	// handle — a double close, and a share-mode reservation dropped twice.
	var opens []*Open
	sessions.RangeSessions(func(s *Session) {
		opens = append(opens, s.TakeAllOpens()...)
	})
	for _, o := range opens {
		if o == nil {
			continue
		}
		// The pool is drained by the time this runs, so no request is in
		// flight — but an async goroutine (a blocking LOCK retrying under
		// Open.mu) can still be winding down, and a cross-connection teardown
		// may be releasing a sibling handle. Hold the handle for its release
		// for the same reason releaseOpens does.
		//
		// The descriptor test is INSIDE the hold on purpose. o.File is written
		// by a release, under this lock; testing it before taking the lock is
		// the anti-pattern lockOpenForMessage had to be fixed for. It is not
		// worth a subtle argument about why this particular one would be safe.
		unlock := lockOpenForRelease(o)
		if o.File == nil {
			// No descriptor means no share-mode reservation either: only real
			// file opens are ever registered (directories, named streams and
			// IPC$ pipes are exempt), so there is nothing to release here.
			unlock()
			continue
		}
		if o.IsDurable && durable != nil &&
			durable.Has(o.DurableClientGuid, o.DurableCreateGuid) {
			durable.Detach(o.DurableClientGuid, o.DurableCreateGuid)
			unlock()
			continue
		}
		// This handle is gone for good.
		sharedShareModes.release(o)
		// Release any byte-range locks this open held before the fd
		// closes: releaseAll's keyFor does an Fstat on the fd, which
		// fails (and silently no-ops) once the file is closed.
		sharedLockManager.releaseAll(o)
		o.File.Close()
		unlock()
	}
}

// ServeConn drives one TCP connection through SMB2 NEGOTIATE → SESSION_SETUP.
// Anything beyond SESSION_SETUP returns STATUS_INVALID_PARAMETER (Plan 4).
func ServeConn(ctx context.Context, c net.Conn, log *slog.Logger, maxFrame uint32, opts ConnOptions) {
	defer c.Close()
	log = log.With("remote", c.RemoteAddr().String())
	log.Info("connection opened")

	// Force the socket shut when the server context is cancelled, so a read
	// blocked on an idle peer unblocks and this connection tears down.
	//
	// The watcher must also exit when THIS connection ends. Selecting on
	// ctx.Done() alone parks the goroutine until server shutdown, so a server
	// that has handled many short-lived connections accumulates one parked
	// goroutine (and its captured net.Conn) per connection for the life of the
	// process — an unauthenticated peer could grow that without bound just by
	// connecting and disconnecting.
	connDone := make(chan struct{})
	defer close(connDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = c.Close()
		case <-connDone:
		}
	}()

	// Wrap the connection in a bufio.Reader so we coalesce the small
	// NBSS-header read with whatever else is on the wire — one syscall
	// instead of two per frame.
	br := bufio.NewReaderSize(c, 64*1024)
	rw := &bufRW{r: br, w: c, c: c}

	conn, err := Negotiate(rw, NegotiatorOptions{
		RequireEncryption: opts.RequireEncryption,
		RequireSigning:    opts.RequireSigning,
		MaxIOSize:         opts.MaxIOSize,
		ServerStartTime:   opts.ServerStartTime,
	}, log)
	if err != nil {
		log.Warn("negotiate failed", "err", err)
		return
	}
	conn.Durable = opts.Durable
	conn.DurableTimeout = opts.DurableTimeout

	// Every response from here on goes through one writer goroutine. Before
	// this, a handler wrote to the socket itself under a shared mutex, so a
	// single completion that met TCP backpressure held that mutex for up to
	// writeTimeout and the whole mount stalled behind it — however many credits
	// the client was holding.
	writer := newConnWriter(rw, log, func() { _ = c.Close() })
	// Registered here rather than left to the teardown block below, so the
	// writer goroutine cannot outlive this function through any path — a panic
	// between here and there included. Close is idempotent, and the teardown
	// block still calls it at the point in the sequence where it belongs.
	defer writer.Close()
	// Handlers that frame their own response (SESSION_SETUP) get a writer that
	// queues rather than one that touches the socket, so nothing can interleave
	// with a frame the writer goroutine is emitting.
	cio := &connIO{r: br, w: writer}

	pool := newConnPool(connWorkers())

	// aborted is set by a handler that has decided the connection must go.
	//
	// It stops the read loop without closing the socket, because the response
	// that made the decision — a signature failure, a deleted session — is
	// still in the writer's queue and the client is owed it. Teardown flushes
	// the queue before the deferred Close takes the socket down.
	var aborted atomic.Bool
	abort := func() {
		aborted.Store(true)
		// Unblock a read already in progress. This ordering matters: the read
		// loop arms its deadline and then tests aborted, so either it sees the
		// flag, or this deadline lands after the one it armed and the read
		// returns at once.
		_ = c.SetReadDeadline(time.Now())
	}

	sessions := NewSessionTable()
	// The server-scoped session index. A nil one means nobody handed this
	// connection a wider scope (the re-exec worker, or a test driving ServeConn
	// directly), and a private index then keeps PreviousSessionId working
	// within this connection instead of not at all.
	index := opts.Sessions
	if index == nil {
		index = NewSessionIndex()
	}
	// dispatcher is assigned just below; the teardown closure needs to see it,
	// so it is declared before the defer that references it. host is the
	// connection's identity in the index, and carries the dispatcher, so it is
	// filled in after both exist.
	var dispatcher *Dispatcher
	host := &sessionHost{sessions: sessions, conn: conn, log: log}
	// On connection drop, close every file descriptor that is NOT held by a
	// live durable-table entry. Durable opens must stay alive so the client can
	// reconnect and reclaim them; ordinary (non-durable) opens must be closed
	// to release kernel fds.
	defer func() {
		// Leave the server-scoped index first. From here on this connection can
		// serve nothing, so an entry still pointing at it could only send a
		// reconnect's teardown into a connection that is already disposing of
		// the same handles. Both paths are safe against each other (the session
		// map decides who releases what), but the narrower window is free.
		index.unregisterHost(host)
		// Wait for every frame still executing. Disposing of handles below
		// closes descriptors those frames are reading and writing through.
		pool.drain()
		// Wake every outstanding CHANGE_NOTIFY goroutine so it exits and drops
		// its watch descriptors instead of blocking forever on a dead client.
		if dispatcher != nil {
			dispatcher.CancelAllNotifies()
		}
		// Flush what is still queued — typically the very response that
		// decided to end the connection — then stop the writer goroutine.
		writer.Close()
		// Drop every server-side-copy resume key this connection issued. They
		// are capabilities naming open handles, so nothing may hold one (or,
		// through it, an *Open and its fd) once the connection is gone — not
		// even the durable opens deliberately left alive just below.
		conn.resumeKeys.clear()
		releaseConnOpens(sessions, opts.Durable)
	}()
	ssHandler := &SessionSetupHandler{
		Conn:              conn,
		Sessions:          sessions,
		Users:             opts.Users,
		Shares:            opts.Shares,
		Log:               log,
		RequireEncryption: opts.RequireEncryption,
		Index:             index,
		Host:              host,
		Limiter:           opts.AuthLimiter,
		RemoteIP:          remoteIPOf(c.RemoteAddr()),
	}
	dispatcher = &Dispatcher{
		Conn:              conn,
		Sessions:          sessions,
		Shares:            opts.Shares,
		Log:               log,
		locks:             sharedLockManager,
		RequireEncryption: opts.RequireEncryption,
		RequireSigning:    opts.RequireSigning,
		out:               writer,
		async:             &asyncTable{},
		Index:             index,
	}
	// Tearing a session down is the same work LOGOFF does, so it runs through
	// this connection's dispatcher — which is what makes the resume keys, the
	// CHANGE_NOTIFY registrations and the durable entries that get released the
	// ones belonging to the connection that owns the session, even when the
	// teardown was ordered by a different connection's SESSION_SETUP.
	host.disp = dispatcher

	// serveFrame executes one inbound frame's chain and sends its responses.
	// It reports false when the connection must be dropped.
	//
	// The chain runs on a per-frame clone of the dispatcher: the "previous
	// handle" FileID, the previous member's status and the encryption decision
	// are all per-frame, and two frames running side by side on the pool would
	// otherwise overwrite each other's.
	serveFrame := func(frame []byte, encrypted bool) bool {
		fd := dispatcher.forFrame(encrypted)
		// The chain's responses go back as one compounded frame however the
		// chain ends: a chain that aborts the connection still owes the client
		// the error that aborted it.
		defer fd.flush(cio)

		// Walk the (possibly compound) chain, dispatching each message over
		// its own slice. The first message's NextCommand gives the offset of
		// the next message; subsequent messages have FlagRelatedOps set.
		off := 0
		for off < len(frame) {
			if len(frame)-off < smb2.HeaderSize {
				log.Warn("undersized frame after negotiate", "off", off, "len", len(frame))
				return false
			}
			hdr, err := smb2.DecodeHeader(frame[off : off+smb2.HeaderSize])
			if err != nil {
				log.Warn("bad header", "err", err, "off", off)
				return false
			}
			end := len(frame)
			if hdr.NextCommand != 0 {
				// NextCommand is the byte offset from this message's header to
				// the next one in the chain. It must clear this message's own
				// 64-byte header and must not run past the frame; a value in
				// (0, HeaderSize) would make body = msgBytes[HeaderSize:] slice
				// out of range and panic the whole (unauthenticated) connection.
				if hdr.NextCommand < smb2.HeaderSize {
					log.Warn("NextCommand shorter than header", "next_command", hdr.NextCommand)
					return false
				}
				end = off + int(hdr.NextCommand)
				if end > len(frame) {
					log.Warn("NextCommand overruns frame", "next_command", hdr.NextCommand, "len", len(frame))
					return false
				}
			}
			msgBytes := frame[off:end]
			body := msgBytes[smb2.HeaderSize:]

			switch hdr.Command {
			case smb2.CommandSessionSetup:
				sess, err := ssHandler.HandleSessionSetup(cio, hdr, body, msgBytes)
				if err != nil {
					log.Warn("session-setup error", "err", err)
					return false
				}
				// Fire the post-auth hook once per resolved session (used by the
				// privilege-drop worker). A nil hook is the default no-op path.
				if sess != nil && opts.OnAuthenticated != nil {
					if err := opts.OnAuthenticated(sess); err != nil {
						log.Error("post-auth hook failed; closing connection", "err", err)
						return false
					}
				}
			default:
				if !fd.Dispatch(cio, hdr, body, msgBytes) {
					return false
				}
			}

			if hdr.NextCommand == 0 {
				break
			}
			off = end
		}
		return true
	}

	for {
		// Bound how long a connection may sit without sending a complete frame.
		// Without a deadline a client that opens a socket and stalls (or dribbles
		// a partial NBSS header) pins a goroutine and its buffers indefinitely.
		// SMB clients are chatty — real ones send at least an ECHO keepalive
		// well inside this window — and the deadline is reset per frame.
		_ = c.SetReadDeadline(time.Now().Add(idleTimeout))
		if aborted.Load() {
			return
		}
		frame, err := transport.ReadFrame(br, maxFrame)
		if err != nil {
			switch {
			case aborted.Load():
				// A handler asked for the connection to end; its response is
				// in the writer's queue and teardown will flush it.
			case errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF):
				log.Info("connection closed by peer")
			default:
				log.Warn("read error", "err", err)
			}
			return
		}
		// SMB3 transform header: decrypt before any further handling. This
		// stays on the read goroutine so that the frame the pool receives is
		// always plaintext, which is what lets the check below see whether it
		// is one of the frames that may not run concurrently.
		var encryptResp bool
		if smb3.IsTransform(frame) {
			if conn.Selection.Cipher == 0 {
				log.Warn("transform header received but no cipher negotiated")
				return
			}
			if len(frame) < smb3.TransformHeaderSize {
				log.Warn("transform frame too short", "len", len(frame))
				return
			}
			sessID := binary.LittleEndian.Uint64(frame[44:52])
			sess := sessions.Get(sessID)
			if sess == nil || len(sess.C2SCipherKey) == 0 {
				log.Warn("transform: unknown session or no key", "session_id", sessID)
				return
			}
			plain, _, err := smb3.DecryptTransform(uint16(conn.Selection.Cipher), sess.C2SCipherKey, frame)
			if err != nil {
				log.Warn("transform decrypt failed", "err", err)
				return
			}
			frame = plain
			encryptResp = true
			sess.SetGotEncrypted()
		}

		if frameNeedsSerialExecution(frame) {
			// Run it here, with nothing else in flight.
			pool.drain()
			if !serveFrame(frame, encryptResp) {
				return
			}
			continue
		}

		// ReadFrame hands out a fresh buffer every time, so the worker owns
		// this frame outright and the next read cannot disturb it.
		f, enc := frame, encryptResp
		pool.submit(func() {
			if !serveFrame(f, enc) {
				abort()
			}
		})
	}
}

// remoteIPOf 返回对端地址的 IP 部分，无法解析时返回原字符串。
func remoteIPOf(a net.Addr) string {
	if a == nil {
		return ""
	}
	if ta, ok := a.(*net.TCPAddr); ok {
		return ta.IP.String()
	}
	host, _, err := net.SplitHostPort(a.String())
	if err != nil {
		return a.String()
	}
	return host
}
