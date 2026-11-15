package dnsprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAliDNSDefaultLineAndTTL(t *testing.T) {
	var updated map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch q.Get("Action") {
		case "DescribeSubDomainRecords":
			_, _ = w.Write([]byte(`{"TotalCount":2,"DomainRecords":{"Record":[` +
				`{"RecordId":"tel","RR":"www","Type":"A","Value":"9.9.9.9","TTL":600,"Line":"telecom"},` +
				`{"RecordId":"def","RR":"www","Type":"A","Value":"1.1.1.1","TTL":300,"Line":"default"}]}}`))
		case "UpdateDomainRecord":
			updated = map[string]string{}
			for k := range q {
				updated[k] = q.Get(k)
			}
			_, _ = w.Write([]byte(`{"RecordId":"def"}`))
		default:
			t.Errorf("意外动作 %s", q.Get("Action"))
		}
	}))
	defer srv.Close()
	p, _ := New("alidns", Credentials{ID: "ak", Secret: "sk"}, srv.Client())
	p.(*aliDNS).endpoint = srv.URL + "/"
	got, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "www", Type: "A", Value: "2.2.2.2"})
	if err != nil || got != ActionUpdated {
		t.Fatalf("%v %v", got, err)
	}
	if updated["RecordId"] != "def" {
		t.Fatalf("应更新默认线路记录: %v", updated)
	}
	if updated["TTL"] != "300" {
		t.Fatalf("未指定 TTL 应沿用 300: %v", updated)
	}
}

// DNSPod 传统接口：未指定 TTL 时修改记录沿用现有 TTL。
