package waf

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/i18n"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/webpage"
)

// SafeLine 是接入长亭雷池检测引擎（safeline-detector）的设置。雷池由用户自行部署，
// Gately 只把请求送去检测并按结果放行或拦截，入口、证书、认证与封禁仍由 Gately 负责。
type SafeLine struct {
	Enable bool `json:"enable"`
	// Addr 为检测引擎地址：host:port，或 unix: 开头的 Unix 套接字路径（如 unix:/data/safeline/resources/detector/snserver.sock）
	Addr string `json:"addr"`
	// TimeoutMs 为单次检测的超时（毫秒），0 表示默认 1000
	TimeoutMs int `json:"timeoutMs"`
	// BodyLimitKB 为送检的请求体上限（KB），超出部分不送检、原样转发给后端，0 表示默认 1024
	BodyLimitKB int `json:"bodyLimitKB"`
	// FailClosed 为 true 时检测引擎不可用（连接失败、超时）则拒绝请求并返回 503；默认放行，避免雷池故障拖垮站点
	FailClosed bool `json:"failClosed"`
}

// 雷池接入的默认值与范围，默认值与官方 lua-resty-t1k 一致
const (
	defaultSafeLineTimeoutMs = 1000
	maxSafeLineTimeoutMs     = 30000
	defaultSafeLineBodyKB    = 1024
	maxSafeLineBodyKB        = 64 * 1024
)

// validate 校验雷池设置并补全默认值；未启用时只做范围检查，便于先保存地址再启用。
func (s *SafeLine) validate() error {
	s.Addr = strings.TrimSpace(s.Addr)
	if s.TimeoutMs == 0 {
		s.TimeoutMs = defaultSafeLineTimeoutMs
	}
	if s.TimeoutMs < 10 || s.TimeoutMs > maxSafeLineTimeoutMs {
		return fmt.Errorf("雷池检测超时应为 10~%d 毫秒", maxSafeLineTimeoutMs)
	}
	if s.BodyLimitKB == 0 {
		s.BodyLimitKB = defaultSafeLineBodyKB
	}
	if s.BodyLimitKB < 0 || s.BodyLimitKB > maxSafeLineBodyKB {
		return fmt.Errorf("雷池送检请求体上限应为 0~%d KB", maxSafeLineBodyKB)
	}
	if !s.Enable && s.Addr == "" {
		return nil
	}
	return validateSafeLineAddr(s.Addr)
}

// validateSafeLineAddr 检查检测引擎地址的格式。
func validateSafeLineAddr(addr string) error {
	if addr == "" {
		return errors.New("请填写雷池检测引擎地址")
	}
	if p, ok := strings.CutPrefix(addr, "unix:"); ok {
		if !strings.HasPrefix(p, "/") {
			return errors.New("雷池检测引擎的 Unix 套接字应为绝对路径，如 unix:/data/safeline/resources/detector/snserver.sock")
		}
		return nil
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil || host == "" {
		return fmt.Errorf("雷池检测引擎地址 %q 无效，应为 主机:端口（如 172.22.222.5:8000）", addr)
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("雷池检测引擎地址 %q 的端口无效", addr)
	}
	return nil
}

// safelineErrLogEvery 是检测引擎不可用时写错误日志的最小间隔，避免每个请求都刷一条
const safelineErrLogEvery = 30 * time.Second

// safelineChecker 是实例中雷池检测的运行状态。
type safelineChecker struct {
	cfg       SafeLine
	detect    bool
	client    *t1kClient
	bodyLimit int64
	skipPaths []string
	webdav    bool
	log       *logbuf.Logger
	name      string

	// errors 为检测失败次数；lastErr 为最近一次失败的原因，成功后清空
	errors     atomic.Int64
	mu         sync.Mutex
	lastErr    string
	lastLogged time.Time
}

func newSafeLineChecker(in *Instance, log *logbuf.Logger) *safelineChecker {
	s := in.SafeLine
	return &safelineChecker{
		cfg:       s,
		detect:    in.Mode == ModeDetect,
		client:    newT1KClient(s.Addr, time.Duration(s.TimeoutMs)*time.Millisecond),
		bodyLimit: int64(s.BodyLimitKB) << 10,
		skipPaths: splitLines(in.SkipBodyPaths),
		webdav:    in.WebDAV,
		log:       log,
		name:      in.Name,
	}
}

// status 返回最近一次检测失败的原因，正常时为空。
func (s *safelineChecker) status() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

func (s *safelineChecker) recordErr(err error) {
	s.errors.Add(1)
	now := time.Now()
	s.mu.Lock()
	s.lastErr = err.Error()
	logIt := now.Sub(s.lastLogged) >= safelineErrLogEvery
	if logIt {
		s.lastLogged = now
	}
	s.mu.Unlock()
	if logIt {
		action := "已放行请求"
		if s.cfg.FailClosed {
			action = "已拒绝请求"
		}
		s.log.Errorf("雷池检测引擎 %s 不可用（%s）: %v", s.cfg.Addr, action, err)
	}
}

func (s *safelineChecker) recordOK() {
	s.mu.Lock()
	s.lastErr = ""
	s.mu.Unlock()
}

// skipBody 判断请求体是否不送检：与 Coraza 的设置一致，按路径跳过，WebDAV 兼容模式下文件内容不检查。
func (s *safelineChecker) skipBody(r *http.Request) bool {
	for _, p := range s.skipPaths {
		if strings.HasPrefix(r.URL.Path, p) {
			return true
		}
	}
	if s.webdav && (r.Method == http.MethodPut || strings.HasPrefix(r.Header.Get("Content-Type"), "application/octet-stream")) {
		return true
	}
	return false
}

// streamingContentType 判断长度未知的请求体是否可能是双向流（gRPC 等）：
// 这类请求体要等后端响应后客户端才继续发送，先读满上限再转发会卡死请求，因此不送检。
func streamingContentType(ct string) bool {
	return strings.HasPrefix(ct, "application/grpc") || strings.HasPrefix(ct, "application/connect")
}

// readBodyPrefix 读出请求体前面不超过 limit 的部分供送检，并把请求体还原为“已读部分 + 剩余部分”，后端收到的内容不变。
func readBodyPrefix(r *http.Request, limit int64) []byte {
	if r.Body == nil || r.Body == http.NoBody || r.ContentLength == 0 || limit <= 0 {
		return nil
	}
	// gRPC 只走 HTTP/2 及以上，HTTP/1.x 请求伪造 gRPC 的 Content-Type 不能借此跳过请求体检测
	if r.ContentLength < 0 && r.ProtoMajor >= 2 && streamingContentType(r.Header.Get("Content-Type")) {
		return nil
	}
	n := limit
	if r.ContentLength > 0 && r.ContentLength < n {
		n = r.ContentLength
	}
	var buf bytes.Buffer
	// 读取出错（访客中途断开等）时只送检已读到的部分，错误留给后续读取请求体的处理器
	_, _ = io.CopyN(&buf, r.Body, n)
	data := buf.Bytes()
	r.Body = &prefixedBody{Reader: io.MultiReader(bytes.NewReader(data), r.Body), Closer: r.Body}
	return data
}

// prefixedBody 把已读出的请求体前缀接回原请求体前面。
type prefixedBody struct {
	io.Reader
	io.Closer
}

// requestHeader 生成送检的请求行与请求头（HTTP/1.1 文本格式，HTTP/2、HTTP/3 请求同样按此格式送检）。
func requestHeader(r *http.Request) []byte {
	var b bytes.Buffer
	// 用访客原始的请求目标；绝对形式（http://host/path）与空值时改用解析后的路径与查询串
	uri := r.RequestURI
	if !strings.HasPrefix(uri, "/") {
		uri = r.URL.RequestURI()
	}
	proto := r.Proto
	if proto == "" {
		proto = "HTTP/1.1"
	}
	fmt.Fprintf(&b, "%s %s %s\r\nHost: %s\r\n", r.Method, uri, proto, r.Host)
	h := r.Header
	// Go 会把 Content-Length 从请求头中移到 ContentLength 字段，送检时补回，便于检测引擎按长度解析请求体
	if r.ContentLength > 0 && h.Get("Content-Length") == "" {
		h = h.Clone()
		h.Set("Content-Length", strconv.FormatInt(r.ContentLength, 10))
	}
	_ = h.Write(&b)
	b.WriteString("\r\n")
	return b.Bytes()
}

// requestExtra 生成 Extra 段：来源与本地地址、域名、协议等，检测引擎据此记录攻击来源。
func requestExtra(r *http.Request, begin time.Time) []byte {
	remote, rport := "", uint16(0)
	if ap, err := netip.ParseAddrPort(r.RemoteAddr); err == nil {
		remote, rport = ap.Addr().Unmap().String(), ap.Port()
	}
	local, lport := "127.0.0.1", uint16(80)
	if r.TLS != nil {
		lport = 443
	}
	if a, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		if ap, err := netip.ParseAddrPort(a.String()); err == nil {
			local, lport = ap.Addr().Unmap().String(), ap.Port()
		}
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	var b strings.Builder
	line := func(k, v string) {
		b.WriteString(k)
		b.WriteString(v)
		b.WriteByte('\n')
	}
	line(t1kExtraNewVer, "")
	line(t1kExtraRemotePort, strconv.Itoa(int(rport)))
	line(t1kExtraLocalPort, strconv.Itoa(int(lport)))
	line(t1kExtraRemoteAddr, remote)
	line(t1kExtraLocalAddr, local)
	line(t1kExtraServerName, hostOnly(r.Host))
	line(t1kExtraScheme, scheme)
	line(t1kExtraProxyName, "gately")
	line(t1kExtraUUID, t1kUUID())
	// 只检测请求，不送检响应
	line(t1kExtraHasRspIfOK, "n")
	line(t1kExtraHasRspIfBlk, "n")
	line(t1kExtraReqBeginTime, strconv.FormatInt(begin.UnixMicro(), 10))
	return []byte(b.String())
}

// detectRequest 把请求送去检测，返回检测结果。
func (s *safelineChecker) detectRequest(r *http.Request) (*t1kResult, error) {
	begin := time.Now()
	var body []byte
	if !s.skipBody(r) {
		body = readBodyPrefix(r, s.bodyLimit)
	}
	secs := []t1kSection{{tag: t1kTagHeader, data: requestHeader(r)}}
	if len(body) > 0 {
		secs = append(secs, t1kSection{tag: t1kTagBody, data: body})
	}
	secs = append(secs,
		t1kSection{tag: t1kTagExtra, data: requestExtra(r, begin)},
		t1kSection{tag: t1kTagVersion, data: []byte(t1kProto)},
	)
	res, err := s.client.roundTrip(secs)
	if err != nil {
		return nil, err
	}
	if res.action != t1kActionPass && res.action != t1kActionBlock {
		return nil, fmt.Errorf("未知的检测结果 %q", res.action)
	}
	return res, nil
}

// check 检测请求，返回 true 表示继续交给后续处理器；返回 false 时已写出拦截或 503 响应。
// c 用于累计实例的拦截与命中次数。
func (s *safelineChecker) check(w http.ResponseWriter, r *http.Request, c *compiled) bool {
	res, err := s.detectRequest(r)
	if err != nil {
		s.recordErr(err)
		if s.cfg.FailClosed {
			http.Error(w, "503 Service Unavailable", http.StatusServiceUnavailable)
			return false
		}
		return true
	}
	s.recordOK()
	if !res.blocked() {
		// 放行时检测引擎也可能要求附加响应头（如人机验证的 Cookie），拦截模式下照做
		if !s.detect {
			applyExtraHeaders(w, res)
		}
		return true
	}
	c.matched.Add(1)
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	host := hostOnly(r.Host)
	eventID := res.eventID()
	fields := logbuf.Fields{logbuf.FieldIP: ip, logbuf.FieldHost: host, logbuf.FieldPath: r.URL.Path}
	if eventID != "" {
		fields[logbuf.FieldRule] = eventID
	}
	if s.detect {
		s.log.With(fields).Warnf("雷池检测到攻击（仅检测，已放行）: %s %s %s%s，事件 ID %s", ip, r.Method, r.Host, r.URL.Path, orNone(eventID))
		return true
	}
	c.blocked.Add(1)
	s.log.With(fields).Warnf("雷池拦截了请求: %s %s %s%s，事件 ID %s", ip, r.Method, r.Host, r.URL.Path, orNone(eventID))
	events.Publish(events.Event{
		Type:    events.WAFBlocked,
		Source:  s.name,
		Level:   events.LevelWarn,
		Title:   "WAF 拦截了请求",
		Message: r.Method + " " + r.Host + r.URL.Path,
		Fields:  map[string]string{"ip": r.RemoteAddr, "host": r.Host, "path": r.URL.Path},
	})
	httpx.MarkWAFBlocked(r)
	writeSafeLineBlock(w, r, res, eventID)
	return false
}

func orNone(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// applyExtraHeaders 把检测引擎要求的附加响应头加到响应上。
// 只接受 Set-Cookie（如人机验证的 sl-session）与 X- 开头的头，长度、编码、跳转等会与后端响应冲突的头一律丢弃。
func applyExtraHeaders(w http.ResponseWriter, res *t1kResult) {
	for _, kv := range res.headers() {
		name := http.CanonicalHeaderKey(strings.TrimSpace(kv[0]))
		if name != "Set-Cookie" && !strings.HasPrefix(name, "X-") {
			continue
		}
		w.Header().Add(name, kv[1])
	}
}

// writeSafeLineBlock 写出拦截响应，状态码按检测引擎的建议（无效时为 403）。
// 检测引擎只返回事件 ID、不返回拦截页：浏览器看到 Gately 的拦截页并附上事件 ID，便于访客反馈误判；
// 其他客户端得到与官方插件相同的 JSON。
func writeSafeLineBlock(w http.ResponseWriter, r *http.Request, res *t1kResult, eventID string) {
	code, err := strconv.Atoi(strings.TrimSpace(res.status))
	if err != nil || code < 400 || code > 599 {
		code = http.StatusForbidden
	}
	applyExtraHeaders(w, res)
	w.Header().Set("Cache-Control", "no-store")
	if webpage.WantsHTML(r) {
		hint := ""
		if eventID != "" {
			hint = i18n.Tr(i18n.RequestLang(r), "事件 ID") + ": " + eventID
		}
		webpage.Error(w, r, webpage.Problem{Code: code, Title: "请求已被拦截",
			Detail: "这个请求被 Web 应用防火墙识别为攻击并已拦截。如果是误判，请把事件 ID 告知网站管理员。", Hint: hint, Lock: true})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	fmt.Fprintf(w, `{"code":%d,"success":false,"message":"blocked by Chaitin SafeLine Web Application Firewall","event_id":%q}`, code, eventID)
}

// SafeLineTestResult 是测试雷池连接的结果。
type SafeLineTestResult struct {
	// LatencyMs 为心跳往返耗时（毫秒）
	LatencyMs float64 `json:"latencyMs"`
	// Blocked 表示示例攻击请求被检测为攻击，说明检测引擎工作正常
	Blocked bool `json:"blocked"`
	// EventID 为示例攻击在雷池中的事件 ID
	EventID string `json:"eventId,omitempty"`
}

// TestSafeLine 测试检测引擎：先发心跳确认连接，再送检一个示例 SQL 注入请求，确认检测引擎能识别攻击。
func TestSafeLine(s SafeLine) (*SafeLineTestResult, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	if err := validateSafeLineAddr(s.Addr); err != nil {
		return nil, err
	}
	cl := newT1KClient(s.Addr, time.Duration(s.TimeoutMs)*time.Millisecond)
	defer cl.close()
	start := time.Now()
	if err := cl.heartbeat(); err != nil {
		return nil, fmt.Errorf("连接雷池检测引擎失败: %w", err)
	}
	out := &SafeLineTestResult{LatencyMs: float64(time.Since(start).Microseconds()) / 1000}
	req, _ := http.NewRequest(http.MethodGet, "http://gately-test.invalid/?id=1%27%20AND%201=1%20UNION%20SELECT%20username,password%20FROM%20users--", nil)
	req.RemoteAddr = "127.0.0.1:1"
	req.Header.Set("User-Agent", "Gately SafeLine test")
	chk := &safelineChecker{client: cl}
	res, err := chk.detectRequest(req)
	if err != nil {
		return nil, fmt.Errorf("送检示例请求失败: %w", err)
	}
	out.Blocked = res.blocked()
	out.EventID = res.eventID()
	return out, nil
}
