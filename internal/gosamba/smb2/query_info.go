// Gately 修改：报文中的偏移与长度改用 bounds 包在 uint64 中判断边界，32 位平台上不会因 int 转换为负数而越界。

package smb2

import (
	"encoding/binary"
	"fmt"

	"github.com/wangwanjie/gately/internal/gosamba/bounds"
)

// InfoType for QUERY_INFO.
const (
	InfoTypeFile       uint8 = 0x01
	InfoTypeFilesystem uint8 = 0x02
	InfoTypeSecurity   uint8 = 0x03
	InfoTypeQuota      uint8 = 0x04
)

// File information classes (subset).
const (
	FileBasicInformation         uint8 = 0x04
	FileStandardInformation      uint8 = 0x05
	FileInternalInformation      uint8 = 0x06
	FileEaInformation            uint8 = 0x07
	FileAccessInformation        uint8 = 0x08
	FileNameInformation          uint8 = 0x09
	FilePositionInformation      uint8 = 0x0E
	FileFullEaInformation        uint8 = 0x0F
	FileModeInformation          uint8 = 0x10
	FileAlignmentInformation     uint8 = 0x11
	FileAllInformation           uint8 = 0x12
	FileAlternateNameInformation uint8 = 0x15
	FileStreamInformation        uint8 = 0x16
	FileNetworkOpenInformation   uint8 = 0x22
	FileAttributeTagInformation  uint8 = 0x23
)

// FS information classes (subset).
const (
	FileFsVolumeInformation    uint8 = 0x01
	FileFsSizeInformation      uint8 = 0x03
	FileFsDeviceInformation    uint8 = 0x04
	FileFsAttributeInformation uint8 = 0x05
	FileFsFullSizeInformation  uint8 = 0x07
)

type QueryInfoRequest struct {
	InfoType        uint8
	FileInfoClass   uint8
	OutputBufferLen uint32
	AdditionalInfo  uint32
	Flags           uint32
	FileID          [16]byte
	InputBuffer     []byte
}

func DecodeQueryInfoRequest(body []byte) (QueryInfoRequest, error) {
	if len(body) < 40 {
		return QueryInfoRequest{}, fmt.Errorf("%w: QUERY_INFO body min 40", ErrShortBuffer)
	}
	if ss := binary.LittleEndian.Uint16(body[0:]); ss != 41 {
		return QueryInfoRequest{}, fmt.Errorf("%w: QUERY_INFO StructureSize %d", ErrBadStructureSize, ss)
	}
	const headerSize = 64
	r := QueryInfoRequest{
		InfoType:        body[2],
		FileInfoClass:   body[3],
		OutputBufferLen: binary.LittleEndian.Uint32(body[4:]),
		AdditionalInfo:  binary.LittleEndian.Uint32(body[16:]),
		Flags:           binary.LittleEndian.Uint32(body[20:]),
	}
	copy(r.FileID[:], body[24:40])
	inOffAbs := binary.LittleEndian.Uint16(body[8:])
	inLen := binary.LittleEndian.Uint32(body[12:])
	if inLen > 0 {
		if start, end, ok := bounds.RelSpan(uint64(inOffAbs), headerSize, uint64(inLen), len(body)); ok {
			r.InputBuffer = append([]byte(nil), body[start:end]...)
		}
	}
	return r, nil
}

type QueryInfoResponse struct {
	Buffer []byte
}

func EncodeQueryInfoResponse(r QueryInfoResponse) []byte {
	const fixed = 8
	out := make([]byte, fixed+len(r.Buffer))
	binary.LittleEndian.PutUint16(out[0:], 9) // StructureSize
	const headerSize = 64
	if len(r.Buffer) > 0 {
		binary.LittleEndian.PutUint16(out[2:], uint16(headerSize+fixed))
		binary.LittleEndian.PutUint32(out[4:], uint32(len(r.Buffer)))
		copy(out[fixed:], r.Buffer)
	}
	return out
}
