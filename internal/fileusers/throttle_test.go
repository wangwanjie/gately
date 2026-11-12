package fileusers

import (
	"fmt"
	"testing"
	"time"
)

// 失败达到上限后拒绝该来源，窗口过后恢复，其他来源不受影响。
func TestThrottle(t *testing.T) {
	now := time.Now()
	th := &Throttle{now: func() time.Time { return now }}
	for i := 0; i < ThrottleMaxFailures; i++ {
		if !th.Reserve("203.0.113.9") {
			t.Fatalf("第 %d 次尝试不应被拒绝", i+1)
		}
		th.Done("203.0.113.9", false)
	}
	if th.Reserve("203.0.113.9") {
		t.Fatal("失败次数达到上限后应拒绝")
	}
	if !th.Reserve("203.0.113.10") {
		t.Fatal("其他来源不应受影响")
	}
	th.Done("203.0.113.10", true)
	now = now.Add(ThrottleWindow + time.Second)
	if !th.Reserve("203.0.113.9") {
		t.Fatal("窗口过后应恢复")
	}
	th.Done("203.0.113.9", true)
}

// 并发占位同样计入上限，成功登录不重置失败计数，IPv6 按 /64 聚合。
func TestThrottleConcurrentAndKeys(t *testing.T) {
	th := &Throttle{}
	held := 0
	for th.Reserve("198.51.100.1") {
		held++
		if held > ThrottleMaxFailures {
			t.Fatal("并发占位超过上限")
		}
	}
	if held != ThrottleMaxFailures {
		t.Fatalf("held=%d", held)
	}
	th2 := &Throttle{}
	for i := 0; i < ThrottleMaxFailures-1; i++ {
		th2.Reserve("198.51.100.2")
		th2.Done("198.51.100.2", false)
	}
	th2.Reserve("198.51.100.2")
	th2.Done("198.51.100.2", true)
	th2.Reserve("198.51.100.2")
	th2.Done("198.51.100.2", false)
	if th2.Reserve("198.51.100.2") {
		t.Fatal("成功登录不应清零失败计数")
	}
	th3 := &Throttle{}
	for i := 0; i < ThrottleMaxFailures; i++ {
		th3.Reserve("2001:db8:1:2::1")
		th3.Done("2001:db8:1:2::1", false)
	}
	if th3.Reserve("2001:db8:1:2:ffff::7") {
		t.Fatal("同一 /64 应共享限流")
	}
}

// 窗口内的来源数量超过上限时淘汰最早的一条，记录表不会无限增长。
func TestThrottleBounded(t *testing.T) {
	now := time.Now()
	th := &Throttle{now: func() time.Time { return now }}
	for i := 0; i < throttleMaxKeys+500; i++ {
		ip := fmt.Sprintf("2001:db8:%x:%x::1", i>>16, i&0xffff)
		now = now.Add(time.Millisecond)
		if !th.Reserve(ip) {
			t.Fatalf("第 %d 个来源不应被拒绝", i)
		}
		th.Done(ip, false)
	}
	if len(th.m) > throttleMaxKeys {
		t.Fatalf("记录数 %d 超过上限 %d", len(th.m), throttleMaxKeys)
	}
	// 最新的来源仍在表中
	if th.m[throttleKey(fmt.Sprintf("2001:db8:%x:%x::1", (throttleMaxKeys+499)>>16, (throttleMaxKeys+499)&0xffff))] == nil {
		t.Fatal("最新的来源应保留")
	}
}
