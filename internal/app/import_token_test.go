package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/webservice"
	"github.com/wangwanjie/gately/internal/thirdauth"
)

// tokenUpload 以 API 令牌上传导入文件。
func tokenUpload(t *testing.T, url, secret string, files map[string][]byte) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for name, data := range files {
		fw, _ := mw.CreateFormFile("files", name)
		_, _ = fw.Write(data)
	}
	_ = mw.Close()
	req, _ := http.NewRequest("POST", url, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

// 只有 import:write 的令牌不能借 Lucky 导入改写只允许登录会话修改的系统设置，也不能写入第三方登录身份表。
func TestImportTokenCannotChangeSessionOnlySettings(t *testing.T) {
	// 测试会核对导入报告里的中文提示：固定服务端语言为源语言
	t.Setenv("GATELY_LANG", "zh-CN")
	dir := t.TempDir()
	a, err := New(Options{ConfigDir: dir, Modules: []module.Module{webservice.New(nil, nil)}})
	if err != nil {
		t.Fatal(err)
	}
	a.logger = log.New(io.Discard, "", 0)
	if err := a.auth.ChangeCredentials(mustInitialPassword(t, a), "admin", "test-password"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	doJSON(t, c, "POST", srv.URL+"/api/login", map[string]string{"username": "admin", "password": "test-password"})

	// 管理员先收紧设置：仅内网访问、自定义 DNS、登录失败上限 3 次、允许列表里只有一个身份
	_, body := doJSON(t, c, "GET", srv.URL+"/api/system/settings", nil)
	var s map[string]any
	if err := json.Unmarshal(body, &s); err != nil {
		t.Fatal(err)
	}
	s["lanOnly"], s["maxLoginFailures"], s["dnsServers"] = true, 3, []string{"223.5.5.5"}
	s["thirdLogin"] = map[string]any{"enable": false, "allowed": []string{"github:1 # me"}, "skipTotp": false}
	if code, body := doJSON(t, c, "PUT", srv.URL+"/api/system/settings", s); code != 200 {
		t.Fatalf("保存设置失败 %d %s", code, body)
	}
	before := a.settingsSnapshot()
	ttl := a.auth.TokenTTLHours()
	if !before.LANOnly || before.MaxLoginFailures != 3 {
		t.Fatalf("测试前提不成立: %+v", before)
	}

	_, tok := createToken(t, srv.URL, c, map[string]any{"name": "imp", "scopes": []string{"import:write"}})
	identities := `{"List":[{"Key":"k1","Enable":true,"Type":"github","ID":"666","Name":"x"}],"Config":null,"ConfVer":1}`
	base := `{"AdminWebListenPort":16601,"AllowInternetaccess":true,"MaxConsecutiveLoginFailures":100,"TokenExpirationHour":8760,
	  "CustomDNSList":"203.0.113.53","TimeZone":"Pacific/Auckland","SetGCPercent":true,"GCPercent":900,
	  "EnableThirdAuthLogin":true,"AllowAllThirdAuthUsers":true,"ThirdAuthLoginSkipTwoFA":true}`
	for _, q := range []string{"", "?enable=1", "?dryRun=1"} {
		code, out := tokenUpload(t, srv.URL+"/api/import/lucky"+q, tok, map[string][]byte{
			"lucky_base.lkcf": []byte(base), "lucky_thirdPartyAuthManager.lkcf": []byte(identities),
		})
		if code != 200 {
			t.Fatalf("导入请求应正常返回报告 %d %s", code, out)
		}
		if !strings.Contains(out, "API 令牌不能导入全局设置") || !strings.Contains(out, "API 令牌不能导入第三方登录身份") {
			t.Fatalf("报告应说明这两项需要登录会话导入: %s", out)
		}
	}
	after := a.settingsSnapshot()
	if !after.LANOnly || after.MaxLoginFailures != 3 || strings.Join(after.DNSServers, ",") != "223.5.5.5" ||
		after.TimeZone != before.TimeZone || after.GCPercent != before.GCPercent {
		t.Fatalf("令牌导入不应改动全局设置: %+v", after)
	}
	if strings.Join(after.ThirdLogin.Allowed, "|") != "github:1 # me" || after.ThirdLogin.SkipTOTP {
		t.Fatalf("令牌导入不应改动后台第三方登录: %+v", after.ThirdLogin)
	}
	if got := a.auth.TokenTTLHours(); got != ttl {
		t.Fatalf("令牌导入不应改动会话有效期: %d → %d", ttl, got)
	}
	if ids, _ := thirdauth.LoadLuckyIdentities(a.store); len(ids.List) != 0 {
		t.Fatalf("令牌导入不应写入身份表: %+v", ids.List)
	}

	// 同样的文件由管理员会话导入则生效，并在报告里逐个列出新加入允许列表的身份
	code, body := uploadFiles(t, c, srv.URL+"/api/import/lucky?dryRun=1", "files", map[string][]byte{
		"lucky_base.lkcf": []byte(base), "lucky_thirdPartyAuthManager.lkcf": []byte(identities),
	})
	if code != 200 || !strings.Contains(string(body), "新增到管理后台允许列表的第三方身份") || !strings.Contains(string(body), "github:666 # x") {
		t.Fatalf("预览应列出将要加入允许列表的身份: %d %s", code, body)
	}
	if got := a.settingsSnapshot(); !got.LANOnly || len(got.ThirdLogin.Allowed) != 1 {
		t.Fatalf("预览不应写入: %+v", got)
	}
	code, body = uploadFiles(t, c, srv.URL+"/api/import/lucky", "files", map[string][]byte{
		"lucky_base.lkcf": []byte(base), "lucky_thirdPartyAuthManager.lkcf": []byte(identities),
	})
	if code != 200 {
		t.Fatalf("会话导入失败 %d %s", code, body)
	}
	got := a.settingsSnapshot()
	if got.LANOnly || got.MaxLoginFailures != 100 || strings.Join(got.ThirdLogin.Allowed, "|") != "github:1 # me|github:666 # x" || !got.ThirdLogin.SkipTOTP {
		t.Fatalf("会话导入应照常迁移: %+v", got)
	}
}

// 身份表里的通配与无效标识不会进入允许列表：否则导入一份他人给的配置，就可能让某个登录方式的任何账号成为管理员。
func TestImportRejectsWildcardIdentity(t *testing.T) {
	// 测试会核对导入报告里的中文提示：固定服务端语言为源语言
	t.Setenv("GATELY_LANG", "zh-CN")
	dir := t.TempDir()
	a, err := New(Options{ConfigDir: dir, Modules: []module.Module{webservice.New(nil, nil)}})
	if err != nil {
		t.Fatal(err)
	}
	a.logger = log.New(io.Discard, "", 0)
	if err := a.auth.ChangeCredentials(mustInitialPassword(t, a), "admin", "test-password"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	doJSON(t, c, "POST", srv.URL+"/api/login", map[string]string{"username": "admin", "password": "test-password"})

	identities := `{"List":[
	 {"Key":"ok","Enable":true,"Type":"github","ID":"42","Name":"real\nline2"},
	 {"Key":"w1","Enable":true,"Type":"github","ID":"*","Name":"all"},
	 {"Key":"w2","Enable":true,"Type":"google","ID":" * ","Name":"all"},
	 {"Key":"w3","Enable":true,"Type":"github","ID":"7\ngithub:*","Name":"split"},
	 {"Key":"w4","Enable":true,"Type":"github","ID":"7 #x","Name":"remark"},
	 {"Key":"w5","Enable":true,"Type":"github:*","ID":"9","Name":"type"},
	 {"Key":"w6","Enable":true,"Type":"qq","ID":"","Name":"empty"}],"Config":null,"ConfVer":1}`
	base := `{"AdminWebListenPort":16601,"EnableThirdAuthLogin":true,"AllowAllThirdAuthUsers":true}`
	rp := `{"ReverseProxyRuleList":[{"RuleName":"r","Enable":true,"ListenPort":4434,"Network":"tcp4",
	  "DefaultProxy":{"Locations":["http://127.0.0.1:1"],"WebServiceType":"reverseproxy","OtherParams":{"WebAuth":true,"AllowAllThirdUsers":true}}}]}`
	code, body := uploadFiles(t, c, srv.URL+"/api/import/lucky", "files", map[string][]byte{
		"lucky_base.lkcf": []byte(base), "lucky_reverseproxy.lkcf": []byte(rp), "lucky_thirdPartyAuthManager.lkcf": []byte(identities),
	})
	if code != 200 {
		t.Fatalf("导入失败 %d %s", code, body)
	}
	if !strings.Contains(string(body), "6 个身份的账号标识无效") {
		t.Fatalf("报告应说明忽略了无效身份: %s", body)
	}
	allowed := a.settingsSnapshot().ThirdLogin.Allowed
	if len(allowed) != 1 || allowed[0] != "github:42 # real line2" {
		t.Fatalf("允许列表只应包含有效身份，且一个身份只占一项: %q", allowed)
	}
	for _, e := range allowed {
		if _, v, _ := thirdauth.ParseEntry(e); v == "*" {
			t.Fatalf("允许列表不应出现通配: %q", allowed)
		}
	}
	// 任意账号都不在允许列表内
	if thirdauth.Allowed(allowed, thirdauth.Identity{Provider: "github", Primary: "31337"}) {
		t.Fatal("未列出的账号不应被允许")
	}
	_, lbody := doJSON(t, c, "GET", srv.URL+"/api/webservice/listeners", nil)
	if strings.Contains(string(lbody), `:*`) || !strings.Contains(string(lbody), "github:42") {
		t.Fatalf("站点的允许身份同样不应出现通配: %s", lbody)
	}
	if ids, _ := thirdauth.LoadLuckyIdentities(a.store); len(ids.List) != 1 {
		t.Fatalf("身份表只应记录有效身份: %+v", ids.List)
	}
}
