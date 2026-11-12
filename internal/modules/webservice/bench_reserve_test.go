package webservice

import (
	"fmt"
	"testing"
	"time"
)

// BenchmarkReserveManySources 衡量登录失败表里已有 2 万个近期来源后，每次密码校验占位（reserve + release）的开销。
func BenchmarkReserveManySources(b *testing.B) {
	a := newAuthenticator("secret")
	keys := make([]string, 20000)
	for i := range keys {
		keys[i] = fmt.Sprintf("10.%d.%d.%d", i>>16, i>>8&255, i&255)
		if a.reserve(keys[i]) {
			a.release(keys[i], true)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k := keys[i%len(keys)]
		if a.reserve(k) {
			a.release(k, true)
		}
	}
}

// TestReserveSweepsExpired 验证来源很多时过期的失败记录仍会被清理，进行中的记录保留。
func TestReserveSweepsExpired(t *testing.T) {
	a := newAuthenticator("secret")
	for i := 0; i < 5000; i++ {
		k := fmt.Sprintf("old-%d", i)
		if !a.reserve(k) {
			t.Fatal("首次占位应成功")
		}
		if i != 0 {
			a.release(k, false)
		}
	}
	a.mu.Lock()
	for _, f := range a.fails {
		f.first = f.first.Add(-failureReset - time.Minute)
	}
	a.lastFailSweep = time.Time{}
	a.mu.Unlock()
	if !a.reserve("fresh") {
		t.Fatal("新来源应成功")
	}
	a.mu.Lock()
	n := len(a.fails)
	_, busy := a.fails["old-0"]
	a.mu.Unlock()
	if !busy {
		t.Fatal("有进行中校验的记录不应被清理")
	}
	if n > 5 {
		t.Fatalf("过期记录应被清理, 剩余 %d", n)
	}
}
