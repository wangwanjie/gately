package dnsprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// dnslaMock 模拟 DNS.LA 接口，记录按 id 保存。
type dnslaMock struct {
	t      *testing.T
	recs   map[string]map[string]any
	nextID int
	// 记录最近一次写入的请求体
	lastBody map[string]any
}

func newDnslaMock(t *testing.T) (*dnslaMock, *httptest.Server) {
	m := &dnslaMock{t: t, recs: map[string]map[string]any{}, nextID: 100}
	srv := httptest.NewServer(http.HandlerFunc(m.serve))
	return m, srv
}

func (m *dnslaMock) serve(w http.ResponseWriter, r *http.Request) {
	user, pass, ok := r.BasicAuth()
	if !ok || user != "apiid" || pass != "apisecret" {
		w.WriteHeader(401)
		return
	}
	reply := func(data any) {
		b, _ := json.Marshal(map[string]any{"code": 200, "msg": "", "data": data})
		_, _ = w.Write(b)
	}
	q := r.URL.Query()
	switch {
	case r.Method == "GET" && r.URL.Path == "/domain":
		if q.Get("domain") != "example.com" {
			b, _ := json.Marshal(map[string]any{"code": 404, "msg": "域名不存在"})
			_, _ = w.Write(b)
			return
		}
		reply(map[string]any{"id": "D1", "domain": "example.com"})
	case r.Method == "GET" && r.URL.Path == "/recordList":
		if q.Get("domainId") != "D1" || q.Get("pageIndex") == "" || q.Get("pageSize") == "" {
			m.t.Errorf("recordList 参数错误: %s", r.URL.RawQuery)
		}
		var results []map[string]any
		for _, rec := range m.recs {
			if fmt.Sprint(rec["type"]) == q.Get("type") && rec["host"] == q.Get("host") {
				results = append(results, rec)
			}
		}
		reply(map[string]any{"total": len(results), "results": results})
	case r.Method == "POST" && r.URL.Path == "/record":
		body := readJSON(m.t, r)
		m.lastBody = body
		if body["domainId"] != "D1" {
			m.t.Errorf("domainId = %v", body["domainId"])
		}
		for _, rec := range m.recs {
			if rec["host"] == body["host"] && rec["type"] == body["type"] && rec["data"] == body["data"] {
				b, _ := json.Marshal(map[string]any{"code": 600, "msg": "与已有记录冲突"})
				_, _ = w.Write(b)
				return
			}
		}
		m.nextID++
		id := fmt.Sprint(m.nextID)
		m.recs[id] = map[string]any{"id": id, "host": body["host"], "type": body["type"], "data": body["data"], "ttl": body["ttl"]}
		reply(map[string]any{"id": id})
	case r.Method == "PUT" && r.URL.Path == "/record":
		body := readJSON(m.t, r)
		m.lastBody = body
		rec, ok := m.recs[fmt.Sprint(body["id"])]
		if !ok {
			b, _ := json.Marshal(map[string]any{"code": 404, "msg": "记录不存在"})
			_, _ = w.Write(b)
			return
		}
		rec["data"] = body["data"]
		rec["ttl"] = body["ttl"]
		reply(map[string]any{"id": body["id"]})
	case r.Method == "DELETE" && r.URL.Path == "/record":
		delete(m.recs, q.Get("id"))
		reply(nil)
	default:
		m.t.Errorf("未预期的请求 %s %s", r.Method, r.URL.String())
		w.WriteHeader(404)
	}
}

func newDnslaForTest(t *testing.T, srv *httptest.Server) *dnsla {
	p, err := New("dnsla", Credentials{ID: "apiid", Secret: "apisecret"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	d := p.(*dnsla)
	d.endpoint = srv.URL
	return d
}

func TestDnsla(t *testing.T) {
	m, srv := newDnslaMock(t)
	defer srv.Close()
	d := newDnslaForTest(t, srv)

	rec := Record{Zone: "example.com", Sub: "home", Type: "A"}
	runUpsertSequence(t, d, rec)
	// 创建时使用默认 TTL 与类型枚举 1；之后更新沿用服务商处的 TTL
	for _, r := range m.recs {
		if r["type"] != float64(1) && fmt.Sprint(r["type"]) != "1" {
			t.Fatalf("A 记录类型应为 1: %v", r["type"])
		}
		if fmt.Sprint(r["ttl"]) != "600" {
			t.Fatalf("默认 TTL 应为 600: %v", r["ttl"])
		}
	}

	// IPv6 使用类型 28，根域名主机记录为 @
	got, err := d.Upsert(context.Background(), Record{Zone: "example.com", Sub: "@", Type: "AAAA", Value: "2001:db8::1", TTL: 300})
	if err != nil || got != ActionCreated {
		t.Fatalf("got %s %v", got, err)
	}
	if m.lastBody["type"] != float64(28) || m.lastBody["host"] != "@" || m.lastBody["ttl"] != float64(300) {
		t.Fatalf("请求体错误: %v", m.lastBody)
	}
	// TTL 变化也触发更新
	got, err = d.Upsert(context.Background(), Record{Zone: "example.com", Sub: "@", Type: "AAAA", Value: "2001:db8::1", TTL: 600})
	if err != nil || got != ActionUpdated {
		t.Fatalf("TTL 变化应更新: %s %v", got, err)
	}

	if _, err := d.Upsert(context.Background(), Record{Zone: "example.com", Sub: "x", Type: "MX", Value: "1"}); err == nil {
		t.Fatal("不支持的记录类型应报错")
	}
	if _, err := d.Upsert(context.Background(), Record{Zone: "other.com", Sub: "x", Type: "A", Value: "1.1.1.1"}); err == nil || !strings.Contains(err.Error(), "域名不存在") {
		t.Fatalf("业务错误应透出: %v", err)
	}
}

func TestDnslaAuthError(t *testing.T) {
	_, srv := newDnslaMock(t)
	defer srv.Close()
	p, _ := New("dnsla", Credentials{ID: "apiid", Secret: "bad"}, srv.Client())
	p.(*dnsla).endpoint = srv.URL
	if _, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "a", Type: "A", Value: "1.1.1.1"}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("认证失败应报 401: %v", err)
	}
	if _, err := New("dnsla", Credentials{ID: "x"}, nil); err == nil {
		t.Fatal("缺少 Secret 应报错")
	}
}

func TestDnslaTXT(t *testing.T) {
	m, srv := newDnslaMock(t)
	defer srv.Close()
	d := newDnslaForTest(t, srv)
	ctx := context.Background()

	if err := d.AddTXT(ctx, "example.com", "_acme-challenge", "tok1"); err != nil {
		t.Fatal(err)
	}
	if m.lastBody["type"] != float64(16) || m.lastBody["host"] != "_acme-challenge" || m.lastBody["data"] != "tok1" {
		t.Fatalf("TXT 请求体错误: %v", m.lastBody)
	}
	if err := d.AddTXT(ctx, "example.com", "_acme-challenge", "tok2"); err != nil {
		t.Fatal(err)
	}
	// 重复添加同值：服务商返回冲突，按成功处理
	if err := d.AddTXT(ctx, "example.com", "_acme-challenge", "tok2"); err != nil {
		t.Fatalf("冲突应视为成功: %v", err)
	}
	if len(m.recs) != 2 {
		t.Fatalf("应有 2 条记录: %d", len(m.recs))
	}
	if err := d.DeleteTXT(ctx, "example.com", "_acme-challenge", "tok1"); err != nil {
		t.Fatal(err)
	}
	if len(m.recs) != 1 {
		t.Fatalf("应只剩 1 条记录: %d", len(m.recs))
	}
	for _, r := range m.recs {
		if r["data"] != "tok2" {
			t.Fatalf("误删了其他值: %v", r)
		}
	}
	// 值不存在时不报错
	if err := d.DeleteTXT(ctx, "example.com", "_acme-challenge", "nope"); err != nil {
		t.Fatal(err)
	}
}

func TestDnslaDelete(t *testing.T) {
	m, srv := newDnslaMock(t)
	defer srv.Close()
	d := newDnslaForTest(t, srv)
	ctx := context.Background()
	rec := Record{Zone: "example.com", Sub: "home", Type: "A", Value: "203.0.113.1"}
	if _, err := d.Upsert(ctx, rec); err != nil {
		t.Fatal(err)
	}
	n, err := Delete(ctx, d, rec)
	if err != nil || n != 1 || len(m.recs) != 0 {
		t.Fatalf("n=%d err=%v recs=%v", n, err, m.recs)
	}
	// 值与服务商处不一致时不删除
	if _, err := d.Upsert(ctx, rec); err != nil {
		t.Fatal(err)
	}
	rec.Value = "198.51.100.9"
	if _, err := Delete(ctx, d, rec); err == nil || len(m.recs) != 1 {
		t.Fatalf("值不一致应拒绝删除: %v", err)
	}
	if !SupportsDelete("dnsla") || !SupportsTXT("dnsla") {
		t.Fatal("应声明支持删除与 ACME")
	}
}
