// Gately 新增：AUTHENTICATE 消息字段偏移取 0x80000000 以上的值时，解析应返回错误，不能 panic。
// 这些取值在 32 位平台上经 int 转换后为负数，曾越过边界检查后用负下标切片。

package ntlm

import (
	"encoding/binary"
	"testing"
)

// authMsg 构造一条字段都为空的 AUTHENTICATE 消息，fields 指定某些字段的偏移与长度。
func authMsg(size int, fields map[int][2]uint32) []byte {
	b := make([]byte, size)
	copy(b, Signature[:])
	binary.LittleEndian.PutUint32(b[8:], MessageTypeAuthenticate)
	for off, f := range fields {
		binary.LittleEndian.PutUint16(b[off:], uint16(f[1]))
		binary.LittleEndian.PutUint16(b[off+2:], uint16(f[1]))
		binary.LittleEndian.PutUint32(b[off+4:], f[0])
	}
	return b
}

func TestDecodeAuthenticateHugeOffset(t *testing.T) {
	for _, field := range []int{12, 20, 28, 36, 44, 52} {
		for _, off := range []uint32{0x80000000, 0x80000040, 0xFFFFFFF0, 0xFFFFFFFF} {
			b := authMsg(96, map[int][2]uint32{field: {off, 16}})
			if _, err := DecodeAuthenticate(b); err == nil {
				t.Fatalf("字段 %d 偏移 %#x 应返回错误", field, off)
			}
			// 消息长度按合法字段计算，越界的字段不能让结果超出消息或变成负数
			if n := AuthenticateLen(b); n != len(b) {
				t.Fatalf("字段 %d 偏移 %#x: AuthenticateLen=%d，期望 %d", field, off, n, len(b))
			}
		}
	}
}

func TestSmallestPayloadOffsetHugeOffset(t *testing.T) {
	// 字段 UserName 偏移超大、Workstation 偏移正常：结果应取正常的那个，不能因 int 转换为负数而成为最小值
	b := authMsg(120, map[int][2]uint32{36: {0x80000000, 8}, 44: {104, 8}})
	if got := smallestPayloadOffset(b); got != 104 {
		t.Fatalf("smallestPayloadOffset=%d，期望 104", got)
	}
}

// FuzzDecodeAuthenticate 对 AUTHENTICATE 解析与 MIC 相关的长度计算输入任意消息，任何输入都不能 panic。
func FuzzDecodeAuthenticate(f *testing.F) {
	f.Add(authMsg(96, map[int][2]uint32{36: {0x80000000, 16}}))
	f.Add(authMsg(120, map[int][2]uint32{20: {88, 24}, 36: {112, 8}}))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, b []byte) {
		if a, err := DecodeAuthenticate(b); err == nil {
			_ = MICRequired(a.NtResponse)
		}
		if n := AuthenticateLen(b); n < 0 || n > len(b) {
			t.Fatalf("AuthenticateLen=%d 超出消息长度 %d", n, len(b))
		}
		_, _ = DecodeNegotiate(b)
		_, _ = DecodeAVList(b)
	})
}
