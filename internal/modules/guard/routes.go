package guard

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/i18n"
	"github.com/wangwanjie/gately/internal/module"
)

// Status 是模块运行状态。
type Status struct {
	Firewall FirewallStatus `json:"firewall"`
	// Bans 为当前封禁数量
	Bans int `json:"bans"`
	// UserTrapPorts 为防火墙不可用时由 Gately 自己监听的诱饵端口
	UserTrapPorts []int `json:"userTrapPorts"`
	// Platform 描述运行环境，界面据此提示可用的日志来源
	Platform Platform `json:"platform"`
}

// Platform 是运行环境信息。
type Platform struct {
	OS       string `json:"os"`
	InDocker bool   `json:"inDocker"`
	// Journal 为是否找到 journalctl，Docker 为 Docker 管理模块是否开启
	Journal bool `json:"journal"`
	Docker  bool `json:"docker"`
}

// BanView 是封禁列表中的一项，Jail 为触发封禁的规则名称。
type BanView struct {
	banlist.Entry
	Jail string `json:"jail,omitempty"`
}

type banRequest struct {
	// Key 为 IP 地址或 CIDR 网段
	Key string `json:"key"`
	// Seconds 为封禁时长，0 表示默认时长，-1 表示永久
	Seconds int    `json:"seconds"`
	Reason  string `json:"reason"`
}

type testRequest struct {
	// Source 为规则来源，为 web 时 Filter、Ignore 按路径规则解析，Lines 为待测试的请求路径
	Source string `json:"source,omitempty"`
	Filter string `json:"filter"`
	Ignore string `json:"ignore"`
	// Lines 为待测试的日志内容，每行一条
	Lines string `json:"lines"`
}

// TestResult 是一行日志的测试结果。
type TestResult struct {
	Line    string `json:"line"`
	Matched bool   `json:"matched"`
	Host    string `json:"host,omitempty"`
	// Ignored 为 true 表示地址在白名单中，不会计数
	Ignored bool `json:"ignored,omitempty"`
}

// Routes 实现 module.Module。
func (m *Module) Routes(r module.Router) {
	jails := &crud.Collection[Jail]{
		Mu:       &m.mu,
		Items:    &m.cfg.Jails,
		Save:     m.saveLocked,
		Key:      func(j *Jail) *string { return &j.Key },
		Enabled:  func(j *Jail) *bool { return &j.Enable },
		Group:    func(j *Jail) *string { return &j.Group },
		Validate: func(j *Jail) error { return j.Validate() },
		View:     m.jailViewLocked,
		Changed:  m.jailChangedLocked,
		Removed:  m.jailRemovedLocked,
	}
	jails.Register(r, "/jails")
	r.HandleFunc("GET /presets", func(w http.ResponseWriter, _ *http.Request) { httpx.OK(w, Presets) })
	r.HandleFunc("POST /jails/test", m.handleTest)
	r.HandleFunc("GET /status", m.handleStatus)
	r.HandleFunc("GET /settings", m.handleGetSettings)
	r.HandleFunc("PUT /settings", m.handlePutSettings)
	r.HandleFunc("GET /bans", m.handleListBans)
	r.HandleFunc("POST /bans", m.handleAddBan)
	r.HandleFunc("DELETE /bans", m.handleRemoveBan)
	r.HandleFunc("GET /firewall/rules", m.handleRules)
	module.DocOf(r, "GET /presets", module.Doc{Summary: "常见服务的规则模板", Resp: []Jail{}})
	module.DocOf(r, "POST /jails/test", module.Doc{Summary: "用日志样例测试过滤规则", Req: &testRequest{}, Resp: []TestResult{}})
	module.DocOf(r, "GET /status", module.Doc{Summary: "防火墙同步状态与运行环境", Resp: &Status{}})
	module.DocOf(r, "GET /settings", module.Doc{Summary: "全局设置", Resp: &Settings{}})
	module.DocOf(r, "PUT /settings", module.Doc{Summary: "修改全局设置", Req: &Settings{}, Resp: &Settings{}})
	module.DocOf(r, "GET /bans", module.Doc{Summary: "当前封禁的 IP", Resp: []BanView{}})
	module.DocOf(r, "POST /bans", module.Doc{Summary: "手动封禁 IP 或网段", Req: &banRequest{}, Resp: &banlist.Entry{}})
	module.DocOf(r, "DELETE /bans", module.Doc{Summary: "解除封禁（body 为 {\"key\": 地址}）", Req: &banRequest{}})
	module.DocOf(r, "GET /firewall/rules", module.Doc{Summary: "Gately 写入 nftables 的规则（文本）"})
}

// jailChangedLocked 在规则新增、修改或启停后重启该规则。
func (m *Module) jailChangedLocked(key string) {
	for _, j := range m.cfg.Jails {
		if j.Key == key {
			m.startJailLocked(j)
			return
		}
	}
}

// jailRemovedLocked 在规则删除后停止它；已产生的封禁保留到期满。
func (m *Module) jailRemovedLocked(key string) {
	if r := m.jails[key]; r != nil {
		r.stop()
		delete(m.jails, key)
	}
	m.refreshWebLocked()
}

func (m *Module) handleTest(w http.ResponseWriter, r *http.Request) {
	var req testRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if req.Source == SourceWeb {
		m.testWeb(w, req)
		return
	}
	mt, err := newMatcher(Jail{Filter: req.Filter, Ignore: req.Ignore})
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if len(mt.filters) == 0 {
		httpx.Error(w, http.StatusBadRequest, "至少填写一条过滤规则")
		return
	}
	lines := splitLines(req.Lines)
	if len(lines) > 1000 {
		lines = lines[:1000]
	}
	m.mu.Lock()
	ignore := m.cur().ignoreSet(m.localSnapshotLocked())
	m.mu.Unlock()
	out := make([]TestResult, 0, len(lines))
	for _, l := range lines {
		res := TestResult{Line: l}
		if a, ok := mt.match(l); ok {
			res.Matched = true
			res.Host = a.String()
			res.Ignored = ignore.contains(a)
		}
		out = append(out, res)
	}
	httpx.OK(w, out)
}

func (m *Module) handleStatus(w http.ResponseWriter, _ *http.Request) {
	m.mu.Lock()
	st := Status{Firewall: m.fw.snapshot(), Bans: banlist.Default.Len(), UserTrapPorts: []int{}}
	if m.trap != nil {
		st.UserTrapPorts = m.trap.listening()
	}
	m.mu.Unlock()
	_, err := exec.LookPath("journalctl")
	st.Platform = Platform{OS: runtime.GOOS, InDocker: fileExists("/.dockerenv") || fileExists("/run/.containerenv"),
		Journal: err == nil, Docker: m.deps.FollowDocker != nil}
	httpx.OK(w, st)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func (m *Module) handleGetSettings(w http.ResponseWriter, _ *http.Request) {
	httpx.OK(w, m.cur())
}

func (m *Module) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var s Settings
	if err := httpx.ReadJSON(r, &s); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if err := s.Validate(); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	old := m.cfg.Settings
	m.cfg.Settings = s
	if err := m.saveLocked(); err != nil {
		m.cfg.Settings = old
		httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
		return
	}
	m.settings.Store(&s)
	if m.started {
		banlist.Default.SetIgnore(s.ignoreSet(m.local).overlaps)
		m.fw.apply(m.ctx, s)
		m.applyTrapLocked()
	}
	httpx.OK(w, s)
}

func (m *Module) handleListBans(w http.ResponseWriter, _ *http.Request) {
	m.mu.Lock()
	names := map[string]string{}
	for _, j := range m.cfg.Jails {
		names[banlist.SourceJailPrefix+j.Key] = j.Name
	}
	m.mu.Unlock()
	list := banlist.Default.List()
	out := make([]BanView, 0, len(list))
	for _, e := range list {
		out = append(out, BanView{Entry: e, Jail: names[e.Source]})
	}
	httpx.OK(w, out)
}

func (m *Module) handleAddBan(w http.ResponseWriter, r *http.Request) {
	var req banRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	e, err := ManualBan(req.Key, req.Seconds, req.Reason, httpx.ClientIP(r), m.cur())
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	m.logs.Logger("").Infof("手动封禁 %s", e.Key)
	httpx.OK(w, i18n.Localize(r, e))
}

// ManualBan 校验并执行一次手动封禁：不能封禁当前访问者自己的地址；seconds 为 0 时沿用默认时长，-1 表示永久。
func ManualBan(input string, seconds int, reason, requester string, s Settings) (banlist.Entry, error) {
	key, err := banlist.ParseKey(input)
	if err != nil {
		return banlist.Entry{}, err
	}
	if p, ok := banlist.KeyPrefix(key); ok {
		if a, err := netip.ParseAddr(requester); err == nil && p.Contains(a.Unmap()) {
			return banlist.Entry{}, errors.New("不能封禁你当前使用的地址")
		}
	}
	if seconds != -1 && seconds != 0 && (seconds < 60 || seconds > maxBanSeconds) {
		return banlist.Entry{}, errors.New("封禁时长应在 1 分钟到 365 天之间")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "手动封禁"
	}
	o := s.banOptions(seconds, reason, banlist.SourceManual)
	// 手动封禁不受白名单限制，也不按累计次数递增
	o.Force, o.Increment = true, false
	e, _, err := banlist.Default.Ban(key, o)
	return e, err
}

func (m *Module) handleRemoveBan(w http.ResponseWriter, r *http.Request) {
	var req banRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	key := strings.TrimSpace(req.Key)
	if !banlist.Default.Unban(key) {
		// 也接受未规范化的输入，例如 IPv6 的单个地址
		if k, err := banlist.ParseKey(key); err == nil {
			key = k
			banlist.Default.Unban(k)
		}
	}
	m.logs.Logger("").Infof("解除封禁 %s", key)
	httpx.OK(w, map[string]bool{"ok": true})
}

func (m *Module) handleRules(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	out, err := m.fw.rules(ctx)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	httpx.OK(w, map[string]string{"rules": out})
}

// testWeb 用请求路径测试路径规则，只比较路径，不考虑监听、状态码与域名条件。
func (m *Module) testWeb(w http.ResponseWriter, req testRequest) {
	wm, err := newWebMatcher(Jail{Filter: req.Filter, Ignore: req.Ignore})
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	lines := splitLines(req.Lines)
	if len(lines) > 1000 {
		lines = lines[:1000]
	}
	out := make([]TestResult, 0, len(lines))
	for _, l := range lines {
		out = append(out, TestResult{Line: l, Matched: wm.pathMatched(l)})
	}
	httpx.OK(w, out)
}
