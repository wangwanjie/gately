package webservice

import (
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
}

func (o *SiteBatchOptions) empty() bool {
	return o == nil || (o.Compress == nil && o.AccessLog == nil && o.CORS == nil && o.ForwardedHeaders == nil && o.DenyClose == nil && o.WAF == nil)
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
}

// SiteBatchResult 为批量操作的结果：Changed 为实际发生变化的站点数，Unchanged 为已经符合要求而无需修改的站点数。
type SiteBatchResult struct {
	Changed   int `json:"changed"`
	Unchanged int `json:"unchanged"`
}

func (m *Module) registerBatch(r module.Router) {
	r.HandleFunc("POST /sites/batch", m.handleSiteBatch)
	module.DocOf(r, "POST /sites/batch", module.Doc{Summary: "批量操作站点：启停、删除、增删或替换域名、修改部分选项；全部校验通过才保存，任一站点失败都不修改", Req: &SiteBatchRequest{}, Resp: &SiteBatchResult{}})
}

// normalizeBatchDomains 规范化并去重批量提交的域名，写法错误时整体报错。
func normalizeBatchDomains(in []string) ([]string, error) {
	var out []string
	for _, raw := range in {
		d, ok := hostname.Normalize(raw, true)
		if !ok {
			return nil, fmt.Errorf("域名 %q 格式错误", strings.TrimSpace(raw))
		}
		if d != "" && !slices.Contains(out, d) {
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
	default:
		httpx.Error(w, http.StatusBadRequest, "未知的批量操作 %q", req.Action)
		return
	}
	refs := make([]SiteRef, 0, len(req.Sites))
	for _, ref := range req.Sites {
		if !slices.Contains(refs, ref) {
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

	m.mu.Lock()
	defer m.mu.Unlock()
	// 修改先作用在监听器副本上，全部校验通过后再保存，任一失败都不做任何修改
	oldListeners := m.cfg.Listeners
	listeners := slices.Clone(oldListeners)
	touched := map[string]bool{}
	cloned := map[string]bool{}
	// drop 记录各监听器要删除的站点 key
	drop := map[string]map[string]bool{}
	// introduced 记录各监听器中本次新加到站点上的域名，用于检查与同一监听器其他站点冲突
	introduced := map[string]map[string]bool{}
	res := SiteBatchResult{}
	for _, ref := range refs {
		i := slices.IndexFunc(listeners, func(l Listener) bool { return l.Key == ref.Listener })
		if i < 0 {
			httpx.Error(w, http.StatusNotFound, "监听器 %s 不存在", ref.Listener)
			return
		}
		l := &listeners[i]
		if !cloned[l.Key] {
			// 首次修改该监听器时复制站点列表，避免改到当前配置
			l.Sites = slices.Clone(l.Sites)
			cloned[l.Key] = true
		}
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
			if opts.WAF != nil {
				v := strings.TrimSpace(*opts.WAF)
				opts.WAF = &v
			}
			changed = applyBatchOptions(site, &opts)
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
		if err := m.checkIPSetsLocked(l); err != nil {
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
	httpx.OK(w, res)
}
