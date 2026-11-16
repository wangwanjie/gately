package i18n

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// 处理器声明已自行翻译时（日志接口），成功响应原样输出；错误响应仍由中间件翻译。
func TestSkipTranslation(t *testing.T) {
	old := catalogFor("en")
	catalogMu.Lock()
	catalogs["en"] = buildCatalog(map[string]string{"监听失败": "listen failed", "参数无效": "invalid parameter"})
	catalogMu.Unlock()
	t.Cleanup(func() {
		catalogMu.Lock()
		catalogs["en"] = old
		catalogMu.Unlock()
	})
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", jsonType)
		switch r.URL.Query().Get("mode") {
		case "skip":
			SkipTranslation(r)
			_, _ = w.Write([]byte(`[{"message":"监听失败"}]`))
		case "error":
			SkipTranslation(r)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"参数无效"}`))
		default:
			_, _ = w.Write([]byte(`[{"message":"监听失败"}]`))
		}
	}))
	get := func(mode, lang string) string {
		req := httptest.NewRequest(http.MethodGet, "/api/webservice/logs?mode="+mode, nil)
		req.Header.Set("Accept-Language", lang)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Body.String()
	}
	if got := get("", "en"); got != `[{"message":"listen failed"}]` {
		t.Errorf("默认应由中间件翻译: %s", got)
	}
	if got := get("skip", "en"); got != `[{"message":"监听失败"}]` {
		t.Errorf("声明已翻译的响应应原样输出: %s", got)
	}
	if got := get("error", "en"); got != `{"error":"invalid parameter"}` {
		t.Errorf("错误响应仍应翻译: %s", got)
	}
	if got := get("skip", "zh-CN"); got != `[{"message":"监听失败"}]` {
		t.Errorf("简体中文请求不经过翻译: %s", got)
	}
	// 不经过中间件的请求调用 SkipTranslation 没有任何效果
	SkipTranslation(httptest.NewRequest(http.MethodGet, "/api/x", nil))
}
