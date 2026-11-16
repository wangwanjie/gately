package netpipe

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

// connPair 返回一对相连的 TCP 连接。
func connPair(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	acc := make(chan net.Conn, 1)
	go func() {
		c, _ := ln.Accept()
		acc <- c
	}()
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	s := <-acc
	t.Cleanup(func() {
		c.Close()
		s.Close()
	})
	return c, s
}

// 双方都不发数据时按时超时；任一方发了数据（含发数据前就已到达）立即返回且不取走数据；ctx 取消立即返回。
func TestWaitFirstData(t *testing.T) {
	// 空连接：client 侧的 a 与 target 侧的 b 都没有数据
	a, aPeer := connPair(t)
	b, bPeer := connPair(t)
	start := time.Now()
	if err := WaitFirstData(context.Background(), a, b, 200*time.Millisecond); !errors.Is(err, ErrNoData) {
		t.Fatalf("空连接应返回 ErrNoData: %v", err)
	}
	if d := time.Since(start); d < 150*time.Millisecond || d > 2*time.Second {
		t.Fatalf("等待时长不对: %v", d)
	}

	// 服务端先发言：b 的对端写入，等待在 b 端返回，数据仍可读
	go func() {
		time.Sleep(50 * time.Millisecond)
		_, _ = bPeer.Write([]byte("SSH-2.0-test\r\n"))
	}()
	start = time.Now()
	if err := WaitFirstData(context.Background(), a, b, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("有数据时应立即返回: %v", d)
	}
	buf := make([]byte, 64)
	_ = b.SetReadDeadline(time.Now().Add(time.Second))
	if n, err := b.Read(buf); err != nil || string(buf[:n]) != "SSH-2.0-test\r\n" {
		t.Fatalf("等待不应取走数据: %q %v", buf[:n], err)
	}

	// 数据在等待开始前就已到达也不能漏掉（边沿触发的轮询器不会再通知）
	if _, err := aPeer.Write([]byte("GET / HTTP/1.1\r\n")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := WaitFirstData(context.Background(), a, b, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	_ = a.SetReadDeadline(time.Now().Add(time.Second))
	if n, err := io.ReadFull(a, buf[:16]); err != nil || string(buf[:n]) != "GET / HTTP/1.1\r\n" {
		t.Fatalf("等待不应取走数据: %q %v", buf[:n], err)
	}
	// 等待结束后截止时间已清除
	_ = a.SetReadDeadline(time.Time{})

	// 对端关闭也算有动静，交给 Join 处理
	c, cPeer := connPair(t)
	d, _ := connPair(t)
	cPeer.Close()
	if err := WaitFirstData(context.Background(), c, d, 2*time.Second); err != nil {
		t.Fatalf("对端关闭应立即返回: %v", err)
	}

	// ctx 取消立即返回
	e, _ := connPair(t)
	f, _ := connPair(t)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	start = time.Now()
	if err := WaitFirstData(ctx, e, f, 10*time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("ctx 取消应返回其错误: %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("ctx 取消后应立即返回")
	}
}

// UDP 套接字的等待不取走数据报。
func TestWaitReadableUDP(t *testing.T) {
	srv, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	c, err := net.Dial("udp4", srv.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	raw, _ := rawConn(c)
	_ = c.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if err := WaitReadable(raw); !isTimeout(err) {
		t.Fatalf("没有数据报时应超时: %v", err)
	}
	if _, err := c.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 16)
	_, from, err := srv.ReadFrom(buf)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = srv.WriteTo([]byte("pong"), from)
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	if err := WaitReadable(raw); err != nil {
		t.Fatal(err)
	}
	if n, err := c.Read(buf); err != nil || string(buf[:n]) != "pong" {
		t.Fatalf("数据报应仍可读: %q %v", buf[:n], err)
	}
}
