package docker

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestTaskCancelConcurrent 并发取消、查询同一个任务：只追加一次取消提示，最终为 cancelled，-race 下无数据竞争。
func TestTaskCancelConcurrent(t *testing.T) {
	m := &Module{}
	id := m.startTask("pull", "并发取消", func(ctx context.Context, progress func(string)) error {
		for {
			select {
			case <-ctx.Done():
				progress("收尾")
				return ctx.Err()
			default:
				progress("进行中")
				time.Sleep(time.Millisecond)
			}
		}
	})
	var wg sync.WaitGroup
	var okN, endedN int
	var mu sync.Mutex
	for i := 0; i < 16; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, err := tasks.cancelTask(id)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				okN++
			case errors.Is(err, errTaskNotRunning):
				// 任务已经结束后再取消返回“已结束”
				endedN++
			default:
				t.Errorf("取消返回意外错误: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			_, _ = tasks.snapshot(id)
		}()
	}
	wg.Wait()
	tk := waitTask(t, id)
	if okN == 0 || okN+endedN != 16 {
		t.Fatalf("取消结果不符: 成功 %d，已结束 %d", okN, endedN)
	}
	if tk.Status != taskCancelled || tk.Cancelling || tk.Error != "" {
		t.Fatalf("并发取消后应为 cancelled: %+v", tk)
	}
	all := strings.Join(tk.Lines, "\n")
	if n := strings.Count(all, "已请求中断"); n != 1 {
		t.Fatalf("取消提示应只出现一次，实际 %d 次", n)
	}
	if !strings.HasSuffix(all, "任务已中断") {
		t.Fatalf("最后一行应为“任务已中断”: %q", tk.Lines[len(tk.Lines)-1])
	}
	if len(tk.Lines) > maxTaskLines {
		t.Fatalf("进度行应不超过 %d 行，实际 %d", maxTaskLines, len(tk.Lines))
	}
}

// TestTaskTimeoutIsErrorNotCancelled 任务超时属于失败：记为 error，不当作用户取消。
func TestTaskTimeoutIsErrorNotCancelled(t *testing.T) {
	old := waitTimeout
	waitTimeout = 50 * time.Millisecond
	t.Cleanup(func() { waitTimeout = old })
	m := &Module{}
	var cancelled bool
	id := m.startTask("build", "超时", func(ctx context.Context, progress func(string)) error {
		<-ctx.Done()
		cancelled = userCancelled(ctx)
		return ctx.Err()
	})
	tk := waitTask(t, id)
	if tk.Status != taskError || !strings.Contains(tk.Error, "deadline") {
		t.Fatalf("超时应记为 error: %+v", tk)
	}
	if cancelled {
		t.Fatalf("超时不应判定为用户取消")
	}
	if _, err := tasks.cancelTask(id); !errors.Is(err, errTaskNotRunning) {
		t.Fatalf("已结束的任务取消应返回已结束: %v", err)
	}
}
