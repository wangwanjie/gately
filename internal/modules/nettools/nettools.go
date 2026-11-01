// Package nettools 提供网络诊断工具：Ping、Traceroute、TCP 端口探测、DNS 查询、测速与局域网扫描。
// 工具以后台任务运行，接口立即返回任务 ID，界面轮询输出。
package nettools

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// ID 是模块标识。
const ID = "nettools"

const (
	maxRunning   = 4
	maxTasks     = 50
	maxTaskLines = 500
	taskTimeout  = 5 * time.Minute
)

// 工具名称
const (
	ToolPing       = "ping"
	ToolTraceroute = "traceroute"
	ToolTCP        = "tcp"
	ToolDNS        = "dns"
	ToolSpeed      = "speed"
	ToolScan       = "scan"
)

// defaultDeny 是默认禁止探测的地址：云厂商元数据服务，避免工具被用于窃取实例凭据。
var defaultDeny = []string{"169.254.169.254/32", "100.100.100.200/32", "fd00:ec2::254/128"}

// Settings 是模块设置。
type Settings struct {
	// Deny 为禁止探测的 IP / 网段，在默认列表之外追加
	Deny []string `json:"deny"`
}

type fileConfig struct {
	Settings Settings `json:"settings"`
}

// Request 是运行工具的请求，各工具只使用相关字段。
type Request struct {
	Tool string `json:"tool"`
	// Target 为主机名或 IP（ping / traceroute / tcp / dns 的查询名 / speed 不使用）
	Target string `json:"target"`
	// Count 为 ping 次数，默认 4，最多 20
	Count int `json:"count"`
	// Ports 为 tcp 探测端口，如 "22,80,8000-8100"
	Ports string `json:"ports"`
	// Type 为 DNS 记录类型：A AAAA CNAME MX TXT NS PTR，默认 A
	Type string `json:"type"`
	// Server 为 DNS 服务器：223.5.5.5、223.5.5.5:5353、https://dns.alidns.com/dns-query
	Server string `json:"server"`
	// URL 为测速下载地址，留空使用默认
	URL string `json:"url"`
	// Seconds 为测速时长，默认 8，最多 30
	Seconds int `json:"seconds"`
	// CIDR 为局域网扫描网段，留空使用本机主网卡所在网段
	CIDR string `json:"cidr"`
}

// Task 是一次工具运行。
type Task struct {
	ID       string    `json:"id"`
	Tool     string    `json:"tool"`
	Title    string    `json:"title"`
	Status   string    `json:"status"` // running / done / error / canceled
	Error    string    `json:"error,omitempty"`
	Lines    []string  `json:"lines"`
	Result   any       `json:"result,omitempty"`
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished,omitzero"`

	cancel context.CancelFunc
}

// output 供工具写入进度与结构化结果。
type output struct {
	m *Module
	t *Task
}

func (o output) Line(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	o.m.mu.Lock()
	defer o.m.mu.Unlock()
	o.t.Lines = append(o.t.Lines, line)
	if len(o.t.Lines) > maxTaskLines {
		o.t.Lines = o.t.Lines[len(o.t.Lines)-maxTaskLines:]
	}
}

func (o output) SetResult(v any) {
	o.m.mu.Lock()
	o.t.Result = v
	o.m.mu.Unlock()
}

type toolFunc func(ctx context.Context, m *Module, req Request, out output) error

// Module 是网络诊断模块。
type Module struct {
	mu    sync.Mutex
	store *config.Store
	log   *logbuf.Logger
	cfg   fileConfig
	deny  []netip.Prefix
	tasks map[string]*Task
	seq   int
	tools map[string]toolFunc
}

// New 创建模块。
func New() *Module {
	m := &Module{tasks: map[string]*Task{}}
	m.tools = map[string]toolFunc{
		ToolPing:       pingTool,
		ToolTraceroute: tracerouteTool,
		ToolTCP:        tcpTool,
		ToolDNS:        dnsTool,
		ToolSpeed:      speedTool,
		ToolScan:       scanTool,
	}
	return m
}

// ID 实现 module.Module。
func (m *Module) ID() string { return ID }

// Title 实现 module.Module。
func (m *Module) Title() string { return "网络诊断" }

// Init 实现 module.Module。
func (m *Module) Init(env module.Env) error {
	m.store = env.Store
	m.log = env.Logs.Logger("网络诊断")
	if _, err := m.store.Load(ID, &m.cfg); err != nil {
		return err
	}
	var err error
	m.deny, err = buildDeny(m.cfg.Settings.Deny)
	return err
}

// Start 实现 module.Module。
func (m *Module) Start(context.Context) error { return nil }

// Stop 实现 module.Module：取消所有运行中的任务。
func (m *Module) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.tasks {
		if t.Status == "running" && t.cancel != nil {
			t.cancel()
		}
	}
	return nil
}

// buildDeny 把默认与自定义禁止列表解析为网段。
func buildDeny(custom []string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, s := range append(append([]string{}, defaultDeny...), custom...) {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if strings.Contains(s, "/") {
			p, err := netip.ParsePrefix(s)
			if err != nil {
				return nil, fmt.Errorf("无效的网段 %q", s)
			}
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(s)
		if err != nil {
			return nil, fmt.Errorf("无效的 IP %q", s)
		}
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}

// denied 判断地址是否在禁止列表中。
func (m *Module) denied(a netip.Addr) bool {
	a = a.Unmap()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.deny {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// resolve 把主机名解析为 IP 列表，并拒绝禁止列表中的地址。
func (m *Module) resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, errors.New("请填写目标地址")
	}
	var addrs []netip.Addr
	if a, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		addrs = []netip.Addr{a.Unmap()}
	} else {
		if strings.ContainsAny(host, " /\\?#@") || strings.HasPrefix(host, "-") {
			return nil, fmt.Errorf("目标地址 %q 无效", host)
		}
		ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("解析 %s 失败: %w", host, err)
		}
		for _, ip := range ips {
			addrs = append(addrs, ip.Unmap())
		}
	}
	for _, a := range addrs {
		if m.denied(a) {
			return nil, fmt.Errorf("目标 %s 在禁止探测列表中", a)
		}
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("%s 没有解析结果", host)
	}
	return addrs, nil
}

func (m *Module) start(req Request) (*Task, error) {
	fn, ok := m.tools[req.Tool]
	if !ok {
		return nil, fmt.Errorf("未知的工具 %q", req.Tool)
	}
	m.mu.Lock()
	running := 0
	for _, t := range m.tasks {
		if t.Status == "running" {
			running++
		}
	}
	if running >= maxRunning {
		m.mu.Unlock()
		return nil, fmt.Errorf("同时运行的任务已达上限 %d，请稍后再试", maxRunning)
	}
	m.seq++
	ctx, cancel := context.WithTimeout(context.Background(), taskTimeout)
	t := &Task{
		ID: strconv.FormatInt(time.Now().Unix(), 36) + "-" + strconv.Itoa(m.seq), Tool: req.Tool, Title: title(req),
		Status: "running", Lines: []string{}, Started: time.Now(), cancel: cancel,
	}
	m.tasks[t.ID] = t
	m.pruneLocked()
	m.mu.Unlock()

	go func() {
		defer cancel()
		err := fn(ctx, m, req, output{m: m, t: t})
		m.mu.Lock()
		defer m.mu.Unlock()
		t.Finished = time.Now()
		switch {
		case errors.Is(ctx.Err(), context.Canceled):
			t.Status = "canceled"
		case err != nil:
			t.Status, t.Error = "error", err.Error()
		default:
			t.Status = "done"
		}
	}()
	return t, nil
}

func title(r Request) string {
	names := map[string]string{ToolPing: "Ping", ToolTraceroute: "路由追踪", ToolTCP: "端口探测", ToolDNS: "DNS 查询", ToolSpeed: "测速", ToolScan: "局域网扫描"}
	t := names[r.Tool]
	// 前端会把整个表单一起提交，只取当前工具实际使用的字段，避免测速、扫描标题带上其他页填写的目标
	var arg string
	switch r.Tool {
	case ToolSpeed:
		arg = speedHost(r.URL)
	case ToolScan:
		arg = strings.TrimSpace(r.CIDR)
	default:
		arg = strings.TrimSpace(r.Target)
	}
	if arg == "" {
		return t
	}
	return t + " " + arg
}

// pruneLocked 只保留最近 maxTasks 个任务，运行中的任务不会被清理。
func (m *Module) pruneLocked() {
	if len(m.tasks) <= maxTasks {
		return
	}
	list := make([]*Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		list = append(list, t)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Started.Before(list[j].Started) })
	for _, t := range list {
		if len(m.tasks) <= maxTasks {
			break
		}
		if t.Status != "running" {
			delete(m.tasks, t.ID)
		}
	}
}

func (m *Module) snapshot(t *Task) Task {
	c := *t
	c.Lines = append([]string{}, t.Lines...)
	c.cancel = nil
	return c
}

// Routes 实现 module.Module。
func (m *Module) Routes(r module.Router) {
	r.HandleFunc("POST /run", m.handleRun)
	r.HandleFunc("GET /tasks", m.handleTasks)
	r.HandleFunc("GET /tasks/{id}", m.handleTask)
	r.HandleFunc("DELETE /tasks/{id}", m.handleCancel)
	r.HandleFunc("GET /settings", m.handleGetSettings)
	r.HandleFunc("PUT /settings", m.handlePutSettings)
	r.HandleFunc("GET /networks", m.handleNetworks)
	module.DocOf(r, "POST /run", module.Doc{Summary: "运行诊断工具，返回任务（tool: ping / traceroute / tcp / dns / speed / scan）", Req: &Request{}, Resp: &Task{}})
	module.DocOf(r, "GET /tasks", module.Doc{Summary: "最近的诊断任务", Resp: []Task{}})
	module.DocOf(r, "GET /tasks/{id}", module.Doc{Summary: "任务详情与输出", Resp: &Task{}})
	module.DocOf(r, "DELETE /tasks/{id}", module.Doc{Summary: "取消运行中的任务，或删除已结束的任务"})
	module.DocOf(r, "GET /settings", module.Doc{Summary: "诊断设置", Resp: &Settings{}})
	module.DocOf(r, "PUT /settings", module.Doc{Summary: "修改禁止探测列表", Req: &Settings{}, Resp: &Settings{}})
	module.DocOf(r, "GET /networks", module.Doc{Summary: "本机可用于局域网扫描的网段", Resp: []string{}})
}

func (m *Module) handleRun(w http.ResponseWriter, r *http.Request) {
	var req Request
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	t, err := m.start(req)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	httpx.OK(w, m.snapshot(t))
}

func (m *Module) handleTasks(w http.ResponseWriter, _ *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		c := m.snapshot(t)
		// 列表不带输出，减小体积
		c.Lines, c.Result = nil, nil
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Started.After(out[j].Started) })
	httpx.OK(w, out)
}

func (m *Module) handleTask(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[r.PathValue("id")]
	if !ok {
		httpx.Error(w, http.StatusNotFound, "任务不存在")
		return
	}
	httpx.OK(w, m.snapshot(t))
}

func (m *Module) handleCancel(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[r.PathValue("id")]
	if !ok {
		httpx.Error(w, http.StatusNotFound, "任务不存在")
		return
	}
	if t.Status == "running" {
		t.cancel()
	} else {
		delete(m.tasks, t.ID)
	}
	httpx.OK(w, map[string]bool{"ok": true})
}

func (m *Module) handleGetSettings(w http.ResponseWriter, _ *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.cfg.Settings
	if s.Deny == nil {
		s.Deny = []string{}
	}
	httpx.OK(w, s)
}

func (m *Module) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var s Settings
	if err := httpx.ReadJSON(r, &s); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	var clean []string
	for _, d := range s.Deny {
		if d = strings.TrimSpace(d); d != "" {
			clean = append(clean, d)
		}
	}
	s.Deny = clean
	deny, err := buildDeny(s.Deny)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	old := m.cfg.Settings
	m.cfg.Settings = s
	if err := m.store.Save(ID, m.cfg); err != nil {
		m.cfg.Settings = old
		httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
		return
	}
	m.deny = deny
	httpx.OK(w, s)
}
