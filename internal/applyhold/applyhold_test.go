package applyhold

import (
	"reflect"
	"testing"
)

// reset 把包级状态恢复为未暂缓，避免测试之间互相影响
func reset(t *testing.T) {
	t.Helper()
	Drop()
	t.Cleanup(Drop)
}

// 不在暂缓期间：Defer 返回 false，调用方立即生效，动作不登记
func TestDeferInactive(t *testing.T) {
	reset(t)
	called := false
	if Defer("portforward", "a", func() { called = true }) {
		t.Fatal("未暂缓时 Defer 应返回 false")
	}
	if Active() {
		t.Fatal("未调用 Begin 时不应处于暂缓期间")
	}
	if n := Release(); n != 0 {
		t.Fatalf("没有登记的动作，Release 应返回 0，实际 %d", n)
	}
	if called {
		t.Fatal("未登记的动作不应被执行")
	}
}

// 暂缓期间：按登记顺序执行，同一模块同一 key 只保留第一次登记，不同模块的同名 key 分开登记
func TestReleaseOrderAndDedup(t *testing.T) {
	reset(t)
	Begin()
	if !Active() {
		t.Fatal("Begin 后应处于暂缓期间")
	}
	var order []string
	add := func(module, key, tag string) {
		if !Defer(module, key, func() { order = append(order, tag) }) {
			t.Fatalf("暂缓期间 Defer(%s, %s) 应返回 true", module, key)
		}
	}
	add("webservice", "l1", "web-l1")
	add("portforward", "r1", "pf-r1")
	add("webservice", "l1", "web-l1-again")
	add("protoroute", "l1", "pr-l1")

	want := [][2]string{{"webservice", "l1"}, {"portforward", "r1"}, {"protoroute", "l1"}}
	if got := Pending(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Pending = %v，期望 %v", got, want)
	}
	if n := Release(); n != 3 {
		t.Fatalf("Release 应执行 3 个动作，实际 %d", n)
	}
	if wantOrder := []string{"web-l1", "pf-r1", "pr-l1"}; !reflect.DeepEqual(order, wantOrder) {
		t.Fatalf("执行顺序 = %v，期望 %v", order, wantOrder)
	}
	if Active() || len(Pending()) != 0 {
		t.Fatal("Release 后应结束暂缓并清空登记")
	}
}

// 生效动作执行时暂缓已结束：动作内再次进入同一入口会真正执行，而不是重新登记
func TestReleaseRunsWithoutHold(t *testing.T) {
	reset(t)
	Begin()
	reentered := false
	Defer("webservice", "l1", func() {
		if Defer("webservice", "l1", func() {}) {
			t.Error("生效动作执行时不应再处于暂缓期间")
		}
		reentered = true
	})
	Release()
	if !reentered {
		t.Fatal("登记的动作未执行")
	}
	if len(Pending()) != 0 {
		t.Fatal("动作内的再次进入不应留下登记")
	}
}

// Drop 结束暂缓并丢弃登记的动作，之后的 Release 不再执行它们
func TestDrop(t *testing.T) {
	reset(t)
	Begin()
	called := false
	Defer("portforward", "r1", func() { called = true })
	Drop()
	if Active() {
		t.Fatal("Drop 后应结束暂缓")
	}
	if n := Release(); n != 0 || called {
		t.Fatalf("Drop 后 Release 不应执行任何动作（执行 %d 个，called=%v）", n, called)
	}
}
