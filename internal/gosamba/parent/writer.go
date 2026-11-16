// Gately 修改：仅在 Linux 与 macOS 编译（构建约束）；写协程中的 panic 只断开本连接。

//go:build linux || darwin

package parent

import (
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// Bounds on the outbound queue.
//
// The queue exists so that no request-handling goroutine ever waits on the
// socket: a CHANGE_NOTIFY completion or a blocking-lock completion that lands
// while the peer has stopped reading used to hold the connection's write mutex
// for a whole writeTimeout (two minutes), during which the dispatch loop could
// not answer anything at all. Handing frames to a single writer goroutine moves
// that wait off every producer.
//
// It is bounded both ways, because either bound alone is useless:
//   - by frame count, at twice the credit window, so a client that honours the
//     credits the server grants it can never overflow the queue even when every
//     one of its requests is an async one that draws both an interim
//     STATUS_PENDING and a completion;
//   - by bytes, because a single READ response can be MaxIOSize, and a
//     count-only bound would let a peer that has stopped reading park gigabytes
//     of response buffers in memory.
//
// Crossing either bound means the peer is not draining its socket, and the
// connection is dropped rather than the queue grown.
const (
	writeQueueDepth = 2 * creditWindow
	writeQueueBytes = 64 << 20

	// writerDrainTimeout bounds how long connection teardown waits for the
	// frames still queued to reach a peer that is no longer reading them.
	writerDrainTimeout = 5 * time.Second
)

var (
	// errWriterClosed is returned by send once the connection is tearing down.
	// It is an expected outcome, not a fault: an async completion goroutine
	// that wakes after the connection has gone gets this instead of writing to
	// a dead socket.
	errWriterClosed = errors.New("parent: connection writer closed")

	// errWriteQueueFull means the peer has stopped reading and the connection
	// is being dropped.
	errWriteQueueFull = errors.New("parent: outbound queue full; peer not reading")
)

// connWriter owns the write side of one connection.
//
// Exactly one goroutine (run) touches the socket, so producers never serialize
// against each other and never block on TCP backpressure. The queue channel is
// deliberately never closed: shutdown closes a separate `closed` channel that
// send selects on, which is what makes "a send racing teardown" a returned
// error rather than a panic on a closed channel.
type connWriter struct {
	w   io.Writer
	log *slog.Logger

	// q carries complete NBSS frames, ownership transferred to the writer.
	q chan []byte
	// closed is shut once, by signalClose, to refuse further sends.
	closed chan struct{}
	// done is closed by run when the writer goroutine has returned.
	done chan struct{}
	once sync.Once

	// queued is the total size of the frames currently in q.
	queued atomic.Int64
	// discard tells run to drop the backlog instead of flushing it, set when
	// the socket is already broken or the peer has stopped reading.
	discard atomic.Bool

	// kill forces the underlying socket shut. It is used when the peer has
	// stopped reading: the write in progress must fail now rather than at its
	// two-minute deadline.
	kill func()
}

// newConnWriter starts the writer goroutine for w. kill, if non-nil, must close
// the underlying connection.
func newConnWriter(w io.Writer, log *slog.Logger, kill func()) *connWriter {
	cw := &connWriter{
		w:      w,
		log:    log,
		q:      make(chan []byte, writeQueueDepth),
		closed: make(chan struct{}),
		done:   make(chan struct{}),
		kill:   kill,
	}
	go cw.run()
	return cw
}

// send queues one complete NBSS frame and takes ownership of buf.
//
// It never blocks. When the queue is full the peer is not reading, so the
// connection is dropped instead of the frame being held: a producer that waits
// here is the very stall this type exists to remove.
func (cw *connWriter) send(buf []byte) error {
	if len(buf) == 0 {
		return nil
	}
	select {
	case <-cw.closed:
		return errWriterClosed
	default:
	}

	n := int64(len(buf))
	if cw.queued.Add(n) > writeQueueBytes {
		cw.queued.Add(-n)
		cw.stall("outbound queue over its byte budget")
		return errWriteQueueFull
	}
	select {
	case cw.q <- buf:
		return nil
	case <-cw.closed:
		cw.queued.Add(-n)
		return errWriterClosed
	default:
		cw.queued.Add(-n)
		cw.stall("outbound queue full")
		return errWriteQueueFull
	}
}

// Write implements io.Writer so that the paths which frame their own response
// and expect a plain writer (SESSION_SETUP) go through the same queue instead
// of racing the writer goroutine for the socket.
//
// transport.WriteFrame and WritePreframed each emit a whole frame in a single
// Write, so one Write is one frame. p is copied because an io.Writer may not
// retain its argument.
func (cw *connWriter) Write(p []byte) (int, error) {
	buf := make([]byte, len(p))
	copy(buf, p)
	if err := cw.send(buf); err != nil {
		return 0, err
	}
	return len(p), nil
}

// run is the single goroutine that writes to the socket.
func (cw *connWriter) run() {
	defer close(cw.done)
	// Gately 新增：写协程 panic 时关闭连接，读循环随之结束，不让 panic 结束整个进程
	defer recoverConnPanic(cw.log, "writer", cw.kill)
	for {
		select {
		case buf := <-cw.q:
			if !cw.writeOne(buf) {
				return
			}
		case <-cw.closed:
			cw.flush()
			return
		}
	}
}

// flush writes the frames already queued at shutdown and returns. send refuses
// new frames once closed is shut, so the backlog is finite and this terminates.
// A frame handed over in the moment between send's closed check and its
// enqueue may be left behind; the connection is going away regardless.
func (cw *connWriter) flush() {
	for !cw.discard.Load() {
		select {
		case buf := <-cw.q:
			if !cw.writeOne(buf) {
				return
			}
		default:
			return
		}
	}
}

// writeOne writes one frame, reporting whether the writer may continue.
func (cw *connWriter) writeOne(buf []byte) bool {
	cw.queued.Add(-int64(len(buf)))
	if _, err := cw.w.Write(buf); err != nil {
		if cw.log != nil {
			cw.log.Debug("response write failed; dropping connection", "err", err)
		}
		// The socket is broken: drop the backlog and let the read side notice.
		cw.discard.Store(true)
		cw.signalClose()
		if cw.kill != nil {
			cw.kill()
		}
		return false
	}
	return true
}

// stall tears the connection down because the peer has stopped reading.
func (cw *connWriter) stall(reason string) {
	if cw.log != nil {
		cw.log.Warn("dropping connection: "+reason,
			"queued_bytes", cw.queued.Load(), "depth", writeQueueDepth)
	}
	cw.discard.Store(true)
	cw.signalClose()
	if cw.kill != nil {
		cw.kill()
	}
}

// signalClose refuses further sends. It is idempotent and never waits, so it is
// safe to call from the writer goroutine itself.
func (cw *connWriter) signalClose() { cw.once.Do(func() { close(cw.closed) }) }

// Close stops the writer and waits for its goroutine to exit.
//
// Frames already queued are flushed first: the response that decided to drop
// the connection is typically the last thing in the queue, and the client is
// owed it. The wait is bounded — a peer that is no longer reading must not hold
// this goroutine for a whole two-minute write deadline — after which the socket
// is forced shut, failing the write in progress.
//
// Close is idempotent and safe to call concurrently.
func (cw *connWriter) Close() {
	cw.signalClose()
	t := time.NewTimer(writerDrainTimeout)
	defer t.Stop()
	select {
	case <-cw.done:
	case <-t.C:
		cw.discard.Store(true)
		if cw.kill != nil {
			cw.kill()
		}
		<-cw.done
	}
}

// connIO pairs the connection's buffered reader with the writer goroutine's
// queue, for the handlers that still want a plain io.ReadWriter.
type connIO struct {
	r io.Reader
	w *connWriter
}

func (c *connIO) Read(p []byte) (int, error)  { return c.r.Read(p) }
func (c *connIO) Write(p []byte) (int, error) { return c.w.Write(p) }
