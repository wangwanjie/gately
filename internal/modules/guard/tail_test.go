package guard

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"
)

// tailHarness 在后台运行 tailFile 并收集读到的行。
type tailHarness struct {
	t      *testing.T
	mu     sync.Mutex
	lines  []string
	cancel context.CancelFunc
	done   chan struct{}
}

func startTail(t *testing.T, p string) *tailHarness {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	h := &tailHarness{t: t, cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(h.done)
		tailFile(ctx, p, func(l string) {
			h.mu.Lock()
			h.lines = append(h.lines, l)
			h.mu.Unlock()
		}, func(string) {})
	}()
	t.Cleanup(h.stop)
	return h
}

func (h *tailHarness) get() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string{}, h.lines...)
}

// waitFor 等到读到至少 n 行，返回等待的时长；超过 limit 判失败
func (h *tailHarness) waitFor(n int, limit time.Duration) time.Duration {
	h.t.Helper()
	start := time.Now()
	for time.Since(start) < limit {
		if len(h.get()) >= n {
			return time.Since(start)
		}
		time.Sleep(time.Millisecond)
	}
	h.t.Fatalf("%s 内未读到 %d 行: %q", limit, n, h.get())
	return 0
}

func (h *tailHarness) stop() {
	h.cancel()
	<-h.done
}

// setTailTiming 临时调整轮询参数
func setTailTiming(t *testing.T, poll, backoff, grace time.Duration) {
	t.Helper()
	oldPoll, oldBackoff, oldGrace := tailPoll, retryBackoff, rotateGrace
	tailPoll, retryBackoff, rotateGrace = poll, backoff, grace
	t.Cleanup(func() {
		tailPoll, retryBackoff, rotateGrace = oldPoll, oldBackoff, oldGrace
	})
}

// copytruncate：轮转程序复制内容后把原文件截断为空，之后的写入从头读取
func TestTailFileCopyTruncate(t *testing.T) {
	setTailTiming(t, 20*time.Millisecond, 20*time.Millisecond, 50*time.Millisecond)
	p := filepath.Join(t.TempDir(), "access.log")
	appendFile(t, p, "history\n")
	h := startTail(t, p)
	time.Sleep(50 * time.Millisecond)
	appendFile(t, p, "one\ntwo\n")
	h.waitFor(2, 3*time.Second)
	if err := os.Truncate(p, 0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	appendFile(t, p, "three\n")
	h.waitFor(3, 3*time.Second)
	h.stop()
	if got, want := h.get(), []string{"one", "two", "three"}; !slices.Equal(got, want) {
		t.Fatalf("读取结果 %q，期望 %q", got, want)
	}
}

// logrotate create 模式：改名后立即建好新文件，写入方重新打开前仍写旧文件，这部分内容不能丢；新文件从头读取
func TestTailFileRotateCreateGrace(t *testing.T) {
	setTailTiming(t, 20*time.Millisecond, 20*time.Millisecond, 500*time.Millisecond)
	p := filepath.Join(t.TempDir(), "access.log")
	appendFile(t, p, "history\n")
	h := startTail(t, p)
	time.Sleep(50 * time.Millisecond)
	// 模拟写入方：一直持有旧文件的描述符
	wr, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer wr.Close()
	if _, err := wr.WriteString("before\n"); err != nil {
		t.Fatal(err)
	}
	h.waitFor(1, 3*time.Second)
	if err := os.Rename(p, p+".1"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	// 等 tailFile 发现轮转并打开新文件后，写入方才写旧文件（还没收到重新打开的信号）
	time.Sleep(100 * time.Millisecond)
	if _, err := wr.WriteString("late old\n"); err != nil {
		t.Fatal(err)
	}
	h.waitFor(2, 3*time.Second)
	appendFile(t, p, "new\n")
	h.waitFor(3, 3*time.Second)
	h.stop()
	if got, want := h.get(), []string{"before", "late old", "new"}; !slices.Equal(got, want) {
		t.Fatalf("读取结果 %q，期望 %q", got, want)
	}
}

// 删除后重建：旧文件宽限期后关闭，新文件从头读取
func TestTailFileDeleteRecreate(t *testing.T) {
	setTailTiming(t, 20*time.Millisecond, 20*time.Millisecond, 50*time.Millisecond)
	p := filepath.Join(t.TempDir(), "access.log")
	appendFile(t, p, "history\n")
	h := startTail(t, p)
	time.Sleep(50 * time.Millisecond)
	appendFile(t, p, "a\n")
	h.waitFor(1, 3*time.Second)
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	appendFile(t, p, "b\n")
	h.waitFor(2, 3*time.Second)
	h.stop()
	if got, want := h.get(), []string{"a", "b"}; !slices.Equal(got, want) {
		t.Fatalf("读取结果 %q，期望 %q", got, want)
	}
}

// 取消后 tailFile 的所有协程都退出
func TestTailFileCloseNoLeak(t *testing.T) {
	setTailTiming(t, 20*time.Millisecond, 20*time.Millisecond, 50*time.Millisecond)
	dir := t.TempDir()
	before := runtime.NumGoroutine()
	var hs []*tailHarness
	for i := range 8 {
		p := filepath.Join(dir, "f"+string(rune('a'+i))+".log")
		if i%2 == 0 {
			appendFile(t, p, "x\n")
		}
		hs = append(hs, startTail(t, p))
	}
	time.Sleep(50 * time.Millisecond)
	for _, h := range hs {
		h.stop()
	}
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before {
		t.Fatalf("协程未全部退出：之前 %d，之后 %d", before, n)
	}
}
