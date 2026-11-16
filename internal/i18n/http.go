package i18n

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
)

// RequestLang 返回请求期望的语言：优先 Accept-Language，没有时使用服务端语言。
func RequestLang(r *http.Request) string {
	if l := FromAcceptLanguage(r.Header.Get("Accept-Language")); l != "" {
		return l
	}
	return ServerLang()
}

// TrDeep 递归翻译 JSON 值（map / slice / string）中的全部字符串，只用于完全由程序生成的数据。
func TrDeep(lang string, v any) any {
	switch x := v.(type) {
	case string:
		return Tr(lang, x)
	case []any:
		for i := range x {
			x[i] = TrDeep(lang, x[i])
		}
		return x
	case map[string]any:
		for k, e := range x {
			x[k] = TrDeep(lang, e)
		}
		return x
	}
	return v
}

// messageKeys 为程序生成的说明性字段（错误、状态说明、提示、标题等）。配置数据只翻译这些字段，
// 名称、备注、命令、记录值等用户填写的内容保持原样：客户端读出后原样保存时不会被改写成译文。
var messageKeys = map[string]bool{
	"error": true, "errors": true, "lastError": true, "ipError": true, "errorDetail": true,
	"detail": true, "details": true, "message": true, "reason": true, "warnings": true,
	"mappingNote": true, "hint": true, "title": true, "label": true, "issues": true,
}

// stringMessageKeys 只在值为字符串时按说明字段翻译：同名字段在别处是用户配置对象，
// 如 STUN 状态的 mapping 是端口映射结果说明，证书的 mapping 却是用户填写的目录与脚本；
// STUN 状态的 firewall 是自动放行防火墙的结果说明，入侵防护状态里同名字段是布尔值或对象。
var stringMessageKeys = map[string]bool{
	"mapping": true, "unsupported": true, "firewall": true,
}

// TrFields 递归翻译 JSON 值中 keys 字段（及 messageKeys）下的字符串，其他字段原样保留。
func TrFields(lang string, v any, keys ...string) any {
	extra := map[string]bool{}
	for _, k := range keys {
		extra[k] = true
	}
	return trFields(lang, v, extra)
}

func trFields(lang string, v any, extra map[string]bool) any {
	switch x := v.(type) {
	case []any:
		for i := range x {
			x[i] = trFields(lang, x[i], extra)
		}
	case map[string]any:
		for k, e := range x {
			if messageKeys[k] || extra[k] {
				x[k] = TrDeep(lang, e)
			} else if str, ok := e.(string); ok && stringMessageKeys[k] {
				x[k] = Tr(lang, str)
			} else {
				x[k] = trFields(lang, e, extra)
			}
		}
	}
	return v
}

// fullTranslate 返回 true 的接口数据全部由程序生成（状态、日志、文档、服务商与权限说明、网络工具与 Docker 任务输出等），
// 所有字符串都翻译；这些接口不返回可编辑的用户配置。其他接口只翻译 messageKeys 字段。
func fullTranslate(path string) bool {
	switch {
	case path == "/api/status", path == "/api/logs", path == "/api/openapi.json",
		strings.HasSuffix(path, "/logs"), strings.HasSuffix(path, "/logs/sources"), strings.HasPrefix(path, "/api/events"),
		strings.HasPrefix(path, "/api/nettools/tasks"), strings.HasPrefix(path, "/api/docker/tasks"),
		path == "/api/guard/presets", path == "/api/notify/event-types",
		strings.HasPrefix(path, "/api/tokens/scopes"), strings.HasSuffix(path, "/providers"):
		return true
	}
	return false
}

// skipTranslate 返回 true 的接口内容来自用户文件或外部程序，不做翻译。
func skipTranslate(path string) bool {
	// 日志导出是流式下载，由接口逐条翻译，这里不能缓存整个响应
	if strings.HasSuffix(path, "/logs/export") {
		return true
	}
	for _, p := range []string{"/files", "/content", "/download", "/backup", "/terminal", "/sftp", "/stats/", "/containers/"} {
		if strings.Contains(path, p) && !strings.HasSuffix(path, "/logs") {
			return true
		}
	}
	return false
}

// Middleware 按请求语言翻译 JSON 响应：错误响应（状态码 >= 400）与程序生成的状态、日志、文档数据全部翻译，
// 其他 GET 响应只翻译 messageKeys 字段，用户填写的配置内容不翻译。语言为简体中文时不做任何处理。
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lang := RequestLang(r)
		if lang == SourceLang || !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		tw := &transWriter{ResponseWriter: w, lang: lang, path: r.URL.Path, get: r.Method == http.MethodGet}
		next.ServeHTTP(tw, r.WithContext(context.WithValue(r.Context(), transWriterKey{}, tw)))
		tw.finish()
	})
}

type transWriterKey struct{}

// SkipTranslation 声明本次请求的成功响应已由处理器按请求语言翻译好（见 RequestLang），中间件原样输出。
// 须在写出响应之前调用；错误响应（状态码 >= 400）仍由中间件翻译。不经过 Middleware 的请求调用它没有任何效果。
func SkipTranslation(r *http.Request) {
	if tw, ok := r.Context().Value(transWriterKey{}).(*transWriter); ok {
		tw.skip = true
	}
}

type transWriter struct {
	http.ResponseWriter
	lang   string
	path   string
	get    bool
	status int
	// buf 缓存响应体，取自 captureBufs，finish 写出后归还
	buf *bytes.Buffer
	// capture 为 true 时响应体被缓存，结束后翻译再输出
	capture bool
	decided bool
	// skip 为 true 时成功响应不再翻译，见 SkipTranslation
	skip bool
}

func (t *transWriter) decide(status int) {
	if t.decided {
		return
	}
	t.decided = true
	t.status = status
	ct := t.Header().Get("Content-Type")
	isJSON := strings.Contains(ct, "json")
	isText := strings.HasPrefix(ct, "text/plain")
	switch {
	case status >= 400 && (isJSON || isText):
		t.capture = true
	case status == http.StatusOK && isJSON && t.get && !t.skip && !skipTranslate(t.path):
		t.capture = true
	}
}

func (t *transWriter) WriteHeader(status int) {
	t.decide(status)
	if t.capture {
		return
	}
	t.ResponseWriter.WriteHeader(status)
}

func (t *transWriter) Write(p []byte) (int, error) {
	if !t.decided {
		t.decide(http.StatusOK)
	}
	if t.capture {
		if t.buf == nil {
			t.buf = captureBufs.Get().(*bytes.Buffer)
		}
		return t.buf.Write(p)
	}
	return t.ResponseWriter.Write(p)
}

// captureBufs 复用缓存响应体的缓冲区：管理界面每隔几秒轮询一次，不必每个请求都重新分配几百 KB
var captureBufs = sync.Pool{New: func() any { return new(bytes.Buffer) }}

// captureBufKeep 以上的缓冲区不放回对象池
const captureBufKeep = 1 << 20

func (t *transWriter) Flush() {
	if t.capture {
		return
	}
	if f, ok := t.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack 经 ResponseController 沿 Unwrap 链找到底层连接：内层可能是只实现 Unwrap 的包装（如统一网关的 gatewayWriter），
// 直接做 http.Hijacker 类型断言会失败，WebSocket 在写出 101 后无法接管连接
func (t *transWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(t.ResponseWriter).Hijack()
}

func (t *transWriter) Unwrap() http.ResponseWriter { return t.ResponseWriter }

func (t *transWriter) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(struct{ io.Writer }{t}, r)
}

// finish 翻译缓存的响应体并输出。
//
// 响应里没有汉字时（多数配置类接口）原样写出：不解析、不重新编码，字段顺序与数字都保持处理器写出的样子。
// 有汉字时只改写需要翻译的那些字符串，其余字节照抄，见 rewriteJSON。
func (t *transWriter) finish() {
	if !t.capture {
		return
	}
	var body []byte
	if t.buf != nil {
		body = t.buf.Bytes()
		defer func() {
			// 响应已经写出，缓冲区可以复用
			if t.buf.Cap() <= captureBufKeep {
				t.buf.Reset()
				captureBufs.Put(t.buf)
			}
			t.buf = nil
		}()
	}
	ct := t.Header().Get("Content-Type")
	changed := false
	if strings.Contains(ct, "json") {
		if mayHaveHanJSON(body) {
			deep := t.status >= 400 || fullTranslate(t.path)
			if out, ok := rewriteJSON(t.lang, body, deep); ok {
				body, changed = out, true
			}
		}
	} else if hasHan(body) {
		body = []byte(Tr(t.lang, strings.TrimRight(string(body), "\n")) + "\n")
		changed = true
	}
	if changed {
		// 内容长度变了；没改动时处理器设置的长度仍然有效
		t.Header().Del("Content-Length")
	}
	t.ResponseWriter.WriteHeader(t.status)
	_, _ = t.ResponseWriter.Write(body)
}

// Localize 把程序生成的结果（如导入预览、部署结果）转为按请求语言翻译后的 JSON 值。
// 中间件只翻译 GET 响应与错误响应，POST 接口返回的说明文字需要用它显式翻译。
// 与 GET 响应一样只翻译 messageKeys 字段，keys 为本接口额外需要翻译的字段；名称、备注等用户内容保持原样。
func Localize(r *http.Request, v any, keys ...string) any {
	lang := RequestLang(r)
	if lang == SourceLang {
		return v
	}
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var x any
	if err := json.Unmarshal(b, &x); err != nil {
		return v
	}
	return TrFields(lang, x, keys...)
}
