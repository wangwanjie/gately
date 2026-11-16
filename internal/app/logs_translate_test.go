package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/i18n"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// 日志接口按请求语言返回译文：带格式串的日志按格式串翻译（参数里含有格式串的固定片段也不会切错），
// 整句写入的日志整句翻译，字段值同样翻译；缓冲里的日志（包括与它共用的字段表）不被改动，响应里不出现格式串。
func TestLogsAPITranslatesWithFormat(t *testing.T) {
	a, srv, c := loginApp(t)
	pf := a.modules[0].logs
	pf.Clear()
	const format = "日志条数不能超过 %d"
	plain := "日志条数不能超过 20000"
	want := i18n.Tr("en", plain)
	if want == plain {
		t.Fatalf("测试用的文案 %q 已不在英文译文表里，请换一条", format)
	}
	fields := logbuf.Fields{logbuf.FieldIP: "10.0.0.5", logbuf.FieldRule: plain}
	l := pf.Logger("规则A")
	l.With(fields).Warnf(format, 20000)
	pf.Add(logbuf.LevelInfo, "规则A", plain)
	// 第一个参数里含有格式串的固定片段“: ”，第二个参数是另一条文案
	l.Errorf("%s: %v", "a: b", plain)
	l.Infof("10.0.0.5 GET a.example.com/x 404 1ms")

	get := func(query, lang string) (string, []logbuf.Entry) {
		t.Helper()
		req, _ := http.NewRequest("GET", srv.URL+"/api/portforward/logs?"+query, nil)
		req.Header.Set("Accept-Language", lang)
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: %d %s", query, resp.StatusCode, raw)
		}
		var out []logbuf.Entry
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("响应不是日志列表: %s", raw)
		}
		return string(raw), out
	}
	raw, es := get("", "en")
	if len(es) != 4 {
		t.Fatalf("应返回 4 条: %s", raw)
	}
	if es[0].Message != want || es[1].Message != want {
		t.Errorf("日志内容未翻译: %q / %q，应为 %q", es[0].Message, es[1].Message, want)
	}
	if es[2].Message != "a: b: "+want {
		t.Errorf("参数含固定片段的日志翻译不对: %q", es[2].Message)
	}
	if es[3].Message != "10.0.0.5 GET a.example.com/x 404 1ms" {
		t.Errorf("不含中文的日志应保持原样: %q", es[3].Message)
	}
	if es[0].Fields[logbuf.FieldRule] != want || es[0].Fields[logbuf.FieldIP] != "10.0.0.5" {
		t.Errorf("字段值的翻译不对: %v", es[0].Fields)
	}
	if strings.Contains(raw, "Format") || strings.Contains(raw, "Args") || strings.Contains(raw, "%d") {
		t.Errorf("响应里不应出现格式串: %s", raw)
	}
	// 缓冲里的日志与字段表保持原文
	if mem := pf.Since(0); mem[0].Message != plain || mem[0].Fields[logbuf.FieldRule] != plain || fields[logbuf.FieldRule] != plain {
		t.Errorf("缓冲里的日志被改写: %+v", mem[0])
	}
	// 简体中文请求返回原文
	if _, zh := get("", "zh-CN"); zh[0].Message != plain || zh[0].Fields[logbuf.FieldRule] != plain {
		t.Errorf("简体中文请求应返回原文: %+v", zh[0])
	}
	// 关键词按译文匹配（用户在界面上看到并输入的是译文），也仍能按原文匹配
	word := strings.ToLower(strings.Fields(want)[0])
	if _, hit := get("q="+word, "en"); len(hit) != 3 {
		t.Errorf("按译文关键词 %q 应匹配 3 条，得到 %d 条", word, len(hit))
	}
	if _, hit := get("q=404", "en"); len(hit) != 1 {
		t.Errorf("按原文关键词应匹配 1 条，得到 %d 条", len(hit))
	}
	// 参数错误的响应仍由中间件翻译
	req, _ := http.NewRequest("GET", srv.URL+"/api/portforward/logs?since=x", nil)
	req.Header.Set("Accept-Language", "en")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest || strings.Contains(string(body), "参数无效") {
		t.Errorf("错误响应应翻译: %d %s", resp.StatusCode, body)
	}
}

// 只读身份的日志响应要先脱敏再翻译：处理器不自行翻译，仍交给后面的脱敏与翻译中间件，凭据不会因为翻译顺序变化而漏出。
func TestLogsAPIReadOnlyStillRedactedAndTranslated(t *testing.T) {
	a, _, _ := loginApp(t)
	pf := a.modules[0].logs
	pf.Clear()
	plain := "日志条数不能超过 20000"
	want := i18n.Tr("en", plain)
	pf.Logger("r").Warnf("日志条数不能超过 %d", 20000)
	pf.Logger("r").Errorf("请求 %s 失败", "https://user:hunter2secret@example.com/hook")
	h := i18n.Middleware(a.redactReadOnly(a.logsHandler(pf)))
	serve := func(write bool) string {
		req := httptest.NewRequest("GET", "/api/portforward/logs", nil)
		req.Header.Set("Accept-Language", "en")
		req = req.WithContext(auth.WithScopeCheck(req.Context(), func(need string) bool {
			return write || strings.HasSuffix(need, ":read")
		}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Body.String()
	}
	ro := serve(false)
	if strings.Contains(ro, "hunter2secret") {
		t.Errorf("只读身份的日志里出现了凭据: %s", ro)
	}
	if !strings.Contains(ro, want) || strings.Contains(ro, plain) {
		t.Errorf("只读身份的日志应翻译: %s", ro)
	}
	rw := serve(true)
	if !strings.Contains(rw, "hunter2secret") || !strings.Contains(rw, want) || strings.Contains(rw, plain) {
		t.Errorf("有写权限的身份应看到翻译后的完整日志: %s", rw)
	}
}
