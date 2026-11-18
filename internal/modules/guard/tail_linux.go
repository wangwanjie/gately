//go:build linux

package guard

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

const (
	// inotifyDirMask 监视日志所在目录：文件新建、移入、移出、删除，以及目录自身被删除或改名
	inotifyDirMask = unix.IN_CREATE | unix.IN_MOVED_TO | unix.IN_MOVED_FROM | unix.IN_DELETE |
		unix.IN_DELETE_SELF | unix.IN_MOVE_SELF | unix.IN_ONLYDIR
	// inotifyFileMask 监视已打开的文件：写入与截断（IN_MODIFY）、链接数变化（IN_ATTRIB，删除时触发）、改名和删除
	inotifyFileMask = unix.IN_MODIFY | unix.IN_ATTRIB | unix.IN_MOVE_SELF | unix.IN_DELETE_SELF
)

// inotifyWatcher 用一个 inotify 实例监视日志文件及其所在目录。后台协程读取事件，只把与被跟踪文件相关的事件
// 转成一次唤醒信号；信号通道容量为 1，tailFile 读取期间到达的事件自然合并。
type inotifyWatcher struct {
	// file 包装 inotify 描述符（非阻塞），由 Go 运行时轮询，Close 能唤醒阻塞中的 Read
	file *os.File
	fd   int
	dir  string
	base string
	// dirWd 为目录监视的 wd，-1 表示尚未监视（例如目录还不存在或监视数达到上限）
	dirWd atomic.Int32
	// dirLost 表示目录已被删除或改名，下次等待前需重新监视
	dirLost atomic.Bool
	// refs 记录文件监视的引用计数：同一个 inode 重复监视会得到同一个 wd
	refs map[int]int
	wake chan struct{}
	done chan struct{}
}

// inotifyDisabled 供测试强制走轮询
var inotifyDisabled atomic.Bool

// newTailWatcher 优先使用 inotify，不可用（例如实例数达到上限）时退回轮询。
func newTailWatcher(path string) tailWatcher {
	if inotifyDisabled.Load() {
		return pollWatcher{}
	}
	fd, err := unix.InotifyInit1(unix.IN_NONBLOCK | unix.IN_CLOEXEC)
	if err != nil {
		return pollWatcher{}
	}
	w := &inotifyWatcher{
		file: os.NewFile(uintptr(fd), "inotify"),
		fd:   fd,
		dir:  filepath.Dir(path),
		base: filepath.Base(path),
		refs: map[int]int{},
		wake: make(chan struct{}, 1),
		done: make(chan struct{}),
	}
	w.dirWd.Store(-1)
	w.ensureDir()
	go w.readLoop()
	return w
}

// ensureDir 在目录尚未监视或监视已失效时（重新）监视目录；失败时保持轮询兜底，下次等待前再试。
func (w *inotifyWatcher) ensureDir() {
	if w.dirLost.Swap(false) {
		if wd := w.dirWd.Swap(-1); wd >= 0 {
			_, _ = unix.InotifyRmWatch(w.fd, uint32(wd))
		}
	}
	if w.dirWd.Load() >= 0 {
		return
	}
	wd, err := unix.InotifyAddWatch(w.fd, w.dir, inotifyDirMask)
	if err == nil {
		w.dirWd.Store(int32(wd))
	}
}

func (w *inotifyWatcher) watchFile(f *os.File, path string) int {
	wd := -1
	// 优先通过 /proc/self/fd 监视已打开的这个文件本身，避免打开后、监视前恰好轮转而监视到另一个文件
	if rc, err := f.SyscallConn(); err == nil {
		_ = rc.Control(func(fd uintptr) {
			if n, err := unix.InotifyAddWatch(w.fd, "/proc/self/fd/"+strconv.Itoa(int(fd)), inotifyFileMask); err == nil {
				wd = n
			}
		})
	}
	if wd < 0 {
		n, err := unix.InotifyAddWatch(w.fd, path, inotifyFileMask)
		if err != nil {
			// 监视数达到上限等情况：这个文件的写入改由轮询兜底
			return -1
		}
		wd = n
	}
	w.refs[wd]++
	return wd
}

func (w *inotifyWatcher) unwatchFile(h int) {
	if h < 0 {
		return
	}
	w.refs[h]--
	if w.refs[h] > 0 {
		return
	}
	delete(w.refs, h)
	// 文件已删除时内核已自动移除监视，忽略错误
	_, _ = unix.InotifyRmWatch(w.fd, uint32(h))
}

func (w *inotifyWatcher) wait(ctx context.Context, d time.Duration) bool {
	w.ensureDir()
	return sleepCtx(ctx, d, w.wake)
}

func (w *inotifyWatcher) close() {
	w.file.Close()
	<-w.done
}

// readLoop 持续读取 inotify 事件，一次读取可能包含多个事件，有相关事件时发出一次唤醒信号。
func (w *inotifyWatcher) readLoop() {
	defer close(w.done)
	buf := make([]byte, 64*(unix.SizeofInotifyEvent+unix.NAME_MAX+1))
	for {
		n, err := w.file.Read(buf)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			// 描述符已关闭时正常退出；其他错误也退出，之后靠轮询兜底
			return
		}
		if w.relevant(buf[:n]) {
			select {
			case w.wake <- struct{}{}:
			default:
			}
		}
	}
}

// relevant 解析一批事件，判断其中是否有与被跟踪文件相关的事件。
func (w *inotifyWatcher) relevant(b []byte) bool {
	hit := false
	dirWd := w.dirWd.Load()
	for len(b) >= unix.SizeofInotifyEvent {
		wd := int32(binary.NativeEndian.Uint32(b[0:4]))
		mask := binary.NativeEndian.Uint32(b[4:8])
		nameLen := int(binary.NativeEndian.Uint32(b[12:16]))
		end := unix.SizeofInotifyEvent + nameLen
		if end > len(b) {
			break
		}
		name := b[unix.SizeofInotifyEvent:end]
		if i := bytes.IndexByte(name, 0); i >= 0 {
			name = name[:i]
		}
		b = b[end:]
		switch {
		case mask&unix.IN_Q_OVERFLOW != 0:
			// 事件队列溢出，可能漏掉了相关事件
			hit = true
		case wd == dirWd:
			if mask&(unix.IN_IGNORED|unix.IN_DELETE_SELF|unix.IN_MOVE_SELF) != 0 {
				// 目录被删除或改名：标记后由等待方重新监视，并唤醒检查文件状态
				w.dirLost.Store(true)
				hit = true
			} else if string(name) == w.base {
				hit = true
			}
		case mask&unix.IN_IGNORED != 0:
			// 取消文件监视（或文件被删除）产生的事件，无需处理
		default:
			// 被跟踪文件（含轮转后宽限期内的旧文件）的事件
			hit = true
		}
	}
	return hit
}
