package webservice

import (
	"net/http"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wangwanjie/gately/internal/geoip"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/visitors"
)

// 访客审计：按访客地址聚合全部站点（含未匹配站点的请求）的访问情况，与站点是否开启访问日志无关。
// 数据见 internal/visitors，这里负责上报请求与提供接口。

// visitorsPath 返回访客审计数据文件的位置（配置目录下，不进入只收集顶层 *.json 的配置备份）。
func (m *Module) visitorsPath() string {
	return filepath.Join(m.store.Dir(), "visitors", "visitors.jsonl.gz")
}

// initVisitors 创建访客存储并读取落盘数据；设置无效（手工编辑配置）时改用默认值并记录日志。
func (m *Module) initVisitors() {
	lg := m.logs.Logger("")
	m.visits = visitors.New(m.visitorsPath(), lg.Warnf)
	set := m.cfg.Visitors
	if err := set.Normalize(); err != nil {
		lg.Warnf("访客审计设置无效（%v），已改用默认值", err)
		set = visitors.Settings{Disable: set.Disable}
		_ = set.Normalize()
	}
	m.visits.Apply(set)
	if err := m.visits.Load(); err != nil {
		lg.Warnf("读取访客审计数据失败: %v", err)
	}
	m.auth.visits = m.visits
}

// visitorCountry 返回访客地址的国家代码，内网与本机地址返回 LAN。
func visitorCountry(a netip.Addr) string {
	if geoip.IsLocal(a) {
		return "LAN"
	}
	return geoip.Default.Country(a)
}

// listenerNamesLocked 返回监听 key 到显示名称的映射，界面据此显示站点所属的监听。
func (m *Module) listenerNamesLocked() map[string]string {
	out := make(map[string]string, len(m.cfg.Listeners))
	for _, l := range m.cfg.Listeners {
		out[l.Key] = listenerLogLabel(l, m.cfg.Listeners)
	}
	return out
}

// visitorsResponse 是访客列表接口的返回值。
type visitorsResponse struct {
	visitors.Result
	Info visitors.Info `json:"info"`
	// Listeners 为监听 key 到名称的映射
	Listeners map[string]string `json:"listeners"`
}

func (m *Module) handleVisitors(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	atoi := func(k string) int {
		n, _ := strconv.Atoi(q.Get(k))
		return n
	}
	query := visitors.Query{
		Days: atoi("days"), Q: clip(q.Get("q"), 128), Filter: q.Get("filter"), Site: clip(q.Get("site"), 256),
		Sort: q.Get("sort"), Offset: atoi("offset"), Limit: atoi("limit"),
		Banned: m.auth.banned, Country: visitorCountry,
	}
	res := m.visits.Query(query)
	m.mu.Lock()
	names := m.listenerNamesLocked()
	m.mu.Unlock()
	httpx.OK(w, visitorsResponse{Result: res, Info: m.visits.Info(), Listeners: names})
}

// visitorDetail 是单个访客接口的返回值。
type visitorDetail struct {
	visitors.Detail
	Listeners map[string]string `json:"listeners"`
}

func (m *Module) handleVisitorDetail(w http.ResponseWriter, r *http.Request) {
	a, err := netip.ParseAddr(strings.TrimSpace(r.URL.Query().Get("ip")))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "IP 地址无效")
		return
	}
	d, ok := m.visits.Detail(a, m.auth.banned, visitorCountry)
	if !ok {
		httpx.Error(w, http.StatusNotFound, "没有这个访客的记录")
		return
	}
	m.mu.Lock()
	names := m.listenerNamesLocked()
	m.mu.Unlock()
	httpx.OK(w, visitorDetail{Detail: d, Listeners: names})
}

// visitorsDeleteRequest 删除访客记录：All 为 true 时清空全部，否则删除 IPs 中的地址。
type visitorsDeleteRequest struct {
	IPs []string `json:"ips"`
	All bool     `json:"all"`
}

// maxDeleteIPs 是一次最多删除的地址数。
const maxDeleteIPs = 1000

func (m *Module) handleDeleteVisitors(w http.ResponseWriter, r *http.Request) {
	var req visitorsDeleteRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	var n int
	if req.All {
		n = m.visits.Clear()
	} else {
		if len(req.IPs) == 0 || len(req.IPs) > maxDeleteIPs {
			httpx.Error(w, http.StatusBadRequest, "请选择要删除的访客（一次最多 1000 个）")
			return
		}
		addrs := make([]netip.Addr, 0, len(req.IPs))
		for _, s := range req.IPs {
			a, err := netip.ParseAddr(strings.TrimSpace(s))
			if err != nil {
				httpx.Error(w, http.StatusBadRequest, "IP 地址无效")
				return
			}
			addrs = append(addrs, a)
		}
		n = m.visits.Delete(addrs)
	}
	// 删除是审计数据的变化，立即写盘，不等延迟写盘
	if err := m.visits.Flush(); err != nil {
		m.logs.Logger("").Warnf("访客审计数据写盘失败: %v", err)
	}
	httpx.OK(w, map[string]int{"deleted": n})
}

func (m *Module) handleVisitorSettings(w http.ResponseWriter, _ *http.Request) {
	httpx.OK(w, m.visits.Info())
}

func (m *Module) handlePutVisitorSettings(w http.ResponseWriter, r *http.Request) {
	var set visitors.Settings
	if err := httpx.ReadJSON(r, &set); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if err := set.Normalize(); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	m.mu.Lock()
	old := m.cfg.Visitors
	m.cfg.Visitors = set
	if err := m.saveLocked(); err != nil {
		m.cfg.Visitors = old
		m.mu.Unlock()
		httpx.Error(w, http.StatusInternalServerError, "%v", err)
		return
	}
	m.mu.Unlock()
	m.visits.Apply(set)
	if err := m.visits.Flush(); err != nil {
		m.logs.Logger("").Warnf("访客审计数据写盘失败: %v", err)
	}
	httpx.OK(w, m.visits.Info())
}

// recordsVisits 判断访客审计是否在记录。
func (a *authenticator) recordsVisits() bool {
	return a != nil && a.visits.Active()
}

// recordVisit 在请求结束时上报访客审计。admin 为管理后台登录用户名（被拦截的请求为空）；
// 未匹配域名而由默认站点处理的请求记为未匹配站点。只记录路径不记录查询串。
func (h *siteHandler) recordVisit(r *http.Request, client netip.Addr, rec *statusRecorder, origPath, admin string) {
	if !h.auth.recordsVisits() {
		return
	}
	hit := visitors.Hit{Addr: client, Listener: h.listener, Host: hostOnly(r.Host), Method: r.Method, Path: origPath,
		Status: rec.status, Deny: rec.deny, Bytes: rec.bytes, Admin: admin, UA: r.UserAgent()}
	if h.deflt {
		hit.Unmatched = true
	} else {
		hit.Site, hit.SiteName = h.site.Key, siteLabel(h.site)
	}
	if rec.req != nil {
		if u := rec.req.user.Load(); u != nil {
			hit.User = *u
		}
	}
	h.auth.visits.Record(hit)
}
