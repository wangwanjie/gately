package dnsprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// edgeOneFake 模拟 EdgeOne 接口：站点列表（支持分页）与记录表。
type edgeOneFake struct {
	t       *testing.T
	zones   []map[string]string
	records []edgeOneRecord
	next    int
	actions []string
	// 最近一次各动作的请求体
	last map[string]map[string]any
	// 非空时所有动作返回该业务错误
	failCode string
}

func (f *edgeOneFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	action := r.Header.Get("X-TC-Action")
	f.actions = append(f.actions, action)
	// 校验鉴权：用请求头里的时间戳按同一算法重新计算签名
	sec, _ := strconv.ParseInt(r.Header.Get("X-TC-Timestamp"), 10, 64)
	want := tc3Authorization("AKID", "SKEY", "teo", r.Host, action, body, time.Unix(sec, 0))
	if r.Header.Get("Authorization") != want {
		f.t.Errorf("签名不一致: %s", r.Header.Get("Authorization"))
	}
	if r.Method != http.MethodPost || r.Header.Get("X-TC-Version") != "2022-09-01" ||
		r.Header.Get("Content-Type") != "application/json; charset=utf-8" {
		f.t.Errorf("请求头错误: %s %v", r.Method, r.Header)
	}
	params := map[string]any{}
	_ = json.Unmarshal(body, &params)
	if f.last == nil {
		f.last = map[string]map[string]any{}
	}
	f.last[action] = params
	if f.failCode != "" {
		_, _ = w.Write([]byte(`{"Response":{"Error":{"Code":"` + f.failCode + `","Message":"boom"},"RequestId":"x"}}`))
		return
	}
	resp := map[string]any{"RequestId": "x"}
	switch action {
	case "DescribeZones":
		offset, limit := int(params["Offset"].(float64)), int(params["Limit"].(float64))
		if limit > 100 {
			f.t.Errorf("DescribeZones Limit 超过 100: %d", limit)
		}
		filters := params["Filters"].([]any)
		fl := filters[0].(map[string]any)
		if fl["Name"] != "zone-name" || fl["Values"].([]any)[0] == "" {
			f.t.Errorf("过滤条件错误: %v", fl)
		}
		// 模拟服务端不按名称缩小范围、全部分页返回，验证客户端翻页与忽略大小写
		matched := f.zones
		end := offset + limit
		if end > len(matched) {
			end = len(matched)
		}
		if offset > len(matched) {
			offset = len(matched)
		}
		resp["TotalCount"] = len(matched)
		resp["Zones"] = matched[offset:end]
	case "DescribeDnsRecords":
		if params["ZoneId"] != "zone-1" {
			f.t.Errorf("ZoneId = %v", params["ZoneId"])
		}
		var list []edgeOneRecord
		for _, rec := range f.records {
			list = append(list, rec)
		}
		resp["TotalCount"] = len(list)
		resp["DnsRecords"] = list
	case "CreateDnsRecord":
		f.next++
		f.records = append(f.records, edgeOneRecord{
			RecordID: fmt.Sprintf("record-%d", f.next), Name: params["Name"].(string), Type: params["Type"].(string),
			Content: params["Content"].(string), Location: "Default", TTL: int(params["TTL"].(float64)), Status: "enable",
		})
		resp["RecordId"] = fmt.Sprintf("record-%d", f.next)
	case "ModifyDnsRecords":
		for _, it := range params["DnsRecords"].([]any) {
			m := it.(map[string]any)
			for i := range f.records {
				if f.records[i].RecordID == m["RecordId"] {
					f.records[i].Content = m["Content"].(string)
					if v, ok := m["TTL"]; ok {
						f.records[i].TTL = int(v.(float64))
					}
				}
			}
		}
	case "DeleteDnsRecords":
		ids := params["RecordIds"].([]any)
		var keep []edgeOneRecord
		for _, rec := range f.records {
			drop := false
			for _, id := range ids {
				if id == rec.RecordID {
					drop = true
				}
			}
			if !drop {
				keep = append(keep, rec)
			}
		}
		f.records = keep
	}
	out, _ := json.Marshal(map[string]any{"Response": resp})
	_, _ = w.Write(out)
}

func newEdgeOneTest(t *testing.T) (*edgeOne, *edgeOneFake, func()) {
	f := &edgeOneFake{t: t}
	// 150 个站点，目标站点在第二页
	for i := 0; i < 150; i++ {
		f.zones = append(f.zones, map[string]string{"ZoneId": "zone-x" + strconv.Itoa(i), "ZoneName": fmt.Sprintf("other%d.com", i)})
	}
	f.zones[120] = map[string]string{"ZoneId": "zone-1", "ZoneName": "Example.com"}
	srv := httptest.NewServer(f)
	p, err := New("edgeone", Credentials{ID: "AKID", Secret: "SKEY"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	e := p.(*edgeOne)
	e.endpoint = srv.URL
	return e, f, srv.Close
}

func TestEdgeOneUpsert(t *testing.T) {
	e, f, done := newEdgeOneTest(t)
	defer done()
	runUpsertSequence(t, e, Record{Zone: "example.com", Sub: "home", Type: "A"})
	if len(f.records) != 1 || f.records[0].Name != "home.example.com" || f.records[0].Content != "203.0.113.2" || f.records[0].TTL != 600 {
		t.Fatalf("记录 = %+v", f.records)
	}
	if _, ok := f.last["CreateDnsRecord"]["Location"]; ok {
		t.Fatal("默认线路不应传 Location")
	}
	if f.last["ModifyDnsRecords"]["ZoneId"] != "zone-1" {
		t.Fatalf("Modify 参数 = %v", f.last["ModifyDnsRecords"])
	}
	// TTL 变化也算更新，且被限制在 60 以上
	got, err := e.Upsert(context.Background(), Record{Zone: "example.com", Sub: "home", Type: "A", Value: "203.0.113.2", TTL: 10})
	if err != nil || got != ActionUpdated || f.records[0].TTL != 60 {
		t.Fatalf("got %s %v ttl=%d", got, err, f.records[0].TTL)
	}
	// 根域名与 IPv6 记录互不影响
	if got, err := e.Upsert(context.Background(), Record{Zone: "example.com", Sub: "@", Type: "AAAA", Value: "2001:db8::1"}); err != nil || got != ActionCreated {
		t.Fatalf("got %s %v", got, err)
	}
	if f.records[1].Name != "example.com" {
		t.Fatalf("根域名记录名 = %s", f.records[1].Name)
	}
}

func TestEdgeOneTXTAndDelete(t *testing.T) {
	e, f, done := newEdgeOneTest(t)
	defer done()
	ctx := context.Background()
	if err := e.AddTXT(ctx, "example.com", "_acme-challenge", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := e.AddTXT(ctx, "example.com", "_acme-challenge", "v2"); err != nil {
		t.Fatal(err)
	}
	if len(f.records) != 2 || f.records[0].Name != "_acme-challenge.example.com" || f.records[0].TTL != txtTTL {
		t.Fatalf("记录 = %+v", f.records)
	}
	if err := e.DeleteTXT(ctx, "example.com", "_acme-challenge", "v1"); err != nil {
		t.Fatal(err)
	}
	if len(f.records) != 1 || f.records[0].Content != "v2" {
		t.Fatalf("删除后 = %+v", f.records)
	}
	// 值不存在时不报错也不删除
	if err := e.DeleteTXT(ctx, "example.com", "_acme-challenge", "nope"); err != nil || len(f.records) != 1 {
		t.Fatalf("err=%v records=%+v", err, f.records)
	}

	// 删除动态域名记录：值需与最近同步一致
	rec := Record{Zone: "example.com", Sub: "home", Type: "A", Value: "203.0.113.1"}
	if _, err := e.Upsert(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if n, err := Delete(ctx, e, Record{Zone: "example.com", Sub: "home", Type: "A", Value: "203.0.113.9"}); err == nil || n != 0 {
		t.Fatalf("值不一致应拒绝删除: %d %v", n, err)
	}
	if n, err := Delete(ctx, e, rec); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if len(f.records) != 1 {
		t.Fatalf("删除后 = %+v", f.records)
	}
	if !SupportsDelete("edgeone") || !SupportsTXT("edgeone") {
		t.Fatal("应声明支持 TXT 与删除")
	}
}

func TestEdgeOneErrors(t *testing.T) {
	e, f, done := newEdgeOneTest(t)
	defer done()
	ctx := context.Background()
	// 站点不存在：提示需要 NS 接入
	_, err := e.Upsert(ctx, Record{Zone: "missing.org", Sub: "a", Type: "A", Value: "1.1.1.1"})
	if err == nil || !strings.Contains(err.Error(), "NS 方式接入") {
		t.Fatalf("err = %v", err)
	}
	// 业务错误带错误码
	f.failCode = "AuthFailure.SignatureFailure"
	_, err = e.Upsert(ctx, Record{Zone: "example.com", Sub: "a", Type: "A", Value: "1.1.1.1"})
	if err == nil || !strings.Contains(err.Error(), "AuthFailure.SignatureFailure") || !strings.Contains(err.Error(), "DescribeZones") {
		t.Fatalf("err = %v", err)
	}
	if _, err := New("edgeone", Credentials{ID: "a"}, nil); err == nil {
		t.Fatal("缺少 SecretKey 应报错")
	}
	if _, err := New("edgeone", Credentials{ID: "a", Secret: "b", Extra: map[string]string{"endpoint": "http://teo.example.com"}}, nil); err == nil {
		t.Fatal("非 https 接口地址应报错")
	}
	p, err := New("edgeone", Credentials{ID: "a", Secret: "b", Extra: map[string]string{"endpoint": "https://teo.intl.tencentcloudapi.com"}}, nil)
	if err != nil || p.(*edgeOne).endpoint != "https://teo.intl.tencentcloudapi.com" {
		t.Fatalf("国际站地址 %v", err)
	}
	if p, _ := New("edgeone", Credentials{ID: "a", Secret: "b"}, nil); p.(*edgeOne).endpoint != "https://teo.tencentcloudapi.com" {
		t.Fatal("默认接口地址错误")
	}
}
