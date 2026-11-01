package dnsprovider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// 选择规则：只有一条时直接删；多条时按最近同步的值挑；挑不出来时一条都不删。
func TestPickTargets(t *testing.T) {
	one := []remoteRecord{{id: "1", values: []string{"1.1.1.1"}}}
	two := []remoteRecord{{id: "1", values: []string{"1.1.1.1"}}, {id: "2", values: []string{"2.2.2.2"}}}
	dup := []remoteRecord{{id: "1", values: []string{"1.1.1.1"}}, {id: "2", values: []string{"1.1.1.1"}}}
	set := []remoteRecord{{id: "rs", values: []string{"1.1.1.1", "3.3.3.3"}}}
	cases := []struct {
		name  string
		recs  []remoteRecord
		value string
		want  string
	}{
		{"没有记录", nil, "1.1.1.1", ""},
		{"没有记录且值未知", nil, "", ""},
		{"一条且值相同", one, "1.1.1.1", "1"},
		// 只有一条时也要核对值：值未知或不一致时可能是用户自己的记录
		{"一条且值未知", one, "", "err"},
		{"一条且值已变", one, "9.9.9.9", "err"},
		{"多条按值挑选", two, "2.2.2.2", "2"},
		{"多条值未知", two, "", "err"},
		{"多条都不匹配", two, "9.9.9.9", "err"},
		{"多条同值都删除", dup, "1.1.1.1", "1,2"},
		{"记录集还有其他值", set, "1.1.1.1", "err"},
	}
	for _, c := range cases {
		got, err := pickTargets(c.recs, Record{Zone: "example.com", Sub: "www", Type: "A", Value: c.value})
		ids := "err"
		if err == nil {
			var s []string
			for _, r := range got {
				s = append(s, r.id)
			}
			ids = strings.Join(s, ",")
		}
		if ids != c.want {
			t.Errorf("%s: got %s want %s (%v)", c.name, ids, c.want, err)
		}
	}
	// TXT 去引号、CNAME 去结尾点且不区分大小写
	if !sameValue("TXT", `"hello"`, "hello") || !sameValue("CNAME", "Target.Example.com.", "target.example.com") || sameValue("A", "1.1.1.1", "1.1.1.2") {
		t.Fatal("记录值比较错误")
	}
}

// 阿里云：只删除默认线路上同名同类型的记录。
func TestAliDNSDelete(t *testing.T) {
	var deleted []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch q.Get("Action") {
		case "DescribeSubDomainRecords":
			_, _ = w.Write([]byte(`{"DomainRecords":{"Record":[
				{"RecordId":"r1","RR":"www","Type":"A","Value":"1.1.1.1","Line":"default"},
				{"RecordId":"r2","RR":"www","Type":"A","Value":"2.2.2.2","Line":"telecom"}]}}`))
		case "DeleteDomainRecord":
			deleted = append(deleted, q.Get("RecordId"))
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("意外调用 %s", q.Get("Action"))
		}
	}))
	defer srv.Close()
	p, _ := New("alidns", Credentials{ID: "ak", Secret: "sk"}, srv.Client())
	p.(*aliDNS).endpoint = srv.URL + "/"
	p.(*aliDNS).now = func() time.Time { return time.Unix(1700000000, 0) }
	n, err := Delete(context.Background(), p, Record{Zone: "example.com", Sub: "www", Type: "A", Value: "1.1.1.1"})
	if err != nil || n != 1 || strings.Join(deleted, ",") != "r1" {
		t.Fatalf("默认线路: %d %v %v", n, err, deleted)
	}
	deleted = nil
	n, err = Delete(context.Background(), p, Record{Zone: "example.com", Sub: "www", Type: "A", Line: "telecom", Value: "2.2.2.2"})
	if err != nil || n != 1 || strings.Join(deleted, ",") != "r2" {
		t.Fatalf("指定线路: %d %v %v", n, err, deleted)
	}
}

// Cloudflare：有多条同名记录且值都不匹配时报错，不删除任何记录；记录不存在时返回 0。
func TestCloudflareDelete(t *testing.T) {
	records := []map[string]any{
		{"id": "a", "type": "A", "name": "home.example.com", "content": "1.1.1.1"},
		{"id": "b", "type": "A", "name": "home.example.com", "content": "2.2.2.2"},
	}
	var deleted []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/zones":
			_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"z1","name":"example.com"}]}`))
		case r.Method == "GET" && r.URL.Path == "/zones/z1/dns_records":
			b, _ := json.Marshal(map[string]any{"success": true, "result": records})
			_, _ = w.Write(b)
		case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/zones/z1/dns_records/"):
			deleted = append(deleted, strings.TrimPrefix(r.URL.Path, "/zones/z1/dns_records/"))
			_, _ = w.Write([]byte(`{"success":true,"result":{}}`))
		default:
			t.Errorf("意外请求 %s %s", r.Method, r.URL)
		}
	}))
	defer srv.Close()
	p, _ := New("cloudflare", Credentials{Secret: "tok"}, srv.Client())
	p.(*cloudflare).endpoint = srv.URL
	ctx := context.Background()
	rec := Record{Zone: "example.com", Sub: "home", Type: "A"}

	if _, err := Delete(ctx, p, rec); err == nil || len(deleted) != 0 {
		t.Fatalf("值未知且有多条时应拒绝: %v %v", err, deleted)
	}
	rec.Value = "2.2.2.2"
	if n, err := Delete(ctx, p, rec); err != nil || n != 1 || deleted[0] != "b" {
		t.Fatalf("应按值删除 b: %d %v %v", n, err, deleted)
	}
	records = nil
	if n, err := Delete(ctx, p, rec); err != nil || n != 0 {
		t.Fatalf("记录不存在时应返回 0: %d %v", n, err)
	}
}

// 华为云：整个记录集删除。
func TestHuaweiCloudDelete(t *testing.T) {
	deleted := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/zones":
			_, _ = w.Write([]byte(`{"zones":[{"id":"z1","name":"example.com."}]}`))
		case r.Method == "GET":
			_, _ = w.Write([]byte(`{"recordsets":[{"id":"rs1","name":"www.example.com.","type":"A","records":["1.1.1.1"]}]}`))
		case r.Method == "DELETE":
			deleted = r.URL.Path
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	p, _ := New("huaweicloud", Credentials{ID: "ak", Secret: "sk"}, srv.Client())
	p.(*huaweiCloud).endpoint = srv.URL
	if n, err := Delete(context.Background(), p, Record{Zone: "example.com", Sub: "www", Type: "A", Value: "1.1.1.1"}); err != nil || n != 1 || deleted != "/v2/zones/z1/recordsets/rs1" {
		t.Fatalf("%d %v %s", n, err, deleted)
	}
}

// 腾讯云：主机记录模糊匹配返回的其他记录不会被删除。
func TestTencentCloudDelete(t *testing.T) {
	var deleted []float64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var params map[string]any
		_ = json.NewDecoder(r.Body).Decode(&params)
		switch r.Header.Get("X-TC-Action") {
		case "DescribeRecordList":
			_, _ = w.Write([]byte(`{"Response":{"RecordList":[
				{"RecordId":1,"Value":"1.1.1.1","Type":"A","Name":"www","Line":"默认"},
				{"RecordId":2,"Value":"1.1.1.1","Type":"A","Name":"www2","Line":"默认"}]}}`))
		case "DeleteRecord":
			deleted = append(deleted, params["RecordId"].(float64))
			_, _ = w.Write([]byte(`{"Response":{}}`))
		}
	}))
	defer srv.Close()
	p, _ := New("tencentcloud", Credentials{ID: "sid", Secret: "skey"}, srv.Client())
	p.(*tencentCloud).endpoint = srv.URL
	if n, err := Delete(context.Background(), p, Record{Zone: "example.com", Sub: "www", Type: "A", Value: "1.1.1.1"}); err != nil || n != 1 || len(deleted) != 1 || deleted[0] != 1 {
		t.Fatalf("%d %v %v", n, err, deleted)
	}
}

// 不支持删除的服务商返回 ErrDeleteUnsupported，描述中不声明删除能力。
func TestDeleteUnsupported(t *testing.T) {
	p, _ := New("duckdns", Credentials{Secret: "x"}, http.DefaultClient)
	if _, err := Delete(context.Background(), p, Record{Zone: "duckdns.org", Sub: "a", Type: "A"}); !errors.Is(err, ErrDeleteUnsupported) {
		t.Fatalf("应返回 ErrDeleteUnsupported: %v", err)
	}
	if m, _ := Lookup("duckdns"); m.SupportsDelete {
		t.Fatal("duckdns 不应声明支持删除")
	}
	if m, _ := Lookup("cloudflare"); !m.SupportsDelete {
		t.Fatal("cloudflare 应声明支持删除")
	}
	for _, m := range List() {
		if m.Name == "alidns" && !m.SupportsDelete {
			t.Fatal("列表中 alidns 应声明支持删除")
		}
	}
}
