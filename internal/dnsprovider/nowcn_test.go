package dnsprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// nowcnMock 模拟“域名独立解析 API”：校验签名，记录按 id 保存。
type nowcnMock struct {
	t      *testing.T
	secret string
	recs   []map[string]any
	nextID int
	// 最近一次写入类接口的参数
	last map[string]string
	// 最近一次列表请求的参数
	lastList map[string]string
}

func newNowcnMock(t *testing.T) (*nowcnMock, *httptest.Server) {
	m := &nowcnMock{t: t, secret: "sk", nextID: 100}
	return m, httptest.NewServer(http.HandlerFunc(m.serve))
}

func (m *nowcnMock) serve(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	params := map[string]string{}
	for k := range q {
		params[k] = q.Get(k)
	}
	sig := params["Signature"]
	delete(params, "Signature")
	if r.Method != "GET" {
		m.t.Errorf("接口应使用 GET: %s", r.Method)
	}
	if params["AccessInstanceID"] != "inst" || params["SignatureMethod"] != "HMAC-SHA1" || len(params["SignatureNonce"]) < 8 || params["Timestamp"] == "" {
		m.t.Errorf("公共参数错误: %v", params)
	}
	// 鉴权失败时真实服务返回 200 与空响应体
	if sig == "" || sig != nowcnSign(m.secret, params) {
		w.WriteHeader(200)
		return
	}
	reply := func(v any) {
		b, _ := json.Marshal(v)
		_, _ = w.Write(b)
	}
	if params["Domain"] != "example.com" {
		reply(map[string]any{"RequestId": "r", "error": "域名不存在"})
		return
	}
	switch r.URL.Path {
	case "/api/dns/DescribeRecordIndex":
		m.lastList = params
		var data []map[string]any
		for _, rec := range m.recs {
			if rec["Host"] == params["Host"] && rec["Type"] == params["Type"] {
				data = append(data, rec)
			}
		}
		// 服务端过滤不精确时多返回一条无关记录，覆盖客户端的精确过滤
		data = append(data, map[string]any{"id": 1, "Domain": "example.com", "Host": params["Host"] + "x", "Type": params["Type"], "Value": "9.9.9.9", "State": 0})
		reply(map[string]any{"RequestId": "r", "Data": data})
	case "/api/dns/AddDomainRecord":
		m.last = params
		m.nextID++
		// 记录列表里 id 是数字，新增接口返回的 Id 是字符串
		m.recs = append(m.recs, map[string]any{"id": m.nextID, "Domain": "example.com", "Host": params["Host"], "Type": params["Type"], "Value": params["Value"], "Ttl": params["Ttl"], "State": 0})
		reply(map[string]any{"RequestId": "r", "Id": strconv.Itoa(m.nextID)})
	case "/api/dns/UpdateDomainRecord":
		m.last = params
		for _, rec := range m.recs {
			if strconv.Itoa(rec["id"].(int)) == params["Id"] {
				rec["Value"] = params["Value"]
				rec["Ttl"] = params["Ttl"]
				reply(map[string]any{"RequestId": "r", "Id": params["Id"]})
				return
			}
		}
		reply(map[string]any{"RequestId": "r", "error": "记录不存在"})
	case "/api/dns/DeleteDomainRecord":
		m.last = params
		for i, rec := range m.recs {
			if strconv.Itoa(rec["id"].(int)) == params["Id"] {
				m.recs = append(m.recs[:i], m.recs[i+1:]...)
				reply(map[string]any{"RequestId": "r"})
				return
			}
		}
		reply(map[string]any{"RequestId": "r", "error": "记录不存在"})
	default:
		m.t.Errorf("未预期的路径 %s", r.URL.Path)
		w.WriteHeader(404)
	}
}

// testNowcnFamily 对共用同一套接口的三家服务商跑同一组用例。
// setEndpoint 把已创建服务商的接口地址改为测试服务。
func testNowcnFamily(t *testing.T, name string, setEndpoint func(p Provider, url string)) {
	m, srv := newNowcnMock(t)
	defer srv.Close()
	ctx := context.Background()
	newP := func(secret string) Provider {
		p, err := New(name, Credentials{ID: "inst", Secret: secret}, srv.Client())
		if err != nil {
			t.Fatal(err)
		}
		setEndpoint(p, srv.URL)
		return p
	}
	p := newP("sk")

	// 创建 / 未变化 / 更新
	runUpsertSequence(t, p, Record{Zone: "example.com", Sub: "home", Type: "A"})
	if len(m.recs) != 1 {
		t.Fatalf("应只有 1 条记录: %v", m.recs)
	}
	if m.last["Domain"] != "example.com" || m.last["Host"] != "home" || m.last["Type"] != "A" || m.last["Value"] != "203.0.113.2" || m.last["Ttl"] != "600" || m.last["Id"] == "" {
		t.Fatalf("更新参数错误: %v", m.last)
	}
	if m.lastList["Host"] != "home" || m.lastList["Type"] != "A" {
		t.Fatalf("列表应按主机与类型查询: %v", m.lastList)
	}

	// 根域名 AAAA，自定义 TTL；TTL 变化触发更新
	rec := Record{Zone: "example.com", Sub: "@", Type: "AAAA", Value: "2001:db8::1", TTL: 300}
	if got, err := p.Upsert(ctx, rec); err != nil || got != ActionCreated {
		t.Fatalf("got %s %v", got, err)
	}
	if m.last["Host"] != "@" || m.last["Ttl"] != "300" {
		t.Fatalf("创建参数错误: %v", m.last)
	}
	if got, _ := p.Upsert(ctx, rec); got != ActionUnchanged {
		t.Fatalf("got %s", got)
	}
	rec.TTL = 600
	if got, err := p.Upsert(ctx, rec); err != nil || got != ActionUpdated {
		t.Fatalf("TTL 变化应更新: %s %v", got, err)
	}

	// 业务错误
	if _, err := p.Upsert(ctx, Record{Zone: "other.com", Sub: "a", Type: "A", Value: "1.1.1.1"}); err == nil || !strings.Contains(err.Error(), "域名不存在") {
		t.Fatalf("业务错误应透出: %v", err)
	}
	// 签名错误：服务端返回 200 空响应体，应按错误处理
	if _, err := newP("wrong").Upsert(ctx, Record{Zone: "example.com", Sub: "a", Type: "A", Value: "1.1.1.1"}); err == nil || !strings.Contains(err.Error(), "响应为空") {
		t.Fatalf("空响应应报错: %v", err)
	}
	if _, err := New(name, Credentials{Secret: "sk"}, nil); err == nil {
		t.Fatal("缺少 AccessInstanceID 应报错")
	}

	// TXT：新增两个值，删除其一后保留另一个
	m.recs = nil
	tp := p.(TXTProvider)
	if err := tp.AddTXT(ctx, "example.com", "_acme-challenge", "tok1"); err != nil {
		t.Fatal(err)
	}
	if m.last["Type"] != "TXT" || m.last["Host"] != "_acme-challenge" || m.last["Value"] != "tok1" {
		t.Fatalf("TXT 参数错误: %v", m.last)
	}
	if err := tp.AddTXT(ctx, "example.com", "_acme-challenge", "tok2"); err != nil {
		t.Fatal(err)
	}
	if err := tp.DeleteTXT(ctx, "example.com", "_acme-challenge", "tok1"); err != nil {
		t.Fatal(err)
	}
	if len(m.recs) != 1 || m.recs[0]["Value"] != "tok2" {
		t.Fatalf("应只删除 tok1: %v", m.recs)
	}
	if err := tp.DeleteTXT(ctx, "example.com", "_acme-challenge", "nope"); err != nil {
		t.Fatal(err)
	}

	// 删除动态域名解析
	m.recs = nil
	dr := Record{Zone: "example.com", Sub: "home", Type: "A", Value: "203.0.113.1"}
	if _, err := p.Upsert(ctx, dr); err != nil {
		t.Fatal(err)
	}
	n, err := Delete(ctx, p, dr)
	if err != nil || n != 1 || len(m.recs) != 0 {
		t.Fatalf("n=%d err=%v recs=%v", n, err, m.recs)
	}
	if m.last["Domain"] != "example.com" || m.last["Id"] == "" {
		t.Fatalf("删除参数错误: %v", m.last)
	}
	if !SupportsDelete(name) || !SupportsTXT(name) {
		t.Fatal("应声明支持删除与 ACME")
	}
}

func TestNowcn(t *testing.T) {
	testNowcnFamily(t, "nowcn", func(p Provider, url string) { p.(*nowcn).endpoint = url })
}

// TestNowcnSignature 校验签名算法：参数排序、RFC3986 编码与 HMAC-SHA1（密钥为 Secret 加 "&"）。
func TestNowcnSignature(t *testing.T) {
	params := map[string]string{"Domain": "example.com", "Host": "a b*", "AccessInstanceID": "inst"}
	got := nowcnSign("sk", params)
	// 待签名串：GET&%2F&<编码后的规范化查询串>，其中空格为 %20、* 为 %2A
	again := nowcnSign("sk", map[string]string{"AccessInstanceID": "inst", "Host": "a b*", "Domain": "example.com"})
	if got != again || got == "" {
		t.Fatalf("签名应与参数顺序无关: %s %s", got, again)
	}
	if nowcnSign("other", params) == got {
		t.Fatal("密钥不同签名应不同")
	}
	if nowcnPercentEncode("a b*~/") != "a%20b%2A~%2F" {
		t.Fatalf("编码错误: %s", nowcnPercentEncode("a b*~/"))
	}
}
