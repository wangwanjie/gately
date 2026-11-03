package webservice

import (
	"cmp"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/hostname"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/module"
)

// 批量操作站点的动作。
const (
	BatchEnable        = "enable"
	BatchDisable       = "disable"
	BatchDelete        = "delete"
	BatchAddDomains    = "addDomains"
	BatchRemoveDomains = "removeDomains"
	// BatchReplaceDomain 把域名中的后缀 From 换成 To（From 本身及其子域名，通配写法随之替换），用于整体更换主域名
	BatchReplaceDomain = "replaceDomain"
	BatchSet           = "set"
	// BatchMove 把站点移到另一个监听器
	BatchMove = "move"
)

// maxBatchDomains 为一次批量增删域名最多提交的域名数。
const maxBatchDomains = 100

// SiteRef 指向某个监听器下的站点，Site 为 default 时指默认站点。
type SiteRef struct {
	Listener string `json:"listener"`
	Site     string `json:"site"`
}

// SiteBatchOptions 是批量修改选项时允许改动的设置，只改提供了的字段。
// 仅开放不改变访问权限、不涉及后端地址与凭据的开关；认证、访问控制、后端地址等需在站点中逐个修改。
type SiteBatchOptions struct {
	Compress         *bool `json:"compress,omitempty"`
	AccessLog        *bool `json:"accessLog,omitempty"`
	CORS             *bool `json:"cors,omitempty"`
	ForwardedHeaders *bool `json:"forwardedHeaders,omitempty"`
	DenyClose        *bool `json:"denyClose,omitempty"`
	// WAF 为 WAF 实例 key，空字符串表示取消引用
	WAF *string `json:"waf,omitempty"`
	// ErrorPage 为错误页样式（style 与 page），站点各自的拦截后端错误设置保持不变
	ErrorPage *ErrorPageStyle `json:"errorPage,omitempty"`
}

func (o *SiteBatchOptions) empty() bool {
	return o == nil || (o.Compress == nil && o.AccessLog == nil && o.CORS == nil && o.ForwardedHeaders == nil && o.DenyClose == nil && o.WAF == nil && o.ErrorPage == nil)
}

// SiteBatchRequest 为 POST /sites/batch 的请求体。
type SiteBatchRequest struct {
	Sites  []SiteRef `json:"sites"`
	Action string    `json:"action"`
	// Domains 用于 addDomains 与 removeDomains
	Domains []string `json:"domains,omitempty"`
	// From 与 To 用于 replaceDomain
	From    string            `json:"from,omitempty"`
	To      string            `json:"to,omitempty"`
	Options *SiteBatchOptions `json:"options,omitempty"`
	// Move 用于 move：目标监听器与放置位置
	Move *SiteMoveTarget `json:"move,omitempty"`
}

// SiteMoveTarget 为移动站点的目标。站点按提交顺序连续放置；已在目标监听器中的站点保持不动。
type SiteMoveTarget struct {
	Listener string `json:"listener"`
	// Group 非 nil 时把站点放入该分组（空字符串为未分组）并排在该分组末尾；nil 时保留各站点原有分组，放在目标监听器末尾
	Group *string `json:"group,omitempty"`
	// Before 与 After 为目标监听器中的站点 key，把站点插到它之前或之后并加入它的分组（优先于 Group），二者最多填一个
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}

// SiteMoved 记录一个站点移动前后的位置；目标监听器中已有相同 key 时站点会换用新 key。
type SiteMoved struct {
	From SiteRef `json:"from"`
	To   SiteRef `json:"to"`
}

// SiteBatchResult 为批量操作的结果：Changed 为实际发生变化的站点数，Unchanged 为已经符合要求而无需修改的站点数。
type SiteBatchResult struct {
	Changed   int `json:"changed"`
	Unchanged int `json:"unchanged"`
	// Moved 为 move 动作中实际移动的站点
	Moved []SiteMoved `json:"moved,omitempty"`
}

func (m *Module) registerBatch(r module.Router) {
	r.HandleFunc("POST /sites/batch", m.handleSiteBatch)
	module.DocOf(r, "POST /sites/batch", module.Doc{Summary: "批量操作站点：启停、删除、增删或替换域名、修改部分选项、移到其他监听器；全部校验通过才保存，任一站点失败都不修改", Req: &SiteBatchRequest{}, Resp: &SiteBatchResult{}})
}

// normalizeBatchDomains 规范化并去重批量提交的域名，写法错误时整体报错。
func normalizeBatchDomains(in []string) ([]string, error) {
	// 先按原始数量粗略限制（允许一定的重复），避免超大请求体逐个规范化占用 CPU
	if len(in) > maxBatchDomains*2 {
		return nil, fmt.Errorf("一次最多处理 %d 个域名", maxBatchDomains)
	}
	var out []string
	seen := map[string]bool{}
	for _, raw := range in {
		d, ok := hostname.Normalize(raw, true)
		if !ok {
			return nil, fmt.Errorf("域名 %q 格式错误", strings.TrimSpace(raw))
		}
		if d != "" && !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("请填写域名")
	}
	if len(out) > maxBatchDomains {
		return nil, fmt.Errorf("一次最多处理 %d 个域名", maxBatchDomains)
	}
	return out, nil
}

// replaceDomainSuffix 把域名 d 中的后缀 from 换成 to：d 等于 from，或以 .from 结尾时替换，其余原样返回。
// 通配域名 *.from 随之变成 *.to；单独的 * 不受影响。
func replaceDomainSuffix(d, from, to string) string {
	switch {
	case d == from:
		return to
	case strings.HasSuffix(d, "."+from):
		return d[:len(d)-len(from)] + to
	}
	return d
}

// applyDomainAction 对站点的域名做增、删、替换，返回新的域名列表（不修改原切片）。
func applyDomainAction(domains []string, action string, list []string, from, to string) []string {
	out := slices.Clone(domains)
	switch action {
	case BatchAddDomains:
		for _, d := range list {
			if !slices.Contains(out, d) {
				out = append(out, d)
			}
		}
	case BatchRemoveDomains:
		out = slices.DeleteFunc(out, func(d string) bool { return slices.Contains(list, d) })
	case BatchReplaceDomain:
		var next []string
		for _, d := range out {
			if d = replaceDomainSuffix(d, from, to); !slices.Contains(next, d) {
				next = append(next, d)
			}
		}
		out = next
	}
	return out
}

// applyBatchOptions 把选项写入站点，返回是否有变化。
func applyBatchOptions(s *Site, o *SiteBatchOptions) bool {
	changed := false
	set := func(dst *bool, v *bool) {
		if v != nil && *dst != *v {
			*dst = *v
			changed = true
		}
	}
	set(&s.Compress, o.Compress)
	set(&s.AccessLog, o.AccessLog)
	set(&s.CORS, o.CORS)
	set(&s.ForwardedHeaders, o.ForwardedHeaders)
	set(&s.DenyClose, o.DenyClose)
	if o.WAF != nil && s.WAF != *o.WAF {
		s.WAF = *o.WAF
		changed = true
	}
	if o.ErrorPage != nil && (s.ErrorPage.Style != o.ErrorPage.Style || s.ErrorPage.Page != o.ErrorPage.Page) {
		s.ErrorPage.Style, s.ErrorPage.Page = o.ErrorPage.Style, o.ErrorPage.Page
		changed = true
	}
	return changed
}

// checkDomainConflicts 检查本次新加的域名是否出现在同一监听器的多个站点中：同一监听器内域名只会匹配到第一个站点，
// 批量把同一域名加到多个站点几乎总是误操作。只检查本次引入的域名，已有的重复不影响批量操作。
func checkDomainConflicts(l *Listener, added map[string]bool) error {
	if len(added) == 0 {
		return nil
	}
	owner := map[string]string{}
	for i := range l.Sites {
		s := &l.Sites[i]
		for _, d := range s.Domains {
			if !added[d] {
				continue
			}
			if prev, ok := owner[d]; ok {
				return fmt.Errorf("域名 %s 同时出现在站点 %s 与 %s 中，同一监听器内只有第一个站点生效", d, prev, listenerSiteLabel(l, s))
			}
			owner[d] = listenerSiteLabel(l, s)
		}
	}
	return nil
}

func (m *Module) handleSiteBatch(w http.ResponseWriter, r *http.Request) {
	var req SiteBatchRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	// 请求参数校验不依赖配置，先于加锁完成
	var domains []string
	var from, to string
	switch req.Action {
	case BatchEnable, BatchDisable, BatchDelete:
	case BatchAddDomains, BatchRemoveDomains:
		var err error
		if domains, err = normalizeBatchDomains(req.Domains); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
	case BatchReplaceDomain:
		var ok1, ok2 bool
		from, ok1 = hostname.Normalize(req.From, false)
		to, ok2 = hostname.Normalize(req.To, false)
		if !ok1 || !ok2 || from == "" || to == "" || strings.HasPrefix(from, "*") || strings.HasPrefix(to, "*") {
			httpx.Error(w, http.StatusBadRequest, "请填写要替换的域名与新域名（不含通配符）")
			return
		}
	case BatchSet:
		if req.Options.empty() {
			httpx.Error(w, http.StatusBadRequest, "请选择要修改的选项")
			return
		}
		if req.Options.WAF != nil && utf8.RuneCountInString(strings.TrimSpace(*req.Options.WAF)) > 64 {
			httpx.Error(w, http.StatusBadRequest, "WAF 实例标识过长")
			return
		}
		// WAF 中间件找不到实例时直接放行，写错的引用会让选中站点全部失去防护，批量设置时先确认实例存在
		if req.Options.WAF != nil {
			if key := strings.TrimSpace(*req.Options.WAF); key != "" {
				if p, ok := m.waf.(interface{ HasInstance(string) bool }); ok && !p.HasInstance(key) {
					httpx.Error(w, http.StatusBadRequest, "引用的 WAF 实例不存在")
					return
				}
			}
		}
		// 先规范化错误页设置，与站点现有设置比较时才能识别“无需修改”；引用的页面是否存在在保存前随监听一起检查
		if e := req.Options.ErrorPage; e != nil {
			o := ErrorPageOptions{Style: e.Style, Page: e.Page}
			if err := o.validate(); err != nil {
				httpx.Error(w, http.StatusBadRequest, "%v", err)
				return
			}
			e.Style, e.Page = o.Style, o.Page
		}
	case BatchMove:
		if req.Move == nil || req.Move.Listener == "" {
			httpx.Error(w, http.StatusBadRequest, "请选择目标监听")
			return
		}
		if req.Move.Before != "" && req.Move.After != "" {
			httpx.Error(w, http.StatusBadRequest, "插入位置只能指定一个")
			return
		}
		if req.Move.Group != nil {
			g := *req.Move.Group
			if err := crud.NormalizeGroup(&g); err != nil {
				httpx.Error(w, http.StatusBadRequest, "%v", err)
				return
			}
			req.Move.Group = &g
		}
	default:
		httpx.Error(w, http.StatusBadRequest, "未知的批量操作 %q", req.Action)
		return
	}
	// 先按原始数量粗略限制（允许一定的重复），再用 map 去重，避免超大请求体占用 CPU
	if len(req.Sites) > crud.MaxBatch*2 {
		httpx.Error(w, http.StatusBadRequest, "一次最多操作 %d 个站点", crud.MaxBatch)
		return
	}
	refs := make([]SiteRef, 0, len(req.Sites))
	seenRefs := map[SiteRef]bool{}
	for _, ref := range req.Sites {
		if !seenRefs[ref] {
			seenRefs[ref] = true
			refs = append(refs, ref)
		}
	}
	if len(refs) == 0 {
		httpx.Error(w, http.StatusBadRequest, "请选择要操作的站点")
		return
	}
	if len(refs) > crud.MaxBatch {
		httpx.Error(w, http.StatusBadRequest, "一次最多操作 %d 个站点", crud.MaxBatch)
		return
	}

	// 移动站点串行执行，回调结束后才允许下一次移动，保证各次移动的回调按先后顺序执行；
	// 先于下面的 defer 注册，在回调全部执行完之后才解锁
	if req.Action == BatchMove {
		m.moveMu.Lock()
		defer m.moveMu.Unlock()
	}
	// 移动站点后要通知引用站点标识的其他模块（性能监控告警规则）。回调在释放模块锁之后执行，
	// 避免与对方模块读取站点快照时的加锁顺序相反而死锁；这个 defer 先注册，晚于解锁执行
	var renamed map[string]string
	var hooks []func(map[string]string)
	defer func() {
		for _, fn := range hooks {
			fn(renamed)
		}
	}()
	m.mu.Lock()
	defer m.mu.Unlock()
	// 修改先作用在监听器副本上，全部校验通过后再保存，任一失败都不做任何修改
	oldListeners := m.cfg.Listeners
	listeners := slices.Clone(oldListeners)
	touched := map[string]bool{}
	cloned := map[string]bool{}
	// ensureCloned 在首次修改监听器时复制站点列表，避免改到当前配置；
	// 校验会原地规范化路径规则与按用户分流，这两个切片也要复制，校验失败时当前配置保持原样
	ensureCloned := func(l *Listener) {
		if cloned[l.Key] {
			return
		}
		l.Sites = slices.Clone(l.Sites)
		for k := range l.Sites {
			cloneSiteSlices(&l.Sites[k])
		}
		cloneSiteSlices(&l.Default)
		cloned[l.Key] = true
	}
	// drop 记录各监听器要删除的站点 key
	drop := map[string]map[string]bool{}
	// introduced 记录各监听器中本次新加到站点上的域名，用于检查与同一监听器其他站点冲突
	introduced := map[string]map[string]bool{}
	// moving 为要移到目标监听器的站点（按提交顺序），movedFrom 为它们原来所在的监听器
	var moving []Site
	var movedFrom []string
	target := -1
	if req.Action == BatchMove {
		if target = slices.IndexFunc(listeners, func(l Listener) bool { return l.Key == req.Move.Listener }); target < 0 {
			httpx.Error(w, http.StatusNotFound, "目标监听 %s 不存在", req.Move.Listener)
			return
		}
	}
	res := SiteBatchResult{}
	for _, ref := range refs {
		i := slices.IndexFunc(listeners, func(l Listener) bool { return l.Key == ref.Listener })
		if i < 0 {
			httpx.Error(w, http.StatusNotFound, "监听器 %s 不存在", ref.Listener)
			return
		}
		l := &listeners[i]
		ensureCloned(l)
		var site *Site
		isDefault := ref.Site == "default"
		if isDefault {
			site = &l.Default
		} else if j := slices.IndexFunc(l.Sites, func(s Site) bool { return s.Key == ref.Site }); j >= 0 {
			site = &l.Sites[j]
		} else {
			httpx.Error(w, http.StatusNotFound, "站点 %s 不存在", ref.Site)
			return
		}
		changed := false
		switch req.Action {
		case BatchEnable, BatchDisable:
			want := req.Action == BatchEnable
			if site.Enable != want {
				site.Enable = want
				changed = true
			}
		case BatchDelete:
			if isDefault {
				httpx.Error(w, http.StatusBadRequest, "默认站点无法删除，只能停用或恢复默认")
				return
			}
			if drop[l.Key] == nil {
				drop[l.Key] = map[string]bool{}
			}
			drop[l.Key][site.Key] = true
			changed = true
		case BatchAddDomains, BatchRemoveDomains, BatchReplaceDomain:
			if isDefault {
				httpx.Error(w, http.StatusBadRequest, "默认站点没有域名，不能批量修改域名")
				return
			}
			next := applyDomainAction(site.Domains, req.Action, domains, from, to)
			if !slices.Equal(next, site.Domains) {
				for _, d := range next {
					if !slices.Contains(site.Domains, d) {
						if introduced[l.Key] == nil {
							introduced[l.Key] = map[string]bool{}
						}
						introduced[l.Key][d] = true
					}
				}
				site.Domains = next
				changed = true
			}
		case BatchSet:
			opts := *req.Options
			// 断开连接与固定文本站点没有错误页，不设置样式（保存时也会清空）
			if !isDefault && (site.Type == TypeClose || site.Type == TypeText) {
				opts.ErrorPage = nil
			}
			if opts.WAF != nil {
				v := strings.TrimSpace(*opts.WAF)
				opts.WAF = &v
			}
			changed = applyBatchOptions(site, &opts)
		case BatchMove:
			if isDefault {
				httpx.Error(w, http.StatusBadRequest, "默认站点属于所在监听，不能移到其他监听")
				return
			}
			// 已在目标监听器中的站点不动（调整同一监听内的顺序走排序接口）
			if i != target {
				if drop[l.Key] == nil {
					drop[l.Key] = map[string]bool{}
				}
				drop[l.Key][site.Key] = true
				moving = append(moving, *site)
				movedFrom = append(movedFrom, l.Key)
				changed = true
			}
		}
		if changed {
			res.Changed++
			touched[l.Key] = true
		} else {
			res.Unchanged++
		}
	}
	if res.Changed == 0 {
		httpx.OK(w, res)
		return
	}
	if len(moving) > 0 {
		tl := &listeners[target]
		ensureCloned(tl)
		moved, err := placeMovedSites(tl, moving, req.Move)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		touched[tl.Key] = true
		// 移入的站点的全部域名都算新引入的，与目标监听器中已有站点重复时拒绝：同一监听内只有第一个站点生效
		added := map[string]bool{}
		for _, s := range moving {
			for _, d := range s.Domains {
				added[d] = true
			}
		}
		introduced[tl.Key] = added
		renamed = map[string]string{}
		for k, s := range moving {
			from := SiteRef{Listener: movedFrom[k], Site: s.Key}
			to := SiteRef{Listener: tl.Key, Site: moved[k]}
			res.Moved = append(res.Moved, SiteMoved{From: from, To: to})
			renamed[from.Listener+"/"+from.Site] = to.Listener + "/" + to.Site
		}
	}
	for i := range listeners {
		l := &listeners[i]
		if !touched[l.Key] {
			continue
		}
		if d := drop[l.Key]; len(d) > 0 {
			l.Sites = slices.DeleteFunc(l.Sites, func(s Site) bool { return d[s.Key] })
		}
		if err := l.Validate(); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		if err := m.checkRefsLocked(l); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		if err := checkDomainConflicts(l, introduced[l.Key]); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
	}
	m.cfg.Listeners = listeners
	if err := m.saveLocked(); err != nil {
		m.cfg.Listeners = oldListeners
		httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
		return
	}
	// 每个监听器只重载一次；只改站点时热替换站点表，不断开现有连接
	for _, l := range m.cfg.Listeners {
		if touched[l.Key] {
			m.restartLocked(l.Key)
		}
	}
	if len(renamed) > 0 {
		// 站点的会话作用域随“监听/站点”变化，已登录的访客需要重新登录；
		// 分享链接的已兑换次数随站点迁移，避免限次链接因移动而重新可用
		m.auth.moveShareUses(renamed)
		hooks = slices.Clone(m.sitesMoved)
	}
	httpx.OK(w, res)
}

// placeMovedSites 把移入的站点放进目标监听器 l，返回各站点在目标监听器中的 key（与 moving 一一对应）。
// 目标中已有相同 key 时换用随机新 key：自动编号的 key 容易在之后新建站点时被复用，随机 key 不会。
func placeMovedSites(l *Listener, moving []Site, to *SiteMoveTarget) ([]string, error) {
	used := map[string]bool{}
	for _, s := range l.Sites {
		used[s.Key] = true
	}
	keys := make([]string, len(moving))
	in := make([]Site, len(moving))
	for k, s := range moving {
		for s.Key == "" || s.Key == "default" || used[s.Key] {
			s.Key = crud.NewKey()
		}
		used[s.Key] = true
		keys[k] = s.Key
		in[k] = s
	}
	// 确定插入位置与分组：指定了相邻站点时插到它前后并加入它的分组；只指定分组时排在该分组末尾
	at := len(l.Sites)
	group := to.Group
	if anchor := cmp.Or(to.Before, to.After); anchor != "" {
		j := slices.IndexFunc(l.Sites, func(s Site) bool { return s.Key == anchor })
		if j < 0 {
			return nil, fmt.Errorf("目标监听中的站点 %s 不存在", anchor)
		}
		at = j
		if to.After != "" {
			at = j + 1
		}
		g := l.Sites[j].Group
		group = &g
	} else if group != nil {
		if last := lastIndexOfGroup(l.Sites, *group); last >= 0 {
			at = last + 1
		} else if *group != "" {
			// 目标中还没有这个分组时放在未分组站点之前，与管理界面拖入新分组的显示顺序一致
			if first := slices.IndexFunc(l.Sites, func(s Site) bool { return s.Group == "" }); first >= 0 {
				at = first
			}
		}
	}
	if group != nil {
		for k := range in {
			in[k].Group = *group
		}
	}
	l.Sites = slices.Insert(l.Sites, at, in...)
	return keys, nil
}

// lastIndexOfGroup 返回分组中最后一个站点的下标，没有时返回 -1。
func lastIndexOfGroup(sites []Site, group string) int {
	for i := len(sites) - 1; i >= 0; i-- {
		if sites[i].Group == group {
			return i
		}
	}
	return -1
}

// cloneSiteSlices 复制站点中会被校验原地改写的切片。
func cloneSiteSlices(s *Site) {
	s.Locations = slices.Clone(s.Locations)
	s.UserRoutes = slices.Clone(s.UserRoutes)
}
