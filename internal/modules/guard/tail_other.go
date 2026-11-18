//go:build !linux

package guard

// newTailWatcher 在非 Linux 平台上只按超时轮询。
func newTailWatcher(string) tailWatcher {
	return pollWatcher{}
}
