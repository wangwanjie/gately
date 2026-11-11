package wol

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/module"
)

// 客户端方式：被控设备上运行一个客户端脚本（Windows 为 PowerShell，Linux、macOS、OpenWrt 为 sh），
// 主动长轮询 /api/wol/public/agent/poll 取指令，收到后先确认、再在本机执行关机、重启或睡眠。
// 设备不必开放任何端口或 SSH，管理后台也不保存设备的登录凭据；客户端只认这三种固定操作，
// 服务端无法让它执行任意命令。指令只在发出后的短时间内有效，客户端离线时直接报错，不会排队到下次上线再执行。
//
// 协议（请求体为 application/x-www-form-urlencoded，凭据放在请求体的 token 字段或 Authorization: Bearer 头中）：
//
//	POST poll  字段 instance（客户端本次运行的随机标识）、host、os、version
//	           响应 text/plain："noop"（没有指令，立即再轮询）或 "cmd <id> <action>"
//	           401 凭据无效；409 同一凭据已有其他客户端在线（客户端应等待后重试）
//	POST ack   字段 instance、id、result（ok 表示已收到并开始执行，fail 表示执行失败）、msg（失败原因）
//	GET/POST agent.sh、agent.ps1  下载客户端脚本（同样要求凭据，脚本本身不含凭据）

// 时间参数，测试可调小
var (
	// agentPollWait 为一次长轮询最多挂起的时间，低于常见反向代理 60 秒的读超时
	agentPollWait = 25 * time.Second
	// agentOnlineGrace 为两次轮询之间仍视为在线的时间
	agentOnlineGrace = 10 * time.Second
	// agentAckWait 为发出指令后等待客户端确认的时间，超时视为失败且指令作废
	agentAckWait = 10 * time.Second
)

// agentFormLimit 为客户端请求体的大小上限。
const agentFormLimit = 4 << 10

// errAgentOffline 表示客户端未连接。
var errAgentOffline = errors.New("设备上的客户端未连接")

//go:embed agent/gately-agent.sh agent/gately-agent.ps1
var agentScripts embed.FS

// agentScriptFiles 为可下载的客户端脚本：下载名 → 内嵌文件路径。
var agentScriptFiles = map[string]string{
	"gately-agent.sh":  "agent/gately-agent.sh",
	"gately-agent.ps1": "agent/gately-agent.ps1",
}

// newAgentToken 生成客户端凭据：32 字节随机数的十六进制。
func newAgentToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// validAgentToken 判断凭据格式：32 到 128 个十六进制字符（恢复的备份中的凭据也须符合）。
func validAgentToken(s string) bool {
	if len(s) < 32 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// agentCmd 是发给客户端的一条指令。
type agentCmd struct {
	id     string
	action string
	// acked 在客户端确认后关闭
	acked chan struct{}
}

// agentPoll 是一次挂起中的长轮询。
type agentPoll struct {
	// ch 用于投递指令，容量为 1；只在持有 hub.mu 且该轮询仍登记在连接上时写入
	ch chan *agentCmd
	// stop 在同一客户端发起新的轮询或连接被移除时关闭，旧轮询立即返回 noop
	stop chan struct{}
}

// agentConn 是一台设备上客户端的连接状态。
type agentConn struct {
	// tokenSum 为连接时所用凭据的摘要，凭据重新生成后旧连接作废
	tokenSum [32]byte
	instance string
	// Host、OS、Version 由客户端上报，Addr 为来源地址
	host, os, version, addr string
	// since 为本次上线时间，lastSeen 为最近一次轮询结束（或开始）的时间
	since, lastSeen time.Time
	poll            *agentPoll
	// queued 为发出时客户端恰好不在轮询中的指令，下次轮询时取走；pending 为等待确认的指令
	queued, pending *agentCmd
	// last 为最近确认的指令，客户端随后报告执行失败时据此写日志
	last *agentCmd
	// conflictLog 为最近一次记录“凭据被多个客户端使用”的时间，避免刷屏
	conflictLog time.Time
}

// active 判断客户端是否在线：正在轮询，或刚结束一次轮询。
func (c *agentConn) active(now time.Time) bool {
	return c.poll != nil || now.Sub(c.lastSeen) < agentOnlineGrace
}

// agentHub 管理各设备客户端的连接与指令投递。
type agentHub struct {
	mu    sync.Mutex
	conns map[string]*agentConn
}

func newAgentHub() *agentHub {
	return &agentHub{conns: map[string]*agentConn{}}
}

// online 判断设备的客户端是否在线。
func (h *agentHub) online(key string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	c := h.conns[key]
	return c != nil && c.active(time.Now())
}

// drop 移除设备的客户端连接，挂起中的轮询立即返回，之后的轮询因凭据无效被拒绝。
func (h *agentHub) drop(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if c := h.conns[key]; c != nil {
		if c.poll != nil {
			close(c.poll.stop)
			c.poll = nil
		}
		delete(h.conns, key)
	}
}

// AgentInfo 是客户端的连接信息，供管理界面展示。
type AgentInfo struct {
	Online  bool   `json:"online"`
	Host    string `json:"host"`
	OS      string `json:"os"`
	Version string `json:"version"`
	// Addr 为客户端的来源地址
	Addr string `json:"addr"`
	// Since 为本次上线时间，LastSeen 为最近一次轮询的时间（Unix 毫秒），未知时为 0
	Since    int64 `json:"since"`
	LastSeen int64 `json:"lastSeen"`
}

// info 返回全部连接信息，按设备 key 索引。
func (h *agentHub) info() map[string]AgentInfo {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	out := make(map[string]AgentInfo, len(h.conns))
	for k, c := range h.conns {
		seen := c.lastSeen
		if c.poll != nil {
			seen = now
		}
		info := AgentInfo{Online: c.active(now), Host: c.host, OS: c.os, Version: c.version, Addr: c.addr, Since: c.since.UnixMilli()}
		// 确认关机等指令后 lastSeen 被清零以立即显示离线，此时没有可显示的通信时间，输出 0
		if !seen.IsZero() {
			info.LastSeen = seen.UnixMilli()
		}
		out[k] = info
	}
	return out
}

// send 向设备的客户端发出指令并等待确认。客户端正在轮询时立即投递，刚结束一次轮询时留到下次轮询取走；
// 超时未确认则作废指令，不会在客户端下次上线时补执行。
func (h *agentHub) send(ctx context.Context, key, action string) error {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	cmd := &agentCmd{id: hex.EncodeToString(b), action: action, acked: make(chan struct{})}
	h.mu.Lock()
	c := h.conns[key]
	if c == nil || !c.active(time.Now()) {
		h.mu.Unlock()
		return errAgentOffline
	}
	c.pending = cmd
	delivered := false
	if c.poll != nil {
		select {
		case c.poll.ch <- cmd:
			delivered = true
		default:
		}
	}
	if !delivered {
		c.queued = cmd
	}
	h.mu.Unlock()

	timer := time.NewTimer(agentAckWait)
	defer timer.Stop()
	var err error
	select {
	case <-cmd.acked:
		return nil
	case <-timer.C:
		err = errors.New("客户端未确认指令，可能已离线或网络中断")
	case <-ctx.Done():
		err = ctx.Err()
	}
	// 作废未确认的指令，避免客户端之后取走并执行
	h.mu.Lock()
	if c := h.conns[key]; c != nil {
		if c.queued == cmd {
			c.queued = nil
		}
		if c.pending == cmd {
			c.pending = nil
		}
	}
	h.mu.Unlock()
	return err
}

// agentPower 经客户端执行电源操作。
func (m *Module) agentPower(ctx context.Context, d Device, action string) error {
	log := m.logs.Logger(d.Name)
	if err := m.agents.send(ctx, d.Key, action); err != nil {
		log.Errorf(actionTexts[action].failed, err)
		return err
	}
	log.Infof("%s", actionTexts[action].acked)
	return nil
}

// syncAgentLocked 在设备修改后调用（持有 m.mu，见 crud.Collection.Changed）：不再使用客户端方式时断开客户端。
// 凭据只能经重新生成接口更换，由该接口断开旧连接。
func (m *Module) syncAgentLocked(key string) {
	for _, d := range m.cfg.Devices {
		if d.Key == key && d.ShutdownMode == ModeAgent {
			return
		}
	}
	m.agents.drop(key)
}

// agentToken 取请求中的客户端凭据：先看 Authorization: Bearer 头，再看表单中的 token 字段。
// 读取请求体后还原 r.Body，供处理函数再次读取。
func agentToken(r *http.Request) (string, url.Values) {
	form := url.Values{}
	if r.Method == http.MethodPost && r.Body != nil {
		raw, _ := io.ReadAll(io.LimitReader(r.Body, agentFormLimit+1))
		r.Body = io.NopCloser(bytes.NewReader(raw))
		if len(raw) <= agentFormLimit {
			form, _ = url.ParseQuery(string(raw))
		}
	}
	if v, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		return strings.TrimSpace(v), form
	}
	return form.Get("token"), form
}

// agentDevice 按凭据查找使用客户端方式的设备。逐个比较摘要，比较耗时与凭据内容无关。
func (m *Module) agentDevice(token string) (Device, [32]byte, bool) {
	sum := sha256.Sum256([]byte(token))
	if !validAgentToken(token) {
		return Device{}, sum, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var found Device
	ok := false
	for _, d := range m.cfg.Devices {
		if d.ShutdownMode != ModeAgent || d.AgentToken == "" {
			continue
		}
		ds := sha256.Sum256([]byte(d.AgentToken))
		if subtle.ConstantTimeCompare(sum[:], ds[:]) == 1 && !ok {
			found, ok = d, true
		}
	}
	return found, sum, ok
}

// PublicAuth 实现 module.PublicRouter：请求带有某台设备的有效客户端凭据。
func (m *Module) PublicAuth(r *http.Request) bool {
	tok, _ := agentToken(r)
	_, _, ok := m.agentDevice(tok)
	return ok
}

// PublicRoutes 实现 module.PublicRouter：客户端轮询、确认与脚本下载。
func (m *Module) PublicRoutes(r module.Router) {
	r.HandleFunc("POST /agent/poll", m.handleAgentPoll)
	r.HandleFunc("POST /agent/ack", m.handleAgentAck)
	r.HandleFunc("GET /agent/{name}", m.handleAgentScriptPublic)
	r.HandleFunc("POST /agent/{name}", m.handleAgentScriptPublic)
}

// agentRoutes 注册管理界面使用的客户端接口。
func (m *Module) agentRoutes(r module.Router) {
	r.HandleFunc("GET /agents", func(w http.ResponseWriter, _ *http.Request) {
		httpx.OK(w, m.agents.info())
	})
	module.DocOf(r, "GET /agents", module.Doc{Summary: "客户端连接信息，按设备 key 索引", Resp: map[string]AgentInfo{}})
	r.HandleFunc("POST /devices/{key}/agent-token", m.handleAgentToken)
	module.DocOf(r, "POST /devices/{key}/agent-token", module.Doc{Summary: "重新生成客户端凭据，已连接的客户端随即断开", Resp: map[string]string{}})
	r.HandleFunc("GET /agent-script/{name}", func(w http.ResponseWriter, req *http.Request) {
		m.writeAgentScript(w, req.PathValue("name"))
	})
	module.DocOf(r, "GET /agent-script/{name}", module.Doc{Summary: "下载客户端脚本（gately-agent.sh 或 gately-agent.ps1）"})
	r.HandleFunc("GET /agent-scripts", func(w http.ResponseWriter, _ *http.Request) {
		httpx.OK(w, agentScriptSums())
	})
	module.DocOf(r, "GET /agent-scripts", module.Doc{Summary: "客户端脚本的 SHA-256，按文件名索引；安装命令据此校验下载的脚本", Resp: map[string]string{}})
}

// agentScriptSums 返回各客户端脚本的 SHA-256（十六进制小写）。安装命令先校验下载的脚本再以 root / SYSTEM 执行，
// 经明文 HTTP 或跳过证书校验下载时，同一网络中的人篡改脚本也会被发现。
func agentScriptSums() map[string]string {
	out := map[string]string{}
	for name, p := range agentScriptFiles {
		if b, err := agentScripts.ReadFile(p); err == nil {
			sum := sha256.Sum256(b)
			out[name] = hex.EncodeToString(sum[:])
		}
	}
	return out
}

// handleAgentToken 重新生成设备的客户端凭据并断开旧客户端。
func (m *Module) handleAgentToken(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	m.mu.Lock()
	idx := -1
	for i, d := range m.cfg.Devices {
		if d.Key == key {
			idx = i
			break
		}
	}
	if idx < 0 {
		m.mu.Unlock()
		httpx.Error(w, http.StatusNotFound, "设备不存在")
		return
	}
	d := &m.cfg.Devices[idx]
	if d.ShutdownMode != ModeAgent {
		m.mu.Unlock()
		httpx.Error(w, http.StatusBadRequest, "该设备的关机方式不是客户端")
		return
	}
	old := d.AgentToken
	d.AgentToken = newAgentToken()
	tok, name := d.AgentToken, d.Name
	if err := m.saveLocked(); err != nil {
		d.AgentToken = old
		m.mu.Unlock()
		httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
		return
	}
	m.mu.Unlock()
	m.agents.drop(key)
	m.logs.Logger(name).Infof("已重新生成客户端凭据，旧客户端需用新凭据重新安装")
	httpx.OK(w, map[string]string{"agentToken": tok})
}

// writeAgentScript 输出客户端脚本。PowerShell 5.1 按系统代码页读取没有 BOM 的脚本，文件本身带 UTF-8 BOM。
func (m *Module) writeAgentScript(w http.ResponseWriter, name string) {
	p, ok := agentScriptFiles[name]
	if !ok {
		httpx.Error(w, http.StatusNotFound, "脚本不存在")
		return
	}
	b, err := agentScripts.ReadFile(p)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "%v", err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b)
}

// handleAgentScriptPublic 供客户端安装命令下载脚本，要求凭据：未越过安全入口的访客无从知道这个接口的存在。
func (m *Module) handleAgentScriptPublic(w http.ResponseWriter, r *http.Request) {
	tok, _ := agentToken(r)
	if _, _, ok := m.agentDevice(tok); !ok {
		agentText(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	m.writeAgentScript(w, r.PathValue("name"))
}

// agentText 输出纯文本响应，客户端脚本无需解析 JSON。
func agentText(w http.ResponseWriter, code int, s string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_, _ = io.WriteString(w, s+"\n")
}

// cleanField 规范化客户端上报的文本：只保留可打印 ASCII 中的常见字符并截断，避免写入日志与界面的内容含控制字符。
func cleanField(s string, max int) string {
	var b strings.Builder
	for _, c := range s {
		if b.Len() >= max {
			break
		}
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune(" ._-()/+:,", c) {
			b.WriteRune(c)
		}
	}
	return strings.TrimSpace(b.String())
}

// validInstance 判断客户端运行标识：8 到 64 个字母、数字、下划线或连字符。
func validInstance(s string) bool {
	if len(s) < 8 || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// agentPollHook 仅供测试：在轮询校验凭据之后、登记连接之前调用，用于复现两者之间的并发修改。
var agentPollHook func()

// handleAgentPoll 处理客户端的长轮询。同一设备同时只保留一个轮询：同一客户端重连时替换旧轮询，
// 其他客户端（例如同一凭据装在了两台机器上）在原客户端在线期间被拒绝，避免互相抢占、指令落到别的机器上。
func (m *Module) handleAgentPoll(w http.ResponseWriter, r *http.Request) {
	tok, form := agentToken(r)
	d, sum, ok := m.agentDevice(tok)
	if !ok {
		agentText(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	instance := form.Get("instance")
	if !validInstance(instance) {
		agentText(w, http.StatusBadRequest, "bad request")
		return
	}
	host, osName, version := cleanField(form.Get("host"), 64), cleanField(form.Get("os"), 64), cleanField(form.Get("version"), 32)
	addr := httpx.ClientIP(r)
	log := m.logs.Logger(d.Name)
	now := time.Now()

	if agentPollHook != nil {
		agentPollHook()
	}
	h := m.agents
	h.mu.Lock()
	c := h.conns[d.Key]
	if c != nil && c.tokenSum != sum {
		// 凭据已重新生成：旧连接作废
		if c.poll != nil {
			close(c.poll.stop)
		}
		c = nil
	}
	if c != nil && c.instance != instance && c.active(now) {
		logIt := now.Sub(c.conflictLog) > time.Minute
		if logIt {
			c.conflictLog = now
		}
		h.mu.Unlock()
		if logIt {
			log.Warnf("同一凭据有多个客户端在连接（%s 来自 %s），只保留先连接的客户端；请为每台设备单独配置", host, addr)
		}
		agentText(w, http.StatusConflict, "conflict")
		return
	}
	fresh := c == nil || !c.active(now) || c.instance != instance
	if c == nil {
		c = &agentConn{}
		h.conns[d.Key] = c
	}
	if fresh {
		c.since = now
		c.pending, c.queued, c.last = nil, nil, nil
	}
	c.tokenSum, c.instance, c.host, c.os, c.version, c.addr = sum, instance, host, osName, version, addr
	c.lastSeen = now
	if c.poll != nil {
		// 同一客户端重连（旧连接多半已断开）：旧轮询里还没送出的指令转给新轮询
		select {
		case cmd := <-c.poll.ch:
			c.queued = cmd
		default:
		}
		close(c.poll.stop)
		c.poll = nil
	}
	// 有待取走的指令时立即返回
	if cmd := c.queued; cmd != nil {
		c.queued = nil
		h.mu.Unlock()
		agentText(w, http.StatusOK, "cmd "+cmd.id+" "+cmd.action)
		return
	}
	p := &agentPoll{ch: make(chan *agentCmd, 1), stop: make(chan struct{})}
	c.poll = p
	h.mu.Unlock()
	// 校验凭据与登记连接之间，管理员可能已重新生成凭据或让设备改用其他关机方式（随后的 drop 先于登记执行），
	// 旧凭据的轮询就会留在连接表里接收之后的指令：登记后再校验一次，凭据已失效时撤销登记
	if _, _, ok := m.agentDevice(tok); !ok {
		h.mu.Lock()
		if cur := h.conns[d.Key]; cur == c && c.poll == p {
			delete(h.conns, d.Key)
		}
		h.mu.Unlock()
		agentText(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if fresh {
		log.Infof("客户端已连接：%s（%s，版本 %s，来自 %s）", host, osName, version, addr)
	}

	timer := time.NewTimer(agentPollWait)
	defer timer.Stop()
	var cmd *agentCmd
	select {
	case cmd = <-p.ch:
	case <-p.stop:
	case <-timer.C:
	case <-r.Context().Done():
	}
	h.mu.Lock()
	if cur := h.conns[d.Key]; cur == c && c.poll == p {
		c.poll = nil
		c.lastSeen = time.Now()
		// 客户端断开了连接（进程退出、设备断网或关机）：立即视为离线，不再等宽限期，
		// 换一台机器或重装后的客户端可以马上接入；同一客户端重连时照常恢复
		if r.Context().Err() != nil {
			c.lastSeen = time.Time{}
		}
	}
	// 结束轮询与投递之间的竞争：超时或被替换的同时收到了指令，照常送出
	if cmd == nil {
		select {
		case cmd = <-p.ch:
		default:
		}
	}
	// 客户端已断开：指令留给下次轮询（仍在等待确认期内才有效）
	if cmd != nil && r.Context().Err() != nil {
		if cur := h.conns[d.Key]; cur == c && c.pending == cmd {
			c.queued = cmd
		}
		cmd = nil
	}
	h.mu.Unlock()
	if r.Context().Err() != nil {
		return
	}
	if cmd != nil {
		agentText(w, http.StatusOK, "cmd "+cmd.id+" "+cmd.action)
		return
	}
	agentText(w, http.StatusOK, "noop")
}

// handleAgentAck 处理客户端的指令确认与执行失败报告。
func (m *Module) handleAgentAck(w http.ResponseWriter, r *http.Request) {
	tok, form := agentToken(r)
	d, sum, ok := m.agentDevice(tok)
	if !ok {
		agentText(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, result := form.Get("id"), form.Get("result")
	if !validInstance(form.Get("instance")) || id == "" || len(id) > 32 || (result != "ok" && result != "fail") {
		agentText(w, http.StatusBadRequest, "bad request")
		return
	}
	log := m.logs.Logger(d.Name)
	h := m.agents
	h.mu.Lock()
	c := h.conns[d.Key]
	if c == nil || c.tokenSum != sum || c.instance != form.Get("instance") {
		h.mu.Unlock()
		agentText(w, http.StatusConflict, "conflict")
		return
	}
	if result == "fail" {
		var action string
		if c.last != nil && c.last.id == id {
			action = c.last.action
		}
		h.mu.Unlock()
		if action == "" {
			agentText(w, http.StatusNotFound, "unknown command")
			return
		}
		log.Errorf(actionTexts[action].agentFailed, cleanField(form.Get("msg"), 200))
		agentText(w, http.StatusOK, "ok")
		return
	}
	cmd := c.pending
	if cmd == nil || cmd.id != id {
		h.mu.Unlock()
		// 指令已超时作废：告诉客户端不要执行
		agentText(w, http.StatusGone, "expired")
		return
	}
	c.pending, c.last = nil, cmd
	if c.queued == cmd {
		c.queued = nil
	}
	// 关机、重启、睡眠后客户端随即离线，不等宽限期结束就显示为离线
	c.lastSeen = time.Time{}
	if c.poll != nil {
		close(c.poll.stop)
		c.poll = nil
	}
	close(cmd.acked)
	h.mu.Unlock()
	agentText(w, http.StatusOK, "ok")
}
