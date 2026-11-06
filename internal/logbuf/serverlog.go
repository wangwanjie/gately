package logbuf

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// serverLogThrottle 为同类服务器错误的最短记录间隔，避免浏览器重试或扫描刷屏
var serverLogThrottle = time.Minute

// serverLogMaxKeys 限制节流表大小，防止大量不同 SNI 等内容撑大内存
const serverLogMaxKeys = 256

// ServerErrorLog 返回可设置为 http.Server.ErrorLog 的标准日志器，
// 把 net/http 内部错误（如 TLS 握手失败）写入本模块日志，同时保留 docker logs 输出。
func (l *Logger) ServerErrorLog() *log.Logger {
	return l.ServerErrorLogWith(nil)
}

// ServerErrorLogWith 与 ServerErrorLog 相同，另由 isNoise 判断某个 TLS 握手失败原因是否只是访客侧噪声
// （例如扫描器用不属于任何站点的域名握手），返回 true 时按调试级别记录。isNoise 可为 nil。
func (l *Logger) ServerErrorLogWith(isNoise func(reason string) bool) *log.Logger {
	return log.New(&serverLogWriter{l: l, seen: map[string]*throttleState{}, isNoise: isNoise}, "", 0)
}

type throttleState struct {
	last       time.Time
	suppressed int
}

type serverLogWriter struct {
	l    *Logger
	mu   sync.Mutex
	seen map[string]*throttleState
	// isNoise 为调用方提供的额外噪声判断，见 ServerErrorLogWith
	isNoise func(reason string) bool
}

func (w *serverLogWriter) Write(p []byte) (int, error) {
	msg := strings.TrimSpace(string(p))
	level, text, key := classifyServerLog(msg)
	if level == LevelWarn && w.isNoise != nil && strings.HasPrefix(key, "tls:") && w.isNoise(strings.TrimPrefix(key, "tls:")) {
		// 调用方判定的噪声（如不属于任何站点的 SNI）原因各不相同，统一按一个键节流，避免随机域名撑满节流表
		level, key = LevelDebug, "tls-noise:caller"
	}
	if level == LevelDebug && key == "" {
		// 客户端断开、超时等常见噪声只记调试级别，不做节流
		w.l.buf.Add(level, w.l.source, text)
		return len(p), nil
	}
	w.mu.Lock()
	now := time.Now()
	st, ok := w.seen[key]
	if ok && now.Sub(st.last) < serverLogThrottle {
		st.suppressed++
		w.mu.Unlock()
		return len(p), nil
	}
	if !ok {
		if len(w.seen) >= serverLogMaxKeys {
			w.seen = map[string]*throttleState{}
		}
		st = &throttleState{}
		w.seen[key] = st
	}
	if st.suppressed > 0 {
		text += fmt.Sprintf("（此前 %s 内同类错误另有 %d 次）", serverLogThrottle, st.suppressed)
	}
	st.last, st.suppressed = now, 0
	w.mu.Unlock()
	w.l.buf.Add(level, w.l.source, text)
	return len(p), nil
}

// classifyServerLog 解析 net/http 的错误日志，返回级别、展示文本与节流键。
func classifyServerLog(msg string) (level, text, key string) {
	const tlsPrefix = "http: TLS handshake error from "
	rest, ok := strings.CutPrefix(msg, tlsPrefix)
	if !ok {
		return LevelWarn, msg, msg
	}
	// 格式为 "<地址>: <原因>"，IPv6 地址带方括号，按第一个 ": " 切分即可
	addr, reason, found := strings.Cut(rest, ": ")
	if !found {
		return LevelWarn, "TLS 握手失败: " + rest, rest
	}
	text = fmt.Sprintf("TLS 握手失败（来自 %s）: %s", addr, reason)
	lower := strings.ToLower(reason)
	if lower == "eof" || strings.Contains(lower, "connection reset") || strings.Contains(lower, "i/o timeout") || strings.Contains(lower, "broken pipe") {
		return LevelDebug, text, ""
	}
	if isClientTLSNoise(lower) {
		// 访客只支持已淘汰的协议版本或套件、往 HTTPS 端口发明文等，多为公网扫描器，
		// 服务端无从处理，按调试级别记录；仍按原因节流，避免扫描器成批握手时刷掉其他日志
		k, _, _ := strings.Cut(lower, ";")
		return LevelDebug, text, "tls-noise:" + k
	}
	// 节流键去掉来源地址，同一原因（如缺少某域名证书）只按原因合并
	return LevelWarn, text, "tls:" + reason
}

// clientTLSNoise 为 crypto/tls 与 net/http 中由访客能力或行为导致、与服务端配置无关的握手失败原因（小写）。
// 对端拒绝本机证书（remote error: tls: bad certificate 等）可能说明证书过期或不匹配，不在此列；
// 对端回报 protocol version not supported 只说明它不支持本监听器允许的版本（最低版本设置），列入。
var clientTLSNoise = []string{
	"client offered only unsupported versions",
	"remote error: tls: protocol version not supported",
	"unsupported sslv2 handshake received",
	"no cipher suite supported by both client and server",
	"no key exchanges supported by both client and server",
	"no supported elliptic curves offered",
	"client requested unsupported application protocols",
	"client sent an http request to an https server",
	"first record does not look like a tls handshake",
	"oversized record received",
	"unexpected eof",
}

// isClientTLSNoise 判断小写的握手失败原因是否属于访客侧噪声。
func isClientTLSNoise(lower string) bool {
	for _, s := range clientTLSNoise {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}
