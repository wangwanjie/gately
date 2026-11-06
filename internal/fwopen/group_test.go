package fwopen

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// waitIdle 等待 Group 的后台协程处理完全部待办。
func waitIdle(t *testing.T, g *Group) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		g.mu.Lock()
		idle := !g.running && len(g.pending) == 0
		g.mu.Unlock()
		if idle {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("Group 未在 3 秒内处理完")
}

func TestGroupWantAndClear(t *testing.T) {
	fw4 := target{Kind: "nft", Family: "inet", Table: "fw4", Chain: "input", Busy: true}
	f := &fakeBackend{chains: []target{fw4}, rules: map[string][]existing{}}
	g := NewGroup("web")
	g.m = newTestManager(f)

	g.Want("a", []Port{{Proto: "tcp", Port: 50663, Family: "6"}}, nil)
	g.Want("b", []Port{{Proto: "tcp", Port: 443}}, nil)
	waitIdle(t, g)
	if n := len(f.rules[fw4.String()]); n != 2 {
		t.Fatalf("rules = %+v", f.rules)
	}
	if s := g.Status("a"); !strings.Contains(s, "TCP 50663 (IPv6)") {
		t.Fatalf("status = %q", s)
	}

	// 改端口：旧端口删除、新端口放行
	g.Want("a", []Port{{Proto: "tcp", Port: 50664, Family: "6"}}, nil)
	waitIdle(t, g)
	got := strings.Join(comments(f, fw4.String()), ",")
	if strings.Contains(got, "50663") || !strings.Contains(got, "tcp6-50664") {
		t.Fatalf("rules = %s", got)
	}

	// 撤销后状态为空，规则删除；未登记过的 key 撤销不产生待办
	g.Want("a", nil, nil)
	g.Want("never", nil, nil)
	waitIdle(t, g)
	if g.Status("a") != "" || len(f.rules[fw4.String()]) != 1 {
		t.Fatalf("status=%q rules=%+v", g.Status("a"), f.rules)
	}
	if g.Status("never") != "" {
		t.Fatal("never should be empty")
	}

	// 连续快速切换：以最后一次登记为准
	for i := 0; i < 20; i++ {
		g.Want("b", nil, nil)
		g.Want("b", []Port{{Proto: "tcp", Port: 8443}}, nil)
	}
	waitIdle(t, g)
	got = strings.Join(comments(f, fw4.String()), ",")
	if strings.Count(got, "web:b:") != 1 || !strings.Contains(got, "tcp-8443") {
		t.Fatalf("rules = %s", got)
	}
}

// ClearAll 同步删除全部规则，之后的状态为空。
func TestGroupClearAll(t *testing.T) {
	fw4 := target{Kind: "nft", Family: "inet", Table: "fw4", Chain: "input", Busy: true}
	f := &fakeBackend{chains: []target{fw4}, rules: map[string][]existing{}}
	g := NewGroup("pf")
	g.m = newTestManager(f)
	g.Want("a", []Port{{Proto: "udp", Port: 5000, End: 5010}}, nil)
	g.Want("b", []Port{{Proto: "tcp", Port: 22}}, nil)
	g.ClearAll()
	waitIdle(t, g)
	if n := len(f.rules[fw4.String()]); n != 0 {
		t.Fatalf("rules = %+v", f.rules)
	}
	if g.Status("a") != "" || g.Status("b") != "" {
		t.Fatal("status should be empty")
	}
}

func TestGroupFailed(t *testing.T) {
	ipt := target{Kind: "iptables", Family: "ip", Table: "filter", Chain: "INPUT", Policy: "drop"}
	f := &fakeBackend{chains: []target{ipt}, rules: map[string][]existing{}, addErr: errors.New("没有修改防火墙的权限")}
	g := NewGroup("web")
	g.m = newTestManager(f)
	g.Want("a", []Port{{Proto: "tcp", Port: 443}}, nil)
	waitIdle(t, g)
	if !g.Failed("a") || !strings.Contains(g.Status("a"), "失败") {
		t.Fatalf("failed=%v status=%q", g.Failed("a"), g.Status("a"))
	}
	f.addErr = nil
	g.Want("a", []Port{{Proto: "tcp", Port: 8443}}, nil)
	waitIdle(t, g)
	if g.Failed("a") || g.Failed("never") {
		t.Fatal("should succeed")
	}
}
