package webservice

import (
	"bufio"
	"container/list"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/httpcompress"
)

// maxCacheEntry 是单个缓存条目的上限，同时不超过总容量的 1/4。
const maxCacheEntry = 4 << 20

type cacheEntry struct {
	key string
	// path 为规范化后的请求路径，按路径清空缓存时使用
	path    string
	status  int
	header  http.Header
	body    []byte
	expires time.Time
	stored  time.Time
}

// respCache 是按字节容量限制的 LRU 内存缓存。
type respCache struct {
	mu    sync.Mutex
	max   int64
	size  int64
	ttl   time.Duration
	ll    *list.List
	items map[string]*list.Element
	now   func() time.Time
	// rules 为按路径的缓存规则，已按具体程度从高到低排序
	rules []cacheRule
}

// cacheRule 是编译后的缓存规则。
type cacheRule struct {
	path     string
	noCache  bool
	ttl      time.Duration
	override bool
}

func newRespCache(c CacheConfig) *respCache {
	rc := &respCache{max: int64(c.MaxMB) << 20, ttl: time.Duration(c.TTL) * time.Second, ll: list.New(), items: map[string]*list.Element{}, now: time.Now}
	for _, r := range c.Rules {
		cr := cacheRule{path: r.Path, noCache: r.NoCache, override: r.Override, ttl: rc.ttl}
		// 规则未设置缓存时间时沿用站点默认
		if r.TTL > 0 {
			cr.ttl = time.Duration(r.TTL) * time.Second
		}
		rc.rules = append(rc.rules, cr)
	}
	// 更具体的规则优先：去掉 * 后字符更多的在前，相同时保持配置顺序
	sort.SliceStable(rc.rules, func(i, j int) bool {
		return cachePathWeight(rc.rules[i].path) > cachePathWeight(rc.rules[j].path)
	})
	return rc
}

// cachePathWeight 返回规则路径的具体程度（* 以外的字符数）。
func cachePathWeight(p string) int {
	return len(p) - strings.Count(p, "*")
}

// cachePathMatch 判断规范化后的请求路径 p 是否命中规则路径 pattern：
// 不含 * 时按路径段边界做前缀匹配，含 * 时整条路径按通配匹配，* 匹配任意字符（包括 /）。
func cachePathMatch(pattern, p string) bool {
	if !strings.Contains(pattern, "*") {
		return prefixMatch(pattern, p)
	}
	return wildcardMatch(pattern, p)
}

// wildcardMatch 是只支持 * 的通配匹配，采用贪心回溯，时间复杂度为 O(len(pattern)*len(s))，不会因构造的路径退化成指数级。
func wildcardMatch(pattern, s string) bool {
	pi, si := 0, 0
	star, mark := -1, 0
	for si < len(s) {
		switch {
		case pi < len(pattern) && pattern[pi] == '*':
			star, mark = pi, si
			pi++
		case pi < len(pattern) && pattern[pi] == s[si]:
			pi++
			si++
		case star >= 0:
			// 回到上一个 *，让它多吞一个字符
			pi = star + 1
			mark++
			si = mark
		default:
			return false
		}
	}
	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}
	return pi == len(pattern)
}

// match 返回请求路径命中的缓存规则，未命中时返回 nil（使用站点默认设置）。
func (c *respCache) match(p string) *cacheRule {
	for i := range c.rules {
		if cachePathMatch(c.rules[i].path, p) {
			return &c.rules[i]
		}
	}
	return nil
}

// purge 删除路径命中 pattern 的缓存条目，pattern 为空时清空全部，返回删除的条数。
func (c *respCache) purge(pattern string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for el := c.ll.Front(); el != nil; {
		next := el.Next()
		if pattern == "" || cachePathMatch(pattern, el.Value.(*cacheEntry).path) {
			c.removeLocked(el)
			n++
		}
		el = next
	}
	return n
}

// stats 返回当前的缓存条目数与占用字节数。
func (c *respCache) stats() (int, int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len(), c.size
}

func (c *respCache) get(key string) *cacheEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil
	}
	e := el.Value.(*cacheEntry)
	if c.now().After(e.expires) {
		c.removeLocked(el)
		return nil
	}
	c.ll.MoveToFront(el)
	return e
}

func (c *respCache) removeLocked(el *list.Element) {
	e := el.Value.(*cacheEntry)
	c.ll.Remove(el)
	delete(c.items, e.key)
	c.size -= int64(len(e.body))
}

func (c *respCache) put(e *cacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if old, ok := c.items[e.key]; ok {
		c.removeLocked(old)
	}
	c.items[e.key] = c.ll.PushFront(e)
	c.size += int64(len(e.body))
	for c.size > c.max && c.ll.Len() > 0 {
		c.removeLocked(c.ll.Back())
	}
}

// credentialHeaders 是常见的凭据请求头：带这些头的请求内容可能因人而异，任何模式下都不缓存。
var credentialHeaders = []string{
	"Authorization", "Proxy-Authorization", "Cookie",
	"X-Api-Key", "X-Auth-Token", "X-Access-Token", "X-CSRF-Token",
	"X-Emby-Token", "X-Emby-Authorization", "X-MediaBrowser-Token", "X-Plex-Token", "X-Jellyfin-Token",
}

// credentialParams 是常见的凭据查询参数（比较时不区分大小写）。
var credentialParams = map[string]bool{"api_key": true, "x-plex-token": true, "token": true, "access_token": true}

// requestCacheable 判断请求是否可以走缓存。带凭据或 Cookie 的请求内容可能因人而异，一律不缓存。
func requestCacheable(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	if r.Header.Get("Range") != "" {
		return false
	}
	for _, k := range credentialHeaders {
		if r.Header.Get(k) != "" {
			return false
		}
	}
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return false
	}
	if hasCredentialParam(r.URL.RawQuery) {
		return false
	}
	if ambiguousPath(r) {
		return false
	}
	return !strings.Contains(strings.ToLower(r.Header.Get("Cache-Control")), "no-cache")
}

// hasCredentialParam 判断查询串是否带凭据参数。按 & 与 ; 两种分隔符逐个取参数名，
// 后端可能把 ; 当分隔符，用 url.ParseQuery 会整段丢掉 a=1;token=x 这类参数，因此自行拆分。
func hasCredentialParam(rawQuery string) bool {
	for _, part := range strings.FieldsFunc(rawQuery, func(c rune) bool { return c == '&' || c == ';' }) {
		k, _, _ := strings.Cut(part, "=")
		if u, err := url.QueryUnescape(k); err == nil {
			k = u
		}
		if credentialParams[strings.ToLower(strings.TrimSpace(k))] {
			return true
		}
	}
	return false
}

// ambiguousPath 判断请求的原始路径是否含 ;、编码的 / 或 \、或字面 \。
// 这类路径在 Gately 规范化后与后端的理解可能不同（矩阵参数、编码斜杠、反斜杠当分隔符），
// 按规范化路径匹配的规则与缓存键可能对不上后端实际返回的内容，一律不走缓存。
func ambiguousPath(r *http.Request) bool {
	raw := r.RequestURI
	if i := strings.IndexByte(raw, '?'); i >= 0 {
		raw = raw[:i]
	}
	for _, p := range []string{raw, r.URL.RawPath, r.URL.Path} {
		if strings.ContainsAny(p, ";\\") {
			return true
		}
		lp := strings.ToLower(p)
		if strings.Contains(lp, "%2f") || strings.Contains(lp, "%5c") {
			return true
		}
	}
	return false
}

func cacheKey(r *http.Request) string {
	key := r.Host + "|" + r.URL.RequestURI() + "|" + httpcompress.Negotiate(r.Header.Get("Accept-Encoding")) + "|" + r.Header.Get("Accept-Encoding")
	// 按用户分流或传递用户名时，不同后端、不同用户的响应分开缓存
	if id, ok := r.Context().Value(ctxKeyIdentity{}).(requestIdentity); ok {
		key += "|" + id.cacheTag
	}
	return key
}

// freshness 根据响应头计算可缓存时长。ok 为 false 表示不可缓存。
// override 为 true 时忽略 Cache-Control 的 no-store、no-cache 与 max-age，固定使用 fallback，
// 但仍尊重 private（后端明确声明内容属于某个用户）；带 Set-Cookie 或 Vary: * 的响应无论如何都不缓存。
func freshness(h http.Header, fallback time.Duration, override bool) (time.Duration, bool) {
	if h.Get("Set-Cookie") != "" || strings.TrimSpace(h.Get("Vary")) == "*" {
		return 0, false
	}
	directives := strings.Split(strings.ToLower(strings.Join(h.Values("Cache-Control"), ",")), ",")
	if override {
		for _, d := range directives {
			// private 也可能写成 private="Set-Cookie" 的带字段形式，同样视为私有
			if d = strings.TrimSpace(d); d == "private" || strings.HasPrefix(d, "private=") {
				return 0, false
			}
		}
		return fallback, fallback > 0
	}
	var maxAge, sMax = -1, -1
	for _, d := range directives {
		d = strings.TrimSpace(d)
		switch {
		case strings.HasPrefix(d, "private="):
			return 0, false
		case d == "no-store" || d == "private" || d == "no-cache":
			return 0, false
		case strings.HasPrefix(d, "s-maxage="):
			if v, err := strconv.Atoi(strings.TrimPrefix(d, "s-maxage=")); err == nil {
				sMax = v
			}
		case strings.HasPrefix(d, "max-age="):
			if v, err := strconv.Atoi(strings.TrimPrefix(d, "max-age=")); err == nil {
				maxAge = v
			}
		}
	}
	switch {
	case sMax >= 0:
		return time.Duration(sMax) * time.Second, sMax > 0
	case maxAge >= 0:
		return time.Duration(maxAge) * time.Second, maxAge > 0
	}
	return fallback, fallback > 0
}

// captureWriter 在透传响应的同时收集内容，用于写入缓存。
type captureWriter struct {
	http.ResponseWriter
	limit   int
	status  int
	body    []byte
	over    bool
	started bool
}

func (c *captureWriter) WriteHeader(code int) {
	if !c.started {
		c.status = code
		c.started = true
	}
	c.ResponseWriter.WriteHeader(code)
}

func (c *captureWriter) Write(p []byte) (int, error) {
	if !c.started {
		c.status, c.started = http.StatusOK, true
	}
	if !c.over {
		if len(c.body)+len(p) > c.limit {
			c.over, c.body = true, nil
		} else {
			c.body = append(c.body, p...)
		}
	}
	return c.ResponseWriter.Write(p)
}

func (c *captureWriter) Flush() { _ = http.NewResponseController(c.ResponseWriter).Flush() }

func (c *captureWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	c.over = true
	return http.NewResponseController(c.ResponseWriter).Hijack()
}

func (c *captureWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }

// hopHeaders 不应进入缓存。
var hopHeaders = []string{"Connection", "Keep-Alive", "Proxy-Connection", "Transfer-Encoding", "Upgrade", "Date", "Set-Cookie"}

// wrap 返回带缓存的处理器。
func (c *respCache) wrap(next http.Handler) http.Handler {
	entryLimit := int(c.max / 4)
	if entryLimit > maxCacheEntry {
		entryLimit = maxCacheEntry
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requestCacheable(r) {
			next.ServeHTTP(w, r)
			return
		}
		// 规则按规范化后的路径匹配，避免用 /api//x、/static/../api 之类的写法绕过不缓存的规则
		p := cleanReqPath(r.URL.Path)
		ttl, override := c.ttl, false
		if rule := c.match(p); rule != nil {
			if rule.noCache {
				next.ServeHTTP(w, r)
				return
			}
			ttl, override = rule.ttl, rule.override
		}
		key := cacheKey(r)
		if e := c.get(key); e != nil {
			h := w.Header()
			for k, v := range e.header {
				// 本次请求由 Gately 写入的跨域头（按本次的 Origin）与 Vary: Origin 保留，缓存的同名头追加在后
				if mergedOnHit(k) {
					h[k] = append(h[k], v...)
					continue
				}
				h[k] = append([]string(nil), v...)
			}
			h.Set("X-Cache", "HIT")
			h.Set("Age", strconv.Itoa(int(c.now().Sub(e.stored).Seconds())))
			w.WriteHeader(e.status)
			_, _ = w.Write(e.body)
			return
		}
		// 记下进入缓存前 Gately 已写好的跨域头与 Vary，存储时只去掉这些值，后端自己返回的同名头不受影响
		pre := corsSnapshot(w.Header())
		cw := &captureWriter{ResponseWriter: w, limit: entryLimit}
		w.Header().Set("X-Cache", "MISS")
		next.ServeHTTP(cw, r)
		if cw.over || cw.status != http.StatusOK || len(cw.body) == 0 {
			return
		}
		h := w.Header()
		if strings.HasPrefix(strings.ToLower(h.Get("Content-Type")), "text/event-stream") {
			return
		}
		ttl, ok := freshness(h, ttl, override)
		if !ok {
			return
		}
		stored := h.Clone()
		// 去掉 Gately 按本次 Origin 写入的跨域头，避免命中时把别的来源的 Access-Control-Allow-Origin 发给本次访客
		for k, v := range pre {
			stored[k] = removeValues(stored[k], v)
			if len(stored[k]) == 0 {
				delete(stored, k)
			}
		}
		if !backendCacheSafe(stored, pre) {
			return
		}
		stored.Del("X-Cache")
		stored.Del("Age")
		for _, k := range hopHeaders {
			stored.Del(k)
		}
		now := c.now()
		c.put(&cacheEntry{key: key, path: p, status: cw.status, header: stored, body: cw.body, stored: now, expires: now.Add(ttl)})
	})
}

// corsSnapshot 返回 h 中 Gately 已写入的 Access-Control-* 与 Vary 头的副本。
func corsSnapshot(h http.Header) http.Header {
	var pre http.Header
	for k, v := range h {
		if mergedOnHit(k) {
			if pre == nil {
				pre = http.Header{}
			}
			pre[k] = append([]string(nil), v...)
		}
	}
	return pre
}

// mergedOnHit 判断该响应头是否可能由 Gately 按本次请求写入，命中缓存时与缓存的值合并而非覆盖。
func mergedOnHit(k string) bool {
	return k == "Vary" || strings.HasPrefix(k, "Access-Control-")
}

// removeValues 从 vs 中按出现次数去掉 drop 里的值（每个只去掉一次），其余值保持原顺序。
func removeValues(vs, drop []string) []string {
	if len(vs) == 0 || len(drop) == 0 {
		return vs
	}
	left := map[string]int{}
	for _, d := range drop {
		left[d]++
	}
	var out []string
	for _, v := range vs {
		if left[v] > 0 {
			left[v]--
			continue
		}
		out = append(out, v)
	}
	return out
}

// backendCacheSafe 判断去掉 Gately 自己的跨域头后，后端响应能否给所有访客共用（强制缓存同样适用）：
// Vary 只允许 Accept-Encoding（缓存键已包含），Gately 处理了本次跨域时再允许 Origin（命中时按本次来源重写跨域头）；
// 后端自己按来源返回的 Access-Control-Allow-Origin（非 *）因来源而异，同样不缓存。
func backendCacheSafe(stored, pre http.Header) bool {
	gatelyCORS := len(pre["Access-Control-Allow-Origin"]) > 0
	for _, v := range stored.Values("Vary") {
		for _, f := range strings.Split(v, ",") {
			f = strings.ToLower(strings.TrimSpace(f))
			switch {
			case f == "" || f == "accept-encoding":
			case f == "origin" && gatelyCORS:
			default:
				return false
			}
		}
	}
	for _, v := range stored.Values("Access-Control-Allow-Origin") {
		if strings.TrimSpace(v) != "*" {
			return false
		}
	}
	return true
}
