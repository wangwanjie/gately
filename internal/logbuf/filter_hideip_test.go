package logbuf

import "testing"

// TestFilterHideIP 验证 HideIP 只对带 ip 字段的日志调用，且与站点条件同时生效。
func TestFilterHideIP(t *testing.T) {
	var calls []string
	f := &Filter{HideIP: func(ip string) bool {
		calls = append(calls, ip)
		return ip == "1.2.3.4"
	}}
	if f.Empty() {
		t.Fatal("只有 HideIP 时不应视为空")
	}
	hidden := Entry{Level: LevelInfo, Message: "a", Fields: Fields{FieldIP: "1.2.3.4", FieldSite: "博客"}}
	shown := Entry{Level: LevelInfo, Message: "b", Fields: Fields{FieldIP: "5.6.7.8", FieldSite: "博客"}}
	noIP := Entry{Level: LevelInfo, Message: "1.2.3.4 出现在消息中"}
	if f.Match(&hidden) || !f.Match(&shown) || !f.Match(&noIP) {
		t.Fatal("HideIP 只应隐藏 ip 字段命中的日志，消息中出现的地址不受影响")
	}
	if len(calls) != 2 {
		t.Fatalf("没有 ip 字段的日志不应调用 HideIP，实际调用 %v", calls)
	}
	f.Site = "其他"
	if f.Match(&shown) {
		t.Fatal("站点条件不符时不应匹配")
	}
}
