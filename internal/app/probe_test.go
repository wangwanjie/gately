package app

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func probeTarget(t *testing.T, c *http.Client, base, target string) (int, probeResult, string) {
	t.Helper()
	code, body := doJSON(t, c, "POST", base+"/api/system/probe", map[string]string{"target": target})
	var r probeResult
	_ = json.Unmarshal(body, &r)
	return code, r, string(body)
}

// 测试连通只返回可达与否、状态码与失败类别，不返回响应头与响应体，也不跟随跳转
func TestProbeReportsStatusOnly(t *testing.T) {
	_, srv, c := loginApp(t)
	var followed atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		followed.Add(1)
	}))
	defer other.Close()
	var method atomic.Value
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method.Store(r.Method + " " + r.URL.Path)
		if r.URL.Path == "/" {
			w.Header().Set("X-Secret-Header", "top-secret-header")
			w.Header().Set("Location", other.URL+"/next")
			w.WriteHeader(http.StatusFound)
			_, _ = w.Write([]byte("top-secret-body"))
		}
	}))
	defer backend.Close()

	code, r, body := probeTarget(t, c, srv.URL, backend.URL+"/admin/secret?x=1")
	if code != 200 || !r.Reachable || r.Status != http.StatusFound {
		t.Fatalf("应可达并返回 302: %d %s", code, body)
	}
	if strings.Contains(body, "top-secret") || strings.Contains(body, other.URL) {
		t.Fatalf("响应不应包含后端的响应头或响应体: %s", body)
	}
	if got := method.Load(); got != "HEAD /" {
		t.Fatalf("应只向根路径发 HEAD，实际 %v", got)
	}
	if followed.Load() != 0 {
		t.Fatal("不应跟随跳转")
	}

	// 只填主机:端口时只测试 TCP 连接
	host := strings.TrimPrefix(backend.URL, "http://")
	if code, r, body := probeTarget(t, c, srv.URL, host); code != 200 || !r.Reachable || r.Status != 0 {
		t.Fatalf("TCP 测试应可达且没有状态码: %d %s", code, body)
	}

	// 关闭的端口：拒绝连接
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := ln.Addr().String()
	_ = ln.Close()
	if code, r, body := probeTarget(t, c, srv.URL, "http://"+closed); code != 200 || r.Reachable || r.Reason != "refused" {
		t.Fatalf("关闭的端口应报告 refused: %d %s", code, body)
	}
}

// https 后端使用自签名证书时提示需要跳过证书校验；用 https 访问明文端口报告协议不符
func TestProbeTLS(t *testing.T) {
	_, srv, c := loginApp(t)
	tlsBackend := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer tlsBackend.Close()
	code, r, body := probeTarget(t, c, srv.URL, tlsBackend.URL)
	if code != 200 || !r.Reachable || !r.Insecure || r.Status != 200 {
		t.Fatalf("自签名证书应可达并标记 insecure: %d %s", code, body)
	}
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer plain.Close()
	code, r, body = probeTarget(t, c, srv.URL, strings.Replace(plain.URL, "http://", "https://", 1))
	if code != 200 || r.Reachable || r.Reason != "protocol" {
		t.Fatalf("https 访问明文端口应报告 protocol: %d %s", code, body)
	}
}

// 只接受 http / https 与 主机:端口，拒绝其他协议、用户信息与非法端口
func TestProbeRejectsBadTargets(t *testing.T) {
	_, srv, c := loginApp(t)
	for _, bad := range []string{"", "file:///etc/passwd", "gopher://127.0.0.1:70/", "ftp://127.0.0.1:21", "http://user:pw@127.0.0.1:80",
		"127.0.0.1", "127.0.0.1:0", "127.0.0.1:70000", "http://127.0.0.1:99999", "http:///x"} {
		if code, _, body := probeTarget(t, c, srv.URL, bad); code != 400 {
			t.Errorf("%q 应被拒绝，实际 %d %s", bad, code, body)
		}
	}
}

// 只有管理员登录会话能调用：只读账号与 API 令牌（即使有全部模块权限）都被拒绝
func TestProbeAdminSessionOnly(t *testing.T) {
	_, srv, admin := loginApp(t)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer backend.Close()

	if code, body := doJSON(t, admin, "POST", srv.URL+"/api/account/users", map[string]string{"username": "viewer", "password": "tide-Lamp-47-oak"}); code != 200 {
		t.Fatalf("创建只读账号失败 %d %s", code, body)
	}
	jar, _ := cookiejar.New(nil)
	ro := &http.Client{Jar: jar}
	if code, body := doJSON(t, ro, "POST", srv.URL+"/api/login", map[string]string{"username": "viewer", "password": "tide-Lamp-47-oak"}); code != 200 {
		t.Fatalf("只读账号登录失败 %d %s", code, body)
	}
	if code, _, _ := probeTarget(t, ro, srv.URL, backend.URL); code != http.StatusForbidden {
		t.Fatalf("只读账号应被拒绝，实际 %d", code)
	}

	_, secret := createToken(t, srv.URL, admin, map[string]any{"name": "全部", "scopes": []string{"portforward:write", "metrics:read"}})
	if code, _ := tokenRequest(t, srv.URL, "POST", "/api/system/probe", secret, map[string]string{"target": backend.URL}); code != http.StatusForbidden {
		t.Fatalf("API 令牌应被拒绝，实际 %d", code)
	}

	// 未登录
	if code, _, _ := probeTarget(t, &http.Client{}, srv.URL, backend.URL); code != http.StatusUnauthorized {
		t.Fatalf("未登录应返回 401，实际 %d", code)
	}
}
