package waf

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/luckyimport"
	"github.com/wangwanjie/gately/internal/module"
)

func newModule(t *testing.T, insts ...Instance) *Module {
	t.Helper()
	st, _ := config.Open(t.TempDir())
	m := New()
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(200, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	for _, in := range insts {
		if err := in.Validate(); err != nil {
			t.Fatal(err)
		}
		m.cfg.Instances = append(m.cfg.Instances, in)
		m.buildLocked(in.Key)
	}
	return m
}

func status(t *testing.T, h http.Handler, target string) int {
	t.Helper()
	req := httptest.NewRequest("GET", target, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "text/html")
	req.RemoteAddr = "203.0.113.7:4000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

var ok = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })

func TestCRSBlocksAttacks(t *testing.T) {
	m := newModule(t, Instance{Key: "full", Name: "full", Enable: true, RuleSets: []RuleSet{{Enable: true, Type: RuleCRS}}})
	h := m.Middleware("full", ok)
	if c := status(t, h, "/index.html?page=2"); c != 200 {
		t.Fatalf("正常请求不应被拦截: %d", c)
	}
	attacks := []string{
		"/?file=" + url.QueryEscape("../../../../etc/passwd"),
		"/?id=" + url.QueryEscape("1' OR '1'='1' UNION SELECT password FROM users--"),
		"/?q=" + url.QueryEscape("<script>alert(document.cookie)</script>"),
	}
	for _, a := range attacks {
		if c := status(t, h, a); c != 403 {
			t.Errorf("攻击请求应被拦截 %s: %d", a, c)
		}
	}
	v := m.viewLocked(&m.cfg.Instances[0]).(InstanceView)
	if v.Blocked != int64(len(attacks)) {
		t.Fatalf("拦截计数错误: %d", v.Blocked)
	}
	logs := m.logs.Since(0)
	if len(logs) == 0 || !strings.Contains(logs[0].Message, "203.0.113.7") {
		t.Fatalf("应记录命中日志并包含客户端 IP: %v", logs)
	}
	// 命中日志带有结构化字段：Coraza 的命中信息里没有域名，要从登记的请求信息中补上
	f := logs[0].Fields
	if f[logbuf.FieldIP] != "203.0.113.7" || f[logbuf.FieldHost] == "" || f[logbuf.FieldRule] == "" || f[logbuf.FieldPath] == "" {
		t.Fatalf("命中日志字段不完整: %+v", f)
	}
	n := 0
	pending.Range(func(any, any) bool { n++; return true })
	if n != 0 {
		t.Fatalf("请求结束后应清理登记的请求信息，剩余 %d 条", n)
	}
}

func TestCRSSubset(t *testing.T) {
	// 只启用 LFI 规则子集
	m := newModule(t, Instance{Key: "lfi", Name: "lfi", Enable: true, InboundThreshold: 6, OutboundThreshold: 4,
		RuleSets: []RuleSet{{Enable: true, Type: RuleCRS, Param: "REQUEST-901-INITIALIZATION.conf\nREQUEST-930-APPLICATION-ATTACK-LFI.conf\nREQUEST-949-BLOCKING-EVALUATION.conf"}}})
	h := m.Middleware("lfi", ok)
	if c := status(t, h, "/?f="+url.QueryEscape("../../../etc/shadow")); c != 403 {
		t.Fatalf("LFI 应被拦截: %d", c)
	}
	// 未启用 SQL 注入规则，不拦截
	if c := status(t, h, "/?id="+url.QueryEscape("1' UNION SELECT 1--")); c != 200 {
		t.Fatalf("未启用的规则不应拦截: %d", c)
	}
}

func TestDetectModeCustomRuleAndPassThrough(t *testing.T) {
	m := newModule(t,
		Instance{Key: "det", Name: "det", Enable: true, Mode: ModeDetect, RuleSets: []RuleSet{{Enable: true, Type: RuleCRS}}},
		Instance{Key: "cus", Name: "cus", Enable: true, RuleSets: []RuleSet{{Enable: true, Type: RuleCustom,
			Param: `SecRule REQUEST_URI "@beginsWith /admin" "id:10001,phase:1,deny,status:403,msg:'禁止访问后台'"`}}},
		Instance{Key: "off", Name: "off", Enable: false, RuleSets: []RuleSet{{Enable: true, Type: RuleCustom, Param: `SecRule REQUEST_URI "@rx ." "id:10002,phase:1,deny,status:403"`}}},
	)
	if c := status(t, m.Middleware("det", ok), "/?f="+url.QueryEscape("../../etc/passwd")); c != 200 {
		t.Fatalf("仅检测模式不应拦截: %d", c)
	}
	if v := m.viewLocked(&m.cfg.Instances[0]).(InstanceView); v.Matched == 0 {
		t.Fatal("仅检测模式应记录命中")
	}
	if c := status(t, m.Middleware("cus", ok), "/admin/login"); c != 403 {
		t.Fatalf("自定义规则应拦截: %d", c)
	}
	if c := status(t, m.Middleware("off", ok), "/x"); c != 200 {
		t.Fatalf("停用实例应放行: %d", c)
	}
	if c := status(t, m.Middleware("missing", ok), "/x"); c != 200 {
		t.Fatalf("不存在的实例应放行: %d", c)
	}
}

func TestValidate(t *testing.T) {
	bad := []Instance{
		{Name: ""},
		{Name: "a", Mode: "x"},
		{Name: "a", Paranoia: 5},
		{Name: "a", RuleSets: []RuleSet{{Enable: true, Type: RuleCRS, Param: "NOPE.conf"}}},
		{Name: "a", RuleSets: []RuleSet{{Enable: true, Type: RuleCustom, Param: "SecRule BROKEN"}}},
	}
	for i, in := range bad {
		if err := in.Validate(); err == nil {
			t.Errorf("第 %d 个非法实例未报错", i)
		}
	}
	if len(CRSFiles()) < 20 {
		t.Fatal("内置 CRS 规则文件数量异常")
	}
}

// TestImportRealSample 仅在设置 GATELY_LUCKY_WAF_SAMPLE 时运行。
func TestImportRealSample(t *testing.T) {
	path := os.Getenv("GATELY_LUCKY_WAF_SAMPLE")
	if path == "" {
		t.Skip("未设置 GATELY_LUCKY_WAF_SAMPLE")
	}
	data, _ := os.ReadFile(path)
	doc, err := luckyimport.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	m := newModule(t)
	rep, err := m.ImportLucky(doc, luckyimport.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("found=%d imported=%d %v", rep.Found, rep.Imported, rep.Warnings)
	if rep.Imported != rep.Found {
		t.Fatal("有实例未能导入")
	}
	for _, in := range m.cfg.Instances {
		h := m.Middleware(in.Key, ok)
		t.Logf("%s(%s) LFI 拦截=%v", in.Name, in.Key, status(t, h, "/?f="+url.QueryEscape("../../etc/passwd")) == 403)
	}
}

// postJSON 以 JSON 请求体提交，返回状态码。
func postJSON(t *testing.T, h http.Handler, body string) int {
	t.Helper()
	req := httptest.NewRequest("PUT", "/api/x", strings.NewReader(body))
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.7:4000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestJSONFieldNamesNotLFI(t *testing.T) {
	// 与 hk_main 的 waf-main 一样只启用 LFI 规则，阈值取 CRS 默认的 5：命中一条严重规则即拦截
	m := newModule(t, Instance{Key: "lfi", Name: "lfi", Enable: true, InboundThreshold: 5, OutboundThreshold: 4,
		RuleSets: []RuleSet{{Enable: true, Type: RuleCRS, Param: "REQUEST-901-INITIALIZATION.conf\nREQUEST-930-APPLICATION-ATTACK-LFI.conf\nREQUEST-949-BLOCKING-EVALUATION.conf"}}})
	h := m.Middleware("lfi", ok)
	// 字段名展开后为 json.credentials.secret、json.default.forwardAuth.url 等，曾被 930120 误判
	benign := []string{
		`{"name":"ddns","credentials":{"id":"LTAI","secret":"xxx"}}`,
		`{"port":443,"default":{"forwardedHeaders":true,"forwardAuth":{"url":"http://127.0.0.1:9091","loginUrl":""}}}`,
		`{"history":[1,2],"profile":{"secrets":"a"},"bash_alias":"x"}`,
	}
	for _, b := range benign {
		if c := postJSON(t, h, b); c != 200 {
			t.Errorf("普通 JSON 不应被拦截 %s: %d", b, c)
		}
	}
	// 字段值中的攻击载荷照常拦截
	if c := postJSON(t, h, `{"file":"../../../../etc/passwd"}`); c != 403 {
		t.Fatalf("JSON 值中的 LFI 应被拦截: %d", c)
	}
	// 非 JSON 参数名不受排除影响
	if c := status(t, h, "/?"+url.QueryEscape(".bash_history")+"=1"); c != 403 {
		t.Fatalf("查询参数名中的敏感文件名应被拦截: %d", c)
	}
}

// post 以 POST 提交请求体，并确认后端收到的请求体完整无缺。
func post(t *testing.T, h http.Handler, target, body string) int {
	t.Helper()
	req := httptest.NewRequest("POST", target, strings.NewReader(body))
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "203.0.113.7:4000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK && rec.Body.String() != body {
		t.Fatalf("后端收到的请求体不完整: %d / %d 字节", rec.Body.Len(), len(body))
	}
	return rec.Code
}

// echo 原样返回请求体，用于检查经过 WAF 后请求体是否完整。
var echo = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.Copy(w, r.Body) })

func TestRequestBodyLimit(t *testing.T) {
	crs := []RuleSet{{Enable: true, Type: RuleCRS}}
	big := "data=" + strings.Repeat("a", 2<<20)
	attack := "q=" + url.QueryEscape("1' OR '1'='1' UNION SELECT password FROM users--") + "&"
	m := newModule(t,
		Instance{Key: "reject", Name: "reject", Enable: true, BodyLimitMB: 1, RuleSets: crs},
		Instance{Key: "partial", Name: "partial", Enable: true, BodyLimitMB: 1, BodyLimitAction: BodyLimitPartial, RuleSets: crs},
		Instance{Key: "skip", Name: "skip", Enable: true, BodyLimitMB: 1, SkipBodyPaths: "/upload\n/dav/", RuleSets: crs},
	)
	if c := post(t, m.Middleware("reject", echo), "/", big); c != http.StatusRequestEntityTooLarge {
		t.Fatalf("超过上限且为 reject 时应返回 413: %d", c)
	}
	h := m.Middleware("partial", echo)
	if c := post(t, h, "/", big); c != http.StatusOK {
		t.Fatalf("partial 模式应放行超过上限的请求体: %d", c)
	}
	if c := post(t, h, "/", attack+big); c != http.StatusForbidden {
		t.Fatalf("partial 模式仍应检查前面的部分: %d", c)
	}
	h = m.Middleware("skip", echo)
	if c := post(t, h, "/upload/file", attack+big); c != http.StatusOK {
		t.Fatalf("跳过路径下不应检查请求体: %d", c)
	}
	if c := post(t, h, "/other", attack); c != http.StatusForbidden {
		t.Fatalf("其他路径仍应检查请求体: %d", c)
	}
	if c := status(t, h, "/upload/?id="+url.QueryEscape("1' OR '1'='1' UNION SELECT password FROM users--")); c != http.StatusForbidden {
		t.Fatalf("跳过路径下仍应检查 URL 参数: %d", c)
	}
}

func TestBodyLimitValidate(t *testing.T) {
	bad := []Instance{
		{Name: "a", BodyLimitMB: -1},
		{Name: "a", BodyLimitMB: maxBodyLimitMB + 1},
		{Name: "a", BodyLimitAction: "drop"},
		{Name: "a", SkipBodyPaths: "upload"},
		{Name: "a", SkipBodyPaths: "/a\"b"},
	}
	for _, in := range bad {
		if err := in.Validate(); err == nil {
			t.Errorf("应拒绝无效配置: %+v", in)
		}
	}
	in := Instance{Name: "a", SkipBodyPaths: "  /upload \n\n/dav/"}
	if err := in.Validate(); err != nil || in.SkipBodyPaths != "/upload\n/dav/" || in.BodyLimitAction != BodyLimitReject {
		t.Fatalf("应整理路径并补全默认值: %v %+v", err, in)
	}
}

// do 发送指定方法、请求头与请求体，返回状态码。
func do(t *testing.T, h http.Handler, method, target string, hdr map[string]string, body string) int {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "*/*")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	req.RemoteAddr = "203.0.113.7:4000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestExpectAndWebDAV(t *testing.T) {
	crs := []RuleSet{{Enable: true, Type: RuleCRS}}
	m := newModule(t,
		Instance{Key: "plain", Name: "plain", Enable: true, RuleSets: crs},
		Instance{Key: "dav", Name: "dav", Enable: true, WebDAV: true, BodyLimitMB: 1, BodyLimitAction: BodyLimitPartial, RuleSets: crs},
	)
	plain, dav := m.Middleware("plain", echo), m.Middleware("dav", echo)
	form := map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Expect": "100-continue"}
	// curl 等客户端上传时带的 Expect: 100-continue 不应被当作受限请求头拦截
	if c := do(t, plain, "POST", "/", form, "a=1"); c != http.StatusOK {
		t.Fatalf("带 Expect 头的普通上传应放行: %d", c)
	}
	davHdr := map[string]string{"Content-Type": "application/xml", "Depth": "1", "Lock-Token": "<opaquelocktoken:1>", "If": "(<opaquelocktoken:1>)"}
	if c := do(t, plain, "PROPFIND", "/dav/", davHdr, "<propfind/>"); c != http.StatusForbidden {
		t.Fatalf("默认实例应拦截 WebDAV 方法与锁请求头: %d", c)
	}
	if c := do(t, dav, "PROPFIND", "/dav/", davHdr, "<propfind/>"); c != http.StatusOK {
		t.Fatalf("WebDAV 兼容模式应放行 PROPFIND 与锁请求头: %d", c)
	}
	for _, method := range []string{"MKCOL", "MOVE", "COPY", "LOCK", "UNLOCK", "DELETE"} {
		if c := do(t, dav, method, "/dav/a", map[string]string{"Destination": "/dav/b"}, ""); c != http.StatusOK {
			t.Fatalf("WebDAV 兼容模式应放行 %s: %d", method, c)
		}
	}
	upload := map[string]string{"Content-Type": "image/jpeg", "Content-Range": "bytes 0-9/100", "Expect": "100-continue"}
	if c := do(t, plain, "PUT", "/dav/a.jpg", upload, strings.Repeat("x", 10)); c != http.StatusForbidden {
		t.Fatalf("默认实例应拦截 PUT 与非白名单内容类型: %d", c)
	}
	if c := do(t, dav, "PUT", "/dav/a.jpg", upload, strings.Repeat("x", 3<<20)); c != http.StatusOK {
		t.Fatalf("WebDAV 兼容模式应放行任意内容类型的大文件上传: %d", c)
	}
	// 二进制文件内容里难免出现像注入语句的片段，WebDAV 模式下 PUT 与 octet-stream 不检查请求体
	binary := "a=1&q=" + url.QueryEscape("1' OR '1'='1' UNION SELECT password FROM users--") + "&<script>alert(1)</script>"
	if c := do(t, dav, "PUT", "/dav/a.bin", map[string]string{"Content-Type": "application/octet-stream"}, binary); c != http.StatusOK {
		t.Fatalf("WebDAV 兼容模式不应检查 PUT 的文件内容: %d", c)
	}
	if c := do(t, dav, "PUT", "/dav/noctype.bin", nil, binary); c != http.StatusOK {
		t.Fatalf("WebDAV 兼容模式应放行不带 Content-Type 的 PUT: %d", c)
	}
	if c := do(t, dav, "POST", "/dav/raw", map[string]string{"Content-Type": "application/octet-stream"}, binary); c != http.StatusOK {
		t.Fatalf("WebDAV 兼容模式不应检查 octet-stream 请求体: %d", c)
	}
	attack := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	if c := do(t, dav, "POST", "/form", attack, binary); c != http.StatusForbidden {
		t.Fatalf("WebDAV 兼容模式仍应检查表单请求体: %d", c)
	}
	if c := do(t, dav, "POST", "/?id="+url.QueryEscape("1' OR '1'='1' UNION SELECT password FROM users--"), attack, "a=1"); c != http.StatusForbidden {
		t.Fatalf("WebDAV 兼容模式仍应拦截注入攻击: %d", c)
	}
}

// OAuth/OIDC 回调的 scope 参数（如 "email profile openid"）会命中 CRS 930120（OS 文件访问），
// 用自定义规则组只对该回调路径排除 ARGS:scope，其余检查与其他路径保持不变。
func TestOAuthScopeExclusionByCustomRule(t *testing.T) {
	cb := "/oauth_google_callback.php?state=abc&iss=https://accounts.google.com&code=4%2F0Ab&scope=email+profile+https://www.googleapis.com/auth/userinfo.email+https://www.googleapis.com/auth/userinfo.profile+openid&authuser=2&prompt=none"
	crs := RuleSet{Enable: true, Type: RuleCRS}
	exclude := RuleSet{Enable: true, Type: RuleCustom,
		Param: `SecRule REQUEST_FILENAME "@streq /oauth_google_callback.php" "id:10100,phase:1,pass,nolog,ctl:ruleRemoveTargetById=930120;ARGS:scope"`}
	m := newModule(t,
		Instance{Key: "plain", Name: "plain", Enable: true, Mode: ModeBlock, RuleSets: []RuleSet{crs}},
		Instance{Key: "excl", Name: "excl", Enable: true, Mode: ModeBlock, RuleSets: []RuleSet{exclude, crs}},
	)
	if c := status(t, m.Middleware("plain", ok), cb); c != 403 {
		t.Fatalf("未排除时应复现误报: %d", c)
	}
	h := m.Middleware("excl", ok)
	if c := status(t, h, cb); c != 200 {
		t.Fatalf("排除后回调应放行: %d", c)
	}
	if c := status(t, h, "/x.php?scope="+url.QueryEscape("../../etc/passwd")); c != 403 {
		t.Fatalf("其他路径仍应拦截: %d", c)
	}
	if c := status(t, h, "/oauth_google_callback.php?code="+url.QueryEscape("../../etc/passwd")); c != 403 {
		t.Fatalf("回调路径的其他参数仍应拦截: %d", c)
	}
}
