package dnsprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCallbackTimestampVariable(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query().Get("t") + "|" + r.URL.Query().Get("u")
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	p, err := New("callback", Credentials{Extra: map[string]string{"url": srv.URL + "/?t={timestamp}&u=#{timestamp}"}}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	before := time.Now().Unix()
	if _, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "www", Type: "A", Value: "1.1.1.1"}); err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(got, "|")
	if len(parts) != 2 || parts[0] != parts[1] {
		t.Fatalf("两种写法应得到同一个时间戳: %q", got)
	}
	ts, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || ts < before || ts > before+5 {
		t.Fatalf("时间戳不合理: %q", got)
	}
}

// 未配置 TTL 时 {ttl} 与 #{ttl} 取 600，配置后取配置值。
func TestCallbackTTLVariableDefault(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query().Get("a") + "|" + r.URL.Query().Get("b")
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	p, err := New("callback", Credentials{Extra: map[string]string{"url": srv.URL + "/?a={ttl}&b=#{ttl}"}}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "www", Type: "A", Value: "1.1.1.1"}); err != nil {
		t.Fatal(err)
	}
	if got != "600|600" {
		t.Fatalf("未配置 TTL 应为 600: %q", got)
	}
	if _, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "www", Type: "A", Value: "1.1.1.1", TTL: 120}); err != nil {
		t.Fatal(err)
	}
	if got != "120|120" {
		t.Fatalf("配置 TTL 应取配置值: %q", got)
	}
}
