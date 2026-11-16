// Package backup 实现定时配置备份：按 cron 表达式打包全部配置，写入本地目录或存储挂载中的挂载点
// （本地目录、WebDAV），支持口令加密与按数量保留，并在成功或失败时发布通知事件。
package backup

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	robfig "github.com/robfig/cron/v3"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/backup"
	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/fsperm"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/storage"
)

// ID 是模块标识。
const ID = "backup"

// 默认值
const (
	defaultSchedule = "0 3 * * *"
	defaultKeep     = 7
	maxKeep         = 365
	localDirName    = "backups"
)

var parser = robfig.NewParser(robfig.Minute | robfig.Hour | robfig.Dom | robfig.Month | robfig.Dow | robfig.Descriptor)

// Storage 由存储挂载模块实现，备份通过它写入挂载点。
type Storage interface {
	MountList() []storage.MountInfo
	PutFile(mount, dir, name string, data []byte) error
	ListNames(mount, dir string) ([]string, error)
	RemoveFile(mount, p string) error
}

// Settings 是定时备份的全局设置。
type Settings struct {
	Enable bool `json:"enable"`
	// Schedule 为 5 段 cron 表达式或 @daily 等描述符
	Schedule string `json:"schedule"`
	// Keep 为每个目的地保留的备份数量
	Keep int `json:"keep"`
	// Password 非空时备份被加密；恢复时需要提供同一口令
	Password string `json:"password"`
	// LocalEnable 决定是否写入配置目录下的 backups 子目录
	LocalEnable bool `json:"localEnable"`
}

// Target 是一个远程备份目的地，写入存储挂载中的某个挂载点。
type Target struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Enable bool   `json:"enable"`
	// Mount 为存储挂载中的挂载点 key
	Mount string `json:"mount"`
	// Dir 为挂载点内的目录，留空为根目录
	Dir string `json:"dir"`
}

type fileConfig struct {
	Settings Settings `json:"settings"`
	Targets  []Target `json:"targets"`
}

// Result 是一个目的地在一次备份中的结果。
type Result struct {
	Target string `json:"target"`
	OK     bool   `json:"ok"`
	File   string `json:"file,omitempty"`
	Error  string `json:"error,omitempty"`
	Pruned int    `json:"pruned,omitempty"`
}

// Status 是最近一次备份的概况。
type Status struct {
	Running bool      `json:"running"`
	LastRun time.Time `json:"lastRun,omitzero"`
	NextRun time.Time `json:"nextRun,omitzero"`
	// Manual 表示最近一次由手动触发
	Manual  bool     `json:"manual"`
	Results []Result `json:"results"`
}

// LocalFile 是本地备份目录中的一个文件。
type LocalFile struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
}

// Validate 校验并规范化设置。
func (s *Settings) Validate() error {
	s.Schedule = strings.TrimSpace(s.Schedule)
	if s.Schedule == "" {
		s.Schedule = defaultSchedule
	}
	if _, err := parser.Parse(s.Schedule); err != nil {
		return fmt.Errorf("计划表达式无效: %v", err)
	}
	if s.Keep == 0 {
		s.Keep = defaultKeep
	}
	if s.Keep < 1 || s.Keep > maxKeep {
		return fmt.Errorf("保留数量需在 1 到 %d 之间", maxKeep)
	}
	return nil
}

// Validate 校验并规范化目的地。
func (t *Target) Validate() error {
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" {
		return errors.New("名称不能为空")
	}
	if strings.TrimSpace(t.Mount) == "" {
		return errors.New("请选择存储挂载点")
	}
	t.Dir = strings.Trim(strings.TrimSpace(t.Dir), "/")
	return nil
}

// Module 是备份模块。
type Module struct {
	mu      sync.Mutex
	store   *config.Store
	logs    *logbuf.Buffer
	log     *logbuf.Logger
	storage Storage
	cfg     fileConfig

	cron    *robfig.Cron
	entry   robfig.EntryID
	running bool
	status  Status
	now     func() time.Time
}

// New 创建模块。st 为存储挂载模块，可为 nil（此时只支持本地目录）。
func New(st Storage) *Module {
	return &Module{storage: st, now: time.Now}
}

// ID 实现 module.Module。
func (m *Module) ID() string { return ID }

// Title 实现 module.Module。
func (m *Module) Title() string { return "配置备份" }

// Init 实现 module.Module。
func (m *Module) Init(env module.Env) error {
	m.store, m.logs = env.Store, env.Logs
	m.log = m.logs.Logger("备份")
	m.cfg.Settings = Settings{Schedule: defaultSchedule, Keep: defaultKeep, LocalEnable: true}
	if _, err := m.store.Load(ID, &m.cfg); err != nil {
		return err
	}
	return m.cfg.Settings.Validate()
}

// Start 实现 module.Module。
func (m *Module) Start(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rescheduleLocked()
	return nil
}

// Stop 实现 module.Module。
func (m *Module) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopCronLocked()
	return nil
}

func (m *Module) stopCronLocked() {
	if m.cron != nil {
		m.cron.Stop()
		m.cron = nil
	}
	m.status.NextRun = time.Time{}
}

// rescheduleLocked 按当前设置重建定时器。
func (m *Module) rescheduleLocked() {
	m.stopCronLocked()
	if !m.cfg.Settings.Enable {
		return
	}
	c := robfig.New(robfig.WithParser(parser))
	id, err := c.AddFunc(m.cfg.Settings.Schedule, func() { m.Run(false) })
	if err != nil {
		m.log.Errorf("计划表达式无效: %v", err)
		return
	}
	m.cron, m.entry = c, id
	c.Start()
	m.status.NextRun = c.Entry(id).Next
}

func (m *Module) saveLocked() error { return m.store.Save(ID, m.cfg) }

// localDir 返回本地备份目录。
func (m *Module) localDir() string {
	return filepath.Join(m.store.Dir(), localDirName)
}

// Run 执行一次备份。已有备份在运行时直接返回。manual 表示由用户触发。
func (m *Module) Run(manual bool) Status {
	m.mu.Lock()
	if m.running {
		st := m.status
		m.mu.Unlock()
		return st
	}
	m.running = true
	st := m.cfg.Settings
	targets := make([]Target, 0, len(m.cfg.Targets))
	for _, t := range m.cfg.Targets {
		if t.Enable {
			targets = append(targets, t)
		}
	}
	m.mu.Unlock()

	now := m.now()
	var results []Result
	data, err := backup.Build(m.store.Dir(), st.Password, now)
	name := backup.Name(now, st.Password != "")
	if err != nil {
		results = append(results, Result{Target: "打包", Error: err.Error()})
	} else {
		if st.LocalEnable {
			results = append(results, m.writeLocal(name, data, st.Keep))
		}
		for _, t := range targets {
			results = append(results, m.writeTarget(t, name, data, st.Keep))
		}
		if len(results) == 0 {
			results = append(results, Result{Target: "无目的地", Error: "未启用本地备份，也没有启用的远程目的地"})
		}
	}

	failed := 0
	for _, r := range results {
		if !r.OK {
			failed++
		}
	}
	m.mu.Lock()
	m.running = false
	m.status.LastRun, m.status.Manual, m.status.Results = now, manual, results
	if m.cron != nil {
		m.status.NextRun = m.cron.Entry(m.entry).Next
	}
	out := m.status
	m.mu.Unlock()

	m.report(results, failed, name)
	return out
}

// report 记录日志并发布事件。
func (m *Module) report(results []Result, failed int, name string) {
	var lines []string
	for _, r := range results {
		if r.OK {
			lines = append(lines, "✓ "+r.Target)
		} else {
			lines = append(lines, "✗ "+r.Target+"："+r.Error)
		}
	}
	detail := strings.Join(lines, "\n")
	if failed > 0 {
		m.log.Errorf("备份 %s 有 %d 个目的地失败\n%s", name, failed, detail)
		events.Publish(events.Event{Type: events.BackupFailed, Source: "配置备份", Level: events.LevelError,
			Title: "配置备份失败", Message: detail})
		return
	}
	m.log.Infof("备份 %s 完成\n%s", name, detail)
	events.Publish(events.Event{Type: events.BackupDone, Source: "配置备份", Level: events.LevelInfo,
		Title: "配置备份完成", Message: name + "\n" + detail})
}

func (m *Module) writeLocal(name string, data []byte, keep int) Result {
	res := Result{Target: "本地目录 (" + localDirName + ")", File: name}
	dir := m.localDir()
	if err := fsperm.MkdirAll(dir, 0o700); err != nil {
		res.Error = err.Error()
		return res
	}
	if err := fsperm.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		res.Error = err.Error()
		return res
	}
	res.OK = true
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	for _, old := range backup.Pick(names, keep) {
		if os.Remove(filepath.Join(dir, old)) == nil {
			res.Pruned++
		}
	}
	return res
}

func (m *Module) writeTarget(t Target, name string, data []byte, keep int) Result {
	res := Result{Target: t.Name, File: name}
	if m.storage == nil {
		res.Error = "存储挂载不可用"
		return res
	}
	if err := m.storage.PutFile(t.Mount, t.Dir, name, data); err != nil {
		res.Error = err.Error()
		return res
	}
	res.OK = true
	names, err := m.storage.ListNames(t.Mount, t.Dir)
	if err != nil {
		// 写入已成功，清理失败不算备份失败
		m.log.Warnf("%s：列出旧备份失败: %v", t.Name, err)
		return res
	}
	for _, old := range backup.Pick(names, keep) {
		if m.storage.RemoveFile(t.Mount, t.Dir+"/"+old) == nil {
			res.Pruned++
		}
	}
	return res
}

// settingsView 隐藏口令，仅告知是否已设置。
type settingsView struct {
	Settings
	Password    string `json:"password"`
	HasPassword bool   `json:"hasPassword"`
}

// Routes 实现 module.Module。
func (m *Module) Routes(r module.Router) {
	targets := &crud.Collection[Target]{
		Mu:       &m.mu,
		Items:    &m.cfg.Targets,
		Save:     m.saveLocked,
		Key:      func(t *Target) *string { return &t.Key },
		Enabled:  func(t *Target) *bool { return &t.Enable },
		Validate: func(t *Target) error { return t.Validate() },
	}
	targets.Register(r, "/targets")
	r.HandleFunc("GET /settings", m.handleGetSettings)
	r.HandleFunc("PUT /settings", m.handlePutSettings)
	// 查看被脱敏的备份加密口令明文；只允许登录会话调用，API 令牌一律拒绝（见 apitoken.Required）
	r.HandleFunc("GET /secrets", crud.SecretsHandler(&m.mu, func() map[string]string {
		return map[string]string{"password": m.cfg.Settings.Password}
	}))
	r.HandleFunc("POST /run", m.handleRun)
	r.HandleFunc("GET /status", m.handleStatus)
	r.HandleFunc("GET /mounts", m.handleMounts)
	r.HandleFunc("GET /local", m.handleLocalList)
	r.HandleFunc("GET /local/{name}", m.handleLocalDownload)
	module.DocOf(r, "GET /settings", module.Doc{Summary: "备份设置", Resp: &settingsView{}})
	module.DocOf(r, "PUT /settings", module.Doc{Summary: "修改备份设置（口令传占位符表示保持不变）", Req: &Settings{}, Resp: &settingsView{}})
	module.DocOf(r, "GET /secrets", module.Doc{Summary: "查看密钥明文", Resp: map[string]string{}})
	module.DocOf(r, "POST /run", module.Doc{Summary: "立即备份一次", Resp: &Status{}})
	module.DocOf(r, "GET /status", module.Doc{Summary: "最近一次备份结果与下次执行时间", Resp: &Status{}})
	module.DocOf(r, "GET /mounts", module.Doc{Summary: "可作为备份目的地的存储挂载点", Resp: []storage.MountInfo{}})
	module.DocOf(r, "GET /local", module.Doc{Summary: "本地备份文件列表", Resp: []LocalFile{}})
	module.DocOf(r, "GET /local/{name}", module.Doc{Summary: "下载本地备份文件"})
}

func (m *Module) view() settingsView {
	s := m.cfg.Settings
	return settingsView{Settings: s, Password: crud.Mask(s.Password), HasPassword: s.Password != ""}
}

func (m *Module) handleGetSettings(w http.ResponseWriter, _ *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	httpx.OK(w, m.view())
}

func (m *Module) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var in Settings
	if err := httpx.ReadJSON(r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	crud.KeepSecret(m.cfg.Settings.Password, &in.Password)
	// 加密口令只允许登录会话修改：令牌能改口令（包括清空）的话，之后生成的备份就能被它解开，
	// 而备份里有会话签名密钥等全部密钥。其余设置（计划、保留数量、开关）令牌仍可修改
	if auth.IsToken(r.Context()) && in.Password != m.cfg.Settings.Password {
		httpx.Error(w, http.StatusForbidden, "备份加密口令只能在登录会话中修改，API 令牌不能修改")
		return
	}
	if err := in.Validate(); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	old := m.cfg.Settings
	m.cfg.Settings = in
	if err := m.saveLocked(); err != nil {
		m.cfg.Settings = old
		httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
		return
	}
	m.rescheduleLocked()
	httpx.OK(w, m.view())
}

func (m *Module) handleRun(w http.ResponseWriter, _ *http.Request) {
	httpx.OK(w, m.Run(true))
}

func (m *Module) handleStatus(w http.ResponseWriter, _ *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.status
	st.Running = m.running
	if m.cron != nil {
		st.NextRun = m.cron.Entry(m.entry).Next
	}
	httpx.OK(w, st)
}

func (m *Module) handleMounts(w http.ResponseWriter, _ *http.Request) {
	if m.storage == nil {
		httpx.OK(w, []storage.MountInfo{})
		return
	}
	httpx.OK(w, m.storage.MountList())
}

func (m *Module) handleLocalList(w http.ResponseWriter, _ *http.Request) {
	entries, _ := os.ReadDir(m.localDir())
	out := []LocalFile{}
	for _, e := range entries {
		if e.IsDir() || !backup.IsBackupName(e.Name()) {
			continue
		}
		if info, err := e.Info(); err == nil {
			out = append(out, LocalFile{Name: e.Name(), Size: info.Size(), ModTime: info.ModTime()})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	httpx.OK(w, out)
}

func (m *Module) handleLocalDownload(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	// 仅允许下载本模块生成的备份文件，且文件名不得含路径
	if name != filepath.Base(name) || !backup.IsBackupName(name) {
		httpx.Error(w, http.StatusBadRequest, "文件名无效")
		return
	}
	data, err := os.ReadFile(filepath.Join(m.localDir(), name))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "备份不存在")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}
