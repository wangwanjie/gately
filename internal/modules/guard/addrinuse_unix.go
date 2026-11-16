//go:build unix

package guard

import "syscall"

// errAddrInUse 为端口已被占用的错误
var errAddrInUse error = syscall.EADDRINUSE
