package knock

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/knockgate"
	"github.com/wangwanjie/gately/internal/module"
)

// RuleView 是规则的接口输出：密钥脱敏，附加运行统计。
type RuleView struct {
	Rule
	Stats ruleStats `json:"stats"`
	// UsedBy 为引用该规则的端口转发、协议分流、STUN 规则与路由
	UsedBy []string `json:"usedBy"`
}

// GrantView 是一个有效授权。
type GrantView struct {
	// Addr 为放行的地址；网页登录按设置放行整个 IPv6 网段时带前缀长度（如 2001:db8:1:2::/64）
	Addr     string    `json:"addr"`
	Rule     string    `json:"rule"`
	RuleName string    `json:"ruleName"`
	Until    time.Time `json:"until"`
	// Ports 为写入防火墙的放行端口；只用于协议分流、端口转发、Web 站点的授权为空
	Ports []string `json:"ports"`
	// Source 为授权来源：spa（敲门报文）、web（在 Web 站点的放行入口登录）
	Source string `json:"source"`
	// User、Site、SiteName 为网页登录放行的用户与入口站点
	User     string `json:"user,omitempty"`
	Site     string `json:"site,omitempty"`
	SiteName string `json:"siteName,omitempty"`
}

// Status 是模块运行状态。
type Status struct {
	// Listening 为是否正在监听 UDP 敲门端口，ListenError 为监听失败的原因
	Listening   bool   `json:"listening"`
	Port        int    `json:"port"`
	ListenError string `json:"listenError,omitempty"`
	// OpenStatus 为自动放行敲门端口的结果说明
	OpenStatus string         `json:"openStatus,omitempty"`
	Firewall   FirewallStatus `json:"firewall"`
	Grants     []GrantView    `json:"grants"`
	Counters   Counters       `json:"counters"`
	OS         string         `json:"os"`
	// Now 为服务器当前时间，界面据此计算授权剩余时间（避免浏览器时钟偏差）
	Now time.Time `json:"now"`
}

type revokeRequest struct {
	// Addr 为要撤销授权的地址；网段授权写成 地址/前缀长度（与状态中授权的 addr 相同）
	Addr string `json:"addr"`
	// Rule 为规则 key，留空表示撤销该地址的全部授权
	Rule string `json:"rule"`
}

// Routes 实现 module.Module。
func (m *Module) Routes(r module.Router) {
	rules := &crud.Collection[Rule]{
		Mu:       &m.mu,
		Items:    &m.cfg.Rules,
		Save:     m.saveLocked,
		Key:      func(x *Rule) *string { return &x.Key },
		Enabled:  func(x *Rule) *bool { return &x.Enable },
		Group:    func(x *Rule) *string { return &x.Group },
		Validate: func(x *Rule) error { return x.Validate() },
		Check:    m.checkRuleLocked,
		// 启用（单个与批量）同样检查：停用的规则保存时可能没有冲突，之后修改敲门端口或协议分流后才冲突
		CheckEnable: m.checkRuleLocked,
		Merge: func(old, upd *Rule) {
			crud.KeepSecret(old.EncKey, &upd.EncKey)
			crud.KeepSecret(old.HMACKey, &upd.HMACKey)
		},
		// 复制的规则生成新密钥：两条规则共用密钥时敲门只会匹配到前一条
		Copy: func(_, dst *Rule) {
			if dst.EncKey == crud.SecretMask {
				dst.EncKey = ""
			}
			if dst.HMACKey == crud.SecretMask {
				dst.HMACKey = ""
			}
		},
		View: m.ruleViewLocked,
		// 被端口转发、协议分流等引用的规则不能删除：删除后这些入口会拒绝全部连接
		BeforeRemove: func(x *Rule) error {
			if users := knockgate.Default.Users(x.Key); len(users) > 0 {
				return fmt.Errorf("规则正被 %s 使用，请先取消引用", strings.Join(users, "、"))
			}
			return nil
		},
		Changed: m.ruleChangedLocked,
		Removed: m.ruleChangedLocked,
		Secrets: func(x *Rule) map[string]string {
			return map[string]string{"encKey": x.EncKey, "hmacKey": x.HMACKey}
		},
		Max: 256,
	}
	rules.Register(r, "/rules")
	r.HandleFunc("GET /settings", m.handleGetSettings)
	r.HandleFunc("PUT /settings", m.handlePutSettings)
	r.HandleFunc("GET /status", m.handleStatus)
	r.HandleFunc("DELETE /grants", m.handleRevoke)
	r.HandleFunc("GET /firewall/rules", m.handleFirewallRules)
	module.DocOf(r, "GET /settings", module.Doc{Summary: "全局设置", Resp: &Settings{}})
	module.DocOf(r, "PUT /settings", module.Doc{Summary: "修改全局设置", Req: &Settings{}, Resp: &Settings{}})
	module.DocOf(r, "GET /status", module.Doc{Summary: "监听、防火墙拦截状态与当前有效的授权", Resp: &Status{}})
	module.DocOf(r, "DELETE /grants", module.Doc{Summary: "撤销授权（body 为 {\"addr\": 地址, \"rule\": 规则 key}，rule 留空撤销该地址的全部授权；网段授权的地址写成 地址/前缀长度）", Req: &revokeRequest{}})
	module.DocOf(r, "GET /firewall/rules", module.Doc{Summary: "Gately 写入系统防火墙的拦截规则（文本）"})
}

// checkRuleLocked 检查规则与全局设置、协议分流的冲突：开启防火墙拦截时不能拦截敲门端口自己，
// 也不能拦截协议分流接收敲门报文（敲门路由）的 TCP 端口，否则敲门报文在建立连接前就被丢弃。
func (m *Module) checkRuleLocked(x *Rule) error {
	if !x.Firewall {
		return nil
	}
	spaPorts := knockgate.Default.SPAPorts()
	for _, p := range x.Ports {
		if m.cfg.Settings.Port != 0 && p == "udp/"+strconv.Itoa(m.cfg.Settings.Port) {
			return errors.New("不能在防火墙中拦截敲门端口本身")
		}
		for _, sp := range spaPorts {
			if p == "tcp/"+strconv.Itoa(sp) {
				return fmt.Errorf("TCP %d 是协议分流接收敲门报文的端口，不能在防火墙中拦截", sp)
			}
		}
	}
	return nil
}

func (m *Module) ruleViewLocked(x *Rule) any {
	v := RuleView{Rule: *x}
	v.EncKey, v.HMACKey = crud.Mask(x.EncKey), crud.Mask(x.HMACKey)
	v.Ports = append([]string{}, x.Ports...)
	v.UsedBy = knockgate.Default.Users(x.Key)
	m.stateMu.Lock()
	if st := m.stats[x.Key]; st != nil {
		v.Stats = *st
	}
	m.stateMu.Unlock()
	return v
}

// ruleChangedLocked 在规则新增、修改、启停或删除后撤销它的授权（密钥或端口可能已变化）并重新应用。
func (m *Module) ruleChangedLocked(key string) {
	m.revokeRuleLocked(key)
	m.applyLocked()
}

func (m *Module) handleGetSettings(w http.ResponseWriter, _ *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	httpx.OK(w, m.cfg.Settings)
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
	if s.Port != 0 {
		for _, x := range m.cfg.Rules {
			if x.Enable && x.Firewall {
				for _, p := range x.Ports {
					if p == "udp/"+strconv.Itoa(s.Port) {
						httpx.Error(w, http.StatusBadRequest, "规则「%s」在防火墙中拦截了 UDP %d，不能用作敲门端口", x.Name, s.Port)
						return
					}
				}
			}
		}
	}
	old := m.cfg.Settings
	m.cfg.Settings = s
	if err := m.saveLocked(); err != nil {
		m.cfg.Settings = old
		httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
		return
	}
	m.applyLocked()
	httpx.OK(w, s)
}

func (m *Module) handleStatus(w http.ResponseWriter, _ *http.Request) {
	m.mu.Lock()
	st := Status{Listening: m.conn != nil, Port: m.connPort, ListenError: m.listenErr, OS: runtime.GOOS, Now: m.now()}
	if st.Port == 0 {
		st.Port = m.cfg.Settings.Port
	}
	if m.cfg.Settings.AutoFirewall && m.conn != nil {
		st.OpenStatus = m.open.Status("listen")
	}
	names := map[string]string{}
	for _, x := range m.cfg.Rules {
		names[x.Key] = x.Name
	}
	m.mu.Unlock()
	st.Firewall = m.fw.snapshot()
	m.stateMu.Lock()
	st.Counters = m.counters
	fwPorts := map[[2]string][]string{}
	for k := range m.fwGrants {
		id := [2]string{k.addr.String(), k.rule}
		fwPorts[id] = append(fwPorts[id], k.access.String())
	}
	m.stateMu.Unlock()
	st.Grants = []GrantView{}
	for _, g := range knockgate.Default.List() {
		var ports []string
		if g.Source == knockgate.SourceWeb {
			// 网页登录授权按记录的实际地址写入防火墙，网段授权也显示这些地址上的放行端口
			for _, a := range g.Addrs {
				for _, p := range fwPorts[[2]string{a.String(), g.Rule}] {
					if !slices.Contains(ports, p) {
						ports = append(ports, p)
					}
				}
			}
		} else if g.Single() {
			ports = slices.Clone(fwPorts[[2]string{g.Addr().String(), g.Rule}])
		}
		if ports == nil {
			ports = []string{}
		}
		// fwPorts 由 map 遍历得到，顺序不定：排序后界面显示与接口输出稳定
		slices.Sort(ports)
		st.Grants = append(st.Grants, GrantView{Addr: g.String(), Rule: g.Rule, RuleName: names[g.Rule], Until: g.Until, Ports: ports,
			Source: g.Source, User: g.User, Site: g.Site, SiteName: g.SiteName})
	}
	httpx.OK(w, st)
}

func (m *Module) handleRevoke(w http.ResponseWriter, r *http.Request) {
	var req revokeRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	addr := strings.TrimSpace(req.Addr)
	var n int
	if strings.Contains(addr, "/") {
		// 网页登录放行的 IPv6 网段：只在进程内判断，没有写入防火墙的放行
		p, err := netip.ParsePrefix(addr)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "地址无效")
			return
		}
		n = knockgate.Default.RevokePrefix(p, strings.TrimSpace(req.Rule))
	} else {
		a, err := netip.ParseAddr(addr)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "地址无效")
			return
		}
		addr = a.Unmap().String()
		n = m.revoke(a.Unmap(), strings.TrimSpace(req.Rule))
	}
	m.logs.Logger("").Infof("撤销 %s 的 %d 个授权", addr, n)
	httpx.OK(w, map[string]int{"revoked": n})
}

func (m *Module) handleFirewallRules(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	out, err := m.fw.rules(ctx)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	httpx.OK(w, map[string]string{"rules": out})
}
