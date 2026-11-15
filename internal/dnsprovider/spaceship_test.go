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

// spaceshipFake 模拟 Spaceship 记录接口，记录保存在内存中。
type spaceshipFake struct {
	t     *testing.T
	items []spaceshipItem
	// pageCap 大于 0 时每页最多返回这么多条，用来验证翻页
	pageCap int
}

func (f *spaceshipFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-API-Key") != "key" || r.Header.Get("X-API-Secret") != "sec" {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"detail":"Invalid credentials"}`))
		return
	}
	if r.URL.Path != "/api/v1/dns/records/example.com" {
		f.t.Errorf("路径错误 %s", r.URL.Path)
	}
	switch r.Method {
	case "GET":
		take, _ := strconv.Atoi(r.URL.Query().Get("take"))
		skip, _ := strconv.Atoi(r.URL.Query().Get("skip"))
		if take < 1 || take > 500 {
			f.t.Errorf("take 越界: %d", take)
		}
		if f.pageCap > 0 && take > f.pageCap {
			take = f.pageCap
		}
		end := min(skip+take, len(f.items))
		start := min(skip, end)
		_ = json.NewEncoder(w).Encode(map[string]any{"items": f.items[start:end], "total": len(f.items)})
	case "PUT":
		var body struct {
			Items []spaceshipItem `json:"items"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		for _, it := range body.Items {
			replaced := false
			for i := range f.items {
				if f.items[i].Type == it.Type && f.items[i].Name == it.Name && f.items[i].content() == it.content() {
					f.items[i] = it
					replaced = true
				}
			}
			if !replaced {
				f.items = append(f.items, it)
			}
		}
		w.WriteHeader(204)
	case "DELETE":
		var body []spaceshipItem
		_ = json.NewDecoder(r.Body).Decode(&body)
		for _, d := range body {
			if d.TTL != 0 {
				f.t.Errorf("删除请求不应带 ttl: %+v", d)
			}
			for i := range f.items {
				if f.items[i].Type == d.Type && f.items[i].Name == d.Name && f.items[i].content() == d.content() {
					f.items = append(f.items[:i], f.items[i+1:]...)
					break
				}
			}
		}
		w.WriteHeader(204)
	default:
		f.t.Errorf("意外方法 %s", r.Method)
	}
}

func newSpaceshipForTest(t *testing.T, f *spaceshipFake) (Provider, *httptest.Server) {
	srv := httptest.NewServer(f)
	p, err := New("spaceship", Credentials{ID: "key", Secret: "sec"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	p.(*spaceship).endpoint = srv.URL + "/api/v1"
	return p, srv
}

func TestSpaceshipUpsert(t *testing.T) {
	f := &spaceshipFake{t: t}
	// 其他记录必须保持不变
	f.items = []spaceshipItem{{Type: "A", Name: "other", Address: "9.9.9.9", TTL: 3600}, {Type: "TXT", Name: "home", Value: "keep", TTL: 600}}
	p, srv := newSpaceshipForTest(t, f)
	defer srv.Close()
	runUpsertSequence(t, p, Record{Zone: "example.com", Sub: "home", Type: "A"})
	if len(f.items) != 3 || f.items[0].Address != "9.9.9.9" || f.items[1].Value != "keep" || f.items[2].Address != "203.0.113.2" || f.items[2].TTL != 600 {
		t.Fatalf("记录不符: %+v", f.items)
	}
	// 根域名用 @；TTL 超出上限取 3600
	if got, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "", Type: "AAAA", Value: "2001:db8::1", TTL: 86400}); err != nil || got != ActionCreated {
		t.Fatalf("got %s %v", got, err)
	}
	if last := f.items[len(f.items)-1]; last.Name != "@" || last.TTL != 3600 {
		t.Fatalf("根记录不符: %+v", last)
	}
	// 同名同类型有多条时收敛为一条
	f.items = append(f.items, spaceshipItem{Type: "A", Name: "multi", Address: "1.1.1.1", TTL: 600}, spaceshipItem{Type: "A", Name: "multi", Address: "2.2.2.2", TTL: 600})
	if got, _ := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "multi", Type: "A", Value: "2.2.2.2"}); got != ActionUpdated {
		t.Fatalf("got %s", got)
	}
	n := 0
	for _, it := range f.items {
		if it.Name == "multi" {
			n++
			if it.Address != "2.2.2.2" {
				t.Fatalf("保留了旧值: %+v", it)
			}
		}
	}
	if n != 1 {
		t.Fatalf("multi 应只剩一条: %+v", f.items)
	}
}

func TestSpaceshipPaginationTXTDelete(t *testing.T) {
	f := &spaceshipFake{t: t, pageCap: 2}
	for i := 0; i < 5; i++ {
		f.items = append(f.items, spaceshipItem{Type: "A", Name: "h" + strconv.Itoa(i), Address: "10.0.0." + strconv.Itoa(i), TTL: 600})
	}
	p, srv := newSpaceshipForTest(t, f)
	defer srv.Close()
	ctx := context.Background()
	if got, err := p.Upsert(ctx, Record{Zone: "example.com", Sub: "h4", Type: "A", Value: "10.0.0.4"}); err != nil || got != ActionUnchanged {
		t.Fatalf("翻页后应识别为未变化: %s %v", got, err)
	}
	txt := p.(TXTProvider)
	for _, v := range []string{"v1", "v2"} {
		if err := txt.AddTXT(ctx, "example.com", "_acme-challenge.www", v); err != nil {
			t.Fatal(err)
		}
	}
	last := f.items[len(f.items)-1]
	if last.Type != "TXT" || last.Name != "_acme-challenge.www" || last.Value != "v2" || last.TTL != txtTTL {
		t.Fatalf("TXT 不符: %+v", last)
	}
	if err := txt.DeleteTXT(ctx, "example.com", "_acme-challenge.www", "v1"); err != nil {
		t.Fatal(err)
	}
	if len(f.items) != 6 || f.items[len(f.items)-1].Value != "v2" {
		t.Fatalf("应只删除 v1: %+v", f.items)
	}
	rec := Record{Zone: "example.com", Sub: "h1", Type: "A", Value: "10.0.0.1"}
	if n, err := Delete(ctx, p, rec); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	for _, it := range f.items {
		if it.Name == "h1" {
			t.Fatalf("h1 应已删除: %+v", f.items)
		}
	}
	if len(f.items) != 5 {
		t.Fatalf("不应影响其他记录: %+v", f.items)
	}
}

func TestSpaceshipErrors(t *testing.T) {
	f := &spaceshipFake{t: t}
	srv := httptest.NewServer(f)
	defer srv.Close()
	p, _ := New("spaceship", Credentials{ID: "key", Secret: "bad"}, srv.Client())
	p.(*spaceship).endpoint = srv.URL + "/api/v1"
	_, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "a", Type: "A", Value: "1.1.1.1"})
	if err == nil || !strings.Contains(err.Error(), "Invalid credentials") {
		t.Fatalf("应返回服务商错误: %v", err)
	}
	if _, err := New("spaceship", Credentials{ID: "k"}, nil); err == nil {
		t.Fatal("缺少 Secret 应报错")
	}
}
