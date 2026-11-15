package dnsprovider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// 官方文档中的签名示例（BOS UploadPart），签名头为空表示只签名推荐头。
func TestBceAuthorizationDocExample(t *testing.T) {
	ts := time.Date(2015, 4, 27, 8, 23, 49, 0, time.UTC)
	got := bceAuthorization(strings.Repeat("a", 32), strings.Repeat("b", 32), "PUT", "/v1/test/myfolder/readme.txt",
		url.Values{"partNumber": {"9"}, "uploadId": {"a44cc9bab11cbd156984767aad637851"}},
		map[string]string{
			"Host": "bj.bcebos.com", "Content-Length": "8", "Content-MD5": "NFzcPqhviddjRNnSOGo4rw==",
			"Content-Type": "text/plain", "x-bce-date": "2015-04-27T08:23:49Z",
		}, "", ts, 1800)
	want := "bce-auth-v1/" + strings.Repeat("a", 32) + "/2015-04-27T08:23:49Z/1800//d74a04362e6a848f5b39b15421cb449427f419c95a480fd6b8cf9fc783e2999e"
	if got != want {
		t.Fatalf("签名不一致:\n got %s\nwant %s", got, want)
	}
}

// baiduCloudFake 模拟百度云解析接口。
type baiduCloudFake struct {
	t       *testing.T
	records []baiduCloudRecord
	next    int64
	calls   []string
	// 最近一次各路径的请求体
	last map[string]map[string]any
}

func (f *baiduCloudFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.calls = append(f.calls, r.URL.Path)
	// 重新计算认证字符串并比对
	date := r.Header.Get("x-bce-date")
	ts, err := time.Parse("2006-01-02T15:04:05Z", date)
	if err != nil {
		f.t.Errorf("x-bce-date 无效: %q", date)
	}
	want := bceAuthorization("AK", "SK", r.Method, r.URL.Path, nil, map[string]string{"host": r.Host, "x-bce-date": date}, "host;x-bce-date", ts, 1800)
	if r.Header.Get("Authorization") != want {
		f.t.Errorf("鉴权头不一致: %s", r.Header.Get("Authorization"))
	}
	if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json; charset=utf-8" {
		f.t.Errorf("方法或类型错误: %s %s", r.Method, r.Header.Get("Content-Type"))
	}
	params := map[string]any{}
	_ = json.Unmarshal(body, &params)
	if f.last == nil {
		f.last = map[string]map[string]any{}
	}
	f.last[r.URL.Path] = params
	switch r.URL.Path {
	case "/v1/domain/resolve/list":
		if params["domain"] != "example.com" {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"code":"DomainNotExists","message":"domain not found","requestId":"x"}`))
			return
		}
		no, size := int(params["pageNo"].(float64)), int(params["pageSize"].(float64))
		start := (no - 1) * size
		end := start + size
		if start > len(f.records) {
			start = len(f.records)
		}
		if end > len(f.records) {
			end = len(f.records)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"pageNo": no, "pageSize": size, "totalCount": end - start, "result": f.records[start:end]})
	case "/v1/domain/resolve/add":
		f.next++
		f.records = append(f.records, baiduCloudRecord{
			RecordID: f.next, Domain: params["domain"].(string), View: "DEFAULT", RdType: params["rdType"].(string),
			TTL: int(params["ttl"].(float64)), RData: params["rdata"].(string), ZoneName: params["zoneName"].(string), Status: "RUNNING",
		})
	case "/v1/domain/resolve/edit":
		for i := range f.records {
			if float64(f.records[i].RecordID) == params["recordId"].(float64) {
				f.records[i].RData = params["rdata"].(string)
				f.records[i].TTL = int(params["ttl"].(float64))
			}
		}
	case "/v1/domain/resolve/delete":
		if params["zoneName"] != "example.com" {
			f.t.Errorf("delete 缺少 zoneName: %v", params)
		}
		var keep []baiduCloudRecord
		for _, rec := range f.records {
			if float64(rec.RecordID) != params["recordId"].(float64) {
				keep = append(keep, rec)
			}
		}
		f.records = keep
	}
}

func newBaiduCloudTest(t *testing.T) (*baiduCloud, *baiduCloudFake, func()) {
	f := &baiduCloudFake{t: t}
	srv := httptest.NewServer(f)
	p, err := New("baiducloud", Credentials{ID: "AK", Secret: "SK"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	b := p.(*baiduCloud)
	b.endpoint = srv.URL
	return b, f, srv.Close
}

func TestBaiduCloudUpsert(t *testing.T) {
	b, f, done := newBaiduCloudTest(t)
	defer done()
	runUpsertSequence(t, b, Record{Zone: "example.com", Sub: "home", Type: "A"})
	if len(f.records) != 1 || f.records[0].RData != "203.0.113.2" || f.records[0].TTL != 600 || f.records[0].Domain != "home" {
		t.Fatalf("记录 = %+v", f.records)
	}
	edit := f.last["/v1/domain/resolve/edit"]
	if edit["rdType"] != "A" || edit["zoneName"] != "example.com" || edit["view"] != "DEFAULT" || edit["domain"] != "home" {
		t.Fatalf("edit 请求体 = %v", edit)
	}
	// 根域名主机记录为 @，TTL 被限制到 60 以上
	if got, err := b.Upsert(t.Context(), Record{Zone: "example.com", Sub: "", Type: "AAAA", Value: "2001:db8::1", TTL: 5}); err != nil || got != ActionCreated {
		t.Fatalf("got %s %v", got, err)
	}
	if f.records[1].Domain != "@" || f.records[1].TTL != 60 || f.records[1].RdType != "AAAA" {
		t.Fatalf("记录 = %+v", f.records[1])
	}
	// 非默认线路的同名记录不被改动
	f.records = append(f.records, baiduCloudRecord{RecordID: 900, Domain: "isp", View: "CNC", RdType: "A", TTL: 600, RData: "1.1.1.1"})
	if got, _ := b.Upsert(t.Context(), Record{Zone: "example.com", Sub: "isp", Type: "A", Value: "2.2.2.2"}); got != ActionCreated {
		t.Fatalf("got %s", got)
	}
	if f.records[2].RData != "1.1.1.1" {
		t.Fatalf("联通线路记录被改动: %+v", f.records[2])
	}
}

func TestBaiduCloudPaging(t *testing.T) {
	b, f, done := newBaiduCloudTest(t)
	defer done()
	for i := 1; i <= 230; i++ {
		f.records = append(f.records, baiduCloudRecord{RecordID: int64(i), Domain: fmt.Sprintf("h%d", i), View: "DEFAULT", RdType: "A", TTL: 600, RData: "1.1.1.1"})
	}
	f.next = 1000
	// 目标记录在第三页，翻页后应识别为同值未变化
	if got, err := b.Upsert(t.Context(), Record{Zone: "example.com", Sub: "h225", Type: "A", Value: "1.1.1.1"}); err != nil || got != ActionUnchanged {
		t.Fatalf("got %s %v", got, err)
	}
}

func TestBaiduCloudTXTAndDelete(t *testing.T) {
	b, f, done := newBaiduCloudTest(t)
	defer done()
	ctx := t.Context()
	if err := b.AddTXT(ctx, "example.com", "_acme-challenge", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := b.AddTXT(ctx, "example.com", "_acme-challenge", "v2"); err != nil {
		t.Fatal(err)
	}
	if len(f.records) != 2 || f.records[0].RdType != "TXT" || f.records[0].TTL != txtTTL || f.records[0].Domain != "_acme-challenge" {
		t.Fatalf("记录 = %+v", f.records)
	}
	if err := b.DeleteTXT(ctx, "example.com", "_acme-challenge", "v1"); err != nil || len(f.records) != 1 || f.records[0].RData != "v2" {
		t.Fatalf("err=%v records=%+v", err, f.records)
	}
	if err := b.DeleteTXT(ctx, "example.com", "_acme-challenge", "none"); err != nil || len(f.records) != 1 {
		t.Fatalf("err=%v", err)
	}

	rec := Record{Zone: "example.com", Sub: "home", Type: "A", Value: "203.0.113.1"}
	_, _ = b.Upsert(ctx, rec)
	if n, err := Delete(ctx, b, Record{Zone: "example.com", Sub: "home", Type: "A", Value: "9.9.9.9"}); err == nil || n != 0 {
		t.Fatalf("值不一致应拒绝: %d %v", n, err)
	}
	if n, err := Delete(ctx, b, rec); err != nil || n != 1 || len(f.records) != 1 {
		t.Fatalf("n=%d err=%v records=%+v", n, err, f.records)
	}
	if !SupportsDelete("baiducloud") || !SupportsTXT("baiducloud") {
		t.Fatal("应声明支持 TXT 与删除")
	}
}

func TestBaiduCloudErrors(t *testing.T) {
	b, _, done := newBaiduCloudTest(t)
	defer done()
	_, err := b.Upsert(t.Context(), Record{Zone: "missing.org", Sub: "a", Type: "A", Value: "1.1.1.1"})
	if err == nil || !strings.Contains(err.Error(), "DomainNotExists") || !strings.Contains(err.Error(), "domain not found") {
		t.Fatalf("err = %v", err)
	}
	if _, err := New("baiducloud", Credentials{ID: "a"}, nil); err == nil {
		t.Fatal("缺少 Secret 应报错")
	}
}
