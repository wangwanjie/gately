package webservice

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/i18n"
	"github.com/wangwanjie/gately/internal/module"
)

// 监听列表（GET /api/webservice/listeners）的变化检测。
//
// 「Web 服务」页每 5 秒轮询一次完整列表，站点多时每次数百 KB，非简体中文界面还要整体翻译一遍。
// 响应带弱校验 ETag：客户端在下次请求的 If-None-Match 中带上它，内容没有变化时返回 304、不带响应体，
// 省去翻译、压缩、传输与页面的重新渲染。约定见 listeners-api 说明（ETag 的取值对客户端不透明）。
//
// 本接口之后还要经过只读脱敏与按语言翻译两层中间件，它们都只由“处理器输出的字节、访客语言、是否只读”决定，
// 因此 ETag 按处理器输出的字节加上这两项计算，等价于按最终写出的字节计算：同一 ETag 一定对应同样的最终内容，
// 中英文等不同语言、管理员与只读账号的 ETag 互不相同。另加进程启动时生成的随机值，
// 升级（翻译目录可能变化）或重启后旧的 ETag 一律失效。

// etagSalt 为进程启动时生成的随机值
var etagSalt = func() []byte {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return b
}()

// etagRouter 在登记路由时给 pattern 对应的处理器加上 ETag，其余路由与接口文档原样转交。
type etagRouter struct {
	module.Router
	pattern string
	// perm 为该接口所属模块的写权限，没有写权限的身份会被只读脱敏，ETag 据此区分
	perm string
}

func (e etagRouter) HandleFunc(pattern string, h http.HandlerFunc) {
	if pattern == e.pattern {
		h = withETag(h, e.perm)
	}
	e.Router.HandleFunc(pattern, h)
}

// Doc 转交接口文档登记（底层路由器支持时）。
func (e etagRouter) Doc(pattern string, d module.Doc) {
	module.DocOf(e.Router, pattern, d)
}

// bufferedResponse 收集处理器的完整响应。
type bufferedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (b *bufferedResponse) Header() http.Header { return b.header }

func (b *bufferedResponse) WriteHeader(code int) {
	if b.status == 0 {
		b.status = code
	}
}

func (b *bufferedResponse) Write(p []byte) (int, error) {
	if b.status == 0 {
		b.status = http.StatusOK
	}
	return b.body.Write(p)
}

// listETag 计算响应的 ETag：处理器输出的字节、访客语言、是否只读与进程随机值。
func listETag(r *http.Request, body []byte, perm string) string {
	h := sha256.New()
	h.Write(etagSalt)
	h.Write([]byte(i18n.RequestLang(r)))
	if auth.Allowed(r.Context(), perm) {
		h.Write([]byte{1})
	} else {
		h.Write([]byte{0})
	}
	h.Write(body)
	return `W/"` + hex.EncodeToString(h.Sum(nil)[:16]) + `"`
}

// etagMatches 判断 If-None-Match 是否包含 tag（弱比较：忽略 W/ 前缀）。
func etagMatches(header, tag string) bool {
	if header == "" {
		return false
	}
	want := strings.TrimPrefix(tag, "W/")
	for _, t := range strings.Split(header, ",") {
		t = strings.TrimSpace(t)
		if t == "*" || strings.TrimPrefix(t, "W/") == want {
			return true
		}
	}
	return false
}

// withETag 给 JSON 列表接口加上 ETag 与 If-None-Match 支持。只处理 200 响应，错误等其他响应原样输出。
func withETag(next http.HandlerFunc, perm string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		buf := &bufferedResponse{header: http.Header{}}
		next(buf, r)
		h := w.Header()
		if buf.status != http.StatusOK {
			for k, v := range buf.header {
				h[k] = v
			}
			w.WriteHeader(buf.status)
			_, _ = w.Write(buf.body.Bytes())
			return
		}
		tag := listETag(r, buf.body.Bytes(), perm)
		if etagMatches(r.Header.Get("If-None-Match"), tag) {
			// 304 不带 Content-Type 与响应体：外层的脱敏与翻译中间件只处理 JSON 内容，遇到它原样放行
			h.Set("ETag", tag)
			h.Add("Vary", "Accept-Language")
			h.Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusNotModified)
			return
		}
		for k, v := range buf.header {
			h[k] = v
		}
		h.Set("ETag", tag)
		// 内容随访客语言翻译
		h.Add("Vary", "Accept-Language")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.body.Bytes())
	}
}
