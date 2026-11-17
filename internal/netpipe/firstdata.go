package netpipe

import (
	"context"
	"errors"
	"net"
	"syscall"
	"time"
)

// ErrNoData 表示连接建立后双方在限定时间内都没有发送任何数据。
var ErrNoData = errors.New("连接建立后双方都没有发送数据")

// WaitFirstData 等到 a、b 任一端可读（收到数据或对端关闭），最长 d；都没有动静时返回 ErrNoData。
// 不取走任何数据，返回后照常用 Join 转发。用于丢弃只建立连接、不发送任何数据的空连接：
// 正常协议总有一方在连接后立即发言（HTTP、TLS 由客户端先发，SSH、FTP、MySQL 由服务端先发）。
// ctx 取消时立即返回 ctx 的错误。连接不支持 SyscallConn 时无法不取数据地等待，直接返回 nil（不限制）。
func WaitFirstData(ctx context.Context, a, b net.Conn, d time.Duration) error {
	ra, okA := rawConn(a)
	rb, okB := rawConn(b)
	if !okA || !okB || d <= 0 {
		return nil
	}
	// 快速路径：任一端已经可读就不必等待。客户端先发言的协议（HTTP、TLS）在连接目标的这一个往返里首包通常已经到了，
	// 这样多数连接不必起两个等待协程、反复设置读截止时间
	if ReadableNow(ra) || ReadableNow(rb) {
		return nil
	}
	deadline := time.Now().Add(d)
	_ = a.SetReadDeadline(deadline)
	_ = b.SetReadDeadline(deadline)
	// wake 让仍在等待的一端立即返回：截止时间设为过去
	wake := func() {
		past := time.Unix(1, 0)
		_ = a.SetReadDeadline(past)
		_ = b.SetReadDeadline(past)
	}
	stop := context.AfterFunc(ctx, wake)
	res := make(chan error, 2)
	go func() {
		res <- WaitReadable(ra)
	}()
	go func() {
		res <- WaitReadable(rb)
	}()
	first := <-res
	if !isTimeout(first) {
		// 一端先有动静（或出错，交给 Join 处理），唤醒另一端的等待
		wake()
	}
	second := <-res
	stop()
	_ = a.SetReadDeadline(time.Time{})
	_ = b.SetReadDeadline(time.Time{})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// 任一端在截止时间之前返回（可读、对端关闭或出错）都视为有动静；两端都是超时才是空连接
	if isTimeout(first) && isTimeout(second) {
		return ErrNoData
	}
	return nil
}

// rawConn 返回连接底层套接字的 RawConn。
func rawConn(c net.Conn) (syscall.RawConn, bool) {
	sc, ok := c.(syscall.Conn)
	if !ok {
		return nil, false
	}
	raw, err := sc.SyscallConn()
	return raw, err == nil
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}
