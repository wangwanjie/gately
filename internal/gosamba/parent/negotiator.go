// Gately 修改：仅在 Linux 与 macOS 编译（构建约束）；NegotiatorOptions 新增 MaxFrame，协商帧的上限可调小。

//go:build linux || darwin

package parent

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/wangwanjie/gately/internal/gosamba/smb2"
	"github.com/wangwanjie/gately/internal/gosamba/transport"
)

// Connection holds per-TCP-connection state established during NEGOTIATE.
type Connection struct {
	ClientGuid           [16]byte
	ServerGuid           [16]byte
	Selection            smb2.Selection
	Preauth              *smb2.PreauthHash
	NegotiateRequestMsg  []byte
	NegotiateResponseMsg []byte

	// MaxIOSize is the MaxRead/MaxWrite/MaxTransact size advertised in the
	// NEGOTIATE response. READ/WRITE handlers clamp client-supplied lengths to
	// it so a single request can't drive an unbounded allocation.
	MaxIOSize uint32

	// NegotiatedCapabilities and NegotiatedSecurityMode are the exact
	// Capabilities and SecurityMode values this connection put on the wire in
	// its NEGOTIATE response. They are recorded here — rather than recomputed
	// wherever they are needed again — because FSCTL_VALIDATE_NEGOTIATE_INFO
	// has to echo them back byte for byte: the client saved the NEGOTIATE
	// values and compares Capabilities, ServerGuid, SecurityMode and Dialect
	// to spot a downgrade. Two independent computations of "what we sent"
	// drift the moment either side of the server changes, and the client reads
	// drift as an attack (macOS: EAUTH at mount, ENOTCONN on reconnect).
	NegotiatedCapabilities smb2.Capabilities
	NegotiatedSecurityMode uint16

	// resumeKeys maps SRV_RESUME_KEYs issued by FSCTL_SRV_REQUEST_RESUME_KEY
	// to the opens they name, for FSCTL_SRV_COPYCHUNK. It lives on the
	// connection so the keys die with it; see copychunk.go.
	resumeKeys resumeKeyTable

	// sharingWaits counts the CREATEs this connection currently has parked on
	// another handle's deny mode. A parked CREATE holds one of the connection's
	// bounded worker-pool slots (see connPool.submit, which stops reading
	// frames when they are all busy), so the count is capped at
	// maxSharingWaits and everything past the cap is refused immediately —
	// exactly as it was before the wait existed.
	sharingWaits atomic.Int32

	// AAPLReadDirAttr latches once the client has negotiated AAPL with
	// SUPPORTS_READ_DIR_ATTR. Subsequent QUERY_DIRECTORY level-37 responses
	// then overlay Apple metadata (max_access, rfork_size, FinderInfo,
	// UNIX mode, inode) onto the FileIdBothDirectoryInformation record so
	// Finder doesn't have to re-CREATE/QUERY_INFO each entry.
	AAPLReadDirAttr bool

	// Durable is the server-scoped durable-handle table. It is shared across
	// every connection (set from ConnOptions) so a durable open survives the
	// drop of the TCP connection that created it: a client reconnecting within
	// DurableTimeout can reclaim its handle.
	Durable *DurableTable
	// DurableTimeout caps how long a durable handle stays reclaimable.
	DurableTimeout time.Duration
}

// NegotiatorOptions configures behavior.
type NegotiatorOptions struct {
	RequireEncryption bool
	RequireSigning    bool
	MaxIOSize         uint32
	ServerStartTime   uint64
	// MaxFrame 为 Gately 新增：读取协商帧的上限，0 表示 transport.MaxFrameSize
	MaxFrame uint32
}

// Negotiate reads one NEGOTIATE request from rw and writes the response back.
// Handles the macOS/Windows multi-protocol negotiate: if the first frame is
// an SMB1 NEGOTIATE_PROTOCOL, reply with DialectRevision=0x02FF and loop back
// to read the real SMB2 NEGOTIATE.
func Negotiate(rw io.ReadWriter, opts NegotiatorOptions, log *slog.Logger) (*Connection, error) {
	maxFrame := opts.MaxFrame
	if maxFrame == 0 {
		maxFrame = transport.MaxFrameSize
	}
	frame, err := transport.ReadFrame(rw, maxFrame)
	if err != nil {
		return nil, fmt.Errorf("read negotiate frame: %w", err)
	}
	if len(frame) >= 4 && frame[0] == 0xFF && frame[1] == 'S' && frame[2] == 'M' && frame[3] == 'B' {
		// SMB1 multi-protocol NEGOTIATE.
		log.Info("smb1 multi-protocol negotiate; upgrading to SMB2")
		if err := writeMultiProtocolUpgradeResponse(rw, opts); err != nil {
			return nil, fmt.Errorf("write smb2 upgrade response: %w", err)
		}
		// The client's next frame will be a real SMB2 NEGOTIATE.
		frame, err = transport.ReadFrame(rw, maxFrame)
		if err != nil {
			return nil, fmt.Errorf("read smb2 negotiate after upgrade: %w", err)
		}
	}
	if len(frame) < smb2.HeaderSize {
		return nil, fmt.Errorf("negotiate frame too short: %d bytes", len(frame))
	}
	hdr, err := smb2.DecodeHeader(frame[:smb2.HeaderSize])
	if err != nil {
		return nil, fmt.Errorf("decode header: %w", err)
	}
	if hdr.Command != smb2.CommandNegotiate {
		return nil, fmt.Errorf("expected NEGOTIATE, got %s", hdr.Command)
	}
	req, err := smb2.DecodeNegotiateRequest(frame[smb2.HeaderSize:])
	if err != nil {
		return nil, fmt.Errorf("decode negotiate body: %w", err)
	}

	sel, err := smb2.Select(req, opts.RequireEncryption)
	if err != nil {
		return nil, fmt.Errorf("select: %w", err)
	}

	conn := &Connection{
		ClientGuid: req.ClientGuid,
		Selection:  sel,
		Preauth:    smb2.NewPreauthHash(),
	}
	if _, err := rand.Read(conn.ServerGuid[:]); err != nil {
		return nil, fmt.Errorf("server guid: %w", err)
	}

	conn.NegotiateRequestMsg = append([]byte(nil), frame...)
	// Preauth-integrity hash is only used for 3.1.1.
	if sel.Dialect == smb2.Dialect311 {
		conn.Preauth.Update(conn.NegotiateRequestMsg)
	}

	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("salt: %w", err)
	}
	maxIO := opts.MaxIOSize
	if maxIO == 0 {
		maxIO = 8 << 20 // 8 MiB — clients negotiate down if they need to.
	}
	// Remember what we advertised so READ/WRITE/QUERY handlers can reject a
	// client that asks for more than the negotiated maximum instead of trying
	// to allocate an arbitrary wire-supplied size.
	conn.MaxIOSize = maxIO
	secMode := smb2.NegotiateSigningEnabled
	if opts.RequireSigning {
		secMode |= smb2.NegotiateSigningRequired
	}
	caps := smb2.CapLargeMTU
	if sel.Cipher != 0 && sel.Dialect >= smb2.Dialect300 {
		caps |= smb2.CapEncryption
	}

	// Record what we are about to advertise before encoding it, so
	// FSCTL_VALIDATE_NEGOTIATE_INFO echoes these exact values instead of a
	// second, independently-derived guess at them.
	conn.NegotiatedSecurityMode = secMode
	conn.NegotiatedCapabilities = caps

	respBody := smb2.NegotiateResponse{
		SecurityMode:    conn.NegotiatedSecurityMode,
		Dialect:         sel.Dialect,
		ServerGuid:      conn.ServerGuid,
		Capabilities:    conn.NegotiatedCapabilities,
		MaxTransactSize: maxIO,
		MaxReadSize:     maxIO,
		MaxWriteSize:    maxIO,
		SystemTime:      filetimeNow(),
		ServerStartTime: opts.ServerStartTime,
		SecurityBuffer:  smb2.NegotiateSecurityBlob,
	}
	if sel.Dialect == smb2.Dialect311 {
		respBody.PreauthIntegrity = &smb2.PreauthIntegrityContext{
			HashAlgorithms: []smb2.Hash{sel.Hash},
			Salt:           salt,
		}
		if sel.Cipher != 0 {
			respBody.Encryption = &smb2.EncryptionContext{Ciphers: []smb2.Cipher{sel.Cipher}}
		}
		if req.SigningCaps != nil {
			respBody.SigningCaps = &smb2.SigningCapsContext{Algorithms: []smb2.SigningAlgo{sel.SigningAlgo}}
		}
	}

	body, err := smb2.EncodeNegotiateResponse(respBody)
	if err != nil {
		return nil, fmt.Errorf("encode negotiate response: %w", err)
	}

	respHdr := smb2.Header{
		CreditCharge:   1,
		Command:        smb2.CommandNegotiate,
		CreditResponse: grantCredits(hdr.CreditCharge, hdr.CreditResponse),
		Flags:          smb2.FlagServerToRedir,
		MessageID:      hdr.MessageID,
	}
	full := make([]byte, smb2.HeaderSize+len(body))
	if err := smb2.EncodeHeader(full[:smb2.HeaderSize], respHdr); err != nil {
		return nil, err
	}
	copy(full[smb2.HeaderSize:], body)

	conn.NegotiateResponseMsg = append([]byte(nil), full...)
	if sel.Dialect == smb2.Dialect311 {
		conn.Preauth.Update(conn.NegotiateResponseMsg)
	}

	if err := transport.WriteFrame(rw, full); err != nil {
		return nil, fmt.Errorf("write negotiate response: %w", err)
	}

	log.Info("negotiated",
		"dialect", fmt.Sprintf("0x%04x", uint16(sel.Dialect)),
		"cipher", fmt.Sprintf("0x%04x", uint16(sel.Cipher)),
		"signing", fmt.Sprintf("0x%04x", uint16(sel.SigningAlgo)),
	)
	return conn, nil
}

// filetimeNow returns the current time as Windows FILETIME.
func filetimeNow() uint64 {
	const epochDelta = 11644473600
	now := time.Now().UTC()
	secs := uint64(now.Unix() + epochDelta)
	return secs*10_000_000 + uint64(now.Nanosecond())/100
}

var ErrNegotiate = errors.New("negotiate")

// writeMultiProtocolUpgradeResponse writes the SMB2 NEGOTIATE response with
// DialectRevision=0x02FF that tells a multi-protocol-negotiating client
// (macOS Finder, Windows pre-SMB2-only clients) to send a real SMB2 NEGOTIATE
// next. Per MS-SMB2 §3.3.5.3.1: no negotiate contexts, no security buffer.
func writeMultiProtocolUpgradeResponse(rw io.ReadWriter, opts NegotiatorOptions) error {
	const dialect02FF = 0x02FF

	// 64-byte body (StructureSize=65 includes one variable byte, but we send no
	// security buffer or contexts so the body stays at 64 bytes).
	body := make([]byte, 64)
	body[0] = 65
	// SecurityMode: signing enabled (don't require — the client hasn't picked).
	body[2] = 0x01
	// DialectRevision = 0x02FF
	body[4], body[5] = 0xFF, 0x02
	// NegotiateContextCount = 0
	// ServerGuid: random
	if _, err := rand.Read(body[8:24]); err != nil {
		return err
	}
	// Capabilities = 0
	// MaxTransactSize / MaxReadSize / MaxWriteSize: reasonable defaults
	for _, off := range []int{28, 32, 36} {
		body[off] = 0x00
		body[off+1] = 0x00
		body[off+2] = 0x10
		body[off+3] = 0x00 // 1 MiB
	}
	// SystemTime
	t := filetimeNow()
	for i := 0; i < 8; i++ {
		body[40+i] = byte(t >> (8 * i))
	}
	// ServerStartTime, SecurityBuffer*, NegotiateContextOffset all zero.

	// For SMB1 multi-protocol upgrade, there's no SMB2 credit request from the
	// client. Grant a small initial window; the real SMB2 NEGOTIATE response
	// will grow it based on the client's actual request.
	respHdr := smb2.Header{
		CreditCharge:   1,
		Command:        smb2.CommandNegotiate,
		CreditResponse: grantCredits(1, 1),
		Flags:          smb2.FlagServerToRedir,
		MessageID:      0,
	}
	full := make([]byte, smb2.HeaderSize+len(body))
	if err := smb2.EncodeHeader(full[:smb2.HeaderSize], respHdr); err != nil {
		return err
	}
	copy(full[smb2.HeaderSize:], body)
	_ = dialect02FF // documentation reference
	return transport.WriteFrame(rw, full)
}
