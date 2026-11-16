// Gately 修改：报文中的偏移与长度改用 bounds 包在 uint64 中判断边界，32 位平台上不会因 int 转换为负数而越界。

package smb2

import (
	"encoding/binary"
	"fmt"

	"github.com/wangwanjie/gately/internal/gosamba/bounds"
)

// Dialect identifies an SMB2/3 dialect revision.
type Dialect uint16

const (
	Dialect202 Dialect = 0x0202
	Dialect210 Dialect = 0x0210
	Dialect300 Dialect = 0x0300
	Dialect302 Dialect = 0x0302
	Dialect311 Dialect = 0x0311
)

// SecurityMode flags carried in NEGOTIATE.
const (
	NegotiateSigningEnabled  uint16 = 0x0001
	NegotiateSigningRequired uint16 = 0x0002
)

// Capabilities flags.
type Capabilities uint32

const (
	CapDFS               Capabilities = 0x00000001
	CapLeasing           Capabilities = 0x00000002
	CapLargeMTU          Capabilities = 0x00000004
	CapMultiChannel      Capabilities = 0x00000008
	CapPersistentHandles Capabilities = 0x00000010
	CapDirectoryLeasing  Capabilities = 0x00000020
	CapEncryption        Capabilities = 0x00000040
)

// NegotiateContextType identifies a negotiate context.
type NegotiateContextType uint16

const (
	CtxPreauthIntegrityCaps NegotiateContextType = 0x0001
	CtxEncryptionCaps       NegotiateContextType = 0x0002
	CtxCompressionCaps      NegotiateContextType = 0x0003
	CtxNetnameNegotiateCaps NegotiateContextType = 0x0005
	CtxTransportCaps        NegotiateContextType = 0x0006
	CtxRDMATransformCaps    NegotiateContextType = 0x0007
	CtxSigningCaps          NegotiateContextType = 0x0008
)

// Cipher identifies an encryption cipher.
type Cipher uint16

const (
	CipherAES128CCM Cipher = 0x0001
	CipherAES128GCM Cipher = 0x0002
	CipherAES256CCM Cipher = 0x0003
	CipherAES256GCM Cipher = 0x0004
)

// Hash identifies a preauth-integrity hash.
type Hash uint16

const HashSHA512 Hash = 0x0001

// SigningAlgo identifies a signing algorithm.
type SigningAlgo uint16

const (
	SigningHMACSHA256 SigningAlgo = 0x0000
	SigningAESCMAC    SigningAlgo = 0x0001
	SigningAESGMAC    SigningAlgo = 0x0002
)

const negotiateRequestStructureSize = 36

// NegotiateRequest is a decoded NEGOTIATE request body.
type NegotiateRequest struct {
	SecurityMode uint16
	Capabilities Capabilities
	ClientGuid   [16]byte
	Dialects     []Dialect

	PreauthIntegrity PreauthIntegrityContext
	Encryption       *EncryptionContext
	SigningCaps      *SigningCapsContext
}

type PreauthIntegrityContext struct {
	HashAlgorithms []Hash
	Salt           []byte
}

type EncryptionContext struct {
	Ciphers []Cipher
}

type SigningCapsContext struct {
	Algorithms []SigningAlgo
}

// DecodeNegotiateRequest parses the body bytes (header already stripped).
// Negotiate-context offsets in the wire are absolute from the SMB2 header start;
// we assume the standard 64-byte header.
func DecodeNegotiateRequest(body []byte) (NegotiateRequest, error) {
	const headerSize = 64
	if len(body) < negotiateRequestStructureSize {
		return NegotiateRequest{}, fmt.Errorf("%w: NEGOTIATE body min %d, got %d", ErrShortBuffer, negotiateRequestStructureSize, len(body))
	}
	if ss := binary.LittleEndian.Uint16(body[0:]); ss != negotiateRequestStructureSize {
		return NegotiateRequest{}, fmt.Errorf("%w: NEGOTIATE StructureSize %d", ErrBadStructureSize, ss)
	}

	dialectCount := binary.LittleEndian.Uint16(body[2:])
	var req NegotiateRequest
	req.SecurityMode = binary.LittleEndian.Uint16(body[4:])
	req.Capabilities = Capabilities(binary.LittleEndian.Uint32(body[8:]))
	copy(req.ClientGuid[:], body[12:28])

	ctxOffsetAbs := binary.LittleEndian.Uint32(body[28:])
	ctxCount := binary.LittleEndian.Uint16(body[32:])

	if int(negotiateRequestStructureSize)+int(dialectCount)*2 > len(body) {
		return NegotiateRequest{}, fmt.Errorf("%w: dialect array overruns body", ErrShortBuffer)
	}
	for i := 0; i < int(dialectCount); i++ {
		d := Dialect(binary.LittleEndian.Uint16(body[negotiateRequestStructureSize+i*2:]))
		req.Dialects = append(req.Dialects, d)
	}

	has311 := false
	for _, d := range req.Dialects {
		if d == Dialect311 {
			has311 = true
			break
		}
	}
	if !has311 {
		return req, nil
	}
	if ctxCount == 0 {
		return req, nil
	}
	if ctxOffsetAbs < headerSize {
		return NegotiateRequest{}, fmt.Errorf("smb2: negotiate context offset %d before body start", ctxOffsetAbs)
	}
	// 偏移先在 uint64 中换算并确认不超出报文体，再转换为 int
	off, _, ok := bounds.RelSpan(uint64(ctxOffsetAbs), headerSize, 0, len(body))
	if !ok {
		return NegotiateRequest{}, fmt.Errorf("%w: negotiate context offset %d beyond body", ErrShortBuffer, ctxOffsetAbs)
	}
	for i := uint16(0); i < ctxCount; i++ {
		if off+8 > len(body) {
			return NegotiateRequest{}, fmt.Errorf("%w: context %d header overruns body", ErrShortBuffer, i)
		}
		ctype := NegotiateContextType(binary.LittleEndian.Uint16(body[off:]))
		dlen := int(binary.LittleEndian.Uint16(body[off+2:]))
		dstart := off + 8
		dend := dstart + dlen
		if dend > len(body) {
			return NegotiateRequest{}, fmt.Errorf("%w: context %d data overruns body", ErrShortBuffer, i)
		}
		data := body[dstart:dend]
		switch ctype {
		case CtxPreauthIntegrityCaps:
			pic, err := decodePreauth(data)
			if err != nil {
				return NegotiateRequest{}, err
			}
			req.PreauthIntegrity = pic
		case CtxEncryptionCaps:
			ec, err := decodeEncryption(data)
			if err != nil {
				return NegotiateRequest{}, err
			}
			req.Encryption = &ec
		case CtxSigningCaps:
			sc, err := decodeSigningCaps(data)
			if err != nil {
				return NegotiateRequest{}, err
			}
			req.SigningCaps = &sc
		default:
			// Ignore Compression, Netname, Transport, RDMA — irrelevant for v1.
		}
		off = dend
		if pad := off & 7; pad != 0 {
			off += 8 - pad
		}
	}
	return req, nil
}

func decodePreauth(d []byte) (PreauthIntegrityContext, error) {
	if len(d) < 4 {
		return PreauthIntegrityContext{}, fmt.Errorf("%w: preauth ctx min 4", ErrShortBuffer)
	}
	hashCount := int(binary.LittleEndian.Uint16(d[0:]))
	saltLen := int(binary.LittleEndian.Uint16(d[2:]))
	need := 4 + hashCount*2 + saltLen
	if len(d) < need {
		return PreauthIntegrityContext{}, fmt.Errorf("%w: preauth need %d got %d", ErrShortBuffer, need, len(d))
	}
	out := PreauthIntegrityContext{
		HashAlgorithms: make([]Hash, hashCount),
		Salt:           append([]byte(nil), d[4+hashCount*2:4+hashCount*2+saltLen]...),
	}
	for i := 0; i < hashCount; i++ {
		out.HashAlgorithms[i] = Hash(binary.LittleEndian.Uint16(d[4+i*2:]))
	}
	return out, nil
}

func decodeEncryption(d []byte) (EncryptionContext, error) {
	if len(d) < 2 {
		return EncryptionContext{}, fmt.Errorf("%w: encryption ctx min 2", ErrShortBuffer)
	}
	n := int(binary.LittleEndian.Uint16(d[0:]))
	if len(d) < 2+n*2 {
		return EncryptionContext{}, fmt.Errorf("%w: encryption ctx", ErrShortBuffer)
	}
	out := EncryptionContext{Ciphers: make([]Cipher, n)}
	for i := 0; i < n; i++ {
		out.Ciphers[i] = Cipher(binary.LittleEndian.Uint16(d[2+i*2:]))
	}
	return out, nil
}

func decodeSigningCaps(d []byte) (SigningCapsContext, error) {
	if len(d) < 2 {
		return SigningCapsContext{}, fmt.Errorf("%w: signing-caps ctx min 2", ErrShortBuffer)
	}
	n := int(binary.LittleEndian.Uint16(d[0:]))
	if len(d) < 2+n*2 {
		return SigningCapsContext{}, fmt.Errorf("%w: signing-caps ctx", ErrShortBuffer)
	}
	out := SigningCapsContext{Algorithms: make([]SigningAlgo, n)}
	for i := 0; i < n; i++ {
		out.Algorithms[i] = SigningAlgo(binary.LittleEndian.Uint16(d[2+i*2:]))
	}
	return out, nil
}

const negotiateResponseStructureSize = 65

// NegotiateResponse is the encoder input.
type NegotiateResponse struct {
	SecurityMode    uint16
	Dialect         Dialect
	ServerGuid      [16]byte
	Capabilities    Capabilities
	MaxTransactSize uint32
	MaxReadSize     uint32
	MaxWriteSize    uint32
	SystemTime      uint64
	ServerStartTime uint64
	SecurityBuffer  []byte

	PreauthIntegrity *PreauthIntegrityContext
	Encryption       *EncryptionContext
	SigningCaps      *SigningCapsContext
}

// EncodeNegotiateResponse builds the body bytes (no SMB2 header).
//
// For 3.1.1: PreauthIntegrity is required; Encryption / SigningCaps are
// included if non-nil. For 3.0.x / 2.x: no negotiate contexts are emitted
// (any non-nil PreauthIntegrity / Encryption / SigningCaps fields are ignored).
func EncodeNegotiateResponse(r NegotiateResponse) ([]byte, error) {
	const headerSize = 64

	is311 := r.Dialect == Dialect311
	if is311 && r.PreauthIntegrity == nil {
		return nil, fmt.Errorf("smb2: NEGOTIATE response requires PreauthIntegrity context for 3.1.1")
	}

	ctxCount := 0
	if is311 {
		ctxCount = 1
		if r.Encryption != nil {
			ctxCount++
		}
		if r.SigningCaps != nil {
			ctxCount++
		}
	}

	body := make([]byte, 64)
	body[0] = byte(negotiateResponseStructureSize)
	body[1] = 0x00
	binary.LittleEndian.PutUint16(body[2:], r.SecurityMode)
	binary.LittleEndian.PutUint16(body[4:], uint16(r.Dialect))
	binary.LittleEndian.PutUint16(body[6:], uint16(ctxCount))
	copy(body[8:24], r.ServerGuid[:])
	binary.LittleEndian.PutUint32(body[24:], uint32(r.Capabilities))
	binary.LittleEndian.PutUint32(body[28:], r.MaxTransactSize)
	binary.LittleEndian.PutUint32(body[32:], r.MaxReadSize)
	binary.LittleEndian.PutUint32(body[36:], r.MaxWriteSize)
	binary.LittleEndian.PutUint64(body[40:], r.SystemTime)
	binary.LittleEndian.PutUint64(body[48:], r.ServerStartTime)

	secBufOffsetInBody := 64
	secBufOffsetAbs := secBufOffsetInBody + headerSize
	binary.LittleEndian.PutUint16(body[56:], uint16(secBufOffsetAbs))
	binary.LittleEndian.PutUint16(body[58:], uint16(len(r.SecurityBuffer)))

	body = append(body, r.SecurityBuffer...)
	if !is311 {
		// Pre-3.1.1 has no NegotiateContext fields; just leave bytes 60..63 zero
		// (NegotiateContextOffset / Reserved2). The server must not emit any
		// negotiate contexts for these dialects (MS-SMB2 §3.3.5.4).
		return body, nil
	}
	for len(body)%8 != 0 {
		body = append(body, 0x00)
	}
	ctxOffsetAbs := len(body) + headerSize
	binary.LittleEndian.PutUint32(body[60:64], uint32(ctxOffsetAbs))

	body = appendCtx(body, CtxPreauthIntegrityCaps, encodePreauth(*r.PreauthIntegrity), true)
	if r.Encryption != nil {
		body = appendCtx(body, CtxEncryptionCaps, encodeEncryption(*r.Encryption), true)
	}
	if r.SigningCaps != nil {
		body = appendCtx(body, CtxSigningCaps, encodeSigningCaps(*r.SigningCaps), false)
	}
	return body, nil
}

func appendCtx(body []byte, t NegotiateContextType, data []byte, padAfter bool) []byte {
	hdr := make([]byte, 8)
	binary.LittleEndian.PutUint16(hdr[0:], uint16(t))
	binary.LittleEndian.PutUint16(hdr[2:], uint16(len(data)))
	body = append(body, hdr...)
	body = append(body, data...)
	if padAfter {
		for len(body)%8 != 0 {
			body = append(body, 0x00)
		}
	}
	return body
}

func encodePreauth(p PreauthIntegrityContext) []byte {
	out := make([]byte, 4+len(p.HashAlgorithms)*2+len(p.Salt))
	binary.LittleEndian.PutUint16(out[0:], uint16(len(p.HashAlgorithms)))
	binary.LittleEndian.PutUint16(out[2:], uint16(len(p.Salt)))
	for i, h := range p.HashAlgorithms {
		binary.LittleEndian.PutUint16(out[4+i*2:], uint16(h))
	}
	copy(out[4+len(p.HashAlgorithms)*2:], p.Salt)
	return out
}

func encodeEncryption(e EncryptionContext) []byte {
	out := make([]byte, 2+len(e.Ciphers)*2)
	binary.LittleEndian.PutUint16(out[0:], uint16(len(e.Ciphers)))
	for i, c := range e.Ciphers {
		binary.LittleEndian.PutUint16(out[2+i*2:], uint16(c))
	}
	return out
}

func encodeSigningCaps(s SigningCapsContext) []byte {
	out := make([]byte, 2+len(s.Algorithms)*2)
	binary.LittleEndian.PutUint16(out[0:], uint16(len(s.Algorithms)))
	for i, a := range s.Algorithms {
		binary.LittleEndian.PutUint16(out[2+i*2:], uint16(a))
	}
	return out
}
