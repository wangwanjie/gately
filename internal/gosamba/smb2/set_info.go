// Gately 修改：报文中的偏移与长度改用 bounds 包在 uint64 中判断边界，32 位平台上不会因 int 转换为负数而越界。

package smb2

import (
	"encoding/binary"
	"fmt"

	"github.com/wangwanjie/gately/internal/gosamba/bounds"
)

// File information classes used in SET_INFO requests.
const (
	FileBasicInformationSet        uint8 = 0x04
	FileRenameInformation          uint8 = 0x0A
	FileDispositionInformation     uint8 = 0x0D
	FileAllocationInformation      uint8 = 0x13
	FileEndOfFileInformation       uint8 = 0x14
	FileValidDataLengthInformation uint8 = 0x27
)

type SetInfoRequest struct {
	InfoType       uint8
	FileInfoClass  uint8
	BufferLength   uint32
	AdditionalInfo uint32
	FileID         [16]byte
	Buffer         []byte
}

func DecodeSetInfoRequest(body []byte) (SetInfoRequest, error) {
	if len(body) < 32 {
		return SetInfoRequest{}, fmt.Errorf("%w: SET_INFO body min 32", ErrShortBuffer)
	}
	if ss := binary.LittleEndian.Uint16(body[0:]); ss != 33 {
		return SetInfoRequest{}, fmt.Errorf("%w: SET_INFO StructureSize %d", ErrBadStructureSize, ss)
	}
	const headerSize = 64
	r := SetInfoRequest{
		InfoType:       body[2],
		FileInfoClass:  body[3],
		BufferLength:   binary.LittleEndian.Uint32(body[4:]),
		AdditionalInfo: binary.LittleEndian.Uint32(body[12:]),
	}
	bufOffAbs := binary.LittleEndian.Uint16(body[8:])
	copy(r.FileID[:], body[16:32])
	if r.BufferLength > 0 {
		start, end, ok := bounds.RelSpan(uint64(bufOffAbs), headerSize, uint64(r.BufferLength), len(body))
		if !ok {
			return SetInfoRequest{}, fmt.Errorf("%w: SET_INFO buffer out of bounds", ErrShortBuffer)
		}
		r.Buffer = append([]byte(nil), body[start:end]...)
	}
	return r, nil
}

func EncodeSetInfoResponse() []byte {
	out := make([]byte, 2)
	binary.LittleEndian.PutUint16(out[0:], 2)
	return out
}
