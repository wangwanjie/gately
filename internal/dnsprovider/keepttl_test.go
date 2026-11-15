package dnsprovider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDNSPodKeepsTTL(t *testing.T) {
	var modify map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.URL.Path {
		case "/Record.List":
			_, _ = w.Write([]byte(`{"status":{"code":"1"},"records":[{"id":"9","name":"nas","type":"A","value":"1.1.1.1","line":"默认","ttl":"120"}]}`))
		case "/Record.Modify":
			modify = map[string]string{}
			for k := range r.PostForm {
				modify[k] = r.PostForm.Get(k)
			}
			_, _ = w.Write([]byte(`{"status":{"code":"1"}}`))
		default:
			t.Errorf("意外请求 %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	p, _ := New("dnspod", Credentials{ID: "1", Secret: "t"}, srv.Client())
	p.(*dnspod).endpoint = srv.URL
	if got, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "nas", Type: "A", Value: "2.2.2.2"}); err != nil || got != ActionUpdated {
		t.Fatalf("%v %v", got, err)
	}
	if modify["ttl"] != "120" {
		t.Fatalf("应沿用 TTL 120: %v", modify)
	}
}

// 腾讯云 DNSPod 3.0：未指定 TTL 时 ModifyRecord 沿用现有 TTL。

func TestTencentCloudKeepsTTL(t *testing.T) {
	var modify string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch r.Header.Get("X-TC-Action") {
		case "DescribeRecordList":
			_, _ = w.Write([]byte(`{"Response":{"RecordList":[{"RecordId":12,"Value":"1.1.1.1","Type":"A","Name":"@","Line":"默认","TTL":180}]}}`))
		case "ModifyRecord":
			modify = string(body)
			_, _ = w.Write([]byte(`{"Response":{"RequestId":"x"}}`))
		default:
			t.Errorf("意外动作 %s", r.Header.Get("X-TC-Action"))
		}
	}))
	defer srv.Close()
	p, _ := New("tencentcloud", Credentials{ID: "id", Secret: "key"}, srv.Client())
	tc := p.(*tencentCloud)
	tc.endpoint = srv.URL
	tc.now = func() time.Time { return time.Unix(1700000000, 0) }
	if got, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "@", Type: "A", Value: "2.2.2.2"}); err != nil || got != ActionUpdated {
		t.Fatalf("%v %v", got, err)
	}
	if !strings.Contains(modify, `"TTL":180`) {
		t.Fatalf("应沿用 TTL 180: %s", modify)
	}
}

// 华为云：未指定 TTL 时 PUT 沿用记录集现有 TTL。

func TestHuaweiCloudKeepsTTL(t *testing.T) {
	var put map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/zones":
			_, _ = w.Write([]byte(`{"zones":[{"id":"z1","name":"example.com."}]}`))
		case r.Method == "GET":
			_, _ = w.Write([]byte(`{"recordsets":[{"id":"rs1","name":"www.example.com.","type":"A","ttl":120,"records":["1.1.1.1"]}]}`))
		case r.Method == "PUT":
			put = readJSON(t, r)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	p, _ := New("huaweicloud", Credentials{ID: "ak", Secret: "sk"}, srv.Client())
	h := p.(*huaweiCloud)
	h.endpoint = srv.URL
	h.now = func() time.Time { return time.Unix(1700000000, 0) }
	if got, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "www", Type: "A", Value: "2.2.2.2"}); err != nil || got != ActionUpdated {
		t.Fatalf("%v %v", got, err)
	}
	if put["ttl"] != float64(120) {
		t.Fatalf("应沿用 TTL 120: %v", put)
	}
}

// Gandi / NS1：未指定 TTL 时更新沿用现有 TTL，不被重置为 300 / 3600。

func TestGandiNS1KeepTTL(t *testing.T) {
	var gandiPut, ns1Post string
	gsrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			_, _ = w.Write([]byte(`{"rrset_values":["1.1.1.1"],"rrset_ttl":900}`))
			return
		}
		b, _ := io.ReadAll(r.Body)
		gandiPut = string(b)
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer gsrv.Close()
	g, _ := New("gandi", Credentials{Secret: "k"}, gsrv.Client())
	g.(*gandi).endpoint = gsrv.URL
	if got, err := g.Upsert(context.Background(), Record{Zone: "example.com", Sub: "www", Type: "A", Value: "2.2.2.2"}); err != nil || got != ActionUpdated {
		t.Fatalf("%v %v", got, err)
	}
	if !strings.Contains(gandiPut, `"rrset_ttl":900`) {
		t.Fatalf("Gandi 应沿用 TTL 900: %s", gandiPut)
	}

	nsrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			_, _ = w.Write([]byte(`{"ttl":60,"answers":[{"answer":["1.1.1.1"]}]}`))
			return
		}
		b, _ := io.ReadAll(r.Body)
		ns1Post = string(b)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer nsrv.Close()
	n, _ := New("ns1", Credentials{Secret: "k"}, nsrv.Client())
	n.(*ns1).endpoint = nsrv.URL
	if got, err := n.Upsert(context.Background(), Record{Zone: "example.com", Sub: "www", Type: "A", Value: "2.2.2.2"}); err != nil || got != ActionUpdated {
		t.Fatalf("%v %v", got, err)
	}
	if !strings.Contains(ns1Post, `"ttl":60`) {
		t.Fatalf("NS1 应沿用 TTL 60: %s", ns1Post)
	}
}

// 回调支持 {timestamp}（当前 Unix 秒），也兼容 #{timestamp} 写法。
