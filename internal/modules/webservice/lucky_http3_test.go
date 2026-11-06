package webservice

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/luckyimport"
)

// Lucky 的 HTTP/3 只在开启 HTTPS 时沿用；未开 HTTPS 时提示后忽略，导入的监听能通过校验。
func TestConvertLuckyHTTP3WithoutTLS(t *testing.T) {
	raw, _ := json.Marshal([]map[string]any{
		{"RuleName": "plain", "Enable": true, "ListenPort": 8080, "Network": "tcp", "Http3": true,
			"DefaultProxy": map[string]any{"Locations": []string{"http://192.168.1.8:80"}, "WebServiceType": "reverseproxy"}},
	})
	ls, warns, _, err := convertLuckyListeners(raw, luckyimport.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(ls) != 1 || ls[0].HTTP3 {
		t.Fatalf("listeners = %+v", ls)
	}
	if !strings.Contains(strings.Join(warns, "\n"), "HTTP/3 需要开启 HTTPS") {
		t.Fatalf("warns = %v", warns)
	}
	if err := ls[0].Validate(); err != nil {
		t.Fatalf("导入的监听应能通过校验: %v", err)
	}
}
