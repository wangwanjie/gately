package netpipe

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
)

// roundTrip 经 Join 在 client 与 server 之间各转发一次数据，返回两端收到的内容。
func roundTrip(t testing.TB, mk func() (net.Conn, net.Conn), payload []byte) {
	c1, c2 := mk() // c1 为客户端侧，c2 为 Join 的 a 端
	s1, s2 := mk() // s1 为 Join 的 b 端，s2 为服务端侧
	done := make(chan [2]int64, 1)
	go func() {
		in, out := Join(context.Background(), c2, s1)
		done <- [2]int64{in, out}
	}()
	go func() {
		got := make([]byte, len(payload))
		if _, err := io.ReadFull(s2, got); err != nil || !bytes.Equal(got, payload) {
			t.Error("服务端收到的数据不符")
		}
		_, _ = s2.Write(payload)
		_ = closeWriteOrClose(s2)
	}()
	if _, err := c1.Write(payload); err != nil {
		t.Fatal(err)
	}
	back := make([]byte, len(payload))
	if _, err := io.ReadFull(c1, back); err != nil || !bytes.Equal(back, payload) {
		t.Fatal("客户端收到的数据不符")
	}
	_ = closeWriteOrClose(c1)
	n := <-done
	if n[0] != int64(len(payload)) || n[1] != int64(len(payload)) {
		t.Fatalf("字节数不符: %v", n)
	}
	for _, c := range []net.Conn{c1, c2, s1, s2} {
		c.Close()
	}
}

func closeWriteOrClose(c net.Conn) error {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return c.Close()
}

// wrapped 隐藏 ReadFrom / WriteTo / CloseWrite，模拟 TLS 或带缓冲的包装连接。
type wrapped struct{ net.Conn }

func tcpPair(t testing.TB) func() (net.Conn, net.Conn) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	return func() (net.Conn, net.Conn) {
		ch := make(chan net.Conn, 1)
		go func() {
			c, _ := ln.Accept()
			ch <- c
		}()
		c, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		return c, <-ch
	}
}

func TestJoinWrappedAndTCP(t *testing.T) {
	payload := bytes.Repeat([]byte("gately"), 20000)
	roundTrip(t, tcpPair(t), payload)
	roundTrip(t, tcpPair(t), []byte("hello"))
}

// BenchmarkJoinWrapped 衡量不能走 splice 的连接（包装连接）每条连接的转发开销。
func BenchmarkJoinWrapped(b *testing.B) {
	mk := tcpPair(b)
	wrap := func() (net.Conn, net.Conn) {
		x, y := mk()
		return wrapped{x}, wrapped{y}
	}
	payload := []byte("hello gately")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		roundTrip(b, wrap, payload)
	}
}
