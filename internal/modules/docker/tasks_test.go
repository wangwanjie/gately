package docker

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// waitLine 等待任务输出包含 sub 的进度行。
func waitLine(t *testing.T, id, sub string) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if tk, ok := tasks.snapshot(id); ok && strings.Contains(strings.Join(tk.Lines, "\n"), sub) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	tk, _ := tasks.snapshot(id)
	t.Fatalf("等待输出 %q 超时: %+v", sub, tk)
}

func TestTaskCancelPull(t *testing.T) {
	disconnected := make(chan struct{})
	f := &resFake{handlers: map[string]http.HandlerFunc{
		"POST /images/create": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"status":"Pulling from library/nginx","id":"latest"}` + "\n"))
			w.(http.Flusher).Flush()
			// 模拟卡住的拉取：直到客户端断开为止不再输出
			select {
			case <-r.Context().Done():
				close(disconnected)
			case <-time.After(10 * time.Second):
			}
		},
	}}
	m, do := newResEnv(t, f)
	id := taskID(t, do("POST", "/images/pull", "application/json", strings.NewReader(`{"ref":"nginx"}`)))
	waitLine(t, id, "Pulling from")

	rec := do("POST", "/tasks/"+id+"/cancel", "", nil)
	var snap Task
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &snap) != nil || !snap.Cancelling {
		t.Fatalf("取消应成功并标记收尾中: %d %s", rec.Code, rec.Body.String())
	}
	tk := waitTask(t, id)
	if tk.Status != taskCancelled || tk.Cancelling || tk.Error != "" {
		t.Fatalf("取消后状态应为 cancelled: %+v", tk)
	}
	select {
	case <-disconnected:
	case <-time.After(5 * time.Second):
		t.Fatalf("取消后应断开与 Docker 的拉取请求，Docker 才会中止拉取")
	}
	// 运行日志记为“已中断任务”，不记拉取失败
	var logs []string
	for _, e := range m.logs.Since(0) {
		logs = append(logs, e.Message)
	}
	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "已中断任务") || strings.Contains(joined, "失败") {
		t.Fatalf("取消后的日志不符:\n%s", joined)
	}
	// 已结束的任务不能再取消；不存在的任务返回 404
	if rec = do("POST", "/tasks/"+id+"/cancel", "", nil); rec.Code != 409 {
		t.Fatalf("已结束的任务取消应 409: %d", rec.Code)
	}
	if rec = do("POST", "/tasks/nope/cancel", "", nil); rec.Code != 404 {
		t.Fatalf("不存在的任务应 404: %d", rec.Code)
	}
	// 取消后的任务可以被清理
	do("DELETE", "/tasks", "", nil)
	if _, ok := tasks.snapshot(id); ok {
		t.Fatalf("已取消的任务应能清理")
	}
}

func TestTaskCancelRecordsActualResult(t *testing.T) {
	m := &Module{}
	release := make(chan struct{})
	// 不响应取消、照常完成的任务（例如正在重建容器的步骤）按实际结果记为完成
	id := m.startTask("upgrade", "test", func(ctx context.Context, progress func(string)) error {
		<-release
		return nil
	})
	if _, err := tasks.cancelTask(id); err != nil {
		t.Fatal(err)
	}
	// 重复取消不重复追加提示
	if _, err := tasks.cancelTask(id); err != nil {
		t.Fatal(err)
	}
	close(release)
	tk := waitTask(t, id)
	if tk.Status != taskDone {
		t.Fatalf("取消时已完成的任务应记为 done: %+v", tk)
	}
	if n := strings.Count(strings.Join(tk.Lines, "\n"), "已请求中断"); n != 1 {
		t.Fatalf("取消提示应只出现一次，实际 %d 次", n)
	}

	// 取消后以错误结束的任务记为已取消，错误信息不当作失败显示
	id = m.startTask("build", "test2", func(ctx context.Context, progress func(string)) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if _, err := tasks.cancelTask(id); err != nil {
		t.Fatal(err)
	}
	if tk = waitTask(t, id); tk.Status != taskCancelled {
		t.Fatalf("应为 cancelled: %+v", tk)
	}
}

// installSignalCLI 放一个假的 docker：compose up 输出 started 后等待，收到中断信号时（ignore 为假）输出 got-int 退出。
func installSignalCLI(t *testing.T, ignore bool) {
	t.Helper()
	dir := t.TempDir()
	trap := `trap 'echo got-int; kill $p 2>/dev/null; exit 130' INT`
	if ignore {
		trap = `trap '' INT`
	}
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = compose ] && [ \"$2\" = version ]; then echo 2.30.0; exit 0; fi\n" +
		trap + "\necho started\nsleep 5 &\np=$!\nwait $p\necho finished\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestComposeCancelInterrupts(t *testing.T) {
	installSignalCLI(t, false)
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services: {}\n"), 0o644)
	_, do := newResEnv(t, composeFake(dir))
	id := taskID(t, do("POST", "/compose/web/up", "", nil))
	waitLine(t, id, "started")
	start := time.Now()
	if rec := do("POST", "/tasks/"+id+"/cancel", "", nil); rec.Code != 200 {
		t.Fatalf("取消失败: %d", rec.Code)
	}
	tk := waitTask(t, id)
	joined := strings.Join(tk.Lines, "\n")
	if tk.Status != taskCancelled || !strings.Contains(joined, "got-int") || strings.Contains(joined, "finished") {
		t.Fatalf("compose 应收到中断信号后退出: %+v", tk)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("收到中断信号后应很快结束，耗时 %v", time.Since(start))
	}
}

func TestComposeCancelKillsAfterGrace(t *testing.T) {
	installSignalCLI(t, true)
	old := composeStopGrace
	composeStopGrace = 300 * time.Millisecond
	t.Cleanup(func() { composeStopGrace = old })
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services: {}\n"), 0o644)
	_, do := newResEnv(t, composeFake(dir))
	id := taskID(t, do("POST", "/compose/web/up", "", nil))
	waitLine(t, id, "started")
	start := time.Now()
	do("POST", "/tasks/"+id+"/cancel", "", nil)
	tk := waitTask(t, id)
	if tk.Status != taskCancelled || strings.Contains(strings.Join(tk.Lines, "\n"), "finished") {
		t.Fatalf("忽略中断信号的进程应在等待期后被结束: %+v", tk)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("应在等待期后强制结束，耗时 %v", time.Since(start))
	}
}
