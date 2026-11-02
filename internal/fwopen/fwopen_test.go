package fwopen

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeBackend 在内存中模拟若干条链。
type fakeBackend struct {
	chains []target
	rules  map[string][]existing
	addErr error
	next   int
}

func (f *fakeBackend) targets(context.Context) ([]target, map[string][]existing, error) {
	have := map[string][]existing{}
	for k, v := range f.rules {
		have[k] = append([]existing{}, v...)
	}
	return append([]target{}, f.chains...), have, nil
}

func (f *fakeBackend) add(_ context.Context, t target, c string, _ Port) error {
	if f.addErr != nil {
		return f.addErr
	}
	f.next++
	f.rules[t.String()] = append(f.rules[t.String()], existing{Comment: c, Ref: string(rune('a' + f.next))})
	return nil
}

func (f *fakeBackend) del(_ context.Context, t target, e existing) error {
	list := f.rules[t.String()]
	for i, r := range list {
		if r.Ref == e.Ref {
			f.rules[t.String()] = append(list[:i], list[i+1:]...)
			return nil
		}
	}
	return errors.New("not found")
}

func comments(f *fakeBackend, key string) []string {
	var out []string
	for _, r := range f.rules[key] {
		out = append(out, r.Comment)
	}
	return out
}

func newTestManager(be backend) *manager {
	// looping 置为 true，测试中不启动定时对账
	return &manager{be: be, owners: map[string]*ownerState{}, looping: true}
}

func TestReconcileAddRemove(t *testing.T) {
	fw4 := target{Kind: "nft", Family: "inet", Table: "fw4", Chain: "input", Policy: "accept", Busy: true}
	idle := target{Kind: "nft", Family: "ip", Table: "other", Chain: "input", Policy: "accept"}
	f := &fakeBackend{chains: []target{fw4, idle}, rules: map[string][]existing{
		// 上次运行遗留、已不再需要的规则
		fw4.String(): {{Comment: comment("stun:old", Port{"tcp", 1000}), Ref: "z"}},
	}}
	m := newTestManager(f)

	status, _ := m.set("stun:k1", []Port{{"tcp", 34811}, {"TCP", 34811}, {"udp", 0}}, nil)
	if !strings.Contains(status, "TCP 34811") || !strings.Contains(status, "nftables inet fw4 input") {
		t.Fatalf("status = %q", status)
	}
	got := comments(f, fw4.String())
	if len(got) != 1 || got[0] != ownPrefix()+"stun:k1:tcp:34811" {
		t.Fatalf("fw4 rules = %v", got)
	}
	// 默认放行且没有其他规则的链不需要插入
	if len(f.rules[idle.String()]) != 0 {
		t.Fatalf("idle chain should stay empty: %v", f.rules[idle.String()])
	}

	// 再次对账不重复插入
	m.set("stun:k1", []Port{{"tcp", 34811}}, nil)
	if n := len(f.rules[fw4.String()]); n != 1 {
		t.Fatalf("duplicate rules: %d", n)
	}

	// 规则被外部清除（防火墙重载）后补回
	f.rules[fw4.String()] = nil
	m.reconcile(context.Background(), true)
	if n := len(f.rules[fw4.String()]); n != 1 {
		t.Fatalf("not restored: %d", n)
	}

	m.clear("stun:k1")
	if n := len(f.rules[fw4.String()]); n != 0 {
		t.Fatalf("rules left after clear: %v", f.rules[fw4.String()])
	}
	if m.status("stun:k1") != "" {
		t.Fatal("status should be empty after clear")
	}
}

func TestReconcileFailureAndNoFirewall(t *testing.T) {
	f := &fakeBackend{rules: map[string][]existing{}}
	m := newTestManager(f)
	if s, _ := m.set("a", []Port{{"udp", 5000}}, nil); !strings.Contains(s, "无需放行") {
		t.Fatalf("status = %q", s)
	}
	f.chains = []target{{Kind: "iptables", Family: "ip", Table: "filter", Chain: "INPUT", Policy: "drop"}}
	f.addErr = errors.New("boom")
	s, ok := m.set("a", []Port{{"udp", 5000}}, nil)
	if ok {
		t.Fatal("should fail")
	}
	if !strings.Contains(s, "失败") || !strings.Contains(s, "boom") {
		t.Fatalf("status = %q", s)
	}
}

// 同一主机上其他 Gately 实例（不同配置目录）的规则不会被本实例删除。
func TestReconcileKeepsOtherInstances(t *testing.T) {
	fw4 := target{Kind: "nft", Family: "inet", Table: "fw4", Chain: "input", Policy: "drop"}
	SetInstance("/tmp/other-gately")
	foreign := comment("stun:x", Port{"udp", 2000})
	SetInstance("/tmp/this-gately")
	defer func() {
		instMu.Lock()
		instance = "default"
		instMu.Unlock()
	}()
	if foreign == comment("stun:x", Port{"udp", 2000}) {
		t.Fatal("instances should produce different comments")
	}
	if _, ok := ownerOf(foreign); ok {
		t.Fatal("foreign comment must not belong to this instance")
	}
	f := &fakeBackend{chains: []target{fw4}, rules: map[string][]existing{
		fw4.String(): {{Comment: foreign, Ref: "f"}},
	}}
	m := newTestManager(f)
	m.set("stun:k", []Port{{"tcp", 3000}}, nil)
	m.clear("stun:k")
	// Sweep 式对账（没有登记的端口）也不删除其他实例的规则
	m.reconcile(context.Background(), false)
	got := comments(f, fw4.String())
	if len(got) != 1 || got[0] != foreign {
		t.Fatalf("rules = %v", got)
	}
}

func TestOwnerOf(t *testing.T) {
	if o, ok := ownerOf(comment("stun:abc", Port{"udp", 9})); !ok || o != "stun:abc" {
		t.Fatalf("got %q %v", o, ok)
	}
	if _, ok := ownerOf("!fw4: something"); ok {
		t.Fatal("foreign comment matched")
	}
}

func TestParseNFT(t *testing.T) {
	chains := `{"nftables":[{"metainfo":{"version":"1.0.6"}},
	{"chain":{"family":"inet","table":"fw4","name":"input","handle":1,"type":"filter","hook":"input","prio":0,"policy":"accept"}},
	{"chain":{"family":"inet","table":"fw4","name":"input_wan","handle":2}},
	{"chain":{"family":"inet","table":"gately","name":"input","type":"filter","hook":"input","prio":-10,"policy":"accept"}},
	{"chain":{"family":"ip6","table":"x","name":"input","type":"filter","hook":"input","policy":"drop"}},
	{"chain":{"family":"inet","table":"firewalld","name":"filter_INPUT","type":"filter","hook":"input","policy":"accept"}},
	{"chain":{"family":"ip","table":"nat","name":"PREROUTING","type":"nat","hook":"prerouting","policy":"accept"}}]}`
	list, err := parseNFTChains([]byte(chains))
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Table != "fw4" || list[1].Chain != "filter_INPUT" {
		t.Fatalf("chains = %+v", list)
	}
	rules := `{"nftables":[{"metainfo":{}},
	{"rule":{"family":"inet","table":"fw4","chain":"input","handle":7,"comment":"gately-open:stun:k:tcp:1","expr":[]}},
	{"rule":{"family":"inet","table":"fw4","chain":"input","handle":8,"comment":"!fw4: Handle wan IPv4/IPv6 input traffic","expr":[]}}]}`
	busy, got, err := parseNFTRules([]byte(rules))
	if err != nil || !busy || len(got) != 1 || got[0].Ref != "7" {
		t.Fatalf("busy=%v got=%+v err=%v", busy, got, err)
	}
}

func TestParseIPTables(t *testing.T) {
	out := `-P INPUT DROP
-A INPUT -p tcp -m tcp --dport 34811 -m comment --comment "gately-open:stun:k:tcp:34811" -j ACCEPT
-A INPUT -i lo -j ACCEPT
`
	tg, rules := parseIPTables(out)
	if tg.Policy != "drop" || !tg.Busy || len(rules) != 1 {
		t.Fatalf("target=%+v rules=%+v", tg, rules)
	}
	want := "-p tcp -m tcp --dport 34811 -m comment --comment gately-open:stun:k:tcp:34811 -j ACCEPT"
	if rules[0].Ref != want {
		t.Fatalf("ref = %q", rules[0].Ref)
	}
}
