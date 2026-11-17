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

// ReadableNow 不阻塞地判断套接字此刻是否可读（有数据、对端已关闭或有待报告的错误），不取走数据。
func ReadableNow(raw syscall.RawConn) bool {
	ready := false
	var b [1]byte
	_ = raw.Read(func(fd uintptr) bool {
		for {
			_, _, err := syscall.Recvfrom(int(fd), b[:], syscall.MSG_PEEK)
			if errors.Is(err, syscall.EINTR) {
				continue
			}
			// EAGAIN 表示暂无数据；其余情况（拿到数据、读到对端关闭、套接字错误）都算有动静
			ready = !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EWOULDBLOCK)
			// 返回 true：只看这一次，不交给轮询器等待
			return true
		}
	})
	return ready
}
