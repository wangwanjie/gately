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
	// IdentityStats 为各身份的运行统计：身份 ID -> 统计，没有记录的身份不出现
	IdentityStats map[string]ruleStats `json:"identityStats"`
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
	// Identity 为敲门报文授权的身份名称（用共享密钥敲门时为空）
	Identity string `json:"identity,omitempty"`
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
			mergeIdentitySecrets(old.Identities, upd.Identities)
		},
		// 复制的规则生成新密钥：两条规则共用密钥时敲门只会匹配到前一条
		Copy: func(_, dst *Rule) {
			if dst.EncKey == crud.SecretMask {
				dst.EncKey = ""
			}
			if dst.HMACKey == crud.SecretMask {
				dst.HMACKey = ""
			}
			// 身份同样生成新密钥：各人的密钥只属于原规则
			for i := range dst.Identities {
				dst.Identities[i].EncKey, dst.Identities[i].HMACKey = "", ""
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
		Changed: m.ruleEditedLocked,
		Removed: m.ruleChangedLocked,
		// 身份的密钥以 identities.<身份 ID>.encKey / .hmacKey 为字段名
		Secrets: func(x *Rule) map[string]string {
			out := map[string]string{"encKey": x.EncKey, "hmacKey": x.HMACKey}
			for _, id := range x.Identities {
				out["identities."+id.ID+".encKey"] = id.EncKey
				out["identities."+id.ID+".hmacKey"] = id.HMACKey
			}
			return out
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
	if err := m.checkKeysLocked(x); err != nil {
		return err
	}
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

// checkKeysLocked 检查规则的 HMAC 密钥（共享密钥与各身份）没有与其他规则重复，并限制全部启用规则接受敲门的密钥总数。
// 密钥重复时报文只会匹配到排在前面的那个，后面的规则或身份永远认不出来；密钥总数决定伪造报文的最大校验成本。
func (m *Module) checkKeysLocked(x *Rule) error {
	// 值为身份名称，共享密钥为空串
	mine := map[string]string{x.HMACKey: ""}
	for _, id := range x.Identities {
		mine[id.HMACKey] = id.Name
	}
	total := x.acceptKeys()
	for _, o := range m.cfg.Rules {
		if o.Key == x.Key && x.Key != "" {
			continue
		}
		total += o.acceptKeys()
		others := append([]string{o.HMACKey}, func() []string {
			out := make([]string, 0, len(o.Identities))
			for _, id := range o.Identities {
				out = append(out, id.HMACKey)
			}
			return out
		}()...)
		for _, k := range others {
			if name, dup := mine[k]; dup {
				if name == "" {
					return fmt.Errorf("共享密钥的 HMAC 密钥与规则「%s」中的密钥相同，请重新生成", o.Name)
				}
				return fmt.Errorf("身份「%s」的 HMAC 密钥与规则「%s」中的密钥相同，请重新生成", name, o.Name)
			}
		}
	}
	if total > maxTotalKeys {
		return fmt.Errorf("全部启用规则接受敲门的密钥（共享密钥与身份）合计不能超过 %d 个", maxTotalKeys)
	}
	return nil
}

func (m *Module) ruleViewLocked(x *Rule) any {
	v := RuleView{Rule: *x, IdentityStats: map[string]ruleStats{}}
	v.EncKey, v.HMACKey = crud.Mask(x.EncKey), crud.Mask(x.HMACKey)
	v.Ports = append([]string{}, x.Ports...)
	v.Identities = make([]Identity, len(x.Identities))
	for i, id := range x.Identities {
		id.EncKey, id.HMACKey = crud.Mask(id.EncKey), crud.Mask(id.HMACKey)
		v.Identities[i] = id
	}
	v.UsedBy = knockgate.Default.Users(x.Key)
	m.stateMu.Lock()
	if st := m.stats[x.Key]; st != nil {
		v.Stats = *st
	}
	for id, st := range m.idStats[x.Key] {
		v.IdentityStats[id] = *st
	}
	m.stateMu.Unlock()
	return v
}

// mergeIdentitySecrets 把提交中仍为脱敏占位符的身份密钥按身份 ID 换回原值；找不到原身份（新身份或 ID 被改动）时
// 清空，由校验生成新密钥，不会把占位符当作密钥保存。
func mergeIdentitySecrets(old, upd []Identity) {
	byID := make(map[string]*Identity, len(old))
	for i := range old {
		byID[old[i].ID] = &old[i]
	}
	for i := range upd {
		u := &upd[i]
		o := byID[u.ID]
		if u.ID == "" {
			o = nil
		}
		if u.EncKey == crud.SecretMask {
			u.EncKey = ""
			if o != nil {
				u.EncKey = o.EncKey
			}
		}
		if u.HMACKey == crud.SecretMask {
			u.HMACKey = ""
			if o != nil {
				u.HMACKey = o.HMACKey
			}
		}
	}
}

// ruleChangedLocked 在规则删除后撤销它的全部授权并重新应用。
func (m *Module) ruleChangedLocked(key string) {
	m.revokeRuleLocked(key)
	m.applyLocked()
}

// ruleEditedLocked 在规则新增、修改或启停后重新应用。规则停用时撤销全部授权；仍启用时只撤销敲门报文取得的授权
// （密钥可能已变化），网页登录取得的授权与密钥无关，保留下来，按新的端口重新写入系统防火墙，
// 否则改个名称或分组就会让所有已登录用户的放行失效。
//
// 只改了身份、名称、分组、备注或单地址放行等不影响已有放行的字段时，只撤销密钥已失效（身份被删除、停用，
// 更换了密钥，或停用了共享密钥）的敲门授权：给规则添加一个人不会让其他人的放行全部失效。
func (m *Module) ruleEditedLocked(key string) {
	i := slices.IndexFunc(m.cfg.Rules, func(x Rule) bool { return x.Key == key })
	if i < 0 || !m.cfg.Rules[i].Enable {
		m.ruleChangedLocked(key)
		return
	}
	cur := m.cfg.Rules[i]
	// 快照在 applyLocked 之前仍是修改前的规则；修改前停用的规则不在快照中，也没有授权
	var old *Rule
	for _, r := range m.snap.Load().rules {
		if r.rule.Key == key {
			old = &r.rule
			break
		}
	}
	if old != nil && samePolicy(*old, cur) {
		valid := cur.creds()
		kept := map[netip.Addr]bool{}
		knockgate.Default.RevokeFunc(func(g knockgate.Grant) bool {
			if g.Rule != key || g.Source == knockgate.SourceWeb {
				return false
			}
			if valid[g.Cred] {
				kept[g.Addr()] = true
				return false
			}
			return true
		})
		ids := map[string]bool{}
		for _, id := range cur.Identities {
			ids[id.ID] = true
		}
		m.stateMu.Lock()
		// 保留仍有效的敲门授权的防火墙放行（端口没有变化）；其余放行删掉，重新应用时按 fwGrants 重建防火墙。
		// 网页授权随后由 syncWebFirewall 写回
		for k := range m.fwGrants {
			if k.rule == key && (k.web || !kept[k.addr]) {
				delete(m.fwGrants, k)
			}
		}
		for id := range m.idStats[key] {
			if !ids[id] {
				delete(m.idStats[key], id)
			}
		}
		m.stateMu.Unlock()
		m.applyLocked()
		m.kickWeb()
		return
	}
	knockgate.Default.RevokeFunc(func(g knockgate.Grant) bool { return g.Rule == key && g.Source != knockgate.SourceWeb })
	m.stateMu.Lock()
	// 防火墙放行记录全部清掉：重新应用时规则的端口可能已变，网页授权随后由 syncWebFirewall 按新端口写回
	for k := range m.fwGrants {
		if k.rule == key {
			delete(m.fwGrants, k)
		}
	}
	delete(m.stats, key)
	delete(m.idStats, key)
	m.stateMu.Unlock()
	m.applyLocked()
	m.kickWeb()
}

// samePolicy 判断两个版本的规则放行范围相同：端口、防火墙拦截、时长、用户名与地址要求不变时，
// 已有的敲门授权仍符合规则，只需按凭据判断是否失效。
func samePolicy(a, b Rule) bool {
	// 密钥与 HMAC 算法不在比较之列：更换后凭据标识随之改变，对应授权按凭据撤销。
	// 新增影响放行范围的字段时要加到这里，否则修改该字段不会撤销已有授权
	return slices.Equal(a.Ports, b.Ports) && a.Firewall == b.Firewall && a.Timeout == b.Timeout && a.MaxTimeout == b.MaxTimeout &&
		a.User == b.User && a.RequireAddr == b.RequireAddr && a.Enable == b.Enable
}

// creds 返回规则当前接受敲门的凭据标识（见 credID）：未停用的共享密钥与启用的身份。
func (r *Rule) creds() map[string]bool {
	out := map[string]bool{}
	if k, err := r.spaKey(); err == nil && !r.SharedKeyOff {
		out[credID("", k)] = true
	}
	for _, id := range r.Identities {
		if !id.Enable {
			continue
		}
		if k, err := id.spaKey(r.HMACType); err == nil {
			out[credID(id.ID, k)] = true
		}
	}
	return out
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
		v := GrantView{Addr: g.String(), Rule: g.Rule, RuleName: names[g.Rule], Until: g.Until, Ports: ports,
			Source: g.Source, User: g.User, Site: g.Site, SiteName: g.SiteName}
		if g.Source != knockgate.SourceWeb {
			// 敲门报文授权的 Owner.User 为身份名称
			v.Identity, v.User = g.User, ""
		}
		st.Grants = append(st.Grants, v)
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
