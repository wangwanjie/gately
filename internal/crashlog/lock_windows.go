//go:build windows

package crashlog

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// tryLock 以非阻塞方式对文件加排他锁；锁随文件关闭或进程结束自动释放。
func tryLock(f *os.File) error {
	ol := new(windows.Overlapped)
	err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return ErrLocked
	}
	return err
}
