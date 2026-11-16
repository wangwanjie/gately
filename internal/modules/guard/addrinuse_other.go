//go:build !unix

package guard

import "syscall"

// errAddrInUse 为端口已被占用的错误（Windows 为 WSAEADDRINUSE）
var errAddrInUse error = syscall.Errno(10048)
