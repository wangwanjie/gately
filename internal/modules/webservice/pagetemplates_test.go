package webservice

import (
	"testing"

	"github.com/wangwanjie/gately/internal/webpage"
)

// 登录、改密、Passkey、放行入口等页面模板在首次使用时才解析，这里确认它们都能解析。
func TestPageTemplatesParse(t *testing.T) {
	if err := webpage.CheckTemplates(); err != nil {
		t.Fatal(err)
	}
}
