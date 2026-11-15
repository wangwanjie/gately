package dnsprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClouDNSUpsertAndTXT(t *testing.T) {
	type rec struct{ ID, Type, Host, Value, TTL string }
	var records []rec
	next := 10
	var lastAdd map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("sub-auth-user") != "bob" || r.Form.Get("auth-password") != "pw" {
			_, _ = w.Write([]byte(`{"status":"Failed","statusDescription":"Invalid authentication, incorrect auth-id or auth-password."}`))
			return
		}
		if r.Form.Get("domain-name") != "example.com" {
			t.Errorf("domain-name = %s", r.Form.Get("domain-name"))
		}
		switch r.URL.Path {
		case "/dns/records.json":
			if len(records) == 0 {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			out := map[string]map[string]string{}
			for _, x := range records {
				out[x.ID] = map[string]string{"id": x.ID, "type": x.Type, "host": x.Host, "record": x.Value, "ttl": x.TTL, "status": "1"}
			}
			_ = json.NewEncoder(w).Encode(out)
		case "/dns/add-record.json":
			next++
			if r.Form.Get("record-type") == "" || r.Form.Get("record-type") != r.Form.Get("type") {
				t.Errorf("add-record 应同时带 record-type 与 type: %v", r.Form)
			}
			lastAdd = map[string]string{"type": r.Form.Get("type"), "host": r.Form.Get("host"), "ttl": r.Form.Get("ttl")}
			records = append(records, rec{strings.Repeat("1", 1) + string(rune('0'+next%10)) + "0", r.Form.Get("type"), r.Form.Get("host"), r.Form.Get("record"), r.Form.Get("ttl")})
			_, _ = w.Write([]byte(`{"status":"Success","statusDescription":"The record was added successfully.","data":{"id":1}}`))
		case "/dns/mod-record.json":
			for i := range records {
				if records[i].ID == r.Form.Get("record-id") {
					records[i].Value = r.Form.Get("record")
					records[i].TTL = r.Form.Get("ttl")
				}
			}
			_, _ = w.Write([]byte(`{"status":"Success","statusDescription":"The record was modified successfully."}`))
		case "/dns/delete-record.json":
			var keep []rec
			for _, x := range records {
				if x.ID != r.Form.Get("record-id") {
					keep = append(keep, x)
				}
			}
			records = keep
			_, _ = w.Write([]byte(`{"status":"Success","statusDescription":"The record was deleted successfully."}`))
		default:
			t.Errorf("意外请求 %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	p, err := New("cloudns", Credentials{ID: "bob", Secret: "pw", Extra: map[string]string{"authType": "sub-auth-user"}}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	p.(*cloudns).endpoint = srv.URL + "/dns/"
	runUpsertSequence(t, p, Record{Zone: "example.com", Sub: "home", Type: "A"})
	if lastAdd["type"] != "A" || lastAdd["host"] != "home" || lastAdd["ttl"] != "3600" {
		t.Fatalf("add 参数 = %v", lastAdd)
	}
	if len(records) != 1 || records[0].Value != "203.0.113.2" {
		t.Fatalf("records = %+v", records)
	}
	// 自定义 TTL 取整到允许值
	if got, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "home", Type: "A", Value: "203.0.113.2", TTL: 100}); err != nil || got != ActionUpdated {
		t.Fatalf("TTL 变化应更新: %s %v", got, err)
	}
	if records[0].TTL != "300" {
		t.Fatalf("TTL = %s", records[0].TTL)
	}

	// TXT
	records = nil
	txt := p.(TXTProvider)
	_ = txt.AddTXT(context.Background(), "example.com", "_acme-challenge", "v1")
	_ = txt.AddTXT(context.Background(), "example.com", "_acme-challenge", "v2")
	if len(records) != 2 || records[0].TTL != "60" {
		t.Fatalf("records = %+v", records)
	}
	_ = txt.DeleteTXT(context.Background(), "example.com", "_acme-challenge", "v1")
	if len(records) != 1 || records[0].Value != "v2" {
		t.Fatalf("删除后 = %+v", records)
	}

	// 按条删除解析
	records = []rec{{"500", "A", "home", "203.0.113.9", "3600"}}
	n, err := Delete(context.Background(), p, Record{Zone: "example.com", Sub: "home", Type: "A", Value: "203.0.113.9"})
	if err != nil || n != 1 || len(records) != 0 {
		t.Fatalf("n=%d err=%v records=%+v", n, err, records)
	}

	// 认证失败（HTTP 200 + status=Failed）
	bad, _ := New("cloudns", Credentials{ID: "bob", Secret: "x", Extra: map[string]string{"authType": "sub-auth-user"}}, srv.Client())
	bad.(*cloudns).endpoint = srv.URL + "/dns/"
	if _, err := bad.Upsert(context.Background(), Record{Zone: "example.com", Sub: "home", Type: "A", Value: "1.1.1.1"}); err == nil || !strings.Contains(err.Error(), "Invalid authentication") {
		t.Fatalf("应返回服务商错误: %v", err)
	}
	if _, err := New("cloudns", Credentials{ID: "1", Secret: "p", Extra: map[string]string{"authType": "bogus"}}, nil); err == nil {
		t.Fatal("非法认证类型应报错")
	}
	if _, err := New("cloudns", Credentials{ID: "1"}, nil); err == nil {
		t.Fatal("缺少密码应报错")
	}
}

func TestClouDNSDefaultAuthAndRoot(t *testing.T) {
	var hosts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("auth-id") != "42" {
			t.Errorf("默认应使用 auth-id: %v", r.Form)
		}
		if r.URL.Path == "/dns/add-record.json" {
			hosts = append(hosts, "["+r.Form.Get("host")+"]")
		}
		if r.URL.Path == "/dns/records.json" {
			// 另有一条同类型的子域名记录，不应被选中；list 的 host 过滤参数应为 "@"
			if r.Form.Get("host") != "@" || r.Form.Get("type") != "A" {
				t.Errorf("records.json 参数 = %v", r.Form)
			}
			_, _ = w.Write([]byte(`{"7":{"id":"7","type":"A","host":"www","record":"9.9.9.9","ttl":"3600"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"Success","statusDescription":"ok"}`))
	}))
	defer srv.Close()
	p, _ := New("cloudns", Credentials{ID: "42", Secret: "pw"}, srv.Client())
	p.(*cloudns).endpoint = srv.URL + "/dns/"
	if got, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "@", Type: "A", Value: "1.1.1.1"}); err != nil || got != ActionCreated {
		t.Fatalf("got %s %v", got, err)
	}
	if len(hosts) != 1 || hosts[0] != "[@]" {
		t.Fatalf("根域名 host = %v", hosts)
	}
}
