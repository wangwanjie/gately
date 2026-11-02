package webservice

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wangwanjie/gately/internal/httpx"
)

// 站点的实时连接与历史记录。
//
// 列出的单位是“进行中的请求”而非 TCP 连接：HTTP/1.1 的长连接在两次请求之间不属于任何站点，
// HTTP/2、HTTP/3 的一条连接可同时承载多个请求，证书相同时还可能跨站点复用，只有按请求才能准确归属到站点。
// 已升级的 WebSocket 在升级后一直占用这个请求，因此同样会列出，直到连接关闭。
//
// 断开一个请求时：HTTP/1.x 关闭整条 TCP 连接，HTTP/2、HTTP/3 只重置这个请求流，已升级的连接直接关闭。

const (
	// connShards 为进行中请求表的分片数（2 的幂），减少高并发下登记与注销的锁竞争
	connShards = 16
	// connHistorySize 为每个站点保留的历史记录条数
	connHistorySize = 200
	// connHistoryMinDuration 为进入历史记录的最短持续时间；被手动断开的请求不受此限制。
	// 普通短请求每秒可能有成百上千个，全部收录会让历史在一秒内被冲掉，短请求请查看访问日志
	connHistoryMinDuration = time.Second
	// maxConnList 为接口一次最多返回的进行中请求条数（按已持续时间从长到短）
	maxConnList = 500
	// 列表中路径与域名的最大字节数
	maxConnPath = 512
	maxConnHost = 255
)

// errConnClosed 是请求被管理员断开后，继续写出响应时返回的错误。
var errConnClosed = errors.New("connection closed by administrator")

// connSeq 为进程内递增的请求编号，跨站点唯一。
var connSeq atomic.Uint64

// activeReq 是一个进行中的请求。
type activeReq struct {
	id     uint64
	client netip.AddrPort
	// peer 为与访客地址不同的直接对端（经可信代理转发时为代理地址），否则无效
	peer   netip.Addr
	proto  string
	method string
	host   string
	path   string
	start  time.Time
	// down 为发给访客的字节数，up 为收到的请求体字节数（WebSocket 升级后为双向数据）
	down atomic.Int64
	up   atomic.Int64
	// upgraded 表示连接已被接管（WebSocket 等协议升级）
	upgraded atomic.Bool
	killed   atomic.Bool
	// cancel 取消请求上下文；断开时以 errConnClosed 为原因，反向代理据此区分管理员断开与后端错误
	cancel context.CancelCauseFunc
	// w 为服务器交给处理器的原始 ResponseWriter（各层包装之前），断开时用于让阻塞中的读写立即失败
	w http.ResponseWriter
	// mu 保护 conn 与 done：conn 为协议升级后接管的连接，断开时直接关闭；
	// done 表示处理器已返回（end 已调用），此后 w 可能已被 net/http 回收（HTTP/2 会放回池中复用），
	// 也可能属于同一条长连接上的下一个请求，断开不能再对它设置截止时间
	mu   sync.Mutex
	conn net.Conn
	done bool
	// upstream 为反向代理为这个请求选中的后端（非代理站点为 nil），拓扑视图据此把访客地址归到具体后端
	upstream atomic.Pointer[upstream]
}

// ctxKeyActiveReq 为请求上下文中登记项的键。
type ctxKeyActiveReq struct{}

// close 断开请求：取消请求上下文（反向代理随之结束后端请求、关闭已升级的后端连接），
// 并关闭已接管的连接。后续写出的响应会被丢弃，处理结束后由 ServeHTTP 中断连接或请求流。
// 返回 false 表示已经断开过。
func (a *activeReq) close() bool {
	// 整个断开过程持有 mu：与 end 互斥，保证设置截止时间时处理器尚未返回、w 仍属于这个请求
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.done || !a.killed.CompareAndSwap(false, true) {
		return false
	}
	a.cancel(errConnClosed)
	if c := a.conn; c != nil {
		c.Close()
		return true
	}
	// 访客不读取响应（或 HTTP/2 流量控制窗口耗尽）时处理器会阻塞在写出上，只靠标记无法结束。
	// 把读写截止时间设为现在：HTTP/1.x 阻塞中的读写立即返回超时错误，HTTP/2 立即重置这个请求流，HTTP/3 中断流的读写。
	// 这几个方法可以在处理器之外的 goroutine 调用；连接已被接管时返回错误，忽略即可
	now := time.Now()
	rc := http.NewResponseController(a.w)
	_ = rc.SetWriteDeadline(now)
	_ = rc.SetReadDeadline(now)
	return true
}

// attach 记录协议升级后接管的连接。断开发生在接管之前时立即关闭它。
func (a *activeReq) attach(c net.Conn) {
	a.upgraded.Store(true)
	a.mu.Lock()
	a.conn = c
	a.mu.Unlock()
	if a.killed.Load() {
		c.Close()
	}
}

// ConnView 是接口返回的一条连接（进行中或已结束）。
type ConnView struct {
	ID string `json:"id"`
	// IP、Port 为访客地址（经可信代理时为真实访客地址，端口沿用对端端口）
	IP   string `json:"ip"`
	Port uint16 `json:"port"`
	// Peer 为与访客地址不同的直接对端地址（如 CDN 节点），相同时省略
	Peer      string    `json:"peer,omitempty"`
	Proto     string    `json:"proto"`
	Method    string    `json:"method"`
	Host      string    `json:"host"`
	Path      string    `json:"path"`
	WebSocket bool      `json:"websocket,omitempty"`
	Start     time.Time `json:"start"`
	// End 与 Result 仅历史记录有：Result 为 done（正常结束）或 closed（被断开）
	End        *time.Time `json:"end,omitempty"`
	DurationMs int64      `json:"durationMs"`
	Down       int64      `json:"down"`
	Up         int64      `json:"up"`
	Result     string     `json:"result,omitempty"`
}

// ConnsView 是一个站点的连接列表。
type ConnsView struct {
	// Total 为进行中的请求总数（含被搜索条件过滤掉、超出返回上限的）
	Total int `json:"total"`
	// Matched 为符合搜索条件的进行中请求数；Active 最多返回 maxConnList 条
	Matched int        `json:"matched"`
	Active  []ConnView `json:"active"`
	History []ConnView `json:"history"`
	// Closed 为监听启动以来在该站点手动断开的请求数
	Closed int64 `json:"closed"`
}

// connTracker 记录一个站点进行中的请求与最近结束的长请求。站点热更新时由新处理器沿用。
type connTracker struct {
	shards [connShards]struct {
		mu sync.Mutex
		m  map[uint64]*activeReq
	}
	histMu sync.Mutex
	// hist 为环形缓冲，next 为下一个写入位置，full 表示已写满一圈
	hist   [connHistorySize]ConnView
	next   int
	full   bool
	closed atomic.Int64
}

func newConnTracker() *connTracker {
	t := &connTracker{}
	for i := range t.shards {
		t.shards[i].m = map[uint64]*activeReq{}
	}
	return t
}

// begin 登记一个请求，返回带取消能力的请求与登记项。w 为服务器交给处理器的原始 ResponseWriter。
// 调用方必须在请求结束后调用 end。
func (t *connTracker) begin(w http.ResponseWriter, r *http.Request, client netip.AddrPort) (*http.Request, *activeReq) {
	ctx, cancel := context.WithCancelCause(r.Context())
	a := &activeReq{
		id:     connSeq.Add(1),
		client: client,
		proto:  r.Proto,
		method: clip(r.Method, 32),
		host:   clip(r.Host, maxConnHost),
		// 只记录路径不记录查询串：查询串中常有令牌、授权码等敏感信息
		path:   clip(r.URL.Path, maxConnPath),
		start:  time.Now(),
		cancel: cancel,
		w:      w,
	}
	if p, ok := r.Context().Value(ctxKeyPeer{}).(peerInfo); ok && p.addr.IsValid() && p.addr.Unmap() != client.Addr() {
		a.peer = p.addr.Unmap()
	}
	s := &t.shards[a.id&(connShards-1)]
	s.mu.Lock()
	s.m[a.id] = a
	s.mu.Unlock()
	return r.WithContext(context.WithValue(ctx, ctxKeyActiveReq{}, a)), a
}

// end 注销请求；持续时间达到阈值或被断开的请求写入历史记录。
func (t *connTracker) end(a *activeReq) {
	a.mu.Lock()
	a.done = true
	a.mu.Unlock()
	a.cancel(context.Canceled)
	s := &t.shards[a.id&(connShards-1)]
	s.mu.Lock()
	delete(s.m, a.id)
	s.mu.Unlock()
	now := time.Now()
	killed := a.killed.Load()
	if !killed && now.Sub(a.start) < connHistoryMinDuration {
		return
	}
	v := a.view(now)
	v.End = &now
	v.Result = "done"
	if killed {
		v.Result = "closed"
	}
	t.histMu.Lock()
	t.hist[t.next] = v
	t.next = (t.next + 1) % connHistorySize
	if t.next == 0 {
		t.full = true
	}
	t.histMu.Unlock()
}

// view 生成请求的快照。
func (a *activeReq) view(now time.Time) ConnView {
	v := ConnView{
		ID: formatConnID(a.id), IP: a.client.Addr().String(), Port: a.client.Port(),
		Proto: a.proto, Method: a.method, Host: a.host, Path: a.path, WebSocket: a.upgraded.Load(),
		Start: a.start, DurationMs: now.Sub(a.start).Milliseconds(), Down: a.down.Load(), Up: a.up.Load(),
	}
	if a.peer.IsValid() {
		v.Peer = a.peer.String()
	}
	return v
}

// active 返回全部进行中的请求。
func (t *connTracker) active() []*activeReq {
	var out []*activeReq
	for i := range t.shards {
		s := &t.shards[i]
		s.mu.Lock()
		for _, a := range s.m {
			out = append(out, a)
		}
		s.mu.Unlock()
	}
	return out
}

// find 按编号查找进行中的请求。
func (t *connTracker) find(id uint64) *activeReq {
	s := &t.shards[id&(connShards-1)]
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[id]
}

// closeConn 断开指定请求，返回是否找到并断开。
func (t *connTracker) closeConn(id uint64) bool {
	a := t.find(id)
	if a == nil || !a.close() {
		return false
	}
	t.closed.Add(1)
	return true
}

// closeWhere 断开满足条件的全部请求，返回断开的数量。
func (t *connTracker) closeWhere(match func(*activeReq) bool) int {
	n := 0
	for _, a := range t.active() {
		if match(a) && a.close() {
			t.closed.Add(1)
			n++
		}
	}
	return n
}

// snapshot 返回连接列表。q 非空时只返回访客地址或对端地址包含 q 的请求（历史记录同样过滤）。
func (t *connTracker) snapshot(q string, now time.Time) ConnsView {
	q = strings.TrimSpace(q)
	all := t.active()
	v := ConnsView{Total: len(all), Active: []ConnView{}, History: []ConnView{}, Closed: t.closed.Load()}
	// 持续时间最长的排在前面，同时开始的按编号排序保证顺序稳定
	sort.Slice(all, func(i, j int) bool {
		if !all[i].start.Equal(all[j].start) {
			return all[i].start.Before(all[j].start)
		}
		return all[i].id < all[j].id
	})
	for _, a := range all {
		cv := a.view(now)
		if !connMatches(cv, q) {
			continue
		}
		v.Matched++
		if len(v.Active) < maxConnList {
			v.Active = append(v.Active, cv)
		}
	}
	t.histMu.Lock()
	n := t.next
	if t.full {
		n = connHistorySize
	}
	// 从最近结束的开始倒序取出
	for i := 1; i <= n; i++ {
		h := t.hist[(t.next-i+connHistorySize)%connHistorySize]
		if connMatches(h, q) {
			v.History = append(v.History, h)
		}
	}
	t.histMu.Unlock()
	return v
}

// connMatches 判断连接的访客地址或对端地址是否包含搜索词。
func connMatches(v ConnView, q string) bool {
	return q == "" || strings.Contains(v.IP, q) || (v.Peer != "" && strings.Contains(v.Peer, q))
}

// formatConnID 把请求编号格式化为十进制字符串（JSON 中的大整数在浏览器里会丢精度）。
func formatConnID(id uint64) string {
	return strconv.FormatUint(id, 10)
}

// trackedBody 统计请求体的字节数。
type trackedBody struct {
	io.ReadCloser
	req *activeReq
}

func (b *trackedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.req.up.Add(int64(n))
	return n, err
}

// trackedConn 统计协议升级后接管的连接上的双向字节数。
type trackedConn struct {
	net.Conn
	req *activeReq
	// rate 为站点的秒级速率，发给访客的字节同样计入
	rate *rateRing
}

func (c *trackedConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.req.up.Add(int64(n))
	return n, err
}

func (c *trackedConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	c.req.down.Add(int64(n))
	if c.rate != nil && n > 0 {
		c.rate.add(time.Now().Unix(), 0, int64(n))
	}
	return n, err
}

// Hijack 让 WebSocket 等协议升级能接管底层连接，接管后的连接同样计入流量并可被断开。
func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	c, brw, err := http.NewResponseController(s.ResponseWriter).Hijack()
	if err != nil || s.req == nil {
		return c, brw, err
	}
	tc := &trackedConn{Conn: c, req: s.req, rate: s.rate}
	s.req.attach(tc)
	return tc, brw, nil
}

// FlushError 让断开后的刷新同样失败，避免把尚未写出的响应头发给访客。
func (s *statusRecorder) FlushError() error {
	if s.req != nil && s.req.killed.Load() {
		return errConnClosed
	}
	s.wroteHeader = true
	s.markFirstByte()
	return http.NewResponseController(s.ResponseWriter).Flush()
}

// closeConnRequest 是断开连接的请求。
type closeConnRequest struct {
	Listener string `json:"listener"`
	Site     string `json:"site"`
	ID       string `json:"id"`
	// BanSeconds 大于 0 时同时封禁该访客地址（IPv6 为所在 /64 网段）这么多秒，
	// 并断开该地址在全部站点上进行中的请求
	BanSeconds int `json:"banSeconds"`
}

// closeConnResult 是断开连接的结果。
type closeConnResult struct {
	// Closed 为实际断开的请求数（封禁时含该地址在其他站点上的请求）
	Closed int `json:"closed"`
	// BanKey 为封禁的键，未封禁时为空
	BanKey string `json:"banKey,omitempty"`
}

func (m *Module) handleConns(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	m.mu.Lock()
	h := m.findSiteLocked(q.Get("listener"), q.Get("site"))
	m.mu.Unlock()
	if h == nil {
		httpx.Error(w, http.StatusNotFound, "站点不存在或所属监听未运行")
		return
	}
	httpx.OK(w, h.conns.snapshot(clip(q.Get("q"), 64), time.Now()))
}

func (m *Module) handleCloseConn(w http.ResponseWriter, r *http.Request) {
	var req closeConnRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	id, err := strconv.ParseUint(strings.TrimSpace(req.ID), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "连接编号无效")
		return
	}
	if req.BanSeconds < 0 || time.Duration(req.BanSeconds)*time.Second > maxBanDuration {
		httpx.Error(w, http.StatusBadRequest, "封禁时长不能超过 30 天")
		return
	}
	m.mu.Lock()
	h := m.findSiteLocked(req.Listener, req.Site)
	m.mu.Unlock()
	if h == nil {
		httpx.Error(w, http.StatusNotFound, "站点不存在或所属监听未运行")
		return
	}
	// 编号只在所属站点内查找，不能借一个站点的接口断开其他站点的请求
	a := h.conns.find(id)
	if a == nil {
		httpx.Error(w, http.StatusNotFound, "连接已结束")
		return
	}
	if req.BanSeconds == 0 {
		n := 0
		if h.conns.closeConn(id) {
			n = 1
		}
		httpx.OK(w, closeConnResult{Closed: n})
		return
	}
	addr := a.client.Addr()
	if p := httpx.ClientIP(r); p != "" && limitKey(p) == limitKeyAddr(addr) {
		httpx.Error(w, http.StatusBadRequest, "不能封禁你当前使用的地址")
		return
	}
	if msg := m.unbannableReason(addr); msg != "" {
		httpx.Error(w, http.StatusBadRequest, "%s", msg)
		return
	}
	key := m.auth.ban(addr.String(), time.Duration(req.BanSeconds)*time.Second, "从实时连接封禁", true)
	if key == "" {
		httpx.Error(w, http.StatusInternalServerError, "封禁失败")
		return
	}
	// 封禁只拦截新请求，已在进行中的请求（如 WebSocket、大文件下载）需要一并断开
	n := m.closeConnsByKey(key)
	httpx.OK(w, closeConnResult{Closed: n, BanKey: key})
}

// unbannableReason 返回不能从实时连接封禁该地址的原因，可以封禁时返回空串。
// 手动封禁会绕过入侵防护的白名单，这里挡住封禁后会拦截一大批无关访客的地址：
// 回环与未指定地址（经 frp、未带 PROXY 协议的本机转发进来的请求都显示为它），以及可信代理（取不到真实 IP 时列表里显示的是代理地址）。
// IPv6 按 /64 封禁，只要与可信代理网段有重叠也拒绝。
func (m *Module) unbannableReason(addr netip.Addr) string {
	addr = addr.Unmap()
	if !addr.IsValid() || addr.IsLoopback() || addr.IsUnspecified() {
		return "不能封禁回环地址：经本机转发（如 frp、未带 PROXY 协议的端口转发）进来的请求都显示为该地址，封禁会拦截全部这类访客"
	}
	bits := 32
	if addr.Is6() {
		bits = 64
	}
	banned, _ := addr.Prefix(bits)
	if p := m.realIP.p.Load(); p != nil {
		for _, pf := range p.from {
			if pf.Overlaps(banned) {
				return "该地址属于可信代理，封禁会拦截经它访问的全部访客；请检查真实 IP 请求头设置是否取到了访客地址"
			}
		}
	}
	return ""
}

// closeConnsByKey 断开全部监听、全部站点上访客地址属于 key（单个地址或 IPv6 /64 网段）的进行中请求。
func (m *Module) closeConnsByKey(key string) int {
	var trackers []*connTracker
	seen := map[*connTracker]bool{}
	m.mu.Lock()
	for _, run := range m.runners {
		for _, s := range run.current().all() {
			if !seen[s.conns] {
				seen[s.conns] = true
				trackers = append(trackers, s.conns)
			}
		}
	}
	m.mu.Unlock()
	n := 0
	for _, t := range trackers {
		n += t.closeWhere(func(a *activeReq) bool {
			return limitKeyAddr(a.client.Addr()) == key
		})
	}
	return n
}
