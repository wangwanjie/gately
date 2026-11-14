package app

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/logbuf"
)

func TestLogExportAPI(t *testing.T) {
	a, srv, c := loginApp(t)
	pf := a.modules[0].logs
	l := pf.Logger("导出测试")
	l.With(logbuf.Fields{logbuf.FieldIP: "10.0.0.5", logbuf.FieldStatus: "404", logbuf.FieldPath: "/x"}).Infof("10.0.0.5 GET a.example.com/x 404")
	l.With(logbuf.Fields{logbuf.FieldIP: "10.0.0.6", logbuf.FieldStatus: "200", logbuf.FieldPath: "=cmd|' /C calc'!A0"}).Warnf("=HYPERLINK(\"http://evil\")")
	pf.Logger("其他来源").Infof("不应导出")
	base := srv.URL + "/api/portforward/logs/export?source=" + url.QueryEscape("导出测试")

	// 默认 JSON Lines：附件、按时间顺序、只含满足条件的日志
	code, h, body := getRaw(t, c, base, "zh-CN")
	if code != 200 {
		t.Fatalf("导出失败 %d %s", code, body)
	}
	if ct := h.Get("Content-Type"); !strings.HasPrefix(ct, "application/x-ndjson") {
		t.Fatalf("Content-Type 不对: %s", ct)
	}
	if cd := h.Get("Content-Disposition"); !strings.HasPrefix(cd, `attachment; filename="gately-portforward-logs-`) || !strings.HasSuffix(cd, `.jsonl"`) {
		t.Fatalf("Content-Disposition 不对: %s", cd)
	}
	if h.Get("Cache-Control") != "no-store" {
		t.Fatalf("导出不应缓存: %v", h)
	}
	var got []logbuf.Entry
	sc := bufio.NewScanner(bytes.NewReader(body))
	for sc.Scan() {
		var e logbuf.Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("每行应是一条 JSON: %q", sc.Text())
		}
		got = append(got, e)
	}
	if len(got) != 2 || got[0].Message != "10.0.0.5 GET a.example.com/x 404" || got[1].Fields[logbuf.FieldStatus] != "200" || got[0].Seq >= got[1].Seq {
		t.Fatalf("导出内容不对: %+v", got)
	}

	// CSV：带 BOM 与表头，以 = 开头的单元格加单引号，防止表格软件执行公式
	code, h, body = getRaw(t, c, base+"&format=csv&level=warn", "zh-CN")
	if code != 200 || !strings.HasPrefix(h.Get("Content-Type"), "text/csv") {
		t.Fatalf("CSV 导出失败 %d %v", code, h)
	}
	if !bytes.HasPrefix(body, []byte("\ufeff")) {
		t.Fatal("CSV 应以 UTF-8 BOM 开头")
	}
	rows, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(body, []byte("\ufeff")))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || strings.Join(rows[0][:4], ",") != "time,level,source,message" {
		t.Fatalf("CSV 表头或行数不对: %v", rows)
	}
	row := rows[1]
	if row[1] != "warn" || row[2] != "导出测试" || row[3] != `'=HYPERLINK("http://evil")` {
		t.Fatalf("CSV 行内容不对: %v", row)
	}
	pathCol := -1
	for i, k := range rows[0] {
		if k == "path" {
			pathCol = i
		}
	}
	if pathCol < 0 || row[pathCol] != "'=cmd|' /C calc'!A0" {
		t.Fatalf("字段列也应防公式注入: %v", row)
	}

	// 参数错误返回 400 与 JSON 错误信息，并按语言翻译
	for _, q := range []string{"&format=xlsx", "&level=fatal", "&status=abc"} {
		if code, _, body := getRaw(t, c, base+q, "en"); code != 400 || !bytes.Contains(body, []byte(`"error"`)) {
			t.Errorf("%s 应返回 400，得到 %d %s", q, code, body)
		}
	}
	if _, _, body := getRaw(t, c, base+"&format=xlsx", "en"); !bytes.Contains(body, []byte("Invalid format parameter")) {
		t.Errorf("错误信息应翻译: %s", body)
	}

	// 系统日志导出：英文界面得到译文，与日志页一致
	code, _, body = getRaw(t, c, srv.URL+"/api/logs/export?q="+url.QueryEscape("signed in from"), "en")
	if code != 200 || !bytes.Contains(body, []byte("signed in from")) {
		t.Fatalf("系统日志导出应按语言翻译 %d %s", code, body)
	}
	// 下载链接带不上界面语言的请求头，lang 参数优先于 Accept-Language
	code, _, body = getRaw(t, c, srv.URL+"/api/logs/export?lang=en&q="+url.QueryEscape("signed in from"), "zh-CN")
	if code != 200 || !bytes.Contains(body, []byte("signed in from")) {
		t.Fatalf("lang 参数应决定导出语言 %d %s", code, body)
	}
	// 无效的 lang 忽略，按 Accept-Language
	code, _, body = getRaw(t, c, srv.URL+"/api/logs/export?lang=xx&q="+url.QueryEscape("登录"), "zh-CN")
	if code != 200 || !bytes.Contains(body, []byte("登录")) {
		t.Fatalf("无效的 lang 应忽略 %d %s", code, body)
	}
}

func TestLogExportReadOnlyRedactsAndScopes(t *testing.T) {
	a, srv, admin := loginApp(t)
	pf := a.modules[0].logs
	pf.Logger("通知").With(logbuf.Fields{logbuf.FieldHost: "https://hooks.example.com/abc?token=s3cret"}).Infof("推送到 https://user:pa55@hooks.example.com/services/T0ABCDEF1234567890 失败")

	if code, body := doJSON(t, admin, "POST", srv.URL+"/api/account/users", map[string]string{"username": "viewer", "password": "tide-Lamp-47-oak"}); code != 200 {
		t.Fatalf("创建只读账号失败 %d %s", code, body)
	}
	jar, _ := cookiejar.New(nil)
	ro := &http.Client{Jar: jar}
	if code, body := doJSON(t, ro, "POST", srv.URL+"/api/login", map[string]string{"username": "viewer", "password": "tide-Lamp-47-oak"}); code != 200 {
		t.Fatalf("只读账号登录失败 %d %s", code, body)
	}
	u := srv.URL + "/api/portforward/logs/export?source=" + url.QueryEscape("通知")
	// 管理员看到原文
	_, _, body := getRaw(t, admin, u, "zh-CN")
	if !bytes.Contains(body, []byte("pa55")) || !bytes.Contains(body, []byte("s3cret")) {
		t.Fatalf("管理员导出应保留原文: %s", body)
	}
	// 只读账号可以导出（与查看日志同为读权限），凭据与查看日志接口一样脱敏
	code, _, body := getRaw(t, ro, u, "zh-CN")
	if code != 200 {
		t.Fatalf("只读账号应能导出 %d %s", code, body)
	}
	if bytes.Contains(body, []byte("pa55")) || bytes.Contains(body, []byte("s3cret")) || bytes.Contains(body, []byte("T0ABCDEF1234567890")) {
		t.Fatalf("只读账号导出的内容应脱敏: %s", body)
	}
	_, _, view := getRaw(t, ro, srv.URL+"/api/portforward/logs?source="+url.QueryEscape("通知"), "zh-CN")
	if bytes.Contains(view, []byte("pa55")) {
		t.Fatalf("对照：查看接口同样脱敏: %s", view)
	}
	// 脱敏不能改到内存中的原日志
	if es := pf.Query(0, 0, 0, &logbuf.Filter{Source: "通知"}); len(es) != 1 || !strings.Contains(es[0].Fields[logbuf.FieldHost], "s3cret") {
		t.Fatalf("原日志不应被修改: %+v", es)
	}
	// 只读账号不能清空日志
	if code, _ := doJSON(t, ro, "DELETE", srv.URL+"/api/portforward/logs", nil); code != 403 {
		t.Fatalf("只读账号清空日志应返回 403，得到 %d", code)
	}

	// API 令牌：模块读权限可以导出与列出来源，没有该模块权限时拒绝
	token := func(scopes ...string) string {
		_, body := doJSON(t, admin, "POST", srv.URL+"/api/tokens", map[string]any{"name": "导出测试", "scopes": scopes})
		var created struct {
			Secret string `json:"secret"`
		}
		_ = json.Unmarshal(body, &created)
		if created.Secret == "" {
			t.Fatalf("创建令牌失败: %s", body)
		}
		return created.Secret
	}
	call := func(secret, path string) int {
		req, _ := http.NewRequest("GET", srv.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+secret)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	readTok := token("portforward:read")
	otherTok := token("status:read")
	logsTok := token("logs:read")
	for _, cs := range []struct {
		tok, path string
		want      int
	}{
		{readTok, "/api/portforward/logs/export", 200},
		{readTok, "/api/portforward/logs/sources", 200},
		{otherTok, "/api/portforward/logs/export", 403},
		{otherTok, "/api/portforward/logs/sources", 403},
		{readTok, "/api/logs/export", 403},
		{logsTok, "/api/logs/export", 200},
		{logsTok, "/api/logs/sources", 200},
	} {
		if got := call(cs.tok, cs.path); got != cs.want {
			t.Errorf("%s: 状态码 %d，期望 %d", cs.path, got, cs.want)
		}
	}
}

func TestCSVCell(t *testing.T) {
	for in, want := range map[string]string{"": "", "abc": "abc", "=1+1": "'=1+1", "+1": "'+1", "-1": "'-1", "@x": "'@x", "\tx": "'\tx", "\rx": "'\rx", "a=b": "a=b"} {
		if got := csvCell(in); got != want {
			t.Errorf("csvCell(%q) = %q，期望 %q", in, got, want)
		}
	}
}
