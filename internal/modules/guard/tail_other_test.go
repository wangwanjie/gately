//go:build !linux

package guard

import "testing"

// 非 Linux 平台只走轮询
func TestTailWatcherPollOnNonLinux(t *testing.T) {
	if _, ok := newTailWatcher("/tmp/x.log").(pollWatcher); !ok {
		t.Fatal("非 Linux 平台应使用轮询")
	}
}
