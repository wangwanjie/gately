package dnsprovider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGcore(t *testing.T) {
	var cur *gcoreRRSet
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "APIKey tok" {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/zones/example.com/") {
			t.Errorf("路径错误 %s", r.URL.Path)
		}
		methods = append(methods, r.Method+" "+r.URL.Path)
		switch r.Method {
		case "GET":
			if cur == nil {
				w.WriteHeader(404)
				_, _ = w.Write([]byte(`{"error":"rrset not found"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(cur)
		case "POST", "PUT":
			var body gcoreRRSet
			b, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(b, &body); err != nil {
				t.Errorf("请求体错误: %s", b)
			}
			cur = &body
			_, _ = w.Write([]byte(`{}`))
		case "DELETE":
			cur = nil
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	p, err := New("gcore", Credentials{Secret: "tok"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	p.(*gcore).endpoint = srv.URL
	runUpsertSequence(t, p, Record{Zone: "example.com", Sub: "home", Type: "A"})
	if got := methods[0]; got != "GET /zones/example.com/home.example.com/A" {
		t.Fatalf("首个请求 = %s", got)
	}
	if methods[1] != "POST /zones/example.com/home.example.com/A" {
		t.Fatalf("创建应使用 POST: %v", methods)
	}
	if cur.TTL != 120 || len(cur.Records) != 1 || cur.Records[0].first() != "203.0.113.2" || !cur.Records[0].Enabled {
		t.Fatalf("记录集 = %+v", cur)
	}
	// 根域名使用完整域名作为记录集名称
	cur = nil
	if got, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "@", Type: "AAAA", Value: "::1", TTL: 300}); err != nil || got != ActionCreated {
		t.Fatalf("got %s %v", got, err)
	}
	if cur.TTL != 300 {
		t.Fatalf("TTL = %d", cur.TTL)
	}

	// TXT：两个值共存，逐个删除
	cur = nil
	txt := p.(TXTProvider)
	if err := txt.AddTXT(context.Background(), "example.com", "_acme-challenge", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := txt.AddTXT(context.Background(), "example.com", "_acme-challenge", "v2"); err != nil {
		t.Fatal(err)
	}
	if len(cur.Records) != 2 {
		t.Fatalf("records = %+v", cur)
	}
	_ = txt.DeleteTXT(context.Background(), "example.com", "_acme-challenge", "v1")
	if len(cur.Records) != 1 || cur.Records[0].first() != "v2" {
		t.Fatalf("删除后 = %+v", cur)
	}
	_ = txt.DeleteTXT(context.Background(), "example.com", "_acme-challenge", "v2")
	if cur != nil {
		t.Fatal("应整体删除")
	}

	// 删除解析
	cur = &gcoreRRSet{TTL: 120, Records: []gcoreRR{{Content: []any{"203.0.113.9"}, Enabled: true}}}
	n, err := Delete(context.Background(), p, Record{Zone: "example.com", Sub: "home", Type: "A", Value: "203.0.113.9"})
	if err != nil || n != 1 || cur != nil {
		t.Fatalf("n=%d err=%v cur=%v", n, err, cur)
	}

	// 错误响应
	bad, _ := New("gcore", Credentials{Secret: "x"}, srv.Client())
	bad.(*gcore).endpoint = srv.URL
	if _, err := bad.Upsert(context.Background(), Record{Zone: "example.com", Sub: "home", Type: "A", Value: "1.1.1.1"}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("应返回 401 错误: %v", err)
	}
	if _, err := New("gcore", Credentials{}, nil); err == nil {
		t.Fatal("缺少 token 应报错")
	}
}
