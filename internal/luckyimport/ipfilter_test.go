package luckyimport

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/iplist"
)

func ipFilterDoc(t *testing.T) Document {
	t.Helper()
	now := time.Now().Unix()
	rules := []map[string]any{
		{"Name": "whitelist", "Key": "whitelist", "Action": 0, "SubRuleList": []map[string]any{
			{"Key": "a", "Enable": true, "Type": "txt", "LongTermValid": true, "IPTextSets": "192.168.1.0/24\n203.0.113.5 # 公司\n\n1.1.1.1-1.1.1.9"},
			{"Key": "b", "Enable": true, "Type": "txt", "LongTermValid": false, "ValidTimestamp": now + 3600, "IPTextSets": "2001:db8::/32"},
			{"Key": "c", "Enable": true, "Type": "txt", "LongTermValid": false, "ValidTimestamp": now - 3600, "IPTextSets": "198.51.100.1"},
			{"Key": "d", "Enable": false, "Type": "txt", "LongTermValid": true, "IPTextSets": "198.51.100.2"},
			{"Key": "e", "Enable": true, "Type": "ipdb", "LongTermValid": true},
		}},
		{"Name": "blacklist", "Key": "blacklist", "Action": 1, "SubRuleList": []map[string]any{
			{"Key": "f", "Enable": true, "Type": "txt", "LongTermValid": true, "IPTextSets": "192.0.2.66"},
		}},
		{"Name": "globalblacklist", "Key": "globalblacklist", "Action": 1, "SubRuleList": []map[string]any{
			{"Key": "g", "Enable": true, "Type": "txt", "LongTermValid": true, "IPTextSets": "192.0.2.66\n192.0.2.77"},
		}},
	}
	raw, _ := json.Marshal(rules)
	return Document{"RuleList": raw}
}

func TestIPFilterLists(t *testing.T) {
	if IPFilterLists([]Document{{"Other": json.RawMessage(`1`)}}, time.Now()) != nil {
		t.Fatal("没有 IP 过滤配置时应返回 nil")
	}
	lists := IPFilterLists([]Document{ipFilterDoc(t)}, time.Now())
	wl := lists["whitelist"]
	// 过期与停用的子规则不导入；IP 库子规则与地址范围计入 Skipped
	if got := strings.Join(wl.Entries, ","); got != "192.168.1.0/24,203.0.113.5,2001:db8::/32" || wl.Skipped != 2 {
		t.Fatalf("白名单 = %q skipped=%d", got, wl.Skipped)
	}
}

func TestSafeModeAccess(t *testing.T) {
	lists := IPFilterLists([]Document{ipFilterDoc(t)}, time.Now())

	a, warns := SafeModeAccess("whitelist", lists)
	if a.IPMode != iplist.ModeWhitelist || len(a.IPList) != 3 {
		t.Fatalf("白名单模式应导入白名单条目: %+v", a)
	}
	if err := a.Normalize(); err != nil {
		t.Fatalf("导入的名单应能通过校验: %v", err)
	}
	joined := strings.Join(warns, "\n")
	for _, want := range []string{"2 条", "全局黑名单的 2 条"} {
		if !strings.Contains(joined, want) {
			t.Errorf("缺少提示 %q:\n%s", want, joined)
		}
	}

	// 黑名单模式合并黑名单与全局黑名单并去重
	a, _ = SafeModeAccess("blacklist", lists)
	if a.IPMode != iplist.ModeBlacklist || strings.Join(a.IPList, ",") != "192.0.2.66,192.0.2.77" {
		t.Fatalf("黑名单模式结果错误: %+v", a)
	}

	// 没有 IP 过滤配置：白名单无法还原，给出提示；黑名单保持关闭
	if a, warns := SafeModeAccess("whitelist", nil); a.IPMode != "" || len(warns) != 1 || !strings.Contains(warns[0], "lucky_ipfilter") {
		t.Fatalf("缺少 IP 过滤配置时应提示: %+v %v", a, warns)
	}
	if a, warns := SafeModeAccess("blacklist", nil); a.IPMode != "" || len(warns) != 0 {
		t.Fatalf("黑名单为空时不应设置访问控制: %+v %v", a, warns)
	}
	// 白名单为空：不设置名单（否则会拒绝全部来源），并提示
	empty := map[string]IPFilterList{"whitelist": {}}
	if a, warns := SafeModeAccess("whitelist", empty); a.IPMode != "" || len(warns) != 1 {
		t.Fatalf("白名单为空时应提示: %+v %v", a, warns)
	}
}
