// Package webprobe 把 Web 服务处理完的请求交给入侵防护，按路径探测规则计数封禁。
//
// 扫描器探测路径时请求量很大，这里不走事件总线（每个事件为每个订阅者开协程，还会经过通知中心），
// 而是在请求结束时同步调用入侵防护注册的处理函数，未注册时只有一次原子读取的开销。
package webprobe

import (
	"net/netip"
	"strings"
	"sync/atomic"
)

// Hit 是 Web 服务处理完的一次请求。
type Hit struct {
	// Addr 为客户端地址（已按可信代理、PROXY 协议还原）
	Addr netip.Addr
	// Listener 为监听的 key
	Listener string
	// Site 为处理请求的站点 key；没有任何站点处理（域名未匹配且没有默认站点）时为空
	Site string
	// Unknown 表示请求的域名没有匹配任何站点，由默认站点处理或直接返回 404
	Unknown bool
	Method  string
	Host    string
	// Path 为请求路径（已解码，未规范化）
	Path string
	// Status 为响应状态码，断开连接记为 444
	Status int
}

var handler atomic.Pointer[func(Hit)]

// SetHandler 注册处理函数，传 nil 取消注册。同一时间只有一个处理函数。
func SetHandler(f func(Hit)) {
	if f == nil {
		handler.Store(nil)
		return
	}
	handler.Store(&f)
}

// Report 在请求结束时调用，没有注册处理函数时直接返回。
func Report(h Hit) {
	if f := handler.Load(); f != nil {
		(*f)(h)
	}
}

// TLS 握手失败的原因类别，规则按类别选择要统计的失败。
const (
	// TLSEOF 对端在握手中途关闭连接（EOF、unexpected EOF）：端口探测与扫描器常见，浏览器预连接被丢弃时也会出现
	TLSEOF = "eof"
	// TLSReset 对端重置连接（connection reset、broken pipe）
	TLSReset = "reset"
	// TLSTimeout 握手超时（i/o timeout）
	TLSTimeout = "timeout"
	// TLSProtocol 对端发来的不是有效的 TLS：明文 HTTP、SSLv2、乱码、过大的记录、版本过低等
	TLSProtocol = "protocol"
	// TLSNegotiation 双方没有共同的套件、密钥交换、曲线或应用层协议
	TLSNegotiation = "negotiation"
	// TLSNoCert 请求了本监听器不提供的域名（没有匹配的证书）
	TLSNoCert = "nocert"
	// TLSAlert 对端发来 TLS 警报并中止握手（例如不信任本机证书），多为客户端自身的信任问题，统计时要谨慎
	TLSAlert = "alert"
	// TLSOther 其他原因
	TLSOther = "other"
)

// TLSKinds 是全部原因类别，供规则校验与界面选择。
var TLSKinds = []string{TLSEOF, TLSReset, TLSTimeout, TLSProtocol, TLSNegotiation, TLSNoCert, TLSAlert, TLSOther}

// TLSFail 是一次 TLS 握手失败。
type TLSFail struct {
	// Addr 为客户端地址（已按可信代理、PROXY 协议还原）
	Addr netip.Addr
	// Listener 为监听的 key
	Listener string
	// Kind 为原因类别，见 TLS* 常量
	Kind string
	// Reason 为 crypto/tls、net/http 给出的原始原因
	Reason string
}

var tlsHandler atomic.Pointer[func(TLSFail)]

// SetTLSHandler 注册握手失败的处理函数，传 nil 取消注册。同一时间只有一个处理函数。
func SetTLSHandler(f func(TLSFail)) {
	if f == nil {
		tlsHandler.Store(nil)
		return
	}
	tlsHandler.Store(&f)
}

// TLSEnabled 返回是否有处理函数，调用方据此跳过分类等准备工作。
func TLSEnabled() bool {
	return tlsHandler.Load() != nil
}

// ReportTLS 在握手失败时调用，没有注册处理函数时直接返回。
func ReportTLS(f TLSFail) {
	if h := tlsHandler.Load(); h != nil {
		(*h)(f)
	}
}

// 握手失败原因中表示对端发来的不是有效 TLS 的片段（小写）。
var tlsProtocolReasons = []string{
	"client sent an http request to an https server",
	"first record does not look like a tls handshake",
	"unsupported sslv2 handshake received",
	"oversized record received",
	"client offered only unsupported versions",
	"remote error: tls: protocol version not supported",
	"tls: record overflow",
	"tls: unexpected message",
	"tls: received record with version",
	"tls: handshake message",
}

// 握手失败原因中表示双方无法协商出共同参数的片段（小写）。
var tlsNegotiationReasons = []string{
	"no cipher suite supported by both client and server",
	"no key exchanges supported by both client and server",
	"no supported elliptic curves offered",
	"client requested unsupported application protocols",
	"handshake failure",
	"no application protocol",
}

// ClassifyTLS 按握手失败原因（net/http 日志中的原因部分）归类。
// 缺少证书的原因由证书模块的错误句给出，格式属于 Web 服务，不在这里识别（见 Web 服务的 reportTLSFail）。
func ClassifyTLS(reason string) string {
	lower := strings.ToLower(reason)
	switch {
	case lower == "eof" || strings.Contains(lower, "unexpected eof"):
		return TLSEOF
	case strings.Contains(lower, "connection reset") || strings.Contains(lower, "broken pipe"):
		return TLSReset
	case strings.Contains(lower, "i/o timeout") || strings.Contains(lower, "deadline exceeded"):
		return TLSTimeout
	case containsAny(lower, tlsProtocolReasons):
		return TLSProtocol
	case containsAny(lower, tlsNegotiationReasons):
		return TLSNegotiation
	case strings.Contains(lower, "remote error: tls:"):
		return TLSAlert
	}
	return TLSOther
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
