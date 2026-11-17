package webservice

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// 列表接口读到的监听（带 running、stats、siteCerts 等展示字段）原样提交修改或复制都应成功。
// v0.6.8 新增 siteCerts 后，未剥离它的界面保存站点被以未知字段拒绝；以后再加展示字段，这个测试同样兜住。
func TestListenerViewRoundTrip(t *testing.T) {
	st, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := New(anyCert{selfSigned(t, "a.test")}, nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	// 监听停用，保存时不实际监听端口
	m.cfg.Listeners = []Listener{{Key: "l1", ListenIP: "127.0.0.1", Port: 18093, TLS: true, MinTLS: "1.2", Sites: []Site{
		{Key: "a", Enable: true, Domains: []string{"a.test"}, Type: TypeText, Text: "a", BasicAuth: true, Users: []string{"alice:pass1234"}},
	}}}
	mux := http.NewServeMux()
	m.Routes(prefixRouter{mux, "/api/webservice"})
	call := func(method, path string, body []byte) (int, []byte) {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w.Code, w.Body.Bytes()
	}

	code, body := call("GET", "/api/webservice/listeners", nil)
	var list []map[string]json.RawMessage
	if code != 200 || json.Unmarshal(body, &list) != nil || len(list) != 1 {
		t.Fatalf("列表: %d %s", code, body)
	}
	for _, k := range []string{"siteCerts", "stats", "running"} {
		if _, ok := list[0][k]; !ok {
			t.Fatalf("列表应带展示字段 %s，测试才有意义: %s", k, body)
		}
	}
	raw, _ := json.Marshal(list[0])
	if code, body := call("PUT", "/api/webservice/listeners/l1", raw); code != 200 {
		t.Fatalf("原样提交修改: %d %s", code, body)
	}
	if code, body := call("POST", "/api/webservice/listeners/l1/copy", raw); code != 200 {
		t.Fatalf("原样提交复制: %d %s", code, body)
	}
	// 掩码的站点用户密码沿用原值
	if !verifyUser(m.cfg.Listeners[0].Sites[0].Users, "alice", "pass1234") {
		t.Fatalf("站点用户密码应沿用: %v", m.cfg.Listeners[0].Sites[0].Users)
	}
}
