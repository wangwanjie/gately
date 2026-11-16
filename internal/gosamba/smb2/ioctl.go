// Gately 修改：报文中的偏移与长度改用 bounds 包在 uint64 中判断边界，32 位平台上不会因 int 转换为负数而越界。

package smb2

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/wangwanjie/gately/internal/gosamba/bounds"
)

const (
	FsctlValidateNegotiateInfo uint32 = 0x00140204
	FsctlPipeTransceive        uint32 = 0x0011C017
	FsctlPipeWait              uint32 = 0x00110018
	FsctlDfsGetReferrals       uint32 = 0x00060194
	FsctlQueryNetworkIfaceInfo uint32 = 0x001401FC
	FsctlSrvCopyChunk          uint32 = 0x001440F2
	FsctlSrvCopyChunkWrite     uint32 = 0x001480F2
	FsctlSrvRequestResumeKey   uint32 = 0x00140078
)

const IoctlIsFsctl uint32 = 0x00000001

type IoctlRequest struct {
	CtlCode     uint32
	FileID      [16]byte
	InputBuffer []byte
	Flags       uint32
}

func DecodeIoctlRequest(body []byte) (IoctlRequest, error) {
	if len(body) < 56 {
		return IoctlRequest{}, fmt.Errorf("%w: IOCTL body min 56", ErrShortBuffer)
	}
	if ss := binary.LittleEndian.Uint16(body[0:]); ss != 57 {
		return IoctlRequest{}, fmt.Errorf("%w: IOCTL StructureSize %d", ErrBadStructureSize, ss)
	}
	const headerSize = 64
	r := IoctlRequest{CtlCode: binary.LittleEndian.Uint32(body[4:])}
	copy(r.FileID[:], body[8:24])
	inOffAbs := binary.LittleEndian.Uint32(body[24:])
	inLen := binary.LittleEndian.Uint32(body[28:])
	r.Flags = binary.LittleEndian.Uint32(body[48:])
	if inLen > 0 {
		if start, end, ok := bounds.RelSpan(uint64(inOffAbs), headerSize, uint64(inLen), len(body)); ok {
			r.InputBuffer = append([]byte(nil), body[start:end]...)
		}
	}
	return r, nil
}

type IoctlResponse struct {
	CtlCode      uint32
	FileID       [16]byte
	OutputBuffer []byte
	Flags        uint32
}

func EncodeIoctlResponse(r IoctlResponse) []byte {
	const fixed = 48
	out := make([]byte, fixed+len(r.OutputBuffer))
	binary.LittleEndian.PutUint16(out[0:], 49) // StructureSize
	binary.LittleEndian.PutUint32(out[4:], r.CtlCode)
	copy(out[8:24], r.FileID[:])
	const headerSize = 64
	binary.LittleEndian.PutUint32(out[24:], uint32(headerSize+fixed)) // input offset
	// input length is 0 (we never echo input)
	binary.LittleEndian.PutUint32(out[32:], uint32(headerSize+fixed)) // output offset
	binary.LittleEndian.PutUint32(out[36:], uint32(len(r.OutputBuffer)))
	binary.LittleEndian.PutUint32(out[40:], r.Flags)
	copy(out[fixed:], r.OutputBuffer)
	return out
}

// --- FSCTL_VALIDATE_NEGOTIATE_INFO (MS-SMB2 2.2.32.6) ---

// ValidateNegotiateInfoResponse is the VALIDATE_NEGOTIATE_INFO response body.
// Every field must repeat, byte for byte, what the NEGOTIATE response carried:
// the client saved those values at negotiate time and compares all four here to
// detect a downgrade by a man in the middle. The macOS client (smbfs_smb_2.c,
// smb2fs_smb_validate_neg_info) fails the mount with EAUTH on any mismatch, so
// a "close enough" answer is worse than no answer at all.
type ValidateNegotiateInfoResponse struct {
	Capabilities Capabilities
	Guid         [16]byte
	SecurityMode uint16
	Dialect      Dialect
}

// EncodeValidateNegotiateInfoResponse serializes r into the 24-byte
// VALIDATE_NEGOTIATE_INFO response buffer.
func EncodeValidateNegotiateInfoResponse(r ValidateNegotiateInfoResponse) []byte {
	out := make([]byte, 24)
	binary.LittleEndian.PutUint32(out[0:], uint32(r.Capabilities))
	copy(out[4:20], r.Guid[:])
	binary.LittleEndian.PutUint16(out[20:], r.SecurityMode)
	binary.LittleEndian.PutUint16(out[22:], uint16(r.Dialect))
	return out
}

// --- Server-side copy (MS-SMB2 2.2.32.1 - 2.2.32.4) ---

// ResumeKeyLen is the fixed width of an SRV_RESUME_KEY. It is opaque to the
// client, which only ever echoes it back in an SRV_COPYCHUNK_COPY.
const ResumeKeyLen = 24

// srvCopyChunkHeaderLen is SourceKey(24) + ChunkCount(4) + Reserved(4).
const srvCopyChunkHeaderLen = 32

// srvCopyChunkEntryLen is SourceOffset(8) + TargetOffset(8) + Length(4) +
// Reserved(4).
const srvCopyChunkEntryLen = 24

// ErrCopyChunkLimit reports an SRV_COPYCHUNK_COPY whose ChunkCount is zero or
// above the server's advertised ceiling. It is distinct from ErrShortBuffer
// because the server answers it with STATUS_INVALID_PARAMETER *plus* a
// populated SRV_COPYCHUNK_RESPONSE carrying the real limits, which is how a
// client learns what to retry with.
var ErrCopyChunkLimit = errors.New("smb2: copychunk request outside server limits")

// EncodeResumeKeyResponse builds the SRV_REQUEST_RESUME_KEY response
// (MS-SMB2 2.2.32.3): the 24-byte key, ContextLength = 0, and the four bytes of
// (unused) Context that Windows servers append. The macOS client asks for 0x20
// bytes of output and rejects anything shorter than 24, so the full 32 bytes is
// what interoperates.
func EncodeResumeKeyResponse(key [ResumeKeyLen]byte) []byte {
	out := make([]byte, 32)
	copy(out[0:ResumeKeyLen], key[:])
	// out[24:28] ContextLength = 0, out[28:32] Context — both zero.
	return out
}

// SrvCopyChunk is one source-range/target-range pair of an SRV_COPYCHUNK_COPY
// (MS-SMB2 2.2.32.1.1).
type SrvCopyChunk struct {
	SourceOffset uint64
	TargetOffset uint64
	Length       uint32
}

// SrvCopyChunkCopy is the SRV_COPYCHUNK_COPY request body (MS-SMB2 2.2.32.1):
// the source handle's resume key followed by the ranges to copy.
type SrvCopyChunkCopy struct {
	SourceKey [ResumeKeyLen]byte
	Chunks    []SrvCopyChunk
}

// DecodeSrvCopyChunkCopy parses an SRV_COPYCHUNK_COPY. maxChunks bounds the
// wire-supplied ChunkCount *before* the chunk slice is allocated, so a client
// claiming four billion chunks cannot drive the allocation; a count of zero or
// one above maxChunks comes back as ErrCopyChunkLimit.
func DecodeSrvCopyChunkCopy(b []byte, maxChunks uint32) (SrvCopyChunkCopy, error) {
	if len(b) < srvCopyChunkHeaderLen {
		return SrvCopyChunkCopy{}, fmt.Errorf("%w: SRV_COPYCHUNK_COPY min %d, got %d",
			ErrShortBuffer, srvCopyChunkHeaderLen, len(b))
	}
	var c SrvCopyChunkCopy
	copy(c.SourceKey[:], b[0:ResumeKeyLen])
	count := binary.LittleEndian.Uint32(b[24:])
	if count == 0 {
		return c, fmt.Errorf("%w: ChunkCount 0", ErrCopyChunkLimit)
	}
	if count > maxChunks {
		return c, fmt.Errorf("%w: ChunkCount %d exceeds %d", ErrCopyChunkLimit, count, maxChunks)
	}
	// count 已受 maxChunks 限制，仍在 uint64 中计算所需长度，不依赖调用方传入的上限大小
	need := uint64(srvCopyChunkHeaderLen) + uint64(count)*srvCopyChunkEntryLen
	if uint64(len(b)) < need {
		return c, fmt.Errorf("%w: SRV_COPYCHUNK_COPY needs %d bytes for %d chunks, got %d",
			ErrShortBuffer, need, count, len(b))
	}
	c.Chunks = make([]SrvCopyChunk, count)
	for i := range c.Chunks {
		off := srvCopyChunkHeaderLen + i*srvCopyChunkEntryLen
		c.Chunks[i] = SrvCopyChunk{
			SourceOffset: binary.LittleEndian.Uint64(b[off:]),
			TargetOffset: binary.LittleEndian.Uint64(b[off+8:]),
			Length:       binary.LittleEndian.Uint32(b[off+16:]),
		}
	}
	return c, nil
}

// EncodeSrvCopyChunkResponse builds the 12-byte SRV_COPYCHUNK_RESPONSE
// (MS-SMB2 2.2.32.2).
//
// The three fields carry two different meanings depending on the status the
// response is sent with. On STATUS_SUCCESS they are the counts actually
// written. On STATUS_INVALID_PARAMETER they are the server's ceilings —
// ChunksWritten is the maximum chunk count, ChunkBytesWritten the maximum bytes
// per chunk, TotalBytesWritten the maximum bytes per request — which is how a
// client that asked for too much learns what to retry with. Either way it goes
// out in the Buffer of a normal IOCTL response, not in the error data: the
// macOS client re-parses the failed reply as a StructureSize-49 IOCTL response.
func EncodeSrvCopyChunkResponse(chunksWritten, chunkBytesWritten, totalBytesWritten uint32) []byte {
	out := make([]byte, 12)
	binary.LittleEndian.PutUint32(out[0:], chunksWritten)
	binary.LittleEndian.PutUint32(out[4:], chunkBytesWritten)
	binary.LittleEndian.PutUint32(out[8:], totalBytesWritten)
	return out
}
