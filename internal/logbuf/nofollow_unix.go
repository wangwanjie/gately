//go:build unix

package logbuf

import "syscall"

// oNoFollow 让打开文件时不跟随符号链接：日志目录可以设在外接硬盘等其他用户也能写入的目录，
// 以 root 运行时，其他用户事先放好的同名符号链接不能把日志写进任意文件
const oNoFollow = syscall.O_NOFOLLOW
