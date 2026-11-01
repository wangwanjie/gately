package luckyimport

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/iplist"
)

// IPFilterList 是 Lucky IP 过滤中一条规则（whitelist、blacklist、globalblacklist 或自定义规则）的固定条目。
type IPFilterList struct {
	// Action 为规则动作：0 放行（白名单语义），1 拒绝（黑名单语义）
	Action int
	// Entries 为可直接用于 Gately 来源 IP 访问控制的 IP / 网段
	Entries []string
	// Skipped 为无法导入的条目数：IP 库（ipdb）与自动记录类子规则、地址范围或无法识别的行
	Skipped int
}

// luckyIPFilterRule 对应 Lucky IP 过滤配置（lucky_ipfilter 的 RuleList）中的一条规则。
type luckyIPFilterRule struct {
	Key    string
	Action int
	// SubRuleList 中每项是一组 IP，Lucky 前端新建子规则的默认值为 {Type:"txt", LongTermValid:true, IPTextSets:""}
	SubRuleList []struct {
		Enable bool
		// Type 为 txt（手填 IP 文本）、ipdb / ipdb_all（IP 库）等，只有 txt 能按固定名单导入
		Type           string
		LongTermValid  bool
		ValidTimestamp int64
		IPTextSets     string
	}
}

// 内置的三条 IP 过滤规则 key，用于识别 IP 过滤配置文件
var builtinIPFilterKeys = map[string]bool{"whitelist": true, "blacklist": true, "globalblacklist": true}

// IPFilterLists 汇总本次上传的 Lucky IP 过滤配置，返回 规则 key -> 名单；没有上传 IP 过滤配置时返回 nil。
// 已停用或已过期的子规则不导入，也不计入 Skipped。
func IPFilterLists(docs []Document, now time.Time) map[string]IPFilterList {
	var out map[string]IPFilterList
	for _, doc := range docs {
		raw, ok := doc["RuleList"]
		if !ok {
			continue
		}
		var rules []luckyIPFilterRule
		if json.Unmarshal(raw, &rules) != nil {
			continue
		}
		// RuleList 是通用字段名，只有含内置名单规则的才是 IP 过滤配置
		isFilter := false
		for _, r := range rules {
			if builtinIPFilterKeys[r.Key] {
				isFilter = true
			}
		}
		if !isFilter {
			continue
		}
		if out == nil {
			out = map[string]IPFilterList{}
		}
		for _, r := range rules {
			l := out[r.Key]
			l.Action = r.Action
			for _, sub := range r.SubRuleList {
				if !sub.Enable || (!sub.LongTermValid && sub.ValidTimestamp < now.Unix()) {
					continue
				}
				if sub.Type != "txt" {
					l.Skipped++
					continue
				}
				entries, skipped := parseIPText(sub.IPTextSets)
				l.Entries = append(l.Entries, entries...)
				l.Skipped += skipped
			}
			out[r.Key] = l
		}
	}
	return out
}

// parseIPText 解析 Lucky 手填的 IP 文本：每行一个 IP 或网段，# 之后为注释；地址范围与无法识别的行计入 skipped。
func parseIPText(text string) (entries []string, skipped int) {
	for _, line := range strings.Split(text, "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if _, err := iplist.ParsePrefixes(fields[:1]); err != nil {
			skipped++
			continue
		}
		entries = append(entries, fields[0])
	}
	return entries, skipped
}

// SafeModeAccess 把 Lucky 端口转发 / STUN 规则的安全模式（Options.SafeMode）转换为来源 IP 访问控制。
// SafeMode 是 Lucky IP 过滤规则的 key：disable 表示不过滤；whitelist、blacklist、globalblacklist 为内置规则，
// 也可以是用户自建规则的 key。规则动作为放行时按白名单导入，为拒绝时按黑名单导入，拒绝规则另外合并全局黑名单。
// lists 为 IPFilterLists 的结果，为 nil 表示本次导入没有 IP 过滤配置。返回的提示不带规则名，由调用方加前缀。
func SafeModeAccess(mode string, lists map[string]IPFilterList) (iplist.Access, []string) {
	if mode == "" || mode == "disable" {
		return iplist.Access{}, nil
	}
	l, found := lists[mode]
	// 内置规则的动作固定；自建规则以配置中的动作为准
	accept := mode == "whitelist" || (found && !builtinIPFilterKeys[mode] && l.Action == 0)
	var warns []string
	if !found && !builtinIPFilterKeys[mode] {
		// 引用的自建规则不在本次导入中，无法判断是白名单还是黑名单
		if lists == nil {
			return iplist.Access{}, []string{fmt.Sprintf("原规则使用 IP 过滤规则 %q，但本次导入没有 Lucky 的 IP 过滤配置（lucky_ipfilter），未设置来源 IP 名单，请注意暴露面", mode)}
		}
		return iplist.Access{}, []string{fmt.Sprintf("原规则使用的 IP 过滤规则 %q 不在 Lucky 的 IP 过滤配置中，未设置来源 IP 名单，请注意暴露面", mode)}
	}
	if accept {
		if lists == nil {
			return iplist.Access{}, []string{"原规则启用了白名单模式，但本次导入没有 Lucky 的 IP 过滤配置（lucky_ipfilter），未设置来源 IP 名单，请注意暴露面"}
		}
		if l.Skipped > 0 {
			warns = append(warns, fmt.Sprintf("Lucky 白名单中有 %d 条 IP 库、自动记录或地址范围条目无法导入", l.Skipped))
		}
		if gb := lists["globalblacklist"]; len(gb.Entries) > 0 {
			warns = append(warns, fmt.Sprintf("Lucky 全局黑名单的 %d 条未导入到白名单规则，可在入侵防护中封禁", len(gb.Entries)))
		}
		if len(l.Entries) == 0 {
			return iplist.Access{}, append(warns, "原规则启用了白名单模式，但 Lucky 白名单中没有可导入的 IP / 网段，未设置来源 IP 名单，请注意暴露面")
		}
		return iplist.Access{IPMode: iplist.ModeWhitelist, IPList: dedupe(l.Entries)}, warns
	}
	// 拒绝规则：本规则与全局黑名单合并
	gb := lists["globalblacklist"]
	skipped, entries := l.Skipped, append([]string{}, l.Entries...)
	if mode != "globalblacklist" {
		skipped += gb.Skipped
		entries = append(entries, gb.Entries...)
	}
	if skipped > 0 {
		warns = append(warns, fmt.Sprintf("Lucky 黑名单中有 %d 条 IP 库、自动记录或地址范围条目无法导入", skipped))
	}
	entries = dedupe(entries)
	if len(entries) == 0 {
		return iplist.Access{}, warns
	}
	return iplist.Access{IPMode: iplist.ModeBlacklist, IPList: entries}, warns
}

// dedupe 去掉重复项并保持原顺序。
func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
