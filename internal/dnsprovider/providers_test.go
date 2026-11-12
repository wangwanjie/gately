package dnsprovider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// runUpsertSequence 依次以 新值 -> 相同值 -> 新值 调用，期望 创建 -> 不变 -> 更新。
func runUpsertSequence(t *testing.T, p Provider, rec Record) {
	t.Helper()
	steps := []struct{ value, want string }{
		{"203.0.113.1", ActionCreated},
		{"203.0.113.1", ActionUnchanged},
		{"203.0.113.2", ActionUpdated},
	}
	for _, s := range steps {
		rec.Value = s.value
		got, err := p.Upsert(context.Background(), rec)
		if err != nil {
			t.Fatalf("value=%s: %v", s.value, err)
		}
		if got != s.want {
			t.Fatalf("value=%s: got %s want %s", s.value, got, s.want)
		}
	}
}

func readJSON(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	b, _ := io.ReadAll(r.Body)
	m := map[string]any{}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("请求体不是 JSON 对象: %s", b)
		}
	}
	return m
}

func TestCloudflare(t *testing.T) {
	value := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(403)
			_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":9109,"message":"Invalid access token"}]}`))
			return
		}
		switch {
		case r.Method == "GET" && r.URL.Path == "/zones":
			_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"z1","name":"example.com"}]}`))
		case r.Method == "GET" && r.URL.Path == "/zones/z1/dns_records":
			if r.URL.Query().Get("name") != "home.example.com" {
				t.Errorf("name 参数错误: %s", r.URL.RawQuery)
			}
			if value == "" {
				_, _ = w.Write([]byte(`{"success":true,"result":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"r1","type":"A","name":"home.example.com","content":"` + value + `","ttl":1,"proxied":true}]}`))
		case r.Method == "POST" && r.URL.Path == "/zones/z1/dns_records":
			value = readJSON(t, r)["content"].(string)
			_, _ = w.Write([]byte(`{"success":true,"result":{}}`))
		case r.Method == "PATCH" && r.URL.Path == "/zones/z1/dns_records/r1":
			body := readJSON(t, r)
			// PATCH 不带 proxied，服务商处的代理状态保持不变
			if _, ok := body["proxied"]; ok {
				t.Error("更新时不应改动代理状态")
			}
			value = body["content"].(string)
			_, _ = w.Write([]byte(`{"success":true,"result":{}}`))
		default:
			t.Errorf("意外请求 %s %s", r.Method, r.URL)
		}
	}))
	defer srv.Close()
	p, err := New("cloudflare", Credentials{Secret: "tok"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	p.(*cloudflare).endpoint = srv.URL
	runUpsertSequence(t, p, Record{Zone: "example.com", Sub: "home", Type: "A"})

	// ID 不是邮箱（例如 Lucky 配置中残留的其他服务商 AccessKey）时仍按 API Token 认证
	value = ""
	p2, _ := New("cloudflare", Credentials{ID: "LTAI5tExample", Secret: "tok"}, srv.Client())
	p2.(*cloudflare).endpoint = srv.URL
	if _, err := p2.Upsert(context.Background(), Record{Zone: "example.com", Sub: "home", Type: "A", Value: "1.1.1.1"}); err != nil {
		t.Fatalf("非邮箱 ID 应使用 API Token: %v", err)
	}

	bad, _ := New("cloudflare", Credentials{Secret: "x"}, srv.Client())
	bad.(*cloudflare).endpoint = srv.URL
	if _, err := bad.Upsert(context.Background(), Record{Zone: "example.com", Sub: "home", Type: "A", Value: "1.1.1.1"}); err == nil || !strings.Contains(err.Error(), "Invalid access token") {
		t.Fatalf("应透出 Cloudflare 错误信息: %v", err)
	}
}

func TestTencentCloud(t *testing.T) {
	value := ""
	now := time.Unix(1700000000, 0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.Header.Get("X-TC-Action")
		body, _ := io.ReadAll(r.Body)
		u, _ := url.Parse("http://" + r.Host)
		want := tc3Authorization("sid", "skey", "dnspod", u.Host, action, body, now)
		if r.Header.Get("Authorization") != want {
			t.Errorf("签名不一致:\n%s\n%s", r.Header.Get("Authorization"), want)
		}
		var params map[string]any
		_ = json.Unmarshal(body, &params)
		switch action {
		case "DescribeRecordList":
			if value == "" {
				_, _ = w.Write([]byte(`{"Response":{"Error":{"Code":"ResourceNotFound.NoDataOfRecord","Message":"no"}}}`))
				return
			}
			_, _ = w.Write([]byte(`{"Response":{"RecordList":[{"RecordId":12,"Value":"` + value + `","Type":"A","Name":"@","Line":"默认","TTL":600}]}}`))
		case "CreateRecord":
			if params["SubDomain"] != "@" || params["RecordLine"] != "默认" {
				t.Errorf("创建参数错误: %v", params)
			}
			value = params["Value"].(string)
			_, _ = w.Write([]byte(`{"Response":{"RecordId":12}}`))
		case "ModifyRecord":
			if params["RecordId"].(float64) != 12 {
				t.Errorf("RecordId 错误: %v", params)
			}
			value = params["Value"].(string)
			_, _ = w.Write([]byte(`{"Response":{}}`))
		}
	}))
	defer srv.Close()
	p, _ := New("tencentcloud", Credentials{ID: "sid", Secret: "skey"}, srv.Client())
	tc := p.(*tencentCloud)
	tc.endpoint = srv.URL
	tc.now = func() time.Time { return now }
	runUpsertSequence(t, p, Record{Zone: "example.com", Sub: "@", Type: "A"})
}

func TestDNSPodToken(t *testing.T) {
	value := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("login_token") != "123,abc" {
			t.Errorf("login_token 错误: %s", r.Form.Get("login_token"))
		}
		switch r.URL.Path {
		case "/Record.List":
			if value == "" {
				_, _ = w.Write([]byte(`{"status":{"code":"10","message":"No records"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"status":{"code":"1"},"records":[{"id":"9","name":"nas","type":"AAAA","value":"` + value + `","line":"默认","ttl":"600"}]}`))
		case "/Record.Create", "/Record.Modify":
			if r.URL.Path == "/Record.Modify" && r.Form.Get("record_id") != "9" {
				t.Errorf("record_id 错误")
			}
			value = r.Form.Get("value")
			_, _ = w.Write([]byte(`{"status":{"code":"1"}}`))
		}
	}))
	defer srv.Close()
	p, _ := New("dnspod", Credentials{ID: "123", Secret: "abc"}, srv.Client())
	p.(*dnspod).endpoint = srv.URL
	steps := []struct{ v, want string }{{"2001:db8::1", ActionCreated}, {"2001:db8::1", ActionUnchanged}, {"2001:db8::2", ActionUpdated}}
	for _, s := range steps {
		got, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "nas", Type: "AAAA", Value: s.v})
		if err != nil || got != s.want {
			t.Fatalf("%s: %s %v", s.v, got, err)
		}
	}
}

func TestHuaweiCloud(t *testing.T) {
	value := ""
	now := time.Unix(1700000000, 0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "SDK-HMAC-SHA256 Access=ak, SignedHeaders=host;x-sdk-date, Signature=") || r.Header.Get("X-Sdk-Date") != "20231114T221320Z" {
			t.Errorf("签名头错误: %s %s", auth, r.Header.Get("X-Sdk-Date"))
		}
		switch {
		case r.URL.Path == "/v2/zones":
			_, _ = w.Write([]byte(`{"zones":[{"id":"z1","name":"example.com."}]}`))
		case r.Method == "GET" && r.URL.Path == "/v2/zones/z1/recordsets":
			if r.URL.Query().Get("name") != "www.example.com." {
				t.Errorf("name 错误: %s", r.URL.RawQuery)
			}
			if value == "" {
				_, _ = w.Write([]byte(`{"recordsets":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"recordsets":[{"id":"rs1","name":"www.example.com.","type":"A","ttl":300,"records":["` + value + `"]}]}`))
		case r.Method == "POST" && r.URL.Path == "/v2/zones/z1/recordsets", r.Method == "PUT" && r.URL.Path == "/v2/zones/z1/recordsets/rs1":
			body := readJSON(t, r)
			value = body["records"].([]any)[0].(string)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("意外请求 %s %s", r.Method, r.URL)
		}
	}))
	defer srv.Close()
	p, _ := New("huaweicloud", Credentials{ID: "ak", Secret: "sk"}, srv.Client())
	h := p.(*huaweiCloud)
	h.endpoint = srv.URL
	h.now = func() time.Time { return now }
	runUpsertSequence(t, p, Record{Zone: "example.com", Sub: "www", Type: "A"})
}

func TestGoDaddy(t *testing.T) {
	value := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "sso-key k:s" || r.URL.Path != "/v1/domains/example.com/records/A/@" {
			t.Errorf("请求错误: %s %s", r.Header.Get("Authorization"), r.URL.Path)
		}
		if r.Method == "GET" {
			if value == "" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(`[{"data":"` + value + `","ttl":600}]`))
			return
		}
		var body []map[string]any
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		value = body[0]["data"].(string)
	}))
	defer srv.Close()
	p, _ := New("godaddy", Credentials{ID: "k", Secret: "s"}, srv.Client())
	p.(*godaddy).endpoint = srv.URL
	runUpsertSequence(t, p, Record{Zone: "example.com", Sub: "@", Type: "A"})
}

func TestPorkbun(t *testing.T) {
	value := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := readJSON(t, r)
		if body["apikey"] != "k" || body["secretapikey"] != "s" {
			t.Errorf("凭据错误: %v", body)
		}
		switch r.URL.Path {
		case "/dns/retrieveByNameType/example.com/A/vpn":
			if value == "" {
				_, _ = w.Write([]byte(`{"status":"SUCCESS","records":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"status":"SUCCESS","records":[{"id":"1","content":"` + value + `","ttl":"600"}]}`))
		case "/dns/create/example.com":
			if body["name"] != "vpn" {
				t.Errorf("name 错误: %v", body)
			}
			value = body["content"].(string)
			_, _ = w.Write([]byte(`{"status":"SUCCESS"}`))
		case "/dns/editByNameType/example.com/A/vpn":
			value = body["content"].(string)
			_, _ = w.Write([]byte(`{"status":"SUCCESS"}`))
		default:
			t.Errorf("意外路径 %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	p, _ := New("porkbun", Credentials{ID: "k", Secret: "s"}, srv.Client())
	p.(*porkbun).endpoint = srv.URL
	runUpsertSequence(t, p, Record{Zone: "example.com", Sub: "vpn", Type: "A"})
}

func TestDynv6(t *testing.T) {
	value := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("鉴权头错误")
		}
		switch {
		case r.URL.Path == "/zones/by-name/my.dynv6.net":
			_, _ = w.Write([]byte(`{"id":7}`))
		case r.Method == "GET" && r.URL.Path == "/zones/7/records":
			if value == "" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(`[{"id":3,"name":"","type":"A","data":"` + value + `"}]`))
		case r.Method == "POST", r.Method == "PATCH" && r.URL.Path == "/zones/7/records/3":
			value = readJSON(t, r)["data"].(string)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("意外请求 %s %s", r.Method, r.URL)
		}
	}))
	defer srv.Close()
	p, _ := New("dynv6", Credentials{Secret: "tok"}, srv.Client())
	p.(*dynv6).endpoint = srv.URL
	runUpsertSequence(t, p, Record{Zone: "my.dynv6.net", Sub: "@", Type: "A"})
}

func TestDuckDNSAndCallback(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		_, _ = w.Write([]byte("OK"))
	}))
	defer srv.Close()
	p, _ := New("duckdns", Credentials{Secret: "tok"}, srv.Client())
	p.(*duckdns).endpoint = srv.URL
	if _, err := p.Upsert(context.Background(), Record{Zone: "duckdns.org", Sub: "myhome", Type: "AAAA", Value: "2001:db8::1"}); err != nil {
		t.Fatal(err)
	}
	if got.Get("domains") != "myhome" || got.Get("ipv6") != "2001:db8::1" || got.Get("token") != "tok" {
		t.Fatalf("DuckDNS 参数错误: %v", got)
	}

	cb, err := New("callback", Credentials{Extra: map[string]string{
		"url": srv.URL + "/?d={domain}&ip={ip}&t={recordType}", "successContent": "OK",
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cb.Upsert(context.Background(), Record{Zone: "example.com", Sub: "a", Type: "A", Value: "1.2.3.4"}); err != nil {
		t.Fatal(err)
	}
	if got.Get("d") != "a.example.com" || got.Get("ip") != "1.2.3.4" || got.Get("t") != "A" {
		t.Fatalf("回调参数错误: %v", got)
	}
}

func TestRegistry(t *testing.T) {
	names := map[string]bool{}
	for _, m := range List() {
		names[m.Name] = true
	}
	for _, n := range []string{"alidns", "cloudflare", "tencentcloud", "dnspod", "dnspodcom", "huaweicloud", "godaddy", "porkbun", "dynv6", "duckdns", "callback"} {
		if !names[n] {
			t.Errorf("缺少服务商 %s", n)
		}
	}
	if _, err := New("nope", Credentials{}, nil); err == nil {
		t.Fatal("未知服务商应报错")
	}
	if _, err := New("alidns", Credentials{ID: "x"}, nil); err == nil {
		t.Fatal("缺少 Secret 应报错")
	}
}

// dynv6 的 zone 是用户自己的完整域名（如 my.dynv6.net）；主域名误填为 dynv6.net 时应自动回退到更长的 zone。
func TestDynv6ZoneFallback(t *testing.T) {
	value := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/zones/by-name/my.dynv6.net":
			_, _ = w.Write([]byte(`{"id":7}`))
		case r.URL.Path == "/zones/by-name/dynv6.net", r.URL.Path == "/zones/by-name/a.my.dynv6.net":
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
		case r.Method == "GET" && r.URL.Path == "/zones/7/records":
			if value == "" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(`[{"id":3,"name":"a","type":"AAAA","data":"` + value + `"}]`))
		case r.Method == "POST", r.Method == "PATCH" && r.URL.Path == "/zones/7/records/3":
			b := readJSON(t, r)
			if r.Method == "POST" && b["name"] != "a" {
				t.Errorf("name 错误: %v", b)
			}
			value = b["data"].(string)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("意外请求 %s %s", r.Method, r.URL)
		}
	}))
	defer srv.Close()
	p, _ := New("dynv6", Credentials{Secret: "tok"}, srv.Client())
	p.(*dynv6).endpoint = srv.URL
	runUpsertSequence(t, p, Record{Zone: "dynv6.net", Sub: "a.my", Type: "AAAA"})
	// 子域名仅一级时：zone=dynv6.net sub=my 同样应落到 my.dynv6.net 的根记录
}
