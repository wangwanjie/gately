package dnsprovider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// aliESAFake 模拟 ESA 接口，记录表按 RecordId 存放。
type aliESAFake struct {
	t       *testing.T
	records []aliESARecord
	next    int64
	last    map[string]url.Values
}

func (f *aliESAFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	action := q.Get("Action")
	if f.last == nil {
		f.last = map[string]url.Values{}
	}
	f.last[action] = q
	// 校验签名：去掉 Signature 后按同一算法重新计算
	params := map[string]string{}
	for k := range q {
		if k != "Signature" {
			params[k] = q.Get(k)
		}
	}
	want := aliESASign(r.Method, params, "SK")
	if r.URL.RawQuery != want {
		f.t.Errorf("签名不一致:\n got %s\nwant %s", r.URL.RawQuery, want)
	}
	if q.Get("AccessKeyId") != "AK" || q.Get("Version") != "2024-09-10" || q.Get("SignatureMethod") != "HMAC-SHA1" {
		f.t.Errorf("公共参数错误: %v", q)
	}
	write := func(v any) {
		b, _ := json.Marshal(v)
		_, _ = w.Write(b)
	}
	switch action {
	case "ListSites":
		if r.Method != http.MethodGet {
			f.t.Errorf("ListSites 应为 GET")
		}
		if q.Get("SiteName") == "example.com" {
			write(map[string]any{"TotalCount": 1, "Sites": []aliESASite{{SiteID: 77, SiteName: "Example.com", AccessType: "NS"}}})
		} else {
			write(map[string]any{"TotalCount": 0, "Sites": []aliESASite{}})
		}
	case "ListRecords":
		if q.Get("SiteId") != "77" || q.Has("RecordMatchType") {
			f.t.Errorf("ListRecords 参数错误: %v", q)
		}
		var out []aliESARecord
		for _, rec := range f.records {
			if rec.RecordName == q.Get("RecordName") && rec.RecordType == q.Get("Type") {
				out = append(out, rec)
			}
		}
		write(map[string]any{"TotalCount": len(out), "Records": out})
	case "CreateRecord":
		if r.Method != http.MethodPost {
			f.t.Errorf("CreateRecord 应为 POST")
		}
		var data struct{ Value string }
		_ = json.Unmarshal([]byte(q.Get("Data")), &data)
		ttl, _ := strconv.Atoi(q.Get("Ttl"))
		f.next++
		rec := aliESARecord{RecordID: f.next, RecordName: q.Get("RecordName"), RecordType: q.Get("Type"), Ttl: ttl}
		rec.Data.Value = data.Value
		f.records = append(f.records, rec)
		write(map[string]any{"RecordId": f.next})
	case "UpdateRecord":
		id, _ := strconv.ParseInt(q.Get("RecordId"), 10, 64)
		var data struct{ Value string }
		_ = json.Unmarshal([]byte(q.Get("Data")), &data)
		ttl, _ := strconv.Atoi(q.Get("Ttl"))
		for i := range f.records {
			if f.records[i].RecordID == id {
				f.records[i].Data.Value = data.Value
				f.records[i].Ttl = ttl
			}
		}
		write(map[string]any{"RequestId": "x"})
	case "DeleteRecord":
		id, _ := strconv.ParseInt(q.Get("RecordId"), 10, 64)
		var keep []aliESARecord
		for _, rec := range f.records {
			if rec.RecordID != id {
				keep = append(keep, rec)
			}
		}
		f.records = keep
		write(map[string]any{"RequestId": "x"})
	case "Boom":
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"Code":"Record.TTLExceedLimit","Message":"ttl bad","RequestId":"x"}`))
	}
}

func newAliESATest(t *testing.T) (*aliESA, *aliESAFake, func()) {
	f := &aliESAFake{t: t}
	srv := httptest.NewServer(f)
	p, err := New("aliesa", Credentials{ID: "AK", Secret: "SK"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	a := p.(*aliESA)
	a.endpoint = srv.URL
	return a, f, srv.Close
}

func TestAliESAUpsert(t *testing.T) {
	a, f, done := newAliESATest(t)
	defer done()
	runUpsertSequence(t, a, Record{Zone: "example.com", Sub: "home", Type: "A"})
	if len(f.records) != 1 || f.records[0].RecordType != "A/AAAA" || f.records[0].Data.Value != "203.0.113.2" || f.records[0].Ttl != 600 {
		t.Fatalf("记录 = %+v", f.records)
	}
	if f.last["CreateRecord"].Get("RecordName") != "home.example.com" || f.last["CreateRecord"].Get("Proxied") != "" {
		t.Fatalf("Create 参数 = %v", f.last["CreateRecord"])
	}
	// 同名 IPv6 并入同一条记录，IPv4 保持不变
	if got, err := a.Upsert(t.Context(), Record{Zone: "example.com", Sub: "home", Type: "AAAA", Value: "2001:db8::1"}); err != nil || got != ActionUpdated {
		t.Fatalf("got %s %v", got, err)
	}
	if f.records[0].Data.Value != "203.0.113.2,2001:db8::1" {
		t.Fatalf("合并值 = %s", f.records[0].Data.Value)
	}
	if got, _ := a.Upsert(t.Context(), Record{Zone: "example.com", Sub: "home", Type: "AAAA", Value: "2001:0db8:0:0:0:0:0:1"}); got != ActionUnchanged {
		t.Fatalf("IPv6 写法不同但等价应为未变化: %s", got)
	}
	// 更新 IPv4 时保留 IPv6
	if got, _ := a.Upsert(t.Context(), Record{Zone: "example.com", Sub: "home", Type: "A", Value: "203.0.113.3", TTL: 120}); got != ActionUpdated {
		t.Fatalf("got %s", got)
	}
	if f.records[0].Data.Value != "203.0.113.3,2001:db8::1" || f.records[0].Ttl != 120 {
		t.Fatalf("记录 = %+v", f.records[0])
	}
	// 根域名记录名为站点名
	if _, err := a.Upsert(t.Context(), Record{Zone: "example.com", Sub: "@", Type: "A", Value: "1.1.1.1"}); err != nil {
		t.Fatal(err)
	}
	if f.records[1].RecordName != "example.com" {
		t.Fatalf("记录名 = %s", f.records[1].RecordName)
	}
}

func TestAliESATXTAndDelete(t *testing.T) {
	a, f, done := newAliESATest(t)
	defer done()
	ctx := t.Context()
	_ = a.AddTXT(ctx, "example.com", "_acme-challenge", "v1")
	_ = a.AddTXT(ctx, "example.com", "_acme-challenge", "v2")
	if len(f.records) != 2 || f.records[0].RecordType != "TXT" || f.records[0].Ttl != txtTTL || f.records[0].RecordName != "_acme-challenge.example.com" {
		t.Fatalf("记录 = %+v", f.records)
	}
	if err := a.DeleteTXT(ctx, "example.com", "_acme-challenge", "v1"); err != nil || len(f.records) != 1 || f.records[0].Data.Value != "v2" {
		t.Fatalf("err=%v records=%+v", err, f.records)
	}
	if err := a.DeleteTXT(ctx, "example.com", "_acme-challenge", "none"); err != nil || len(f.records) != 1 {
		t.Fatalf("err=%v", err)
	}

	// 动态域名删除：IPv4 与 IPv6 共用一条记录，只去掉本记录的地址
	f.records = nil
	v4 := Record{Zone: "example.com", Sub: "home", Type: "A", Value: "203.0.113.1"}
	v6 := Record{Zone: "example.com", Sub: "home", Type: "AAAA", Value: "2001:db8::1"}
	_, _ = a.Upsert(ctx, v4)
	_, _ = a.Upsert(ctx, v6)
	if n, err := Delete(ctx, a, Record{Zone: "example.com", Sub: "home", Type: "A", Value: "9.9.9.9"}); err == nil || n != 0 {
		t.Fatalf("值不一致应拒绝: %d %v", n, err)
	}
	if n, err := Delete(ctx, a, v4); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if len(f.records) != 1 || f.records[0].Data.Value != "2001:db8::1" {
		t.Fatalf("应保留 IPv6: %+v", f.records)
	}
	if n, err := Delete(ctx, a, v6); err != nil || n != 1 || len(f.records) != 0 {
		t.Fatalf("n=%d err=%v records=%+v", n, err, f.records)
	}
	if !SupportsDelete("aliesa") || !SupportsTXT("aliesa") {
		t.Fatal("应声明支持 TXT 与删除")
	}
}

func TestAliESAErrors(t *testing.T) {
	a, _, done := newAliESATest(t)
	defer done()
	_, err := a.Upsert(t.Context(), Record{Zone: "missing.org", Sub: "a", Type: "A", Value: "1.1.1.1"})
	if err == nil || !strings.Contains(err.Error(), "未找到站点") {
		t.Fatalf("err = %v", err)
	}
	err = a.call(t.Context(), http.MethodGet, "Boom", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "Record.TTLExceedLimit") || !strings.Contains(err.Error(), "ttl bad") {
		t.Fatalf("err = %v", err)
	}
	if _, err := New("aliesa", Credentials{ID: "a"}, nil); err == nil {
		t.Fatal("缺少 Secret 应报错")
	}
	if _, err := New("aliesa", Credentials{ID: "a", Secret: "b", Extra: map[string]string{"endpoint": "http://esa.example.com"}}, nil); err == nil {
		t.Fatal("非 https 接口地址应报错")
	}
	if p, err := New("aliesa", Credentials{ID: "a", Secret: "b"}, nil); err != nil || p.(*aliESA).endpoint != "https://esa.cn-hangzhou.aliyuncs.com" {
		t.Fatalf("默认接口地址 %v", err)
	}
}
