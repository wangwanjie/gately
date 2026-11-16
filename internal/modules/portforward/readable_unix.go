//go:build unix

package portforward

import (
	"errors"
	"syscall"
)

// waitReadable 阻塞到 UDP 连接有数据报可读（或读截止时间到、连接关闭、收到 ICMP 错误），不读取数据。
// Go 的网络轮询是边沿触发的，等待前必须先确认缓冲区里确实没有数据：用 MSG_PEEK 偷看 1 字节，
// 返回 EAGAIN 才交给轮询器等待，否则会漏掉等待开始前已经到达的回包。偷看不会取走数据报。
// 偷看拿到的套接字错误（如目标端口不可达）会被内核清除，原样返回，与直接 Read 得到的错误一致。
func waitReadable(raw syscall.RawConn) error {
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
