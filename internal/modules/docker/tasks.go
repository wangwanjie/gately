package docker

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/httpx"
)

// Task 是后台长任务（拉取镜像、构建、compose up 等）。接口立即返回任务 ID，界面轮询进度，
// 这样移动端切到后台或网络抖动后重新打开页面仍能看到结果。
type Task struct {
	ID       string    `json:"id"`
	Kind     string    `json:"kind"`   // pull / push / build / compose / upgrade / volume 等
	Title    string    `json:"title"`  // 展示用标题
	Status   string    `json:"status"` // running / done / error / cancelled
	Error    string    `json:"error,omitempty"`
	Lines    []string  `json:"lines"` // 最近的进度行（最多 maxTaskLines 行）
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished,omitempty"`
	// Cancelling 表示已请求取消、任务正在收尾（例如等待 compose 进程退出）
	Cancelling bool `json:"cancelling,omitempty"`

	// cancel 取消任务的 context；cancelled 记录是否由用户取消
	cancel    context.CancelFunc
	cancelled bool
}

// 任务状态
const (
	taskRunning   = "running"
	taskDone      = "done"
	taskError     = "error"
	taskCancelled = "cancelled"
)

const (
	maxTaskLines = 300
	maxTasks     = 50
)

type taskStore struct {
	mu    sync.Mutex
	seq   int
	tasks map[string]*Task
}

var tasks = &taskStore{tasks: map[string]*Task{}}

// startTask 在后台执行 fn 并返回任务 ID。fn 通过 progress 追加进度行；返回错误时任务状态为 error。
func (m *Module) startTask(kind, title string, fn func(ctx context.Context, progress func(line string)) error) string {
	tasks.mu.Lock()
	tasks.seq++
	id := strconv.FormatInt(time.Now().Unix(), 36) + "-" + strconv.Itoa(tasks.seq)
	// 取消任务即取消该 context：拉取、推送、构建等 Docker 请求随之断开，Docker 会中止对应操作；
	// compose 命令先收到中断信号，由它自行收尾。重建容器的关键步骤使用独立超时，不会因取消停在一半
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	t := &Task{ID: id, Kind: kind, Title: title, Status: taskRunning, Lines: []string{}, Started: time.Now(), cancel: cancel}
	tasks.tasks[id] = t
	tasks.prune()
	tasks.mu.Unlock()

	progress := func(line string) {
		tasks.mu.Lock()
		defer tasks.mu.Unlock()
		t.appendLine(line)
	}
	go func() {
		defer cancel()
		err := fn(ctx, progress)
		tasks.mu.Lock()
		defer tasks.mu.Unlock()
		t.Finished = time.Now()
		t.Cancelling = false
		switch {
		case err != nil && t.cancelled:
			// 取消后任务以错误结束（通常是 context canceled），记为已取消；取消时已经完成的任务仍按实际结果记录
			t.Status = taskCancelled
			t.appendLine("任务已中断")
		case err != nil:
			t.Status, t.Error = taskError, err.Error()
		default:
			t.Status = taskDone
		}
	}()
	return id
}

// userCancelled 判断任务 context 是否被用户取消（超时为 DeadlineExceeded，不算取消）；
// 被取消的任务由 startTask 记录“已中断任务”，各任务不再另记失败日志。
func userCancelled(ctx context.Context) bool {
	return errors.Is(ctx.Err(), context.Canceled)
}

// appendLine 追加一行进度，超过上限时丢弃最早的行；调用方需持有锁。
func (t *Task) appendLine(line string) {
	t.Lines = append(t.Lines, line)
	if len(t.Lines) > maxTaskLines {
		t.Lines = t.Lines[len(t.Lines)-maxTaskLines:]
	}
}

// errTaskNotRunning 表示任务已经结束，无法取消
var errTaskNotRunning = errors.New("任务已结束")

// cancelTask 请求取消进行中的任务，返回取消后的任务快照。
func (s *taskStore) cancelTask(id string) (Task, error) {
	s.mu.Lock()
	t, ok := s.tasks[id]
	if !ok {
		s.mu.Unlock()
		return Task{}, errTaskNotFound
	}
	if t.Status != taskRunning {
		s.mu.Unlock()
		return Task{}, errTaskNotRunning
	}
	if !t.cancelled {
		t.cancelled, t.Cancelling = true, true
		t.appendLine("已请求中断，正在停止…")
		t.cancel()
	}
	s.mu.Unlock()
	snap, _ := s.snapshot(id)
	return snap, nil
}

// errTaskNotFound 表示任务不存在（已被清理或 ID 错误）
var errTaskNotFound = errors.New("任务不存在")

// prune 只保留最近 maxTasks 个任务，调用方需持有锁。
func (s *taskStore) prune() {
	if len(s.tasks) <= maxTasks {
		return
	}
	list := make([]*Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		list = append(list, t)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Started.After(list[j].Started) })
	for _, t := range list[maxTasks:] {
		if t.Status != taskRunning {
			delete(s.tasks, t.ID)
		}
	}
}

func (s *taskStore) snapshot(id string) (Task, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return Task{}, false
	}
	c := *t
	c.Lines = append([]string{}, t.Lines...)
	return c, true
}

// registerTaskRoutes 注册任务查询接口。
func (m *Module) registerTaskRoutes(add func(pattern string, h http.HandlerFunc)) {
	add("GET /tasks", func(w http.ResponseWriter, r *http.Request) {
		tasks.mu.Lock()
		out := make([]Task, 0, len(tasks.tasks))
		for _, t := range tasks.tasks {
			c := *t
			c.Lines = nil // 列表不带进度行，详情接口再取
			out = append(out, c)
		}
		tasks.mu.Unlock()
		sort.Slice(out, func(i, j int) bool { return out[i].Started.After(out[j].Started) })
		httpx.OK(w, out)
	})
	add("GET /tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		t, ok := tasks.snapshot(r.PathValue("id"))
		if !ok {
			httpx.Error(w, http.StatusNotFound, "任务不存在")
			return
		}
		httpx.OK(w, t)
	})
	add("POST /tasks/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		t, err := tasks.cancelTask(r.PathValue("id"))
		switch {
		case errors.Is(err, errTaskNotFound):
			httpx.Error(w, http.StatusNotFound, "任务不存在")
			return
		case errors.Is(err, errTaskNotRunning):
			httpx.Error(w, http.StatusConflict, "任务已结束")
			return
		}
		m.logInfo("已中断任务：%s", t.Title)
		httpx.OK(w, t)
	})
	add("DELETE /tasks", func(w http.ResponseWriter, r *http.Request) {
		tasks.mu.Lock()
		for id, t := range tasks.tasks {
			if t.Status != taskRunning {
				delete(tasks.tasks, id)
			}
		}
		tasks.mu.Unlock()
		httpx.OK(w, map[string]bool{"ok": true})
	})
}
