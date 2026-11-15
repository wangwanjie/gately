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

func TestDeSEC(t *testing.T) {
	var cur *desecRRSet
	var reqs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token tok" {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"detail":"Invalid token."}`))
			return
		}
		reqs = append(reqs, r.Method+" "+r.URL.Path)
		isCollection := r.URL.Path == "/domains/example.com/rrsets/"
		switch {
		case r.Method == "GET" && !isCollection:
			if cur == nil {
				w.WriteHeader(404)
				_, _ = w.Write([]byte(`{"detail":"Not found."}`))
				return
			}
			_ = json.NewEncoder(w).Encode(cur)
		case r.Method == "POST" && isCollection:
			var body desecRRSet
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &body)
			cur = &body
			w.WriteHeader(201)
			_, _ = w.Write(b)
		case r.Method == "PUT" && !isCollection:
			var body desecRRSet
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &body)
			cur = &body
			_, _ = w.Write(b)
		case r.Method == "DELETE" && !isCollection:
			cur = nil
			w.WriteHeader(204)
		default:
			t.Errorf("意外请求 %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	p, err := New("desec", Credentials{Secret: "tok"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	p.(*desec).endpoint = srv.URL
	runUpsertSequence(t, p, Record{Zone: "example.com", Sub: "home", Type: "A"})
	if reqs[0] != "GET /domains/example.com/rrsets/home/A/" || reqs[1] != "POST /domains/example.com/rrsets/" {
		t.Fatalf("请求 = %v", reqs)
	}
	if reqs[len(reqs)-1] != "PUT /domains/example.com/rrsets/home/A/" {
		t.Fatalf("更新应使用 PUT: %v", reqs)
	}
	if cur.TTL != 3600 || cur.Subname != "home" || len(cur.Records) != 1 || cur.Records[0] != "203.0.113.2" {
		t.Fatalf("记录集 = %+v", cur)
	}

	// 根域名：创建时 subname 为空，路径用 @
	cur = nil
	reqs = nil
	if got, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "@", Type: "AAAA", Value: "::1", TTL: 999999}); err != nil || got != ActionCreated {
		t.Fatalf("got %s %v", got, err)
	}
	if reqs[0] != "GET /domains/example.com/rrsets/@/AAAA/" || cur.Subname != "" || cur.TTL != 86400 {
		t.Fatalf("reqs=%v cur=%+v", reqs, cur)
	}

	// TXT 值带引号；多值合并；重复添加不重复
	cur = nil
	txt := p.(TXTProvider)
	_ = txt.AddTXT(context.Background(), "example.com", "_acme-challenge", "v1")
	if cur == nil || len(cur.Records) != 1 || cur.Records[0] != `"v1"` {
		t.Fatalf("TXT = %+v", cur)
	}
	_ = txt.AddTXT(context.Background(), "example.com", "_acme-challenge", "v2")
	_ = txt.AddTXT(context.Background(), "example.com", "_acme-challenge", "v2")
	if len(cur.Records) != 2 || cur.Records[1] != `"v2"` {
		t.Fatalf("合并后 = %+v", cur)
	}
	_ = txt.DeleteTXT(context.Background(), "example.com", "_acme-challenge", "v1")
	if cur == nil || len(cur.Records) != 1 || cur.Records[0] != `"v2"` {
		t.Fatalf("删除后 = %+v", cur)
	}
	_ = txt.DeleteTXT(context.Background(), "example.com", "_acme-challenge", "v2")
	if cur != nil {
		t.Fatal("应整体删除")
	}

	// TXT 类型的 Upsert 自动补引号，引号值与原值视为未变化
	if got, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "t", Type: "TXT", Value: "hello"}); err != nil || got != ActionCreated {
		t.Fatalf("got %s %v", got, err)
	}
	if cur.Records[0] != `"hello"` {
		t.Fatalf("TXT = %v", cur.Records)
	}
	if got, _ := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "t", Type: "TXT", Value: "hello"}); got != ActionUnchanged {
		t.Fatalf("got %s", got)
	}

	// 删除解析：多值记录集拒绝删除，单值删除
	cur = &desecRRSet{Subname: "home", Type: "A", Records: []string{"1.1.1.1", "2.2.2.2"}, TTL: 3600}
	if _, err := Delete(context.Background(), p, Record{Zone: "example.com", Sub: "home", Type: "A", Value: "1.1.1.1"}); err == nil {
		t.Fatal("记录集有其他值时应拒绝删除")
	}
	cur = &desecRRSet{Subname: "home", Type: "A", Records: []string{"1.1.1.1"}, TTL: 3600}
	n, err := Delete(context.Background(), p, Record{Zone: "example.com", Sub: "home", Type: "A", Value: "1.1.1.1"})
	if err != nil || n != 1 || cur != nil {
		t.Fatalf("n=%d err=%v cur=%v", n, err, cur)
	}

	// 错误响应
	bad, _ := New("desec", Credentials{Secret: "x"}, srv.Client())
	bad.(*desec).endpoint = srv.URL
	if _, err := bad.Upsert(context.Background(), Record{Zone: "example.com", Sub: "home", Type: "A", Value: "1.1.1.1"}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("应返回 401 错误: %v", err)
	}
	if _, err := New("desec", Credentials{}, nil); err == nil {
		t.Fatal("缺少 token 应报错")
	}
}

// TestDeSECMinimumTTL 验证指定的 TTL 低于 3600 时按域名的 minimum_ttl 取下限，读取失败时退回 3600；
// 未指定 TTL 时沿用记录集现有 TTL。
func TestDeSECMinimumTTL(t *testing.T) {
	var cur *desecRRSet
	minimum := 60
	domainFail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/domains/example.dedyn.io/":
			if domainFail {
				w.WriteHeader(500)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "example.dedyn.io", "minimum_ttl": minimum})
		case r.Method == "GET":
			if cur == nil {
				w.WriteHeader(404)
				return
			}
			_ = json.NewEncoder(w).Encode(cur)
		case r.Method == "POST" || r.Method == "PUT":
			var body desecRRSet
			_ = json.NewDecoder(r.Body).Decode(&body)
			cur = &body
			_ = json.NewEncoder(w).Encode(body)
		default:
			t.Errorf("意外请求 %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	p, _ := New("desec", Credentials{Secret: "tok"}, srv.Client())
	p.(*desec).endpoint = srv.URL
	ctx := context.Background()
	// 域名允许 60 秒：120 原样使用
	if got, err := p.Upsert(ctx, Record{Zone: "example.dedyn.io", Sub: "home", Type: "A", Value: "203.0.113.1", TTL: 120}); err != nil || got != ActionCreated || cur.TTL != 120 {
		t.Fatalf("got %s %v cur=%+v", got, err, cur)
	}
	// 低于 minimum_ttl 时取 minimum_ttl
	minimum = 300
	if got, err := p.Upsert(ctx, Record{Zone: "example.dedyn.io", Sub: "home", Type: "A", Value: "203.0.113.1", TTL: 60}); err != nil || got != ActionUpdated || cur.TTL != 300 {
		t.Fatalf("got %s %v cur=%+v", got, err, cur)
	}
	// 未指定 TTL：值变化时沿用现有 300
	if got, err := p.Upsert(ctx, Record{Zone: "example.dedyn.io", Sub: "home", Type: "A", Value: "203.0.113.2"}); err != nil || got != ActionUpdated || cur.TTL != 300 {
		t.Fatalf("got %s %v cur=%+v", got, err, cur)
	}
	// 读不到 minimum_ttl 时退回 3600
	domainFail = true
	if got, err := p.Upsert(ctx, Record{Zone: "example.dedyn.io", Sub: "home", Type: "A", Value: "203.0.113.2", TTL: 60}); err != nil || got != ActionUpdated || cur.TTL != 3600 {
		t.Fatalf("got %s %v cur=%+v", got, err, cur)
	}
}
