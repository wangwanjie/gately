// Gately 新增：报文偏移与长度取 0x80000000 以上的值时，解析函数应返回错误或忽略该字段，不能 panic。
// 这些取值在 32 位平台上经 int 转换后为负数，曾越过边界检查后用负下标切片。

package smb2

import (
	"encoding/binary"
	"testing"
)

// huge 是 32 位平台上 int 转换后为负数的典型取值。
var huge = []uint32{0x80000000, 0x80000040, 0xFFFFFFC0, 0xFFFFFFFF}

func TestNegotiateHugeContextOffset(t *testing.T) {
	for _, off := range huge {
		body := make([]byte, 36+2+8+8)
		binary.LittleEndian.PutUint16(body[0:], negotiateRequestStructureSize)
		binary.LittleEndian.PutUint16(body[2:], 1) // DialectCount
		binary.LittleEndian.PutUint32(body[28:], off)
		binary.LittleEndian.PutUint16(body[32:], 1) // NegotiateContextCount
		binary.LittleEndian.PutUint16(body[36:], uint16(Dialect311))
		if _, err := DecodeNegotiateRequest(body); err == nil {
			t.Fatalf("NegotiateContextOffset=%#x 应返回错误", off)
		}
	}
}

func TestCreateHugeContexts(t *testing.T) {
	for _, v := range huge {
		for _, swap := range []bool{false, true} {
			body := make([]byte, 56+16)
			binary.LittleEndian.PutUint16(body[0:], 57)
			off, n := v, uint32(16)
			if swap {
				off, n = 64+56, v
			}
			binary.LittleEndian.PutUint32(body[48:], off)
			binary.LittleEndian.PutUint32(body[52:], n)
			if _, err := DecodeCreateRequest(body); err == nil {
				t.Fatalf("CreateContexts 偏移 %#x 长度 %#x 应返回错误", off, n)
			}
		}
	}
}

func TestIterateCreateContextsHugeFields(t *testing.T) {
	for _, v := range huge {
		raw := make([]byte, 32)
		binary.LittleEndian.PutUint32(raw[0:], v)   // Next
		binary.LittleEndian.PutUint16(raw[4:], 16)  // NameOffset
		binary.LittleEndian.PutUint16(raw[6:], 4)   // NameLength
		binary.LittleEndian.PutUint16(raw[10:], 24) // DataOffset
		binary.LittleEndian.PutUint32(raw[12:], v)  // DataLength
		copy(raw[16:], "MxAc")
		n := 0
		IterateCreateContexts(raw, func(c CreateContext) bool {
			n++
			if string(c.Name) != "MxAc" || c.Data != nil {
				t.Fatalf("Next/DataLength=%#x: 名称应正常解析、数据应忽略，得到 %q %v", v, c.Name, c.Data)
			}
			return true
		})
		if n != 1 {
			t.Fatalf("Next=%#x 应只遍历一项，实际 %d", v, n)
		}
	}
}

func TestWriteHugeLength(t *testing.T) {
	for _, v := range huge {
		body := make([]byte, 48+8)
		binary.LittleEndian.PutUint16(body[0:], 49)
		binary.LittleEndian.PutUint16(body[2:], 64+48)
		binary.LittleEndian.PutUint32(body[4:], v)
		if _, err := DecodeWriteRequest(body); err == nil {
			t.Fatalf("WRITE Length=%#x 应返回错误", v)
		}
	}
}

func TestSetInfoHugeLength(t *testing.T) {
	for _, v := range huge {
		body := make([]byte, 32+8)
		binary.LittleEndian.PutUint16(body[0:], 33)
		binary.LittleEndian.PutUint32(body[4:], v)
		binary.LittleEndian.PutUint16(body[8:], 64+32)
		if _, err := DecodeSetInfoRequest(body); err == nil {
			t.Fatalf("SET_INFO BufferLength=%#x 应返回错误", v)
		}
	}
}

func TestIoctlAndQueryInfoHugeInput(t *testing.T) {
	for _, v := range huge {
		for _, swap := range []bool{false, true} {
			off, n := v, uint32(8)
			if swap {
				off, n = 64+56, v
			}
			body := make([]byte, 56+8)
			binary.LittleEndian.PutUint16(body[0:], 57)
			binary.LittleEndian.PutUint32(body[24:], off)
			binary.LittleEndian.PutUint32(body[28:], n)
			r, err := DecodeIoctlRequest(body)
			if err != nil || r.InputBuffer != nil {
				t.Fatalf("IOCTL 输入偏移 %#x 长度 %#x 应忽略输入缓冲区，得到 %v %v", off, n, r.InputBuffer, err)
			}
		}
		body := make([]byte, 40+8)
		binary.LittleEndian.PutUint16(body[0:], 41)
		binary.LittleEndian.PutUint16(body[8:], 64+40)
		binary.LittleEndian.PutUint32(body[12:], v)
		r, err := DecodeQueryInfoRequest(body)
		if err != nil || r.InputBuffer != nil {
			t.Fatalf("QUERY_INFO InputBufferLength=%#x 应忽略输入缓冲区，得到 %v %v", v, r.InputBuffer, err)
		}
	}
}

// FuzzDecodeRequests 对全部请求解析函数输入任意报文体，任何输入都不能 panic。
func FuzzDecodeRequests(f *testing.F) {
	seed := func(size int, set func(b []byte)) {
		b := make([]byte, size)
		set(b)
		f.Add(b)
	}
	seed(64, func(b []byte) {
		binary.LittleEndian.PutUint16(b[0:], negotiateRequestStructureSize)
		binary.LittleEndian.PutUint16(b[2:], 1)
		binary.LittleEndian.PutUint32(b[28:], 0x80000040)
		binary.LittleEndian.PutUint16(b[32:], 1)
		binary.LittleEndian.PutUint16(b[36:], uint16(Dialect311))
	})
	seed(80, func(b []byte) {
		binary.LittleEndian.PutUint16(b[0:], 57)
		binary.LittleEndian.PutUint32(b[48:], 64+56)
		binary.LittleEndian.PutUint32(b[52:], 24)
	})
	seed(64, func(b []byte) {
		binary.LittleEndian.PutUint16(b[0:], 49)
		binary.LittleEndian.PutUint16(b[2:], 64+48)
		binary.LittleEndian.PutUint32(b[4:], 0x80000000)
	})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, body []byte) {
		_, _ = DecodeNegotiateRequest(body)
		_, _ = DecodeSessionSetupRequest(body)
		_, _ = DecodeTreeConnectRequest(body)
		_, _ = DecodeTreeDisconnectRequest(body)
		if r, err := DecodeCreateRequest(body); err == nil {
			IterateCreateContexts(r.CreateContexts, func(CreateContext) bool { return true })
		}
		IterateCreateContexts(body, func(CreateContext) bool { return true })
		_, _ = DecodeCloseRequest(body)
		_, _ = DecodeFlushRequest(body)
		_, _ = DecodeReadRequest(body)
		_, _ = DecodeWriteRequest(body)
		_, _ = DecodeLockRequest(body)
		if r, err := DecodeIoctlRequest(body); err == nil {
			_, _ = DecodeSrvCopyChunkCopy(r.InputBuffer, 256)
		}
		_, _ = DecodeSrvCopyChunkCopy(body, 256)
		_, _ = DecodeQueryDirectoryRequest(body)
		_, _ = DecodeChangeNotifyRequest(body)
		_, _ = DecodeQueryInfoRequest(body)
		_, _ = DecodeSetInfoRequest(body)
		_, _ = DecodeHeader(body)
		if tok, err := UnwrapNTLM(body); err == nil {
			_ = tok
		}
	})
}
