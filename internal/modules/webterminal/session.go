package webterminal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/i18n"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// janitorInterval 是后台回收（空闲超时、断线保留超时）的检查间隔，测试可调小。
var janitorInterval = 5 * time.Second

// clientQueueSize 是每个客户端的待发送帧队列长度，写满说明客户端过慢，将被踢出（可重新附着回放）。
const clientQueueSize = 1024

// frame 是待发送给浏览器的一帧。
type frame struct {
	typ  websocket.MessageType
	data []byte
}

func textFrame(t, d string) frame {
	m := map[string]string{"t": t}
	if d != "" {
		m["d"] = d
	}
	b, _ := json.Marshal(m)
	return frame{typ: websocket.MessageText, data: b}
}

// client 是附着在会话上的一个 WebSocket 客户端，由独立的写协程串行发送。
type client struct {
	ws   *websocket.Conn
	send chan frame
	quit chan struct{}
	done chan struct{}
	once sync.Once
}

func newClient(ws *websocket.Conn) *client {
	return &client{ws: ws, send: make(chan frame, clientQueueSize), quit: make(chan struct{}), done: make(chan struct{})}
}

// enqueue 非阻塞入队，队列已满返回 false。
func (c *client) enqueue(f frame) bool {
	select {
	case c.send <- f:
		return true
	default:
		return false
	}
}

// finish 通知写协程发完队列中剩余的帧后关闭连接。
func (c *client) finish() {
	c.once.Do(func() {
		close(c.quit)
	})
}

// writer 串行发送队列中的帧，直到 finish 或 ctx 取消。
func (c *client) writer(ctx context.Context) {
	defer close(c.done)
	write := func(f frame) bool {
		wctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		if err := c.ws.Write(wctx, f.typ, f.data); err != nil {
			_ = c.ws.CloseNow()
			return false
		}
		return true
	}
	for {
		select {
		case f := <-c.send:
			if !write(f) {
				return
			}
		case <-c.quit:
			for {
				select {
				case f := <-c.send:
					if !write(f) {
						return
					}
				default:
					_ = c.ws.Close(websocket.StatusNormalClosure, "")
					return
				}
			}
		case <-ctx.Done():
			_ = c.ws.CloseNow()
			return
		}
	}
}

// hubSession 是一个持久终端会话：独立于浏览器连接存活，输出写入环形缓冲供重连回放。
type hubSession struct {
	hub       *hub
	id        string
	connKey   string
	connName  string
	typ       string
	createdAt time.Time
	log       *logbuf.Logger
	sess      session

	// wmu 串行化向终端写入与调整大小
	wmu sync.Mutex

	mu         sync.Mutex
	remark     string
	clients    map[*client]struct{}
	ring       []byte
	ringCap    int
	cols, rows int
	detachedAt time.Time
	ended      bool

	lastActive atomic.Int64
	bytesIn    atomic.Int64
	bytesOut   atomic.Int64
}

// sessionView 是会话的对外描述。
type sessionView struct {
	ID         string    `json:"id"`
	ConnKey    string    `json:"connKey"`
	ConnName   string    `json:"connName"`
	Type       string    `json:"type"`
	Remark     string    `json:"remark"`
	CreatedAt  time.Time `json:"createdAt"`
	LastActive time.Time `json:"lastActive"`
	// Attached 为当前附着的客户端数量
	Attached int   `json:"attached"`
	Cols     int   `json:"cols"`
	Rows     int   `json:"rows"`
	BytesIn  int64 `json:"bytesIn"`
	BytesOut int64 `json:"bytesOut"`
}

func (s *hubSession) view() sessionView {
	s.mu.Lock()
	defer s.mu.Unlock()
	return sessionView{
		ID: s.id, ConnKey: s.connKey, ConnName: s.connName, Type: s.typ, Remark: s.remark,
		CreatedAt: s.createdAt, LastActive: time.Unix(0, s.lastActive.Load()),
		Attached: len(s.clients), Cols: s.cols, Rows: s.rows,
		BytesIn: s.bytesIn.Load(), BytesOut: s.bytesOut.Load(),
	}
}

func (s *hubSession) touch() {
	s.lastActive.Store(time.Now().UnixNano())
}

// attach 把客户端挂到会话上：先发送会话编号，再回放缓冲，最后加入广播列表（三步在同一把锁内，不丢不重）。
func (s *hubSession) attach(c *client, cols, rows int) error {
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		return errors.New("会话已结束")
	}
	c.enqueue(textFrame("s", s.id))
	snap := replaySnapshot(s.ring)
	for len(snap) > 0 {
		n := min(len(snap), 32*1024)
		c.enqueue(frame{typ: websocket.MessageBinary, data: snap[:n]})
		snap = snap[n:]
	}
	s.clients[c] = struct{}{}
	s.detachedAt = time.Time{}
	s.mu.Unlock()
	if cols > 0 && rows > 0 {
		s.resize(cols, rows)
	}
	return nil
}

// replaySnapshot 复制缓冲，并跳过开头被截断的 UTF-8 续字节。
func replaySnapshot(ring []byte) []byte {
	out := append([]byte(nil), ring...)
	skip := 0
	for skip < len(out) && skip < 3 && out[skip]&0xC0 == 0x80 {
		skip++
	}
	return out[skip:]
}

// detach 摘除客户端；没有客户端后按 sessionKeepAlive 决定保留还是立即结束。
func (s *hubSession) detach(c *client) {
	s.mu.Lock()
	if _, ok := s.clients[c]; !ok {
		s.mu.Unlock()
		return
	}
	delete(s.clients, c)
	empty := len(s.clients) == 0 && !s.ended
	if empty {
		s.detachedAt = time.Now()
	}
	s.mu.Unlock()
	if empty && s.hub.keepAliveMinutes() <= 0 {
		s.end("浏览器已断开")
	}
}

// output 记录终端输出并广播给全部客户端。
func (s *hubSession) output(p []byte) {
	chunk := append([]byte(nil), p...)
	s.bytesOut.Add(int64(len(p)))
	s.touch()
	var slow []*client
	s.mu.Lock()
	s.ring = append(s.ring, chunk...)
	// 缓冲超过两倍容量时压缩，避免每次都搬移数据
	if len(s.ring) > 2*s.ringCap {
		s.ring = append([]byte(nil), s.ring[len(s.ring)-s.ringCap:]...)
	}
	for c := range s.clients {
		if !c.enqueue(frame{typ: websocket.MessageBinary, data: chunk}) {
			slow = append(slow, c)
		}
	}
	s.mu.Unlock()
	for _, c := range slow {
		s.detach(c)
		c.finish()
	}
}

// input 把浏览器输入写入终端。
func (s *hubSession) input(data string) {
	s.bytesIn.Add(int64(len(data)))
	s.touch()
	s.wmu.Lock()
	_, err := s.sess.Write([]byte(data))
	s.wmu.Unlock()
	if err != nil {
		s.end("写入终端失败")
	}
}

func (s *hubSession) resize(cols, rows int) {
	s.mu.Lock()
	s.cols, s.rows = cols, rows
	s.mu.Unlock()
	s.wmu.Lock()
	_ = s.sess.Resize(cols, rows)
	s.wmu.Unlock()
}

// end 结束会话：通知所有客户端、关闭终端并从会话表移除。reason 用于客户端提示。
func (s *hubSession) end(reason string) {
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		return
	}
	s.ended = true
	clients := make([]*client, 0, len(s.clients))
	for c := range s.clients {
		clients = append(clients, c)
	}
	s.clients = map[*client]struct{}{}
	started := s.createdAt
	s.mu.Unlock()
	s.hub.remove(s.id)
	for _, c := range clients {
		c.enqueue(textFrame("x", reason))
		c.finish()
	}
	_ = s.sess.Close()
	s.log.Infof("会话结束（%s），持续 %s", reason, time.Since(started).Round(time.Second))
}

// pump 持续读取终端输出，直到终端退出。
func (s *hubSession) pump() {
	buf := make([]byte, 32*1024)
	for {
		n, err := s.sess.Read(buf)
		if n > 0 {
			s.output(buf[:n])
		}
		if err != nil {
			break
		}
	}
	s.end("会话已退出")
}

// expireCheck 检查空闲超时与断线保留超时。
func (s *hubSession) expireCheck(now time.Time, idleMin, keepMin int) {
	s.mu.Lock()
	reason := ""
	switch {
	case s.ended:
	case idleMin > 0 && now.Sub(time.Unix(0, s.lastActive.Load())) > time.Duration(idleMin)*time.Minute:
		reason = "空闲超时，会话已结束"
	case len(s.clients) == 0 && !s.detachedAt.IsZero() && keepMin > 0 && now.Sub(s.detachedAt) > time.Duration(keepMin)*time.Minute:
		reason = "断线保留超时，会话已结束"
	}
	s.mu.Unlock()
	if reason != "" {
		s.end(reason)
	}
}

// hub 是会话表。
type hub struct {
	mu       sync.Mutex
	sessions map[string]*hubSession
	reserved int

	// janitor 控制后台回收协程
	janitorStop chan struct{}

	// settings 由模块提供，返回当前设置快照
	settings func() Settings
}

func newHub(settings func() Settings) *hub {
	return &hub{sessions: map[string]*hubSession{}, settings: settings}
}

func (h *hub) keepAliveMinutes() int { return h.settings().SessionKeepAlive }

// reserve 占用一个会话名额，超过上限返回 false；占位避免并发请求同时通过检查。
func (h *hub) reserve(max int) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.sessions)+h.reserved >= max {
		return false
	}
	h.reserved++
	return true
}

func (h *hub) unreserve() {
	h.mu.Lock()
	h.reserved--
	h.mu.Unlock()
}

// commit 把占位转为真实会话。
func (h *hub) commit(s *hubSession) {
	h.mu.Lock()
	h.reserved--
	h.sessions[s.id] = s
	h.mu.Unlock()
}

func (h *hub) remove(id string) {
	h.mu.Lock()
	delete(h.sessions, id)
	h.mu.Unlock()
}

func (h *hub) get(id string) *hubSession {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sessions[id]
}

func (h *hub) all() []*hubSession {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]*hubSession, 0, len(h.sessions))
	for _, s := range h.sessions {
		out = append(out, s)
	}
	return out
}

func (h *hub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.sessions)
}

// closeAll 结束全部会话并停止后台回收。
func (h *hub) closeAll(reason string) {
	h.mu.Lock()
	if h.janitorStop != nil {
		close(h.janitorStop)
		h.janitorStop = nil
	}
	h.mu.Unlock()
	for _, s := range h.all() {
		s.end(reason)
	}
}

// ensureJanitor 启动后台回收协程（幂等）。
func (h *hub) ensureJanitor() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.janitorStop != nil {
		return
	}
	stop := make(chan struct{})
	h.janitorStop = stop
	interval := janitorInterval
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case now := <-t.C:
				st := h.settings()
				for _, s := range h.all() {
					s.expireCheck(now, st.IdleTimeout, st.SessionKeepAlive)
				}
			}
		}
	}()
}

func newSessionID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// parseSize 读取 cols / rows 查询参数。
func parseSize(r *http.Request) (cols, rows int) {
	cols, _ = strconv.Atoi(r.URL.Query().Get("cols"))
	rows, _ = strconv.Atoi(r.URL.Query().Get("rows"))
	if cols <= 0 || rows <= 0 {
		return 0, 0
	}
	return cols, rows
}

// clientMsg 是浏览器发来的消息：t=i 输入，t=r 调整窗口大小，t=p 心跳。
type clientMsg struct {
	T string `json:"t"`
	D string `json:"d"`
	C int    `json:"c"`
	R int    `json:"r"`
}

// session 是一个已建立的终端会话的抽象。
type session interface {
	io.ReadWriter
	Resize(cols, rows int) error
	Close() error
}

// settingsSnapshot 返回当前设置副本。
func (m *Module) settingsSnapshot() Settings {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg.Settings
}

// findConn 返回连接配置的副本。
func (m *Module) findConn(key string) (Connection, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.cfg.Connections {
		if m.cfg.Connections[i].Key == key {
			return m.cfg.Connections[i], true
		}
	}
	return Connection{}, false
}

// handleWS 新建会话并把当前 WebSocket 附着上去。
func (m *Module) handleWS(w http.ResponseWriter, r *http.Request) {
	if !m.requireAccess(w, r) {
		return
	}
	conn, ok := m.findConn(r.PathValue("key"))
	if !ok {
		httpx.Error(w, http.StatusNotFound, "连接不存在")
		return
	}
	settings := m.settingsSnapshot()
	if !m.hub.reserve(settings.MaxSessions) {
		httpx.Error(w, http.StatusTooManyRequests, "会话数已达上限 %d", settings.MaxSessions)
		return
	}
	reserved := true
	defer func() {
		if reserved {
			m.hub.unreserve()
		}
	}()
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	ws.SetReadLimit(1 << 20)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	cols, rows := parseSize(r)
	if cols == 0 {
		cols, rows = 80, 24
	}
	// 会话建立之前的提示直接写入连接
	notice := func(msg string) {
		f := textFrame("n", msg)
		_ = ws.Write(ctx, f.typ, f.data)
	}
	log := m.logs.Logger(conn.Name)

	var sess session
	switch conn.Type {
	case TypeLocal:
		if !settings.AllowLocalShell {
			notice("本机 Shell 未开启，请在 Web 终端设置中允许后再使用")
			_ = ws.Close(websocket.StatusPolicyViolation, "local shell disabled")
			return
		}
		sess, err = startLocal(conn.Local, cols, rows)
		// 本机 Shell 运行在 Gately 容器内时，在这里停止或重建本容器的命令会随 Gately 一起被终止
		if err == nil && inContainer() {
			notice(i18n.Tr(i18n.RequestLang(r), containerShellNotice))
		}
	case TypeTelnet:
		sess, err = startTelnet(ctx, &conn, cols, rows)
	default:
		sess, err = m.startSSH(ctx, &conn, cols, rows, notice)
	}
	if err != nil {
		log.Warnf("连接失败: %v", err)
		notice("连接失败: " + err.Error())
		_ = ws.Close(websocket.StatusNormalClosure, "")
		return
	}

	hs := &hubSession{
		hub: m.hub, id: newSessionID(), connKey: conn.Key, connName: conn.Name, typ: conn.Type,
		createdAt: time.Now(), log: log, sess: sess,
		clients: map[*client]struct{}{}, ringCap: max(settings.BufferSize, 1) * 1024,
		cols: cols, rows: rows,
	}
	hs.touch()
	m.hub.commit(hs)
	reserved = false
	m.hub.ensureJanitor()
	log.Infof("会话开始（来自 %s）", r.RemoteAddr)

	c := newClient(ws)
	go c.writer(ctx)
	_ = hs.attach(c, 0, 0)
	go hs.pump()
	m.serveClient(ctx, ws, hs, c, settings.HeartbeatInterval)
}

// handleAttach 附着到已有会话。
func (m *Module) handleAttach(w http.ResponseWriter, r *http.Request) {
	if !m.requireAccess(w, r) {
		return
	}
	hs := m.hub.get(r.PathValue("id"))
	if hs == nil {
		httpx.Error(w, http.StatusNotFound, "会话不存在或已结束")
		return
	}
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	ws.SetReadLimit(1 << 20)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	c := newClient(ws)
	go c.writer(ctx)
	cols, rows := parseSize(r)
	if err := hs.attach(c, cols, rows); err != nil {
		c.enqueue(textFrame("x", err.Error()))
		c.finish()
		<-c.done
		return
	}
	hs.log.Infof("客户端附着（来自 %s）", r.RemoteAddr)
	m.serveClient(ctx, ws, hs, c, m.settingsSnapshot().HeartbeatInterval)
}

// serveClient 处理一个已附着客户端的读循环与服务端心跳，返回时客户端已摘除。
func (m *Module) serveClient(ctx context.Context, ws *websocket.Conn, hs *hubSession, c *client, heartbeat int) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if heartbeat <= 0 {
		heartbeat = 30
	}
	interval := time.Duration(heartbeat) * time.Second
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				pctx, pcancel := context.WithTimeout(ctx, max(2*interval, 10*time.Second))
				err := ws.Ping(pctx)
				pcancel()
				if err != nil {
					_ = ws.CloseNow()
					return
				}
			}
		}
	}()
	for {
		_, data, err := ws.Read(ctx)
		if err != nil {
			break
		}
		var msg clientMsg
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		switch msg.T {
		case "i":
			hs.input(msg.D)
		case "r":
			if msg.C > 0 && msg.R > 0 {
				hs.resize(msg.C, msg.R)
			}
		case "p":
			c.enqueue(textFrame("p", ""))
		}
	}
	hs.detach(c)
	c.finish()
	<-c.done
}

func (m *Module) registerSessionRoutes(r module.Router) {
	r.HandleFunc("GET /connections/{key}/ws", m.handleWS)
	r.HandleFunc("GET /sessions/{id}/ws", m.handleAttach)
	r.HandleFunc("GET /sessions", func(w http.ResponseWriter, req *http.Request) {
		list := m.hub.all()
		out := make([]sessionView, 0, len(list))
		for _, s := range list {
			out = append(out, s.view())
		}
		sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
		httpx.OK(w, out)
	})
	r.HandleFunc("GET /sessions/{id}", func(w http.ResponseWriter, req *http.Request) {
		s := m.hub.get(req.PathValue("id"))
		if s == nil {
			httpx.Error(w, http.StatusNotFound, "会话不存在或已结束")
			return
		}
		httpx.OK(w, s.view())
	})
	r.HandleFunc("PUT /sessions/{id}/remark", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Remark string `json:"remark"`
		}
		if err := httpx.ReadJSON(req, &body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		s := m.hub.get(req.PathValue("id"))
		if s == nil {
			httpx.Error(w, http.StatusNotFound, "会话不存在或已结束")
			return
		}
		s.mu.Lock()
		s.remark = body.Remark
		s.mu.Unlock()
		httpx.OK(w, map[string]bool{"ok": true})
	})
	r.HandleFunc("GET /sessions/{id}/stats", func(w http.ResponseWriter, req *http.Request) {
		s := m.hub.get(req.PathValue("id"))
		if s == nil {
			httpx.Error(w, http.StatusNotFound, "会话不存在或已结束")
			return
		}
		v := s.view()
		now := time.Now()
		httpx.OK(w, map[string]any{
			"bytesIn": v.BytesIn, "bytesOut": v.BytesOut,
			"durationSec": int64(now.Sub(v.CreatedAt).Seconds()),
			"idleSec":     int64(now.Sub(v.LastActive).Seconds()),
			"attached":    v.Attached, "cols": v.Cols, "rows": v.Rows,
		})
	})
	r.HandleFunc("DELETE /sessions/{id}", func(w http.ResponseWriter, req *http.Request) {
		s := m.hub.get(req.PathValue("id"))
		if s == nil {
			httpx.Error(w, http.StatusNotFound, "会话不存在或已结束")
			return
		}
		s.end("会话已被手动结束")
		httpx.OK(w, map[string]bool{"ok": true})
	})
}
