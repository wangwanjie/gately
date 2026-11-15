package dnsprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDynadot(t *testing.T) {
	var last map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/set_ddns" {
			t.Errorf("请求不符: %s %s", r.Method, r.URL.Path)
		}
		q := r.URL.Query()
		last = map[string]string{}
		for k := range q {
			last[k] = q.Get(k)
		}
		if q.Get("pwd") != "ddnspw" {
			_, _ = w.Write([]byte(`{"status":"error","error_code":-1,"content":["invalid password"]}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"success","error_code":0,"content":["ok"]}`))
	}))
	defer srv.Close()
	p, err := New("dynadot", Credentials{Secret: "ddnspw"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	p.(*dynadot).endpoint = srv.URL + "/set_ddns"
	ctx := context.Background()

	// 子域名：只带 subDomain，不带根域名；未指定 TTL 取 600
	if got, err := p.Upsert(ctx, Record{Zone: "example.com", Sub: "home.lab", Type: "A", Value: "203.0.113.1"}); err != nil || got != ActionUpdated {
		t.Fatalf("got %s %v", got, err)
	}
	if last["domain"] != "example.com" || last["subDomain"] != "home.lab" || last["type"] != "A" || last["ip"] != "203.0.113.1" ||
		last["ttl"] != "600" || last["containRoot"] != "false" {
		t.Fatalf("参数不符: %v", last)
	}
	// 根域名：subDomain 为 "@"，containRoot 为 true
	if _, err := p.Upsert(ctx, Record{Zone: "example.com", Sub: "@", Type: "AAAA", Value: "2001:db8::1", TTL: 300}); err != nil {
		t.Fatal(err)
	}
	if last["subDomain"] != "@" || last["containRoot"] != "true" || last["type"] != "AAAA" || last["ttl"] != "300" {
		t.Fatalf("根域名参数不符: %v", last)
	}
	// 不支持的类型
	if _, err := p.Upsert(ctx, Record{Zone: "example.com", Sub: "a", Type: "TXT", Value: "x"}); err == nil {
		t.Fatal("TXT 应报错")
	}
	if _, ok := p.(TXTProvider); ok {
		t.Fatal("Dynadot 不应实现 TXT")
	}
	if _, ok := p.(recordDeleter); ok {
		t.Fatal("Dynadot 不应实现删除")
	}
	// 密码错误
	bad, _ := New("dynadot", Credentials{Secret: "wrongpw"}, srv.Client())
	bad.(*dynadot).endpoint = srv.URL + "/set_ddns"
	if _, err := bad.Upsert(ctx, Record{Zone: "example.com", Sub: "a", Type: "A", Value: "1.1.1.1"}); err == nil || !strings.Contains(err.Error(), "invalid password") {
		t.Fatalf("应返回服务商错误: %v", err)
	}
	// 连接失败不泄露密码
	srv.Close()
	if _, err := p.Upsert(ctx, Record{Zone: "example.com", Sub: "a", Type: "A", Value: "1.1.1.1"}); err == nil || strings.Contains(err.Error(), "ddnspw") {
		t.Fatalf("错误信息不得包含密码: %v", err)
	}
	if _, err := New("dynadot", Credentials{}, nil); err == nil {
		t.Fatal("缺少密码应报错")
	}
}
