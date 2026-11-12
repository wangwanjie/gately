// Gately 修改：按客户端请求的格式回复裸 NTLMSSP 或 SPNEGO；AES-256 时另备 L=256 的签名密钥，签名密钥的读写都经过 signingMu（见 parent/gately.go）；仅在 Linux 与 macOS 编译（构建约束）。

//go:build linux || darwin

package parent

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	osuser "os/user"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/wangwanjie/gately/internal/gosamba/config"
	"github.com/wangwanjie/gately/internal/gosamba/ntlm"
	"github.com/wangwanjie/gately/internal/gosamba/smb2"
	"github.com/wangwanjie/gately/internal/gosamba/smb3"
	"github.com/wangwanjie/gately/internal/gosamba/transport"
)

// Tree is one TREE_CONNECT'd share within a Session.
type Tree struct {
	ID    uint32
	Share config.ShareConfig
}

// Open is a single open file/dir handle within a Session.
type Open struct {
	FileID        [16]byte
	Path          string // OS-absolute path (the base file, even for streams)
	IsDir         bool
	IsPipe        bool     // virtual handle on IPC$ (srvsvc, lsarpc, etc.)
	PipeName      string   // e.g. "srvsvc" — set on IPC$ pipe opens
	File          *os.File // nil for dirs and streams
	Tree          *Tree
	DeleteOnClose bool

	// LinkPath is the in-share symlink this CREATE traversed to reach Path,
	// and is EMPTY on every handle that did not traverse one.
	//
	// It exists because the two halves of a handle refer to different objects
	// once a symlink is involved. Path is the link's TARGET — that is what the
	// descriptor is open on, what READ and WRITE move bytes through, what
	// QUERY_INFO reports, what the share-mode and byte-range-lock tables are
	// keyed by, and what a durable reclaim re-opens. Making it the target is
	// what made an in-share symlink openable at all (see handleCreate), and it
	// is deliberate: a handle whose metadata described the link while its data
	// came from the target would report a size that does not match what READ
	// returns.
	//
	// But the handle's NAME is still the link. DELETE_ON_CLOSE and
	// FileRenameInformation are namespace operations, and running them against
	// Path made deleting a symlink over SMB delete the file it pointed at and
	// renaming one rename the target. POSIX draws the line in exactly this
	// place: open(2) follows the final symlink, unlink(2) and rename(2) do not.
	//
	// Only namePath() reads this field, and only those two operations call
	// namePath(), so an empty LinkPath leaves every pre-existing code path
	// working off Path exactly as before.
	LinkPath string

	// IsStream marks an ephemeral named-alternate-data-stream handle. macOS
	// uses NTFS stream syntax (foo.txt:com.apple.metadata:_kMDItemUserTags:$DATA)
	// to write extended attributes. Rather than persist a separate file per
	// xattr (which would litter the share with literal `:`-named files), we
	// accept reads/writes on a synthetic in-memory buffer and discard on
	// close — matching the existing "silently accept EA writes" pattern.
	IsStream   bool
	StreamName string
	streamBuf  []byte
	// streamWritten is set once the client WRITEs to this stream handle.
	// streamSynthetic marks a buffer we fabricated (e.g. an empty AFP_AfpInfo
	// blob) rather than loaded from disk; such a buffer is only flushed back to
	// an xattr if the client actually wrote to it, so a mere read doesn't
	// litter the file with a zeroed metadata stream.
	streamWritten   bool
	streamSynthetic bool

	// GrantedAccess is the access mask the CREATE actually granted on this
	// handle. Reported back via FileAccessInformation / FileAllInformation —
	// macOS reads this to decide whether to even attempt READ/WRITE/QUERY_DIR
	// on the handle. See durableWriteAccess for the bits that oblige the
	// server to keep a writable descriptor behind the handle.
	GrantedAccess uint32

	// Durable handle bookkeeping. When IsDurable is set, this Open is
	// registered in the connection's DurableTable under
	// (DurableClientGuid, DurableCreateGuid). A clean CLOSE removes the
	// entry; a dropped connection leaves it for reclaim until expiry.
	IsDurable         bool
	DurableClientGuid [16]byte
	DurableCreateGuid [16]byte

	// pipeOut buffers a queued DCE/RPC response that the next READ/IOCTL will
	// drain. Filled by handleWrite (transactional write+read pattern is used by
	// some clients).
	pipeOut []byte

	// Directory enumeration state (for repeated QUERY_DIRECTORY calls).
	dirEntries []os.DirEntry
	dirSent    int
	dirRestart bool

	// mu serializes the messages of concurrent frames that name this handle.
	//
	// Several of the fields above are mutable per-handle state with no lock of
	// their own: the enumeration cursor, a named stream's buffer, a pipe's
	// queued DCE/RPC response. They were safe only because a connection served
	// one request at a time. The dispatcher now takes this lock around every
	// message that carries a FileID (see lockOpenForMessage), shared for
	// READ/WRITE on an ordinary file — which pread/pwrite and touch nothing
	// here — and exclusive for everything else.
	mu sync.RWMutex
}

// namePath returns the path that NAMES this handle in the share's directory
// tree: the symlink the CREATE traversed, if it traversed one, and otherwise
// the handle's own path.
//
// This is the path the two namespace operations must act on — the unlink behind
// DELETE_ON_CLOSE and the rename behind FileRenameInformation. Everything else
// (the descriptor, READ/WRITE, QUERY_INFO, SET_INFO end-of-file, byte-range
// locks, durable reclaim) deliberately keeps using Path, which is the object the
// handle actually reads and writes.
//
// For a handle that never traversed a symlink — every handle before this change
// and the overwhelming majority after it — LinkPath is empty and this is just
// Path.
func (o *Open) namePath() string {
	if o.LinkPath != "" {
		return o.LinkPath
	}
	return o.Path
}

// durableWriteAccess is the set of GrantedAccess bits that promise the client
// it may modify the file through this handle. A handle carrying any of them
// must sit on a writable descriptor: the client trusts the granted mask and
// issues WRITE / SET_INFO without re-checking, so a read-only descriptor behind
// such a mask surfaces as a failure mid-transfer rather than at open time.
// It is the same set of bits handleCreate treats as "wants write".
const durableWriteAccess = smb2.AccessFileWriteData | smb2.AccessFileAppendData |
	smb2.AccessGenericWrite | smb2.AccessGenericAll

// Session holds per-SMB-session state once auth completes.
type Session struct {
	ID         uint64
	User       config.UserConfig
	IsGuest    bool
	SigningKey []byte
	// signingKeyAlt 为 Gately 补丁新增：协商 AES-256 时按 L=256 派生的签名密钥，见 parent/gately.go 的 verifySigned
	signingKeyAlt  []byte
	signingMu      sync.Mutex
	S2CCipherKey   []byte
	C2SCipherKey   []byte
	ApplicationKey []byte

	// Authenticated latches true once SESSION_SETUP completes successfully
	// (type-3 NTLM verified, or an accepted guest). Until then the session
	// exists only to carry the multi-leg NTLM handshake; the dispatcher must
	// refuse every non-SESSION_SETUP command on an unauthenticated session.
	Authenticated bool

	// gotEncrypted latches once the client has sent an encrypted frame.
	// We then reply encrypted for the rest of the session. It is read from
	// the CHANGE_NOTIFY async goroutine while the read loop may set it, so it
	// is accessed atomically.
	gotEncrypted atomic.Bool

	pendingChallenge [8]byte

	// preauth is this session's fork of the SMB 3.1.1 preauth-integrity chain.
	// Per MS-SMB2 §3.3.5.5.3 the chain forks per session: a new session copies
	// Connection.PreauthIntegrityHashValue (which only NEGOTIATE updates) and
	// folds its own SESSION_SETUP messages into the copy. Keeping the chain on
	// the connection instead would let a second session on the same TCP
	// connection derive its keys from a hash polluted by the first session's
	// handshake, so its keys would not match what the client computed.
	preauth *smb2.PreauthHash

	// ntlmNegotiate and ntlmChallenge hold the exact NTLMSSP type-1 and type-2
	// bytes of this session's handshake. They are retained solely so the
	// type-3 MIC can be recomputed (MS-NLMP §3.2.5.1.2), and are released as
	// soon as authentication completes.
	ntlmNegotiate []byte
	ntlmChallenge []byte

	mu    sync.Mutex
	trees map[uint32]*Tree
	opens map[[16]byte]*Open
	// dead latches when TakeAllOpens has claimed this session's handles for
	// release — LOGOFF, connection teardown, or a reconnect superseding it.
	// Once set, AddOpen refuses, because a handle added to the map after that
	// point is reachable by nobody: no later RemoveOpen or TakeAllOpens will
	// ever run on it, so its descriptor and its share-mode reservation would be
	// held for the life of the process, and a leaked reservation makes the file
	// unopenable by every client on every connection. See AddOpen.
	dead       bool
	nextTreeID atomic.Uint32
}

func (s *Session) initTables() {
	if s.trees == nil {
		s.trees = make(map[uint32]*Tree)
	}
	if s.opens == nil {
		s.opens = make(map[[16]byte]*Open)
	}
	if s.nextTreeID.Load() == 0 {
		s.nextTreeID.Store(1)
	}
}

// SetGotEncrypted latches that the client has sent at least one encrypted
// frame on this session. Safe to call concurrently with GotEncrypted.
func (s *Session) SetGotEncrypted() { s.gotEncrypted.Store(true) }

// GotEncrypted reports whether the client has sent an encrypted frame. It is
// read from the CHANGE_NOTIFY async goroutine and written by the read loop,
// so it is backed by an atomic.
func (s *Session) GotEncrypted() bool { return s.gotEncrypted.Load() }

func (s *Session) AddTree(share config.ShareConfig) *Tree {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initTables()
	t := &Tree{ID: s.nextTreeID.Add(1), Share: share}
	s.trees[t.ID] = t
	return t
}

func (s *Session) GetTree(id uint32) *Tree {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.trees[id]
}

func (s *Session) RemoveTree(id uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.trees, id)
}

// RemoveTreeAndOpens removes a tree and detaches every open belonging to it,
// returning those opens so the caller can release their locks and descriptors.
// TREE_DISCONNECT must not leave a share's file handles behind: MS-SMB2
// §3.3.5.9 requires the server to close them, and without it every fd stays
// open until the whole connection dies.
func (s *Session) RemoveTreeAndOpens(id uint32) []*Open {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.trees, id)
	var out []*Open
	for fid, o := range s.opens {
		if o.Tree != nil && o.Tree.ID == id {
			out = append(out, o)
			delete(s.opens, fid)
		}
	}
	return out
}

// TakeAllOpens removes and returns every open in the session, for LOGOFF or
// session teardown, and marks the session dead so nothing can be added behind
// it. The caller owns the release of everything returned.
//
// The dead latch is not bookkeeping, it is the other half of the ownership
// rule. Taking the map is only "I now own every handle this session has" if the
// map cannot be repopulated afterwards, and it can be: handleCreate resolves
// its session at the top of the message and then does real filesystem work
// before AddOpen, so a CREATE already in flight on the owning connection can
// reach AddOpen after a teardown has emptied the map. Without the latch that
// handle is orphaned — nothing can ever close its descriptor or drop its
// share-mode reservation.
func (s *Session) TakeAllOpens() []*Open {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dead = true
	out := make([]*Open, 0, len(s.opens))
	for fid, o := range s.opens {
		out = append(out, o)
		delete(s.opens, fid)
	}
	s.trees = make(map[uint32]*Tree)
	return out
}

// OpenCount reports how many file handles the session currently holds.
func (s *Session) OpenCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.opens)
}

// TreeCount reports how many trees the session currently holds.
func (s *Session) TreeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.trees)
}

// AddOpen registers a handle with the session.
//
// It reports false when the session has already been torn down (see
// TakeAllOpens). A caller that gets false still owns the handle it built and
// MUST release it — descriptor, share-mode reservation and anything else it
// acquired — and answer the client STATUS_USER_SESSION_DELETED, because the
// session the handle would have belonged to no longer exists.
func (s *Session) AddOpen(o *Open) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dead {
		return false
	}
	s.initTables()
	s.opens[o.FileID] = o
	return true
}

// AddOpenToTree registers a handle only if its tree is still connected.
//
// AddOpen's dead latch covers a session torn down under an in-flight CREATE,
// but TREE_DISCONNECT does not latch anything — RemoveTreeAndOpens sweeps the
// opens map and a CREATE that has not reached AddOpen yet is not in it. A
// CREATE can now park for up to sharingViolationWait waiting out a sharing
// conflict, which makes that window wide enough to matter: the handle would be
// published onto a tree no request can name again, so no CLOSE and no second
// TREE_DISCONNECT would ever release its descriptor or its share-mode
// reservation.
func (s *Session) AddOpenToTree(o *Open, treeID uint32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dead {
		return false
	}
	if _, ok := s.trees[treeID]; !ok {
		return false
	}
	s.initTables()
	s.opens[o.FileID] = o
	return true
}

func (s *Session) GetOpen(id [16]byte) *Open {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.opens[id]
}

func (s *Session) RemoveOpen(id [16]byte) *Open {
	s.mu.Lock()
	defer s.mu.Unlock()
	o := s.opens[id]
	delete(s.opens, id)
	return o
}

// RangeOpens calls fn for each open currently in the session. The session lock
// is held during the iteration, so fn must not call any Session method that
// also acquires the lock.
func (s *Session) RangeOpens(fn func(*Open)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, o := range s.opens {
		fn(o)
	}
}

// maxHalfOpenSessions bounds how many sessions one connection may have sitting
// in the middle of an NTLM handshake at once. Every NTLMSSP type-1 leg creates
// a session and nothing but a successful type-3 (or the connection dying) ever
// retires it, so without a cap an unauthenticated peer can hold one socket open
// and spray type-1 messages until the process runs out of memory. A real client
// has exactly one handshake in flight; even a client re-authenticating several
// users over one connection stays far below this. Mirrors the maxOpensPerSession
// cap in dispatch.go.
const maxHalfOpenSessions = 64

// ErrTooManyHalfOpenSessions is returned when a connection exceeds
// maxHalfOpenSessions. It is fatal to the connection: SESSION_SETUP errors tear
// the connection down, which is the right answer for a peer behaving this way.
var ErrTooManyHalfOpenSessions = errors.New("too many unauthenticated sessions on one connection")

// SessionTable is one TCP connection's session map: the sessions the dispatcher
// will serve on this connection, and the half-open cap that bounds an
// unauthenticated peer. Its counts are inherently per-connection.
//
// It is deliberately NOT the server-wide view. A session must only be usable
// from the connection that created it — Dispatch and the transform-header
// decrypt both resolve a SessionId through this table — so making it shared
// would let one connection drive another's sessions. The server-wide view lives
// in SessionIndex, which exists for exactly one purpose (finding the session a
// reconnecting client names in PreviousSessionId) and hands out no *Session to
// a serving path.
//
// SessionIds themselves come from the process-wide allocSessionID, not from
// this table, so an id names exactly one session across the whole server.
type SessionTable struct {
	mu   sync.Mutex
	byID map[uint64]*Session
	// halfOpen holds the ids of sessions created for an NTLM handshake that
	// has not completed. Membership is tracked here rather than by scanning
	// Session.Authenticated because that field is written by the read loop
	// without the table lock; keeping the bookkeeping in the table keeps the
	// cap race-free without changing how the rest of the server reads it.
	halfOpen map[uint64]struct{}
}

func NewSessionTable() *SessionTable {
	return &SessionTable{
		byID:     make(map[uint64]*Session),
		halfOpen: make(map[uint64]struct{}),
	}
}

func (t *SessionTable) New() *Session {
	id := allocSessionID()
	s := &Session{ID: id}
	t.mu.Lock()
	t.byID[id] = s
	t.mu.Unlock()
	return s
}

// NewHalfOpen creates a session for an in-flight NTLM handshake, refusing once
// the connection already holds maxHalfOpenSessions of them. The session is
// retired from the half-open set by MarkAuthenticated or Remove.
func (t *SessionTable) NewHalfOpen() (*Session, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.halfOpen) >= maxHalfOpenSessions {
		return nil, fmt.Errorf("%w (limit %d)", ErrTooManyHalfOpenSessions, maxHalfOpenSessions)
	}
	id := allocSessionID()
	s := &Session{ID: id}
	t.byID[id] = s
	t.halfOpen[id] = struct{}{}
	return s, nil
}

// MarkAuthenticated retires a session from the half-open set, freeing its slot.
// Idempotent, so a client that re-runs SESSION_SETUP on a live session doesn't
// double-free.
func (t *SessionTable) MarkAuthenticated(id uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.halfOpen, id)
}

// HalfOpenCount reports how many sessions are still mid-handshake.
func (t *SessionTable) HalfOpenCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.halfOpen)
}

// Remove drops a session from the table, invalidating its SessionId. LOGOFF
// must do this: leaving the entry keeps the id usable for further commands.
func (t *SessionTable) Remove(id uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.byID, id)
	delete(t.halfOpen, id)
}

func (t *SessionTable) Get(id uint64) *Session {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.byID[id]
}

// RangeSessions calls fn for each session in the table. The table lock is held
// during the iteration, so fn must not call SessionTable methods that also
// acquire the lock.
func (t *SessionTable) RangeSessions(fn func(*Session)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, s := range t.byID {
		fn(s)
	}
}

// SessionSetupHandler runs the NTLM exchange.
type SessionSetupHandler struct {
	Conn     *Connection
	Sessions *SessionTable
	Users    []config.UserConfig
	Shares   []config.ShareConfig
	Log      *slog.Logger

	// RequireEncryption mirrors the server's encryption policy. It decides
	// whether the SESSION_SETUP response carries SMB2_SESSION_FLAG_ENCRYPT_DATA,
	// which is the ONLY way a client learns it must start encrypting. The
	// dispatcher refuses cleartext once the session is up, so failing to send
	// this flag does not merely weaken security — it breaks the session
	// outright, because the client keeps sending in the clear and every
	// request is denied.
	RequireEncryption bool

	// Index is the server-scoped session index. Every session this handler
	// authenticates is registered in it under Host, and a reconnecting client's
	// PreviousSessionId is resolved through it — which is what lets a session
	// living on a DIFFERENT TCP connection be found and closed.
	//
	// When it is nil nothing is registered and PreviousSessionId can never
	// match, so the reconnect degrades to "the old session is left to the idle
	// reaper". ServeConn never leaves it nil: it falls back to an index of its
	// own, which reduces the scope to this one connection but keeps the
	// behaviour.
	Index *SessionIndex

	// Host identifies this connection to the index: the session table whose
	// SessionId must be invalidated, and the dispatcher that owns the resume
	// keys, notify registrations and durable entries of anything released.
	Host *sessionHost

	// Gately 新增：按来源地址限制认证失败次数。Limiter 为空时不限流；RemoteIP 为对端 IP。
	Limiter  AuthLimiter
	RemoteIP string
}

// AuthLimiter 是按来源地址的认证失败限流器（fileusers.Throttle 满足该接口）。
// Reserve 在校验密码前调用，返回 false 表示该来源失败过多，应直接拒绝；
// 返回 true 后必须调用 Done 报告结果。
type AuthLimiter interface {
	Reserve(ip string) bool
	Done(ip string, ok bool)
}

// clipUserName 截断写入日志的用户名：用户名来自未认证的 NTLM 消息，长度只受帧大小限制。
func clipUserName(s string) string {
	const max = 64
	if len(s) <= max {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// hasGuestShare reports whether any configured share allows anonymous access.
func (h *SessionSetupHandler) hasGuestShare() bool {
	for _, s := range h.Shares {
		if s.GuestOK {
			return true
		}
	}
	return false
}

// guestSystemUser returns a UserConfig stand-in for the anonymous guest.
// SystemUID/GID resolve to "nobody" (typically uid 65534 on Linux).
func guestSystemUser() config.UserConfig {
	uid, gid := 65534, 65534
	if u, err := lookupGuestUser(); err == nil {
		uid, gid = u.UID, u.GID
	}
	return config.UserConfig{
		Name:        "guest",
		SystemUser:  "nobody",
		SystemUID:   uid,
		SystemGID:   gid,
		AllowShares: []string{"*"},
	}
}

type uidGid struct{ UID, GID int }

func lookupGuestUser() (uidGid, error) {
	u, err := osuser.Lookup("nobody")
	if err != nil {
		return uidGid{}, err
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	return uidGid{UID: uid, GID: gid}, nil
}

// HandleSessionSetup processes one SESSION_SETUP request and writes the response.
// The full request frame (header + body) is passed in for preauth folding.
func (h *SessionSetupHandler) HandleSessionSetup(rw io.ReadWriter, hdr smb2.Header, body, fullRequestFrame []byte) (*Session, error) {
	req, err := smb2.DecodeSessionSetupRequest(body)
	if err != nil {
		return nil, fmt.Errorf("decode session-setup: %w", err)
	}
	ntlmMsg, err := smb2.UnwrapNTLM(req.SecurityBuffer)
	if err != nil {
		return nil, err
	}
	// Gately 修改：Linux 内核 cifs 客户端（sec=ntlmssp，默认）直接发送不带 SPNEGO 的 NTLMSSP 消息，
	// 并要求回复同样不带 SPNEGO，否则报 "blob signature incorrect"。与 Samba 一致，按请求的格式回复。
	raw := bytes.HasPrefix(req.SecurityBuffer, ntlmsspSignature)
	if len(ntlmMsg) < 12 {
		return nil, errors.New("ntlm message too short")
	}
	msgType := uint32(ntlmMsg[8]) | uint32(ntlmMsg[9])<<8 | uint32(ntlmMsg[10])<<16 | uint32(ntlmMsg[11])<<24

	switch msgType {
	case ntlm.MessageTypeNegotiate:
		return h.handleType1(rw, hdr, ntlmMsg, fullRequestFrame, raw)
	case ntlm.MessageTypeAuthenticate:
		// PreviousSessionId is honoured inside handleType3, between the point
		// where the client has proved who it is and the point where it is told
		// the session is up. Both halves of that matter: closing a session is a
		// teardown an unauthenticated peer must never be able to trigger by
		// naming someone else's SessionId, and a client told "you are set up"
		// must not be able to re-open its files before the old session has let
		// go of them — which is the whole point of MS-SMB2 §3.3.5.5.3.
		return h.handleType3(rw, hdr, ntlmMsg, fullRequestFrame, req.PreviousSessionID, raw)
	default:
		return nil, fmt.Errorf("unexpected NTLM message type 0x%x", msgType)
	}
}

// closePreviousSession implements the PreviousSessionId half of MS-SMB2
// §3.3.5.5.3: a client that reconnects because its own side of the old session
// broke names that session's id, and the server must close it.
//
// Without this the old session's descriptors, byte-range locks, share-mode
// reservations, server-side-copy resume keys and change-notify watches survive
// until the idle reaper runs — and those stale reservations block the very
// client that just reconnected.
//
// The lookup goes through the server-scoped index, so the previous session is
// found whether it lives on this connection or on another one. The
// cross-connection case is the common one and the one macOS actually hits: the
// client's own side breaks and it reconnects on a fresh TCP connection while
// the server still believes the old one is alive. SessionIndex.supersede does
// the identity check, removes the entry and drives the teardown on the
// connection that owns the session.
//
// A zero id, the current session's own id, an id nobody holds and an id owned
// by someone else are all silent no-ops: SESSION_SETUP must still succeed.
func (h *SessionSetupHandler) closePreviousSession(prevID uint64, sess *Session) {
	var guid [16]byte
	if h.Conn != nil {
		guid = h.Conn.ClientGuid
	}
	h.Index.supersede(prevID, sess, guid, h.Log)
}

func (h *SessionSetupHandler) handleType1(rw io.ReadWriter, hdr smb2.Header, type1, requestFrame []byte, raw bool) (*Session, error) {
	sess, err := h.Sessions.NewHalfOpen()
	if err != nil {
		return nil, err
	}

	if _, err := rand.Read(sess.pendingChallenge[:]); err != nil {
		return nil, err
	}

	avPairs := []ntlm.AVPair{
		{ID: ntlm.AVNbComputerName, Value: ntlm.UTF16LE("GOSAMBA")},
		{ID: ntlm.AVNbDomainName, Value: ntlm.UTF16LE("WORKGROUP")},
		{ID: ntlm.AVDnsComputerName, Value: ntlm.UTF16LE("gosamba")},
		{ID: ntlm.AVDnsDomainName, Value: ntlm.UTF16LE("workgroup")},
		{ID: ntlm.AVTimestamp, Value: filetimeBytes()},
	}
	type2 := ntlm.EncodeChallenge(ntlm.ChallengeMessage{
		TargetName: "GOSAMBA",
		Flags: ntlm.NegotiateUnicode | ntlm.NegotiateNTLM | ntlm.NegotiateExtendedSessionSecurity |
			ntlm.NegotiateTargetInfo | ntlm.NegotiateAlwaysSign | ntlm.RequestTarget |
			ntlm.TargetTypeServer | ntlm.Negotiate128 | ntlm.Negotiate56 |
			ntlm.NegotiateKeyExch | ntlm.NegotiateVersion | ntlm.NegotiateSign,
		Challenge:  sess.pendingChallenge,
		TargetInfo: avPairs,
	})

	// Retain both halves of the handshake so handleType3 can recompute the
	// NTLMSSP MIC. type1 is the mechToken UnwrapNTLM pulled out of the client's
	// SPNEGO NegTokenInit, trimmed to the token itself — so these are exactly
	// the NEGOTIATE_MESSAGE bytes the client hashed, whatever DER the client
	// put after the token. type2 is verbatim what we are about to send.
	sess.ntlmNegotiate = append([]byte(nil), type1...)
	sess.ntlmChallenge = append([]byte(nil), type2...)

	spnego := wrapNTLMResp(raw, smb2.SPNEGOAcceptIncomplete, type2)

	respBody := smb2.EncodeSessionSetupResponse(smb2.SessionSetupResponse{
		SessionFlags:   0,
		SecurityBuffer: spnego,
	})
	respHdr := smb2.Header{
		CreditCharge:   hdr.CreditCharge,
		Status:         uint32(smb2.StatusMoreProcessingReq),
		Command:        smb2.CommandSessionSetup,
		CreditResponse: grantCredits(hdr.CreditCharge, hdr.CreditResponse),
		Flags:          smb2.FlagServerToRedir,
		MessageID:      hdr.MessageID,
		SessionID:      sess.ID,
	}
	full := make([]byte, smb2.HeaderSize+len(respBody))
	if err := smb2.EncodeHeader(full[:smb2.HeaderSize], respHdr); err != nil {
		return nil, err
	}
	copy(full[smb2.HeaderSize:], respBody)

	// Fold request and response into this session's preauth chain (MS-SMB2
	// §3.1.4.4.1). The chain forks here: the session starts from a copy of the
	// connection hash (which carries only the NEGOTIATE exchange) so a second
	// session on this connection is not salted with the first one's
	// SESSION_SETUP messages.
	sess.preauth = forkPreauth(h.Conn.Preauth)
	sess.preauth.Update(requestFrame)
	sess.preauth.Update(full)

	if err := transport.WriteFrame(rw, full); err != nil {
		return nil, err
	}
	return nil, nil
}

// handleType3 在认证前向限流器占位：来源失败过多时直接拒绝并写日志，否则按结果记录成功或失败。
func (h *SessionSetupHandler) handleType3(rw io.ReadWriter, hdr smb2.Header, type3, requestFrame []byte, prevSessionID uint64, raw bool) (*Session, error) {
	if h.Limiter == nil {
		return h.verifyType3(rw, hdr, type3, requestFrame, prevSessionID, raw)
	}
	if !h.Limiter.Reserve(h.RemoteIP) {
		name := ""
		if auth, err := ntlm.DecodeAuthenticate(type3); err == nil {
			name = auth.UserName
		}
		return nil, h.failAuth(rw, hdr, fmt.Errorf("too many failed logins (user %q)", clipUserName(name)))
	}
	sess, err := h.verifyType3(rw, hdr, type3, requestFrame, prevSessionID, raw)
	h.Limiter.Done(h.RemoteIP, err == nil && sess != nil)
	return sess, err
}

func (h *SessionSetupHandler) verifyType3(rw io.ReadWriter, hdr smb2.Header, type3, requestFrame []byte, prevSessionID uint64, raw bool) (*Session, error) {
	sess := h.Sessions.Get(hdr.SessionID)
	if sess == nil {
		return nil, fmt.Errorf("unknown session id %d", hdr.SessionID)
	}
	auth, err := ntlm.DecodeAuthenticate(type3)
	if err != nil {
		return nil, fmt.Errorf("decode AUTH: %w", err)
	}

	var user *config.UserConfig
	for i := range h.Users {
		if strings.EqualFold(h.Users[i].Name, auth.UserName) {
			user = &h.Users[i]
			break
		}
	}

	isGuest := false
	if user == nil {
		// Guest fallback: empty username or "GUEST", and at least one share allows it.
		if (auth.UserName == "" || strings.EqualFold(auth.UserName, "GUEST")) && h.hasGuestShare() {
			gu := guestSystemUser()
			user = &gu
			isGuest = true
		} else {
			return nil, h.failAuth(rw, hdr, fmt.Errorf("unknown user %q", clipUserName(auth.UserName)))
		}
	}

	// Fold the type-3 request into this session's preauth chain, only for
	// 3.1.1 — pre-3.1.1 dialects don't use preauth integrity at all. A session
	// that somehow reached type-3 without a type-1 leg has no fork yet, so make
	// one from the connection chain rather than dereferencing nil.
	dialect := h.Conn.Selection.Dialect
	if sess.preauth == nil {
		sess.preauth = forkPreauth(h.Conn.Preauth)
	}
	if dialect == smb2.Dialect311 {
		sess.preauth.Update(requestFrame)
	}

	if !isGuest {
		sbk, err := ntlm.VerifyNTLMv2(user.NTHash, auth.UserName, auth.DomainName, sess.pendingChallenge, auth.NtResponse)
		if err != nil {
			return nil, h.failAuth(rw, hdr, fmt.Errorf("user %q: %w", clipUserName(auth.UserName), err))
		}

		// For NTLMv2 with NegotiateKeyExch, KeyExchangeKey = SessionBaseKey.
		sessionKey := sbk[:]
		if auth.Flags&ntlm.NegotiateKeyExch != 0 && len(auth.EncryptedRandomSessionKey) == 16 {
			sessionKey = make([]byte, 16)
			rc4xor(sbk[:], auth.EncryptedRandomSessionKey, sessionKey)
		}
		// sessionKey is now ExportedSessionKey, which is what the MIC is keyed
		// with, so the MIC can be checked before it is used to derive anything.
		if err := h.verifyMIC(sess, auth, type3, sessionKey); err != nil {
			return nil, h.failAuth(rw, hdr, fmt.Errorf("user %q: %w", clipUserName(auth.UserName), err))
		}

		// 签名密钥先算到局部变量，最后在锁内一次写入：同一会话可能被其他连接（会话绑定、断线重连）并发读取
		var signKey, signKeyAlt []byte
		switch dialect {
		case smb2.Dialect311:
			preauth := sess.preauth.Sum()
			// The cipher keys are derived at the length the negotiated cipher
			// actually consumes: MS-SMB2 §3.1.4.2 uses L=256 for the AES-256
			// ciphers and L=128 otherwise. Deriving 128 bits unconditionally
			// (as this did) makes every AES-256 session unusable — newAEAD
			// rejects the short key, so every encrypted frame fails — and
			// AES-256-GCM is exactly what a current Windows client picks. The
			// signing and application keys stay at 128 bits for every cipher:
			// SMB3 signing is always AES-128-CMAC/GMAC.
			cipherBits := smb3.CipherKeyBits(uint16(h.Conn.Selection.Cipher))
			signKey = smb3.KDF(sessionKey, []byte("SMBSigningKey\x00"), preauth[:], 128)
			sess.S2CCipherKey = smb3.KDF(sessionKey, []byte("SMBS2CCipherKey\x00"), preauth[:], cipherBits)
			sess.C2SCipherKey = smb3.KDF(sessionKey, []byte("SMBC2SCipherKey\x00"), preauth[:], cipherBits)
			sess.ApplicationKey = smb3.KDF(sessionKey, []byte("SMBAppKey\x00"), preauth[:], 128)
			if cipherBits == 256 {
				signKeyAlt = smb3.KDF(sessionKey, []byte("SMBSigningKey\x00"), preauth[:], 256)[:16]
			}
		case smb2.Dialect300, smb2.Dialect302:
			// 3.0 / 3.0.2 use fixed context strings instead of preauth. 128 bits
			// is right for every key here and needs no cipher-dependent length:
			// the AES-256 ciphers are negotiated through a 3.1.1 negotiate
			// context, so these dialects only ever run AES-128-CCM (which is
			// also all Select() will choose for them).
			signKey = smb3.KDF(sessionKey, []byte("SMB2AESCMAC\x00"), []byte("SmbSign\x00"), 128)
			sess.S2CCipherKey = smb3.KDF(sessionKey, []byte("SMB2AESCCM\x00"), []byte("ServerOut\x00"), 128)
			sess.C2SCipherKey = smb3.KDF(sessionKey, []byte("SMB2AESCCM\x00"), []byte("ServerIn \x00"), 128)
			sess.ApplicationKey = smb3.KDF(sessionKey, []byte("SMB2APP\x00"), []byte("SmbRpc\x00"), 128)
		default:
			// 2.0.2 / 2.1: Session.SigningKey = SessionKey (HMAC-SHA256 signing,
			// no SMB3 KDF). We store the 16-byte session key directly.
			signKey = append([]byte(nil), sessionKey...)
		}
		sess.setSigningKeys(signKey, signKeyAlt)
	}
	sess.User = *user
	sess.IsGuest = isGuest
	// Authentication is complete: the dispatcher may now serve commands on
	// this session. (Set before writing the response so a pipelined follow-up
	// request can never race ahead of the flag.)
	sess.Authenticated = true
	// The handshake is over: give the half-open slot back so a long-lived
	// connection that re-authenticates never accumulates against the cap, and
	// drop the retained NTLM messages now that the MIC has been checked.
	h.Sessions.MarkAuthenticated(sess.ID)
	sess.ntlmNegotiate = nil
	sess.ntlmChallenge = nil
	// The session now has a security context, so it can be named by a later
	// reconnect's PreviousSessionId — and can be superseded by one. Registering
	// only authenticated sessions is what makes a half-open session (which
	// nobody owns yet) unclosable by anybody.
	h.Index.register(sess, h.Host)

	// Close the session this one is replacing, BEFORE the response goes out.
	// The response is what tells the client its reconnect succeeded, and the
	// next thing a reconnecting client does is re-open the files it had open —
	// so anything still holding those files' share-mode reservations or
	// byte-range locks when the response lands is a sharing violation the
	// client sees for its own handles. Answering first and cleaning up
	// afterwards leaves exactly the window this change exists to close.
	h.closePreviousSession(prevSessionID, sess)

	var sessFlags uint16
	switch {
	case isGuest:
		sessFlags = smb2.SessionFlagIsGuest
	case h.RequireEncryption && h.Conn != nil && h.Conn.Selection.Cipher != 0:
		// Tell the client to encrypt everything from here on (MS-SMB2
		// §3.3.5.5.3). A guest session has no keys, and a session with no
		// negotiated cipher cannot encrypt at all — asserting the flag in
		// either case would ask for something the client cannot do.
		sessFlags = smb2.SessionFlagEncryptData
	}
	spnego := wrapNTLMResp(raw, smb2.SPNEGOAcceptCompleted, nil)
	respBody := smb2.EncodeSessionSetupResponse(smb2.SessionSetupResponse{
		SessionFlags:   sessFlags,
		SecurityBuffer: spnego,
	})
	hdrFlags := smb2.FlagServerToRedir
	if !isGuest {
		hdrFlags |= smb2.FlagSigned
	}
	respHdr := smb2.Header{
		CreditCharge:   hdr.CreditCharge,
		Status:         uint32(smb2.StatusSuccess),
		Command:        smb2.CommandSessionSetup,
		CreditResponse: grantCredits(hdr.CreditCharge, hdr.CreditResponse),
		Flags:          hdrFlags,
		MessageID:      hdr.MessageID,
		SessionID:      sess.ID,
	}
	full := make([]byte, smb2.HeaderSize+len(respBody))
	_ = smb2.EncodeHeader(full[:smb2.HeaderSize], respHdr)
	copy(full[smb2.HeaderSize:], respBody)
	if !isGuest {
		smb3.SignMessage(uint16(h.Conn.Selection.SigningAlgo), sess.signingKey(), full)
	}

	if err := transport.WriteFrame(rw, full); err != nil {
		return nil, err
	}
	h.Log.Info("session authenticated",
		"session_id", sess.ID,
		"smb_user", user.Name,
		"system_uid", user.SystemUID,
		"guest", isGuest,
	)
	return sess, nil
}

// verifyMIC checks the NTLMSSP AUTHENTICATE message-integrity code.
//
// The MIC is what authenticates the parts of the handshake that NTLMv2 itself
// leaves unprotected — above all the NEGOTIATE_MESSAGE's flags, which an active
// attacker would otherwise be free to rewrite (stripping NEGOTIATE_SEAL, say)
// without either end noticing. exportedSessionKey is the key the client used,
// i.e. the post-key-exchange session key.
//
// Whether a MIC is expected is decided by the MsvAvFlags bit inside the NTLMv2
// response, not by the presence of a MIC field: those AV pairs are covered by
// NTProofStr, which VerifyNTLMv2 has already checked, so the bit cannot have
// been cleared in flight. That closes the obvious downgrade — an attacker
// zeroing the MIC field and hoping the server shrugs.
func (h *SessionSetupHandler) verifyMIC(sess *Session, auth ntlm.AuthenticateMessage, type3, exportedSessionKey []byte) error {
	if !ntlm.MICRequired(auth.NtResponse) {
		// Client didn't compute one. Nothing to verify, and nothing an
		// attacker could have removed.
		return nil
	}
	if !auth.HasMIC {
		return errors.New("ntlm: AUTHENTICATE asserts a MIC but has no MIC field")
	}
	if len(sess.ntlmNegotiate) == 0 || len(sess.ntlmChallenge) == 0 {
		// Only reachable if the type-3 arrived on a session that never ran
		// through handleType1, in which case there is no handshake to hash.
		return errors.New("ntlm: MIC required but the handshake messages were not retained")
	}
	// Trim any SPNEGO tail (typically the mechListMIC that follows the
	// responseToken) so the hash covers exactly the AUTHENTICATE_MESSAGE.
	authMsg := type3[:ntlm.AuthenticateLen(type3)]
	if !ntlm.VerifyMIC(exportedSessionKey, sess.ntlmNegotiate, sess.ntlmChallenge, authMsg, auth.MIC) {
		return errors.New("ntlm: AUTHENTICATE MIC mismatch (handshake was tampered with)")
	}
	return nil
}

// forkPreauth returns an independent copy of a preauth chain. smb2.PreauthHash
// is a plain value (a 64-byte rolling state), so copying the struct copies the
// chain up to that point.
func forkPreauth(base *smb2.PreauthHash) *smb2.PreauthHash {
	if base == nil {
		return smb2.NewPreauthHash()
	}
	forked := *base
	return &forked
}

func (h *SessionSetupHandler) failAuth(rw io.ReadWriter, hdr smb2.Header, cause error) error {
	respHdr := smb2.Header{
		CreditCharge:   hdr.CreditCharge,
		Status:         uint32(smb2.StatusLogonFailure),
		Command:        smb2.CommandSessionSetup,
		CreditResponse: grantCredits(hdr.CreditCharge, hdr.CreditResponse),
		Flags:          smb2.FlagServerToRedir,
		MessageID:      hdr.MessageID,
		SessionID:      hdr.SessionID,
	}
	errBody := []byte{0x09, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	full := make([]byte, smb2.HeaderSize+len(errBody))
	_ = smb2.EncodeHeader(full[:smb2.HeaderSize], respHdr)
	copy(full[smb2.HeaderSize:], errBody)
	_ = transport.WriteFrame(rw, full)
	h.Log.Warn("auth failed", "err", cause)
	return cause
}

// rc4xor is a tiny RC4 implementation used once for NTLM key-exchange decrypt.
func rc4xor(key, in, out []byte) {
	var s [256]byte
	for i := 0; i < 256; i++ {
		s[i] = byte(i)
	}
	j := 0
	for i := 0; i < 256; i++ {
		j = (j + int(s[i]) + int(key[i%len(key)])) & 0xff
		s[i], s[j] = s[j], s[i]
	}
	i, jj := 0, 0
	for k := range in {
		i = (i + 1) & 0xff
		jj = (jj + int(s[i])) & 0xff
		s[i], s[jj] = s[jj], s[i]
		out[k] = in[k] ^ s[(int(s[i])+int(s[jj]))&0xff]
	}
}

// filetimeBytes returns the current time as Windows FILETIME (8 bytes LE).
func filetimeBytes() []byte {
	t := filetimeNow()
	return []byte{
		byte(t), byte(t >> 8), byte(t >> 16), byte(t >> 24),
		byte(t >> 32), byte(t >> 40), byte(t >> 48), byte(t >> 56),
	}
}
