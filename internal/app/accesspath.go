package app

// 访问路径（配置变更防护的第二层）：识别当前请求经过了哪些 Gately 转发（端口转发、协议分流、Web 服务），
// 保存前判断修改是否会影响这条路径，管理界面据此在保存前明确提醒。
//
// 四层转发连到本机目标时按连接登记（hops.Track），Web 服务反代到管理界面时在请求上附带签名的路径头
// （hops.Encode），这里从管理界面收到的连接出发逐跳向外追溯。名称从各模块的配置文件读取，
// 应用本身不依赖这些模块的代码。

import (
	"encoding/json"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/hops"
	"github.com/wangwanjie/gately/internal/httpx"
)

// 访问路径上一跳的类型
const (
	hopClient  = "client"  // 访客
	hopLocal   = "local"   // 本机：本机的浏览器，或 frp、Cloudflare 隧道、其他反向代理等程序
	hopGateway = "gateway" // 飞牛统一网关
	hopAdmin   = "admin"   // 管理界面
)

// maxPathHops 限制追溯的跳数，防止异常登记形成环
const maxPathHops = 8

// pathHop 是访问路径上的一跳。Kind 为 client、local、gateway、admin 或模块 ID（portforward、protoroute、webservice）。
type pathHop struct {
	Kind string `json:"kind"`
	// Rule / Name 为规则或监听的 key 与名称
	Rule string `json:"rule,omitempty"`
	Name string `json:"name,omitempty"`
	// Route 为协议分流命中的路由名称（默认目标为空）
	Route string `json:"route,omitempty"`
	// Site / SiteName 为 Web 服务站点的 key 与名称（未命名时为第一个域名）
	Site     string `json:"site,omitempty"`
	SiteName string `json:"siteName,omitempty"`
	// Listen 为这一跳接入连接的本地地址
	Listen string `json:"listen,omitempty"`
	// Addr 为访客或本机程序的地址
	Addr string `json:"addr,omitempty"`
	// HTTPS 表示管理界面这一跳经 HTTPS 监听
	HTTPS bool `json:"https,omitempty"`
	// Missing 表示配置中已找不到这条规则（刚被删除或改了 key）
	Missing bool `json:"missing,omitempty"`
}

type accessPath struct {
	// Hops 由外向内：访客 → 转发 → 管理界面
	Hops []pathHop `json:"hops"`
}

// viaModule 返回路径上属于某模块的跳的序号。
func (p accessPath) viaModule(mod string) []int {
	var out []int
	for i, h := range p.Hops {
		if h.Kind == mod {
			out = append(out, i)
		}
	}
	return out
}

func (a *App) loadPathConfig(mod string) map[string]json.RawMessage {
	data, err := a.store.ReadRaw(mod)
	if err != nil || data == nil {
		return nil
	}
	var cfg struct {
		Rules     []json.RawMessage `json:"rules"`
		Listeners []json.RawMessage `json:"listeners"`
	}
	if json.Unmarshal(data, &cfg) != nil {
		return nil
	}
	out := map[string]json.RawMessage{}
	for _, raw := range append(cfg.Rules, cfg.Listeners...) {
		var k struct {
			Key string `json:"key"`
		}
		if json.Unmarshal(raw, &k) == nil && k.Key != "" {
			out[k.Key] = raw
		}
	}
	return out
}

// moduleSettings 读取模块配置中的 settings 对象。
func (a *App) moduleSettings(mod string) map[string]any {
	data, err := a.store.ReadRaw(mod)
	if err != nil || data == nil {
		return nil
	}
	var cfg struct {
		Settings map[string]any `json:"settings"`
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg.Settings
}

type namedItem struct {
	Key     string   `json:"key"`
	Name    string   `json:"name"`
	Domains []string `json:"domains"`
}

type namedListener struct {
	namedItem
	Sites   []namedItem `json:"sites"`
	Default namedItem   `json:"default"`
}

// fillNames 按配置补上规则、监听与站点的名称。
func (a *App) fillNames(p *accessPath) {
	cache := map[string]map[string]json.RawMessage{}
	for i := range p.Hops {
		h := &p.Hops[i]
		switch h.Kind {
		case "portforward", "protoroute", "webservice":
		default:
			continue
		}
		if cache[h.Kind] == nil {
			cache[h.Kind] = a.loadPathConfig(h.Kind)
		}
		raw, ok := cache[h.Kind][h.Rule]
		if !ok {
			h.Missing = true
			continue
		}
		var l namedListener
		_ = json.Unmarshal(raw, &l)
		h.Name = l.Name
		if h.Kind != "webservice" {
			continue
		}
		site := l.Default
		for _, s := range l.Sites {
			if s.Key == h.Site {
				site = s
				break
			}
		}
		h.SiteName = site.Name
		if h.SiteName == "" && len(site.Domains) > 0 {
			h.SiteName = site.Domains[0]
		}
	}
}

// isLocalHost 判断地址是否为本机（回环地址或本机网卡地址）。
func isLocalHost(ip netip.Addr) bool {
	ip = ip.Unmap()
	if ip.IsLoopback() {
		return true
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok {
			if x, ok := netip.AddrFromSlice(n.IP); ok && x.Unmap() == ip {
				return true
			}
		}
	}
	return false
}

// traceAccessPath 追溯请求的访问路径。
func (a *App) traceAccessPath(r *http.Request) accessPath {
	admin := pathHop{Kind: hopAdmin, HTTPS: r.TLS != nil}
	la, _ := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if la != nil && la.Network() != "unix" {
		admin.Listen = hops.Normalize(la.String())
	}
	// 由内向外收集，最后反转
	inner := []pathHop{admin}
	if la != nil && la.Network() == "unix" {
		// 经飞牛统一网关的 Unix Socket 进入：网关之外的路径无法得知
		inner = append(inner, pathHop{Kind: hopGateway}, pathHop{Kind: hopClient, Addr: r.RemoteAddr})
		return reversePath(inner)
	}
	peer := hops.Normalize(r.RemoteAddr)
	client := ""
	if hh, ok := hops.Decode(r.Header.Get(hops.Header)); ok && hh.Module == "webservice" {
		// 经本实例多层站点（443 站点 → 本机 444 监听器 → 管理界面）时由内向外逐层加入，从最外一层的对端继续追溯
		var h hops.Hop
		for _, h = range hh.Chain() {
			inner = append(inner, pathHop{Kind: "webservice", Rule: h.Rule, Site: h.Site, Listen: h.Listen})
		}
		peer = h.Peer
		// Web 服务识别的访客地址只在它不同于 TCP 对端时才有额外信息（取自可信代理的请求头或 PROXY 协议），
		// 否则就是前一跳转发拨出的地址
		if h.Client != "" && hops.Normalize(h.Client) != h.Peer {
			if ap, err := netip.ParseAddrPort(h.Client); err == nil && !isLocalHost(ap.Addr()) {
				client = h.Client
			}
		}
	}
	for i := 0; i < maxPathHops && peer != ""; i++ {
		h, ok := hops.Lookup(peer)
		if !ok {
			break
		}
		inner = append(inner, pathHop{Kind: h.Module, Rule: h.Rule, Route: h.Route, Listen: h.Listen})
		peer = h.Peer
	}
	if ap, err := netip.ParseAddrPort(peer); err == nil && isLocalHost(ap.Addr()) {
		inner = append(inner, pathHop{Kind: hopLocal, Addr: peer})
		// 本机程序（如 frp、隧道）之前的访客只能取 Web 服务按可信代理识别出的地址
		if client != "" && client != peer {
			inner = append(inner, pathHop{Kind: hopClient, Addr: client})
		}
	} else if peer != "" {
		inner = append(inner, pathHop{Kind: hopClient, Addr: peer})
	} else if client != "" {
		inner = append(inner, pathHop{Kind: hopClient, Addr: client})
	}
	p := reversePath(inner)
	a.fillNames(&p)
	return p
}

func reversePath(inner []pathHop) accessPath {
	slices.Reverse(inner)
	return accessPath{Hops: inner}
}

func (a *App) handleAccessPath(w http.ResponseWriter, r *http.Request) {
	httpx.OK(w, a.traceAccessPath(r))
}

// ---- 保存前的风险判断 ----

// 风险类型
const (
	riskEdit        = "edit"        // 修改路径上规则、监听或站点的转发相关设置
	riskDelete      = "delete"      // 删除路径上的规则、监听或站点
	riskDisable     = "disable"     // 停用路径上的规则、监听或站点
	riskModuleOff   = "moduleOff"   // 关闭路径经过的整个模块
	riskSiteMove    = "siteMove"    // 把路径上的站点移到其他监听
	riskAdminListen = "adminListen" // 修改当前使用的管理界面监听地址
	riskLANOnly     = "lanOnly"     // 开启仅限内网访问，而当前来自外网
	riskForceHTTPS  = "forceHTTPS"  // 开启强制 HTTPS，而当前经 HTTP 访问
	// riskListenUnavailable 为路径上的规则或监听改用的监听地址在本机不可用（例如 macOS 上未配置的 127.0.0.2），生效后会启动失败
	riskListenUnavailable = "listenUnavailable"
)

type pathRisk struct {
	Kind string `json:"kind"`
	// Hop 为受影响的跳在路径中的序号，-1 表示不针对某一跳
	Hop int `json:"hop"`
	// Module 为 moduleOff 时被关闭的模块
	Module string `json:"module,omitempty"`
	// Fields 为 edit 时改动的设置项（JSON 字段名），站点的字段带 site. 前缀
	Fields []string `json:"fields,omitempty"`
	// Addr / Error 为 listenUnavailable 时不可用的监听地址与系统给出的原因
	Addr  string `json:"addr,omitempty"`
	Error string `json:"error,omitempty"`
}

type accessCheckRequest struct {
	Method string          `json:"method"`
	Path   string          `json:"path"`
	Body   json.RawMessage `json:"body"`
}

type accessCheckResult struct {
	Path  accessPath `json:"path"`
	Risks []pathRisk `json:"risks"`
}

func (a *App) handleAccessCheck(w http.ResponseWriter, r *http.Request) {
	var req accessCheckRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	p := a.traceAccessPath(r)
	path, _, _ := strings.Cut(req.Path, "?")
	risks := a.accessRisks(r, p, strings.ToUpper(req.Method), path, req.Body)
	if risks == nil {
		risks = []pathRisk{}
	}
	httpx.OK(w, accessCheckResult{Path: p, Risks: risks})
}

// cosmeticFields 为只影响展示的字段，修改它们不影响访问。
var cosmeticFields = map[string]bool{"name": true, "group": true, "remark": true, "note": true, "description": true}

// accessRisks 判断一次写请求是否影响访问路径。
func (a *App) accessRisks(r *http.Request, p accessPath, method, path string, body json.RawMessage) []pathRisk {
	if path == "/api/system/settings" && method == http.MethodPut {
		return a.settingsRisks(r, p, body)
	}
	rest, ok := strings.CutPrefix(path, "/api/")
	if !ok {
		return nil
	}
	mod, tail, _ := strings.Cut(rest, "/")
	idx := p.viaModule(mod)
	if len(idx) == 0 {
		return nil
	}
	var risks []pathRisk
	hopOf := func(key string) int {
		for _, i := range idx {
			if p.Hops[i].Rule == key {
				return i
			}
		}
		return -1
	}
	parts := strings.Split(tail, "/")
	switch {
	case tail == "settings" && method == http.MethodPut:
		// 模块总开关：关闭后路径上的规则全部停止
		var s struct {
			Enable *bool `json:"enable"`
		}
		if json.Unmarshal(body, &s) == nil && s.Enable != nil && !*s.Enable {
			if cur := a.moduleSettings(mod); cur != nil && cur["enable"] != false {
				risks = append(risks, pathRisk{Kind: riskModuleOff, Hop: idx[0], Module: mod})
			}
		}
	case mod == "webservice" && tail == "sites/batch" && method == http.MethodPost:
		risks = append(risks, siteBatchRisks(p, idx, body)...)
	case len(parts) == 2 && (parts[0] == "rules" || parts[0] == "listeners"):
		i := hopOf(parts[1])
		if i < 0 {
			break
		}
		switch method {
		case http.MethodDelete:
			risks = append(risks, pathRisk{Kind: riskDelete, Hop: i})
		case http.MethodPut:
			risks = append(risks, a.itemEditRisks(mod, p.Hops[i], i, body)...)
		}
	case len(parts) == 3 && parts[2] == "enable" && method == http.MethodPut:
		if i := hopOf(parts[1]); i >= 0 && enableFalse(body) {
			risks = append(risks, pathRisk{Kind: riskDisable, Hop: i})
		}
	case len(parts) == 1 && (parts[0] == "rules-enable" || parts[0] == "listeners-enable") && method == http.MethodPut:
		var b struct {
			Keys   []string `json:"keys"`
			Enable bool     `json:"enable"`
		}
		if json.Unmarshal(body, &b) == nil && !b.Enable {
			for _, k := range b.Keys {
				if i := hopOf(k); i >= 0 {
					risks = append(risks, pathRisk{Kind: riskDisable, Hop: i})
				}
			}
		}
	case len(parts) == 1 && (parts[0] == "rules-delete" || parts[0] == "listeners-delete") && method == http.MethodPost:
		var b struct {
			Keys []string `json:"keys"`
		}
		if json.Unmarshal(body, &b) == nil {
			for _, k := range b.Keys {
				if i := hopOf(k); i >= 0 {
					risks = append(risks, pathRisk{Kind: riskDelete, Hop: i})
				}
			}
		}
	}
	return risks
}

func enableFalse(body json.RawMessage) bool {
	var b struct {
		Enable *bool `json:"enable"`
	}
	return json.Unmarshal(body, &b) == nil && b.Enable != nil && !*b.Enable
}

// itemEditRisks 比较提交的规则或监听与已保存的内容：停用、删除路径上的站点或改动转发相关字段（不含名称、分组、备注）。
func (a *App) itemEditRisks(mod string, hop pathHop, i int, body json.RawMessage) []pathRisk {
	stored := a.loadPathConfig(mod)[hop.Rule]
	if stored == nil {
		return nil
	}
	var next, cur map[string]any
	if json.Unmarshal(body, &next) != nil || json.Unmarshal(stored, &cur) != nil {
		return nil
	}
	if v, ok := next["enable"].(bool); ok && !v && cur["enable"] == true {
		return []pathRisk{{Kind: riskDisable, Hop: i}}
	}
	skip := map[string]bool{"enable": true}
	if mod == "webservice" {
		// 站点单独比较：只关心路径上的那个站点
		skip["sites"], skip["default"] = true, true
	}
	fields := changedFields(next, cur, skip, "")
	if mod == "webservice" {
		ns, nok := findSite(next, hop.Site)
		cs, cok := findSite(cur, hop.Site)
		switch {
		case cok && !nok:
			return []pathRisk{{Kind: riskDelete, Hop: i}}
		case cok && nok:
			if v, ok := ns["enable"].(bool); ok && !v && cs["enable"] == true {
				return []pathRisk{{Kind: riskDisable, Hop: i}}
			}
			fields = append(fields, changedFields(ns, cs, map[string]bool{"enable": true}, "site.")...)
		}
	}
	if len(fields) == 0 {
		return nil
	}
	risks := []pathRisk{{Kind: riskEdit, Hop: i, Fields: fields}}
	// 改了监听地址：先试一下新地址能否在本机使用，不能用时说明原因，免得生效后才发现监听启动失败
	for _, f := range listenFields {
		if !slices.Contains(fields, f) {
			continue
		}
		if ip, _ := next[f].(string); ip != "" {
			if err := listenProblem(ip); err != nil {
				risks = append(risks, pathRisk{Kind: riskListenUnavailable, Hop: i, Addr: ip, Error: err.Error()})
			}
		}
	}
	return risks
}

// listenFields 为规则与监听中表示监听地址的字段：端口转发、协议分流为 listenAddress，Web 服务为 listenIp
var listenFields = []string{"listenAddress", "listenIp"}

// listenProblem 检查监听地址能否在本机使用：在该地址的随机端口上试着监听一下，不与正在使用的端口冲突。
// 空地址与 0.0.0.0、:: 等通配地址总能使用。地址不是本机地址时（例如 macOS 默认只配置了 127.0.0.1，
// 127.0.0.2 需要先 ifconfig lo0 alias）返回系统的错误。
func listenProblem(ip string) error {
	ip = strings.Trim(strings.TrimSpace(ip), "[]")
	if ip == "" {
		return nil
	}
	a, err := netip.ParseAddr(ip)
	if err != nil {
		// 不是 IP（例如格式错误）交给模块自己的校验
		return nil
	}
	if a.IsUnspecified() {
		return nil
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(a.String(), "0"))
	if err != nil {
		return err
	}
	_ = ln.Close()
	return nil
}

// findSite 在监听中按 key 找站点（默认站点的 key 可能为空或不在站点列表中）。
func findSite(listener map[string]any, key string) (map[string]any, bool) {
	if sites, ok := listener["sites"].([]any); ok {
		for _, s := range sites {
			if m, ok := s.(map[string]any); ok && m["key"] == key && key != "" {
				return m, true
			}
		}
	}
	if d, ok := listener["default"].(map[string]any); ok {
		if dk, _ := d["key"].(string); dk == key || key == "" {
			return d, true
		}
	}
	return nil, false
}

// changedFields 返回两边都有、且内容不同的顶层字段（跳过 skip 与只影响展示的字段）。
// 只在一边出现的字段不算：提交的内容可能带着运行状态等只读字段，也可能省略了没有变化的字段。
func changedFields(next, cur map[string]any, skip map[string]bool, prefix string) []string {
	var out []string
	for k, nv := range next {
		if skip[k] || cosmeticFields[k] {
			continue
		}
		cv, ok := cur[k]
		if !ok {
			continue
		}
		if !looseEqual(nv, cv) {
			out = append(out, prefix+k)
		}
	}
	slices.Sort(out)
	return out
}

// looseEqual 比较两个 JSON 值：null 与空数组、空对象、空字符串视为相同；提交的掩码（密码显示为 ******）视为未改；
// 嵌套对象中只影响展示的字段（如路由名称）不参与比较。
func looseEqual(n, c any) bool {
	if isEmptyJSON(n) && isEmptyJSON(c) {
		return true
	}
	switch nv := n.(type) {
	case string:
		if nv == crud.SecretMask {
			return true
		}
		cv, ok := c.(string)
		return ok && nv == cv
	case []any:
		cv, ok := c.([]any)
		if !ok || len(nv) != len(cv) {
			return false
		}
		for i := range nv {
			if !looseEqual(nv[i], cv[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		cv, ok := c.(map[string]any)
		if !ok {
			return false
		}
		return len(changedFields(nv, cv, nil, "")) == 0 && len(changedFields(cv, nv, nil, "")) == 0
	}
	return n == c
}

func isEmptyJSON(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case []any:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	case string:
		return x == ""
	}
	return false
}

// siteBatchRisks 判断 Web 服务站点批量操作是否涉及路径上的站点。
func siteBatchRisks(p accessPath, idx []int, body json.RawMessage) []pathRisk {
	var b struct {
		Sites []struct {
			Listener string `json:"listener"`
			Site     string `json:"site"`
		} `json:"sites"`
		Action string `json:"action"`
	}
	if json.Unmarshal(body, &b) != nil {
		return nil
	}
	kind := ""
	switch b.Action {
	case "disable":
		kind = riskDisable
	case "delete":
		kind = riskDelete
	case "move":
		kind = riskSiteMove
	case "removeDomains", "replaceDomain":
		kind = riskEdit
	default:
		return nil
	}
	var risks []pathRisk
	for _, s := range b.Sites {
		for _, i := range idx {
			if p.Hops[i].Rule == s.Listener && p.Hops[i].Site == s.Site {
				r := pathRisk{Kind: kind, Hop: i}
				if kind == riskEdit {
					r.Fields = []string{"site.domains"}
				}
				risks = append(risks, r)
			}
		}
	}
	return risks
}

// settingsRisks 判断全局设置的修改是否影响当前访问：关闭路径经过的模块、修改正在使用的管理界面监听、
// 外网访问时开启仅限内网、经 HTTP 访问时开启强制 HTTPS。
func (a *App) settingsRisks(r *http.Request, p accessPath, body json.RawMessage) []pathRisk {
	var next Settings
	if json.Unmarshal(body, &next) != nil {
		return nil
	}
	next.normalize()
	cur := a.settingsSnapshot()
	// 命令行 -listen 覆盖时比较配置文件中的地址
	curListen := cur.Listen
	if a.opts.Listen != "" {
		curListen = a.fileListen
	}
	var risks []pathRisk
	for _, mod := range next.DisabledModules {
		if slices.Contains(cur.DisabledModules, mod) {
			continue
		}
		if idx := p.viaModule(mod); len(idx) > 0 {
			risks = append(risks, pathRisk{Kind: riskModuleOff, Hop: idx[0], Module: mod})
		}
	}
	adminIdx := len(p.Hops) - 1
	if adminIdx >= 0 && p.Hops[adminIdx].Kind == hopAdmin && p.Hops[adminIdx].Listen != "" {
		admin := p.Hops[adminIdx]
		if (!admin.HTTPS && next.Listen != curListen) || (admin.HTTPS && next.HTTPSListen != cur.HTTPSListen) {
			risks = append(risks, pathRisk{Kind: riskAdminListen, Hop: adminIdx})
		}
	}
	if next.LANOnly && !cur.LANOnly && !isPrivate(httpx.ClientIP(r)) {
		risks = append(risks, pathRisk{Kind: riskLANOnly, Hop: -1})
	}
	if next.ForceHTTPS && !cur.ForceHTTPS && r.TLS == nil && a.httpsReady.Load() && a.hasCert(r.Host) {
		risks = append(risks, pathRisk{Kind: riskForceHTTPS, Hop: -1})
	}
	return risks
}
