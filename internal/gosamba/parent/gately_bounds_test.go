// Gately 新增：客户端给出的偏移与长度取 0x80000000 以上的值时，连接处理应返回错误或断开连接，不能 panic。
// 这些取值在 32 位平台上经 int 转换后为负数，曾越过边界检查后用负下标切片，整个进程随之退出。

//go:build linux || darwin

package parent

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/wangwanjie/gately/internal/gosamba/smb2"
	"github.com/wangwanjie/gately/internal/gosamba/transport"
)

// serveFrames 把 frames 依次作为客户端报文交给 ServeConn，返回服务端写出的内容与连接是否被关闭。
func serveFrames(t *testing.T, frames ...[]byte) (*bytes.Buffer, bool) {
	t.Helper()
	in := &bytes.Buffer{}
	for _, f := range frames {
		if err := transport.WriteFrame(in, f); err != nil {
			t.Fatal(err)
		}
	}
	out := &bytes.Buffer{}
	c := &fakeConn{rd: in, wr: out}
	ServeConn(context.Background(), c, slog.New(slog.NewTextHandler(io.Discard, nil)), transport.MaxFrameSize, ConnOptions{RequireSigning: true})
	c.closedMu.Lock()
	defer c.closedMu.Unlock()
	return out, c.closed
}

func TestServeConnHugeNegotiateContextOffset(t *testing.T) {
	for _, off := range []uint32{0x80000040, 0xFFFFFFC0} {
		frame := buildClientNegotiate311()
		// NegotiateContextOffset 位于报文体第 28 字节
		binary.LittleEndian.PutUint32(frame[smb2.HeaderSize+28:], off)
		out, closed := serveFrames(t, frame)
		if !closed {
			t.Fatalf("NegotiateContextOffset=%#x: 连接应被关闭", off)
		}
		if out.Len() != 0 {
			t.Fatalf("NegotiateContextOffset=%#x: 协商失败时不应回复，实际写出 %d 字节", off, out.Len())
		}
	}
}

func TestServeConnHugeNextCommand(t *testing.T) {
	for _, next := range []uint32{0x80000000, 0xFFFFFFC0} {
		hdr := make([]byte, smb2.HeaderSize)
		_ = smb2.EncodeHeader(hdr, smb2.Header{CreditCharge: 1, Command: smb2.CommandEcho, MessageID: 1, NextCommand: next})
		echo := append(hdr, 4, 0, 0, 0)
		_, closed := serveFrames(t, buildClientNegotiate311(), echo)
		if !closed {
			t.Fatalf("NextCommand=%#x: 连接应被关闭", next)
		}
		if !frameNeedsSerialExecution(echo) {
			t.Fatalf("NextCommand=%#x: 无法解析的报文链应按串行处理", next)
		}
	}
}

func TestParseFullEaListHugeNext(t *testing.T) {
	buf := make([]byte, 16)
	binary.LittleEndian.PutUint32(buf[0:], 0x80000000) // NextEntryOffset
	buf[5] = 2                                         // EaNameLength
	copy(buf[8:], "ab\x00")
	got := parseFullEaList(buf)
	if len(got) != 1 || got[0].Name != "ab" {
		t.Fatalf("应解析出第一项并在越界的下一项处停止，得到 %+v", got)
	}
}

func TestRenameHugeFileNameLength(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, sess, tree := newRelatedChainDispatcher(t, dir)
	var buf bytes.Buffer
	d.handleCreate(&buf, smb2.Header{Command: smb2.CommandCreate, TreeID: tree.ID, SessionID: sess.ID},
		buildCreateBody("a.txt", smb2.CreateDispositionOpen, 0, 0x12019f, nil), sess)
	fr := buf.Bytes()
	if len(fr) < 4+smb2.HeaderSize+80 {
		t.Fatalf("CREATE 回复过短: %d", len(fr))
	}
	id := createFileID(fr[4+smb2.HeaderSize:])
	for _, n := range []uint32{0x80000000, 0xFFFFFFF0} {
		info := renameInfoBuf("b.txt", false)
		binary.LittleEndian.PutUint32(info[16:], n)
		buf.Reset()
		d.handleSetInfo(&buf, smb2.Header{Command: smb2.CommandSetInfo, TreeID: tree.ID, SessionID: sess.ID},
			buildSetInfoBody(byte(smb2.InfoTypeFile), byte(smb2.FileRenameInformation), id, info), sess)
		resp := buf.Bytes()
		if len(resp) < 4+smb2.HeaderSize {
			t.Fatalf("FileNameLength=%#x: 没有回复", n)
		}
		h, _ := smb2.DecodeHeader(resp[4 : 4+smb2.HeaderSize])
		if smb2.Status(h.Status) != smb2.StatusInvalidParameter {
			t.Fatalf("FileNameLength=%#x: 状态 %#x，期望 STATUS_INVALID_PARAMETER", n, h.Status)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); err != nil {
		t.Fatalf("原文件不应被改名: %v", err)
	}
}
