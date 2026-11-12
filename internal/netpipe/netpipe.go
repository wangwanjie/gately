// Package netpipe 提供两个连接之间的双向转发。
package netpipe

import (
	"context"
	"io"
	"net"
	"sync"
)

// copyBufSize 与 io.Copy 的默认缓冲区大小一致。
const copyBufSize = 32 * 1024

// bufPool 复用拷贝缓冲区：不能走内核零拷贝（splice）的连接每个方向要用一块缓冲区，
// 每条连接都新分配 2 x 32 KB 会给高连接率的转发带来明显的 GC 压力。
var bufPool = sync.Pool{New: func() any {
	b := make([]byte, copyBufSize)
	return &b
}}

type readerOnly struct{ io.Reader }
type writerOnly struct{ io.Writer }

// splicable 判断连接能否交给标准库走内核零拷贝（两端都是 TCP 或 Unix 套接字时 Linux 上使用 splice，不经过用户态缓冲区）。
func splicable(c net.Conn) bool {
	switch c.(type) {
	case *net.TCPConn, *net.UnixConn:
		return true
	}
	return false
}

// copyConn 把 src 的数据复制到 dst。两端都是原生套接字时仍用 io.Copy 以保留 splice；
// 其余情况（TLS、带缓冲的包装连接，或不支持 splice 的系统）使用池化缓冲区。
func copyConn(dst, src net.Conn) int64 {
	if splicable(dst) && splicable(src) && spliceSupported {
		n, _ := io.Copy(dst, src)
		return n
	}
	bp := bufPool.Get().(*[]byte)
	// 包一层隐藏 ReadFrom / WriteTo，否则标准库会绕过传入的缓冲区自行分配
	n, _ := io.CopyBuffer(writerOnly{dst}, readerOnly{src}, *bp)
	bufPool.Put(bp)
	return n
}

// Join 在 a 与 b 之间双向复制数据，直到两个方向都结束或 ctx 取消。
// 返回 a→b 与 b→a 的字节数。调用方负责在返回后关闭连接。
func Join(ctx context.Context, a, b net.Conn) (aToB, bToA int64) {
	// ctx 取消时主动关闭两端，使阻塞的读写返回
	stop := context.AfterFunc(ctx, func() {
		a.Close()
		b.Close()
	})
	defer stop()

	done := make(chan struct{}, 2)
	go func() {
		aToB = copyConn(b, a)
		closeWrite(b)
		done <- struct{}{}
	}()
	go func() {
		bToA = copyConn(a, b)
		closeWrite(a)
		done <- struct{}{}
	}()
	<-done
	<-done
	return aToB, bToA
}

// closeWrite 半关闭写方向，让对端读到 EOF，同时保留另一方向的传输。
func closeWrite(c net.Conn) {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
		return
	}
	_ = c.Close()
}
