package app

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/module"
)

// pubStub 是带免登录接口的测试模块：请求体中 token=good 视为有效凭据。
type pubStub struct{}

func (pubStub) ID() string                  { return "pubstub" }
func (pubStub) Title() string               { return "pubstub" }
func (pubStub) Init(module.Env) error       { return nil }
func (pubStub) Start(context.Context) error { return nil }
func (pubStub) Stop() error                 { return nil }
func (pubStub) Routes(r module.Router) {
	r.HandleFunc("GET /secret", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "secret") })
}
func (pubStub) PublicAuth(r *http.Request) bool { return stubToken(r) == "good" }

func (pubStub) PublicRoutes(r module.Router) {
	r.HandleFunc("POST /hello", func(w http.ResponseWriter, req *http.Request) {
		if stubToken(req) != "good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, "hi")
	})
}

// stubToken 读取请求体中的 token 并还原请求体（与 module.PublicRouter 的约定一致）。
func stubToken(r *http.Request) string {
	if r.Body == nil {
		return ""
	}
	b, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(strings.NewReader(string(b)))
	v, _ := url.ParseQuery(string(b))
	return v.Get("token")
}

// 模块的免登录接口不需要登录；设置安全入口后，带有效凭据的请求越过入口，无效凭据与其他接口仍被拦截。
func TestModulePublicRoutes(t *testing.T) {
	a, err := New(Options{ConfigDir: t.TempDir(), Modules: []module.Module{pubStub{}}})
	if err != nil {
		t.Fatal(err)
	}
	a.logger = log.New(io.Discard, "", 0)
	if err := a.auth.ChangeCredentials(mustInitialPassword(t, a), "admin", "test-password"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	cj, _ := cookiejar.New(nil)
	c := &http.Client{Jar: cj}
	if code, body := doJSON(t, c, "POST", srv.URL+"/api/login", map[string]string{"username": "admin", "password": "test-password"}); code != 200 {
		t.Fatalf("登录失败 %d %s", code, body)
	}
	post := func(token string) (int, string) {
		resp, err := http.Post(srv.URL+"/api/pubstub/public/hello", "application/x-www-form-urlencoded", strings.NewReader("token="+token))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	if code, body := post("good"); code != 200 || body != "hi" {
		t.Fatalf("免登录接口应可访问: %d %s", code, body)
	}
	if code, _ := post("bad"); code != http.StatusUnauthorized {
		t.Fatalf("无效凭据应由模块拒绝: %d", code)
	}
	resp, _ := http.Get(srv.URL + "/api/pubstub/secret")
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("模块的其他接口仍需登录: %d", resp.StatusCode)
	}
	// 免登录接口写进接口文档，标明不使用登录会话与 API 令牌
	_, doc := doJSON(t, c, "GET", srv.URL+"/api/openapi.json", nil)
	var spec struct {
		Paths map[string]map[string]map[string]any `json:"paths"`
	}
	_ = json.Unmarshal(doc, &spec)
	op := spec.Paths["/api/pubstub/public/hello"]["post"]
	if op == nil {
		t.Fatalf("免登录接口应出现在接口文档中: %s", doc)
	}
	if _, ok := op["security"]; ok || op["x-token-allowed"] != false || op["x-required-scope"] != scopePublic {
		t.Fatalf("免登录接口应标明不使用令牌: %v", op)
	}

	_, body := doJSON(t, c, "GET", srv.URL+"/api/system/settings", nil)
	var s settingsView
	_ = json.Unmarshal(body, &s)
	s.SafePath = "my-entry"
	if code, body := doJSON(t, c, "PUT", srv.URL+"/api/system/settings", s); code != 200 {
		t.Fatalf("保存设置失败 %d %s", code, body)
	}
	if code, body := post("good"); code != 200 || body != "hi" {
		t.Fatalf("有效凭据应越过安全入口，且处理函数仍能读到请求体: %d %s", code, body)
	}
	if code, _ := post("bad"); code != http.StatusNotFound {
		t.Fatalf("无效凭据应被安全入口拦截: %d", code)
	}
	jar, _ := cookiejar.New(nil)
	fresh := &http.Client{Jar: jar}
	if code, _ := doJSON(t, fresh, "GET", srv.URL+"/api/pubstub/secret", nil); code != http.StatusNotFound {
		t.Fatalf("其他接口仍被安全入口拦截: %d", code)
	}
	// 凭据只对本模块的 public 路径有效
	r2, _ := http.Post(srv.URL+"/api/pubstub/secret", "application/x-www-form-urlencoded", strings.NewReader("token=good"))
	r2.Body.Close()
	if r2.StatusCode != http.StatusNotFound {
		t.Fatalf("凭据不能让其他路径越过安全入口: %d", r2.StatusCode)
	}
}
