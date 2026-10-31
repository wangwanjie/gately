package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/modules/registry"
)

// newDocApp 创建带全部模块的实例（不监听端口），用于文档与权限测试。
func newDocApp(t *testing.T) *App {
	t.Helper()
	mods, certs := registry.New()
	a, err := New(Options{ConfigDir: t.TempDir(), Certs: certs, Modules: mods})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// 每个已登记的接口都必须有文档摘要，新增接口忘记补文档时测试失败。
func TestAPIDocsCoverAllRoutes(t *testing.T) {
	a := newDocApp(t)
	m := a.buildDoc()
	if len(m.Endpoints) < 100 {
		t.Fatalf("接口数量异常: %d", len(m.Endpoints))
	}
	for _, e := range m.Endpoints {
		if e.Summary == "" {
			t.Errorf("接口缺少文档摘要，请在 apicatalog.go 补充: %s %s", e.Method, e.Path)
		}
	}
	spec, md, err := a.APIDocs("")
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(spec, &parsed); err != nil || parsed["openapi"] != "3.0.3" {
		t.Fatalf("OpenAPI 输出无效: %v", err)
	}
	if !strings.Contains(string(md), "gately.cn") {
		t.Fatal("Markdown 应包含官网地址")
	}
}

func tokenRequest(t *testing.T, base, method, path, secret string, body any) (int, string) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		rd = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, base+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(out)
}

func createToken(t *testing.T, base string, c *http.Client, in map[string]any) (id, secret string) {
	t.Helper()
	code, body := doJSON(t, c, "POST", base+"/api/tokens", in)
	if code != 200 {
		t.Fatalf("创建令牌失败 %d %s", code, body)
	}
	var r struct {
		Token  struct{ ID string } `json:"token"`
		Secret string              `json:"secret"`
	}
	_ = json.Unmarshal(body, &r)
	return r.Token.ID, r.Secret
}

func TestTokenScopes(t *testing.T) {
	_, srv, c := loginApp(t)
	rule := map[string]any{"name": "web", "enable": false, "protocols": []string{"tcp4"}, "listenAddress": "127.0.0.1",
		"listenPorts": "18999", "targetAddresses": []string{"127.0.0.1"}, "targetPorts": "80"}

	_, readSecret := createToken(t, srv.URL, c, map[string]any{"name": "只读", "scopes": []string{"portforward:read"}})
	wid, writeSecret := createToken(t, srv.URL, c, map[string]any{"name": "读写", "scopes": []string{"portforward:write"}})

	// 只读令牌：可读不可写
	if code, body := tokenRequest(t, srv.URL, "GET", "/api/portforward/rules", readSecret, nil); code != 200 {
		t.Fatalf("只读令牌应能读取 %d %s", code, body)
	}
	if code, _ := tokenRequest(t, srv.URL, "POST", "/api/portforward/rules", readSecret, rule); code != 403 {
		t.Fatalf("只读令牌写入应 403，实际 %d", code)
	}
	// 读写令牌：写权限包含读权限，但无权访问其他模块
	if code, body := tokenRequest(t, srv.URL, "POST", "/api/portforward/rules", writeSecret, rule); code != 200 {
		t.Fatalf("读写令牌应能写入 %d %s", code, body)
	}
	if code, _ := tokenRequest(t, srv.URL, "GET", "/api/portforward/rules", writeSecret, nil); code != 200 {
		t.Fatalf("写权限应包含读 %d", code)
	}
	if code, _ := tokenRequest(t, srv.URL, "GET", "/api/status", writeSecret, nil); code != 403 {
		t.Fatalf("未授权的 status 应 403，实际 %d", code)
	}

	// 账号、令牌管理、备份、系统设置永远拒绝令牌，即使拥有全部权限
	_, allSecret := createToken(t, srv.URL, c, map[string]any{"name": "全部", "scopes": []string{"*"}})
	for _, p := range [][2]string{
		{"GET", "/api/tokens"}, {"GET", "/api/system/backup"}, {"GET", "/api/system/settings"},
		{"POST", "/api/system/restart"}, {"GET", "/api/account/totp"},
	} {
		if code, _ := tokenRequest(t, srv.URL, p[0], p[1], allSecret, nil); code != 403 {
			t.Fatalf("%s %s 不应允许令牌访问，实际 %d", p[0], p[1], code)
		}
	}
	if code, _ := tokenRequest(t, srv.URL, "GET", "/api/status", allSecret, nil); code != 200 {
		t.Fatalf("全部权限令牌应能读取状态，实际 %d", code)
	}

	// 停用后立即失效；删除后同样失效
	if code, body := doJSON(t, c, "PUT", srv.URL+"/api/tokens/"+wid, map[string]any{"name": "读写", "scopes": []string{"portforward:write"}, "disabled": true}); code != 200 {
		t.Fatalf("停用失败 %d %s", code, body)
	}
	if code, _ := tokenRequest(t, srv.URL, "GET", "/api/portforward/rules", writeSecret, nil); code != 401 {
		t.Fatalf("停用的令牌应 401，实际 %d", code)
	}

	// 列表不泄露哈希与明文，并记录最近使用
	_, body := doJSON(t, c, "GET", srv.URL+"/api/tokens", nil)
	if strings.Contains(string(body), readSecret) || strings.Contains(string(body), `"hash"`) {
		t.Fatalf("令牌列表不应包含明文或哈希: %s", body)
	}
	if !strings.Contains(string(body), "lastUsedAt") {
		t.Fatalf("列表缺少最近使用信息: %s", body)
	}
	// 非法权限被拒绝
	if code, _ := doJSON(t, c, "POST", srv.URL+"/api/tokens", map[string]any{"name": "x", "scopes": []string{"nope:read"}}); code != 400 {
		t.Fatalf("未知权限应 400，实际 %d", code)
	}
}

func TestLegacyTokenMigration(t *testing.T) {
	dir := t.TempDir()
	a, err := New(Options{ConfigDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	s := a.settingsSnapshot()
	s.LegacyAPIToken = "legacy-token-0123456789"
	if err := a.store.Save(settingsName, s); err != nil {
		t.Fatal(err)
	}

	// 重新启动：旧令牌被迁移为遗留令牌，并从设置中清除
	b, err := New(Options{ConfigDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if b.settingsSnapshot().LegacyAPIToken != "" {
		t.Fatal("迁移后应清除旧设置字段")
	}
	list := b.tokens.List()
	if len(list) != 1 || !list[0].Legacy {
		t.Fatalf("应有一条遗留令牌: %+v", list)
	}
	base := newHandlerServer(t, b)
	if code, body := tokenRequest(t, base, "GET", "/api/system/info", "legacy-token-0123456789", nil); code != 200 {
		t.Fatalf("遗留令牌应保持可用 %d %s", code, body)
	}
	// 再次启动不会重复迁移
	c, err := New(Options{ConfigDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.tokens.List()) != 1 {
		t.Fatal("不应重复迁移")
	}
}

// newHandlerServer 为已有实例启动测试服务并返回地址。
func newHandlerServer(t *testing.T, a *App) string {
	t.Helper()
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	return srv.URL
}

// 可执行命令的权限域都必须是真实存在的模块，防止模块改名后提示失效。
func TestExecDomainsAreModules(t *testing.T) {
	a := newDocApp(t)
	got := map[string]bool{}
	for _, d := range a.scopeDomains() {
		if d.CanExec {
			got[d.ID] = true
		}
	}
	for id := range execDomains {
		if !got[id] {
			t.Errorf("execDomains 中的 %s 不是已注册的模块", id)
		}
	}
	if !got["ddns"] || !got["cron"] || got["portforward"] {
		t.Fatalf("可执行命令标记不正确: %v", got)
	}
}
