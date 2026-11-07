//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly || windows)

package crashlog

import "os"

// tryLock 在不支持文件锁的系统上总是成功，同一配置目录同时运行两个实例时无法识别。
func tryLock(f *os.File) error {
	return nil
}
