package dnsprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// hipmMock 模拟 HiPM DNSMgr 面板：API 令牌访问域名列表时返回裸数组，记录列表返回 {total, list}。
type hipmMock struct {
	t        *testing.T
	recs     []map[string]any
	nextID   int
	lastBody map[string]any
	// 最近一次记录列表请求的查询参数
	lastQuery map[string]string
	// 域名列表是否使用 {list, total} 包装
	wrapDomains bool
}

func newHipmMock(t *testing.T) (*hipmMock, *httptest.Server) {
	m := &hipmMock{t: t, nextID: 100}
	return m, httptest.NewServer(http.HandlerFunc(m.serve))
}

func (m *hipmMock) serve(w http.ResponseWriter, r *http.Request) {
	reply := func(status int, msg string, data any) {
		code := 0
		if status != 200 {
			code = status
		}
		w.WriteHeader(status)
		b, _ := json.Marshal(map[string]any{"code": code, "msg": msg, "data": data})
		_, _ = w.Write(b)
	}
	if r.Header.Get("Authorization") != "Bearer tok" {
		reply(401, "Unauthorized", nil)
		return
	}
	switch {
	case r.Method == "GET" && r.URL.Path == "/api/domains":
		domains := []map[string]any{{"id": 3, "name": "other.com"}, {"id": 5, "name": "example.com"}}
		if m.wrapDomains {
			reply(200, "success", map[string]any{"list": domains, "total": 2})
			return
		}
		reply(200, "success", domains)
	case r.Method == "GET" && r.URL.Path == "/api/domains/5/records":
		q := r.URL.Query()
		m.lastQuery = map[string]string{"subdomain": q.Get("subdomain"), "type": q.Get("type")}
		var list []map[string]any
		for _, rec := range m.recs {
			if rec["type"] == q.Get("type") {
				list = append(list, rec)
			}
		}
		reply(200, "success", map[string]any{"total": len(list), "list": list})
	case r.Method == "POST" && r.URL.Path == "/api/domains/5/records":
		body := readJSON(m.t, r)
		m.lastBody = body
		m.nextID++
		m.recs = append(m.recs, map[string]any{"id": strconv.Itoa(m.nextID), "name": body["name"], "type": body["type"], "value": body["value"], "ttl": body["ttl"], "line": "default"})
		reply(200, "success", nil)
	case r.Method == "PUT" && strings.HasPrefix(r.URL.Path, "/api/domains/5/records/"):
		body := readJSON(m.t, r)
		m.lastBody = body
		id := strings.TrimPrefix(r.URL.Path, "/api/domains/5/records/")
		for _, rec := range m.recs {
			if rec["id"] == id {
				rec["value"] = body["value"]
				rec["ttl"] = body["ttl"]
				reply(200, "success", nil)
				return
			}
		}
		reply(400, "Failed to update record", nil)
	case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/api/domains/5/records/"):
		id := strings.TrimPrefix(r.URL.Path, "/api/domains/5/records/")
		for i, rec := range m.recs {
			if rec["id"] == id {
				m.recs = append(m.recs[:i], m.recs[i+1:]...)
				reply(200, "success", nil)
				return
			}
		}
		reply(400, "Failed to delete record", nil)
	default:
		m.t.Errorf("未预期的请求 %s %s", r.Method, r.URL.String())
		reply(404, "not found", nil)
	}
}

func newHipmForTest(t *testing.T, srv *httptest.Server, id string) Provider {
	p, err := New("hipmdnsmgr", Credentials{ID: id, Secret: "tok"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestHipmDnsMgr(t *testing.T) {
	m, srv := newHipmMock(t)
	defer srv.Close()
	// 面板地址带 /api 与结尾斜杠时自动规范化
	p := newHipmForTest(t, srv, srv.URL+"/api/")
	ctx := context.Background()

	runUpsertSequence(t, p, Record{Zone: "example.com", Sub: "home", Type: "A"})
	if len(m.recs) != 1 {
		t.Fatalf("应只有 1 条记录: %v", m.recs)
	}
	if m.lastBody["name"] != "home" || m.lastBody["type"] != "A" || m.lastBody["value"] != "203.0.113.2" || m.lastBody["ttl"] != float64(600) {
		t.Fatalf("更新请求体错误: %v", m.lastBody)
	}
	if _, has := m.lastBody["line"]; has {
		t.Fatalf("更新不应传线路: %v", m.lastBody)
	}
	if m.lastQuery["subdomain"] != "home" || m.lastQuery["type"] != "A" {
		t.Fatalf("列表过滤参数错误: %v", m.lastQuery)
	}

	// 根域名不传主机过滤，按名称精确匹配；TTL 变化触发更新
	rec := Record{Zone: "example.com", Sub: "@", Type: "AAAA", Value: "2001:db8::1", TTL: 300}
	if got, err := p.Upsert(ctx, rec); err != nil || got != ActionCreated {
		t.Fatalf("got %s %v", got, err)
	}
	if m.lastBody["name"] != "@" || m.lastBody["ttl"] != float64(300) {
		t.Fatalf("创建请求体错误: %v", m.lastBody)
	}
	if m.lastQuery["subdomain"] != "" {
		t.Fatalf("根域名不应带主机过滤: %v", m.lastQuery)
	}
	if got, _ := p.Upsert(ctx, rec); got != ActionUnchanged {
		t.Fatalf("got %s", got)
	}
	rec.TTL = 600
	if got, err := p.Upsert(ctx, rec); err != nil || got != ActionUpdated {
		t.Fatalf("TTL 变化应更新: %s %v", got, err)
	}

	// 域名列表使用 {list,total} 包装时同样能解析
	m.wrapDomains = true
	if got, _ := p.Upsert(ctx, rec); got != ActionUnchanged {
		t.Fatalf("包装格式 got %s", got)
	}
	m.wrapDomains = false

	if _, err := p.Upsert(ctx, Record{Zone: "nope.com", Sub: "a", Type: "A", Value: "1.1.1.1"}); err == nil || !strings.Contains(err.Error(), "未找到域名") {
		t.Fatalf("未知域名应报错: %v", err)
	}
}

func TestHipmDnsMgrErrors(t *testing.T) {
	_, srv := newHipmMock(t)
	defer srv.Close()
	bad, err := New("hipmdnsmgr", Credentials{ID: srv.URL, Secret: "bad"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bad.Upsert(context.Background(), Record{Zone: "example.com", Sub: "a", Type: "A", Value: "1.1.1.1"}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("认证失败应报 401: %v", err)
	}
	if _, err := New("hipmdnsmgr", Credentials{ID: "dnsmgr.example.com", Secret: "x"}, nil); err == nil {
		t.Fatal("缺少协议的面板地址应报错")
	}
	if _, err := New("hipmdnsmgr", Credentials{ID: "https://a.com"}, nil); err == nil {
		t.Fatal("缺少令牌应报错")
	}
	// HTTP 200 但业务 code 非 0
	biz := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":403,"msg":"Permission denied"}`))
	}))
	defer biz.Close()
	p, _ := New("hipmdnsmgr", Credentials{ID: biz.URL, Secret: "tok"}, biz.Client())
	if _, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "a", Type: "A", Value: "1.1.1.1"}); err == nil || !strings.Contains(err.Error(), "Permission denied") {
		t.Fatalf("业务错误应透出: %v", err)
	}
	base, _ := hipmBaseURL("https://x.com/api/")
	if base != "https://x.com" {
		t.Fatalf("规范化错误: %s", base)
	}
}

func TestHipmDnsMgrTXTAndDelete(t *testing.T) {
	m, srv := newHipmMock(t)
	defer srv.Close()
	p := newHipmForTest(t, srv, srv.URL)
	ctx := context.Background()
	tp := p.(TXTProvider)

	if err := tp.AddTXT(ctx, "example.com", "_acme-challenge", "tok1"); err != nil {
		t.Fatal(err)
	}
	if m.lastBody["type"] != "TXT" || m.lastBody["name"] != "_acme-challenge" || m.lastBody["value"] != "tok1" || m.lastBody["ttl"] != float64(txtTTL) {
		t.Fatalf("TXT 请求体错误: %v", m.lastBody)
	}
	if err := tp.AddTXT(ctx, "example.com", "_acme-challenge", "tok2"); err != nil {
		t.Fatal(err)
	}
	if err := tp.DeleteTXT(ctx, "example.com", "_acme-challenge", "tok1"); err != nil {
		t.Fatal(err)
	}
	if len(m.recs) != 1 || m.recs[0]["value"] != "tok2" {
		t.Fatalf("应只删除 tok1: %v", m.recs)
	}
	if err := tp.DeleteTXT(ctx, "example.com", "_acme-challenge", "nope"); err != nil {
		t.Fatal(err)
	}

	// 删除动态域名解析：值一致才删除
	m.recs = nil
	dr := Record{Zone: "example.com", Sub: "home", Type: "A", Value: "203.0.113.1"}
	if _, err := p.Upsert(ctx, dr); err != nil {
		t.Fatal(err)
	}
	wrong := dr
	wrong.Value = "198.51.100.9"
	if _, err := Delete(ctx, p, wrong); err == nil || len(m.recs) != 1 {
		t.Fatalf("值不一致应拒绝删除: %v", err)
	}
	n, err := Delete(ctx, p, dr)
	if err != nil || n != 1 || len(m.recs) != 0 {
		t.Fatalf("n=%d err=%v recs=%v", n, err, m.recs)
	}
	if !SupportsDelete("hipmdnsmgr") || !SupportsTXT("hipmdnsmgr") {
		t.Fatal("应声明支持删除与 ACME")
	}
}
