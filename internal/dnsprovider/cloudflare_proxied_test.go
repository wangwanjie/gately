package dnsprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCloudflareProxiedTTLAndMultiRecords(t *testing.T) {
	var posted, patched map[string]any
	var patchedPath string
	listJSON := `[]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/zones":
			_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"z1","name":"example.com"}]}`))
		case r.Method == "GET" && r.URL.Path == "/zones/z1/dns_records":
			_, _ = w.Write([]byte(`{"success":true,"result":` + listJSON + `}`))
		case r.Method == "POST":
			posted = readJSON(t, r)
			_, _ = w.Write([]byte(`{"success":true,"result":{}}`))
		case r.Method == "PATCH":
			patched = readJSON(t, r)
			patchedPath = r.URL.Path
			_, _ = w.Write([]byte(`{"success":true,"result":{}}`))
		default:
			t.Errorf("意外请求 %s %s", r.Method, r.URL)
		}
	}))
	defer srv.Close()
	p, _ := New("cloudflare", Credentials{Secret: "tok", Extra: map[string]string{"proxied": "true"}}, srv.Client())
	p.(*cloudflare).endpoint = srv.URL
	ctx := context.Background()

	// 新建且开启代理：指定 TTL 也必须发送 1
	if got, err := p.Upsert(ctx, Record{Zone: "example.com", Sub: "home", Type: "A", Value: "1.1.1.1", TTL: 300}); err != nil || got != ActionCreated {
		t.Fatalf("创建: %v %v", got, err)
	}
	if posted["ttl"].(float64) != 1 || posted["proxied"] != true {
		t.Fatalf("代理记录应使用自动 TTL: %v", posted)
	}

	// 已有代理记录（TTL 1），配置 TTL 300 且值相同：不应反复更新
	listJSON = `[{"id":"r1","type":"A","name":"home.example.com","content":"1.1.1.1","ttl":1,"proxied":true}]`
	if got, err := p.Upsert(ctx, Record{Zone: "example.com", Sub: "home", Type: "A", Value: "1.1.1.1", TTL: 300}); err != nil || got != ActionUnchanged {
		t.Fatalf("代理记录不应因 TTL 差异更新: %v %v", got, err)
	}
	// 值变化时 TTL 仍固定为 1
	patched = nil
	if got, err := p.Upsert(ctx, Record{Zone: "example.com", Sub: "home", Type: "A", Value: "2.2.2.2", TTL: 300}); err != nil || got != ActionUpdated {
		t.Fatalf("更新: %v %v", got, err)
	}
	if patched["ttl"].(float64) != 1 {
		t.Fatalf("代理记录更新时 TTL 应为 1: %v", patched)
	}

	// 同名多条记录，其中一条已是目标值：视为不变，不改动另一条
	listJSON = `[{"id":"r1","type":"A","name":"home.example.com","content":"9.9.9.9","ttl":120,"proxied":false},` +
		`{"id":"r2","type":"A","name":"home.example.com","content":"2.2.2.2","ttl":120,"proxied":false}]`
	patched = nil
	if got, err := p.Upsert(ctx, Record{Zone: "example.com", Sub: "home", Type: "A", Value: "2.2.2.2"}); err != nil || got != ActionUnchanged {
		t.Fatalf("多条记录中已有目标值应不变: %v %v", got, err)
	}
	if patched != nil {
		t.Fatalf("不应改动任何记录: %v", patched)
	}
	// 目标值不在其中时改第一条，未指定 TTL 则不发送 ttl（保留手工设置的 120）
	if got, err := p.Upsert(ctx, Record{Zone: "example.com", Sub: "home", Type: "A", Value: "3.3.3.3"}); err != nil || got != ActionUpdated {
		t.Fatalf("更新: %v %v", got, err)
	}
	if patchedPath != "/zones/z1/dns_records/r1" {
		t.Fatalf("应更新第一条: %s", patchedPath)
	}
	if _, ok := patched["ttl"]; ok {
		t.Fatalf("未指定 TTL 时不应发送 ttl: %v", patched)
	}
}

// 阿里云：未指定线路时只改默认线路的记录；未指定 TTL 时沿用记录现有 TTL。
