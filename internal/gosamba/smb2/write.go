// Gately 修改：报文中的偏移与长度改用 bounds 包在 uint64 中判断边界，32 位平台上不会因 int 转换为负数而越界。

package smb2

import (
	"encoding/binary"
	"fmt"

	"github.com/wangwanjie/gately/internal/gosamba/bounds"
)

type WriteRequest struct {
	Length uint32
	Offset uint64
	FileID [16]byte
	Flags  uint32
	Data   []byte
}

func DecodeWriteRequest(body []byte) (WriteRequest, error) {
	if len(body) < 48 {
		return WriteRequest{}, fmt.Errorf("%w: WRITE body min 48", ErrShortBuffer)
	}
	if ss := binary.LittleEndian.Uint16(body[0:]); ss != 49 {
		return WriteRequest{}, fmt.Errorf("%w: WRITE StructureSize %d", ErrBadStructureSize, ss)
	}
	const headerSize = 64
	dataOffAbs := binary.LittleEndian.Uint16(body[2:])
	r := WriteRequest{
		Length: binary.LittleEndian.Uint32(body[4:]),
		Offset: binary.LittleEndian.Uint64(body[8:]),
		Flags:  binary.LittleEndian.Uint32(body[44:]),
	}
	copy(r.FileID[:], body[16:32])
	if r.Length > 0 {
		start, end, ok := bounds.RelSpan(uint64(dataOffAbs), headerSize, uint64(r.Length), len(body))
		if !ok {
			return WriteRequest{}, fmt.Errorf("%w: WRITE Buffer out of bounds", ErrShortBuffer)
		}
		r.Data = body[start:end]
	}
	return r, nil
}

type WriteResponse struct {
	Count uint32
}

func EncodeWriteResponse(r WriteResponse) []byte {
	out := make([]byte, 16)
	binary.LittleEndian.PutUint16(out[0:], 17)
	binary.LittleEndian.PutUint32(out[4:], r.Count)
	return out
}
