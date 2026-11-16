//go:build unix

package netpipe

import (
	"errors"
	"syscall"
)

// WaitReadable 阻塞到套接字有数据可读（UDP 有数据报、TCP 有数据或对端已关闭），或读截止时间到、连接关闭、收到套接字错误，
// 不取走数据。raw 为 UDP 或 TCP 连接的 SyscallConn。
// Go 的网络轮询是边沿触发的，等待前必须先确认缓冲区里确实没有数据：用 MSG_PEEK 偷看 1 字节，
// 返回 EAGAIN 才交给轮询器等待，否则会漏掉等待开始前已经到达的数据。
// 偷看拿到的套接字错误（如 UDP 目标端口不可达）会被内核清除，原样返回，与直接 Read 得到的错误一致。
func WaitReadable(raw syscall.RawConn) error {
	var peekErr error
	var b [1]byte
	err := raw.Read(func(fd uintptr) bool {
		for {
			_, _, err := syscall.Recvfrom(int(fd), b[:], syscall.MSG_PEEK)
			switch {
			case err == nil:
				return true
			case errors.Is(err, syscall.EINTR):
				continue
			case errors.Is(err, syscall.EAGAIN), errors.Is(err, syscall.EWOULDBLOCK):
				return false
			default:
				peekErr = err
				return true
			}
		}
	})
	if err != nil {
		return err
	}
	return peekErr
}
