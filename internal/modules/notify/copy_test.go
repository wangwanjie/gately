package notify

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// 复制渠道：只有占位符沿用模板的密钥；留空表示副本不要密钥（避免改了地址后把模板的令牌发过去）；
// 新填写的密钥原样保存。副本排在模板之后，响应不返回密钥。
func TestCopyChannel(t *testing.T) {
	st, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := New()
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(100, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	m.cfg.Channels = []Channel{
		{Key: "c1", Name: "ntfy", Kind: KindNtfy, Target: "topic", Secret: "tk_secret"},
		{Key: "c2", Name: "other", Kind: KindNtfy, Target: "t2"},
	}
	mux := http.NewServeMux()
	m.Routes(testRouter{mux})
	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/channels/c1/copy", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}
	w := post(`{"name":"keep","kind":"ntfy","target":"topic","url":"https://evil.example.com","secret":"********"}`)
	if w.Code != 200 || strings.Contains(w.Body.String(), "tk_secret") || !strings.Contains(w.Body.String(), `"hasSecret":true`) {
		t.Fatalf("复制失败或响应含密钥: %d %s", w.Code, w.Body.String())
	}
	if w := post(`{"name":"clear","kind":"ntfy","target":"topic","url":"https://evil.example.com","secret":""}`); w.Code != 200 {
		t.Fatalf("复制失败: %d %s", w.Code, w.Body.String())
	}
	if w := post(`{"name":"new","kind":"ntfy","target":"topic","secret":"tk_new"}`); w.Code != 200 {
		t.Fatalf("复制失败: %d %s", w.Code, w.Body.String())
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	got := map[string]string{}
	var order []string
	for _, c := range m.cfg.Channels {
		got[c.Name] = c.Secret
		order = append(order, c.Name)
	}
	// 每次都插在模板之后，最近一次复制的最靠前
	if strings.Join(order, ",") != "ntfy,new,clear,keep,other" {
		t.Fatalf("副本应插在模板之后: %v", order)
	}
	if got["keep"] != "tk_secret" || got["clear"] != "" || got["new"] != "tk_new" || got["ntfy"] != "tk_secret" {
		t.Fatalf("密钥处理不符: %v", got)
	}
}
