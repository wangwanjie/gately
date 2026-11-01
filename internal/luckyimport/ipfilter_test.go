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

func TestSafeModeAccessKeys(t *testing.T) {
	lists := IPFilterLists([]Document{ipFilterDoc(t)}, time.Now())
	// 关闭（disable）与空值：即使名单有内容也不设置访问控制
	for _, mode := range []string{"disable", ""} {
		if a, warns := SafeModeAccess(mode, lists); a.IPMode != "" || len(warns) != 0 {
			t.Errorf("%q 应不设置访问控制: %+v %v", mode, a, warns)
		}
	}
	// 直接选择全局黑名单
	if a, _ := SafeModeAccess("globalblacklist", lists); a.IPMode != iplist.ModeBlacklist || strings.Join(a.IPList, ",") != "192.0.2.66,192.0.2.77" {
		t.Errorf("全局黑名单结果错误: %+v", a)
	}
	// 自建规则按动作决定白名单或黑名单
	lists["office"] = IPFilterList{Action: 0, Entries: []string{"10.1.0.0/16"}}
	lists["bad"] = IPFilterList{Action: 1, Entries: []string{"10.9.9.9"}}
	if a, _ := SafeModeAccess("office", lists); a.IPMode != iplist.ModeWhitelist || a.IPList[0] != "10.1.0.0/16" {
		t.Errorf("放行规则应导入为白名单: %+v", a)
	}
	if a, _ := SafeModeAccess("bad", lists); a.IPMode != iplist.ModeBlacklist || strings.Join(a.IPList, ",") != "10.9.9.9,192.0.2.66,192.0.2.77" {
		t.Errorf("拒绝规则应导入为黑名单并合并全局黑名单: %+v", a)
	}
	// 引用不存在的自建规则：不设置名单并提示
	if a, warns := SafeModeAccess("missing", lists); a.IPMode != "" || len(warns) != 1 || !strings.Contains(warns[0], "missing") {
		t.Errorf("引用不存在的规则应提示: %+v %v", a, warns)
	}
	if a, warns := SafeModeAccess("missing", nil); a.IPMode != "" || len(warns) != 1 || !strings.Contains(warns[0], "lucky_ipfilter") {
		t.Errorf("没有 IP 过滤配置时引用自建规则应提示: %+v %v", a, warns)
	}
	// 其它配置文件里的 RuleList 不当作 IP 过滤配置
	other := Document{"RuleList": json.RawMessage(`[{"Key":"abc","Action":0}]`)}
	if IPFilterLists([]Document{other}, time.Now()) != nil {
		t.Error("不含内置名单规则的 RuleList 不应识别为 IP 过滤配置")
	}
}
