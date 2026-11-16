// Gately 修改：仅在 Linux 与 macOS 编译（构建约束）；阻塞锁等待协程中的 panic 只断开本连接。

//go:build linux || darwin

package parent

import (
	"errors"
	"io"
	"math"
	"sync/atomic"
	"time"

	"github.com/wangwanjie/gately/internal/gosamba/smb2"
)

// lockKind is the OS-independent classification of an SMB2 LOCK_ELEMENT,
// mapped from its SMB2_LOCKFLAG_* bits. Both the linux (OFD) and darwin
// (in-process table) lockManager implementations consume this type.
type lockKind int

const (
	lockUnlock lockKind = iota
	lockShared
	lockExclusive
)

var (
	// errLockConflict is returned by lockManager.applyLock when a requested
	// lock range conflicts with an existing lock held by a different owner.
	// handleLock maps it to STATUS_LOCK_NOT_GRANTED.
	errLockConflict = errors.New("lock conflict")

	// errLockLimit is returned when the server refuses to track another byte
	// range for this file or handle (see maxLockRangesPerFile). It maps to
	// STATUS_INSUFFICIENT_RESOURCES: the lock is not granted, but the reason
	// is server capacity rather than another client's lock.
	errLockLimit = errors.New("lock range limit exceeded")

	// errLockRange is returned for a range the server cannot express — an
	// offset past 2^63-1, which no POSIX file offset can reach. It maps to
	// STATUS_INVALID_PARAMETER.
	errLockRange = errors.New("lock range not representable")
)

const (
	// lockWaitTimeout bounds how long a blocking LOCK request is parked before
	// the server gives up and answers STATUS_LOCK_NOT_GRANTED. Windows waits
	// indefinitely and relies on the client cancelling; a bounded wait keeps a
	// forgotten waiter from pinning a goroutine and a handle forever.
	lockWaitTimeout = 30 * time.Second

	// maxPendingLockWaits bounds how many blocking LOCK requests may be parked
	// across the whole process. Past the cap a blocking request degrades to
	// the non-blocking answer (STATUS_LOCK_NOT_GRANTED) rather than letting a
	// client spawn goroutines without limit.
	maxPendingLockWaits = 256
)

// pendingLockWaits counts parked blocking LOCK requests process-wide.
var pendingLockWaits atomic.Int64

// interimLockBody is the 9-byte SMB2 ERROR response body (StructureSize=9,
// no error context, no data) that accompanies a STATUS_PENDING interim reply
// and any later error completion for a parked LOCK (MS-SMB2 §2.2.2).
var interimLockBody = []byte{0x09, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

// lockOp is one validated SMB2 LOCK_ELEMENT: the wire element plus the
// lockKind its flags select.
type lockOp struct {
	el   smb2.LockElement
	kind lockKind
}

// blocking reports whether the client asked the server to wait for this
// element instead of failing at once — i.e. SMB2_LOCKFLAG_FAIL_IMMEDIATELY is
// absent. The flag is meaningless on an unlock, which never waits.
func (o lockOp) blocking() bool {
	return o.kind != lockUnlock && o.el.Flags&smb2.LockFlagFailImmediately == 0
}

// handleLock processes SMB2 LOCK requests (MS-SMB2 §3.3.5.14).
//
// Each SMB2 LOCK_ELEMENT is mapped to a lockKind and applied via the
// Dispatcher's per-OS lockManager:
//
//	SMB2 flag                       | lockKind
//	--------------------------------|---------------
//	SMB2_LOCKFLAG_SHARED_LOCK       | lockShared
//	SMB2_LOCKFLAG_EXCLUSIVE_LOCK    | lockExclusive
//	SMB2_LOCKFLAG_UNLOCK            | lockUnlock
//
// SMB2_LOCKFLAG_FAIL_IMMEDIATELY (0x10) decides what a conflict means. With
// the flag set the request is answered STATUS_LOCK_NOT_GRANTED right away.
// Without it the client asked to *wait*, so the request is parked: the server
// sends an interim STATUS_PENDING with an AsyncId and completes it later from
// a goroutine, when the conflicting range is released, when SMB2_CANCEL or the
// handle's disappearance completes it, or when lockWaitTimeout expires (the
// server never waits forever, and never blocks the connection's read loop —
// stalling the dispatcher on a lock held by a remote peer would freeze every
// other request on that connection).
//
// The whole request is atomic: if any element fails, the handle's lock state
// is restored to exactly what it was before the request, including ranges it
// already held that a later element re-requested.
//
// On Linux, lockManager delegates to kernel OFD (Open File Description)
// locks so that two independent os.File objects opened on the same path
// produce genuinely independent lock domains — exactly what SMB2 per-handle
// locking requires. OFD locks don't exist on darwin, so the darwin
// lockManager keeps an in-process table keyed by file identity, enforcing
// the same per-open-handle conflict semantics in userspace.
func (d *Dispatcher) handleLock(rw io.ReadWriter, hdr smb2.Header, body []byte, sess *Session) bool {
	req, err := smb2.DecodeLockRequest(body)
	if err != nil {
		d.Log.Warn("lock: decode failed", "err", err)
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}

	open := sess.GetOpen(req.FileID)
	if open == nil {
		d.Log.Warn("lock: unknown FileID", "fid", req.FileID)
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}

	if open.File == nil {
		// Directories and virtual handles don't support byte-range locking.
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}

	if d.locks == nil {
		// A dispatcher with no lock manager cannot honour byte-range locking;
		// saying so beats a nil dereference that would kill the connection.
		d.Log.Warn("lock: no lock manager configured")
		d.respondError(rw, hdr, smb2.StatusInternalError, sess)
		return true
	}

	ops, ok := decodeLockOps(req.Locks)
	if !ok {
		d.Log.Warn("lock: invalid lock elements", "count", len(req.Locks))
		d.respondError(rw, hdr, smb2.StatusInvalidParameter, sess)
		return true
	}

	idx, err := d.applyLockOps(open, ops)
	if err == nil {
		d.respondSuccess(rw, hdr, sess, smb2.EncodeLockResponse())
		return true
	}

	if errors.Is(err, errLockConflict) {
		d.Log.Debug("lock: conflict",
			"element", idx,
			"offset", ops[idx].el.Offset,
			"length", ops[idx].el.Length,
			"flags", ops[idx].el.Flags,
		)
		// Without FAIL_IMMEDIATELY the client asked to wait rather than be
		// refused, so park the request instead of answering it now.
		if ops[idx].blocking() && d.startLockWait(rw, hdr, sess, open, ops) {
			return true // interim STATUS_PENDING sent; completion comes later
		}
	} else {
		d.Log.Warn("lock: apply error", "element", idx, "err", err)
	}

	d.respondError(rw, hdr, lockStatus(err), sess)
	return true
}

// decodeLockOps classifies and validates every element of a LOCK request. It
// reports false for an element the server must reject with
// STATUS_INVALID_PARAMETER.
func decodeLockOps(els []smb2.LockElement) ([]lockOp, bool) {
	ops := make([]lockOp, 0, len(els))
	for _, el := range els {
		var kind lockKind
		switch {
		case el.Flags&smb2.LockFlagUnlock != 0:
			kind = lockUnlock
		case el.Flags&smb2.LockFlagSharedLock != 0:
			kind = lockShared
		case el.Flags&smb2.LockFlagExclusiveLock != 0:
			kind = lockExclusive
		default:
			return nil, false
		}
		// A file offset is signed on both platforms (fcntl's l_start is an
		// int64, and no POSIX file reaches 2^63). Linux answered such an
		// element STATUS_LOCK_NOT_GRANTED while darwin cheerfully tracked a
		// range no file can contain; rejecting it here makes both platforms
		// answer the same client request the same way.
		if el.Offset > math.MaxInt64 {
			return nil, false
		}
		ops = append(ops, lockOp{el: el, kind: kind})
	}
	return ops, true
}

// lockStatus maps an applyLock failure to the NTSTATUS the client expects.
func lockStatus(err error) smb2.Status {
	switch {
	case errors.Is(err, errLockConflict):
		return smb2.StatusLockNotGranted
	case errors.Is(err, errLockLimit):
		return smb2.StatusInsufficientResources
	case errors.Is(err, errLockRange):
		return smb2.StatusInvalidParameter
	}
	return smb2.StatusInternalError
}

// applyLockOps applies every element of one LOCK request atomically and
// returns the index of the element that failed (or -1 on success).
//
// MS-SMB2 §3.3.5.14 requires all-or-nothing semantics: on failure the handle's
// lock state must be exactly what it was on entry. Undoing the request by
// unlocking each element applied so far is not enough — if the handle already
// held one of those ranges before the request, that unlock throws away a lock
// the client legitimately owns, and it silently loses it. So the handle's
// prior ranges are snapshotted and restored instead.
func (d *Dispatcher) applyLockOps(open *Open, ops []lockOp) (int, error) {
	// A single-element request has nothing to undo: either its one element is
	// applied, or nothing happened at all.
	var snap []lockRange
	if len(ops) > 1 {
		snap = d.locks.snapshotRanges(open)
	}
	for i, op := range ops {
		if err := d.locks.applyLock(open, op.el.Offset, op.el.Length, op.kind); err != nil {
			if len(ops) > 1 {
				d.restoreLocks(open, snap)
			}
			return i, err
		}
	}
	return -1, nil
}

// restoreLocks puts open's byte-range lock state back to snap.
func (d *Dispatcher) restoreLocks(open *Open, snap []lockRange) {
	// Drop what this request added and the handle did not hold before.
	for _, r := range rangesDiff(d.locks.snapshotRanges(open), snap) {
		if err := d.locks.applyLock(open, r.start, r.length, lockUnlock); err != nil {
			d.Log.Warn("lock: rollback unlock failed",
				"offset", r.start, "length", r.length, "err", err)
		}
	}
	// Re-apply the whole prior state, not merely the entries that went
	// missing: on linux the kernel splits an OFD lock when a sub-range of it
	// is unlocked, so a range the table still reports as held can already have
	// a hole punched in it by the unlocks above. Re-locking a range the handle
	// still holds is a no-op on both platforms.
	for _, r := range snap {
		if err := d.locks.applyLock(open, r.start, r.length, r.kind()); err != nil {
			d.Log.Warn("lock: rollback could not restore a pre-existing lock",
				"path", open.Path, "offset", r.start, "length", r.length, "err", err)
		}
	}
}

// snapshotRanges returns the byte ranges open currently holds, as recorded by
// the lock table. Both platform lockManagers carry the same table (darwin as
// the authority, linux as a shadow of the kernel's OFD locks), so this
// bookkeeping is written once here rather than duplicated per OS.
func (m *lockManager) snapshotRanges(open *Open) []lockRange {
	key, err := m.keyFor(open)
	if err != nil {
		return nil
	}
	return m.tbl.snapshot(key, open)
}

// releaseSignal exposes the lock table's "a range was released" broadcast to
// parked LOCK requests.
func (m *lockManager) releaseSignal() <-chan struct{} { return m.tbl.releaseSignal() }

// startLockWait parks a blocking LOCK request and sends the interim
// STATUS_PENDING response. It reports false when the server is already at its
// pending-request cap, in which case the caller answers the conflict directly.
func (d *Dispatcher) startLockWait(rw io.ReadWriter, hdr smb2.Header, sess *Session, open *Open, ops []lockOp) bool {
	if pendingLockWaits.Add(1) > maxPendingLockWaits {
		pendingLockWaits.Add(-1)
		d.Log.Warn("lock: too many pending blocking locks, failing immediately")
		return false
	}
	asyncID := d.nextAsyncID()
	// Register in the same table CHANGE_NOTIFY uses, so SMB2_CANCEL, CLOSE,
	// TREE_DISCONNECT, LOGOFF and connection teardown all complete this
	// request instead of leaving the client waiting on a handle that is gone.
	reg := &notifyReg{open: open, cancel: make(chan struct{}), status: smb2.StatusCancelled}
	d.registerNotify(hdr.MessageID, reg)
	d.sendAsync(rw, hdr, sess, asyncID, smb2.StatusPending, interimLockBody)
	go d.awaitLock(rw, hdr, sess, open, ops, asyncID, reg)
	return true
}

// awaitLock retries a parked LOCK request until it is granted, cancelled or
// times out, then posts the async completion. It runs on its own goroutine:
// the connection's read loop has already returned and keeps serving other
// requests while this one waits.
func (d *Dispatcher) awaitLock(rw io.ReadWriter, hdr smb2.Header, sess *Session, open *Open, ops []lockOp, asyncID uint64, reg *notifyReg) {
	defer pendingLockWaits.Add(-1)
	// Gately 新增：panic 时断开连接，不让 panic 结束整个进程
	defer recoverConnPanic(d.Log, "lock-wait", d.killConn)
	defer d.unregisterNotify(hdr.MessageID)

	timeout := time.NewTimer(lockWaitTimeout)
	defer timeout.Stop()

	for {
		// Subscribe before retrying: a release landing between the attempt and
		// the select below must not be missed, or this request sleeps until
		// its timeout while the range sits free.
		released := d.locks.releaseSignal()

		// Hold the handle for the retry. applyLockOps fcntls on open.File, and
		// this goroutine is the one place that touches a descriptor without the
		// dispatcher's per-message hold — so without this it can race a release
		// (a CLOSE, a TREE_DISCONNECT, or a cross-connection session teardown)
		// that is closing the very fd it is about to lock. The lock is taken
		// SHARED and only around the attempt: the select below must not hold
		// it, or a release would wait out this request's whole timeout.
		// handleLock's own call is left alone — it already runs under this
		// lock, held exclusively for the length of the message.
		open.mu.RLock()
		idx, err := d.applyLockOps(open, ops)
		open.mu.RUnlock()
		switch {
		case err == nil:
			d.sendAsync(rw, hdr, sess, asyncID, smb2.StatusSuccess, smb2.EncodeLockResponse())
			return
		case errors.Is(err, errLockConflict) && ops[idx].blocking():
			// Still held by someone else: keep waiting.
		default:
			d.sendAsync(rw, hdr, sess, asyncID, lockStatus(err), interimLockBody)
			return
		}

		select {
		case <-released:
		case <-reg.cancel:
			// SMB2_CANCEL, or the handle going away, completed us. Either way
			// the client gets STATUS_CANCELLED: the notify-specific cleanup
			// status this registration carries means nothing for a LOCK.
			d.sendAsync(rw, hdr, sess, asyncID, smb2.StatusCancelled, interimLockBody)
			return
		case <-timeout.C:
			d.Log.Debug("lock: blocking request timed out", "path", open.Path)
			d.sendAsync(rw, hdr, sess, asyncID, smb2.StatusLockNotGranted, interimLockBody)
			return
		}
	}
}
