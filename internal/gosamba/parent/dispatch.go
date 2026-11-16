// Gately 修改：按用户解析共享、签名校验改用 Session.verifySigned、签名密钥经 signingKey() 加锁读取（见 parent/gately.go）；仅在 Linux 与 macOS 编译（构建约束）；客户端给出的偏移与长度改用 bounds 包在 uint64 中判断边界；CHANGE_NOTIFY 协程中的 panic 只断开本连接。

//go:build linux || darwin

package parent

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/unix"

	"github.com/wangwanjie/gately/internal/gosamba/config"
	"github.com/wangwanjie/gately/internal/gosamba/inotify"
	"github.com/wangwanjie/gately/internal/gosamba/smb2"
	"github.com/wangwanjie/gately/internal/gosamba/transport"
	"github.com/wangwanjie/gately/internal/gosamba/vfs"

	"github.com/wangwanjie/gately/internal/gosamba/bounds"
)

// Dispatcher routes post-auth SMB2 commands to per-command handlers.
type Dispatcher struct {
	Conn     *Connection
	Sessions *SessionTable
	Shares   []config.ShareConfig
	Log      *slog.Logger

	// Index is the server-scoped session index (see sessionindex.go). The
	// dispatcher needs it for exactly one thing: LOGOFF deletes a session, and
	// an entry left behind would keep a dead SessionId reachable by a later
	// reconnect's PreviousSessionId. Every other removal is the index's own.
	Index *SessionIndex

	// locks is the per-OS byte-range lock manager backing handleLock/handleClose.
	locks *lockManager

	// RequireEncryption / RequireSigning mirror the server security policy.
	// When set, an authenticated (non-guest) session's requests must arrive
	// encrypted / signed respectively; otherwise they are rejected. This is
	// what makes the "required" modes actually enforced on inbound traffic
	// rather than merely advertised in NEGOTIATE.
	RequireEncryption bool
	RequireSigning    bool

	// --- connection-scoped, shared by every per-frame clone ---

	// out is the connection's writer goroutine. Every response is handed to it
	// rather than written inline, so no handler ever waits on the socket. It is
	// nil for a Dispatcher built directly (unit tests drive one handler against
	// a plain buffer), in which case responses are written to the io.Writer the
	// handler was given.
	out *connWriter

	// async is the connection's table of outstanding async requests
	// (CHANGE_NOTIFY and blocking LOCK). Each entry lets CLOSE /
	// TREE_DISCONNECT / LOGOFF complete a pending request (MS-SMB2 §3.3.5.19
	// requires STATUS_NOTIFY_CLEANUP rather than leaving the client waiting
	// forever) and lets SMB2_CANCEL finish the original request instead of
	// being answered with a second response.
	//
	// It is a pointer because ServeConn clones the Dispatcher per inbound
	// frame: the chain state below is per-frame, but this table must stay
	// shared across every frame on the connection.
	async *asyncTable

	// --- per-frame: fresh in every clone forFrame returns ---

	// LastCreatedFileID / HasLastCreated are set by handleCreate and consumed
	// by the chain loop to satisfy "previous handle" FileIDs in compound
	// related ops.
	LastCreatedFileID [16]byte
	HasLastCreated    bool

	// lastChainStatus is the status the most recent op in this compound
	// chain produced. Subsequent related ops referencing the previous-
	// handle sentinel FileID inherit this when it's non-success
	// (MS-SMB2 §3.3.5.2.7).
	lastChainStatus smb2.Status

	// encryptChain records that this frame arrived inside an SMB3 transform
	// header, so every response to it goes back encrypted. forFrame sets it
	// once and nothing writes it afterwards, which is what lets the async
	// completion goroutines read it without synchronization.
	encryptChain bool

	// pending buffers this frame's responses so the members of a compound
	// request can go back as one compounded frame. It is nil for a Dispatcher
	// that is not running a chain, and responses then go straight out.
	pending *pendingResponses
}

// notifyReg is one outstanding async request — a CHANGE_NOTIFY watch or a
// parked blocking LOCK. cancel is closed exactly once (guarded by the async
// table's mutex) to wake the waiting goroutine; status carries the completion
// the canceller wants the client to see.
type notifyReg struct {
	open      *Open
	cancel    chan struct{}
	status    smb2.Status
	cancelled bool
}

// maxPreCancelled bounds the set of MessageIds an SMB2_CANCEL named before the
// request it cancels had registered. A client cannot grow it without bound by
// cancelling ids it never used.
const maxPreCancelled = 256

// asyncTable is one connection's outstanding async requests, keyed by the
// MessageId of the request that created them.
type asyncTable struct {
	mu   sync.Mutex
	regs map[uint64]*notifyReg

	// preCancelled remembers cancels that arrived before their request did.
	//
	// Frames are dispatched concurrently, so an SMB2_CANCEL can overtake the
	// CHANGE_NOTIFY or blocking LOCK it names. Dropping it there would leave
	// the client waiting on a request it has already given up on, so the id is
	// held until the request registers and is then cancelled immediately.
	preCancelled map[uint64]smb2.Status

	// nextID allocates AsyncIds for STATUS_PENDING responses. It is
	// connection-scoped: an AsyncId identifies one outstanding request to the
	// client, so per-frame counters would collide.
	nextID atomic.Uint64
}

// asyncTable returns the connection-scoped table, creating it on first use.
//
// ServeConn installs one before any frame is dispatched, so the lazy path is
// reached only by a Dispatcher built as a bare struct literal (unit tests
// driving a single handler), where the first touch is the synchronous
// registerNotify that precedes the goroutine it is registering for.
func (d *Dispatcher) asyncTable() *asyncTable {
	if d.async == nil {
		d.async = &asyncTable{}
	}
	return d.async
}

// nextAsyncID allocates an AsyncId for an interim STATUS_PENDING response.
func (d *Dispatcher) nextAsyncID() uint64 { return d.asyncTable().nextID.Add(1) }

// registerNotify records an outstanding async request keyed by its MessageId.
// A cancel that arrived first is applied at once.
func (d *Dispatcher) registerNotify(msgID uint64, reg *notifyReg) {
	t := d.asyncTable()
	t.mu.Lock()
	defer t.mu.Unlock()
	if status, ok := t.preCancelled[msgID]; ok {
		delete(t.preCancelled, msgID)
		reg.cancelled = true
		reg.status = status
		close(reg.cancel)
		return
	}
	if t.regs == nil {
		t.regs = make(map[uint64]*notifyReg)
	}
	t.regs[msgID] = reg
}

func (d *Dispatcher) unregisterNotify(msgID uint64) {
	if d.async == nil {
		return
	}
	d.async.mu.Lock()
	defer d.async.mu.Unlock()
	delete(d.async.regs, msgID)
}

// cancelNotify completes one outstanding request with the given status. It
// reports false when there is nothing (yet) to cancel.
func (d *Dispatcher) cancelNotify(msgID uint64, status smb2.Status) bool {
	t := d.asyncTable()
	t.mu.Lock()
	defer t.mu.Unlock()
	reg, ok := t.regs[msgID]
	if !ok {
		// The request has not registered yet — its frame may still be waiting
		// for a worker. Remember the cancel so registerNotify can apply it.
		if len(t.preCancelled) < maxPreCancelled {
			if t.preCancelled == nil {
				t.preCancelled = make(map[uint64]smb2.Status)
			}
			t.preCancelled[msgID] = status
		}
		return false
	}
	if reg.cancelled {
		return false
	}
	reg.cancelled = true
	reg.status = status
	close(reg.cancel)
	return true
}

// cancelNotifiesForOpens completes every request registered against any of the
// given handles. Called when those handles go away so the client is not left
// waiting on a watch whose directory handle no longer exists.
func (d *Dispatcher) cancelNotifiesForOpens(opens []*Open) {
	if len(opens) == 0 || d.async == nil {
		return
	}
	set := make(map[*Open]struct{}, len(opens))
	for _, o := range opens {
		set[o] = struct{}{}
	}
	d.async.mu.Lock()
	defer d.async.mu.Unlock()
	for _, reg := range d.async.regs {
		if reg.cancelled {
			continue
		}
		if _, ok := set[reg.open]; !ok {
			continue
		}
		reg.cancelled = true
		reg.status = smb2.StatusNotifyCleanup
		close(reg.cancel)
	}
}

// notifyStatus reads the completion status a canceller left on reg under the
// async table's lock, so the waiting goroutine does not race the cancel that
// woke it.
func (d *Dispatcher) notifyStatus(reg *notifyReg) smb2.Status {
	t := d.asyncTable()
	t.mu.Lock()
	defer t.mu.Unlock()
	return reg.status
}

// CancelAllNotifies completes every outstanding async request on this
// connection. It is called during connection teardown: the waiting goroutines
// block until an event or a cancel, so without this each abandoned
// CHANGE_NOTIFY would leak a goroutine and its watch descriptors for the life
// of the process.
func (d *Dispatcher) CancelAllNotifies() {
	if d.async == nil {
		return
	}
	d.async.mu.Lock()
	defer d.async.mu.Unlock()
	for _, reg := range d.async.regs {
		if reg.cancelled {
			continue
		}
		reg.cancelled = true
		reg.status = smb2.StatusNotifyCleanup
		close(reg.cancel)
	}
}

// notifyFilterAllows reports whether an event action is one the client asked
// for in its CompletionFilter. A filter of 0 is treated as "everything", which
// is what clients that do not care send.
func notifyFilterAllows(filter, action uint32) bool {
	if filter == 0 {
		return true
	}
	switch action {
	case smb2.FileActionAdded, smb2.FileActionRemoved:
		return filter&(smb2.NotifyFileName|smb2.NotifyDirName) != 0
	case smb2.FileActionRenamedOldName, smb2.FileActionRenamedNewName:
		return filter&(smb2.NotifyFileName|smb2.NotifyDirName) != 0
	case smb2.FileActionModified:
		return filter&(smb2.NotifyLastWrite|smb2.NotifySize|smb2.NotifyAttributes) != 0
	}
	return false
}

// Per-session resource caps. A client that opens handles or trees without ever
// closing them would otherwise exhaust the process's file descriptors; these
// bounds are far above any legitimate workload (Finder and rclone peak in the
// low hundreds of concurrent opens) but keep a runaway or hostile client from
// taking the server down.
const (
	maxOpensPerSession = 4096
	maxTreesPerSession = 256
)

// maxStreamSize bounds an in-memory alternate-data-stream / resource-fork
// buffer. It caps client-driven allocation and, more importantly, keeps a
// wire-supplied stream offset from going negative when narrowed to int. The
// on-disk xattr store imposes its own (smaller) limit at CLOSE.
const maxStreamSize = 64 << 20 // 64 MiB

// encryptionExempt reports whether a command may arrive in cleartext even when
// encryption is required. These carry no confidential share data: TREE_CONNECT
// precedes the client learning the share's encryption policy, and the rest are
// keepalive / teardown / async-control messages that real clients (go-smb2)
// send unencrypted. They are still subject to the signing requirement.
func encryptionExempt(cmd smb2.Command) bool {
	switch cmd {
	case smb2.CommandTreeConnect, smb2.CommandTreeDisconnect, smb2.CommandLogoff,
		smb2.CommandEcho, smb2.CommandCancel, smb2.CommandOplockBreak:
		return true
	}
	return false
}

// forFrame returns the Dispatcher to run one inbound frame's chain on.
//
// The connection-scoped fields — the session table, the async table, the
// writer goroutine, the logger — are all pointers and stay shared. The
// compound-chain state is fresh: the previous-handle FileID, the previous
// member's status, whether this frame arrived encrypted, and the buffer its
// responses accumulate in. That is what lets two frames execute concurrently
// on the connection's worker pool without corrupting each other, which they
// did while this state lived on the one shared Dispatcher.
//
// The copy is safe only because the Dispatcher holds no mutex or atomic of its
// own; anything that must be shared lives behind one of its pointers.
func (d *Dispatcher) forFrame(encrypted bool) *Dispatcher {
	// Materialize the shared table before copying, so clones cannot each
	// lazily create one of their own and lose track of each other's
	// outstanding requests.
	d.asyncTable()
	f := *d
	f.LastCreatedFileID = [16]byte{}
	f.HasLastCreated = false
	f.lastChainStatus = smb2.StatusSuccess
	f.encryptChain = encrypted
	f.pending = &pendingResponses{}
	return &f
}

// hasPreviousHandleSentinel reports whether body's FileID slot holds the
// all-FF "use the previous handle" sentinel.
func hasPreviousHandleSentinel(cmd smb2.Command, body []byte) bool {
	off := fileIDOffsetInBody(cmd)
	if off < 0 || len(body) < off+16 {
		return false
	}
	return [16]byte(body[off:off+16]) == previousHandleFileID
}

// previousHandleFileID is the SMB2 sentinel meaning "use the FileID returned
// by the immediately preceding CREATE in this compound chain".
var previousHandleFileID = [16]byte{
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
}

// fileIDOffsetInBody returns where the 16-byte FileID lives in the body of
// the given command, or -1 if the command has no FileID field.
func fileIDOffsetInBody(cmd smb2.Command) int {
	switch cmd {
	case smb2.CommandClose, smb2.CommandFlush, smb2.CommandIoctl, smb2.CommandLock:
		return 8
	case smb2.CommandRead, smb2.CommandWrite, smb2.CommandSetInfo:
		return 16
	case smb2.CommandQueryInfo:
		return 24
	case smb2.CommandQueryDirectory, smb2.CommandChangeNotify:
		return 8
	}
	return -1
}

// SubstitutePreviousHandleFileID replaces the all-FF sentinel FileID in body
// with the FileID returned by the most recent CREATE in this compound chain.
func (d *Dispatcher) SubstitutePreviousHandleFileID(cmd smb2.Command, body []byte) {
	if !d.HasLastCreated {
		return
	}
	off := fileIDOffsetInBody(cmd)
	if off < 0 || len(body) < off+16 {
		return
	}
	if [16]byte(body[off:off+16]) == previousHandleFileID {
		copy(body[off:off+16], d.LastCreatedFileID[:])
	}
}

// Dispatch handles one frame. Returns false to indicate the caller should
// drop the connection.
func (d *Dispatcher) Dispatch(rw io.ReadWriter, hdr smb2.Header, body, frame []byte) bool {
	sess := d.Sessions.Get(hdr.SessionID)
	if sess == nil {
		d.respondError(rw, hdr, smb2.StatusUserSessionDeleted, nil)
		return false
	}

	// Refuse every command on a session that has not completed SESSION_SETUP.
	// handleType1 registers the session (so the multi-leg NTLM handshake can
	// find it) before any credentials are verified; without this gate an
	// unauthenticated client could TREE_CONNECT to IPC$ and enumerate shares,
	// or reach the DCERPC parser, with only a NEGOTIATE + type-1 sent.
	if !sess.Authenticated {
		d.Log.Warn("command on unauthenticated session — denying",
			"cmd", hdr.Command, "session_id", hdr.SessionID)
		d.respondError(rw, hdr, smb2.StatusAccessDenied, nil)
		return false
	}

	encrypted := d.encryptChain

	// Enforce the server's inbound security policy. Guests carry no session
	// keys (they opted out of per-session crypto), so the requirements apply
	// only to key-bearing (non-guest) sessions; a guest_ok share is an
	// explicit decision to accept unauthenticated traffic.
	// Gately：签名密钥可能被会话认证流程并发写入，经加锁的 signingKey() 读取
	if !sess.IsGuest && len(sess.signingKey()) > 0 {
		// Require encryption for every message that can carry share data. The
		// no-data control commands are exempt: TREE_CONNECT must precede the
		// client learning the share's encryption policy, and ECHO/LOGOFF/
		// TREE_DISCONNECT/CANCEL/OPLOCK_BREAK carry no confidential payload —
		// go-smb2 (rclone) legitimately sends these in the clear. They remain
		// signing-enforced below. A policy miss here is answered ACCESS_DENIED
		// but does not drop the connection: a stray cleartext keepalive must
		// not tear down an otherwise-healthy mount.
		if d.RequireEncryption && !encrypted && !encryptionExempt(hdr.Command) {
			d.Log.Warn("unencrypted request but encryption required — denying",
				"cmd", hdr.Command, "session_id", hdr.SessionID)
			d.respondError(rw, hdr, smb2.StatusAccessDenied, sess)
			return true
		}
		// Signing is subsumed by transform encryption (AEAD authenticates the
		// whole message). For cleartext frames, a signature is mandatory when
		// signing is required, and verified whenever one is present.
		if !encrypted {
			if d.RequireSigning || hdr.Flags&smb2.FlagSigned != 0 {
				if hdr.Flags&smb2.FlagSigned == 0 ||
					!sess.verifySigned(uint16(d.Conn.Selection.SigningAlgo), frame) {
					d.Log.Warn("inbound signature missing or invalid — dropping",
						"cmd", hdr.Command,
						"msg_id", hdr.MessageID,
						"session_id", hdr.SessionID,
						"required", d.RequireSigning,
					)
					d.respondError(rw, hdr, smb2.StatusAccessDenied, sess)
					return false
				}
			}
		}
	}

	// Now that signature verification is done, in a related compound
	// chain replace any "previous handle" sentinel FileID with the one
	// returned by the most recent CREATE. If the previous op failed and
	// this op is referencing that handle, inherit the prior status (per
	// MS-SMB2 §3.3.5.2.7) — otherwise we'd send INVALID_PARAMETER and
	// confuse clients (Finder reads it as a permission fault).
	//
	// The one op that must never be swallowed that way is a CLOSE naming a
	// handle this chain actually opened. macOS sends CREATE/SomeOp/CLOSE as one
	// related chain for almost everything it does, so a failure in the middle
	// used to leave the handle in the session's map with its descriptor, its
	// byte-range locks and its share-mode reservation held until the connection
	// died. In the delete chain (CREATE / SET_INFO FileDispositionInformation /
	// CLOSE) macOS opens deny-all, so that stranded reservation made the file
	// unopenable by every client on every connection.
	//
	// ksmbd draws the line in the same place: it stores the chain's FID only
	// when the CREATE succeeded, resolves a sentinel CLOSE against it and
	// executes the close whatever an earlier member did, then clears the FID so
	// a later sentinel op in the same chain finds nothing
	// (fs/smb/server/smb2pdu.c, smb2_close).
	if hdr.Flags&smb2.FlagRelatedOps != 0 {
		sentinel := hasPreviousHandleSentinel(hdr.Command, body)
		closesLiveHandle := sentinel && d.HasLastCreated && hdr.Command == smb2.CommandClose
		if sentinel && d.lastChainStatus != smb2.StatusSuccess && !closesLiveHandle {
			d.respondError(rw, hdr, d.lastChainStatus, sess)
			return true
		}
		d.SubstitutePreviousHandleFileID(hdr.Command, body)
		if closesLiveHandle {
			// The handle is consumed; a second sentinel op in this chain names
			// nothing and falls back to the inherited-status arm above.
			d.HasLastCreated = false
		}
	}

	// Frames run concurrently, so two messages can now reach the same handle at
	// once. Hold the handle for the length of this message; see
	// lockOpenForMessage for why that is done here rather than per handler.
	if unlock := lockOpenForMessage(sess, hdr.Command, body); unlock != nil {
		defer unlock()
	}

	switch hdr.Command {
	case smb2.CommandTreeConnect:
		return d.handleTreeConnect(rw, hdr, body, sess)
	case smb2.CommandTreeDisconnect:
		return d.handleTreeDisconnect(rw, hdr, body, sess)
	case smb2.CommandCreate:
		return d.handleCreate(rw, hdr, body, sess)
	case smb2.CommandRead:
		return d.handleRead(rw, hdr, body, sess)
	case smb2.CommandWrite:
		return d.handleWrite(rw, hdr, body, sess)
	case smb2.CommandFlush:
		return d.handleFlush(rw, hdr, body, sess)
	case smb2.CommandSetInfo:
		return d.handleSetInfo(rw, hdr, body, sess)
	case smb2.CommandClose:
		return d.handleClose(rw, hdr, body, sess)
	case smb2.CommandQueryDirectory:
		return d.handleQueryDirectory(rw, hdr, body, sess)
	case smb2.CommandQueryInfo:
		return d.handleQueryInfo(rw, hdr, body, sess)
	case smb2.CommandIoctl:
		return d.handleIoctl(rw, hdr, body, sess)
	case smb2.CommandLogoff:
		return d.handleLogoff(rw, hdr, sess)
	case smb2.CommandEcho:
		d.respondSuccess(rw, hdr, sess, []byte{0x04, 0x00, 0x00, 0x00})
		return true
	case smb2.CommandChangeNotify:
		return d.handleChangeNotify(rw, hdr, body, sess)
	case smb2.CommandLock:
		return d.handleLock(rw, hdr, body, sess)
	case smb2.CommandCancel:
		// SMB2_CANCEL never gets a response of its own (MS-SMB2 §3.3.5.16):
		// the cancelled request completes with STATUS_CANCELLED instead.
		// Replying here produced a second response for one MessageId and
		// corrupted the client's outstanding-request table.
		if !d.cancelNotify(hdr.MessageID, smb2.StatusCancelled) {
			d.Log.Debug("cancel for unknown request", "msg_id", hdr.MessageID)
		}
		return true
	case smb2.CommandOplockBreak:
		// MS-SMB2 §3.3.5.22 lists STATUS_INVALID_OPLOCK_PROTOCOL,
		// STATUS_INVALID_PARAMETER and STATUS_FILE_CLOSED as the failures for a
		// break the server cannot match; STATUS_NOT_SUPPORTED is not among
		// them. We grant no oplocks or leases, so no acknowledgement can ever
		// match one.
		d.respondError(rw, hdr, smb2.StatusInvalidOplockProtocol, sess)
		return true
	default:
		d.Log.Warn("unhandled command", "cmd", hdr.Command)
		d.respondError(rw, hdr, smb2.StatusNotSupported, sess)
		return true
	}
}

// lockOpenForMessage locks the handle a message names, for as long as that
// message runs, and returns the matching unlock (nil when there is no handle).
//
// Until this connection served frames in parallel a handle could only be
// touched by one request at a time, and its mutable state — the directory
// enumeration cursor, a named stream's in-memory buffer, a pipe's queued
// DCE/RPC response, the delete-on-close flag — was written with no lock at
// all. Taking the handle's lock in the dispatcher restores exactly that
// ordering for two requests on one handle while leaving requests on different
// handles fully parallel, and does it in one place rather than in every
// handler.
//
// READ and WRITE on an ordinary file handle take the lock shared: they
// pread/pwrite at an explicit offset and touch no field of the Open, and they
// are what a client pipelines hardest, so serializing them would give back
// most of what running frames in parallel just won. A pipe or stream handle is
// served out of those mutable buffers instead of a descriptor, so it is
// excluded from the shared case.
//
// The shared/exclusive decision is made from IsDir, IsPipe and IsStream alone,
// never from open.File. Those three are set once when the handle is created and
// never written again; open.File is not — a release nils it, under this very
// lock. Testing it here, before the lock is taken, is a read of a mutable field
// with no synchronization at all, and it races a teardown that is nilling it at
// that instant. (It was also redundant: File is non-nil for exactly the handles
// these three flags are false for.) Deciding from the immutable flags is what
// makes open.File a field that is only ever read under this lock and only ever
// written under it exclusively. A handle whose descriptor has already gone
// takes the shared path and its handler finds File nil under the lock, which is
// the ordinary "stale FileID" answer.
func lockOpenForMessage(sess *Session, cmd smb2.Command, body []byte) func() {
	off := fileIDOffsetInBody(cmd)
	if off < 0 || len(body) < off+16 {
		return nil
	}
	open := sess.GetOpen([16]byte(body[off : off+16]))
	if open == nil {
		return nil
	}
	if (cmd == smb2.CommandRead || cmd == smb2.CommandWrite) &&
		!open.IsDir && !open.IsPipe && !open.IsStream {
		open.mu.RLock()
		return open.mu.RUnlock
	}
	open.mu.Lock()
	return open.mu.Unlock
}

// respondSuccess emits a STATUS_SUCCESS response with the given body.
func (d *Dispatcher) respondSuccess(rw io.Writer, hdr smb2.Header, sess *Session, body []byte) {
	d.lastChainStatus = smb2.StatusSuccess
	d.emit(rw, sess, d.buildResponse(hdr, smb2.StatusSuccess, body))
}

// respondError emits an error response with a small error body.
func (d *Dispatcher) respondError(rw io.Writer, hdr smb2.Header, status smb2.Status, sess *Session) {
	errBody := []byte{0x09, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	d.lastChainStatus = status
	d.emit(rw, sess, d.buildResponse(hdr, status, errBody))
}

// buildResponse encodes one sync response into a fresh buffer that reserves
// transport.FrameHeaderSize bytes of NBSS headroom at the front, so a response
// that ends up alone in its frame goes on the wire without being copied again.
//
// It deliberately does not sign. A member of a compounded response is signed
// over its NextCommand field and its padding, and neither exists until the
// shape of the whole frame is known; signing happens in sendCompound and
// sealAndSend instead.
func (d *Dispatcher) buildResponse(reqHdr smb2.Header, status smb2.Status, body []byte) []byte {
	respHdr := smb2.Header{
		CreditCharge:   reqHdr.CreditCharge,
		Status:         uint32(status),
		Command:        reqHdr.Command,
		CreditResponse: grantCredits(reqHdr.CreditCharge, reqHdr.CreditResponse),
		Flags:          smb2.FlagServerToRedir,
		MessageID:      reqHdr.MessageID,
		TreeID:         reqHdr.TreeID,
		SessionID:      reqHdr.SessionID,
	}
	out := make([]byte, transport.FrameHeaderSize+smb2.HeaderSize+len(body))
	msg := out[transport.FrameHeaderSize:]
	_ = smb2.EncodeHeader(msg[:smb2.HeaderSize], respHdr)
	copy(msg[smb2.HeaderSize:], body)
	return out
}

// --- TREE_CONNECT ---

func (d *Dispatcher) handleTreeConnect(rw io.ReadWriter, hdr smb2.Header, body []byte, sess *Session) bool {
	req, err := smb2.DecodeTreeConnectRequest(body)
	if err != nil {
		d.Log.Warn("tree-connect decode failed", "err", err)
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	if sess.TreeCount() >= maxTreesPerSession {
		d.Log.Warn("tree-connect: tree limit reached", "limit", maxTreesPerSession, "session_id", sess.ID)
		d.respondError(rw, hdr, smb2.StatusInsufficientResources, sess)
		return true
	}
	// Path is "\\server\share". Extract the trailing share name.
	parts := strings.Split(strings.ReplaceAll(req.Path, "/", "\\"), "\\")
	shareName := parts[len(parts)-1]
	// Don't log routine IPC$ probing — clients hit it constantly.
	if !strings.EqualFold(shareName, "IPC$") {
		d.Log.Debug("tree-connect", "path", req.Path, "share_name", shareName)
	}

	if shareName == "" {
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}

	// IPC$ — accept the connect (Samba does the same; refusing causes some
	// clients to retry forever). Unsupported pipes are rejected at the
	// CREATE step with OBJECT_NAME_NOT_FOUND, which matches Samba's
	// smb2_create.c behavior for unknown pipe names.
	if strings.EqualFold(shareName, "IPC$") {
		ipc := config.ShareConfig{Name: "IPC$"}
		tree := sess.AddTree(ipc)
		resp := smb2.EncodeTreeConnectResponse(smb2.TreeConnectResponse{
			ShareType:     smb2.ShareTypePipe,
			ShareFlags:    0,
			Capabilities:  0,
			MaximalAccess: 0x001F00A9,
		})
		d.respondSuccessWithTreeID(rw, hdr, sess, resp, tree.ID)
		return true
	}

	var share *config.ShareConfig
	shares := d.sharesFor(sess)
	for i := range shares {
		if strings.EqualFold(shares[i].Name, shareName) {
			share = &shares[i]
			break
		}
	}
	if share == nil {
		d.Log.Warn("tree-connect: unknown share", "share_name", shareName, "configured_shares", shareNames(shares))
		d.respondError(rw, hdr, smb2.StatusObjectNameNotFound, sess)
		return true
	}

	// Guests can only mount shares marked guest_ok.
	if sess.IsGuest && !share.GuestOK {
		d.Log.Warn("guest denied non-guest share", "share_name", shareName)
		d.respondError(rw, hdr, smb2.StatusAccessDenied, sess)
		return true
	}

	// Check user is allowed (named users only — guest passes via guest_ok).
	if !sess.IsGuest && !shareAllowed(sess.User, share.Name) {
		d.respondError(rw, hdr, smb2.StatusAccessDenied, sess)
		return true
	}

	tree := sess.AddTree(*share)
	// SMB2 caching flags: MANUAL (0x0) for read-only, AUTO (0x10) for read-write.
	// Apple clients write-back-cache aggressively under AUTO, which surfaces as
	// phantom EACCES on RO shares — MANUAL avoids that.
	shareFlags := uint32(0x00000010) // AUTO_CACHING
	if share.ReadOnly {
		shareFlags = 0x00000000 // MANUAL_CACHING
	}
	if d.RequireEncryption {
		// SMB2_SHAREFLAG_ENCRYPT_DATA: tell the client that traffic on this
		// tree must be encrypted. Clients (go-smb2/rclone included) encrypt
		// per-tree based on this flag, not on the global NEGOTIATE cap alone;
		// without it they keep sending cleartext and our inbound encryption
		// check would reject every post-TREE_CONNECT request.
		shareFlags |= 0x00008000
	}
	maximalAccess := uint32(0x001F01FF) // generic all
	if share.ReadOnly {
		maximalAccess = 0x001200A9 // read + execute
	}
	respBody := smb2.EncodeTreeConnectResponse(smb2.TreeConnectResponse{
		ShareType:     smb2.ShareTypeDisk,
		ShareFlags:    shareFlags,
		Capabilities:  0,
		MaximalAccess: maximalAccess,
	})
	d.respondSuccessWithTreeID(rw, hdr, sess, respBody, tree.ID)
	return true
}

func shareNames(shares []config.ShareConfig) []string {
	names := make([]string, len(shares))
	for i, s := range shares {
		names[i] = s.Name
	}
	return names
}

func shareNamesAndPaths(shares []config.ShareConfig) []string {
	out := make([]string, len(shares))
	for i, s := range shares {
		out[i] = s.Name + "=" + s.Path
	}
	return out
}

// syntheticDirEntry stands in for "." and ".." in directory listings.
type syntheticDirEntry struct {
	name string
	info os.FileInfo
}

func (s syntheticDirEntry) Name() string      { return s.name }
func (s syntheticDirEntry) IsDir() bool       { return true }
func (s syntheticDirEntry) Type() os.FileMode { return os.ModeDir }

// Info never hands back a nil os.FileInfo. The "." / ".." entries are built
// from an os.Lstat that can fail (the directory can be renamed or removed
// between the ReadDir and the stat), and a nil interface here would be
// dereferenced by encodeDirRecord — panicking the whole server process, since
// nothing in the serve path recovers. Reporting an error instead makes the
// enumerator skip the entry.
func (s syntheticDirEntry) Info() (os.FileInfo, error) {
	if s.info == nil {
		return nil, os.ErrNotExist
	}
	return s.info, nil
}

func (d *Dispatcher) respondSuccessWithTreeID(rw io.Writer, hdr smb2.Header, sess *Session, body []byte, treeID uint32) {
	hdr.TreeID = treeID
	d.lastChainStatus = smb2.StatusSuccess
	d.emit(rw, sess, d.buildResponse(hdr, smb2.StatusSuccess, body))
}

func shareAllowed(u config.UserConfig, shareName string) bool {
	for _, a := range u.AllowShares {
		if a == "*" || strings.EqualFold(a, shareName) {
			return true
		}
	}
	return false
}

// visibleShares returns the shares sess is permitted to connect to, applying
// exactly the rules handleTreeConnect enforces. Share enumeration must not
// reveal shares the caller could not mount: without this filter NetShareEnumAll
// listed every configured share — including their names — to any authenticated
// user, regardless of their allow_shares list.
func visibleShares(all []config.ShareConfig, sess *Session) []config.ShareConfig {
	if sess == nil {
		return nil
	}
	out := make([]config.ShareConfig, 0, len(all))
	for _, s := range all {
		if sess.IsGuest {
			if s.GuestOK {
				out = append(out, s)
			}
			continue
		}
		if shareAllowed(sess.User, s.Name) {
			out = append(out, s)
		}
	}
	return out
}

// --- TREE_DISCONNECT ---

// releaseOpens closes a batch of handles: byte-range locks first, then the
// descriptor, and finally the durable-table entry so a disconnected tree or
// session cannot be reclaimed and does not pin an fd.
//
// Every caller reached these handles by removing them from their Session's
// opens map, which is what makes this run exactly once per handle — see the
// ownership rule on sessionHost.closeSession. Nothing here is a "release if
// still held" check, and nothing here may be skipped: a share-mode reservation
// nobody releases makes the file permanently unopenable by every client.
//
// Each handle is held exclusively for its own release. Open.mu is the lock the
// dispatcher takes for the length of every message naming that handle
// (lockOpenForMessage), so taking it here is how a release waits for an
// in-flight READ, WRITE or QUERY_DIRECTORY to finish before the descriptor
// behind it is closed. That matters most for a cross-connection teardown, where
// the requests still in flight belong to a different connection's worker pool
// and there is no drain to wait on.
func (d *Dispatcher) releaseOpens(opens []*Open) {
	// Complete any CHANGE_NOTIFY or parked blocking LOCK still registered
	// against these handles FIRST, for two reasons: the client gets
	// STATUS_NOTIFY_CLEANUP rather than waiting on a dead handle, and a parked
	// request is woken before this loop asks for its handle — a blocking LOCK
	// retries under Open.mu, so waking it is what keeps the wait below short.
	d.cancelNotifiesForOpens(opens)
	for _, o := range opens {
		if o == nil {
			continue
		}
		unlock := lockOpenForRelease(o)
		// A server-side-copy resume key is a capability naming this handle; it
		// must not outlive it on any teardown path.
		if d.Conn != nil {
			d.Conn.resumeKeys.release(o)
		}
		// Give up the share-mode reservation first: these handles are gone for
		// good (the tree, session or connection that owned them is being torn
		// down), and a reservation nobody can ever release makes the file
		// permanently unopenable by every client.
		sharedShareModes.release(o)
		if o.IsDurable && d.Conn != nil && d.Conn.Durable != nil {
			d.Conn.Durable.Remove(o.DurableClientGuid, o.DurableCreateGuid)
		}
		if o.File != nil {
			if d.locks != nil {
				d.locks.releaseAll(o)
			}
			o.File.Close()
			o.File = nil
		}
		unlock()
	}
}

// lockOpenForRelease takes o's per-handle lock so the release that follows does
// not run underneath a request still using the descriptor, and returns the
// matching unlock.
//
// The wait is unbounded on purpose, and that is safe because nothing ever holds
// Open.mu across an unbounded wait. The dispatcher holds it for the length of
// one message; the two commands that would park — CHANGE_NOTIFY and a blocking
// LOCK — register their wait and return, so neither holds it while parked
// (releaseOpens wakes both before it reaches this loop anyway); and no handler
// ever waits on the socket, which belongs to the writer goroutine. CREATE is a
// third command that can park, on a sharing conflict, but that wait is
// upstream of this lock: fileIDOffsetInBody(CommandCreate) is -1, so
// lockOpenForMessage returns nil for CREATE, and handleCreate does not take
// open.mu until after acquireShareMode has already returned. A future change
// must keep that ordering — moving the wait below open.mu.Lock() would let a
// parked CREATE hold the lock for up to sharingViolationWait. So the longest
// this can wait is one message's worth of file I/O.
//
// A bounded wait was tried and removed. Giving up and releasing anyway means
// writing o.File while a reader may still be in the handler — a data race the
// detector finds, on the one field whose whole safety argument is that it is
// written only under this lock. Giving up and NOT releasing leaks the
// descriptor, the byte-range locks and the share-mode reservation, and a leaked
// reservation makes the file unopenable by every client on every connection for
// the life of the process. Neither is better than waiting for an I/O that is
// going to finish.
//
// No caller may already hold this lock: releaseOpens is reached only from
// TREE_DISCONNECT, LOGOFF and a session teardown, none of which carries a
// FileID, so lockOpenForMessage took nothing; releaseConnOpens runs after the
// pool has drained. Each handle appears once in a release batch, because the
// batch came out of a map.
func lockOpenForRelease(o *Open) func() {
	o.mu.Lock()
	return o.mu.Unlock
}

func (d *Dispatcher) handleTreeDisconnect(rw io.ReadWriter, hdr smb2.Header, body []byte, sess *Session) bool {
	if _, err := smb2.DecodeTreeDisconnectRequest(body); err != nil {
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	// Close every handle opened on this tree — otherwise its fds and locks
	// survive until the whole connection drops.
	d.releaseOpens(sess.RemoveTreeAndOpens(hdr.TreeID))
	d.respondSuccess(rw, hdr, sess, smb2.EncodeTreeDisconnectResponse())
	return true
}

// handleLogoff tears the session down: every open is closed and the SessionId
// is invalidated. Previously LOGOFF only sent a success response, leaving the
// id usable and every descriptor and lock in place.
func (d *Dispatcher) handleLogoff(rw io.ReadWriter, hdr smb2.Header, sess *Session) bool {
	d.releaseOpens(sess.TakeAllOpens())
	d.respondSuccess(rw, hdr, sess, []byte{0x04, 0x00, 0x00, 0x00})
	if d.Sessions != nil {
		d.Sessions.Remove(sess.ID)
	}
	// And out of the server-scoped index, or a later reconnect naming this id
	// in PreviousSessionId would find a session that no longer exists on any
	// connection — and the entry would outlive the server's memory of it.
	d.Index.unregister(sess.ID)
	return true
}

// --- CREATE ---

func (d *Dispatcher) handleCreate(rw io.ReadWriter, hdr smb2.Header, body []byte, sess *Session) bool {
	req, err := smb2.DecodeCreateRequest(body)
	if err != nil {
		d.Log.Warn("create decode failed", "err", err)
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	tree := sess.GetTree(hdr.TreeID)
	if tree == nil {
		d.Log.Warn("create: unknown tree", "tree_id", hdr.TreeID)
		d.respondError(rw, hdr, smb2.StatusNetworkNameDeleted, sess)
		return true
	}
	if sess.OpenCount() >= maxOpensPerSession {
		d.Log.Warn("create: open limit reached", "limit", maxOpensPerSession, "session_id", sess.ID)
		d.respondError(rw, hdr, smb2.StatusInsufficientResources, sess)
		return true
	}
	d.Log.Debug("create",
		"tree", tree.Share.Name,
		"name", req.Name,
		"disposition", req.CreateDisposition,
		"options", req.CreateOptions,
		"access", req.DesiredAccess,
	)

	// Parse durable-handle and lease create contexts up front. A reconnect
	// (DH2C/DHnC) short-circuits the normal CREATE path: we reclaim the saved
	// Open and re-open its backing file rather than re-running disposition.
	durReq, durRec, leaseReq := parseDurableContexts(req.CreateContexts)
	if durRec.present && tree.Share.Path != "" {
		if d.handleDurableReconnect(rw, hdr, sess, tree, durRec, leaseReq) {
			return true
		}
		// Reclaim failed/expired → OBJECT_NAME_NOT_FOUND so the client
		// re-opens fresh (MS-SMB2 §3.3.5.9.7/.12).
		d.respondError(rw, hdr, smb2.StatusObjectNameNotFound, sess)
		return true
	}

	// IPC$ tree: open virtual pipe handles. We support \PIPE\srvsvc (share
	// enumeration); other pipes return OBJECT_NAME_NOT_FOUND so the client
	// stops retrying.
	if tree.Share.Path == "" {
		pipeName := strings.TrimPrefix(strings.TrimPrefix(req.Name, "\\"), "PIPE\\")
		pipeName = strings.TrimPrefix(pipeName, "pipe\\")
		switch strings.ToLower(pipeName) {
		case "srvsvc":
			open := &Open{
				Tree:     tree,
				IsPipe:   true,
				PipeName: "srvsvc",
			}
			if _, err := rand.Read(open.FileID[:]); err != nil {
				d.respondError(rw, hdr, smb2.StatusInternalError, sess)
				return true
			}
			// Held for the same reason as the file arm below: once AddOpen
			// returns, a teardown can claim this handle, and the response is
			// still being built out of it.
			open.mu.Lock()
			defer open.mu.Unlock()
			if !sess.AddOpen(open) {
				// The session was torn down under this CREATE. A pipe handle
				// holds no descriptor and no reservation, so there is nothing
				// to give back — just tell the client its session is gone.
				d.respondError(rw, hdr, smb2.StatusUserSessionDeleted, sess)
				return true
			}
			d.LastCreatedFileID = open.FileID
			d.HasLastCreated = true
			now := filetimeFromTime(time.Now())
			resp := smb2.EncodeCreateResponse(smb2.CreateResponse{
				CreateAction:   smb2.CreateActionOpened,
				CreationTime:   now,
				LastAccessTime: now,
				LastWriteTime:  now,
				ChangeTime:     now,
				FileAttributes: smb2.FileAttrNormal,
				FileID:         open.FileID,
			})
			d.respondSuccess(rw, hdr, sess, resp)
			return true
		default:
			d.respondError(rw, hdr, smb2.StatusObjectNameNotFound, sess)
			return true
		}
	}

	// A timewarp token asks for the file as it was at a point in time. This
	// server keeps no previous versions, so MS-SMB2 §3.3.5.9.5 requires the
	// CREATE to fail with STATUS_NOT_FOUND rather than quietly serving the live
	// file — which is what a snapshot-mounted macOS client would otherwise
	// browse and copy while its UI claims to be showing the snapshot. The
	// refusal is a status only: a timewarp RESPONSE context would hit the
	// client's unknown-name arm and fail the CREATE with EBADRPC instead
	// (see timewarp.go).
	if hasTimewarpContext(req.CreateContexts) {
		d.Log.Debug("create: timewarp token refused (no previous versions)",
			"tree", tree.Share.Name, "name", req.Name)
		d.respondError(rw, hdr, smb2.StatusNotFound, sess)
		return true
	}

	baseName, streamName, streamOK := splitStreamName(req.Name)
	if !streamOK {
		// Non-$DATA stream type (e.g. $INDEX_ALLOCATION) — Samba returns
		// EINVAL here, which maps to STATUS_INVALID_PARAMETER.
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	if streamName != "" {
		return d.handleCreateNamedStream(rw, hdr, sess, tree, req, baseName, streamName)
	}

	// Use normalization-insensitive resolution so an NFD-named file (created
	// on macOS) can be found by an NFC lookup (Windows/Linux), and vice-versa.
	// ResolveSecureNorm still enforces symlink-containment inside the share.
	osPath, err := vfs.ResolveSecureNorm(tree.Share.Path, baseName, shareFoldsCase(tree))
	if err != nil {
		d.respondError(rw, hdr, statusFromResolveErr(err), sess)
		return true
	}
	// Gately 补丁：被屏蔽的路径（共享目录中的 Gately 配置目录）按不存在处理，见 gately_guard.go。
	// 判断会解析符号链接，指向配置目录的链接同样被拒绝
	if shareHidden(tree, osPath) {
		d.respondError(rw, hdr, smb2.StatusObjectNameNotFound, sess)
		return true
	}
	wantedDir := req.CreateOptions&smb2.CreateOptDirectoryFile != 0
	wantedNonDir := req.CreateOptions&smb2.CreateOptNonDirFile != 0
	wantsWrite := req.DesiredAccess&(smb2.AccessFileWriteData|smb2.AccessFileAppendData|smb2.AccessGenericAll|smb2.AccessGenericWrite) != 0

	// Enforce read-only share restrictions before any mutation.
	if tree.Share.ReadOnly {
		// Any disposition other than plain Open (=1) would create, overwrite, or
		// supersede — deny all of them.
		if req.CreateDisposition != smb2.CreateDispositionOpen {
			d.respondError(rw, hdr, smb2.StatusAccessDenied, sess)
			return true
		}
		// Deny write/append/delete access bits, generic-write/all.
		const writeMask = smb2.AccessFileWriteData | smb2.AccessFileAppendData |
			smb2.AccessGenericWrite | smb2.AccessGenericAll |
			0x00000100 | // FILE_WRITE_ATTRIBUTES
			0x00010000 // DELETE
		if req.DesiredAccess&writeMask != 0 {
			d.respondError(rw, hdr, smb2.StatusAccessDenied, sess)
			return true
		}
		// DELETE_ON_CLOSE would silently remove the file on close.
		if req.CreateOptions&smb2.CreateOptDeleteOnClose != 0 {
			d.respondError(rw, hdr, smb2.StatusAccessDenied, sess)
			return true
		}
		// mkdir on a read-only share: DirectoryFile + any creating disposition.
		// (We already rejected non-Open dispositions above, so this is covered;
		// guard here for clarity if the logic above ever changes.)
		if wantedDir && req.CreateDisposition != smb2.CreateDispositionOpen {
			d.respondError(rw, hdr, smb2.StatusAccessDenied, sess)
			return true
		}
	}

	st, statErr := os.Lstat(osPath)
	exists := statErr == nil
	isDir := exists && st.IsDir()
	// linkPath stays empty unless the branch below actually traverses an
	// in-share symlink; see Open.LinkPath for why the empty case matters.
	var linkPath string
	if !exists && !os.IsNotExist(statErr) {
		d.Log.Warn("create: lstat failed", "path", osPath, "err", statErr)
		d.respondError(rw, hdr, statusFromErr(statErr), sess)
		return true
	}

	// An in-share symlink is served as the file it points at.
	//
	// ResolveSecureNorm deliberately returns the LEXICAL path of a symlink, so
	// without this every call below would run against the link itself: the open
	// is O_NOFOLLOW, which turns into ELOOP, and Lstat reports the link's own
	// size (the length of the target path). That combination is exactly the
	// reported symptom — the server does not claim FILE_SUPPORTS_REPARSE_POINTS,
	// so macOS never sets FILE_OPEN_REPARSE_POINT and takes the
	// FILE_ATTRIBUTE_NORMAL answer at face value; Finder lists the link as an
	// ordinary small file and every open of it is refused.
	//
	// Containment is re-proven here rather than inherited from the resolve
	// above, so a link repointed outside the share since then is still refused.
	//
	// The target becomes the path of the whole handle, not just of the open:
	// QUERY_INFO, READ, WRITE and a durable reclaim all work from open.Path,
	// and a handle whose metadata described the link while its data came from
	// the target would be a worse bug than the one being fixed.
	//
	// The link itself is NOT forgotten, though. It is kept in open.LinkPath,
	// because the handle's data and the handle's name refer to different objects
	// from here on: the data is the target's, the name is still the link's.
	// DELETE_ON_CLOSE and FileRenameInformation are the two operations that act
	// on the name, and running them against the target made deleting a symlink
	// over SMB delete the file it pointed at. POSIX splits it in exactly this
	// place — open(2) follows the final link, unlink(2) and rename(2) do not —
	// and so does this server, via Open.namePath().
	if exists && st.Mode()&os.ModeSymlink != 0 {
		target, lerr := vfs.ResolveLink(tree.Share.Path, osPath)
		if lerr != nil {
			d.Log.Debug("create: symlink not followed", "path", osPath, "err", lerr)
			d.respondError(rw, hdr, statusFromResolveErr(lerr), sess)
			return true
		}
		tst, terr := os.Lstat(target)
		if terr != nil {
			d.Log.Warn("create: lstat of symlink target failed", "path", target, "err", terr)
			d.respondError(rw, hdr, statusFromErr(terr), sess)
			return true
		}
		linkPath = osPath
		osPath, st, isDir = target, tst, tst.IsDir()
	}

	// Directory/non-directory validation, BEFORE the disposition switch.
	//
	// It used to run after it, and the switch creates before it checks: a
	// DIRECTORY_FILE create for a name that did not exist was created as a
	// regular FILE by the OVERWRITE_IF/SUPERSEDE arm and only then refused with
	// STATUS_NOT_A_DIRECTORY, leaving a zero-byte file behind from a request
	// that failed. Nothing in these checks needs the switch's result: the stat
	// is already done, and for a name that does not exist the answer depends
	// only on the request.
	if wantedDir && wantedNonDir {
		// MS-SMB2 §3.3.5.9: the two options are mutually exclusive. Refusing
		// here also stops the same stray-object bug in its other form, where
		// FILE_CREATE mkdir'd the directory and the NON_DIRECTORY_FILE check
		// then failed the request with the new directory still on disk.
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	if wantedDir && truncatingDisposition(req.CreateDisposition) {
		// MS-FSA §2.1.5.1: a directory is only ever brought into existence by
		// FILE_CREATE or FILE_OPEN_IF. Truncation has no meaning for one, and
		// the OVERWRITE_IF/SUPERSEDE arms below cannot honour DIRECTORY_FILE —
		// they would create a regular file.
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	// FILE_CREATE is left to the switch: an existing name must be reported as
	// STATUS_OBJECT_NAME_COLLISION whatever its type, and that arm creates
	// nothing when the name is taken, so it cannot leave anything behind.
	if exists && req.CreateDisposition != smb2.CreateDispositionCreate {
		if wantedDir && !isDir {
			d.respondError(rw, hdr, smb2.StatusNotADirectory, sess)
			return true
		}
		if wantedNonDir && isDir {
			d.respondError(rw, hdr, smb2.StatusFileIsADirectory, sess)
			return true
		}
	}

	// Share-access pre-check (MS-SMB2 §3.3.5.9). The authoritative test is the
	// acquire further down, which runs against the fd we actually opened and is
	// atomic with recording our own reservation. This earlier pass exists only
	// so that a CREATE which is going to be refused cannot destroy data on its
	// way out: the three truncating dispositions empty the file below, before
	// any descriptor is opened, so without it an open that loses the sharing
	// check would still have wiped a file another client holds exclusively.
	//
	// It is deliberately limited to those dispositions. Running it over every
	// CREATE would replace more specific answers with STATUS_SHARING_VIOLATION —
	// FILE_CREATE on an existing name must still report
	// STATUS_OBJECT_NAME_COLLISION — and the non-truncating dispositions have
	// nothing to protect, since they do not touch the file before the acquire.
	//
	// This check deliberately does NOT wait out a transient conflict the way
	// the acquire below does. It reserves nothing, so several CREATEs parked
	// here would all be woken by the same release, all re-run this check
	// against a now-empty entry list, all pass, and all truncate the file —
	// after which only one of them would go on to win the acquire, having
	// already destroyed the data on its way to being refused. Absorbing the
	// wait belongs solely in the authoritative acquire, whose test-and-reserve
	// is atomic. (The macOS delete chain this package's sharing-wait exists for
	// opens with FILE_OPEN, not a truncating disposition, so nothing that wait
	// was built for depends on this pre-check waiting too.)
	if truncatingDisposition(req.CreateDisposition) && exists && !isDir {
		if key, ok := shareKeyForPath(osPath); ok &&
			!sharedShareModes.check(key, req.DesiredAccess, req.ShareAccess) {
			d.respondError(rw, hdr, smb2.StatusSharingViolation, sess)
			return true
		}
	}

	// Apply disposition logic.
	createAction := uint32(smb2.CreateActionOpened)
	switch req.CreateDisposition {
	case smb2.CreateDispositionOpen:
		if !exists {
			d.respondError(rw, hdr, smb2.StatusObjectNameNotFound, sess)
			return true
		}
	case smb2.CreateDispositionCreate:
		if exists {
			d.respondError(rw, hdr, smb2.StatusObjectNameCollision, sess)
			return true
		}
		if wantedDir {
			if err := os.Mkdir(osPath, 0775); err != nil {
				d.Log.Warn("create: mkdir failed", "path", osPath, "err", err)
				d.respondError(rw, hdr, statusFromErr(err), sess)
				return true
			}
			isDir = true
		} else {
			f, err := os.OpenFile(osPath, os.O_RDWR|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0664)
			if err != nil {
				d.Log.Warn("create: open EXCL failed", "path", osPath, "err", err)
				d.respondError(rw, hdr, statusFromErr(err), sess)
				return true
			}
			f.Close()
		}
		st, _ = os.Lstat(osPath)
		exists = true
		createAction = smb2.CreateActionCreated
	case smb2.CreateDispositionOpenIf:
		if !exists {
			if wantedDir {
				if err := os.Mkdir(osPath, 0775); err != nil {
					d.respondError(rw, hdr, statusFromErr(err), sess)
					return true
				}
				isDir = true
			} else {
				f, err := os.OpenFile(osPath, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0664)
				if err != nil {
					d.respondError(rw, hdr, statusFromErr(err), sess)
					return true
				}
				f.Close()
			}
			st, _ = os.Lstat(osPath)
			exists = true
			createAction = smb2.CreateActionCreated
		}
	case smb2.CreateDispositionOverwrite:
		if !exists {
			d.respondError(rw, hdr, smb2.StatusObjectNameNotFound, sess)
			return true
		}
		if !isDir {
			if err := os.Truncate(osPath, 0); err != nil {
				d.respondError(rw, hdr, statusFromErr(err), sess)
				return true
			}
		}
		createAction = smb2.CreateActionOverwritten
		st, _ = os.Lstat(osPath)
	case smb2.CreateDispositionOverwriteIf:
		if exists && !isDir {
			if err := os.Truncate(osPath, 0); err != nil {
				d.respondError(rw, hdr, statusFromErr(err), sess)
				return true
			}
			createAction = smb2.CreateActionOverwritten
		} else if !exists {
			f, err := os.OpenFile(osPath, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0664)
			if err != nil {
				d.respondError(rw, hdr, statusFromErr(err), sess)
				return true
			}
			f.Close()
			createAction = smb2.CreateActionCreated
		}
		st, _ = os.Lstat(osPath)
		exists = true
	case smb2.CreateDispositionSupersede:
		// FILE_SUPERSEDE is not FILE_OVERWRITE_IF. It shared that arm, so the
		// response claimed FILE_WAS_OVERWRITTEN (3) for what the client asked
		// to have superseded (0). macOS records the action without branching on
		// it today, so this is cosmetic there and correctness for every client
		// that does read it.
		//
		// Like Samba, superseding an existing file is implemented as a
		// truncation rather than an unlink-and-recreate: the inode survives, so
		// the FileId already handed out stays valid and no other handle to the
		// file is broken. There are no DOS attributes to discard along with the
		// contents — this server derives FileAttributes from the filesystem
		// (directory or normal) and stores none of its own — so the reset the
		// spec describes is a no-op here rather than something skipped.
		if exists && !isDir {
			if err := os.Truncate(osPath, 0); err != nil {
				d.respondError(rw, hdr, statusFromErr(err), sess)
				return true
			}
			createAction = smb2.CreateActionSuperseded
		} else if !exists {
			f, err := os.OpenFile(osPath, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0664)
			if err != nil {
				d.respondError(rw, hdr, statusFromErr(err), sess)
				return true
			}
			f.Close()
			createAction = smb2.CreateActionCreated
		}
		st, _ = os.Lstat(osPath)
		exists = true
	default:
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	// The wantedDir/wantedNonDir checks that used to sit here now run before
	// the switch, so a request that is going to be refused for asking for the
	// wrong object type no longer creates the object first.

	// Granted-access mask, reported back both in the MxAc create context and
	// via FileAccessInformation / FileAllInformation.
	//
	// It is deliberately NOT narrowed to what the client asked for in CREATE —
	// Samba reports the maximum too, and macOS refuses to LIST a directory
	// whose mask lacks FILE_LIST_DIRECTORY (= FILE_READ_DATA) even when the
	// CREATE itself succeeded.
	//
	// It IS narrowed to what this object's POSIX permissions really grant. For
	// macOS the MxAc mask is the only input to smbfs_vnop_access(): the client
	// never asks for FileAccessInformation separately, so a share-wide constant
	// made access(W_OK) answer true on a mode 0444 file (the write then failing
	// with EACCES) and marked every file on the share executable — the AAPL
	// reply declares the server UNIX-based, so the client takes the execute bit
	// at face value instead of falling back to "readable implies executable".
	// That also contradicted the POSIX mode the AAPL directory overlay ships,
	// so ls -l and access() disagreed about the same file.
	//
	// The share's read-only flag remains the ceiling; permissions only subtract
	// from it, so a read-only share still reports no write access for a mode
	// 0666 file.
	granted := maximalAccess(osPath, tree.Share.ReadOnly)

	open := &Open{
		Path:          osPath,
		LinkPath:      linkPath,
		IsDir:         isDir,
		Tree:          tree,
		GrantedAccess: granted,
		DeleteOnClose: req.CreateOptions&smb2.CreateOptDeleteOnClose != 0,
	}
	if !isDir {
		// Always open RW when possible so that later SET_INFO (truncate / EOF)
		// or rename ops on this handle don't fail with EBADF. If RW fails
		// (e.g. read-only filesystem), fall back to read-only when the client
		// only asked for read-only access.
		// O_NOFOLLOW on the leaf closes the TOCTOU window where a symlink
		// swapped in after ResolveSecure would otherwise be followed silently.
		f, err := os.OpenFile(osPath, os.O_RDWR|syscall.O_NOFOLLOW, 0)
		if err != nil && !wantsWrite && !errors.Is(err, syscall.ELOOP) {
			// Read-only fallback. ELOOP is excluded because the retry would
			// fail exactly the same way — O_NOFOLLOW refuses a symlink whatever
			// the access mode.
			f, err = os.OpenFile(osPath, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
		}
		if err != nil {
			// ELOOP can only mean a symlink was swapped in after the resolve
			// and the in-share re-resolve above, since osPath is the resolved
			// target by now. It is reported through statusFromErr like every
			// other errno, which maps it to STATUS_OBJECT_PATH_NOT_FOUND —
			// the same answer the resolve gives for a link it cannot follow.
			// The two used to disagree, one saying ACCESS_DENIED and the other
			// OBJECT_PATH_NOT_FOUND for the same unopenable symlink.
			d.respondError(rw, hdr, statusFromErr(err), sess)
			return true
		}
		open.File = f
	}
	if _, err := rand.Read(open.FileID[:]); err != nil {
		if open.File != nil {
			open.File.Close()
		}
		d.respondError(rw, hdr, smb2.StatusInternalError, sess)
		return true
	}

	// Reserve this handle's share mode (MS-SMB2 §3.3.5.9). This is the
	// authoritative check: it is keyed off the descriptor we just opened, so a
	// rename racing the resolve cannot make us reserve the wrong file, and the
	// test-and-insert is atomic so two clients racing on the same file cannot
	// both be admitted.
	//
	// It happens after the FileID is minted because every step from here to the
	// response is infallible — there is no path that could drop the handle and
	// leave the reservation behind.
	if shareModeApplies(open) {
		key, keyOK := shareKeyForFd(int(open.File.Fd()))
		if keyOK && !d.acquireShareMode(key, open, req.DesiredAccess, req.ShareAccess) {
			// Refused: hand back the descriptor before answering, or the fd
			// leaks for the life of the process.
			open.File.Close()
			open.File = nil
			d.Log.Debug("create refused: sharing violation",
				"path", osPath, "access", req.DesiredAccess, "share_access", req.ShareAccess)
			d.respondError(rw, hdr, smb2.StatusSharingViolation, sess)
			return true
		}
	}
	// Hold the handle from the instant before it becomes reachable until this
	// CREATE has finished building its response out of it.
	//
	// AddOpen is the publication point: the moment it returns, the handle is in
	// the session's map and a teardown on any connection can claim it and
	// release it. Everything below still uses it — it fstats open.File for the
	// QFid response, and applyDurableAndLease writes IsDurable and the durable
	// GUIDs onto it — so without this the release's "o.File = nil" races this
	// function's own reads, and its IsDurable write races the release's read of
	// the same field. Open.mu is the lock releaseOpens takes per handle, so
	// holding it here makes a teardown wait for this CREATE to finish rather
	// than dismantle the handle half-way through it.
	//
	// The lock is taken BEFORE AddOpen, not after, so there is no instant in
	// which the handle is reachable and unheld. Nothing else can be holding it:
	// this Open was allocated a few lines ago and no other goroutine has ever
	// seen it.
	open.mu.Lock()
	defer open.mu.Unlock()
	if !sess.AddOpenToTree(open, hdr.TreeID) {
		// The session was torn down, or this tree was disconnected, while this
		// CREATE was running — a LOGOFF, the connection dropping, a reconnect
		// on another connection naming it in PreviousSessionId, or a
		// TREE_DISCONNECT for hdr.TreeID racing this CREATE on another frame
		// of the same connection. A CREATE that lost a sharing conflict can
		// park for up to sharingViolationWait before reaching this point, so
		// checking the tree here (not just at resolve time, above) is what
		// keeps a TREE_DISCONNECT that lands during that wait from being
		// missed. Everything acquired above belongs to nobody now: the handle
		// is not in any session's map, so no CLOSE, no TREE_DISCONNECT and no
		// teardown will ever reach it. Give it all back here or the descriptor
		// and the share-mode reservation are held for the life of the
		// process, and a leaked reservation makes the file unopenable by
		// every client. The durable registration happens below this point, so
		// there is no durable entry to retire.
		d.Log.Warn("session torn down or tree disconnected under an in-flight CREATE; releasing the handle",
			"path", osPath, "session_id", hdr.SessionID, "tree_id", hdr.TreeID)
		releaseOpen(open)
		open.File = nil
		d.respondError(rw, hdr, smb2.StatusUserSessionDeleted, sess)
		return true
	}

	d.LastCreatedFileID = open.FileID
	d.HasLastCreated = true

	attrs := uint32(smb2.FileAttrNormal)
	allocSize := uint64(0)
	endOfFile := uint64(0)
	if isDir {
		attrs = smb2.FileAttrDirectory
	} else if st != nil {
		allocSize = uint64(st.Size())
		endOfFile = uint64(st.Size())
	}
	mtime := filetimeFromTime(time.Now())
	if st != nil {
		mtime = filetimeFromTime(st.ModTime())
	}
	// F9: use real birth time (statx btime) for CreationTime in the CREATE
	// response when the filesystem provides it. Falls back to ModTime.
	createCtime := mtime
	if bt, ok := birthTime(osPath); ok {
		createCtime = filetimeFromTime(bt)
	}

	// SMB2_CREATE_QUERY_ON_DISK_ID (QFid) wants the real POSIX identity of the
	// object we just opened. Prefer fstat on the handle (immune to a rename
	// racing the CREATE); directories have no *os.File here, so fall back to
	// the lstat we already did. A failed stat leaves both zero, and QFid is
	// then omitted rather than answered with a zero id.
	qfidInfo := st
	if open.File != nil {
		if fi, ferr := open.File.Stat(); ferr == nil {
			qfidInfo = fi
		}
	}
	diskFileID, volumeID := unixInodeAndDev(qfidInfo)

	// Register a durable handle and collect the extra response contexts
	// (DH2Q/DHnQ echo, RqLs lease grant) to append to the AAPL/MxAc/QFid set.
	respCtxs := buildCreateResponseContexts(req.CreateContexts, d.Conn, tree, granted, diskFileID, volumeID)
	respCtxs = d.applyDurableAndLease(open, durReq, leaseReq, respCtxs, sess.User.Name)

	resp := smb2.EncodeCreateResponse(smb2.CreateResponse{
		CreateAction:   createAction,
		CreationTime:   createCtime,
		LastAccessTime: mtime,
		LastWriteTime:  mtime,
		ChangeTime:     mtime,
		AllocationSize: allocSize,
		EndOfFile:      endOfFile,
		FileAttributes: attrs,
		FileID:         open.FileID,
		CreateContexts: respCtxs,
	})
	d.respondSuccess(rw, hdr, sess, resp)
	return true
}

// truncatingDisposition reports whether a CreateDisposition empties the file it
// opens. The three that do are the ones that must not run before the sharing
// check, and the ones that cannot bring a directory into existence.
func truncatingDisposition(disposition uint32) bool {
	return disposition == smb2.CreateDispositionOverwrite ||
		disposition == smb2.CreateDispositionOverwriteIf ||
		disposition == smb2.CreateDispositionSupersede
}

// statusFromResolveErr maps a path-resolution failure to an NTSTATUS.
//
// A symlink the server will not follow is reported as
// STATUS_OBJECT_PATH_NOT_FOUND — the same status statusFromErr gives for the
// ELOOP an O_NOFOLLOW open of one produces, so the two layers cannot disagree
// about the same link the way they used to. From the client's side the
// statement is accurate: the path it asked for does not lead anywhere this
// share can serve.
//
// Everything else — a lexical ".." escape, a symlink whose target is outside
// the share — stays ACCESS_DENIED. That distinction is deliberate: a proven
// containment failure is a refusal, not a missing name, and reporting it as one
// would invite a client to retry by creating the name.
func statusFromResolveErr(err error) smb2.Status {
	if errors.Is(err, vfs.ErrDanglingLink) {
		return smb2.StatusObjectPathNotFound
	}
	return smb2.StatusAccessDenied
}

// statusFromErr maps a filesystem error to the NTSTATUS a client expects.
//
// Getting this right is directly user-visible: clients branch on the status to
// decide whether to retry, to surface "disk full", or to give up. Collapsing
// every errno to STATUS_INTERNAL_ERROR makes a full disk look like a server
// bug, and makes rmdir-on-non-empty look unrecoverable.
//
// The errno→NTSTATUS table follows Samba's unix_dos_nt_errmap
// (source3/lib/errmap_unix.c) so we behave like the reference POSIX-backed SMB
// server. errors.Is unwraps the *os.PathError / *os.LinkError wrappers the os
// package returns.
func statusFromErr(err error) smb2.Status {
	if err == nil {
		return smb2.StatusSuccess
	}
	// The specific errno tests run first. The os.Is* predicates below are
	// deliberately coarse — syscall.Errno.Is folds ENOTEMPTY into fs.ErrExist,
	// for instance — so consulting them first would report a non-empty
	// directory as OBJECT_NAME_COLLISION.
	switch {
	// ENOSPC/EDQUOT/EFBIG all mean "the write cannot be stored". Reporting
	// DISK_FULL lets the client tell the user why instead of retrying.
	case errors.Is(err, syscall.ENOSPC), errors.Is(err, syscall.EDQUOT), errors.Is(err, syscall.EFBIG):
		return smb2.StatusDiskFull
	case errors.Is(err, syscall.ENOTEMPTY):
		return smb2.StatusDirectoryNotEmpty
	case errors.Is(err, syscall.EISDIR):
		return smb2.StatusFileIsADirectory
	case errors.Is(err, syscall.ENOTDIR):
		return smb2.StatusNotADirectory
	case errors.Is(err, syscall.EMFILE), errors.Is(err, syscall.ENFILE):
		return smb2.StatusTooManyOpenedFiles
	// Samba maps a read-only filesystem to ACCESS_DENIED rather than
	// MEDIA_WRITE_PROTECTED; every client already handles ACCESS_DENIED.
	case errors.Is(err, syscall.EROFS):
		return smb2.StatusAccessDenied
	case errors.Is(err, syscall.ENAMETOOLONG):
		return smb2.StatusObjectNameInvalid
	case errors.Is(err, syscall.EXDEV):
		return smb2.StatusNotSameDevice
	// ELOOP means a path component was a symlink loop (or O_NOFOLLOW hit a
	// symlink). Samba reports it as a path lookup failure.
	case errors.Is(err, syscall.ELOOP):
		return smb2.StatusObjectPathNotFound
	case errors.Is(err, syscall.EMLINK):
		return smb2.StatusTooManyLinks
	case errors.Is(err, syscall.ENOMEM), errors.Is(err, syscall.ENOBUFS):
		return smb2.StatusInsufficientResources
	case errors.Is(err, syscall.EIO):
		return smb2.StatusIODeviceError
	case errors.Is(err, syscall.EBADF):
		return smb2.StatusInvalidHandle
	case errors.Is(err, syscall.EINVAL):
		return smb2.StatusInvalidParameter
	case errors.Is(err, syscall.ENOSYS), errors.Is(err, syscall.ENOTSUP):
		return smb2.StatusNotSupported
	}
	// Fall back to the portable sentinels (fs.ErrNotExist, fs.ErrPermission,
	// fs.ErrExist) for errors that carry no errno at all.
	switch {
	case os.IsNotExist(err):
		return smb2.StatusObjectNameNotFound
	case os.IsPermission(err):
		return smb2.StatusAccessDenied
	case os.IsExist(err):
		return smb2.StatusObjectNameCollision
	}
	return smb2.StatusInternalError
}

// --- READ ---

func (d *Dispatcher) handleRead(rw io.ReadWriter, hdr smb2.Header, body []byte, sess *Session) bool {
	req, err := smb2.DecodeReadRequest(body)
	if err != nil {
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	open := sess.GetOpen(req.FileID)
	if open == nil {
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	if open.IsPipe {
		// Drain any queued DCE/RPC response from a prior WRITE.
		if len(open.pipeOut) == 0 {
			d.respondError(rw, hdr, smb2.StatusEndOfFile, sess)
			return true
		}
		// 先在 uint64 中与队列长度比较再转换，32 位平台上 int(req.Length) 可能变成负数
		n := bounds.Clamp(uint64(req.Length), len(open.pipeOut))
		out := open.pipeOut[:n]
		open.pipeOut = open.pipeOut[n:]
		d.respondSuccess(rw, hdr, sess, smb2.EncodeReadResponse(smb2.ReadResponse{Data: out}))
		return true
	}
	if open.IsStream {
		// Compare in uint64 before narrowing to int: a wire offset past 2^63
		// would otherwise become negative and panic the streamBuf slice.
		if req.Offset >= uint64(len(open.streamBuf)) {
			d.respondError(rw, hdr, smb2.StatusEndOfFile, sess)
			return true
		}
		off := int(req.Offset)
		// 读取长度超出流末尾时截到末尾；在 uint64 中计算，32 位平台上不会溢出为负数
		end := off + bounds.Clamp(uint64(req.Length), len(open.streamBuf)-off)
		d.respondSuccess(rw, hdr, sess, smb2.EncodeReadResponse(smb2.ReadResponse{Data: open.streamBuf[off:end]}))
		return true
	}
	if open.File == nil {
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	// Clamp the request to the negotiated MaxReadSize so a single READ can't
	// force an arbitrary (up to 4 GiB) allocation. A compliant client never
	// asks for more than it negotiated.
	if maxIO := d.Conn.MaxIOSize; maxIO != 0 && req.Length > maxIO {
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	// A read may not cross another handle's exclusive byte-range lock. This is
	// deliberately checked over the *requested* range and before the EOF
	// clamp below: byte-range locks may extend past end-of-file, and a lock
	// conflict outranks END_OF_FILE for a read that straddles both.
	if d.locks != nil && d.locks.conflictsWith(open, req.Offset, uint64(req.Length), false) {
		d.respondError(rw, hdr, smb2.StatusFileLockConflict, sess)
		return true
	}
	length, eof := readLengthForOpen(open, req.Offset, req.Length)
	if eof {
		d.respondError(rw, hdr, smb2.StatusEndOfFile, sess)
		return true
	}
	buf, data, prefix := d.newReadResponseFrame(sess, int(length))
	n, err := open.File.ReadAt(data, int64(req.Offset))
	if err != nil && err != io.EOF {
		d.Log.Warn("read failed", "path", open.Path, "err", err)
		d.respondError(rw, hdr, statusFromErr(err), sess)
		return true
	}
	if n == 0 {
		d.respondError(rw, hdr, smb2.StatusEndOfFile, sess)
		return true
	}
	d.sendReadResponseFrame(rw, hdr, sess, buf, prefix, n)
	return true
}

// readLengthForOpen decides how many bytes a READ at offset may actually
// allocate for, given that it asked for want.
//
// The client is under no obligation to clamp its own reads to end-of-file.
// macOS deliberately does not: smbfs_io.c ("Dont check for reads past EOF ...
// Just try the read request as is") issues every read at the full negotiated
// quantum, so the tail of any transfer — and every read of a file smaller than
// the quantum — over-asks. Honouring want literally meant allocating and
// zeroing a megabyte to return four kilobytes.
//
// The second return value is true when the offset is at or past end-of-file,
// which the caller answers with STATUS_END_OF_FILE. That is the same status
// the old code produced when the pread came back with zero bytes, so the wire
// behaviour is unchanged; only the allocation is.
//
// The clamp applies to regular files only. A character device or FIFO reports
// a meaningless st_size, and clamping to it would invent a short read where
// the descriptor really does have bytes to give.
func readLengthForOpen(open *Open, offset uint64, want uint32) (length uint32, eof bool) {
	var st unix.Stat_t
	if err := unix.Fstat(int(open.File.Fd()), &st); err != nil {
		return want, false
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Size < 0 {
		return want, false
	}
	size := uint64(st.Size)
	if offset >= size {
		return 0, true
	}
	if avail := size - offset; uint64(want) > avail {
		return uint32(avail), false
	}
	return want, false
}

// readResponseFixed is the fixed part of the SMB2 READ response body. With the
// 64-byte header ahead of it this puts DataOffset at 80, which is what the
// response advertises and what clients (Apple's parser underflows below 80)
// require.
const readResponseFixed = 16

// newReadResponseFrame allocates the one buffer a READ response needs and
// returns it together with the sub-slice the file data is to be pread into.
//
// The whole point is that this is a single allocation covering every layer:
// NBSS header, SMB2 header, READ body and payload. The payload is never copied
// — the pread lands on its final resting place on the wire. Building the
// response the obvious way instead (read into a buffer, encode the body into a
// second, prepend the header into a third, frame into a fourth) cost four
// allocations and three payload-sized copies per response.
//
// The NBSS header is always reserved, even when the response will be
// encrypted: the emit path slices the headroom off before handing the message
// to smb3.EncryptTransform, so the four bytes never reach the AEAD, and
// reserving them unconditionally keeps every response buffer the same shape.
func (d *Dispatcher) newReadResponseFrame(sess *Session, n int) (buf, data []byte, prefix int) {
	prefix = transport.FrameHeaderSize
	buf = make([]byte, prefix+smb2.HeaderSize+readResponseFixed+n)
	return buf, buf[prefix+smb2.HeaderSize+readResponseFixed:], prefix
}

// sendReadResponseFrame fills in the headers of a frame from
// newReadResponseFrame and writes it. prefix is that call's third return value
// and n is how many bytes the pread actually delivered — which may be fewer
// than the buffer holds if the file shrank between the fstat and the read.
func (d *Dispatcher) sendReadResponseFrame(rw io.Writer, reqHdr smb2.Header, sess *Session, buf []byte, prefix, n int) {
	// A short read leaves unused tail bytes; cut them off rather than send
	// zero padding the client would count as data.
	buf = buf[:prefix+smb2.HeaderSize+readResponseFixed+n]
	msg := buf[prefix:]

	_ = smb2.EncodeHeader(msg[:smb2.HeaderSize], smb2.Header{
		CreditCharge:   reqHdr.CreditCharge,
		Status:         uint32(smb2.StatusSuccess),
		Command:        reqHdr.Command,
		CreditResponse: grantCredits(reqHdr.CreditCharge, reqHdr.CreditResponse),
		Flags:          smb2.FlagServerToRedir,
		MessageID:      reqHdr.MessageID,
		TreeID:         reqHdr.TreeID,
		SessionID:      reqHdr.SessionID,
	})

	// READ response body, byte-for-byte what smb2.EncodeReadResponse emits.
	// Reserved (byte 3), DataRemaining (bytes 8-11) and Reserved2 (12-15) stay
	// zero, which they already are in a freshly allocated buffer.
	body := msg[smb2.HeaderSize:]
	binary.LittleEndian.PutUint16(body[0:], 17) // StructureSize
	body[2] = byte(smb2.HeaderSize + readResponseFixed)
	binary.LittleEndian.PutUint32(body[4:], uint32(n))

	d.lastChainStatus = smb2.StatusSuccess
	d.emit(rw, sess, buf)
}

// willEncryptResponse reports whether a response on this chain goes back
// wrapped in an SMB3 transform header. It mirrors the condition writeFrame
// applies; the read path needs to know it up front, because an encrypted
// response cannot be pre-framed.
func (d *Dispatcher) willEncryptResponse(sess *Session) bool {
	return sess != nil && len(sess.S2CCipherKey) > 0 &&
		d.Conn != nil && d.Conn.Selection.Cipher != 0 &&
		(sess.GotEncrypted() || d.encryptChain)
}

// --- WRITE ---

func (d *Dispatcher) handleWrite(rw io.ReadWriter, hdr smb2.Header, body []byte, sess *Session) bool {
	req, err := smb2.DecodeWriteRequest(body)
	if err != nil {
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	open := sess.GetOpen(req.FileID)
	if open == nil {
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	// Enforce read-only share: writes are never permitted.
	if open.Tree != nil && open.Tree.Share.ReadOnly && !open.IsPipe {
		d.respondError(rw, hdr, smb2.StatusAccessDenied, sess)
		return true
	}
	if open.IsPipe {
		// Run DCE/RPC and queue the response for the next READ.
		if open.PipeName == "srvsvc" {
			if out := dcerpcHandle(req.Data, visibleShares(d.sharesFor(sess), sess)); out != nil {
				open.pipeOut = append(open.pipeOut, out...)
			}
		}
		d.respondSuccess(rw, hdr, sess, smb2.EncodeWriteResponse(smb2.WriteResponse{Count: uint32(len(req.Data))}))
		return true
	}
	if open.IsStream {
		// Streams are held wholly in memory and persisted as an xattr, so a
		// huge offset is both meaningless and dangerous: int(req.Offset) on a
		// >2^63 value goes negative (panicking the slice), and a large offset
		// would drive an unbounded make([]byte, end). Reject anything past the
		// stream size cap before touching the buffer.
		if req.Offset > maxStreamSize || uint64(len(req.Data)) > maxStreamSize ||
			req.Offset+uint64(len(req.Data)) > maxStreamSize {
			d.respondError(rw, hdr, smb2.StatusDiskFull, sess)
			return true
		}
		off := int(req.Offset)
		end := off + len(req.Data)
		if end > len(open.streamBuf) {
			grown := make([]byte, end)
			copy(grown, open.streamBuf)
			open.streamBuf = grown
		}
		copy(open.streamBuf[off:end], req.Data)
		open.streamWritten = true
		d.respondSuccess(rw, hdr, sess, smb2.EncodeWriteResponse(smb2.WriteResponse{Count: uint32(len(req.Data))}))
		return true
	}
	if open.File == nil {
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	// Clamp to the negotiated MaxWriteSize, mirroring the READ path. Without
	// this the only bound on a WRITE was transport.MaxFrameSize (16 MiB)
	// against an advertised 8 MiB, so a client could push twice what it was
	// told the server would take. macOS never does — it clamps itself to
	// 2 MiB — but a hostile or simply non-macOS client is not obliged to.
	if d.Conn != nil && d.Conn.MaxIOSize != 0 && req.Length > d.Conn.MaxIOSize {
		d.Log.Warn("WRITE exceeds negotiated MaxWriteSize",
			"requested", req.Length, "max", d.Conn.MaxIOSize)
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	// A write may not touch any byte another handle holds locked, shared or
	// exclusive.
	if d.locks != nil && d.locks.conflictsWith(open, req.Offset, uint64(len(req.Data)), true) {
		d.respondError(rw, hdr, smb2.StatusFileLockConflict, sess)
		return true
	}
	n, err := open.File.WriteAt(req.Data, int64(req.Offset))
	if err != nil {
		d.Log.Warn("write failed", "path", open.Path, "err", err)
		d.respondError(rw, hdr, statusFromErr(err), sess)
		return true
	}
	d.respondSuccess(rw, hdr, sess, smb2.EncodeWriteResponse(smb2.WriteResponse{Count: uint32(n)}))
	return true
}

// --- FLUSH ---

func (d *Dispatcher) handleFlush(rw io.ReadWriter, hdr smb2.Header, body []byte, sess *Session) bool {
	req, err := smb2.DecodeFlushRequest(body)
	if err != nil {
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	open := sess.GetOpen(req.FileID)
	if open == nil {
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	if open.File != nil {
		_ = open.File.Sync()
	}
	d.respondSuccess(rw, hdr, sess, smb2.EncodeFlushResponse())
	return true
}

// --- SET_INFO ---

func (d *Dispatcher) handleSetInfo(rw io.ReadWriter, hdr smb2.Header, body []byte, sess *Session) bool {
	req, err := smb2.DecodeSetInfoRequest(body)
	if err != nil {
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	open := sess.GetOpen(req.FileID)
	if open == nil {
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	d.Log.Debug("set-info",
		"info_type", req.InfoType,
		"info_class", req.FileInfoClass,
		"path", open.Path,
		"buf_len", len(req.Buffer),
	)
	// Enforce read-only share: SET_INFO is always a write mutation.
	if open.Tree != nil && open.Tree.Share.ReadOnly {
		d.respondError(rw, hdr, smb2.StatusAccessDenied, sess)
		return true
	}
	if open.IsStream {
		// Honor EOF (resize the in-memory buffer) and Disposition (delete-on-
		// close → remove the backing xattr at CLOSE). Rename/EA/Basic on a
		// stream are accepted silently.
		switch {
		case req.InfoType == smb2.InfoTypeFile && req.FileInfoClass == smb2.FileEndOfFileInformation && len(req.Buffer) >= 8:
			// The size is client-chosen and drives a make() on an in-memory
			// buffer, so it needs exactly the cap the stream WRITE path
			// already applies — otherwise one SET_INFO makes the server
			// allocate terabytes. Compare as uint64 first: int() of a value
			// past 2^63 goes negative and would slip through as "truncate".
			wire := binary.LittleEndian.Uint64(req.Buffer[0:])
			if wire > maxStreamSize {
				d.respondError(rw, hdr, smb2.StatusDiskFull, sess)
				return true
			}
			size := int(wire)
			switch {
			case size <= 0:
				open.streamBuf = nil
			case size <= len(open.streamBuf):
				open.streamBuf = open.streamBuf[:size]
			default:
				grown := make([]byte, size)
				copy(grown, open.streamBuf)
				open.streamBuf = grown
			}
		case req.InfoType == smb2.InfoTypeFile && req.FileInfoClass == smb2.FileDispositionInformation && len(req.Buffer) >= 1:
			open.DeleteOnClose = req.Buffer[0] != 0
		}
		d.respondSuccess(rw, hdr, sess, smb2.EncodeSetInfoResponse())
		return true
	}
	// Silently accept SD / quota / FS-info SET_INFO. We don't persist ACLs,
	// but Samba and Windows answer SUCCESS here so clients (Finder especially)
	// don't conclude the handle is broken. Refusing surfaces as "you don't
	// have permission to see its contents" on macOS for newly created dirs.
	if req.InfoType == smb2.InfoTypeSecurity ||
		req.InfoType == smb2.InfoTypeQuota ||
		req.InfoType == smb2.InfoTypeFilesystem {
		d.respondSuccess(rw, hdr, sess, smb2.EncodeSetInfoResponse())
		return true
	}
	if req.InfoType != smb2.InfoTypeFile {
		d.respondError(rw, hdr, smb2.StatusNotSupported, sess)
		return true
	}
	switch req.FileInfoClass {
	case smb2.FileEndOfFileInformation:
		if len(req.Buffer) < 8 {
			d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
			return true
		}
		size := int64(binary.LittleEndian.Uint64(req.Buffer[0:]))
		if err := truncateOpen(open, size); err != nil {
			d.respondError(rw, hdr, statusFromErr(err), sess)
			return true
		}
	case smb2.FileAllocationInformation:
		// Best-effort: ignore (POSIX doesn't have a portable preallocation primitive).
	case smb2.FileDispositionInformation:
		if len(req.Buffer) < 1 {
			d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
			return true
		}
		open.DeleteOnClose = req.Buffer[0] != 0
	case smb2.FileRenameInformation:
		// Buffer: ReplaceIfExists(1) + Reserved(7) + RootDirectory(8) + FileNameLength(4) + FileName(N)
		if len(req.Buffer) < 20 {
			d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
			return true
		}
		replace := req.Buffer[0] != 0
		nameLen := binary.LittleEndian.Uint32(req.Buffer[16:])
		// 文件名长度来自客户端，在 uint64 中确认不越过缓冲区再切片
		nameStart, nameEnd, ok := bounds.Span(20, uint64(nameLen), len(req.Buffer))
		if !ok {
			d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
			return true
		}
		newName := decodeUTF16LE(req.Buffer[nameStart:nameEnd])
		// Use normalization-insensitive resolution for the rename destination so
		// that lookups across NFC/NFD boundaries work. For a new name (doesn't
		// exist yet), it falls through to the requested name unchanged.
		newPath, err := vfs.ResolveSecureNorm(open.Tree.Share.Path, newName, shareFoldsCase(open.Tree))
		if err != nil {
			d.respondError(rw, hdr, smb2.StatusAccessDenied, sess)
			return true
		}
		// Gately 补丁：不能改名到被屏蔽的路径中，也不能移动被屏蔽的部分或它的上级目录
		if shareTouches(open.Tree, newPath) || shareTouches(open.Tree, open.namePath()) {
			d.respondError(rw, hdr, smb2.StatusAccessDenied, sess)
			return true
		}
		// Lstat, not Stat, so an existing symlink at the destination is judged
		// as an object in its own right rather than by whatever it points at.
		// That is the same rule the rename below follows.
		if !replace {
			if _, err := os.Lstat(newPath); err == nil {
				d.respondError(rw, hdr, smb2.StatusObjectNameCollision, sess)
				return true
			}
		}
		// Rename moves the NAME, so for a handle opened through an in-share
		// symlink it moves the link and leaves the target where it is (see
		// Open.LinkPath). Renaming open.Path here renamed the target instead,
		// so `mv link.txt other.txt` over SMB silently renamed the real file
		// and left the link dangling.
		//
		// The destination side needs no special handling and gets none:
		// ResolveSecureNorm returns the LEXICAL path of a symlink destination
		// and os.Rename does not follow a symlink at the destination, so
		// renaming ONTO a link replaces the link and leaves the file that link
		// pointed at untouched. That is rename(2), and it is the only answer
		// consistent with the source side above.
		if err := os.Rename(open.namePath(), newPath); err != nil {
			d.respondError(rw, hdr, statusFromErr(err), sess)
			return true
		}
		// Only the name moved. A link handle keeps reading and writing the same
		// target, so Path must not be touched — the new name is the link's.
		if open.LinkPath != "" {
			open.LinkPath = newPath
		} else {
			open.Path = newPath
		}
	case smb2.FileFullEaInformation:
		// Parse the FILE_FULL_EA_INFORMATION list (MS-FSCC §2.4.15) and persist
		// each EA as a user.* xattr. macOS uses this to seed Versions/Quarantine/
		// AppleDouble metadata. If the filesystem lacks xattr support we accept
		// silently (matching the prior accept-and-drop behavior) so clients
		// don't conclude the handle is broken.
		for _, ea := range parseFullEaList(req.Buffer) {
			if err := setEAOnFile(open.File, open.Path, ea.Name, ea.Value); err != nil {
				if errors.Is(err, errXattrUnsupported) {
					break
				}
				d.respondError(rw, hdr, statusFromErr(err), sess)
				return true
			}
		}
	case smb2.FileBasicInformationSet:
		// Only LastWriteTime/LastAccessTime are applied; POSIX has no setter
		// for CreationTime or ChangeTime, and a 0 / -1 field means "leave
		// unchanged" (MS-FSCC §2.4.7).
		if len(req.Buffer) >= 32 {
			lastWrite := int64(binary.LittleEndian.Uint64(req.Buffer[16:]))
			lastAccess := int64(binary.LittleEndian.Uint64(req.Buffer[8:]))
			if lastWrite > 0 && lastWrite != -1 {
				wt := timeFromFiletime(uint64(lastWrite))
				at := wt
				if lastAccess > 0 && lastAccess != -1 {
					at = timeFromFiletime(uint64(lastAccess))
				}
				// Report a failed timestamp write instead of swallowing it.
				// rclone compares modification times to decide whether a file
				// still needs uploading; answering SUCCESS for a time we never
				// set makes it treat a stale copy as up to date indefinitely.
				if err := setTimesOnOpen(open, at, wt); err != nil {
					d.Log.Warn("set-info: setting times failed", "path", open.Path, "err", err)
					d.respondError(rw, hdr, statusFromErr(err), sess)
					return true
				}
			}
		}
	default:
		d.Log.Warn("set-info: unsupported class", "class", req.FileInfoClass)
		d.respondError(rw, hdr, smb2.StatusNotSupported, sess)
		return true
	}
	d.respondSuccess(rw, hdr, sess, smb2.EncodeSetInfoResponse())
	return true
}

// truncateOpen resizes the file behind an Open.
//
// It uses the descriptor CREATE already opened with O_NOFOLLOW rather than
// re-resolving the path: os.Truncate follows symlinks, so a symlink swapped in
// between CREATE and this SET_INFO would redirect the resize to whatever it
// points at, bypassing the O_NOFOLLOW protection on the open. Directory
// handles carry no descriptor and fall back to the path (where truncate is
// EISDIR anyway).
func truncateOpen(open *Open, size int64) error {
	if open.File != nil {
		return open.File.Truncate(size)
	}
	return os.Truncate(open.Path, size)
}

// setTimesOnOpen applies access/modification times to the file behind an Open.
//
// os.Chtimes resolves the path and follows symlinks, which reopens the TOCTOU
// window that CREATE's O_NOFOLLOW closed. utimensat with AT_SYMLINK_NOFOLLOW
// never traverses a symlink at the leaf and, unlike futimes(2), keeps the full
// nanosecond resolution the client's 100 ns FILETIME deserves — a microsecond
// rounding would make the mtime the client reads back differ from the one it
// set.
func setTimesOnOpen(open *Open, atime, mtime time.Time) error {
	ts := []unix.Timespec{
		unix.NsecToTimespec(atime.UnixNano()),
		unix.NsecToTimespec(mtime.UnixNano()),
	}
	return unix.UtimesNanoAt(unix.AT_FDCWD, open.Path, ts, unix.AT_SYMLINK_NOFOLLOW)
}

func decodeUTF16LE(b []byte) string {
	if len(b)%2 != 0 {
		b = b[:len(b)-1]
	}
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i < len(b); i += 2 {
		u = append(u, uint16(b[i])|uint16(b[i+1])<<8)
	}
	// Surrogate pairs must be recombined into one rune; decoding each code
	// unit separately corrupts every non-BMP filename (emoji and friends),
	// which utf16leName then re-encodes as U+FFFD.
	return string(utf16.Decode(u))
}

func timeFromFiletime(ft uint64) time.Time {
	if ft == 0 {
		return time.Now()
	}
	secs := int64(ft/10_000_000) - filetimeEpochDelta
	nanos := int64(ft%10_000_000) * 100
	return time.Unix(secs, nanos).UTC()
}

// --- CLOSE ---

func (d *Dispatcher) handleClose(rw io.ReadWriter, hdr smb2.Header, body []byte, sess *Session) bool {
	req, err := smb2.DecodeCloseRequest(body)
	if err != nil {
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	open := sess.RemoveOpen(req.FileID)
	if open == nil {
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	// Drop the share-mode reservation before anything below can return early.
	// CLOSE has several error exits (a failed stream flush, a failed
	// delete-on-close) and the handle is out of the session table on every one
	// of them, so releasing anywhere else would leak the entry on those paths
	// and leave the file unopenable by every client.
	sharedShareModes.release(open)
	// Closing the directory handle must complete any CHANGE_NOTIFY watching it
	// (MS-SMB2 §3.3.5.19), otherwise the client waits on a handle that is gone.
	d.cancelNotifiesForOpens([]*Open{open})
	// Retire any server-side-copy resume key issued for this handle: the key is
	// a capability to read the file behind it, so it dies with the handle.
	if d.Conn != nil {
		d.Conn.resumeKeys.release(open)
	}
	// A clean CLOSE of a durable handle means it is no longer reclaimable —
	// drop its durable-table entry (a dropped connection, by contrast, leaves
	// it for reclaim until expiry).
	if open.IsDurable && d.Conn != nil && d.Conn.Durable != nil {
		d.Conn.Durable.Remove(open.DurableClientGuid, open.DurableCreateGuid)
	}
	if open.File != nil {
		if d.locks != nil {
			d.locks.releaseAll(open)
		}
		open.File.Close()
	}
	if open.IsPipe {
		// No POSTQUERY_ATTRIB echo here even when asked: a pipe has no times,
		// no size and no attributes, so the only thing we could report is a
		// block of zeros — which the client would reject anyway (see
		// closeAttribsUsable).
		d.respondSuccess(rw, hdr, sess, smb2.EncodeCloseResponse(smb2.CloseResponse{}))
		return true
	}
	if open.IsStream {
		// Persist the stream buffer to its backing xattr, or remove it on
		// delete-on-close. A zero-length buffer still writes an (empty) xattr so
		// the stream "exists". ENOTSUP (no xattr support) is tolerated silently.
		//
		// This branch always returns, which is what keeps delete-on-close of a
		// stream from ever reaching the unlink below: deleting
		// `dir:com.apple.metadata:...` drops the xattr and leaves the directory
		// (and, for a file stream, the file) untouched. A stream handle never
		// traverses a symlink either, so its LinkPath is always empty.
		if open.DeleteOnClose {
			if err := removeStreamXattr(open.Path, open.StreamName); err != nil && !errors.Is(err, errXattrUnsupported) {
				// The client asked for the stream to be gone; if it isn't,
				// say so rather than reporting a delete that never happened.
				d.Log.Warn("stream delete-on-close failed", "path", open.Path, "stream", open.StreamName, "err", err)
				d.respondError(rw, hdr, statusFromErr(err), sess)
				return true
			}
		} else if open.streamSynthetic && !open.streamWritten {
			// A fabricated blob (e.g. empty AFP_AfpInfo) the client only read —
			// don't persist it, so we don't litter the file with metadata xattrs.
		} else if err := writeStreamXattr(open.Path, open.StreamName, open.streamBuf); err != nil && !errors.Is(err, errXattrUnsupported) {
			// This is the only point at which stream bytes reach disk, so a
			// swallowed error here loses everything the client wrote while
			// CLOSE still reports SUCCESS. Every failure must be surfaced.
			//
			// Linux user.* xattrs are size-limited (~64 KiB on ext4). A resource
			// fork larger than that yields E2BIG — map to STATUS_DISK_FULL,
			// which is the closest "your data did not fit" status.
			d.Log.Warn("stream flush failed", "path", open.Path, "stream", open.StreamName, "err", err)
			if errors.Is(err, syscall.E2BIG) {
				d.respondError(rw, hdr, smb2.StatusDiskFull, sess)
				return true
			}
			d.respondError(rw, hdr, statusFromErr(err), sess)
			return true
		}
		now := filetimeFromTime(time.Now())
		resp := smb2.CloseResponse{
			CreationTime:   now,
			LastAccessTime: now,
			LastWriteTime:  now,
			ChangeTime:     now,
			AllocationSize: uint64(len(open.streamBuf)),
			EndOfFile:      uint64(len(open.streamBuf)),
			FileAttributes: smb2.FileAttrNormal,
		}
		// The stream's size is exact and the times are the ones a fresh
		// QUERY_INFO on this handle would have reported, so the echo is honest.
		if closeAttribsUsable(req, resp) {
			resp.Flags = smb2.CloseFlagPostQueryAttrib
		}
		d.respondSuccess(rw, hdr, sess, smb2.EncodeCloseResponse(resp))
		return true
	}
	deleted := false
	if open.DeleteOnClose {
		// Delete the NAME this handle was opened under, which for a handle that
		// reached its file through an in-share symlink is the link and not the
		// target (see Open.LinkPath). Unlinking open.Path here is what made
		// deleting a symlink over SMB delete the file it pointed at — and, for a
		// link to a directory, delete the directory and everything in it.
		victim := open.namePath()
		// Never let DELETE_ON_CLOSE on the tree root remove the shared
		// directory itself — that would take the whole share offline. The
		// refusal has to be visible: a SUCCESS here would tell the client the
		// share directory is gone when it is not.
		//
		// Testing the name rather than open.Path is what keeps a symlink
		// pointing AT the share root deletable: removing that link does not
		// touch the shared directory, so there is nothing to refuse.
		if open.Tree != nil && open.Tree.Share.Path != "" &&
			filepath.Clean(victim) == filepath.Clean(open.Tree.Share.Path) {
			d.Log.Warn("refusing delete-on-close of the share root", "path", victim)
			d.respondError(rw, hdr, smb2.StatusAccessDenied, sess)
			return true
		}
		// Gately 补丁：被屏蔽的部分及其上级目录不能删除
		if shareTouches(open.Tree, victim) {
			d.respondError(rw, hdr, smb2.StatusAccessDenied, sess)
			return true
		}
		// A failed unlink must not be reported as a successful CLOSE: the
		// client (rclone especially) records the file as deleted and never
		// retries, while the file is still on disk. Surface the real reason —
		// STATUS_DIRECTORY_NOT_EMPTY for a populated directory, ACCESS_DENIED
		// for a sticky/read-only parent, and so on.
		//
		// An already-vanished path is not an error: the client's intent (the
		// name is gone) holds either way.
		if err := os.Remove(victim); err != nil && !os.IsNotExist(err) {
			d.Log.Warn("delete-on-close failed", "path", victim, "err", err)
			d.respondError(rw, hdr, statusFromErr(err), sess)
			return true
		}
		deleted = true
	}
	// A handle whose name was just removed has no attributes to echo. This used
	// to fall out of the Lstat below failing, which is no longer guaranteed: when
	// the name was a symlink, unlinking it leaves open.Path — the target — alive
	// and well, and stat'ing it would echo the surviving target's size and times
	// for a file the client has just been told is gone. Leave resp zeroed and let
	// closeAttribsUsable keep POSTQUERY_ATTRIB off, which is exactly what
	// happened for every non-symlink delete before.
	var st os.FileInfo
	if !deleted {
		st, _ = os.Lstat(open.Path)
	}
	resp := smb2.CloseResponse{}
	if st != nil {
		mt := filetimeFromTime(st.ModTime())
		// F9: use btime for CreationTime in CLOSE response when available.
		ct := mt
		if bt, ok := birthTime(open.Path); ok {
			ct = filetimeFromTime(bt)
		}
		resp.CreationTime = ct
		resp.LastAccessTime = mt
		resp.LastWriteTime = mt
		resp.ChangeTime = mt
		resp.AllocationSize = uint64(st.Size())
		resp.EndOfFile = uint64(st.Size())
		if st.IsDir() {
			resp.FileAttributes = smb2.FileAttrDirectory
		} else {
			resp.FileAttributes = smb2.FileAttrNormal
		}
	}
	// A failed Lstat (deleted by DELETE_ON_CLOSE, or the name vanished under
	// us) leaves resp zeroed, and the guard keeps the flag off — the client
	// then asks for the attributes itself instead of trusting a lie.
	if closeAttribsUsable(req, resp) {
		resp.Flags = smb2.CloseFlagPostQueryAttrib
	}
	d.respondSuccess(rw, hdr, sess, smb2.EncodeCloseResponse(resp))
	return true
}

// closeAttribsUsable reports whether a CLOSE response may echo
// SMB2_CLOSE_FLAG_POSTQUERY_ATTRIB. Two conditions must hold: the client asked
// for the attributes, and we actually have them.
//
// The second condition is not optional. macOS sets the flag on every CLOSE and
// its parser throws away the *entire* attribute block if CreationTime,
// LastAccessTime, LastWriteTime or ChangeTime comes back as zero, logging the
// peer as a "Bad SMB 2/3 Server". Echoing the flag over a zeroed response is
// therefore strictly worse than not echoing it: the client re-reads the
// metadata with a fresh CREATE/QUERY_INFO/CLOSE either way, and we have also
// told it we are broken.
func closeAttribsUsable(req smb2.CloseRequest, resp smb2.CloseResponse) bool {
	if req.Flags&smb2.CloseFlagPostQueryAttrib == 0 {
		return false
	}
	return resp.CreationTime != 0 && resp.LastAccessTime != 0 &&
		resp.LastWriteTime != 0 && resp.ChangeTime != 0
}

// --- QUERY_DIRECTORY ---

func (d *Dispatcher) handleQueryDirectory(rw io.ReadWriter, hdr smb2.Header, body []byte, sess *Session) bool {
	req, err := smb2.DecodeQueryDirectoryRequest(body)
	if err != nil {
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	open := sess.GetOpen(req.FileID)
	if open == nil || !open.IsDir {
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	d.Log.Debug("query-dir",
		"path", open.Path,
		"info_class", req.FileInformationClass,
		"flags", req.Flags,
		"pattern", req.FileName,
		"buf_len", req.OutputBufferLength,
	)
	// OutputBufferLength comes straight off the wire and is used as an
	// allocation size, so an unclamped value lets one request ask for up to
	// 4 GiB. MS-SMB2 §3.3.5.18 says a request whose OutputBufferLength exceeds
	// Connection.MaxTransactSize is failed with STATUS_INVALID_PARAMETER; a
	// compliant client never asks for more than it negotiated. Mirrors the
	// READ length clamp in handleRead.
	if d.Conn != nil && d.Conn.MaxIOSize != 0 && req.OutputBufferLength > d.Conn.MaxIOSize {
		d.Log.Warn("query-dir: output buffer exceeds negotiated max",
			"requested", req.OutputBufferLength, "max", d.Conn.MaxIOSize)
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	// Reject an information class we have no encoder for *before* enumerating.
	// Falling through to a differently-shaped record makes the client parse
	// garbage; MS-SMB2 §3.3.5.18 mandates STATUS_INVALID_INFO_CLASS instead.
	if !supportedDirInfoClass(req.FileInformationClass) {
		d.Log.Warn("query-dir: unsupported info class",
			"class", req.FileInformationClass, "path", open.Path)
		d.respondError(rw, hdr, smb2.StatusInvalidInfoClass, sess)
		return true
	}
	if req.Flags&smb2.QueryDirRestartScans != 0 || open.dirEntries == nil {
		entries, err := os.ReadDir(open.Path)
		if err != nil {
			d.Log.Warn("query-dir: ReadDir failed", "path", open.Path, "err", err, "status", statusFromErr(err))
			d.respondError(rw, hdr, statusFromErr(err), sess)
			return true
		}
		// Gately 补丁：列表中去掉被屏蔽的条目（共享目录中的 Gately 配置目录及指向它的符号链接）
		if open.Tree != nil && open.Tree.Share.Guard != nil {
			kept := entries[:0]
			for _, e := range entries {
				if e.IsDir() || e.Type()&os.ModeSymlink != 0 {
					if fi, ierr := e.Info(); ierr == nil && open.Tree.Share.Guard.HiddenChild(open.Path, fi) {
						continue
					}
				}
				kept = append(kept, e)
			}
			entries = kept
		}
		// Prepend synthetic "." and ".." entries — Windows/macOS clients expect
		// them. Both stats can fail even though the ReadDir above succeeded
		// (the directory may be renamed or removed in between), and a nil
		// os.FileInfo baked into a syntheticDirEntry is later dereferenced
		// during record encoding, panicking the server. Omit an entry we
		// cannot stat rather than carrying a nil through.
		all := make([]os.DirEntry, 0, len(entries)+2)
		dirInfo, dirErr := os.Lstat(open.Path)
		if dirErr == nil {
			all = append(all, syntheticDirEntry{name: ".", info: dirInfo})
		} else {
			d.Log.Warn("query-dir: cannot stat directory, omitting \".\"", "path", open.Path, "err", dirErr)
		}
		parentInfo, parentErr := os.Lstat(filepath.Dir(open.Path))
		if parentErr != nil {
			// Fall back to the directory's own info — but only when that stat
			// actually succeeded, otherwise ".." is omitted too.
			parentInfo, parentErr = dirInfo, dirErr
		}
		if parentErr == nil {
			all = append(all, syntheticDirEntry{name: "..", info: parentInfo})
		} else {
			d.Log.Warn("query-dir: cannot stat parent, omitting \"..\"", "path", open.Path, "err", parentErr)
		}
		all = append(all, entries...)

		// Apply pattern filter (SMB-style glob). Hide names containing `:` so
		// any pre-existing stream-syntax pollution (created before stream
		// parsing was wired up) is invisible to clients — `:` is illegal in
		// NTFS names anyway, so a real SMB client could never have
		// legitimately created such a name.
		//
		// The matcher folds case only where the backing filesystem does.
		// macOS resolves a single name through a one-entry QUERY_DIRECTORY
		// whose pattern IS the leaf name, so matching `foo` against an on-disk
		// `Foo` on a case-sensitive share told the client a file existed that
		// the following CREATE could not open — and the client cached that
		// ENOENT. One probe, read once per enumeration rather than per entry,
		// drives both this and the resolver.
		pattern := req.FileName
		if pattern == "" {
			pattern = "*"
		}
		caseSensitive := shareCaseSensitive(open.Tree)
		filtered := all[:0:0]
		for _, e := range all {
			if strings.ContainsRune(e.Name(), ':') {
				continue
			}
			if matchSMBPattern(pattern, e.Name(), caseSensitive) {
				filtered = append(filtered, e)
			}
		}
		open.dirEntries = filtered
		open.dirSent = 0
		d.Log.Debug("query-dir: enumerated",
			"path", open.Path,
			"total", len(all),
			"matched", len(filtered),
			"pattern", pattern,
		)
	}

	// Empty result on a fresh enumeration → STATUS_NO_SUCH_FILE.
	// Empty result on a continuation → STATUS_NO_MORE_FILES.
	if len(open.dirEntries) == 0 {
		d.respondError(rw, hdr, smb2.StatusNoSuchFile, sess)
		return true
	}
	if open.dirSent >= len(open.dirEntries) {
		d.respondError(rw, hdr, smb2.StatusNoMoreFiles, sess)
		return true
	}

	limit := len(open.dirEntries) - open.dirSent
	if req.Flags&smb2.QueryDirReturnSingleEntry != 0 {
		limit = 1
	}
	// AAPL READ_DIR_ATTR overlay applies only on disk shares (not IPC$) and
	// only for the level-37 record class.
	useAAPL := d.Conn != nil && d.Conn.AAPLReadDirAttr &&
		open.Tree != nil && open.Tree.Share.Path != "" &&
		req.FileInformationClass == smb2.InfoFileIdBothDirectoryInformation
	buf, consumed, encoded, err := encodeDirEntriesLimited(open, bounds.Clamp(uint64(req.OutputBufferLength), math.MaxInt32), req.FileInformationClass, limit, useAAPL)
	if err != nil {
		if errors.Is(err, errUnsupportedDirInfoClass) {
			d.respondError(rw, hdr, smb2.StatusInvalidInfoClass, sess)
			return true
		}
		d.respondError(rw, hdr, smb2.StatusInternalError, sess)
		return true
	}
	// Advance the enumeration cursor by the entries actually *consumed*, not
	// by the records encoded. An entry skipped during encoding (it vanished
	// between the scan and the encode) otherwise leaves dirSent pointing at an
	// entry we already walked past, so the next QUERY_DIRECTORY re-emits files
	// the client has already seen. rclone re-lists directories constantly, so
	// a drifting cursor is immediately visible as duplicated entries.
	open.dirSent += consumed
	if encoded == 0 {
		if open.dirSent >= len(open.dirEntries) {
			d.respondError(rw, hdr, smb2.StatusNoMoreFiles, sess)
			return true
		}
		// Entries remain but not even one fits in OutputBufferLength.
		// Answering NO_MORE_FILES here silently truncates the listing;
		// MS-SMB2 §3.3.5.18 requires STATUS_INFO_LENGTH_MISMATCH so the
		// client retries with a buffer big enough for one record.
		d.Log.Warn("query-dir: output buffer too small for one entry",
			"path", open.Path, "buf_len", req.OutputBufferLength)
		d.respondError(rw, hdr, smb2.StatusInfoLengthMismatch, sess)
		return true
	}
	d.respondSuccess(rw, hdr, sess, smb2.EncodeQueryDirectoryResponse(smb2.QueryDirectoryResponse{Buffer: buf}))
	return true
}

// matchSMBPattern lives in wildcard.go: SMB search patterns use the DOS
// grammar (only `*` and `?` are metacharacters), not shell globbing, and they
// fold case only on a share whose backing filesystem does.

// errUnsupportedDirInfoClass is returned when encodeDirRecord has no encoder
// for the requested class. handleQueryDirectory turns it into
// STATUS_INVALID_INFO_CLASS — the request is pre-validated by
// supportedDirInfoClass, so this is a belt-and-braces guard that keeps a
// newly-listed class from silently emitting a wrong-shaped record.
var errUnsupportedDirInfoClass = errors.New("query-dir: unsupported information class")

// supportedDirInfoClass reports whether encodeDirRecord can emit the requested
// FileInformationClass. It must stay in lockstep with encodeDirRecord's switch.
func supportedDirInfoClass(c uint8) bool {
	switch c {
	case smb2.InfoFileDirectoryInformation,
		smb2.InfoFileFullDirectoryInformation,
		smb2.InfoFileBothDirectoryInformation,
		smb2.InfoFileNamesInformation,
		smb2.InfoFileIdBothDirectoryInformation,
		smb2.InfoFileIdFullDirectoryInformation:
		return true
	}
	return false
}

// The per-entry work an AAPL directory listing does, indirected so a test can
// count what one listing really performs — the same way the vfs package
// indirects os.ReadDir. Nothing but a test ever reassigns them.
//
// Only two of the three touch the filesystem. entryAccessMask is a pure
// function of the stat the encoder already holds, which is the whole point of
// it: see dirEntryAccessMask.
var (
	entryAccessMask = dirEntryAccessMask
	entryRforkSize  = streamXattrSize
	entryFinderInfo = readAFPFinderInfo
)

// posixIdentity is the identity a directory listing derives its access hints
// against: the credentials this process is running as.
//
// With the per-user privilege-drop worker enabled those ARE the authenticated
// SMB user's credentials — the worker drops in OnAuthenticated, and a
// QUERY_DIRECTORY cannot arrive before a session and a tree exist — and without
// it every user's I/O runs as the one server identity anyway. Either way it is
// the identity whose permissions decide whether a later READ or WRITE succeeds.
type posixIdentity struct {
	uid    uint32
	gid    uint32
	groups []uint32
	root   bool
}

// currentPosixIdentity reads the process credentials. It is read once per
// response (see encodeDirEntriesLimited) rather than cached in a package-level
// variable on purpose: the privilege drop happens IN-PROCESS, so a cache
// populated before it would keep answering as root for the life of the worker.
//
// It is a var so a test can stand in an identity it does not run as; the group
// and "other" triads are otherwise unreachable from a test that owns every file
// it creates.
var currentPosixIdentity = func() posixIdentity {
	euid, egid := os.Geteuid(), os.Getegid()
	id := posixIdentity{uid: uint32(euid), gid: uint32(egid), root: euid == 0}
	if gs, gerr := os.Getgroups(); gerr == nil {
		id.groups = make([]uint32, 0, len(gs))
		for _, g := range gs {
			id.groups = append(id.groups, uint32(g))
		}
	}
	return id
}

// inGroup reports whether gid is the identity's primary or any supplementary
// group. Supplementary groups matter: a share whose files belong to a "share"
// group is the normal deployment, and ignoring them would report every such
// file through the "other" triad — read-only padlocks all over Finder.
func (id posixIdentity) inGroup(gid uint32) bool {
	if id.gid == gid {
		return true
	}
	for _, g := range id.groups {
		if g == gid {
			return true
		}
	}
	return false
}

// permits reports the rights the identity has on a file with permission bits
// perm owned by uid/gid, by the rule access(2) itself implements: the OWNER
// triad if the uid matches, else the GROUP triad if the file's group is one of
// ours, else the OTHER triad. Exactly one triad applies — they are not
// accumulated, so a mode 0466 file really is read-only to its owner and saying
// otherwise would over-report.
func (id posixIdentity) permits(perm, uid, gid uint32) (readable, writable, executable bool) {
	if id.root {
		// root bypasses the read and write bits outright. access(2) still
		// grants X_OK only when SOME execute bit is set, so that one is real.
		return true, true, perm&0o111 != 0
	}
	var bits uint32
	switch {
	case id.uid == uid:
		bits = (perm >> 6) & 7
	case id.inGroup(gid):
		bits = (perm >> 3) & 7
	default:
		bits = perm & 7
	}
	return bits&4 != 0, bits&2 != 0, bits&1 != 0
}

// dirEntryAccessMask is a listing entry's max_access: the share ceiling
// narrowed by the POSIX permissions already visible in the stat the encoder
// holds. It performs NO syscall — info has been stat'ed a few lines earlier in
// the same I/O slot, and id is read once per response.
//
// This deliberately does NOT use maximalAccess() from maxaccess.go, and the
// split is the point:
//
//   - CREATE resolves access once per open and can afford faccessat, which sees
//     what mode bits cannot — POSIX/NFSv4 ACLs, a read-only mount (EROFS), an
//     immutable flag. That is the AUTHORITATIVE answer and it stays there.
//   - This runs once per directory entry, on the hottest path in the server.
//     Its value is only a hint macOS uses to seed a vnode's max-access cache;
//     the real check still happens at CREATE, where a wrong hint is corrected.
//     Three faccessat calls per file to sharpen a hint cost far more than the
//     hint is worth — ~6 us per call where this was measured, so ~24 us per
//     entry with the Finder Info read, or ~120 ms of pure syscall time on a
//     5000-entry directory. This derivation costs single-digit nanoseconds
//     (BenchmarkDirEntryAccessMask).
//
// So this is approximate where the two can disagree: an ACL that grants or
// denies beyond the mode bits, a read-only mount, or an immutable flag is
// invisible here. In each case CREATE still reports and enforces the truth.
func dirEntryAccessMask(info os.FileInfo, id posixIdentity, shareReadOnly bool) uint32 {
	ceiling := shareAccessCeiling(shareReadOnly)
	if info == nil {
		return ceiling
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st == nil {
		// No stat behind this FileInfo — never the case for os.ReadDir on a
		// unix filesystem. There is nothing to narrow with, so report the
		// ceiling: the value this field held before it was per-entry, and
		// never zero.
		return ceiling
	}
	// The permission bits come from os.FileMode.Perm() rather than st.Mode:
	// they are the same nine bits, spelled in a type that does not change
	// width between linux (uint32) and darwin (uint16). Only the owner and
	// group ids need the raw stat.
	//
	// narrowAccessMask is maxaccess.go's, unchanged: the mask arithmetic is
	// shared with the CREATE path so the two can only ever differ in where the
	// permission answers came from, never in what a given answer means.
	readable, writable, executable := id.permits(uint32(info.Mode().Perm()), st.Uid, st.Gid)
	return narrowAccessMask(ceiling, readable, writable, executable)
}

// Offsets inside the 32-byte FinderInfo/ExtendedFinderInfo blob that Apple's
// smb_2.h documents as "Normal Finder Info and Extended Finder Info":
//
//	struct finder_file_info {          struct finder_folder_info {
//	    uint32 finder_type;     @0         uint64 reserved1;            @0
//	    uint32 finder_creator;  @4         uint16 finder_flags;         @8
//	    uint16 finder_flags;    @8         uint32 old_location;        @10
//	    uint32 old_location;   @10         uint16 old_view_flags;      @14
//	    uint16 reserved;       @14         uint32 old_scroll_position; @16
//	    uint32 reserved2;      @16         uint32 finder_date_added;   @20
//	    uint32 date_added;     @20         uint16 finder_ext_flags;    @24
//	    uint16 finder_ext_flags;@24        ...
//	    ...                            }
//	}
//
// The two layouts put the still-used extended fields at the same offsets, and
// differ only in what the first eight bytes mean: type+creator for a file, the
// Finder window rect for a folder. Both are carried through verbatim, so one
// extraction serves both — which is why this takes no "is it a directory" flag.
const (
	finderInfoHeadOff      = 0  // file: type(4)+creator(4); folder: reserved1(8)
	finderInfoFlagsOff     = 8  // Finder flags (2)
	finderInfoDateAddedOff = 20 // date added (4)
	finderInfoExtFlagsOff  = 24 // extended Finder flags (2)
)

// compressedFinderInfo builds the 16 bytes Apple's READ_DIR_ATTR overlay
// carries in ShortName[8..23], from the 32-byte FinderInfo stored in the
// entry's AFP_AfpInfo stream.
//
// The wire layout is the order smb_smb_2.c parses (smb2_smb_parse_query_dir_both_dir_info):
//
//	file:   type(4)  creator(4)  finder_flags(2)  finder_ext_flags(2)  date_added(4)
//	folder: reserved1(8)         finder_flags(2)  finder_ext_flags(2)  date_added(4)
//
// Note the ext flags come BEFORE date added on the wire even though the struct
// declarations in smb_2.h list them the other way round; the parser's read
// order is what is authoritative.
//
// Endianness: the client reads each field with md_get_uint*le and then memcpys
// the parsed struct straight into fa_finder_info, which userland consumes as a
// native (big-endian) FinderInfo blob. An le-read followed by a native store on
// a little-endian host preserves the bytes, so every field must go on the wire
// in exactly the byte order it has on disk. That makes each field a verbatim
// copy out of the stored blob — no byte swapping anywhere — which is also what
// keeps this consistent with the non-AAPL path, where the client reads the very
// same bytes out of the AFP_AfpInfo stream itself.
func compressedFinderInfo(fi [finderInfoSize]byte) [16]byte {
	var out [16]byte
	copy(out[0:8], fi[finderInfoHeadOff:finderInfoHeadOff+8])
	copy(out[8:10], fi[finderInfoFlagsOff:finderInfoFlagsOff+2])
	copy(out[10:12], fi[finderInfoExtFlagsOff:finderInfoExtFlagsOff+2])
	copy(out[12:16], fi[finderInfoDateAddedOff:finderInfoDateAddedOff+4])
	return out
}

// encodeDirEntriesLimited packs at most `limit` records (or as many as fit in
// maxBytes) starting at open.dirSent. When useAAPL is true and infoClass is
// FileIdBothDirectoryInformation, each record carries the Apple overlay so
// Finder gets FinderInfo/rfork/mode in one round-trip.
//
// It returns the packed buffer, the number of entries consumed from
// open.dirEntries (what the caller must add to open.dirSent), and the number
// of records actually encoded. The two counts differ whenever an entry is
// dropped mid-batch because its Info() failed — it was unlinked between
// os.ReadDir and here. The cursor has to follow `consumed`: advancing by the
// record count would leave it behind the scan position, so the next call
// re-encodes entries the client already received.
//
// Records are linked as they are appended, carrying the previous record's start
// offset in a local. Re-deriving it by walking the NextEntryOffset chain from
// the head of the buffer made linking quadratic in the number of records in a
// *single* response: at the 8 MiB this server advertises as MaxTransactSize
// that is ~75k records and ~2.8G list steps, seconds of CPU that any client
// honouring the advertised buffer can ask for over and over.
//
// Every record is also priced from its name before a single syscall is made for
// it. A record is a fixed size per class plus two bytes per UTF-16 code unit of
// the name, rounded up to 8, so the entry that straddles the end of the buffer
// is left for the next call instead of being stat'ed, xattr-probed, encoded and
// then discarded. Every per-entry probe the AAPL overlay needs — the access
// check, the resource-fork sizing and the Finder Info read — therefore belongs
// in the one slot below the fit check, never above it.
func encodeDirEntriesLimited(open *Open, maxBytes int, infoClass uint8, limit int, useAAPL bool) (out []byte, consumed, encoded int, err error) {
	fixed, ok := dirRecordFixedSize(infoClass)
	if !ok {
		return nil, 0, 0, errUnsupportedDirInfoClass
	}
	// maxBytes is client-chosen (bounded by MaxTransactSize upstream) and most
	// listings are far smaller, so cap the up-front reservation and let append
	// grow rather than allocating megabytes on every request.
	initial := maxBytes
	if initial > 64<<10 {
		initial = 64 << 10
	}
	if initial < 0 {
		initial = 0
	}
	out = make([]byte, 0, initial)
	// The share ceiling is now only the FALLBACK for max_access: every AAPL
	// entry reports its own per-file mask below. A probe that cannot answer
	// keeps the ceiling — never zero, which Apple reads as "this server thinks
	// we are Windows" (smbfs_smb_2.c) and turns into folders that show as
	// access denied.
	readOnlyShare := open.Tree != nil && open.Tree.Share.ReadOnly
	shareAccess := shareAccessCeiling(readOnlyShare)
	// The identity the per-entry access hints are derived against, read ONCE
	// per response — three credential lookups amortised over every record in
	// it — and not at all for a listing without the Apple overlay.
	var ident posixIdentity
	if useAAPL {
		ident = currentPosixIdentity()
	}
	// FILE_NAMES_INFORMATION is NextEntryOffset + FileIndex + FileNameLength +
	// the name (MS-FSCC §2.4.26) — nothing the stat could fill in. Calling
	// Info() for it is one lstat per entry for fields that are never encoded.
	needStat := infoClass != smb2.InfoFileNamesInformation
	// Start offset of the record appended most recently, so the chain is linked
	// in constant time per record rather than by re-walking it.
	prevStart := 0
	for i := open.dirSent; i < len(open.dirEntries) && encoded < limit; i++ {
		ent := open.dirEntries[i]
		name := ent.Name()
		// Price the record from the name alone, before any I/O. An entry that
		// does not fit is re-offered on the next call, so its lstat and
		// resource-fork probe would be done twice and used once.
		if len(out)+padTo8(fixed+utf16leLen(name)) > maxBytes {
			break
		}
		var info os.FileInfo
		if needStat {
			fi, ierr := ent.Info()
			if ierr != nil || fi == nil {
				// The entry vanished between the scan and now (or is an
				// unstattable synthetic "."/".."). Skip the record but still
				// count the entry as consumed so the cursor keeps pace with i.
				consumed = i + 1 - open.dirSent
				continue
			}
			info = fi
		}
		// --- per-entry I/O slot -------------------------------------------
		// Everything below runs AFTER the fit check, so the entry that
		// straddles the end of the buffer costs no syscall at all: it is
		// re-offered on the next call, where its probes would be paid twice
		// and used once.
		//
		// An AAPL entry costs ONE syscall here that it did not cost before:
		// the getxattr that reads its Finder Info. That is irreducible — a
		// named attribute is one read — and it is the whole point of the
		// field. The per-file access mask adds none at all: it is derived from
		// the stat above. BenchmarkAAPLDirListing measures both separately.
		var (
			rforkSize  uint64
			finderInfo [16]byte
		)
		maxAccess := shareAccess
		if useAAPL && info != nil {
			entryPath := filepath.Join(open.Path, name)
			// Per-file maximal access rather than the share-wide constant.
			// macOS seeds a vnode's max-access cache straight from this field
			// (smb_smb_2.c sets FA_MAX_ACCESS_VALID from it), so a listing
			// that answers FILE_ALL_ACCESS for a file the server cannot write
			// re-introduces exactly the over-reporting the CREATE path stopped
			// doing. Derived from the stat, never from a syscall of its own;
			// an answer of zero would be read by macOS as "this server thinks
			// we are Windows", so it falls back to the share ceiling.
			if granted := entryAccessMask(info, ident, readOnlyShare); granted != 0 {
				maxAccess = granted
			}
			if !info.IsDir() {
				// Report the AAPL resource-fork size from its backing ADS
				// xattr. Only the length is wanted, so size the attribute
				// rather than reading the whole fork into memory to call
				// len() on it.
				if n, xerr := entryRforkSize(entryPath, rforkStreamName); xerr == nil {
					rforkSize = uint64(n)
				}
			}
			// Real Finder Info out of the AFP_AfpInfo stream. Apple sets
			// FA_FINDERINFO_VALID from this field unconditionally, so zeros
			// here do not mean "ask me properly": smbfs_attrlist.c stops
			// asking, copies the zeros onto the vnode and starts its cache
			// timer, losing type/creator, the invisible bit, label colour and
			// date-added — and Finder can write the zeros back. An entry with
			// no stored blob still ships zeros, which is the honest answer.
			if raw, ok := entryFinderInfo(entryPath); ok {
				finderInfo = compressedFinderInfo(raw)
			}
		}
		rec := encodeDirRecordFinderInfo(name, info, infoClass, useAAPL, maxAccess, rforkSize, finderInfo)
		if rec == nil {
			return nil, 0, 0, errUnsupportedDirInfoClass
		}
		// Pad from the encoded record, never from the estimate above: the
		// estimate only gates the fit check, and a one-byte drift in the
		// padding is a silent wire corruption rather than an error. Should the
		// real record ever overrun the budget, leave it for the next call.
		padded := padTo8(len(rec))
		if len(out)+padded > maxBytes {
			break
		}
		if encoded > 0 {
			binary.LittleEndian.PutUint32(out[prevStart:], uint32(len(out)-prevStart))
		}
		prevStart = len(out)
		out = append(out, rec...)
		out = append(out, dirRecordPadding[:padded-len(rec)]...)
		encoded++
		consumed = i + 1 - open.dirSent
	}
	return out, consumed, encoded, nil
}

// dirRecordPadding supplies the zero bytes that align a record to 8; a record
// never needs more than seven.
var dirRecordPadding [8]byte

// padTo8 rounds n up to the next multiple of 8. Both of SMB2's chained layouts
// are 8-byte aligned, so a padded length is exactly the offset field that links
// to the next element: NextEntryOffset for a directory record, NextCommand for
// a member of a compounded response.
func padTo8(n int) int {
	if r := n % 8; r != 0 {
		return n + 8 - r
	}
	return n
}

// dirRecordFixedSize returns the size of the name-independent part of a record
// for infoClass, and whether encodeDirRecord can emit that class at all.
//
// It must stay in lockstep with encodeDirRecord's per-class layout: these sizes
// are fixed by MS-FSCC, and they are what lets the enumerator decide whether an
// entry fits before doing any I/O for it. TestDirEncode_FixedSizesMatchEncoder
// pins them to what encodeDirRecord actually emits.
func dirRecordFixedSize(infoClass uint8) (int, bool) {
	switch infoClass {
	case smb2.InfoFileDirectoryInformation:
		return 64, true // MS-FSCC §2.4.10
	case smb2.InfoFileFullDirectoryInformation:
		return 68, true // MS-FSCC §2.4.14
	case smb2.InfoFileNamesInformation:
		return 12, true // MS-FSCC §2.4.26
	case smb2.InfoFileBothDirectoryInformation:
		return 94, true // MS-FSCC §2.4.8
	case smb2.InfoFileIdFullDirectoryInformation:
		return 80, true // MS-FSCC §2.4.20
	case smb2.InfoFileIdBothDirectoryInformation:
		return 104, true // MS-FSCC §2.4.17
	}
	return 0, false
}

// encodeDirRecord builds a single directory entry record. Supported classes:
// FileDirectoryInformation, FileFullDirectoryInformation,
// FileBothDirectoryInformation, FileNamesInformation,
// FileIdBothDirectoryInformation and FileIdFullDirectoryInformation.
//
// It returns nil for any other class. It must NOT fall back to a different
// class: every class has its own fixed-part size, so emitting (say) a
// 94-byte FILE_BOTH_DIR_INFORMATION record for a client that asked for the
// 80-byte FILE_ID_FULL_DIR_INFORMATION makes the client parse garbage —
// mis-sized names, bogus sizes, and a corrupt listing. Callers turn nil into
// STATUS_INVALID_INFO_CLASS.
//
// When useAAPL is true and infoClass is FileIdBothDirectoryInformation, the
// record overlays Apple's AAPL fields onto the record per Apple's spec
// (max_access in EaSize, rfork_size+FinderInfo in ShortName, UNIX mode in
// Reserved2). maxAccess is the maximal access mask reported as max_access. See
// Samba's smb2_trans2.c SMB_FIND_ID_BOTH_DIRECTORY_INFO case.
//
// This form reports no Finder Info; encodeDirRecordFinderInfo takes the
// compressed 16 bytes for an entry that has some.
func encodeDirRecord(name string, info os.FileInfo, infoClass uint8, useAAPL bool, maxAccess uint32, rforkSize uint64) []byte {
	return encodeDirRecordFinderInfo(name, info, infoClass, useAAPL, maxAccess, rforkSize, [16]byte{})
}

// encodeDirRecordFinderInfo is encodeDirRecord plus the compressed Finder Info
// the Apple overlay carries in ShortName[8..23]. It is one function rather than
// a post-encode patch so the whole record layout stays in a single place: the
// offsets are fixed by MS-FSCC and Apple accumulates a fixed size per class, so
// a byte written at the wrong offset is a silent corruption, not an error.
// finderInfo is all zeros for an entry with no stored Finder Info.
func encodeDirRecordFinderInfo(name string, info os.FileInfo, infoClass uint8, useAAPL bool, maxAccess uint32, rforkSize uint64, finderInfo [16]byte) []byte {
	nameU16 := utf16leName(name)
	switch infoClass {
	case smb2.InfoFileDirectoryInformation:
		// MS-FSCC §2.4.10 — 64 fixed bytes + name.
		const fixed = 64
		out := make([]byte, fixed+len(nameU16))
		binary.LittleEndian.PutUint64(out[8:], filetimeFromTime(info.ModTime()))
		binary.LittleEndian.PutUint64(out[16:], filetimeFromTime(info.ModTime()))
		binary.LittleEndian.PutUint64(out[24:], filetimeFromTime(info.ModTime()))
		binary.LittleEndian.PutUint64(out[32:], filetimeFromTime(info.ModTime()))
		size := uint64(info.Size())
		if info.IsDir() {
			size = 0
		}
		binary.LittleEndian.PutUint64(out[40:], size)
		binary.LittleEndian.PutUint64(out[48:], size)
		attrs := uint32(smb2.FileAttrNormal)
		if info.IsDir() {
			attrs = smb2.FileAttrDirectory
		}
		binary.LittleEndian.PutUint32(out[56:], attrs)
		binary.LittleEndian.PutUint32(out[60:], uint32(len(nameU16)))
		copy(out[fixed:], nameU16)
		return out
	case smb2.InfoFileFullDirectoryInformation:
		// MS-FSCC §2.4.14 — 68 fixed bytes (adds EaSize) + name.
		const fixed = 68
		out := make([]byte, fixed+len(nameU16))
		binary.LittleEndian.PutUint64(out[8:], filetimeFromTime(info.ModTime()))
		binary.LittleEndian.PutUint64(out[16:], filetimeFromTime(info.ModTime()))
		binary.LittleEndian.PutUint64(out[24:], filetimeFromTime(info.ModTime()))
		binary.LittleEndian.PutUint64(out[32:], filetimeFromTime(info.ModTime()))
		size := uint64(info.Size())
		if info.IsDir() {
			size = 0
		}
		binary.LittleEndian.PutUint64(out[40:], size)
		binary.LittleEndian.PutUint64(out[48:], size)
		attrs := uint32(smb2.FileAttrNormal)
		if info.IsDir() {
			attrs = smb2.FileAttrDirectory
		}
		binary.LittleEndian.PutUint32(out[56:], attrs)
		binary.LittleEndian.PutUint32(out[60:], uint32(len(nameU16)))
		copy(out[fixed:], nameU16)
		return out
	case smb2.InfoFileNamesInformation:
		// MS-FSCC §2.4.26 — 12 fixed (NextEntryOffset+FileIndex+FileNameLength) + name.
		const fixed = 12
		out := make([]byte, fixed+len(nameU16))
		binary.LittleEndian.PutUint32(out[8:], uint32(len(nameU16)))
		copy(out[fixed:], nameU16)
		return out
	case smb2.InfoFileIdBothDirectoryInformation:
		// Layout (104 fixed bytes + name):
		// 0  NextEntryOffset (4)
		// 4  FileIndex (4)
		// 8  CreationTime (8)
		// 16 LastAccessTime (8)
		// 24 LastWriteTime (8)
		// 32 ChangeTime (8)
		// 40 EndOfFile (8)
		// 48 AllocationSize (8)
		// 56 FileAttributes (4)
		// 60 FileNameLength (4)
		// 64 EaSize (4)
		// 68 ShortNameLength (1)
		// 69 Reserved (1)
		// 70 ShortName (24)
		// 94 Reserved2 (2)
		// 96 FileId (8)
		// 104 FileName (variable)
		const fixed = 104
		out := make([]byte, fixed+len(nameU16))
		binary.LittleEndian.PutUint64(out[8:], filetimeFromTime(info.ModTime()))
		binary.LittleEndian.PutUint64(out[16:], filetimeFromTime(info.ModTime()))
		binary.LittleEndian.PutUint64(out[24:], filetimeFromTime(info.ModTime()))
		binary.LittleEndian.PutUint64(out[32:], filetimeFromTime(info.ModTime()))
		size := uint64(info.Size())
		if info.IsDir() {
			size = 0
		}
		binary.LittleEndian.PutUint64(out[40:], size)
		binary.LittleEndian.PutUint64(out[48:], size)
		attrs := uint32(smb2.FileAttrNormal)
		if info.IsDir() {
			attrs = smb2.FileAttrDirectory
		}
		binary.LittleEndian.PutUint32(out[56:], attrs)
		binary.LittleEndian.PutUint32(out[60:], uint32(len(nameU16)))
		unixMode, inode := unixModeAndInode(info)
		if useAAPL {
			// Apple overlay (matches Samba's vfs_fruit + smb2_trans2.c):
			//  EaSize          = max_access
			//  ShortNameLength = 24 (Apple writes literal 24 even though spec says 0)
			//  ShortName[0..7] = rfork_size (from the AFP_AfpResource stream)
			//  ShortName[8..23]= compressed FinderInfo (from AFP_AfpInfo)
			//  Reserved2       = UNIX mode (low 16 bits)
			//  FileId          = inode
			binary.LittleEndian.PutUint32(out[64:], maxAccess)
			out[68] = 24
			out[69] = 0
			// ShortName[0..7] = rfork_size (from the AFP_AfpResource ADS xattr;
			// 0 when no resource fork is stored).
			binary.LittleEndian.PutUint64(out[70:], rforkSize)
			// ShortName[8..23] = compressed Finder Info, already laid out by
			// compressedFinderInfo; all zeros when the entry has none stored.
			copy(out[78:94], finderInfo[:])
			binary.LittleEndian.PutUint16(out[94:], unixMode)
			binary.LittleEndian.PutUint64(out[96:], inode)
		} else if inode != 0 {
			// Always populate FileId so clients have a stable identifier.
			binary.LittleEndian.PutUint64(out[96:], inode)
		}
		copy(out[fixed:], nameU16)
		return out
	case smb2.InfoFileIdFullDirectoryInformation:
		// MS-FSCC §2.4.20 FILE_ID_FULL_DIR_INFORMATION — 80 fixed bytes + name:
		// 0  NextEntryOffset (4)
		// 4  FileIndex (4)
		// 8  CreationTime (8)
		// 16 LastAccessTime (8)
		// 24 LastWriteTime (8)
		// 32 ChangeTime (8)
		// 40 EndOfFile (8)
		// 48 AllocationSize (8)
		// 56 FileAttributes (4)
		// 60 FileNameLength (4)
		// 64 EaSize (4)
		// 68 Reserved (4)
		// 72 FileId (8)
		// 80 FileName (variable)
		const fixed = 80
		out := make([]byte, fixed+len(nameU16))
		binary.LittleEndian.PutUint64(out[8:], filetimeFromTime(info.ModTime()))
		binary.LittleEndian.PutUint64(out[16:], filetimeFromTime(info.ModTime()))
		binary.LittleEndian.PutUint64(out[24:], filetimeFromTime(info.ModTime()))
		binary.LittleEndian.PutUint64(out[32:], filetimeFromTime(info.ModTime()))
		size := uint64(info.Size())
		if info.IsDir() {
			size = 0
		}
		binary.LittleEndian.PutUint64(out[40:], size)
		binary.LittleEndian.PutUint64(out[48:], size)
		attrs := uint32(smb2.FileAttrNormal)
		if info.IsDir() {
			attrs = smb2.FileAttrDirectory
		}
		binary.LittleEndian.PutUint32(out[56:], attrs)
		binary.LittleEndian.PutUint32(out[60:], uint32(len(nameU16)))
		// EaSize (64) and Reserved (68) stay zero. FileId carries the inode so
		// clients have a stable identifier, matching what the IdBoth class does.
		if _, inode := unixModeAndInode(info); inode != 0 {
			binary.LittleEndian.PutUint64(out[72:], inode)
		}
		copy(out[fixed:], nameU16)
		return out
	case smb2.InfoFileBothDirectoryInformation:
		// MS-FSCC §2.4.8 FILE_BOTH_DIR_INFORMATION: like IdBoth but with no
		// FileId field. Layout (94 bytes fixed + name).
		const fixed = 94
		out := make([]byte, fixed+len(nameU16))
		binary.LittleEndian.PutUint64(out[8:], filetimeFromTime(info.ModTime()))
		binary.LittleEndian.PutUint64(out[16:], filetimeFromTime(info.ModTime()))
		binary.LittleEndian.PutUint64(out[24:], filetimeFromTime(info.ModTime()))
		binary.LittleEndian.PutUint64(out[32:], filetimeFromTime(info.ModTime()))
		size := uint64(info.Size())
		if info.IsDir() {
			size = 0
		}
		binary.LittleEndian.PutUint64(out[40:], size)
		binary.LittleEndian.PutUint64(out[48:], size)
		attrs := uint32(smb2.FileAttrNormal)
		if info.IsDir() {
			attrs = smb2.FileAttrDirectory
		}
		binary.LittleEndian.PutUint32(out[56:], attrs)
		binary.LittleEndian.PutUint32(out[60:], uint32(len(nameU16)))
		copy(out[fixed:], nameU16)
		return out
	default:
		return nil
	}
}

// --- QUERY_INFO ---

func (d *Dispatcher) handleQueryInfo(rw io.ReadWriter, hdr smb2.Header, body []byte, sess *Session) bool {
	req, err := smb2.DecodeQueryInfoRequest(body)
	if err != nil {
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	open := sess.GetOpen(req.FileID)
	if open == nil {
		d.Log.Warn("query-info: unknown file id")
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	d.Log.Debug("query-info",
		"info_type", req.InfoType,
		"info_class", req.FileInfoClass,
		"path", open.Path,
	)

	switch req.InfoType {
	case smb2.InfoTypeFile:
		info := openFileInfo(open)
		buf, ok := encodeFileInfo(req.FileInfoClass, info, open)
		if !ok {
			d.Log.Warn("query-info: unsupported file info class", "class", req.FileInfoClass, "path", open.Path)
			d.respondError(rw, hdr, smb2.StatusNotSupported, sess)
			return true
		}
		d.respondSuccess(rw, hdr, sess, smb2.EncodeQueryInfoResponse(smb2.QueryInfoResponse{Buffer: buf}))
		return true
	case smb2.InfoTypeFilesystem:
		buf, ok := encodeFsInfo(req.FileInfoClass, open)
		if !ok {
			d.Log.Warn("query-info: unsupported fs info class", "class", req.FileInfoClass, "path", open.Path)
			d.respondError(rw, hdr, smb2.StatusNotSupported, sess)
			return true
		}
		d.respondSuccess(rw, hdr, sess, smb2.EncodeQueryInfoResponse(smb2.QueryInfoResponse{Buffer: buf}))
		return true
	case smb2.InfoTypeSecurity:
		buf := minimalSelfRelativeSD()
		d.respondSuccess(rw, hdr, sess, smb2.EncodeQueryInfoResponse(smb2.QueryInfoResponse{Buffer: buf}))
		return true
	default:
		d.respondError(rw, hdr, smb2.StatusNotSupported, sess)
		return true
	}
}

func openFileInfo(o *Open) os.FileInfo {
	if o.IsStream {
		return streamFileInfo{name: o.StreamName, size: int64(len(o.streamBuf))}
	}
	info, _ := os.Lstat(o.Path)
	return info
}

// fileIDFor returns the identifier this server reports for a handle. It is the
// exact value encodeDirRecord writes into the FileId field of the
// FILE_ID_BOTH/FULL_DIR_INFORMATION records — the real inode, obtained from the
// same unixModeAndInode helper — so a file enumerated in a directory listing and
// the same file stat'ed through QUERY_INFO can never report two different ids.
//
// This is not cosmetic on macOS. smbfs treats a changed id for a path as "this
// path now holds a different object": see node_vtype_changed and smbfs_nget in
// smbfs_node.c, which purge the vnode from the name cache, pull it out of the
// node hash and drop it, zero the attribute and symlink cache timers, and
// invalidate the page cache. Two encoders disagreeing means that happens for
// every file, on every stat.
//
// Named-stream handles carry a synthetic streamFileInfo with no syscall.Stat_t
// behind it, so they have no inode of their own; we fall back to the inode of
// the base file the stream hangs off (o.Path is the base file even for a stream
// Open). That matches NTFS, where every stream of a file shares the file's MFT
// record number, and it keeps a stream handle from reporting 0 — a zero id tells
// the client the server has no usable File IDs and makes it stop trusting them.
func fileIDFor(info os.FileInfo, o *Open) uint64 {
	if _, ino := unixModeAndInode(info); ino != 0 {
		return ino
	}
	// Synthetic FileInfo (named streams): report the base file's inode.
	if o != nil && o.Path != "" {
		if st, err := os.Lstat(o.Path); err == nil {
			if _, ino := unixModeAndInode(st); ino != 0 {
				return ino
			}
		}
	}
	return 0
}

func encodeFileInfo(class uint8, info os.FileInfo, o *Open) ([]byte, bool) {
	if info == nil {
		return nil, false
	}
	t := filetimeFromTime(info.ModTime())
	// F9: use real birth time (statx btime) for CreationTime when available.
	// Falls back to ModTime when the filesystem/kernel doesn't provide btime
	// (e.g. tmpfs, older kernels < 4.11, or filesystems that don't populate
	// stx_btime). LastAccess / LastWrite / ChangeTime remain as ModTime.
	ctime := t
	if o != nil && o.Path != "" && !o.IsStream {
		if bt, ok := birthTime(o.Path); ok {
			ctime = filetimeFromTime(bt)
		}
	}
	attrs := uint32(smb2.FileAttrNormal)
	if info.IsDir() {
		attrs = smb2.FileAttrDirectory
	}
	switch class {
	case smb2.FileBasicInformation:
		out := make([]byte, 40)
		binary.LittleEndian.PutUint64(out[0:], ctime) // CreationTime
		binary.LittleEndian.PutUint64(out[8:], t)     // LastAccessTime
		binary.LittleEndian.PutUint64(out[16:], t)    // LastWriteTime
		binary.LittleEndian.PutUint64(out[24:], t)    // ChangeTime
		binary.LittleEndian.PutUint32(out[32:], attrs)
		return out, true
	case smb2.FileStandardInformation:
		out := make([]byte, 24)
		size := uint64(info.Size())
		if info.IsDir() {
			size = 0
		}
		binary.LittleEndian.PutUint64(out[0:], size)
		binary.LittleEndian.PutUint64(out[8:], size)
		binary.LittleEndian.PutUint32(out[16:], 1) // NumberOfLinks
		if info.IsDir() {
			out[21] = 0x01
		}
		return out, true
	case smb2.FileInternalInformation:
		out := make([]byte, 8)
		// IndexNumber — the real inode, the same value the directory encoder
		// reports as this file's FileId.
		binary.LittleEndian.PutUint64(out[0:], fileIDFor(info, o))
		return out, true
	case smb2.FileEaInformation:
		return make([]byte, 4), true
	case smb2.FileFullEaInformation:
		// Encode the persisted EAs as a FILE_FULL_EA_INFORMATION list. An empty
		// list (no EAs, or a filesystem without xattr support) is a valid
		// zero-length buffer — what NTFS returns for "no EAs".
		eas, err := listEAs(o.Path)
		if err != nil {
			return []byte{}, true
		}
		return encodeFullEaList(eas), true
	case smb2.FileAccessInformation:
		out := make([]byte, 4)
		access := o.GrantedAccess
		if access == 0 {
			access = smb2.AccessFileReadData | smb2.AccessFileReadAttributes | smb2.AccessFileReadEa | smb2.AccessReadControl
		}
		binary.LittleEndian.PutUint32(out[0:], access)
		return out, true
	case smb2.FilePositionInformation:
		return make([]byte, 8), true
	case smb2.FileModeInformation:
		return make([]byte, 4), true
	case smb2.FileAlignmentInformation:
		return make([]byte, 4), true
	case smb2.FileNetworkOpenInformation:
		out := make([]byte, 56)
		size := uint64(info.Size())
		if info.IsDir() {
			size = 0
		}
		binary.LittleEndian.PutUint64(out[0:], ctime) // CreationTime
		binary.LittleEndian.PutUint64(out[8:], t)     // LastAccessTime
		binary.LittleEndian.PutUint64(out[16:], t)    // LastWriteTime
		binary.LittleEndian.PutUint64(out[24:], t)    // ChangeTime
		binary.LittleEndian.PutUint64(out[32:], size)
		binary.LittleEndian.PutUint64(out[40:], size)
		binary.LittleEndian.PutUint32(out[48:], attrs)
		return out, true
	case smb2.FileAttributeTagInformation:
		// FileAttributes(4) + ReparseTag(4)
		out := make([]byte, 8)
		binary.LittleEndian.PutUint32(out[0:], attrs)
		return out, true
	case smb2.FileAlternateNameInformation:
		// FileNameLength(4) + FileName(N) — empty 8.3 alt name.
		out := make([]byte, 4)
		return out, true
	case smb2.FileStreamInformation:
		// Default ::$DATA entry plus one :<name>:$DATA entry per persisted ADS
		// stream. A directory reports only its named streams (it has no unnamed
		// data stream) — encodeStreamInfoList drops the ::$DATA entry for one,
		// so a folder carrying Finder metadata enumerates it here instead of
		// always claiming it has no streams at all.
		return encodeStreamInfoList(o.Path, info.Size()), true
	case smb2.FileNameInformation:
		// FileNameLength(4) + FileName(N) — return basename.
		nameU16 := utf16leName(filepath.Base(o.Path))
		out := make([]byte, 4+len(nameU16))
		binary.LittleEndian.PutUint32(out[0:], uint32(len(nameU16)))
		copy(out[4:], nameU16)
		return out, true
	case smb2.FileAllInformation:
		nameU16 := utf16leName(filepath.Base(o.Path))
		const fixedHead = 40 + 24 + 8 + 4 + 4 + 8 + 4 + 4 // 96
		out := make([]byte, fixedHead+4+len(nameU16))
		size := uint64(info.Size())
		if info.IsDir() {
			size = 0
		}
		// Basic (40)
		binary.LittleEndian.PutUint64(out[0:], ctime) // CreationTime
		binary.LittleEndian.PutUint64(out[8:], t)     // LastAccessTime
		binary.LittleEndian.PutUint64(out[16:], t)    // LastWriteTime
		binary.LittleEndian.PutUint64(out[24:], t)    // ChangeTime
		binary.LittleEndian.PutUint32(out[32:], attrs)
		// Standard (24) at offset 40
		binary.LittleEndian.PutUint64(out[40:], size)
		binary.LittleEndian.PutUint64(out[48:], size)
		binary.LittleEndian.PutUint32(out[56:], 1)
		if info.IsDir() {
			out[61] = 0x01 // Directory
		}
		// Internal (8) at offset 64 — IndexNumber, the real inode. Identical
		// to FileInternalInformation and to the directory encoder's FileId.
		binary.LittleEndian.PutUint64(out[64:], fileIDFor(info, o))
		// Ea (4) at 72 — zero
		// Access (4) at 76
		access := o.GrantedAccess
		if access == 0 {
			access = smb2.AccessFileReadData | smb2.AccessFileReadAttributes | smb2.AccessFileReadEa | smb2.AccessReadControl
		}
		binary.LittleEndian.PutUint32(out[76:], access)
		// Position (8) at 80 — zero
		// Mode (4) at 88 — zero
		// Alignment (4) at 92 — zero
		// NameInformation: FileNameLength(4) + FileName at offset 96
		binary.LittleEndian.PutUint32(out[fixedHead:], uint32(len(nameU16)))
		copy(out[fixedHead+4:], nameU16)
		return out, true
	}
	return nil, false
}

func encodeFsInfo(class uint8, o *Open) ([]byte, bool) {
	switch class {
	case smb2.FileFsAttributeInformation:
		// FileSystemAttributes(4) + MaxFileNameLength(4) + FileSystemNameLength(4) + Name (UTF-16LE)
		//
		// Bits we claim (MS-FSCC §2.5.1):
		//   0x00000001 FILE_CASE_SENSITIVE_SEARCH — only when the share's backing filesystem really is (see fsAttributes)
		//   0x00000002 FILE_CASE_PRESERVED_NAMES
		//   0x00000004 FILE_UNICODE_ON_DISK
		//   0x00000008 FILE_PERSISTENT_ACLS
		//   0x00000040 FILE_SUPPORTS_SPARSE_FILES
		//   0x00040000 FILE_NAMED_STREAMS  — needed so macOS will round-trip
		//                                    AppleDouble metadata via streams
		//                                    rather than creating ._foo files.
		//   0x00800000 FILE_SUPPORTS_EXTENDED_ATTRIBUTES — required for macOS
		//                                    Versions ("permanent version
		//                                    storage") to enable on the share.
		fsAttrs := fsAttributes(o.Tree)
		name := utf16leName("NTFS")
		out := make([]byte, 12+len(name))
		binary.LittleEndian.PutUint32(out[0:], fsAttrs)
		binary.LittleEndian.PutUint32(out[4:], 255)
		binary.LittleEndian.PutUint32(out[8:], uint32(len(name)))
		copy(out[12:], name)
		return out, true
	case smb2.FileFsVolumeInformation:
		// CreationTime(8) + SerialNumber(4) + LabelLength(4) + Reserved(2) + Label
		label := utf16leName(filepath.Base(o.Tree.Share.Path))
		out := make([]byte, 18+len(label))
		binary.LittleEndian.PutUint64(out[0:], filetimeNow())
		binary.LittleEndian.PutUint32(out[8:], 0xCAFE1337)
		binary.LittleEndian.PutUint32(out[12:], uint32(len(label)))
		copy(out[18:], label)
		return out, true
	case smb2.FileFsSizeInformation:
		total, avail, spu, bps := fsStats(o.Tree.Share.Path)
		out := make([]byte, 24)
		binary.LittleEndian.PutUint64(out[0:], total)
		binary.LittleEndian.PutUint64(out[8:], avail)
		binary.LittleEndian.PutUint32(out[16:], spu)
		binary.LittleEndian.PutUint32(out[20:], bps)
		return out, true
	case smb2.FileFsFullSizeInformation:
		total, avail, spu, bps := fsStats(o.Tree.Share.Path)
		out := make([]byte, 32)
		binary.LittleEndian.PutUint64(out[0:], total)
		binary.LittleEndian.PutUint64(out[8:], avail)
		binary.LittleEndian.PutUint64(out[16:], avail)
		binary.LittleEndian.PutUint32(out[24:], spu)
		binary.LittleEndian.PutUint32(out[28:], bps)
		return out, true
	case smb2.FileFsDeviceInformation:
		// DeviceType(4) + Characteristics(4)
		out := make([]byte, 8)
		binary.LittleEndian.PutUint32(out[0:], 0x07) // FILE_DEVICE_DISK
		binary.LittleEndian.PutUint32(out[4:], 0x20) // FILE_DEVICE_IS_MOUNTED
		return out, true
	}
	return nil, false
}

// --- IOCTL ---

func (d *Dispatcher) handleIoctl(rw io.ReadWriter, hdr smb2.Header, body []byte, sess *Session) bool {
	req, err := smb2.DecodeIoctlRequest(body)
	if err != nil {
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	d.Log.Debug("ioctl", "ctl_code", fmt.Sprintf("0x%08x", req.CtlCode), "input_len", len(req.InputBuffer))
	switch req.CtlCode {
	case smb2.FsctlPipeTransceive:
		open := sess.GetOpen(req.FileID)
		if open == nil || !open.IsPipe || open.PipeName != "srvsvc" {
			// Unknown pipe — reply with bind_nak so the client stops retrying.
			var callID uint32
			if len(req.InputBuffer) >= 16 {
				callID = binary.LittleEndian.Uint32(req.InputBuffer[12:16])
			}
			d.respondSuccess(rw, hdr, sess, smb2.EncodeIoctlResponse(smb2.IoctlResponse{
				CtlCode:      req.CtlCode,
				FileID:       req.FileID,
				OutputBuffer: dcerpcBindNakBytes(callID),
			}))
			return true
		}
		out := dcerpcHandle(req.InputBuffer, visibleShares(d.sharesFor(sess), sess))
		if out == nil {
			d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
			return true
		}
		d.respondSuccess(rw, hdr, sess, smb2.EncodeIoctlResponse(smb2.IoctlResponse{
			CtlCode:      req.CtlCode,
			FileID:       req.FileID,
			OutputBuffer: out,
		}))
		return true
	case smb2.FsctlDfsGetReferrals:
		// We are not a DFS root. Reply with FSDRIVER_NOT_DFS so the client
		// stops asking. Some old clients use STATUS_NOT_FOUND here too.
		d.respondError(rw, hdr, smb2.StatusFsDriverRequired, sess)
		return true
	case smb2.FsctlPipeWait:
		// Decline gracefully so clients fall back to read+write.
		d.respondError(rw, hdr, smb2.StatusNotSupported, sess)
		return true
	case smb2.FsctlSrvRequestResumeKey:
		return d.handleResumeKey(rw, hdr, req, sess)
	case smb2.FsctlSrvCopyChunk, smb2.FsctlSrvCopyChunkWrite:
		return d.handleCopyChunk(rw, hdr, req, sess)
	case smb2.FsctlValidateNegotiateInfo:
		// Repeat, byte for byte, what this connection's NEGOTIATE response put
		// on the wire. The client saved those four values at negotiate time and
		// compares every one of them here to detect a downgrade; the macOS
		// client fails with EAUTH (a failed mount, or ENOTCONN mid-reconnect)
		// on any mismatch. The values are read back off the Connection rather
		// than recomputed so the two can never drift apart.
		resp := smb2.EncodeIoctlResponse(smb2.IoctlResponse{
			CtlCode: req.CtlCode,
			FileID:  req.FileID,
			OutputBuffer: smb2.EncodeValidateNegotiateInfoResponse(smb2.ValidateNegotiateInfoResponse{
				Capabilities: d.Conn.NegotiatedCapabilities,
				Guid:         d.Conn.ServerGuid,
				SecurityMode: d.Conn.NegotiatedSecurityMode,
				Dialect:      d.Conn.Selection.Dialect,
			}),
		})
		d.respondSuccess(rw, hdr, sess, resp)
		return true
	default:
		d.respondError(rw, hdr, smb2.StatusNotSupported, sess)
		return true
	}
}

// --- helpers ---

// unixModeAndInode pulls the POSIX mode bits (low 16 bits of st_mode, including
// the type field) and inode number out of os.FileInfo. Returns (0, 0) for
// FileInfo implementations that don't expose syscall.Stat_t (synthetic streams,
// non-Linux platforms).
func unixModeAndInode(info os.FileInfo) (uint16, uint64) {
	if info == nil {
		return 0, 0
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st == nil {
		return 0, 0
	}
	return uint16(st.Mode & 0xFFFF), uint64(st.Ino)
}

// unixInodeAndDev pulls the inode and device numbers out of os.FileInfo for the
// SMB2_CREATE_QUERY_ON_DISK_ID (QFid) response context. Returns (0, 0) for
// FileInfo implementations that don't expose syscall.Stat_t (synthetic stream
// handles), which is the caller's signal to omit the context entirely.
func unixInodeAndDev(info os.FileInfo) (uint64, uint64) {
	if info == nil {
		return 0, 0
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st == nil {
		return 0, 0
	}
	return uint64(st.Ino), devToUint64(st.Dev)
}

// devToUint64 widens a st_dev to uint64 without sign-extending. The concrete
// type of syscall.Stat_t.Dev differs per platform (int32 on darwin, uint64 on
// linux), so this goes through an interface type switch rather than needing a
// per-GOOS file.
func devToUint64(dev any) uint64 {
	switch v := dev.(type) {
	case int32:
		return uint64(uint32(v))
	case uint32:
		return uint64(v)
	case int64:
		return uint64(v)
	case uint64:
		return v
	}
	return 0
}

const filetimeEpochDelta = 11644473600

func filetimeFromTime(t time.Time) uint64 {
	t = t.UTC()
	if t.Year() < 1601 {
		return 0
	}
	secs := uint64(t.Unix() + filetimeEpochDelta)
	return secs*10_000_000 + uint64(t.Nanosecond())/100
}

func utf16leName(s string) []byte {
	out := make([]byte, 0, len(s)*2)
	for _, r := range s {
		if r <= 0xFFFF {
			out = append(out, byte(r), byte(r>>8))
		} else {
			r -= 0x10000
			hi := 0xD800 + uint16(r>>10)
			lo := 0xDC00 + uint16(r&0x3FF)
			out = append(out, byte(hi), byte(hi>>8), byte(lo), byte(lo>>8))
		}
	}
	return out
}

// utf16leLen returns the number of bytes utf16leName would produce for s,
// without building the encoding. The enumerator uses it to size a record before
// deciding whether the entry fits; it must agree with utf16leName byte for
// byte, which TestDirEncode_UTF16LenMatchesEncoder checks.
func utf16leLen(s string) int {
	n := 0
	for _, r := range s {
		if r <= 0xFFFF {
			n += 2
		} else {
			n += 4
		}
	}
	return n
}

// silence unused-vars for the dispatcher's helpers when not referenced.
var _ = errors.New
var _ = fmt.Errorf

// handleChangeNotify watches the directory referenced by FileID via inotify
// and replies asynchronously when an event arrives. The immediate reply is
// STATUS_PENDING with an AsyncId; the eventual completion is signed and
// posted from a goroutine.
func (d *Dispatcher) handleChangeNotify(rw io.ReadWriter, hdr smb2.Header, body []byte, sess *Session) bool {
	req, err := smb2.DecodeChangeNotifyRequest(body)
	if err != nil {
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}
	open := sess.GetOpen(req.FileID)
	if open == nil || !open.IsDir {
		// macOS treats this server as an OS X server (we answer the AAPL
		// server-query context) and then starts a "server message"
		// CHANGE_NOTIFY whose FileId is the all-FF sentinel — a Mac-to-Mac
		// side channel we do not implement. That request is NOT part of a
		// related compound chain, so the substitution above never runs and no
		// handle can ever match. Answering STATUS_INVALID_PARAMETER maps to
		// EINVAL, which Apple's client retries until its received-notify
		// counter passes SMBFS_MAX_RCVD_NOTIFY (4) — five wasted round-trips
		// per mount plus a logged warning. STATUS_NOT_SUPPORTED maps to
		// ENOTSUP, which process_svrmsg_items() turns into a clean one-shot
		// "svrmsg notify not supported" shutdown of the watch.
		//
		// Only the unrelated sentinel changes: inside a related chain the
		// all-FF FileId means "the previous CREATE's handle" and must keep
		// its existing error so a broken compound chain is still diagnosed
		// as such.
		if open == nil && hdr.Flags&smb2.FlagRelatedOps == 0 && req.FileID == previousHandleFileID {
			d.respondError(rw, hdr, smb2.StatusNotSupported, sess)
			return true
		}
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}

	w, err := inotify.New(open.Path)
	if err != nil {
		d.Log.Warn("inotify init failed", "path", open.Path, "err", err)
		d.respondError(rw, hdr, smb2.StatusNotSupported, sess)
		return true
	}

	asyncID := d.nextAsyncID()
	watchTree := req.Flags&smb2.NotifyWatchTree != 0
	filter := req.CompletionFilter
	maxOut := req.OutputBufferLength

	reg := &notifyReg{open: open, cancel: make(chan struct{}), status: smb2.StatusCancelled}
	d.registerNotify(hdr.MessageID, reg)

	// Send STATUS_PENDING (async-format header) immediately.
	d.sendAsync(rw, hdr, sess, asyncID, smb2.StatusPending, []byte{0x09, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})

	// accept filters one raw watcher event down to a wire entry, honouring
	// SMB2_WATCH_TREE (without it only the immediate directory is reported)
	// and the client's CompletionFilter.
	accept := func(ev inotify.InotifyEvent) (smb2.NotifyEntry, bool) {
		rel, err := filepath.Rel(open.Path, ev.Path)
		if err != nil {
			return smb2.NotifyEntry{}, false
		}
		// Gately 补丁：被屏蔽部分里的变化不通知，否则递归监视上级目录就能得知其中的文件名
		if shareHidden(open.Tree, ev.Path) {
			return smb2.NotifyEntry{}, false
		}
		if !watchTree && strings.Contains(rel, string(filepath.Separator)) {
			// A change in a subdirectory: only a recursive watch reports it.
			return smb2.NotifyEntry{}, false
		}
		action := actionForEvent(ev.Event)
		if action == 0 || !notifyFilterAllows(filter, action) {
			return smb2.NotifyEntry{}, false
		}
		return smb2.NotifyEntry{Action: action, Name: strings.ReplaceAll(rel, "/", "\\")}, true
	}

	go func() {
		// Gately 新增：panic 时断开连接（客户端会重新连接并重新注册监视），不让 panic 结束整个进程
		defer recoverConnPanic(d.Log, "change-notify", d.killConn)
		defer w.Close()
		defer d.unregisterNotify(hdr.MessageID)
		go func() {
			defer recoverConnPanic(d.Log, "change-notify-watch", d.killConn)
			w.Watch()
		}()

		var entries []smb2.NotifyEntry
		cancelled := false

		for len(entries) == 0 && !cancelled {
			select {
			case ev, ok := <-w.Events:
				if !ok || ev.Event == inotify.WatchStop {
					cancelled = true
					continue
				}
				e, keep := accept(ev)
				if !keep {
					continue
				}
				entries = append(entries, e)
				// Drain a short burst so a batch of changes ships in one reply.
				drainTimer := time.NewTimer(30 * time.Millisecond)
				for drain := true; drain; {
					select {
					case ev2, ok := <-w.Events:
						if !ok || ev2.Event == inotify.WatchStop {
							drain = false
							continue
						}
						if e2, keep2 := accept(ev2); keep2 {
							entries = append(entries, e2)
						}
					case <-drainTimer.C:
						drain = false
					case <-reg.cancel:
						drain = false
					}
				}
				drainTimer.Stop()
			case <-reg.cancel:
				cancelled = true
			}
		}

		if cancelled && len(entries) == 0 {
			// CLOSE/TREE_DISCONNECT/CANCEL completed us: answer the original
			// request with a plain error frame, not a CHANGE_NOTIFY body.
			status := d.notifyStatus(reg)
			d.sendAsync(rw, hdr, sess, asyncID, status,
				[]byte{0x09, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
			return
		}

		buf := smb2.EncodeFileNotifyInformation(entries)
		// MS-SMB2 §3.3.5.19: if the changes do not fit in the buffer the client
		// offered, report STATUS_NOTIFY_ENUM_DIR with no records so it re-scans
		// the directory itself. Previously the oversized buffer was sent anyway.
		if maxOut != 0 && uint32(len(buf)) > maxOut {
			d.sendAsync(rw, hdr, sess, asyncID, smb2.StatusNotifyEnumDir,
				smb2.EncodeChangeNotifyResponse(smb2.ChangeNotifyResponse{}))
			return
		}
		d.sendAsync(rw, hdr, sess, asyncID, smb2.StatusSuccess,
			smb2.EncodeChangeNotifyResponse(smb2.ChangeNotifyResponse{Buffer: buf}))
	}()
	return true
}

func actionForEvent(t inotify.EventType) uint32 {
	switch t {
	case inotify.FileCreate, inotify.FolderCreate:
		return smb2.FileActionAdded
	case inotify.Delete:
		return smb2.FileActionRemoved
	case inotify.Modified:
		return smb2.FileActionModified
	case inotify.MovedFrom:
		return smb2.FileActionRenamedOldName
	case inotify.MovedTo:
		return smb2.FileActionRenamedNewName
	}
	return 0
}

// sendAsync builds and writes an SMB2 async-format response (FlagAsyncCommand
// set, AsyncId in bytes 32-39 instead of TreeId/Reserved). The whole frame
// is signed end-to-end like a sync response.
func (d *Dispatcher) sendAsync(rw io.Writer, reqHdr smb2.Header, sess *Session, asyncID uint64, status smb2.Status, body []byte) {
	respHdr := smb2.Header{
		CreditCharge:   reqHdr.CreditCharge,
		Status:         uint32(status),
		Command:        reqHdr.Command,
		CreditResponse: grantCredits(reqHdr.CreditCharge, reqHdr.CreditResponse),
		Flags:          smb2.FlagServerToRedir,
		MessageID:      reqHdr.MessageID,
		SessionID:      reqHdr.SessionID,
	}
	out := make([]byte, transport.FrameHeaderSize+smb2.HeaderSize+len(body))
	msg := out[transport.FrameHeaderSize:]
	_ = smb2.EncodeAsyncHeader(msg[:smb2.HeaderSize], respHdr, asyncID)
	copy(msg[smb2.HeaderSize:], body)
	// An async response never joins a compounded reply: the interim
	// STATUS_PENDING goes out the instant the request is parked, and the
	// completion comes from a goroutine long after the chain flushed. It is
	// also not tied to the chain's encryption decision — by the time it is
	// sent there is no chain — so only the session's own state applies.
	encrypt := sess != nil && len(sess.S2CCipherKey) > 0 &&
		d.Conn != nil && d.Conn.Selection.Cipher != 0 && sess.GotEncrypted()
	_ = d.sealAndSend(rw, sess, out, encrypt)
}
