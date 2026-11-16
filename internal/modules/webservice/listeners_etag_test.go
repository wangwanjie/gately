package webservice

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/httpcompress"
	"github.com/wangwanjie/gately/internal/i18n"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// docRouter 记录登记的接口文档，确认加了 ETag 的路由文档没有丢。
type docRouter struct {
	prefixRouter
	docs map[string]bool
}

func (d docRouter) Doc(pattern string, _ module.Doc) { d.docs[pattern] = true }

// etagChain 按管理接口的顺序组装压缩、翻译中间件与模块路由；write 决定身份是否有写权限（没有时外层会做只读脱敏）。
func etagChain(t *testing.T) (*Module, func(lang, inm, enc string, write bool) *httptest.ResponseRecorder, map[string]bool) {
	t.Helper()
	m := New(nil, nil)
	st, _ := config.Open(t.TempDir())
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.cfg.Listeners = []Listener{{Key: "l1", Name: "HTTP", Port: 18080, Sites: []Site{redirectSite("s1", "a.example.com")}}}
	// 程序生成的说明字段按访客语言翻译，用来确认翻译后的内容同样受 ETag 约束
	m.errors["l1"] = "未运行"
	m.mu.Unlock()
	mux := http.NewServeMux()
	docs := map[string]bool{}
	m.Routes(docRouter{prefixRouter{mux, "/api/webservice"}, docs})
	h := httpcompress.Handler(i18n.Middleware(mux))
	do := func(lang, inm, enc string, write bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/api/webservice/listeners", nil)
		req.Header.Set("Accept-Language", lang)
		if inm != "" {
			req.Header.Set("If-None-Match", inm)
		}
		if enc != "" {
			req.Header.Set("Accept-Encoding", enc)
		}
		req = req.WithContext(auth.WithScopeCheck(req.Context(), func(need string) bool {
			return write || strings.HasSuffix(need, ":read")
		}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	return m, do, docs
}

func TestListenersETag(t *testing.T) {
	m, do, docs := etagChain(t)
	if !docs["GET /listeners"] || !docs["PUT /listeners/{key}"] {
		t.Fatal("加 ETag 后监听接口的文档应照常登记")
	}
	tags := map[string]string{}
	for _, lang := range []string{"zh-CN", "en"} {
		a := do(lang, "", "", true)
		tag := a.Header().Get("ETag")
		if a.Code != 200 || !strings.HasPrefix(tag, `W/"`) || a.Body.Len() == 0 {
			t.Fatalf("%s: 首次请求应返回 200 与弱 ETag: %d %q", lang, a.Code, tag)
		}
		if !strings.Contains(a.Header().Get("Vary"), "Accept-Language") {
			t.Fatalf("%s: 应声明 Vary: Accept-Language", lang)
		}
		// 同一 ETag 对应的最终内容（翻译之后）完全相同
		b := do(lang, "", "", true)
		if b.Header().Get("ETag") != tag || !bytes.Equal(a.Body.Bytes(), b.Body.Bytes()) {
			t.Fatalf("%s: 内容未变时 ETag 与内容应保持一致", lang)
		}
		nm := do(lang, tag, "", true)
		if nm.Code != http.StatusNotModified || nm.Body.Len() != 0 || nm.Header().Get("ETag") != tag || nm.Header().Get("Content-Type") != "" {
			t.Fatalf("%s: If-None-Match 命中应返回无响应体的 304: %d %q len=%d", lang, nm.Code, nm.Header().Get("ETag"), nm.Body.Len())
		}
		// 列表中多个 ETag、带空格时同样识别
		if nm := do(lang, `W/"other", `+tag, "", true); nm.Code != http.StatusNotModified {
			t.Fatalf("%s: 多个 ETag 时应能识别: %d", lang, nm.Code)
		}
		tags[lang] = tag
	}
	if en := do("en", "", "", true).Body.String(); !strings.Contains(en, `"error":"Not running"`) {
		t.Fatalf("英文界面应拿到翻译后的内容: %s", en)
	}
	if zh := do("zh-CN", "", "", true).Body.String(); !strings.Contains(zh, `"error":"未运行"`) {
		t.Fatalf("中文界面应为原文: %s", zh)
	}
	if tags["zh-CN"] == tags["en"] {
		t.Fatal("不同语言的 ETag 应不同")
	}
	// 拿英文界面的 ETag 请求中文内容不能得到 304
	if w := do("zh-CN", tags["en"], "", true); w.Code != 200 {
		t.Fatalf("跨语言的 ETag 不应命中: %d", w.Code)
	}
	// 只读账号的内容会被脱敏，ETag 与管理员的不同
	ro := do("zh-CN", "", "", false)
	if ro.Header().Get("ETag") == tags["zh-CN"] {
		t.Fatal("只读账号与管理员的 ETag 应不同")
	}
	if w := do("zh-CN", tags["zh-CN"], "", false); w.Code != 200 {
		t.Fatalf("管理员的 ETag 不应让只读账号命中: %d", w.Code)
	}
	// 压缩后 ETag 不变（弱校验），带它的请求照样 304
	gz := do("en", "", "gzip", true)
	if gz.Header().Get("Content-Encoding") != "gzip" && gz.Body.Len() >= 1024 {
		t.Fatal("较大的响应应被压缩")
	}
	if gz.Header().Get("Content-Encoding") == "gzip" {
		zr, err := gzip.NewReader(gz.Body)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, zr)
	}
	if gz.Header().Get("ETag") != tags["en"] {
		t.Fatalf("压缩不应改变弱 ETag: %q %q", gz.Header().Get("ETag"), tags["en"])
	}
	if w := do("en", tags["en"], "gzip", true); w.Code != http.StatusNotModified || w.Header().Get("Content-Encoding") != "" {
		t.Fatalf("压缩时 304 不带内容编码: %d %q", w.Code, w.Header().Get("Content-Encoding"))
	}

	// 配置变化后 ETag 改变，旧 ETag 拿到 200 与新内容
	m.mu.Lock()
	m.cfg.Listeners[0].Name = "HTTP 2"
	m.mu.Unlock()
	for _, lang := range []string{"zh-CN", "en"} {
		w := do(lang, tags[lang], "", true)
		if w.Code != 200 || w.Header().Get("ETag") == tags[lang] || !strings.Contains(w.Body.String(), "HTTP 2") {
			t.Fatalf("%s: 内容变化后应返回新内容: %d", lang, w.Code)
		}
	}
}

// 非 200 的响应与其他接口不受影响。
func TestETagPassThrough(t *testing.T) {
	h := withETag(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":"x"}`)
	}, "webservice:write")
	req := httptest.NewRequest("GET", "/", nil).WithContext(context.Background())
	req.Header.Set("If-None-Match", "*")
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusForbidden || w.Header().Get("ETag") != "" || w.Body.String() != `{"error":"x"}` {
		t.Fatalf("错误响应应原样输出: %d %q", w.Code, w.Body.String())
	}
}
