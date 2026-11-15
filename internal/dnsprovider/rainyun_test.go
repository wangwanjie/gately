package dnsprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

type rainyunMock struct {
	t        *testing.T
	recs     []map[string]any
	nextID   int
	lastBody map[string]any
	// 域名列表请求的 options，用于校验过滤条件
	lastOptions string
	// 记录列表请求次数，用于校验翻页
	listCalls int
}

func newRainyunMock(t *testing.T) (*rainyunMock, *httptest.Server) {
	m := &rainyunMock{t: t, nextID: 1000}
	return m, httptest.NewServer(http.HandlerFunc(m.serve))
}

func (m *rainyunMock) serve(w http.ResponseWriter, r *http.Request) {
	reply := func(code int, msg string, data any) {
		b, _ := json.Marshal(map[string]any{"code": code, "message": msg, "data": data})
		_, _ = w.Write(b)
	}
	if r.Header.Get("x-api-key") != "key" {
		w.WriteHeader(401)
		reply(401, "invalid api key", nil)
		return
	}
	switch {
	case r.Method == "GET" && r.URL.Path == "/product/domain":
		m.lastOptions = r.URL.Query().Get("options")
		reply(200, "", map[string]any{"TotalRecords": 2, "Records": []map[string]any{
			{"id": 7, "domain": "other.com"},
			{"id": 42, "domain": "example.com"},
		}})
	case r.Method == "GET" && r.URL.Path == "/product/domain/42/dns/":
		m.listCalls++
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		page, _ := strconv.Atoi(r.URL.Query().Get("page_no"))
		if limit == 0 || page == 0 {
			m.t.Errorf("缺少分页参数: %s", r.URL.RawQuery)
		}
		// 按每页 limit 条切片，便于测试翻页
		start := (page - 1) * limit
		end := start + limit
		if start > len(m.recs) {
			start = len(m.recs)
		}
		if end > len(m.recs) {
			end = len(m.recs)
		}
		reply(200, "", map[string]any{"TotalRecords": len(m.recs), "Records": m.recs[start:end]})
	case r.Method == "POST" && r.URL.Path == "/product/domain/42/dns":
		body := readJSON(m.t, r)
		m.lastBody = body
		m.nextID++
		m.recs = append(m.recs, map[string]any{
			"record_id": m.nextID, "host": body["host"], "type": body["type"], "value": body["value"],
			"line": body["line"], "ttl": body["ttl"], "level": body["level"],
		})
		reply(200, "", nil)
	case r.Method == "PATCH" && r.URL.Path == "/product/domain/42/dns":
		body := readJSON(m.t, r)
		m.lastBody = body
		for _, rec := range m.recs {
			if fmt.Sprint(rec["record_id"]) == fmt.Sprint(body["record_id"]) {
				rec["value"] = body["value"]
				rec["ttl"] = body["ttl"]
				reply(200, "", nil)
				return
			}
		}
		reply(404, "记录不存在", nil)
	case r.Method == "DELETE" && r.URL.Path == "/product/domain/42/dns":
		id := r.URL.Query().Get("record_id")
		for i, rec := range m.recs {
			if strconv.Itoa(int(rec["record_id"].(int))) == id {
				m.recs = append(m.recs[:i], m.recs[i+1:]...)
				reply(200, "", nil)
				return
			}
		}
		reply(404, "记录不存在", nil)
	default:
		m.t.Errorf("未预期的请求 %s %s", r.Method, r.URL.String())
		w.WriteHeader(404)
	}
}

func newRainyunForTest(t *testing.T, srv *httptest.Server) *rainyun {
	p, err := New("rainyun", Credentials{Secret: "key"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	y := p.(*rainyun)
	y.endpoint = srv.URL + "/product"
	return y
}

func TestRainyun(t *testing.T) {
	m, srv := newRainyunMock(t)
	defer srv.Close()
	y := newRainyunForTest(t, srv)

	runUpsertSequence(t, y, Record{Zone: "example.com", Sub: "home", Type: "A"})
	if len(m.recs) != 1 {
		t.Fatalf("应只有 1 条记录: %v", m.recs)
	}
	if m.lastBody["host"] != "home" || m.lastBody["line"] != "DEFAULT" || m.lastBody["type"] != "A" || m.lastBody["value"] != "203.0.113.2" {
		t.Fatalf("更新请求体错误: %v", m.lastBody)
	}
	if m.lastBody["record_id"] == nil || m.lastBody["ttl"] != float64(600) {
		t.Fatalf("更新应带 record_id 与 TTL: %v", m.lastBody)
	}
	// 域名 ID 通过名称过滤查得
	if !strings.Contains(m.lastOptions, `"domains.Domain":"example.com"`) {
		t.Fatalf("域名列表应按名称过滤: %s", m.lastOptions)
	}

	// 根域名 IPv6，线路与 TTL 自定义
	got, err := y.Upsert(context.Background(), Record{Zone: "example.com", Sub: "@", Type: "AAAA", Value: "2001:db8::1", TTL: 300, Line: "ltel"})
	if err != nil || got != ActionCreated {
		t.Fatalf("got %s %v", got, err)
	}
	if m.lastBody["host"] != "@" || m.lastBody["line"] != "LTEL" || m.lastBody["ttl"] != float64(300) {
		t.Fatalf("创建请求体错误: %v", m.lastBody)
	}
	// 同名同类型但线路不同视为另一条记录
	got, err = y.Upsert(context.Background(), Record{Zone: "example.com", Sub: "@", Type: "AAAA", Value: "2001:db8::1"})
	if err != nil || got != ActionCreated {
		t.Fatalf("默认线路应新建: %s %v", got, err)
	}

	if _, err := y.Upsert(context.Background(), Record{Zone: "nope.com", Sub: "a", Type: "A", Value: "1.1.1.1"}); err == nil || !strings.Contains(err.Error(), "未找到域名") {
		t.Fatalf("未知域名应报错: %v", err)
	}
}

func TestRainyunPaging(t *testing.T) {
	m, srv := newRainyunMock(t)
	defer srv.Close()
	y := newRainyunForTest(t, srv)
	// 目标记录排在第 2 页，验证翻页能找到它而不是重复创建
	for i := 0; i < 100; i++ {
		m.recs = append(m.recs, map[string]any{"record_id": i + 1, "host": "h" + strconv.Itoa(i), "type": "A", "value": "10.0.0.1", "line": "DEFAULT", "ttl": 600, "level": 10})
	}
	m.recs = append(m.recs, map[string]any{"record_id": 500, "host": "home", "type": "A", "value": "203.0.113.1", "line": "DEFAULT", "ttl": 600, "level": 10})
	got, err := y.Upsert(context.Background(), Record{Zone: "example.com", Sub: "home", Type: "A", Value: "203.0.113.1"})
	if err != nil || got != ActionUnchanged {
		t.Fatalf("got %s %v", got, err)
	}
	if m.listCalls != 2 {
		t.Fatalf("应请求 2 页: %d", m.listCalls)
	}
}

func TestRainyunErrors(t *testing.T) {
	_, srv := newRainyunMock(t)
	defer srv.Close()
	p, _ := New("rainyun", Credentials{Secret: "bad"}, srv.Client())
	p.(*rainyun).endpoint = srv.URL + "/product"
	if _, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "a", Type: "A", Value: "1.1.1.1"}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("认证失败应报 401: %v", err)
	}
	if _, err := New("rainyun", Credentials{}, nil); err == nil {
		t.Fatal("缺少密钥应报错")
	}
	// HTTP 200 但业务 code 非 200
	biz := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":30001,"message":"余额不足"}`))
	}))
	defer biz.Close()
	p2, _ := New("rainyun", Credentials{Secret: "key"}, biz.Client())
	p2.(*rainyun).endpoint = biz.URL
	if _, err := p2.Upsert(context.Background(), Record{Zone: "example.com", Sub: "a", Type: "A", Value: "1.1.1.1"}); err == nil || !strings.Contains(err.Error(), "余额不足") {
		t.Fatalf("业务错误应透出: %v", err)
	}
}

func TestRainyunTXTAndDelete(t *testing.T) {
	m, srv := newRainyunMock(t)
	defer srv.Close()
	y := newRainyunForTest(t, srv)
	ctx := context.Background()

	if err := y.AddTXT(ctx, "example.com", "_acme-challenge", "tok1"); err != nil {
		t.Fatal(err)
	}
	if m.lastBody["type"] != "TXT" || m.lastBody["host"] != "_acme-challenge" || m.lastBody["value"] != "tok1" || m.lastBody["ttl"] != float64(600) {
		t.Fatalf("TXT 请求体错误: %v", m.lastBody)
	}
	if err := y.AddTXT(ctx, "example.com", "_acme-challenge", "tok2"); err != nil {
		t.Fatal(err)
	}
	if err := y.DeleteTXT(ctx, "example.com", "_acme-challenge", "tok1"); err != nil {
		t.Fatal(err)
	}
	if len(m.recs) != 1 || m.recs[0]["value"] != "tok2" {
		t.Fatalf("应只删除 tok1: %v", m.recs)
	}
	if err := y.DeleteTXT(ctx, "example.com", "_acme-challenge", "nope"); err != nil {
		t.Fatal(err)
	}

	// 删除动态域名解析
	rec := Record{Zone: "example.com", Sub: "home", Type: "A", Value: "203.0.113.1"}
	if _, err := y.Upsert(ctx, rec); err != nil {
		t.Fatal(err)
	}
	n, err := Delete(ctx, y, rec)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	for _, r := range m.recs {
		if r["host"] == "home" {
			t.Fatalf("记录应已删除: %v", m.recs)
		}
	}
	if !SupportsDelete("rainyun") || !SupportsTXT("rainyun") {
		t.Fatal("应声明支持删除与 ACME")
	}
}
