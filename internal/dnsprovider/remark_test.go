package dnsprovider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// 阿里云：新增后单独设置备注；只改备注时不调用 UpdateDomainRecord（值未变会报 DomainRecordDuplicate）；
// 未填备注时不改动服务商处的备注。
func TestAliDNSRemark(t *testing.T) {
	var actions []string
	value, remark := "", ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		action := q.Get("Action")
		actions = append(actions, action)
		switch action {
		case "DescribeSubDomainRecords":
			if value == "" {
				_, _ = w.Write([]byte(`{"DomainRecords":{"Record":[]}}`))
				return
			}
			b, _ := json.Marshal(map[string]any{"DomainRecords": map[string]any{"Record": []map[string]any{
				{"RecordId": "r1", "RR": "www", "Type": "A", "Value": value, "TTL": 600, "Line": "default", "Remark": remark},
			}}})
			_, _ = w.Write(b)
		case "AddDomainRecord":
			if q.Get("Remark") != "" {
				t.Error("新增接口不接受备注参数")
			}
			value = q.Get("Value")
			_, _ = w.Write([]byte(`{"RecordId":"r1"}`))
		case "UpdateDomainRecord":
			value = q.Get("Value")
			_, _ = w.Write([]byte(`{"RecordId":"r1"}`))
		case "UpdateDomainRecordRemark":
			if q.Get("RecordId") != "r1" {
				t.Errorf("备注 RecordId 错误: %v", q)
			}
			remark = q.Get("Remark")
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("意外调用 %s", action)
		}
	}))
	defer srv.Close()
	p, _ := New("alidns", Credentials{ID: "ak", Secret: "sk"}, srv.Client())
	p.(*aliDNS).endpoint = srv.URL + "/"
	p.(*aliDNS).now = func() time.Time { return time.Unix(1700000000, 0) }

	steps := []struct {
		value, remark, want, calls string
	}{
		{"1.1.1.1", "家里 NAS", ActionCreated, "DescribeSubDomainRecords,AddDomainRecord,UpdateDomainRecordRemark"},
		{"1.1.1.1", "家里 NAS", ActionUnchanged, "DescribeSubDomainRecords"},
		{"1.1.1.1", "书房 NAS", ActionUpdated, "DescribeSubDomainRecords,UpdateDomainRecordRemark"},
		{"2.2.2.2", "书房 NAS", ActionUpdated, "DescribeSubDomainRecords,UpdateDomainRecord"},
		// 未填备注：服务商处已有的备注保持不变，也不算作变化
		{"2.2.2.2", "", ActionUnchanged, "DescribeSubDomainRecords"},
	}
	for _, s := range steps {
		actions = nil
		got, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "www", Type: "A", Value: s.value, Remark: s.remark})
		if err != nil {
			t.Fatal(err)
		}
		if got != s.want || strings.Join(actions, ",") != s.calls {
			t.Fatalf("%s/%s: got %s %v, want %s %s", s.value, s.remark, got, actions, s.want, s.calls)
		}
	}
	// 阿里云不接受空格，写入时换成下划线；比较时同样换算，不会每轮都判为变化
	if remark != "书房_NAS" {
		t.Fatalf("服务商处备注应为 书房_NAS: %q", remark)
	}
}

// Cloudflare：新建与更新都写 comment；未填备注时 PATCH 不带 comment，保留手动填写的备注。
func TestCloudflareComment(t *testing.T) {
	var cur map[string]any
	var patches []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/zones":
			_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"z1","name":"example.com"}]}`))
		case r.Method == "GET" && r.URL.Path == "/zones/z1/dns_records":
			list := []map[string]any{}
			if cur != nil {
				list = append(list, cur)
			}
			b, _ := json.Marshal(map[string]any{"success": true, "result": list})
			_, _ = w.Write(b)
		case r.Method == "POST":
			cur = readJSON(t, r)
			cur["id"] = "r1"
			_, _ = w.Write([]byte(`{"success":true,"result":{}}`))
		case r.Method == "PATCH":
			body := readJSON(t, r)
			patches = append(patches, body)
			for k, v := range body {
				cur[k] = v
			}
			_, _ = w.Write([]byte(`{"success":true,"result":{}}`))
		default:
			t.Errorf("意外请求 %s %s", r.Method, r.URL)
		}
	}))
	defer srv.Close()
	p, _ := New("cloudflare", Credentials{Secret: "tok"}, srv.Client())
	p.(*cloudflare).endpoint = srv.URL
	up := func(value, remark, want string) {
		t.Helper()
		got, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "home", Type: "A", Value: value, Remark: remark})
		if err != nil || got != want {
			t.Fatalf("%s/%s: %s %v, want %s", value, remark, got, err, want)
		}
	}
	up("1.1.1.1", "Gately", ActionCreated)
	if cur["comment"] != "Gately" {
		t.Fatalf("新建时应写 comment: %v", cur)
	}
	up("1.1.1.1", "Gately", ActionUnchanged)
	up("1.1.1.1", "家里", ActionUpdated)
	if cur["comment"] != "家里" {
		t.Fatalf("只改备注时应更新 comment: %v", cur)
	}
	up("2.2.2.2", "", ActionUpdated)
	if _, ok := patches[len(patches)-1]["comment"]; ok || cur["comment"] != "家里" {
		t.Fatalf("未填备注时不应改动 comment: %v %v", patches[len(patches)-1], cur)
	}
}

// 腾讯云：新建、修改都带 Remark；只改备注时调用 ModifyRecordRemark。
func TestTencentCloudRemark(t *testing.T) {
	var actions []string
	value, remark := "", ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.Header.Get("X-TC-Action")
		actions = append(actions, action)
		b, _ := io.ReadAll(r.Body)
		var params map[string]any
		_ = json.Unmarshal(b, &params)
		switch action {
		case "DescribeRecordList":
			if value == "" {
				_, _ = w.Write([]byte(`{"Response":{"Error":{"Code":"ResourceNotFound.NoDataOfRecord","Message":"no"}}}`))
				return
			}
			out, _ := json.Marshal(map[string]any{"Response": map[string]any{"RecordList": []map[string]any{
				{"RecordId": 12, "Value": value, "Type": "A", "Name": "www", "Line": "默认", "TTL": 600, "Remark": remark},
			}}})
			_, _ = w.Write(out)
		case "CreateRecord", "ModifyRecord":
			value = params["Value"].(string)
			remark, _ = params["Remark"].(string)
			_, _ = w.Write([]byte(`{"Response":{}}`))
		case "ModifyRecordRemark":
			if params["RecordId"].(float64) != 12 || params["Domain"] != "example.com" {
				t.Errorf("备注参数错误: %v", params)
			}
			remark = params["Remark"].(string)
			_, _ = w.Write([]byte(`{"Response":{}}`))
		}
	}))
	defer srv.Close()
	p, _ := New("tencentcloud", Credentials{ID: "sid", Secret: "skey"}, srv.Client())
	p.(*tencentCloud).endpoint = srv.URL
	steps := []struct{ value, remark, want, last string }{
		{"1.1.1.1", "a", ActionCreated, "CreateRecord"},
		{"1.1.1.1", "a", ActionUnchanged, "DescribeRecordList"},
		{"1.1.1.1", "b", ActionUpdated, "ModifyRecordRemark"},
		{"2.2.2.2", "c", ActionUpdated, "ModifyRecord"},
	}
	for _, s := range steps {
		actions = nil
		got, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "www", Type: "A", Value: s.value, Remark: s.remark})
		if err != nil || got != s.want || actions[len(actions)-1] != s.last {
			t.Fatalf("%s/%s: %s %v %v", s.value, s.remark, got, err, actions)
		}
		if remark != s.remark {
			t.Fatalf("服务商处备注 %q，应为 %q", remark, s.remark)
		}
	}
}

// DNSPod：创建后用返回的记录 ID 调用 Record.Remark。
func TestDNSPodRemark(t *testing.T) {
	value, remark, remarkID := "", "", ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.URL.Path {
		case "/Record.List":
			if value == "" {
				_, _ = w.Write([]byte(`{"status":{"code":"10","message":"No records"}}`))
				return
			}
			out, _ := json.Marshal(map[string]any{"status": map[string]string{"code": "1"}, "records": []map[string]string{
				{"id": "9", "name": "nas", "type": "A", "value": value, "line": "默认", "ttl": "600", "remark": remark},
			}})
			_, _ = w.Write(out)
		case "/Record.Create":
			value = r.Form.Get("value")
			_, _ = w.Write([]byte(`{"status":{"code":"1"},"record":{"id":"9","name":"nas"}}`))
		case "/Record.Remark":
			remark, remarkID = r.Form.Get("remark"), r.Form.Get("record_id")
			_, _ = w.Write([]byte(`{"status":{"code":"1"}}`))
		default:
			t.Errorf("意外请求 %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	p, _ := New("dnspod", Credentials{ID: "1", Secret: "x"}, srv.Client())
	p.(*dnspod).endpoint = srv.URL
	rec := Record{Zone: "example.com", Sub: "nas", Type: "A", Value: "1.1.1.1", Remark: "NAS"}
	if got, err := p.Upsert(context.Background(), rec); err != nil || got != ActionCreated {
		t.Fatalf("%s %v", got, err)
	}
	if remark != "NAS" || remarkID != "9" {
		t.Fatalf("创建后应设置备注: %q %q", remark, remarkID)
	}
	if got, err := p.Upsert(context.Background(), rec); err != nil || got != ActionUnchanged {
		t.Fatalf("备注一致时应不变: %s %v", got, err)
	}
}

// 华为云：备注写在记录集的 description 中。
func TestHuaweiCloudDescription(t *testing.T) {
	var cur map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/zones":
			_, _ = w.Write([]byte(`{"zones":[{"id":"z1","name":"example.com."}]}`))
		case r.Method == "GET":
			list := []map[string]any{}
			if cur != nil {
				list = append(list, cur)
			}
			b, _ := json.Marshal(map[string]any{"recordsets": list})
			_, _ = w.Write(b)
		default:
			cur = readJSON(t, r)
			cur["id"] = "rs1"
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	p, _ := New("huaweicloud", Credentials{ID: "ak", Secret: "sk"}, srv.Client())
	p.(*huaweiCloud).endpoint = srv.URL
	for _, s := range []struct{ remark, want string }{{"a", ActionCreated}, {"a", ActionUnchanged}, {"b", ActionUpdated}} {
		got, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "www", Type: "A", Value: "1.1.1.1", Remark: s.remark})
		if err != nil || got != s.want || cur["description"] != s.remark {
			t.Fatalf("%s: %s %v %v", s.remark, got, err, cur)
		}
	}
}

// 支持备注的服务商在描述中声明，界面据此提示备注会同步到服务商。
func TestSupportsRemarkMeta(t *testing.T) {
	for _, n := range []string{"alidns", "cloudflare", "tencentcloud", "dnspod", "dnspodcom", "huaweicloud"} {
		if m, _ := Lookup(n); !m.SupportsRemark {
			t.Errorf("%s 应声明支持备注", n)
		}
	}
	if m, _ := Lookup("godaddy"); m.SupportsRemark {
		t.Error("godaddy 不支持备注")
	}
}

// 阿里云备注只接受数字、中文、英文字母与 _ - , . ，。，最多 50 个字符。
func TestAliRemarkFormat(t *testing.T) {
	cases := map[string]string{
		"Gately 备注测试：阿里云": "Gately_备注测试_阿里云",
		"家里 NAS，书房。":      "家里_NAS，书房。",
		"  a  b  ":        "a_b",
		"v1.2-test_ok,x":  "v1.2-test_ok,x",
		"!!!":             "",
	}
	for in, want := range cases {
		if got := aliRemark(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
	if n := len([]rune(aliRemark(strings.Repeat("长", 80)))); n != aliRemarkMax {
		t.Fatalf("应截断为 %d 个字符，实际 %d", aliRemarkMax, n)
	}
}
