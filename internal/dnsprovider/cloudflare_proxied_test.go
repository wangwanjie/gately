package dnsprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCloudflareProxiedTTL(t *testing.T) {
	var posted, patched map[string]any
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
}

// 同名同类型的多条记录逐条更新，未配置 TTL 时更新用 1（自动）。
func TestCloudflareUpdatesAllSameNameRecords(t *testing.T) {
	patches := map[string]map[string]any{}
	listJSON := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/zones":
			_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"z1","name":"example.com"}]}`))
		case r.Method == "GET" && r.URL.Path == "/zones/z1/dns_records":
			_, _ = w.Write([]byte(`{"success":true,"result":` + listJSON + `}`))
		case r.Method == "PATCH":
			patches[r.URL.Path] = readJSON(t, r)
			_, _ = w.Write([]byte(`{"success":true,"result":{}}`))
		default:
			t.Errorf("意外请求 %s %s", r.Method, r.URL)
		}
	}))
	defer srv.Close()
	p, _ := New("cloudflare", Credentials{Secret: "tok"}, srv.Client())
	p.(*cloudflare).endpoint = srv.URL
	ctx := context.Background()
	rec := Record{Zone: "example.com", Sub: "home", Type: "A", Value: "2.2.2.2"}

	// 两条都不是目标值：两条都改，TTL 未配置时为 1，未管理代理状态时不发送 proxied
	listJSON = `[{"id":"r1","type":"A","name":"home.example.com","content":"9.9.9.9","ttl":120,"proxied":false},` +
		`{"id":"r2","type":"A","name":"home.example.com","content":"8.8.8.8","ttl":120,"proxied":false}]`
	if got, err := p.Upsert(ctx, rec); err != nil || got != ActionUpdated {
		t.Fatalf("更新: %v %v", got, err)
	}
	if len(patches) != 2 {
		t.Fatalf("应更新两条记录: %v", patches)
	}
	for path, body := range patches {
		if body["content"] != "2.2.2.2" || body["ttl"].(float64) != 1 {
			t.Fatalf("%s 请求体错误: %v", path, body)
		}
		if _, ok := body["proxied"]; ok {
			t.Fatalf("未配置代理状态时不应发送 proxied: %v", body)
		}
	}

	// 一条已是目标值：只改另一条，整体仍算更新
	clear(patches)
	listJSON = `[{"id":"r1","type":"A","name":"home.example.com","content":"2.2.2.2","ttl":120,"proxied":false},` +
		`{"id":"r2","type":"A","name":"home.example.com","content":"8.8.8.8","ttl":120,"proxied":false}]`
	if got, err := p.Upsert(ctx, rec); err != nil || got != ActionUpdated {
		t.Fatalf("更新: %v %v", got, err)
	}
	if len(patches) != 1 || patches["/zones/z1/dns_records/r2"] == nil {
		t.Fatalf("应只更新 r2: %v", patches)
	}

	// 全部已是目标值（未配置 TTL 时不比较 TTL）：不变
	clear(patches)
	listJSON = `[{"id":"r1","type":"A","name":"home.example.com","content":"2.2.2.2","ttl":120,"proxied":false},` +
		`{"id":"r2","type":"A","name":"home.example.com","content":"2.2.2.2","ttl":300,"proxied":false}]`
	if got, err := p.Upsert(ctx, rec); err != nil || got != ActionUnchanged {
		t.Fatalf("全部已是目标值应不变: %v %v", got, err)
	}
	if len(patches) != 0 {
		t.Fatalf("不应改动任何记录: %v", patches)
	}

	// 配置了 TTL 时，记录 TTL 不同也要更新
	rec.TTL = 300
	if got, err := p.Upsert(ctx, rec); err != nil || got != ActionUpdated {
		t.Fatalf("TTL 不同应更新: %v %v", got, err)
	}
	if len(patches) != 1 || patches["/zones/z1/dns_records/r1"]["ttl"].(float64) != 300 {
		t.Fatalf("应只更新 r1 并发送 TTL 300: %v", patches)
	}
}

// 配置了代理状态（true/false）时同步已有记录，代理状态变化也算有变化；留空不管理。
func TestCloudflareProxiedSync(t *testing.T) {
	var patched map[string]any
	var posted map[string]any
	listJSON := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/zones":
			_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"z1","name":"example.com"}]}`))
		case r.Method == "GET":
			_, _ = w.Write([]byte(`{"success":true,"result":` + listJSON + `}`))
		case r.Method == "POST":
			posted = readJSON(t, r)
			_, _ = w.Write([]byte(`{"success":true,"result":{}}`))
		case r.Method == "PATCH":
			patched = readJSON(t, r)
			_, _ = w.Write([]byte(`{"success":true,"result":{}}`))
		}
	}))
	defer srv.Close()
	newProvider := func(proxied string) Provider {
		extra := map[string]string{}
		if proxied != "" {
			extra["proxied"] = proxied
		}
		p, _ := New("cloudflare", Credentials{Secret: "tok", Extra: extra}, srv.Client())
		p.(*cloudflare).endpoint = srv.URL
		return p
	}
	ctx := context.Background()
	rec := Record{Zone: "example.com", Sub: "home", Type: "A", Value: "1.1.1.1", TTL: 300}
	plain := `[{"id":"r1","type":"A","name":"home.example.com","content":"1.1.1.1","ttl":300,"proxied":false}]`
	proxiedRec := `[{"id":"r1","type":"A","name":"home.example.com","content":"1.1.1.1","ttl":1,"proxied":true}]`

	// 值相同、代理状态不同：同步为 true，TTL 强制为 1
	listJSON, patched = plain, nil
	if got, err := newProvider("true").Upsert(ctx, rec); err != nil || got != ActionUpdated {
		t.Fatalf("开启代理: %v %v", got, err)
	}
	if patched["proxied"] != true || patched["ttl"].(float64) != 1 {
		t.Fatalf("应开启代理并使用 TTL 1: %v", patched)
	}
	// 已是期望状态：不变
	listJSON, patched = proxiedRec, nil
	if got, err := newProvider("true").Upsert(ctx, rec); err != nil || got != ActionUnchanged {
		t.Fatalf("状态一致应不变: %v %v", got, err)
	}
	// 同步为 false：关闭代理，TTL 取配置值
	patched = nil
	if got, err := newProvider("false").Upsert(ctx, rec); err != nil || got != ActionUpdated {
		t.Fatalf("关闭代理: %v %v", got, err)
	}
	if patched["proxied"] != false || patched["ttl"].(float64) != 300 {
		t.Fatalf("应关闭代理并使用 TTL 300: %v", patched)
	}
	// 留空：不管理，已有代理记录保持不变
	patched = nil
	if got, err := newProvider("").Upsert(ctx, rec); err != nil || got != ActionUnchanged {
		t.Fatalf("未管理代理状态应不变: %v %v", got, err)
	}
	if patched != nil {
		t.Fatalf("不应发送更新: %v", patched)
	}
	// 留空时值变化：沿用记录的代理状态，TTL 强制 1，且不发送 proxied
	rec.Value = "2.2.2.2"
	if got, err := newProvider("").Upsert(ctx, rec); err != nil || got != ActionUpdated {
		t.Fatalf("更新: %v %v", got, err)
	}
	if _, ok := patched["proxied"]; ok || patched["ttl"].(float64) != 1 {
		t.Fatalf("不应发送 proxied 且 TTL 应为 1: %v", patched)
	}
	// 新建：留空默认 false，false/true 按配置
	listJSON = `[]`
	if _, err := newProvider("").Upsert(ctx, rec); err != nil || posted["proxied"] != false {
		t.Fatalf("新建默认不代理: %v %v", posted, err)
	}
	if _, err := newProvider("true").Upsert(ctx, rec); err != nil || posted["proxied"] != true || posted["ttl"].(float64) != 1 {
		t.Fatalf("新建应开启代理: %v %v", posted, err)
	}
}
