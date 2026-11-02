package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// ID 是模块标识。
const ID = "docker"

// Settings 是模块配置。
type Settings struct {
	// Host 为 Docker 地址，默认 unix:///var/run/docker.sock
	Host string `json:"host"`
	// Mirrors 是镜像加速地址（如 docker.m.daocloud.io），直接拉取失败时依次尝试
	Mirrors []string `json:"mirrors"`
	// Registries 是私有仓库认证信息，拉取与推送时通过 X-Registry-Auth 传给 Docker
	Registries []Registry `json:"registries"`
	// AutoCheckHours 大于 0 时按该间隔自动检查容器镜像更新
	AutoCheckHours int `json:"autoCheckHours"`
}

// Registry 是一个镜像仓库的登录信息。
type Registry struct {
	Server   string `json:"server"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// UpdateInfo 是镜像更新检查结果。
type UpdateInfo struct {
	Checked   time.Time `json:"checked"`
	HasUpdate bool      `json:"hasUpdate"`
	Error     string    `json:"error,omitempty"`
}

// Module 是 Docker 管理模块。
type Module struct {
	mu       sync.Mutex
	store    *config.Store
	logs     *logbuf.Buffer
	settings Settings
	updates  map[string]UpdateInfo // 容器 ID -> 检查结果
	busy     map[string]bool

	// 定时检查更新：parent 为 Start 传入的上下文，auto 为当前运行的检查协程
	autoMu sync.Mutex
	// stopWatch 结束容器事件订阅
	stopWatch context.CancelFunc
	parent    context.Context
	auto      *autoRunner
	// notified 记录定时检查已通知过的更新（容器 ID -> 最新镜像 ID），同一个新镜像只通知一次，由 mu 保护
	notified map[string]string

	composeMu    sync.Mutex
	composeCache composeInfo

	// 总览页摘要缓存
	summary summaryCache
}

// New 创建模块。
func New() *Module {
	return &Module{updates: map[string]UpdateInfo{}, busy: map[string]bool{}}
}

// ID 实现 module.Module。
func (m *Module) ID() string { return ID }

// Title 实现 module.Module。
func (m *Module) Title() string { return "Docker 管理" }

// Init 实现 module.Module。
func (m *Module) Init(env module.Env) error {
	m.store, m.logs = env.Store, env.Logs
	_, err := m.store.Load(ID, &m.settings)
	return err
}

// Start 实现 module.Module：按设置启动定时检查更新。
func (m *Module) Start(ctx context.Context) error {
	m.autoMu.Lock()
	m.parent = ctx
	m.autoMu.Unlock()
	m.restartAutoCheck()
	// 容器异常退出事件订阅，供通知中心使用
	wctx, cancel := context.WithCancel(ctx)
	m.autoMu.Lock()
	m.stopWatch = cancel
	m.autoMu.Unlock()
	go m.runEventWatch(wctx)
	// 读取上次升级自身留下的辅助容器结果
	go m.collectSelfUpgrades(wctx)
	return nil
}

// Stop 实现 module.Module：停止定时检查并等待其退出。
func (m *Module) Stop() error {
	m.autoMu.Lock()
	m.parent = nil
	if m.stopWatch != nil {
		m.stopWatch()
		m.stopWatch = nil
	}
	m.autoMu.Unlock()
	m.restartAutoCheck()
	return nil
}

func (m *Module) client() (*Client, error) {
	m.mu.Lock()
	host := m.settings.Host
	m.mu.Unlock()
	return NewClient(host)
}

// ContainerView 是界面展示的容器信息。
type ContainerView struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Image    string            `json:"image"`
	ImageID  string            `json:"imageId"`
	State    string            `json:"state"`
	Status   string            `json:"status"`
	Created  int64             `json:"created"`
	Started  string            `json:"started,omitempty"`
	Ports    []string          `json:"ports"`
	Networks []string          `json:"networks"`
	Mounts   int               `json:"mounts"`
	Project  string            `json:"project,omitempty"`
	Update   *UpdateInfo       `json:"update,omitempty"`
	Busy     bool              `json:"busy"`
	Self     bool              `json:"self,omitempty"`
	Labels   map[string]string `json:"-"`
}

// Routes 实现 module.Module。
func (m *Module) Routes(r module.Router) {
	m.registerTaskRoutes(r.HandleFunc)
	m.registerResourceRoutes(r.HandleFunc)
	m.registerSystemRoutes(r.HandleFunc)
	module.DocOf(r, "GET /settings/secrets", module.Doc{Summary: "查看仓库密码明文（键为 `registry:<仓库地址>`）", Resp: map[string]string{}})
	m.registerContainerRoutes(r.HandleFunc)
	m.registerContainerFileRoutes(r.HandleFunc)
	m.registerExecRoutes(r.HandleFunc)
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// containerImage 从 inspect 结果中取镜像引用与当前镜像 ID。
func containerImage(info map[string]json.RawMessage) (ref, imageID, name string, err error) {
	var cfg struct {
		Image string `json:"Image"`
	}
	if err = json.Unmarshal(info["Config"], &cfg); err != nil {
		return
	}
	_ = json.Unmarshal(info["Image"], &imageID)
	_ = json.Unmarshal(info["Name"], &name)
	ref = cfg.Image
	if ref == "" || len(ref) == 71 && ref[:7] == "sha256:" {
		err = errors.New("容器使用镜像 ID 创建，无法检查更新")
	}
	return
}

// checkUpdate 拉取最新镜像并与容器当前镜像比较。
func (m *Module) checkUpdate(ctx context.Context, c *Client, id string) (bool, error) {
	has, _, err := m.checkUpdateMemo(ctx, c, id, nil)
	return has, err
}

// checkUpdateMemo 与 checkUpdate 相同，另外返回拉取到的最新镜像 ID；
// memo 记录本轮已拉取的镜像引用及其最新镜像 ID，批量检查时同一引用只拉取一次。
func (m *Module) checkUpdateMemo(ctx context.Context, c *Client, id string, memo map[string]string) (bool, string, error) {
	info, err := c.Inspect(ctx, id)
	if err != nil {
		return false, "", err
	}
	ref, cur, _, err := containerImage(info)
	if err != nil {
		return false, "", err
	}
	latest, ok := memo[ref]
	if !ok {
		if err := m.pullImageWithMirrors(ctx, c, ref, nil); err != nil {
			return false, "", fmt.Errorf("拉取 %s 失败: %w", ref, err)
		}
		latest, err = c.ImageID(ctx, ref)
		if err != nil {
			return false, "", err
		}
		if memo != nil {
			memo[ref] = latest
		}
	}
	return latest != "" && latest != cur, latest, nil
}

// upgrade 拉取最新镜像并按原配置重建容器；新容器启动失败时恢复旧容器。
// ctx 只约束检查与拉取阶段；停止旧容器之后的每一步使用独立超时，保证恢复流程不因调用方取消而中断。
func (m *Module) upgrade(ctx context.Context, c *Client, id string) (string, error) {
	info, err := c.Inspect(ctx, id)
	if err != nil {
		return "", err
	}
	ref, _, name, err := containerImage(info)
	if err != nil {
		return "", err
	}
	name = trimSlash(name)
	log := m.logs.Logger(name)
	log.Infof("开始升级，拉取 %s", ref)
	if err := m.pullImageWithMirrors(ctx, c, ref, func(line string) { log.Infof("%s", line) }); err != nil {
		return "", fmt.Errorf("拉取镜像失败: %w", err)
	}
	body, extraNets, err := recreateBody(info)
	if err != nil {
		return "", err
	}
	return m.rebuild(c, id, info, name, name, body, extraNets, log)
}

// rebuildLogger 是重建过程的日志输出：模块内为 *logbuf.Logger，升级自身的辅助容器内输出到标准输出。
type rebuildLogger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// rebuild 用 body 重建容器：停止旧容器 -> 改名备份 -> 创建新容器 -> 连接其余网络 -> 启动，任一步失败都恢复旧容器。
// 旧容器原本在运行才会启动新容器。停止旧容器之后的每一步使用独立超时，保证恢复流程不因调用方取消而中断。
func (m *Module) rebuild(c *Client, id string, info map[string]json.RawMessage, name, newName string, body map[string]any, extraNets []networkAttach, log rebuildLogger) (string, error) {
	var state struct {
		Running bool `json:"Running"`
	}
	_ = json.Unmarshal(info["State"], &state)
	// 调用方传入的 id 可能是容器名或 ID 前缀。旧容器改名备份后原名归新容器所有，
	// 之后按原名操作（恢复、删除旧容器）会落到新容器上，因此统一改用 inspect 得到的完整 ID
	var fullID string
	_ = json.Unmarshal(info["Id"], &fullID)
	if fullID != "" {
		id = fullID
	}

	// step 为单个容器操作提供独立超时
	step := func(fn func(context.Context) error) error {
		sctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
		defer cancel()
		return fn(sctx)
	}
	backup := name + "_gately_old_" + time.Now().Format("20060102150405")
	if state.Running {
		if err := step(func(c2 context.Context) error { return c.Action(c2, id, "stop") }); err != nil {
			return "", fmt.Errorf("停止旧容器失败: %w", err)
		}
	}
	if err := step(func(c2 context.Context) error { return c.Rename(c2, id, backup) }); err != nil {
		if state.Running {
			_ = step(func(c2 context.Context) error { return c.Action(c2, id, "start") })
		}
		return "", fmt.Errorf("重命名旧容器失败: %w", err)
	}
	restore := func(cause error) error {
		log.Errorf("重建失败，恢复旧容器: %v", cause)
		if err := step(func(c2 context.Context) error { return c.Rename(c2, id, name) }); err != nil {
			log.Errorf("恢复旧容器名称失败，请手动处理 %s: %v", backup, err)
		}
		if state.Running {
			if err := step(func(c2 context.Context) error { return c.Action(c2, id, "start") }); err != nil {
				log.Errorf("重新启动旧容器失败: %v", err)
			}
		}
		return cause
	}
	var newID string
	if err := step(func(c2 context.Context) error {
		var err error
		newID, err = c.Create(c2, newName, body)
		return err
	}); err != nil {
		return "", restore(fmt.Errorf("创建新容器失败: %w", err))
	}
	discard := func(cause error) (string, error) {
		_ = step(func(c2 context.Context) error { return c.Remove(c2, newID) })
		return "", restore(cause)
	}
	// 旧版 Docker 创建时只接受一个网络，其余网络在创建后逐个连接
	for _, n := range extraNets {
		if err := step(func(c2 context.Context) error { return c.ConnectNetwork(c2, n.name, newID, n.endpoint) }); err != nil {
			return discard(fmt.Errorf("连接网络 %s 失败: %w", n.name, err))
		}
	}
	if state.Running {
		if err := step(func(c2 context.Context) error { return c.Action(c2, newID, "start") }); err != nil {
			return discard(fmt.Errorf("启动新容器失败: %w", err))
		}
	}
	if err := step(func(c2 context.Context) error { return c.Remove(c2, id) }); err != nil {
		log.Warnf("删除旧容器 %s 失败，请手动清理: %v", backup, err)
	}
	log.Infof("重建完成，新容器 %s", shortID(newID))
	return newID, nil
}

func trimSlash(s string) string {
	for len(s) > 0 && s[0] == '/' {
		s = s[1:]
	}
	return s
}

// networkAttach 是创建后需要追加连接的网络。
type networkAttach struct {
	name     string
	endpoint map[string]any
}

// recreateBody 用旧容器的配置构造创建请求：Config 与 HostConfig 原样保留，网络只保留别名与固定 IP 等用户配置。
// 多网络容器只在创建请求中放主网络，其余网络通过返回值在创建后连接。
func recreateBody(info map[string]json.RawMessage) (map[string]any, []networkAttach, error) {
	var cfg map[string]any
	if err := json.Unmarshal(info["Config"], &cfg); err != nil {
		return nil, nil, err
	}
	// 主机名默认取自容器 ID，沿用会导致新容器使用旧 ID 作为主机名
	if host, _ := cfg["Hostname"].(string); host != "" {
		var id string
		_ = json.Unmarshal(info["Id"], &id)
		if len(id) >= len(host) && id[:len(host)] == host {
			delete(cfg, "Hostname")
		}
	}
	var hostCfg map[string]any
	if err := json.Unmarshal(info["HostConfig"], &hostCfg); err != nil {
		return nil, nil, err
	}
	keepAnonymousVolumes(info, hostCfg)
	var ns struct {
		Networks map[string]struct {
			Aliases    []string       `json:"Aliases"`
			IPAMConfig map[string]any `json:"IPAMConfig"`
			Links      []string       `json:"Links"`
		} `json:"Networks"`
	}
	_ = json.Unmarshal(info["NetworkSettings"], &ns)
	names := make([]string, 0, len(ns.Networks))
	for n := range ns.Networks {
		names = append(names, n)
	}
	sort.Strings(names)
	// 主网络优先取 NetworkMode 指定的网络
	if mode, _ := hostCfg["NetworkMode"].(string); mode != "" {
		for i, n := range names {
			if n == mode {
				names[0], names[i] = names[i], names[0]
				break
			}
		}
	}
	endpoints := map[string]any{}
	var extra []networkAttach
	for i, netName := range names {
		n := ns.Networks[netName]
		ep := map[string]any{}
		if len(n.Aliases) > 0 {
			ep["Aliases"] = n.Aliases
		}
		if len(n.IPAMConfig) > 0 {
			ep["IPAMConfig"] = n.IPAMConfig
		}
		if len(n.Links) > 0 {
			ep["Links"] = n.Links
		}
		if i == 0 {
			endpoints[netName] = ep
		} else {
			extra = append(extra, networkAttach{name: netName, endpoint: ep})
		}
	}
	body := cfg
	body["HostConfig"] = hostCfg
	body["NetworkingConfig"] = map[string]any{"EndpointsConfig": endpoints}
	return body, extra, nil
}

// keepAnonymousVolumes 把镜像声明、未显式挂载的匿名卷加入 Binds，新容器沿用原数据卷，避免升级后数据“消失”。
func keepAnonymousVolumes(info map[string]json.RawMessage, hostCfg map[string]any) {
	var mounts []struct {
		Type        string `json:"Type"`
		Name        string `json:"Name"`
		Destination string `json:"Destination"`
		RW          bool   `json:"RW"`
	}
	_ = json.Unmarshal(info["Mounts"], &mounts)
	covered := map[string]bool{}
	var binds []any
	if b, ok := hostCfg["Binds"].([]any); ok {
		binds = b
	}
	for _, b := range binds {
		if s, ok := b.(string); ok {
			parts := strings.Split(s, ":")
			if len(parts) >= 2 {
				covered[parts[1]] = true
			}
		}
	}
	if ms, ok := hostCfg["Mounts"].([]any); ok {
		for _, m := range ms {
			if mm, ok := m.(map[string]any); ok {
				if t, _ := mm["Target"].(string); t != "" {
					covered[t] = true
				}
			}
		}
	}
	added := false
	for _, m := range mounts {
		if m.Type != "volume" || m.Name == "" || covered[m.Destination] {
			continue
		}
		bind := m.Name + ":" + m.Destination
		if !m.RW {
			bind += ":ro"
		}
		binds = append(binds, bind)
		covered[m.Destination] = true
		added = true
	}
	if added {
		hostCfg["Binds"] = binds
	}
}

// registryAuth 返回镜像引用对应仓库的 X-Registry-Auth 头值（base64url 编码的 JSON），没有配置时返回空字符串。
func (m *Module) registryAuth(ref string) string {
	host := registryHost(ref)
	m.mu.Lock()
	regs := append([]Registry{}, m.settings.Registries...)
	m.mu.Unlock()
	for _, r := range regs {
		if normalizeRegistry(r.Server) != host {
			continue
		}
		if r.Username == "" && r.Password == "" {
			return ""
		}
		addr := r.Server
		if host == "docker.io" {
			addr = "https://index.docker.io/v1/"
		}
		return encodeAuth(addr, r.Username, r.Password)
	}
	return ""
}

// registryHost 返回镜像引用所属仓库的域名。首段含点、冒号或为 localhost 时视为域名，否则是 Docker Hub（docker.io）。
func registryHost(ref string) string {
	first, _, found := strings.Cut(ref, "/")
	if !found {
		return "docker.io"
	}
	if strings.ContainsAny(first, ".:") || first == "localhost" || first != strings.ToLower(first) {
		return normalizeRegistry(first)
	}
	return "docker.io"
}

// normalizeRegistry 统一仓库地址写法：去掉协议与路径、转小写，并把 Docker Hub 的各种别名归一为 docker.io。
func normalizeRegistry(server string) string {
	s := strings.ToLower(strings.TrimSpace(server))
	s = strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	switch s {
	case "", "docker.io", "index.docker.io", "registry-1.docker.io", "registry.hub.docker.com":
		return "docker.io"
	}
	return s
}

// ContainerNames 返回全部容器的名称，供证书部署等功能选择要重启的容器。
func (m *Module) ContainerNames(ctx context.Context) ([]string, error) {
	c, err := m.client()
	if err != nil {
		return nil, err
	}
	list, err := c.Containers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(list))
	for _, ct := range list {
		out = append(out, ct.Name())
	}
	sort.Strings(out)
	return out, nil
}

// FollowLogs 持续读取容器新产生的日志行，直到 ctx 取消或容器日志流结束，供入侵防护的日志规则使用。
func (m *Module) FollowLogs(ctx context.Context, name string, onLine func(string)) error {
	c, err := m.client()
	if err != nil {
		return err
	}
	var pending []byte
	err = c.FollowLogs(ctx, name, func(b []byte) {
		pending = append(pending, b...)
		for {
			i := bytes.IndexByte(pending, '\n')
			if i < 0 {
				break
			}
			line := strings.TrimRight(string(pending[:i]), "\r")
			pending = pending[i+1:]
			if line != "" {
				onLine(line)
			}
		}
		// 没有换行的超长内容直接丢弃，避免占用内存
		if len(pending) > 64<<10 {
			pending = pending[:0]
		}
	})
	if ctx.Err() != nil {
		return nil
	}
	if err == nil || errors.Is(err, io.EOF) {
		return errors.New("日志流已结束，容器可能已停止")
	}
	return err
}

// RestartContainers 依次重启指定容器（名称或 ID），例如证书更新后让服务加载新证书。
func (m *Module) RestartContainers(ctx context.Context, names []string) error {
	c, err := m.client()
	if err != nil {
		return err
	}
	var errs []error
	for _, name := range names {
		if err := c.Action(ctx, name, "restart"); err != nil {
			errs = append(errs, fmt.Errorf("重启容器 %s 失败: %w", name, err))
			continue
		}
		m.logs.Add(logbuf.LevelInfo, "", fmt.Sprintf("证书已更新，重启容器 %s", name))
	}
	return errors.Join(errs...)
}
