package dnsprovider

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// trafficRouteFake 模拟火山引擎 DNS 接口。
type trafficRouteFake struct {
	t       *testing.T
	records []trafficRouteRecord
	next    int
	last    map[string]map[string]any
}

func (f *trafficRouteFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	q := r.URL.Query()
	action := q.Get("Action")
	// 重新计算签名并比对
	want := trafficRouteAuthorization("AK", "SK", r.Method, r.Host, r.URL.RawQuery, r.Header.Get("Content-Type"),
		sha256Hex(body), r.Header.Get("X-Date"))
	if r.Header.Get("Authorization") != want {
		f.t.Errorf("鉴权头不一致: %s", r.Header.Get("Authorization"))
	}
	if r.Header.Get("X-Content-Sha256") != sha256Hex(body) || q.Get("Version") != "2018-08-01" {
		f.t.Errorf("请求头或版本错误: %v %v", r.Header, q)
	}
	params := map[string]any{}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &params)
	}
	if f.last == nil {
		f.last = map[string]map[string]any{}
	}
	f.last[action] = params
	write := func(result any) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ResponseMetadata": map[string]any{"RequestId": "x", "Action": action}, "Result": result})
	}
	isRead := action == "ListZones" || action == "ListRecords"
	if action != "Boom" && isRead != (r.Method == http.MethodGet) {
		f.t.Errorf("%s 使用了 %s", action, r.Method)
	}
	switch action {
	case "ListZones":
		if q.Get("Key") == "example.com" {
			write(map[string]any{"Total": 2, "Zones": []map[string]any{{"ZID": 11, "ZoneName": "example.com.cn"}, {"ZID": 12, "ZoneName": "Example.com"}}})
		} else {
			write(map[string]any{"Total": 0, "Zones": []any{}})
		}
	case "ListRecords":
		if q.Get("ZID") != "12" || q.Get("SearchMode") != "exact" {
			f.t.Errorf("ListRecords 参数 = %v", q)
		}
		var out []trafficRouteRecord
		for _, rec := range f.records {
			if rec.Host == q.Get("Host") && rec.Type == q.Get("Type") {
				out = append(out, rec)
			}
		}
		write(map[string]any{"TotalCount": len(out), "Records": out})
	case "CreateRecord":
		f.next++
		rec := trafficRouteRecord{RecordID: strconv.Itoa(1000 + f.next), Host: params["Host"].(string), Type: params["Type"].(string),
			Value: params["Value"].(string), Line: params["Line"].(string), TTL: 600}
		if v, ok := params["TTL"]; ok {
			rec.TTL = int(v.(float64))
		}
		f.records = append(f.records, rec)
		write(map[string]any{"RecordID": rec.RecordID})
	case "UpdateRecord":
		for i := range f.records {
			if f.records[i].RecordID == params["RecordID"] {
				f.records[i].Value = params["Value"].(string)
				if v, ok := params["TTL"]; ok {
					f.records[i].TTL = int(v.(float64))
				}
			}
		}
		write(map[string]any{})
	case "DeleteRecord":
		var keep []trafficRouteRecord
		for _, rec := range f.records {
			if rec.RecordID != params["RecordID"] {
				keep = append(keep, rec)
			}
		}
		f.records = keep
		write(map[string]any{})
	case "Boom":
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"ResponseMetadata":{"RequestId":"x","Error":{"Code":"ErrZoneNotFound","Message":"zone missing"}}}`))
	}
}

func newTrafficRouteTest(t *testing.T) (*trafficRoute, *trafficRouteFake, func()) {
	f := &trafficRouteFake{t: t}
	srv := httptest.NewServer(f)
	p, err := New("trafficroute", Credentials{ID: "AK", Secret: "SK"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	tr := p.(*trafficRoute)
	tr.endpoint = srv.URL
	return tr, f, srv.Close
}

func TestTrafficRouteUpsert(t *testing.T) {
	tr, f, done := newTrafficRouteTest(t)
	defer done()
	runUpsertSequence(t, tr, Record{Zone: "example.com", Sub: "home", Type: "A"})
	if len(f.records) != 1 || f.records[0].Value != "203.0.113.2" || f.records[0].Line != "default" || f.records[0].Host != "home" {
		t.Fatalf("记录 = %+v", f.records)
	}
	up := f.last["UpdateRecord"]
	if up["Host"] != "home" || up["Line"] != "default" || up["Type"] != "A" {
		t.Fatalf("Update 请求体 = %v", up)
	}
	if _, ok := f.last["CreateRecord"]["TTL"]; ok {
		t.Fatal("未指定 TTL 时不应传 TTL")
	}
	// 指定 TTL 时变化也触发更新；根域名主机记录为 @
	if got, _ := tr.Upsert(t.Context(), Record{Zone: "example.com", Sub: "home", Type: "A", Value: "203.0.113.2", TTL: 900}); got != ActionUpdated || f.records[0].TTL != 900 {
		t.Fatalf("got %s ttl=%d", got, f.records[0].TTL)
	}
	if got, err := tr.Upsert(t.Context(), Record{Zone: "example.com", Sub: "@", Type: "AAAA", Value: "2001:db8::1", TTL: 600}); err != nil || got != ActionCreated {
		t.Fatalf("got %s %v", got, err)
	}
	if f.records[1].Host != "@" || f.records[1].TTL != 600 {
		t.Fatalf("记录 = %+v", f.records[1])
	}
}

func TestTrafficRouteTXTAndDelete(t *testing.T) {
	tr, f, done := newTrafficRouteTest(t)
	defer done()
	ctx := t.Context()
	_ = tr.AddTXT(ctx, "example.com", "_acme-challenge", "v1")
	_ = tr.AddTXT(ctx, "example.com", "_acme-challenge", "v2")
	if len(f.records) != 2 || f.records[0].Type != "TXT" || f.records[0].TTL != txtTTL || f.records[0].Host != "_acme-challenge" {
		t.Fatalf("记录 = %+v", f.records)
	}
	if err := tr.DeleteTXT(ctx, "example.com", "_acme-challenge", "v1"); err != nil || len(f.records) != 1 || f.records[0].Value != "v2" {
		t.Fatalf("err=%v records=%+v", err, f.records)
	}
	if err := tr.DeleteTXT(ctx, "example.com", "_acme-challenge", "none"); err != nil || len(f.records) != 1 {
		t.Fatalf("err=%v", err)
	}

	rec := Record{Zone: "example.com", Sub: "home", Type: "A", Value: "203.0.113.1"}
	_, _ = tr.Upsert(ctx, rec)
	if n, err := Delete(ctx, tr, Record{Zone: "example.com", Sub: "home", Type: "A", Value: "9.9.9.9"}); err == nil || n != 0 {
		t.Fatalf("值不一致应拒绝: %d %v", n, err)
	}
	if n, err := Delete(ctx, tr, rec); err != nil || n != 1 || len(f.records) != 1 {
		t.Fatalf("n=%d err=%v records=%+v", n, err, f.records)
	}
	if !SupportsDelete("trafficroute") || !SupportsTXT("trafficroute") {
		t.Fatal("应声明支持 TXT 与删除")
	}
}

func TestTrafficRouteErrors(t *testing.T) {
	tr, _, done := newTrafficRouteTest(t)
	defer done()
	_, err := tr.Upsert(t.Context(), Record{Zone: "missing.org", Sub: "a", Type: "A", Value: "1.1.1.1"})
	if err == nil || !strings.Contains(err.Error(), "未找到域名") {
		t.Fatalf("err = %v", err)
	}
	err = tr.call(t.Context(), http.MethodGet, "Boom", nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "ErrZoneNotFound") || !strings.Contains(err.Error(), "zone missing") {
		t.Fatalf("err = %v", err)
	}
	if _, err := New("trafficroute", Credentials{ID: "a"}, nil); err == nil {
		t.Fatal("缺少 Secret 应报错")
	}
}
