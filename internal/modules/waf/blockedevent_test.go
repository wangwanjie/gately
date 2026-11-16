package waf

import (
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/wangwanjie/gately/internal/events"
)

// WAF 拦截事件中的访客文本（域名、路径）会进入通知与日志：控制字符换成转义形式，过长的路径截断。
func TestBlockedEventSanitizesVisitorText(t *testing.T) {
	req := httptest.NewRequest("GET", "http://a.test/x%0A%0DInjected:%20yes%22%7D?q=1", nil)
	req.RemoteAddr = "203.0.113.7:4000"
	e := blockedEvent("w1", req)
	if e.Type != events.WAFBlocked || e.Source != "w1" || e.Fields["ip"] != "203.0.113.7:4000" || e.Fields["host"] != "a.test" {
		t.Fatalf("事件字段不对: %+v", e)
	}
	for _, s := range []string{e.Message, e.Fields["path"]} {
		if strings.ContainsAny(s, "\r\n") {
			t.Fatalf("事件文本不应含换行: %q", s)
		}
	}
	if e.Fields["path"] != `/x\n\rInjected: yes"}` || e.Message != `GET a.test/x\n\rInjected: yes"}` {
		t.Fatalf("控制字符应换成转义形式: %q %q", e.Fields["path"], e.Message)
	}
	long := httptest.NewRequest("GET", "http://a.test/"+strings.Repeat("长", 1000), nil)
	if p := blockedEvent("w1", long).Fields["path"]; utf8.RuneCountInString(p) > maxEventText+1 || !strings.HasSuffix(p, "…") {
		t.Fatalf("过长的路径应截断: %d", utf8.RuneCountInString(p))
	}
}
