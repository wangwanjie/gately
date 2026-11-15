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

// nameComFake 模拟 Name.com 记录接口。
type nameComFake struct {
	t     *testing.T
	recs  []nameComRecord
	next  int
	perPg int
}

func (f *nameComFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u, p, ok := r.BasicAuth()
	if !ok || u != "user" || p != "tok" {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"message":"Unauthorized","details":"Authentication Error - Invalid Credentials"}`))
		return
	}
	const base = "/v4/domains/example.com/records"
	switch {
	case r.Method == "GET" && r.URL.Path == base:
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		per := f.perPg
		if per == 0 {
			per = 1000
		}
		start := min((page-1)*per, len(f.recs))
		end := min(start+per, len(f.recs))
		resp := map[string]any{"records": f.recs[start:end]}
		if end < len(f.recs) {
			resp["nextPage"] = page + 1
		}
		_ = json.NewEncoder(w).Encode(resp)
	case r.Method == "POST" && r.URL.Path == base:
		var rec nameComRecord
		_ = json.NewDecoder(r.Body).Decode(&rec)
		f.next++
		rec.ID = f.next
		f.recs = append(f.recs, rec)
		_ = json.NewEncoder(w).Encode(rec)
	case r.Method == "PUT" && strings.HasPrefix(r.URL.Path, base+"/"):
		id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, base+"/"))
		var rec nameComRecord
		_ = json.NewDecoder(r.Body).Decode(&rec)
		for i := range f.recs {
			if f.recs[i].ID == id {
				rec.ID = id
				f.recs[i] = rec
				_ = json.NewEncoder(w).Encode(rec)
				return
			}
		}
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, base+"/"):
		id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, base+"/"))
		for i := range f.recs {
			if f.recs[i].ID == id {
				f.recs = append(f.recs[:i], f.recs[i+1:]...)
				_, _ = w.Write([]byte(`{}`))
				return
			}
		}
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	default:
		f.t.Errorf("意外请求 %s %s", r.Method, r.URL)
		w.WriteHeader(404)
	}
}

func newNameComForTest(t *testing.T, f *nameComFake) (Provider, *httptest.Server) {
	srv := httptest.NewServer(f)
	p, err := New("namecom", Credentials{ID: "user", Secret: "tok"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	p.(*nameCom).endpoint = srv.URL
	return p, srv
}

func TestNameComUpsert(t *testing.T) {
	f := &nameComFake{t: t}
	p, srv := newNameComForTest(t, f)
	defer srv.Close()
	runUpsertSequence(t, p, Record{Zone: "example.com", Sub: "home", Type: "A"})
	if len(f.recs) != 1 || f.recs[0].Host != "home" || f.recs[0].Answer != "203.0.113.2" {
		t.Fatalf("记录不符: %+v", f.recs)
	}
	// 根域名主机为空；TTL 低于下限取 300
	if got, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "@", Type: "AAAA", Value: "2001:db8::1", TTL: 60}); err != nil || got != ActionCreated {
		t.Fatalf("got %s %v", got, err)
	}
	if f.recs[1].Host != "" || f.recs[1].TTL != 300 {
		t.Fatalf("根记录不符: %+v", f.recs[1])
	}
	if got, _ := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "@", Type: "AAAA", Value: "2001:db8::1", TTL: 300}); got != ActionUnchanged {
		t.Fatalf("got %s", got)
	}
	if got, _ := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "@", Type: "AAAA", Value: "2001:db8::1", TTL: 600}); got != ActionUpdated || f.recs[1].TTL != 600 {
		t.Fatalf("got %s %+v", got, f.recs[1])
	}
}

func TestNameComPaginationTXTDelete(t *testing.T) {
	f := &nameComFake{t: t, perPg: 2}
	for i := 0; i < 5; i++ {
		f.next++
		f.recs = append(f.recs, nameComRecord{ID: f.next, Host: "h" + strconv.Itoa(i), Type: "A", Answer: "10.0.0." + strconv.Itoa(i), TTL: 300})
	}
	p, srv := newNameComForTest(t, f)
	defer srv.Close()
	ctx := context.Background()
	if got, err := p.Upsert(ctx, Record{Zone: "example.com", Sub: "h4", Type: "A", Value: "10.0.0.4"}); err != nil || got != ActionUnchanged {
		t.Fatalf("翻页后应识别为未变化: %s %v", got, err)
	}
	txt := p.(TXTProvider)
	for _, v := range []string{"v1", "v2"} {
		if err := txt.AddTXT(ctx, "example.com", "_acme-challenge", v); err != nil {
			t.Fatal(err)
		}
	}
	if last := f.recs[len(f.recs)-1]; last.Type != "TXT" || last.Answer != "v2" || last.TTL != txtTTL {
		t.Fatalf("TXT 不符: %+v", last)
	}
	if err := txt.DeleteTXT(ctx, "example.com", "_acme-challenge", "v1"); err != nil {
		t.Fatal(err)
	}
	if len(f.recs) != 6 || f.recs[len(f.recs)-1].Answer != "v2" {
		t.Fatalf("应只删除 v1: %+v", f.recs)
	}
	rec := Record{Zone: "example.com", Sub: "h2", Type: "A", Value: "10.0.0.2"}
	if n, err := Delete(ctx, p, rec); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if len(f.recs) != 5 {
		t.Fatalf("应只删除 h2: %+v", f.recs)
	}
}

func TestNameComErrors(t *testing.T) {
	f := &nameComFake{t: t}
	srv := httptest.NewServer(f)
	defer srv.Close()
	p, _ := New("namecom", Credentials{ID: "user", Secret: "bad"}, srv.Client())
	p.(*nameCom).endpoint = srv.URL
	_, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "a", Type: "A", Value: "1.1.1.1"})
	if err == nil || !strings.Contains(err.Error(), "Invalid Credentials") {
		t.Fatalf("应返回服务商错误: %v", err)
	}
	if _, err := New("namecom", Credentials{ID: "user"}, nil); err == nil {
		t.Fatal("缺少 Token 应报错")
	}
	if m, ok := Lookup("namecom"); !ok || m.Title != "Name.com" || !m.SupportsDelete {
		t.Fatalf("Meta 不符: %+v", m)
	}
}
