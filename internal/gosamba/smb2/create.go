// Gately 修改：报文中的偏移与长度改用 bounds 包在 uint64 中判断边界，32 位平台上不会因 int 转换为负数而越界。

package smb2

import (
	"encoding/binary"
	"fmt"

	"github.com/wangwanjie/gately/internal/gosamba/bounds"
)

// Common DesiredAccess bits (file).
const (
	AccessFileReadData       uint32 = 0x00000001
	AccessFileWriteData      uint32 = 0x00000002
	AccessFileAppendData     uint32 = 0x00000004
	AccessFileReadEa         uint32 = 0x00000008
	AccessFileReadAttributes uint32 = 0x00000080
	AccessReadControl        uint32 = 0x00020000
	AccessGenericRead        uint32 = 0x80000000
	AccessGenericWrite       uint32 = 0x40000000
	AccessGenericExecute     uint32 = 0x20000000
	AccessGenericAll         uint32 = 0x10000000
)

// FileAttributes bits.
const (
	FileAttrReadOnly  uint32 = 0x00000001
	FileAttrHidden    uint32 = 0x00000002
	FileAttrSystem    uint32 = 0x00000004
	FileAttrDirectory uint32 = 0x00000010
	FileAttrArchive   uint32 = 0x00000020
	FileAttrNormal    uint32 = 0x00000080
)

// CreateDisposition values.
const (
	CreateDispositionSupersede   uint32 = 0
	CreateDispositionOpen        uint32 = 1
	CreateDispositionCreate      uint32 = 2
	CreateDispositionOpenIf      uint32 = 3
	CreateDispositionOverwrite   uint32 = 4
	CreateDispositionOverwriteIf uint32 = 5
)

// CreateAction values (response).
const (
	CreateActionSuperseded  uint32 = 0
	CreateActionOpened      uint32 = 1
	CreateActionCreated     uint32 = 2
	CreateActionOverwritten uint32 = 3
)

// CreateOptions bits.
const (
	CreateOptDirectoryFile uint32 = 0x00000001
	CreateOptNonDirFile    uint32 = 0x00000040
	CreateOptDeleteOnClose uint32 = 0x00001000
)

type CreateRequest struct {
	RequestedOplock    uint8
	ImpersonationLevel uint32
	DesiredAccess      uint32
	FileAttributes     uint32
	ShareAccess        uint32
	CreateDisposition  uint32
	CreateOptions      uint32
	Name               string
	CreateContexts     []byte // raw, not parsed in v1
}

func DecodeCreateRequest(body []byte) (CreateRequest, error) {
	if len(body) < 56 {
		return CreateRequest{}, fmt.Errorf("%w: CREATE body min 56", ErrShortBuffer)
	}
	if ss := binary.LittleEndian.Uint16(body[0:]); ss != 57 {
		return CreateRequest{}, fmt.Errorf("%w: CREATE StructureSize %d", ErrBadStructureSize, ss)
	}
	const headerSize = 64
	r := CreateRequest{
		RequestedOplock:    body[3],
		ImpersonationLevel: binary.LittleEndian.Uint32(body[4:]),
		DesiredAccess:      binary.LittleEndian.Uint32(body[24:]),
		FileAttributes:     binary.LittleEndian.Uint32(body[28:]),
		ShareAccess:        binary.LittleEndian.Uint32(body[32:]),
		CreateDisposition:  binary.LittleEndian.Uint32(body[36:]),
		CreateOptions:      binary.LittleEndian.Uint32(body[40:]),
	}
	nameOffAbs := binary.LittleEndian.Uint16(body[44:])
	nameLen := binary.LittleEndian.Uint16(body[46:])
	ccOffAbs := binary.LittleEndian.Uint32(body[48:])
	ccLen := binary.LittleEndian.Uint32(body[52:])

	if nameLen > 0 {
		off := int(nameOffAbs) - headerSize
		if off < 0 || off+int(nameLen) > len(body) {
			return CreateRequest{}, fmt.Errorf("%w: CREATE Name out of bounds", ErrShortBuffer)
		}
		r.Name = decodeUTF16LE(body[off : off+int(nameLen)])
	}
	if ccLen > 0 {
		start, end, ok := bounds.RelSpan(uint64(ccOffAbs), headerSize, uint64(ccLen), len(body))
		if !ok {
			return CreateRequest{}, fmt.Errorf("%w: CREATE CreateContexts out of bounds", ErrShortBuffer)
		}
		r.CreateContexts = append([]byte(nil), body[start:end]...)
	}
	return r, nil
}

type CreateResponse struct {
	OplockLevel    uint8
	Flags          uint8
	CreateAction   uint32
	CreationTime   uint64
	LastAccessTime uint64
	LastWriteTime  uint64
	ChangeTime     uint64
	AllocationSize uint64
	EndOfFile      uint64
	FileAttributes uint32
	FileID         [16]byte
	CreateContexts []byte // serialized CreateContext list (already padded internally)
}

func EncodeCreateResponse(r CreateResponse) []byte {
	const headerSize = 64
	const fixed = 88
	body := make([]byte, fixed)
	binary.LittleEndian.PutUint16(body[0:], 89) // StructureSize
	body[2] = r.OplockLevel
	body[3] = r.Flags
	binary.LittleEndian.PutUint32(body[4:], r.CreateAction)
	binary.LittleEndian.PutUint64(body[8:], r.CreationTime)
	binary.LittleEndian.PutUint64(body[16:], r.LastAccessTime)
	binary.LittleEndian.PutUint64(body[24:], r.LastWriteTime)
	binary.LittleEndian.PutUint64(body[32:], r.ChangeTime)
	binary.LittleEndian.PutUint64(body[40:], r.AllocationSize)
	binary.LittleEndian.PutUint64(body[48:], r.EndOfFile)
	binary.LittleEndian.PutUint32(body[56:], r.FileAttributes)
	// Reserved2 at 60
	copy(body[64:80], r.FileID[:])
	if len(r.CreateContexts) == 0 {
		binary.LittleEndian.PutUint32(body[80:], 0)
		binary.LittleEndian.PutUint32(body[84:], 0)
		return append(body, 0) // 1-byte buffer placeholder
	}
	// Pad body to 8 so CreateContexts starts on an 8-byte boundary.
	for len(body)%8 != 0 {
		body = append(body, 0)
	}
	ctxOffset := uint32(len(body) + headerSize)
	binary.LittleEndian.PutUint32(body[80:], ctxOffset)
	binary.LittleEndian.PutUint32(body[84:], uint32(len(r.CreateContexts)))
	body = append(body, r.CreateContexts...)
	return body
}

// CreateContext is a parsed/serializable SMB2 create context entry.
type CreateContext struct {
	Name []byte // up to 8 bytes
	Data []byte
}

// IterateCreateContexts walks raw the linked-list format and yields each entry
// to fn. Stops on the first error or when fn returns false.
func IterateCreateContexts(raw []byte, fn func(CreateContext) bool) {
	off := 0
	for off < len(raw) {
		if len(raw)-off < 16 {
			return
		}
		next := uint64(binary.LittleEndian.Uint32(raw[off:]))
		nameOff := uint64(binary.LittleEndian.Uint16(raw[off+4:]))
		nameLen := uint64(binary.LittleEndian.Uint16(raw[off+6:]))
		dataOff := uint64(binary.LittleEndian.Uint16(raw[off+10:]))
		dataLen := uint64(binary.LittleEndian.Uint32(raw[off+12:]))
		var name, data []byte
		if nameLen > 0 {
			if s, e, ok := bounds.Span(uint64(off)+nameOff, nameLen, len(raw)); ok {
				name = raw[s:e]
			}
		}
		if dataLen > 0 {
			if s, e, ok := bounds.Span(uint64(off)+dataOff, dataLen, len(raw)); ok {
				data = raw[s:e]
			}
		}
		if !fn(CreateContext{Name: name, Data: data}) {
			return
		}
		if next == 0 {
			return
		}
		// 下一项超出缓冲区时结束遍历
		nextOff, _, ok := bounds.Span(uint64(off)+next, 0, len(raw))
		if !ok {
			return
		}
		off = nextOff
	}
}

// EncodeCreateContexts serializes a list of contexts into the wire format.
func EncodeCreateContexts(ctxs []CreateContext) []byte {
	if len(ctxs) == 0 {
		return nil
	}
	var out []byte
	for i, c := range ctxs {
		// Each entry: Next(4) Reserved(2)? actually:
		//   Next(4) NameOffset(2) NameLen(2) Reserved(2) DataOffset(2) DataLen(4)
		// followed by name, padding to 8, data, padding to 8 (except last data).
		entryStart := len(out)
		hdr := make([]byte, 16)
		nameOff := uint16(16)
		dataOff := uint16(0)
		// Header without Next yet.
		binary.LittleEndian.PutUint16(hdr[4:], nameOff)
		binary.LittleEndian.PutUint16(hdr[6:], uint16(len(c.Name)))
		// Reserved at 8..9 stays zero.
		binary.LittleEndian.PutUint32(hdr[12:], uint32(len(c.Data)))
		out = append(out, hdr...)
		out = append(out, c.Name...)
		// Pad name region to 8.
		for (len(out)-entryStart)%8 != 0 {
			out = append(out, 0)
		}
		dataOff = uint16(len(out) - entryStart)
		binary.LittleEndian.PutUint16(out[entryStart+10:], dataOff)
		out = append(out, c.Data...)
		// Pad to 8 between entries (not after the last).
		if i != len(ctxs)-1 {
			for (len(out)-entryStart)%8 != 0 {
				out = append(out, 0)
			}
			binary.LittleEndian.PutUint32(out[entryStart:], uint32(len(out)-entryStart))
		}
	}
	return out
}
