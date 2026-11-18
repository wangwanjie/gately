//go:build linux

package guard

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// inotifyLatency 是事件驱动下读到新内容的时限，远小于 1 秒的轮询兜底周期
const inotifyLatency = 100 * time.Millisecond

func requireInotify(t *testing.T, p string) {
	t.Helper()
	w := newTailWatcher(p)
	defer w.close()
	if _, ok := w.(*inotifyWatcher); !ok {
		t.Skip("inotify 不可用")
	}
}

// 轮询兜底保持默认的 1 秒（重试 5 秒），追加、rename 轮转、copytruncate、文件后出现都应在 100 ms 内读到
func TestTailFileInotifyLatency(t *testing.T) {
	setTailTiming(t, time.Second, 5*time.Second, time.Second)
	dir := t.TempDir()
	p := filepath.Join(dir, "access.log")
	requireInotify(t, p)
	appendFile(t, p, "history\n")
	h := startTail(t, p)
	time.Sleep(50 * time.Millisecond)
	n := 0
	check := func(what string) {
		t.Helper()
		n++
		if d := h.waitFor(n, 3*time.Second); d > inotifyLatency {
			t.Fatalf("%s 用了 %s 才读到", what, d)
		}
	}
	for i := range 20 {
		appendFile(t, p, "line\n")
		check(fmt.Sprintf("第 %d 次追加", i+1))
	}
	// 半行先缓冲，补全后立即读到
	appendFile(t, p, "half")
	time.Sleep(20 * time.Millisecond)
	appendFile(t, p, " line\n")
	check("补全半行")
	// rename 轮转后新建文件
	if err := os.Rename(p, p+".1"); err != nil {
		t.Fatal(err)
	}
	appendFile(t, p, "after rename\n")
	check("rename 轮转")
	// copytruncate：截断后立即被唤醒并从头读取。截断与随后的写入若在同一瞬间完成且新内容超过原读取位置，
	// 无法从文件大小上分辨（轮询实现同样如此，窗口还更大），这里留出写入方的正常间隔
	if err := os.Truncate(p, 0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	appendFile(t, p, "after truncate\n")
	check("copytruncate")
	// 删除后过一会儿才出现
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	appendFile(t, p, "reappear\n")
	check("文件重新出现")
	h.stop()
	want := slices.Repeat([]string{"line"}, 20)
	want = append(want, "half line", "after rename", "after truncate", "reappear")
	if got := h.get(); !slices.Equal(got, want) {
		t.Fatalf("读取结果 %q，期望 %q", got, want)
	}
}

// 启动时目录里还没有文件：新建后立即读到（不必等 5 秒重试）
func TestTailFileInotifyLateAppear(t *testing.T) {
	setTailTiming(t, time.Second, 5*time.Second, time.Second)
	p := filepath.Join(t.TempDir(), "late.log")
	requireInotify(t, p)
	h := startTail(t, p)
	time.Sleep(50 * time.Millisecond)
	appendFile(t, p, "a\n")
	if d := h.waitFor(1, 3*time.Second); d > inotifyLatency {
		t.Fatalf("新建文件用了 %s 才读到", d)
	}
}

// 高速写入：一次唤醒读到末尾，事件合并，内容不丢不乱
func TestTailFileInotifyBurst(t *testing.T) {
	setTailTiming(t, time.Second, 5*time.Second, time.Second)
	p := filepath.Join(t.TempDir(), "burst.log")
	requireInotify(t, p)
	appendFile(t, p, "")
	h := startTail(t, p)
	time.Sleep(50 * time.Millisecond)
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	const total = 20000
	for range total {
		if _, err := f.WriteString("burst line\n"); err != nil {
			t.Fatal(err)
		}
	}
	h.waitFor(total, 5*time.Second)
}

func countFDs(t *testing.T) int {
	t.Helper()
	ents, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Skip(err)
	}
	return len(ents)
}

// 退出后 inotify 描述符、被跟踪文件（含轮转后的旧文件）都已关闭
func TestTailFileInotifyCloseNoFDLeak(t *testing.T) {
	setTailTiming(t, time.Second, 5*time.Second, 10*time.Second)
	dir := t.TempDir()
	p := filepath.Join(dir, "a.log")
	requireInotify(t, p)
	appendFile(t, p, "x\n")
	before := countFDs(t)
	h := startTail(t, p)
	time.Sleep(50 * time.Millisecond)
	// 制造一个仍在宽限期内的旧文件
	if err := os.Rename(p, p+".1"); err != nil {
		t.Fatal(err)
	}
	appendFile(t, p, "y\n")
	h.waitFor(1, 3*time.Second)
	if n := countFDs(t); n < before+3 {
		t.Fatalf("运行中应持有 inotify、新文件、旧文件 3 个描述符：之前 %d，现在 %d", before, n)
	}
	h.stop()
	if n := countFDs(t); n != before {
		t.Fatalf("描述符泄漏：之前 %d，之后 %d", before, n)
	}
}

// inotify 不可用时退回轮询，行为不变
func TestTailFilePollFallbackLinux(t *testing.T) {
	inotifyDisabled.Store(true)
	t.Cleanup(func() { inotifyDisabled.Store(false) })
	setTailTiming(t, 20*time.Millisecond, 20*time.Millisecond, 50*time.Millisecond)
	p := filepath.Join(t.TempDir(), "a.log")
	if _, ok := newTailWatcher(p).(pollWatcher); !ok {
		t.Fatal("禁用 inotify 后应使用轮询")
	}
	appendFile(t, p, "history\n")
	h := startTail(t, p)
	time.Sleep(50 * time.Millisecond)
	appendFile(t, p, "a\n")
	h.waitFor(1, 3*time.Second)
	if err := os.Rename(p, p+".1"); err != nil {
		t.Fatal(err)
	}
	appendFile(t, p, "b\n")
	h.waitFor(2, 3*time.Second)
	if got := h.get(); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("读取结果 %q", got)
	}
}

// 监视绑定到已打开的文件本身：即使打开后路径已被轮转走，仍监视到这个文件
func TestInotifyWatchOpenedFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.log")
	requireInotify(t, p)
	appendFile(t, p, "x\n")
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := os.Rename(p, p+".1"); err != nil {
		t.Fatal(err)
	}
	w := newTailWatcher(p).(*inotifyWatcher)
	defer w.close()
	h := w.watchFile(f, p)
	if h < 0 {
		t.Fatal("应能通过 /proc/self/fd 监视已改名的文件")
	}
	appendFile(t, p+".1", "y\n")
	select {
	case <-w.wake:
	case <-time.After(time.Second):
		t.Fatal("写入已改名的文件应唤醒")
	}
	w.unwatchFile(h)
	if len(w.refs) != 0 {
		t.Fatalf("取消监视后引用应清空: %v", w.refs)
	}
}
