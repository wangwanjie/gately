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
"authSecret":"zzz","selfServicePassword":true,"upstreams":[{"url":"http://u:p@10.0.0.1:80/x?k=1"}]}],"hasPassword":true}`

func TestRedactJSON(t *testing.T) {
	var v any
	if err := json.Unmarshal([]byte(sampleListener), &v); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(redactJSON(v))
	s := string(out)
	for _, leak := range []string{"hunter2", "alice:pw", "abcdef", "Bearer xyz", "zzz", "u:p@", "k=1"} {
		if strings.Contains(s, leak) {
			t.Errorf("脱敏后仍含明文 %q: %s", leak, s)
		}
	}
	// 名称、布尔值、非敏感头不应被改动
	for _, keep := range []string{"bob:", "a.example.com", `"selfServicePassword":true`, `"hasPassword":true`, "X-Plain: ********"} {
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
