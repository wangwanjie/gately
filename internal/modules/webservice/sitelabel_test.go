package webservice

import (
	"strings"
	"testing"
)

// 站点既没有名称也没有域名时，错误按序号称呼站点，不出现“站点 : ”的空名前缀。
func TestSiteErrorLabelWithoutName(t *testing.T) {
	l := Listener{Port: 8080, Sites: []Site{
		{Enable: true, Domains: []string{"a.example.com"}, Type: TypeText, Text: "ok"},
		{Enable: true, Type: TypeProxy, Targets: []string{"http://127.0.0.1:1"}},
	}}
	err := l.Validate()
	if err == nil {
		t.Fatal("缺少域名应报错")
	}
	if strings.HasPrefix(err.Error(), "站点 :") || !strings.HasPrefix(err.Error(), "第 2 个站点: ") {
		t.Fatalf("错误前缀 = %q", err.Error())
	}
}
