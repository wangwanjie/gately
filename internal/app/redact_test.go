package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/auth"
)

const sampleListener = `{"key":"l1","sites":[{"host":"a.example.com","users":["bob:hunter2","alice:pw"],
"shareLinks":[{"id":"s1","token":"abcdef"}],"requestHeaders":["Authorization: Bearer xyz","X-Plain: v"],
"authSecret":"zzz","selfServicePassword":true,"targets":["http://admin:tpw@10.0.0.2:8080/?key=tq"],"remark":"see https://example.com/docs","upstreams":[{"url":"http://u:p@10.0.0.1:80/x?k=1"}]}],"hasPassword":true}`

func TestRedactJSON(t *testing.T) {
	var v any
	if err := json.Unmarshal([]byte(sampleListener), &v); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(redactJSON(v))
	s := string(out)
	for _, leak := range []string{"hunter2", "alice:pw", "abcdef", "Bearer xyz", "zzz", "u:p@", "k=1", "tpw", "key=tq"} {
		if strings.Contains(s, leak) {
			t.Errorf("脱敏后仍含明文 %q: %s", leak, s)
		}
	}
	// 名称、布尔值、非敏感头不应被改动
	for _, keep := range []string{"bob:", "a.example.com", `"selfServicePassword":true`, `"hasPassword":true`, "X-Plain: ********", "admin:", "https://example.com/docs"} {
		if !strings.Contains(s, keep) {
			t.Errorf("脱敏不应改动 %q: %s", keep, s)
		}
	}
}

// 路径中的令牌与夹在文本中的地址也要脱敏，普通路径保持原样
func TestRedactURLInText(t *testing.T) {
	in := map[string]any{
		"url":    "https://discord.com/api/webhooks/123456789012345678/AbCdEfGhIjKlMnOpQrStUvWxYz0123456789",
		"feishu": "https://open.feishu.cn/open-apis/bot/v2/hook/0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b",
		"error":  `Post "https://hook.example.com/notify?token=abc123secret": dial tcp: i/o timeout`,
		"target": "http://10.0.0.2:8080/api/v1/status",
	}
	out, _ := json.Marshal(redactJSON(in))
	s := string(out)
	for _, leak := range []string{"AbCdEfGhIjKlMnOpQrStUvWxYz0123456789", "0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b", "abc123secret"} {
		if strings.Contains(s, leak) {
			t.Errorf("脱敏后仍含明文 %q: %s", leak, s)
		}
	}
	for _, keep := range []string{"https://discord.com/api/webhooks/123456789012345678/********", "open-apis/bot/v2/hook/", "dial tcp: i/o timeout", `"http://10.0.0.2:8080/api/v1/status"`} {
		if !strings.Contains(s, keep) {
			t.Errorf("脱敏不应改动 %q: %s", keep, s)
		}
	}
}

func serveAs(t *testing.T, a *App, write bool, path string) *httptest.ResponseRecorder {
	t.Helper()
	h := a.redactReadOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(sampleListener))
	}))
	req := httptest.NewRequest("GET", path, nil)
	req = req.WithContext(auth.WithScopeCheck(req.Context(), func(need string) bool {
		return write || strings.HasSuffix(need, ":read")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// 只读身份拿不到站点凭据明文，具备写权限的身份保持原样。
func TestRedactReadOnlyOnly(t *testing.T) {
	a := &App{}
	ro := serveAs(t, a, false, "/api/webservice/listeners")
	if strings.Contains(ro.Body.String(), "hunter2") || strings.Contains(ro.Body.String(), "abcdef") {
		t.Fatalf("只读身份不应看到明文: %s", ro.Body.String())
	}
	if ro.Header().Get("Content-Length") == "" {
		t.Error("脱敏后应更新 Content-Length")
	}
	rw := serveAs(t, a, true, "/api/webservice/listeners")
	if !strings.Contains(rw.Body.String(), "hunter2") {
		t.Fatalf("写权限身份应看到原值: %s", rw.Body.String())
	}
}
