package webservice

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeCasdoor 模拟 Casdoor 原生 /api/forward-auth：带 X-Original-URL（nginx auth_request 方式）且未登录时返回
// 401 + 动态 Location，并设置 state Cookie；已登录返回 200 与身份头。测试可通过 X-Test-Location 覆盖 Location，
// 通过 X-Test-Status 改用其他状态码。
func fakeCasdoor(t *testing.T) (*httptest.Server, func() http.Header) {
	t.Helper()
	var mu sync.Mutex
	var seen http.Header
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = r.Header.Clone()
		mu.Unlock()
		if c, err := r.Cookie("casdoor_session_id"); err == nil && c.Value == "ok" {
			w.Header().Set("X-Forwarded-User", "alice")
			w.WriteHeader(http.StatusOK)
			return
		}
		loc := "https://login.example.com/login/oauth/authorize?client_id=c&state=" + r.Header.Get("X-Original-URL")
		if v, ok := r.Header["X-Test-Location"]; ok {
			delete(w.Header(), "Location")
			for _, l := range v {
				// -none- 表示不返回 Location
				if l != "-none-" {
					w.Header().Add("Location", l)
				}
			}
		} else {
			w.Header().Set("Location", loc)
		}
		w.Header().Add("Set-Cookie", "casdoor_state=s1; Path=/; HttpOnly")
		w.Header().Add("Set-Cookie", "casdoor_nonce=n1; Path=/")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Internal", "secret")
		status := http.StatusUnauthorized
		if r.Header.Get("X-Test-Status") == "403" {
			status = http.StatusForbidden
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte("auth-body"))
	}))
	t.Cleanup(s.Close)
	return s, func() http.Header {
		mu.Lock()
		defer mu.Unlock()
		return seen
	}
}

// TestForwardAuthLoginFromLocation 开启「使用响应中的登录地址」后，认证接口 401 带的动态 Location 转成 302 跳转，
// Cookie 与缓存策略随跳转传给浏览器；关闭时保持 401 原样返回；静态登录地址优先；2xx、403 语义不变。
func TestForwardAuthLoginFromLocation(t *testing.T) {
	authz, seen := fakeCasdoor(t)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok:" + r.Header.Get("X-Forwarded-User")))
	}))
	defer backend.Close()
	copyHeaders := []string{"X-Forwarded-User"}
	l := Listener{Sites: []Site{{
		Enable: true, Domains: []string{"on.test"}, Targets: []string{backend.URL},
		ForwardAuth: ForwardAuth{URL: authz.URL + "/api/forward-auth", LoginFromLocation: true, CopyHeaders: copyHeaders},
	}, {
		Enable: true, Domains: []string{"off.test"}, Targets: []string{backend.URL},
		ForwardAuth: ForwardAuth{URL: authz.URL + "/api/forward-auth", CopyHeaders: copyHeaders},
	}, {
		Enable: true, Domains: []string{"static.test"}, Targets: []string{backend.URL},
		ForwardAuth: ForwardAuth{URL: authz.URL + "/api/forward-auth", LoginURL: "https://static.example.com/?rd={url}", LoginFromLocation: true, CopyHeaders: copyHeaders},
	}}}
	addr, _ := startRunner(t, l, nil, nil)

	// 开启：401 + Location 转为 302，Location 原样采用（含动态 state）
	resp, body := get(t, noRedirect(), addr, "on.test", "/app?x=1", nil)
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("开启后未登录应 302，实际 %d", resp.StatusCode)
	}
	want := "https://login.example.com/login/oauth/authorize?client_id=c&state=http://on.test/app?x=1"
	if got := resp.Header.Get("Location"); got != want {
		t.Fatalf("Location = %q; want %q", got, want)
	}
	if seen().Get("X-Original-URL") != "http://on.test/app?x=1" {
		t.Fatalf("认证子请求仍应带 X-Original-URL: %v", seen())
	}
	if cs := resp.Header.Values("Set-Cookie"); len(cs) != 2 || !strings.HasPrefix(cs[0], "casdoor_state=s1") || !strings.HasPrefix(cs[1], "casdoor_nonce=n1") {
		t.Fatalf("Set-Cookie 应随跳转传递: %v", cs)
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control 应随跳转传递: %q", resp.Header.Get("Cache-Control"))
	}
	// 认证服务的其他头与响应体不外泄
	if resp.Header.Get("X-Internal") != "" || strings.Contains(body, "auth-body") {
		t.Fatalf("不应外泄认证响应的其他内容: hdr=%v body=%q", resp.Header, body)
	}

	// 非 GET 请求同样跳转，与静态登录地址的行为一致
	req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/submit", strings.NewReader("a=1"))
	req.Host = "on.test"
	if resp, err := noRedirect().Do(req); err != nil || resp.StatusCode != http.StatusFound {
		t.Fatalf("POST 未登录应 302: %v %v", err, resp)
	} else {
		resp.Body.Close()
	}

	// 已登录：放行并复制身份头
	if _, body := get(t, noRedirect(), addr, "on.test", "/", map[string]string{"Cookie": "casdoor_session_id=ok", "X-Forwarded-User": "mallory"}); body != "ok:alice" {
		t.Fatalf("已登录应放行: %q", body)
	}

	// 403 带 Location 也不转换，原样返回
	resp, _ = get(t, noRedirect(), addr, "on.test", "/", map[string]string{"X-Test-Status": "403"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("403 应原样返回，实际 %d", resp.StatusCode)
	}

	// 关闭：保持 401 + Location 原样返回
	resp, _ = get(t, noRedirect(), addr, "off.test", "/", nil)
	if resp.StatusCode != http.StatusUnauthorized || !strings.HasPrefix(resp.Header.Get("Location"), "https://login.example.com/") {
		t.Fatalf("未开启时应原样返回 401: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}

	// 同时填写静态登录地址时静态地址优先
	resp, _ = get(t, noRedirect(), addr, "static.test", "/", nil)
	if resp.StatusCode != http.StatusFound || !strings.HasPrefix(resp.Header.Get("Location"), "https://static.example.com/?rd=") {
		t.Fatalf("静态登录地址应优先: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
}

// TestForwardAuthLoginFromLocationRejects 认证接口给出的 Location 不是唯一的 http(s) 绝对地址时不跳转，按 401 原样返回，
// 避免把 javascript:、协议相对地址或带用户信息的地址交给浏览器。
func TestForwardAuthLoginFromLocationRejects(t *testing.T) {
	authz, _ := fakeCasdoor(t)
	l := Listener{Sites: []Site{{
		Enable: true, Domains: []string{"on.test"}, Type: TypeText, Text: "ok",
		ForwardAuth: ForwardAuth{URL: authz.URL, LoginFromLocation: true},
	}}}
	addr, _ := startRunner(t, l, nil, nil)
	for name, locs := range map[string][]string{
		"缺少":    {"-none-"},
		"空":     {""},
		"相对路径":  {"/login"},
		"协议相对":  {"//evil.example.com/login"},
		"脚本":    {"javascript:alert(1)"},
		"数据":    {"data:text/html,hi"},
		"ftp":   {"ftp://login.example.com/"},
		"无主机":   {"https:///login"},
		"用户信息":  {"https://user:pw@login.example.com/"},
		"多个":    {"https://a.example.com/", "https://b.example.com/"},
		"超长":    {"https://login.example.com/" + strings.Repeat("a", 9000)},
		"含制表符":  {"https://login.example.com/\tx"},
		"非法转义":  {"https://login.example.com/%zz"},
		"反斜杠主机": {"https:\\\\evil.example.com"},
	} {
		req, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/", nil)
		req.Host = "on.test"
		req.Header["X-Test-Location"] = locs
		resp, err := noRedirect().Do(req)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s: Location %q 应按 401 原样返回，实际 %d %q", name, locs, resp.StatusCode, resp.Header.Get("Location"))
		}
	}
	// 大小写不同的协议名可以接受
	if to, ok := authLoginLocation([]string{" HTTPS://login.example.com/a?b=1 "}); !ok || to != "HTTPS://login.example.com/a?b=1" {
		t.Fatalf("HTTPS 大写应接受: %q %v", to, ok)
	}
}

// TestForwardAuthLoginFromLocationJSON 新字段在配置中的读写：关闭时不写入 JSON，旧配置读入默认关闭。
func TestForwardAuthLoginFromLocationJSON(t *testing.T) {
	b, _ := json.Marshal(ForwardAuth{URL: "http://a"})
	if strings.Contains(string(b), "loginFromLocation") {
		t.Fatalf("关闭时不应写入: %s", b)
	}
	var f ForwardAuth
	if err := json.Unmarshal([]byte(`{"url":"http://127.0.0.1:8000/api/forward-auth","loginFromLocation":true}`), &f); err != nil || !f.LoginFromLocation {
		t.Fatalf("读入失败: %v %+v", err, f)
	}
	if err := f.validate(); err != nil {
		t.Fatalf("校验失败: %v", err)
	}
}
