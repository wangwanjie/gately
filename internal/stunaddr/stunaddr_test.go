package stunaddr

import (
	"net/netip"
	"testing"
)

func TestSetLookupDelete(t *testing.T) {
	a := netip.MustParseAddrPort("203.0.113.1:40000")
	b := netip.MustParseAddrPort("203.0.113.2:40001")
	Set("k2", "web", b)
	Set("k1", "web", a)
	Set("k3", "", a)
	defer Delete("k1")
	defer Delete("k2")
	defer Delete("k3")

	// 同名规则取 key 排序最小的一条
	if got, ok := Lookup("web"); !ok || got != a {
		t.Fatalf("同名规则应取 k1: %v %v", got, ok)
	}
	// 没有同名规则时按 key 查找
	if got, ok := Lookup("k3"); !ok || got != a {
		t.Fatalf("应能按 key 查找: %v %v", got, ok)
	}
	if _, ok := Lookup("none"); ok {
		t.Fatal("不存在的规则不应查到地址")
	}

	// 规则改名后按 key 覆盖，旧名字不再残留
	Set("k1", "web2", a)
	if got, _ := Lookup("web"); got != b {
		t.Fatalf("改名后 web 只剩 k2: %v", got)
	}
	Delete("k2")
	if _, ok := Lookup("web"); ok {
		t.Fatal("删除后不应再查到")
	}
	if got, ok := Lookup("web2"); !ok || got != a {
		t.Fatalf("新名字应可查到: %v %v", got, ok)
	}
}

func TestParseVarAndVars(t *testing.T) {
	cases := map[string][2]string{
		"STUN_web_PORT":   {"web", "PORT"},
		"STUN_my_rule_IP": {"my_rule", "IP"},
		"STUN_a_b_ADDR":   {"a_b", "ADDR"},
		"STUN_规则名_PORT":   {"规则名", "PORT"},
	}
	for name, want := range cases {
		rule, part, ok := ParseVar(name)
		if !ok || rule != want[0] || part != want[1] {
			t.Errorf("ParseVar(%q)=%q %q %v", name, rule, part, ok)
		}
	}
	for _, bad := range []string{"STUN__PORT", "STUN_web", "web_PORT", "ipv4Addr"} {
		if _, _, ok := ParseVar(bad); ok {
			t.Errorf("ParseVar(%q) 不应识别为 STUN 变量", bad)
		}
	}

	Set("kv", "web", netip.MustParseAddrPort("203.0.113.5:40000"))
	defer Delete("kv")
	vals, missing := Vars("{STUN_web_IP}:{STUN_web_PORT}", "{ipv4Addr} {STUN_web_ADDR} {STUN_gone_PORT} {STUN_gone_IP}")
	if vals["STUN_web_IP"] != "203.0.113.5" || vals["STUN_web_PORT"] != "40000" || vals["STUN_web_ADDR"] != "203.0.113.5:40000" || len(vals) != 3 {
		t.Fatalf("变量值错误: %v", vals)
	}
	if len(missing) != 1 || missing[0] != "gone" {
		t.Fatalf("缺失规则错误: %v", missing)
	}
	if !References("port={STUN_web_PORT}", "web") || References("{STUN_web2_PORT}", "web") {
		t.Fatal("References 判断错误")
	}
}

// JSON 正文里的变量与字面量花括号相邻时也要能取到
func TestVarsInJSONBody(t *testing.T) {
	Set("kj", "web", netip.MustParseAddrPort("203.0.113.6:40001"))
	defer Delete("kj")
	vals, missing := Vars(`{"port":"{STUN_web_PORT}","addr":"{STUN_web_ADDR}","x":{"y":"{STUN_none_IP}"}}`)
	if vals["STUN_web_PORT"] != "40001" || vals["STUN_web_ADDR"] != "203.0.113.6:40001" || len(vals) != 2 {
		t.Fatalf("JSON 正文中的变量值错误: %v", vals)
	}
	if len(missing) != 1 || missing[0] != "none" {
		t.Fatalf("JSON 正文中的缺失规则错误: %v", missing)
	}
	// 只有 } 或只有 { 的文本不应出错
	if vals, missing := Vars("}{", "{", "}"); len(vals) != 0 || len(missing) != 0 {
		t.Fatalf("不成对的花括号不应取到变量: %v %v", vals, missing)
	}
}
