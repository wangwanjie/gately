package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// JSON 模板中位于字符串内的变量按 JSON 字符串转义，字符串外与非 JSON 模板保持原样。
func TestRenderJSON(t *testing.T) {
	vars := map[string]string{
		"path":  `/a","admin":true,"x":"`,
		"text":  "line1\nline2\t\"q\" \\ <b>",
		"port":  "8443",
		"ip":    "1.2.3.4",
		"empty": "",
	}
	cases := map[string]string{
		// 字符串内：转义引号、反斜杠、换行与制表符（不转义 <>&，与原样拼接一致）
		`{"msg":"{path}"}`:          `{"msg":"/a\",\"admin\":true,\"x\":\""}`,
		`{"msg":"WAF: {text} end"}`: `{"msg":"WAF: line1\nline2\t\"q\" \\ <b> end"}`,
		`{"a":{"b":"{text}"}}`:      `{"a":{"b":"line1\nline2\t\"q\" \\ <b>"}}`,
		`[{"ip":"{ip}"},"{empty}"]`: `[{"ip":"1.2.3.4"},""]`,
		// 字符串外的变量（数字等）原样替换
		`{"port":{port},"ip":"{ip}"}`: `{"port":8443,"ip":"1.2.3.4"}`,
		// 字符串中的转义引号不影响判断
		`{"a":"x\"{ip}\"y","b":"{text}"}`: `{"a":"x\"1.2.3.4\"y","b":"line1\nline2\t\"q\" \\ <b>"}`,
		// 未知变量保持原样
		`{"a":"{nope}","b":"{ip}"}`: `{"a":"{nope}","b":"1.2.3.4"}`,
		// 不是 JSON 的模板保持原样
		"{text}":                 "line1\nline2\t\"q\" \\ <b>",
		"msg={path}&port={port}": `msg=/a","admin":true,"x":"&port=8443`,
		`{"broken": {path}`:      `{"broken": /a","admin":true,"x":"`,
	}
	for in, want := range cases {
		if got := RenderJSON(in, vars); got != want {
			t.Errorf("RenderJSON(%q)\n got %q\nwant %q", in, got, want)
		}
	}
	// 转义后的结果是合法 JSON，字段没有被追加或覆盖
	var out map[string]any
	if err := json.Unmarshal([]byte(RenderJSON(`{"msg":"{path}","admin":false}`, vars)), &out); err != nil || out["admin"] != false || out["msg"] != vars["path"] {
		t.Fatalf("访客文本不应改变 JSON 结构: %v %v", out, err)
	}
}

// Send 对 JSON 请求体做转义，Content-Type 明确为其他类型时原样发送；请求头中的变量去掉换行。
func TestSendEscapesVisitorText(t *testing.T) {
	var gotBody, gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotHeader = r.Header.Get("X-Path")
	}))
	defer srv.Close()
	vars := map[string]string{"path": "/x\"\r\nX-Evil: 1"}
	cfg := Config{Enable: true, URL: srv.URL, Method: "POST", Headers: []string{"X-Path: {path}"}, Body: `{"path":"{path}"}`}
	if _, err := Send(context.Background(), cfg, vars); err != nil {
		t.Fatal(err)
	}
	if gotBody != `{"path":"/x\"\r\nX-Evil: 1"}` {
		t.Fatalf("JSON 请求体应转义: %q", gotBody)
	}
	if gotHeader != `/x"  X-Evil: 1` {
		t.Fatalf("请求头中的变量应去掉换行: %q", gotHeader)
	}
	cfg.Headers = []string{"Content-Type: text/plain"}
	cfg.Body = `{"path":"{path}"}`
	if _, err := Send(context.Background(), cfg, vars); err != nil {
		t.Fatal(err)
	}
	if gotBody != "{\"path\":\"/x\"\r\nX-Evil: 1\"}" {
		t.Fatalf("非 JSON 内容类型应原样发送: %q", gotBody)
	}
	cfg.Headers = []string{"Content-Type: application/vnd.api+json"}
	if _, err := Send(context.Background(), cfg, vars); err != nil {
		t.Fatal(err)
	}
	if gotBody != `{"path":"/x\"\r\nX-Evil: 1"}` {
		t.Fatalf("+json 内容类型应转义: %q", gotBody)
	}
}
