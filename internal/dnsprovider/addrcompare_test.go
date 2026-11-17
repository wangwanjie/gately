package dnsprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 服务商以展开、大写写法返回 IPv6 时，同步视为未变化，不发更新请求；删除时也能按地址匹配到记录。
func TestDeSECExpandedIPv6Unchanged(t *testing.T) {
	cur := &desecRRSet{Subname: "home", Type: "AAAA", Records: []string{"2001:0DB8:0000:0000:0000:0000:0000:0001"}, TTL: 3600}
	var writes []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			if cur == nil {
				w.WriteHeader(404)
				_, _ = w.Write([]byte(`{"detail":"Not found."}`))
				return
			}
			_ = json.NewEncoder(w).Encode(cur)
		case "DELETE":
			writes = append(writes, r.Method)
			cur = nil
			w.WriteHeader(204)
		default:
			writes = append(writes, r.Method)
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()
	p, err := New("desec", Credentials{Secret: "tok"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	p.(*desec).endpoint = srv.URL
	rec := Record{Zone: "example.com", Sub: "home", Type: "AAAA", Value: "2001:db8::1"}
	got, err := p.Upsert(context.Background(), rec)
	if err != nil || got != ActionUnchanged || len(writes) != 0 {
		t.Fatalf("同一地址的不同写法应视为未变化: %s %v %v", got, err, writes)
	}
	// 地址确实不同时仍然更新（测试服务器对 PUT 返回 500，说明发出了更新请求）
	if _, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "home", Type: "AAAA", Value: "2001:db8::2"}); err == nil || len(writes) != 1 || writes[0] != "PUT" {
		t.Fatalf("地址不同应发出更新: %v %v", err, writes)
	}
	writes = nil
	n, err := Delete(context.Background(), p, rec)
	if err != nil || n != 1 || cur != nil {
		t.Fatalf("删除应按地址匹配展开写法的记录: n=%d err=%v cur=%v", n, err, cur)
	}
}

// 非地址的值仍按原规则比较；无法解析为地址的 A 记录值退回字符串比较。
func TestSameValueNonAddress(t *testing.T) {
	if !sameValue("A", "not-an-ip", "not-an-ip") || sameValue("A", "not-an-ip", "203.0.113.1") {
		t.Fatal("无法解析的地址应按字符串比较")
	}
	if sameValue("CNAME", "a.example.com", "b.example.com") || !sameValue("CNAME", "A.example.com.", "a.example.com") {
		t.Fatal("CNAME 比较错误")
	}
	if sameValue("TXT", `"Hello"`, "hello") {
		t.Fatal("TXT 区分大小写")
	}
}
