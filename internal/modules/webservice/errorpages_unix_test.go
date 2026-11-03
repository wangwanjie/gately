//go:build unix

package webservice

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// 命名管道在打开时就会阻塞，页面文件指向它时必须在打开前拒绝，不能让输出错误页的请求挂起。
func TestReadErrorPageFileFIFO(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "pipe.html")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, _, _, err := readErrorPageFile(fifo)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("命名管道应拒绝")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("读取命名管道时阻塞")
	}
}
