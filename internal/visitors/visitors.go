// Package visitors 实现访客审计：按访客地址聚合 Web 服务收到的请求，记录谁在何时访问了哪些站点与路径、
// 以什么身份访问、被拦截了多少次。数据只保存聚合结果（每个访客一条记录、按天计数），不保存逐条请求，
// 逐条请求仍以站点访问日志为准。
//
// 内存占用有上限：访客数超过上限时先淘汰只被拦截或只访问未匹配站点的地址（扫描器），再淘汰最久未出现的；
// 每条记录内的站点、路径、身份、User-Agent 列表都有条数上限，访客可控的字符串一律截断。
// 数据在有变化后延迟写盘（gzip 压缩的 JSON Lines），程序退出时同步写盘，启动时读回并按保留天数清理。
package visitors

import (
	"net/netip"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

// 设置的默认值与取值范围。
const (
	DefaultDays = 30
	MinDays     = 1
	MaxDays     = 180

	DefaultMax = 10000
	MinMax     = 100
	MaxMax     = 100000
)

// 每条记录内各列表的条数上限与字符串长度上限。
const (
	maxSites   = 12
	maxPaths   = 10
	maxIdents  = 6
	maxAgents  = 4
	maxDeny    = 8
	maxPathLen = 160
	maxHostLen = 253
	maxUALen   = 256
	maxNameLen = 128
	maxMethod  = 16
)

// Hit 是一次请求的访客信息，由 Web 服务在请求结束时上报。
type Hit struct {
	Addr netip.Addr
	Time time.Time
	// Listener 为监听 key；Site 为站点 key，未匹配任何站点（含默认站点）时为空
	Listener string
	Site     string
	SiteName string
	// Unmatched 表示请求的域名没有对应的站点（由默认站点处理或直接 404）
	Unmatched bool
	Host      string
	Method    string
	// Path 为请求路径，不含查询串（查询串中常有令牌、授权码）
	Path   string
	Status int
	// Deny 为拦截原因（ban、ip、country、rateLimit、waf），未被拦截为空
	Deny  string
	Bytes int64
	// User 为站点认证识别出的访客身份；Admin 为带本实例管理后台登录态时的管理员用户名
	User  string
	Admin string
	UA    string
}

// Day 是某一天（本地时间）的计数。D 为自 1970-01-01 起的天数。
type Day struct {
	D      int32  `json:"d"`
	Req    uint32 `json:"req"`
	Denied uint32 `json:"denied"`
}

// SiteStat 是访客访问某个站点的计数。
type SiteStat struct {
	Listener  string `json:"listener"`
	Key       string `json:"key"`
	Name      string `json:"name"`
	Unmatched bool   `json:"unmatched,omitempty"`
	Req       uint32 `json:"req"`
	Denied    uint32 `json:"denied"`
	Last      int64  `json:"last"`
}

// PathStat 是访客访问某个路径的计数，Status 为最近一次的状态码。
type PathStat struct {
	Path   string `json:"path"`
	Req    uint32 `json:"req"`
	Last   int64  `json:"last"`
	Status int    `json:"status"`
}

// IdentStat 是访客使用过的身份：Kind 为 user（站点认证）或 admin（管理后台登录态）。
type IdentStat struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	Req  uint32 `json:"req"`
	Last int64  `json:"last"`
}

// AgentStat 是访客使用过的 User-Agent。
type AgentStat struct {
	UA   string `json:"ua"`
	Req  uint32 `json:"req"`
	Last int64  `json:"last"`
}

// Count 是按名称的计数（拦截原因）。
type Count struct {
	Name string `json:"name"`
	N    uint32 `json:"n"`
}

// LastReq 是访客最近一次请求的摘要。
type LastReq struct {
	Time     int64  `json:"time"`
	Method   string `json:"method"`
	Host     string `json:"host"`
	Path     string `json:"path"`
	Status   int    `json:"status"`
	Deny     string `json:"deny,omitempty"`
	SiteName string `json:"siteName"`
}

// Visitor 是一个访客地址的聚合记录，同时是落盘格式。时间均为 Unix 秒。
type Visitor struct {
	IP       string `json:"ip"`
	First    int64  `json:"first"`
	Last     int64  `json:"last"`
	Requests uint64 `json:"requests"`
	Denied   uint64 `json:"denied"`
	// Noise 为被拦截或访问未匹配站点的请求数，与 Requests 相等时视为扫描类访客，超出上限时优先淘汰
	Noise uint64 `json:"noise"`
	Bytes int64  `json:"bytes"`
	// Status 依次为 1xx~5xx 的次数；444（断开连接）计入 4xx，同时计入 Closed
	Status     [5]uint32   `json:"status"`
	Closed     uint32      `json:"closed"`
	Deny       []Count     `json:"deny"`
	Days       []Day       `json:"days"`
	Sites      []SiteStat  `json:"sites"`
	Paths      []PathStat  `json:"paths"`
	Identities []IdentStat `json:"identities"`
	Agents     []AgentStat `json:"agents"`
	LastReq    LastReq     `json:"lastReq"`
}

// Settings 是访客审计设置。
type Settings struct {
	// Disable 为 true 时停止记录（已有数据保留，可手动清空）
	Disable bool `json:"disable"`
	// Days 为保留天数：最近一次访问早于该天数的访客被删除，按天计数也只保留这么多天
	Days int `json:"days"`
	// Max 为最多保留的访客数
	Max int `json:"max"`
}

// Normalize 把未填写的值换成默认值，并检查取值范围。
func (s *Settings) Normalize() error {
	if s.Days == 0 {
		s.Days = DefaultDays
	}
	if s.Max == 0 {
		s.Max = DefaultMax
	}
	if s.Days < MinDays || s.Days > MaxDays {
		return errDays
	}
	if s.Max < MinMax || s.Max > MaxMax {
		return errMax
	}
	return nil
}

// Store 保存全部访客记录。零值不可用，用 New 创建；nil 的 *Store 上调用 Record 什么也不做。
type Store struct {
	mu  sync.Mutex
	m   map[netip.Addr]*Visitor
	set Settings
	// active 为“记录中”的标记，供请求路径无锁判断
	active atomic.Bool
	path   string
	// dirty 表示有未写盘的变化；timer 为已安排的延迟写盘
	dirty bool
	timer *time.Timer
	// saveMu 串行化写盘，避免延迟写盘与退出时的写盘交错
	saveMu sync.Mutex
	// saved 为最近一次写盘的时间与文件大小，saveErr 为最近一次写盘失败的原因
	saved    time.Time
	fileSize int64
	saveErr  string
	// now 便于测试注入时间
	now func() time.Time
	// warn 记录写盘、读取失败等问题，可为 nil
	warn func(format string, args ...any)
}

// New 创建访客存储；path 为落盘文件路径，为空时只保存在内存中。
func New(path string, warn func(format string, args ...any)) *Store {
	s := &Store{m: map[netip.Addr]*Visitor{}, path: path, now: time.Now, warn: warn}
	s.set = Settings{Days: DefaultDays, Max: DefaultMax}
	s.active.Store(true)
	return s
}

// Active 判断是否正在记录（存储存在且未停用）。
func (s *Store) Active() bool {
	return s != nil && s.active.Load()
}

// Settings 返回当前设置。
func (s *Store) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.set
}

// Apply 应用设置（调用方已 Normalize）：保留天数或上限变小时立即清理并安排写盘。
func (s *Store) Apply(set Settings) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.set = set
	s.active.Store(!set.Disable)
	if s.pruneLocked(s.now()) {
		s.markDirtyLocked()
	}
	if len(s.m) > set.Max {
		s.evictLocked(len(s.m) - set.Max)
		s.markDirtyLocked()
	}
}

// dayOf 返回时间所在的本地日期编号（自 1970-01-01 起的天数）。
func dayOf(t time.Time) int32 {
	_, off := t.Zone()
	sec := t.Unix() + int64(off)
	d := sec / 86400
	// 1970 年之前的时间向下取整
	if sec < 0 && sec%86400 != 0 {
		d--
	}
	return int32(d)
}

// clip 把 s 截断到最多 n 字节（不截断多字节字符），并去掉控制字符；结果总是新分配的字符串，
// 不会因为引用请求里的长字符串而让整段内存无法回收。
func clip(v string, n int) string {
	if len(v) > n {
		cut := n
		for cut > 0 && !utf8.RuneStart(v[cut]) {
			cut--
		}
		v = v[:cut]
	}
	if strings.IndexFunc(v, isControl) >= 0 {
		v = strings.Map(func(r rune) rune {
			if isControl(r) {
				return -1
			}
			return r
		}, v)
	}
	return strings.Clone(v)
}

// reuse 在新值与旧值相同时沿用旧字符串，避免同一访客的连续请求反复分配内存；否则返回截断后的新值。
func reuse(old, v string, n int) string {
	if v == old {
		return old
	}
	return clip(v, n)
}

func isControl(r rune) bool {
	return r < 0x20 || r == 0x7f || r == utf8.RuneError
}

// Record 记录一次请求。停用时直接返回。
func (s *Store) Record(h Hit) {
	if !s.Active() || !h.Addr.IsValid() {
		return
	}
	addr := h.Addr.Unmap()
	if h.Time.IsZero() {
		h.Time = s.now()
	}
	now := h.Time.Unix()
	day := dayOf(h.Time)
	denied := h.Deny != ""
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.set.Disable {
		return
	}
	v := s.m[addr]
	if v == nil {
		if len(s.m) >= s.set.Max {
			// 一次淘汰约 5%，避免每个新访客都排序一遍全部记录
			s.evictLocked(max(1, s.set.Max/20))
		}
		v = &Visitor{IP: addr.String(), First: now}
		s.m[addr] = v
	}
	if now > v.Last {
		v.Last = now
	}
	v.Requests++
	if denied {
		v.Denied++
	}
	if denied || h.Unmatched {
		v.Noise++
	}
	if h.Bytes > 0 {
		v.Bytes += h.Bytes
	}
	if c := h.Status / 100; c >= 1 && c <= 5 {
		v.Status[c-1]++
	}
	if h.Status == 444 {
		v.Closed++
	}
	if denied {
		addCount(&v.Deny, clip(h.Deny, 32))
	}
	addDay(v, day, denied)
	s.addSite(v, h, now, denied)
	path := h.Path
	if path == "" {
		path = "/"
	}
	addPath(v, path, now, h.Status)
	if h.User != "" {
		addIdent(v, h.User, "user", now)
	}
	if h.Admin != "" {
		addIdent(v, h.Admin, "admin", now)
	}
	if h.UA != "" {
		addAgent(v, h.UA, now)
	}
	if now >= v.LastReq.Time {
		r := &v.LastReq
		r.Time, r.Status = now, h.Status
		r.Method, r.Host, r.Path = reuse(r.Method, h.Method, maxMethod), reuse(r.Host, h.Host, maxHostLen), reuse(r.Path, path, maxPathLen)
		r.Deny, r.SiteName = reuse(r.Deny, h.Deny, 32), reuse(r.SiteName, h.SiteName, maxNameLen)
	}
	s.markDirtyLocked()
}

func addCount(list *[]Count, name string) {
	for i := range *list {
		if (*list)[i].Name == name {
			(*list)[i].N++
			return
		}
	}
	if len(*list) < maxDeny {
		*list = append(*list, Count{Name: name, N: 1})
	}
}

// addDay 累加当天的计数；按天的列表按日期升序，通常只需比较最后一项。
func addDay(v *Visitor, day int32, denied bool) {
	n := len(v.Days)
	var d *Day
	if n > 0 && v.Days[n-1].D == day {
		d = &v.Days[n-1]
	} else {
		i, found := slices.BinarySearchFunc(v.Days, day, func(x Day, t int32) int { return int(x.D) - int(t) })
		if !found {
			v.Days = slices.Insert(v.Days, i, Day{D: day})
		}
		d = &v.Days[i]
	}
	d.Req++
	if denied {
		d.Denied++
	}
}

// replaceIndex 返回列表满时被替换的项：请求数最少的，相同时最久未出现的。
func replaceIndex(n int, req func(int) uint32, last func(int) int64) int {
	idx := 0
	for i := 1; i < n; i++ {
		if req(i) < req(idx) || (req(i) == req(idx) && last(i) < last(idx)) {
			idx = i
		}
	}
	return idx
}

func (s *Store) addSite(v *Visitor, h Hit, now int64, denied bool) {
	for i := range v.Sites {
		st := &v.Sites[i]
		if st.Listener == h.Listener && st.Key == h.Site && st.Unmatched == h.Unmatched {
			st.Req++
			if denied {
				st.Denied++
			}
			st.Last = now
			// 站点改名后显示新名称
			if h.SiteName != st.Name {
				st.Name = clip(h.SiteName, maxNameLen)
			}
			return
		}
	}
	st := SiteStat{Listener: clip(h.Listener, maxNameLen), Key: clip(h.Site, maxNameLen), Name: clip(h.SiteName, maxNameLen),
		Unmatched: h.Unmatched, Req: 1, Last: now}
	if denied {
		st.Denied = 1
	}
	if len(v.Sites) < maxSites {
		v.Sites = append(v.Sites, st)
		return
	}
	i := replaceIndex(len(v.Sites), func(i int) uint32 { return v.Sites[i].Req }, func(i int) int64 { return v.Sites[i].Last })
	v.Sites[i] = st
}

func addPath(v *Visitor, path string, now int64, status int) {
	// 先按截断后的长度比较，超长路径截断后仍能与已有记录合并
	if len(path) > maxPathLen || strings.IndexFunc(path, isControl) >= 0 {
		path = clip(path, maxPathLen)
	}
	for i := range v.Paths {
		p := &v.Paths[i]
		if p.Path == path {
			p.Req++
			p.Last = now
			p.Status = status
			return
		}
	}
	p := PathStat{Path: strings.Clone(path), Req: 1, Last: now, Status: status}
	if len(v.Paths) < maxPaths {
		v.Paths = append(v.Paths, p)
		return
	}
	i := replaceIndex(len(v.Paths), func(i int) uint32 { return v.Paths[i].Req }, func(i int) int64 { return v.Paths[i].Last })
	v.Paths[i] = p
}

func addIdent(v *Visitor, name, kind string, now int64) {
	name = clip(name, maxNameLen)
	for i := range v.Identities {
		id := &v.Identities[i]
		if id.Name == name && id.Kind == kind {
			id.Req++
			id.Last = now
			return
		}
	}
	id := IdentStat{Name: name, Kind: kind, Req: 1, Last: now}
	if len(v.Identities) < maxIdents {
		v.Identities = append(v.Identities, id)
		return
	}
	i := replaceIndex(len(v.Identities), func(i int) uint32 { return v.Identities[i].Req }, func(i int) int64 { return v.Identities[i].Last })
	v.Identities[i] = id
}

func addAgent(v *Visitor, ua string, now int64) {
	if len(ua) > maxUALen || strings.IndexFunc(ua, isControl) >= 0 {
		ua = clip(ua, maxUALen)
	}
	for i := range v.Agents {
		a := &v.Agents[i]
		if a.UA == ua {
			a.Req++
			a.Last = now
			return
		}
	}
	a := AgentStat{UA: strings.Clone(ua), Req: 1, Last: now}
	if len(v.Agents) < maxAgents {
		v.Agents = append(v.Agents, a)
		return
	}
	i := replaceIndex(len(v.Agents), func(i int) uint32 { return v.Agents[i].Req }, func(i int) int64 { return v.Agents[i].Last })
	v.Agents[i] = a
}

// noise 判断访客是否只有被拦截或访问未匹配站点的请求，且从未以任何身份访问过。
func (v *Visitor) noise() bool {
	return v.Noise >= v.Requests && len(v.Identities) == 0
}

// evictLocked 淘汰 n 个访客：先淘汰扫描类访客，同类中最久未出现的先淘汰。
func (s *Store) evictLocked(n int) {
	if n <= 0 || len(s.m) == 0 {
		return
	}
	type cand struct {
		addr  netip.Addr
		noise bool
		last  int64
	}
	all := make([]cand, 0, len(s.m))
	for a, v := range s.m {
		all = append(all, cand{a, v.noise(), v.Last})
	}
	slices.SortFunc(all, func(x, y cand) int {
		if x.noise != y.noise {
			if x.noise {
				return -1
			}
			return 1
		}
		if x.last != y.last {
			if x.last < y.last {
				return -1
			}
			return 1
		}
		return x.addr.Compare(y.addr)
	})
	for i := 0; i < n && i < len(all); i++ {
		delete(s.m, all[i].addr)
	}
}

// pruneLocked 删除最近一次访问早于保留天数的访客，并删除各记录中超出保留天数的按天计数；有变化时返回 true。
func (s *Store) pruneLocked(now time.Time) bool {
	cutDay := dayOf(now) - int32(s.set.Days) + 1
	cutSec := now.Unix() - int64(s.set.Days)*86400
	changed := false
	for a, v := range s.m {
		if v.Last < cutSec {
			delete(s.m, a)
			changed = true
			continue
		}
		i := 0
		for i < len(v.Days) && v.Days[i].D < cutDay {
			i++
		}
		if i > 0 {
			v.Days = slices.Delete(v.Days, 0, i)
			changed = true
		}
	}
	return changed
}

// Delete 删除指定地址的记录，返回删除的条数。
func (s *Store) Delete(addrs []netip.Addr) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, a := range addrs {
		a = a.Unmap()
		if _, ok := s.m[a]; ok {
			delete(s.m, a)
			n++
		}
	}
	if n > 0 {
		s.markDirtyLocked()
	}
	return n
}

// Clear 删除全部记录，返回删除的条数。
func (s *Store) Clear() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.m)
	s.m = map[netip.Addr]*Visitor{}
	s.markDirtyLocked()
	return n
}

// Len 返回访客数。
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.m)
}

// Get 返回一个访客记录的副本。
func (s *Store) Get(a netip.Addr) (Visitor, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[a.Unmap()]
	if !ok {
		return Visitor{}, false
	}
	return v.clone(), true
}

// clone 深拷贝记录，交给调用方在锁外使用。
func (v *Visitor) clone() Visitor {
	c := *v
	c.Deny = slices.Clone(v.Deny)
	c.Days = slices.Clone(v.Days)
	c.Sites = slices.Clone(v.Sites)
	c.Paths = slices.Clone(v.Paths)
	c.Identities = slices.Clone(v.Identities)
	c.Agents = slices.Clone(v.Agents)
	return c
}
