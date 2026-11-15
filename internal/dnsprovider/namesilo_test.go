package dnsprovider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

type nsFakeRec struct {
	id, typ, host, value string
	ttl                  int
}

// nameSiloFake 模拟 NameSilo 接口；list 返回的 host 按 fullHost 决定是完整域名还是相对名。
type nameSiloFake struct {
	t        *testing.T
	recs     []nsFakeRec
	next     int
	fullHost bool
	calls    []string
}

func (f *nameSiloFake) reply(w http.ResponseWriter, code int, detail, inner string) {
	_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><namesilo><request><operation>x</operation></request><reply><code>%d</code><detail>%s</detail>%s</reply></namesilo>`, code, detail, inner)
}

func (f *nameSiloFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("key") != "k123" {
		f.reply(w, 280, "Invalid API Key", "")
		return
	}
	if q.Get("version") != "1" || q.Get("type") != "xml" || q.Get("domain") != "example.com" {
		f.t.Errorf("通用参数错误: %s", r.URL.RawQuery)
	}
	op := strings.TrimPrefix(r.URL.Path, "/api/")
	f.calls = append(f.calls, op)
	switch op {
	case "dnsListRecords":
		var sb strings.Builder
		for _, rec := range f.recs {
			host := rec.host
			if f.fullHost {
				if host == "" {
					host = "example.com"
				} else {
					host += ".example.com"
				}
			}
			fmt.Fprintf(&sb, `<resource_record><record_id>%s</record_id><type>%s</type><host>%s</host><value>%s</value><ttl>%d</ttl><distance>0</distance></resource_record>`, rec.id, rec.typ, host, rec.value, rec.ttl)
		}
		f.reply(w, 300, "success", sb.String())
	case "dnsAddRecord":
		f.next++
		ttl, _ := strconv.Atoi(q.Get("rrttl"))
		f.recs = append(f.recs, nsFakeRec{id: "id" + strconv.Itoa(f.next), typ: q.Get("rrtype"), host: q.Get("rrhost"), value: q.Get("rrvalue"), ttl: ttl})
		f.reply(w, 300, "success", "<record_id>id"+strconv.Itoa(f.next)+"</record_id>")
	case "dnsUpdateRecord":
		for i := range f.recs {
			if f.recs[i].id == q.Get("rrid") {
				f.recs[i].host = q.Get("rrhost")
				f.recs[i].value = q.Get("rrvalue")
				f.recs[i].ttl, _ = strconv.Atoi(q.Get("rrttl"))
				f.reply(w, 300, "success", "")
				return
			}
		}
		f.reply(w, 280, "Invalid record", "")
	case "dnsDeleteRecord":
		for i := range f.recs {
			if f.recs[i].id == q.Get("rrid") {
				f.recs = append(f.recs[:i], f.recs[i+1:]...)
				f.reply(w, 300, "success", "")
				return
			}
		}
		f.reply(w, 280, "Invalid record", "")
	default:
		f.t.Errorf("意外操作 %s", op)
	}
}

func newNameSiloForTest(t *testing.T, f *nameSiloFake, key string) (Provider, *httptest.Server) {
	srv := httptest.NewServer(f)
	p, err := New("namesilo", Credentials{Secret: key}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	p.(*nameSilo).endpoint = srv.URL + "/api"
	return p, srv
}

func TestNameSiloUpsert(t *testing.T) {
	for _, full := range []bool{false, true} {
		f := &nameSiloFake{t: t, fullHost: full}
		p, srv := newNameSiloForTest(t, f, "k123")
		runUpsertSequence(t, p, Record{Zone: "example.com", Sub: "home", Type: "A"})
		if len(f.recs) != 1 || f.recs[0].host != "home" || f.recs[0].value != "203.0.113.2" || f.recs[0].ttl != 3600 {
			t.Fatalf("full=%v 记录不符: %+v", full, f.recs)
		}
		// 根域名：rrhost 为空；TTL 低于下限时取 3600
		if got, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "@", Type: "AAAA", Value: "2001:db8::1", TTL: 120}); err != nil || got != ActionCreated {
			t.Fatalf("got %s %v", got, err)
		}
		if f.recs[1].host != "" || f.recs[1].ttl != 3600 {
			t.Fatalf("根记录不符: %+v", f.recs[1])
		}
		if got, _ := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "@", Type: "AAAA", Value: "2001:db8::1"}); got != ActionUnchanged {
			t.Fatalf("got %s", got)
		}
		// 多级子域名
		if got, _ := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "a.b", Type: "A", Value: "1.1.1.1"}); got != ActionCreated {
			t.Fatalf("got %s", got)
		}
		if got, _ := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "a.b", Type: "A", Value: "1.1.1.1"}); got != ActionUnchanged {
			t.Fatalf("got %s", got)
		}
		srv.Close()
	}
}

func TestNameSiloTXTAndDelete(t *testing.T) {
	f := &nameSiloFake{t: t, fullHost: true}
	p, srv := newNameSiloForTest(t, f, "k123")
	defer srv.Close()
	txt := p.(TXTProvider)
	ctx := context.Background()
	for _, v := range []string{"v1", "v2"} {
		if err := txt.AddTXT(ctx, "example.com", "_acme-challenge", v); err != nil {
			t.Fatal(err)
		}
	}
	if len(f.recs) != 2 || f.recs[0].typ != "TXT" || f.recs[0].host != "_acme-challenge" {
		t.Fatalf("TXT 记录不符: %+v", f.recs)
	}
	if err := txt.DeleteTXT(ctx, "example.com", "_acme-challenge", "v1"); err != nil {
		t.Fatal(err)
	}
	if len(f.recs) != 1 || f.recs[0].value != "v2" {
		t.Fatalf("应只删除 v1: %+v", f.recs)
	}
	rec := Record{Zone: "example.com", Sub: "home", Type: "A", Value: "203.0.113.1"}
	if _, err := p.Upsert(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if n, err := Delete(ctx, p, rec); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if len(f.recs) != 1 {
		t.Fatalf("A 记录应已删除: %+v", f.recs)
	}
}

func TestNameSiloErrors(t *testing.T) {
	f := &nameSiloFake{t: t}
	p, srv := newNameSiloForTest(t, f, "wrong")
	_, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "a", Type: "A", Value: "1.1.1.1"})
	if err == nil || !strings.Contains(err.Error(), "Invalid API Key") || strings.Contains(err.Error(), "wrong") {
		t.Fatalf("应返回服务商错误且不含密钥: %v", err)
	}
	srv.Close()
	// 连接失败的错误信息不得泄露 key
	_, err = p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "a", Type: "A", Value: "1.1.1.1"})
	if err == nil || strings.Contains(err.Error(), "wrong") {
		t.Fatalf("错误信息不得包含密钥: %v", err)
	}
	if _, err := New("namesilo", Credentials{}, nil); err == nil {
		t.Fatal("缺少 Key 应报错")
	}
}
