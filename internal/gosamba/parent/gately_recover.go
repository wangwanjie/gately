// Gately 新增：连接处理中的 panic 只断开当前连接，不让整个进程退出。

//go:build linux || darwin

package parent

import (
	"fmt"
	"log/slog"
	"runtime/debug"
)

// connPanicHook 在捕获到连接处理中的 panic 后调用，测试用它确认 panic 确实发生并被捕获；生产环境为 nil。
var connPanicHook func(where string, v any)

// recoverConnPanic 必须直接以 defer 调用：捕获当前 goroutine 中的 panic，记录错误日志（含调用栈），
// 再执行 onPanic（通常是关闭连接）。SMB 服务与网关的其他服务运行在同一进程里，
// 某个连接触发的程序缺陷只应断开这一个连接。
func recoverConnPanic(log *slog.Logger, where string, onPanic func()) {
	v := recover()
	if v == nil {
		return
	}
	if log != nil {
		log.Error("panic while serving connection; connection dropped", "where", where, "panic", fmt.Sprint(v), "stack", string(debug.Stack()))
	}
	if connPanicHook != nil {
		connPanicHook(where, v)
	}
	if onPanic != nil {
		onPanic()
	}
}

// killConn 关闭分发器所属的连接，用于后台协程（CHANGE_NOTIFY、阻塞锁等待）捕获 panic 之后。
// 测试中直接构造的分发器没有写协程，此时什么也不做。
func (d *Dispatcher) killConn() {
	if d.out != nil && d.out.kill != nil {
		d.out.kill()
	}
}
