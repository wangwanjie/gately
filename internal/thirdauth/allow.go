package thirdauth

import (
	"fmt"
	"strings"

	"github.com/wangwanjie/gately/internal/config"
)

// ParseEntry 解析允许列表中的一项：提供方key:标识，行尾可用 " #" 追加备注。
func ParseEntry(s string) (provider, value string, ok bool) {
	if i := strings.Index(s, " #"); i >= 0 {
		s = s[:i]
	}
	provider, value, ok = strings.Cut(strings.TrimSpace(s), ":")
	provider, value = strings.TrimSpace(provider), strings.TrimSpace(value)
	return provider, value, ok && provider != "" && value != ""
}

// ValidateEntries 校验允许列表的格式。
func ValidateEntries(entries []string) error {
	for _, e := range entries {
		if _, _, ok := ParseEntry(e); !ok {
			return fmt.Errorf("第三方身份格式应为 提供方:标识，实际 %q", e)
		}
	}
	return nil
}

// Allowed 判断身份是否在允许列表中。列表项的标识可匹配主标识或任一别名（不区分大小写），
// 提供方key:* 表示该提供方的所有用户。
func Allowed(entries []string, id Identity) bool {
	return allowed(entries, id, true)
}

// AllowedExplicit 判断身份是否被允许列表逐个列出：不计 提供方key:* 通配。
// 通配允许的是任何能在该提供方注册账号的人，不能据此认定访客可信（例如跳过 WAF）。
func AllowedExplicit(entries []string, id Identity) bool {
	return allowed(entries, id, false)
}

func allowed(entries []string, id Identity, wildcard bool) bool {
	for _, e := range entries {
		p, v, ok := ParseEntry(e)
		if !ok || !strings.EqualFold(p, id.Provider) {
			continue
		}
		if (wildcard && v == "*") || strings.EqualFold(v, id.Primary) {
			return true
		}
		for _, a := range id.Aliases {
			if strings.EqualFold(v, a) {
				return true
			}
		}
	}
	return false
}

// Providers 返回允许列表中出现、且已配置的提供方（保持列表顺序、去重）。
func Providers(entries []string, configured map[string]Provider) []Provider {
	seen := map[string]bool{}
	var out []Provider
	for _, e := range entries {
		p, _, ok := ParseEntry(e)
		key := strings.ToLower(p)
		if prov, found := configured[key]; ok && found && !seen[key] {
			seen[key] = true
			out = append(out, prov)
		}
	}
	return out
}

// ---- Lucky 身份表 ----

// LuckyIdentity 是 Lucky 第三方登录管理中登记的一个身份。
type LuckyIdentity struct {
	Key    string `json:"Key"`
	Enable bool   `json:"Enable"`
	Type   string `json:"Type"`
	ID     string `json:"ID"`
	Name   string `json:"Name"`
	EMail  string `json:"EMail"`
}

// Entry 转换为 Gately 允许列表项。优先使用稳定 ID（GitHub 数字 ID、Google sub、QQ openid），
// 用户名与邮箱写在备注里便于辨认。
func (i LuckyIdentity) Entry() string {
	remark := strings.TrimSpace(strings.Join(nonEmpty(i.Name, i.EMail), " "))
	e := strings.ToLower(i.Type) + ":" + i.ID
	if remark != "" {
		e += " # " + remark
	}
	return e
}

func nonEmpty(s ...string) []string {
	var out []string
	for _, v := range s {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// luckyIdentitiesFile 保存导入的 Lucky 身份表，供之后导入站点与基础设置时按 Key 解析。
const luckyIdentitiesFile = "lucky_identities"

// LuckyIdentities 是持久化的 Lucky 身份表。
type LuckyIdentities struct {
	List []LuckyIdentity `json:"list"`
}

// LoadLuckyIdentities 读取已导入的身份表（不存在时为空）。
func LoadLuckyIdentities(store *config.Store) (LuckyIdentities, error) {
	var ids LuckyIdentities
	_, err := store.Load(luckyIdentitiesFile, &ids)
	return ids, err
}

// Save 写入身份表。
func (ids LuckyIdentities) Save(store *config.Store) error {
	return store.Save(luckyIdentitiesFile, ids)
}

// Resolve 把 Lucky 的身份 Key 列表（all 为真时为全部启用的身份）解析为允许列表项，
// missing 返回在身份表里找不到的 Key 数量。
func (ids LuckyIdentities) Resolve(keys []string, all bool) (entries []string, missing int) {
	byKey := map[string]LuckyIdentity{}
	for _, i := range ids.List {
		byKey[i.Key] = i
	}
	if all {
		for _, i := range ids.List {
			if i.Enable {
				entries = append(entries, i.Entry())
			}
		}
		return entries, 0
	}
	for _, k := range keys {
		i, ok := byKey[k]
		if !ok {
			missing++
			continue
		}
		if i.Enable {
			entries = append(entries, i.Entry())
		}
	}
	return entries, missing
}
