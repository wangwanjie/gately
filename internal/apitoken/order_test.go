package apitoken

import (
	"strings"
	"testing"
	"time"
)

// names 返回列表中令牌名称，便于比较顺序
func names(list []View) string {
	out := make([]string, 0, len(list))
	for _, v := range list {
		out = append(out, v.Name)
	}
	return strings.Join(out, ",")
}

func TestTokenOrderAndGroup(t *testing.T) {
	m, store := newManager(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tick := 0
	m.now = func() time.Time {
		tick++
		return base.Add(time.Duration(tick) * time.Minute)
	}
	ids := map[string]string{}
	for _, n := range []string{"a", "b", "c"} {
		v, _, err := m.Create(Input{Name: n, Scopes: []string{"ddns:read"}})
		if err != nil {
			t.Fatal(err)
		}
		ids[n] = v.ID
	}
	// 未手动排序时按创建时间从新到旧
	if got := names(m.List()); got != "c,b,a" {
		t.Fatalf("默认顺序应为从新到旧，实际 %s", got)
	}
	if err := m.Order([]string{ids["b"], ids["a"]}); err == nil {
		t.Fatal("缺少条目的排序应被拒绝")
	}
	if err := m.Order([]string{ids["b"], ids["a"], ids["a"]}); err == nil {
		t.Fatal("重复条目的排序应被拒绝")
	}
	if err := m.Order([]string{ids["b"], ids["a"], ids["c"]}); err != nil {
		t.Fatal(err)
	}
	if got := names(m.List()); got != "b,a,c" {
		t.Fatalf("手动排序后应按保存顺序，实际 %s", got)
	}
	// 手动排序后新令牌排在最前
	v, _, err := m.Create(Input{Name: "d", Scopes: []string{"ddns:read"}})
	if err != nil {
		t.Fatal(err)
	}
	ids["d"] = v.ID
	if got := names(m.List()); got != "d,b,a,c" {
		t.Fatalf("新令牌应排在最前，实际 %s", got)
	}

	if err := m.SetGroup([]string{ids["a"], ids["c"]}, "  脚本 "); err != nil {
		t.Fatal(err)
	}
	if err := m.SetGroup([]string{ids["a"], "nope"}, "x"); err != ErrNotFound {
		t.Fatalf("未知令牌应返回 ErrNotFound，实际 %v", err)
	}
	if err := m.SetGroup([]string{ids["a"]}, strings.Repeat("组", 65)); err == nil {
		t.Fatal("过长的分组名称应被拒绝")
	}
	if err := m.SetGroup(nil, "x"); err == nil {
		t.Fatal("未选择条目应被拒绝")
	}
	groups := map[string]string{}
	for _, v := range m.List() {
		groups[v.Name] = v.Group
	}
	if groups["a"] != "脚本" || groups["c"] != "脚本" || groups["b"] != "" {
		t.Fatalf("分组设置有误: %v", groups)
	}

	// 修改时不提供分组保持原分组，提供时改为新分组
	if _, err := m.Update(ids["a"], Input{Name: "a2", Scopes: []string{"ddns:read"}}); err != nil {
		t.Fatal(err)
	}
	g := "运维"
	if _, err := m.Update(ids["c"], Input{Name: "c", Scopes: []string{"ddns:read"}, Group: &g}); err != nil {
		t.Fatal(err)
	}
	// 重新加载后顺序与分组保持
	m2, err := Open(store, []string{"ddns"})
	if err != nil {
		t.Fatal(err)
	}
	list := m2.List()
	if got := names(list); got != "d,b,a2,c" {
		t.Fatalf("重新加载后顺序应保持，实际 %s", got)
	}
	if list[2].Group != "脚本" || list[3].Group != "运维" {
		t.Fatalf("重新加载后分组应保持: %+v", list)
	}
}
