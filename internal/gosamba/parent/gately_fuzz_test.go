// Gately 新增：认证前连接处理的模糊测试。

//go:build linux || darwin

package parent

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/wangwanjie/gately/internal/gosamba/config"
	"github.com/wangwanjie/gately/internal/gosamba/smb2"
	"github.com/wangwanjie/gately/internal/gosamba/transport"
	"github.com/wangwanjie/gately/internal/gosamba/userdb"
)

// fuzzSessionSetup 构造携带 sec 安全缓冲区的 SESSION_SETUP 帧。
func fuzzSessionSetup(sec []byte, sessID uint64) []byte {
	hdr := make([]byte, smb2.HeaderSize)
	_ = smb2.EncodeHeader(hdr, smb2.Header{CreditCharge: 1, Command: smb2.CommandSessionSetup, MessageID: 1, SessionID: sessID})
	body := make([]byte, 24)
	binary.LittleEndian.PutUint16(body[0:], 25)
	binary.LittleEndian.PutUint16(body[12:], smb2.HeaderSize+24)
	binary.LittleEndian.PutUint16(body[14:], uint16(len(sec)))
	return append(append(hdr, body...), sec...)
}

// FuzzServeConnPreAuth 把任意字节作为客户端在 NEGOTIATE 之后发送的内容交给 ServeConn（含 NBSS 分帧），
// 也把任意字节当作 NEGOTIATE 报文体。任何输入都不能引发 panic（panic 会被连接级 recover 捕获，这里视为失败）。
func FuzzServeConnPreAuth(f *testing.F) {
	frame := func(payload []byte) []byte {
		var b bytes.Buffer
		_ = transport.WriteFrame(&b, payload)
		return b.Bytes()
	}
	type1 := append([]byte("NTLMSSP\x00\x01\x00\x00\x00"), make([]byte, 28)...)
	type3 := append([]byte("NTLMSSP\x00\x03\x00\x00\x00"), make([]byte, 80)...)
	binary.LittleEndian.PutUint32(type3[40:], 0x80000000)
	f.Add(frame(fuzzSessionSetup(type1, 0)), false)
	f.Add(frame(fuzzSessionSetup(smb2.WrapNTLMResp(smb2.SPNEGOAcceptIncomplete, type3), 1)), false)
	f.Add(append(frame(fuzzSessionSetup(type1, 0)), frame(fuzzSessionSetup(type3, 1))...), false)
	neg := buildClientNegotiate311()
	binary.LittleEndian.PutUint32(neg[smb2.HeaderSize+28:], 0x80000040)
	f.Add(neg, true)
	f.Add(buildClientNegotiate311(), true)

	users := []config.UserConfig{{Name: "alice", NTHash: userdb.NTHash("pw"), AllowShares: []string{"*"}, Shares: []config.ShareConfig{}}}
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	f.Fuzz(func(t *testing.T, data []byte, asNegotiate bool) {
		var panicked any
		connPanicHook = func(where string, v any) { panicked = fmt.Sprintf("%s: %v", where, v) }
		defer func() { connPanicHook = nil }()
		in := &bytes.Buffer{}
		if asNegotiate {
			// data 作为第一个帧的内容（NEGOTIATE 或其他任意报文）
			if err := transport.WriteFrame(in, data); err != nil {
				return
			}
		} else {
			_ = transport.WriteFrame(in, buildClientNegotiate311())
			in.Write(data)
		}
		c := &fakeConn{rd: in, wr: io.Discard}
		ServeConn(context.Background(), c, lg, 1<<20, ConnOptions{RequireSigning: true, MaxIOSize: 1 << 20, Users: users})
		if panicked != nil {
			t.Fatalf("连接处理 panic: %v", panicked)
		}
	})
}
