package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/auth"
)

const sampleListener = `{"key":"l1","sites":[{"host":"a.example.com","users":["bob:hunter2","alice:pw"],
"shareLinks":[{"id":"s1","token":"abcdef"}],"requestHeaders":["Authorization: Bearer xyz","X-Plain: v"],
"authSecret":"zzz","selfServicePassword":true,"targets":["http://admin:tpw@10.0.0.2:8080/?key=tq"],"remark":"see https://example.com/docs","upstreams":[{"url":"http://u:p@10.0.0.1:80/x?k=1"}]}],"hasPassword":true}`

func TestRedactJSON(t *testing.T) {
	var v any
	if err := json.Unmarshal([]byte(sampleListener), &v); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(redactJSON(v))
	s := string(out)
	for _, leak := range []string{"hunter2", "alice:pw", "abcdef", "Bearer xyz", "zzz", "u:p@", "k=1", "tpw", "key=tq"} {
		if strings.Contains(s, leak) {
			t.Errorf("脱敏后仍含明文 %q: %s", leak, s)
		}
	}
	// 名称、布尔值、非敏感头不应被改动
	for _, keep := range []string{"bob:", "a.example.com", `"selfServicePassword":true`, `"hasPassword":true`, "X-Plain: ********", "admin:", "https://example.com/docs"} {
		if !strings.Contains(s, keep) {
			t.Errorf("脱敏不应改动 %q: %s", keep, s)
		}
	}
}

// 路径中的令牌与夹在文本中的地址也要脱敏，普通路径保持原样
func TestRedactURLInText(t *testing.T) {
	in := map[string]any{
		"url":    "https://discord.com/api/webhooks/123456789012345678/AbCdEfGhIjKlMnOpQrStUvWxYz0123456789",
		"feishu": "https://open.feishu.cn/open-apis/bot/v2/hook/0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b",
		"error":  `Post "https://hook.example.com/notify?token=abc123secret": dial tcp: i/o timeout`,
		"target": "http://10.0.0.2:8080/api/v1/status",
	}
	out, _ := json.Marshal(redactJSON(in))
	s := string(out)
	for _, leak := range []string{"AbCdEfGhIjKlMnOpQrStUvWxYz0123456789", "0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b", "abc123secret"} {
		if strings.Contains(s, leak) {
			t.Errorf("脱敏后仍含明文 %q: %s", leak, s)
		}
	}
	for _, keep := range []string{"https://discord.com/api/webhooks/123456789012345678/********", "open-apis/bot/v2/hook/", "dial tcp: i/o timeout", `"http://10.0.0.2:8080/api/v1/status"`} {
		if !strings.Contains(s, keep) {
			t.Errorf("脱敏不应改动 %q: %s", keep, s)
		}
	}
}

func serveAs(t *testing.T, a *App, write bool, path string) *httptest.ResponseRecorder {
	t.Helper()
	h := a.redactReadOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(sampleListener))
	}))
	req := httptest.NewRequest("GET", path, nil)
	req = req.WithContext(auth.WithScopeCheck(req.Context(), func(need string) bool {
		return write || strings.HasSuffix(need, ":read")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// 只读身份拿不到站点凭据明文，具备写权限的身份保持原样。
func TestRedactReadOnlyOnly(t *testing.T) {
	a := &App{}
	ro := serveAs(t, a, false, "/api/webservice/listeners")
	if strings.Contains(ro.Body.String(), "hunter2") || strings.Contains(ro.Body.String(), "abcdef") {
		t.Fatalf("只读身份不应看到明文: %s", ro.Body.String())
	}
	if ro.Header().Get("Content-Length") == "" {
		t.Error("脱敏后应更新 Content-Length")
	}
	rw := serveAs(t, a, true, "/api/webservice/listeners")
	if !strings.Contains(rw.Body.String(), "hunter2") {
		t.Fatalf("写权限身份应看到原值: %s", rw.Body.String())
	}
}

// 网络唤醒设备的客户端凭据与关机请求头对只读身份脱敏（配置历史对比等接口返回原始配置）。
func TestRedactWOLAgent(t *testing.T) {
	var v any
	raw := `{"devices":[{"name":"pc","agentToken":"0123456789abcdef0123456789abcdef","shutdownHttp":{"url":"http://ha.lan/api?token=s3cretS3cret123","headers":["Authorization: Bearer hatoken"],"body":"{\"apikey\":\"bodysecret42\"}"}}]}`
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(redactJSON(v))
	s := string(out)
	for _, leak := range []string{"0123456789abcdef", "hatoken", "s3cretS3cret123", "bodysecret42"} {
		if strings.Contains(s, leak) {
			t.Errorf("脱敏后仍含明文 %q: %s", leak, s)
		}
	}
}

// EasyTier 等托管程序的环境变量与附加参数常带密钥：只读身份只能看到变量名
func TestRedactEnvAndExtraArgs(t *testing.T) {
	in := map[string]any{
		"name":      "et1",
		"extraArgs": "--network-secret=s3cret --no-tun",
		"env":       "ET_NETWORK_SECRET=abc\nRUST_LOG=info\n\nNOEQUALS",
		"nested":    map[string]any{"env": map[string]any{"TOKEN": "t0k", "EMPTY": ""}},
	}
	out, _ := json.Marshal(redactJSON(in))
	s := string(out)
	for _, leak := range []string{"s3cret", "abc", "info", "t0k", "NOEQUALS"} {
		if strings.Contains(s, leak) {
			t.Errorf("脱敏后仍含明文 %q: %s", leak, s)
		}
	}
	for _, keep := range []string{"ET_NETWORK_SECRET=********", "RUST_LOG=********", `"TOKEN":"********"`, `"EMPTY":""`, `"name":"et1"`} {
		if !strings.Contains(s, keep) {
			t.Errorf("应保留 %q: %s", keep, s)
		}
	}
}

// DNS 服务商凭据的附加字段：自定义回调的请求头（多行文本）与请求体、OVH 的 Consumer Key 以及未登记的字段都脱敏，
// 接入点、请求方法、成功标识等非密钥字段保留，回调地址按地址规则脱敏。
func TestRedactDDNSCredentialExtra(t *testing.T) {
	raw := `[{"name":"cb","provider":"callback","credentials":{"id":"","secret":"topsecret","extra":{
	  "url":"https://ddns.example.com/update?domain={domain}&key=urlkey123","method":"POST",
	  "headers":"Authorization: Bearer hdrtoken\nX-Api-Key: hdrkey\nnocolonline","body":"{\"apikey\":\"bodykey\"}","successContent":"good\nnochg"}}},
	 {"name":"ovh","provider":"ovh","credentials":{"id":"appkey","secret":"appsecret","extra":{"consumerKey":"consumer123","endpoint":"ovh-eu","Future":"futuresecret"}}},
	 {"name":"cf","provider":"cloudflare","credentials":{"id":"","secret":"cftoken","extra":{"proxied":"true"}}},
	 {"name":"term","credentials":[{"name":"c","password":"credpw","username":"root"}]}]`
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(redactJSON(v))
	s := string(out)
	for _, leak := range []string{"topsecret", "urlkey123", "hdrtoken", "hdrkey", "nocolonline", "bodykey", "appsecret", "consumer123", "futuresecret", "cftoken", "credpw"} {
		if strings.Contains(s, leak) {
			t.Errorf("脱敏后仍含明文 %q: %s", leak, s)
		}
	}
	for _, keep := range []string{`"method":"POST"`, `"headers":"********"`, `"successContent":"good\nnochg"`, `"endpoint":"ovh-eu"`,
		`"proxied":"true"`, `"id":"appkey"`, "https://ddns.example.com/update?", `"username":"root"`, `"provider":"callback"`} {
		if !strings.Contains(s, keep) {
			t.Errorf("应保留 %q: %s", keep, s)
		}
	}
}

// 守护进程的环境变量与各类附加参数、命令与脚本、HTTP 请求体模板对只读身份整段隐藏；
// 版本更新说明等其他名为 body 的字段不受影响。
func TestRedactArgsCommandsAndBodies(t *testing.T) {
	in := map[string]any{
		"instances": []any{map[string]any{
			"name": "ts", "daemonEnv": "TS_AUTHKEY=tskey-envsecret\nTS_DEBUG=1", "extraUpArgs": "--auth-key=tskey-upsecret --ssh",
			"daemonExtraArgs": "--state=mem: --x=daemonsecret", "hostname": "nas",
		}},
		"tasks": []any{
			map[string]any{"name": "c", "type": "command", "spec": "0 3 * * *", "command": "mysql -pcmdsecret"},
			map[string]any{"name": "h", "type": "http", "http": map[string]any{"url": "https://x.example/hook", "method": "POST", "headers": []any{"X-K: hdrsecret"}, "body": `{"token":"bodysecret"}`}},
		},
		"rule":     map[string]any{"script": "echo scriptsecret", "scriptEnable": true},
		"device":   map[string]any{"shutdownCommand": "sshpass -p shutdownsecret poweroff"},
		"args":     []any{"--token", "listsecret"},
		"release":  map[string]any{"version": "v1.2.3", "body": "更新说明：修复若干问题", "publishedAt": "2026-01-01"},
		"icon":     map[string]any{"name": "nas", "body": "<path d=\"M0 0\"/>"},
		"emptyCmd": map[string]any{"command": "", "extraArgs": ""},
		// 请求头写成多行文本时只隐藏值
		"callback": map[string]any{"headers": "Authorization: Bearer textsecret\nX-Plain: v\nnocolonsecret\n"},
	}
	out, _ := json.Marshal(redactJSON(in))
	s := string(out)
	for _, leak := range []string{"envsecret", "upsecret", "daemonsecret", "cmdsecret", "hdrsecret", "bodysecret", "scriptsecret", "shutdownsecret", "listsecret", "--ssh", "TS_DEBUG=1", "textsecret", "nocolonsecret"} {
		if strings.Contains(s, leak) {
			t.Errorf("脱敏后仍含明文 %q: %s", leak, s)
		}
	}
	for _, keep := range []string{"TS_AUTHKEY=********", `"hostname":"nas"`, `"spec":"0 3 * * *"`, `"scriptEnable":true`, "更新说明：修复若干问题", `M0 0`,
		`"method":"POST"`, "https://x.example/hook", `"command":""`, `"extraArgs":""`, `"version":"v1.2.3"`,
		`Authorization: ********\nX-Plain: ********\n********\n`} {
		if !strings.Contains(s, keep) {
			t.Errorf("应保留 %q: %s", keep, s)
		}
	}
}
