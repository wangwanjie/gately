//go:build !unix

package netpipe

import "syscall"

// WaitReadable 阻塞到套接字有数据可读（或读截止时间到、连接关闭），不取走数据。raw 为 UDP 或 TCP 连接的 SyscallConn。
// Windows 上 RawConn.Read 的回调第一次返回 false 时，标准库会发起一次零字节接收并等它完成（UDP 带 MSG_PEEK；
// TCP 的零字节接收本身不取走数据），IOCP 完成即表示可读，已在缓冲区里的数据会让它立即完成，不会漏掉；
// 第二次回调返回 true 即结束等待。接收出错（如 UDP 目标端口不可达时 Windows 报告的 WSAECONNRESET）原样返回，
// 与直接 Read 得到的错误一致。其他非 Unix 平台的 RawConn.Read 同样先调用回调、返回 false 时等待可读。
func WaitReadable(raw syscall.RawConn) error {
	waited := false
	return raw.Read(func(uintptr) bool {
		if waited {
			return true
		}
		waited = true
		return false
	})
}
