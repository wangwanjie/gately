package webpage

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// initParsed 记录包初始化完成、任何测试运行之前已经解析过的模板名称
var initParsed []string

func TestMain(m *testing.M) {
	for name, p := range pages {
		if p.t != nil {
			initParsed = append(initParsed, name)
		}
	}
	for _, s := range styleList {
		if s.tpl.t != nil {
			initParsed = append(initParsed, s.id)
		}
	}
	if panelTpl.t != nil {
		initParsed = append(initParsed, "panel")
	}
	os.Exit(m.Run())
}

// 启动时不解析任何页面模板，首次使用时才解析；全部模板都能解析。
func TestTemplatesParse(t *testing.T) {
	if len(initParsed) > 0 {
		t.Fatalf("启动时不应解析模板: %v", initParsed)
	}
	if err := CheckTemplates(); err != nil {
		t.Fatal(err)
	}
	// 解析后照常渲染：默认风格、内置风格
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Accept", "text/html")
	if b := string(RenderError(r, Problem{Code: 502, Title: "无法连接到后端服务"}, "")); !strings.Contains(b, "502") {
		t.Fatal("默认风格渲染失败")
	}
	for _, id := range StyleIDs() {
		if b := string(RenderError(r, Problem{Code: 404, Title: "页面不存在"}, id)); !strings.Contains(b, "404") {
			t.Fatalf("风格 %s 渲染失败", id)
		}
	}
}

// 模板有语法错误时 CheckTemplates 报出模板名称，渲染时 panic（与原先在初始化时 panic 一致）。
func TestLazyTemplateError(t *testing.T) {
	l := newLazyTemplate("bad", layoutSrc, `{{define "body"}}{{if}}{{end}}`)
	if _, err := l.parse(); err == nil || !strings.Contains(err.Error(), "bad") {
		t.Fatalf("应报出模板错误: %v", err)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("解析失败时 get 应 panic")
		}
	}()
	l.get()
}
