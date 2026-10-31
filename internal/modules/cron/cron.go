// Package cron 实现计划任务模块：按 cron 表达式定时执行命令或 HTTP 请求，并保留最近的执行记录。
package cron

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	robfig "github.com/robfig/cron/v3"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/script"
	"github.com/wangwanjie/gately/internal/webhook"
)

// ID 是模块标识。
const ID = "cron"

// 任务类型
const (
	TypeCommand = "command"
	TypeHTTP    = "http"
)

// 每个任务保留的执行记录数
const historySize = 20

// parser 支持可选的秒字段（6 段）与 @every、@daily 等描述符。
var parser = robfig.NewParser(robfig.SecondOptional | robfig.Minute | robfig.Hour | robfig.Dom | robfig.Month | robfig.Dow | robfig.Descriptor)

// Task 是一个计划任务。
type Task struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Group 为列表中的分组名称，只影响管理界面的展示，留空为未分组
	Group  string `json:"group"`
	Enable bool   `json:"enable"`
	// Spec 为 cron 表达式，例如 "0 3 * * *"、"*/30 * * * * *"、"@every 10m"
	Spec    string         `json:"spec"`
	Type    string         `json:"type"`
	Command string         `json:"command"`
	HTTP    webhook.Config `json:"http"`
	Remark  string         `json:"remark"`
}

// Run 是一次执行记录。
type Run struct {
	Start    time.Time `json:"start"`
	Duration int64     `json:"durationMs"`
	OK       bool      `json:"ok"`
	Output   string    `json:"output"`
	Manual   bool      `json:"manual"`
}

type fileConfig struct {
	Tasks []Task `json:"tasks"`
}

// Validate 校验任务。
func (t *Task) Validate() error {
	if err := crud.NormalizeGroup(&t.Group); err != nil {
		return err
	}
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" {
		return errors.New("名称不能为空")
	}
	t.Spec = strings.TrimSpace(t.Spec)
	if _, err := parser.Parse(t.Spec); err != nil {
		return fmt.Errorf("cron 表达式无效: %w", err)
	}
	switch t.Type {
	case "", TypeCommand:
		t.Type = TypeCommand
		if strings.TrimSpace(t.Command) == "" {
			return errors.New("命令不能为空")
		}
	case TypeHTTP:
		t.HTTP.Enable = true
		if err := t.HTTP.Validate(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("未知的任务类型 %q", t.Type)
	}
	return nil
}

// Module 是计划任务模块。
type Module struct {
	mu      sync.Mutex
	store   *config.Store
	logs    *logbuf.Buffer
	cfg     fileConfig
	sched   *robfig.Cron
	entries map[string]robfig.EntryID
	history map[string][]Run
	running map[string]bool
	ctx     context.Context
}

// New 创建模块。
func New() *Module {
	return &Module{entries: map[string]robfig.EntryID{}, history: map[string][]Run{}, running: map[string]bool{}}
}

// ID 实现 module.Module。
func (m *Module) ID() string { return ID }

// Title 实现 module.Module。
func (m *Module) Title() string { return "计划任务" }

// Init 实现 module.Module。
func (m *Module) Init(env module.Env) error {
	m.store, m.logs = env.Store, env.Logs
	_, err := m.store.Load(ID, &m.cfg)
	return err
}

// Start 实现 module.Module。
func (m *Module) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ctx = ctx
	m.sched = robfig.New(robfig.WithParser(parser))
	for _, t := range m.cfg.Tasks {
		if t.Enable {
			m.scheduleLocked(t)
		}
	}
	m.sched.Start()
	return nil
}

// Stop 实现 module.Module，等待正在执行的任务结束。
func (m *Module) Stop() error {
	m.mu.Lock()
	s := m.sched
	m.mu.Unlock()
	if s != nil {
		<-s.Stop().Done()
	}
	return nil
}

func (m *Module) saveLocked() error { return m.store.Save(ID, m.cfg) }

func (m *Module) scheduleLocked(t Task) {
	if m.sched == nil {
		return
	}
	key := t.Key
	id, err := m.sched.AddFunc(t.Spec, func() { m.execute(key, false) })
	if err != nil {
		m.logs.Logger(t.Name).Errorf("调度失败: %v", err)
		return
	}
	m.entries[key] = id
}

func (m *Module) unscheduleLocked(key string) {
	if id, ok := m.entries[key]; ok {
		m.sched.Remove(id)
		delete(m.entries, key)
	}
}

func (m *Module) changedLocked(key string) {
	m.unscheduleLocked(key)
	for _, t := range m.cfg.Tasks {
		if t.Key == key && t.Enable {
			m.scheduleLocked(t)
		}
	}
}

// execute 执行一次任务。同一任务上一次尚未结束时跳过本次。
func (m *Module) execute(key string, manual bool) {
	m.mu.Lock()
	var task *Task
	for i := range m.cfg.Tasks {
		if m.cfg.Tasks[i].Key == key {
			t := m.cfg.Tasks[i]
			task = &t
		}
	}
	if task == nil || m.running[key] {
		m.mu.Unlock()
		return
	}
	m.running[key] = true
	ctx := m.ctx
	m.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}

	log := m.logs.Logger(task.Name)
	start := time.Now()
	var out string
	var err error
	switch task.Type {
	case TypeHTTP:
		var res *webhook.Result
		res, err = webhook.Send(ctx, task.HTTP, map[string]string{"time": start.Format("2006-01-02 15:04:05"), "taskName": task.Name})
		if res != nil {
			out = fmt.Sprintf("HTTP %d %s", res.StatusCode, res.Body)
		}
	default:
		var b []byte
		b, err = script.Run(task.Command, map[string]string{"taskName": task.Name})
		out = string(b)
	}
	run := Run{Start: start, Duration: time.Since(start).Milliseconds(), OK: err == nil, Output: script.Summarize([]byte(out), 4000), Manual: manual}
	if err != nil {
		run.Output = strings.TrimSpace(err.Error() + "\n" + run.Output)
		log.Errorf("执行失败（%dms）: %v", run.Duration, err)
	} else {
		log.Infof("执行完成（%dms）", run.Duration)
	}
	m.mu.Lock()
	delete(m.running, key)
	h := append(m.history[key], run)
	if len(h) > historySize {
		h = h[len(h)-historySize:]
	}
	m.history[key] = h
	m.mu.Unlock()
}

// TaskView 是带运行状态的任务。
type TaskView struct {
	Task
	Running bool      `json:"running"`
	NextRun time.Time `json:"nextRun,omitzero"`
	History []Run     `json:"history"`
}

func (m *Module) viewLocked(t *Task) any {
	v := TaskView{Task: *t, Running: m.running[t.Key], History: append([]Run{}, m.history[t.Key]...)}
	if id, ok := m.entries[t.Key]; ok && m.sched != nil {
		v.NextRun = m.sched.Entry(id).Next
	}
	return v
}

// Routes 实现 module.Module。
func (m *Module) Routes(r module.Router) {
	tasks := &crud.Collection[Task]{
		Mu:       &m.mu,
		Items:    &m.cfg.Tasks,
		Save:     m.saveLocked,
		Key:      func(t *Task) *string { return &t.Key },
		Enabled:  func(t *Task) *bool { return &t.Enable },
		Group:    func(t *Task) *string { return &t.Group },
		Validate: func(t *Task) error { return t.Validate() },
		View:     m.viewLocked,
		Changed:  m.changedLocked,
		Removed: func(key string) {
			m.unscheduleLocked(key)
			delete(m.history, key)
		},
	}
	tasks.Register(r, "/tasks")
	r.HandleFunc("POST /tasks/{key}/run", func(w http.ResponseWriter, req *http.Request) {
		key := req.PathValue("key")
		m.mu.Lock()
		busy := m.running[key]
		m.mu.Unlock()
		if busy {
			httpx.Error(w, http.StatusConflict, "任务正在执行")
			return
		}
		go m.execute(key, true)
		httpx.OK(w, map[string]bool{"ok": true})
	})
	r.HandleFunc("POST /parse", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Spec string `json:"spec"`
		}
		if err := httpx.ReadJSON(req, &body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		s, err := parser.Parse(strings.TrimSpace(body.Spec))
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		// 返回接下来 5 次执行时间，帮助用户确认表达式
		var next []time.Time
		t := time.Now()
		for i := 0; i < 5; i++ {
			t = s.Next(t)
			next = append(next, t)
		}
		httpx.OK(w, map[string]any{"next": next})
	})
}
