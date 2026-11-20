package webservice

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// TestSSOForMultipleDomains 验证多个单点登录域：host 按所在的域匹配，嵌套时取最具体的域。
func TestSSOForMultipleDomains(t *testing.T) {
	var s sharedStore
	s.set(nil, []string{"Example.com.", "example.org", "", "a.example.org"})
	e := s.get()
	for host, want := range map[string]string{
		"example.com":     "example.com",
		"x.example.com":   "example.com",
		"y.example.org":   "example.org",
		"b.a.example.org": "a.example.org",
		"notexample.com":  "",
		"example.net":     "",
	} {
		if got := e.ssoFor(host); got != want {
			t.Errorf("ssoFor(%s) = %q，期望 %q", host, got, want)
		}
	}
}

// TestNormalizeSSODomains 验证单点登录域列表的整理与校验。
func TestNormalizeSSODomains(t *testing.T) {
	got, err := normalizeSSODomains([]string{" Example.COM. ", "", "example.org", "example.com"})
	if err != nil || !slices.Equal(got, []string{"example.com", "example.org"}) {
		t.Fatalf("应规范化并去重: %v %v", got, err)
	}
	if got, err := normalizeSSODomains(nil); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("空列表应得到空切片（JSON 为 []）: %#v %v", got, err)
	}
	for _, bad := range [][]string{
		{"example.com", "a.example.com"},
		{"a.example.com", "example.com"},
		{"localhost"},
		{"example.com/x"},
	} {
		if _, err := normalizeSSODomains(bad); err == nil {
			t.Errorf("应拒绝 %v", bad)
		}
	}
	// 后缀相同但不是子域名的不算嵌套
	if _, err := normalizeSSODomains([]string{"example.com", "myexample.com"}); err != nil {
		t.Fatalf("myexample.com 不是 example.com 的子域名: %v", err)
	}
	many := make([]string, maxSSODomains+1)
	for i := range many {
		many[i] = "d" + strings.Repeat("x", i+1) + ".test"
	}
	if _, err := normalizeSSODomains(many); err == nil {
		t.Fatal("超过上限应被拒绝")
	}
}

// TestSSOSessionPerDomain 验证每个单点登录域各自通用登录态：example.com 下登录后同域其他站点免登录，
// 会话记录所属的域，带到 example.org 的站点无效；早期版本未记录域的会话仍按原规则通用。
func TestSSOSessionPerDomain(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()
	users, err := normalizeAuthUsers([]AuthUser{{Name: "alice", Password: "pw"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	site := func(key, domain string) Site {
		return Site{Enable: true, Key: key, Domains: []string{domain}, Targets: []string{backend.URL}, WebAuth: true, SharedUsers: true}
	}
	l := Listener{Sites: []Site{
		site("a", "a.one.test"), site("b", "b.one.test"),
		site("c", "c.two.test"), site("d", "d.two.test"),
	}}
	addr, r := startRunner(t, l, nil, nil)
	r.auth.shared.set(users, []string{"one.test", "two.test"})
	noFollow := noRedirect()

	login := func(host string) *http.Cookie {
		form := url.Values{"username": {"alice"}, "password": {"pw"}, "next": {"/"}}
		req, _ := http.NewRequest("POST", "http://"+addr+authPath+"/login", strings.NewReader(form.Encode()))
		req.Host = host
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := noFollow.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		for _, c := range resp.Cookies() {
			if c.Name == ssoCookie {
				return c
			}
		}
		t.Fatalf("%s 登录应写出单点登录 Cookie: %d", host, resp.StatusCode)
		return nil
	}
	status := func(host string, c *http.Cookie) int {
		resp, _ := get(t, noFollow, addr, host, "/", map[string]string{"Cookie": c.Name + "=" + c.Value})
		return resp.StatusCode
	}

	one := login("a.one.test")
	two := login("c.two.test")
	if one.Domain != "one.test" || two.Domain != "two.test" {
		t.Fatalf("单点登录 Cookie 应写到各自的域: %s %s", one.Domain, two.Domain)
	}
	if status("b.one.test", one) != 200 || status("d.two.test", two) != 200 {
		t.Fatal("同一单点登录域内的其他站点应免登录")
	}
	// 浏览器不会跨域发送 Cookie；即使被带到另一个域的站点，会话记录的域不符也无效
	if status("c.two.test", one) == 200 || status("a.one.test", two) == 200 {
		t.Fatal("单点登录会话不应在其他单点登录域的站点上生效")
	}
	// 早期版本签发的会话没有记录域（Host 为空），在任一单点登录域下仍按原规则有效
	legacy := &http.Cookie{Name: ssoCookie, Value: r.auth.signBlob("session", siteSession{User: "alice", Scope: "sso", Shared: true, Cred: r.auth.sharedCred(users[0])}, sessionTTL)}
	if status("b.one.test", legacy) != 200 {
		t.Fatal("早期版本的单点登录会话应继续有效")
	}
	// 从单点登录域中移除后，该域的会话不再有效
	r.auth.shared.set(users, []string{"two.test"})
	if status("b.one.test", one) == 200 || status("d.two.test", two) != 200 {
		t.Fatal("移除的单点登录域的会话应失效，其他域不受影响")
	}
}

// TestLegacySSODomainConfig 验证早期版本的单个单点登录域在加载时并入列表，保存后文件只留新字段；
// 认证用户接口也仍接受旧字段。
func TestLegacySSODomainConfig(t *testing.T) {
	dir := t.TempDir()
	st, _ := config.Open(dir)
	raw := `{"settings":{"authSecret":"0123456789abcdef0123456789abcdef","ssoDomain":"Old.example"},"listeners":[]}`
	if err := os.WriteFile(filepath.Join(dir, ID+".json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	m := New(nil, nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	if s := m.cfg.Settings; !slices.Equal(s.SSODomains, []string{"old.example"}) || s.SSODomain != "" {
		t.Fatalf("旧字段应并入列表: %v %q", s.SSODomains, s.SSODomain)
	}
	if m.auth.shared.get().ssoFor("a.old.example") != "old.example" {
		t.Fatal("并入的单点登录域应生效")
	}

	put := func(body string) (int, UsersView) {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/users", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		m.handlePutUsers(rec, r)
		var out UsersView
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	if code, out := put(`{"authUsers":[],"ssoDomains":["one.test","two.test"]}`); code != 200 || !slices.Equal(out.SSODomains, []string{"one.test", "two.test"}) {
		t.Fatalf("保存多个单点登录域失败: %d %v", code, out.SSODomains)
	}
	data, err := os.ReadFile(filepath.Join(dir, ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"ssoDomain"`) || !strings.Contains(string(data), `"ssoDomains"`) {
		t.Fatalf("保存后文件应只有新字段: %s", data)
	}
	if code, out := put(`{"authUsers":[],"ssoDomain":"legacy.test"}`); code != 200 || !slices.Equal(out.SSODomains, []string{"legacy.test"}) {
		t.Fatalf("旧字段应仍可使用: %d %v", code, out.SSODomains)
	}
	if code, _ := put(`{"authUsers":[],"ssoDomains":["x.test","a.x.test"]}`); code != 400 {
		t.Fatalf("互相嵌套的单点登录域应被拒绝: %d", code)
	}
}
