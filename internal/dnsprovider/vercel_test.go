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

// vercelFake 模拟 Vercel DNS 接口，记录保存在内存中。
type vercelFake struct {
	t    *testing.T
	recs []map[string]any
	next int
	// pageSize 大于 0 时按时间戳翻页，每页返回这么多条
	pageSize int
	team     string
}

func (f *vercelFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer tok" {
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"Not authorized"}}`))
		return
	}
	if got := r.URL.Query().Get("teamId"); got != f.team {
		f.t.Errorf("teamId = %q, want %q", got, f.team)
	}
	p := r.URL.Path
	switch {
	case r.Method == "GET" && p == "/v5/domains/example.com/records":
		start := 0
		if u := r.URL.Query().Get("until"); u != "" {
			start, _ = strconv.Atoi(u)
		}
		end := len(f.recs)
		resp := map[string]any{}
		if f.pageSize > 0 && start+f.pageSize < end {
			end = start + f.pageSize
			resp["pagination"] = map[string]any{"count": end - start, "next": end, "prev": nil}
		} else {
			resp["pagination"] = map[string]any{"count": end - start, "next": nil, "prev": nil}
		}
		resp["records"] = f.recs[start:end]
		_ = json.NewEncoder(w).Encode(resp)
	case r.Method == "POST" && p == "/v2/domains/example.com/records":
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		f.next++
		b["id"] = "rec_" + strconv.Itoa(f.next)
		if _, ok := b["ttl"]; !ok {
			b["ttl"] = float64(60)
		}
		f.recs = append(f.recs, b)
		_, _ = w.Write([]byte(`{"uid":"` + b["id"].(string) + `"}`))
	case r.Method == "PATCH" && strings.HasPrefix(p, "/v1/domains/records/"):
		id := strings.TrimPrefix(p, "/v1/domains/records/")
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		for _, rec := range f.recs {
			if rec["id"] == id {
				for k, v := range b {
					rec[k] = v
				}
				_, _ = w.Write([]byte(`{"id":"` + id + `"}`))
				return
			}
		}
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"error":{"code":"not_found","message":"no record"}}`))
	case r.Method == "DELETE" && strings.HasPrefix(p, "/v2/domains/example.com/records/"):
		id := strings.TrimPrefix(p, "/v2/domains/example.com/records/")
		for i, rec := range f.recs {
			if rec["id"] == id {
				f.recs = append(f.recs[:i], f.recs[i+1:]...)
				_, _ = w.Write([]byte(`{}`))
				return
			}
		}
		w.WriteHeader(404)
	default:
		f.t.Errorf("意外请求 %s %s", r.Method, r.URL)
		w.WriteHeader(404)
	}
}

func newVercelForTest(t *testing.T, f *vercelFake, extra map[string]string) (Provider, *httptest.Server) {
	srv := httptest.NewServer(f)
	p, err := New("vercel", Credentials{Secret: "tok", Extra: extra}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	p.(*vercel).endpoint = srv.URL
	return p, srv
}

func TestVercelUpsert(t *testing.T) {
	f := &vercelFake{t: t}
	p, srv := newVercelForTest(t, f, nil)
	defer srv.Close()
	runUpsertSequence(t, p, Record{Zone: "example.com", Sub: "home", Type: "A"})
	if len(f.recs) != 1 || f.recs[0]["name"] != "home" || f.recs[0]["value"] != "203.0.113.2" {
		t.Fatalf("记录不符: %v", f.recs)
	}
	// 根域名名称为空字符串
	if got, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "@", Type: "AAAA", Value: "2001:db8::1", TTL: 30}); err != nil || got != ActionCreated {
		t.Fatalf("got %s %v", got, err)
	}
	root := f.recs[1]
	if root["name"] != "" || root["ttl"] != float64(60) {
		t.Fatalf("根记录名称应为空且 TTL 取最小 60: %v", root)
	}
	// 值大小写不同视为相同
	if got, _ := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "@", Type: "AAAA", Value: "2001:DB8::1", TTL: 60}); got != ActionUnchanged {
		t.Fatalf("got %s", got)
	}
	// TTL 变化触发更新
	if got, _ := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "@", Type: "AAAA", Value: "2001:db8::1", TTL: 300}); got != ActionUpdated {
		t.Fatalf("got %s", got)
	}
	if f.recs[1]["ttl"] != float64(300) {
		t.Fatalf("ttl 未更新: %v", f.recs[1])
	}
}

func TestVercelTeamAndPagination(t *testing.T) {
	f := &vercelFake{t: t, team: "team_abc", pageSize: 2}
	for i := 0; i < 5; i++ {
		f.next++
		f.recs = append(f.recs, map[string]any{"id": "rec_" + strconv.Itoa(f.next), "name": "h" + strconv.Itoa(i), "type": "A", "value": "10.0.0." + strconv.Itoa(i), "ttl": float64(60)})
	}
	p, srv := newVercelForTest(t, f, map[string]string{"teamId": "team_abc"})
	defer srv.Close()
	// 目标记录在第三页，翻页后应识别为未变化
	if got, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "h4", Type: "A", Value: "10.0.0.4"}); err != nil || got != ActionUnchanged {
		t.Fatalf("got %s %v", got, err)
	}
}

func TestVercelTXTAndDelete(t *testing.T) {
	f := &vercelFake{t: t}
	p, srv := newVercelForTest(t, f, nil)
	defer srv.Close()
	txt := p.(TXTProvider)
	ctx := context.Background()
	if err := txt.AddTXT(ctx, "example.com", "_acme-challenge", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := txt.AddTXT(ctx, "example.com", "_acme-challenge", "v2"); err != nil {
		t.Fatal(err)
	}
	if len(f.recs) != 2 || f.recs[0]["type"] != "TXT" || f.recs[0]["ttl"] != float64(txtTTL) {
		t.Fatalf("TXT 记录不符: %v", f.recs)
	}
	if err := txt.DeleteTXT(ctx, "example.com", "_acme-challenge", "v1"); err != nil {
		t.Fatal(err)
	}
	if len(f.recs) != 1 || f.recs[0]["value"] != "v2" {
		t.Fatalf("应只删除 v1: %v", f.recs)
	}
	if err := txt.DeleteTXT(ctx, "example.com", "_acme-challenge", "none"); err != nil {
		t.Fatal(err)
	}

	// 按条删除 A 记录
	rec := Record{Zone: "example.com", Sub: "home", Type: "A", Value: "203.0.113.1"}
	if _, err := p.Upsert(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if n, err := Delete(ctx, p, rec); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if len(f.recs) != 1 {
		t.Fatalf("A 记录应已删除: %v", f.recs)
	}
}

func TestVercelErrors(t *testing.T) {
	f := &vercelFake{t: t}
	srv := httptest.NewServer(f)
	defer srv.Close()
	p, _ := New("vercel", Credentials{Secret: "bad"}, srv.Client())
	p.(*vercel).endpoint = srv.URL
	_, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "a", Type: "A", Value: "1.1.1.1"})
	if err == nil || !strings.Contains(err.Error(), "Not authorized") {
		t.Fatalf("应返回服务商错误: %v", err)
	}
	if _, err := New("vercel", Credentials{}, nil); err == nil {
		t.Fatal("缺少 Token 应报错")
	}
	if m, ok := Lookup("vercel"); !ok || !m.SupportsDelete || len(m.Extra) != 1 {
		t.Fatalf("Meta 不符: %+v", m)
	}
}
