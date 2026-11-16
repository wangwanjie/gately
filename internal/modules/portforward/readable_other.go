//go:build !unix

package portforward

import "syscall"

// waitReadable 阻塞到 UDP 连接有数据报可读（或读截止时间到、连接关闭），不读取数据。
// Windows 上 RawConn.Read 的回调第一次返回 false 时，标准库会发起一次零字节的 MSG_PEEK 接收并等它完成
// （IOCP 完成即表示有数据报，已在缓冲区里的数据报会让它立即完成，不会漏掉），第二次回调返回 true 即结束等待。
// 接收出错（如目标端口不可达时 Windows 报告的 WSAECONNRESET）原样返回，与直接 Read 得到的错误一致。
func waitReadable(raw syscall.RawConn) error {
	waited := false
	return raw.Read(func(uintptr) bool {
		if waited {
			return true
		}
		waited = true
		return false
	})
}
