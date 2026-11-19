package app

import (
	"encoding/json"
	"reflect"
	"testing"
)

// findSite：按 key 在站点列表中找；key 为空或等于默认站点的 key 时返回默认站点
func TestFindSite(t *testing.T) {
	var listener map[string]any
	if err := json.Unmarshal([]byte(`{
		"sites": [{"key": "s1", "name": "a"}, {"key": "s2", "name": "b"}],
		"default": {"key": "d", "name": "default"}
	}`), &listener); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		key  string
		name string
		ok   bool
	}{
		{"s2", "b", true},
		{"d", "default", true},
		{"", "default", true},
		{"missing", "", false},
	}
	for _, c := range cases {
		m, ok := findSite(listener, c.key)
		if ok != c.ok {
			t.Fatalf("findSite(%q) ok = %v，期望 %v", c.key, ok, c.ok)
		}
		if ok && m["name"] != c.name {
			t.Fatalf("findSite(%q) 找到 %v，期望名称 %q", c.key, m["name"], c.name)
		}
	}
	// 没有站点列表与默认站点的监听：任何 key 都找不到
	if _, ok := findSite(map[string]any{}, ""); ok {
		t.Fatal("空监听不应找到站点")
	}
}

// siteBatchRisks：批量停用、删除、移动、改域名涉及路径上的站点时报对应风险，其他站点与其他操作不报
func TestSiteBatchRisks(t *testing.T) {
	p := accessPath{Hops: []pathHop{
		{Kind: hopClient},
		{Kind: "webservice", Rule: "l1", Site: "s1"},
		{Kind: hopAdmin},
	}}
	idx := []int{1}
	body := func(action string, sites ...[2]string) json.RawMessage {
		var list []map[string]string
		for _, s := range sites {
			list = append(list, map[string]string{"listener": s[0], "site": s[1]})
		}
		b, _ := json.Marshal(map[string]any{"action": action, "sites": list})
		return b
	}
	onPath := [2]string{"l1", "s1"}
	other := [2]string{"l1", "s2"}

	cases := []struct {
		action string
		want   []pathRisk
	}{
		{"disable", []pathRisk{{Kind: riskDisable, Hop: 1}}},
		{"delete", []pathRisk{{Kind: riskDelete, Hop: 1}}},
		{"move", []pathRisk{{Kind: riskSiteMove, Hop: 1}}},
		{"removeDomains", []pathRisk{{Kind: riskEdit, Hop: 1, Fields: []string{"site.domains"}}}},
		{"replaceDomain", []pathRisk{{Kind: riskEdit, Hop: 1, Fields: []string{"site.domains"}}}},
		// 启用、加域名等不会切断当前访问
		{"enable", nil},
		{"addDomains", nil},
	}
	for _, c := range cases {
		got := siteBatchRisks(p, idx, body(c.action, other, onPath))
		if !reflect.DeepEqual(got, c.want) {
			t.Fatalf("%s：风险 = %+v，期望 %+v", c.action, got, c.want)
		}
	}
	// 只涉及路径外的站点
	if got := siteBatchRisks(p, idx, body("delete", other, [2]string{"l2", "s1"})); got != nil {
		t.Fatalf("路径外的站点不应报风险：%+v", got)
	}
	// 请求体格式错误时不拦保存
	if got := siteBatchRisks(p, idx, json.RawMessage(`{"sites": "x"}`)); got != nil {
		t.Fatalf("格式错误的请求体不应报风险：%+v", got)
	}
}
