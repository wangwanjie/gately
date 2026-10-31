// Package filebrowser 托管 FileBrowser Quantum（gtsteffaniak/filebrowser）文件管理器：
// 在界面中维护实例（监听地址、共享目录、登录方式、管理员账号），生成配置文件并守护外部进程。
//
// 原版 filebrowser/filebrowser 已于 2026 年归档、不再修复安全问题，因此托管的是仍在维护的 Quantum 分支。
package filebrowser

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wangwanjie/gately/internal/binstall"
	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/fsperm"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/proc"
)

// ID 是模块标识。
const ID = "filebrowser"

// binName 是可执行文件名（Windows 下查找与安装时自动加 .exe）。
const binName = "filebrowser"

// Settings 是模块级配置。
type Settings struct {
	// BinPath 为空时依次在配置目录 bin 与 PATH 下查找 filebrowser
	BinPath string `json:"binPath"`
}

type fileConfig struct {
	Settings Settings `json:"settings"`
	// Install 为在线安装设置
	Install   binstall.Config `json:"install"`
	Instances []Instance      `json:"instances"`
}

// instRuntime 是实例的运行时信息。
type instRuntime struct {
	sup *proc.Supervisor
	err string
}

// Module 是 File Browser 模块。
type Module struct {
	mu      sync.Mutex
	store   *config.Store
	logs    *logbuf.Buffer
	cfg     fileConfig
	runs    map[string]*instRuntime
	started bool
	// version、quantum 缓存已探测的二进制版本，以及它是否为 FileBrowser Quantum
	version     string
	quantum     bool
	versionPath string
	// client 返回下载程序使用的 HTTP 客户端，由框架按“更新代理”设置注入
	client  func() *http.Client
	install *binstall.Installer
}

// New 创建模块。
func New() *Module {
	return &Module{
		runs:   map[string]*instRuntime{},
		client: func() *http.Client { return http.DefaultClient },
	}
}

// ID 实现 module.Module。
func (m *Module) ID() string { return ID }

// Title 实现 module.Module。
func (m *Module) Title() string { return "File Browser" }

// SetHTTPClient 实现 module.HTTPClientAware。
func (m *Module) SetHTTPClient(fn func() *http.Client) {
	if fn != nil {
		m.client = fn
	}
}

// Init 实现 module.Module。
func (m *Module) Init(env module.Env) error {
	m.store, m.logs = env.Store, env.Logs
	m.install = m.newInstaller()
	_, err := m.store.Load(ID, &m.cfg)
	return err
}

// Start 实现 module.Module。
func (m *Module) Start(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = true
	for i := range m.cfg.Instances {
		m.applyLocked(m.cfg.Instances[i].Key)
	}
	return nil
}

// Stop 实现 module.Module。
func (m *Module) Stop() error {
	m.mu.Lock()
	m.started = false
	runs := m.runs
	m.runs = map[string]*instRuntime{}
	m.mu.Unlock()
	for _, rt := range runs {
		rt.sup.Stop()
	}
	return nil
}

func (m *Module) saveLocked() error { return m.store.Save(ID, m.cfg) }

func (m *Module) find(key string) *Instance {
	for i := range m.cfg.Instances {
		if m.cfg.Instances[i].Key == key {
			return &m.cfg.Instances[i]
		}
	}
	return nil
}

// localBin 返回一键安装的目标路径：<配置目录>/bin/filebrowser。
func (m *Module) localBin() string {
	return m.install.MainPath()
}

// resolveBin 查找可执行文件：优先使用配置的路径，其次是在线安装到配置目录 bin 下的程序，最后才是 PATH。
// PATH 中的 filebrowser 很可能是已归档的原版，用户在界面里安装的 Quantum 应当优先生效。
func (m *Module) resolveBin() (string, error) {
	return proc.Resolve(m.cfg.Settings.BinPath, m.store.Dir(), binName)
}

// dataDir 返回实例的数据目录，保存生成的配置、数据库与缓存。
func (m *Module) dataDir(key string) string {
	return filepath.Join(m.store.Dir(), ID, key)
}

// writeConfig 把实例配置写入数据目录，返回配置文件路径。文件权限 0600，目录 0700。
func (m *Module) writeConfig(inst *Instance) (string, error) {
	dir := m.dataDir(inst.Key)
	if err := fsperm.MkdirAll(filepath.Join(m.store.Dir(), ID), 0o700); err != nil {
		return "", err
	}
	if err := fsperm.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	data, err := inst.Config(dir)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "config.yaml")
	tmp := path + ".tmp"
	if err := fsperm.WriteFile(tmp, data, 0o600); err != nil {
		return "", err
	}
	return path, os.Rename(tmp, path)
}

// applyLocked 按配置启停指定实例；调用方持有 mu。
func (m *Module) applyLocked(key string) {
	inst := m.find(key)
	if inst == nil {
		m.dropLocked(key)
		return
	}
	rt := m.runs[key]
	if rt == nil {
		rt = &instRuntime{sup: proc.New(m.logs.Logger(inst.Name))}
		m.runs[key] = rt
	}
	rt.sup.Stop()
	rt.err = ""
	if !m.started || !inst.Enable {
		return
	}
	log := m.logs.Logger(inst.Name)
	bin, err := m.resolveBin()
	if err != nil {
		rt.err = err.Error()
		log.Errorf("%v", err)
		return
	}
	// 共享目录不存在时上游会直接退出并被反复重启，这里提前报错，等待用户修正
	for _, s := range inst.Sources {
		if st, err := os.Stat(s.Path); err != nil || !st.IsDir() {
			rt.err = fmt.Sprintf("共享目录 %s 不存在或不是目录", s.Path)
			log.Errorf("%s", rt.err)
			return
		}
	}
	cfgPath, err := m.writeConfig(inst)
	if err != nil {
		rt.err = "写入配置文件失败: " + err.Error()
		log.Errorf("%s", rt.err)
		return
	}
	// 名称可能被修改，重建托管器以使用新的日志来源
	rt.sup = proc.New(log)
	rt.sup.Start(proc.Spec{Path: bin, Args: []string{"-c", cfgPath}, Env: inst.Env(), Dir: m.dataDir(key)})
}

// dropLocked 停止并移除实例的运行时；调用方持有 mu。
func (m *Module) dropLocked(key string) {
	if rt := m.runs[key]; rt != nil {
		rt.sup.Stop()
		delete(m.runs, key)
	}
}

// removeLocked 停止实例并删除其数据目录（数据库中的用户、分享链接与缓存）。
func (m *Module) removeLocked(key string) {
	m.dropLocked(key)
	if key == "" || strings.ContainsAny(key, `/\.`) {
		return
	}
	if err := os.RemoveAll(m.dataDir(key)); err != nil {
		m.logs.Logger("filebrowser").Warnf("删除实例数据目录失败: %v", err)
	}
}

// restartAllLocked 重启全部已启用的实例，用于更换可执行文件之后。
func (m *Module) restartAllLocked() {
	for i := range m.cfg.Instances {
		if m.cfg.Instances[i].Enable {
			m.applyLocked(m.cfg.Instances[i].Key)
		}
	}
}

// InstanceView 是界面展示的实例。
type InstanceView struct {
	Instance
	State proc.State `json:"state"`
	Error string     `json:"error"`
}

func (m *Module) viewLocked(inst *Instance) InstanceView {
	v := InstanceView{Instance: *inst}
	v.AdminPassword = crud.Mask(v.AdminPassword)
	// 列表中的切片与配置共用底层数组，复制一份避免界面数据被后续修改影响
	v.Sources = append([]Source(nil), inst.Sources...)
	v.AccessURLs = append([]string(nil), inst.AccessURLs...)
	if rt := m.runs[inst.Key]; rt != nil {
		v.State = rt.sup.State()
		v.Error = rt.err
		if v.Error == "" {
			v.Error = v.State.Error
		}
	}
	return v
}

// settingsView 是 GET /settings 的返回内容。
type settingsView struct {
	Settings Settings `json:"settings"`
	// Binary 为实际使用的可执行文件路径
	Binary string `json:"binary"`
	// Version 为探测到的版本，例如 v2.0.2-stable
	Version string `json:"version"`
	// Quantum 为 false 表示找到的程序不是 FileBrowser Quantum（例如已归档的原版）
	Quantum bool `json:"quantum"`
	// InstallPath 为“下载安装”写入的位置
	InstallPath string `json:"installPath"`
	Error       string `json:"error"`
}

// settingsView 返回设置与程序信息。探测版本要执行外部程序（最长数秒），在锁外进行，
// 避免卡住的程序阻塞实例列表轮询与启停。
func (m *Module) settingsView() settingsView {
	m.mu.Lock()
	out := settingsView{Settings: m.cfg.Settings, InstallPath: m.localBin()}
	bin, err := m.resolveBin()
	if err != nil {
		m.mu.Unlock()
		out.Error = err.Error()
		return out
	}
	cached := m.versionPath == bin
	version, quantum := m.version, m.quantum
	m.mu.Unlock()
	if !cached {
		version, quantum = probeVersion(bin)
		m.mu.Lock()
		m.versionPath, m.version, m.quantum = bin, version, quantum
		m.mu.Unlock()
	}
	out.Binary, out.Version, out.Quantum = bin, version, quantum
	if !out.Quantum {
		out.Error = "找到的程序不是 FileBrowser Quantum，可能是已停止维护的原版 filebrowser，请点击“下载安装”或更换可执行文件"
	}
	return out
}

// Routes 实现 module.Module。
func (m *Module) Routes(r module.Router) {
	instances := &crud.Collection[Instance]{
		Mu:       &m.mu,
		Items:    &m.cfg.Instances,
		Save:     m.saveLocked,
		Key:      func(i *Instance) *string { return &i.Key },
		Enabled:  func(i *Instance) *bool { return &i.Enable },
		Group:    func(i *Instance) *string { return &i.Group },
		Validate: func(i *Instance) error { return i.Validate() },
		Merge:    func(old, updated *Instance) { crud.KeepSecret(old.AdminPassword, &updated.AdminPassword) },
		View:     func(i *Instance) any { return m.viewLocked(i) },
		// 查看被脱敏的管理员密码明文
		Secrets: func(i *Instance) map[string]string { return map[string]string{"adminPassword": i.AdminPassword} },
		Changed: m.applyLocked,
		Removed: m.removeLocked,
	}
	instances.Register(r, "/instances")
	r.HandleFunc("POST /instances/{key}/restart", func(w http.ResponseWriter, req *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		key := req.PathValue("key")
		if m.find(key) == nil {
			httpx.Error(w, http.StatusNotFound, "实例不存在")
			return
		}
		m.applyLocked(key)
		httpx.OK(w, map[string]bool{"ok": true})
	})
	r.HandleFunc("GET /settings", func(w http.ResponseWriter, req *http.Request) {
		httpx.OK(w, m.settingsView())
	})
	r.HandleFunc("PUT /settings", func(w http.ResponseWriter, req *http.Request) {
		var s Settings
		if err := httpx.ReadJSON(req, &s); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		s.BinPath = strings.TrimSpace(s.BinPath)
		m.mu.Lock()
		defer m.mu.Unlock()
		old := m.cfg.Settings
		m.cfg.Settings = s
		if err := m.saveLocked(); err != nil {
			m.cfg.Settings = old
			httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
			return
		}
		// 二进制路径变化后重启全部已启用的实例
		m.versionPath = ""
		m.restartAllLocked()
		httpx.OK(w, map[string]bool{"ok": true})
	})
	m.install.Register(r)
	r.HandleFunc("POST /reset", func(w http.ResponseWriter, req *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if err := m.resetLocked(); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "%v", err)
			return
		}
		httpx.OK(w, map[string]bool{"ok": true})
	})
}

// resetLocked 恢复默认配置：停止并删除全部实例，连同各实例的数据目录（File Browser 的用户、分享链接与缓存），
// 清空程序路径与安装设置。共享目录中的文件与安装的程序不受影响。调用方持有 mu。
func (m *Module) resetLocked() error {
	old := m.cfg
	m.cfg = fileConfig{}
	if err := m.saveLocked(); err != nil {
		m.cfg = old
		return fmt.Errorf("保存失败: %v", err)
	}
	for key := range m.runs {
		m.dropLocked(key)
	}
	m.versionPath = ""
	if err := os.RemoveAll(filepath.Join(m.store.Dir(), ID)); err != nil {
		return fmt.Errorf("删除实例数据失败: %v", err)
	}
	m.logs.Logger("filebrowser").Infof("已恢复默认配置")
	return nil
}

// Summary 实现 module.Summarizer。
func (m *Module) Summary() []module.SummaryItem {
	m.mu.Lock()
	defer m.mu.Unlock()
	enabled, running, failed := 0, 0, 0
	for i := range m.cfg.Instances {
		inst := &m.cfg.Instances[i]
		if !inst.Enable {
			continue
		}
		enabled++
		if rt := m.runs[inst.Key]; rt != nil {
			if rt.sup.State().Running {
				running++
			} else if rt.err != "" || rt.sup.State().Error != "" {
				failed++
			}
		}
	}
	level := "ok"
	if failed > 0 {
		level = "error"
	} else if running < enabled {
		level = "warn"
	}
	return []module.SummaryItem{{Label: "运行中", Value: fmt.Sprintf("%d / %d", running, enabled), Level: level}}
}
