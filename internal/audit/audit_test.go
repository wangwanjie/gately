package audit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/auth"
)

func TestRecordAndSearch(t *testing.T) {
	l := New(t.TempDir())
	l.Record(Entry{Actor: "admin", IP: "1.1.1.1", Category: CategoryLogin, Detail: "密码登录"})
	l.Record(Entry{Actor: "api-token:ci", IP: "2.2.2.2", Category: CategoryChange, Module: "ddns", Method: "PUT", Path: "/api/ddns/tasks/a", Status: 200})
	l.Record(Entry{Actor: "admin", IP: "1.1.1.1", Category: CategoryChange, Module: "ssl", Method: "DELETE", Path: "/api/ssl/certs/b", Status: 200})

	res := l.Search(Query{})
	if res.Total != 3 || res.Entries[0].Module != "ssl" {
		t.Fatalf("应按时间倒序返回 3 条: %+v", res)
	}
	if got := l.Search(Query{Module: "ddns"}); got.Total != 1 || got.Entries[0].Actor != "api-token:ci" {
		t.Fatalf("按模块过滤失败: %+v", got)
	}
	if got := l.Search(Query{Text: "2.2.2"}); got.Total != 1 {
		t.Fatalf("文本过滤失败: %+v", got)
	}
	if got := l.Search(Query{Limit: 1, Offset: 1}); got.Total != 3 || len(got.Entries) != 1 || got.Entries[0].Module != "ddns" {
		t.Fatalf("分页失败: %+v", got)
	}
}

func TestRotate(t *testing.T) {
	l := New(t.TempDir())
	l.maxBytes = 200
	l.keep = 2
	for i := 0; i < 20; i++ {
		l.Record(Entry{Actor: "admin", Category: CategoryChange, Path: "/api/x", Time: time.Unix(int64(i), 0)})
	}
	res := l.Search(Query{Limit: 1000})
	if res.Total == 0 || res.Total > 20 {
		t.Fatalf("轮转后记录数异常: %d", res.Total)
	}
	if !res.Entries[0].Time.Equal(time.Unix(19, 0)) {
		t.Fatalf("最新一条应排在最前: %v", res.Entries[0].Time)
	}
}

func TestMiddleware(t *testing.T) {
	l := New(t.TempDir())
	inner := l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inner.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), "tester")))
	})
	do := func(method, path string) {
		req := httptest.NewRequest(method, path, nil)
		req.RemoteAddr = "9.9.9.9:1234"
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
	do("GET", "/api/ddns/tasks")                 // 普通读取不记录
	do("POST", "/api/ddns/tasks")                // 变更
	do("GET", "/api/system/backup")              // 敏感读取
	do("POST", "/api/nettools/run")              // 被跳过
	do("PUT", "/api/account/ui-language")        // 界面语言偏好不记录
	do("POST", "/api/webservice/panels/preview") // 导航面板预览不保存，不记录
	do("GET", "/api/webterminal/connections/x/ws")
	do("GET", "/api/ddns/tasks/x/secrets") // 取回密码明文
	do("GET", "/api/backup/local")         // 备份文件列表会被轮询，不记录
	do("GET", "/api/backup/local/a.zip")   // 下载备份文件
	res := l.Search(Query{})
	if res.Total != 5 {
		t.Fatalf("应记录 5 条，实际 %d: %+v", res.Total, res.Entries)
	}
	e := l.Search(Query{Category: CategoryChange}).Entries[0]
	if e.Actor != "tester" || e.IP != "9.9.9.9" || e.Module != "ddns" || e.Status != http.StatusCreated {
		t.Fatalf("记录内容不符: %+v", e)
	}
}

func TestMiddlewareRecordsVia(t *testing.T) {
	l := New(t.TempDir())
	inner := l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inner.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), "tester")))
	})
	req := httptest.NewRequest("POST", "/api/ddns/tasks", nil)
	req.RemoteAddr = "127.0.0.1:5000"
	req.Header.Set("X-Real-IP", "198.51.100.7")
	h.ServeHTTP(httptest.NewRecorder(), req)
	e := l.Search(Query{}).Entries[0]
	if e.IP != "198.51.100.7" || e.Via != "127.0.0.1" {
		t.Fatalf("经本机代理的请求应记录真实来源与代理地址: %+v", e)
	}
}
