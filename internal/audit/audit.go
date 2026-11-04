// Package audit 记录管理后台的操作审计日志：谁、何时、从哪个 IP、做了什么。
//
// 日志以 JSON Lines 追加写入配置目录下的 audit.log，超过大小上限后轮转。
// 审计只记录方法、路径、结果与操作者，不记录请求体，因此不会泄露密钥。
package audit

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/httpx"
)

// 操作类别
const (
	CategoryLogin  = "login"  // 登录、登出
	CategoryChange = "change" // 配置或状态变更
	CategoryAccess = "access" // 读取敏感内容（备份、诊断包、终端连接等）
)

// Entry 是一条审计记录。
type Entry struct {
	Time time.Time `json:"time"`
	// Actor 为操作者：用户名，或 api-token:<令牌名>
	Actor string `json:"actor"`
	IP    string `json:"ip"`
	// Via 为转发该请求的本机代理地址；直连时为空
	Via string `json:"via,omitempty"`
	// Category 见 Category* 常量
	Category string `json:"category"`
	// Module 为所属模块 ID，系统级操作为空
	Module string `json:"module,omitempty"`
	Method string `json:"method,omitempty"`
	Path   string `json:"path,omitempty"`
	// Status 为 HTTP 状态码，非请求类记录为 0
	Status int `json:"status,omitempty"`
	// Detail 为补充说明（例如登录失败原因）
	Detail string `json:"detail,omitempty"`
}

// Log 是审计日志文件的句柄。
type Log struct {
	path     string
	maxBytes int64
	keep     int
	mu       sync.Mutex
	// hook 在每条记录写入后调用，用于联动通知中心；为 nil 时不调用
	hook func(Entry)
}

// 默认轮转参数：单文件 5 MiB，保留 3 份历史。
const (
	defaultMaxBytes = 5 << 20
	defaultKeep     = 3
)

// New 返回写入 dir/audit.log 的审计日志。
func New(dir string) *Log {
	return &Log{path: filepath.Join(dir, "audit.log"), maxBytes: defaultMaxBytes, keep: defaultKeep}
}

// SetHook 设置记录写入后的回调（在写入协程内同步调用，应当快速返回）。
func (l *Log) SetHook(h func(Entry)) {
	l.mu.Lock()
	l.hook = h
	l.mu.Unlock()
}

// Record 追加一条记录。写入失败只丢弃该条，不影响业务请求。
func (l *Log) Record(e Entry) {
	if l == nil {
		return
	}
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	line = append(line, '\n')
	l.mu.Lock()
	l.rotateLocked()
	if f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		// 部分文件系统会忽略创建权限（见 fsperm）；审计记录写入不频繁，每次都显式设置
		_ = f.Chmod(0o600)
		_, _ = f.Write(line)
		_ = f.Close()
	}
	hook := l.hook
	l.mu.Unlock()
	if hook != nil {
		hook(e)
	}
}

// rotateLocked 在主文件超限时依次后移历史文件：audit.log -> audit.log.1 -> audit.log.2 ...
func (l *Log) rotateLocked() {
	st, err := os.Stat(l.path)
	if err != nil || st.Size() < l.maxBytes {
		return
	}
	_ = os.Remove(l.path + "." + itoa(l.keep))
	for i := l.keep - 1; i >= 1; i-- {
		_ = os.Rename(l.path+"."+itoa(i), l.path+"."+itoa(i+1))
	}
	_ = os.Rename(l.path, l.path+".1")
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

// Query 是查询条件，空字段不参与过滤。
type Query struct {
	Category string
	Module   string
	Actor    string
	// Text 在路径、操作者、IP、详情中做不区分大小写的子串匹配
	Text string
	// Limit 默认 100，上限 1000
	Limit int
	// Offset 跳过最新的若干条匹配记录，用于翻页
	Offset int
}

// Result 是一页查询结果。
type Result struct {
	Entries []Entry `json:"entries"`
	// Total 为匹配的总条数（仅统计保留的日志文件）
	Total int `json:"total"`
}

// Search 按时间倒序返回匹配记录。
func (l *Log) Search(q Query) Result {
	if q.Limit <= 0 {
		q.Limit = 100
	}
	if q.Limit > 1000 {
		q.Limit = 1000
	}
	text := strings.ToLower(q.Text)
	l.mu.Lock()
	var files [][]byte
	// 先读最新的文件，再读历史文件
	files = append(files, readFile(l.path))
	for i := 1; i <= l.keep; i++ {
		files = append(files, readFile(l.path+"."+itoa(i)))
	}
	l.mu.Unlock()

	res := Result{Entries: []Entry{}}
	for _, data := range files {
		lines := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
		for i := len(lines) - 1; i >= 0; i-- {
			if len(lines[i]) == 0 {
				continue
			}
			var e Entry
			if json.Unmarshal(lines[i], &e) != nil || !q.match(e, text) {
				continue
			}
			if res.Total >= q.Offset && len(res.Entries) < q.Limit {
				res.Entries = append(res.Entries, e)
			}
			res.Total++
		}
	}
	return res
}

func (q Query) match(e Entry, text string) bool {
	if q.Category != "" && e.Category != q.Category {
		return false
	}
	if q.Module != "" && e.Module != q.Module {
		return false
	}
	if q.Actor != "" && e.Actor != q.Actor {
		return false
	}
	if text != "" {
		hay := strings.ToLower(e.Path + " " + e.Actor + " " + e.IP + " " + e.Via + " " + e.Detail + " " + e.Method)
		if !strings.Contains(hay, text) {
			return false
		}
	}
	return true
}

func readFile(p string) []byte {
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(bufio.NewReader(f))
	return buf.Bytes()
}

// statusWriter 记录响应状态码，同时保留 Hijacker、Flusher 等接口（WebSocket 终端需要）。
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

// Hijack 转发给底层连接，WebSocket 升级依赖它。经 ResponseController 沿 Unwrap 链查找，
// 内层是只实现 Unwrap 的包装（如统一网关的 gatewayWriter）时也能接管连接。
func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err != nil {
		return nil, nil, err
	}
	// 升级成功后不会再写响应头，按 101 记录
	if w.status == 0 {
		w.status = http.StatusSwitchingProtocols
	}
	return conn, rw, nil
}

// Flush 转发给底层写入器，保证流式响应及时发送。
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// sensitiveReads 是会读取敏感内容或建立交互通道的 GET 路径，需要记录访问。
func sensitiveRead(path string) bool {
	switch path {
	case "/api/system/backup", "/api/system/diagnostics":
		return true
	}
	// 只有下载本地备份文件才算敏感读取；备份页会轮询文件列表 /api/backup/local，
	// 列表只含文件名与大小，记录它会持续刷审计并不断触发敏感访问通知
	if strings.HasPrefix(path, "/api/backup/local/") {
		return true
	}
	for _, seg := range strings.Split(path, "/") {
		switch seg {
		case "ws", "exec", "download", "export", "secrets":
			return true
		}
	}
	return false
}

// skipped 是只读性质的 POST 或高频接口，不写入审计。
func skipped(method, path string) bool {
	// 记录管理界面语言只是界面偏好：进入管理界面时也会上报，记入审计会刷屏并触发配置变更通知
	if method == http.MethodPut && path == "/api/account/ui-language" {
		return true
	}
	if method != http.MethodPost {
		return false
	}
	// 网络诊断、自检、Passkey 注册参数下发都只是读取或计算，不改变配置
	return strings.HasPrefix(path, "/api/nettools/") || path == "/api/system/selfcheck" || path == "/api/account/passkeys/begin"
}

// Middleware 记录经过认证后的变更类请求与敏感读取。必须挂在认证中间件之内，才能取到操作者。
func (l *Log) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutating := r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions
		category := CategoryChange
		if !mutating {
			if !sensitiveRead(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			category = CategoryAccess
		} else if skipped(r.Method, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		status := sw.status
		if status == 0 {
			status = http.StatusOK
		}
		l.Record(Entry{
			Actor:    auth.UserFromContext(r.Context()),
			IP:       httpx.ClientIP(r),
			Via:      Via(r),
			Category: category,
			Module:   moduleOf(r.URL.Path),
			Method:   r.Method,
			Path:     r.URL.Path,
			Status:   status,
		})
	})
}

// Via 返回转发该请求的代理地址：客户端地址取自代理请求头时为 TCP 对端，直连时为空；
// 经由已标记的代理（如飞牛统一网关）时为代理名称。
func Via(r *http.Request) string {
	if v := httpx.ViaOf(r); v != "" {
		return v
	}
	if peer := httpx.PeerIP(r); peer != httpx.ClientIP(r) {
		return peer
	}
	return ""
}

// moduleOf 从 /api/<模块>/... 提取模块 ID；系统级路径返回空。
func moduleOf(path string) string {
	rest := strings.TrimPrefix(path, "/api/")
	mod, _, _ := strings.Cut(rest, "/")
	switch mod {
	case "", "account", "system", "tokens", "import", "logs", "session", "status", "login", "logout":
		return ""
	}
	return mod
}
