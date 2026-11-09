package webservice

import (
	"net/http"
	"strings"

	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/module"
)

// CacheStatsView 是站点响应缓存的当前状态。
type CacheStatsView struct {
	// Enabled 为站点当前运行中的配置是否启用了缓存
	Enabled bool `json:"enabled"`
	// Entries 与 Bytes 为当前缓存的条目数与占用字节数，MaxBytes 为容量上限
	Entries  int   `json:"entries"`
	Bytes    int64 `json:"bytes"`
	MaxBytes int64 `json:"maxBytes"`
}

// cachePurgeRequest 是清空站点缓存的请求。
type cachePurgeRequest struct {
	Listener string `json:"listener"`
	Site     string `json:"site"`
	// Path 为要清空的路径，写法同缓存规则路径；留空清空整个站点的缓存
	Path string `json:"path"`
}

// cachePurgeResult 是清空缓存的结果，附带清空后的缓存状态。
type cachePurgeResult struct {
	Removed int `json:"removed"`
	CacheStatsView
}

// registerCache 注册站点缓存的查看与清空接口。
func (m *Module) registerCache(r module.Router) {
	r.HandleFunc("GET /cache", m.handleCacheStats)
	r.HandleFunc("POST /cache/purge", m.handleCachePurge)
	module.DocOf(r, "GET /cache", module.Doc{Summary: "站点响应缓存的条目数与占用（?listener=&site=）", Resp: &CacheStatsView{}})
	module.DocOf(r, "POST /cache/purge", module.Doc{Summary: "清空站点的响应缓存：path 留空清空全部，否则只清空路径命中的条目（写法同缓存规则路径，支持 *）", Req: &cachePurgeRequest{}, Resp: &cachePurgeResult{}})
}

// siteCache 返回站点运行中的缓存，站点不存在时 ok 为 false，未启用缓存时 c 为 nil。
func (m *Module) siteCache(listener, site string) (c *respCache, ok bool) {
	m.mu.Lock()
	h := m.findSiteLocked(listener, site)
	m.mu.Unlock()
	if h == nil {
		return nil, false
	}
	return h.cache, true
}

func cacheStats(c *respCache) CacheStatsView {
	if c == nil {
		return CacheStatsView{}
	}
	n, size := c.stats()
	return CacheStatsView{Enabled: true, Entries: n, Bytes: size, MaxBytes: c.max}
}

func (m *Module) handleCacheStats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	c, ok := m.siteCache(q.Get("listener"), q.Get("site"))
	if !ok {
		httpx.Error(w, http.StatusNotFound, "站点不存在或所属监听未运行")
		return
	}
	httpx.OK(w, cacheStats(c))
}

func (m *Module) handleCachePurge(w http.ResponseWriter, r *http.Request) {
	var req cachePurgeRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	req.Path = strings.TrimSpace(req.Path)
	if req.Path != "" {
		if err := validateCachePath(req.Path); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
	}
	c, ok := m.siteCache(req.Listener, req.Site)
	if !ok {
		httpx.Error(w, http.StatusNotFound, "站点不存在或所属监听未运行")
		return
	}
	if c == nil {
		httpx.Error(w, http.StatusBadRequest, "站点未启用响应缓存")
		return
	}
	n := c.purge(req.Path)
	httpx.OK(w, cachePurgeResult{Removed: n, CacheStatsView: cacheStats(c)})
}
