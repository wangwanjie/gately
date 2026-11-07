//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package crashlog

import (
	"errors"
	"os"
	"syscall"
)

// tryLock 以非阻塞方式对文件加排他锁；锁随文件关闭或进程结束（包括崩溃、被强制结束）自动释放。
func tryLock(f *os.File) error {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return ErrLocked
	}
	return err
}
