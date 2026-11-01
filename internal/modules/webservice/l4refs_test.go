package webservice

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/portforward"
	"github.com/wangwanjie/gately/internal/modules/protoroute"
	"github.com/wangwanjie/gately/internal/modules/stun"
)

// prefixRouter 把模块路由挂到 /api/<模块 ID> 下，与正式启动时一致。
type prefixRouter struct {
	mux    *http.ServeMux
	prefix string
}

func (p prefixRouter) HandleFunc(pattern string, h http.HandlerFunc) {
	method, path, _ := strings.Cut(pattern, " ")
	p.mux.HandleFunc(method+" "+p.prefix+path, h)
}

// TestL4RulesReferenceIPLists 通过 HTTP 接口验证：端口转发、协议分流、STUN 规则引用订阅列表后，
// 列表的 usedBy 列出这些规则、删除被引用的列表返回 409；引用不存在的列表或名单非法时保存失败；移除引用后可以删除。
func TestL4RulesReferenceIPLists(t *testing.T) {
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "127.0.0.0/8\n")
	}))
	defer src.Close()

	mux := http.NewServeMux()
	logs := logbuf.New(50, log.New(io.Discard, "", 0))
	mods := []module.Module{New(nil, nil), portforward.New(), protoroute.New(), stun.New()}
	for _, m := range mods {
		st, _ := config.Open(t.TempDir())
		if err := m.Init(module.Env{Store: st, Logs: logs}); err != nil {
			t.Fatal(err)
		}
		m.Routes(prefixRouter{mux, "/api/" + m.ID()})
	}
	do := func(method, path string, body any) (int, map[string]any) {
		t.Helper()
		var rd io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		}
		req := httptest.NewRequest(method, path, rd)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}

	code, l := do("POST", "/api/webservice/iplists", map[string]any{"name": "本机", "url": src.URL, "enable": true})
	if code != 200 {
		t.Fatalf("创建列表失败: %d %v", code, l)
	}
	key := l["key"].(string)

	access := map[string]any{"ipMode": "whitelist", "ipList": []string{}, "ipSets": []string{key}}
	with := func(m map[string]any) map[string]any {
		for k, v := range access {
			m[k] = v
		}
		return m
	}
	rules := []struct {
		path string
		body map[string]any
	}{
		{"/api/portforward/rules", with(map[string]any{"name": "ssh", "protocols": []string{"tcp"}, "listenPorts": "60001", "targetAddresses": []string{"127.0.0.1"}})},
		{"/api/protoroute/rules", with(map[string]any{"name": "mux", "protocols": []string{"tcp"}, "listenPorts": "60002", "targetAddresses": []string{"127.0.0.1"}})},
		{"/api/stun/rules", with(map[string]any{"name": "nas", "protocol": "tcp4", "useGlobalServers": true, "targetAddresses": []string{"127.0.0.1"}, "targetPort": 22})},
	}
	var created []string
	for _, r := range rules {
		code, out := do("POST", r.path, r.body)
		if code != 200 {
			t.Fatalf("%s 保存失败: %d %v", r.path, code, out)
		}
		created = append(created, r.path+"/"+out["key"].(string))

		// 引用不存在的列表、模式非法、网段非法、白名单为空都应被拒绝
		for name, patch := range map[string]map[string]any{
			"不存在的列表": {"ipSets": []string{"no-such-list"}},
			"模式非法":   {"ipMode": "allow"},
			"网段非法":   {"ipList": []string{"10.0.0.0/33"}, "ipSets": []string{}},
			"白名单为空":  {"ipList": []string{}, "ipSets": []string{}},
		} {
			b := map[string]any{}
			for k, v := range r.body {
				b[k] = v
			}
			for k, v := range patch {
				b[k] = v
			}
			if code, out := do("POST", r.path, b); code != http.StatusBadRequest {
				t.Errorf("%s %s 应返回 400，实际 %d %v", r.path, name, code, out)
			}
		}
	}

	req := httptest.NewRequest("GET", "/api/webservice/iplists", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var views []IPListView
	if err := json.Unmarshal(w.Body.Bytes(), &views); err != nil || len(views) != 1 {
		t.Fatalf("读取列表失败: %v %s", err, w.Body.String())
	}
	usedBy, _ := json.Marshal(views[0].UsedBy)
	for _, want := range []string{"portforward / ssh", "protoroute / mux", "stun / nas"} {
		if !strings.Contains(string(usedBy), want) {
			t.Errorf("usedBy 应包含 %q: %s", want, usedBy)
		}
	}
	if code, out := do("DELETE", "/api/webservice/iplists/"+key, nil); code != http.StatusConflict {
		t.Fatalf("删除被引用的列表应返回 409，实际 %d %v", code, out)
	}

	// 删除引用后可以删除列表
	for _, p := range created {
		if code, out := do("DELETE", p, nil); code != 200 {
			t.Fatalf("删除规则失败 %s: %d %v", p, code, out)
		}
	}
	if code, out := do("DELETE", "/api/webservice/iplists/"+key, nil); code != 200 {
		t.Fatalf("移除引用后应能删除列表，实际 %d %v", code, out)
	}
}
